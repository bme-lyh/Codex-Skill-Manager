package manager

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

func TestAssessInstallSourceClassifiesMixedRepository(t *testing.T) {
	m := newTestManager(t)
	source := filepath.Join(t.TempDir(), "mixed")
	writeTestSkill(t, source, "demo")
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("Installation guide"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "package.json"), []byte(`{"name":"mixed"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	preview, err := m.PrepareLocal(source)
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := m.AssessInstallSource(preview.ID)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Classification != "mixed" || assessment.Gate != model.AssessmentGateAttention {
		t.Fatalf("unexpected mixed assessment: %#v", assessment)
	}
	if !assessment.EnhancedScanRecommended || len(assessment.Targets) != 1 || !assessment.Targets[0].Supported {
		t.Fatalf("assessment did not expose the expected target and enhanced check: %#v", assessment)
	}
	loaded, err := m.GetProjectAssessment(assessment.ID)
	if err != nil || loaded.AssessmentDigest != assessment.AssessmentDigest {
		t.Fatalf("persisted assessment did not round-trip: %#v, %v", loaded, err)
	}
}

func TestAssessmentFailsClosedForCriticalRisk(t *testing.T) {
	m := newTestManager(t)
	source := filepath.Join(t.TempDir(), "critical")
	writeTestSkill(t, source, "critical")
	if err := os.WriteFile(filepath.Join(source, "critical", "danger.ps1"), []byte("Remove-Item C:\\temp -Recurse"), 0o600); err != nil {
		t.Fatal(err)
	}
	preview, err := m.PrepareLocal(source)
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := m.AssessInstallSource(preview.ID)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Gate != model.AssessmentGateBlocked {
		t.Fatalf("Critical assessment gate = %q", assessment.Gate)
	}
	if _, err := m.ApplyInstall(preview.ID, []string{"critical"}, true); err == nil {
		t.Fatal("Critical assessment did not block standard apply")
	}
}

func TestLegacyCriticalIgnoreCannotBypassAssessment(t *testing.T) {
	m := newTestManager(t)
	source := filepath.Join(t.TempDir(), "critical-legacy-ignore")
	writeTestSkill(t, source, "critical")
	if err := os.WriteFile(filepath.Join(source, "critical", "danger.ps1"), []byte("Remove-Item C:\\temp -Recurse"), 0o600); err != nil {
		t.Fatal(err)
	}
	preview, err := m.PrepareLocal(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range preview.Scan.Findings {
		if finding.Severity == model.RiskCritical {
			if err := m.store.SetFindingIgnored(finding, true, "legacy row"); err != nil {
				t.Fatal(err)
			}
		}
	}
	assessment, err := m.AssessInstallSource(preview.ID)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Gate != model.AssessmentGateBlocked {
		t.Fatalf("legacy Critical ignore bypassed the gate: %#v", assessment)
	}
}

func TestProjectScanReloadRechecksMandatoryAssessment(t *testing.T) {
	m := newTestManager(t)
	source := filepath.Join(t.TempDir(), "critical-scan")
	writeTestSkill(t, source, "critical")
	if err := os.WriteFile(filepath.Join(source, "critical", "danger.ps1"), []byte("Remove-Item C:\\temp -Recurse"), 0o600); err != nil {
		t.Fatal(err)
	}
	preview, err := m.PrepareLocal(source)
	if err != nil {
		t.Fatal(err)
	}
	scan := model.CodexProjectScanResult{
		ID: "project-scan-blocked", SourcePlanID: preview.ID, Status: "completed",
		ExpiresAt: preview.ExpiresAt,
	}
	if err := saveProjectScan(m.Config.Paths.DataRoot, scan); err != nil {
		t.Fatal(err)
	}
	if _, err := m.GetProjectScan(scan.ID); err == nil || !strings.Contains(err.Error(), "assessment") {
		t.Fatalf("project scan reload bypassed mandatory assessment: %v", err)
	}
}

func TestHighRiskAcceptanceRequiresConfirmationAndReason(t *testing.T) {
	m := newTestManager(t)
	fingerprint := fmt.Sprintf("%064x", 42)
	cluster := model.RiskCluster{ID: "risk-high", RuleID: "CSM-PERSIST-001", Severity: model.RiskHigh, Fingerprints: []string{fingerprint}}
	if err := m.store.SaveScan(model.ScanReport{ID: "high-scan", CompletedAt: time.Now().UTC(), Clusters: []model.RiskCluster{cluster}}); err != nil {
		t.Fatal(err)
	}
	if err := m.SetRiskClusterIgnored(cluster, true, "", true); err == nil {
		t.Fatal("High risk was accepted without a reason")
	}
	if err := m.SetRiskClusterIgnored(cluster, true, "reviewed and accepted", false); err == nil {
		t.Fatal("High risk was accepted without explicit confirmation")
	}
	if err := m.SetRiskClusterIgnored(cluster, true, "reviewed and accepted", true); err != nil {
		t.Fatalf("explicit High risk acceptance failed: %v", err)
	}
}

func TestStandardApplyRequiresFinalHighRiskConfirmation(t *testing.T) {
	m := newTestManager(t)
	preview := githubPreviewFixture(t, m, "plan-high-apply", strings.Repeat("a", 40), []string{"demo"})
	preview.Scan.Findings = []model.Finding{{
		RuleID: "CSM-PERSIST-001", Title: "Persistence", Severity: model.RiskHigh,
		File: "skills/demo/SKILL.md", Line: 1, Evidence: "scheduled persistence",
	}}
	preview.Scan = m.decorateScan(preview.Scan, map[string]string{})
	if err := m.store.SaveScan(preview.Scan); err != nil {
		t.Fatal(err)
	}
	cluster := preview.Scan.Clusters[0]
	if err := m.SetRiskClusterIgnored(cluster, true, "reviewed and accepted", true); err != nil {
		t.Fatal(err)
	}
	if err := sealInstallPreview(&preview); err != nil {
		t.Fatal(err)
	}
	m.previews[preview.ID] = preview
	if _, err := m.ApplyInstall(preview.ID, []string{"demo"}, false); err == nil || !strings.Contains(err.Error(), "final apply confirmation") {
		t.Fatalf("standard apply accepted High risk without final confirmation: %v", err)
	}
	if _, err := m.ApplyInstall(preview.ID, []string{"demo"}, true); err != nil {
		t.Fatalf("confirmed High risk apply failed: %v", err)
	}
}

func TestReservedSystemTargetIsCaseInsensitive(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.Restore(".SYSTEM", "tx-any"); err == nil {
		t.Fatal("restore accepted the reserved .SYSTEM target")
	}
	preview := githubPreviewFixture(t, m, "plan-system", strings.Repeat("a", 40), []string{".SYSTEM"})
	m.previews[preview.ID] = preview
	if _, err := m.ApplyInstall(preview.ID, []string{".SYSTEM"}, false); err == nil {
		t.Fatal("install accepted the reserved .SYSTEM target")
	}
	for _, name := range []string{".", "..", ".system.", ".SYSTEM "} {
		if _, err := m.Quarantine([]string{name}); err == nil {
			t.Fatalf("quarantine accepted unsafe target %q", name)
		}
	}
}

func TestProjectAssessmentRejectsUnsafeTargetContract(t *testing.T) {
	m := newTestManager(t)
	source := filepath.Join(t.TempDir(), "assessment-target")
	writeTestSkill(t, source, "demo")
	preview, err := m.PrepareLocal(source)
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := m.AssessInstallSource(preview.ID)
	if err != nil {
		t.Fatal(err)
	}
	assessment.Targets[0].PermissionIDs = []string{"future-permission"}
	assessment.AssessmentDigest, _ = projectAssessmentDigest(assessment)
	if err := verifyProjectAssessment(assessment); err == nil {
		t.Fatal("assessment accepted an unknown target permission")
	}
}

func TestGetProjectAssessmentCrossBindsTargetAndExpiry(t *testing.T) {
	m := newTestManager(t)
	source := filepath.Join(t.TempDir(), "assessment-binding")
	writeTestSkill(t, source, "demo")
	preview, err := m.PrepareLocal(source)
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := m.AssessInstallSource(preview.ID)
	if err != nil {
		t.Fatal(err)
	}
	assessment.Targets[0].Path = filepath.Join(t.TempDir(), "demo")
	assessment.ExpiresAt = assessment.ExpiresAt.Add(time.Hour)
	assessment.AssessmentDigest, _ = projectAssessmentDigest(assessment)
	if err := saveProjectAssessment(m.Config.Paths.DataRoot, assessment); err != nil {
		t.Fatal(err)
	}
	if _, err := m.GetProjectAssessment(assessment.ID); err == nil {
		t.Fatal("GetProjectAssessment accepted recomputed target/expiry tampering")
	}
}

func TestMixedKnownAndUnknownClusterSeverityFailsClosed(t *testing.T) {
	m := newTestManager(t)
	report := model.ScanReport{ID: "unknown-mixed", Findings: []model.Finding{
		{RuleID: "CSM-NET-001", Severity: model.RiskMedium, File: "a.md", Evidence: "same"},
		{RuleID: "CSM-NET-001", Severity: model.RiskSeverity("future"), File: "b.md", Evidence: "same"},
	}}
	decorated := m.decorateScan(report, map[string]string{})
	if len(decorated.Clusters) != 1 || decorated.Clusters[0].Severity != model.RiskSeverity("future") ||
		decorated.ActiveHighestSeverity != model.RiskSeverity("future") {
		t.Fatalf("unknown severity did not dominate its cluster: %#v", decorated)
	}
}

func TestProjectAssessmentRejectsUnknownGate(t *testing.T) {
	now := time.Now().UTC()
	value := model.ProjectAssessment{
		ID: "assessment-test", SourcePlanID: "plan-test", Gate: "future-value",
		CreatedAt: now, ExpiresAt: now.Add(time.Hour), SourceDigest: strings.Repeat("a", 64),
	}
	value.AssessmentDigest, _ = projectAssessmentDigest(value)
	if err := verifyProjectAssessment(value); err == nil {
		t.Fatal("unknown assessment gate did not fail closed")
	}
}
