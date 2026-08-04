package provenance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

func TestDetectsKnownBundle(t *testing.T) {
	source := Detect(model.Skill{Name: "review-pr", Path: t.TempDir()})
	if source.Repository != "tirth8205/code-review-graph" || source.Confidence != 1 || source.SourceAssociation != model.SourceAssociationRemote {
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
	if source.Repository != "acme/tools" || source.RequestedRef != "v2" || source.SourcePath != "skills/demo" || source.SourceAssociation != model.SourceAssociationRemote {
		t.Fatalf("unexpected explicit source: %#v", source)
	}
}

func TestUnknownSkillGetsIndependentLocalGroup(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("---\nname: custom\ndescription: fixture\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	source := Detect(model.Skill{Name: "custom", Path: root})
	if source.SourceAssociation != model.SourceAssociationUnlinked {
		t.Fatalf("unexpected source association: %#v", source)
	}
	if source.Provider != "local" || source.GroupID == "local:adopted" || source.GroupName != "本地 · custom" {
		t.Fatalf("unexpected local fallback: %#v", source)
	}
}

func TestDetectInRootQualifiesSourceIdentity(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("---\nname: custom\ndescription: fixture\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	source := DetectInRoot(model.RootIDAgents, model.Skill{Name: "custom", RootID: model.RootIDAgents, Path: root})
	if source.RootID != model.RootIDAgents || source.GroupID == "" || !strings.HasPrefix(source.GroupID, model.RootIDAgents+":") {
		t.Fatalf("source was not root-qualified: %#v", source)
	}
}
