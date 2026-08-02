package main

import (
	"context"
	"errors"
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

func (a *App) PrepareAdoption(names []string) (model.AdoptionPreview, error) {
	if err := a.ready(); err != nil {
		return model.AdoptionPreview{}, err
	}
	return a.mgr.PrepareAdoption(names)
}

func (a *App) ApplyAdoption(planID string, names []string) (model.Transaction, error) {
	if err := a.ready(); err != nil {
		return model.Transaction{}, err
	}
	return a.mgr.ApplyAdoption(planID, names)
}

func (a *App) CreateGroup(name string) (model.Transaction, error) {
	if err := a.ready(); err != nil {
		return model.Transaction{}, err
	}
	return a.mgr.CreateGroup(name)
}

func (a *App) RenameGroup(id, name string) (model.Transaction, error) {
	if err := a.ready(); err != nil {
		return model.Transaction{}, err
	}
	return a.mgr.RenameGroup(id, name)
}

func (a *App) ReorderGroups(ids []string) (model.Transaction, error) {
	if err := a.ready(); err != nil {
		return model.Transaction{}, err
	}
	return a.mgr.ReorderGroups(ids)
}

func (a *App) MoveSkillsToGroup(names []string, groupID string) (model.Transaction, error) {
	if err := a.ready(); err != nil {
		return model.Transaction{}, err
	}
	return a.mgr.MoveSkillsToGroup(names, groupID)
}

func (a *App) PrepareGitHub(rawURL, ref string) (model.InstallPreview, error) {
	if err := a.ready(); err != nil {
		return model.InstallPreview{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 5*time.Minute)
	defer cancel()
	return a.mgr.PrepareGitHub(ctx, rawURL, ref)
}

func (a *App) PrepareLocal(path string) (model.InstallPreview, error) {
	if err := a.ready(); err != nil {
		return model.InstallPreview{}, err
	}
	return a.mgr.PrepareLocal(path)
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

func (a *App) ApplyInstall(planID string, skills []string, acceptHighRisk bool) (model.Transaction, error) {
	if err := a.ready(); err != nil {
		return model.Transaction{}, err
	}
	return a.mgr.ApplyInstall(planID, skills, acceptHighRisk)
}

// ScanProjectWithCodex is the read-only first phase for assisted installation.
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
) (model.AssistedInstallResult, error) {
	if err := a.ready(); err != nil {
		return model.AssistedInstallResult{}, err
	}
	return a.mgr.ApplyAssistedInstall(
		a.ctx,
		planID,
		skills,
		permissionIDs,
		projectRoot,
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

func (a *App) AuditSkill(name string) (model.ScanReport, error) {
	if err := a.ready(); err != nil {
		return model.ScanReport{}, err
	}
	if strings.TrimSpace(name) == "" {
		return a.mgr.Audit("")
	}
	return a.mgr.AuditSkills([]string{name})
}

func (a *App) AuditSkills(names []string) (model.ScanReport, error) {
	if err := a.ready(); err != nil {
		return model.ScanReport{}, err
	}
	return a.mgr.AuditSkills(names)
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

func (a *App) PrepareUpdate(groupID string) (model.InstallPreview, error) {
	if err := a.ready(); err != nil {
		return model.InstallPreview{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
	defer cancel()
	return a.mgr.PrepareUpdate(ctx, groupID)
}

func (a *App) QuarantineSkills(names []string) (model.Transaction, error) {
	if err := a.ready(); err != nil {
		return model.Transaction{}, err
	}
	return a.mgr.Quarantine(names)
}

func (a *App) RestoreSkill(name, transactionID string) (model.Transaction, error) {
	if err := a.ready(); err != nil {
		return model.Transaction{}, err
	}
	return a.mgr.Restore(name, transactionID)
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

func (a *App) ListQuarantine() ([]QuarantineItem, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	out := make([]QuarantineItem, 0)
	seen := map[string]bool{}
	roots := []string{
		a.mgr.Config.Paths.QuarantineRoot,
		filepath.Join(a.mgr.Config.Paths.SkillsRoot, ".csm-quarantine"),
	}
	for _, root := range roots {
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
				content := filepath.Join(root, skill.Name(), tx.Name(), "content")
				key := strings.ToLower(skill.Name() + "\x00" + tx.Name())
				if _, err := os.Stat(content); err == nil && !seen[key] {
					seen[key] = true
					out = append(out, QuarantineItem{Skill: skill.Name(), TransactionID: tx.Name(), Path: content})
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
	}, nil
}
