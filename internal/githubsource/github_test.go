package githubsource

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

func TestExtractZipWritesOnlyContainedEntries(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "staging")
	if err := os.MkdirAll(dest, 0o700); err != nil {
		t.Fatal(err)
	}
	data := buildZipFixture(t, []zipFixtureEntry{
		{name: "repo-root/", mode: os.ModeDir | 0o700},
		{name: "repo-root/SKILL.md", content: "---\nname: safe\n---\n"},
		{name: "repo-root/nested/file.txt", content: "expected"},
	})

	root, err := extractZip(data, dest)
	if err != nil {
		t.Fatal(err)
	}
	expectedRoot := filepath.Join(dest, "repo-root")
	if root != expectedRoot {
		t.Fatalf("root = %q, want %q", root, expectedRoot)
	}
	content, err := os.ReadFile(filepath.Join(expectedRoot, "nested", "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "expected" {
		t.Fatalf("content = %q", content)
	}
}

func TestExtractZipRejectsPathTraversalAndAbsolutePaths(t *testing.T) {
	tests := []struct {
		name  string
		entry string
	}{
		{name: "parent prefix", entry: "../escape.txt"},
		{name: "nested parent prefix", entry: "repo-root/../../escape.txt"},
		{name: "normalized parent element", entry: "repo-root/../escape.txt"},
		{name: "windows parent element", entry: `repo-root\..\escape.txt`},
		{name: "rooted path", entry: "/absolute.txt"},
		{name: "drive path", entry: "C:/absolute.txt"},
		{name: "current directory", entry: "."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent := t.TempDir()
			dest := filepath.Join(parent, "staging")
			if err := os.MkdirAll(dest, 0o700); err != nil {
				t.Fatal(err)
			}
			data := buildZipFixture(t, []zipFixtureEntry{{name: tt.entry, content: "untrusted"}})

			if _, err := extractZip(data, dest); err == nil {
				t.Fatalf("expected %q to be rejected", tt.entry)
			}
			if _, err := os.Stat(filepath.Join(parent, "escape.txt")); !os.IsNotExist(err) {
				t.Fatalf("archive wrote outside staging root: %v", err)
			}
		})
	}
}

func TestExtractZipRejectsSymbolicLinks(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "staging")
	if err := os.MkdirAll(dest, 0o700); err != nil {
		t.Fatal(err)
	}
	data := buildZipFixture(t, []zipFixtureEntry{
		{name: "repo-root/", mode: os.ModeDir | 0o700},
		{name: "repo-root/link", content: "target", mode: os.ModeSymlink | 0o600},
	})

	if _, err := extractZip(data, dest); err == nil {
		t.Fatal("expected symbolic link entry to be rejected")
	}
}

func TestExtractZipRejectsWindowsUnsafePaths(t *testing.T) {
	tests := []struct {
		name  string
		entry string
	}{
		{name: "alternate data stream", entry: "repo-root/file.txt:payload"},
		{name: "reserved device", entry: "repo-root/NUL.txt"},
		{name: "reserved numbered device", entry: "repo-root/com1.log"},
		{name: "trailing dot", entry: "repo-root/file.txt."},
		{name: "trailing space", entry: "repo-root/file.txt "},
		{name: "invalid character", entry: "repo-root/file<name>.txt"},
		{name: "backslash separator", entry: `repo-root\file.txt`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "staging")
			data := buildZipFixture(t, []zipFixtureEntry{
				{name: "repo-root/", mode: os.ModeDir | 0o700},
				{name: tt.entry, content: "untrusted"},
			})
			if _, err := extractZip(data, dest); err == nil {
				t.Fatalf("expected unsafe Windows path %q to be rejected", tt.entry)
			}
		})
	}
}

func TestExtractZipRejectsDuplicateAndConflictingPathsBeforeWriting(t *testing.T) {
	tests := []struct {
		name    string
		entries []zipFixtureEntry
	}{
		{
			name: "case-insensitive duplicate",
			entries: []zipFixtureEntry{
				{name: "repo-root/File.txt", content: "first"},
				{name: "repo-root/file.TXT", content: "second"},
			},
		},
		{
			name: "exact duplicate",
			entries: []zipFixtureEntry{
				{name: "repo-root/file.txt", content: "first"},
				{name: "repo-root/file.txt", content: "second"},
			},
		},
		{
			name: "file used as directory",
			entries: []zipFixtureEntry{
				{name: "repo-root/node", content: "file"},
				{name: "repo-root/node/child.txt", content: "child"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "staging")
			entries := append([]zipFixtureEntry{
				{name: "repo-root/", mode: os.ModeDir | 0o700},
			}, tt.entries...)
			data := buildZipFixture(t, entries)
			if _, err := extractZip(data, dest); err == nil {
				t.Fatal("expected ambiguous archive paths to be rejected")
			}
			if _, err := os.Stat(filepath.Join(dest, "repo-root")); !os.IsNotExist(err) {
				t.Fatalf("archive preflight wrote data before rejecting paths: %v", err)
			}
		})
	}
}

func TestExtractZipAllowsDotsInsideOrdinaryFilename(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "staging")
	data := buildZipFixture(t, []zipFixtureEntry{
		{name: "repo-root/", mode: os.ModeDir | 0o700},
		{name: "repo-root/file..name.txt", content: "safe"},
	})
	root, err := extractZip(data, dest)
	if err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(filepath.Join(root, "file..name.txt")); err != nil ||
		string(content) != "safe" {
		t.Fatalf("ordinary dotted filename was not extracted: content=%q err=%v", content, err)
	}
}

func TestExtractZipAcceptsExactLimits(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "staging")
	data := buildZipFixture(t, []zipFixtureEntry{
		{name: "repo-root/", mode: os.ModeDir | 0o700},
		{name: "repo-root/first.txt", content: "12345"},
		{name: "repo-root/second.txt", content: "67890"},
	})
	limits := zipExtractionLimits{MaxEntries: 3, MaxEntryBytes: 5, MaxTotalBytes: 10}

	root, err := extractZipWithLimits(data, dest, limits)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"first.txt", "second.txt"} {
		content, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if len(content) != 5 {
			t.Fatalf("%s extracted %d bytes, want 5", name, len(content))
		}
	}
}

func TestExtractZipRejectsEntryCountOverLimit(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "staging")
	data := buildZipFixture(t, []zipFixtureEntry{
		{name: "repo-root/", mode: os.ModeDir | 0o700},
		{name: "repo-root/first.txt", content: "a"},
		{name: "repo-root/second.txt", content: "b"},
	})
	limits := zipExtractionLimits{MaxEntries: 2, MaxEntryBytes: 10, MaxTotalBytes: 20}

	if _, err := extractZipWithLimits(data, dest, limits); err == nil ||
		!strings.Contains(err.Error(), "entry count exceeds limit") {
		t.Fatalf("expected entry-count limit error, got %v", err)
	}
}

func TestExtractZipRejectsSingleEntryOverLimit(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "staging")
	data := buildZipFixture(t, []zipFixtureEntry{
		{name: "repo-root/", mode: os.ModeDir | 0o700},
		{name: "repo-root/large.txt", content: "123456"},
	})
	limits := zipExtractionLimits{MaxEntries: 2, MaxEntryBytes: 5, MaxTotalBytes: 10}

	if _, err := extractZipWithLimits(data, dest, limits); err == nil ||
		!strings.Contains(err.Error(), "entry") ||
		!strings.Contains(err.Error(), "uncompressed size limit") {
		t.Fatalf("expected single-entry size limit error, got %v", err)
	}
}

func TestExtractZipRejectsTotalBytesOverLimit(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "staging")
	data := buildZipFixture(t, []zipFixtureEntry{
		{name: "repo-root/", mode: os.ModeDir | 0o700},
		{name: "repo-root/first.txt", content: "1234"},
		{name: "repo-root/second.txt", content: "5678"},
	})
	limits := zipExtractionLimits{MaxEntries: 3, MaxEntryBytes: 4, MaxTotalBytes: 7}

	if _, err := extractZipWithLimits(data, dest, limits); err == nil ||
		!strings.Contains(err.Error(), "total limit") {
		t.Fatalf("expected total size limit error, got %v", err)
	}
}

func TestExtractZipRejectsCompressedBomb(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "staging")
	uncompressed := strings.Repeat("A", 1<<20)
	data := buildZipFixture(t, []zipFixtureEntry{
		{name: "repo-root/", mode: os.ModeDir | 0o700},
		{name: "repo-root/bomb.txt", content: uncompressed, method: zip.Deflate},
	})
	if len(data) >= len(uncompressed)/10 {
		t.Fatalf("fixture is not meaningfully compressed: archive=%d, content=%d", len(data), len(uncompressed))
	}
	limits := zipExtractionLimits{
		MaxEntries:    2,
		MaxEntryBytes: 64 << 10,
		MaxTotalBytes: 2 << 20,
	}

	if _, err := extractZipWithLimits(data, dest, limits); err == nil ||
		!strings.Contains(err.Error(), "uncompressed size limit") {
		t.Fatalf("expected compressed bomb to hit extraction limit, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "repo-root", "bomb.txt")); !os.IsNotExist(err) {
		t.Fatalf("compressed bomb left an extracted file: %v", err)
	}

	// Exercise the streaming guard directly as well as the metadata preflight
	// above. A highly compressed entry must stop after limit+1 output bytes,
	// rather than expanding its full body before the limit is checked.
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var bomb *zip.File
	for _, file := range reader.File {
		if file.Name == "repo-root/bomb.txt" {
			bomb = file
			break
		}
	}
	if bomb == nil {
		t.Fatal("compressed bomb fixture entry not found")
	}
	streamingRoot := filepath.Join(t.TempDir(), "streaming")
	if err := os.MkdirAll(streamingRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	streamingTarget := filepath.Join(streamingRoot, "bomb.txt")
	if _, err := extractZipFile(bomb, streamingTarget, 0, limits); err == nil ||
		!strings.Contains(err.Error(), "uncompressed size limit") {
		t.Fatalf("expected streaming extraction limit error, got %v", err)
	}
	if _, err := os.Stat(streamingTarget); !os.IsNotExist(err) {
		t.Fatalf("streaming guard left a partial file: %v", err)
	}
}

func TestParseSupportedGitHubURLs(t *testing.T) {
	tests := []struct {
		raw, owner, repo, ref, path string
	}{
		{"https://github.com/owner/repo", "owner", "repo", "", ""},
		{"https://github.com/owner/repo.git", "owner", "repo", "", ""},
		{"https://github.com/owner/repo.git/", "owner", "repo", "", ""},
		{"https://github.com/owner/repo/tree/main/skills/example", "owner", "repo", "main", "skills/example"},
		{"https://github.com/owner/repo/blob/v1/skills/example/SKILL.md", "owner", "repo", "v1", "skills/example"},
	}
	for _, tt := range tests {
		got, err := Parse(tt.raw)
		if err != nil {
			t.Fatalf("%s: %v", tt.raw, err)
		}
		if got.Owner != tt.owner || got.Repo != tt.repo || got.Ref != tt.ref || got.SourcePath != tt.path {
			t.Fatalf("%s: %#v", tt.raw, got)
		}
	}
}

func TestParseRejectsNonGitHubAndSSH(t *testing.T) {
	for _, raw := range []string{"https://example.com/a/b", "http://github.com/a/b", "git@github.com:a/b.git"} {
		if _, err := Parse(raw); err == nil {
			t.Fatalf("expected rejection: %s", raw)
		}
	}
}

func TestParseRejectsEncodedAndWindowsSourcePathEscapes(t *testing.T) {
	tests := []string{
		"https://github.com/owner/repo/tree/main/%2e%2e/secrets",
		"https://github.com/owner/repo/tree/main/skills/%2E%2E/secrets",
		"https://github.com/owner/repo/tree/main/skills%5cexample",
		"https://github.com/owner/repo/tree/main/C:%2fWindows",
		"https://github.com/owner/repo/tree/main/file.txt%3Apayload",
		"https://github.com/owner/repo/tree/main/NUL.txt",
		"https://github.com/owner/repo/tree/main/skill.",
		"https://github.com/owner/repo/tree/%2e%2e/skills",
		"https://github.com/%2e%2e/repo",
		"https://github.com/owner/repo/issues",
	}
	for _, raw := range tests {
		if _, err := Parse(raw); err == nil {
			t.Fatalf("expected unsafe encoded source path to be rejected: %s", raw)
		}
	}
}

func TestAPIErrorCapturesRateLimitReset(t *testing.T) {
	reset := time.Now().UTC().Add(10 * time.Minute).Unix()
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Status:     "403 Forbidden",
		Header:     make(http.Header),
	}
	resp.Header.Set("X-RateLimit-Limit", "60")
	resp.Header.Set("X-RateLimit-Remaining", "0")
	resp.Header.Set("X-RateLimit-Reset", strconv.FormatInt(reset, 10))
	rateErr, ok := apiError(resp, []byte(`{"message":"API rate limit exceeded"}`)).(*APIError)
	if !ok || rateErr.RetryAt == nil || rateErr.Remaining != 0 || rateErr.Limit != 60 {
		t.Fatalf("unexpected rate limit error: %#v", rateErr)
	}
}

func TestDownloadArchiveReturnsStructuredRateLimitError(t *testing.T) {
	reset := time.Now().UTC().Add(10 * time.Minute).Unix()
	attempts := 0
	client := &Client{http: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		header := make(http.Header)
		header.Set("X-RateLimit-Limit", "60")
		header.Set("X-RateLimit-Remaining", "0")
		header.Set("X-RateLimit-Reset", strconv.FormatInt(reset, 10))
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Status:     "403 Forbidden",
			Header:     header,
			Body: io.NopCloser(strings.NewReader(
				`{"message":"API rate limit exceeded"}`,
			)),
		}, nil
	})}}

	_, err := client.Download(
		context.Background(),
		model.Repository{FullName: "owner/repo", CommitSHA: strings.Repeat("a", 40)},
		filepath.Join(t.TempDir(), "staging"),
		1<<20,
	)
	var rateErr *APIError
	if !errors.As(err, &rateErr) {
		t.Fatalf("expected structured APIError, got %T: %v", err, err)
	}
	if rateErr.StatusCode != http.StatusForbidden || rateErr.Limit != 60 ||
		rateErr.Remaining != 0 || rateErr.ResetAt == nil || rateErr.RetryAt == nil {
		t.Fatalf("unexpected archive rate-limit details: %#v", rateErr)
	}
	if attempts != 1 {
		t.Fatalf("long rate-limit delay must not be retried automatically; attempts=%d", attempts)
	}
}

func TestDownloadArchiveRetriesTransientFailureAndExtracts(t *testing.T) {
	data := buildZipFixture(t, []zipFixtureEntry{
		{name: "repo-root/", mode: os.ModeDir | 0o700},
		{name: "repo-root/SKILL.md", content: "---\nname: safe\n---\n"},
	})
	attempts := 0
	client := &Client{http: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			header := make(http.Header)
			header.Set("Retry-After", "0")
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Status:     "503 Service Unavailable",
				Header:     header,
				Body:       io.NopCloser(strings.NewReader(`{"message":"try again"}`)),
			}, nil
		}
		return &http.Response{
			StatusCode:    http.StatusOK,
			Status:        "200 OK",
			Header:        make(http.Header),
			Body:          io.NopCloser(bytes.NewReader(data)),
			ContentLength: int64(len(data)),
		}, nil
	})}}

	root, err := client.Download(
		context.Background(),
		model.Repository{FullName: "owner/repo", CommitSHA: strings.Repeat("a", 40)},
		filepath.Join(t.TempDir(), "staging"),
		1<<20,
	)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("attempts=%d, want 2", attempts)
	}
	if _, err := os.Stat(filepath.Join(root, "SKILL.md")); err != nil {
		t.Fatalf("retried archive was not extracted: %v", err)
	}
}

func TestDownloadArchiveRetriesAreBounded(t *testing.T) {
	attempts := 0
	client := &Client{http: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		header := make(http.Header)
		header.Set("Retry-After", "0")
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Status:     "503 Service Unavailable",
			Header:     header,
			Body:       io.NopCloser(strings.NewReader(`{"message":"try again"}`)),
		}, nil
	})}}

	_, err := client.Download(
		context.Background(),
		model.Repository{FullName: "owner/repo", CommitSHA: strings.Repeat("a", 40)},
		filepath.Join(t.TempDir(), "staging"),
		1<<20,
	)
	var responseErr *APIError
	if !errors.As(err, &responseErr) ||
		responseErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected final structured transient error, got %T: %v", err, err)
	}
	if attempts != maxArchiveDownloadAttempts {
		t.Fatalf("attempts=%d, want bounded maximum %d", attempts, maxArchiveDownloadAttempts)
	}
}

func TestDiscoverDeduplicatesIdenticalSkillsAndPrefersCanonicalPath(t *testing.T) {
	root := t.TempDir()
	writeDiscoverySkill(t, filepath.Join(root, "skills", "academic-research-suite"), "academic-research-suite", "same")
	writeDiscoverySkill(t, filepath.Join(root, "plugins", "ars-codex", "skills", "academic-research-suite"), "academic-research-suite", "same")

	candidates, err := Discover(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected one deduplicated candidate, got %#v", candidates)
	}
	if candidates[0].SourcePath != "skills/academic-research-suite" {
		t.Fatalf("expected canonical shortest path, got %s", candidates[0].SourcePath)
	}
}

func TestDiscoverRejectsDifferentSkillsWithSameName(t *testing.T) {
	root := t.TempDir()
	writeDiscoverySkill(t, filepath.Join(root, "skills", "one"), "duplicate", "first")
	writeDiscoverySkill(t, filepath.Join(root, "plugins", "two"), "duplicate", "second")

	if _, err := Discover(root, ""); err == nil {
		t.Fatal("expected ambiguous duplicate Skill names to be rejected")
	}
}

func TestDiscoverSkipsGeneratedAndDependencyDirectories(t *testing.T) {
	root := t.TempDir()
	writeDiscoverySkill(t, filepath.Join(root, "skills", "kept"), "kept", "expected")
	for _, directory := range []string{
		".git", ".hg", ".nox", ".svn", ".tox", ".venv", "__pycache__",
		"build", "dist", "env", "node_modules", "venv", "vendor",
	} {
		writeDiscoverySkill(
			t,
			filepath.Join(root, directory, "ignored"),
			"ignored-"+strings.TrimLeft(directory, "."),
			"should not be discovered",
		)
	}

	candidates, err := Discover(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Name != "kept" {
		t.Fatalf("expected only the relevant Skill, got %#v", candidates)
	}
}

func TestDiscoverRejectsSourcePathEscapesAtBoundary(t *testing.T) {
	root := t.TempDir()
	for _, sourcePath := range []string{
		"../outside",
		"skills/../outside",
		`skills\..\outside`,
		"C:/Windows",
		"NUL",
		"skill.",
	} {
		if _, err := Discover(root, sourcePath); err == nil {
			t.Fatalf("expected source path to be rejected at discovery boundary: %q", sourcePath)
		}
	}
}

func TestDiscoverRejectsSymbolicLinkSourcePath(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "real")
	writeDiscoverySkill(t, target, "real", "fixture")
	link := filepath.Join(root, "linked")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symbolic links are unavailable in this environment: %v", err)
	}
	if _, err := Discover(root, "linked"); err == nil ||
		!strings.Contains(strings.ToLower(err.Error()), "symbolic") {
		t.Fatalf("expected symbolic-link source path rejection, got %v", err)
	}
}

type zipFixtureEntry struct {
	name    string
	content string
	mode    os.FileMode
	method  uint16
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func buildZipFixture(t *testing.T, entries []zipFixtureEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, entry := range entries {
		method := entry.method
		if method == 0 {
			method = zip.Store
		}
		header := &zip.FileHeader{Name: entry.name, Method: method}
		if entry.mode != 0 {
			header.SetMode(entry.mode)
		}
		file, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(entry.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func writeDiscoverySkill(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: fixture\n---\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
