package manager

import (
	"context"
	"database/sql"
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

	"github.com/bme-lyh/Codex-Skill-Manager/internal/codexreview"
	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

type AssistedInstallProgressFunc func(model.AssistedInstallProgress)

type assistedAnalysisProgressSequencer struct {
	mu          sync.Mutex
	referenceID string
	runID       string
	startedAt   time.Time
	sequence    uint64
	terminal    bool
	steps       []model.AssistedInstallProgressStep
	currentStep string
	activity    int
}

func assistedAnalysisStages() []model.AssistedInstallProgressStep {
	return []model.AssistedInstallProgressStep{
		{ID: "inventory", Kind: "inventory", Title: "Repository inventory", Status: "queued"},
		{ID: "codex-analysis", Kind: "codex-analysis", Title: "Enhanced project scan", Status: "queued"},
		{ID: "validation", Kind: "validation", Title: "Local plan validation", Status: "queued"},
		{ID: "dependency-lock", Kind: "dependency-lock", Title: "Dependency lock", Status: "queued"},
		{ID: "finalizing", Kind: "finalizing", Title: "Plan finalization", Status: "queued"},
	}
}

func (s *assistedAnalysisProgressSequencer) next(
	value model.AssistedInstallProgress,
	terminal bool,
) (model.AssistedInstallProgress, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.terminal {
		return model.AssistedInstallProgress{}, false
	}
	if terminal {
		s.terminal = true
	}
	if len(s.steps) == 0 {
		s.steps = assistedAnalysisStages()
	}
	s.mergeCodexSteps(value.Steps)
	stageID := assistedAnalysisStageID(value.CurrentStepID, value.Phase)
	if stageID == "" {
		stageID = s.currentStep
	}
	if stageID != "" {
		s.markAnalysisStagesBefore(stageID)
		status := "running"
		if value.Error != "" || strings.Contains(strings.ToLower(value.Phase), "failed") ||
			strings.EqualFold(value.Phase, "cancelled") {
			status = "failed"
		}
		s.setAnalysisStage(stageID, status, value.Message, value.Error)
		s.currentStep = stageID
	}
	if terminal && value.Error == "" && !strings.EqualFold(value.Phase, "cancelled") {
		s.completeAllAnalysisStages()
		s.currentStep = ""
	}
	if value.ActivityCount > s.activity {
		s.activity = value.ActivityCount
	}
	s.sequence++
	value.ReferenceID = s.referenceID
	value.RunID = s.runID
	value.Sequence = s.sequence
	value.StartedAt = s.startedAt
	value.UpdatedAt = time.Now().UTC()
	value.Terminal = terminal
	value.CurrentStepID = s.currentStep
	value.ActivityCount = s.activity
	value.Steps = append([]model.AssistedInstallProgressStep(nil), s.steps...)
	value.TotalSteps = len(s.steps)
	value.CompletedSteps = 0
	for _, step := range s.steps {
		if step.Status == "completed" {
			value.CompletedSteps++
		}
	}
	if terminal {
		completedAt := value.UpdatedAt
		value.CompletedAt = &completedAt
	} else {
		value.CompletedAt = nil
	}
	return value, true
}

func (s *assistedAnalysisProgressSequencer) bumpActivity() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activity++
	return s.activity
}

func (s *assistedAnalysisProgressSequencer) mergeCodexSteps(
	incoming []model.AssistedInstallProgressStep,
) {
	for _, step := range incoming {
		stageID := assistedAnalysisStageID(step.ID, step.Kind)
		if stageID == "" {
			continue
		}
		s.setAnalysisStage(stageID, step.Status, step.Message, step.Error)
		for index := range s.steps {
			if s.steps[index].ID != stageID {
				continue
			}
			if step.StartedAt != nil && s.steps[index].StartedAt == nil {
				s.steps[index].StartedAt = step.StartedAt
			}
			if step.CompletedAt != nil && s.steps[index].CompletedAt == nil {
				s.steps[index].CompletedAt = step.CompletedAt
			}
			break
		}
	}
}

func assistedAnalysisStageID(values ...string) string {
	for _, value := range values {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "inventory", "preparing":
			return "inventory"
		case "codex-analysis", "analyzing":
			return "codex-analysis"
		case "validation":
			return "validation"
		case "dependency-lock", "dependency-lock-failed":
			return "dependency-lock"
		case "finalizing", "finalization-failed":
			return "finalizing"
		}
	}
	return ""
}

func (s *assistedAnalysisProgressSequencer) markAnalysisStagesBefore(id string) {
	now := time.Now().UTC()
	for index := range s.steps {
		if s.steps[index].ID == id {
			break
		}
		if s.steps[index].Status == "queued" || s.steps[index].Status == "running" {
			s.steps[index].Status = "completed"
			s.steps[index].CompletedAt = &now
		}
	}
}

func (s *assistedAnalysisProgressSequencer) setAnalysisStage(
	id string,
	status string,
	message string,
	errorMessage string,
) {
	now := time.Now().UTC()
	for index := range s.steps {
		step := &s.steps[index]
		if step.ID != id {
			continue
		}
		switch status {
		case "completed":
			if step.Status != "completed" {
				step.Status = "completed"
				step.CompletedAt = &now
			}
		case "failed", "cancelled":
			if step.Status != "completed" {
				step.Status = "failed"
				step.CompletedAt = &now
			}
		case "running":
			if step.Status == "queued" {
				step.Status = "running"
				step.StartedAt = &now
			}
		}
		if message != "" {
			step.Message = message
		}
		if errorMessage != "" {
			step.Error = errorMessage
		}
		return
	}
}

func (s *assistedAnalysisProgressSequencer) completeAllAnalysisStages() {
	now := time.Now().UTC()
	for index := range s.steps {
		if s.steps[index].Status != "completed" {
			s.steps[index].Status = "completed"
			s.steps[index].CompletedAt = &now
		}
	}
}

// AnalyzeInstallFromProjectScan is the consent boundary for the new assisted
// flow. A completed project scan must be loaded and verified before Codex is
// asked to produce an executable installation proposal.
func (m *Manager) AnalyzeInstallFromProjectScan(
	ctx context.Context,
	projectScanID string,
	progress AssistedInstallProgressFunc,
) (model.AssistedInstallPlan, error) {
	scan, err := m.GetProjectScan(projectScanID)
	if err != nil {
		return model.AssistedInstallPlan{}, err
	}
	return m.analyzeInstallWithCodex(ctx, scan.SourcePlanID, &scan, progress)
}

func (m *Manager) analyzeInstallWithCodex(
	ctx context.Context,
	sourcePlanID string,
	projectScan *model.CodexProjectScanResult,
	progress AssistedInstallProgressFunc,
) (model.AssistedInstallPlan, error) {
	sourcePlanID = strings.TrimSpace(sourcePlanID)
	preview, err := m.assistedSourcePreview(sourcePlanID)
	if err != nil {
		return model.AssistedInstallPlan{}, err
	}
	if _, err := m.enforceProjectAssessmentGate(preview); err != nil {
		return model.AssistedInstallPlan{}, fmt.Errorf("project assessment does not permit plan generation: %w", err)
	}
	if time.Now().UTC().After(preview.ExpiresAt) {
		return model.AssistedInstallPlan{}, errors.New("source install plan has expired")
	}
	if projectScan != nil {
		if projectScan.SourcePlanID != preview.ID || projectScan.Status != "completed" {
			return model.AssistedInstallPlan{}, errors.New("a completed project scan for this source is required")
		}
		if _, err := m.GetProjectScan(projectScan.ID); err != nil {
			return model.AssistedInstallPlan{}, err
		}
	}

	analysisContext, cancel := context.WithCancel(ctx)
	startedAt := time.Now().UTC()
	analysisRunID := "tx-analysis-" + startedAt.Format("20060102T150405.000000000")
	if err := m.registerAssistedCancel([]string{sourcePlanID, analysisRunID}, cancel); err != nil {
		cancel()
		return model.AssistedInstallPlan{}, err
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
	}
	emit := func(value model.AssistedInstallProgress, terminal bool) {
		normalized, ok := sequencer.next(value, terminal)
		if ok {
			m.rememberAssistedProgress(normalized, progress)
		}
	}
	failAnalysis := func(phase string, cause error) {
		message := m.assistedMessage(
			"安装计划生成失败",
			"Installation planning failed",
		)
		if errors.Is(cause, context.Canceled) {
			phase = "cancelled"
			message = m.assistedMessage(
				"安装计划生成已取消",
				"Installation planning was cancelled",
			)
		}
		emit(model.AssistedInstallProgress{
			Phase:   phase,
			Message: message,
			Error:   cause.Error(),
		}, true)
	}
	relay := func(value model.AssistedInstallProgress) {
		// Codex reports only one stage of this manager-owned run. Suppress its
		// internal terminal marker so dependency locking and finalization remain
		// on the same stable run and the manager emits exactly one terminal.
		value.Terminal = false
		value.CompletedAt = nil
		emit(value, false)
	}

	var plan model.AssistedInstallPlan
	if projectScan != nil {
		plan, err = codexreview.AnalyzeInstallWithProjectScan(
			analysisContext,
			cfg,
			preview,
			m.Config.Paths.StagingRoot,
			projectScan,
			codexreview.AssistedInstallProgressFunc(relay),
		)
	} else {
		plan, err = codexreview.AnalyzeInstall(
			analysisContext,
			cfg,
			preview,
			m.Config.Paths.StagingRoot,
			codexreview.AssistedInstallProgressFunc(relay),
		)
	}
	if err != nil {
		failAnalysis("failed", err)
		return model.AssistedInstallPlan{}, err
	}
	if err := codexreview.VerifyAssistedInstallContext(preview.StagingPath, plan.ContextDigest); err != nil {
		failAnalysis("failed", err)
		return model.AssistedInstallPlan{}, err
	}
	emit(model.AssistedInstallProgress{
		Phase:   "dependency-lock",
		Message: m.assistedMessage("正在准备 Python 依赖锁", "Preparing the Python dependency lock"),
	}, false)
	plan, err = m.lockAssistedPythonDependencies(
		analysisContext,
		plan,
		func(step model.AssistedInstallStep, completed int, total int) {
			emit(model.AssistedInstallProgress{
				Phase: "dependency-lock",
				Message: m.assistedMessage(
					fmt.Sprintf("正在锁定 Python 依赖（%d/%d）：%s%s", completed+1, total, step.PythonPackage, step.VersionSpec),
					fmt.Sprintf("Locking Python dependencies (%d/%d): %s%s", completed+1, total, step.PythonPackage, step.VersionSpec),
				),
				CurrentStepID: "dependency-lock",
				ActivityCount: sequencer.bumpActivity(),
			}, false)
		},
	)
	if err != nil {
		failAnalysis("dependency-lock-failed", err)
		return model.AssistedInstallPlan{}, err
	}
	emit(model.AssistedInstallProgress{
		Phase:   "finalizing",
		Message: m.assistedMessage("正在校验并保存安装计划", "Validating and saving the installation plan"),
	}, false)
	configPath, err := m.codexConfigPath()
	if err != nil {
		failAnalysis("finalization-failed", err)
		return model.AssistedInstallPlan{}, err
	}
	configFingerprint, err := fileFingerprint(configPath)
	if err != nil {
		failAnalysis("finalization-failed", err)
		return model.AssistedInstallPlan{}, err
	}
	if projectScan != nil {
		plan.ProjectScanID = projectScan.ID
		plan.ProjectScanDigest = projectScan.ScanDigest
	}
	plan.TargetRootID = preview.TargetRootID
	plan, err = codexreview.FinalizeAssistedInstallPlan(
		plan,
		preview.Skills,
		configFingerprint,
	)
	if err != nil {
		failAnalysis("finalization-failed", err)
		return model.AssistedInstallPlan{}, err
	}
	plan.TransactionID = ""
	plan.RecoveryStatus = ""
	plan.SelectedSkills = nil
	if err := saveAssistedInstallPlan(m.Config.Paths.DataRoot, plan); err != nil {
		failAnalysis("finalization-failed", err)
		return model.AssistedInstallPlan{}, err
	}
	m.assistData.Lock()
	m.assisted[plan.ID] = plan
	m.assistData.Unlock()

	emit(model.AssistedInstallProgress{
		Phase: plan.Status,
		Message: m.assistedMessage(
			"Codex 安装计划已生成",
			"The Codex installation plan is ready for review",
		),
		CompletedSteps: len(plan.Steps),
		TotalSteps:     len(plan.Steps),
		Steps:          progressStepsFromPlan(plan.Steps),
	}, true)
	return plan, nil
}

func (m *Manager) GetAssistedInstallPlan(planID string) (model.AssistedInstallPlan, error) {
	planID = strings.TrimSpace(planID)
	var (
		plan model.AssistedInstallPlan
		err  error
	)
	if validAssistedPlanID(planID) {
		plan, err = m.assistedInstallPlan(planID)
	} else if validAssistedReferenceID(planID) {
		plan, err = m.assistedInstallPlanBySource(planID)
	} else {
		err = errors.New("invalid assisted-install plan reference")
	}
	if err != nil {
		return model.AssistedInstallPlan{}, err
	}
	preview, err := m.assistedSourcePreview(plan.SourcePlanID)
	if err != nil {
		return model.AssistedInstallPlan{}, fmt.Errorf("load assisted-install source plan: %w", err)
	}
	if plan.ProjectScanID != "" {
		scan, scanErr := m.GetProjectScan(plan.ProjectScanID)
		if scanErr != nil {
			return model.AssistedInstallPlan{}, scanErr
		}
		if scan.SourcePlanID != plan.SourcePlanID ||
			!strings.EqualFold(scan.ScanDigest, plan.ProjectScanDigest) {
			return model.AssistedInstallPlan{}, errors.New(
				"assisted-install plan is not bound to the approved project scan",
			)
		}
	}
	if err := codexreview.VerifyAssistedInstallPlan(
		cloneAssistedPlanForVerification(plan),
		preview.Skills,
		plan.ConfigFingerprint,
	); err != nil {
		return model.AssistedInstallPlan{}, err
	}
	if plan.Status == "running" && !m.hasActiveAssistedRun(plan.ID, plan.TransactionID) {
		tx, txErr := m.store.Transaction(plan.TransactionID)
		if txErr != nil {
			return model.AssistedInstallPlan{}, fmt.Errorf(
				"load orphaned assisted-install transaction: %w",
				txErr,
			)
		}
		plan, _, err = m.interruptOrphanedAssistedInstall(plan, tx)
		if err != nil {
			return model.AssistedInstallPlan{}, err
		}
	}
	ignored, ignoreErr := m.store.IgnoredFindings()
	if ignoreErr == nil {
		plan.Scan = m.decorateScan(plan.Scan, ignored)
	}
	return plan, nil
}

func (m *Manager) interruptOrphanedAssistedTransactions(
	transactions []model.Transaction,
) ([]model.Transaction, error) {
	out := append([]model.Transaction(nil), transactions...)
	for index := range out {
		tx := out[index]
		if !strings.EqualFold(strings.TrimSpace(tx.Status), "running") ||
			m.hasActiveAssistedRun(tx.ID) {
			continue
		}
		plan, err := m.assistedPlanForTransaction(tx)
		if err != nil {
			// Keep the dashboard available even when a legacy/corrupt record has
			// lost its plan. It cannot be auto-recovered without that plan, but
			// it must no longer masquerade as an active run.
			tx.Status = "interrupted"
			tx.CompletedAt = time.Now().UTC()
			tx.RecoveryStatus = "required"
			tx.Error = fmt.Sprintf("assisted-install recovery plan is unavailable: %v", err)
			if saveErr := m.store.SaveTransaction(tx); saveErr != nil {
				return nil, errors.Join(err, saveErr)
			}
			out[index] = tx
			continue
		}
		_, interrupted, err := m.interruptOrphanedAssistedInstall(plan, tx)
		if err != nil {
			return nil, err
		}
		out[index] = interrupted
	}
	return out, nil
}

func (m *Manager) interruptOrphanedAssistedInstall(
	plan model.AssistedInstallPlan,
	tx model.Transaction,
) (model.AssistedInstallPlan, model.Transaction, error) {
	if plan.TransactionID == "" || plan.TransactionID != tx.ID ||
		tx.Type != "assisted-install" {
		return model.AssistedInstallPlan{}, model.Transaction{},
			errors.New("orphaned assisted-install plan and transaction do not match")
	}
	message := m.assistedMessage(
		"应用在安装完成前退出；请先回滚已记录的修改",
		"The application exited before installation completed; roll back recorded changes first",
	)
	completedAt := time.Now().UTC()
	plan.Status = "interrupted"
	plan.RecoveryStatus = "required"
	for index := range plan.Steps {
		if plan.Steps[index].Status == "running" {
			plan.Steps[index].Status = "interrupted"
			plan.Steps[index].Error = message
		}
	}
	tx.Status = "interrupted"
	tx.CompletedAt = completedAt
	tx.RecoveryStatus = "required"
	tx.Error = message
	tx.Steps = cloneAssistedSteps(plan.Steps)
	if err := m.checkpointAssistedInstall(plan, tx); err != nil {
		return model.AssistedInstallPlan{}, model.Transaction{}, fmt.Errorf(
			"persist interrupted assisted-install recovery state: %w",
			err,
		)
	}
	return plan, tx, nil
}

func (m *Manager) ApplyAssistedInstall(
	ctx context.Context,
	planID string,
	selectedSkills []string,
	permissionIDs []string,
	projectRoot string,
	progress AssistedInstallProgressFunc,
) (model.AssistedInstallResult, error) {
	return m.ApplyAssistedInstallForRoot(ctx, planID, selectedSkills, permissionIDs, projectRoot, "", progress)
}

func (m *Manager) ApplyAssistedInstallForRoot(
	ctx context.Context,
	planID string,
	selectedSkills []string,
	permissionIDs []string,
	projectRoot string,
	targetRootID string,
	progress AssistedInstallProgressFunc,
) (model.AssistedInstallResult, error) {
	m.assistMu.Lock()
	defer m.assistMu.Unlock()

	plan, _, chosen, approved, err := m.preflightAssistedInstall(
		planID,
		selectedSkills,
		permissionIDs,
		projectRoot,
	)
	if err != nil {
		return model.AssistedInstallResult{}, err
	}
	if targetRootID != "" && targetRootID != plan.TargetRootID {
		return model.AssistedInstallResult{}, errors.New("assisted-install plan target root does not match apply target")
	}
	selectedNames := make([]string, 0, len(chosen))
	for _, candidate := range chosen {
		selectedNames = append(selectedNames, candidate.Name)
	}
	sort.Strings(selectedNames)
	plan.SelectedSkills = append([]string(nil), selectedNames...)

	runContext, cancel := context.WithCancel(ctx)
	transactionID := "tx-" + time.Now().UTC().Format("20060102T150405.000000000")
	if err := m.registerAssistedCancel([]string{plan.ID, transactionID}, cancel); err != nil {
		cancel()
		return model.AssistedInstallResult{}, err
	}
	defer func() {
		cancel()
		m.unregisterAssistedCancel([]string{plan.ID, transactionID})
	}()

	plan.Status = "running"
	plan.TransactionID = transactionID
	plan.RecoveryStatus = ""
	for index := range plan.Permissions {
		plan.Permissions[index].Approved = approved[plan.Permissions[index].ID]
	}
	for index := range plan.Steps {
		step := &plan.Steps[index]
		step.TargetPath = ""
		step.BackupPath = ""
		step.ManifestPath = ""
		step.ChildTransactionID = ""
		step.OutputHashes = nil
		step.OriginalMissing = false
		step.AppliedHash = ""
		step.Error = ""
		step.StartedAt = nil
		step.CompletedAt = nil
		switch {
		case step.Kind == model.AssistedInstallStepManual && step.Required:
			step.Status = "manual-pending"
		case step.Kind == model.AssistedInstallStepManual:
			step.Status = "skipped"
		case !allPermissionsApproved(step.PermissionIDs, approved):
			step.Status = "skipped"
		default:
			step.Status = "queued"
		}
	}
	tx := model.Transaction{
		ID:          transactionID,
		RootID:      plan.TargetRootID,
		Type:        "assisted-install",
		Status:      "running",
		Targets:     append([]string(nil), selectedNames...),
		ProjectRoot: plan.ProjectRoot,
		StartedAt:   time.Now().UTC(),
		Steps:       cloneAssistedSteps(plan.Steps),
	}
	if err := m.store.SaveTransaction(tx); err != nil {
		return model.AssistedInstallResult{}, err
	}
	if err := m.storeAssistedPlan(plan); err != nil {
		return m.failAssistedBeforeMutation(plan, tx, err)
	}
	approvalData, _ := json.Marshal(map[string]any{
		"planDigest":  plan.PlanDigest,
		"permissions": sortedTrueKeys(approved),
		"projectRoot": plan.ProjectRoot,
		"skills":      selectedNames,
	})
	if err := m.store.Approve(plan.ID, "approve:"+plan.PlanDigest, string(approvalData)); err != nil {
		return m.failAssistedBeforeMutation(plan, tx, err)
	}

	execution := m.startAssistedExecutionProgress(plan, tx, progress)
	tools := map[string]managedPythonTool{}
	for index := range plan.Steps {
		step := &plan.Steps[index]
		if step.Status == "skipped" || step.Status == "manual-pending" {
			status := step.Status
			message := m.assistedMessage(
				"此可选步骤未获授权，已跳过",
				"This optional step was not approved and was skipped",
			)
			if status == "manual-pending" {
				message = m.assistedMessage(
					"此步骤需要按计划中的说明手动完成",
					"This step remains for manual completion using the plan instructions",
				)
			}
			m.updateAssistedExecutionProgress(plan.ID, transactionID, progress, true, func(value *model.AssistedInstallProgress) {
				updateProgressStep(value, step.ID, status, message, "")
			})
			continue
		}
		if err := runContext.Err(); err != nil {
			return m.failAssistedExecution(
				plan,
				tx,
				index,
				execution,
				err,
				progress,
			)
		}
		if err := m.prepareAssistedStepIntent(&plan, &tx, index); err != nil {
			return m.failAssistedExecution(
				plan,
				tx,
				index,
				execution,
				err,
				progress,
			)
		}
		now := time.Now().UTC()
		step.Status = "running"
		step.StartedAt = &now
		if err := m.checkpointAssistedInstall(plan, tx); err != nil {
			return m.failAssistedExecution(
				plan,
				tx,
				index,
				execution,
				fmt.Errorf("persist step intent before execution: %w", err),
				progress,
			)
		}
		m.updateAssistedExecutionProgress(plan.ID, transactionID, progress, true, func(value *model.AssistedInstallProgress) {
			value.Phase = "executing"
			value.CurrentStepID = step.ID
			value.Message = step.Title
			updateProgressStep(value, step.ID, "running", step.Description, "")
		})

		stepErr := m.executeAssistedInstallStep(
			runContext,
			&plan,
			&tx,
			index,
			chosen,
			plan.ProjectRoot,
			tools,
			func() {
				m.bumpAssistedActivity(plan.ID, transactionID, progress)
			},
		)
		if stepErr != nil {
			return m.failAssistedExecution(
				plan,
				tx,
				index,
				execution,
				stepErr,
				progress,
			)
		}
		completedAt := time.Now().UTC()
		step.Status = "completed"
		step.CompletedAt = &completedAt
		tx.Steps = cloneAssistedSteps(plan.Steps)
		if err := m.checkpointAssistedInstall(plan, tx); err != nil {
			return m.failAssistedExecution(
				plan,
				tx,
				index,
				execution,
				fmt.Errorf("persist completed step: %w", err),
				progress,
			)
		}
		m.updateAssistedExecutionProgress(plan.ID, transactionID, progress, true, func(value *model.AssistedInstallProgress) {
			updateProgressStep(value, step.ID, "completed", m.assistedMessage(
				"步骤已完成",
				"Step completed",
			), "")
		})
		if err := runContext.Err(); err != nil {
			return m.failAssistedExecution(
				plan,
				tx,
				-1,
				execution,
				err,
				progress,
			)
		}
	}

	completedAt := time.Now().UTC()
	finalStatus := "completed"
	if hasManualPendingAssistedStep(plan.Steps) {
		finalStatus = "partial"
	}
	plan.Status = finalStatus
	plan.RecoveryStatus = "available"
	tx.Status = finalStatus
	tx.CompletedAt = completedAt
	tx.Steps = cloneAssistedSteps(plan.Steps)
	tx.RecoveryStatus = "available"
	if err := m.checkpointAssistedInstall(plan, tx); err != nil {
		return m.failAssistedExecution(
			plan,
			tx,
			len(plan.Steps),
			execution,
			err,
			progress,
		)
	}
	m.recordTransaction(tx)
	finalProgress := m.updateAssistedExecutionProgress(
		plan.ID,
		transactionID,
		progress,
		true,
		func(value *model.AssistedInstallProgress) {
			value.Phase = finalStatus
			if finalStatus == "partial" {
				value.Message = m.assistedMessage(
					"支持的步骤已完成；仍有步骤需要手动处理",
					"Supported steps completed; manual steps remain",
				)
			} else {
				value.Message = m.assistedMessage("计划安装已完成", "Planned installation completed")
			}
			value.CurrentStepID = ""
			value.Terminal = true
			value.CompletedAt = &completedAt
		},
	)
	return model.AssistedInstallResult{
		ReferenceID: plan.ID,
		RunID:       transactionID,
		Plan:        plan,
		Transaction: tx,
		Progress:    &finalProgress,
	}, nil
}

func (m *Manager) GetAssistedInstallProgress(referenceID string) (model.AssistedInstallProgress, error) {
	referenceID = strings.TrimSpace(referenceID)
	if !validAssistedReferenceID(referenceID) {
		return model.AssistedInstallProgress{}, errors.New("invalid assisted-install reference ID")
	}
	m.assistData.Lock()
	if value, ok := m.progress[referenceID]; ok {
		m.assistData.Unlock()
		return cloneAssistedProgress(value), nil
	}
	m.assistData.Unlock()
	value, err := loadAssistedInstallProgress(m.Config.Paths.DataRoot, referenceID)
	if err == nil {
		value = m.interruptOrphanedAssistedAnalysis(value)
		m.assistData.Lock()
		m.progress[referenceID] = value
		if value.RunID != "" {
			m.progress[value.RunID] = value
		}
		m.assistData.Unlock()
		return cloneAssistedProgress(value), nil
	}
	plan, planErr := m.assistedInstallPlan(referenceID)
	if planErr != nil {
		return model.AssistedInstallProgress{}, err
	}
	now := time.Now().UTC()
	value = model.AssistedInstallProgress{
		ReferenceID: plan.ID,
		RunID:       plan.TransactionID,
		Phase:       plan.Status,
		Message:     assistedPlanStatusMessage(plan.Status, m.Config.Locale),
		Steps:       progressStepsFromPlan(plan.Steps),
		TotalSteps:  len(plan.Steps),
		StartedAt:   plan.CreatedAt,
		UpdatedAt:   now,
		Terminal:    plan.Status != "running",
	}
	recountAssistedProgress(&value)
	return value, nil
}

func (m *Manager) interruptOrphanedAssistedAnalysis(
	value model.AssistedInstallProgress,
) model.AssistedInstallProgress {
	if value.Terminal || !strings.HasPrefix(value.RunID, "tx-analysis-") ||
		m.hasActiveAssistedRun(value.ReferenceID, value.RunID) {
		return value
	}
	now := time.Now().UTC()
	value.Sequence++
	value.Phase = "interrupted"
	value.Message = m.assistedMessage(
		"应用退出导致扫描中断，请重新运行增强项目扫描",
		"The scan was interrupted when the application exited; run the enhanced project scan again",
	)
	value.Error = m.assistedMessage(
		"未找到仍在运行的增强项目扫描任务",
		"No active enhanced project scan task was found",
	)
	value.UpdatedAt = now
	value.CompletedAt = &now
	value.Terminal = true
	_ = saveAssistedInstallProgress(m.Config.Paths.DataRoot, value.ReferenceID, value)
	return value
}

func (m *Manager) CancelAssistedInstall(referenceID string) error {
	referenceID = strings.TrimSpace(referenceID)
	if !validAssistedReferenceID(referenceID) {
		return errors.New("invalid assisted-install reference ID")
	}
	m.assistData.Lock()
	cancel := m.cancels[referenceID]
	m.assistData.Unlock()
	if cancel == nil {
		progress, err := m.GetAssistedInstallProgress(referenceID)
		if err == nil && progress.Terminal {
			return nil
		}
		return errors.New("planned installation is not currently running")
	}
	cancel()
	return nil
}

func (m *Manager) preflightAssistedInstall(
	planID string,
	selectedSkills []string,
	permissionIDs []string,
	projectRoot string,
) (
	model.AssistedInstallPlan,
	model.InstallPreview,
	[]model.CandidateSkill,
	map[string]bool,
	error,
) {
	plan, err := m.assistedInstallPlan(planID)
	if err != nil {
		return model.AssistedInstallPlan{}, model.InstallPreview{}, nil, nil, err
	}
	if time.Now().UTC().After(plan.ExpiresAt) {
		return model.AssistedInstallPlan{}, model.InstallPreview{}, nil, nil, errors.New(
			"assisted-install plan has expired",
		)
	}
	switch plan.Status {
	case "ready", "manual-required", "failed", "cancelled":
	case "interrupted":
		return model.AssistedInstallPlan{}, model.InstallPreview{}, nil, nil, errors.New(
			"the previous execution was interrupted; roll back its recorded changes before creating a new plan",
		)
	default:
		return model.AssistedInstallPlan{}, model.InstallPreview{}, nil, nil, fmt.Errorf(
			"assisted-install plan cannot run from status %s",
			plan.Status,
		)
	}
	if (plan.Status == "failed" || plan.Status == "cancelled") &&
		plan.RecoveryStatus != "completed" {
		return model.AssistedInstallPlan{}, model.InstallPreview{}, nil, nil, errors.New(
			"the previous execution has not been fully recovered; roll it back before retrying",
		)
	}
	preview, err := m.assistedSourcePreview(plan.SourcePlanID)
	if err != nil {
		return model.AssistedInstallPlan{}, model.InstallPreview{}, nil, nil, err
	}
	if plan.TargetRootID == "" || plan.TargetRootID != preview.TargetRootID {
		return model.AssistedInstallPlan{}, model.InstallPreview{}, nil, nil, errors.New(
			"assisted-install plan target root does not match its source plan",
		)
	}
	if _, err := m.enforceProjectAssessmentGate(preview); err != nil {
		return model.AssistedInstallPlan{}, model.InstallPreview{}, nil, nil, err
	}
	if time.Now().UTC().After(preview.ExpiresAt) {
		return model.AssistedInstallPlan{}, model.InstallPreview{}, nil, nil, errors.New(
			"source install plan has expired",
		)
	}
	if plan.ProjectScanID != "" {
		scan, scanErr := m.GetProjectScan(plan.ProjectScanID)
		if scanErr != nil {
			return model.AssistedInstallPlan{}, model.InstallPreview{}, nil, nil, scanErr
		}
		if scan.SourcePlanID != plan.SourcePlanID ||
			!strings.EqualFold(scan.ScanDigest, plan.ProjectScanDigest) {
			return model.AssistedInstallPlan{}, model.InstallPreview{}, nil, nil, errors.New(
				"assisted-install plan is not bound to the approved project scan",
			)
		}
	}
	if err := codexreview.VerifyAssistedInstallPlan(
		cloneAssistedPlanForVerification(plan),
		preview.Skills,
		plan.ConfigFingerprint,
	); err != nil {
		return model.AssistedInstallPlan{}, model.InstallPreview{}, nil, nil, err
	}
	if err := codexreview.VerifyAssistedInstallContext(
		preview.StagingPath,
		plan.ContextDigest,
	); err != nil {
		return model.AssistedInstallPlan{}, model.InstallPreview{}, nil, nil, err
	}
	selectedSkills = uniqueNonEmpty(selectedSkills)
	if len(selectedSkills) == 0 {
		return model.AssistedInstallPlan{}, model.InstallPreview{}, nil, nil, errors.New(
			"at least one Skill must be selected",
		)
	}
	chosen := chooseCandidates(preview.Skills, selectedSkills)
	if len(chosen) != len(selectedSkills) {
		return model.AssistedInstallPlan{}, model.InstallPreview{}, nil, nil, errors.New(
			"the selection contains a Skill that is not in the approved source plan",
		)
	}
	if err := m.verifyInstallPreview(preview, chosen); err != nil {
		return model.AssistedInstallPlan{}, model.InstallPreview{}, nil, nil, err
	}
	ignored, err := m.store.IgnoredFindings()
	if err != nil {
		return model.AssistedInstallPlan{}, model.InstallPreview{}, nil, nil, err
	}
	scan := m.decorateScan(preview.Scan, ignored)
	for _, cluster := range scan.Clusters {
		if !cluster.Ignored &&
			(cluster.Severity == model.RiskCritical || cluster.Severity == model.RiskHigh) {
			return model.AssistedInstallPlan{}, model.InstallPreview{}, nil, nil, errors.New(
				"high or critical local warnings remain; review and ignore them explicitly before installation",
			)
		}
	}
	approved, err := validateAssistedPermissions(plan.Permissions, permissionIDs)
	if err != nil {
		return model.AssistedInstallPlan{}, model.InstallPreview{}, nil, nil, err
	}
	if err := validateAssistedPermissionDependencies(plan.Steps, approved); err != nil {
		return model.AssistedInstallPlan{}, model.InstallPreview{}, nil, nil, err
	}
	scheduledSupportedSteps := 0
	for _, step := range plan.Steps {
		if step.Supported && step.Kind != model.AssistedInstallStepManual &&
			allPermissionsApproved(step.PermissionIDs, approved) {
			scheduledSupportedSteps++
		}
		if step.Kind != model.AssistedInstallStepManual &&
			step.Required && !allPermissionsApproved(step.PermissionIDs, approved) {
			return model.AssistedInstallPlan{}, model.InstallPreview{}, nil, nil, fmt.Errorf(
				"required permissions for step %q were not approved",
				step.Title,
			)
		}
		if step.Kind == model.AssistedInstallStepManagedPythonTool &&
			allPermissionsApproved(step.PermissionIDs, approved) &&
			!strings.EqualFold(preview.Repository.Provider, "github") {
			return model.AssistedInstallPlan{}, model.InstallPreview{}, nil, nil, errors.New(
				"managed PyPI integration requires a GitHub source so package ownership can be verified",
			)
		}
		if step.Kind == model.AssistedInstallStepManagedPythonTool &&
			allPermissionsApproved(step.PermissionIDs, approved) {
			wheelhouse, pathErr := assistedPythonWheelhouse(
				m.Config.Paths.StagingRoot,
				plan.ID,
				step.ID,
			)
			if pathErr != nil {
				return model.AssistedInstallPlan{}, model.InstallPreview{}, nil, nil, pathErr
			}
			if _, _, lockErr := verifyLockedWheelhouse(
				wheelhouse,
				step.PythonWheels,
				step.PythonPackage,
				strings.TrimPrefix(step.VersionSpec, "=="),
			); lockErr != nil {
				return model.AssistedInstallPlan{}, model.InstallPreview{}, nil, nil, fmt.Errorf(
					"approved Python dependency cache is unavailable or changed: %w",
					lockErr,
				)
			}
		}
	}
	if scheduledSupportedSteps == 0 {
		return model.AssistedInstallPlan{}, model.InstallPreview{}, nil, nil, errors.New(
			"the plan contains no supported step that can be executed; review its manual instructions instead",
		)
	}
	plan.ProjectRoot = ""
	if hasScheduledAssistedStep(
		plan.Steps,
		model.AssistedInstallStepConfigureCodexMCP,
		approved,
	) {
		configPath, pathErr := m.codexConfigPath()
		if pathErr != nil {
			return model.AssistedInstallPlan{}, model.InstallPreview{}, nil, nil, pathErr
		}
		currentFingerprint, fingerprintErr := fileFingerprint(configPath)
		if fingerprintErr != nil {
			return model.AssistedInstallPlan{}, model.InstallPreview{}, nil, nil, fingerprintErr
		}
		if currentFingerprint != plan.ConfigFingerprint {
			return model.AssistedInstallPlan{}, model.InstallPreview{}, nil, nil, errors.New(
				"Codex configuration changed after analysis; create and approve a new installation plan",
			)
		}
		validatedProjectRoot, rootErr := validateAssistedProjectRoot(projectRoot)
		if rootErr != nil {
			return model.AssistedInstallPlan{}, model.InstallPreview{}, nil, nil, rootErr
		}
		plan.ProjectRoot = validatedProjectRoot
	}
	return plan, preview, chosen, approved, nil
}

func (m *Manager) prepareAssistedStepIntent(
	plan *model.AssistedInstallPlan,
	tx *model.Transaction,
	index int,
) error {
	step := &plan.Steps[index]
	switch step.Kind {
	case model.AssistedInstallStepInstallSkills:
		if step.ChildTransactionID == "" {
			step.ChildTransactionID = "tx-" + time.Now().UTC().Format("20060102T150405.000000000")
		}
	case model.AssistedInstallStepManagedPythonTool:
		packageName := strings.TrimSpace(step.PythonPackage)
		version := strings.TrimSpace(strings.TrimPrefix(step.VersionSpec, "=="))
		if !pythonPackageName.MatchString(packageName) || !pythonVersion.MatchString(version) {
			return errors.New("managed Python package metadata is invalid")
		}
		step.TargetPath = filepath.Join(
			m.Config.Paths.DataRoot,
			"tools",
			"python",
			normalizePackagePath(packageName),
			version+"-"+shortPlanDigest(plan.ID),
		)
		step.ManifestPath = filepath.Join(step.TargetPath, "csm-tool-manifest.json")
		if err := ensureWithinOrEqual(filepath.Join(m.Config.Paths.DataRoot, "tools"), step.TargetPath); err != nil {
			return errors.New("managed Python target escapes the application tools directory")
		}
	case model.AssistedInstallStepConfigureCodexMCP:
		if !managedMCPName.MatchString(step.MCPServerName) {
			return errors.New("Codex MCP server name is invalid")
		}
		configPath, err := m.codexConfigPath()
		if err != nil {
			return err
		}
		currentHash, err := fileFingerprint(configPath)
		if err != nil {
			return err
		}
		if currentHash != plan.ConfigFingerprint {
			return errors.New("Codex configuration changed after approval; create a new installation plan")
		}
		step.TargetPath = configPath
		step.OriginalMissing = currentHash == missingFileFingerprint
		if step.OriginalMissing {
			step.BackupPath = ""
		} else {
			step.BackupPath = filepath.Join(
				m.Config.Paths.BackupsRoot,
				"_transactions",
				tx.ID,
				"codex-config.toml",
			)
		}
		step.ManifestPath = filepath.Join(
			m.Config.Paths.DataRoot,
			"integrations",
			"mcp",
			strings.ToLower(step.MCPServerName)+".json",
		)
	}
	tx.Steps = cloneAssistedSteps(plan.Steps)
	return nil
}

func (m *Manager) executeAssistedInstallStep(
	ctx context.Context,
	plan *model.AssistedInstallPlan,
	tx *model.Transaction,
	index int,
	chosen []model.CandidateSkill,
	projectRoot string,
	tools map[string]managedPythonTool,
	onActivity func(),
) error {
	step := &plan.Steps[index]
	switch step.Kind {
	case model.AssistedInstallStepInstallSkills:
		names := make([]string, 0, len(chosen))
		allowed := make(map[string]bool, len(step.SkillNames))
		for _, name := range step.SkillNames {
			allowed[strings.ToLower(name)] = true
		}
		for _, candidate := range chosen {
			if allowed[strings.ToLower(candidate.Name)] {
				names = append(names, candidate.Name)
			}
		}
		if len(names) == 0 {
			return errors.New("the approved install step contains none of the selected Skills")
		}
		child, err := m.applyInstallWithTransactionID(
			plan.SourcePlanID,
			names,
			step.ChildTransactionID,
		)
		step.BackupPath = strings.Join(child.BackupPaths, "\n")
		return err
	case model.AssistedInstallStepManagedPythonTool:
		tool, err := m.installManagedPythonTool(
			ctx,
			plan.ID,
			tx.ID,
			step.ID,
			plan.Repository.FullName,
			step.PythonPackage,
			strings.TrimPrefix(step.VersionSpec, "=="),
			step.Entrypoint,
			step.PythonWheels,
			onActivity,
		)
		if err != nil {
			return err
		}
		step.TargetPath = tool.RootPath
		step.ManifestPath = filepath.Join(tool.RootPath, "csm-tool-manifest.json")
		step.OutputHashes = make(map[string]string, len(tool.WheelHashes)+1)
		for name, hash := range tool.WheelHashes {
			step.OutputHashes[name] = hash
		}
		step.OutputHashes["entrypoint"] = tool.EntryPath
		manifestHash, hashErr := fileFingerprint(step.ManifestPath)
		if hashErr != nil {
			return hashErr
		}
		step.AppliedHash = manifestHash
		tools[strings.ToLower(step.Entrypoint)] = tool
		return nil
	case model.AssistedInstallStepConfigureCodexMCP:
		tool, ok := tools[strings.ToLower(step.Entrypoint)]
		if !ok {
			return fmt.Errorf("managed tool %q is not available for MCP configuration", step.Entrypoint)
		}
		mutation, err := m.configureManagedMCP(
			plan.ID,
			tx.ID,
			step.MCPServerName,
			tool.EntryPath,
			step.MCPArgs,
			projectRoot,
			plan.ConfigFingerprint,
			func(intent mcpMutation) error {
				step.TargetPath = intent.ConfigPath
				step.BackupPath = intent.BackupPath
				step.ManifestPath = intent.ManifestPath
				step.AppliedHash = intent.AppliedHash
				step.OriginalMissing = intent.OriginalMissing
				return m.checkpointAssistedInstall(*plan, *tx)
			},
		)
		if err != nil {
			return err
		}
		step.TargetPath = mutation.ConfigPath
		step.BackupPath = mutation.BackupPath
		step.ManifestPath = mutation.ManifestPath
		step.AppliedHash = mutation.AppliedHash
		step.OriginalMissing = mutation.OriginalMissing
		return nil
	case model.AssistedInstallStepManual:
		return nil
	default:
		return fmt.Errorf("unsupported assisted-install step kind: %s", step.Kind)
	}
}

func (m *Manager) failAssistedBeforeMutation(
	plan model.AssistedInstallPlan,
	tx model.Transaction,
	cause error,
) (model.AssistedInstallResult, error) {
	completedAt := time.Now().UTC()
	plan.Status = "failed"
	plan.RecoveryStatus = "completed"
	tx.Status = "failed"
	tx.Error = cause.Error()
	tx.RecoveryStatus = "completed"
	tx.CompletedAt = completedAt
	tx.Steps = cloneAssistedSteps(plan.Steps)
	if persistErr := m.checkpointAssistedInstall(plan, tx); persistErr != nil {
		cause = errors.Join(cause, persistErr)
	}
	m.recordTransaction(tx)
	return model.AssistedInstallResult{
		ReferenceID: plan.ID,
		RunID:       tx.ID,
		Plan:        plan,
		Transaction: tx,
	}, cause
}

func (m *Manager) failAssistedExecution(
	plan model.AssistedInstallPlan,
	tx model.Transaction,
	stepIndex int,
	_ model.AssistedInstallProgress,
	cause error,
	progress AssistedInstallProgressFunc,
) (model.AssistedInstallResult, error) {
	cancelled := errors.Is(cause, context.Canceled)
	if stepIndex >= 0 && stepIndex < len(plan.Steps) {
		now := time.Now().UTC()
		step := &plan.Steps[stepIndex]
		if cancelled {
			step.Status = "cancelled"
		} else {
			step.Status = "failed"
		}
		step.Error = cause.Error()
		step.CompletedAt = &now
	}
	tx.Steps = cloneAssistedSteps(plan.Steps)
	m.updateAssistedExecutionProgress(plan.ID, tx.ID, progress, true, func(value *model.AssistedInstallProgress) {
		value.Phase = "recovering"
		value.Message = m.assistedMessage(
			"执行已停止，正在恢复已完成的修改",
			"Execution stopped; recovering completed changes",
		)
		value.Error = cause.Error()
		if stepIndex >= 0 && stepIndex < len(plan.Steps) {
			updateProgressStep(value, plan.Steps[stepIndex].ID, plan.Steps[stepIndex].Status, "", cause.Error())
		}
	})
	recoveryErr := m.recoverAssistedSteps(&plan, &tx)
	if stepIndex >= 0 && stepIndex < len(plan.Steps) {
		step := plan.Steps[stepIndex]
		if step.Kind == model.AssistedInstallStepInstallSkills && step.ChildTransactionID != "" {
			child, childErr := m.store.Transaction(step.ChildTransactionID)
			if childErr != nil {
				recoveryErr = errors.Join(
					recoveryErr,
					fmt.Errorf("verify failed Skill installation recovery: %w", childErr),
				)
			} else if child.RecoveryStatus == "required" {
				recoveryErr = errors.Join(
					recoveryErr,
					errors.New("the child Skill installation could not fully recover its changes"),
				)
			}
		}
	}
	completedAt := time.Now().UTC()
	if cancelled {
		plan.Status = "cancelled"
		tx.Status = "cancelled"
	} else {
		plan.Status = "failed"
		tx.Status = "failed"
	}
	plan.RecoveryStatus = "completed"
	tx.RecoveryStatus = "completed"
	if recoveryErr != nil {
		plan.RecoveryStatus = "required"
		tx.RecoveryStatus = "required"
		cause = fmt.Errorf("%w; automatic recovery was incomplete: %v", cause, recoveryErr)
	}
	tx.Error = cause.Error()
	tx.CompletedAt = completedAt
	tx.Steps = cloneAssistedSteps(plan.Steps)
	if persistErr := m.checkpointAssistedInstall(plan, tx); persistErr != nil {
		plan.RecoveryStatus = "required"
		tx.RecoveryStatus = "required"
		cause = errors.Join(cause, persistErr)
		tx.Error = cause.Error()
		_ = m.checkpointAssistedInstall(plan, tx)
	}
	m.recordTransaction(tx)
	finalProgress := m.updateAssistedExecutionProgress(
		plan.ID,
		tx.ID,
		progress,
		true,
		func(value *model.AssistedInstallProgress) {
			if cancelled {
				value.Phase = "cancelled"
				value.Message = m.assistedMessage("执行已取消", "Execution cancelled")
			} else {
				value.Phase = "failed"
				value.Message = m.assistedMessage("安装执行失败", "Installation execution failed")
			}
			if recoveryErr == nil {
				value.Message += m.assistedMessage(
					"，已恢复完成的修改",
					"; completed changes were recovered",
				)
			} else {
				value.Message += m.assistedMessage(
					"，部分修改需要人工恢复",
					"; some changes require manual recovery",
				)
			}
			value.Error = cause.Error()
			value.Terminal = true
			value.CompletedAt = &completedAt
		},
	)
	return model.AssistedInstallResult{
		ReferenceID: plan.ID,
		RunID:       tx.ID,
		Plan:        plan,
		Transaction: tx,
		Progress:    &finalProgress,
	}, cause
}

func (m *Manager) recoverAssistedSteps(
	plan *model.AssistedInstallPlan,
	tx *model.Transaction,
) error {
	var recoveryErrors []error
	for index := len(plan.Steps) - 1; index >= 0; index-- {
		step := &plan.Steps[index]
		if step.Status != "completed" && step.Status != "running" &&
			step.Status != "interrupted" && step.Status != "failed" &&
			step.Status != "cancelled" {
			continue
		}
		var err error
		switch step.Kind {
		case model.AssistedInstallStepConfigureCodexMCP:
			currentHash, fingerprintErr := fileFingerprint(step.TargetPath)
			if fingerprintErr != nil {
				err = fingerprintErr
			} else if currentHash != plan.ConfigFingerprint {
				var mutation mcpMutation
				mutation, err = m.validatedMCPRecoveryMutation(*step, tx.ID)
				if err == nil {
					err = restoreMCPConfig(m.Config.Paths.QuarantineRoot, tx.ID, mutation)
				}
			}
		case model.AssistedInstallStepInstallSkills:
			if step.ChildTransactionID != "" {
				var child model.Transaction
				child, err = m.validatedAssistedChildTransaction(step.ChildTransactionID, *tx)
				if errors.Is(err, sql.ErrNoRows) &&
					(step.Status == "running" || step.Status == "interrupted" ||
						step.Status == "failed" || step.Status == "cancelled") {
					// The child transaction is written before its first Skill
					// mutation. No row therefore means the parent intent was
					// persisted but the child never started.
					err = nil
				}
				if err == nil {
					if child.ID != "" &&
						(child.Status != "failed" || child.RecoveryStatus != "completed") {
						_, err = m.Rollback(step.ChildTransactionID)
					}
				}
			}
		case model.AssistedInstallStepManagedPythonTool:
			if step.TargetPath != "" {
				err = m.validateManagedToolRecovery(*step, plan.ID, tx.ID)
				if err == nil {
					var quarantine string
					quarantine, err = moveManagedToolToQuarantine(
						m.Config.Paths.DataRoot,
						step.TargetPath,
						tx.ID,
					)
					if err == nil && quarantine != "" {
						step.BackupPath = quarantine
					}
				}
			}
		}
		if err != nil {
			step.Error = strings.TrimSpace(step.Error + " Recovery: " + err.Error())
			recoveryErrors = append(recoveryErrors, fmt.Errorf("%s: %w", step.Title, err))
		} else if step.Kind != model.AssistedInstallStepManual {
			step.Status = "rolled-back"
		}
		tx.Steps = cloneAssistedSteps(plan.Steps)
		if checkpointErr := m.checkpointAssistedInstall(*plan, *tx); checkpointErr != nil {
			recoveryErrors = append(
				recoveryErrors,
				fmt.Errorf("persist recovery for %s: %w", step.Title, checkpointErr),
			)
		}
	}
	return errors.Join(recoveryErrors...)
}

func (m *Manager) validatedAssistedChildTransaction(
	childID string,
	parent model.Transaction,
) (model.Transaction, error) {
	if !strings.HasPrefix(childID, "tx-") || filepath.Base(childID) != childID {
		return model.Transaction{}, errors.New("recorded child installation transaction ID is invalid")
	}
	child, err := m.store.Transaction(childID)
	if err != nil {
		return model.Transaction{}, err
	}
	if child.Type != "install" {
		return model.Transaction{}, errors.New("recorded child transaction is not a Skill installation")
	}
	switch child.Status {
	case "running", "completed", "failed":
	default:
		return model.Transaction{}, errors.New("recorded child transaction has an invalid recovery state")
	}
	if child.StartedAt.Before(parent.StartedAt) {
		return model.Transaction{}, errors.New("recorded child installation predates its assisted transaction")
	}
	if !parent.CompletedAt.IsZero() && child.CompletedAt.After(parent.CompletedAt) {
		return model.Transaction{}, errors.New("recorded child installation completed after its assisted transaction")
	}
	allowed := map[string]bool{}
	for _, name := range parent.Targets {
		allowed[name] = true
	}
	if len(child.Targets) == 0 {
		return model.Transaction{}, errors.New("recorded child installation has no targets")
	}
	for _, name := range child.Targets {
		if !allowed[name] {
			return model.Transaction{}, errors.New("recorded child installation target is outside its assisted transaction")
		}
	}
	return child, nil
}

func (m *Manager) validatedMCPRecoveryMutation(
	step model.AssistedInstallStep,
	transactionID string,
) (mcpMutation, error) {
	expectedConfig, err := m.codexConfigPath()
	if err != nil {
		return mcpMutation{}, err
	}
	if !strings.EqualFold(filepath.Clean(step.TargetPath), filepath.Clean(expectedConfig)) {
		return mcpMutation{}, errors.New("recorded Codex configuration recovery target is invalid")
	}
	expectedManifest := filepath.Join(
		m.Config.Paths.DataRoot,
		"integrations",
		"mcp",
		strings.ToLower(step.MCPServerName)+".json",
	)
	if !strings.EqualFold(filepath.Clean(step.ManifestPath), filepath.Clean(expectedManifest)) {
		return mcpMutation{}, errors.New("recorded MCP ownership recovery target is invalid")
	}
	if step.OriginalMissing {
		if step.BackupPath != "" {
			return mcpMutation{}, errors.New("unexpected Codex configuration backup was recorded")
		}
	} else {
		expectedBackup := filepath.Join(
			m.Config.Paths.BackupsRoot,
			"_transactions",
			transactionID,
			"codex-config.toml",
		)
		if !strings.EqualFold(filepath.Clean(step.BackupPath), filepath.Clean(expectedBackup)) {
			return mcpMutation{}, errors.New("recorded Codex configuration backup is invalid")
		}
		if err := ensureWithinOrEqual(m.Config.Paths.BackupsRoot, step.BackupPath); err != nil {
			return mcpMutation{}, err
		}
	}
	if decoded, err := hex.DecodeString(step.AppliedHash); err != nil || len(decoded) != 32 {
		return mcpMutation{}, errors.New("recorded Codex configuration hash is invalid")
	}
	return mcpMutation{
		ConfigPath:      step.TargetPath,
		BackupPath:      step.BackupPath,
		AppliedHash:     step.AppliedHash,
		OriginalMissing: step.OriginalMissing,
		ManifestPath:    step.ManifestPath,
	}, nil
}

func (m *Manager) validateManagedToolRecovery(
	step model.AssistedInstallStep,
	planID string,
	transactionID string,
) error {
	toolsRoot := filepath.Join(m.Config.Paths.DataRoot, "tools")
	if err := ensureWithinOrEqual(toolsRoot, step.TargetPath); err != nil {
		return errors.New("recorded managed tool recovery target is invalid")
	}
	expectedTarget := filepath.Join(
		m.Config.Paths.DataRoot,
		"tools",
		"python",
		normalizePackagePath(step.PythonPackage),
		strings.TrimPrefix(step.VersionSpec, "==")+"-"+shortPlanDigest(planID),
	)
	if !strings.EqualFold(filepath.Clean(step.TargetPath), filepath.Clean(expectedTarget)) {
		return errors.New("recorded managed tool recovery target does not match the approved plan")
	}
	targetInfo, err := os.Lstat(step.TargetPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !targetInfo.IsDir() || targetInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("recorded managed tool recovery target is not a real directory")
	}
	expectedManifest := filepath.Join(step.TargetPath, "csm-tool-manifest.json")
	if !strings.EqualFold(filepath.Clean(step.ManifestPath), filepath.Clean(expectedManifest)) {
		return errors.New("recorded managed tool manifest target is invalid")
	}
	fingerprint, err := fileFingerprint(step.ManifestPath)
	if errors.Is(err, os.ErrNotExist) ||
		(fingerprint == missingFileFingerprint &&
			(step.Status == "running" || step.Status == "interrupted" ||
				step.Status == "failed" || step.Status == "cancelled")) {
		// The intent was journaled before creating the environment. A missing
		// manifest therefore identifies an incomplete, manager-owned target,
		// which is safe to move to quarantine but never to execute.
		return nil
	}
	if err != nil {
		return err
	}
	if fingerprint != step.AppliedHash {
		return errors.New("managed tool ownership record changed after installation")
	}
	data, err := os.ReadFile(step.ManifestPath)
	if err != nil {
		return err
	}
	var manifest struct {
		PlanID        string `json:"planId"`
		TransactionID string `json:"transactionId"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	if manifest.PlanID != planID || manifest.TransactionID != transactionID {
		return errors.New("managed tool ownership record does not match the recovery transaction")
	}
	return nil
}

func (m *Manager) rollbackAssistedInstall(original model.Transaction) (model.Transaction, error) {
	m.assistMu.Lock()
	defer m.assistMu.Unlock()
	if original.RecoveryStatus == "completed" {
		return model.Transaction{}, errors.New("planned installation has already been rolled back")
	}
	plan, err := m.assistedPlanForTransaction(original)
	if err != nil {
		return model.Transaction{}, err
	}
	tx := model.Transaction{
		ID:        "tx-" + time.Now().UTC().Format("20060102T150405.000000000"),
		Type:      "rollback-assisted-install",
		Status:    "running",
		Targets:   append([]string(nil), original.Targets...),
		StartedAt: time.Now().UTC(),
		Steps:     cloneAssistedSteps(plan.Steps),
	}
	if err := m.store.SaveTransaction(tx); err != nil {
		return model.Transaction{}, err
	}
	recoveryErr := m.recoverAssistedSteps(&plan, &original)
	completedAt := time.Now().UTC()
	tx.CompletedAt = completedAt
	tx.Steps = cloneAssistedSteps(plan.Steps)
	if recoveryErr != nil {
		tx.Status = "failed"
		tx.Error = recoveryErr.Error()
		tx.RecoveryStatus = "required"
		original.RecoveryStatus = "required"
		plan.RecoveryStatus = "required"
		recoveryErr = errors.Join(
			recoveryErr,
			m.checkpointAssistedInstall(plan, original),
			m.store.SaveTransaction(tx),
		)
		tx.Error = recoveryErr.Error()
		m.recordTransaction(tx)
		return tx, recoveryErr
	}
	tx.Status = "completed"
	tx.RecoveryStatus = "completed"
	original.RecoveryStatus = "completed"
	plan.RecoveryStatus = "completed"
	if plan.Status == "completed" || plan.Status == "partial" {
		plan.Status = "rolled-back"
	}
	if persistErr := errors.Join(
		m.checkpointAssistedInstall(plan, original),
		m.store.SaveTransaction(tx),
	); persistErr != nil {
		tx.Status = "failed"
		tx.Error = persistErr.Error()
		tx.RecoveryStatus = "required"
		original.RecoveryStatus = "required"
		plan.RecoveryStatus = "required"
		_ = m.checkpointAssistedInstall(plan, original)
		_ = m.store.SaveTransaction(tx)
		m.recordTransaction(tx)
		return tx, persistErr
	}
	m.recordTransaction(tx)
	return tx, nil
}

func (m *Manager) assistedPlanForTransaction(
	transaction model.Transaction,
) (model.AssistedInstallPlan, error) {
	m.assistData.Lock()
	for _, plan := range m.assisted {
		if plan.TransactionID == transaction.ID {
			m.assistData.Unlock()
			return plan, nil
		}
	}
	m.assistData.Unlock()
	root := assistedInstallPlanRoot(m.Config.Paths.DataRoot)
	entries, err := os.ReadDir(root)
	if err != nil {
		return model.AssistedInstallPlan{}, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") ||
			strings.HasSuffix(entry.Name(), ".progress.json") {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(root, entry.Name()))
		if readErr != nil {
			continue
		}
		var plan model.AssistedInstallPlan
		if json.Unmarshal(data, &plan) == nil && plan.TransactionID == transaction.ID {
			return plan, nil
		}
	}
	return model.AssistedInstallPlan{}, errors.New("assisted-install plan for transaction was not found")
}

func (m *Manager) assistedSourcePreview(planID string) (model.InstallPreview, error) {
	planID = strings.TrimSpace(planID)
	if filepath.Base(planID) != planID || !strings.HasPrefix(planID, "plan-") {
		return model.InstallPreview{}, errors.New("invalid source install plan ID")
	}
	m.mu.Lock()
	preview, ok := m.previews[planID]
	m.mu.Unlock()
	if ok {
		if err := m.verifyInstallPreviewMetadata(preview, planID); err != nil {
			return model.InstallPreview{}, err
		}
		return preview, nil
	}
	preview, err := loadPreview(m.Config.Paths.DataRoot, planID)
	if err != nil {
		return model.InstallPreview{}, err
	}
	if err := m.verifyInstallPreviewMetadata(preview, planID); err != nil {
		return model.InstallPreview{}, err
	}
	m.mu.Lock()
	m.previews[planID] = preview
	m.mu.Unlock()
	return preview, nil
}

func (m *Manager) assistedInstallPlan(planID string) (model.AssistedInstallPlan, error) {
	planID = strings.TrimSpace(planID)
	if filepath.Base(planID) != planID || !strings.HasPrefix(planID, "assisted-plan-") {
		return model.AssistedInstallPlan{}, errors.New("invalid assisted-install plan ID")
	}
	m.assistData.Lock()
	plan, ok := m.assisted[planID]
	m.assistData.Unlock()
	if ok {
		return plan, nil
	}
	plan, err := loadAssistedInstallPlan(m.Config.Paths.DataRoot, planID)
	if err != nil {
		return model.AssistedInstallPlan{}, err
	}
	m.assistData.Lock()
	m.assisted[planID] = plan
	m.assistData.Unlock()
	return plan, nil
}

func (m *Manager) assistedInstallPlanBySource(sourcePlanID string) (model.AssistedInstallPlan, error) {
	if !validAssistedReferenceID(sourcePlanID) || validAssistedPlanID(sourcePlanID) {
		return model.AssistedInstallPlan{}, errors.New("invalid assisted-install source reference")
	}
	var (
		selected model.AssistedInstallPlan
		found    bool
	)
	m.assistData.Lock()
	for _, plan := range m.assisted {
		if plan.SourcePlanID == sourcePlanID && (!found || plan.CreatedAt.After(selected.CreatedAt)) {
			selected = plan
			found = true
		}
	}
	m.assistData.Unlock()
	if found {
		return selected, nil
	}
	entries, err := os.ReadDir(assistedInstallPlanRoot(m.Config.Paths.DataRoot))
	if err != nil {
		return model.AssistedInstallPlan{}, fmt.Errorf(
			"assisted-install plan for source %q is not available: %w",
			sourcePlanID,
			err,
		)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "assisted-plan-") ||
			!strings.HasSuffix(name, ".json") {
			continue
		}
		planID := strings.TrimSuffix(name, ".json")
		plan, loadErr := loadAssistedInstallPlan(m.Config.Paths.DataRoot, planID)
		if loadErr != nil || plan.SourcePlanID != sourcePlanID {
			continue
		}
		if !found || plan.CreatedAt.After(selected.CreatedAt) {
			selected = plan
			found = true
		}
	}
	if !found {
		return model.AssistedInstallPlan{}, fmt.Errorf(
			"assisted-install analysis for source %q has not produced a plan",
			sourcePlanID,
		)
	}
	m.assistData.Lock()
	m.assisted[selected.ID] = selected
	m.assistData.Unlock()
	return selected, nil
}

func (m *Manager) storeAssistedPlan(plan model.AssistedInstallPlan) error {
	m.assistData.Lock()
	m.assisted[plan.ID] = plan
	m.assistData.Unlock()
	return saveAssistedInstallPlan(m.Config.Paths.DataRoot, plan)
}

func (m *Manager) checkpointAssistedInstall(
	plan model.AssistedInstallPlan,
	tx model.Transaction,
) error {
	tx.Steps = cloneAssistedSteps(plan.Steps)
	planErr := m.storeAssistedPlan(plan)
	transactionErr := m.store.SaveTransaction(tx)
	if planErr != nil {
		planErr = fmt.Errorf("save assisted-install plan checkpoint: %w", planErr)
	}
	if transactionErr != nil {
		transactionErr = fmt.Errorf(
			"save assisted-install transaction checkpoint: %w",
			transactionErr,
		)
	}
	return errors.Join(planErr, transactionErr)
}

func (m *Manager) startAssistedExecutionProgress(
	plan model.AssistedInstallPlan,
	tx model.Transaction,
	progress AssistedInstallProgressFunc,
) model.AssistedInstallProgress {
	value := model.AssistedInstallProgress{
		ReferenceID: plan.ID,
		RunID:       tx.ID,
		Sequence:    1,
		Phase:       "executing",
		Message: m.assistedMessage(
			"正在执行已批准的安装计划",
			"Executing the approved installation plan",
		),
		TotalSteps: len(plan.Steps),
		Steps:      progressStepsFromPlan(plan.Steps),
		StartedAt:  tx.StartedAt,
		UpdatedAt:  tx.StartedAt,
	}
	recountAssistedProgress(&value)
	m.rememberAssistedProgress(value, progress)
	return value
}

func (m *Manager) updateAssistedExecutionProgress(
	planID string,
	runID string,
	progress AssistedInstallProgressFunc,
	persist bool,
	update func(*model.AssistedInstallProgress),
) model.AssistedInstallProgress {
	m.assistData.Lock()
	value, ok := m.progress[planID]
	if !ok || value.RunID != runID {
		now := time.Now().UTC()
		value = model.AssistedInstallProgress{
			ReferenceID: planID,
			RunID:       runID,
			StartedAt:   now,
			UpdatedAt:   now,
		}
	}
	update(&value)
	value.Sequence++
	value.UpdatedAt = time.Now().UTC()
	recountAssistedProgress(&value)
	m.progress[planID] = cloneAssistedProgress(value)
	m.progress[runID] = cloneAssistedProgress(value)
	m.assistData.Unlock()
	if persist {
		m.persistAssistedProgress(value)
	}
	if progress != nil {
		progress(cloneAssistedProgress(value))
	}
	return cloneAssistedProgress(value)
}

func (m *Manager) bumpAssistedActivity(
	planID string,
	runID string,
	progress AssistedInstallProgressFunc,
) {
	var emit *model.AssistedInstallProgress
	m.assistData.Lock()
	value, ok := m.progress[planID]
	if ok && value.RunID == runID {
		value.ActivityCount++
		now := time.Now().UTC()
		if now.Sub(value.UpdatedAt) >= 500*time.Millisecond {
			value.Sequence++
			value.UpdatedAt = now
			copyValue := cloneAssistedProgress(value)
			emit = &copyValue
		}
		m.progress[planID] = cloneAssistedProgress(value)
		m.progress[runID] = cloneAssistedProgress(value)
	}
	m.assistData.Unlock()
	if emit != nil && progress != nil {
		progress(*emit)
	}
}

func (m *Manager) rememberAssistedProgress(
	value model.AssistedInstallProgress,
	progress AssistedInstallProgressFunc,
) {
	value = cloneAssistedProgress(value)
	persist := value.Terminal
	m.assistData.Lock()
	current, ok := m.progress[value.ReferenceID]
	if ok && current.RunID == value.RunID && current.Sequence >= value.Sequence {
		m.assistData.Unlock()
		return
	}
	if !ok || current.Phase != value.Phase || current.CurrentStepID != value.CurrentStepID {
		persist = true
	}
	m.progress[value.ReferenceID] = value
	if value.RunID != "" {
		m.progress[value.RunID] = value
	}
	m.assistData.Unlock()
	if persist {
		m.persistAssistedProgress(value)
	}
	if progress != nil {
		progress(cloneAssistedProgress(value))
	}
}

func (m *Manager) persistAssistedProgress(value model.AssistedInstallProgress) {
	_ = saveAssistedInstallProgress(m.Config.Paths.DataRoot, value.ReferenceID, value)
	if value.RunID != "" && value.RunID != value.ReferenceID &&
		validAssistedReferenceID(value.RunID) {
		_ = saveAssistedInstallProgress(m.Config.Paths.DataRoot, value.RunID, value)
	}
}

func (m *Manager) registerAssistedCancel(ids []string, cancel context.CancelFunc) error {
	m.assistData.Lock()
	defer m.assistData.Unlock()
	for _, id := range ids {
		if id != "" && m.cancels[id] != nil {
			return fmt.Errorf("planned installation is already running for reference %s", id)
		}
	}
	for _, id := range ids {
		if id != "" {
			m.cancels[id] = cancel
		}
	}
	return nil
}

func (m *Manager) unregisterAssistedCancel(ids []string) {
	m.assistData.Lock()
	defer m.assistData.Unlock()
	for _, id := range ids {
		delete(m.cancels, id)
	}
}

func (m *Manager) hasActiveAssistedRun(ids ...string) bool {
	m.assistData.Lock()
	defer m.assistData.Unlock()
	for _, id := range ids {
		if id != "" && m.cancels[id] != nil {
			return true
		}
	}
	return false
}

func validateAssistedPermissions(
	permissions []model.AssistedInstallPermission,
	selected []string,
) (map[string]bool, error) {
	known := make(map[string]model.AssistedInstallPermission, len(permissions))
	for _, permission := range permissions {
		known[permission.ID] = permission
	}
	approved := map[string]bool{}
	for _, id := range uniqueNonEmpty(selected) {
		if _, ok := known[id]; !ok {
			return nil, fmt.Errorf("permission %q is not in the approved installation plan", id)
		}
		approved[id] = true
	}
	for _, permission := range permissions {
		if permission.Required && !approved[permission.ID] {
			return nil, fmt.Errorf("required permission %q was not approved", permission.Title)
		}
	}
	return approved, nil
}

func allPermissionsApproved(ids []string, approved map[string]bool) bool {
	for _, id := range ids {
		if !approved[id] {
			return false
		}
	}
	return true
}

func hasScheduledAssistedStep(
	steps []model.AssistedInstallStep,
	kind string,
	approved map[string]bool,
) bool {
	for _, step := range steps {
		if step.Kind == kind && step.Supported &&
			allPermissionsApproved(step.PermissionIDs, approved) {
			return true
		}
	}
	return false
}

func hasManualPendingAssistedStep(steps []model.AssistedInstallStep) bool {
	for _, step := range steps {
		if step.Status == "manual-pending" {
			return true
		}
	}
	return false
}

func validateAssistedPermissionDependencies(
	steps []model.AssistedInstallStep,
	approved map[string]bool,
) error {
	scheduledTools := map[string]bool{}
	for _, step := range steps {
		if !step.Supported || !allPermissionsApproved(step.PermissionIDs, approved) {
			continue
		}
		switch step.Kind {
		case model.AssistedInstallStepManagedPythonTool:
			scheduledTools[strings.ToLower(step.Entrypoint)] = true
		case model.AssistedInstallStepConfigureCodexMCP:
			if !scheduledTools[strings.ToLower(step.Entrypoint)] {
				return fmt.Errorf(
					"MCP step %q requires approval of its earlier managed tool step",
					step.Title,
				)
			}
		}
	}
	return nil
}

func sortedTrueKeys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for key, value := range values {
		if value {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}

func cloneAssistedSteps(steps []model.AssistedInstallStep) []model.AssistedInstallStep {
	out := append([]model.AssistedInstallStep(nil), steps...)
	for index := range out {
		out[index].SkillNames = append([]string(nil), out[index].SkillNames...)
		out[index].MCPArgs = append([]string(nil), out[index].MCPArgs...)
		out[index].PermissionIDs = append([]string(nil), out[index].PermissionIDs...)
		out[index].PythonWheels = clonePythonWheelLocks(out[index].PythonWheels)
		if out[index].OutputHashes != nil {
			hashes := make(map[string]string, len(out[index].OutputHashes))
			for key, value := range out[index].OutputHashes {
				hashes[key] = value
			}
			out[index].OutputHashes = hashes
		}
	}
	return out
}

func cloneAssistedPlanForVerification(plan model.AssistedInstallPlan) model.AssistedInstallPlan {
	cloned := plan
	cloned.Requirements = append([]model.AssistedInstallRequirement(nil), plan.Requirements...)
	cloned.Steps = cloneAssistedSteps(plan.Steps)
	cloned.Permissions = append([]model.AssistedInstallPermission(nil), plan.Permissions...)
	for index := range cloned.Permissions {
		cloned.Permissions[index].Targets = append(
			[]string(nil),
			cloned.Permissions[index].Targets...,
		)
	}
	cloned.Warnings = append([]string(nil), plan.Warnings...)
	cloned.Skills = append([]model.CandidateSkill(nil), plan.Skills...)
	for index := range cloned.Skills {
		cloned.Skills[index].Files = append(
			[]model.FileRecord(nil),
			cloned.Skills[index].Files...,
		)
	}
	cloned.SelectedSkills = append([]string(nil), plan.SelectedSkills...)
	return cloned
}

func progressStepsFromPlan(steps []model.AssistedInstallStep) []model.AssistedInstallProgressStep {
	out := make([]model.AssistedInstallProgressStep, 0, len(steps))
	for _, step := range steps {
		status := step.Status
		if status == "planned" {
			status = "queued"
		}
		out = append(out, model.AssistedInstallProgressStep{
			ID:          step.ID,
			Kind:        step.Kind,
			Title:       step.Title,
			Status:      status,
			Error:       step.Error,
			StartedAt:   step.StartedAt,
			CompletedAt: step.CompletedAt,
		})
	}
	return out
}

func updateProgressStep(
	progress *model.AssistedInstallProgress,
	id string,
	status string,
	message string,
	errorMessage string,
) {
	now := time.Now().UTC()
	for index := range progress.Steps {
		step := &progress.Steps[index]
		if step.ID != id {
			continue
		}
		step.Status = status
		if message != "" {
			step.Message = message
		}
		if errorMessage != "" {
			step.Error = errorMessage
		}
		if status == "running" && step.StartedAt == nil {
			step.StartedAt = &now
		}
		switch status {
		case "completed", "failed", "cancelled", "skipped", "manual-pending", "rolled-back":
			step.CompletedAt = &now
		}
		return
	}
}

func recountAssistedProgress(progress *model.AssistedInstallProgress) {
	progress.CompletedSteps = 0
	for _, step := range progress.Steps {
		switch step.Status {
		case "completed", "skipped", "manual-pending", "rolled-back":
			progress.CompletedSteps++
		}
	}
}

func cloneAssistedProgress(value model.AssistedInstallProgress) model.AssistedInstallProgress {
	value.Steps = append([]model.AssistedInstallProgressStep(nil), value.Steps...)
	return value
}

func assistedInstallPlanRoot(dataRoot string) string {
	return filepath.Join(dataRoot, "assisted-install")
}

func saveAssistedInstallPlan(dataRoot string, plan model.AssistedInstallPlan) error {
	if !validAssistedPlanID(plan.ID) {
		return errors.New("invalid assisted-install plan ID")
	}
	return writeJSONAtomic(
		filepath.Join(assistedInstallPlanRoot(dataRoot), plan.ID+".json"),
		plan,
	)
}

func loadAssistedInstallPlan(dataRoot string, planID string) (model.AssistedInstallPlan, error) {
	if !validAssistedPlanID(planID) {
		return model.AssistedInstallPlan{}, errors.New("invalid assisted-install plan ID")
	}
	data, err := os.ReadFile(filepath.Join(assistedInstallPlanRoot(dataRoot), planID+".json"))
	if err != nil {
		return model.AssistedInstallPlan{}, err
	}
	var plan model.AssistedInstallPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return model.AssistedInstallPlan{}, err
	}
	if plan.ID != planID {
		return model.AssistedInstallPlan{}, errors.New("assisted-install plan identity mismatch")
	}
	return plan, nil
}

func saveAssistedInstallProgress(
	dataRoot string,
	referenceID string,
	progress model.AssistedInstallProgress,
) error {
	if !validAssistedReferenceID(referenceID) {
		return errors.New("invalid assisted-install progress reference ID")
	}
	return writeJSONAtomic(
		filepath.Join(assistedInstallPlanRoot(dataRoot), referenceID+".progress.json"),
		progress,
	)
}

func loadAssistedInstallProgress(
	dataRoot string,
	referenceID string,
) (model.AssistedInstallProgress, error) {
	if !validAssistedReferenceID(referenceID) {
		return model.AssistedInstallProgress{}, errors.New("invalid assisted-install progress reference ID")
	}
	data, err := os.ReadFile(
		filepath.Join(assistedInstallPlanRoot(dataRoot), referenceID+".progress.json"),
	)
	if err != nil {
		return model.AssistedInstallProgress{}, err
	}
	var progress model.AssistedInstallProgress
	if err := json.Unmarshal(data, &progress); err != nil {
		return model.AssistedInstallProgress{}, err
	}
	return progress, nil
}

func validAssistedReferenceID(value string) bool {
	if len(value) == 0 || len(value) > 160 || filepath.Base(value) != value ||
		filepath.VolumeName(value) != "" {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') && char != '-' && char != '.' {
			return false
		}
	}
	return strings.HasPrefix(value, "assisted-plan-") ||
		strings.HasPrefix(value, "plan-") ||
		strings.HasPrefix(value, "tx-")
}

func validAssistedPlanID(value string) bool {
	return strings.HasPrefix(value, "assisted-plan-") && validAssistedReferenceID(value)
}

func (m *Manager) assistedMessage(chinese string, english string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(m.Config.Locale)), "en") {
		return english
	}
	return chinese
}

func assistedPlanStatusMessage(status string, locale string) string {
	english := strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "en")
	messages := map[string][2]string{
		"ready":           {"安装计划已就绪", "Installation plan is ready"},
		"manual-required": {"计划包含人工步骤", "The plan contains a manual step"},
		"running":         {"安装正在执行", "Installation is running"},
		"completed":       {"安装已完成", "Installation completed"},
		"partial":         {"支持的步骤已完成，仍有人工步骤", "Supported steps completed; manual steps remain"},
		"failed":          {"安装执行失败", "Installation failed"},
		"cancelled":       {"安装已取消", "Installation was cancelled"},
		"interrupted":     {"安装被中断", "Installation was interrupted"},
		"rolled-back":     {"安装已回滚", "Installation was rolled back"},
	}
	message := messages[status]
	if english {
		return message[1]
	}
	return message[0]
}
