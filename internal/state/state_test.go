package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

func TestSecurityStatesAreRootQualified(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	if err := store.SaveSkillSecurityStates([]model.SkillSecurityState{
		{RootID: model.RootIDCodexDefault, SkillName: "same", ContentHash: "a", ReportID: "r1", CheckedAt: now},
		{RootID: model.RootIDAgents, SkillName: "same", ContentHash: "b", ReportID: "r2", CheckedAt: now},
	}); err != nil {
		t.Fatal(err)
	}
	states, err := store.SkillSecurityStates()
	if err != nil {
		t.Fatal(err)
	}
	if states[model.QualifiedSkillIdentity(model.RootIDCodexDefault, "same")].ContentHash != "a" || states[model.QualifiedSkillIdentity(model.RootIDAgents, "same")].ContentHash != "b" {
		t.Fatalf("root-qualified states were conflated: %#v", states)
	}
}

func TestV1SourcesLockMigrationInfersRootWithoutTouchingFiles(t *testing.T) {
	base := t.TempDir()
	codex := filepath.Join(base, "codex")
	agents := filepath.Join(base, "agents")
	local := filepath.Join(agents, "same")
	if err := os.MkdirAll(local, 0o700); err != nil {
		t.Fatal(err)
	}
	lock := model.SourcesLock{SchemaVersion: 1, Packages: map[string]model.PackageLock{
		"pkg": {Provider: "github", Skills: map[string]model.SkillLock{"same": {LocalPath: local}}},
	}}
	data, err := json.Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(base, "sources.lock.json")
	if err := os.WriteFile(lockPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := OpenWithRoots(filepath.Join(base, "data"), []model.SkillRoot{{ID: model.RootIDCodexDefault, Path: codex}, {ID: model.RootIDAgents, Path: agents}})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	// Store keeps the lock beside dataRoot.
	if err := os.WriteFile(filepath.Join(base, "data", "sources.lock.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	migrated, err := store.LoadLock()
	if err != nil {
		t.Fatal(err)
	}
	key := model.QualifiedPackageID(model.RootIDAgents, "pkg")
	if migrated.SchemaVersion != 2 || migrated.Packages[key].RootID != model.RootIDAgents {
		t.Fatalf("unexpected migrated lock: %#v", migrated)
	}
	if _, err := os.Stat(local); err != nil {
		t.Fatalf("migration touched local skill path: %v", err)
	}
}

func TestV1SourcesLockMigrationFailsClosedOnAmbiguousPath(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "same")
	lock := model.SourcesLock{SchemaVersion: 1, Packages: map[string]model.PackageLock{"pkg": {Provider: "local", Skills: map[string]model.SkillLock{"same": {LocalPath: path}}}}}
	store, err := OpenWithRoots(filepath.Join(base, "data"), []model.SkillRoot{{ID: model.RootIDCodexDefault, Path: path}, {ID: model.RootIDAgents, Path: path}})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	data, err := json.Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "data", "sources.lock.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadLock(); err == nil {
		t.Fatal("ambiguous v1 root migration unexpectedly succeeded")
	}
}

func TestUpdateStatusesAreScopedByRoot(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	checked := time.Now().UTC().Truncate(time.Second)
	statuses := []model.UpdateStatus{
		{RootID: model.RootIDCodexDefault, GroupID: "github:owner/repo", GroupName: "Codex", Status: "up-to-date", CheckedAt: checked, OutdatedSkills: []string{}},
		{RootID: model.RootIDAgents, GroupID: "github:owner/repo", GroupName: "Agents", Status: "update-available", CheckedAt: checked.Add(time.Second), OutdatedSkills: []string{"demo"}},
	}
	if err := store.SaveUpdateStatuses(statuses); err != nil {
		t.Fatal(err)
	}
	got, err := store.LatestUpdateStatuses()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].RootID != model.RootIDAgents || got[1].RootID != model.RootIDCodexDefault {
		t.Fatalf("root-scoped update statuses were collapsed: %#v", got)
	}
}

func TestSkillSecurityStatesRoundTrip(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	checkedAt := time.Now().UTC().Truncate(time.Nanosecond)
	expected := model.SkillSecurityState{
		SkillName: "alpha", ContentHash: "content-hash",
		ReportID: "scan-1", CheckedAt: checkedAt,
	}
	if err := store.SaveSkillSecurityStates([]model.SkillSecurityState{expected}); err != nil {
		t.Fatal(err)
	}
	states, err := store.SkillSecurityStates()
	if err != nil {
		t.Fatal(err)
	}
	actual, ok := states["alpha"]
	if !ok || actual.ContentHash != expected.ContentHash || actual.ReportID != expected.ReportID ||
		!actual.CheckedAt.Equal(expected.CheckedAt) {
		t.Fatalf("unexpected persisted security state: %#v", actual)
	}
}

func TestSaveSkillSecurityStatesRejectsIncompleteState(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.SaveSkillSecurityStates([]model.SkillSecurityState{{SkillName: "alpha"}}); err == nil {
		t.Fatal("expected incomplete security state to fail")
	}
}

func TestRecoverableAssistedTransactionsAreNotLimitedByRecentHistory(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	started := time.Now().UTC().Add(-time.Hour)
	recoverable := model.Transaction{
		ID: "tx-assisted-recoverable", Type: "assisted-install", Status: "partial",
		StartedAt: started, RecoveryStatus: "available",
	}
	if err := store.SaveTransaction(recoverable); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 25; index++ {
		tx := model.Transaction{
			ID:   "tx-recent-" + time.Unix(int64(index), 0).UTC().Format("150405"),
			Type: "scan", Status: "completed", StartedAt: started.Add(time.Duration(index+1) * time.Minute),
		}
		if err := store.SaveTransaction(tx); err != nil {
			t.Fatal(err)
		}
	}
	recent, err := store.RecentTransactions(20)
	if err != nil {
		t.Fatal(err)
	}
	for _, tx := range recent {
		if tx.ID == recoverable.ID {
			t.Fatal("fixture did not push recoverable transaction outside recent history")
		}
	}
	recoverableTransactions, err := store.RecoverableAssistedTransactions()
	if err != nil {
		t.Fatal(err)
	}
	if len(recoverableTransactions) != 1 || recoverableTransactions[0].ID != recoverable.ID {
		t.Fatalf("unexpected recoverable assisted transactions: %#v", recoverableTransactions)
	}
}

func TestRecoverableAssistedTransactionsIncludesLegacyRunningRecord(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	running := model.Transaction{
		ID:        "tx-assisted-running",
		Type:      "assisted-install",
		Status:    "running",
		StartedAt: time.Now().UTC(),
	}
	if err := store.SaveTransaction(running); err != nil {
		t.Fatal(err)
	}
	transactions, err := store.RecoverableAssistedTransactions()
	if err != nil {
		t.Fatal(err)
	}
	if len(transactions) != 1 || transactions[0].ID != running.ID {
		t.Fatalf("legacy running transaction was not returned: %#v", transactions)
	}
}
