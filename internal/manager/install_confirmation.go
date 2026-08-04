package manager

// This file owns the small, manager-side authorization object used by the
// Codex-assisted installation flow.  It deliberately does not introduce a
// second execution path: a confirmation only re-binds an already validated
// source preview, project scan, and typed assisted-install plan before the
// existing journaled executor is called.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

// InstallConfirmation is an explicit, one-time human acknowledgement.  The
// digests are copied from manager-owned records and are never accepted from
// model output or from the renderer.
//
// It is exported so the Wails facade can expose a JSON-shaped value without
// adding another model schema migration.  Callers should treat it as opaque
// and pass only ID to ApplyConfirmedAssistedInstall.
type InstallConfirmation struct {
	ID               string     `json:"id"`
	PlanID           string     `json:"planId"`
	SourcePlanID     string     `json:"sourcePlanId"`
	SourceDigest     string     `json:"sourceDigest"`
	ReportDigest     string     `json:"reportDigest"`
	AssessmentDigest string     `json:"assessmentDigest"`
	PlanDigest       string     `json:"planDigest"`
	TargetRootID     string     `json:"targetRootId"`
	SelectedSkills   []string   `json:"selectedSkills"`
	PermissionIDs    []string   `json:"permissionIds"`
	HighRiskAccepted bool       `json:"highRiskAccepted"`
	CreatedAt        time.Time  `json:"createdAt"`
	ExpiresAt        time.Time  `json:"expiresAt"`
	UsedAt           *time.Time `json:"usedAt,omitempty"`
	Digest           string     `json:"digest"`
}

var installConfirmationMu sync.Mutex

func installConfirmationRoot(dataRoot string) string {
	return filepath.Join(dataRoot, "assisted-install-confirmations")
}

func validInstallConfirmationID(value string) bool {
	if value == "" || filepath.Base(value) != value ||
		filepath.VolumeName(value) != "" || !strings.HasPrefix(value, "confirm-") {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '.' {
			continue
		}
		return false
	}
	return len(value) <= 160
}

func confirmationDigest(value InstallConfirmation) (string, error) {
	value.Digest = ""
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func sealInstallConfirmation(value *InstallConfirmation) error {
	if value == nil {
		return errors.New("install confirmation is required")
	}
	digest, err := confirmationDigest(*value)
	if err != nil {
		return err
	}
	value.Digest = digest
	return verifyInstallConfirmation(*value)
}

func verifyInstallConfirmation(value InstallConfirmation) error {
	if !validInstallConfirmationID(value.ID) ||
		!validAssistedPlanID(value.PlanID) || !strings.HasPrefix(value.SourcePlanID, "plan-") {
		return errors.New("invalid install confirmation identity")
	}
	if value.TargetRootID == "" || len(value.SelectedSkills) == 0 {
		return errors.New("install confirmation has no explicit target or Skill selection")
	}
	if value.CreatedAt.IsZero() || value.ExpiresAt.IsZero() ||
		!value.ExpiresAt.After(value.CreatedAt) {
		return errors.New("invalid install confirmation timestamps")
	}
	for _, digest := range []string{
		value.SourceDigest, value.ReportDigest, value.AssessmentDigest, value.PlanDigest,
	} {
		if !validHexDigest(digest, sha256.Size) {
			return errors.New("install confirmation contains an invalid digest")
		}
	}
	seen := map[string]bool{}
	for _, name := range value.SelectedSkills {
		if !validMutableSkillName(name) || seen[name] {
			return errors.New("install confirmation contains an invalid or duplicate Skill")
		}
		seen[name] = true
	}
	seen = map[string]bool{}
	for _, permission := range value.PermissionIDs {
		if strings.TrimSpace(permission) == "" || seen[permission] {
			return errors.New("install confirmation contains an invalid or duplicate permission")
		}
		seen[permission] = true
	}
	expected, err := confirmationDigest(value)
	if err != nil {
		return err
	}
	if value.Digest == "" || !strings.EqualFold(value.Digest, expected) {
		return errors.New("install confirmation integrity digest mismatch")
	}
	return nil
}

func saveInstallConfirmation(dataRoot string, value InstallConfirmation) error {
	if err := verifyInstallConfirmation(value); err != nil {
		return err
	}
	if err := os.MkdirAll(installConfirmationRoot(dataRoot), 0o700); err != nil {
		return err
	}
	return writeJSONAtomic(filepath.Join(installConfirmationRoot(dataRoot), value.ID+".json"), value)
}

func loadInstallConfirmation(dataRoot, id string) (InstallConfirmation, error) {
	if !validInstallConfirmationID(id) {
		return InstallConfirmation{}, errors.New("invalid install confirmation ID")
	}
	data, err := os.ReadFile(filepath.Join(installConfirmationRoot(dataRoot), id+".json"))
	if err != nil {
		return InstallConfirmation{}, err
	}
	var value InstallConfirmation
	if err := json.Unmarshal(data, &value); err != nil {
		return InstallConfirmation{}, err
	}
	if value.ID != id {
		return InstallConfirmation{}, errors.New("install confirmation identity mismatch")
	}
	if err := verifyInstallConfirmation(value); err != nil {
		return InstallConfirmation{}, err
	}
	return value, nil
}

// ConfirmCodexInstall creates the one-time acknowledgement for a completed
// typed Codex plan.  All source/report/assessment digests are loaded from the
// manager's persisted records, so a stale or renderer-edited result cannot be
// used as approval.
func (m *Manager) ConfirmCodexInstall(
	planID string,
	selectedSkills []string,
	permissionIDs []string,
	acceptHighRisk bool,
	targetRootID string,
) (InstallConfirmation, error) {
	planID = strings.TrimSpace(planID)
	if !validAssistedPlanID(planID) {
		return InstallConfirmation{}, errors.New("invalid assisted-install plan ID")
	}
	plan, err := m.GetAssistedInstallPlan(planID)
	if err != nil {
		return InstallConfirmation{}, err
	}
	if plan.ProjectScanID == "" || plan.ProjectScanDigest == "" {
		return InstallConfirmation{}, errors.New("Codex project review is required before confirmation")
	}
	if targetRootID != "" && targetRootID != plan.TargetRootID {
		return InstallConfirmation{}, errors.New("install confirmation target root does not match the plan")
	}
	preview, err := m.assistedSourcePreview(plan.SourcePlanID)
	if err != nil {
		return InstallConfirmation{}, err
	}
	assessment, err := m.GetProjectAssessment(plan.SourcePlanID)
	if err != nil {
		return InstallConfirmation{}, err
	}
	if assessment.Gate != model.AssessmentGateReady && assessment.Gate != model.AssessmentGateAttention {
		return InstallConfirmation{}, fmt.Errorf("project assessment does not permit confirmation: %s", assessment.Gate)
	}
	scan, err := m.GetProjectScan(plan.ProjectScanID)
	if err != nil {
		return InstallConfirmation{}, err
	}
	if scan.SourcePlanID != plan.SourcePlanID || !strings.EqualFold(scan.ScanDigest, plan.ProjectScanDigest) {
		return InstallConfirmation{}, errors.New("Codex review is not bound to the current source plan")
	}
	selectedSkills = uniqueNonEmpty(selectedSkills)
	if len(selectedSkills) == 0 {
		return InstallConfirmation{}, errors.New("at least one Skill must be selected for confirmation")
	}
	allowed := map[string]bool{}
	for _, skill := range preview.Skills {
		allowed[skill.Name] = true
	}
	for _, name := range selectedSkills {
		if !allowed[name] {
			return InstallConfirmation{}, fmt.Errorf("Skill %q is not in the reviewed source plan", name)
		}
	}
	permissionIDs = uniqueNonEmpty(permissionIDs)
	if _, err := validateAssistedPermissions(plan.Permissions, permissionIDs); err != nil {
		return InstallConfirmation{}, err
	}
	if err := validateAssistedPermissionDependencies(plan.Steps, keySet(permissionIDs)); err != nil {
		return InstallConfirmation{}, err
	}
	ignored, err := m.store.IgnoredFindings()
	if err != nil {
		return InstallConfirmation{}, err
	}
	localScan := m.decorateScan(preview.Scan, ignored)
	hasCritical, hasHigh := false, false
	for _, cluster := range localScan.Clusters {
		if cluster.Ignored {
			continue
		}
		switch cluster.Severity {
		case model.RiskCritical:
			hasCritical = true
		case model.RiskHigh:
			hasHigh = true
		}
	}
	if hasCritical {
		return InstallConfirmation{}, errors.New("Critical local risk remains; confirmation cannot bypass the safety boundary")
	}
	if hasHigh && !acceptHighRisk {
		return InstallConfirmation{}, errors.New("High local risk requires explicit confirmation")
	}
	now := time.Now().UTC()
	value := InstallConfirmation{
		ID:               "confirm-" + now.Format("20060102T150405.000000000"),
		PlanID:           plan.ID,
		SourcePlanID:     preview.ID,
		SourceDigest:     preview.PreviewDigest,
		ReportDigest:     scan.ScanDigest,
		AssessmentDigest: assessment.AssessmentDigest,
		PlanDigest:       plan.PlanDigest,
		TargetRootID:     plan.TargetRootID,
		SelectedSkills:   append([]string(nil), selectedSkills...),
		PermissionIDs:    append([]string(nil), permissionIDs...),
		HighRiskAccepted: acceptHighRisk,
		CreatedAt:        now,
		ExpiresAt:        plan.ExpiresAt,
	}
	sort.Strings(value.SelectedSkills)
	sort.Strings(value.PermissionIDs)
	if err := sealInstallConfirmation(&value); err != nil {
		return InstallConfirmation{}, err
	}
	installConfirmationMu.Lock()
	defer installConfirmationMu.Unlock()
	if err := saveInstallConfirmation(m.Config.Paths.DataRoot, value); err != nil {
		return InstallConfirmation{}, err
	}
	return value, nil
}

func keySet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}

// ApplyConfirmedAssistedInstall consumes a valid confirmation and delegates
// execution to the existing assisted-install journal/recovery implementation.
// A confirmation is consumed only after the executor returns a structured
// result, so a preflight failure can be corrected with a fresh review without
// allowing concurrent replay.
func (m *Manager) ApplyConfirmedAssistedInstall(
	ctx context.Context,
	confirmationID string,
	projectRoot string,
	targetRootID string,
	progress AssistedInstallProgressFunc,
) (model.AssistedInstallResult, error) {
	installConfirmationMu.Lock()
	defer installConfirmationMu.Unlock()
	value, err := loadInstallConfirmation(m.Config.Paths.DataRoot, strings.TrimSpace(confirmationID))
	if err != nil {
		return model.AssistedInstallResult{}, err
	}
	if value.UsedAt != nil {
		return model.AssistedInstallResult{}, errors.New("install confirmation has already been used")
	}
	if time.Now().UTC().After(value.ExpiresAt) {
		return model.AssistedInstallResult{}, errors.New("install confirmation has expired")
	}
	if targetRootID != "" && targetRootID != value.TargetRootID {
		return model.AssistedInstallResult{}, errors.New("install confirmation target root does not match")
	}
	plan, err := m.GetAssistedInstallPlan(value.PlanID)
	if err != nil {
		return model.AssistedInstallResult{}, err
	}
	if plan.SourcePlanID != value.SourcePlanID || plan.TargetRootID != value.TargetRootID ||
		!strings.EqualFold(plan.PlanDigest, value.PlanDigest) {
		return model.AssistedInstallResult{}, errors.New("install confirmation is not bound to the current plan")
	}
	preview, err := m.assistedSourcePreview(value.SourcePlanID)
	if err != nil {
		return model.AssistedInstallResult{}, err
	}
	if !strings.EqualFold(preview.PreviewDigest, value.SourceDigest) {
		return model.AssistedInstallResult{}, errors.New("install confirmation source digest changed")
	}
	if plan.ProjectScanID == "" {
		return model.AssistedInstallResult{}, errors.New("install confirmation has no Codex review")
	}
	scan, err := m.GetProjectScan(plan.ProjectScanID)
	if err != nil {
		return model.AssistedInstallResult{}, err
	}
	if !strings.EqualFold(scan.ScanDigest, value.ReportDigest) ||
		!strings.EqualFold(plan.ProjectScanDigest, value.ReportDigest) {
		return model.AssistedInstallResult{}, errors.New("install confirmation report digest changed")
	}
	assessment, err := m.GetProjectAssessment(value.SourcePlanID)
	if err != nil {
		return model.AssistedInstallResult{}, err
	}
	if !strings.EqualFold(assessment.AssessmentDigest, value.AssessmentDigest) {
		return model.AssistedInstallResult{}, errors.New("install confirmation assessment digest changed")
	}
	// Hold the confirmation mutex across execution to prevent a second caller
	// from replaying the same acknowledgement while the journal is running.
	result, err := m.ApplyAssistedInstallForRoot(
		ctx, value.PlanID, value.SelectedSkills, value.PermissionIDs, projectRoot,
		value.TargetRootID, progress,
	)
	if err != nil {
		return model.AssistedInstallResult{}, err
	}
	usedAt := time.Now().UTC()
	value.UsedAt = &usedAt
	if err := sealInstallConfirmation(&value); err != nil {
		return model.AssistedInstallResult{}, fmt.Errorf("seal confirmation use: %w", err)
	}
	if err := saveInstallConfirmation(m.Config.Paths.DataRoot, value); err != nil {
		return model.AssistedInstallResult{}, fmt.Errorf("record confirmation use: %w", err)
	}
	return result, nil
}
