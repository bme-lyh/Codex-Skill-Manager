package codexreview

import (
	"fmt"
	"strings"
	"testing"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

func TestProjectScanSchemaIsStrict(t *testing.T) {
	if err := validateStrictCodexSchema([]byte(projectScanOutputSchema)); err != nil {
		t.Fatalf("project scan schema is not strict: %v", err)
	}
}

func TestProjectScanFocusFilesPrioritizeRiskAndManifests(t *testing.T) {
	files := []installAnalysisFile{
		{Path: "docs/notes.md", Kind: "text", Content: strings.Repeat("n", 1024)},
		{Path: "src/server.py", Kind: "text", Content: strings.Repeat("s", 1024)},
		{Path: "SKILL.md", Kind: "text", Content: strings.Repeat("k", 1024)},
		{Path: ".env", Kind: "binary", Redacted: true, Content: ""},
	}
	report := model.ScanReport{
		Findings: []model.Finding{{File: "src/server.py"}},
	}
	focus := projectScanFocusFiles(files, report)
	if len(focus) != 3 {
		t.Fatalf("got %d focus files, want 3", len(focus))
	}
	seen := map[string]bool{}
	for _, file := range focus {
		seen[file.Path] = true
	}
	if !seen["src/server.py"] || !seen["SKILL.md"] {
		t.Fatalf("risk evidence and Skill manifest must be included: %#v", focus)
	}
	if seen[".env"] {
		t.Fatal("redacted credential content must never enter focused analysis")
	}
}

func TestProjectScanDigestBindsSecurityConclusion(t *testing.T) {
	result := model.CodexProjectScanResult{
		ID:           "project-scan-20260731T120000.000000000",
		SourcePlanID: "plan-source",
		Repository:   model.Repository{Provider: "local", FullName: "sample"},
		Summary:      "sample project",
		Security: model.CodexProjectSecurity{
			Verdict: "mostly-contextual", Summary: "review network access",
			Confidence: 0.8, LocalHighestRisk: model.RiskLow,
		},
		InstallationMethods: []model.CodexProjectInstallMethod{{
			Kind: "skills-only", Title: "Install Skills", Description: "transactional copy",
			Supported: true, Required: true, EvidenceFiles: []string{"SKILL.md"},
		}},
		ContextMode: "layered", ContextFileCount: 2, SummaryFileCount: 2,
		DeepAnalysisFileCount: 1, FocusFiles: []string{"SKILL.md"}, ContextDigest: "context",
	}
	first, err := ProjectScanResultDigest(result)
	if err != nil {
		t.Fatal(err)
	}
	result.Security.Verdict = "high-risk"
	second, err := ProjectScanResultDigest(result)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("security conclusion tampering did not change project scan digest")
	}
}

func TestProjectScanSynthesisPayloadStaysBoundedAtCoverageLimits(t *testing.T) {
	metadata := make([]installAnalysisFile, 0, 400)
	for index := 0; index < 400; index++ {
		metadata = append(metadata, installAnalysisFile{
			Path: fmt.Sprintf("references/%03d-%s.md", index, strings.Repeat("p", 96)),
			Kind: "text", Size: 10, SHA256: strings.Repeat("a", 64),
		})
	}
	summaries := make([]contextChunkSummary, 0, maxContextChunks)
	for index := 0; index < maxContextChunks; index++ {
		signals := make([]contextChunkSignal, 4)
		for signalIndex := range signals {
			signals[signalIndex] = contextChunkSignal{
				Category: "security", Title: strings.Repeat("t", 140),
				Description:   strings.Repeat("d", 450),
				EvidenceFiles: []string{"SKILL.md"},
			}
		}
		summaries = append(summaries, contextChunkSummary{
			ChunkIndex: index + 1, ChunkDigest: fmt.Sprintf("digest-%d", index),
			ReviewedFileCount: 10, Summary: strings.Repeat("s", 900), Signals: signals,
		})
	}
	focus := make([]installAnalysisFile, 0, maxProjectScanFocusFiles)
	for index := 0; index < maxProjectScanFocusFiles; index++ {
		focus = append(focus, installAnalysisFile{
			Path: fmt.Sprintf("focus/%03d.go", index), Kind: "text",
			Content: strings.Repeat("x", maxProjectScanFocusBytes/maxProjectScanFocusFiles),
		})
	}
	payload, err := buildProjectScanInput(
		model.InstallPreview{Repository: model.Repository{Provider: "local", FullName: "large"}},
		"en-US", metadata, summaries, focus, 1000, 600,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) > maxCodexInputBytes {
		t.Fatalf("project scan payload = %d bytes, limit = %d", len(payload), maxCodexInputBytes)
	}
}
