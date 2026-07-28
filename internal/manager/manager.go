package manager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	previews   map[string]model.InstallPreview
	adoptions  map[string]model.AdoptionPreview
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
	store, err := state.Open(cfg.Paths.DataRoot)
	if err != nil {
		return nil, err
	}
	return &Manager{
		Config: cfg, ConfigPath: configPath, store: store,
		github: githubsource.New(), previews: map[string]model.InstallPreview{},
		adoptions: map[string]model.AdoptionPreview{},
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

func (m *Manager) ReviewScanWithCodex(ctx context.Context, report model.ScanReport) (model.ScanReport, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.Config.CodexReview.Enabled {
		return report, errors.New("Codex 辅助复核未启用；请先在设置中启用")
	}
	if len(report.Clusters) == 0 {
		return report, errors.New("当前扫描没有可复核的风险簇")
	}
	tx := model.Transaction{
		ID:   "tx-" + time.Now().UTC().Format("20060102T150405.000000000"),
		Type: "codex-risk-review", Status: "running",
		Targets: []string{report.ID}, StartedAt: time.Now().UTC(),
	}
	_ = m.store.SaveTransaction(tx)
	review, err := codexreview.Review(ctx, m.Config.CodexReview, report, m.Config.Paths.StagingRoot)
	report.CodexReview = &review
	_ = m.store.SaveScan(report)
	m.recordScan(report)
	if err != nil {
		_, failErr := m.fail(tx, err)
		return report, failErr
	}
	tx.Status, tx.CompletedAt = "completed", time.Now().UTC()
	_ = m.store.SaveTransaction(tx)
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

func (m *Manager) Dashboard() (model.Dashboard, error) {
	lock, err := m.store.LoadLock()
	if err != nil {
		return model.Dashboard{}, err
	}
	skills, sourceGroups, relations, err := inventory.Discover(m.Config.Paths.SkillsRoot, lock)
	if err != nil {
		return model.Dashboard{}, err
	}
	layout, err := m.store.LoadGroupLayout()
	if err != nil {
		return model.Dashboard{}, err
	}
	skills, groups := applyGroupLayout(skills, sourceGroups, layout)
	scans, scanErr := m.store.RecentScans(10)
	if scanErr != nil {
		return model.Dashboard{}, scanErr
	}
	latestScans, latestErr := m.store.LatestScansByTarget()
	if latestErr != nil {
		return model.Dashboard{}, latestErr
	}
	history, _ := m.store.RecentTransactions(20)
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
	}
	filteredUpdateStatuses := updateStatuses[:0]
	for _, status := range updateStatuses {
		if validSourceGroups[status.GroupID] {
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
	}
	statusByGroup := map[string]model.UpdateStatus{}
	for _, status := range updateStatuses {
		statusByGroup[status.GroupID] = status
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
		status, ok := statusByGroup[d.Skills[i].SourceGroupID]
		if !ok {
			continue
		}
		d.Skills[i].LastChecked = &status.CheckedAt
		d.Skills[i].UpdateStatus = status.Status
		if status.Status == "update-available" {
			d.Skills[i].UpdateStatus = "up-to-date"
		}
		for _, name := range status.OutdatedSkills {
			if name == d.Skills[i].Name {
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
	for _, report := range latestScans {
		if ensureWithinOrEqual(m.Config.Paths.SkillsRoot, report.Target) != nil {
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

func applyGroupLayout(skills []model.Skill, sourceGroups []model.Group, layout model.GroupLayoutState) ([]model.Skill, []model.Group) {
	preferences := map[string]model.GroupPreference{}
	for _, preference := range layout.Groups {
		preferences[preference.ID] = preference
	}
	assignments := map[string]model.SkillGroupAssignment{}
	for _, assignment := range layout.Assignments {
		assignments[assignment.SkillName] = assignment
	}
	groups := map[string]*model.Group{}
	defaultPosition := 0
	for _, source := range sourceGroups {
		group := source
		group.SkillNames = nil
		if preference, ok := preferences[group.ID]; ok {
			if strings.TrimSpace(preference.Name) != "" {
				group.Name = preference.Name
			}
			group.Position = preference.Position
		} else {
			group.Position = defaultPosition
		}
		defaultPosition++
		groups[group.ID] = &group
	}
	for _, preference := range layout.Groups {
		if !preference.Manual {
			continue
		}
		if _, exists := groups[preference.ID]; exists {
			continue
		}
		group := model.Group{
			ID: preference.ID, Name: preference.Name, Provider: "manual",
			Manual: true, Position: preference.Position, Status: "healthy",
		}
		groups[group.ID] = &group
	}
	for i := range skills {
		skill := &skills[i]
		if skill.SourceGroupID == "" {
			skill.SourceGroupID, skill.SourceGroupName = skill.GroupID, skill.GroupName
		}
		targetID := skill.SourceGroupID
		if assignment, ok := assignments[skill.Name]; ok && !skill.System {
			if _, exists := groups[assignment.GroupID]; exists {
				targetID = assignment.GroupID
			}
		}
		group, exists := groups[targetID]
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

func (m *Manager) CreateGroup(name string) (model.Transaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	name = strings.TrimSpace(name)
	if name == "" {
		return model.Transaction{}, errors.New("group name is required")
	}
	dashboard, err := m.Dashboard()
	if err != nil {
		return model.Transaction{}, err
	}
	for _, group := range dashboard.Groups {
		if strings.EqualFold(group.Name, name) {
			return model.Transaction{}, fmt.Errorf("group name already exists: %s", name)
		}
	}
	layout, err := m.store.LoadGroupLayout()
	if err != nil {
		return model.Transaction{}, err
	}
	id := "group-" + time.Now().UTC().Format("20060102T150405.000000000")
	layout.Groups = append(layout.Groups, model.GroupPreference{ID: id, Name: name, Position: len(dashboard.Groups), Manual: true})
	return m.applyGroupLayoutChange("group-create", []string{id}, layout)
}

func (m *Manager) RenameGroup(id, name string) (model.Transaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
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
		if layout.Groups[i].ID == id {
			layout.Groups[i].Name = name
			found = true
			break
		}
	}
	if !found {
		layout.Groups = append(layout.Groups, model.GroupPreference{
			ID: id, Name: name, Position: target.Position, Manual: target.Manual,
		})
	}
	return m.applyGroupLayoutChange("group-rename", []string{id}, layout)
}

func (m *Manager) ReorderGroups(ids []string) (model.Transaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	dashboard, err := m.Dashboard()
	if err != nil {
		return model.Transaction{}, err
	}
	expected := map[string]model.Group{}
	for _, group := range dashboard.Groups {
		if !group.ReadOnly {
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
	for _, preference := range layout.Groups {
		byID[preference.ID] = preference
	}
	for position, id := range ids {
		preference, exists := byID[id]
		if !exists {
			group := expected[id]
			preference = model.GroupPreference{ID: id, Name: group.Name, Manual: group.Manual}
		}
		preference.Position = position + 1
		byID[id] = preference
	}
	layout.Groups = layout.Groups[:0]
	for _, preference := range byID {
		layout.Groups = append(layout.Groups, preference)
	}
	sort.Slice(layout.Groups, func(i, j int) bool { return layout.Groups[i].Position < layout.Groups[j].Position })
	return m.applyGroupLayoutChange("group-reorder", append([]string(nil), ids...), layout)
}

func (m *Manager) MoveSkillsToGroup(names []string, groupID string) (model.Transaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
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
		if group.ID == groupID {
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
		skills[skill.Name] = skill
	}
	unique := make([]string, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		if filepath.Base(name) != name || strings.ContainsAny(name, "*?") {
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
		if !seen[assignment.SkillName] {
			filtered = append(filtered, assignment)
		}
	}
	layout.Assignments = filtered
	for position, name := range unique {
		layout.Assignments = append(layout.Assignments, model.SkillGroupAssignment{
			SkillName: name, GroupID: groupID, Position: position,
		})
	}
	return m.applyGroupLayoutChange("group-move", unique, layout)
}

func (m *Manager) applyGroupLayoutChange(kind string, targets []string, layout model.GroupLayoutState) (model.Transaction, error) {
	tx := model.Transaction{
		ID:   "tx-" + time.Now().UTC().Format("20060102T150405.000000000"),
		Type: kind, Status: "running", Targets: targets, StartedAt: time.Now().UTC(),
	}
	_ = m.store.SaveTransaction(tx)
	snapshot, err := m.snapshotGroupLayout(tx.ID)
	if err != nil {
		return m.fail(tx, err)
	}
	tx.BackupPaths = append(tx.BackupPaths, snapshot)
	if err := m.store.ReplaceGroupLayout(layout); err != nil {
		return m.fail(tx, err)
	}
	tx.Status, tx.CompletedAt = "completed", time.Now().UTC()
	_ = m.store.SaveTransaction(tx)
	m.recordTransaction(tx)
	return tx, nil
}

func (m *Manager) Audit(target string) (model.ScanReport, error) {
	if target == "" {
		target = m.Config.Paths.SkillsRoot
	}
	if err := ensureWithinOrEqual(m.Config.Paths.SkillsRoot, target); err != nil {
		return model.ScanReport{}, err
	}
	scan := scanner.Scan
	if strings.EqualFold(filepath.Clean(target), filepath.Clean(m.Config.Paths.SkillsRoot)) {
		scan = scanner.ScanSkillsRoot
	}
	report, err := scan(target, m.Config.MaxFiles, m.Config.MaxFileBytes)
	if err != nil {
		report.Status = "failed"
	}
	ignored, ignoreErr := m.store.IgnoredFindings()
	if ignoreErr != nil {
		return model.ScanReport{}, ignoreErr
	}
	report = m.decorateScan(report, ignored)
	_ = m.store.SaveScan(report)
	m.recordScan(report)
	return report, err
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
	reason = strings.TrimSpace(reason)
	if ignored && reason == "" {
		return errors.New("an ignore reason is required to record the manual review")
	}
	if ignored && (finding.RuleID == "CSM-FS-001" || finding.RuleID == "CSM-FILE-002" || finding.RuleID == "CSM-DEL-001") {
		return errors.New("deterministic safety findings must be handled as a cluster with explicit manual override confirmation")
	}
	txType := "ignore-warning"
	if !ignored {
		txType = "restore-warning"
	}
	tx := model.Transaction{
		ID: "tx-" + time.Now().UTC().Format("20060102T150405.000000000"), Type: txType,
		Status: "running", Targets: []string{finding.Fingerprint}, StartedAt: time.Now().UTC(),
	}
	_ = m.store.SaveTransaction(tx)
	if err := m.store.SetFindingIgnored(finding, ignored, reason); err != nil {
		_, failErr := m.fail(tx, err)
		return failErr
	}
	tx.Status, tx.CompletedAt = "completed", time.Now().UTC()
	_ = m.store.SaveTransaction(tx)
	m.recordTransaction(tx)
	return nil
}

func (m *Manager) SetRiskClusterIgnored(cluster model.RiskCluster, ignored bool, reason string, confirmDeterministic bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cluster.ID == "" || cluster.RuleID == "" || len(cluster.Fingerprints) == 0 {
		return errors.New("risk cluster identity is incomplete")
	}
	for _, fingerprint := range cluster.Fingerprints {
		if decoded, err := hex.DecodeString(fingerprint); err != nil || len(decoded) != sha256.Size {
			return errors.New("risk cluster contains an invalid finding fingerprint")
		}
	}
	reason = strings.TrimSpace(reason)
	if ignored && reason == "" {
		return errors.New("an ignore reason is required to record the manual review")
	}
	deterministic := cluster.Deterministic || cluster.RuleID == "CSM-FS-001" ||
		cluster.RuleID == "CSM-FILE-002" || cluster.RuleID == "CSM-DEL-001"
	cluster.Deterministic = deterministic
	if ignored && deterministic && !confirmDeterministic {
		return errors.New("deterministic safety findings require explicit manual override confirmation")
	}
	txType := "ignore-risk-cluster"
	if !ignored {
		txType = "restore-risk-cluster"
	}
	tx := model.Transaction{
		ID: "tx-" + time.Now().UTC().Format("20060102T150405.000000000"), Type: txType,
		Status: "running", Targets: []string{cluster.ID}, StartedAt: time.Now().UTC(),
	}
	_ = m.store.SaveTransaction(tx)
	if ignored && deterministic {
		_ = m.store.Approve(cluster.ID, "manual-override-deterministic", reason)
	}
	if err := m.store.SetClusterIgnored(cluster, ignored, reason); err != nil {
		_, failErr := m.fail(tx, err)
		return failErr
	}
	tx.Status, tx.CompletedAt = "completed", time.Now().UTC()
	_ = m.store.SaveTransaction(tx)
	m.recordTransaction(tx)
	return nil
}

func (m *Manager) PrepareGitHub(ctx context.Context, rawURL, ref string) (model.InstallPreview, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.prepareGitHub(ctx, rawURL, ref, nil)
}

// prepareGitHub resolves and downloads an immutable repository snapshot, then
// scans only the Skill directories that can be selected by the resulting plan.
// Repository-level files are not installation targets and must not affect the
// plan's risk decision.
func (m *Manager) prepareGitHub(ctx context.Context, rawURL, ref string, selected map[string]bool) (model.InstallPreview, error) {
	repo, err := m.github.Resolve(ctx, rawURL, ref)
	if err != nil {
		return model.InstallPreview{}, err
	}
	id := "plan-" + time.Now().UTC().Format("20060102T150405.000000000")
	stage := filepath.Join(m.Config.Paths.StagingRoot, id)
	root, err := m.github.Download(ctx, repo, stage, m.Config.MaxFileBytes)
	if err != nil {
		return model.InstallPreview{}, err
	}
	candidates, err := githubsource.Discover(root, repo.SourcePath)
	if err != nil {
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
	report, scanErr := scanCandidateSkills(root, candidates, m.Config.MaxFiles, m.Config.MaxFileBytes)
	ignored, ignoreErr := m.store.IgnoredFindings()
	if ignoreErr != nil {
		return model.InstallPreview{}, ignoreErr
	}
	report = m.decorateScan(report, ignored)
	_ = m.store.SaveScan(report)
	m.recordScan(report)
	if scanErr != nil {
		return model.InstallPreview{}, scanErr
	}
	preview := model.InstallPreview{
		ID: id, Repository: repo, Skills: candidates, Scan: report,
		StagingPath: root, CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
	}
	m.previews[id] = preview
	if err := savePreview(m.Config.Paths.DataRoot, preview); err != nil {
		return model.InstallPreview{}, err
	}
	return preview, nil
}

func (m *Manager) PrepareLocal(path string) (model.InstallPreview, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	abs, err := filepath.Abs(path)
	if err != nil {
		return model.InstallPreview{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return model.InstallPreview{}, err
	}
	if !info.IsDir() {
		return model.InstallPreview{}, errors.New("local source must be a directory")
	}
	candidates, err := githubsource.Discover(abs, "")
	if err != nil {
		return model.InstallPreview{}, err
	}
	if len(candidates) == 0 {
		return model.InstallPreview{}, errors.New("no valid SKILL.md directories found")
	}
	report, err := scanner.Scan(abs, m.Config.MaxFiles, m.Config.MaxFileBytes)
	if err != nil {
		return model.InstallPreview{}, err
	}
	ignored, ignoreErr := m.store.IgnoredFindings()
	if ignoreErr != nil {
		return model.InstallPreview{}, ignoreErr
	}
	report = m.decorateScan(report, ignored)
	_ = m.store.SaveScan(report)
	m.recordScan(report)
	id := "plan-" + time.Now().UTC().Format("20060102T150405.000000000")
	preview := model.InstallPreview{
		ID: id,
		Repository: model.Repository{
			Provider: "local", Name: filepath.Base(abs), FullName: "local:" + filepath.Base(abs),
			LocalPath: abs, SourcePath: "", ResolvedRef: "local",
		},
		Skills: candidates, Scan: report, StagingPath: abs,
		CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
	}
	m.previews[id] = preview
	if err := savePreview(m.Config.Paths.DataRoot, preview); err != nil {
		return model.InstallPreview{}, err
	}
	return preview, nil
}

func (m *Manager) PrepareAdoption(selected []string) (model.AdoptionPreview, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	selected = uniqueNonEmpty(selected)
	if len(selected) == 0 {
		return model.AdoptionPreview{}, errors.New("at least one unmanaged skill is required")
	}
	lock, err := m.store.LoadLock()
	if err != nil {
		return model.AdoptionPreview{}, err
	}
	skills, _, _, err := inventory.Discover(m.Config.Paths.SkillsRoot, lock)
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
		ID:        "adopt-plan-" + time.Now().UTC().Format("20060102T150405.000000000"),
		Skills:    []model.Skill{},
		Sources:   []model.DetectedSource{},
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
		Scan: model.ScanReport{
			ID: "scan-" + time.Now().UTC().Format("20060102T150405.000000000"), Target: "未管理 Skills",
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
	_ = m.store.SaveScan(preview.Scan)
	m.recordScan(preview.Scan)
	m.adoptions[preview.ID] = preview
	if err := saveAdoptionPreview(m.Config.Paths.DataRoot, preview); err != nil {
		return model.AdoptionPreview{}, err
	}
	return preview, nil
}

func (m *Manager) ApplyAdoption(planID string, selected []string) (model.Transaction, error) {
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
		ID: "tx-" + time.Now().UTC().Format("20060102T150405.000000000"), Type: "manage",
		Status: "running", Targets: selected, StartedAt: time.Now().UTC(),
	}
	_ = m.store.SaveTransaction(tx)
	lock, err := m.store.LoadLock()
	if err != nil {
		return m.fail(tx, err)
	}
	if err := m.snapshotLock(tx.ID); err != nil {
		return m.fail(tx, err)
	}
	for _, name := range selected {
		if _, managed := findManaged(lock, name); managed {
			return m.fail(tx, fmt.Errorf("%s is already managed", name))
		}
		target := filepath.Join(m.Config.Paths.SkillsRoot, name)
		if filepath.Base(target) != name || name == ".system" {
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
		packageID := source.GroupID
		pkg := lock.Packages[packageID]
		if pkg.Skills == nil {
			pkg = model.PackageLock{
				Provider: source.Provider, Repository: source.Repository, SourceURL: source.SourceURL,
				GroupName: source.GroupName, RequestedRef: source.RequestedRef,
				InstalledAt: time.Now().UTC(), Skills: map[string]model.SkillLock{},
			}
		}
		hashes := map[string]string{}
		for _, file := range files {
			hashes[file.Path] = file.SHA256
		}
		pkg.Skills[name] = model.SkillLock{
			SourcePath: source.SourcePath, LocalPath: name, Files: hashes,
			LastScanReport: preview.Scan.ID,
		}
		pkg.UpdatedAt = time.Now().UTC()
		lock.Packages[packageID] = pkg
	}
	if err := m.store.SaveLock(lock); err != nil {
		return m.fail(tx, err)
	}
	tx.Status, tx.CompletedAt = "completed", time.Now().UTC()
	_ = m.store.SaveTransaction(tx)
	m.recordTransaction(tx)
	return tx, nil
}

func (m *Manager) ApplyInstall(planID string, selected []string, acceptHighRisk bool) (model.Transaction, error) {
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
	ignored, err := m.store.IgnoredFindings()
	if err != nil {
		return model.Transaction{}, err
	}
	preview.Scan = m.decorateScan(preview.Scan, ignored)
	if preview.Scan.ActiveHighestSeverity == model.RiskCritical {
		activeCritical := 0
		for _, finding := range preview.Scan.Findings {
			if !finding.Ignored && finding.Severity == model.RiskCritical {
				activeCritical++
			}
		}
		return model.Transaction{}, fmt.Errorf(
			"仍有 %d 个未忽略的 Critical 风险；必须逐条人工核查并记录原因后才能安装",
			activeCritical,
		)
	}
	if preview.Scan.ActiveHighestSeverity == model.RiskHigh && !acceptHighRisk {
		return model.Transaction{}, errors.New("high-risk findings require explicit acceptance")
	}
	chosen := chooseCandidates(preview.Skills, selected)
	if len(chosen) == 0 {
		return model.Transaction{}, errors.New("no skills selected")
	}
	tx := model.Transaction{
		ID:   "tx-" + time.Now().UTC().Format("20060102T150405.000000000"),
		Type: "install", Status: "running", StartedAt: time.Now().UTC(),
	}
	for _, c := range chosen {
		tx.Targets = append(tx.Targets, c.Name)
	}
	_ = m.store.SaveTransaction(tx)

	lock, err := m.store.LoadLock()
	if err != nil {
		return m.fail(tx, err)
	}
	if err := m.snapshotLock(tx.ID); err != nil {
		return m.fail(tx, err)
	}
	touched := []string{}
	backups := map[string]string{}
	provider := preview.Repository.Provider
	if provider == "" {
		provider = "github"
	}
	pkgID := provider + ":" + strings.ToLower(preview.Repository.FullName)
	if provider == "local" {
		pkgID = "local:" + strings.ToLower(filepath.Clean(preview.Repository.LocalPath))
	}
	pkg := lock.Packages[pkgID]
	if pkg.Skills == nil {
		sourceURL := "https://github.com/" + preview.Repository.FullName
		repository := preview.Repository.FullName
		if provider == "local" {
			sourceURL = preview.Repository.LocalPath
			repository = preview.Repository.LocalPath
		}
		pkg = model.PackageLock{
			Provider: provider, Repository: repository, GroupName: repository,
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
		target := filepath.Join(m.Config.Paths.SkillsRoot, c.Name)
		if filepath.Base(target) != c.Name || c.Name == ".system" {
			return m.failInstall(tx, touched, backups, fmt.Errorf("invalid skill name: %s", c.Name))
		}
		source := filepath.Join(preview.StagingPath, filepath.FromSlash(c.SourcePath))
		if _, err := os.Stat(target); err == nil {
			existing, managed := findManaged(lock, c.Name)
			expectedRepository := preview.Repository.FullName
			if provider == "local" {
				expectedRepository = preview.Repository.LocalPath
			}
			if !managed || !strings.EqualFold(existing.Repository, expectedRepository) {
				return m.failInstall(tx, touched, backups, fmt.Errorf("skill name conflict: %s", c.Name))
			}
			backup := transactionContentPath(
				m.Config.Paths.SkillsRoot, m.Config.Paths.BackupsRoot, ".csm-backups", c.Name, tx.ID,
			)
			if err := os.MkdirAll(filepath.Dir(backup), 0o700); err != nil {
				return m.failInstall(tx, touched, backups, err)
			}
			if err := os.Rename(target, backup); err != nil {
				return m.failInstall(tx, touched, backups, fmt.Errorf("backup %s: %w", c.Name, err))
			}
			backups[c.Name] = backup
			tx.BackupPaths = append(tx.BackupPaths, backup)
		}
		touched = append(touched, c.Name)
		if err := copyTree(source, target); err != nil {
			return m.failInstall(tx, touched, backups, err)
		}
		files, err := inventory.HashTree(target)
		if err != nil {
			return m.failInstall(tx, touched, backups, err)
		}
		hashes := map[string]string{}
		for _, f := range files {
			hashes[f.Path] = f.SHA256
		}
		pkg.Skills[c.Name] = model.SkillLock{
			SourcePath: c.SourcePath, LocalPath: c.Name, Files: hashes,
			ResolvedCommit: preview.Repository.CommitSHA,
			Pinned:         preview.Repository.ResolvedRef != preview.Repository.DefaultBranch,
			LastScanReport: preview.Scan.ID,
		}
	}
	pkg.UpdatedAt = time.Now().UTC()
	pkg.ResolvedCommit = preview.Repository.CommitSHA
	lock.Packages[pkgID] = pkg
	if err := m.store.SaveLock(lock); err != nil {
		return m.failInstall(tx, touched, backups, err)
	}
	tx.Status, tx.CompletedAt = "completed", time.Now().UTC()
	_ = m.store.SaveTransaction(tx)
	m.recordTransaction(tx)
	return tx, nil
}

func (m *Manager) AdoptPackage(repository, sourceURL, ref, commit string, mappings map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if repository == "" || len(mappings) == 0 {
		return errors.New("repository and mappings are required")
	}
	lock, err := m.store.LoadLock()
	if err != nil {
		return err
	}
	id := "github:" + strings.ToLower(repository)
	pkg := model.PackageLock{
		Provider: "github", Repository: repository, SourceURL: sourceURL,
		RequestedRef: ref, ResolvedCommit: commit, InstalledAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(), Skills: map[string]model.SkillLock{},
	}
	for name, sourcePath := range mappings {
		local := filepath.Join(m.Config.Paths.SkillsRoot, name)
		files, err := inventory.HashTree(local)
		if err != nil {
			return fmt.Errorf("adopt %s: %w", name, err)
		}
		hashes := map[string]string{}
		for _, f := range files {
			hashes[f.Path] = f.SHA256
		}
		pkg.Skills[name] = model.SkillLock{
			SourcePath: sourcePath, LocalPath: name, ResolvedCommit: commit, Files: hashes,
		}
	}
	lock.Packages[id] = pkg
	return m.store.SaveLock(lock)
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
		previous[status.GroupID] = status
	}
	selected := map[string]bool{}
	for _, id := range groupIDs {
		selected[strings.TrimSpace(id)] = true
	}
	for id, pkg := range lock.Packages {
		if len(selected) > 0 && !selected[id] {
			if status, ok := previous[id]; ok {
				result.Statuses = append(result.Statuses, status)
			}
			continue
		}
		status := model.UpdateStatus{
			GroupID: id, GroupName: pkg.GroupName, Provider: pkg.Provider,
			Repository: pkg.Repository, Status: "unsupported", CheckedAt: checkedAt,
			CurrentCommits: map[string]string{}, OutdatedSkills: []string{},
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
			if old, ok := previous[id]; ok {
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

func (m *Manager) PrepareUpdate(ctx context.Context, groupID string) (model.InstallPreview, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return model.InstallPreview{}, errors.New("explicit source group ID is required")
	}
	lock, err := m.store.LoadLock()
	if err != nil {
		return model.InstallPreview{}, err
	}
	pkg, ok := lock.Packages[groupID]
	if !ok {
		return model.InstallPreview{}, fmt.Errorf("source group not found: %s", groupID)
	}
	if pkg.Provider != "github" || pkg.SourceURL == "" {
		return model.InstallPreview{}, errors.New("this source does not support GitHub updates")
	}
	installed := map[string]bool{}
	for name := range pkg.Skills {
		installed[name] = true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	preview, err := m.prepareGitHub(ctx, pkg.SourceURL, pkg.RequestedRef, installed)
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
		sourcePath := filepath.Clean(filepath.FromSlash(candidate.SourcePath))
		target := filepath.Join(root, sourcePath)
		rel, err := filepath.Rel(root, target)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
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

func finishCandidateScan(report *model.ScanReport) {
	report.HighestSeverity = highestFindingSeverity(report.Findings, false)
	report.ActiveHighestSeverity = report.HighestSeverity
	report.ActiveFindingCount = len(report.Findings)
	if len(report.Findings) > 0 {
		report.Status = "findings"
	}
	report.CompletedAt = time.Now().UTC()
}

func (m *Manager) Quarantine(names []string) (model.Transaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	names = uniqueNonEmpty(names)
	if len(names) == 0 {
		return model.Transaction{}, errors.New("at least one explicit skill name is required")
	}
	for _, name := range names {
		if strings.ContainsAny(name, "*?") || name == ".system" || filepath.Base(name) != name {
			return model.Transaction{}, fmt.Errorf("invalid uninstall target: %s", name)
		}
	}
	tx := model.Transaction{
		ID:   "tx-" + time.Now().UTC().Format("20060102T150405.000000000"),
		Type: "quarantine", Status: "running", Targets: names, StartedAt: time.Now().UTC(),
	}
	_ = m.store.SaveTransaction(tx)
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
		if rollbackErr != nil {
			cause = fmt.Errorf("%w; quarantine rollback failed: %v", cause, rollbackErr)
		}
		return m.fail(tx, cause)
	}
	for _, name := range names {
		source := filepath.Join(m.Config.Paths.SkillsRoot, name)
		if _, err := os.Stat(source); err != nil {
			return rollbackMoved(fmt.Errorf("%s: %w", name, err))
		}
		target := transactionContentPath(
			m.Config.Paths.SkillsRoot, m.Config.Paths.QuarantineRoot, ".csm-quarantine", name, tx.ID,
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
	_ = m.store.SaveTransaction(tx)
	m.recordTransaction(tx)
	return tx, nil
}

func (m *Manager) Restore(name, transactionID string) (model.Transaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if name == "" || transactionID == "" || filepath.Base(name) != name || strings.ContainsAny(name, "*?") {
		return model.Transaction{}, errors.New("explicit skill name and transaction ID are required")
	}
	original, originalErr := m.store.Transaction(transactionID)
	if originalErr != nil {
		return model.Transaction{}, originalErr
	}
	source := transactionPathForName(original, name)
	if source == "" {
		source = transactionContentPath(
			m.Config.Paths.SkillsRoot, m.Config.Paths.QuarantineRoot, ".csm-quarantine", name, transactionID,
		)
	}
	target := filepath.Join(m.Config.Paths.SkillsRoot, name)
	if _, err := os.Stat(target); err == nil {
		return model.Transaction{}, fmt.Errorf("target already exists: %s", target)
	}
	tx := model.Transaction{ID: "tx-" + time.Now().UTC().Format("20060102T150405.000000000"), Type: "restore", Status: "running", Targets: []string{name}, StartedAt: time.Now().UTC()}
	if err := os.Rename(source, target); err != nil {
		return m.fail(tx, err)
	}
	current, err := m.store.LoadLock()
	if err != nil {
		_ = os.Rename(target, source)
		return m.fail(tx, err)
	}
	snapshotPath := filepath.Join(m.Config.Paths.BackupsRoot, "_transactions", transactionID, "sources.lock.json")
	snapshotData, err := os.ReadFile(snapshotPath)
	if err != nil {
		_ = os.Rename(target, source)
		return m.fail(tx, fmt.Errorf("load source snapshot: %w", err))
	}
	var snapshot model.SourcesLock
	if err := json.Unmarshal(snapshotData, &snapshot); err != nil {
		_ = os.Rename(target, source)
		return m.fail(tx, fmt.Errorf("parse source snapshot: %w", err))
	}
	restoredMapping := false
	for id, originalPackage := range snapshot.Packages {
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
		_ = os.Rename(target, source)
		return m.fail(tx, fmt.Errorf("source mapping for %s was not found in transaction %s", name, transactionID))
	}
	if err := m.store.SaveLock(current); err != nil {
		_ = os.Rename(target, source)
		return m.fail(tx, err)
	}
	tx.Status, tx.CompletedAt = "completed", time.Now().UTC()
	_ = m.store.SaveTransaction(tx)
	m.recordTransaction(tx)
	return tx, nil
}

func (m *Manager) Rollback(transactionID string) (model.Transaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	original, err := m.store.Transaction(transactionID)
	if err != nil {
		return model.Transaction{}, err
	}
	if original.Type != "install" && original.Type != "adopt" && original.Type != "manage" &&
		!strings.HasPrefix(original.Type, "group-") {
		return model.Transaction{}, fmt.Errorf("transaction type %s cannot be rolled back; use its dedicated recovery action", original.Type)
	}
	tx := model.Transaction{
		ID:   "tx-" + time.Now().UTC().Format("20060102T150405.000000000"),
		Type: "rollback", Status: "running", Targets: append([]string(nil), original.Targets...),
		StartedAt: time.Now().UTC(),
	}
	_ = m.store.SaveTransaction(tx)
	if strings.HasPrefix(original.Type, "group-") {
		path := filepath.Join(m.Config.Paths.BackupsRoot, "_transactions", transactionID, "groups.json")
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return m.fail(tx, readErr)
		}
		var layout model.GroupLayoutState
		if err := json.Unmarshal(data, &layout); err != nil {
			return m.fail(tx, err)
		}
		if err := m.store.ReplaceGroupLayout(layout); err != nil {
			return m.fail(tx, err)
		}
		tx.Status, tx.CompletedAt = "completed", time.Now().UTC()
		_ = m.store.SaveTransaction(tx)
		m.recordTransaction(tx)
		return tx, nil
	}
	if original.Type == "adopt" || original.Type == "manage" {
		lockSnapshot := filepath.Join(m.Config.Paths.BackupsRoot, "_transactions", transactionID, "sources.lock.json")
		data, readErr := os.ReadFile(lockSnapshot)
		if readErr != nil {
			return m.fail(tx, readErr)
		}
		var lock model.SourcesLock
		if err := json.Unmarshal(data, &lock); err != nil {
			return m.fail(tx, err)
		}
		if err := m.store.SaveLock(lock); err != nil {
			return m.fail(tx, err)
		}
		tx.Status, tx.CompletedAt = "completed", time.Now().UTC()
		_ = m.store.SaveTransaction(tx)
		m.recordTransaction(tx)
		return tx, nil
	}
	for _, name := range original.Targets {
		target := filepath.Join(m.Config.Paths.SkillsRoot, name)
		backup := transactionPathForName(original, name)
		if backup == "" {
			backup = transactionContentPath(
				m.Config.Paths.SkillsRoot, m.Config.Paths.BackupsRoot, ".csm-backups", name, transactionID,
			)
		}
		if _, err := os.Stat(target); err == nil {
			failed := transactionContentPath(
				m.Config.Paths.SkillsRoot, m.Config.Paths.QuarantineRoot, ".csm-quarantine", name, "rollback-"+tx.ID,
			)
			if err := os.MkdirAll(filepath.Dir(failed), 0o700); err != nil {
				return m.fail(tx, err)
			}
			if err := os.Rename(target, failed); err != nil {
				return m.fail(tx, err)
			}
		}
		if _, err := os.Stat(backup); err == nil {
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return m.fail(tx, err)
			}
			if err := os.Rename(backup, target); err != nil {
				return m.fail(tx, err)
			}
		}
	}
	lockSnapshot := filepath.Join(m.Config.Paths.BackupsRoot, "_transactions", transactionID, "sources.lock.json")
	if data, err := os.ReadFile(lockSnapshot); err == nil {
		var lock model.SourcesLock
		if json.Unmarshal(data, &lock) == nil {
			_ = m.store.SaveLock(lock)
		}
	}
	tx.Status, tx.CompletedAt = "completed", time.Now().UTC()
	_ = m.store.SaveTransaction(tx)
	m.recordTransaction(tx)
	return tx, nil
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
		if err := m.AdoptPackage("tirth8205/code-review-graph", "https://github.com/tirth8205/code-review-graph", "main", "", graph); err != nil {
			return err
		}
	}
	if existingMappings(m.Config.Paths.SkillsRoot, career) {
		if err := m.AdoptPackage("rebecha1227-a11y/CareerForge", "https://github.com/rebecha1227-a11y/CareerForge", "main", "f21dc27d1820bfdc67bc4c22b1f20cc2028692d2", career); err != nil {
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
	_ = m.store.SaveTransaction(tx)
	m.recordTransaction(tx)
	return tx, err
}

func (m *Manager) failInstall(tx model.Transaction, touched []string, backups map[string]string, cause error) (model.Transaction, error) {
	var rollbackErr error
	for i := len(touched) - 1; i >= 0; i-- {
		name := touched[i]
		target := filepath.Join(m.Config.Paths.SkillsRoot, name)
		if _, err := os.Stat(target); err == nil {
			failed := transactionContentPath(
				m.Config.Paths.SkillsRoot, m.Config.Paths.QuarantineRoot, ".csm-quarantine", name, "failed-"+tx.ID,
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
	if rollbackErr != nil {
		cause = fmt.Errorf("%w; install rollback failed: %v", cause, rollbackErr)
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
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, p.ID+".json"), data, 0o600)
}

func loadPreview(root, id string) (model.InstallPreview, error) {
	if filepath.Base(id) != id {
		return model.InstallPreview{}, errors.New("invalid plan ID")
	}
	data, err := os.ReadFile(filepath.Join(root, id+".json"))
	if err != nil {
		return model.InstallPreview{}, err
	}
	var p model.InstallPreview
	err = json.Unmarshal(data, &p)
	return p, err
}

func saveAdoptionPreview(root string, preview model.AdoptionPreview) error {
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
	err = json.Unmarshal(data, &preview)
	return preview, err
}

func (m *Manager) decorateScan(report model.ScanReport, ignored map[string]string) model.ScanReport {
	if report.Findings == nil {
		report.Findings = []model.Finding{}
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
			finding.RuleID, canonicalFile, strings.TrimSpace(finding.Evidence),
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
		clusterIdentity := strings.ToLower(strings.Join([]string{
			finding.RuleID, finding.Category, finding.FileClass, fmt.Sprintf("%t", finding.Deterministic),
		}, "\x00"))
		clusterHash := sha256.Sum256([]byte(clusterIdentity))
		finding.ClusterID = fmt.Sprintf("risk-%x", clusterHash[:12])
		finding.Ignored = false
		finding.IgnoreReason = ""
		if reason, ok := ignored[finding.Fingerprint]; ok {
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
				},
				files: map[string]bool{},
			}
			builds[finding.ClusterID] = build
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
			if _, ok := ignored[fingerprint]; !ok {
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
	default:
		return 1
	}
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

func existingMappings(root string, mappings map[string]string) bool {
	for name := range mappings {
		if _, err := os.Stat(filepath.Join(root, name, "SKILL.md")); err != nil {
			return false
		}
	}
	return true
}
