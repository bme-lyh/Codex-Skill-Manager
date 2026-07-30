package manager

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/codexreview"
	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

func TestAssistedInstallRunsAndRollsBackThroughParentTransaction(t *testing.T) {
	m := newTestManager(t)
	preview := assistedLocalPreview(t, m, "demo")
	plan := finalizedAssistedManagerPlan(t, preview, []model.AssistedInstallStep{{
		ID: "install-skills", Kind: model.AssistedInstallStepInstallSkills,
		Title: "Install Skills", Description: "Install the discovered Skill.",
		SkillNames: []string{"demo"},
	}})
	if err := m.storeAssistedPlan(plan); err != nil {
		t.Fatal(err)
	}

	var sequences []uint64
	result, err := m.ApplyAssistedInstall(
		context.Background(),
		plan.ID,
		[]string{"demo"},
		[]string{model.AssistedInstallPermissionSkillsWrite},
		"",
		func(progress model.AssistedInstallProgress) {
			sequences = append(sequences, progress.Sequence)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Transaction.Type != "assisted-install" ||
		result.Transaction.Status != "completed" ||
		result.Transaction.RecoveryStatus != "available" {
		t.Fatalf("unexpected assisted transaction: %#v", result.Transaction)
	}
	if len(result.Transaction.Steps) != 1 ||
		result.Transaction.Steps[0].ChildTransactionID == "" {
		t.Fatalf("child Skill transaction was not journaled: %#v", result.Transaction.Steps)
	}
	if _, err := os.Stat(filepath.Join(m.Config.Paths.SkillsRoot, "demo", "SKILL.md")); err != nil {
		t.Fatalf("Skill was not installed: %v", err)
	}
	for index := 1; index < len(sequences); index++ {
		if sequences[index] <= sequences[index-1] {
			t.Fatalf("progress sequence regressed: %#v", sequences)
		}
	}

	rollback, err := m.Rollback(result.Transaction.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rollback.Type != "rollback-assisted-install" || rollback.Status != "completed" {
		t.Fatalf("unexpected assisted rollback: %#v", rollback)
	}
	if _, err := os.Stat(filepath.Join(m.Config.Paths.SkillsRoot, "demo")); !os.IsNotExist(err) {
		t.Fatalf("newly installed Skill was not removed to quarantine: %v", err)
	}
	restoredPlan, err := m.GetAssistedInstallPlan(plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restoredPlan.Status != "rolled-back" || restoredPlan.RecoveryStatus != "completed" {
		t.Fatalf("plan recovery state was not persisted: %#v", restoredPlan)
	}
}

func TestAssistedInstallRejectsContextDriftBeforeTransaction(t *testing.T) {
	m := newTestManager(t)
	preview := assistedLocalPreview(t, m, "demo")
	plan := finalizedAssistedManagerPlan(t, preview, []model.AssistedInstallStep{{
		ID: "install-skills", Kind: model.AssistedInstallStepInstallSkills,
		Title: "Install Skills", Description: "Install the discovered Skill.",
		SkillNames: []string{"demo"},
	}})
	if err := m.storeAssistedPlan(plan); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(preview.StagingPath, "README.md"),
		[]byte("changed after approval"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ApplyAssistedInstall(
		context.Background(),
		plan.ID,
		[]string{"demo"},
		[]string{model.AssistedInstallPermissionSkillsWrite},
		"",
		nil,
	); err == nil || !strings.Contains(err.Error(), "context changed") {
		t.Fatalf("expected complete repository drift rejection, got %v", err)
	}
	history, err := m.History(50)
	if err != nil {
		t.Fatal(err)
	}
	for _, transaction := range history {
		if transaction.Type == "assisted-install" {
			t.Fatalf("drifted context started a mutation transaction: %#v", transaction)
		}
	}
}

func TestAssistedInstallRequiresExplicitDerivedPermission(t *testing.T) {
	m := newTestManager(t)
	preview := assistedLocalPreview(t, m, "demo")
	plan := finalizedAssistedManagerPlan(t, preview, []model.AssistedInstallStep{{
		ID: "install-skills", Kind: model.AssistedInstallStepInstallSkills,
		Title: "Install Skills", Description: "Install the discovered Skill.",
		SkillNames: []string{"demo"},
	}})
	if err := m.storeAssistedPlan(plan); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ApplyAssistedInstall(
		context.Background(),
		plan.ID,
		[]string{"demo"},
		nil,
		"",
		nil,
	); err == nil || !strings.Contains(err.Error(), "required permission") {
		t.Fatalf("expected explicit permission rejection, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(m.Config.Paths.SkillsRoot, "demo")); !os.IsNotExist(err) {
		t.Fatalf("permission rejection wrote a Skill: %v", err)
	}
}

func TestAssistedInstallCanSkipOptionalToolAndMCPWithoutProjectRoot(t *testing.T) {
	m := newTestManager(t)
	preview := assistedLocalPreview(t, m, "demo")
	plan := finalizedAssistedManagerPlan(t, preview, []model.AssistedInstallStep{
		{
			ID: "install-skills", Kind: model.AssistedInstallStepInstallSkills,
			Title: "Install Skills", Description: "Install the discovered Skill.",
			SkillNames: []string{"demo"},
		},
		{
			ID: "managed-tool", Kind: model.AssistedInstallStepManagedPythonTool,
			Title: "Install managed tool", Description: "Create the optional MCP runtime.",
			PythonPackage: "code-review-graph", VersionSpec: "==2.3.7",
			Entrypoint: "code-review-graph", Required: false,
			PythonWheels: []model.AssistedPythonWheelLock{{
				Name: "code-review-graph", Version: "2.3.7",
				Filename: "code_review_graph-2.3.7-py3-none-any.whl",
				SHA256:   strings.Repeat("a", 64), Tags: []string{"py3-none-any"},
			}},
		},
		{
			ID: "configure-mcp", Kind: model.AssistedInstallStepConfigureCodexMCP,
			Title: "Configure MCP", Description: "Add the optional Codex MCP entry.",
			Entrypoint: "code-review-graph", MCPServerName: "code_review_graph",
			MCPArgs: []string{"serve"}, Required: false,
		},
	})
	if !plan.NeedsProjectRoot {
		t.Fatal("finalized MCP plan did not describe its project-root requirement")
	}
	if err := m.storeAssistedPlan(plan); err != nil {
		t.Fatal(err)
	}
	result, err := m.ApplyAssistedInstall(
		context.Background(),
		plan.ID,
		[]string{"demo"},
		[]string{model.AssistedInstallPermissionSkillsWrite},
		"",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Plan.Steps[1].Status != "skipped" || result.Plan.Steps[2].Status != "skipped" {
		t.Fatalf("optional unapproved integration steps were not skipped: %#v", result.Plan.Steps)
	}
}

func TestAssistedInstallExecutesSupportedStepsAndLeavesRequiredManualPending(t *testing.T) {
	m := newTestManager(t)
	preview := assistedLocalPreview(t, m, "demo")
	plan := finalizedAssistedManagerPlan(t, preview, []model.AssistedInstallStep{
		{
			ID: "install-skills", Kind: model.AssistedInstallStepInstallSkills,
			Title: "Install Skills", Description: "Install the discovered Skill.",
			SkillNames: []string{"demo"},
		},
		{
			ID: "manual-registration", Kind: model.AssistedInstallStepManual,
			Title:       "Review external registration",
			Description: "Complete the documented external registration manually.",
			Required:    true,
		},
	})
	if plan.Status != "manual-required" {
		t.Fatalf("expected a manual-required review plan, got %s", plan.Status)
	}
	if err := m.storeAssistedPlan(plan); err != nil {
		t.Fatal(err)
	}
	result, err := m.ApplyAssistedInstall(
		context.Background(),
		plan.ID,
		[]string{"demo"},
		[]string{model.AssistedInstallPermissionSkillsWrite},
		"",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Plan.Status != "partial" || result.Transaction.Status != "partial" ||
		result.Progress == nil || result.Progress.Phase != "partial" ||
		!result.Progress.Terminal {
		t.Fatalf("manual remainder did not finish as a partial success: %#v", result)
	}
	if result.Plan.RecoveryStatus != "available" ||
		result.Transaction.RecoveryStatus != "available" {
		t.Fatalf("partial installation is not recoverable: %#v", result.Transaction)
	}
	if result.Plan.Steps[0].Status != "completed" ||
		result.Plan.Steps[1].Status != "manual-pending" {
		t.Fatalf("unexpected partial step states: %#v", result.Plan.Steps)
	}
	if result.Progress.CompletedSteps != result.Progress.TotalSteps {
		t.Fatalf("manual-pending step was not counted as processed: %#v", result.Progress)
	}
	if _, err := os.Stat(filepath.Join(m.Config.Paths.SkillsRoot, "demo", "SKILL.md")); err != nil {
		t.Fatalf("supported Skill step was not executed: %v", err)
	}
	if _, err := m.ApplyAssistedInstall(
		context.Background(),
		plan.ID,
		[]string{"demo"},
		[]string{model.AssistedInstallPermissionSkillsWrite},
		"",
		nil,
	); err == nil || !strings.Contains(err.Error(), "cannot run from status partial") {
		t.Fatalf("partial plan incorrectly allowed an automatic retry: %v", err)
	}
}

func TestAssistedInstallPersistsExactSelectedSkillSubset(t *testing.T) {
	m := newTestManager(t)
	source := t.TempDir()
	writeTestSkill(t, source, "alpha")
	writeTestSkill(t, source, "beta")
	if err := os.WriteFile(
		filepath.Join(source, "README.md"),
		[]byte("multi-Skill repository"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	preview, err := m.PrepareLocal(source)
	if err != nil {
		t.Fatal(err)
	}
	plan := finalizedAssistedManagerPlan(t, preview, []model.AssistedInstallStep{{
		ID: "install-skills", Kind: model.AssistedInstallStepInstallSkills,
		Title: "Install Skills", Description: "Install the selected Skills.",
		SkillNames: []string{"alpha", "beta"},
	}})
	if len(plan.SelectedSkills) != 0 {
		t.Fatalf("new analysis unexpectedly selected Skills: %#v", plan.SelectedSkills)
	}
	if err := m.storeAssistedPlan(plan); err != nil {
		t.Fatal(err)
	}
	result, err := m.ApplyAssistedInstall(
		context.Background(),
		plan.ID,
		[]string{"beta"},
		[]string{model.AssistedInstallPermissionSkillsWrite},
		"",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Plan.SelectedSkills) != 1 || result.Plan.SelectedSkills[0] != "beta" {
		t.Fatalf("result expanded the selected subset: %#v", result.Plan.SelectedSkills)
	}
	if len(result.Transaction.Targets) != 1 || result.Transaction.Targets[0] != "beta" {
		t.Fatalf("transaction expanded the selected subset: %#v", result.Transaction.Targets)
	}
	persisted, err := loadAssistedInstallPlan(m.Config.Paths.DataRoot, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted.SelectedSkills) != 1 || persisted.SelectedSkills[0] != "beta" {
		t.Fatalf("persisted JSON expanded the selected subset: %#v", persisted.SelectedSkills)
	}
	if _, err := os.Stat(filepath.Join(m.Config.Paths.SkillsRoot, "alpha")); !os.IsNotExist(err) {
		t.Fatalf("unselected Skill was installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(m.Config.Paths.SkillsRoot, "beta", "SKILL.md")); err != nil {
		t.Fatalf("selected Skill was not installed: %v", err)
	}
}

func TestAssistedPlanCanBeRecoveredBySourceReference(t *testing.T) {
	m := newTestManager(t)
	preview := assistedLocalPreview(t, m, "demo")
	plan := finalizedAssistedManagerPlan(t, preview, []model.AssistedInstallStep{{
		ID: "install-skills", Kind: model.AssistedInstallStepInstallSkills,
		Title: "Install Skills", Description: "Install the discovered Skill.",
		SkillNames: []string{"demo"},
	}})
	if err := m.storeAssistedPlan(plan); err != nil {
		t.Fatal(err)
	}
	m.assistData.Lock()
	delete(m.assisted, plan.ID)
	m.assistData.Unlock()
	recovered, err := m.GetAssistedInstallPlan(plan.SourcePlanID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.ID != plan.ID || recovered.SourcePlanID != preview.ID {
		t.Fatalf("unexpected source-reference recovery: %#v", recovered)
	}
}

func TestRestartMarksOrphanedAssistedPlanAndTransactionRecoverable(t *testing.T) {
	m := newTestManager(t)
	preview := assistedLocalPreview(t, m, "demo")
	plan := finalizedAssistedManagerPlan(t, preview, []model.AssistedInstallStep{{
		ID: "install-skills", Kind: model.AssistedInstallStepInstallSkills,
		Title: "Install Skills", Description: "Install the discovered Skill.",
		SkillNames: []string{"demo"},
	}})
	started := time.Now().UTC().Add(-time.Minute)
	transactionID := "tx-orphaned-assisted-install"
	plan.Status = "running"
	plan.TransactionID = transactionID
	plan.RecoveryStatus = ""
	plan.Steps[0].Status = "running"
	plan.Steps[0].StartedAt = &started
	tx := model.Transaction{
		ID: transactionID, Type: "assisted-install", Status: "running",
		Targets: []string{"demo"}, StartedAt: started,
		Steps: cloneAssistedSteps(plan.Steps),
	}
	if err := m.store.SaveTransaction(tx); err != nil {
		t.Fatal(err)
	}
	if err := m.storeAssistedPlan(plan); err != nil {
		t.Fatal(err)
	}

	restarted, err := Open(m.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	dashboard, err := restarted.Dashboard()
	if err != nil {
		t.Fatal(err)
	}
	var history model.Transaction
	for _, candidate := range dashboard.RecentHistory {
		if candidate.ID == transactionID {
			history = candidate
			break
		}
	}
	if history.ID == "" || history.Status != "interrupted" ||
		history.RecoveryStatus != "required" {
		t.Fatalf("orphaned transaction was not exposed for recovery: %#v", history)
	}
	persistedPlan, err := restarted.GetAssistedInstallPlan(plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persistedPlan.Status != "interrupted" ||
		persistedPlan.RecoveryStatus != "required" ||
		persistedPlan.Steps[0].Status != "interrupted" {
		t.Fatalf("orphaned plan was not persisted as interrupted: %#v", persistedPlan)
	}
	persistedTransaction, err := restarted.store.Transaction(transactionID)
	if err != nil {
		t.Fatal(err)
	}
	if persistedTransaction.Status != "interrupted" ||
		persistedTransaction.RecoveryStatus != "required" ||
		persistedTransaction.CompletedAt.IsZero() {
		t.Fatalf("orphaned transaction checkpoint was incomplete: %#v", persistedTransaction)
	}
	rollback, err := restarted.Rollback(transactionID)
	if err != nil {
		t.Fatal(err)
	}
	if rollback.Status != "completed" || rollback.RecoveryStatus != "completed" {
		t.Fatalf("orphaned transaction could not be rolled back: %#v", rollback)
	}
}

func TestOrphanedAssistedAnalysisProgressBecomesInterrupted(t *testing.T) {
	m := newTestManager(t)
	started := time.Now().UTC().Add(-time.Minute)
	progress := model.AssistedInstallProgress{
		ReferenceID: "plan-source-analysis",
		RunID:       "tx-analysis-orphaned",
		Sequence:    4,
		Phase:       "codex-analysis",
		Message:     "running",
		StartedAt:   started,
		UpdatedAt:   started,
		Terminal:    false,
	}
	if err := saveAssistedInstallProgress(
		m.Config.Paths.DataRoot,
		progress.ReferenceID,
		progress,
	); err != nil {
		t.Fatal(err)
	}
	recovered, err := m.GetAssistedInstallProgress(progress.ReferenceID)
	if err != nil {
		t.Fatal(err)
	}
	if !recovered.Terminal || recovered.Phase != "interrupted" ||
		recovered.Sequence != progress.Sequence+1 || recovered.CompletedAt == nil {
		t.Fatalf("orphaned analysis was not closed safely: %#v", recovered)
	}
	persisted, err := loadAssistedInstallProgress(m.Config.Paths.DataRoot, progress.ReferenceID)
	if err != nil {
		t.Fatal(err)
	}
	if !persisted.Terminal || persisted.Phase != "interrupted" {
		t.Fatalf("interrupted analysis was not persisted: %#v", persisted)
	}
}

func TestAssistedAnalysisProgressSequencerUsesOneRunAndOneTerminal(t *testing.T) {
	started := time.Now().UTC().Add(-time.Second)
	sequencer := assistedAnalysisProgressSequencer{
		referenceID: "plan-source",
		runID:       "tx-analysis-1",
		startedAt:   started,
	}
	var values []model.AssistedInstallProgress
	appendProgress := func(input model.AssistedInstallProgress, terminal bool) {
		t.Helper()
		value, ok := sequencer.next(input, terminal)
		if !ok {
			t.Fatal("sequencer rejected a pre-terminal update")
		}
		values = append(values, value)
	}
	appendProgress(model.AssistedInstallProgress{
		RunID: "codex-run", Phase: "inventory", CurrentStepID: "inventory",
		Steps: []model.AssistedInstallProgressStep{
			{ID: "inventory", Status: "running"},
			{ID: "codex-analysis", Status: "queued"},
			{ID: "validation", Status: "queued"},
		},
	}, false)
	appendProgress(model.AssistedInstallProgress{
		RunID: "codex-run", Phase: "analyzing", CurrentStepID: "codex-analysis",
		ActivityCount: 3,
		Steps: []model.AssistedInstallProgressStep{
			{ID: "inventory", Status: "completed"},
			{ID: "codex-analysis", Status: "running"},
			{ID: "validation", Status: "queued"},
		},
	}, false)
	appendProgress(model.AssistedInstallProgress{
		RunID: "codex-run", Phase: "validation", CurrentStepID: "validation",
		ActivityCount: 2,
		Steps: []model.AssistedInstallProgressStep{
			{ID: "inventory", Status: "completed"},
			{ID: "codex-analysis", Status: "completed"},
			{ID: "validation", Status: "running"},
		},
	}, false)
	appendProgress(model.AssistedInstallProgress{
		RunID: "lock-run", Phase: "dependency-lock", CurrentStepID: "dependency-lock",
		ActivityCount: sequencer.bumpActivity(),
		Message:       "Locking package 1/2",
		// Deliberately supply child-package counters. They must not replace the
		// fixed analysis stages or global completed count.
		CompletedSteps: 0,
		TotalSteps:     2,
	}, false)
	appendProgress(model.AssistedInstallProgress{
		RunID: "lock-run", Phase: "dependency-lock", CurrentStepID: "dependency-lock",
		ActivityCount:  sequencer.bumpActivity(),
		Message:        "Locking package 2/2",
		CompletedSteps: 1,
		TotalSteps:     2,
	}, false)
	appendProgress(model.AssistedInstallProgress{
		RunID: "plan-run", Phase: "finalizing", CurrentStepID: "finalizing",
	}, false)
	appendProgress(model.AssistedInstallProgress{
		RunID: "plan-run", Phase: "ready",
	}, true)
	if _, ok := sequencer.next(model.AssistedInstallProgress{Phase: "failed"}, true); ok {
		t.Fatal("sequencer emitted more than one terminal update")
	}
	previousCompleted := 0
	previousActivity := 0
	completedIDs := map[string]bool{}
	for index, value := range values {
		if value.ReferenceID != "plan-source" || value.RunID != "tx-analysis-1" {
			t.Fatalf("progress identity changed at %d: %#v", index, value)
		}
		if value.Sequence != uint64(index+1) {
			t.Fatalf("progress sequence is not monotonic: %#v", values)
		}
		if !value.StartedAt.Equal(started) {
			t.Fatalf("progress start time changed at %d: %#v", index, value)
		}
		if value.TotalSteps != 5 || len(value.Steps) != 5 {
			t.Fatalf("analysis stage structure changed at %d: %#v", index, value)
		}
		if value.CompletedSteps < previousCompleted {
			t.Fatalf("completed stage count regressed at %d: %#v", index, values)
		}
		if value.ActivityCount < previousActivity {
			t.Fatalf("activity count regressed at %d: %#v", index, values)
		}
		currentCompleted := map[string]bool{}
		for _, step := range value.Steps {
			if step.Status == "completed" {
				currentCompleted[step.ID] = true
			}
		}
		for id := range completedIDs {
			if !currentCompleted[id] {
				t.Fatalf("completed stage %q regressed at event %d: %#v", id, index, values)
			}
		}
		completedIDs = currentCompleted
		previousCompleted = value.CompletedSteps
		previousActivity = value.ActivityCount
	}
	for index := range values[:len(values)-1] {
		if values[index].Terminal {
			t.Fatalf("non-final event %d was terminal: %#v", index, values[index])
		}
	}
	last := values[len(values)-1]
	if !last.Terminal || last.CompletedAt == nil || last.CompletedSteps != last.TotalSteps {
		t.Fatalf("terminal contract is invalid: %#v", values)
	}
}

func TestCheckpointAssistedInstallReportsPlanFailureAndStillJournalsTransaction(t *testing.T) {
	m := newTestManager(t)
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	originalDataRoot := m.Config.Paths.DataRoot
	m.Config.Paths.DataRoot = blocker
	t.Cleanup(func() {
		m.Config.Paths.DataRoot = originalDataRoot
	})
	plan := model.AssistedInstallPlan{
		ID:     "assisted-plan-checkpoint",
		Status: "running",
		Steps: []model.AssistedInstallStep{{
			ID: "step-1", Kind: model.AssistedInstallStepInstallSkills,
			Status: "running",
		}},
	}
	tx := model.Transaction{
		ID: "tx-checkpoint", Type: "assisted-install", Status: "running",
		StartedAt: time.Now().UTC(),
	}
	err := m.checkpointAssistedInstall(plan, tx)
	if err == nil || !strings.Contains(err.Error(), "plan checkpoint") {
		t.Fatalf("expected the plan persistence error to be returned, got %v", err)
	}
	stored, storeErr := m.store.Transaction(tx.ID)
	if storeErr != nil {
		t.Fatalf("transaction journal was not attempted after the plan failure: %v", storeErr)
	}
	if len(stored.Steps) != 1 || stored.Steps[0].Status != "running" {
		t.Fatalf("transaction journal did not retain recovery intent: %#v", stored)
	}
}

func TestValidAssistedReferenceIDRejectsPathAndStreamSyntax(t *testing.T) {
	for _, value := range []string{
		"",
		"tx-a/b",
		`tx-a\b`,
		"tx-config:stream",
		"tx-name?.json",
		strings.Repeat("a", 161),
	} {
		if validAssistedReferenceID(value) {
			t.Fatalf("unsafe assisted-install reference was accepted: %q", value)
		}
	}
	for _, value := range []string{
		"tx-20260730T120000.000000000",
		"plan-20260730T120000.000000000",
		"assisted-plan-20260730T120000.000000000",
	} {
		if !validAssistedReferenceID(value) {
			t.Fatalf("valid assisted-install reference was rejected: %q", value)
		}
	}
}

func TestRegisterAssistedCancelRejectsDuplicateReference(t *testing.T) {
	m := &Manager{cancels: map[string]context.CancelFunc{}}
	_, firstCancel := context.WithCancel(context.Background())
	defer firstCancel()
	if err := m.registerAssistedCancel([]string{"plan-active", "tx-active"}, firstCancel); err != nil {
		t.Fatal(err)
	}

	_, duplicateCancel := context.WithCancel(context.Background())
	defer duplicateCancel()
	if err := m.registerAssistedCancel([]string{"plan-active", "tx-other"}, duplicateCancel); err == nil ||
		!strings.Contains(err.Error(), "already running") {
		t.Fatalf("duplicate assisted reference was not rejected: %v", err)
	}
	if m.cancels["tx-other"] != nil {
		t.Fatal("a partially rejected registration leaked its second reference")
	}

	m.unregisterAssistedCancel([]string{"plan-active", "tx-active"})
	if err := m.registerAssistedCancel([]string{"plan-active"}, duplicateCancel); err != nil {
		t.Fatalf("reference could not be reused after the active run ended: %v", err)
	}
}

func assistedLocalPreview(t *testing.T, m *Manager, skillName string) model.InstallPreview {
	t.Helper()
	source := t.TempDir()
	writeTestSkill(t, source, skillName)
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("repository context"), 0o600); err != nil {
		t.Fatal(err)
	}
	preview, err := m.PrepareLocal(source)
	if err != nil {
		t.Fatal(err)
	}
	return preview
}

func finalizedAssistedManagerPlan(
	t *testing.T,
	preview model.InstallPreview,
	steps []model.AssistedInstallStep,
) model.AssistedInstallPlan {
	t.Helper()
	digest, count, err := codexreview.AssistedInstallContextDigest(preview.StagingPath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	plan := model.AssistedInstallPlan{
		ID:           "assisted-plan-" + now.Format("20060102T150405.000000000"),
		SourcePlanID: preview.ID, Status: "analyzing", Repository: preview.Repository,
		Summary: "Test repository", Approach: "Install the selected Skills.",
		Complexity: "simple", Steps: steps,
		Skills: preview.Skills, Scan: preview.Scan,
		ContextFileCount: count, ContextDigest: digest,
		CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	plan, err = codexreview.FinalizeAssistedInstallPlan(plan, preview.Skills, "missing")
	if err != nil {
		t.Fatal(err)
	}
	return plan
}
