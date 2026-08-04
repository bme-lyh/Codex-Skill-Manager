package manager

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

const groupRiskApprovalDecision = "group-risk-approval"
const groupSecurityApprovalPrefix = "group-security:"

func (m *Manager) sourceGroupRiskApproved(preview model.InstallPreview) bool {
	if strings.EqualFold(preview.Repository.Provider, "github") {
		if repository, err := model.CanonicalGitHubRepository(preview.Repository.FullName); err == nil {
			if policy, policyErr := m.store.SourceTrustPolicy(repository); policyErr == nil && policy.Trusted {
				return true
			}
		}
	}
	approved, err := m.store.HasApproval(preview.ID, groupRiskApprovalDecision)
	if err == nil && approved {
		return true
	}
	if preview.SourceGroupID != "" {
		if report, reportErr := m.store.LatestGroupSecurityReport(preview.TargetRootID, preview.SourceGroupID); reportErr == nil {
			if !groupSecurityReportMatchesPreview(report, preview) {
				return false
			}
			key := groupSecurityApprovalPrefix + report.PolicyVersion + ":" + report.ID
			if report.PolicyVersion == "" {
				key = groupSecurityApprovalPrefix + preview.SourceGroupID
			}
			approved, reportErr := m.store.HasApproval(key, groupRiskApprovalDecision)
			return reportErr == nil && approved
		}
	}
	return false
}

// groupSecurityReportMatchesPreview binds a persisted report to the exact
// source-group contract used by the current install/update preview.  A report
// from a different root, group, repository, commit, or policy version can
// never approve a new plan.
func groupSecurityReportMatchesPreview(report model.GroupSecurityReport, preview model.InstallPreview) bool {
	if strings.TrimSpace(report.RootID) == "" || strings.TrimSpace(preview.TargetRootID) == "" ||
		!strings.EqualFold(strings.TrimSpace(report.RootID), strings.TrimSpace(preview.TargetRootID)) {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(report.GroupID), strings.TrimSpace(preview.SourceGroupID)) {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(report.Provider), strings.TrimSpace(preview.Repository.Provider)) {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(report.Repository), strings.TrimSpace(preview.Repository.FullName)) {
		return false
	}
	if strings.TrimSpace(report.CommitSHA) == "" ||
		!strings.EqualFold(strings.TrimSpace(report.CommitSHA), strings.TrimSpace(preview.Repository.CommitSHA)) {
		return false
	}
	if strings.TrimSpace(report.PolicyVersion) != "" &&
		!strings.EqualFold(strings.TrimSpace(report.PolicyVersion), model.GroupSecurityPolicyVersion) {
		return false
	}
	return true
}

// SourceTrustPolicy returns the current repository-wide GitHub trust decision.
func (m *Manager) SourceTrustPolicy(repository string) (model.SourceTrustPolicy, error) {
	canonical, err := model.CanonicalGitHubRepository(repository)
	if err != nil {
		return model.SourceTrustPolicy{}, err
	}
	return m.store.SourceTrustPolicy(canonical)
}

func (m *Manager) SourceTrustPolicies() ([]model.SourceTrustPolicy, error) {
	return m.store.SourceTrustPolicies()
}

func (m *Manager) SourceTrustAudit(repository string, limit int) ([]model.SourceTrustAudit, error) {
	return m.store.SourceTrustAudit(repository, limit)
}

// SetSourceTrust records an explicit repository-wide trust decision.  Trust is
// advisory and never bypasses immutable commit, path, scanner, or hash gates.
func (m *Manager) SetSourceTrust(repository, reason string) (model.Transaction, error) {
	return m.setSourceTrust(repository, reason, true)
}

func (m *Manager) RevokeSourceTrust(repository, reason string) (model.Transaction, error) {
	return m.setSourceTrust(repository, reason, false)
}

func (m *Manager) setSourceTrust(repository, reason string, trusted bool) (model.Transaction, error) {
	canonical, err := model.CanonicalGitHubRepository(repository)
	if err != nil {
		return model.Transaction{}, err
	}
	now := time.Now().UTC()
	txType := "source-trust-revoke"
	action := "revoke"
	if trusted {
		txType = "source-trust-set"
		action = "set"
	}
	tx := model.Transaction{
		ID: "tx-" + now.Format("20060102T150405.000000000"), Type: txType, Status: "running",
		Targets: []string{canonical}, StartedAt: now,
	}
	if err := m.store.SaveTransaction(tx); err != nil {
		return model.Transaction{}, fmt.Errorf("start source trust transaction: %w", err)
	}
	policy := model.SourceTrustPolicy{
		Repository: canonical, Provider: "github", Trusted: trusted, Reason: strings.TrimSpace(reason),
		PolicyVersion: model.GroupSecurityPolicyVersion, SetAt: now, UpdatedAt: now,
	}
	if !trusted {
		policy.RevokedAt = &now
	}
	audit := model.SourceTrustAudit{
		Repository: canonical, Action: action, Trusted: trusted, Reason: policy.Reason,
		TransactionID: tx.ID, CreatedAt: now,
	}
	if err := m.store.SetSourceTrust(policy, audit); err != nil {
		_, failErr := m.fail(tx, err)
		return tx, failErr
	}
	tx.Status, tx.CompletedAt = "completed", time.Now().UTC()
	if err := m.store.SaveTransaction(tx); err != nil {
		return tx, fmt.Errorf("complete source trust transaction: %w", err)
	}
	m.recordTransaction(tx)
	return tx, nil
}

// ApproveGroupRisk records the explicit human decision used by a source-group
// apply.  Critical findings intentionally allow an empty reason; the apply
// path still revalidates every technical integrity gate before mutation.
func (m *Manager) ApproveGroupRisk(planID, reason string) (model.Transaction, error) {
	planID = strings.TrimSpace(planID)
	if planID == "" {
		return model.Transaction{}, errors.New("group install plan ID is required")
	}
	preview, err := m.groupInstallPreview(planID)
	if err != nil {
		return model.Transaction{}, err
	}
	ignored, err := m.store.IgnoredFindings()
	if err != nil {
		return model.Transaction{}, err
	}
	preview.Scan = m.decorateScan(preview.Scan, ignored)
	if preview.Scan.ActiveFindingCount == 0 {
		return model.Transaction{}, errors.New("group has no active risk requiring approval")
	}
	reason = strings.TrimSpace(reason)
	now := time.Now().UTC()
	tx := model.Transaction{
		ID: "tx-" + now.Format("20060102T150405.000000000"), Type: "group-risk-approval", Status: "running",
		RootID: preview.TargetRootID, GroupID: preview.SourceGroupID, GroupName: preview.SourceGroupName,
		Targets: []string{planID}, StartedAt: now,
	}
	if err := m.store.SaveTransaction(tx); err != nil {
		return model.Transaction{}, err
	}
	if err := m.store.Approve(planID, groupRiskApprovalDecision, reason); err != nil {
		_, failErr := m.fail(tx, err)
		return tx, failErr
	}
	tx.Status, tx.CompletedAt = "completed", time.Now().UTC()
	if err := m.store.SaveTransaction(tx); err != nil {
		return tx, err
	}
	m.recordTransaction(tx)
	return tx, nil
}

// ApproveGroupSecurity persists a one-click human decision for the latest
// security report of a complete source group. The decision can be reused by a
// later group install/update plan after the repository and commit are checked
// again by the apply path.
func (m *Manager) ApproveGroupSecurity(groupID, rootID, reason string) (model.Transaction, error) {
	groupID, rootID = strings.TrimSpace(groupID), strings.TrimSpace(rootID)
	if groupID == "" || rootID == "" {
		return model.Transaction{}, errors.New("group ID and root ID are required")
	}
	report, err := m.store.LatestGroupSecurityReport(rootID, groupID)
	if err != nil {
		return model.Transaction{}, fmt.Errorf("group security report not found: %w", err)
	}
	activeRisk := false
	for _, cluster := range report.Clusters {
		if !cluster.Ignored {
			activeRisk = true
			break
		}
	}
	if !activeRisk {
		return model.Transaction{}, errors.New("group has no active risk requiring approval")
	}
	now := time.Now().UTC()
	tx := model.Transaction{ID: "tx-" + now.Format("20060102T150405.000000000"), Type: "group-security-approval", Status: "running", RootID: rootID, GroupID: groupID, GroupName: report.GroupName, Targets: []string{report.ID}, StartedAt: now}
	if err := m.store.SaveTransaction(tx); err != nil {
		return model.Transaction{}, err
	}
	decisionReason := strings.TrimSpace(reason)
	if err := m.store.Approve(report.ID, groupRiskApprovalDecision, decisionReason); err != nil {
		_, failErr := m.fail(tx, err)
		return tx, failErr
	}
	// New approvals are bound to the report, group, root, commit, and policy
	// version through the composite key.  Legacy v0.14 reports without a
	// policy version keep the group-prefix key so their stored decisions are
	// still readable by the strict matcher's legacy branch.
	approvalKey := groupSecurityApprovalPrefix + report.PolicyVersion + ":" + report.ID
	if report.PolicyVersion == "" {
		approvalKey = groupSecurityApprovalPrefix + groupID
	}
	if err := m.store.Approve(approvalKey, groupRiskApprovalDecision, decisionReason); err != nil {
		_, failErr := m.fail(tx, err)
		return tx, failErr
	}
	tx.Status, tx.CompletedAt = "completed", time.Now().UTC()
	if err := m.store.SaveTransaction(tx); err != nil {
		return tx, err
	}
	m.recordTransaction(tx)
	return tx, nil
}

// PrepareGroupUpdate is the source-group spelling of the legacy update
// preview API.  The returned preview is sealed with every still-valid member
// of the source group.
func (m *Manager) PrepareGroupUpdate(ctx context.Context, groupID string, targetRootID ...string) (model.InstallPreview, error) {
	return m.PrepareUpdate(ctx, groupID, targetRootID...)
}

// ApplyGroupInstall requires the exact set of valid preview Skills.  The
// compatibility ApplyInstall API remains selective for older clients.
func (m *Manager) ApplyGroupInstall(planID string, selected []string, acceptRisk bool, targetRootID ...string) (model.Transaction, error) {
	return m.applyGroupOperation(planID, selected, acceptRisk, "group-install", targetRootID...)
}

func (m *Manager) ApplyGroupUpdate(planID string, selected []string, acceptRisk bool, targetRootID ...string) (model.Transaction, error) {
	return m.applyGroupOperation(planID, selected, acceptRisk, "group-update", targetRootID...)
}

// ApplySourceGroupInstall is a convenience wrapper that selects every member
// recorded in the preview, while ApplyGroupInstall remains useful for proving
// that a caller's explicit selection is complete.
func (m *Manager) ApplySourceGroupInstall(planID string, acceptRisk bool, targetRootID ...string) (model.Transaction, error) {
	preview, err := m.groupInstallPreview(planID)
	if err != nil {
		return model.Transaction{}, err
	}
	selected := make([]string, 0, len(preview.Skills))
	for _, skill := range preview.Skills {
		selected = append(selected, skill.Name)
	}
	return m.ApplyGroupInstall(planID, selected, acceptRisk, targetRootID...)
}

func (m *Manager) groupInstallPreview(planID string) (model.InstallPreview, error) {
	planID = strings.TrimSpace(planID)
	if planID == "" {
		return model.InstallPreview{}, errors.New("install plan ID is required")
	}
	m.mu.Lock()
	preview, ok := m.previews[planID]
	m.mu.Unlock()
	if !ok {
		var err error
		preview, err = loadPreview(m.Config.Paths.DataRoot, planID)
		if err != nil {
			return model.InstallPreview{}, err
		}
	}
	if preview.SourceGroupID == "" {
		preview.SourceGroupID, preview.SourceGroupName = previewSourceGroup(preview)
	}
	return preview, nil
}

func validateCompleteGroupSelection(preview model.InstallPreview, selected []string) ([]string, error) {
	valid := make(map[string]bool, len(preview.Skills))
	for _, skill := range preview.Skills {
		name := strings.TrimSpace(skill.Name)
		if name == "" || !validMutableSkillName(name) || valid[name] {
			if name == "" || !validMutableSkillName(name) {
				return nil, fmt.Errorf("preview contains an invalid group Skill: %q", skill.Name)
			}
			return nil, fmt.Errorf("preview contains duplicate group Skill: %s", name)
		}
		valid[name] = true
	}
	if len(valid) == 0 {
		return nil, errors.New("source group has no valid Skills")
	}
	chosen := uniqueNonEmpty(selected)
	if len(chosen) != len(selected) {
		return nil, errors.New("group target set contains duplicate or empty Skills")
	}
	for _, name := range chosen {
		if !valid[name] {
			return nil, fmt.Errorf("Skill %s is not a valid member of the source group", name)
		}
	}
	sort.Strings(chosen)
	expected := make([]string, 0, len(valid))
	for name := range valid {
		expected = append(expected, name)
	}
	sort.Strings(expected)
	if len(chosen) != len(expected) {
		return nil, fmt.Errorf("source-group operation requires all %d valid Skills; received %d", len(expected), len(chosen))
	}
	for index := range expected {
		if chosen[index] != expected[index] {
			return nil, errors.New("source-group operation target set is incomplete")
		}
	}
	return chosen, nil
}

func (m *Manager) applyGroupOperation(planID string, selected []string, acceptRisk bool, kind string, targetRootID ...string) (model.Transaction, error) {
	preview, err := m.groupInstallPreview(planID)
	if err != nil {
		return model.Transaction{}, err
	}
	chosen, err := validateCompleteGroupSelection(preview, selected)
	if err != nil {
		return model.Transaction{}, err
	}
	if len(targetRootID) > 0 && strings.TrimSpace(targetRootID[0]) != "" && strings.TrimSpace(targetRootID[0]) != preview.TargetRootID {
		return model.Transaction{}, errors.New("group operation plan target root does not match apply target")
	}
	ignored, err := m.store.IgnoredFindings()
	if err != nil {
		return model.Transaction{}, err
	}
	preview.Scan = m.decorateScan(preview.Scan, ignored)
	blocking := isBlockingRiskSeverity(preview.Scan.ActiveHighestSeverity)
	trusted := false
	if strings.EqualFold(preview.Repository.Provider, "github") {
		if repository, canonicalErr := model.CanonicalGitHubRepository(preview.Repository.FullName); canonicalErr == nil {
			if policy, policyErr := m.store.SourceTrustPolicy(repository); policyErr == nil {
				trusted = policy.Trusted
			}
		}
	}
	approved := m.sourceGroupRiskApproved(preview)
	if blocking && !approved {
		return model.Transaction{}, errors.New("active High or Critical risk requires an explicit persisted group approval")
	}
	if blocking && !acceptRisk && !trusted {
		return model.Transaction{}, errors.New("group risk approval requires final apply confirmation")
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
	groupID, groupName := preview.SourceGroupID, preview.SourceGroupName
	if groupID == "" {
		groupID, groupName = previewSourceGroup(preview)
	}
	now := time.Now().UTC()
	parent := model.Transaction{
		ID: "tx-group-" + now.Format("20060102T150405.000000000"), RootID: root.ID,
		Type: kind, Status: "running", GroupID: groupID, GroupName: groupName,
		OperationID: "group-op-" + now.Format("20060102T150405.000000000"),
		Targets:     append([]string(nil), chosen...), StartedAt: now,
	}
	m.mu.Lock()
	m.groupOps[parent.OperationID] = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.groupOps, parent.OperationID)
		m.mu.Unlock()
	}()
	operationStatus := model.GroupStatusInstalling
	op := model.GroupOperation{
		ID: parent.OperationID, ParentTransactionID: parent.ID, RootID: root.ID, GroupID: groupID,
		GroupName: groupName, Kind: kind, Status: operationStatus, TargetSkills: append([]string(nil), chosen...),
		ValidSkills: append([]string(nil), chosen...), PlanID: planID, StartedAt: now,
	}
	for _, name := range chosen {
		op.Steps = append(op.Steps, model.GroupOperationStep{ID: "step-" + name, SkillName: name, Status: "queued"})
	}
	if err := m.store.SaveTransaction(parent); err != nil {
		return model.Transaction{}, err
	}
	if err := m.store.SaveGroupOperation(op); err != nil {
		_, failErr := m.fail(parent, err)
		return parent, failErr
	}
	succeeded := 0
	failures := make([]string, 0)
	for index, name := range chosen {
		started := time.Now().UTC()
		op.Steps[index].Status = "running"
		op.Steps[index].StartedAt = started
		_ = m.store.SaveGroupOperation(op)
		child, childErr := m.applyInstallWithTransactionIDAndPolicy(planID, []string{name}, "", acceptRisk, blocking && approved)
		if childErr != nil {
			op.Steps[index].Status = "failed"
			op.Steps[index].Error = childErr.Error()
			failures = append(failures, fmt.Sprintf("%s: %v", name, childErr))
			parent.ItemResults = append(parent.ItemResults, model.BatchItemResult{Target: name, Status: "failed", Error: childErr.Error()})
		} else {
			op.Steps[index].Status = "completed"
			op.Steps[index].CompletedAt = time.Now().UTC()
			op.Steps[index].TransactionID = child.ID
			op.Steps[index].RecoveryStatus = child.RecoveryStatus
			op.Steps[index].BackupPaths = append([]string(nil), child.BackupPaths...)
			parent.ItemResults = append(parent.ItemResults, model.BatchItemResult{Target: name, Status: "completed", TransactionID: child.ID})
			succeeded++
		}
		_ = m.store.SaveGroupOperation(op)
		_ = m.store.SaveTransaction(parent)
	}
	parent.CompletedAt = time.Now().UTC()
	op.CompletedAt = parent.CompletedAt
	op.RecoveryStatus = "available"
	switch {
	case succeeded == 0:
		parent.Status, op.Status = "failed", model.GroupStatusFailed
	case len(failures) > 0:
		parent.Status, op.Status = "partial", model.GroupStatusPartial
	default:
		parent.Status, op.Status = "completed", model.GroupStatusCompleted
	}
	if len(failures) > 0 {
		parent.Error, op.Error = strings.Join(failures, "; "), strings.Join(failures, "; ")
	}
	if err := m.store.SaveGroupOperation(op); err != nil {
		return parent, err
	}
	if err := m.store.SaveTransaction(parent); err != nil {
		return parent, err
	}
	m.recordTransaction(parent)
	if succeeded == 0 {
		return parent, errors.New(parent.Error)
	}
	return parent, nil
}

func previewSourceGroup(preview model.InstallPreview) (string, string) {
	provider := strings.ToLower(strings.TrimSpace(preview.Repository.Provider))
	if provider == "github" {
		repository, err := model.CanonicalGitHubRepository(preview.Repository.FullName)
		if err == nil {
			return "github:" + repository, preview.Repository.FullName
		}
	}
	if provider == "local" {
		return "local:" + strings.ToLower(strings.TrimSpace(preview.Repository.LocalPath)), preview.Repository.Name
	}
	return provider + ":" + strings.ToLower(strings.TrimSpace(preview.Repository.FullName)), preview.Repository.FullName
}

// AuditGroup scans every installed Skill whose immutable source provenance is
// the requested group and persists a reusable group report.
func (m *Manager) AuditGroup(groupID string, requestedRootID ...string) (model.GroupSecurityReport, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return model.GroupSecurityReport{}, errors.New("source group ID is required")
	}
	rootID := ""
	if len(requestedRootID) > 0 {
		rootID = strings.TrimSpace(requestedRootID[0])
	}
	dashboard, err := m.Dashboard()
	if err != nil {
		return model.GroupSecurityReport{}, err
	}
	var source model.Group
	names := make([]string, 0)
	commitSHA := ""
	for _, candidate := range dashboard.SourceGroups {
		if candidate.ID == groupID && (rootID == "" || candidate.RootID == rootID) {
			source = candidate
			rootID = candidate.RootID
			break
		}
	}
	if source.ID == "" {
		return model.GroupSecurityReport{}, fmt.Errorf("source group not found: %s", groupID)
	}
	for _, skill := range dashboard.Skills {
		if skill.RootID == rootID && skill.SourceGroupID == groupID && !skill.System {
			names = append(names, skill.Name)
			if commitSHA == "" {
				commitSHA = skill.InstalledCommit
			}
		}
	}
	if len(names) == 0 {
		return model.GroupSecurityReport{}, errors.New("source group has no installed Skills")
	}
	sort.Strings(names)
	scan, scanErr := m.AuditSkills(names, rootID)
	report := model.GroupSecurityReport{
		ID: "group-security-" + time.Now().UTC().Format("20060102T150405.000000000"), RootID: rootID,
		GroupID: groupID, GroupName: source.Name, Provider: source.Provider, Repository: source.Repository, CommitSHA: commitSHA,
		PolicyVersion: model.GroupSecurityPolicyVersion,
		Status:        scan.Status, HighestSeverity: scan.HighestSeverity, ActiveHighestSeverity: scan.ActiveHighestSeverity,
		Findings: append([]model.Finding(nil), scan.Findings...), Clusters: append([]model.RiskCluster(nil), scan.Clusters...),
		ScanReportID: scan.ID, CreatedAt: scan.StartedAt, CompletedAt: scan.CompletedAt,
		Skills:  make([]model.GroupSkillSecurity, 0, len(scan.Skills)),
		Summary: model.LocalizedText{En: "Source-group security scan completed.", Zh: "来源组安全扫描已完成。"},
	}
	for _, summary := range scan.Skills {
		status := "checked"
		if summary.Error != "" {
			status = "failed"
		} else if summary.ActiveFindingCount > 0 {
			status = "findings"
		}
		report.Skills = append(report.Skills, model.GroupSkillSecurity{
			SkillName: summary.SkillName, RootID: summary.RootID, Status: status,
			HighestSeverity: summary.HighestSeverity, ActiveFindingCount: summary.ActiveFindingCount,
			FindingCount: summary.FindingCount, ReportID: scan.ID, Error: summary.Error,
		})
	}
	if scanErr != nil {
		report.Error = scanErr.Error()
	}
	if err := m.store.SaveGroupSecurityReport(report); err != nil {
		return model.GroupSecurityReport{}, err
	}
	return report, scanErr
}

func (m *Manager) GetGroupSecurityReport(id string) (model.GroupSecurityReport, error) {
	return m.store.GroupSecurityReport(id)
}

func (m *Manager) AnalyzeGroup(groupID string, requestedRootID ...string) (model.SourceAnalysis, error) {
	report, err := m.AuditGroup(groupID, requestedRootID...)
	if err != nil && report.ID == "" {
		return model.SourceAnalysis{}, err
	}
	now := time.Now().UTC()
	analysis := model.SourceAnalysis{
		ID: "source-analysis-" + now.Format("20060102T150405.000000000"), RootID: report.RootID,
		GroupID: report.GroupID, GroupName: report.GroupName, Provider: report.Provider,
		Repository: report.Repository, CommitSHA: report.CommitSHA, PolicyVersion: report.PolicyVersion,
		Status: report.Status, Security: report,
		ScanReportID: report.ScanReportID, CreatedAt: now, CompletedAt: report.CompletedAt,
		Summary: report.Summary, Skills: make([]string, 0, len(report.Skills)),
	}
	for _, skill := range report.Skills {
		analysis.Skills = append(analysis.Skills, skill.SkillName)
	}
	if err != nil {
		analysis.Error = err.Error()
	}
	if saveErr := m.store.SaveSourceAnalysis(analysis); saveErr != nil {
		return model.SourceAnalysis{}, saveErr
	}
	return analysis, err
}

// GetOrCreateSourceGroupAnalysis stores the immutable understanding and plan
// envelope already produced by a source preview. It is safe to call before any
// Skill is installed and is reused by install, update, and security clients.
func (m *Manager) GetOrCreateSourceGroupAnalysis(planID string) (model.SourceAnalysis, error) {
	preview, err := m.groupInstallPreview(planID)
	if err != nil {
		return model.SourceAnalysis{}, err
	}
	ignored, err := m.store.IgnoredFindings()
	if err != nil {
		return model.SourceAnalysis{}, err
	}
	scan := m.decorateScan(preview.Scan, ignored)
	groupID, groupName := preview.SourceGroupID, preview.SourceGroupName
	if groupID == "" {
		groupID, groupName = previewSourceGroup(preview)
	}
	now := time.Now().UTC()
	report := model.GroupSecurityReport{
		ID: "group-security-preview-" + planID, RootID: preview.TargetRootID, GroupID: groupID, GroupName: groupName,
		Provider: preview.Repository.Provider, Repository: preview.Repository.FullName, CommitSHA: preview.Repository.CommitSHA,
		PolicyVersion: model.GroupSecurityPolicyVersion,
		Status:        scan.Status, HighestSeverity: scan.HighestSeverity, ActiveHighestSeverity: scan.ActiveHighestSeverity,
		Summary: model.LocalizedText{En: "Source understanding and security checks are ready for review.", Zh: "来源理解和安全检查已完成，等待审核。"},
		Skills:  make([]model.GroupSkillSecurity, 0, len(preview.Skills)), Findings: append([]model.Finding(nil), scan.Findings...), Clusters: append([]model.RiskCluster(nil), scan.Clusters...),
		ScanReportID: scan.ID, CreatedAt: now, CompletedAt: scan.CompletedAt,
	}
	for _, candidate := range preview.Skills {
		status := "checked"
		for _, summary := range scan.Skills {
			if summary.SkillName == candidate.Name {
				status = "checked"
				if summary.Error != "" {
					status = "failed"
				} else if summary.ActiveFindingCount > 0 {
					status = "findings"
				}
				break
			}
		}
		report.Skills = append(report.Skills, model.GroupSkillSecurity{SkillName: candidate.Name, RootID: preview.TargetRootID, Status: status, HighestSeverity: scan.ActiveHighestSeverity, ReportID: scan.ID})
	}
	if err := m.store.SaveGroupSecurityReport(report); err != nil {
		return model.SourceAnalysis{}, err
	}
	analysis := model.SourceAnalysis{
		ID: "source-analysis-" + planID, RootID: preview.TargetRootID, GroupID: groupID, GroupName: groupName,
		Provider: preview.Repository.Provider, Repository: preview.Repository.FullName, CommitSHA: preview.Repository.CommitSHA,
		PolicyVersion: model.GroupSecurityPolicyVersion,
		Status:        "ready", Summary: report.Summary, Security: report, ScanReportID: scan.ID, PlanID: planID,
		ContextDigest: preview.PreviewDigest, AnalysisDigest: preview.PreviewDigest, CreatedAt: now, ExpiresAt: preview.ExpiresAt,
	}
	for _, candidate := range preview.Skills {
		analysis.Skills = append(analysis.Skills, candidate.Name)
	}
	if err := m.store.SaveSourceAnalysis(analysis); err != nil {
		return model.SourceAnalysis{}, err
	}
	return analysis, nil
}

func (m *Manager) GetSourceAnalysis(id string) (model.SourceAnalysis, error) {
	return m.store.SourceAnalysis(id)
}

func (m *Manager) SourceAnalyses(limit int) ([]model.SourceAnalysis, error) {
	return m.store.RecentSourceAnalyses(limit)
}

func (m *Manager) GroupOperations(limit int) ([]model.GroupOperation, error) {
	return m.store.RecentGroupOperations(limit)
}

func (m *Manager) GetGroupOperation(id string) (model.GroupOperation, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return model.GroupOperation{}, errors.New("group operation ID is required")
	}
	return m.store.GroupOperation(id)
}

// GetGroupMetadata returns the combined read-only detail contract for one
// source group: its dashboard projection plus the latest persisted analysis,
// security report, operation, and update status.
func (m *Manager) GetGroupMetadata(groupID string, requestedRootID ...string) (model.GroupMetadata, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return model.GroupMetadata{}, errors.New("source group ID is required")
	}
	rootID := ""
	if len(requestedRootID) > 0 {
		rootID = strings.TrimSpace(requestedRootID[0])
	}
	dashboard, err := m.Dashboard()
	if err != nil {
		return model.GroupMetadata{}, err
	}
	var found model.Group
	for _, candidate := range dashboard.SourceGroups {
		if candidate.ID == groupID && (rootID == "" || candidate.RootID == rootID) {
			found = candidate
			rootID = candidate.RootID
			break
		}
	}
	if found.ID == "" {
		for _, candidate := range dashboard.Groups {
			if candidate.ID == groupID && (rootID == "" || candidate.RootID == rootID) {
				found = candidate
				rootID = candidate.RootID
				break
			}
		}
	}
	if found.ID == "" {
		return model.GroupMetadata{}, fmt.Errorf("source group not found: %s", groupID)
	}
	metadata := model.GroupMetadata{Group: found}
	if analysis, analysisErr := m.store.LatestSourceAnalysis(rootID, groupID); analysisErr == nil {
		metadata.Analysis = &analysis
	}
	if report, reportErr := m.store.LatestGroupSecurityReport(rootID, groupID); reportErr == nil {
		metadata.SecurityReport = &report
	}
	if operation, operationErr := m.store.LatestGroupOperation(rootID, groupID); operationErr == nil {
		metadata.LatestOperation = &operation
	}
	if statuses, statusErr := m.store.LatestUpdateStatuses(); statusErr == nil {
		for index := range statuses {
			if statuses[index].RootID == rootID && statuses[index].GroupID == groupID {
				metadata.UpdateStatus = &statuses[index]
				break
			}
		}
	}
	return metadata, nil
}

// reconcileGroupOperations marks source-group parent transactions that were
// left "running" by an application exit as recovery-required.  Completed child
// transactions keep their own journals, so the parent rollback entry remains
// the recovery authority and never runs automatically.
func (m *Manager) reconcileGroupOperations(
	transactions []model.Transaction,
) ([]model.Transaction, error) {
	out := append([]model.Transaction(nil), transactions...)
	for index := range out {
		tx := out[index]
		if !strings.EqualFold(strings.TrimSpace(tx.Status), "running") ||
			m.hasActiveGroupRun(tx.OperationID) {
			continue
		}
		message := "应用在整组操作完成前退出；请先回滚已记录的修改 (The application exited before the source-group operation completed; roll back recorded changes first)"
		completedAt := time.Now().UTC()
		operationID := strings.TrimSpace(tx.OperationID)
		if operationID != "" {
			op, operationErr := m.store.GroupOperation(operationID)
			if operationErr != nil {
				tx.Status = model.GroupStatusFailed
				tx.CompletedAt = completedAt
				tx.RecoveryStatus = "required"
				tx.Error = fmt.Sprintf("source-group operation record is unavailable: %v", operationErr)
				if saveErr := m.store.SaveTransaction(tx); saveErr != nil {
					return nil, errors.Join(
						fmt.Errorf("persist interrupted source-group transaction: %w", operationErr),
						saveErr,
					)
				}
				out[index] = tx
				continue
			}
			op.Status = model.GroupStatusRecoveryRequired
			op.CompletedAt = completedAt
			op.RecoveryStatus = "required"
			op.Error = message
			for stepIndex := range op.Steps {
				if op.Steps[stepIndex].Status == "running" || op.Steps[stepIndex].Status == "queued" {
					op.Steps[stepIndex].Status = "interrupted"
					op.Steps[stepIndex].Error = message
				}
			}
			if saveErr := m.store.SaveGroupOperation(op); saveErr != nil {
				return nil, fmt.Errorf("persist interrupted source-group operation: %w", saveErr)
			}
		}
		tx.Status = model.GroupStatusRecoveryRequired
		tx.CompletedAt = completedAt
		tx.RecoveryStatus = "required"
		tx.Error = message
		if saveErr := m.store.SaveTransaction(tx); saveErr != nil {
			return nil, fmt.Errorf("persist interrupted source-group transaction: %w", saveErr)
		}
		out[index] = tx
	}
	return out, nil
}

func (m *Manager) hasActiveGroupRun(operationID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return operationID != "" && m.groupOps[operationID]
}

// rollbackSourceGroup recovers each completed child install in reverse order.
// Child transactions retain the existing backup/hash-aware recovery logic;
// the parent only coordinates and journals their outcome.
func (m *Manager) rollbackSourceGroup(original model.Transaction) (model.Transaction, error) {
	now := time.Now().UTC()
	tx := model.Transaction{
		ID: "tx-" + now.Format("20060102T150405.000000000"), RootID: original.RootID,
		Type: "rollback", Status: "running", GroupID: original.GroupID, GroupName: original.GroupName,
		OperationID: original.OperationID, Targets: append([]string(nil), original.Targets...), StartedAt: now,
	}
	if err := m.store.SaveTransaction(tx); err != nil {
		return model.Transaction{}, err
	}
	failures := make([]string, 0)
	recovered := 0
	for index := len(original.ItemResults) - 1; index >= 0; index-- {
		item := original.ItemResults[index]
		if strings.TrimSpace(item.TransactionID) == "" || item.Status != "completed" {
			continue
		}
		_, err := m.Rollback(item.TransactionID)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", item.Target, err))
			continue
		}
		recovered++
	}
	tx.CompletedAt = time.Now().UTC()
	tx.RecoveryStatus = "completed"
	switch {
	case len(failures) > 0 && recovered > 0:
		tx.Status = "partial"
		tx.RecoveryStatus = "required"
		tx.Error = strings.Join(failures, "; ")
	case len(failures) > 0:
		tx.Status = "failed"
		tx.RecoveryStatus = "required"
		tx.Error = strings.Join(failures, "; ")
	default:
		tx.Status = "completed"
	}
	if err := m.store.SaveTransaction(tx); err != nil {
		return tx, err
	}
	m.recordTransaction(tx)
	if len(failures) > 0 {
		return tx, errors.New(tx.Error)
	}
	return tx, nil
}
