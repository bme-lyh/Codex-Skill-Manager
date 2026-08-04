package manager

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

// A manager instance may be reopened by the desktop recovery path while an
// older instance is still finishing a transaction.  Keep a process-wide,
// root-scoped lease so two writers cannot interleave backups, source-lock
// updates, or Codex configuration checkpoints for the same root.
var (
	rootLeaseMu sync.Mutex
	rootLeases  = map[string]bool{}
)

func rootLeaseKey(root model.SkillRoot) string {
	return strings.ToLower(filepath.Clean(root.Path))
}

func acquireRootOperationLease(root model.SkillRoot) (func(), error) {
	if strings.TrimSpace(root.Path) == "" {
		return nil, errors.New("skill root path is required for mutation")
	}
	key := rootLeaseKey(root)
	rootLeaseMu.Lock()
	defer rootLeaseMu.Unlock()
	if rootLeases[key] {
		return nil, fmt.Errorf("skill root %q is busy with another operation; retry after it completes", root.ID)
	}
	rootLeases[key] = true
	var once sync.Once
	return func() {
		once.Do(func() {
			rootLeaseMu.Lock()
			delete(rootLeases, key)
			rootLeaseMu.Unlock()
		})
	}, nil
}
