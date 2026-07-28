package provenance

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

func TestDetectsKnownBundle(t *testing.T) {
	source := Detect(model.Skill{Name: "review-pr", Path: t.TempDir()})
	if source.Repository != "tirth8205/code-review-graph" || source.Confidence != 1 {
		t.Fatalf("unexpected known source: %#v", source)
	}
}

func TestDetectsExplicitGitHubSource(t *testing.T) {
	root := t.TempDir()
	content := "---\nname: demo\ndescription: fixture\nrepository: https://github.com/acme/tools/tree/v2/skills/demo\n---\n"
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	source := Detect(model.Skill{Name: "demo", Path: root})
	if source.Repository != "acme/tools" || source.RequestedRef != "v2" || source.SourcePath != "skills/demo" {
		t.Fatalf("unexpected explicit source: %#v", source)
	}
}

func TestUnknownSkillGetsIndependentLocalGroup(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("---\nname: custom\ndescription: fixture\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	source := Detect(model.Skill{Name: "custom", Path: root})
	if source.Provider != "local" || source.GroupID == "local:adopted" || source.GroupName != "本地 · custom" {
		t.Fatalf("unexpected local fallback: %#v", source)
	}
}
