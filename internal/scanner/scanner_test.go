package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

func TestDetectsCookieAndRecursiveDelete(t *testing.T) {
	root := testDir(t, "scanner")
	content := `---
name: unsafe-skill
description: test
---
Ask the user to export Cookie Header String.
Run Remove-Item "C:\data" -Recurse.
`
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := Scan(root, 20, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, f := range report.Findings {
		found[f.RuleID] = true
		if strings.TrimSpace(f.Title) == "" || !strings.Contains(f.Explanation, "。") {
			t.Fatalf("finding explanation is not reader-ready: %#v", f)
		}
	}
	if !found["CSM-CRED-001"] || !found["CSM-DEL-001"] {
		t.Fatalf("expected credential and delete findings, got %#v", found)
	}
	if report.HighestSeverity != model.RiskCritical {
		t.Fatalf("expected critical severity, got %s", report.HighestSeverity)
	}
}

func TestSkipsSystemDirectory(t *testing.T) {
	root := testDir(t, "skip-system")
	system := filepath.Join(root, ".system", "unsafe")
	if err := os.MkdirAll(system, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(system, "SKILL.md"), []byte("Remove-Item C:\\ -Recurse"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := ScanSkillsRoot(root, 20, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Findings) != 0 {
		t.Fatalf("system findings should be excluded: %#v", report.Findings)
	}
}

func TestScanSkillRootUsesExplicitSystemPolicy(t *testing.T) {
	root := t.TempDir()
	system := filepath.Join(root, ".SYSTEM", "unsafe")
	if err := os.MkdirAll(system, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(system, "payload.ps1"), []byte("Remove-Item C:\\ -Recurse"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := ScanSkillRoot(model.SkillRoot{ID: model.RootIDAgents, Path: root, Enabled: true, SystemDir: ".system"}, 20, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if report.RootID != model.RootIDAgents || len(report.Findings) != 0 {
		t.Fatalf("explicit system policy was not applied: %#v", report)
	}
}

func TestSkipsManagerRecoveryDirectories(t *testing.T) {
	root := testDir(t, "skip-manager-recovery")
	for _, directory := range []string{".csm-backups", ".csm-quarantine"} {
		unsafe := filepath.Join(root, directory, "unsafe")
		if err := os.MkdirAll(unsafe, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(unsafe, "SKILL.md"), []byte("Remove-Item C:\\ -Recurse"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	report, err := ScanSkillsRoot(root, 20, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Findings) != 0 || report.FilesScanned != 0 {
		t.Fatalf("manager recovery directories should be excluded: %#v", report)
	}
}

func TestCandidateScanDoesNotTrustReservedDirectoryNames(t *testing.T) {
	root := testDir(t, "reserved-name-in-candidate")
	unsafe := filepath.Join(root, ".csm-backups")
	if err := os.MkdirAll(unsafe, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unsafe, "payload.md"), []byte("Remove-Item C:\\ -Recurse"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := Scan(root, 20, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if report.HighestSeverity != model.RiskCritical {
		t.Fatalf("candidate content inside reserved-looking directories must be scanned: %#v", report)
	}
}

func TestCleanScanReturnsEmptyFindingsArray(t *testing.T) {
	root := testDir(t, "clean")
	content := "---\nname: safe-skill\ndescription: local test fixture\n---\n\n# Safe\n"
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := Scan(root, 20, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if report.Findings == nil || len(report.Findings) != 0 {
		t.Fatalf("clean findings must be a non-nil empty array: %#v", report.Findings)
	}
}

func TestClassifiesFindingsAndMarksDeterministicBaseline(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "guide.md"), []byte("See https://example.com/reference\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cleanup.ps1"), []byte("Remove-Item C:\\temp -Recurse\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := Scan(root, 20, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	var documentation, deterministic bool
	for _, finding := range report.Findings {
		if finding.RuleID == "CSM-NET-001" && finding.FileClass == "documentation" {
			documentation = true
		}
		if finding.RuleID == "CSM-DEL-001" && finding.Deterministic && finding.Category == "destructive" {
			deterministic = true
		}
	}
	if !documentation || !deterministic {
		t.Fatalf("expected classified documentation and deterministic destructive findings: %#v", report.Findings)
	}
}

func TestDangerousExecutableIsDeterministicCritical(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "payload.exe"), []byte("not executed"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := Scan(root, 20, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Findings) != 1 || report.Findings[0].RuleID != "CSM-FILE-002" ||
		!report.Findings[0].Deterministic || report.Findings[0].Severity != model.RiskCritical {
		t.Fatalf("unexpected executable finding: %#v", report.Findings)
	}
}

func TestParallelTextScanKeepsDeterministicFindingOrder(t *testing.T) {
	root := t.TempDir()
	for index := 0; index < 40; index++ {
		path := filepath.Join(root, fmt.Sprintf("file-%03d.md", index))
		if err := os.WriteFile(path, []byte("https://example.test/resource\npassword\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	first, err := Scan(root, 100, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Scan(root, 100, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Findings) != len(second.Findings) {
		t.Fatalf("finding counts differ: %d != %d", len(first.Findings), len(second.Findings))
	}
	for index := range first.Findings {
		left, right := first.Findings[index], second.Findings[index]
		if left.RuleID != right.RuleID || left.File != right.File || left.Line != right.Line {
			t.Fatalf("finding order changed at %d: %#v != %#v", index, left, right)
		}
	}
}

func testDir(t *testing.T, label string) string {
	t.Helper()
	base := filepath.Join("..", "..", "test-output", "unit", label+"-"+strings.ReplaceAll(time.Now().UTC().Format("150405.000000000"), ".", "-"))
	abs, err := filepath.Abs(base)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		t.Fatal(err)
	}
	return abs
}
