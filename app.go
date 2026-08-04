package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/auth"
	"github.com/bme-lyh/Codex-Skill-Manager/internal/config"
	"github.com/bme-lyh/Codex-Skill-Manager/internal/manager"
	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
	"github.com/bme-lyh/Codex-Skill-Manager/internal/scheduler"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx context.Context
	mgr *manager.Manager
}

type QuarantineItem struct {
	Skill         string `json:"skill"`
	RootID        string `json:"rootId"`
	TransactionID string `json:"transactionId"`
	Path          string `json:"path"`
}

func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	mgr, err := manager.Open("")
	if err == nil {
		a.mgr = mgr
	}
}

func (a *App) shutdown(context.Context) {
	if a.mgr != nil {
		_ = a.mgr.Close()
	}
}

func (a *App) ready() error {
	if a.mgr == nil {
		return errors.New("管理核心初始化失败；请检查配置目录权限")
	}
	return nil
}

func (a *App) GetDashboard() (model.Dashboard, error) {
	if err := a.ready(); err != nil {
		return model.Dashboard{}, err
	}
	return a.mgr.Dashboard()
}

func (a *App) BootstrapCurrentSkills() error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.mgr.BootstrapCurrentSkills()
}

func (a *App) PrepareAdoption(names []string, rootID string) (model.AdoptionPreview, error) {
	if err := a.ready(); err != nil {
		return model.AdoptionPreview{}, err
	}
	if err := requireRootID(rootID); err != nil {
		return model.AdoptionPreview{}, err
	}
	return a.mgr.PrepareAdoption(names, rootID)
}

func (a *App) ApplyAdoption(planID string, names []string, rootID string) (model.Transaction, error) {
	if err := a.ready(); err != nil {
		return model.Transaction{}, err
	}
	if err := requireRootID(rootID); err != nil {
		return model.Transaction{}, err
	}
	return a.mgr.ApplyAdoption(planID, names, rootID)
}

// ApplyAdoptionBestEffort is used by select-all management actions so one
// stale or conflicting Skill does not cancel the other explicit targets.
func (a *App) ApplyAdoptionBestEffort(planID string, names []string, rootID string) (model.Transaction, error) {
	if err := a.ready(); err != nil {
		return model.Transaction{}, err
	}
	if err := requireRootID(rootID); err != nil {
		return model.Transaction{}, err
	}
	return a.mgr.ApplyAdoptionBestEffort(planID, names, rootID)
}

func (a *App) CreateGroup(name, rootID string) (model.Transaction, error) {
	if err := a.ready(); err != nil {
		return model.Transaction{}, err
	}
	if err := requireRootID(rootID); err != nil {
		return model.Transaction{}, err
	}
	return a.mgr.CreateGroup(name, rootID)
}

func (a *App) RenameGroup(id, name, rootID string) (model.Transaction, error) {
	if err := a.ready(); err != nil {
		return model.Transaction{}, err
	}
	if err := requireRootID(rootID); err != nil {
		return model.Transaction{}, err
	}
	return a.mgr.RenameGroup(id, name, rootID)
}

func (a *App) ReorderGroups(ids []string, rootID string) (model.Transaction, error) {
	if err := a.ready(); err != nil {
		return model.Transaction{}, err
	}
	if err := requireRootID(rootID); err != nil {
		return model.Transaction{}, err
	}
	return a.mgr.ReorderGroups(ids, rootID)
}

func (a *App) MoveSkillsToGroup(names []string, groupID, rootID string) (model.Transaction, error) {
	if err := a.ready(); err != nil {
		return model.Transaction{}, err
	}
	if err := requireRootID(rootID); err != nil {
		return model.Transaction{}, err
	}
	return a.mgr.MoveSkillsToGroup(names, groupID, rootID)
}

func (a *App) PrepareGitHub(rawURL, ref, rootID string) (model.InstallPreview, error) {
	if err := a.ready(); err != nil {
		return model.InstallPreview{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 5*time.Minute)
	defer cancel()
	if strings.TrimSpace(rootID) == "" {
		rootID = model.RootIDCodexDefault
	}
	return a.mgr.PrepareGitHub(ctx, rawURL, ref, rootID)
}

func (a *App) PrepareLocal(path, rootID string) (model.InstallPreview, error) {
	if err := a.ready(); err != nil {
		return model.InstallPreview{}, err
	}
	if strings.TrimSpace(rootID) == "" {
		rootID = model.RootIDCodexDefault
	}
	return a.mgr.PrepareLocal(path, rootID)
}

func (a *App) SetSourceTrust(repository, reason string) (model.Transaction, error) {
	if err := a.ready(); err != nil {
		return model.Transaction{}, err
	}
	return a.mgr.SetSourceTrust(repository, reason)
}

func (a *App) RevokeSourceTrust(repository, reason string) (model.Transaction, error) {
	if err := a.ready(); err != nil {
		return model.Transaction{}, err
	}
	return a.mgr.RevokeSourceTrust(repository, reason)
}

func (a *App) GetSourceTrustPolicy(repository string) (model.SourceTrustPolicy, error) {
	if err := a.ready(); err != nil {
		return model.SourceTrustPolicy{}, err
	}
	return a.mgr.SourceTrustPolicy(repository)
}

func (a *App) GetSourceTrustPolicies() ([]model.SourceTrustPolicy, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.mgr.SourceTrustPolicies()
}

func (a *App) GetSourceTrustAudit(repository string, limit int) ([]model.SourceTrustAudit, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.mgr.SourceTrustAudit(repository, limit)
}

// LinkLocalSource records an explicit, hash-verified GitHub association for an
// existing local Skill without replacing its files.
func (a *App) LinkLocalSource(skillName, rawURL, ref, rootID string) (model.DetectedSource, error) {
	if err := a.ready(); err != nil {
		return model.DetectedSource{}, err
	}
	if err := requireRootID(rootID); err != nil {
		return model.DetectedSource{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 5*time.Minute)
	defer cancel()
	return a.mgr.LinkLocalSource(ctx, skillName, rawURL, ref, rootID)
}

// AssessInstallSource performs the mandatory deterministic local assessment.
// It does not call Codex, download dependencies, or mutate installation targets.
func (a *App) AssessInstallSource(planID string) (model.ProjectAssessment, error) {
	if a.mgr == nil {
		return model.ProjectAssessment{}, errors.New("manager not initialized")
	}
	return a.mgr.AssessInstallSource(planID)
}

func (a *App) GetProjectAssessment(reference string) (model.ProjectAssessment, error) {
	if a.mgr == nil {
		return model.ProjectAssessment{}, errors.New("manager not initialized")
	}
	return a.mgr.GetProjectAssessment(reference)
}

func (a *App) ApplyInstall(planID string, skills []string, acceptHighRisk bool, rootID string) (model.Transaction, error) {
	if err := a.ready(); err != nil {
		return model.Transaction{}, err
	}
	if err := requireRootID(rootID); err != nil {
		return model.Transaction{}, err
	}
	return a.mgr.ApplyInstall(planID, skills, acceptHighRisk, rootID)
}

// ApplyInstallBestEffort is the select-all variant: every selected Skill gets
// its own journaled child transaction and the parent reports partial success.
func (a *App) ApplyInstallBestEffort(planID string, skills []string, acceptHighRisk bool, rootID string) (model.Transaction, error) {
	if err := a.ready(); err != nil {
		return model.Transaction{}, err
	}
	if err := requireRootID(rootID); err != nil {
		return model.Transaction{}, err
	}
	return a.mgr.ApplyInstallBestEffort(planID, skills, acceptHighRisk, rootID)
}

func (a *App) ApproveGroupRisk(planID, reason string) (model.Transaction, error) {
	if err := a.ready(); err != nil {
		return model.Transaction{}, err
	}
	return a.mgr.ApproveGroupRisk(planID, reason)
}

func (a *App) ApproveGroupSecurity(groupID, rootID, reason string) (model.Transaction, error) {
	if err := a.ready(); err != nil {
		return model.Transaction{}, err
	}
	if err := requireRootID(rootID); err != nil {
		return model.Transaction{}, err
	}
	return a.mgr.ApproveGroupSecurity(groupID, rootID, reason)
}

func (a *App) ApplyGroupInstall(planID string, skills []string, acceptRisk bool, rootID string) (model.Transaction, error) {
	if err := a.ready(); err != nil {
		return model.Transaction{}, err
	}
	if err := requireRootID(rootID); err != nil {
		return model.Transaction{}, err
	}
	return a.mgr.ApplyGroupInstall(planID, skills, acceptRisk, rootID)
}

func (a *App) ApplySourceGroupInstall(planID string, acceptRisk bool, rootID string) (model.Transaction, error) {
	if err := a.ready(); err != nil {
		return model.Transaction{}, err
	}
	if err := requireRootID(rootID); err != nil {
		return model.Transaction{}, err
	}
	return a.mgr.ApplySourceGroupInstall(planID, acceptRisk, rootID)
}

func (a *App) ApplyGroupUpdate(planID string, skills []string, acceptRisk bool, rootID string) (model.Transaction, error) {
	if err := a.ready(); err != nil {
		return model.Transaction{}, err
	}
	if err := requireRootID(rootID); err != nil {
		return model.Transaction{}, err
	}
	return a.mgr.ApplyGroupUpdate(planID, skills, acceptRisk, rootID)
}

// ScanProjectWithCodex is the read-only first phase for planned installation.
// It returns an overview/security/install-method result without creating an
// installation plan or applying any mutation.
func (a *App) ScanProjectWithCodex(planID string) (model.CodexProjectScanResult, error) {
	if err := a.ready(); err != nil {
		return model.CodexProjectScanResult{}, err
	}
	return a.mgr.ScanProjectWithCodex(
		a.ctx,
		planID,
		func(progress model.AssistedInstallProgress) {
			wailsruntime.EventsEmit(a.ctx, "assisted-install-progress", progress)
		},
	)
}

func (a *App) AnalyzeInstallFromProjectScan(scanID string) (model.AssistedInstallPlan, error) {
	if err := a.ready(); err != nil {
		return model.AssistedInstallPlan{}, err
	}
	return a.mgr.AnalyzeInstallFromProjectScan(
		a.ctx,
		scanID,
		func(progress model.AssistedInstallProgress) {
			wailsruntime.EventsEmit(a.ctx, "assisted-install-progress", progress)
		},
	)
}

func (a *App) GetProjectScan(reference string) (model.CodexProjectScanResult, error) {
	if err := a.ready(); err != nil {
		return model.CodexProjectScanResult{}, err
	}
	return a.mgr.GetProjectScan(reference)
}

func (a *App) ApplyAssistedInstall(
	planID string,
	skills []string,
	permissionIDs []string,
	projectRoot string,
	rootID string,
) (model.AssistedInstallResult, error) {
	if err := a.ready(); err != nil {
		return model.AssistedInstallResult{}, err
	}
	if err := requireRootID(rootID); err != nil {
		return model.AssistedInstallResult{}, err
	}
	return a.mgr.ApplyAssistedInstallForRoot(
		a.ctx,
		planID,
		skills,
		permissionIDs,
		projectRoot,
		rootID,
		func(progress model.AssistedInstallProgress) {
			wailsruntime.EventsEmit(a.ctx, "assisted-install-progress", progress)
		},
	)
}

// ConfirmCodexInstall records the single human acknowledgement for a
// digest-bound Codex plan. The manager reloads and validates the source,
// assessment, review and plan records before issuing the opaque confirmation.
func (a *App) ConfirmCodexInstall(
	planID string,
	skills []string,
	permissionIDs []string,
	acceptHighRisk bool,
	rootID string,
) (manager.InstallConfirmation, error) {
	if err := a.ready(); err != nil {
		return manager.InstallConfirmation{}, err
	}
	if err := requireRootID(rootID); err != nil {
		return manager.InstallConfirmation{}, err
	}
	return a.mgr.ConfirmCodexInstall(planID, skills, permissionIDs, acceptHighRisk, rootID)
}

// ApplyConfirmedAssistedInstall consumes the one-time acknowledgement and
// delegates to the existing journaled assisted-install executor. No renderer
// supplied selection or digest is trusted at this boundary.
func (a *App) ApplyConfirmedAssistedInstall(
	confirmationID string,
	projectRoot string,
	rootID string,
) (model.AssistedInstallResult, error) {
	if err := a.ready(); err != nil {
		return model.AssistedInstallResult{}, err
	}
	if err := requireRootID(rootID); err != nil {
		return model.AssistedInstallResult{}, err
	}
	return a.mgr.ApplyConfirmedAssistedInstall(
		a.ctx,
		confirmationID,
		projectRoot,
		rootID,
		func(progress model.AssistedInstallProgress) {
			wailsruntime.EventsEmit(a.ctx, "assisted-install-progress", progress)
		},
	)
}

func (a *App) GetAssistedInstallPlan(planID string) (model.AssistedInstallPlan, error) {
	if err := a.ready(); err != nil {
		return model.AssistedInstallPlan{}, err
	}
	return a.mgr.GetAssistedInstallPlan(planID)
}

func (a *App) GetAssistedInstallProgress(referenceID string) (model.AssistedInstallProgress, error) {
	if err := a.ready(); err != nil {
		return model.AssistedInstallProgress{}, err
	}
	return a.mgr.GetAssistedInstallProgress(referenceID)
}

func (a *App) CancelAssistedInstall(referenceID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.mgr.CancelAssistedInstall(referenceID)
}

func (a *App) AuditSkill(name, rootID string) (model.ScanReport, error) {
	if err := a.ready(); err != nil {
		return model.ScanReport{}, err
	}
	if strings.TrimSpace(name) == "" {
		return model.ScanReport{}, errors.New("Skill name is required")
	}
	if err := requireRootID(rootID); err != nil {
		return model.ScanReport{}, err
	}
	return a.mgr.AuditSkills([]string{name}, rootID)
}

func (a *App) GetScanReport(id, rootID string) (model.ScanReport, error) {
	if err := a.ready(); err != nil {
		return model.ScanReport{}, err
	}
	return a.mgr.Report(id, rootID)
}

func (a *App) AuditSkills(names []string, rootID string) (model.ScanReport, error) {
	if err := a.ready(); err != nil {
		return model.ScanReport{}, err
	}
	if err := requireRootID(rootID); err != nil {
		return model.ScanReport{}, err
	}
	return a.mgr.AuditSkills(names, rootID)
}

func (a *App) RunGroupSecurityCheck(groupID, rootID string) (model.GroupSecurityReport, error) {
	if err := a.ready(); err != nil {
		return model.GroupSecurityReport{}, err
	}
	if err := requireRootID(rootID); err != nil {
		return model.GroupSecurityReport{}, err
	}
	return a.mgr.AuditGroup(groupID, rootID)
}

func (a *App) GetGroupSecurityReport(id string) (model.GroupSecurityReport, error) {
	if err := a.ready(); err != nil {
		return model.GroupSecurityReport{}, err
	}
	return a.mgr.GetGroupSecurityReport(id)
}

func (a *App) GetSourceAnalysis(id string) (model.SourceAnalysis, error) {
	if err := a.ready(); err != nil {
		return model.SourceAnalysis{}, err
	}
	return a.mgr.GetSourceAnalysis(id)
}

func (a *App) GetOrCreateSourceGroupAnalysis(planID string) (model.SourceAnalysis, error) {
	if err := a.ready(); err != nil {
		return model.SourceAnalysis{}, err
	}
	return a.mgr.GetOrCreateSourceGroupAnalysis(planID)
}

func (a *App) GetSourceAnalyses(limit int) ([]model.SourceAnalysis, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.mgr.SourceAnalyses(limit)
}

func (a *App) GetGroupOperations(limit int) ([]model.GroupOperation, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.mgr.GroupOperations(limit)
}

func (a *App) GetGroupOperation(id string) (model.GroupOperation, error) {
	if err := a.ready(); err != nil {
		return model.GroupOperation{}, err
	}
	return a.mgr.GetGroupOperation(id)
}

func (a *App) GetGroupMetadata(groupID, rootID string) (model.GroupMetadata, error) {
	if err := a.ready(); err != nil {
		return model.GroupMetadata{}, err
	}
	return a.mgr.GetGroupMetadata(groupID, rootID)
}

func (a *App) SetFindingIgnored(finding model.Finding, ignored bool, reason string) (bool, error) {
	if err := a.ready(); err != nil {
		return false, err
	}
	if err := a.mgr.SetFindingIgnored(finding, ignored, reason); err != nil {
		return false, err
	}
	return true, nil
}

func (a *App) SetRiskClusterIgnored(cluster model.RiskCluster, ignored bool, reason string, confirmDeterministic bool) (bool, error) {
	if err := a.ready(); err != nil {
		return false, err
	}
	if err := a.mgr.SetRiskClusterIgnored(cluster, ignored, reason, confirmDeterministic); err != nil {
		return false, err
	}
	return true, nil
}

func (a *App) SetRiskClustersIgnored(clusters []model.RiskCluster, ignored bool, reason string) (bool, error) {
	if err := a.ready(); err != nil {
		return false, err
	}
	if err := a.mgr.SetRiskClustersIgnored(clusters, ignored, reason); err != nil {
		return false, err
	}
	return true, nil
}

func (a *App) CheckUpdates() (model.UpdateCheckResult, error) {
	if err := a.ready(); err != nil {
		return model.UpdateCheckResult{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
	defer cancel()
	return a.mgr.CheckUpdates(ctx)
}

func (a *App) CheckUpdatesSelected(groupIDs []string, force bool) (model.UpdateCheckResult, error) {
	if err := a.ready(); err != nil {
		return model.UpdateCheckResult{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
	defer cancel()
	return a.mgr.CheckUpdatesSelected(ctx, groupIDs, force)
}

func (a *App) PrepareUpdate(groupID, rootID string) (model.InstallPreview, error) {
	if err := a.ready(); err != nil {
		return model.InstallPreview{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
	defer cancel()
	if err := requireRootID(rootID); err != nil {
		return model.InstallPreview{}, err
	}
	return a.mgr.PrepareUpdate(ctx, groupID, rootID)
}

func (a *App) PrepareGroupUpdate(groupID, rootID string) (model.InstallPreview, error) {
	if err := a.ready(); err != nil {
		return model.InstallPreview{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
	defer cancel()
	if err := requireRootID(rootID); err != nil {
		return model.InstallPreview{}, err
	}
	return a.mgr.PrepareGroupUpdate(ctx, groupID, rootID)
}

func (a *App) QuarantineSkills(names []string, rootID string) (model.Transaction, error) {
	if err := a.ready(); err != nil {
		return model.Transaction{}, err
	}
	if err := requireRootID(rootID); err != nil {
		return model.Transaction{}, err
	}
	return a.mgr.Quarantine(names, rootID)
}

func (a *App) RestoreSkill(name, transactionID, rootID string) (model.Transaction, error) {
	if err := a.ready(); err != nil {
		return model.Transaction{}, err
	}
	if err := requireRootID(rootID); err != nil {
		return model.Transaction{}, err
	}
	return a.mgr.Restore(name, transactionID, rootID)
}

func (a *App) Rollback(transactionID string) (model.Transaction, error) {
	if err := a.ready(); err != nil {
		return model.Transaction{}, err
	}
	return a.mgr.Rollback(transactionID)
}

func (a *App) GetConfig() (model.Config, error) {
	if err := a.ready(); err != nil {
		return model.Config{}, err
	}
	return a.mgr.Config, nil
}

func (a *App) SaveConfig(cfg model.Config) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.mgr.SaveConfig(cfg)
}

func (a *App) ConfigureSchedule(enabled bool, frequency, at string) error {
	if err := a.ready(); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return scheduler.Configure(exe, a.mgr.ConfigPath, frequency, at, enabled)
}

func (a *App) SaveGitHubToken(token, username string) error {
	return auth.SaveGitHubToken(token, username)
}

func (a *App) ValidateGitHubCredentials() model.GitHubCredentialStatus {
	if err := a.ready(); err != nil {
		return model.GitHubCredentialStatus{Message: err.Error()}
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	return a.mgr.ValidateGitHubCredentials(ctx)
}

func (a *App) GetCodexCLIStatus() model.CodexCLIStatus {
	if err := a.ready(); err != nil {
		return model.CodexCLIStatus{Error: err.Error()}
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	return a.mgr.CodexCLIStatus(ctx)
}

func (a *App) ReviewScanWithCodex(report model.ScanReport, skillNames []string) (model.ScanReport, error) {
	if err := a.ready(); err != nil {
		return model.ScanReport{}, err
	}
	ctx, cancel := context.WithCancel(a.ctx)
	defer cancel()
	return a.mgr.ReviewScanWithCodex(ctx, report, skillNames, func(progress model.CodexReviewProgress) {
		wailsruntime.EventsEmit(a.ctx, "codex-review-progress", progress)
	})
}

func (a *App) ListQuarantine(rootID string) ([]QuarantineItem, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	if err := requireRootID(rootID); err != nil {
		return nil, err
	}
	out := make([]QuarantineItem, 0)
	seen := map[string]bool{}
	rootPath := ""
	for _, root := range a.mgr.Config.SkillRoots {
		if root.ID == rootID && root.Enabled {
			rootPath = root.Path
			break
		}
	}
	if rootPath == "" {
		return nil, fmt.Errorf("unknown or disabled Skill root: %s", rootID)
	}
	roots := []string{
		a.mgr.Config.Paths.QuarantineRoot,
		filepath.Join(rootPath, ".csm-quarantine"),
	}
	history, err := a.mgr.History(1000)
	if err != nil {
		return nil, err
	}
	txRoots := make(map[string]string, len(history))
	for _, tx := range history {
		txRoots[tx.ID] = tx.RootID
	}
	for rootIndex, root := range roots {
		skills, err := os.ReadDir(root)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, skill := range skills {
			if !skill.IsDir() {
				continue
			}
			txs, _ := os.ReadDir(filepath.Join(root, skill.Name()))
			for _, tx := range txs {
				recordedRoot := txRoots[tx.Name()]
				if recordedRoot == "" {
					if rootIndex == 0 {
						recordedRoot = model.RootIDCodexDefault
					} else {
						recordedRoot = rootID
					}
				}
				if recordedRoot != rootID {
					continue
				}
				content := filepath.Join(root, skill.Name(), tx.Name(), "content")
				key := strings.ToLower(skill.Name() + "\x00" + tx.Name())
				if _, err := os.Stat(content); err == nil && !seen[key] {
					seen[key] = true
					out = append(out, QuarantineItem{Skill: skill.Name(), RootID: rootID, TransactionID: tx.Name(), Path: content})
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Skill == out[j].Skill {
			return out[i].TransactionID > out[j].TransactionID
		}
		return out[i].Skill < out[j].Skill
	})
	return out, nil
}

func (a *App) DefaultConfig() (model.Config, error) {
	return config.Default()
}

func (a *App) GetDiagnostics() (map[string]any, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	exists := func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	}
	return map[string]any{
		"version":          model.Version,
		"platform":         "windows",
		"configPath":       a.mgr.ConfigPath,
		"skillsRoot":       a.mgr.Config.Paths.SkillsRoot,
		"dataRoot":         a.mgr.Config.Paths.DataRoot,
		"logsRoot":         a.mgr.Config.Paths.LogsRoot,
		"reportsRoot":      a.mgr.Config.Paths.ReportsRoot,
		"skillsRootExists": exists(a.mgr.Config.Paths.SkillsRoot),
		"dataRootExists":   exists(a.mgr.Config.Paths.DataRoot),
		"skillRoots":       a.mgr.Config.SkillRoots,
		"defaultRootId":    a.mgr.Config.DefaultRootID,
	}, nil
}

func requireRootID(rootID string) error {
	if strings.TrimSpace(rootID) == "" {
		return errors.New("explicit rootId is required")
	}
	return nil
}
