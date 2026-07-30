package manager

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func approvedTestProjectRoot(t *testing.T, path string) string {
	t.Helper()
	approved, err := validateAssistedProjectRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	return approved
}

func TestValidateAssistedProjectRootIsStableAfterApproval(t *testing.T) {
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	first := approvedTestProjectRoot(t, project)
	second := approvedTestProjectRoot(t, first)
	if !strings.EqualFold(first, second) {
		t.Fatalf("approved project root was not stable: first %q, second %q", first, second)
	}
}

func TestConfigureManagedMCPBacksUpAndRestoresConfig(t *testing.T) {
	m := newTestManager(t)
	codexHome := filepath.Join(t.TempDir(), "codex")
	t.Setenv("CODEX_HOME", codexHome)
	configPath := filepath.Join(codexHome, "config.toml")
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		t.Fatal(err)
	}
	original := "model = \"default\"\n"
	if err := os.WriteFile(configPath, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	command := filepath.Join(m.Config.Paths.DataRoot, "tools", "graph", "Scripts", "graph.exe")
	if err := os.MkdirAll(filepath.Dir(command), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(command, []byte("fixture"), 0o700); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	project = approvedTestProjectRoot(t, project)
	before, err := fileFingerprint(configPath)
	if err != nil {
		t.Fatal(err)
	}
	mutation, err := m.configureManagedMCP(
		"assist-plan-1", "tx-1", "code_review_graph", command, []string{"serve"}, project, before,
		func(mcpMutation) error { return nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `[mcp_servers."code_review_graph"]`) ||
		!strings.Contains(text, "cwd = ") || !strings.Contains(text, strconv.Quote(command)) {
		t.Fatalf("managed MCP block missing from config:\n%s", text)
	}
	if mutation.BackupPath == "" || mutation.AppliedHash == "" || mutation.OriginalMissing {
		t.Fatalf("incomplete mutation record: %#v", mutation)
	}
	if err := restoreMCPConfig(m.Config.Paths.QuarantineRoot, "tx-1", mutation); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != original {
		t.Fatalf("config was not restored exactly: %q", restored)
	}
}

func TestConfigureManagedMCPRejectsDriftAndExistingUnownedServer(t *testing.T) {
	m := newTestManager(t)
	codexHome := filepath.Join(t.TempDir(), "codex")
	t.Setenv("CODEX_HOME", codexHome)
	configPath := filepath.Join(codexHome, "config.toml")
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		t.Fatal(err)
	}
	// A semantic dotted-key declaration must conflict even though it has no
	// literal [mcp_servers.<name>] section header.
	existing := "mcp_servers.code_review_graph = { command = \"other.exe\", args = [\"serve\"], cwd = \".\" }\n"
	if err := os.WriteFile(configPath, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	command := filepath.Join(m.Config.Paths.DataRoot, "tools", "graph", "Scripts", "graph.exe")
	if err := os.MkdirAll(filepath.Dir(command), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(command, []byte("fixture"), 0o700); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	project = approvedTestProjectRoot(t, project)
	hash, err := fileFingerprint(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.configureManagedMCP(
		"assist-plan-1", "tx-1", "code_review_graph", command, []string{"serve"}, project, hash,
		func(mcpMutation) error { return nil },
	); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected an unowned server collision, got %v", err)
	}
	if _, err := m.configureManagedMCP(
		"assist-plan-1", "tx-1", "another_server", command, []string{"serve"}, project, "stale",
		func(mcpMutation) error { return nil },
	); err == nil || !strings.Contains(err.Error(), "changed after analysis") {
		t.Fatalf("expected config drift to be rejected, got %v", err)
	}
}

func TestConfigureManagedMCPPersistsIntentBeforeWritingConfig(t *testing.T) {
	m := newTestManager(t)
	codexHome := filepath.Join(t.TempDir(), "codex")
	t.Setenv("CODEX_HOME", codexHome)
	configPath := filepath.Join(codexHome, "config.toml")
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		t.Fatal(err)
	}
	original := "model = \"default\"\n"
	if err := os.WriteFile(configPath, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	command := filepath.Join(m.Config.Paths.DataRoot, "tools", "graph", "Scripts", "graph.exe")
	if err := os.MkdirAll(filepath.Dir(command), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(command, []byte("fixture"), 0o700); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	project = approvedTestProjectRoot(t, project)
	before, err := fileFingerprint(configPath)
	if err != nil {
		t.Fatal(err)
	}
	checkpointCalled := false
	_, err = m.configureManagedMCP(
		"assist-plan-1",
		"tx-1",
		"code_review_graph",
		command,
		[]string{"serve"},
		project,
		before,
		func(intent mcpMutation) error {
			checkpointCalled = true
			if intent.ConfigPath != configPath || intent.AppliedHash == "" ||
				intent.ManifestPath == "" || intent.BackupPath == "" {
				t.Fatalf("incomplete MCP write-ahead intent: %#v", intent)
			}
			current, readErr := os.ReadFile(configPath)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(current) != original {
				t.Fatalf("configuration changed before the intent checkpoint: %q", current)
			}
			return os.ErrPermission
		},
	)
	if err == nil || !strings.Contains(err.Error(), "persist MCP configuration intent") {
		t.Fatalf("expected checkpoint failure, got %v", err)
	}
	if !checkpointCalled {
		t.Fatal("MCP write-ahead checkpoint was not called")
	}
	current, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != original {
		t.Fatalf("configuration changed after a failed intent checkpoint: %q", current)
	}
}

func TestConfigureManagedMCPRefusesConfigDriftAfterCheckpoint(t *testing.T) {
	m := newTestManager(t)
	codexHome := filepath.Join(t.TempDir(), "codex")
	t.Setenv("CODEX_HOME", codexHome)
	configPath := filepath.Join(codexHome, "config.toml")
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		t.Fatal(err)
	}
	original := "model = \"default\"\n"
	if err := os.WriteFile(configPath, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	command := filepath.Join(m.Config.Paths.DataRoot, "tools", "graph", "Scripts", "graph.exe")
	if err := os.MkdirAll(filepath.Dir(command), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(command, []byte("fixture"), 0o700); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	project = approvedTestProjectRoot(t, project)
	before, err := fileFingerprint(configPath)
	if err != nil {
		t.Fatal(err)
	}
	concurrent := "model = \"user-changed-during-checkpoint\"\n"
	_, err = m.configureManagedMCP(
		"assist-plan-1",
		"tx-checkpoint-drift",
		"code_review_graph",
		command,
		[]string{"serve"},
		project,
		before,
		func(mcpMutation) error {
			return os.WriteFile(configPath, []byte(concurrent), 0o600)
		},
	)
	if err == nil || !strings.Contains(err.Error(), "changed after the transaction checkpoint") {
		t.Fatalf("expected checkpoint drift to stop the write, got %v", err)
	}
	current, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(current) != concurrent {
		t.Fatalf("concurrent Codex configuration was overwritten: %q", current)
	}
	manifestPath := filepath.Join(
		m.Config.Paths.DataRoot,
		"integrations",
		"mcp",
		"code_review_graph.json",
	)
	if _, statErr := os.Stat(manifestPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("ownership manifest was written despite config drift: %v", statErr)
	}
}

func TestRestoreManagedMCPRefusesToOverwriteLaterChanges(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.toml")
	backupPath := filepath.Join(root, "backup.toml")
	if err := os.WriteFile(configPath, []byte("installed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backupPath, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	applied, err := fileFingerprint(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("user changed later"), 0o600); err != nil {
		t.Fatal(err)
	}
	err = restoreMCPConfig(filepath.Join(root, "quarantine"), "tx-1", mcpMutation{
		ConfigPath: configPath, BackupPath: backupPath, AppliedHash: applied,
	})
	if err == nil || !strings.Contains(err.Error(), "changed after installation") {
		t.Fatalf("expected drift-aware recovery refusal, got %v", err)
	}
}

func TestCopySingleFileRefusesToReplaceExistingBackup(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.toml")
	target := filepath.Join(root, "backup.toml")
	if err := os.WriteFile(source, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := copySingleFile(source, target); err == nil {
		t.Fatal("expected an existing backup path to be rejected")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "existing" {
		t.Fatalf("existing backup was overwritten: %q", data)
	}
}
