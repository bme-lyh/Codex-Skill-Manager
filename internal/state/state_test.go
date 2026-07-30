package state

import (
	"testing"
	"time"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

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
