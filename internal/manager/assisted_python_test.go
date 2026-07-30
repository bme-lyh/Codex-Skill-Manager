package manager

import (
	"archive/zip"
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

func TestValidatePyPIProjectRequiresMatchingRepositoryAndWheel(t *testing.T) {
	var metadata pypiMetadata
	metadata.Info.Name = "code-review-graph"
	metadata.Info.Version = "2.3.7"
	metadata.Info.ProjectURLs = map[string]string{
		"Source": "https://github.com/tirth8205/code-review-graph",
	}
	metadata.Releases = map[string][]pypiReleaseFile{
		"2.3.7": {{Filename: "code_review_graph-2.3.7-py3-none-any.whl", PackageType: "bdist_wheel"}},
	}
	if err := validatePyPIProject(
		metadata, "code-review-graph", "2.3.7", "tirth8205/code-review-graph",
	); err != nil {
		t.Fatal(err)
	}
	metadata.Info.ProjectURLs["Source"] =
		"https://github.com/tirth8205/code-review-graph.git/"
	if err := validatePyPIProject(
		metadata, "code-review-graph", "2.3.7", "tirth8205/code-review-graph",
	); err != nil {
		t.Fatalf("expected canonical GitHub .git URL to match: %v", err)
	}
	metadata.Info.ProjectURLs["Source"] =
		"https://github.com/tirth8205/code-review-graph"
	if err := validatePyPIProject(
		metadata, "code-review-graph", "2.3.7", "another/repository",
	); err == nil || !strings.Contains(err.Error(), "does not link back") {
		t.Fatalf("expected repository mismatch, got %v", err)
	}
	metadata.Info.ProjectURLs["Source"] =
		"https://example.invalid/project?source=github.com/tirth8205/code-review-graph"
	if err := validatePyPIProject(
		metadata, "code-review-graph", "2.3.7", "tirth8205/code-review-graph",
	); err == nil {
		t.Fatal("a GitHub repository name in an unrelated URL query was accepted")
	}
	metadata.Info.ProjectURLs["Source"] =
		"https://github.com/tirth8205/code-review-graph-malicious"
	if err := validatePyPIProject(
		metadata, "code-review-graph", "2.3.7", "tirth8205/code-review-graph",
	); err == nil {
		t.Fatal("a repository name prefix was accepted as an exact identity")
	}
	metadata.Info.ProjectURLs["Source"] =
		"https://github.com/tirth8205/code-review-graph"
	delete(metadata.Releases, "2.3.7")
	if err := validatePyPIProject(
		metadata, "code-review-graph", "2.3.7", "tirth8205/code-review-graph",
	); err == nil || !strings.Contains(err.Error(), "has no wheel") {
		t.Fatalf("expected source-only release rejection, got %v", err)
	}
}

func TestValidateOfficialPyPIWheelRequiresPublishedFilenameHashAndURL(t *testing.T) {
	hash := strings.Repeat("a", 64)
	wheel := model.AssistedPythonWheelLock{
		Name:     "example-tool",
		Version:  "1.2.3",
		Filename: "example_tool-1.2.3-py3-none-any.whl",
		SHA256:   hash,
	}
	var metadata pypiMetadata
	metadata.Info.Name = "example-tool"
	metadata.Info.Version = "1.2.3"
	release := pypiReleaseFile{
		Filename:    wheel.Filename,
		PackageType: "bdist_wheel",
		URL: "https://files.pythonhosted.org/packages/aa/bb/" +
			wheel.Filename,
	}
	release.Digests.SHA256 = hash
	metadata.URLs = []pypiReleaseFile{release}
	if err := validateOfficialPyPIWheel(metadata, wheel); err != nil {
		t.Fatal(err)
	}

	t.Run("hash drift", func(t *testing.T) {
		changed := metadata
		changed.URLs = append([]pypiReleaseFile(nil), metadata.URLs...)
		changed.URLs[0].Digests.SHA256 = strings.Repeat("b", 64)
		if err := validateOfficialPyPIWheel(changed, wheel); err == nil {
			t.Fatal("a Wheel hash absent from the official PyPI release was accepted")
		}
	})
	t.Run("non-official host", func(t *testing.T) {
		changed := metadata
		changed.URLs = append([]pypiReleaseFile(nil), metadata.URLs...)
		changed.URLs[0].URL = "https://example.invalid/" + wheel.Filename
		if err := validateOfficialPyPIWheel(changed, wheel); err == nil {
			t.Fatal("a non-official Wheel URL was accepted")
		}
	})
	t.Run("filename mismatch", func(t *testing.T) {
		changed := metadata
		changed.URLs = append([]pypiReleaseFile(nil), metadata.URLs...)
		changed.URLs[0].Filename = "another-1.2.3-py3-none-any.whl"
		if err := validateOfficialPyPIWheel(changed, wheel); err == nil {
			t.Fatal("a Wheel filename absent from the official PyPI release was accepted")
		}
	})
}

func TestValidatePipDownloadOutputAllowsOnlyOfficialHTTPSHosts(t *testing.T) {
	official := "Looking in indexes: https://pypi.org/simple\n" +
		"Downloading https://files.pythonhosted.org/packages/aa/example.whl"
	if err := validatePipDownloadOutput(official); err != nil {
		t.Fatal(err)
	}
	for name, output := range map[string]string{
		"direct HTTPS": "Collecting payload @ https://example.invalid/payload.whl",
		"VCS":          "Collecting payload @ git+https://github.com/example/payload.git",
		"local file":   "Processing file:///tmp/payload.whl",
		"credentials":  "Downloading https://user:secret@pypi.org/payload.whl",
		"insecure":     "Downloading http://files.pythonhosted.org/payload.whl",
	} {
		t.Run(name, func(t *testing.T) {
			if err := validatePipDownloadOutput(output); err == nil ||
				!strings.Contains(err.Error(), "non-official") {
				t.Fatalf("expected non-official pip source rejection, got %v", err)
			}
		})
	}
}

func TestAssistedPyPIConnectTargetAllowsOnlyExactHTTPSTunnels(t *testing.T) {
	tests := []struct {
		name       string
		request    *http.Request
		expected   string
		shouldPass bool
	}{
		{
			name: "PyPI",
			request: &http.Request{
				Method: http.MethodConnect, Host: "pypi.org:443",
				RequestURI: "pypi.org:443", URL: &url.URL{},
			},
			expected: "pypi.org:443", shouldPass: true,
		},
		{
			name: "files host case-insensitive",
			request: &http.Request{
				Method: http.MethodConnect, Host: "FILES.PYTHONHOSTED.ORG:443",
				RequestURI: "FILES.PYTHONHOSTED.ORG:443", URL: &url.URL{},
			},
			expected: "files.pythonhosted.org:443", shouldPass: true,
		},
		{
			name: "plaintext HTTP",
			request: &http.Request{
				Method: http.MethodGet, Host: "pypi.org",
				RequestURI: "http://pypi.org/simple", URL: &url.URL{Scheme: "http"},
			},
		},
		{
			name: "unapproved host",
			request: &http.Request{
				Method: http.MethodConnect, Host: "example.invalid:443",
				RequestURI: "example.invalid:443", URL: &url.URL{},
			},
		},
		{
			name: "wrong port",
			request: &http.Request{
				Method: http.MethodConnect, Host: "pypi.org:8443",
				RequestURI: "pypi.org:8443", URL: &url.URL{},
			},
		},
		{
			name: "userinfo",
			request: &http.Request{
				Method: http.MethodConnect, Host: "user@pypi.org:443",
				RequestURI: "user@pypi.org:443", URL: &url.URL{User: url.User("user")},
			},
		},
		{
			name: "trailing dot",
			request: &http.Request{
				Method: http.MethodConnect, Host: "pypi.org.:443",
				RequestURI: "pypi.org.:443", URL: &url.URL{},
			},
		},
		{
			name: "missing explicit port",
			request: &http.Request{
				Method: http.MethodConnect, Host: "pypi.org",
				RequestURI: "pypi.org", URL: &url.URL{},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target, err := assistedPyPIConnectTarget(test.request)
			if test.shouldPass {
				if err != nil || target != test.expected {
					t.Fatalf("target=%q err=%v, want %q", target, err, test.expected)
				}
				return
			}
			if err == nil {
				t.Fatalf("unexpected proxy target acceptance: %q", target)
			}
		})
	}
}

func TestAssistedPyPIProxyTunnelsAllowlistedHostAndClosesWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dialed := make(chan string, 1)
	proxy, err := startAssistedPyPIProxyWithDialer(
		ctx,
		func(_ context.Context, network string, address string) (net.Conn, error) {
			if network != "tcp" {
				return nil, fmt.Errorf("unexpected network %q", network)
			}
			dialed <- address
			local, remote := net.Pipe()
			go func() {
				defer remote.Close()
				buffer := make([]byte, 64)
				for {
					read, readErr := remote.Read(buffer)
					if read > 0 {
						if _, writeErr := remote.Write(buffer[:read]); writeErr != nil {
							return
						}
					}
					if readErr != nil {
						return
					}
				}
			}()
			return local, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	address := proxy.listener.Addr().String()
	connection, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(connection)
	if _, err := fmt.Fprint(
		connection,
		"CONNECT pypi.org:443 HTTP/1.1\r\nHost: pypi.org:443\r\n\r\n",
	); err != nil {
		connection.Close()
		t.Fatal(err)
	}
	status, err := reader.ReadString('\n')
	if err != nil || !strings.Contains(status, " 200 ") {
		connection.Close()
		t.Fatalf("unexpected CONNECT response %q: %v", status, err)
	}
	for {
		header, readErr := reader.ReadString('\n')
		if readErr != nil {
			connection.Close()
			t.Fatal(readErr)
		}
		if header == "\r\n" {
			break
		}
	}
	if _, err := connection.Write([]byte("ping")); err != nil {
		connection.Close()
		t.Fatal(err)
	}
	echo := make([]byte, 4)
	if _, err := io.ReadFull(reader, echo); err != nil || string(echo) != "ping" {
		connection.Close()
		t.Fatalf("unexpected tunnel echo %q: %v", string(echo), err)
	}
	select {
	case target := <-dialed:
		if target != "pypi.org:443" {
			t.Fatalf("unexpected upstream target %q", target)
		}
	case <-time.After(time.Second):
		t.Fatal("proxy did not dial the allowlisted host")
	}
	cancel()
	select {
	case <-proxy.done:
	case <-time.After(time.Second):
		t.Fatal("proxy did not close when its context ended")
	}
	_ = connection.Close()
	if reopened, dialErr := net.DialTimeout("tcp", address, 100*time.Millisecond); dialErr == nil {
		reopened.Close()
		t.Fatal("proxy listener remained open after context cancellation")
	}
}

func TestAssistedPyPIProxyRejectsPlainHTTPAndUnapprovedConnect(t *testing.T) {
	dialed := make(chan struct{}, 1)
	proxy, err := startAssistedPyPIProxyWithDialer(
		context.Background(),
		func(context.Context, string, string) (net.Conn, error) {
			dialed <- struct{}{}
			return nil, errors.New("unexpected upstream dial")
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	for name, request := range map[string]string{
		"plaintext": "GET http://pypi.org/simple HTTP/1.1\r\nHost: pypi.org\r\n\r\n",
		"other host": "CONNECT example.invalid:443 HTTP/1.1\r\n" +
			"Host: example.invalid:443\r\n\r\n",
	} {
		t.Run(name, func(t *testing.T) {
			connection, dialErr := net.DialTimeout(
				"tcp",
				proxy.listener.Addr().String(),
				time.Second,
			)
			if dialErr != nil {
				t.Fatal(dialErr)
			}
			defer connection.Close()
			if _, writeErr := io.WriteString(connection, request); writeErr != nil {
				t.Fatal(writeErr)
			}
			status, readErr := bufio.NewReader(connection).ReadString('\n')
			if readErr != nil || !strings.Contains(status, " 403 ") {
				t.Fatalf("unexpected proxy rejection response %q: %v", status, readErr)
			}
		})
	}
	select {
	case <-dialed:
		t.Fatal("rejected proxy request reached the upstream dialer")
	default:
	}
}

func TestValidateAssistedDownloadResourceUsageEnforcesRuntimeLimits(t *testing.T) {
	t.Run("within limits", func(t *testing.T) {
		root := t.TempDir()
		temporary := filepath.Join(root, "pip-build")
		if err := os.Mkdir(temporary, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(
			filepath.Join(temporary, "metadata"),
			[]byte("safe"),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
		if err := validateAssistedDownloadResourceUsage(root); err != nil {
			t.Fatalf("valid runtime usage was rejected: %v", err)
		}
	})

	t.Run("per file bytes", func(t *testing.T) {
		root := t.TempDir()
		writeSparseDownloadFixture(
			t,
			filepath.Join(root, "oversized.part"),
			maxAssistedWheelBytes+1,
		)
		err := validateAssistedDownloadResourceUsage(root)
		if err == nil || !strings.Contains(err.Error(), "per-file limit") {
			t.Fatalf("expected per-file runtime rejection, got %v", err)
		}
	})

	t.Run("aggregate bytes", func(t *testing.T) {
		root := t.TempDir()
		for index := 0; index < 6; index++ {
			writeSparseDownloadFixture(
				t,
				filepath.Join(root, fmt.Sprintf("wheel-%d.part", index)),
				maxAssistedWheelBytes,
			)
		}
		writeSparseDownloadFixture(t, filepath.Join(root, "overflow.part"), 1)
		err := validateAssistedDownloadResourceUsage(root)
		if err == nil || !strings.Contains(err.Error(), "total runtime limit") {
			t.Fatalf("expected aggregate runtime rejection, got %v", err)
		}
	})

	t.Run("top-level file count", func(t *testing.T) {
		root := t.TempDir()
		for index := 0; index <= maxAssistedWheels; index++ {
			if err := os.WriteFile(
				filepath.Join(root, fmt.Sprintf("wheel-%03d.part", index)),
				nil,
				0o600,
			); err != nil {
				t.Fatal(err)
			}
		}
		err := validateAssistedDownloadResourceUsage(root)
		if err == nil || !strings.Contains(err.Error(), "Wheel file count limit") {
			t.Fatalf("expected runtime file-count rejection, got %v", err)
		}
	})
}

func TestAssistedDownloadResourceMonitorClosesProxyOnLimit(t *testing.T) {
	root := t.TempDir()
	writeSparseDownloadFixture(
		t,
		filepath.Join(root, "oversized.part"),
		maxAssistedWheelBytes+1,
	)
	proxy, err := startAssistedPyPIProxyWithDialer(
		context.Background(),
		func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("unexpected upstream dial")
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	monitor := assistedDownloadResourceMonitor(root, proxy.Close)
	if err := monitor(); err == nil {
		t.Fatal("expected runtime resource monitor rejection")
	}
	select {
	case <-proxy.done:
	case <-time.After(time.Second):
		t.Fatal("resource limit did not close the outbound proxy")
	}
}

func TestRunAssistedCommandWithEnvironmentMonitorCancelsProcess(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	environment := append(
		append([]string(nil), os.Environ()...),
		"CSM_ASSISTED_MONITOR_HELPER=1",
	)
	limitErr := errors.New("runtime download limit reached")
	checks := 0
	started := time.Now()
	_, err = runAssistedCommandWithEnvironmentMonitor(
		context.Background(),
		executable,
		[]string{"-test.run=^TestAssistedCommandMonitorHelperProcess$"},
		"",
		nil,
		environment,
		10*time.Millisecond,
		func() error {
			checks++
			if checks > 1 {
				return limitErr
			}
			return nil
		},
	)
	if !errors.Is(err, limitErr) {
		t.Fatalf("expected monitor error, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("monitored command was not cancelled promptly: %v", elapsed)
	}
}

func TestAssistedCommandMonitorHelperProcess(t *testing.T) {
	if os.Getenv("CSM_ASSISTED_MONITOR_HELPER") != "1" {
		return
	}
	time.Sleep(5 * time.Second)
}

func TestInspectWheelhouseHashesWheelsAndDetectsNativeCode(t *testing.T) {
	root := t.TempDir()
	pure := filepath.Join(root, "pure-1.0-py3-none-any.whl")
	writeWheelFixture(t, pure, map[string]string{"pure/__init__.py": "ok"})
	native := filepath.Join(root, "native-1.0-cp313-win_amd64.whl")
	writeWheelFixture(t, native, map[string]string{"native/module.pyd": "binary"})

	hashes, containsNative, err := inspectWheelhouse(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(hashes) != 2 || hashes[filepath.Base(pure)] == "" ||
		hashes[filepath.Base(native)] == "" || !containsNative {
		t.Fatalf("unexpected wheel inventory: hashes=%#v native=%t", hashes, containsNative)
	}
}

func TestInspectWheelhouseLockCapturesPureAndApprovedNativeIdentity(t *testing.T) {
	platform, nativeFile := nativeTestWheelPlatform()
	if platform == "" {
		t.Skip("test platform has no managed native-Wheel mapping")
	}
	root := t.TempDir()
	writeIdentifiedWheelFixture(
		t,
		filepath.Join(root, "example_tool-1.2.3-py3-none-any.whl"),
		"example-tool",
		"1.2.3",
		true,
		[]string{"py3-none-any"},
		map[string]string{"example_tool/__init__.py": "ok"},
	)
	writeIdentifiedWheelFixture(
		t,
		filepath.Join(root, "native_dep-4.5.6-cp39-abi3-"+platform+".whl"),
		"native-dep",
		"4.5.6",
		false,
		[]string{"cp39-abi3-" + platform},
		map[string]string{nativeFile: "binary"},
	)

	wheels, containsNative, err := inspectWheelhouseLock(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(wheels) != 2 || !containsNative {
		t.Fatalf("unexpected approval-time Wheel lock: %#v native=%t", wheels, containsNative)
	}
	var native model.AssistedPythonWheelLock
	for _, wheel := range wheels {
		if wheel.Native {
			native = wheel
		}
	}
	if native.Name != "native-dep" || native.Version != "4.5.6" ||
		len(native.Tags) != 1 || native.Tags[0] != "cp39-abi3-"+platform ||
		len(native.SHA256) != 64 {
		t.Fatalf("native Wheel identity is incomplete: %#v", native)
	}
	if err := validateManagedPythonWheelLocks(wheels, "example-tool", "1.2.3"); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyLockedWheelhouseRejectsHashDrift(t *testing.T) {
	root := t.TempDir()
	writeIdentifiedWheelFixture(
		t,
		filepath.Join(root, "example_tool-1.2.3-py3-none-any.whl"),
		"example-tool",
		"1.2.3",
		true,
		[]string{"py3-none-any"},
		map[string]string{"example_tool/__init__.py": "ok"},
	)
	wheels, _, err := inspectWheelhouseLock(root)
	if err != nil {
		t.Fatal(err)
	}
	wheels[0].SHA256 = strings.Repeat("0", 64)
	if _, _, err := verifyLockedWheelhouse(
		root,
		wheels,
		"example-tool",
		"1.2.3",
	); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected approved hash drift rejection, got %v", err)
	}
}

func TestInspectWheelhouseLockRejectsNativeCodeHiddenInPureWheel(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"native extension": {"unsafe/module.pyd": "binary"},
		"PE magic":         {"unsafe/payload.dat": "MZ\x90\x00"},
		"ELF magic":        {"unsafe/payload.dat": "\x7fELF\x02\x01"},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeIdentifiedWheelFixture(
				t,
				filepath.Join(root, "unsafe-1.0-py3-none-any.whl"),
				"unsafe",
				"1.0",
				true,
				[]string{"py3-none-any"},
				files,
			)
			if _, _, err := inspectWheelhouseLock(root); err == nil ||
				!strings.Contains(err.Error(), "declares a pure platform tag") {
				t.Fatalf("expected disguised native Wheel rejection, got %v", err)
			}
		})
	}
}

func TestInspectWheelhouseLockRejectsDirectURLDependency(t *testing.T) {
	root := t.TempDir()
	writeWheelFixture(
		t,
		filepath.Join(root, "unsafe-1.0-py3-none-any.whl"),
		map[string]string{
			"unsafe/__init__.py": "ok",
			"unsafe-1.0.dist-info/METADATA": "Metadata-Version: 2.1\n" +
				"Name: unsafe\nVersion: 1.0\n" +
				"Requires-Dist: payload @ https://example.invalid/payload.whl\n\n",
			"unsafe-1.0.dist-info/WHEEL": "Wheel-Version: 1.0\n" +
				"Root-Is-Purelib: true\nTag: py3-none-any\n\n",
		},
	)
	if _, _, err := inspectWheelhouseLock(root); err == nil ||
		!strings.Contains(err.Error(), "direct-URL dependency") {
		t.Fatalf("expected direct-URL dependency rejection, got %v", err)
	}
}

func TestInspectWheelhouseLockRejectsDirectURLOriginAndLegacyDependencyLinks(t *testing.T) {
	for name, metadataFiles := range map[string]map[string]string{
		"PEP 610 origin": {
			"unsafe-1.0.dist-info/direct_url.json": `{"url":"https://example.invalid/payload.whl"}`,
		},
		"legacy dependency link": {
			"unsafe-1.0.dist-info/METADATA": "Metadata-Version: 2.1\n" +
				"Name: unsafe\nVersion: 1.0\n" +
				"Dependency-Link: https://example.invalid/payload.whl\n\n",
		},
		"direct URL header": {
			"unsafe-1.0.dist-info/METADATA": "Metadata-Version: 2.1\n" +
				"Name: unsafe\nVersion: 1.0\n" +
				"Direct-URL: https://example.invalid/payload.whl\n\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			files := map[string]string{
				"unsafe/__init__.py":            "ok",
				"unsafe-1.0.dist-info/METADATA": "Metadata-Version: 2.1\nName: unsafe\nVersion: 1.0\n\n",
				"unsafe-1.0.dist-info/WHEEL": "Wheel-Version: 1.0\n" +
					"Root-Is-Purelib: true\nTag: py3-none-any\n\n",
			}
			for path, content := range metadataFiles {
				files[path] = content
			}
			writeWheelFixture(
				t,
				filepath.Join(root, "unsafe-1.0-py3-none-any.whl"),
				files,
			)
			if _, _, err := inspectWheelhouseLock(root); err == nil ||
				(!strings.Contains(err.Error(), "Direct-URL") &&
					!strings.Contains(err.Error(), "dependency URLs")) {
				t.Fatalf("expected direct dependency origin rejection, got %v", err)
			}
		})
	}
}

func TestInspectWheelArchiveDetectsNativeExtensionsAndExecutableMagic(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		content  string
		expected bool
	}{
		{name: "exe extension", path: "package/tool.exe", content: "text", expected: true},
		{name: "node extension", path: "package/addon.node", content: "text", expected: true},
		{name: "sys extension", path: "package/driver.sys", content: "text", expected: true},
		{name: "versioned shared library", path: "package/lib.so.1", content: "text", expected: true},
		{name: "PE magic", path: "package/payload.dat", content: "MZ\x90\x00", expected: true},
		{name: "ELF magic", path: "package/payload.dat", content: "\x7fELF\x02\x01", expected: true},
		{name: "Mach-O magic", path: "package/payload.dat", content: "\xcf\xfa\xed\xfe", expected: true},
		{name: "plain data", path: "package/payload.dat", content: "plain", expected: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fixture-1.0-py3-none-any.whl")
			writeWheelFixture(t, path, map[string]string{test.path: test.content})
			native, _, err := inspectWheelArchive(path)
			if err != nil {
				t.Fatal(err)
			}
			if native != test.expected {
				t.Fatalf("native=%t, want %t", native, test.expected)
			}
		})
	}
}

func TestWriteHashedRequirementsPinsEveryApprovedWheel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "requirements.lock")
	wheels := []model.AssistedPythonWheelLock{
		{
			Name: "example-tool", Version: "1.2.3",
			Filename: "example_tool-1.2.3-py3-none-any.whl",
			SHA256:   strings.Repeat("a", 64), Tags: []string{"py3-none-any"},
		},
		{
			Name: "dependency", Version: "4.5.6",
			Filename: "dependency-4.5.6-py3-none-any.whl",
			SHA256:   strings.Repeat("b", 64), Tags: []string{"py3-none-any"},
		},
	}
	if err := writeHashedRequirements(path, wheels); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, expected := range []string{
		"example-tool==1.2.3 --hash=sha256:" + strings.Repeat("a", 64),
		"dependency==4.5.6 --hash=sha256:" + strings.Repeat("b", 64),
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("hashed requirements omitted %q: %s", expected, text)
		}
	}
}

func TestInspectWheelhouseRejectsNonWheelAndSymlinkLikeInput(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.tar.gz"), []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := inspectWheelhouse(root); err == nil || !strings.Contains(err.Error(), "not a wheel") {
		t.Fatalf("expected source archive rejection, got %v", err)
	}
}

func TestInspectWheelhouseRejectsUnsafeArchiveEntries(t *testing.T) {
	t.Run("path traversal", func(t *testing.T) {
		root := t.TempDir()
		writeWheelFixture(
			t,
			filepath.Join(root, "unsafe-1.0-py3-none-any.whl"),
			map[string]string{"../escape.py": "unsafe"},
		)
		if _, _, err := inspectWheelhouse(root); err == nil ||
			!strings.Contains(err.Error(), "unsafe path") {
			t.Fatalf("expected wheel path traversal rejection, got %v", err)
		}
	})
	t.Run("symbolic link", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "link-1.0-py3-none-any.whl")
		target, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		writer := zip.NewWriter(target)
		header := &zip.FileHeader{Name: "package/link"}
		header.SetMode(os.ModeSymlink | 0o777)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			target.Close()
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte("../outside")); err != nil {
			target.Close()
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			target.Close()
			t.Fatal(err)
		}
		if err := target.Close(); err != nil {
			t.Fatal(err)
		}
		if _, _, err := inspectWheelhouse(root); err == nil ||
			!strings.Contains(err.Error(), "non-regular") {
			t.Fatalf("expected wheel symbolic link rejection, got %v", err)
		}
	})
	t.Run("Windows device and stream paths", func(t *testing.T) {
		for _, entry := range []string{"C:/outside.py", "package/file.py:payload", "package/CON.txt"} {
			root := t.TempDir()
			writeWheelFixture(
				t,
				filepath.Join(root, "unsafe-1.0-py3-none-any.whl"),
				map[string]string{entry: "unsafe"},
			)
			if _, _, err := inspectWheelhouse(root); err == nil ||
				!strings.Contains(err.Error(), "unsafe path") {
				t.Fatalf("expected Windows path rejection for %q, got %v", entry, err)
			}
		}
	})
	t.Run("case-insensitive duplicate", func(t *testing.T) {
		root := t.TempDir()
		writeWheelFixture(
			t,
			filepath.Join(root, "duplicate-1.0-py3-none-any.whl"),
			map[string]string{"package/Module.py": "one", "package/module.py": "two"},
		)
		if _, _, err := inspectWheelhouse(root); err == nil ||
			!strings.Contains(err.Error(), "duplicate path") {
			t.Fatalf("expected duplicate wheel path rejection, got %v", err)
		}
	})
}

func TestAssistedCommandEnvironmentDropsSecretsAndIndexOverrides(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "should-not-leak")
	t.Setenv("PIP_INDEX_URL", "https://user:password@example.invalid/simple")
	t.Setenv("PIP_FIND_LINKS", "https://example.invalid/wheels")
	t.Setenv("HTTP_PROXY", "http://user:password@example.invalid")
	t.Setenv("HTTPS_PROXY", "http://user:password@example.invalid")
	t.Setenv("ALL_PROXY", "socks5://example.invalid")
	t.Setenv("NO_PROXY", "pypi.org,files.pythonhosted.org")
	t.Setenv("REQUEST_METHOD", "GET")
	values := assistedCommandEnvironment()
	joined := strings.Join(values, "\n")
	if strings.Contains(joined, "should-not-leak") || strings.Contains(joined, "password") ||
		strings.Contains(joined, "example.invalid") {
		t.Fatalf("sensitive environment leaked into child process: %s", joined)
	}
	if !strings.Contains(joined, "PIP_NO_INPUT=1") ||
		!strings.Contains(joined, "PIP_CONFIG_FILE="+os.DevNull) {
		t.Fatalf("non-interactive pip guard missing: %s", joined)
	}
	base := environmentByName(values)
	for _, name := range []string{
		"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "REQUEST_METHOD",
	} {
		if _, ok := base[name]; ok {
			t.Fatalf("inherited proxy bypass %s was not removed", name)
		}
	}

	proxyURL := "http://127.0.0.1:43123"
	temporaryRoot := filepath.Join(t.TempDir(), "pip-temporary")
	download := environmentByName(
		assistedPyPIDownloadEnvironment(proxyURL, temporaryRoot),
	)
	if download["HTTP_PROXY"] != proxyURL || download["HTTPS_PROXY"] != proxyURL {
		t.Fatalf("pip download proxy was not forced: %#v", download)
	}
	if download["NO_PROXY"] != "__csm_no_proxy_bypass__" {
		t.Fatalf("NO_PROXY bypass was not disabled: %#v", download)
	}
	if _, ok := download["ALL_PROXY"]; ok {
		t.Fatalf("ALL_PROXY bypass remained in the pip environment: %#v", download)
	}
	if download["PATH"] != "" {
		t.Fatalf("pip download retained an executable search path: %#v", download)
	}
	for _, name := range []string{"TEMP", "TMP", "TMPDIR"} {
		if download[name] != temporaryRoot {
			t.Fatalf("%s was not constrained to the monitored directory: %#v", name, download)
		}
	}
}

func environmentByName(values []string) map[string]string {
	result := make(map[string]string, len(values))
	for _, item := range values {
		name, value, _ := strings.Cut(item, "=")
		result[strings.ToUpper(name)] = value
	}
	return result
}

func writeSparseDownloadFixture(t *testing.T, path string, size int64) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(size); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeWheelFixture(t *testing.T, path string, files map[string]string) {
	t.Helper()
	target, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(target)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			target.Close()
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			target.Close()
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		target.Close()
		t.Fatal(err)
	}
	if err := target.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeIdentifiedWheelFixture(
	t *testing.T,
	path string,
	name string,
	version string,
	pure bool,
	tags []string,
	files map[string]string,
) {
	t.Helper()
	allFiles := make(map[string]string, len(files)+2)
	for file, content := range files {
		allFiles[file] = content
	}
	distInfo := strings.ReplaceAll(name, "-", "_") + "-" + version + ".dist-info"
	allFiles[distInfo+"/METADATA"] = "Metadata-Version: 2.1\nName: " + name +
		"\nVersion: " + version + "\n\n"
	var wheel strings.Builder
	wheel.WriteString("Wheel-Version: 1.0\n")
	if pure {
		wheel.WriteString("Root-Is-Purelib: true\n")
	} else {
		wheel.WriteString("Root-Is-Purelib: false\n")
	}
	for _, tag := range tags {
		wheel.WriteString("Tag: " + tag + "\n")
	}
	wheel.WriteString("\n")
	allFiles[distInfo+"/WHEEL"] = wheel.String()
	writeWheelFixture(t, path, allFiles)
}

func nativeTestWheelPlatform() (string, string) {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "windows/amd64":
		return "win_amd64", "native_dep/module.pyd"
	case "windows/arm64":
		return "win_arm64", "native_dep/module.pyd"
	case "windows/386":
		return "win32", "native_dep/module.pyd"
	case "linux/amd64":
		return "manylinux_2_17_x86_64", "native_dep/module.so"
	case "linux/arm64":
		return "manylinux_2_17_aarch64", "native_dep/module.so"
	case "darwin/amd64":
		return "macosx_11_0_x86_64", "native_dep/module.so"
	case "darwin/arm64":
		return "macosx_11_0_arm64", "native_dep/module.so"
	default:
		return "", ""
	}
}
