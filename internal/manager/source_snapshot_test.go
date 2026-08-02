package manager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareLocalUsesManagedImmutableSnapshot(t *testing.T) {
	m := newTestManager(t)
	source := filepath.Join(t.TempDir(), "local-source")
	writeTestSkill(t, source, "demo")
	preview, err := m.PrepareLocal(source)
	if err != nil {
		t.Fatal(err)
	}
	if strings.EqualFold(filepath.Clean(preview.StagingPath), filepath.Clean(source)) {
		t.Fatal("local preview retained the live source directory")
	}
	if err := ensureWithinOrEqual(m.Config.Paths.StagingRoot, preview.StagingPath); err != nil {
		t.Fatalf("local snapshot is not managed: %v", err)
	}
	original := filepath.Join(source, "demo", "SKILL.md")
	if err := os.WriteFile(original, []byte("changed after planning"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ApplyInstall(preview.ID, []string{"demo"}, false); err != nil {
		t.Fatal(err)
	}
	installed, err := os.ReadFile(filepath.Join(m.Config.Paths.SkillsRoot, "demo", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(installed), "changed after planning") {
		t.Fatal("installation read from the mutable local source instead of the managed snapshot")
	}
}

func TestSnapshotLocalSourceRejectsLinks(t *testing.T) {
	source := t.TempDir()
	destination := filepath.Join(t.TempDir(), "snapshot")
	target := filepath.Join(source, "target.txt")
	if err := os.WriteFile(target, []byte("target"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(source, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks are unavailable in this Windows environment: %v", err)
	}
	if err := snapshotLocalSource(source, destination, 20, 1<<20); err == nil {
		t.Fatal("local snapshot accepted a linked path")
	}
}

func TestSnapshotLocalSourceRejectsOverlappingDestination(t *testing.T) {
	source := t.TempDir()
	writeTestSkill(t, source, "demo")
	destination := filepath.Join(source, "managed", "plan-overlap")
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := snapshotLocalSource(source, destination, 20, 1<<20); err == nil || !strings.Contains(err.Error(), "overlap") {
		t.Fatalf("overlapping local snapshot was not rejected: %v", err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("rejected snapshot created its destination: %v", err)
	}
}

func TestApplyInstallCreatesMissingSkillsRoot(t *testing.T) {
	m := newTestManager(t)
	if err := os.Remove(m.Config.Paths.SkillsRoot); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "source")
	writeTestSkill(t, source, "demo")
	preview, err := m.PrepareLocal(source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.ApplyInstall(preview.ID, []string{"demo"}, false); err != nil {
		t.Fatalf("fresh install with a missing Skills root failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(m.Config.Paths.SkillsRoot, "demo", "SKILL.md")); err != nil {
		t.Fatalf("fresh install did not create its target: %v", err)
	}
}

func TestSnapshotLocalSourceBoundsDirectoriesAndFilesTogether(t *testing.T) {
	source := t.TempDir()
	for _, name := range []string{"one", "two", "three"} {
		if err := os.Mkdir(filepath.Join(source, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	destination := filepath.Join(t.TempDir(), "snapshot")
	if err := snapshotLocalSource(source, destination, 2, 1<<20); err == nil || !strings.Contains(err.Error(), "entry count") {
		t.Fatalf("directory-only source exceeded no bounded limit: %v", err)
	}
}

func TestPrepareLocalRejectsLinkedRoot(t *testing.T) {
	m := newTestManager(t)
	source := filepath.Join(t.TempDir(), "source")
	writeTestSkill(t, source, "demo")
	link := filepath.Join(t.TempDir(), "source-link")
	if err := os.Symlink(source, link); err != nil {
		t.Skipf("directory symlinks are unavailable in this Windows environment: %v", err)
	}
	if _, err := m.PrepareLocal(link); err == nil {
		t.Fatal("PrepareLocal accepted a linked source root")
	}
}

func TestLoadPreviewRejectsDigestTampering(t *testing.T) {
	m := newTestManager(t)
	preview := githubPreviewFixture(t, m, "plan-tamper", strings.Repeat("a", 40), []string{"demo"})
	if err := savePreview(m.Config.Paths.DataRoot, preview); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(m.Config.Paths.DataRoot, preview.ID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), "owner/repo", "owner/evil", 1))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPreview(m.Config.Paths.DataRoot, preview.ID); err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("tampered preview was not rejected: %v", err)
	}
}
