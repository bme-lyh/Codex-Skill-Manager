package manager

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
	"github.com/bme-lyh/Codex-Skill-Manager/internal/processutil"
)

const (
	maxAssistedWheels          = 256
	maxAssistedWheelBytes      = int64(128 << 20)
	maxAssistedTotalBytes      = int64(768 << 20)
	maxAssistedWheelEntries    = 20_000
	maxAssistedWheelEntryBytes = int64(256 << 20)
	maxAssistedUnpackedBytes   = int64(2 << 30)
	maxAssistedWheelMetadata   = int64(1 << 20)
	assistedDownloadPollPeriod = 100 * time.Millisecond
)

var (
	pythonPackageName    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	pythonVersion        = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+!-]{0,127}$`)
	pythonEntrypoint     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	pythonNameBoundary   = regexp.MustCompile(`[-_.]+`)
	assistedPythonStepID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
	pipOutputURL         = regexp.MustCompile(`(?i)[a-z][a-z0-9+.-]*://[^\s<>"']+`)
)

type managedPythonTool struct {
	RootPath     string
	PythonPath   string
	EntryPath    string
	Wheelhouse   string
	Package      string
	Version      string
	WheelHashes  map[string]string
	ContainsCode bool
}

type pythonCommand struct {
	Path   string
	Prefix []string
}

type assistedProxyDialer func(
	ctx context.Context,
	network string,
	address string,
) (net.Conn, error)

type assistedPyPIProxy struct {
	listener    net.Listener
	server      *http.Server
	dialContext assistedProxyDialer
	closeOnce   sync.Once
	closeErr    error
	done        chan struct{}
	mu          sync.Mutex
	closed      bool
	connections map[net.Conn]bool
}

type pypiReleaseFile struct {
	Filename      string `json:"filename"`
	PackageType   string `json:"packagetype"`
	PythonVersion string `json:"python_version"`
	URL           string `json:"url"`
	Digests       struct {
		SHA256 string `json:"sha256"`
	} `json:"digests"`
}

type pypiMetadata struct {
	Info struct {
		Name        string            `json:"name"`
		Version     string            `json:"version"`
		HomePage    string            `json:"home_page"`
		ProjectURLs map[string]string `json:"project_urls"`
	} `json:"info"`
	Releases map[string][]pypiReleaseFile `json:"releases"`
	URLs     []pypiReleaseFile            `json:"urls"`
}

func (m *Manager) lockAssistedPythonDependencies(
	ctx context.Context,
	plan model.AssistedInstallPlan,
	progress func(step model.AssistedInstallStep, completed int, total int),
) (model.AssistedInstallPlan, error) {
	total := 0
	for _, step := range plan.Steps {
		if step.Kind == model.AssistedInstallStepManagedPythonTool {
			total++
		}
	}
	completed := 0
	if !strings.EqualFold(plan.Repository.Provider, "github") {
		for index := range plan.Steps {
			if plan.Steps[index].Kind != model.AssistedInstallStepManagedPythonTool {
				continue
			}
			if progress != nil {
				progress(plan.Steps[index], completed, total)
			}
			entrypoint := strings.ToLower(plan.Steps[index].Entrypoint)
			packageSpec := plan.Steps[index].PythonPackage + plan.Steps[index].VersionSpec
			downgradeManagedPythonStep(&plan.Steps[index])
			plan.Warnings = appendBoundedAssistedWarning(plan.Warnings, m.assistedMessage(
				fmt.Sprintf("%s 需要人工安装，因为只有 GitHub 来源才能核验 PyPI 包归属。", packageSpec),
				fmt.Sprintf(
					"%s requires manual installation because PyPI ownership can be verified only for a GitHub source.",
					packageSpec,
				),
			))
			m.downgradeDependentMCPSteps(&plan, entrypoint)
			completed++
		}
		return plan, nil
	}

	failedEntrypoints := map[string]bool{}
	for index := range plan.Steps {
		step := &plan.Steps[index]
		if step.Kind != model.AssistedInstallStepManagedPythonTool {
			continue
		}
		packageName := step.PythonPackage
		version := strings.TrimPrefix(step.VersionSpec, "==")
		entrypoint := strings.ToLower(step.Entrypoint)
		if progress != nil {
			progress(*step, completed, total)
		}
		wheels, err := m.resolveManagedPythonWheelLock(
			ctx,
			plan.ID,
			step.ID,
			plan.Repository.FullName,
			packageName,
			version,
		)
		if err == nil {
			step.PythonWheels = wheels
			completed++
			continue
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return model.AssistedInstallPlan{}, err
		}
		failedEntrypoints[entrypoint] = true
		downgradeManagedPythonStep(step)
		packageSpec := packageName + "==" + version
		plan.Warnings = appendBoundedAssistedWarning(plan.Warnings, m.assistedMessage(
			fmt.Sprintf("%s 需要人工安装：%s", packageSpec, managedPythonLockFailureReason(err, "zh-CN")),
			fmt.Sprintf("%s requires manual installation: %s", packageSpec, managedPythonLockFailureReason(err, "en-US")),
		))
		completed++
	}
	for entrypoint := range failedEntrypoints {
		m.downgradeDependentMCPSteps(&plan, entrypoint)
	}
	return plan, nil
}

func (m *Manager) resolveManagedPythonWheelLock(
	ctx context.Context,
	planID string,
	stepID string,
	repository string,
	packageName string,
	version string,
) ([]model.AssistedPythonWheelLock, error) {
	packageName = strings.TrimSpace(packageName)
	version = strings.TrimSpace(strings.TrimPrefix(version, "=="))
	if !pythonPackageName.MatchString(packageName) || !pythonVersion.MatchString(version) {
		return nil, errors.New("managed Python package metadata is invalid")
	}
	if err := verifyPyPIProject(ctx, packageName, version, repository); err != nil {
		return nil, err
	}
	python, err := resolvePython310(ctx)
	if err != nil {
		return nil, err
	}
	wheelhouse, err := assistedPythonWheelhouse(m.Config.Paths.StagingRoot, planID, stepID)
	if err != nil {
		return nil, err
	}
	if _, statErr := os.Stat(wheelhouse); statErr == nil {
		return nil, errors.New("approval-time Python Wheel directory already exists")
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return nil, statErr
	}
	if err := os.MkdirAll(wheelhouse, 0o700); err != nil {
		return nil, err
	}
	if err := ensureResolvedWithin(m.Config.Paths.StagingRoot, wheelhouse); err != nil {
		return nil, errors.New("approval-time Python Wheel directory escapes the staging root")
	}
	outboundProxy, err := startAssistedPyPIProxy(ctx)
	if err != nil {
		return nil, err
	}
	defer outboundProxy.Close()
	proxyURL := outboundProxy.URL()
	spec := packageName + "==" + version
	downloadArgs := append([]string{}, python.Prefix...)
	downloadArgs = append(downloadArgs,
		"-m", "pip", "--isolated", "--proxy", proxyURL, "download",
		"--disable-pip-version-check", "--no-input", "--no-cache-dir",
		"--progress-bar", "off",
		"--only-binary=:all:", "--index-url", "https://pypi.org/simple",
		"--dest", wheelhouse, spec,
	)
	downloadOutput, err := runAssistedCommandWithEnvironmentMonitor(
		ctx,
		python.Path,
		downloadArgs,
		"",
		nil,
		assistedPyPIDownloadEnvironment(proxyURL, wheelhouse),
		assistedDownloadPollPeriod,
		assistedDownloadResourceMonitor(wheelhouse, outboundProxy.Close),
	)
	if err != nil {
		return nil, fmt.Errorf("resolve complete Wheel dependency closure from official PyPI: %w", err)
	}
	if err := validatePipDownloadOutput(downloadOutput); err != nil {
		return nil, err
	}
	wheels, _, err := inspectWheelhouseLock(wheelhouse)
	if err != nil {
		return nil, err
	}
	if err := validateManagedPythonWheelLocks(wheels, packageName, version); err != nil {
		return nil, err
	}
	if err := verifyOfficialPyPIWheelLocks(ctx, wheels); err != nil {
		return nil, err
	}
	return wheels, nil
}

func downgradeManagedPythonStep(step *model.AssistedInstallStep) {
	step.Kind = model.AssistedInstallStepManual
	step.Required = true
	step.Supported = false
	step.Status = "manual"
	step.SkillNames = nil
	step.PythonPackage = ""
	step.VersionSpec = ""
	step.PythonWheels = nil
	step.Entrypoint = ""
	step.MCPServerName = ""
	step.MCPArgs = nil
	step.PermissionIDs = nil
	step.Reversible = false
}

func (m *Manager) downgradeDependentMCPSteps(plan *model.AssistedInstallPlan, entrypoint string) {
	for index := range plan.Steps {
		step := &plan.Steps[index]
		if step.Kind != model.AssistedInstallStepConfigureCodexMCP ||
			!strings.EqualFold(step.Entrypoint, entrypoint) {
			continue
		}
		stepID := step.ID
		downgradeManagedPythonStep(step)
		plan.Warnings = appendBoundedAssistedWarning(plan.Warnings, m.assistedMessage(
			fmt.Sprintf("MCP 步骤 %s 需要人工配置，因为其 Python 工具未能完成依赖锁定。", stepID),
			fmt.Sprintf(
				"MCP step %s requires manual configuration because its Python tool could not be locked.",
				stepID,
			),
		))
	}
}

func appendBoundedAssistedWarning(values []string, value string) []string {
	if len(values) >= 64 {
		return values
	}
	return append(values, value)
}

func managedPythonLockFailureReason(err error, locale string) string {
	message := strings.ToLower(err.Error())
	english := strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "en")
	reason := [2]string{}
	switch {
	case strings.Contains(message, "does not link back"):
		reason = [2]string{"官方 PyPI 项目没有链接回已批准的 GitHub 仓库", "the official PyPI project does not link back to the approved GitHub repository"}
	case strings.Contains(message, "no wheel"):
		reason = [2]string{"该精确版本没有 Wheel，应用不会自动执行源码构建", "the exact release has no Wheel and source builds are not executed automatically"}
	case strings.Contains(message, "python 3.10"):
		reason = [2]string{"没有可用的 Python 3.10 或更高版本", "Python 3.10 or newer is unavailable"}
	case strings.Contains(message, "official pypi metadata"):
		reason = [2]string{"无法核验官方 PyPI 元数据", "official PyPI metadata could not be verified"}
	case strings.Contains(message, "direct-url"),
		strings.Contains(message, "dependency url"),
		strings.Contains(message, "non-official package source"),
		strings.Contains(message, "official pypi release identity"),
		strings.Contains(message, "not the filename and sha256"):
		reason = [2]string{
			"依赖包含非官方来源或无法与 PyPI 官方文件哈希对应",
			"a dependency uses a non-official source or does not match an official PyPI file hash",
		}
	case strings.Contains(message, "dependency closure"):
		reason = [2]string{"无法从官方 PyPI 解析完整 Wheel 依赖集合", "the complete Wheel dependency set could not be resolved from official PyPI"}
	case strings.Contains(message, "platform"):
		reason = [2]string{"某个 Wheel 面向未知或不兼容的平台", "a Wheel targets an unknown or incompatible platform"}
	case strings.Contains(message, "unsafe path"), strings.Contains(message, "non-regular"),
		strings.Contains(message, "encrypted"), strings.Contains(message, "duplicate path"):
		reason = [2]string{"某个 Wheel 未通过压缩包安全校验", "a Wheel failed archive safety validation"}
	case strings.Contains(message, "limit"), strings.Contains(message, "exceed"):
		reason = [2]string{"依赖集合超过受管安装的安全上限", "the dependency set exceeds the managed-install safety limits"}
	default:
		reason = [2]string{"无法安全核验完整依赖锁", "the complete dependency lock could not be verified safely"}
	}
	if english {
		return reason[1]
	}
	return reason[0]
}

func (m *Manager) installManagedPythonTool(
	ctx context.Context,
	planID, transactionID, stepID, repository, packageName, version, entrypoint string,
	approvedWheels []model.AssistedPythonWheelLock,
	onActivity func(),
) (tool managedPythonTool, err error) {
	packageName = strings.TrimSpace(packageName)
	version = strings.TrimSpace(strings.TrimPrefix(version, "=="))
	entrypoint = strings.TrimSpace(entrypoint)
	if !pythonPackageName.MatchString(packageName) || !pythonVersion.MatchString(version) ||
		!pythonEntrypoint.MatchString(entrypoint) {
		return managedPythonTool{}, errors.New("managed Python package metadata is invalid")
	}
	wheelhouse, err := assistedPythonWheelhouse(m.Config.Paths.StagingRoot, planID, stepID)
	if err != nil {
		return managedPythonTool{}, err
	}
	hashes, containsNative, err := verifyLockedWheelhouse(
		wheelhouse,
		approvedWheels,
		packageName,
		version,
	)
	if err != nil {
		return managedPythonTool{}, fmt.Errorf("verify approved Python Wheel lock: %w", err)
	}
	python, err := resolvePython310(ctx)
	if err != nil {
		return managedPythonTool{}, err
	}
	target := filepath.Join(
		m.Config.Paths.DataRoot, "tools", "python",
		normalizePackagePath(packageName), version+"-"+shortPlanDigest(planID),
	)
	if _, statErr := os.Stat(target); statErr == nil {
		return managedPythonTool{}, errors.New("managed Python environment target already exists; use a new plan or recover the previous transaction")
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return managedPythonTool{}, statErr
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return managedPythonTool{}, err
	}
	if err := ensureResolvedWithin(m.Config.Paths.DataRoot, filepath.Dir(target)); err != nil {
		return managedPythonTool{}, errors.New("managed Python target parent escapes the application data root")
	}
	defer func() {
		if err != nil {
			if _, recoveryErr := moveManagedToolToQuarantine(
				m.Config.Paths.DataRoot,
				target,
				"failed-"+transactionID,
			); recoveryErr != nil {
				err = errors.Join(
					err,
					fmt.Errorf("quarantine incomplete managed Python environment: %w", recoveryErr),
				)
			}
		}
	}()
	venvArgs := append([]string{}, python.Prefix...)
	venvArgs = append(venvArgs, "-m", "venv", target)
	if _, err = runAssistedCommand(ctx, python.Path, venvArgs, "", onActivity); err != nil {
		return managedPythonTool{}, fmt.Errorf("create managed Python environment: %w", err)
	}
	if err = ensureResolvedWithin(m.Config.Paths.DataRoot, target); err != nil {
		return managedPythonTool{}, errors.New("managed Python environment escapes the application data root")
	}
	requirementsPath := filepath.Join(target, "csm-requirements.lock")
	if err = writeHashedRequirements(requirementsPath, approvedWheels); err != nil {
		return managedPythonTool{}, fmt.Errorf("write approved Python dependency lock: %w", err)
	}
	venvPython := filepath.Join(target, "Scripts", "python.exe")
	if runtime.GOOS != "windows" {
		venvPython = filepath.Join(target, "bin", "python")
	}
	installArgs := []string{
		"-m", "pip", "--isolated", "install",
		"--disable-pip-version-check", "--no-input", "--no-index",
		"--find-links", wheelhouse, "--require-hashes",
		"--requirement", requirementsPath,
	}
	if _, err = runAssistedCommand(ctx, venvPython, installArgs, "", onActivity); err != nil {
		return managedPythonTool{}, fmt.Errorf("install approved Python Wheels: %w", err)
	}
	entryPath := filepath.Join(target, "Scripts", entrypoint+".exe")
	if runtime.GOOS != "windows" {
		entryPath = filepath.Join(target, "bin", entrypoint)
	}
	entryInfo, statErr := os.Lstat(entryPath)
	if statErr != nil || entryInfo.IsDir() || entryInfo.Mode()&os.ModeSymlink != 0 {
		return managedPythonTool{}, fmt.Errorf("managed Python entrypoint %q was not installed", entrypoint)
	}
	tool = managedPythonTool{
		RootPath: target, PythonPath: venvPython, EntryPath: entryPath,
		Wheelhouse: wheelhouse, Package: packageName, Version: version,
		WheelHashes: hashes, ContainsCode: containsNative,
	}
	if err = writeJSONAtomic(filepath.Join(target, "csm-tool-manifest.json"), map[string]any{
		"schemaVersion":      2,
		"planId":             planID,
		"transactionId":      transactionID,
		"stepId":             stepID,
		"repository":         repository,
		"package":            packageName,
		"version":            version,
		"entrypoint":         entrypoint,
		"wheelLock":          approvedWheels,
		"wheelHashes":        hashes,
		"containsNativeCode": containsNative,
		"installedAt":        time.Now().UTC(),
	}); err != nil {
		return managedPythonTool{}, fmt.Errorf("write managed Python manifest: %w", err)
	}
	return tool, nil
}

func resolvePython310(ctx context.Context) (pythonCommand, error) {
	candidates := []pythonCommand{}
	if path, err := exec.LookPath("py.exe"); err == nil {
		candidates = append(candidates, pythonCommand{Path: path, Prefix: []string{"-3"}})
	}
	for _, name := range []string{"python.exe", "python3.exe", "python3", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			candidates = append(candidates, pythonCommand{Path: path})
		}
	}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		key := strings.ToLower(candidate.Path + "\x00" + strings.Join(candidate.Prefix, "\x00"))
		if seen[key] {
			continue
		}
		seen[key] = true
		args := append([]string{}, candidate.Prefix...)
		args = append(args, "-c", "import sys; print('%d.%d' % sys.version_info[:2])")
		output, err := runAssistedCommand(ctx, candidate.Path, args, "", nil)
		if err != nil {
			continue
		}
		parts := strings.Split(strings.TrimSpace(output), ".")
		if len(parts) != 2 {
			continue
		}
		major, _ := strconv.Atoi(parts[0])
		minor, _ := strconv.Atoi(parts[1])
		if major > 3 || (major == 3 && minor >= 10) {
			return candidate, nil
		}
	}
	return pythonCommand{}, errors.New("Python 3.10 or newer is required for the managed MCP tool")
}

func validatePipDownloadOutput(output string) error {
	for _, raw := range pipOutputURL.FindAllString(output, -1) {
		raw = strings.TrimRight(raw, ".,);]")
		parsed, err := url.Parse(raw)
		if err != nil || !strings.EqualFold(parsed.Scheme, "https") ||
			parsed.User != nil {
			return errors.New("pip download reported a non-official package source")
		}
		switch strings.ToLower(parsed.Hostname()) {
		case "pypi.org", "files.pythonhosted.org":
			continue
		default:
			return errors.New("pip download reported a non-official package source")
		}
	}
	return nil
}

func startAssistedPyPIProxy(ctx context.Context) (*assistedPyPIProxy, error) {
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	return startAssistedPyPIProxyWithDialer(ctx, dialer.DialContext)
}

func startAssistedPyPIProxyWithDialer(
	ctx context.Context,
	dialContext assistedProxyDialer,
) (*assistedPyPIProxy, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("start local PyPI allowlist proxy: %w", err)
	}
	proxy := &assistedPyPIProxy{
		listener:    listener,
		dialContext: dialContext,
		done:        make(chan struct{}),
		connections: map[net.Conn]bool{},
	}
	proxy.server = &http.Server{
		Handler:           proxy,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    8 << 10,
	}
	go func() {
		err := proxy.server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) &&
			!errors.Is(err, net.ErrClosed) {
			_ = proxy.Close()
		}
	}()
	go func() {
		select {
		case <-ctx.Done():
			_ = proxy.Close()
		case <-proxy.done:
		}
	}()
	return proxy, nil
}

func (proxy *assistedPyPIProxy) URL() string {
	return "http://" + proxy.listener.Addr().String()
}

func (proxy *assistedPyPIProxy) Close() error {
	proxy.closeOnce.Do(func() {
		proxy.mu.Lock()
		proxy.closed = true
		connections := make([]net.Conn, 0, len(proxy.connections))
		for connection := range proxy.connections {
			connections = append(connections, connection)
		}
		proxy.connections = nil
		proxy.mu.Unlock()

		listenerErr := proxy.listener.Close()
		serverErr := proxy.server.Close()
		for _, connection := range connections {
			_ = connection.Close()
		}
		if listenerErr != nil && !errors.Is(listenerErr, net.ErrClosed) {
			proxy.closeErr = listenerErr
		}
		if serverErr != nil && !errors.Is(serverErr, http.ErrServerClosed) &&
			!errors.Is(serverErr, net.ErrClosed) {
			proxy.closeErr = errors.Join(proxy.closeErr, serverErr)
		}
		close(proxy.done)
	})
	return proxy.closeErr
}

func (proxy *assistedPyPIProxy) ServeHTTP(
	response http.ResponseWriter,
	request *http.Request,
) {
	target, err := assistedPyPIConnectTarget(request)
	if err != nil {
		response.Header().Set("Connection", "close")
		http.Error(response, "PyPI proxy permits only approved HTTPS tunnels", http.StatusForbidden)
		return
	}
	upstream, err := proxy.dialContext(request.Context(), "tcp", target)
	if err != nil {
		http.Error(response, "PyPI proxy could not reach the approved host", http.StatusBadGateway)
		return
	}
	hijacker, ok := response.(http.Hijacker)
	if !ok {
		_ = upstream.Close()
		http.Error(response, "PyPI proxy tunnel is unavailable", http.StatusInternalServerError)
		return
	}
	downstream, buffered, err := hijacker.Hijack()
	if err != nil {
		_ = upstream.Close()
		return
	}
	if !proxy.trackConnections(downstream, upstream) {
		_ = downstream.Close()
		_ = upstream.Close()
		return
	}
	defer proxy.untrackConnections(downstream, upstream)
	if _, err := io.WriteString(
		buffered,
		"HTTP/1.1 200 Connection Established\r\n\r\n",
	); err != nil {
		_ = downstream.Close()
		_ = upstream.Close()
		return
	}
	if err := buffered.Flush(); err != nil {
		_ = downstream.Close()
		_ = upstream.Close()
		return
	}
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(upstream, buffered)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(downstream, upstream)
		done <- struct{}{}
	}()
	<-done
	_ = downstream.Close()
	_ = upstream.Close()
	<-done
}

func (proxy *assistedPyPIProxy) trackConnections(connections ...net.Conn) bool {
	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	if proxy.closed {
		return false
	}
	for _, connection := range connections {
		proxy.connections[connection] = true
	}
	return true
}

func (proxy *assistedPyPIProxy) untrackConnections(connections ...net.Conn) {
	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	for _, connection := range connections {
		delete(proxy.connections, connection)
	}
}

func assistedPyPIConnectTarget(request *http.Request) (string, error) {
	if request.Method != http.MethodConnect ||
		request.URL == nil ||
		request.URL.Scheme != "" ||
		request.URL.User != nil ||
		request.RequestURI != request.Host {
		return "", errors.New("plaintext or malformed proxy request")
	}
	authority := strings.TrimSpace(request.Host)
	if authority != request.Host || strings.ContainsAny(authority, `@/\?#`) {
		return "", errors.New("proxy target authority is invalid")
	}
	host, port, err := net.SplitHostPort(authority)
	if err != nil || port != "443" {
		return "", errors.New("proxy target must use HTTPS port 443")
	}
	switch strings.ToLower(host) {
	case "pypi.org", "files.pythonhosted.org":
		return net.JoinHostPort(strings.ToLower(host), "443"), nil
	default:
		return "", errors.New("proxy target host is not allowlisted")
	}
}

func verifyPyPIProject(ctx context.Context, packageName, version, repository string) error {
	metadata, err := fetchOfficialPyPIReleaseMetadata(
		ctx,
		officialPyPIClient(),
		packageName,
		version,
	)
	if err != nil {
		return err
	}
	return validatePyPIProject(metadata, packageName, version, repository)
}

func verifyOfficialPyPIWheelLocks(
	ctx context.Context,
	wheels []model.AssistedPythonWheelLock,
) error {
	client := officialPyPIClient()
	metadataByRelease := make(map[string]pypiMetadata, len(wheels))
	for _, wheel := range wheels {
		key := canonicalPythonPackageName(wheel.Name) + "\x00" + wheel.Version
		metadata, ok := metadataByRelease[key]
		if !ok {
			var err error
			metadata, err = fetchOfficialPyPIReleaseMetadata(
				ctx,
				client,
				wheel.Name,
				wheel.Version,
			)
			if err != nil {
				return fmt.Errorf(
					"verify official PyPI origin for %s==%s: %w",
					wheel.Name,
					wheel.Version,
					err,
				)
			}
			metadataByRelease[key] = metadata
		}
		if err := validateOfficialPyPIWheel(metadata, wheel); err != nil {
			return err
		}
	}
	return nil
}

func officialPyPIClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 3 || !strings.EqualFold(request.URL.Scheme, "https") ||
				!strings.EqualFold(request.URL.Hostname(), "pypi.org") ||
				request.URL.User != nil {
				return errors.New("official PyPI metadata redirected outside pypi.org")
			}
			return nil
		},
	}
}

func fetchOfficialPyPIReleaseMetadata(
	ctx context.Context,
	client *http.Client,
	packageName string,
	version string,
) (pypiMetadata, error) {
	endpoint := "https://pypi.org/pypi/" + url.PathEscape(packageName) +
		"/" + url.PathEscape(version) + "/json"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return pypiMetadata{}, err
	}
	request.Header.Set("User-Agent", "Codex-Skill-Manager/"+model.Version)
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return pypiMetadata{}, fmt.Errorf("query official PyPI metadata: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return pypiMetadata{}, fmt.Errorf("official PyPI metadata returned %s", response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil {
		return pypiMetadata{}, err
	}
	if len(data) > 4<<20 {
		return pypiMetadata{}, errors.New("official PyPI metadata exceeds the response limit")
	}
	var metadata pypiMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return pypiMetadata{}, fmt.Errorf("parse official PyPI metadata: %w", err)
	}
	return metadata, nil
}

func validatePyPIProject(metadata pypiMetadata, packageName, version, repository string) error {
	if canonicalPythonPackageName(metadata.Info.Name) != canonicalPythonPackageName(packageName) {
		return errors.New("PyPI project identity does not match the approved package name")
	}
	files := pypiReleaseFiles(metadata, version)
	hasWheel := false
	for _, file := range files {
		if file.PackageType == "bdist_wheel" || strings.HasSuffix(strings.ToLower(file.Filename), ".whl") {
			hasWheel = true
			break
		}
	}
	if !hasWheel {
		return fmt.Errorf("PyPI release %s has no wheel; source builds are never executed automatically", version)
	}
	repository = strings.ToLower(strings.Trim(strings.TrimSpace(repository), "/"))
	repositoryParts := strings.Split(repository, "/")
	if len(repositoryParts) != 2 || repositoryParts[0] == "" || repositoryParts[1] == "" {
		return errors.New("GitHub repository identity is missing from the installation plan")
	}
	urls := []string{metadata.Info.HomePage}
	for _, value := range metadata.Info.ProjectURLs {
		urls = append(urls, value)
	}
	for _, value := range urls {
		parsed, err := url.Parse(strings.TrimSpace(value))
		if err != nil || !strings.EqualFold(parsed.Scheme, "https") ||
			!strings.EqualFold(parsed.Hostname(), "github.com") {
			continue
		}
		repositoryPath := strings.Trim(parsed.Path, "/")
		repositoryPath = strings.TrimSuffix(repositoryPath, ".git")
		parts := strings.Split(repositoryPath, "/")
		if len(parts) >= 2 &&
			strings.EqualFold(parts[0], repositoryParts[0]) &&
			strings.EqualFold(parts[1], repositoryParts[1]) {
			return nil
		}
	}
	return errors.New("PyPI project does not link back to the approved GitHub repository")
}

func validateOfficialPyPIWheel(
	metadata pypiMetadata,
	wheel model.AssistedPythonWheelLock,
) error {
	if canonicalPythonPackageName(metadata.Info.Name) !=
		canonicalPythonPackageName(wheel.Name) ||
		metadata.Info.Version != wheel.Version {
		return fmt.Errorf(
			"official PyPI release identity does not match %s==%s",
			wheel.Name,
			wheel.Version,
		)
	}
	for _, file := range pypiReleaseFiles(metadata, wheel.Version) {
		if file.Filename != wheel.Filename ||
			file.PackageType != "bdist_wheel" ||
			!strings.EqualFold(strings.TrimSpace(file.Digests.SHA256), wheel.SHA256) ||
			!officialPyPIFileURL(file.URL, wheel.Filename) {
			continue
		}
		return nil
	}
	return fmt.Errorf(
		"Python Wheel %s is not the filename and SHA256 published by official PyPI",
		wheel.Filename,
	)
}

func pypiReleaseFiles(metadata pypiMetadata, version string) []pypiReleaseFile {
	if metadata.Info.Version == version && len(metadata.URLs) != 0 {
		return metadata.URLs
	}
	return metadata.Releases[version]
}

func officialPyPIFileURL(raw string, filename string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") ||
		!strings.EqualFold(parsed.Hostname(), "files.pythonhosted.org") ||
		parsed.User != nil {
		return false
	}
	base, err := url.PathUnescape(pathpkg.Base(parsed.Path))
	return err == nil && base == filename
}

func assistedPythonWheelhouse(stagingRoot, planID, stepID string) (string, error) {
	if !validAssistedReferenceID(planID) || !assistedPythonStepID.MatchString(stepID) {
		return "", errors.New("managed Python plan or step identity is invalid")
	}
	wheelhouse := filepath.Join(stagingRoot, planID, "python-locks", stepID)
	if err := ensureWithinOrEqual(stagingRoot, wheelhouse); err != nil {
		return "", errors.New("managed Python Wheel directory is outside the staging root")
	}
	if _, err := os.Lstat(wheelhouse); err == nil {
		if err := ensureResolvedWithin(stagingRoot, wheelhouse); err != nil {
			return "", errors.New("managed Python Wheel directory escapes the staging root")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return wheelhouse, nil
}

func inspectWheelhouseLock(
	root string,
) ([]model.AssistedPythonWheelLock, bool, error) {
	hashes, _, err := inspectWheelhouse(root)
	if err != nil {
		return nil, false, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, false, err
	}
	wheels := make([]model.AssistedPythonWheelLock, 0, len(entries))
	containsNative := false
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		name, version, tags, nativeByTag, err := inspectWheelIdentity(path)
		if err != nil {
			return nil, false, err
		}
		nativeByFile, _, err := inspectWheelArchive(path)
		if err != nil {
			return nil, false, err
		}
		if nativeByFile && !nativeByTag {
			return nil, false, fmt.Errorf(
				"Python Wheel declares a pure platform tag but contains native code: %s",
				entry.Name(),
			)
		}
		if nativeByTag {
			if err := validateNativeWheelPlatform(tags); err != nil {
				return nil, false, fmt.Errorf("%s: %w", entry.Name(), err)
			}
		}
		wheels = append(wheels, model.AssistedPythonWheelLock{
			Name: name, Version: version, Filename: entry.Name(),
			SHA256: hashes[entry.Name()], Native: nativeByTag, Tags: tags,
		})
		containsNative = containsNative || nativeByTag
	}
	sort.Slice(wheels, func(i, j int) bool {
		left := canonicalPythonPackageName(wheels[i].Name)
		right := canonicalPythonPackageName(wheels[j].Name)
		if left == right {
			return wheels[i].Filename < wheels[j].Filename
		}
		return left < right
	})
	return wheels, containsNative, nil
}

func inspectWheelIdentity(path string) (string, string, []string, bool, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return "", "", nil, false, fmt.Errorf("inspect Wheel identity %s: %w", filepath.Base(path), err)
	}
	defer reader.Close()
	var metadataFile *zip.File
	var wheelFile *zip.File
	var distInfoRoot string
	for _, file := range reader.File {
		clean := strings.ReplaceAll(pathpkg.Clean(strings.ReplaceAll(file.Name, `\`, "/")), `\`, "/")
		lower := strings.ToLower(clean)
		switch {
		case strings.HasSuffix(lower, ".dist-info/direct_url.json") &&
			strings.Count(clean, "/") == 1:
			return "", "", nil, false, fmt.Errorf(
				"Python Wheel contains Direct-URL origin metadata: %s",
				filepath.Base(path),
			)
		case strings.HasSuffix(lower, ".dist-info/metadata") && strings.Count(clean, "/") == 1:
			if metadataFile != nil {
				return "", "", nil, false, fmt.Errorf(
					"Python Wheel contains multiple METADATA files: %s",
					filepath.Base(path),
				)
			}
			metadataFile = file
			distInfoRoot = strings.TrimSuffix(lower, "/metadata")
		case strings.HasSuffix(lower, ".dist-info/wheel") && strings.Count(clean, "/") == 1:
			if wheelFile != nil {
				return "", "", nil, false, fmt.Errorf(
					"Python Wheel contains multiple WHEEL files: %s",
					filepath.Base(path),
				)
			}
			wheelFile = file
		}
	}
	if metadataFile == nil || wheelFile == nil {
		return "", "", nil, false, fmt.Errorf(
			"Python Wheel is missing METADATA or WHEEL identity: %s",
			filepath.Base(path),
		)
	}
	wheelRoot := strings.TrimSuffix(
		strings.ToLower(strings.ReplaceAll(pathpkg.Clean(wheelFile.Name), `\`, "/")),
		"/wheel",
	)
	if wheelRoot != distInfoRoot {
		return "", "", nil, false, fmt.Errorf(
			"Python Wheel identity files disagree: %s",
			filepath.Base(path),
		)
	}
	metadata, err := readWheelHeaders(metadataFile)
	if err != nil {
		return "", "", nil, false, err
	}
	wheelHeaders, err := readWheelHeaders(wheelFile)
	if err != nil {
		return "", "", nil, false, err
	}
	name := strings.TrimSpace(metadata.Get("Name"))
	version := strings.TrimSpace(metadata.Get("Version"))
	if !pythonPackageName.MatchString(name) || !pythonVersion.MatchString(version) {
		return "", "", nil, false, fmt.Errorf(
			"Python Wheel has invalid Name or Version metadata: %s",
			filepath.Base(path),
		)
	}
	for _, requirement := range metadata.Values("Requires-Dist") {
		requirement = strings.TrimSpace(requirement)
		if strings.Contains(requirement, "@") || strings.Contains(requirement, "://") ||
			strings.Contains(strings.ToLower(requirement), "file:") {
			return "", "", nil, false, fmt.Errorf(
				"Python Wheel uses a direct-URL dependency instead of official PyPI: %s",
				filepath.Base(path),
			)
		}
	}
	for _, field := range []string{"Direct-URL", "Dependency-Link", "Dependency-Links"} {
		for _, dependencyLink := range metadata.Values(field) {
			if strings.TrimSpace(dependencyLink) != "" {
				return "", "", nil, false, fmt.Errorf(
					"Python Wheel uses direct or legacy dependency URLs instead of official PyPI: %s",
					filepath.Base(path),
				)
			}
		}
	}
	tags := append([]string(nil), wheelHeaders.Values("Tag")...)
	for index := range tags {
		tags[index] = strings.TrimSpace(tags[index])
	}
	sort.Strings(tags)
	if len(tags) == 0 {
		return "", "", nil, false, fmt.Errorf(
			"Python Wheel has no compatibility tag: %s",
			filepath.Base(path),
		)
	}
	native := false
	for _, tag := range tags {
		parts := strings.Split(tag, "-")
		if len(parts) != 3 {
			return "", "", nil, false, fmt.Errorf(
				"Python Wheel has an unknown compatibility tag %q",
				tag,
			)
		}
		if !strings.EqualFold(parts[1], "none") || !strings.EqualFold(parts[2], "any") {
			native = true
		}
	}
	purelib := strings.EqualFold(strings.TrimSpace(wheelHeaders.Get("Root-Is-Purelib")), "true")
	if !native && !purelib {
		return "", "", nil, false, fmt.Errorf(
			"Python Wheel has an inconsistent purelib declaration: %s",
			filepath.Base(path),
		)
	}
	return name, version, tags, native, nil
}

func readWheelHeaders(file *zip.File) (textproto.MIMEHeader, error) {
	if file.FileInfo().Mode()&os.ModeSymlink != 0 ||
		file.UncompressedSize64 == 0 ||
		file.UncompressedSize64 > uint64(maxAssistedWheelMetadata) {
		return nil, fmt.Errorf("Python Wheel metadata size or type is invalid: %s", file.Name)
	}
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, maxAssistedWheelMetadata+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxAssistedWheelMetadata {
		return nil, fmt.Errorf("Python Wheel metadata exceeds the limit: %s", file.Name)
	}
	headers, err := textproto.NewReader(bufio.NewReader(bytes.NewReader(data))).ReadMIMEHeader()
	if err != nil {
		return nil, fmt.Errorf("parse Python Wheel metadata %s: %w", file.Name, err)
	}
	return headers, nil
}

func validateNativeWheelPlatform(tags []string) error {
	for _, tag := range tags {
		parts := strings.Split(tag, "-")
		if len(parts) != 3 {
			return fmt.Errorf("unknown native Wheel tag %q", tag)
		}
		platform := strings.ToLower(parts[2])
		if platform == "any" || nativeWheelPlatformMatches(platform) {
			continue
		}
		return fmt.Errorf(
			"native Wheel platform %q does not match %s/%s",
			platform,
			runtime.GOOS,
			runtime.GOARCH,
		)
	}
	return nil
}

func nativeWheelPlatformMatches(platform string) bool {
	switch runtime.GOOS {
	case "windows":
		switch runtime.GOARCH {
		case "amd64":
			return platform == "win_amd64"
		case "arm64":
			return platform == "win_arm64"
		case "386":
			return platform == "win32"
		}
	case "linux":
		architecture := map[string]string{
			"amd64": "x86_64",
			"arm64": "aarch64",
			"386":   "i686",
		}[runtime.GOARCH]
		return architecture != "" &&
			(strings.HasPrefix(platform, "manylinux") ||
				strings.HasPrefix(platform, "musllinux") ||
				strings.HasPrefix(platform, "linux_")) &&
			strings.HasSuffix(platform, "_"+architecture)
	case "darwin":
		architecture := map[string]string{"amd64": "x86_64", "arm64": "arm64"}[runtime.GOARCH]
		return architecture != "" && strings.HasPrefix(platform, "macosx_") &&
			strings.HasSuffix(platform, "_"+architecture)
	}
	return false
}

func validateManagedPythonWheelLocks(
	wheels []model.AssistedPythonWheelLock,
	rootPackage string,
	rootVersion string,
) error {
	if len(wheels) == 0 || len(wheels) > maxAssistedWheels {
		return fmt.Errorf("Python Wheel lock count is outside the allowed range: %d", len(wheels))
	}
	rootName := canonicalPythonPackageName(rootPackage)
	seenNames := make(map[string]bool, len(wheels))
	seenFiles := make(map[string]bool, len(wheels))
	rootCount := 0
	for _, wheel := range wheels {
		if !pythonPackageName.MatchString(wheel.Name) || !pythonVersion.MatchString(wheel.Version) ||
			filepath.Base(wheel.Filename) != wheel.Filename ||
			strings.ContainsAny(wheel.Filename, `/\:`) ||
			!strings.HasSuffix(strings.ToLower(wheel.Filename), ".whl") ||
			len(wheel.SHA256) != sha256.Size*2 {
			return fmt.Errorf("Python Wheel lock identity is invalid for %q", wheel.Filename)
		}
		if _, err := hex.DecodeString(wheel.SHA256); err != nil ||
			wheel.SHA256 != strings.ToLower(wheel.SHA256) {
			return fmt.Errorf("Python Wheel lock SHA256 is invalid for %s", wheel.Filename)
		}
		if len(wheel.Tags) == 0 || len(wheel.Tags) > 64 {
			return fmt.Errorf("Python Wheel lock tags are invalid for %s", wheel.Filename)
		}
		nativeByTag := false
		seenTags := map[string]bool{}
		for _, tag := range wheel.Tags {
			parts := strings.Split(tag, "-")
			if len(parts) != 3 || seenTags[tag] {
				return fmt.Errorf("Python Wheel lock tag is invalid for %s", wheel.Filename)
			}
			seenTags[tag] = true
			if !strings.EqualFold(parts[1], "none") || !strings.EqualFold(parts[2], "any") {
				nativeByTag = true
			}
		}
		if nativeByTag != wheel.Native {
			return fmt.Errorf("Python Wheel native classification is invalid for %s", wheel.Filename)
		}
		if wheel.Native {
			if err := validateNativeWheelPlatform(wheel.Tags); err != nil {
				return err
			}
		}
		nameKey := canonicalPythonPackageName(wheel.Name)
		fileKey := strings.ToLower(wheel.Filename)
		if seenNames[nameKey] || seenFiles[fileKey] {
			return errors.New("Python Wheel lock contains duplicate package or file identities")
		}
		seenNames[nameKey] = true
		seenFiles[fileKey] = true
		if nameKey == rootName && wheel.Version == rootVersion {
			rootCount++
		}
	}
	if rootCount != 1 {
		return errors.New("Python Wheel lock does not contain exactly one root package")
	}
	return nil
}

func verifyLockedWheelhouse(
	root string,
	approved []model.AssistedPythonWheelLock,
	rootPackage string,
	rootVersion string,
) (map[string]string, bool, error) {
	approved = clonePythonWheelLocks(approved)
	if err := validateManagedPythonWheelLocks(approved, rootPackage, rootVersion); err != nil {
		return nil, false, err
	}
	actual, containsNative, err := inspectWheelhouseLock(root)
	if err != nil {
		return nil, false, err
	}
	if err := validateManagedPythonWheelLocks(actual, rootPackage, rootVersion); err != nil {
		return nil, false, err
	}
	if len(actual) != len(approved) {
		return nil, false, errors.New("cached Python Wheel set differs from the approved dependency lock")
	}
	expected := make(map[string]model.AssistedPythonWheelLock, len(approved))
	for _, wheel := range approved {
		expected[wheel.Filename] = wheel
	}
	hashes := make(map[string]string, len(actual))
	for _, wheel := range actual {
		locked, ok := expected[wheel.Filename]
		if !ok || locked.Name != wheel.Name || locked.Version != wheel.Version ||
			locked.SHA256 != wheel.SHA256 || locked.Native != wheel.Native ||
			!sameStrings(locked.Tags, wheel.Tags) {
			return nil, false, fmt.Errorf(
				"cached Python Wheel does not match the approved lock: %s",
				wheel.Filename,
			)
		}
		hashes[wheel.Filename] = wheel.SHA256
	}
	return hashes, containsNative, nil
}

func writeHashedRequirements(path string, wheels []model.AssistedPythonWheelLock) error {
	wheels = clonePythonWheelLocks(wheels)
	sort.Slice(wheels, func(i, j int) bool {
		left := canonicalPythonPackageName(wheels[i].Name)
		right := canonicalPythonPackageName(wheels[j].Name)
		if left == right {
			return wheels[i].Version < wheels[j].Version
		}
		return left < right
	})
	var content strings.Builder
	content.WriteString("# Generated from the user-approved Codex Skill Manager plan.\n")
	for _, wheel := range wheels {
		if !pythonPackageName.MatchString(wheel.Name) || !pythonVersion.MatchString(wheel.Version) ||
			len(wheel.SHA256) != sha256.Size*2 {
			return errors.New("approved Python dependency lock is invalid")
		}
		fmt.Fprintf(
			&content,
			"%s==%s --hash=sha256:%s\n",
			wheel.Name,
			wheel.Version,
			wheel.SHA256,
		)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(file, content.String()); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func clonePythonWheelLocks(
	values []model.AssistedPythonWheelLock,
) []model.AssistedPythonWheelLock {
	out := append([]model.AssistedPythonWheelLock(nil), values...)
	for index := range out {
		out[index].Tags = append([]string(nil), out[index].Tags...)
		sort.Strings(out[index].Tags)
	}
	return out
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	left = append([]string(nil), left...)
	right = append([]string(nil), right...)
	sort.Strings(left)
	sort.Strings(right)
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func validateAssistedDownloadResourceUsage(root string) error {
	root = filepath.Clean(root)
	var totalBytes int64
	entryCount := 0
	topLevelFiles := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if path != root && errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if path == root {
			return nil
		}
		entryCount++
		if entryCount > maxAssistedWheelEntries {
			return fmt.Errorf(
				"pip download exceeded the runtime directory entry limit of %d",
				maxAssistedWheelEntries,
			)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("pip download created a symbolic link: %s", entry.Name())
		}
		info, err := entry.Info()
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("pip download created a non-regular file: %s", entry.Name())
		}
		if filepath.Dir(path) == root {
			topLevelFiles++
			if topLevelFiles > maxAssistedWheels {
				return fmt.Errorf(
					"pip download exceeded the Wheel file count limit of %d",
					maxAssistedWheels,
				)
			}
		}
		size := info.Size()
		if size < 0 || size > maxAssistedWheelBytes {
			return fmt.Errorf(
				"pip download file exceeds the per-file limit of %d MiB: %s",
				maxAssistedWheelBytes>>20,
				entry.Name(),
			)
		}
		if size > maxAssistedTotalBytes-totalBytes {
			return fmt.Errorf(
				"pip download exceeded the total runtime limit of %d MiB",
				maxAssistedTotalBytes>>20,
			)
		}
		totalBytes += size
		return nil
	})
	if err != nil {
		return fmt.Errorf("monitor pip download resources: %w", err)
	}
	return nil
}

func assistedDownloadResourceMonitor(
	root string,
	closeNetwork func() error,
) func() error {
	return func() error {
		err := validateAssistedDownloadResourceUsage(root)
		if err != nil && closeNetwork != nil {
			_ = closeNetwork()
		}
		return err
	}
}

func inspectWheelhouse(root string) (map[string]string, bool, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, false, err
	}
	if len(entries) == 0 || len(entries) > maxAssistedWheels {
		return nil, false, fmt.Errorf("Python dependency wheel count is outside the allowed range: %d", len(entries))
	}
	hashes := make(map[string]string, len(entries))
	var total int64
	var unpackedTotal int64
	containsNative := false
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 ||
			!strings.HasSuffix(strings.ToLower(entry.Name()), ".whl") {
			return nil, false, fmt.Errorf("Python dependency is not a wheel: %s", entry.Name())
		}
		path := filepath.Join(root, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return nil, false, err
		}
		if info.Size() < 1 || info.Size() > maxAssistedWheelBytes {
			return nil, false, fmt.Errorf("Python wheel exceeds the per-file limit: %s", entry.Name())
		}
		total += info.Size()
		if total > maxAssistedTotalBytes {
			return nil, false, errors.New("Python wheels exceed the total download limit")
		}
		file, err := os.Open(path)
		if err != nil {
			return nil, false, err
		}
		hasher := sha256.New()
		bytesRead, hashErr := io.Copy(hasher, io.LimitReader(file, maxAssistedWheelBytes+1))
		closeErr := file.Close()
		if hashErr != nil {
			return nil, false, hashErr
		}
		if closeErr != nil {
			return nil, false, closeErr
		}
		if bytesRead != info.Size() || bytesRead > maxAssistedWheelBytes {
			return nil, false, fmt.Errorf("Python Wheel changed during inspection: %s", entry.Name())
		}
		hashes[entry.Name()] = hex.EncodeToString(hasher.Sum(nil))
		native, unpacked, err := inspectWheelArchive(path)
		if err != nil {
			return nil, false, err
		}
		if unpackedTotal > maxAssistedUnpackedBytes-unpacked {
			return nil, false, errors.New("Python wheels exceed the total unpacked size limit")
		}
		unpackedTotal += unpacked
		containsNative = containsNative || native
	}
	return hashes, containsNative, nil
}

func inspectWheelArchive(path string) (bool, int64, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return false, 0, fmt.Errorf("inspect wheel %s: %w", filepath.Base(path), err)
	}
	defer reader.Close()
	if len(reader.File) == 0 || len(reader.File) > maxAssistedWheelEntries {
		return false, 0, fmt.Errorf(
			"Python wheel entry count is outside the allowed range: %s",
			filepath.Base(path),
		)
	}
	var unpacked int64
	containsNative := false
	seenPaths := map[string]bool{}
	for _, file := range reader.File {
		name := strings.ReplaceAll(file.Name, `\`, "/")
		clean := pathpkg.Clean(name)
		if clean == "." || pathpkg.IsAbs(clean) || clean == ".." ||
			strings.HasPrefix(clean, "../") || unsafeWindowsArchivePath(clean) {
			return false, 0, fmt.Errorf("Python wheel contains an unsafe path: %s", file.Name)
		}
		pathKey := strings.ToLower(clean)
		if seenPaths[pathKey] {
			return false, 0, fmt.Errorf("Python wheel contains a duplicate path: %s", file.Name)
		}
		seenPaths[pathKey] = true
		mode := file.Mode()
		if mode&os.ModeSymlink != 0 ||
			(!mode.IsRegular() && !mode.IsDir()) {
			return false, 0, fmt.Errorf("Python wheel contains a non-regular entry: %s", file.Name)
		}
		if file.Flags&0x1 != 0 {
			return false, 0, fmt.Errorf("encrypted Python wheel entries are not supported: %s", file.Name)
		}
		if file.UncompressedSize64 > uint64(maxAssistedWheelEntryBytes) {
			return false, 0, fmt.Errorf("Python wheel entry exceeds the unpacked limit: %s", file.Name)
		}
		size := int64(file.UncompressedSize64)
		if unpacked > maxAssistedUnpackedBytes-size {
			return false, 0, fmt.Errorf(
				"Python wheel exceeds the total unpacked limit: %s",
				filepath.Base(path),
			)
		}
		unpacked += size
		native, err := wheelEntryContainsNative(file)
		if err != nil {
			return false, 0, fmt.Errorf(
				"inspect Python Wheel entry %s: %w",
				file.Name,
				err,
			)
		}
		containsNative = containsNative || native
	}
	return containsNative, unpacked, nil
}

func wheelEntryContainsNative(file *zip.File) (bool, error) {
	if file.FileInfo().IsDir() {
		return false, nil
	}
	lower := strings.ToLower(file.Name)
	switch strings.ToLower(pathpkg.Ext(lower)) {
	case ".pyd", ".dll", ".so", ".dylib",
		".exe", ".node", ".sys", ".ocx", ".drv", ".efi", ".com", ".scr", ".cpl",
		".a", ".lib", ".o", ".obj", ".ko":
		return true, nil
	}
	if strings.Contains(pathpkg.Base(lower), ".so.") {
		return true, nil
	}
	if file.UncompressedSize64 < 2 {
		return false, nil
	}
	reader, err := file.Open()
	if err != nil {
		return false, err
	}
	var header [8]byte
	read, readErr := io.ReadFull(reader, header[:])
	closeErr := reader.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) &&
		!errors.Is(readErr, io.ErrUnexpectedEOF) {
		return false, readErr
	}
	if closeErr != nil {
		return false, closeErr
	}
	return nativeExecutableMagic(header[:read]), nil
}

func nativeExecutableMagic(value []byte) bool {
	if len(value) >= 2 && value[0] == 'M' && value[1] == 'Z' {
		return true
	}
	if len(value) < 4 {
		return false
	}
	magic := uint32(value[0])<<24 |
		uint32(value[1])<<16 |
		uint32(value[2])<<8 |
		uint32(value[3])
	switch magic {
	case 0x7f454c46, // ELF
		0xfeedface, 0xcefaedfe, // 32-bit Mach-O
		0xfeedfacf, 0xcffaedfe, // 64-bit Mach-O
		0xcafebabe, 0xbebafeca, // universal Mach-O
		0xcafebabf, 0xbfbafeca: // 64-bit universal Mach-O
		return true
	default:
		return false
	}
}

func unsafeWindowsArchivePath(clean string) bool {
	if strings.Contains(clean, ":") {
		return true
	}
	for _, char := range clean {
		if char < 32 || char == 127 {
			return true
		}
	}
	reserved := map[string]bool{
		"con": true, "prn": true, "aux": true, "nul": true,
		"com1": true, "com2": true, "com3": true, "com4": true, "com5": true,
		"com6": true, "com7": true, "com8": true, "com9": true,
		"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true,
		"lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
	}
	for _, part := range strings.Split(clean, "/") {
		if part == "" || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return true
		}
		base := strings.ToLower(strings.SplitN(part, ".", 2)[0])
		if reserved[base] {
			return true
		}
	}
	return false
}

func runAssistedCommand(
	ctx context.Context,
	path string,
	args []string,
	cwd string,
	onActivity func(),
) (string, error) {
	return runAssistedCommandWithEnvironment(
		ctx,
		path,
		args,
		cwd,
		onActivity,
		assistedCommandEnvironment(),
	)
}

func runAssistedCommandWithEnvironment(
	ctx context.Context,
	path string,
	args []string,
	cwd string,
	onActivity func(),
	environment []string,
) (string, error) {
	return runAssistedCommandWithEnvironmentMonitor(
		ctx,
		path,
		args,
		cwd,
		onActivity,
		environment,
		0,
		nil,
	)
}

func runAssistedCommandWithEnvironmentMonitor(
	ctx context.Context,
	path string,
	args []string,
	cwd string,
	onActivity func(),
	environment []string,
	pollPeriod time.Duration,
	monitor func() error,
) (string, error) {
	if monitor != nil {
		if pollPeriod <= 0 {
			return "", errors.New("assisted command monitor interval must be positive")
		}
		if err := monitor(); err != nil {
			return "", err
		}
	}
	commandContext, cancel := context.WithCancel(ctx)
	defer cancel()
	command := exec.CommandContext(commandContext, path, args...)
	processutil.ConfigureBackground(command)
	command.Env = append([]string(nil), environment...)
	command.Dir = cwd
	var output boundedOutput
	command.Stdout = io.MultiWriter(&output, activityOutput{onActivity: onActivity})
	command.Stderr = io.MultiWriter(&output, activityOutput{onActivity: onActivity})
	if monitor == nil {
		return finishAssistedCommand(&output, command.Run())
	}
	if err := command.Start(); err != nil {
		return finishAssistedCommand(&output, err)
	}
	waited := make(chan error, 1)
	go func() {
		waited <- command.Wait()
	}()
	ticker := time.NewTicker(pollPeriod)
	defer ticker.Stop()
	for {
		select {
		case err := <-waited:
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			if monitorErr := monitor(); monitorErr != nil {
				return "", monitorErr
			}
			return finishAssistedCommand(&output, err)
		case <-ticker.C:
			if monitorErr := monitor(); monitorErr != nil {
				cancel()
				<-waited
				return "", monitorErr
			}
		case <-ctx.Done():
			cancel()
			<-waited
			return "", ctx.Err()
		}
	}
}

func finishAssistedCommand(output *boundedOutput, err error) (string, error) {
	if err != nil {
		message := strings.TrimSpace(output.String())
		if message == "" {
			message = err.Error()
		}
		return "", errors.New(message)
	}
	return strings.TrimSpace(output.String()), nil
}

type boundedOutput struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (output *boundedOutput) Write(data []byte) (int, error) {
	output.mu.Lock()
	defer output.mu.Unlock()
	const limit = 32 << 10
	written := len(data)
	if output.buffer.Len() < limit {
		remaining := limit - output.buffer.Len()
		if len(data) > remaining {
			data = data[len(data)-remaining:]
		}
		_, _ = output.buffer.Write(data)
	}
	return written, nil
}

func (output *boundedOutput) String() string {
	output.mu.Lock()
	defer output.mu.Unlock()
	return output.buffer.String()
}

type activityOutput struct {
	onActivity func()
}

func (writer activityOutput) Write(data []byte) (int, error) {
	if writer.onActivity != nil && bytes.ContainsRune(data, '\n') {
		writer.onActivity()
	}
	return len(data), nil
}

func assistedCommandEnvironment() []string {
	blocked := map[string]bool{
		"OPENAI_API_KEY": true, "CODEX_API_KEY": true,
		"GITHUB_TOKEN": true, "GH_TOKEN": true,
		"PIP_INDEX_URL": true, "PIP_EXTRA_INDEX_URL": true,
		"PIP_TRUSTED_HOST": true, "PYTHONPATH": true, "PYTHONHOME": true,
		"HTTP_PROXY": true, "HTTPS_PROXY": true, "ALL_PROXY": true,
		"FTP_PROXY": true, "NO_PROXY": true, "REQUEST_METHOD": true,
		"AWS_SECRET_ACCESS_KEY": true, "AZURE_CLIENT_SECRET": true,
		"GOOGLE_APPLICATION_CREDENTIALS": true,
	}
	out := make([]string, 0, len(os.Environ())+3)
	for _, item := range os.Environ() {
		name, _, _ := strings.Cut(item, "=")
		upperName := strings.ToUpper(name)
		if !blocked[upperName] && !strings.HasPrefix(upperName, "PIP_") {
			out = append(out, item)
		}
	}
	out = append(
		out,
		"PIP_CONFIG_FILE="+os.DevNull,
		"PIP_NO_INPUT=1",
		"PIP_DISABLE_PIP_VERSION_CHECK=1",
	)
	return out
}

func assistedPyPIDownloadEnvironment(proxyURL, temporaryRoot string) []string {
	out := assistedCommandEnvironment()
	out = append(
		out,
		"HTTP_PROXY="+proxyURL,
		"HTTPS_PROXY="+proxyURL,
		"NO_PROXY=__csm_no_proxy_bypass__",
		"TEMP="+temporaryRoot,
		"TMP="+temporaryRoot,
		"TMPDIR="+temporaryRoot,
		"PATH=",
	)
	if runtime.GOOS != "windows" {
		out = append(
			out,
			"http_proxy="+proxyURL,
			"https_proxy="+proxyURL,
			"no_proxy=__csm_no_proxy_bypass__",
		)
	}
	return out
}

func normalizePackagePath(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, name)
}

func canonicalPythonPackageName(name string) string {
	return pythonNameBoundary.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-")
}

func shortPlanDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:6])
}

func moveManagedToolToQuarantine(dataRoot, target, transactionID string) (string, error) {
	if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	toolsRoot := filepath.Join(dataRoot, "tools")
	if err := ensureWithinOrEqual(toolsRoot, target); err != nil {
		return "", errors.New("managed tool recovery target is outside the application tools directory")
	}
	quarantine := filepath.Join(
		dataRoot, "quarantine-tools", transactionID, filepath.Base(filepath.Dir(target)), filepath.Base(target),
	)
	if err := os.MkdirAll(filepath.Dir(quarantine), 0o700); err != nil {
		return "", err
	}
	if err := os.Rename(target, quarantine); err != nil {
		return "", err
	}
	return quarantine, nil
}

func sortedHashPairs(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for name, hash := range values {
		out = append(out, name+"="+hash)
	}
	sort.Strings(out)
	return out
}
