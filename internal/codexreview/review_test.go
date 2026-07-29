package codexreview

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

func TestExecutableCandidatesKeepsLaterIndependentCLI(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific CLI discovery")
	}
	protected := t.TempDir()
	independent := t.TempDir()
	for _, path := range []string{
		filepath.Join(protected, "codex.exe"),
		filepath.Join(independent, "codex.cmd"),
	} {
		if err := os.WriteFile(path, []byte("@echo off\r\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	candidates := executableCandidates(
		protected+string(os.PathListSeparator)+independent,
		"windows",
		"",
	)
	if len(candidates) != 2 {
		t.Fatalf("expected both PATH candidates, got %v", candidates)
	}
	if candidates[1] != filepath.Join(independent, "codex.cmd") {
		t.Fatalf("expected later independent CLI candidate, got %v", candidates)
	}
}

func TestExecutableCandidatesAddsDefaultNPMDirectory(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific CLI discovery")
	}
	home := t.TempDir()
	npmDir := filepath.Join(home, "AppData", "Roaming", "npm")
	if err := os.MkdirAll(npmDir, 0o700); err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(npmDir, "codex.cmd")
	if err := os.WriteFile(expected, []byte("@echo off\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	candidates := executableCandidates("", "windows", home)
	if len(candidates) != 1 || candidates[0] != expected {
		t.Fatalf("expected default npm CLI %q, got %v", expected, candidates)
	}
}

func TestReviewArgsPlacesGlobalOptionsBeforeExec(t *testing.T) {
	args := reviewArgs(model.CodexReviewConfig{
		Model: "default", ReasoningEffort: "medium",
	}, "schema.json", "result.json")
	execIndex := slices.Index(args, "exec")
	approvalIndex := slices.Index(args, "--ask-for-approval")
	sandboxIndex := slices.Index(args, "--sandbox")
	ephemeralIndex := slices.Index(args, "--ephemeral")
	if execIndex < 0 {
		t.Fatal("exec subcommand missing")
	}
	if approvalIndex < 0 || approvalIndex > execIndex {
		t.Fatalf("approval option must precede exec: %v", args)
	}
	if sandboxIndex < 0 || sandboxIndex > execIndex {
		t.Fatalf("sandbox option must precede exec: %v", args)
	}
	if ephemeralIndex < execIndex {
		t.Fatalf("exec option must follow exec subcommand: %v", args)
	}
}

func TestMissingCapabilitiesUsesFlagsNotVersion(t *testing.T) {
	rootHelp := "--config --model --sandbox --ask-for-approval"
	execHelp := "--ephemeral --skip-git-repo-check --ignore-user-config --ignore-rules --output-schema --output-last-message"
	if missing := missingCapabilitiesFromHelp(rootHelp, execHelp); len(missing) != 0 {
		t.Fatalf("expected compatible capability set, got %v", missing)
	}
	missing := missingCapabilitiesFromHelp(rootHelp, strings.ReplaceAll(execHelp, "--output-schema", ""))
	if !slices.Contains(missing, "exec --output-schema") {
		t.Fatalf("expected missing output-schema capability, got %v", missing)
	}
}

func TestParseModelCatalogOnlyReturnsListedModels(t *testing.T) {
	options, err := parseModelCatalog([]byte(`{"models":[
		{"slug":"gpt-current","display_name":"GPT Current","description":"Current model","visibility":"list","default_reasoning_level":"medium","supported_reasoning_levels":[{"effort":"low","description":"Fast"},{"effort":"medium","description":"Balanced"}]},
		{"slug":"internal-review","display_name":"Internal","visibility":"hidden"},
		{"slug":"","display_name":"Invalid","visibility":"list"}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 1 {
		t.Fatalf("expected one listed model, got %#v", options)
	}
	if options[0].Slug != "gpt-current" || options[0].DisplayName != "GPT Current" {
		t.Fatalf("unexpected model: %#v", options[0])
	}
	if len(options[0].ReasoningLevels) != 2 || options[0].ReasoningLevels[1].Effort != "medium" {
		t.Fatalf("unexpected reasoning levels: %#v", options[0].ReasoningLevels)
	}
}

func TestParseModelCatalogRejectsInvalidJSON(t *testing.T) {
	if _, err := parseModelCatalog([]byte(`not-json`)); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func TestCountContextFilesUsesCompleteTargetButSkipsManagerOwnedSystemData(t *testing.T) {
	root := t.TempDir()
	for _, relative := range []string{
		"SKILL.md",
		filepath.Join("src", "main.go"),
		filepath.Join(".system", "internal", "SKILL.md"),
		filepath.Join(".csm-backups", "old", "SKILL.md"),
	} {
		path := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	count, err := countContextFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected complete user target without manager-owned directories, got %d files", count)
	}
}
