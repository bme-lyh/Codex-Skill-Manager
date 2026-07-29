package githubsource

import (
	"archive/zip"
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
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

func TestParseSupportedGitHubURLs(t *testing.T) {
	tests := []struct {
		raw, owner, repo, ref, path string
	}{
		{"https://github.com/owner/repo", "owner", "repo", "", ""},
		{"https://github.com/owner/repo.git", "owner", "repo", "", ""},
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

type zipFixtureEntry struct {
	name    string
	content string
	mode    os.FileMode
}

func buildZipFixture(t *testing.T, entries []zipFixtureEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Store}
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
