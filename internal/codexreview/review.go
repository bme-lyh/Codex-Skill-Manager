package codexreview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
	"github.com/bme-lyh/Codex-Skill-Manager/internal/processutil"
)

func Status(ctx context.Context, configuredPath string) model.CodexCLIStatus {
	checkedAt := time.Now().UTC()
	path, err := resolveExecutable(ctx, configuredPath)
	if err != nil {
		return model.CodexCLIStatus{CheckedAt: checkedAt, Error: err.Error()}
	}
	result := model.CodexCLIStatus{Available: true, Path: path, CheckedAt: checkedAt}
	version := exec.CommandContext(ctx, path, "--version")
	processutil.ConfigureBackground(version)
	version.Env = sanitizedEnvironment()
	if output, commandErr := version.CombinedOutput(); commandErr == nil {
		result.Version = strings.TrimSpace(string(output))
	}
	result.MissingCapabilities = missingCapabilities(ctx, path)
	result.Compatible = len(result.MissingCapabilities) == 0
	login := exec.CommandContext(ctx, path, "login", "status")
	processutil.ConfigureBackground(login)
	login.Env = sanitizedEnvironment()
	output, loginErr := login.CombinedOutput()
	result.AuthStatus = strings.TrimSpace(string(output))
	if loginErr != nil {
		result.Error = "Codex CLI 尚未登录或登录状态不可用"
		return result
	}
	result.Authenticated = true
	models, catalogErr := modelCatalog(ctx, path)
	if catalogErr != nil {
		result.ModelCatalogError = catalogErr.Error()
	} else {
		result.Models = models
	}
	if !result.Compatible {
		result.Error = "Codex CLI 缺少风险复核所需能力，请更新 CLI 或检查自定义路径"
	}
	return result
}

func modelCatalog(ctx context.Context, path string) ([]model.CodexModelOption, error) {
	catalogCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	command := exec.CommandContext(catalogCtx, path, "debug", "models")
	processutil.ConfigureBackground(command)
	command.Env = sanitizedEnvironment()
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("无法读取 Codex CLI 模型列表：%w", err)
	}
	return parseModelCatalog(output)
}

func parseModelCatalog(data []byte) ([]model.CodexModelOption, error) {
	var catalog struct {
		Models []struct {
			Slug                  string `json:"slug"`
			DisplayName           string `json:"display_name"`
			Description           string `json:"description"`
			DefaultReasoningLevel string `json:"default_reasoning_level"`
			Visibility            string `json:"visibility"`
			ReasoningLevels       []struct {
				Effort      string `json:"effort"`
				Description string `json:"description"`
			} `json:"supported_reasoning_levels"`
		} `json:"models"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("解析 Codex CLI 模型列表：%w", err)
	}
	options := make([]model.CodexModelOption, 0, len(catalog.Models))
	for _, entry := range catalog.Models {
		if entry.Visibility != "list" || strings.TrimSpace(entry.Slug) == "" {
			continue
		}
		option := model.CodexModelOption{
			Slug:                  strings.TrimSpace(entry.Slug),
			DisplayName:           strings.TrimSpace(entry.DisplayName),
			Description:           strings.TrimSpace(entry.Description),
			DefaultReasoningLevel: strings.TrimSpace(entry.DefaultReasoningLevel),
			ReasoningLevels:       make([]model.CodexReasoningOption, 0, len(entry.ReasoningLevels)),
		}
		if option.DisplayName == "" {
			option.DisplayName = option.Slug
		}
		for _, level := range entry.ReasoningLevels {
			if effort := strings.TrimSpace(level.Effort); effort != "" {
				option.ReasoningLevels = append(option.ReasoningLevels, model.CodexReasoningOption{
					Effort: effort, Description: strings.TrimSpace(level.Description),
				})
			}
		}
		options = append(options, option)
	}
	return options, nil
}

type ProgressFunc func(model.CodexReviewProgress)

func Review(
	ctx context.Context,
	cfg model.CodexReviewConfig,
	report model.ScanReport,
	workRoot string,
	requestedSkills []string,
	progress ProgressFunc,
) (model.CodexReviewResult, error) {
	return reviewInBatches(ctx, cfg, report, workRoot, requestedSkills, progress)
}

func countContextFiles(root string) (int, error) {
	count := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && shouldSkipReviewDir(root, path) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("full-context Codex review refuses symbolic links: %s", path)
		}
		count++
		return nil
	})
	return count, err
}

func failed(result model.CodexReviewResult, err error) (model.CodexReviewResult, error) {
	result.Status = "failed"
	result.Error = err.Error()
	result.CompletedAt = time.Now().UTC()
	result.DurationMillis = result.CompletedAt.Sub(result.StartedAt).Milliseconds()
	return result, err
}

func resolveExecutable(ctx context.Context, configured string) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		if !filepath.IsAbs(configured) {
			return "", errors.New("Codex CLI 路径必须是绝对路径")
		}
		info, err := os.Stat(configured)
		if err != nil || info.IsDir() {
			return "", errors.New("配置的 Codex CLI 不可用")
		}
		if !canExecuteCodex(ctx, configured) {
			return "", errors.New("配置的 Codex CLI 无法执行；请检查路径或文件权限")
		}
		return filepath.Clean(configured), nil
	}

	candidates := executableCandidates(os.Getenv("PATH"), runtime.GOOS, userHome())
	for _, candidate := range candidates {
		if canExecuteCodex(ctx, candidate) {
			return candidate, nil
		}
	}
	if len(candidates) > 0 {
		return "", errors.New("找到了 Codex CLI，但现有候选均无法执行；请安装独立 CLI 或在设置中指定路径")
	}
	return "", errors.New("未找到可执行的 Codex CLI；请安装独立 CLI 或在设置中指定路径")
}

func executableCandidates(pathEnv, goos, home string) []string {
	names := []string{"codex"}
	if goos == "windows" {
		names = []string{"codex.exe", "codex.cmd", "codex.bat", "codex.com"}
	}
	directories := filepath.SplitList(pathEnv)
	if goos == "windows" && home != "" {
		// The independent npm CLI is installed here by default. WindowsApps can
		// appear earlier on PATH but its app-bundled executable is not callable
		// by ordinary desktop applications, so keep every candidate and probe it.
		directories = append(directories, filepath.Join(home, "AppData", "Roaming", "npm"))
	}
	seen := map[string]bool{}
	result := make([]string, 0, len(directories))
	for _, directory := range directories {
		directory = strings.TrimSpace(strings.Trim(directory, `"`))
		if directory == "" {
			continue
		}
		for _, name := range names {
			candidate := filepath.Clean(filepath.Join(directory, name))
			key := strings.ToLower(candidate)
			if seen[key] {
				continue
			}
			info, err := os.Stat(candidate)
			if err != nil || info.IsDir() {
				continue
			}
			seen[key] = true
			result = append(result, candidate)
		}
	}
	return result
}

func canExecuteCodex(ctx context.Context, path string) bool {
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	command := exec.CommandContext(probeCtx, path, "--version")
	processutil.ConfigureBackground(command)
	command.Env = sanitizedEnvironment()
	return command.Run() == nil
}

func missingCapabilities(ctx context.Context, path string) []string {
	rootHelp := commandHelp(ctx, path, "--help")
	execHelp := commandHelp(ctx, path, "exec", "--help")
	missing := missingCapabilitiesFromHelp(rootHelp, execHelp)
	features := commandHelp(ctx, path, "features", "list")
	for _, feature := range []string{"shell_tool", "shell_snapshot"} {
		if !codexFeatureAvailable(features, feature) {
			missing = append(missing, "features "+feature)
		}
	}
	return missing
}

func missingCapabilitiesFromHelp(rootHelp, execHelp string) []string {
	requirements := []struct {
		scope string
		help  string
		flags []string
	}{
		{"全局", rootHelp, []string{"--config", "--model", "--sandbox", "--ask-for-approval"}},
		{"exec", execHelp, []string{
			"--ephemeral", "--skip-git-repo-check", "--ignore-user-config", "--ignore-rules",
			"--disable", "--json", "--output-schema", "--output-last-message",
		}},
	}
	missing := make([]string, 0)
	for _, requirement := range requirements {
		for _, flag := range requirement.flags {
			if !strings.Contains(requirement.help, flag) {
				missing = append(missing, requirement.scope+" "+flag)
			}
		}
	}
	return missing
}

func codexFeatureAvailable(output, name string) bool {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == name && fields[1] != "removed" {
			return true
		}
	}
	return false
}

func commandHelp(ctx context.Context, path string, args ...string) string {
	helpCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	command := exec.CommandContext(helpCtx, path, args...)
	processutil.ConfigureBackground(command)
	command.Env = sanitizedEnvironment()
	output, _ := command.CombinedOutput()
	return string(output)
}

func reviewArgs(cfg model.CodexReviewConfig, schemaPath, outputPath string) []string {
	args := []string{}
	if modelName := strings.TrimSpace(cfg.Model); modelName != "" && modelName != "default" {
		args = append(args, "--model", modelName)
	}
	// Approval and sandbox are global CLI options. Keep them before the exec
	// subcommand; exec-specific non-interactive options follow the subcommand.
	args = append(args,
		"-c", fmt.Sprintf("model_reasoning_effort=%q", cfg.ReasoningEffort),
		"--sandbox", "read-only", "--ask-for-approval", "never",
		"exec", "--ephemeral", "--skip-git-repo-check",
		"--ignore-user-config", "--ignore-rules",
		"--json", "--output-schema", schemaPath, "--output-last-message", outputPath, "-",
	)
	return args
}

func userHome() string {
	home, _ := os.UserHomeDir()
	return home
}

func sanitizedEnvironment() []string {
	allowed := map[string]bool{
		"ALL_PROXY": true, "APPDATA": true, "CODEX_HOME": true, "COMSPEC": true,
		"HOMEDRIVE": true, "HOMEPATH": true, "HOME": true,
		"HTTPS_PROXY": true, "HTTP_PROXY": true,
		"LANG": true, "LC_ALL": true, "LOCALAPPDATA": true,
		"NO_PROXY": true, "PATH": true, "PATHEXT": true,
		"SSL_CERT_DIR": true, "SSL_CERT_FILE": true,
		"SYSTEMROOT": true, "TEMP": true, "TMP": true,
		"USERPROFILE": true, "WINDIR": true,
	}
	out := make([]string, 0, len(allowed))
	for _, item := range os.Environ() {
		name, _, _ := strings.Cut(item, "=")
		if allowed[strings.ToUpper(name)] {
			out = append(out, item)
		}
	}
	return out
}
