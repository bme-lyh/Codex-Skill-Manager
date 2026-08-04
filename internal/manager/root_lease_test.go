package manager

import (
	"strings"
	"testing"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

func TestRootOperationLeaseRejectsConcurrentWriterAndReleases(t *testing.T) {
	root := model.SkillRoot{ID: model.RootIDCodexDefault, Path: `C:\tmp\Codex Skills`}
	release, err := acquireRootOperationLease(root)
	if err != nil {
		t.Fatalf("acquire root lease: %v", err)
	}
	if _, err := acquireRootOperationLease(model.SkillRoot{ID: root.ID, Path: `c:\TMP\codex skills`}); err == nil ||
		!strings.Contains(err.Error(), "busy") {
		t.Fatalf("expected same-root busy conflict, got %v", err)
	}
	release()
	releaseAgain, err := acquireRootOperationLease(root)
	if err != nil {
		t.Fatalf("released root lease was not reusable: %v", err)
	}
	releaseAgain()
}
