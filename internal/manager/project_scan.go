package manager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/codexreview"
	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

func projectScanProgressStages() []model.AssistedInstallProgressStep {
	return []model.AssistedInstallProgressStep{
		{ID: "inventory", Kind: "inventory", Title: "Repository inventory", Status: "queued"},
		{ID: "codex-analysis", Kind: "codex-analysis", Title: "File summaries and focused analysis", Status: "queued"},
		{ID: "finalizing", Kind: "finalizing", Title: "Persist project scan", Status: "queued"},
	}
}

// ScanProjectWithCodex is the opt-in, read-only first phase of assisted
// installation. It creates no permissions, dependency locks, configuration
// changes, or installation transaction.
func (m *Manager) ScanProjectWithCodex(
	ctx context.Context,
	sourcePlanID string,
	progress AssistedInstallProgressFunc,
) (model.CodexProjectScanResult, error) {
	sourcePlanID = strings.TrimSpace(sourcePlanID)
	preview, err := m.assistedSourcePreview(sourcePlanID)
	if err != nil {
		return model.CodexProjectScanResult{}, err
	}
	if time.Now().UTC().After(preview.ExpiresAt) {
		return model.CodexProjectScanResult{}, errors.New("source install plan has expired")
	}
	analysisContext, cancel := context.WithCancel(ctx)
	startedAt := time.Now().UTC()
	analysisRunID := "tx-analysis-" + startedAt.Format("20060102T150405.000000000")
	if err := m.registerAssistedCancel([]string{sourcePlanID, analysisRunID}, cancel); err != nil {
		cancel()
		return model.CodexProjectScanResult{}, err
	}
	defer func() {
		cancel()
		m.unregisterAssistedCancel([]string{sourcePlanID, analysisRunID})
	}()

	m.codexMu.Lock()
	defer m.codexMu.Unlock()
	cfg := m.Config.CodexReview
	cfg.OutputLocale = m.Config.Locale
	sequencer := assistedAnalysisProgressSequencer{
		referenceID: sourcePlanID,
		runID:       analysisRunID,
		startedAt:   startedAt,
		steps:       projectScanProgressStages(),
	}
	emit := func(value model.AssistedInstallProgress, terminal bool) {
		normalized, ok := sequencer.next(value, terminal)
		if ok {
			m.rememberAssistedProgress(normalized, progress)
		}
	}
	fail := func(phase string, cause error) {
		emit(model.AssistedInstallProgress{
			Phase: phase,
			Message: m.assistedMessage(
				"Codex 项目扫描失败",
				"Codex project scan failed",
			),
			Error: cause.Error(),
		}, true)
	}

	scan, err := codexreview.AnalyzeProject(
		analysisContext,
		cfg,
		preview,
		m.Config.Paths.StagingRoot,
		func(phase, message string, current, total int) {
			if phase == "completed" {
				emit(model.AssistedInstallProgress{
					Phase: "finalizing", Message: "Persisting the verified project scan",
					CompletedSteps: current, TotalSteps: total,
				}, false)
				return
			}
			emit(model.AssistedInstallProgress{
				Phase: phase, Message: message,
				CompletedSteps: current, TotalSteps: total,
				ActivityCount: current,
			}, false)
		},
	)
	if err != nil {
		fail("failed", err)
		return model.CodexProjectScanResult{}, err
	}
	if err := saveProjectScan(m.Config.Paths.DataRoot, scan); err != nil {
		fail("failed", err)
		return model.CodexProjectScanResult{}, fmt.Errorf("save project scan: %w", err)
	}
	emit(model.AssistedInstallProgress{
		Phase:          "completed",
		Message:        "Project scan completed; no installation action was executed",
		CompletedSteps: 1,
		TotalSteps:     1,
	}, true)
	return scan, nil
}

// GetProjectScan loads a completed scan by scan ID or by its source plan ID.
// The latter makes a background scan recoverable after the desktop window is
// reopened without treating a scan as an installation plan.
func (m *Manager) GetProjectScan(reference string) (model.CodexProjectScanResult, error) {
	reference = strings.TrimSpace(reference)
	var scan model.CodexProjectScanResult
	if validProjectScanID(reference) {
		var err error
		scan, err = loadProjectScan(m.Config.Paths.DataRoot, reference)
		if err != nil {
			return model.CodexProjectScanResult{}, err
		}
	} else if strings.HasPrefix(reference, "plan-") {
		scan, _ = latestProjectScanForSource(m.Config.Paths.DataRoot, reference)
		if scan.ID == "" {
			return model.CodexProjectScanResult{}, fmt.Errorf("project scan for source %q was not found", reference)
		}
	} else {
		return model.CodexProjectScanResult{}, errors.New("invalid project scan reference")
	}
	preview, err := m.assistedSourcePreview(scan.SourcePlanID)
	if err != nil {
		return model.CodexProjectScanResult{}, fmt.Errorf("load project scan source plan: %w", err)
	}
	if scan.Status != "completed" {
		return model.CodexProjectScanResult{}, fmt.Errorf("project scan is not complete: %s", scan.Status)
	}
	if !scan.ExpiresAt.IsZero() && time.Now().UTC().After(scan.ExpiresAt) {
		return model.CodexProjectScanResult{}, errors.New("project scan has expired")
	}
	if scan.SourcePlanID != preview.ID || scan.ContextDigest == "" {
		return model.CodexProjectScanResult{}, errors.New("project scan is not bound to its source plan")
	}
	if err := codexreview.VerifyAssistedInstallContext(preview.StagingPath, scan.ContextDigest); err != nil {
		return model.CodexProjectScanResult{}, err
	}
	digest, err := codexreview.ProjectScanResultDigest(scan)
	if err != nil {
		return model.CodexProjectScanResult{}, err
	}
	if !strings.EqualFold(strings.TrimSpace(scan.ScanDigest), digest) {
		return model.CodexProjectScanResult{}, errors.New("project scan integrity digest mismatch")
	}
	return scan, nil
}

func projectScanRoot(dataRoot string) string {
	return filepath.Join(dataRoot, "project-scans")
}

func saveProjectScan(dataRoot string, scan model.CodexProjectScanResult) error {
	if !validProjectScanID(scan.ID) {
		return errors.New("invalid project scan ID")
	}
	return writeJSONAtomic(filepath.Join(projectScanRoot(dataRoot), scan.ID+".json"), scan)
}

func loadProjectScan(dataRoot, scanID string) (model.CodexProjectScanResult, error) {
	if !validProjectScanID(scanID) {
		return model.CodexProjectScanResult{}, errors.New("invalid project scan ID")
	}
	data, err := os.ReadFile(filepath.Join(projectScanRoot(dataRoot), scanID+".json"))
	if err != nil {
		return model.CodexProjectScanResult{}, err
	}
	var scan model.CodexProjectScanResult
	if err := json.Unmarshal(data, &scan); err != nil {
		return model.CodexProjectScanResult{}, err
	}
	if scan.ID != scanID {
		return model.CodexProjectScanResult{}, errors.New("project scan identity mismatch")
	}
	return scan, nil
}

func latestProjectScanForSource(dataRoot, sourcePlanID string) (model.CodexProjectScanResult, error) {
	entries, err := os.ReadDir(projectScanRoot(dataRoot))
	if err != nil {
		return model.CodexProjectScanResult{}, err
	}
	var selected model.CodexProjectScanResult
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "project-scan-") ||
			!strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		scanID := strings.TrimSuffix(entry.Name(), ".json")
		scan, loadErr := loadProjectScan(dataRoot, scanID)
		if loadErr != nil || scan.SourcePlanID != sourcePlanID || scan.Status != "completed" {
			continue
		}
		if selected.ID == "" || scan.CompletedAt.After(selected.CompletedAt) {
			selected = scan
		}
	}
	return selected, nil
}

func validProjectScanID(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > 160 || filepath.Base(value) != value ||
		!strings.HasPrefix(value, "project-scan-") {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') && char != '-' && char != '.' {
			return false
		}
	}
	return true
}
