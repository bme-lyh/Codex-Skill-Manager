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
