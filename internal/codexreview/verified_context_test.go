package codexreview

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssistedContextIncludesRepositoryDirectoriesNamedLikeManagerState(t *testing.T) {
	root := t.TempDir()
	for relative, content := range map[string]string{
		filepath.Join(".csm-backups", "repository-note.md"):    "repository-owned backup name",
		filepath.Join(".csm-quarantine", "repository-note.md"): "repository-owned quarantine name",
		filepath.Join("docs", ".git", "example.md"):            "nested directory is repository content",
		filepath.Join(".git", "HEAD"):                          "ref: refs/heads/main",
		filepath.Join(".git", "config"):                        "real root VCS metadata",
	} {
		path := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	files, _, count, err := assistedInstallContextSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("expected three repository-owned files, got %d: %#v", count, files)
	}
	paths := make(map[string]bool, len(files))
	for _, file := range files {
		paths[file.Path] = true
	}
	for _, expected := range []string{
		".csm-backups/repository-note.md",
		".csm-quarantine/repository-note.md",
		"docs/.git/example.md",
	} {
		if !paths[expected] {
			t.Fatalf("repository-owned same-name directory was omitted: %s", expected)
		}
	}
	if paths[".git/config"] {
		t.Fatal("root VCS metadata must not enter the Codex context")
	}
}

func TestAssistedContextDoesNotSkipUnverifiedRootVCSName(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".git", "repository-content.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not VCS metadata"), 0o600); err != nil {
		t.Fatal(err)
	}
	files, _, count, err := assistedInstallContextSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || len(files) != 1 || files[0].Path != ".git/repository-content.md" {
		t.Fatalf("unverified root VCS name was incorrectly omitted: %#v", files)
	}
}

func TestOpenVerifiedContextFileRejectsReplacementBetweenResolutionAndOpen(t *testing.T) {
	rootPath := t.TempDir()
	target := filepath.Join(rootPath, "target.txt")
	replacement := filepath.Join(rootPath, "replacement.txt")
	originalMoved := filepath.Join(rootPath, "original-moved.txt")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(replacement, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	walkInfo, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	root, err := resolveVerifiedContextRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	var swapErr error
	opened, err := openVerifiedContextFile(root, target, walkInfo, func() {
		if renameErr := os.Rename(target, originalMoved); renameErr != nil {
			swapErr = renameErr
			return
		}
		swapErr = os.Rename(replacement, target)
	})
	if opened != nil {
		_ = opened.file.Close()
		t.Fatal("replacement race unexpectedly returned an opened context file")
	}
	if swapErr != nil {
		t.Fatalf("fixture could not replace the file: %v", swapErr)
	}
	if err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("expected file identity replacement to be rejected, got %v", err)
	}
}

func TestOpenVerifiedContextFileRejectsSymlinkEscape(t *testing.T) {
	rootPath := t.TempDir()
	outsidePath := filepath.Join(t.TempDir(), "outside-secret.txt")
	if err := os.WriteFile(outsidePath, []byte("host secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(rootPath, "linked.txt")
	if err := os.Symlink(outsidePath, link); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	root, err := resolveVerifiedContextRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	if opened, err := openVerifiedContextFile(root, link, info, nil); err == nil {
		if opened != nil {
			_ = opened.file.Close()
		}
		t.Fatal("expected a symlink escape to be rejected")
	}
}

func TestVerifiedContextFileAllowsStableContainedRead(t *testing.T) {
	rootPath := t.TempDir()
	path := filepath.Join(rootPath, "stable.txt")
	if err := os.WriteFile(path, []byte("stable content"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	root, err := resolveVerifiedContextRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := openVerifiedContextFile(root, path, info, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(opened.file)
	if err != nil {
		_ = opened.file.Close()
		t.Fatal(err)
	}
	if string(data) != "stable content" {
		_ = opened.file.Close()
		t.Fatalf("unexpected stable content: %q", data)
	}
	if err := opened.closeAfterRead(); err != nil {
		t.Fatalf("stable context file failed final identity verification: %v", err)
	}
}
