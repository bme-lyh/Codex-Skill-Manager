package manager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/codexreview"
	"github.com/bme-lyh/Codex-Skill-Manager/internal/config"
	"github.com/bme-lyh/Codex-Skill-Manager/internal/githubsource"
	"github.com/bme-lyh/Codex-Skill-Manager/internal/inventory"
	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
	"github.com/bme-lyh/Codex-Skill-Manager/internal/provenance"
	"github.com/bme-lyh/Codex-Skill-Manager/internal/reporting"
	"github.com/bme-lyh/Codex-Skill-Manager/internal/scanner"
	"github.com/bme-lyh/Codex-Skill-Manager/internal/state"
)

type Manager struct {
	Config     model.Config
	ConfigPath string
	store      *state.Store
	github     *githubsource.Client
	mu         sync.Mutex
	codexMu    sync.Mutex
	assistMu   sync.Mutex
	assistData sync.Mutex
	previews   map[string]model.InstallPreview
	adoptions  map[string]model.AdoptionPreview
	assisted   map[string]model.AssistedInstallPlan
	progress   map[string]model.AssistedInstallProgress
	cancels    map[string]context.CancelFunc
}

func (m *Manager) configuredRoots() []model.SkillRoot {
	roots := config.Roots(m.Config)
	if len(roots) == 0 && m.Config.Paths.SkillsRoot != "" {
		return []model.SkillRoot{{ID: model.RootIDCodexDefault, Name: "Codex Skills", Kind: "codex", Path: m.Config.Paths.SkillsRoot, Enabled: true, SystemDir: ".system"}}
	}
	return roots
}

func (m *Manager) resolveRoot(rootID string) (model.SkillRoot, error) {
	rootID = strings.TrimSpace(rootID)
	if rootID == "" {
		rootID = m.Config.DefaultRootID
		if rootID == "" {
			rootID = model.DefaultRootID
		}
	}
	for _, root := range m.configuredRoots() {
		if root.ID == rootID {
			if !root.Enabled {
				return model.SkillRoot{}, fmt.Errorf("skill root is disabled: %s", rootID)
			}
			return root, nil
		}
	}
	return model.SkillRoot{}, fmt.Errorf("unknown skill root: %s", rootID)
}

func (m *Manager) resolveWritableRoot(rootID string) (model.SkillRoot, error) {
	root, err := m.resolveRoot(rootID)
	if err != nil {
		return model.SkillRoot{}, err
	}
	if root.ReadOnly {
		return model.SkillRoot{}, fmt.Errorf("skill root is read-only: %s", root.ID)
	}
	return root, nil
}

func (m *Manager) resolveRootID(rootID string) (string, error) {
	root, err := m.resolveRoot(rootID)
	if err != nil {
		return "", err
	}
	return root.ID, nil
}

func (m *Manager) targetWithinRoots(target string) bool {
	for _, root := range m.configuredRoots() {
		if ensureWithinOrEqual(root.Path, target) == nil {
			return true
		}
	}
	return false
}

func Open(configPath string) (*Manager, error) {
	if configPath == "" {
		configPath = strings.TrimSpace(os.Getenv("CSM_CONFIG"))
	}
	if configPath == "" {
		if exe, err := os.Executable(); err == nil {
			exeDir := filepath.Dir(exe)
			if _, err := os.Stat(filepath.Join(exeDir, "portable.marker")); err == nil {
				configPath = filepath.Join(exeDir, "data", "config.yaml")
				if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
					cfg, derr := config.Default()
					if derr != nil {
						return nil, derr
					}
					data := filepath.Join(exeDir, "data")
					cfg.Paths.DataRoot = data
					cfg.Paths.LogsRoot = filepath.Join(data, "logs")
					cfg.Paths.ReportsRoot = filepath.Join(data, "reports")
					cfg.Paths.BackupsRoot = filepath.Join(data, "backups")
					cfg.Paths.QuarantineRoot = filepath.Join(data, "quarantine")
					cfg.Paths.CacheRoot = filepath.Join(data, "cache")
					cfg.Paths.StagingRoot = filepath.Join(data, "staging")
					if err := config.EnsureDirs(cfg); err != nil {
						return nil, err
					}
					if err := config.Save(configPath, cfg); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	if configPath == "" {
		configPath = filepath.Join(cfg.Paths.DataRoot, "config.yaml")
	}
	store, err := state.OpenWithRoots(cfg.Paths.DataRoot, config.Roots(cfg))
	if err != nil {
		return nil, err
	}
	return &Manager{
		Config: cfg, ConfigPath: configPath, store: store,
		github: githubsource.New(), previews: map[string]model.InstallPreview{},
		adoptions: map[string]model.AdoptionPreview{},
		assisted:  map[string]model.AssistedInstallPlan{},
		progress:  map[string]model.AssistedInstallProgress{},
		cancels:   map[string]context.CancelFunc{},
	}, nil
}

func (m *Manager) Close() error { return m.store.Close() }

func (m *Manager) SaveConfig(cfg model.Config) error {
	if err := config.Save(m.ConfigPath, cfg); err != nil {
		return err
	}
	m.Config = cfg
	return config.EnsureDirs(cfg)
}

func (m *Manager) ValidateGitHubCredentials(ctx context.Context) model.GitHubCredentialStatus {
	return m.github.ValidateCredentials(ctx)
}

func (m *Manager) CodexCLIStatus(ctx context.Context) model.CodexCLIStatus {
	return codexreview.Status(ctx, m.Config.CodexReview.CLIPath)
}

func (m *Manager) ReviewScanWithCodex(
	ctx context.Context,
	report model.ScanReport,
	requestedSkills []string,
	progress codexreview.ProgressFunc,
) (model.ScanReport, error) {
	m.codexMu.Lock()
	defer m.codexMu.Unlock()
	m.mu.Lock()
	if !m.Config.CodexReview.Enabled {
		m.mu.Unlock()
		return report, errors.New("Codex 辅助复核未启用；请先在设置中启用")
	}
	persisted, err := m.store.Scan(report.ID)
	if err != nil {
		m.mu.Unlock()
		return report, fmt.Errorf("无法读取可信扫描报告：%w", err)
	}
	ignored, err := m.store.IgnoredFindings()
	if err != nil {
		m.mu.Unlock()
		return report, err
	}
	report = m.decorateScan(persisted, ignored)
	cfg := m.Config.CodexReview
	cfg.OutputLocale = m.Config.Locale
	stagingRoot := m.Config.Paths.StagingRoot
	tx := model.Transaction{
		ID:   "tx-" + time.Now().UTC().Format("20060102T150405.000000000"),
		Type: "codex-risk-review", Status: "running",
		Targets: []string{report.ID}, StartedAt: time.Now().UTC(),
	}
	if err := m.store.SaveTransaction(tx); err != nil {
		m.mu.Unlock()
		return report, fmt.Errorf("start Codex review transaction: %w", err)
	}
	m.mu.Unlock()

	review, err := codexreview.Review(ctx, cfg, report, stagingRoot, requestedSkills, progress)
	report.CodexReview = &review

	m.mu.Lock()
	defer m.mu.Unlock()
	_ = m.store.SaveScan(report)
	m.recordScan(report)
	if err != nil {
		_, failErr := m.fail(tx, err)
		return report, failErr
	}
	tx.Status, tx.CompletedAt = "completed", time.Now().UTC()
	if review.Status == "partial" {
		tx.Status = "partial"
		tx.Error = review.Error
	}
	if err := m.store.SaveTransaction(tx); err != nil {
		_, failErr := m.fail(tx, fmt.Errorf("complete Codex review transaction: %w", err))
		return report, failErr
	}
	m.recordTransaction(tx)
	return report, nil
}

func (m *Manager) History(limit int) ([]model.Transaction, error) {
	return m.store.RecentTransactions(limit)
}

func (m *Manager) Reports(limit int) ([]model.ScanReport, error) {
	reports, err := m.store.RecentScans(limit)
	if err != nil {
		return nil, err
	}
	ignored, err := m.store.IgnoredFindings()
	if err != nil {
		return nil, err
	}
	for i := range reports {
		reports[i] = m.decorateScan(reports[i], ignored)
	}
	return reports, nil
}

// Report returns one persisted report with the same ignored-finding decoration
// used by the dashboard. The optional root guard prevents a report from one
// managed root being opened through another root's view.
func (m *Manager) Report(id string, rootID string) (model.ScanReport, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return model.ScanReport{}, errors.New("report ID is required")
	}
	report, err := m.store.Scan(id)
	if err != nil {
		return model.ScanReport{}, err
	}
	if rootID != "" && report.RootID != "" && report.RootID != rootID {
		return model.ScanReport{}, errors.New("report does not belong to the requested root")
	}
	ignored, err := m.store.IgnoredFindings()
	if err != nil {
		return model.ScanReport{}, err
	}
	return m.decorateScan(report, ignored), nil
}

func (m *Manager) Dashboard() (model.Dashboard, error) {
	lock, err := m.store.LoadLock()
	if err != nil {
		return model.Dashboard{}, err
	}
	skills, sourceGroups, relations, err := inventory.DiscoverRoots(m.configuredRoots(), lock)
	if err != nil {
		return model.Dashboard{}, err
	}
	layout, err := m.store.LoadGroupLayout()
	if err != nil {
		return model.Dashboard{}, err
	}
	skills, groups := applyGroupLayout(skills, sourceGroups, layout)
	securityStates, securityStateErr := m.store.SkillSecurityStates()
	if securityStateErr != nil {
		return model.Dashboard{}, securityStateErr
	}
	applySkillSecurityState(skills, lock, securityStates)
	scans, scanErr := m.store.RecentScans(10)
	if scanErr != nil {
		return model.Dashboard{}, scanErr
	}
	latestScans, latestErr := m.store.LatestScansByTarget()
	if latestErr != nil {
		return model.Dashboard{}, latestErr
	}
	history, historyErr := m.store.RecentTransactions(20)
	if historyErr != nil {
		return model.Dashboard{}, historyErr
	}
	recoverableAssisted, recoveryHistoryErr := m.store.RecoverableAssistedTransactions()
	if recoveryHistoryErr != nil {
		return model.Dashboard{}, recoveryHistoryErr
	}
	recoverableAssisted, recoveryHistoryErr = m.interruptOrphanedAssistedTransactions(recoverableAssisted)
	if recoveryHistoryErr != nil {
		return model.Dashboard{}, recoveryHistoryErr
	}
	history = mergeTransactionHistory(history, recoverableAssisted)
	updateStatuses, updateErr := m.store.LatestUpdateStatuses()
	if updateErr != nil {
		return model.Dashboard{}, updateErr
	}
	if skills == nil {
		skills = []model.Skill{}
	}
	if groups == nil {
		groups = []model.Group{}
	}
	if sourceGroups == nil {
		sourceGroups = []model.Group{}
	}
	if relations == nil {
		relations = []model.Relation{}
	}
	if scans == nil {
		scans = []model.ScanReport{}
	}
	if history == nil {
		history = []model.Transaction{}
	}
	if updateStatuses == nil {
		updateStatuses = []model.UpdateStatus{}
	}
	validSourceGroups := map[string]bool{}
	for _, group := range sourceGroups {
		validSourceGroups[group.ID] = true
		validSourceGroups[group.RootID+"\x00"+group.ID] = true
	}
	filteredUpdateStatuses := updateStatuses[:0]
	for _, status := range updateStatuses {
		if validSourceGroups[status.GroupID] || validSourceGroups[status.RootID+"\x00"+status.GroupID] {
			filteredUpdateStatuses = append(filteredUpdateStatuses, status)
		}
	}
	updateStatuses = filteredUpdateStatuses
	ignored, ignoreErr := m.store.IgnoredFindings()
	if ignoreErr != nil {
		return model.Dashboard{}, ignoreErr
	}
	for i := range scans {
		scans[i] = m.decorateScan(scans[i], ignored)
	}
	for i := range latestScans {
		latestScans[i] = m.decorateScan(latestScans[i], ignored)
	}
	d := model.Dashboard{
		Skills: skills, Groups: groups, SourceGroups: sourceGroups, Relations: relations,
		RecentReports: scans, RecentHistory: history, UpdateStatuses: updateStatuses,
		Roots: append([]model.SkillRoot(nil), m.configuredRoots()...), DefaultRootID: m.Config.DefaultRootID,
	}
	statusByGroup := map[string]model.UpdateStatus{}
	ambiguousStatusGroups := map[string]bool{}
	for _, status := range updateStatuses {
		if status.RootID == "" {
			statusByGroup[status.GroupID] = status
		} else {
			statusByGroup[model.QualifiedPackageID(status.RootID, status.GroupID)] = status
			if _, exists := statusByGroup[status.GroupID]; exists {
				ambiguousStatusGroups[status.GroupID] = true
				delete(statusByGroup, status.GroupID)
			} else if !ambiguousStatusGroups[status.GroupID] {
				statusByGroup[status.GroupID] = status
			}
		}
		if d.LastUpdateCheck == nil || status.CheckedAt.After(*d.LastUpdateCheck) {
			checkedAt := status.CheckedAt
			d.LastUpdateCheck = &checkedAt
		}
		if status.Status == "update-available" ||
			((status.Status == "error" || status.Status == "rate-limited") && status.LastSuccessStatus == "update-available") {
			d.UpdateCount++
		}
	}
	for i := range d.Skills {
		status, ok := statusByGroup[model.QualifiedPackageID(d.Skills[i].RootID, d.Skills[i].SourceGroupID)]
		if !ok {
			status, ok = statusByGroup[d.Skills[i].SourceGroupID]
		}
		if !ok {
			continue
		}
		d.Skills[i].LastChecked = &status.CheckedAt
		d.Skills[i].UpdateStatus = status.Status
		if status.Status == "update-available" {
			d.Skills[i].UpdateStatus = "up-to-date"
		}
		for _, name := range status.OutdatedSkills {
			if name == d.Skills[i].Name || name == d.Skills[i].Identity {
				d.Skills[i].UpdateStatus = "update-available"
				break
			}
		}
	}
	for _, skill := range skills {
		switch {
		case skill.System:
			d.SystemCount++
		case skill.Managed:
			d.ManagedCount++
		default:
			d.UnmanagedCount++
		}
	}
	activeRisks := map[string]bool{}
	reportsByID := map[string]model.ScanReport{}
	for _, state := range securityStates {
		skillIdentity := model.QualifiedSkillIdentity(state.RootID, state.SkillName)
		report, ok := reportsByID[state.ReportID]
		if !ok {
			report, err = m.store.Scan(state.ReportID)
			if err != nil {
				continue
			}
			report = m.decorateScan(report, ignored)
			reportsByID[state.ReportID] = report
		}
		for _, cluster := range report.Clusters {
			if report.RootID != "" && state.RootID != "" && report.RootID != state.RootID {
				continue
			}
			if cluster.SkillName != "" && !strings.EqualFold(cluster.SkillName, state.SkillName) {
				continue
			}
			if !cluster.Ignored && (cluster.Severity == model.RiskHigh || cluster.Severity == model.RiskCritical) {
				activeRisks[skillIdentity+"\x00"+cluster.ID] = true
			}
		}
	}
	for _, report := range latestScans {
		if !m.targetWithinRoots(report.Target) {
			continue
		}
		if len(report.Skills) > 0 {
			continue
		}
		for _, cluster := range report.Clusters {
			if !cluster.Ignored && (cluster.Severity == model.RiskHigh || cluster.Severity == model.RiskCritical) {
				activeRisks[report.Target+"\x00"+cluster.ID] = true
			}
		}
	}
	d.RiskCount = len(activeRisks)
	return d, nil
}

func mergeTransactionHistory(recent, required []model.Transaction) []model.Transaction {
	seen := make(map[string]bool, len(recent)+len(required))
	merged := make([]model.Transaction, 0, len(recent)+len(required))
	for _, tx := range append(required, recent...) {
		if strings.TrimSpace(tx.ID) == "" || seen[tx.ID] {
			continue
		}
		seen[tx.ID] = true
		merged = append(merged, tx)
	}
	sort.SliceStable(merged, func(i, j int) bool {
		return merged[i].StartedAt.After(merged[j].StartedAt)
	})
	return merged
}

func applyGroupLayout(skills []model.Skill, sourceGroups []model.Group, layout model.GroupLayoutState) ([]model.Skill, []model.Group) {
	preferences := map[string]model.GroupPreference{}
	for _, preference := range layout.Groups {
		preferences[layoutKey(preference.RootID, preference.ID)] = preference
		preferences[preference.ID] = preference // v1 compatibility
	}
	assignments := map[string]model.SkillGroupAssignment{}
	for _, assignment := range layout.Assignments {
		assignments[layoutKey(assignment.RootID, assignment.SkillName)] = assignment
		assignments[assignment.SkillName] = assignment
	}
	groups := map[string]*model.Group{}
	defaultPosition := 0
	for _, source := range sourceGroups {
		group := source
		group.SkillNames = nil
		if preference, ok := preferences[layoutKey(group.RootID, group.ID)]; ok {
			if strings.TrimSpace(preference.Name) != "" {
				group.Name = preference.Name
			}
			group.Position = preference.Position
		} else if preference, ok := preferences[group.ID]; ok {
			if strings.TrimSpace(preference.Name) != "" {
				group.Name = preference.Name
			}
			group.Position = preference.Position
		} else {
			group.Position = defaultPosition
		}
		defaultPosition++
		groups[layoutKey(group.RootID, group.ID)] = &group
	}
	for _, preference := range layout.Groups {
		if !preference.Manual {
			continue
		}
		if preference.RootID == "" {
			preference.RootID = model.RootIDCodexDefault
		}
		if _, exists := groups[layoutKey(preference.RootID, preference.ID)]; exists {
			continue
		}
		group := model.Group{
			ID: preference.ID, RootID: preference.RootID, Name: preference.Name, Provider: "manual",
			Manual: true, Position: preference.Position, Status: "healthy",
		}
		groups[layoutKey(group.RootID, group.ID)] = &group
	}
	for i := range skills {
		skill := &skills[i]
		if skill.SourceGroupID == "" {
			skill.SourceGroupID, skill.SourceGroupName = skill.GroupID, skill.GroupName
		}
		targetID := skill.SourceGroupID
		assignment, assigned := assignments[layoutKey(skill.RootID, skill.Name)]
		if !assigned {
			assignment, assigned = assignments[skill.Name]
		}
		if assigned && !skill.System {
			if _, exists := groups[layoutKey(skill.RootID, assignment.GroupID)]; exists {
				targetID = assignment.GroupID
			}
		}
		group, exists := groups[layoutKey(skill.RootID, targetID)]
		if !exists {
			group, exists = groups[targetID]
		}
		if !exists {
			continue
		}
		skill.GroupID, skill.GroupName = group.ID, group.Name
		group.SkillNames = append(group.SkillNames, skill.Name)
	}
	out := make([]model.Group, 0, len(groups))
	for _, group := range groups {
		sort.Strings(group.SkillNames)
		out = append(out, *group)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ID == "system" {
			return true
		}
		if out[j].ID == "system" {
			return false
		}
		if out[i].Position == out[j].Position {
			return out[i].Name < out[j].Name
		}
		return out[i].Position < out[j].Position
	})
	return skills, out
}

func layoutKey(rootID, value string) string {
	return rootID + "\x00" + value
}

func (m *Manager) CreateGroup(name string, targetRootID ...string) (model.Transaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rootID := ""
	if len(targetRootID) > 0 {
		rootID = targetRootID[0]
	}
	root, err := m.resolveWritableRoot(rootID)
	if err != nil {
		return model.Transaction{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return model.Transaction{}, errors.New("group name is required")
	}
	dashboard, err := m.Dashboard()
	if err != nil {
		return model.Transaction{}, err
	}
	for _, group := range dashboard.Groups {
		if group.RootID == root.ID && strings.EqualFold(group.Name, name) {
			return model.Transaction{}, fmt.Errorf("group name already exists: %s", name)
		}
	}
	layout, err := m.store.LoadGroupLayout()
	if err != nil {
		return model.Transaction{}, err
	}
	id := "group-" + time.Now().UTC().Format("20060102T150405.000000000")
	layout.Groups = append(layout.Groups, model.GroupPreference{ID: id, RootID: root.ID, Name: name, Position: len(dashboard.Groups), Manual: true})
	return m.applyGroupLayoutChange("group-create", root.ID, []string{id}, layout)
}

func (m *Manager) RenameGroup(id, name string, targetRootID ...string) (model.Transaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rootID := ""
	if len(targetRootID) > 0 {
		rootID = targetRootID[0]
	}
	root, err := m.resolveWritableRoot(rootID)
	if err != nil {
		return model.Transaction{}, err
	}
	id, name = strings.TrimSpace(id), strings.TrimSpace(name)
	if id == "" || name == "" {
		return model.Transaction{}, errors.New("explicit group ID and name are required")
	}
	dashboard, err := m.Dashboard()
	if err != nil {
		return model.Transaction{}, err
	}
	var target *model.Group
	for i := range dashboard.Groups {
		group := &dashboard.Groups[i]
		if group.RootID != root.ID {
			continue
		}
		if strings.EqualFold(group.Name, name) && group.ID != id {
			return model.Transaction{}, fmt.Errorf("group name already exists: %s", name)
		}
		if group.ID == id {
			target = group
		}
	}
	if target == nil {
		return model.Transaction{}, fmt.Errorf("group not found: %s", id)
	}
	if target.ReadOnly {
		return model.Transaction{}, errors.New("system group cannot be renamed")
	}
	layout, err := m.store.LoadGroupLayout()
	if err != nil {
		return model.Transaction{}, err
	}
	found := false
	for i := range layout.Groups {
		preferenceRoot := layout.Groups[i].RootID
		if preferenceRoot == "" {
			preferenceRoot = model.RootIDCodexDefault
		}
		if layout.Groups[i].ID == id && preferenceRoot == root.ID {
			layout.Groups[i].RootID = root.ID
			layout.Groups[i].Name = name
			found = true
			break
		}
	}
	if !found {
		layout.Groups = append(layout.Groups, model.GroupPreference{
			ID: id, RootID: root.ID, Name: name, Position: target.Position, Manual: target.Manual,
		})
	}
	return m.applyGroupLayoutChange("group-rename", root.ID, []string{id}, layout)
}

func (m *Manager) ReorderGroups(ids []string, targetRootID ...string) (model.Transaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rootID := ""
	if len(targetRootID) > 0 {
		rootID = targetRootID[0]
	}
	root, err := m.resolveWritableRoot(rootID)
	if err != nil {
		return model.Transaction{}, err
	}
	dashboard, err := m.Dashboard()
	if err != nil {
		return model.Transaction{}, err
	}
	expected := map[string]model.Group{}
	for _, group := range dashboard.Groups {
		if group.RootID == root.ID && !group.ReadOnly {
			expected[group.ID] = group
		}
	}
	if len(ids) != len(expected) {
		return model.Transaction{}, errors.New("group order must contain every editable group exactly once")
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if _, ok := expected[id]; !ok || seen[id] {
			return model.Transaction{}, fmt.Errorf("invalid or duplicate group ID: %s", id)
		}
		seen[id] = true
	}
	layout, err := m.store.LoadGroupLayout()
	if err != nil {
		return model.Transaction{}, err
	}
	byID := map[string]model.GroupPreference{}
	untouched := make([]model.GroupPreference, 0, len(layout.Groups))
	for _, preference := range layout.Groups {
		preferenceRoot := preference.RootID
		if preferenceRoot == "" {
			preferenceRoot = model.RootIDCodexDefault
		}
		if preferenceRoot == root.ID {
			preference.RootID = root.ID
			byID[preference.ID] = preference
		} else {
			untouched = append(untouched, preference)
		}
	}
	for position, id := range ids {
		preference, exists := byID[id]
		if !exists {
			group := expected[id]
			preference = model.GroupPreference{ID: id, RootID: root.ID, Name: group.Name, Manual: group.Manual}
		}
		preference.Position = position + 1
		byID[id] = preference
	}
	layout.Groups = append(layout.Groups[:0], untouched...)
	for _, preference := range byID {
		layout.Groups = append(layout.Groups, preference)
	}
	sort.Slice(layout.Groups, func(i, j int) bool { return layout.Groups[i].Position < layout.Groups[j].Position })
	return m.applyGroupLayoutChange("group-reorder", root.ID, append([]string(nil), ids...), layout)
}

func (m *Manager) MoveSkillsToGroup(names []string, groupID string, targetRootID ...string) (model.Transaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rootID := ""
	if len(targetRootID) > 0 {
		rootID = targetRootID[0]
	}
	root, err := m.resolveWritableRoot(rootID)
	if err != nil {
		return model.Transaction{}, err
	}
	groupID = strings.TrimSpace(groupID)
	if len(names) == 0 || groupID == "" {
		return model.Transaction{}, errors.New("explicit skills and target group are required")
	}
	dashboard, err := m.Dashboard()
	if err != nil {
		return model.Transaction{}, err
	}
	validGroup := false
	for _, group := range dashboard.Groups {
		if group.RootID == root.ID && group.ID == groupID {
			if group.ReadOnly {
				return model.Transaction{}, errors.New("skills cannot be moved into a system group")
			}
			validGroup = true
		}
	}
	if !validGroup {
		return model.Transaction{}, fmt.Errorf("group not found: %s", groupID)
	}
	skills := map[string]model.Skill{}
	for _, skill := range dashboard.Skills {
		if skill.RootID == root.ID {
			skills[skill.Name] = skill
		}
	}
	unique := make([]string, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		if !validMutableSkillName(name) {
			return model.Transaction{}, fmt.Errorf("invalid skill name: %s", name)
		}
		skill, ok := skills[name]
		if !ok {
			return model.Transaction{}, fmt.Errorf("skill not found: %s", name)
		}
		if skill.System {
			return model.Transaction{}, fmt.Errorf("system skill cannot be moved: %s", name)
		}
		if !seen[name] {
			seen[name] = true
			unique = append(unique, name)
		}
	}
	sort.Strings(unique)
	layout, err := m.store.LoadGroupLayout()
	if err != nil {
		return model.Transaction{}, err
	}
	filtered := layout.Assignments[:0]
	for _, assignment := range layout.Assignments {
		assignmentRoot := assignment.RootID
		if assignmentRoot == "" {
			assignmentRoot = model.RootIDCodexDefault
		}
		if assignmentRoot != root.ID || !seen[assignment.SkillName] {
			filtered = append(filtered, assignment)
		}
	}
	layout.Assignments = filtered
	for position, name := range unique {
		layout.Assignments = append(layout.Assignments, model.SkillGroupAssignment{
			SkillName: name, RootID: root.ID, GroupID: groupID, Position: position,
		})
	}
	return m.applyGroupLayoutChange("group-move", root.ID, unique, layout)
}

func (m *Manager) applyGroupLayoutChange(kind, rootID string, targets []string, layout model.GroupLayoutState) (model.Transaction, error) {
	tx := model.Transaction{
		ID:     "tx-" + time.Now().UTC().Format("20060102T150405.000000000"),
		RootID: rootID, Type: kind, Status: "running", Targets: targets, StartedAt: time.Now().UTC(),
	}
	if err := m.store.SaveTransaction(tx); err != nil {
		return model.Transaction{}, fmt.Errorf("start group layout transaction: %w", err)
	}
	snapshot, err := m.snapshotGroupLayout(tx.ID)
	if err != nil {
		return m.fail(tx, err)
	}
	tx.BackupPaths = append(tx.BackupPaths, snapshot)
	if err := m.store.ReplaceGroupLayout(layout); err != nil {
		return m.fail(tx, err)
	}
	tx.Status, tx.CompletedAt = "completed", time.Now().UTC()
	if err := m.store.SaveTransaction(tx); err != nil {
		tx.RecoveryStatus = "required"
		return m.fail(tx, fmt.Errorf("complete group layout transaction: %w", err))
	}
	m.recordTransaction(tx)
	return tx, nil
}

func (m *Manager) Audit(target string, requestedRootID ...string) (model.ScanReport, error) {
	rootID := ""
	if len(requestedRootID) > 0 {
		rootID = requestedRootID[0]
	}
	root, err := m.resolveRoot(rootID)
	if err != nil {
		return model.ScanReport{}, err
	}
	if target == "" {
		target = root.Path
	}
	if err := ensureWithinOrEqual(root.Path, target); err != nil {
		return model.ScanReport{}, err
	}
	scan := scanner.Scan
	if strings.EqualFold(filepath.Clean(target), filepath.Clean(root.Path)) {
		scan = scanner.ScanSkillsRoot
	}
	report, err := scan(target, m.Config.MaxFiles, m.Config.MaxFileBytes)
	report.RootID = root.ID
	if err != nil {
		report.Status = "failed"
	}
	ignored, ignoreErr := m.store.IgnoredFindings()
	if ignoreErr != nil {
		return model.ScanReport{}, ignoreErr
	}
	report = m.decorateScan(report, ignored)
	summarizeScanSkills(&report)
	_ = m.store.SaveScan(report)
	m.recordScan(report)
	return report, err
}

func (m *Manager) AuditSkills(names []string, requestedRootID ...string) (model.ScanReport, error) {
	names = uniqueNonEmpty(names)
	if len(names) == 0 {
		return model.ScanReport{}, errors.New("at least one explicit Skill is required")
	}
	rootID := ""
	if len(requestedRootID) > 0 {
		rootID = strings.TrimSpace(requestedRootID[0])
	}
	lock, err := m.store.LoadLock()
	if err != nil {
		return model.ScanReport{}, err
	}
	skills, sourceGroups, _, err := inventory.DiscoverRoots(m.configuredRoots(), lock)
	if err != nil {
		return model.ScanReport{}, err
	}
	layout, err := m.store.LoadGroupLayout()
	if err != nil {
		return model.ScanReport{}, err
	}
	skills, _ = applyGroupLayout(skills, sourceGroups, layout)
	byName := make(map[string][]model.Skill, len(skills))
	for _, skill := range skills {
		if !skill.System && (rootID == "" || skill.RootID == rootID) {
			byName[skill.Name] = append(byName[skill.Name], skill)
		}
	}
	selected := make([]model.Skill, 0, len(names))
	selectionErrors := make([]string, 0)
	for _, name := range names {
		matches := byName[name]
		if len(matches) == 0 {
			selectionErrors = append(selectionErrors, fmt.Sprintf("%s: Skill not found or not selectable", name))
			continue
		}
		if len(matches) > 1 {
			selectionErrors = append(selectionErrors, fmt.Sprintf("%s: Skill exists in multiple roots; specify rootId", name))
			continue
		}
		selected = append(selected, matches[0])
	}
	if len(selected) == 0 {
		return model.ScanReport{}, errors.New(strings.Join(selectionErrors, "; "))
	}
	if rootID == "" {
		rootID = selected[0].RootID
	}
	for _, skill := range selected {
		if skill.RootID != rootID {
			return model.ScanReport{}, errors.New("selected Skills span multiple roots; scan one root at a time")
		}
	}
	root, err := m.resolveRoot(rootID)
	if err != nil {
		return model.ScanReport{}, err
	}
	sort.Slice(selected, func(i, j int) bool {
		if selected[i].GroupName == selected[j].GroupName {
			return selected[i].Name < selected[j].Name
		}
		return selected[i].GroupName < selected[j].GroupName
	})

	now := time.Now().UTC()
	report := model.ScanReport{
		ID: "scan-" + now.Format("20060102T150405.000000000"), RootID: root.ID, Target: root.Path,
		StartedAt: now, HighestSeverity: model.RiskInfo, ActiveHighestSeverity: model.RiskInfo,
		Findings: []model.Finding{}, Skills: []model.ScanSkillSummary{},
		Status: "passed", ScannerVersion: scanner.Version,
	}
	if len(selectionErrors) > 0 {
		report.Status = "partial"
		report.Error = strings.Join(selectionErrors, "; ")
	}
	completedSkills := make([]model.Skill, 0, len(selected))
	partialErrors := append([]string(nil), selectionErrors...)
	remaining := m.Config.MaxFiles
	for _, skill := range selected {
		part, scanErr := scanner.Scan(skill.Path, remaining, m.Config.MaxFileBytes)
		if scanErr != nil {
			report.Status = "failed"
			partialErrors = append(partialErrors, fmt.Sprintf("%s: %v", skill.Name, scanErr))
			report.Skills = append(report.Skills, model.ScanSkillSummary{
				SkillName: skill.Name, RootID: skill.RootID, SourcePath: filepath.ToSlash(skill.Name),
				GroupID: skill.GroupID, GroupName: skill.GroupName, Error: scanErr.Error(),
			})
			continue
		}
		relative, relErr := filepath.Rel(root.Path, skill.Path)
		if relErr != nil || relative == "." || strings.HasPrefix(relative, "..") || filepath.IsAbs(relative) {
			report.Status = "partial"
			message := fmt.Sprintf("%s: invalid Skill path", skill.Name)
			partialErrors = append(partialErrors, message)
			report.Skills = append(report.Skills, model.ScanSkillSummary{
				SkillName: skill.Name, RootID: skill.RootID, SourcePath: filepath.ToSlash(skill.Path),
				GroupID: skill.GroupID, GroupName: skill.GroupName, Error: message,
			})
			continue
		}
		sourcePath := filepath.ToSlash(relative)
		for _, finding := range part.Findings {
			finding.File = filepath.ToSlash(filepath.Join(sourcePath, filepath.FromSlash(finding.File)))
			finding.SkillName, finding.RootID = skill.Name, skill.RootID
			finding.GroupID, finding.GroupName = skill.GroupID, skill.GroupName
			report.Findings = append(report.Findings, finding)
		}
		report.FilesScanned += part.FilesScanned
		report.Skills = append(report.Skills, model.ScanSkillSummary{
			SkillName: skill.Name, RootID: skill.RootID, SourcePath: sourcePath, GroupID: skill.GroupID,
			GroupName: skill.GroupName, FilesScanned: part.FilesScanned,
			HighestSeverity: highestFindingSeverity(part.Findings, false),
		})
		completedSkills = append(completedSkills, skill)
		remaining = m.Config.MaxFiles - report.FilesScanned
		if remaining < 0 {
			partialErrors = append(partialErrors, fmt.Sprintf("%s: selected Skills exceed file count limit %d", skill.Name, m.Config.MaxFiles))
			report.Status = "partial"
			break
		}
	}
	finishCandidateScan(&report)
	if len(partialErrors) > 0 {
		report.Status = "partial"
		report.Error = strings.Join(partialErrors, "; ")
	}
	ignored, err := m.store.IgnoredFindings()
	if err != nil {
		return model.ScanReport{}, err
	}
	report = m.decorateScan(report, ignored)
	summarizeScanSkills(&report)
	if err := m.store.SaveScan(report); err != nil {
		return model.ScanReport{}, err
	}
	states := make([]model.SkillSecurityState, 0, len(completedSkills))
	for _, skill := range completedSkills {
		states = append(states, model.SkillSecurityState{
			SkillName: skill.Name, RootID: skill.RootID, ContentHash: skillContentHash(skill.Files),
			ReportID: report.ID, CheckedAt: report.CompletedAt,
		})
	}
	if err := m.store.SaveSkillSecurityStates(states); err != nil {
		return model.ScanReport{}, err
	}
	m.recordScan(report)
	if len(completedSkills) == 0 {
		return report, errors.New(report.Error)
	}
	return report, nil
}

func (m *Manager) SetFindingIgnored(finding model.Finding, ignored bool, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if finding.Fingerprint == "" || finding.RuleID == "" || finding.File == "" {
		return errors.New("finding identity is incomplete")
	}
	if decoded, err := hex.DecodeString(finding.Fingerprint); err != nil || len(decoded) != sha256.Size {
		return errors.New("invalid finding fingerprint")
	}
	persisted, err := m.persistedFinding(finding)
	if err != nil {
		return err
	}
	finding = persisted
	reason = strings.TrimSpace(reason)
	if !validRiskSeverity(finding.Severity) {
		return errors.New("finding has an unknown severity and cannot be modified")
	}
	if ignored && (finding.Severity == model.RiskCritical || finding.Severity == model.RiskHigh) {
		return errors.New("High and Critical findings require the audited risk-cluster decision workflow")
	}
	txType := "ignore-warning"
	if !ignored {
		txType = "restore-warning"
	}
	tx := model.Transaction{
		ID: "tx-" + time.Now().UTC().Format("20060102T150405.000000000"), Type: txType,
		Status: "running", Targets: []string{finding.Fingerprint}, StartedAt: time.Now().UTC(),
	}
	if err := m.store.SaveTransaction(tx); err != nil {
		return fmt.Errorf("start warning decision transaction: %w", err)
	}
	if err := m.store.SetFindingIgnored(finding, ignored, reason); err != nil {
		_, failErr := m.fail(tx, err)
		return failErr
	}
	tx.Status, tx.CompletedAt = "completed", time.Now().UTC()
	if err := m.store.SaveTransaction(tx); err != nil {
		return fmt.Errorf("complete warning decision transaction: %w", err)
	}
	m.recordTransaction(tx)
	return nil
}

func (m *Manager) SetRiskClusterIgnored(cluster model.RiskCluster, ignored bool, reason string, confirmDeterministic bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.setRiskClustersIgnoredLocked([]model.RiskCluster{cluster}, ignored, reason, confirmDeterministic)
}

func (m *Manager) SetRiskClustersIgnored(clusters []model.RiskCluster, ignored bool, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.setRiskClustersIgnoredLocked(clusters, ignored, reason, false)
}

func (m *Manager) setRiskClustersIgnoredLocked(clusters []model.RiskCluster, ignored bool, reason string, confirmHighRisk bool) error {
	if len(clusters) == 0 {
		return errors.New("at least one risk cluster is required")
	}
	reason = strings.TrimSpace(reason)
	resolved := make([]model.RiskCluster, 0, len(clusters))
	for _, requested := range clusters {
		cluster, err := m.persistedRiskCluster(requested)
		if err != nil {
			return err
		}
		if !validRiskSeverity(cluster.Severity) {
			return errors.New("risk cluster has an unknown severity and cannot be modified")
		}
		if ignored && cluster.Severity == model.RiskCritical {
			return errors.New("Critical risk clusters cannot be ignored")
		}
		if ignored && cluster.Severity == model.RiskHigh && (!confirmHighRisk || reason == "") {
			return errors.New("High risk acceptance requires explicit confirmation and a non-empty reason")
		}
		resolved = append(resolved, cluster)
	}
	clusters = resolved
	targets := make([]string, 0, len(clusters))
	for index := range clusters {
		cluster := &clusters[index]
		if cluster.ID == "" || cluster.RuleID == "" || len(cluster.Fingerprints) == 0 {
			return errors.New("risk cluster identity is incomplete")
		}
		for _, fingerprint := range cluster.Fingerprints {
			if decoded, err := hex.DecodeString(fingerprint); err != nil || len(decoded) != sha256.Size {
				return errors.New("risk cluster contains an invalid finding fingerprint")
			}
		}
		cluster.Deterministic = cluster.Deterministic || cluster.RuleID == "CSM-FS-001" ||
			cluster.RuleID == "CSM-FILE-002" || cluster.RuleID == "CSM-DEL-001"
		targets = append(targets, cluster.ID)
	}
	txType := "ignore-risk-cluster"
	if !ignored {
		txType = "restore-risk-cluster"
	}
	if len(clusters) > 1 {
		txType += "-batch"
	}
	tx := model.Transaction{
		ID: "tx-" + time.Now().UTC().Format("20060102T150405.000000000"), Type: txType,
		Status: "running", Targets: targets, StartedAt: time.Now().UTC(),
	}
	if err := m.store.SaveTransaction(tx); err != nil {
		return fmt.Errorf("start risk decision transaction: %w", err)
	}
	if err := m.store.SetClustersIgnored(clusters, ignored, reason); err != nil {
		_, failErr := m.fail(tx, err)
		return failErr
	}
	tx.Status, tx.CompletedAt = "completed", time.Now().UTC()
	if err := m.store.SaveTransaction(tx); err != nil {
		return fmt.Errorf("complete risk decision transaction: %w", err)
	}
	m.recordTransaction(tx)
	return nil
}

func (m *Manager) persistedFinding(requested model.Finding) (model.Finding, error) {
	reports, err := m.store.RecentScans(100)
	if err != nil {
		return model.Finding{}, err
	}
	for _, report := range reports {
		for _, finding := range report.Findings {
			if finding.Fingerprint != requested.Fingerprint {
				continue
			}
			if requested.RuleID != "" && finding.RuleID != requested.RuleID {
				return model.Finding{}, errors.New("finding identity does not match persisted scan evidence")
			}
			if requested.File != "" && filepath.ToSlash(finding.File) != filepath.ToSlash(requested.File) {
				return model.Finding{}, errors.New("finding identity does not match persisted scan evidence")
			}
			return finding, nil
		}
	}
	return model.Finding{}, errors.New("finding was not found in persisted scan evidence")
}

func (m *Manager) persistedRiskCluster(requested model.RiskCluster) (model.RiskCluster, error) {
	if strings.TrimSpace(requested.ID) == "" {
		return model.RiskCluster{}, errors.New("risk cluster ID is required")
	}
	reports, err := m.store.RecentScans(100)
	if err != nil {
		return model.RiskCluster{}, err
	}
	for _, report := range reports {
		for _, cluster := range report.Clusters {
			if cluster.ID != requested.ID {
				continue
			}
			if requested.RuleID != "" && cluster.RuleID != requested.RuleID {
				return model.RiskCluster{}, errors.New("risk cluster identity does not match persisted scan evidence")
			}
			return cluster, nil
		}
	}
	return model.RiskCluster{}, errors.New("risk cluster was not found in persisted scan evidence")
}

func (m *Manager) PrepareGitHub(ctx context.Context, rawURL, ref string, targetRootID ...string) (model.InstallPreview, error) {
	rootID := ""
	if len(targetRootID) > 0 {
		rootID = targetRootID[0]
	}
	return m.prepareGitHub(ctx, rawURL, ref, nil, rootID)
}

func (m *Manager) PrepareGitHubForRoot(ctx context.Context, rawURL, ref, targetRootID string) (model.InstallPreview, error) {
	return m.PrepareGitHub(ctx, rawURL, ref, targetRootID)
}

// prepareGitHub resolves and downloads an immutable repository snapshot, then
// scans only the Skill directories that can be selected by the resulting plan.
// Repository-level files are not installation targets and must not affect the
// plan's risk decision.
func (m *Manager) prepareGitHub(ctx context.Context, rawURL, ref string, selected map[string]bool, targetRootID string) (model.InstallPreview, error) {
	targetRoot, err := m.resolveWritableRoot(targetRootID)
	if err != nil {
		return model.InstallPreview{}, err
	}
	repo, err := m.github.Resolve(ctx, rawURL, ref)
	if err != nil {
		return model.InstallPreview{}, err
	}
	if !immutableGitHubSHA.MatchString(repo.CommitSHA) {
		return model.InstallPreview{}, errors.New("GitHub source did not resolve to a full immutable commit SHA")
	}
	id := "plan-" + time.Now().UTC().Format("20060102T150405.000000000")
	stage := filepath.Join(m.Config.Paths.StagingRoot, id)
	sourceRoot, err := m.github.Download(ctx, repo, stage, m.Config.MaxFileBytes)
	if err != nil {
		return model.InstallPreview{}, err
	}
	candidates, err := githubsource.Discover(sourceRoot, repo.SourcePath)
	if err != nil {
		var conflict *githubsource.SkillNameConflictError
		if errors.As(err, &conflict) && conflict.SuggestedSourcePath != "" {
			pathParts := strings.Split(filepath.ToSlash(conflict.SuggestedSourcePath), "/")
			for index := range pathParts {
				pathParts[index] = url.PathEscape(pathParts[index])
			}
			suggestedURL := fmt.Sprintf(
				"https://github.com/%s/tree/%s/%s",
				repo.FullName,
				repo.CommitSHA,
				strings.Join(pathParts, "/"),
			)
			return model.InstallPreview{}, fmt.Errorf(
				"%w; suggested Codex source URL: %s",
				err,
				suggestedURL,
			)
		}
		return model.InstallPreview{}, err
	}
	if len(candidates) == 0 {
		return model.InstallPreview{}, errors.New("no valid SKILL.md directories found")
	}
	if selected != nil {
		filtered := make([]model.CandidateSkill, 0, len(selected))
		for _, candidate := range candidates {
			if selected[candidate.Name] {
				filtered = append(filtered, candidate)
			}
		}
		candidates = filtered
		if len(candidates) == 0 {
			return model.InstallPreview{}, errors.New("the updated repository no longer contains any installed Skills")
		}
	}
	report, scanErr := scanCandidateSkills(sourceRoot, candidates, m.Config.MaxFiles, m.Config.MaxFileBytes)
	report.RootID = targetRoot.ID
	annotateCandidateScan(&report, candidates, "github:"+strings.ToLower(repo.FullName), repo.FullName)
	ignored, ignoreErr := m.store.IgnoredFindings()
	if ignoreErr != nil {
		return model.InstallPreview{}, ignoreErr
	}
	report = m.decorateScan(report, ignored)
	summarizeScanSkills(&report)
	_ = m.store.SaveScan(report)
	m.recordScan(report)
	if scanErr != nil {
		return model.InstallPreview{}, scanErr
	}
	preview := model.InstallPreview{
		ID: id, TargetRootID: targetRoot.ID, Repository: repo, Skills: candidates, Scan: report,
		StagingPath: sourceRoot, CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
	}
	if err := sealInstallPreview(&preview); err != nil {
		return model.InstallPreview{}, err
	}
	if err := savePreview(m.Config.Paths.DataRoot, preview); err != nil {
		return model.InstallPreview{}, err
	}
	m.mu.Lock()
	m.previews[id] = preview
	m.mu.Unlock()
	return preview, nil
}

func (m *Manager) PrepareLocal(path string, targetRootID ...string) (model.InstallPreview, error) {
	rootID := ""
	if len(targetRootID) > 0 {
		rootID = targetRootID[0]
	}
	root, err := m.resolveWritableRoot(rootID)
	if err != nil {
		return model.InstallPreview{}, err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return model.InstallPreview{}, err
	}
	linkInfo, err := os.Lstat(abs)
	if err != nil {
		return model.InstallPreview{}, err
	}
	if linkInfo.Mode()&os.ModeSymlink != 0 {
		return model.InstallPreview{}, errors.New("local source root cannot be a link or reparse point")
	}
	info, err := os.Stat(abs)
	if err != nil {
		return model.InstallPreview{}, err
	}
	if !info.IsDir() {
		return model.InstallPreview{}, errors.New("local source must be a directory")
	}
	id := "plan-" + time.Now().UTC().Format("20060102T150405.000000000")
	stage := filepath.Join(m.Config.Paths.StagingRoot, id)
	if err := snapshotLocalSource(abs, stage, m.Config.MaxFiles, m.Config.MaxFileBytes); err != nil {
		return model.InstallPreview{}, fmt.Errorf("snapshot local source: %w", err)
	}
	candidates, err := githubsource.Discover(stage, "")
	if err != nil {
		return model.InstallPreview{}, err
	}
	if len(candidates) == 0 {
		return model.InstallPreview{}, errors.New("no valid SKILL.md directories found")
	}
	report, err := scanCandidateSkills(stage, candidates, m.Config.MaxFiles, m.Config.MaxFileBytes)
	if err != nil {
		return model.InstallPreview{}, err
	}
	report.RootID = root.ID
	annotateCandidateScan(
		&report, candidates, "local:"+strings.ToLower(filepath.Clean(abs)), filepath.Base(abs),
	)
	ignored, ignoreErr := m.store.IgnoredFindings()
	if ignoreErr != nil {
		return model.InstallPreview{}, ignoreErr
	}
	report = m.decorateScan(report, ignored)
	summarizeScanSkills(&report)
	_ = m.store.SaveScan(report)
	m.recordScan(report)
	preview := model.InstallPreview{
		ID: id, TargetRootID: root.ID,
		Repository: model.Repository{
			Provider: "local", Name: filepath.Base(abs), FullName: "local:" + filepath.Base(abs),
			LocalPath: abs, SourcePath: "", ResolvedRef: "local",
		},
		Skills: candidates, Scan: report, StagingPath: stage,
		CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
	}
	if err := sealInstallPreview(&preview); err != nil {
		return model.InstallPreview{}, err
	}
	if err := savePreview(m.Config.Paths.DataRoot, preview); err != nil {
		return model.InstallPreview{}, err
	}
	m.mu.Lock()
	m.previews[id] = preview
	m.mu.Unlock()
	return preview, nil
}

func (m *Manager) PrepareAdoption(selected []string, targetRootID ...string) (model.AdoptionPreview, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rootID := ""
	if len(targetRootID) > 0 {
		rootID = strings.TrimSpace(targetRootID[0])
	}
	root, err := m.resolveWritableRoot(rootID)
	if err != nil {
		return model.AdoptionPreview{}, err
	}
	selected = uniqueNonEmpty(selected)
	if len(selected) == 0 {
		return model.AdoptionPreview{}, errors.New("at least one unmanaged skill is required")
	}
	lock, err := m.store.LoadLock()
	if err != nil {
		return model.AdoptionPreview{}, err
	}
	skills, _, _, err := inventory.DiscoverRoots([]model.SkillRoot{root}, lock)
	if err != nil {
		return model.AdoptionPreview{}, err
	}
	available := map[string]model.Skill{}
	for _, skill := range skills {
		if !skill.System && !skill.Managed {
			available[skill.Name] = skill
		}
	}
	preview := model.AdoptionPreview{
		ID:           "adopt-plan-" + time.Now().UTC().Format("20060102T150405.000000000"),
		TargetRootID: root.ID,
		Skills:       []model.Skill{},
		Sources:      []model.DetectedSource{},
		CreatedAt:    time.Now().UTC(),
		ExpiresAt:    time.Now().UTC().Add(24 * time.Hour),
		Scan: model.ScanReport{
			ID: "scan-" + time.Now().UTC().Format("20060102T150405.000000000"), RootID: root.ID, Target: root.Path,
			StartedAt: time.Now().UTC(), HighestSeverity: model.RiskInfo,
			ActiveHighestSeverity: model.RiskInfo, Findings: []model.Finding{},
			Status: "passed", ScannerVersion: scanner.Version,
		},
	}
	for _, name := range selected {
		skill, ok := available[name]
		if !ok {
			return model.AdoptionPreview{}, fmt.Errorf("%s is not an unmanaged skill", name)
		}
		report, scanErr := scanner.Scan(skill.Path, m.Config.MaxFiles, m.Config.MaxFileBytes)
		if scanErr != nil {
			return model.AdoptionPreview{}, fmt.Errorf("scan %s: %w", name, scanErr)
		}
		for _, finding := range report.Findings {
			finding.File = filepath.ToSlash(filepath.Join(name, filepath.FromSlash(finding.File)))
			finding.SkillName, finding.RootID = name, root.ID
			preview.Scan.Findings = append(preview.Scan.Findings, finding)
		}
		preview.Scan.FilesScanned += report.FilesScanned
		if preview.Scan.FilesScanned > m.Config.MaxFiles {
			return model.AdoptionPreview{}, fmt.Errorf("selected skills exceed file count limit %d", m.Config.MaxFiles)
		}
		source := provenance.Detect(skill)
		skill.SourceProvider = source.Provider
		skill.SourceRepository = source.Repository
		skill.SourcePath = source.SourcePath
		skill.SourceGroupID = source.GroupID
		skill.SourceGroupName = source.GroupName
		skill.SourceConfidence = source.Confidence
		skill.SourceEvidence = source.Evidence
		preview.Skills = append(preview.Skills, skill)
		preview.Sources = append(preview.Sources, source)
		preview.Scan.Skills = append(preview.Scan.Skills, model.ScanSkillSummary{
			SkillName: name, RootID: root.ID, SourcePath: name, GroupID: source.GroupID, GroupName: source.GroupName,
			FilesScanned: report.FilesScanned, HighestSeverity: report.HighestSeverity,
		})
		for index := range preview.Scan.Findings {
			if preview.Scan.Findings[index].SkillName == name {
				preview.Scan.Findings[index].GroupID = source.GroupID
				preview.Scan.Findings[index].GroupName = source.GroupName
			}
		}
	}
	preview.Scan.CompletedAt = time.Now().UTC()
	preview.Scan.HighestSeverity = highestFindingSeverity(preview.Scan.Findings, false)
	if len(preview.Scan.Findings) > 0 {
		preview.Scan.Status = "findings"
	}
	ignored, ignoreErr := m.store.IgnoredFindings()
	if ignoreErr != nil {
		return model.AdoptionPreview{}, ignoreErr
	}
	preview.Scan = m.decorateScan(preview.Scan, ignored)
	summarizeScanSkills(&preview.Scan)
	_ = m.store.SaveScan(preview.Scan)
	m.recordScan(preview.Scan)
	if err := sealAdoptionPreview(&preview); err != nil {
		return model.AdoptionPreview{}, err
	}
	m.adoptions[preview.ID] = preview
	if err := saveAdoptionPreview(m.Config.Paths.DataRoot, preview); err != nil {
		return model.AdoptionPreview{}, err
	}
	return preview, nil
}

func (m *Manager) ApplyAdoption(planID string, selected []string, targetRootID ...string) (model.Transaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	preview, ok := m.adoptions[planID]
	if !ok {
		var err error
		preview, err = loadAdoptionPreview(m.Config.Paths.DataRoot, planID)
		if err != nil {
			return model.Transaction{}, err
		}
	}
	if time.Now().UTC().After(preview.ExpiresAt) {
		return model.Transaction{}, errors.New("adoption plan has expired")
	}
	if err := verifyAdoptionPreview(preview, planID); err != nil {
		return model.Transaction{}, err
	}
	if len(targetRootID) > 0 && strings.TrimSpace(targetRootID[0]) != "" && targetRootID[0] != preview.TargetRootID {
		return model.Transaction{}, errors.New("adoption plan target root does not match apply target")
	}
	root, err := m.resolveWritableRoot(preview.TargetRootID)
	if err != nil {
		return model.Transaction{}, err
	}
	releaseRootLease, err := acquireRootOperationLease(root)
	if err != nil {
		return model.Transaction{}, err
	}
	defer releaseRootLease()
	selected = uniqueNonEmpty(selected)
	allowed := map[string]bool{}
	planned := map[string]model.Skill{}
	plannedSources := map[string]model.DetectedSource{}
	for _, skill := range preview.Skills {
		allowed[skill.Name] = true
		planned[skill.Name] = skill
	}
	for _, source := range preview.Sources {
		plannedSources[source.SkillName] = source
	}
	for _, name := range selected {
		if !allowed[name] {
			return model.Transaction{}, fmt.Errorf("%s is not included in the adoption plan", name)
		}
	}
	if len(selected) == 0 {
		return model.Transaction{}, errors.New("no skills selected")
	}
	tx := model.Transaction{
		ID: "tx-" + time.Now().UTC().Format("20060102T150405.000000000"), RootID: root.ID, Type: "manage",
		Status: "running", Targets: selected, StartedAt: time.Now().UTC(),
	}
	if err := m.store.SaveTransaction(tx); err != nil {
		return model.Transaction{}, fmt.Errorf("start adoption transaction: %w", err)
	}
	lock, err := m.store.LoadLock()
	if err != nil {
		return m.fail(tx, err)
	}
	if err := m.snapshotLock(tx.ID); err != nil {
		return m.fail(tx, err)
	}
	securitySnapshot, err := m.snapshotSecurityStates(tx.ID, root.ID, selected)
	if err != nil {
		return m.fail(tx, err)
	}
	tx.BackupPaths = append(tx.BackupPaths, securitySnapshot)
	failAdoption := func(cause error) (model.Transaction, error) {
		recoveryErr := errors.Join(m.restoreLockSnapshot(tx.ID), m.restoreSecurityStates(tx.ID))
		if recoveryErr != nil {
			tx.RecoveryStatus = "required"
			cause = errors.Join(cause, recoveryErr)
		} else {
			tx.RecoveryStatus = "completed"
		}
		return m.fail(tx, cause)
	}
	securityStates := []model.SkillSecurityState{}
	for _, name := range selected {
		if _, managed := findManagedInRoot(lock, root.ID, name); managed {
			return m.fail(tx, fmt.Errorf("%s is already managed", name))
		}
		target := filepath.Join(root.Path, name)
		if !validMutableSkillName(name) {
			return m.fail(tx, fmt.Errorf("invalid skill name: %s", name))
		}
		files, err := inventory.HashTree(target)
		if err != nil {
			return m.fail(tx, err)
		}
		if !sameFileRecords(files, planned[name].Files) {
			return m.fail(tx, fmt.Errorf("%s changed after analysis; create a new adoption plan", name))
		}
		source, ok := plannedSources[name]
		if !ok {
			return m.fail(tx, fmt.Errorf("source analysis is missing for %s", name))
		}
		packageID := model.QualifiedPackageID(root.ID, source.GroupID)
		pkg := lock.Packages[packageID]
		if pkg.Skills == nil {
			pkg = model.PackageLock{
				RootID: root.ID, Provider: source.Provider, Repository: source.Repository, SourceURL: source.SourceURL,
				GroupName: source.GroupName, RequestedRef: source.RequestedRef,
				InstalledAt: time.Now().UTC(), Skills: map[string]model.SkillLock{},
			}
		}
		hashes := map[string]string{}
		for _, file := range files {
			hashes[file.Path] = file.SHA256
		}
		pkg.Skills[name] = model.SkillLock{
			RootID: root.ID, SourcePath: source.SourcePath, LocalPath: name, Files: hashes,
			LastScanReport: preview.Scan.ID,
		}
		pkg.UpdatedAt = time.Now().UTC()
		lock.Packages[packageID] = pkg
		securityStates = append(securityStates, model.SkillSecurityState{
			RootID: root.ID, SkillName: name, ContentHash: skillContentHash(files),
			ReportID: preview.Scan.ID, CheckedAt: scanCheckedAt(preview.Scan),
		})
	}
	if err := m.store.SaveSkillSecurityStates(securityStates); err != nil {
		return failAdoption(err)
	}
	if err := m.store.SaveLock(lock); err != nil {
		return failAdoption(err)
	}
	tx.Status, tx.CompletedAt = "completed", time.Now().UTC()
	if err := m.store.SaveTransaction(tx); err != nil {
		return failAdoption(fmt.Errorf("complete adoption transaction: %w", err))
	}
	m.recordTransaction(tx)
	return tx, nil
}

// ApplyAdoptionBestEffort gives select-all actions independent child
// transactions. A failed Skill is recorded and does not prevent the next
// explicit target from being attempted.
func (m *Manager) ApplyAdoptionBestEffort(planID string, selected []string, targetRootID string) (model.Transaction, error) {
	selected = uniqueNonEmpty(selected)
	if len(selected) == 0 {
		return model.Transaction{}, errors.New("no skills selected")
	}
	parent := model.Transaction{
		ID:     "tx-batch-" + time.Now().UTC().Format("20060102T150405.000000000"),
		RootID: strings.TrimSpace(targetRootID), Type: "manage-batch", Status: "running",
		Targets: append([]string(nil), selected...), StartedAt: time.Now().UTC(),
	}
	var failures []string
	succeeded := 0
	for _, name := range selected {
		child, err := m.ApplyAdoption(planID, []string{name}, targetRootID)
		item := model.BatchItemResult{Target: name, Status: "completed", TransactionID: child.ID}
		if err != nil {
			item.Status = "failed"
			item.Error = err.Error()
			failures = append(failures, fmt.Sprintf("%s: %v", name, err))
		} else {
			succeeded++
		}
		parent.ItemResults = append(parent.ItemResults, item)
	}
	parent.CompletedAt = time.Now().UTC()
	switch {
	case succeeded == 0:
		parent.Status = "failed"
	case len(failures) > 0:
		parent.Status = "partial"
	default:
		parent.Status = "completed"
	}
	if len(failures) > 0 {
		parent.Error = strings.Join(failures, "; ")
	}
	if err := m.store.SaveTransaction(parent); err != nil {
		return parent, errors.Join(errors.New("persist batch adoption transaction"), err)
	}
	m.recordTransaction(parent)
	if succeeded == 0 {
		return parent, errors.New(parent.Error)
	}
	return parent, nil
}

// ApplyInstall requires both the persisted, audited High-risk cluster decision
// and a final caller acknowledgement. The boolean cannot create an acceptance
// record and therefore cannot bypass the backend decision workflow.
func (m *Manager) ApplyInstall(planID string, selected []string, acceptHighRisk bool, targetRootID ...string) (model.Transaction, error) {
	// The target root is sealed into the preview. An optional argument keeps
	// older callers source-compatible while preventing apply-time retargeting.
	if len(targetRootID) > 0 {
		if strings.TrimSpace(targetRootID[0]) == "" {
			targetRootID = nil
		}
	}
	if len(targetRootID) > 0 {
		preview, ok := m.previews[planID]
		if !ok {
			var err error
			preview, err = loadPreview(m.Config.Paths.DataRoot, planID)
			if err != nil {
				return model.Transaction{}, err
			}
		}
		if preview.TargetRootID != targetRootID[0] {
			return model.Transaction{}, errors.New("install plan target root does not match apply target")
		}
	}
	preview, ok := m.previews[planID]
	if !ok {
		var err error
		preview, err = loadPreview(m.Config.Paths.DataRoot, planID)
		if err != nil {
			return model.Transaction{}, err
		}
	}
	root, err := m.resolveWritableRoot(preview.TargetRootID)
	if err != nil {
		return model.Transaction{}, err
	}
	release, err := acquireRootOperationLease(root)
	if err != nil {
		return model.Transaction{}, err
	}
	defer release()
	return m.applyInstallWithTransactionIDAndRisk(planID, selected, "", acceptHighRisk)
}

// ApplyInstallBestEffort runs one journaled child transaction per selected
// Skill. This is used by select-all UI actions; single-plan callers retain the
// atomic ApplyInstall behavior for compatibility.
func (m *Manager) ApplyInstallBestEffort(planID string, selected []string, acceptHighRisk bool, targetRootID string) (model.Transaction, error) {
	selected = uniqueNonEmpty(selected)
	if len(selected) == 0 {
		return model.Transaction{}, errors.New("no skills selected")
	}
	parent := model.Transaction{
		ID:     "tx-batch-" + time.Now().UTC().Format("20060102T150405.000000000"),
		RootID: strings.TrimSpace(targetRootID), Type: "install-batch", Status: "running",
		Targets: append([]string(nil), selected...), StartedAt: time.Now().UTC(),
	}
	var failures []string
	succeeded := 0
	for _, name := range selected {
		child, err := m.ApplyInstall(planID, []string{name}, acceptHighRisk, targetRootID)
		item := model.BatchItemResult{Target: name, Status: "completed", TransactionID: child.ID}
		if err != nil {
			item.Status = "failed"
			item.Error = err.Error()
			failures = append(failures, fmt.Sprintf("%s: %v", name, err))
		} else {
			succeeded++
		}
		parent.ItemResults = append(parent.ItemResults, item)
	}
	parent.CompletedAt = time.Now().UTC()
	switch {
	case succeeded == 0:
		parent.Status = "failed"
	case len(failures) > 0:
		parent.Status = "partial"
	default:
		parent.Status = "completed"
	}
	if len(failures) > 0 {
		parent.Error = strings.Join(failures, "; ")
	}
	if err := m.store.SaveTransaction(parent); err != nil {
		return parent, errors.Join(errors.New("persist batch install transaction"), err)
	}
	m.recordTransaction(parent)
	if succeeded == 0 {
		return parent, errors.New(parent.Error)
	}
	return parent, nil
}

func (m *Manager) applyInstallWithTransactionID(
	planID string,
	selected []string,
	transactionID string,
) (model.Transaction, error) {
	return m.applyInstallWithTransactionIDAndRisk(planID, selected, transactionID, true)
}

func (m *Manager) applyInstallWithTransactionIDAndRisk(
	planID string,
	selected []string,
	transactionID string,
	acceptHighRisk bool,
) (model.Transaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	preview, ok := m.previews[planID]
	if !ok {
		var err error
		preview, err = loadPreview(m.Config.Paths.DataRoot, planID)
		if err != nil {
			return model.Transaction{}, err
		}
	}
	if time.Now().UTC().After(preview.ExpiresAt) {
		return model.Transaction{}, errors.New("install plan has expired")
	}
	if err := m.verifyInstallPreviewMetadata(preview, planID); err != nil {
		return model.Transaction{}, err
	}
	if _, err := m.enforceProjectAssessmentGate(preview); err != nil {
		return model.Transaction{}, err
	}
	ignored, err := m.store.IgnoredFindings()
	if err != nil {
		return model.Transaction{}, err
	}
	preview.Scan = m.decorateScan(preview.Scan, ignored)
	if scanContainsAcceptedHighRisk(preview.Scan) && !acceptHighRisk {
		return model.Transaction{}, errors.New("accepted High risk requires final apply confirmation")
	}
	if isBlockingRiskSeverity(preview.Scan.ActiveHighestSeverity) {
		activeBlockingClusters := 0
		for _, cluster := range preview.Scan.Clusters {
			if !cluster.Ignored && isBlockingRiskSeverity(cluster.Severity) {
				activeBlockingClusters++
			}
		}
		return model.Transaction{}, fmt.Errorf(
			"仍有 %d 个阻断风险簇：Critical 不可忽略，High 必须逐簇确认并填写原因",
			activeBlockingClusters,
		)
	}
	chosen := chooseCandidates(preview.Skills, selected)
	if len(chosen) == 0 {
		return model.Transaction{}, errors.New("no skills selected")
	}
	if err := m.verifyInstallPreview(preview, chosen); err != nil {
		return model.Transaction{}, err
	}
	root, err := m.resolveWritableRoot(preview.TargetRootID)
	if err != nil {
		return model.Transaction{}, err
	}
	if _, err := config.EnsureSkillRoot(m.Config, root.ID); err != nil {
		return model.Transaction{}, fmt.Errorf("create target skill root: %w", err)
	}
	if transactionID == "" {
		transactionID = "tx-" + time.Now().UTC().Format("20060102T150405.000000000")
	}
	if !validAssistedReferenceID(transactionID) || !strings.HasPrefix(transactionID, "tx-") {
		return model.Transaction{}, errors.New("invalid installation transaction ID")
	}
	tx := model.Transaction{
		ID: transactionID, RootID: root.ID,
		Type: "install", Status: "running", StartedAt: time.Now().UTC(),
	}
	for _, c := range chosen {
		// RootID qualifies transaction targets. Keep each target as the bare
		// directory name so recovery never has to reinterpret a display identity.
		tx.Targets = append(tx.Targets, c.Name)
	}
	if err := m.store.SaveTransaction(tx); err != nil {
		return model.Transaction{}, err
	}

	lock, err := m.store.LoadLock()
	if err != nil {
		return m.fail(tx, err)
	}
	if err := m.snapshotLock(tx.ID); err != nil {
		return m.fail(tx, err)
	}
	selectedNames := make([]string, 0, len(chosen))
	for _, candidate := range chosen {
		selectedNames = append(selectedNames, candidate.Name)
	}
	securitySnapshot, err := m.snapshotSecurityStates(tx.ID, root.ID, selectedNames)
	if err != nil {
		return m.fail(tx, err)
	}
	tx.BackupPaths = append(tx.BackupPaths, securitySnapshot)
	touched := []string{}
	backups := map[string]string{}
	securityStates := []model.SkillSecurityState{}
	provider := preview.Repository.Provider
	if provider == "" {
		provider = "github"
	}
	pkgID := provider + ":" + strings.ToLower(preview.Repository.FullName)
	if provider == "local" {
		pkgID = "local:" + strings.ToLower(filepath.Clean(preview.Repository.LocalPath))
	}
	packageKey := model.QualifiedPackageID(root.ID, pkgID)
	pkg, pkgFound := lock.Packages[packageKey]
	if !pkgFound {
		pkg, pkgFound = lock.Packages[pkgID]
	}
	if pkg.Skills == nil {
		sourceURL := "https://github.com/" + preview.Repository.FullName
		repository := preview.Repository.FullName
		if provider == "local" {
			sourceURL = preview.Repository.LocalPath
			repository = preview.Repository.LocalPath
		}
		pkg = model.PackageLock{
			RootID: root.ID, Provider: provider, Repository: repository, GroupName: repository,
			SourceURL:    sourceURL,
			RequestedRef: preview.Repository.ResolvedRef, ResolvedCommit: preview.Repository.CommitSHA,
			InstalledAt: time.Now().UTC(), Skills: map[string]model.SkillLock{},
		}
	}
	for name, skill := range pkg.Skills {
		if skill.ResolvedCommit == "" {
			skill.ResolvedCommit = pkg.ResolvedCommit
			pkg.Skills[name] = skill
		}
	}
	for _, c := range chosen {
		target := filepath.Join(root.Path, c.Name)
		if !validMutableSkillName(c.Name) {
			return m.failInstall(tx, root, touched, backups, fmt.Errorf("invalid skill name: %s", c.Name))
		}
		_, source, sourceErr := candidateSourceTarget(preview.StagingPath, c.SourcePath)
		if sourceErr != nil {
			return m.failInstall(tx, root, touched, backups, fmt.Errorf("invalid Skill source path: %s", c.SourcePath))
		}
		recoveryTracked := false
		if _, err := os.Stat(target); err == nil {
			existing, managed := findManagedInRoot(lock, root.ID, c.Name)
			expectedRepository := preview.Repository.FullName
			if provider == "local" {
				expectedRepository = preview.Repository.LocalPath
			}
			if !managed || !strings.EqualFold(existing.Repository, expectedRepository) {
				return m.failInstall(tx, root, touched, backups, fmt.Errorf("skill name conflict: %s", c.Name))
			}
			backup := transactionContentPath(
				root.Path, m.Config.Paths.BackupsRoot, ".csm-backups", c.Name, tx.ID,
			)
			if err := os.MkdirAll(filepath.Dir(backup), 0o700); err != nil {
				return m.failInstall(tx, root, touched, backups, err)
			}
			if err := os.Rename(target, backup); err != nil {
				return m.failInstall(tx, root, touched, backups, fmt.Errorf("backup %s: %w", c.Name, err))
			}
			backups[c.Name] = backup
			tx.BackupPaths = append(tx.BackupPaths, backup)
			// Track the Skill immediately after the destructive rename. If the
			// following journal checkpoint fails, failInstall must still restore
			// the backup to its original location.
			touched = append(touched, c.Name)
			recoveryTracked = true
			if err := m.store.SaveTransaction(tx); err != nil {
				return m.failInstall(tx, root, touched, backups, err)
			}
		}
		if !recoveryTracked {
			touched = append(touched, c.Name)
		}
		if err := snapshotLocalSource(source, target, m.Config.MaxFiles, m.Config.MaxFileBytes); err != nil {
			return m.failInstall(tx, root, touched, backups, err)
		}
		files, err := inventory.HashTree(target)
		if err != nil {
			return m.failInstall(tx, root, touched, backups, err)
		}
		if !sameFileRecords(files, c.Files) {
			return m.failInstall(tx, root, touched, backups, fmt.Errorf("%s changed while being copied; installation was recovered", c.Name))
		}
		hashes := map[string]string{}
		for _, f := range files {
			hashes[f.Path] = f.SHA256
		}
		pkg.Skills[c.Name] = model.SkillLock{
			RootID: root.ID, SourcePath: c.SourcePath, LocalPath: c.Name, Files: hashes,
			ResolvedCommit: preview.Repository.CommitSHA,
			Pinned:         preview.Repository.ResolvedRef != preview.Repository.DefaultBranch,
			LastScanReport: preview.Scan.ID,
		}
		securityStates = append(securityStates, model.SkillSecurityState{
			RootID: root.ID, SkillName: c.Name, ContentHash: skillContentHash(files),
			ReportID: preview.Scan.ID, CheckedAt: scanCheckedAt(preview.Scan),
		})
	}
	pkg.UpdatedAt = time.Now().UTC()
	pkg.ResolvedCommit = preview.Repository.CommitSHA
	pkg.RootID = root.ID
	lock.Packages[packageKey] = pkg
	if err := m.store.SaveSkillSecurityStates(securityStates); err != nil {
		return m.failInstall(tx, root, touched, backups, err)
	}
	if err := m.store.SaveLock(lock); err != nil {
		return m.failInstall(tx, root, touched, backups, err)
	}
	tx.Status, tx.CompletedAt = "completed", time.Now().UTC()
	if err := m.store.SaveTransaction(tx); err != nil {
		return m.failInstall(tx, root, touched, backups, err)
	}
	m.recordTransaction(tx)
	return tx, nil
}

func (m *Manager) verifyInstallPreview(preview model.InstallPreview, chosen []model.CandidateSkill) error {
	if preview.Repository.Provider == "github" {
		if strings.TrimSpace(preview.Repository.CommitSHA) == "" {
			return errors.New("GitHub install plan is not pinned to an immutable commit")
		}
		if err := ensureWithinOrEqual(m.Config.Paths.StagingRoot, preview.StagingPath); err != nil {
			return fmt.Errorf("install staging path is not managed by this application: %w", err)
		}
	}
	for _, candidate := range chosen {
		_, source, sourceErr := candidateSourceTarget(preview.StagingPath, candidate.SourcePath)
		if sourceErr != nil {
			return fmt.Errorf("invalid source path for %s: %w", candidate.Name, sourceErr)
		}
		if err := ensureResolvedWithinOrEqual(preview.StagingPath, source); err != nil {
			return fmt.Errorf("invalid source path for %s: %w", candidate.Name, err)
		}
		files, err := inventory.HashTree(source)
		if err != nil {
			return fmt.Errorf("verify %s after analysis: %w", candidate.Name, err)
		}
		if !sameFileRecords(files, candidate.Files) {
			return fmt.Errorf("%s changed after analysis; create a new install plan", candidate.Name)
		}
	}
	return nil
}

func (m *Manager) AdoptPackage(repository, sourceURL, ref, commit string, mappings map[string]string, targetRootID ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rootID := ""
	if len(targetRootID) > 0 {
		rootID = targetRootID[0]
	}
	root, err := m.resolveWritableRoot(rootID)
	if err != nil {
		return err
	}
	if repository == "" || len(mappings) == 0 {
		return errors.New("repository and mappings are required")
	}
	canonicalCommit, err := model.CanonicalCommitSHA(commit)
	if err != nil || canonicalCommit == "" {
		return errors.New("GitHub source must be linked to a full immutable commit SHA")
	}
	commit = canonicalCommit
	lock, err := m.store.LoadLock()
	if err != nil {
		return err
	}
	id := "github:" + strings.ToLower(repository)
	packageKey := model.QualifiedPackageID(root.ID, id)
	pkg := model.PackageLock{
		RootID: root.ID, Provider: "github", SourceAssociation: model.SourceAssociationRemote, Repository: repository, SourceURL: sourceURL,
		RequestedRef: ref, ResolvedCommit: commit, InstalledAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(), Skills: map[string]model.SkillLock{},
	}
	for name, sourcePath := range mappings {
		if !validMutableSkillName(name) {
			return fmt.Errorf("invalid Skill name: %s", name)
		}
		local, err := config.SkillTarget(root, name)
		if err != nil {
			return err
		}
		files, err := inventory.HashTree(local)
		if err != nil {
			return fmt.Errorf("adopt %s: %w", name, err)
		}
		hashes := map[string]string{}
		for _, f := range files {
			hashes[f.Path] = f.SHA256
		}
		pkg.Skills[name] = model.SkillLock{
			RootID: root.ID, SourceAssociation: model.SourceAssociationRemote, SourcePath: sourcePath, LocalPath: name, ResolvedCommit: commit, ResolvedRef: commit, Files: hashes,
		}
	}
	lock.Packages[packageKey] = pkg
	return m.store.SaveLock(lock)
}

// LinkLocalSource associates one existing local Skill with a user-supplied
// GitHub source. The remote ref is resolved and staged first; linking fails if
// the remote candidate does not exactly match the installed tree, preserving
// local edits instead of silently replacing them.
func (m *Manager) LinkLocalSource(ctx context.Context, skillName, rawURL, ref, targetRootID string) (model.DetectedSource, error) {
	skillName = strings.TrimSpace(skillName)
	if !validMutableSkillName(skillName) {
		return model.DetectedSource{}, errors.New("a valid Skill name is required")
	}
	root, err := m.resolveWritableRoot(targetRootID)
	if err != nil {
		return model.DetectedSource{}, err
	}
	lock, err := m.store.LoadLock()
	if err != nil {
		return model.DetectedSource{}, err
	}
	skills, _, _, err := inventory.DiscoverRoots([]model.SkillRoot{root}, lock)
	if err != nil {
		return model.DetectedSource{}, err
	}
	var local model.Skill
	for _, candidate := range skills {
		if candidate.Name == skillName && !candidate.System {
			local = candidate
			break
		}
	}
	if local.Name == "" {
		return model.DetectedSource{}, fmt.Errorf("local Skill not found: %s", skillName)
	}
	preview, err := m.PrepareGitHubForRoot(ctx, rawURL, ref, root.ID)
	if err != nil {
		return model.DetectedSource{}, err
	}
	var selected model.CandidateSkill
	for _, candidate := range preview.Skills {
		if candidate.Name == skillName {
			selected = candidate
			break
		}
	}
	if selected.Name == "" {
		return model.DetectedSource{}, fmt.Errorf("remote source does not contain Skill %q", skillName)
	}
	_, remotePath, err := candidateSourceTarget(preview.StagingPath, selected.SourcePath)
	if err != nil {
		return model.DetectedSource{}, err
	}
	remoteFiles, err := inventory.HashTree(remotePath)
	if err != nil {
		return model.DetectedSource{}, err
	}
	localFiles, err := inventory.HashTree(local.Path)
	if err != nil {
		return model.DetectedSource{}, err
	}
	if !sameFileRecords(localFiles, remoteFiles) {
		return model.DetectedSource{}, errors.New("local Skill differs from the requested GitHub source; review the replacement instead of linking it")
	}
	if err := m.AdoptPackage(preview.Repository.FullName, rawURL, preview.Repository.ResolvedRef, preview.Repository.CommitSHA, map[string]string{skillName: selected.SourcePath}, root.ID); err != nil {
		return model.DetectedSource{}, err
	}
	return model.DetectedSource{
		SkillName: skillName, RootID: root.ID, Provider: "github", SourceAssociation: model.SourceAssociationRemote,
		Repository: preview.Repository.FullName, SourceURL: rawURL,
		SourcePath: selected.SourcePath, RequestedRef: preview.Repository.ResolvedRef,
		ResolvedCommit: preview.Repository.CommitSHA, GroupID: "github:" + strings.ToLower(preview.Repository.FullName),
		GroupName: preview.Repository.FullName, Confidence: 1, Evidence: "用户确认并通过完整树哈希校验关联远程来源",
	}, nil
}

func (m *Manager) CheckUpdates(ctx context.Context) (model.UpdateCheckResult, error) {
	return m.CheckUpdatesSelected(ctx, nil, false)
}

func (m *Manager) CheckUpdatesSelected(ctx context.Context, groupIDs []string, force bool) (model.UpdateCheckResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	lock, err := m.store.LoadLock()
	if err != nil {
		return model.UpdateCheckResult{}, err
	}
	checkedAt := time.Now().UTC()
	result := model.UpdateCheckResult{CheckedAt: checkedAt, Statuses: []model.UpdateStatus{}}
	previousStatuses, _ := m.store.LatestUpdateStatuses()
	previous := map[string]model.UpdateStatus{}
	for _, status := range previousStatuses {
		previous[model.QualifiedPackageID(status.RootID, status.GroupID)] = status
		if status.RootID == "" {
			previous[status.GroupID] = status
		}
	}
	selected := map[string]bool{}
	for _, id := range groupIDs {
		selected[strings.TrimSpace(id)] = true
	}
	for id, pkg := range lock.Packages {
		rootID, groupID := updatePackageIdentity(id, pkg)
		qualifiedID := model.QualifiedPackageID(rootID, groupID)
		selectedMatch := len(selected) == 0 || selected[id] || selected[qualifiedID] || selected[groupID]
		if len(selected) > 0 && !selectedMatch {
			if status, ok := previous[qualifiedID]; ok {
				result.Statuses = append(result.Statuses, status)
			}
			continue
		}
		status := model.UpdateStatus{
			GroupID: groupID, RootID: rootID, GroupName: pkg.GroupName, Provider: pkg.Provider,
			Repository: pkg.Repository, Status: "unsupported", CheckedAt: checkedAt,
			CurrentCommits: map[string]string{}, OutdatedSkills: []string{},
		}
		if model.NormalizeSourceAssociation(pkg.Provider, pkg.SourceAssociation) == model.SourceAssociationUnlinked {
			status.Error = "local source is not linked to a verifiable GitHub repository"
		}
		if status.GroupName == "" {
			status.GroupName = pkg.Repository
		}
		for name, skill := range pkg.Skills {
			current := skill.ResolvedCommit
			if current == "" {
				current = pkg.ResolvedCommit
			}
			status.CurrentCommits[name] = current
		}
		if pkg.Provider != "github" || pkg.SourceURL == "" {
			result.Statuses = append(result.Statuses, status)
			continue
		}
		repo, meta, err := m.github.ResolveCached(ctx, pkg.SourceURL, pkg.RequestedRef, force)
		if err != nil {
			status.Status, status.Error = "error", err.Error()
			if old, ok := previous[qualifiedID]; ok {
				if old.Status == "up-to-date" || old.Status == "update-available" {
					status.LastSuccessStatus = old.Status
					value := old.CheckedAt
					status.LastSuccessAt = &value
					status.LastSuccessRemoteCommit = old.RemoteCommit
					status.OutdatedSkills = append([]string(nil), old.OutdatedSkills...)
				} else {
					status.LastSuccessStatus = old.LastSuccessStatus
					status.LastSuccessAt = old.LastSuccessAt
					status.LastSuccessRemoteCommit = old.LastSuccessRemoteCommit
					status.OutdatedSkills = append([]string(nil), old.OutdatedSkills...)
				}
			}
			var apiErr *githubsource.APIError
			if errors.As(err, &apiErr) {
				status.RetryAt = apiErr.RetryAt
				status.RateLimitLimit = apiErr.Limit
				status.RateLimitRemaining = apiErr.Remaining
				if apiErr.RetryAt != nil {
					status.Status = "rate-limited"
				}
			}
			result.Statuses = append(result.Statuses, status)
			continue
		}
		status.FromCache = meta.FromCache
		status.RemoteCommit = repo.CommitSHA
		for name, current := range status.CurrentCommits {
			if current == "" || current != repo.CommitSHA {
				status.OutdatedSkills = append(status.OutdatedSkills, name)
			}
		}
		sort.Strings(status.OutdatedSkills)
		if len(status.OutdatedSkills) == 0 {
			status.Status = "up-to-date"
		} else {
			status.Status = "update-available"
		}
		status.LastSuccessStatus = status.Status
		value := checkedAt
		status.LastSuccessAt = &value
		status.LastSuccessRemoteCommit = status.RemoteCommit
		result.Statuses = append(result.Statuses, status)
	}
	sort.Slice(result.Statuses, func(i, j int) bool {
		return result.Statuses[i].GroupName < result.Statuses[j].GroupName
	})
	if err := m.store.SaveUpdateStatuses(result.Statuses); err != nil {
		return model.UpdateCheckResult{}, err
	}
	return result, nil
}

func updatePackageIdentity(key string, pkg model.PackageLock) (string, string) {
	rootID := strings.TrimSpace(pkg.RootID)
	groupID := strings.TrimSpace(key)
	if index := strings.IndexByte(groupID, '\x00'); index >= 0 {
		if rootID == "" {
			rootID = groupID[:index]
		}
		groupID = groupID[index+1:]
	}
	if rootID != "" {
		prefix := rootID + "\x00"
		if strings.HasPrefix(strings.TrimSpace(key), prefix) {
			groupID = strings.TrimPrefix(strings.TrimSpace(key), prefix)
		}
	}
	return rootID, groupID
}

func (m *Manager) PrepareUpdate(ctx context.Context, groupID string, targetRootID ...string) (model.InstallPreview, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return model.InstallPreview{}, errors.New("explicit source group ID is required")
	}
	rootID := ""
	if len(targetRootID) > 0 {
		rootID = targetRootID[0]
	}
	lock, err := m.store.LoadLock()
	if err != nil {
		return model.InstallPreview{}, err
	}
	var pkg model.PackageLock
	var ok bool
	if rootID != "" {
		pkg, ok = lock.Packages[model.QualifiedPackageID(rootID, groupID)]
		if !ok {
			pkg, ok = lock.Packages[groupID]
		}
	} else {
		matches := make([]model.PackageLock, 0, 1)
		for key, candidate := range lock.Packages {
			if key == groupID || strings.HasSuffix(key, "\x00"+groupID) {
				matches = append(matches, candidate)
			}
		}
		if len(matches) == 1 {
			pkg, ok = matches[0], true
		} else if len(matches) > 1 {
			return model.InstallPreview{}, errors.New("source group exists in multiple roots; specify rootId")
		}
	}
	if !ok {
		return model.InstallPreview{}, fmt.Errorf("source group not found: %s", groupID)
	}
	if pkg.RootID != "" {
		if rootID != "" && pkg.RootID != rootID {
			return model.InstallPreview{}, errors.New("source group does not belong to the requested root")
		}
		rootID = pkg.RootID
	}
	root, err := m.resolveWritableRoot(rootID)
	if err != nil {
		return model.InstallPreview{}, err
	}
	if pkg.Provider != "github" || pkg.SourceURL == "" {
		return model.InstallPreview{}, errors.New("this source does not support GitHub updates")
	}
	installed := map[string]bool{}
	for name := range pkg.Skills {
		installed[name] = true
	}
	preview, err := m.prepareGitHub(ctx, pkg.SourceURL, pkg.RequestedRef, installed, root.ID)
	if err != nil {
		return model.InstallPreview{}, err
	}
	// prepareGitHub already stores the filtered, immutable update plan.
	return preview, nil
}

func scanCandidateSkills(root string, candidates []model.CandidateSkill, maxFiles int, maxFileBytes int64) (model.ScanReport, error) {
	now := time.Now().UTC()
	report := model.ScanReport{
		ID: "scan-" + now.Format("20060102T150405.000000000"), Target: root,
		StartedAt: now, HighestSeverity: model.RiskInfo, ActiveHighestSeverity: model.RiskInfo,
		Findings: []model.Finding{}, Status: "passed", ScannerVersion: scanner.Version,
	}
	plannedFiles := 0
	for _, candidate := range candidates {
		plannedFiles += len(candidate.Files)
	}
	if plannedFiles > maxFiles {
		finishCandidateScan(&report)
		return report, fmt.Errorf(
			"selected Skills contain %d files, exceeding the configured total scan limit of %d; choose a specific Skill path or raise maxFiles after review",
			plannedFiles, maxFiles,
		)
	}
	remaining := maxFiles
	for _, candidate := range candidates {
		sourcePath, target, err := candidateSourceTarget(root, candidate.SourcePath)
		if err != nil {
			finishCandidateScan(&report)
			return report, fmt.Errorf("invalid Skill source path: %s", candidate.SourcePath)
		}
		part, err := scanner.Scan(target, remaining, maxFileBytes)
		report.FilesScanned += part.FilesScanned
		for _, finding := range part.Findings {
			finding.File = filepath.ToSlash(filepath.Join(sourcePath, filepath.FromSlash(finding.File)))
			report.Findings = append(report.Findings, finding)
		}
		if err != nil {
			finishCandidateScan(&report)
			return report, fmt.Errorf("scan %s: %w", candidate.Name, err)
		}
		remaining = maxFiles - report.FilesScanned
		if remaining < 0 {
			finishCandidateScan(&report)
			return report, fmt.Errorf("total file count exceeds configured limit %d", maxFiles)
		}
	}
	finishCandidateScan(&report)
	return report, nil
}

// candidateSourceTarget accepts a repository-root Skill explicitly. A root
// SKILL.md is a valid candidate, but every other candidate must remain a
// normalized descendant of the staged repository.
func candidateSourceTarget(root, sourcePath string) (string, string, error) {
	normalized := strings.TrimSpace(filepath.ToSlash(sourcePath))
	if normalized == "" {
		normalized = "."
	}
	clean := filepath.Clean(filepath.FromSlash(normalized))
	target := filepath.Join(root, clean)
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", "", errors.New("candidate source path escapes the repository root")
	}
	if rel == "." {
		return ".", root, nil
	}
	return filepath.ToSlash(rel), target, nil
}

func finishCandidateScan(report *model.ScanReport) {
	report.HighestSeverity = highestFindingSeverity(report.Findings, false)
	report.ActiveHighestSeverity = report.HighestSeverity
	report.ActiveFindingCount = len(report.Findings)
	if len(report.Findings) > 0 {
		report.Status = "findings"
	}
	report.CompletedAt = time.Now().UTC()
}

func annotateCandidateScan(
	report *model.ScanReport,
	candidates []model.CandidateSkill,
	groupID string,
	groupName string,
) {
	report.Skills = make([]model.ScanSkillSummary, 0, len(candidates))
	for _, candidate := range candidates {
		report.Skills = append(report.Skills, model.ScanSkillSummary{
			SkillName: candidate.Name, SourcePath: filepath.ToSlash(candidate.SourcePath),
			GroupID: groupID, GroupName: groupName, FilesScanned: len(candidate.Files),
			HighestSeverity: model.RiskInfo,
		})
	}
	for index := range report.Findings {
		finding := &report.Findings[index]
		bestName, bestLength := "", -1
		path := strings.Trim(filepath.ToSlash(finding.File), "/")
		for _, candidate := range candidates {
			prefix := strings.Trim(filepath.ToSlash(candidate.SourcePath), "/")
			matches := prefix == "" || prefix == "." || path == prefix || strings.HasPrefix(path, prefix+"/")
			if matches && len(prefix) > bestLength {
				bestName, bestLength = candidate.Name, len(prefix)
			}
		}
		if bestName == "" && len(candidates) == 1 {
			bestName = candidates[0].Name
		}
		finding.SkillName, finding.GroupID, finding.GroupName = bestName, groupID, groupName
	}
}

func (m *Manager) Quarantine(names []string, targetRootID ...string) (model.Transaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rootID := ""
	if len(targetRootID) > 0 {
		rootID = targetRootID[0]
	}
	root, err := m.resolveWritableRoot(rootID)
	if err != nil {
		return model.Transaction{}, err
	}
	names = uniqueNonEmpty(names)
	if len(names) == 0 {
		return model.Transaction{}, errors.New("at least one explicit skill name is required")
	}
	for _, name := range names {
		if !validMutableSkillName(name) {
			return model.Transaction{}, fmt.Errorf("invalid uninstall target: %s", name)
		}
	}
	tx := model.Transaction{
		ID: "tx-" + time.Now().UTC().Format("20060102T150405.000000000"), RootID: root.ID,
		Type: "quarantine", Status: "running", Targets: names, StartedAt: time.Now().UTC(),
	}
	if err := m.store.SaveTransaction(tx); err != nil {
		return model.Transaction{}, fmt.Errorf("start quarantine transaction: %w", err)
	}
	lock, err := m.store.LoadLock()
	if err != nil {
		return m.fail(tx, err)
	}
	if err := m.snapshotLock(tx.ID); err != nil {
		return m.fail(tx, err)
	}
	type movedSkill struct {
		source string
		target string
	}
	moved := []movedSkill{}
	rollbackMoved := func(cause error) (model.Transaction, error) {
		var rollbackErr error
		for i := len(moved) - 1; i >= 0; i-- {
			if err := os.Rename(moved[i].target, moved[i].source); err != nil && rollbackErr == nil {
				rollbackErr = err
			}
		}
		if err := m.restoreLockSnapshot(tx.ID); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
		if rollbackErr != nil {
			tx.RecoveryStatus = "required"
			cause = fmt.Errorf("%w; quarantine rollback failed: %v", cause, rollbackErr)
		} else {
			tx.RecoveryStatus = "completed"
		}
		return m.fail(tx, cause)
	}
	for _, name := range names {
		source := filepath.Join(root.Path, name)
		if _, err := os.Stat(source); err != nil {
			return rollbackMoved(fmt.Errorf("%s: %w", name, err))
		}
		target := transactionContentPath(
			root.Path, m.Config.Paths.QuarantineRoot, ".csm-quarantine", name, tx.ID,
		)
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return rollbackMoved(err)
		}
		if err := os.Rename(source, target); err != nil {
			return rollbackMoved(err)
		}
		moved = append(moved, movedSkill{source: source, target: target})
		tx.BackupPaths = append(tx.BackupPaths, target)
		for id, pkg := range lock.Packages {
			if pkg.RootID != "" && pkg.RootID != root.ID {
				continue
			}
			if _, ok := pkg.Skills[name]; ok {
				delete(pkg.Skills, name)
				lock.Packages[id] = pkg
				break
			}
		}
	}
	if err := m.store.SaveLock(lock); err != nil {
		return rollbackMoved(err)
	}
	tx.Status, tx.CompletedAt = "completed", time.Now().UTC()
	if err := m.store.SaveTransaction(tx); err != nil {
		return rollbackMoved(fmt.Errorf("complete quarantine transaction: %w", err))
	}
	m.recordTransaction(tx)
	return tx, nil
}

func (m *Manager) Restore(name, transactionID string, targetRootID ...string) (model.Transaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rootID := ""
	if len(targetRootID) > 0 {
		rootID = targetRootID[0]
	}
	root, err := m.resolveWritableRoot(rootID)
	if err != nil {
		return model.Transaction{}, err
	}
	if transactionID == "" || !validMutableSkillName(name) {
		return model.Transaction{}, errors.New("explicit skill name and transaction ID are required")
	}
	original, originalErr := m.store.Transaction(transactionID)
	if originalErr != nil {
		return model.Transaction{}, originalErr
	}
	if len(targetRootID) == 0 && original.RootID != "" && original.RootID != root.ID {
		root, err = m.resolveWritableRoot(original.RootID)
		if err != nil {
			return model.Transaction{}, err
		}
	}
	if original.Type != "quarantine" {
		return model.Transaction{}, fmt.Errorf("transaction type %s cannot restore quarantined content", original.Type)
	}
	if len(targetRootID) > 0 && original.RootID != "" && original.RootID != root.ID {
		return model.Transaction{}, errors.New("quarantine transaction belongs to a different root")
	}
	source := transactionPathForName(original, name)
	if source == "" {
		source = transactionContentPath(
			root.Path, m.Config.Paths.QuarantineRoot, ".csm-quarantine", name, transactionID,
		)
	}
	if err := ensureWithinRestoreRoots(root.Path, m.Config.Paths.QuarantineRoot, ".csm-quarantine", source); err != nil {
		return model.Transaction{}, fmt.Errorf("invalid quarantine restore source: %w", err)
	}
	target := filepath.Join(root.Path, name)
	if _, err := os.Stat(target); err == nil {
		return model.Transaction{}, fmt.Errorf("target already exists: %s", target)
	}
	tx := model.Transaction{ID: "tx-" + time.Now().UTC().Format("20060102T150405.000000000"), RootID: root.ID, Type: "restore", Status: "running", Targets: []string{name}, StartedAt: time.Now().UTC()}
	if err := m.store.SaveTransaction(tx); err != nil {
		return model.Transaction{}, fmt.Errorf("start restore transaction: %w", err)
	}
	if err := os.Rename(source, target); err != nil {
		return m.fail(tx, err)
	}
	rollbackTarget := func(cause error) (model.Transaction, error) {
		if err := os.Rename(target, source); err != nil {
			tx.RecoveryStatus = "required"
			cause = errors.Join(cause, fmt.Errorf("restore filesystem rollback failed: %w", err))
		} else {
			tx.RecoveryStatus = "completed"
		}
		return m.fail(tx, cause)
	}
	current, err := m.store.LoadLock()
	if err != nil {
		return rollbackTarget(err)
	}
	previousLock, err := json.Marshal(current)
	if err != nil {
		return rollbackTarget(fmt.Errorf("snapshot current source lock: %w", err))
	}
	rollbackState := func(cause error) (model.Transaction, error) {
		var recoveryErr error
		if err := os.Rename(target, source); err != nil {
			recoveryErr = fmt.Errorf("restore filesystem rollback failed: %w", err)
		}
		var previous model.SourcesLock
		if err := json.Unmarshal(previousLock, &previous); err != nil {
			recoveryErr = errors.Join(recoveryErr, fmt.Errorf("decode prior source lock: %w", err))
		} else if err := m.store.SaveLock(previous); err != nil {
			recoveryErr = errors.Join(recoveryErr, fmt.Errorf("restore prior source lock: %w", err))
		}
		if recoveryErr != nil {
			tx.RecoveryStatus = "required"
			cause = errors.Join(cause, recoveryErr)
		} else {
			tx.RecoveryStatus = "completed"
		}
		return m.fail(tx, cause)
	}
	snapshotPath := filepath.Join(m.Config.Paths.BackupsRoot, "_transactions", transactionID, "sources.lock.json")
	snapshotData, err := os.ReadFile(snapshotPath)
	if err != nil {
		return rollbackState(fmt.Errorf("load source snapshot: %w", err))
	}
	var snapshot model.SourcesLock
	if err := json.Unmarshal(snapshotData, &snapshot); err != nil {
		return rollbackState(fmt.Errorf("parse source snapshot: %w", err))
	}
	restoredMapping := false
	for id, originalPackage := range snapshot.Packages {
		if originalPackage.RootID != "" && originalPackage.RootID != root.ID {
			continue
		}
		originalSkill, ok := originalPackage.Skills[name]
		if !ok {
			continue
		}
		pkg := current.Packages[id]
		if pkg.Skills == nil {
			pkg = originalPackage
			pkg.Skills = map[string]model.SkillLock{}
		}
		pkg.Skills[name] = originalSkill
		pkg.UpdatedAt = time.Now().UTC()
		current.Packages[id] = pkg
		restoredMapping = true
		break
	}
	if !restoredMapping {
		return rollbackState(fmt.Errorf("source mapping for %s was not found in transaction %s", name, transactionID))
	}
	if err := m.store.SaveLock(current); err != nil {
		return rollbackState(err)
	}
	tx.Status, tx.CompletedAt = "completed", time.Now().UTC()
	if err := m.store.SaveTransaction(tx); err != nil {
		return rollbackState(fmt.Errorf("complete restore transaction: %w", err))
	}
	m.recordTransaction(tx)
	return tx, nil
}

func (m *Manager) Rollback(transactionID string) (model.Transaction, error) {
	original, err := m.store.Transaction(transactionID)
	if err != nil {
		return model.Transaction{}, err
	}
	if original.Type == "assisted-install" {
		return m.rollbackAssistedInstall(original)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	root, err := m.resolveWritableRoot(original.RootID)
	if err != nil {
		return model.Transaction{}, err
	}
	if original.Type != "install" && original.Type != "adopt" && original.Type != "manage" &&
		!strings.HasPrefix(original.Type, "group-") {
		return model.Transaction{}, fmt.Errorf("transaction type %s cannot be rolled back; use its dedicated recovery action", original.Type)
	}
	type installRollbackTarget struct {
		name         string
		target       string
		backup       string
		failed       string
		targetExists bool
		backupExists bool
		targetMoved  bool
		backupMoved  bool
	}
	installTargets := make([]*installRollbackTarget, 0, len(original.Targets))
	var previousLock model.SourcesLock
	var rollbackLock model.SourcesLock
	if original.Type == "install" {
		if len(original.Targets) == 0 {
			return model.Transaction{}, errors.New("install transaction has no rollback targets")
		}
		seen := map[string]bool{}
		for _, name := range original.Targets {
			if !validMutableSkillName(name) || seen[strings.ToLower(name)] {
				return model.Transaction{}, fmt.Errorf("invalid rollback target: %s", name)
			}
			seen[strings.ToLower(name)] = true
			target := filepath.Join(root.Path, name)
			targetInfo, targetErr := os.Stat(target)
			targetExists := targetErr == nil
			if targetErr != nil && !os.IsNotExist(targetErr) {
				return model.Transaction{}, fmt.Errorf("inspect rollback target %s: %w", name, targetErr)
			}
			if targetExists && !targetInfo.IsDir() {
				return model.Transaction{}, fmt.Errorf("rollback target is not a directory: %s", name)
			}
			recordedBackup := transactionPathForName(original, name)
			backup := recordedBackup
			if backup == "" {
				backup = transactionContentPath(
					root.Path, m.Config.Paths.BackupsRoot, ".csm-backups", name, transactionID,
				)
			}
			backupInfo, backupErr := os.Stat(backup)
			backupExists := backupErr == nil
			if backupErr != nil && !os.IsNotExist(backupErr) {
				return model.Transaction{}, fmt.Errorf("inspect rollback backup for %s: %w", name, backupErr)
			}
			if recordedBackup != "" && os.IsNotExist(backupErr) {
				return model.Transaction{}, fmt.Errorf("recorded rollback backup is missing for %s", name)
			}
			if backupExists {
				if !backupInfo.IsDir() {
					return model.Transaction{}, fmt.Errorf("rollback backup is not a directory: %s", name)
				}
				if err := ensureWithinRestoreRoots(root.Path, m.Config.Paths.BackupsRoot, ".csm-backups", backup); err != nil {
					return model.Transaction{}, fmt.Errorf("invalid rollback backup for %s: %w", name, err)
				}
			}
			installTargets = append(installTargets, &installRollbackTarget{
				name: name, target: target, backup: backup,
				targetExists: targetExists, backupExists: backupExists,
			})
		}
		var err error
		previousLock, err = m.store.LoadLock()
		if err != nil {
			return model.Transaction{}, fmt.Errorf("load current source lock before rollback: %w", err)
		}
		lockSnapshot := filepath.Join(m.Config.Paths.BackupsRoot, "_transactions", transactionID, "sources.lock.json")
		data, err := os.ReadFile(lockSnapshot)
		if err != nil {
			return model.Transaction{}, fmt.Errorf("load rollback source snapshot: %w", err)
		}
		if err := json.Unmarshal(data, &rollbackLock); err != nil {
			return model.Transaction{}, fmt.Errorf("parse rollback source snapshot: %w", err)
		}
	}
	tx := model.Transaction{
		ID: "tx-" + time.Now().UTC().Format("20060102T150405.000000000"), RootID: root.ID,
		Type: "rollback", Status: "running", Targets: append([]string(nil), original.Targets...),
		StartedAt: time.Now().UTC(),
	}
	if err := m.store.SaveTransaction(tx); err != nil {
		return model.Transaction{}, fmt.Errorf("start rollback transaction: %w", err)
	}
	if original.Type == "install" {
		for _, target := range installTargets {
			target.failed = transactionContentPath(
				root.Path, m.Config.Paths.QuarantineRoot, ".csm-quarantine", target.name, "rollback-"+tx.ID,
			)
			if target.targetExists {
				if err := os.MkdirAll(filepath.Dir(target.failed), 0o700); err != nil {
					return m.fail(tx, fmt.Errorf("prepare rollback quarantine for %s: %w", target.name, err))
				}
			}
			if target.backupExists {
				if err := os.MkdirAll(filepath.Dir(target.target), 0o700); err != nil {
					return m.fail(tx, fmt.Errorf("prepare rollback target for %s: %w", target.name, err))
				}
			}
		}
	}
	if strings.HasPrefix(original.Type, "group-") {
		previousLayout, err := m.store.LoadGroupLayout()
		if err != nil {
			return m.fail(tx, fmt.Errorf("load current group layout before rollback: %w", err))
		}
		path := filepath.Join(m.Config.Paths.BackupsRoot, "_transactions", transactionID, "groups.json")
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return m.fail(tx, readErr)
		}
		var layout model.GroupLayoutState
		if err := json.Unmarshal(data, &layout); err != nil {
			return m.fail(tx, err)
		}
		layout = mergeGroupLayoutRoot(previousLayout, layout, root.ID)
		if err := m.store.ReplaceGroupLayout(layout); err != nil {
			return m.fail(tx, err)
		}
		tx.Status, tx.CompletedAt = "completed", time.Now().UTC()
		tx.RecoveryStatus = "completed"
		if err := m.store.SaveTransaction(tx); err != nil {
			cause := fmt.Errorf("complete group rollback transaction: %w", err)
			if recoveryErr := m.store.ReplaceGroupLayout(previousLayout); recoveryErr != nil {
				tx.RecoveryStatus = "required"
				cause = errors.Join(cause, fmt.Errorf("restore current group layout: %w", recoveryErr))
			}
			return m.fail(tx, cause)
		}
		m.recordTransaction(tx)
		return tx, nil
	}
	if original.Type == "adopt" || original.Type == "manage" {
		previousLock, err := m.store.LoadLock()
		if err != nil {
			return m.fail(tx, fmt.Errorf("load current source lock before rollback: %w", err))
		}
		lockSnapshot := filepath.Join(m.Config.Paths.BackupsRoot, "_transactions", transactionID, "sources.lock.json")
		data, readErr := os.ReadFile(lockSnapshot)
		if readErr != nil {
			return m.fail(tx, readErr)
		}
		var lock model.SourcesLock
		if err := json.Unmarshal(data, &lock); err != nil {
			return m.fail(tx, err)
		}
		lock = mergeLockRoot(previousLock, lock, root.ID)
		if err := m.store.SaveLock(lock); err != nil {
			return m.fail(tx, err)
		}
		if err := m.restoreSecurityStates(transactionID); err != nil {
			_ = m.store.SaveLock(previousLock)
			return m.fail(tx, fmt.Errorf("restore security state snapshot: %w", err))
		}
		tx.Status, tx.CompletedAt = "completed", time.Now().UTC()
		tx.RecoveryStatus = "completed"
		if err := m.store.SaveTransaction(tx); err != nil {
			cause := fmt.Errorf("complete source rollback transaction: %w", err)
			if recoveryErr := m.store.SaveLock(previousLock); recoveryErr != nil {
				tx.RecoveryStatus = "required"
				cause = errors.Join(cause, fmt.Errorf("restore current source lock: %w", recoveryErr))
			}
			return m.fail(tx, cause)
		}
		m.recordTransaction(tx)
		return tx, nil
	}
	rollbackInstall := func(cause error, restoreLock bool) (model.Transaction, error) {
		var recoveryErr error
		for index := len(installTargets) - 1; index >= 0; index-- {
			target := installTargets[index]
			if target.backupMoved {
				if err := os.Rename(target.target, target.backup); err != nil {
					recoveryErr = errors.Join(recoveryErr, fmt.Errorf("return backup for %s: %w", target.name, err))
				}
			}
			if target.targetMoved {
				if err := os.Rename(target.failed, target.target); err != nil {
					recoveryErr = errors.Join(recoveryErr, fmt.Errorf("restore current target %s: %w", target.name, err))
				}
			}
		}
		if restoreLock {
			if err := m.store.SaveLock(previousLock); err != nil {
				recoveryErr = errors.Join(recoveryErr, fmt.Errorf("restore current source lock: %w", err))
			}
		}
		if recoveryErr != nil {
			tx.RecoveryStatus = "required"
			cause = errors.Join(cause, recoveryErr)
		} else {
			tx.RecoveryStatus = "completed"
		}
		return m.fail(tx, cause)
	}
	for _, target := range installTargets {
		if target.targetExists {
			if err := os.Rename(target.target, target.failed); err != nil {
				return rollbackInstall(fmt.Errorf("quarantine current target %s: %w", target.name, err), false)
			}
			target.targetMoved = true
			tx.BackupPaths = append(tx.BackupPaths, target.failed)
		}
		if target.backupExists {
			if err := os.Rename(target.backup, target.target); err != nil {
				return rollbackInstall(fmt.Errorf("restore backup for %s: %w", target.name, err), false)
			}
			target.backupMoved = true
		}
		if err := m.store.SaveTransaction(tx); err != nil {
			return rollbackInstall(fmt.Errorf("checkpoint rollback for %s: %w", target.name, err), false)
		}
	}
	rollbackLock = mergeLockRoot(previousLock, rollbackLock, root.ID)
	if err := m.store.SaveLock(rollbackLock); err != nil {
		return rollbackInstall(fmt.Errorf("restore rollback source snapshot: %w", err), true)
	}
	if err := m.restoreSecurityStates(transactionID); err != nil {
		return rollbackInstall(fmt.Errorf("restore security state snapshot: %w", err), true)
	}
	tx.Status, tx.CompletedAt = "completed", time.Now().UTC()
	tx.RecoveryStatus = "completed"
	if err := m.store.SaveTransaction(tx); err != nil {
		return rollbackInstall(fmt.Errorf("complete rollback transaction: %w", err), true)
	}
	m.recordTransaction(tx)
	return tx, nil
}

func packageRootID(key string, pkg model.PackageLock) string {
	if pkg.RootID != "" {
		return pkg.RootID
	}
	if index := strings.IndexByte(key, '\x00'); index > 0 {
		return key[:index]
	}
	return model.RootIDCodexDefault
}

func mergeLockRoot(current, snapshot model.SourcesLock, rootID string) model.SourcesLock {
	merged := current
	merged.Packages = make(map[string]model.PackageLock, len(current.Packages)+len(snapshot.Packages))
	for key, pkg := range current.Packages {
		merged.Packages[key] = pkg
	}
	for key, pkg := range merged.Packages {
		if packageRootID(key, pkg) == rootID {
			delete(merged.Packages, key)
		}
	}
	for key, pkg := range snapshot.Packages {
		if packageRootID(key, pkg) == rootID {
			merged.Packages[key] = pkg
		}
	}
	merged.SchemaVersion = model.SourcesLockSchemaVersion
	return merged
}

func mergeGroupLayoutRoot(current, snapshot model.GroupLayoutState, rootID string) model.GroupLayoutState {
	rootOf := func(value string) string {
		if value == "" {
			return model.RootIDCodexDefault
		}
		return value
	}
	merged := model.GroupLayoutState{Groups: []model.GroupPreference{}, Assignments: []model.SkillGroupAssignment{}}
	for _, group := range current.Groups {
		if rootOf(group.RootID) != rootID {
			merged.Groups = append(merged.Groups, group)
		}
	}
	for _, group := range snapshot.Groups {
		if rootOf(group.RootID) == rootID {
			group.RootID = rootID
			merged.Groups = append(merged.Groups, group)
		}
	}
	for _, assignment := range current.Assignments {
		if rootOf(assignment.RootID) != rootID {
			merged.Assignments = append(merged.Assignments, assignment)
		}
	}
	for _, assignment := range snapshot.Assignments {
		if rootOf(assignment.RootID) == rootID {
			assignment.RootID = rootID
			merged.Assignments = append(merged.Assignments, assignment)
		}
	}
	return merged
}

func (m *Manager) BootstrapCurrentSkills() error {
	graph := map[string]string{
		"build-graph": "skills/build-graph", "debug-issue": "skills/debug-issue",
		"explore-codebase": "skills/explore-codebase", "refactor-safely": "skills/refactor-safely",
		"review-changes": "skills/review-changes", "review-delta": "skills/review-delta", "review-pr": "skills/review-pr",
	}
	career := map[string]string{
		"job-hunt": "skills/job-hunt", "resume-match": "skills/resume-match",
		"resume-craft": "skills/resume-craft", "cover-letter": "skills/cover-letter",
		"mock-interview": "skills/mock-interview", "offer-decision": "skills/offer-decision",
	}
	if existingMappings(m.Config.Paths.SkillsRoot, graph) {
		if err := m.AdoptPackage("tirth8205/code-review-graph", "https://github.com/tirth8205/code-review-graph", "main", "", graph, model.RootIDCodexDefault); err != nil {
			return err
		}
	}
	if existingMappings(m.Config.Paths.SkillsRoot, career) {
		if err := m.AdoptPackage("rebecha1227-a11y/CareerForge", "https://github.com/rebecha1227-a11y/CareerForge", "main", "f21dc27d1820bfdc67bc4c22b1f20cc2028692d2", career, model.RootIDCodexDefault); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) recordScan(report model.ScanReport) {
	_, _, _ = reporting.WriteScan(m.Config.Paths.ReportsRoot, report)
	_ = reporting.AppendEvent(m.Config.Paths.LogsRoot, "scan", report)
}

func (m *Manager) recordTransaction(tx model.Transaction) {
	_, _, _ = reporting.WriteTransaction(m.Config.Paths.ReportsRoot, tx)
	_ = reporting.AppendEvent(m.Config.Paths.LogsRoot, "transaction", tx)
}

func (m *Manager) fail(tx model.Transaction, err error) (model.Transaction, error) {
	tx.Status, tx.Error, tx.CompletedAt = "failed", err.Error(), time.Now().UTC()
	if saveErr := m.store.SaveTransaction(tx); saveErr != nil {
		err = errors.Join(err, fmt.Errorf("persist failed transaction %s: %w", tx.ID, saveErr))
		tx.Error = err.Error()
	}
	m.recordTransaction(tx)
	return tx, err
}

func (m *Manager) failInstall(tx model.Transaction, root model.SkillRoot, touched []string, backups map[string]string, cause error) (model.Transaction, error) {
	var rollbackErr error
	for i := len(touched) - 1; i >= 0; i-- {
		name := touched[i]
		target := filepath.Join(root.Path, name)
		if _, err := os.Stat(target); err == nil {
			failed := transactionContentPath(
				root.Path, m.Config.Paths.QuarantineRoot, ".csm-quarantine", name, "failed-"+tx.ID,
			)
			if err := os.MkdirAll(filepath.Dir(failed), 0o700); err != nil {
				if rollbackErr == nil {
					rollbackErr = err
				}
			} else if err := os.Rename(target, failed); err != nil && rollbackErr == nil {
				rollbackErr = err
			}
		}
		if backup := backups[name]; backup != "" {
			if _, err := os.Stat(backup); err == nil {
				if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
					if rollbackErr == nil {
						rollbackErr = err
					}
				} else if err := os.Rename(backup, target); err != nil && rollbackErr == nil {
					rollbackErr = err
				}
			}
		}
	}
	if err := m.restoreLockSnapshot(tx.ID); err != nil && rollbackErr == nil {
		rollbackErr = err
	}
	if err := m.restoreSecurityStates(tx.ID); err != nil && rollbackErr == nil {
		rollbackErr = err
	}
	if rollbackErr != nil {
		tx.RecoveryStatus = "required"
		cause = fmt.Errorf("%w; install rollback failed: %v", cause, rollbackErr)
	} else {
		tx.RecoveryStatus = "completed"
	}
	return m.fail(tx, cause)
}

func (m *Manager) restoreLockSnapshot(transactionID string) error {
	path := filepath.Join(m.Config.Paths.BackupsRoot, "_transactions", transactionID, "sources.lock.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var lock model.SourcesLock
	if err := json.Unmarshal(data, &lock); err != nil {
		return err
	}
	return m.store.SaveLock(lock)
}

func (m *Manager) snapshotLock(transactionID string) error {
	lock, err := m.store.LoadLock()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(m.Config.Paths.BackupsRoot, "_transactions", transactionID, "sources.lock.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

type securityStateSnapshot struct {
	RootID string                     `json:"rootId"`
	Names  []string                   `json:"names"`
	States []model.SkillSecurityState `json:"states"`
}

func (m *Manager) snapshotSecurityStates(transactionID, rootID string, names []string) (string, error) {
	current, err := m.store.SkillSecurityStates()
	if err != nil {
		return "", err
	}
	snapshot := securityStateSnapshot{RootID: rootID, Names: uniqueNonEmpty(names), States: []model.SkillSecurityState{}}
	for _, name := range snapshot.Names {
		if value, ok := current[model.QualifiedSkillIdentity(rootID, name)]; ok {
			snapshot.States = append(snapshot.States, value)
		}
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(m.Config.Paths.BackupsRoot, "_transactions", transactionID, "security-states.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func (m *Manager) restoreSecurityStates(transactionID string) error {
	path := filepath.Join(m.Config.Paths.BackupsRoot, "_transactions", transactionID, "security-states.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil // Transactions from releases before 0.11 have no state snapshot.
	}
	if err != nil {
		return err
	}
	var snapshot securityStateSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return err
	}
	return m.store.ReplaceSkillSecurityStates(snapshot.RootID, snapshot.Names, snapshot.States)
}

func (m *Manager) snapshotGroupLayout(transactionID string) (string, error) {
	layout, err := m.store.LoadGroupLayout()
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(layout, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(m.Config.Paths.BackupsRoot, "_transactions", transactionID, "groups.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func chooseCandidates(all []model.CandidateSkill, selected []string) []model.CandidateSkill {
	want := map[string]bool{}
	for _, name := range selected {
		want[name] = true
	}
	var out []model.CandidateSkill
	for _, c := range all {
		if want[c.Name] {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func copyTree(source, target string) error {
	return filepath.WalkDir(source, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(target, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic link is forbidden: %s", path)
		}
		if d.IsDir() {
			return os.MkdirAll(dest, 0o700)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			in.Close()
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeOut := out.Close()
		closeIn := in.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeOut != nil {
			return closeOut
		}
		return closeIn
	})
}

func transactionContentPath(skillsRoot, configuredRoot, localFallback, name, transactionID string) string {
	if sameFilesystemVolume(skillsRoot, configuredRoot) {
		return filepath.Join(configuredRoot, name, transactionID, "content")
	}
	return filepath.Join(skillsRoot, localFallback, name, transactionID, "content")
}

func sameFilesystemVolume(a, b string) bool {
	volumeA := filepath.VolumeName(filepath.Clean(a))
	volumeB := filepath.VolumeName(filepath.Clean(b))
	if volumeA == "" && volumeB == "" {
		return true
	}
	return strings.EqualFold(volumeA, volumeB)
}

func transactionPathForName(tx model.Transaction, name string) string {
	for _, path := range tx.BackupPaths {
		if strings.EqualFold(filepath.Base(filepath.Dir(filepath.Dir(path))), name) {
			return path
		}
	}
	return ""
}

func validMutableSkillName(name string) bool {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name ||
		strings.ContainsAny(name, `*?/\\<>:"|`) || strings.IndexFunc(name, func(r rune) bool { return r < 0x20 }) >= 0 {
		return false
	}
	// Win32 canonicalizes trailing spaces and dots. Reject aliases such as
	// ".system." and ".system " before they reach any filesystem mutation.
	canonical := strings.TrimRight(name, " .")
	return canonical != "" && canonical == name && !strings.EqualFold(canonical, ".system")
}

func ensureWithinRestoreRoots(skillsRoot, configuredRoot, localFallback, candidate string) error {
	if strings.TrimSpace(candidate) == "" || !filepath.IsAbs(candidate) {
		return errors.New("restore source must be an absolute path")
	}
	if err := ensureResolvedWithinOrEqual(configuredRoot, candidate); err == nil {
		return nil
	}
	return ensureResolvedWithinOrEqual(filepath.Join(skillsRoot, localFallback), candidate)
}

func ensureResolvedWithinOrEqual(root, candidate string) error {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	resolvedCandidate, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return err
	}
	return ensureWithinOrEqual(resolvedRoot, resolvedCandidate)
}

func ensureWithinOrEqual(root, candidate string) error {
	base, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	target, err := filepath.Abs(candidate)
	if err != nil {
		return err
	}
	if strings.EqualFold(base, target) || strings.HasPrefix(strings.ToLower(target), strings.ToLower(base)+string(filepath.Separator)) {
		return nil
	}
	return errors.New("target is outside the configured skills root")
}

func savePreview(root string, p model.InstallPreview) error {
	if err := sealInstallPreview(&p); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, p.ID+".json"), data, 0o600)
}

func loadPreview(root, id string) (model.InstallPreview, error) {
	if filepath.Base(id) != id || !strings.HasPrefix(id, "plan-") {
		return model.InstallPreview{}, errors.New("invalid plan ID")
	}
	data, err := os.ReadFile(filepath.Join(root, id+".json"))
	if err != nil {
		return model.InstallPreview{}, err
	}
	var p model.InstallPreview
	if err := json.Unmarshal(data, &p); err != nil {
		return model.InstallPreview{}, err
	}
	if p.ID != id {
		return model.InstallPreview{}, errors.New("install preview identity mismatch")
	}
	expected, err := installPreviewDigest(p)
	if err != nil {
		return model.InstallPreview{}, err
	}
	if p.PreviewDigest == "" || !strings.EqualFold(p.PreviewDigest, expected) {
		return model.InstallPreview{}, errors.New("install preview integrity digest mismatch")
	}
	return p, nil
}

func saveAdoptionPreview(root string, preview model.AdoptionPreview) error {
	if err := sealAdoptionPreview(&preview); err != nil {
		return err
	}
	data, err := json.MarshalIndent(preview, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, preview.ID+".json"), data, 0o600)
}

func loadAdoptionPreview(root, id string) (model.AdoptionPreview, error) {
	if filepath.Base(id) != id || !strings.HasPrefix(id, "adopt-plan-") {
		return model.AdoptionPreview{}, errors.New("invalid adoption plan ID")
	}
	data, err := os.ReadFile(filepath.Join(root, id+".json"))
	if err != nil {
		return model.AdoptionPreview{}, err
	}
	var preview model.AdoptionPreview
	if err = json.Unmarshal(data, &preview); err != nil {
		return model.AdoptionPreview{}, err
	}
	if err := verifyAdoptionPreview(preview, id); err != nil {
		return model.AdoptionPreview{}, err
	}
	return preview, nil
}

func adoptionPreviewDigest(preview model.AdoptionPreview) (string, error) {
	preview.PreviewDigest = ""
	data, err := json.Marshal(preview)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return fmt.Sprintf("%x", digest[:]), nil
}

func sealAdoptionPreview(preview *model.AdoptionPreview) error {
	digest, err := adoptionPreviewDigest(*preview)
	if err != nil {
		return err
	}
	preview.PreviewDigest = digest
	return nil
}

func verifyAdoptionPreview(preview model.AdoptionPreview, expectedID string) error {
	if preview.ID != expectedID || filepath.Base(preview.ID) != preview.ID || !strings.HasPrefix(preview.ID, "adopt-plan-") {
		return errors.New("adoption preview identity mismatch")
	}
	digest, err := adoptionPreviewDigest(preview)
	if err != nil {
		return err
	}
	if preview.PreviewDigest == "" || !strings.EqualFold(preview.PreviewDigest, digest) {
		return errors.New("adoption preview integrity digest mismatch")
	}
	return nil
}

func (m *Manager) decorateScan(report model.ScanReport, ignored map[string]string) model.ScanReport {
	if report.Findings == nil {
		report.Findings = []model.Finding{}
	} else {
		// Install previews are digest-bound value objects, but their slices share
		// backing arrays after a struct copy. Decoration must never mutate the
		// persisted/in-memory preview that will be verified on a later attempt.
		report.Findings = append([]model.Finding(nil), report.Findings...)
	}
	if report.Skills == nil {
		report.Skills = []model.ScanSkillSummary{}
	}
	type clusterBuild struct {
		cluster model.RiskCluster
		files   map[string]bool
	}
	builds := map[string]*clusterBuild{}
	for i := range report.Findings {
		finding := &report.Findings[i]
		if finding.Title == "" {
			finding.Title = finding.RuleID
		}
		canonicalFile := filepath.ToSlash(finding.File)
		if rel, err := filepath.Rel(m.Config.Paths.SkillsRoot, report.Target); err == nil && rel != "." && rel != "" && !strings.HasPrefix(rel, "..") {
			canonicalFile = filepath.ToSlash(filepath.Join(rel, filepath.FromSlash(finding.File)))
		}
		identity := strings.ToLower(strings.Join([]string{
			report.RootID, finding.RuleID, canonicalFile, strings.TrimSpace(finding.Evidence),
		}, "\x00"))
		if finding.Evidence == "" {
			identity += fmt.Sprintf("\x00%d", finding.Line)
		}
		fingerprint := sha256.Sum256([]byte(identity))
		finding.Fingerprint = fmt.Sprintf("%x", fingerprint)
		if finding.FileClass == "" {
			finding.FileClass = classifyFindingFile(finding.File)
		}
		if finding.Category == "" {
			finding.Category = findingCategory(finding.RuleID)
		}
		if finding.RuleID == "CSM-FS-001" || finding.RuleID == "CSM-FILE-002" || finding.RuleID == "CSM-DEL-001" {
			finding.Deterministic = true
		}
		clusterParts := []string{
			report.RootID, finding.RuleID, finding.Category, finding.FileClass, fmt.Sprintf("%t", finding.Deterministic),
		}
		if finding.SkillName != "" || finding.GroupID != "" {
			clusterParts = append([]string{finding.GroupID, finding.SkillName}, clusterParts...)
		}
		clusterIdentity := strings.ToLower(strings.Join(clusterParts, "\x00"))
		clusterHash := sha256.Sum256([]byte(clusterIdentity))
		finding.ClusterID = fmt.Sprintf("risk-%x", clusterHash[:12])
		finding.Ignored = false
		finding.IgnoreReason = ""
		if reason, ok := ignored[finding.Fingerprint]; ok && persistedIgnoreHonored(finding.Severity, reason) {
			finding.Ignored = true
			finding.IgnoreReason = reason
		}
		build := builds[finding.ClusterID]
		if build == nil {
			build = &clusterBuild{
				cluster: model.RiskCluster{
					ID: finding.ClusterID, RuleID: finding.RuleID, Title: finding.Title,
					Severity: finding.Severity, Category: finding.Category, FileClass: finding.FileClass,
					Deterministic: finding.Deterministic, AffectedFiles: []string{},
					Fingerprints: []string{}, SampleFindings: []model.Finding{},
					SkillName: finding.SkillName, GroupID: finding.GroupID, GroupName: finding.GroupName,
				},
				files: map[string]bool{},
			}
			builds[finding.ClusterID] = build
		}
		if riskRank(finding.Severity) > riskRank(build.cluster.Severity) {
			build.cluster.Severity = finding.Severity
		}
		build.cluster.FindingCount++
		build.cluster.Fingerprints = append(build.cluster.Fingerprints, finding.Fingerprint)
		if !build.files[finding.File] {
			build.files[finding.File] = true
			build.cluster.AffectedFiles = append(build.cluster.AffectedFiles, finding.File)
		}
		if len(build.cluster.SampleFindings) < m.Config.CodexReview.MaxSamplePerRisk {
			build.cluster.SampleFindings = append(build.cluster.SampleFindings, *finding)
		}
	}
	report.Clusters = make([]model.RiskCluster, 0, len(builds))
	report.ActiveFindingCount = 0
	report.IgnoredFindingCount = 0
	report.ActiveHighestSeverity = model.RiskInfo
	for _, build := range builds {
		sort.Strings(build.cluster.AffectedFiles)
		sort.Strings(build.cluster.Fingerprints)
		build.cluster.Ignored = true
		for _, fingerprint := range build.cluster.Fingerprints {
			reason, ok := ignored[fingerprint]
			if !ok || !persistedIgnoreHonored(build.cluster.Severity, reason) {
				build.cluster.Ignored = false
				break
			}
		}
		if build.cluster.Ignored {
			report.IgnoredFindingCount++
			if len(build.cluster.SampleFindings) > 0 {
				build.cluster.IgnoreReason = build.cluster.SampleFindings[0].IgnoreReason
			}
		} else {
			report.ActiveFindingCount++
			if riskRank(build.cluster.Severity) > riskRank(report.ActiveHighestSeverity) {
				report.ActiveHighestSeverity = build.cluster.Severity
			}
		}
		report.Clusters = append(report.Clusters, build.cluster)
	}
	sort.Slice(report.Clusters, func(i, j int) bool {
		if report.Clusters[i].GroupName != report.Clusters[j].GroupName {
			return report.Clusters[i].GroupName < report.Clusters[j].GroupName
		}
		if report.Clusters[i].SkillName != report.Clusters[j].SkillName {
			return report.Clusters[i].SkillName < report.Clusters[j].SkillName
		}
		if report.Clusters[i].Severity != report.Clusters[j].Severity {
			return riskRank(report.Clusters[i].Severity) > riskRank(report.Clusters[j].Severity)
		}
		if report.Clusters[i].RuleID != report.Clusters[j].RuleID {
			return report.Clusters[i].RuleID < report.Clusters[j].RuleID
		}
		return report.Clusters[i].FileClass < report.Clusters[j].FileClass
	})
	return report
}

func classifyFindingFile(path string) string {
	clean := strings.ToLower(filepath.ToSlash(path))
	base := strings.ToLower(filepath.Base(clean))
	switch {
	case base == "skill.md" || base == "agents.md" || strings.Contains(clean, "/agents/"):
		return "instruction"
	case strings.Contains(clean, "/test/") || strings.Contains(clean, "/tests/") ||
		strings.Contains(clean, "/fixtures/") || strings.HasPrefix(clean, "test/") ||
		strings.HasPrefix(clean, "tests/") || strings.HasPrefix(clean, "fixtures/"):
		return "test"
	case strings.Contains(clean, "/docs/") || strings.Contains(clean, "/examples/") ||
		strings.HasPrefix(clean, "docs/") || strings.HasPrefix(clean, "examples/") ||
		strings.HasPrefix(base, "readme"):
		return "documentation"
	case strings.Contains(clean, "/scripts/") || strings.Contains(clean, "/src/") ||
		strings.HasPrefix(clean, "scripts/") || strings.HasPrefix(clean, "src/"):
		return "runtime"
	default:
		return "asset"
	}
}

func findingCategory(ruleID string) string {
	switch {
	case strings.HasPrefix(ruleID, "CSM-FS-"), strings.HasPrefix(ruleID, "CSM-FILE-"), strings.HasPrefix(ruleID, "CSM-ENC-"):
		return "filesystem"
	case strings.HasPrefix(ruleID, "CSM-CRED-"):
		return "credentials"
	case strings.HasPrefix(ruleID, "CSM-EXEC-"), strings.HasPrefix(ruleID, "CSM-DOWN-"):
		return "execution"
	case strings.HasPrefix(ruleID, "CSM-DEL-"):
		return "destructive"
	case strings.HasPrefix(ruleID, "CSM-NET-"):
		return "network"
	case strings.HasPrefix(ruleID, "CSM-PERSIST-"):
		return "persistence"
	case strings.HasPrefix(ruleID, "CSM-INJ-"):
		return "prompt-injection"
	case strings.HasPrefix(ruleID, "CSM-OBF-"):
		return "obfuscation"
	case strings.HasPrefix(ruleID, "CSM-CONFIG-"):
		return "configuration"
	default:
		return "other"
	}
}

func highestFindingSeverity(findings []model.Finding, activeOnly bool) model.RiskSeverity {
	highest := model.RiskInfo
	for _, finding := range findings {
		if activeOnly && finding.Ignored {
			continue
		}
		if riskRank(finding.Severity) > riskRank(highest) {
			highest = finding.Severity
		}
	}
	return highest
}

func riskRank(severity model.RiskSeverity) int {
	switch severity {
	case model.RiskCritical:
		return 5
	case model.RiskHigh:
		return 4
	case model.RiskMedium:
		return 3
	case model.RiskLow:
		return 2
	case model.RiskInfo:
		return 1
	default:
		// Unknown severities outrank known blocking values so every aggregate
		// and gate fails closed instead of silently treating them as info.
		return 6
	}
}

func validRiskSeverity(severity model.RiskSeverity) bool {
	switch severity {
	case model.RiskInfo, model.RiskLow, model.RiskMedium, model.RiskHigh, model.RiskCritical:
		return true
	default:
		return false
	}
}

func isBlockingRiskSeverity(severity model.RiskSeverity) bool {
	return severity == model.RiskHigh || severity == model.RiskCritical || !validRiskSeverity(severity)
}

func persistedIgnoreHonored(severity model.RiskSeverity, reason string) bool {
	switch severity {
	case model.RiskCritical:
		return false
	case model.RiskHigh:
		return strings.TrimSpace(reason) != ""
	case model.RiskInfo, model.RiskLow, model.RiskMedium:
		return true
	default:
		return false
	}
}

func scanContainsAcceptedHighRisk(report model.ScanReport) bool {
	for _, cluster := range report.Clusters {
		if cluster.Severity == model.RiskHigh && cluster.Ignored {
			return true
		}
	}
	return false
}

func sameFileRecords(current, planned []model.FileRecord) bool {
	if len(current) != len(planned) {
		return false
	}
	expected := map[string]string{}
	for _, file := range planned {
		expected[file.Path] = file.SHA256
	}
	for _, file := range current {
		if expected[file.Path] != file.SHA256 {
			return false
		}
	}
	return true
}

func findManaged(lock model.SourcesLock, name string) (model.PackageLock, bool) {
	for _, pkg := range lock.Packages {
		if _, ok := pkg.Skills[name]; ok {
			return pkg, true
		}
	}
	return model.PackageLock{}, false
}

func findManagedInRoot(lock model.SourcesLock, rootID, name string) (model.PackageLock, bool) {
	for _, pkg := range lock.Packages {
		if pkg.RootID != "" && pkg.RootID != rootID {
			continue
		}
		if _, ok := pkg.Skills[name]; ok {
			return pkg, true
		}
	}
	return model.PackageLock{}, false
}

func uniqueNonEmpty(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range in {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func applySkillSecurityState(
	skills []model.Skill,
	lock model.SourcesLock,
	states map[string]model.SkillSecurityState,
) {
	legacy := map[string]struct {
		reportID string
		checked  time.Time
	}{}
	for _, pkg := range lock.Packages {
		rootID := pkg.RootID
		if rootID == "" {
			rootID = model.RootIDCodexDefault
		}
		checked := pkg.UpdatedAt
		if checked.IsZero() {
			checked = pkg.InstalledAt
		}
		for name, skill := range pkg.Skills {
			if skill.LastScanReport != "" {
				legacy[model.QualifiedSkillIdentity(rootID, name)] = struct {
					reportID string
					checked  time.Time
				}{skill.LastScanReport, checked}
			}
		}
	}
	for index := range skills {
		skill := &skills[index]
		if skill.System {
			continue
		}
		currentHash := skillContentHash(skill.Files)
		state, ok := states[model.QualifiedSkillIdentity(skill.RootID, skill.Name)]
		if !ok {
			state, ok = states[skill.Name] // v1 compatibility
		}
		if ok {
			checkedAt := state.CheckedAt
			skill.LastSecurityScan = &checkedAt
			skill.SecurityChanged = state.ContentHash != currentHash
			if skill.SecurityChanged {
				skill.SecurityStatus = "changed"
			} else {
				skill.SecurityStatus = "checked"
			}
			continue
		}
		if previous, ok := legacy[model.QualifiedSkillIdentity(skill.RootID, skill.Name)]; ok && !skill.LocalModified {
			checkedAt := previous.checked
			skill.LastSecurityScan = &checkedAt
			skill.SecurityStatus = "checked"
			continue
		}
		skill.SecurityStatus = "not-scanned"
	}
}

func skillContentHash(files []model.FileRecord) string {
	hash := sha256.New()
	for _, file := range files {
		_, _ = io.WriteString(hash, file.Path)
		_, _ = io.WriteString(hash, "\x00")
		_, _ = io.WriteString(hash, file.SHA256)
		_, _ = io.WriteString(hash, "\x00")
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func summarizeScanSkills(report *model.ScanReport) {
	for index := range report.Skills {
		summary := &report.Skills[index]
		summary.HighestSeverity = model.RiskInfo
		for _, cluster := range report.Clusters {
			if cluster.SkillName != summary.SkillName {
				continue
			}
			if riskRank(cluster.Severity) > riskRank(summary.HighestSeverity) {
				summary.HighestSeverity = cluster.Severity
			}
			if cluster.Ignored {
				summary.IgnoredFindingCount++
			} else {
				summary.ActiveFindingCount++
			}
		}
	}
}

func scanCheckedAt(report model.ScanReport) time.Time {
	if !report.CompletedAt.IsZero() {
		return report.CompletedAt
	}
	if !report.StartedAt.IsZero() {
		return report.StartedAt
	}
	return time.Now().UTC()
}

func existingMappings(root string, mappings map[string]string) bool {
	for name := range mappings {
		if _, err := os.Stat(filepath.Join(root, name, "SKILL.md")); err != nil {
			return false
		}
	}
	return true
}
