package manager

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInstallConfirmationDigestBindsUseState(t *testing.T) {
	now := time.Now().UTC()
	value := InstallConfirmation{
		ID:               "confirm-20260101T000000.000000000",
		PlanID:           "assisted-plan-20260101T000000.000000000",
		SourcePlanID:     "plan-20260101T000000.000000000",
		SourceDigest:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ReportDigest:     "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		AssessmentDigest: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		PlanDigest:       "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		TargetRootID:     "codex-default",
		SelectedSkills:   []string{"demo"}, PermissionIDs: []string{"skills-write"},
		CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	if err := sealInstallConfirmation(&value); err != nil {
		t.Fatalf("seal confirmation: %v", err)
	}
	root := t.TempDir()
	if err := saveInstallConfirmation(root, value); err != nil {
		t.Fatalf("save confirmation: %v", err)
	}
	loaded, err := loadInstallConfirmation(root, value.ID)
	if err != nil || loaded.Digest != value.Digest {
		t.Fatalf("load confirmation: value=%#v err=%v", loaded, err)
	}
	used := now.Add(2 * time.Minute)
	loaded.UsedAt = &used
	if err := sealInstallConfirmation(&loaded); err != nil {
		t.Fatalf("seal used confirmation: %v", err)
	}
	if err := saveInstallConfirmation(root, loaded); err != nil {
		t.Fatalf("save used confirmation: %v", err)
	}
	if _, err := loadInstallConfirmation(root, value.ID); err != nil {
		t.Fatalf("load used confirmation: %v", err)
	}
	loaded.UsedAt = nil
	if _, err := confirmationDigest(loaded); err != nil {
		t.Fatalf("digest cleared use marker: %v", err)
	}
	loadedPath := filepath.Join(installConfirmationRoot(root), value.ID+".json")
	data, readErr := os.ReadFile(loadedPath)
	if readErr != nil {
		t.Fatalf("read confirmation: %v", readErr)
	}
	var tampered InstallConfirmation
	if err := json.Unmarshal(data, &tampered); err != nil {
		t.Fatalf("decode confirmation: %v", err)
	}
	tampered.UsedAt = nil
	if err := verifyInstallConfirmation(tampered); err == nil {
		t.Fatal("clearing usedAt unexpectedly passed confirmation integrity check")
	}
}
