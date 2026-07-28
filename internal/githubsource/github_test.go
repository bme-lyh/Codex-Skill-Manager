package githubsource

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

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
