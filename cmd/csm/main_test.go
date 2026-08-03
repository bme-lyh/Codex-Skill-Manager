package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/config"
	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

func TestRunCLIRejectsInvalidCommandUsage(t *testing.T) {
	configPath := writeCLIConfig(t)
	originalConfigureSchedule := configureSchedule
	scheduleCalls := 0
	configureSchedule = func(string, string, string, string, bool) error {
		scheduleCalls++
		return nil
	}
	t.Cleanup(func() {
		configureSchedule = originalConfigureSchedule
	})
	tests := []struct {
		name string
		args []string
	}{
		{name: "audit", args: []string{"audit", "--unknown"}},
		{name: "restore parse", args: []string{"restore", "--skill", "demo", "--transaction"}},
		{name: "restore required", args: []string{"restore", "--skill", "demo"}},
		{name: "rollback parse", args: []string{"rollback", "--unknown"}},
		{name: "rollback required", args: []string{"rollback"}},
		{name: "schedule parse", args: []string{"schedule", "--enabled=maybe"}},
		{name: "schedule frequency", args: []string{"schedule", "--frequency=hourly"}},
		{name: "schedule time", args: []string{"schedule", "--at=tomorrow"}},
		{name: "assisted plan consent missing scan", args: []string{"install", "--assist", "--create-plan"}},
		{name: "assisted plan consent missing flag", args: []string{"install", "--assist", "--project-scan-id", "project-scan-demo"}},
		{name: "assessment missing plan", args: []string{"install", "--assess"}},
		{name: "assessment mixed with apply", args: []string{"install", "--plan-id", "plan-demo", "--assess", "--apply", "--skill", "demo"}},
		{name: "assisted plan consent mixed source", args: []string{
			"install", "--assist", "--project-scan-id", "project-scan-demo", "--create-plan",
			"--local", `D:\skills`,
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string{"--config", configPath, "--json"}, test.args...)
			code, stdout, stderr := executeCLI(args)
			if code != 2 {
				t.Fatalf("exit code = %d, want 2; stdout=%q stderr=%q", code, stdout, stderr)
			}
			if stderr != "" {
				t.Fatalf("JSON mode wrote stderr: %q", stderr)
			}
			var response envelope
			if err := json.Unmarshal([]byte(stdout), &response); err != nil {
				t.Fatalf("decode JSON output: %v; output=%q", err, stdout)
			}
			if response.Status != "error" || strings.TrimSpace(response.Error) == "" {
				t.Fatalf("unexpected error envelope: %#v", response)
			}
		})
	}
	if scheduleCalls != 0 {
		t.Fatalf("invalid schedule usage invoked the scheduler %d time(s)", scheduleCalls)
	}
}

func TestRunCLIAssessesExistingInstallPlan(t *testing.T) {
	configPath := writeCLIConfig(t)
	source := filepath.Join(t.TempDir(), "source")
	skillRoot := filepath.Join(source, "demo")
	if err := os.MkdirAll(skillRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte("---\nname: demo\ndescription: demo skill\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := executeCLI([]string{"--config", configPath, "--json", "install", "--local", source})
	if code != 0 || stderr != "" {
		t.Fatalf("create plan failed: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	var planResponse struct {
		Data model.InstallPreview `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &planResponse); err != nil || planResponse.Data.ID == "" {
		t.Fatalf("decode plan response: %v; output=%q", err, stdout)
	}
	code, stdout, stderr = executeCLI([]string{
		"--config", configPath, "--json", "install", "--plan-id", planResponse.Data.ID, "--assess",
	})
	if code != 0 || stderr != "" {
		t.Fatalf("assess plan failed: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	var assessmentResponse struct {
		Data model.ProjectAssessment `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &assessmentResponse); err != nil {
		t.Fatalf("decode assessment response: %v; output=%q", err, stdout)
	}
	if assessmentResponse.Data.SourcePlanID != planResponse.Data.ID || assessmentResponse.Data.Gate == "" {
		t.Fatalf("unexpected assessment response: %#v", assessmentResponse.Data)
	}
}

func TestRunCLIPreparesInstallForExplicitAgentsRoot(t *testing.T) {
	configPath := writeCLIConfig(t)
	source := filepath.Join(t.TempDir(), "source")
	skillRoot := filepath.Join(source, "demo")
	if err := os.MkdirAll(skillRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte("---\nname: demo\ndescription: demo skill\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := executeCLI([]string{
		"--config", configPath, "--json", "install", "--root", model.RootIDAgents, "--local", source,
	})
	if code != 0 || stderr != "" {
		t.Fatalf("create agents plan failed: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	var response struct {
		Data model.InstallPreview `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &response); err != nil {
		t.Fatalf("decode plan response: %v; output=%q", err, stdout)
	}
	if response.Data.TargetRootID != model.RootIDAgents {
		t.Fatalf("targetRootId = %q, want %q", response.Data.TargetRootID, model.RootIDAgents)
	}
}

func TestReportDecisionEligibilityFailsClosedForUnknownSeverity(t *testing.T) {
	tests := []struct {
		severity model.RiskSeverity
		restore  bool
		eligible bool
	}{
		{model.RiskInfo, false, true},
		{model.RiskLow, false, true},
		{model.RiskMedium, false, true},
		{model.RiskHigh, false, false},
		{model.RiskCritical, false, false},
		{model.RiskHigh, true, true},
		{model.RiskCritical, true, true},
		{model.RiskSeverity("future"), false, false},
		{model.RiskSeverity("future"), true, false},
	}
	for _, test := range tests {
		if got := reportDecisionEligible(test.severity, test.restore); got != test.eligible {
			t.Fatalf("reportDecisionEligible(%q, %v) = %v, want %v", test.severity, test.restore, got, test.eligible)
		}
	}
}

func TestRunCLIRejectsUnknownCommandBeforeOpeningState(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "missing", "config.yaml")
	code, stdout, stderr := executeCLI([]string{
		"--config", configPath,
		"--json",
		"not-a-command",
	})
	if code != 2 {
		t.Fatalf("exit code = %d, want 2; stdout=%q stderr=%q", code, stdout, stderr)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("unknown command opened or created state: %v", err)
	}
}

func TestRunCLIVersionJSON(t *testing.T) {
	code, stdout, stderr := executeCLI([]string{"version", "--json"})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stdout=%q stderr=%q", code, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("version wrote stderr: %q", stderr)
	}
	var response struct {
		SchemaVersion string            `json:"schemaVersion"`
		Command       string            `json:"command"`
		Status        string            `json:"status"`
		Data          map[string]string `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &response); err != nil {
		t.Fatalf("decode JSON output: %v; output=%q", err, stdout)
	}
	if response.SchemaVersion != "1.0" || response.Command != "version" || response.Status != "ok" {
		t.Fatalf("unexpected version envelope: %#v", response)
	}
	if response.Data["version"] != model.Version {
		t.Fatalf("version = %q, want %q", response.Data["version"], model.Version)
	}
}

func TestRunCLISeparatesOperationalFailureFromInvalidUsage(t *testing.T) {
	configPath := writeCLIConfig(t)
	code, stdout, stderr := executeCLI([]string{
		"--config", configPath,
		"--json",
		"rollback", "--transaction", "missing-transaction",
	})
	if code != 1 {
		t.Fatalf("exit code = %d, want 1; stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func executeCLI(args []string) (int, string, string) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runCLI(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func writeCLIConfig(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	cfg, err := config.Default()
	if err != nil {
		t.Fatal(err)
	}
	dataRoot := filepath.Join(root, "data")
	cfg.Paths = model.Paths{
		SkillsRoot:     filepath.Join(root, "skills"),
		DataRoot:       dataRoot,
		LogsRoot:       filepath.Join(dataRoot, "logs"),
		ReportsRoot:    filepath.Join(dataRoot, "reports"),
		BackupsRoot:    filepath.Join(dataRoot, "backups"),
		QuarantineRoot: filepath.Join(dataRoot, "quarantine"),
		CacheRoot:      filepath.Join(dataRoot, "cache"),
		StagingRoot:    filepath.Join(dataRoot, "staging"),
	}
	cfg.SkillRoots = []model.SkillRoot{
		{ID: model.RootIDCodexDefault, Name: "Codex Skills", Kind: "codex", Path: cfg.Paths.SkillsRoot, Enabled: true, SystemDir: ".system"},
		{ID: model.RootIDAgents, Name: "Agents Skills", Kind: "agents", Path: filepath.Join(root, "agents-skills"), Enabled: true, SystemDir: ".system"},
	}
	cfg.Roots = append([]model.SkillRoot(nil), cfg.SkillRoots...)
	cfg.DefaultRootID = model.RootIDCodexDefault
	configPath := filepath.Join(root, "config.yaml")
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	return configPath
}
