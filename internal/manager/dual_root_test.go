package manager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/config"
	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

func TestDualRootInstallDefaultsToCodexAndBindsPreview(t *testing.T) {
	m, codexRoot, agentsRoot := newDualRootTestManager(t)
	sourceRoot := filepath.Join(filepath.Dir(m.Config.Paths.DataRoot), "package")
	writeTestSkill(t, sourceRoot, "demo")

	preview, err := m.PrepareLocal(sourceRoot)
	if err != nil {
		t.Fatal(err)
	}
	if preview.TargetRootID != model.RootIDCodexDefault {
		t.Fatalf("default target root = %q, want %q", preview.TargetRootID, model.RootIDCodexDefault)
	}
	if _, err := m.ApplyInstall(preview.ID, []string{"demo"}, false, model.RootIDAgents); err == nil ||
		!strings.Contains(err.Error(), "target root does not match") {
		t.Fatalf("apply accepted a root different from the sealed preview: %v", err)
	}
	if _, err := os.Stat(codexRoot); !os.IsNotExist(err) {
		t.Fatalf("rejected apply created or changed the Codex root: %v", err)
	}
	if _, err := os.Stat(agentsRoot); !os.IsNotExist(err) {
		t.Fatalf("rejected apply created or changed the Agents root: %v", err)
	}
	tampered := preview
	tampered.TargetRootID = model.RootIDAgents
	m.previews[preview.ID] = tampered
	if _, err := m.ApplyInstall(preview.ID, []string{"demo"}, false); err == nil ||
		!strings.Contains(err.Error(), "integrity digest mismatch") {
		t.Fatalf("apply accepted a target root changed after preview sealing: %v", err)
	}
	m.previews[preview.ID] = preview

	tx, err := m.ApplyInstall(preview.ID, []string{"demo"}, false, model.RootIDCodexDefault)
	if err != nil {
		t.Fatal(err)
	}
	if tx.RootID != model.RootIDCodexDefault {
		t.Fatalf("install transaction root = %q", tx.RootID)
	}
	if _, err := os.Stat(filepath.Join(codexRoot, "demo", "SKILL.md")); err != nil {
		t.Fatalf("default install did not write the Codex root: %v", err)
	}
	if _, err := os.Stat(agentsRoot); !os.IsNotExist(err) {
		t.Fatalf("default install unexpectedly created the Agents root: %v", err)
	}
}

func TestDualRootAllowsSameNameAndKeepsIdentitiesSeparate(t *testing.T) {
	m, codexRoot, agentsRoot := newDualRootTestManager(t)
	sourceRoot := filepath.Join(filepath.Dir(m.Config.Paths.DataRoot), "package")
	writeTestSkill(t, sourceRoot, "shared")

	for _, rootID := range []string{model.RootIDCodexDefault, model.RootIDAgents} {
		preview, err := m.PrepareLocal(sourceRoot, rootID)
		if err != nil {
			t.Fatalf("prepare %s: %v", rootID, err)
		}
		if preview.TargetRootID != rootID {
			t.Fatalf("preview target = %q, want %q", preview.TargetRootID, rootID)
		}
		if _, err := m.ApplyInstall(preview.ID, []string{"shared"}, false, rootID); err != nil {
			t.Fatalf("apply %s: %v", rootID, err)
		}
	}

	for _, root := range []string{codexRoot, agentsRoot} {
		if _, err := os.Stat(filepath.Join(root, "shared", "SKILL.md")); err != nil {
			t.Fatalf("same-name Skill missing from %s: %v", root, err)
		}
	}
	dashboard, err := m.Dashboard()
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, skill := range dashboard.Skills {
		if skill.Name == "shared" {
			found[skill.RootID] = true
			if skill.Identity != model.SkillIdentity(skill.RootID, skill.Name) {
				t.Fatalf("unexpected Skill identity: %#v", skill)
			}
		}
	}
	if !found[model.RootIDCodexDefault] || !found[model.RootIDAgents] {
		t.Fatalf("dashboard did not retain both root identities: %#v", found)
	}
}

func TestDualRootSystemDirectoriesAreReadOnly(t *testing.T) {
	m, codexRoot, agentsRoot := newDualRootTestManager(t)
	for _, test := range []struct {
		id   string
		root string
	}{
		{id: model.RootIDCodexDefault, root: codexRoot},
		{id: model.RootIDAgents, root: agentsRoot},
	} {
		writeTestSkill(t, test.root, ".system")
		if _, err := m.Quarantine([]string{".system"}, test.id); err == nil ||
			!strings.Contains(err.Error(), "invalid uninstall target") {
			t.Fatalf("%s .system quarantine was not rejected: %v", test.id, err)
		}
		if _, err := os.Stat(filepath.Join(test.root, ".system", "SKILL.md")); err != nil {
			t.Fatalf("%s .system was mutated: %v", test.id, err)
		}
	}
}

func TestDualRootQuarantineRestoreAndRollbackPreserveAgentsIdentity(t *testing.T) {
	m, codexRoot, agentsRoot := newDualRootTestManager(t)
	sourceRoot := filepath.Join(filepath.Dir(m.Config.Paths.DataRoot), "package")
	writeTestSkill(t, sourceRoot, "demo")
	preview, err := m.PrepareLocal(sourceRoot, model.RootIDAgents)
	if err != nil {
		t.Fatal(err)
	}
	installed, err := m.ApplyInstall(preview.ID, []string{"demo"}, false, model.RootIDAgents)
	if err != nil {
		t.Fatal(err)
	}
	if installed.RootID != model.RootIDAgents {
		t.Fatalf("install transaction root = %q", installed.RootID)
	}

	quarantined, err := m.Quarantine([]string{"demo"}, model.RootIDAgents)
	if err != nil {
		t.Fatal(err)
	}
	if quarantined.RootID != model.RootIDAgents {
		t.Fatalf("quarantine transaction root = %q", quarantined.RootID)
	}
	if _, err := os.Stat(filepath.Join(agentsRoot, "demo")); !os.IsNotExist(err) {
		t.Fatalf("quarantine did not remove the Agents copy: %v", err)
	}
	if _, err := os.Stat(codexRoot); !os.IsNotExist(err) {
		t.Fatalf("Agents quarantine touched the Codex root: %v", err)
	}
	if _, err := m.Restore("demo", quarantined.ID, model.RootIDCodexDefault); err == nil {
		t.Fatal("restore accepted a target root different from the quarantine transaction")
	}
	if _, err := os.Stat(filepath.Join(agentsRoot, "demo")); !os.IsNotExist(err) {
		t.Fatalf("rejected cross-root restore changed the Agents root: %v", err)
	}
	if _, err := os.Stat(codexRoot); !os.IsNotExist(err) {
		t.Fatalf("rejected cross-root restore changed the Codex root: %v", err)
	}

	restored, err := m.Restore("demo", quarantined.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.RootID != model.RootIDAgents {
		t.Fatalf("restore transaction root = %q", restored.RootID)
	}
	if _, err := os.Stat(filepath.Join(agentsRoot, "demo", "SKILL.md")); err != nil {
		t.Fatalf("restore did not return the Agents copy: %v", err)
	}

	rolledBack, err := m.Rollback(installed.ID)
	if err != nil {
		t.Fatalf("rollback did not use the install transaction root: %v", err)
	}
	if rolledBack.RootID != model.RootIDAgents {
		t.Fatalf("rollback transaction root = %q", rolledBack.RootID)
	}
	if _, err := os.Stat(filepath.Join(agentsRoot, "demo")); !os.IsNotExist(err) {
		t.Fatalf("rollback did not remove the Agents install: %v", err)
	}
}

func newDualRootTestManager(t *testing.T) (*Manager, string, string) {
	t.Helper()
	root := t.TempDir()
	data := filepath.Join(root, "data")
	codexRoot := filepath.Join(root, ".codex", "skills")
	agentsRoot := filepath.Join(root, ".agents", "skills")
	cfg := model.Config{
		SchemaVersion: 2,
		Paths: model.Paths{
			SkillsRoot: codexRoot, DataRoot: data,
			LogsRoot: filepath.Join(data, "logs"), ReportsRoot: filepath.Join(data, "reports"),
			BackupsRoot: filepath.Join(data, "backups"), QuarantineRoot: filepath.Join(data, "quarantine"),
			CacheRoot: filepath.Join(data, "cache"), StagingRoot: filepath.Join(data, "staging"),
		},
		SkillRoots: model.DefaultSkillRoots(codexRoot, agentsRoot), DefaultRootID: model.RootIDCodexDefault,
		Schedule: model.Schedule{Frequency: "weekly", Time: "09:00"},
		Locale:   "zh-CN", GitHubHost: "github.com", MaxFileBytes: 20 << 20, MaxFiles: 2000,
	}
	if err := config.EnsureDirs(cfg); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "config.yaml")
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	m, err := Open(configPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m, codexRoot, agentsRoot
}
