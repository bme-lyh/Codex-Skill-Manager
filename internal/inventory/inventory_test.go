package inventory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

func TestReadFrontmatter(t *testing.T) {
	root := testInventoryDir(t)
	path := filepath.Join(root, "SKILL.md")
	if err := os.WriteFile(path, []byte("---\nname: example\ndescription: Example skill\n---\n# Body\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fm, err := ReadFrontmatter(path)
	if err != nil {
		t.Fatal(err)
	}
	if fm.Name != "example" || fm.Description != "Example skill" {
		t.Fatalf("unexpected frontmatter: %#v", fm)
	}
}

func TestReadFrontmatterRequiresDescription(t *testing.T) {
	root := testInventoryDir(t)
	path := filepath.Join(root, "SKILL.md")
	if err := os.WriteFile(path, []byte("---\nname: example\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFrontmatter(path); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestDiscoverRootsAllowsSameNameAndKeepsSystemReadOnly(t *testing.T) {
	base := t.TempDir()
	first := filepath.Join(base, "codex")
	second := filepath.Join(base, "agents")
	for _, root := range []string{first, second} {
		if err := os.MkdirAll(filepath.Join(root, "same"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "same", "SKILL.md"), []byte("---\nname: same\ndescription: duplicate\n---\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(second, ".SYSTEM", "builtin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(second, ".SYSTEM", "builtin", "SKILL.md"), []byte("---\nname: builtin\ndescription: system\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	roots := []model.SkillRoot{
		{ID: model.RootIDCodexDefault, Path: first, Enabled: true, SystemDir: ".system"},
		{ID: model.RootIDAgents, Path: second, Enabled: true, SystemDir: ".system"},
	}
	skills, _, _, err := DiscoverRoots(roots, model.SourcesLock{SchemaVersion: 2, Packages: map[string]model.PackageLock{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 3 {
		t.Fatalf("unexpected discovered skills: %#v", skills)
	}
	seen := map[string]model.Skill{}
	for _, skill := range skills {
		seen[skill.Identity] = skill
	}
	if _, ok := seen[model.SkillIdentity(model.RootIDCodexDefault, "same")]; !ok {
		t.Fatal("codex root duplicate missing")
	}
	if _, ok := seen[model.SkillIdentity(model.RootIDAgents, "same")]; !ok {
		t.Fatal("agents root duplicate missing")
	}
	system, ok := seen[model.SkillIdentity(model.RootIDAgents, "builtin")]
	if !ok || !system.System {
		t.Fatalf("system skill was not marked read-only: %#v", system)
	}
}

func testInventoryDir(t *testing.T) string {
	t.Helper()
	base := filepath.Join("..", "..", "test-output", "unit", "inventory-"+strings.ReplaceAll(time.Now().UTC().Format("150405.000000000"), ".", "-"))
	abs, err := filepath.Abs(base)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		t.Fatal(err)
	}
	return abs
}
