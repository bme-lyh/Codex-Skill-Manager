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
	configPath := filepath.Join(root, "config.yaml")
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	return configPath
}
