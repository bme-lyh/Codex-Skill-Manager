package manager

import (
	"strings"
	"testing"
	"time"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/codexreview"
	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

func TestProjectScanPersistenceRejectsDigestTampering(t *testing.T) {
	m := newTestManager(t)
	preview := assistedLocalPreview(t, m, "demo")
	contextDigest, contextFileCount, err := codexreview.AssistedInstallContextDigest(preview.StagingPath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	scan := model.CodexProjectScanResult{
		ID:           "project-scan-" + now.Format("20060102T150405.000000000"),
		SourcePlanID: preview.ID,
		Status:       "completed",
		Repository:   preview.Repository,
		Summary:      "A local Skill package.",
		Security: model.CodexProjectSecurity{
			Verdict: "no-material-risk", Summary: "No material issue found.",
			Confidence: 0.8, LocalHighestRisk: preview.Scan.ActiveHighestSeverity,
			LocalFindingCount: preview.Scan.ActiveFindingCount,
			Concerns:          []model.CodexConcern{},
		},
		InstallationMethods: []model.CodexProjectInstallMethod{{
			Kind: "skills-only", Title: "Install Skills",
			Description: "Use the transactional Skill installer.",
			Supported:   true, Required: true, EvidenceFiles: []string{"demo/SKILL.md"},
		}},
		ContextMode: "layered", ContextFileCount: contextFileCount,
		SummaryFileCount: contextFileCount, DeepAnalysisFileCount: 1,
		FocusFiles: []string{"demo/SKILL.md"}, ContextDigest: contextDigest,
		StartedAt: now, CompletedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	scan.ScanDigest, err = codexreview.ProjectScanResultDigest(scan)
	if err != nil {
		t.Fatal(err)
	}
	if err := saveProjectScan(m.Config.Paths.DataRoot, scan); err != nil {
		t.Fatal(err)
	}
	loaded, err := m.GetProjectScan(scan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ScanDigest != scan.ScanDigest {
		t.Fatalf("loaded scan digest = %q, want %q", loaded.ScanDigest, scan.ScanDigest)
	}

	scan.Summary = "tampered summary"
	if err := saveProjectScan(m.Config.Paths.DataRoot, scan); err != nil {
		t.Fatal(err)
	}
	if _, err := m.GetProjectScan(scan.ID); err == nil ||
		!strings.Contains(err.Error(), "digest") {
		t.Fatalf("expected project scan digest rejection, got %v", err)
	}
}

func TestProjectScanProgressDoesNotClaimDependencyInstallation(t *testing.T) {
	stages := projectScanProgressStages()
	if len(stages) != 3 {
		t.Fatalf("project scan has %d progress stages, want 3", len(stages))
	}
	for _, stage := range stages {
		if stage.ID == "dependency-lock" {
			t.Fatal("read-only project scan must not expose a dependency-lock stage")
		}
	}
}
