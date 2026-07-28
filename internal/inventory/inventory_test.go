package inventory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
