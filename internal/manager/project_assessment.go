package manager

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

const (
	maxAssessmentEvidence = 32
	maxAssessmentFiles    = 5000
)

type assessmentInventory struct {
	files          int
	limited        bool
	documentation  []string
	pluginMarkers  []string
	projectMarkers []string
	triggerMarkers []string
}

func (m *Manager) AssessInstallSource(sourcePlanID string) (model.ProjectAssessment, error) {
	preview, err := m.assistedSourcePreview(sourcePlanID)
	if err != nil {
		return model.ProjectAssessment{}, err
	}
	return m.assessInstallPreview(preview, true)
}

func (m *Manager) GetProjectAssessment(reference string) (model.ProjectAssessment, error) {
	reference = strings.TrimSpace(reference)
	var assessment model.ProjectAssessment
	var err error
	if validAssessmentID(reference) {
		assessment, err = loadProjectAssessment(m.Config.Paths.DataRoot, reference)
	} else if strings.HasPrefix(reference, "plan-") && filepath.Base(reference) == reference {
		assessment, err = latestProjectAssessmentForSource(m.Config.Paths.DataRoot, reference)
	} else {
		return model.ProjectAssessment{}, errors.New("invalid project assessment reference")
	}
	if err != nil {
		return model.ProjectAssessment{}, err
	}
	preview, err := m.assistedSourcePreview(assessment.SourcePlanID)
	if err != nil {
		return model.ProjectAssessment{}, err
	}
	if err := m.verifyAssessmentAgainstPreview(assessment, preview); err != nil {
		return model.ProjectAssessment{}, err
	}
	if time.Now().UTC().After(assessment.ExpiresAt) {
		return model.ProjectAssessment{}, errors.New("project assessment has expired")
	}
	if err := verifyProjectAssessment(assessment); err != nil {
		return model.ProjectAssessment{}, err
	}
	return assessment, nil
}

func (m *Manager) verifyAssessmentAgainstPreview(assessment model.ProjectAssessment, preview model.InstallPreview) error {
	if !strings.EqualFold(assessment.SourceDigest, preview.PreviewDigest) || assessment.SourcePlanID != preview.ID {
		return errors.New("project assessment source digest mismatch")
	}
	if assessment.CreatedAt.Before(preview.CreatedAt) || !assessment.CreatedAt.Before(assessment.ExpiresAt) ||
		!assessment.ExpiresAt.Equal(preview.ExpiresAt) {
		return errors.New("project assessment timestamps are not bound to the source preview")
	}
	if assessment.Repository.Provider != preview.Repository.Provider ||
		assessment.Repository.FullName != preview.Repository.FullName ||
		assessment.Repository.CommitSHA != preview.Repository.CommitSHA ||
		assessment.Repository.LocalPath != preview.Repository.LocalPath {
		return errors.New("project assessment repository is not bound to the source preview")
	}
	if len(assessment.Targets) != len(preview.Skills) {
		return errors.New("project assessment targets do not match the source preview")
	}
	root, err := m.resolveRoot(preview.TargetRootID)
	if err != nil {
		return err
	}
	expected := make(map[string]string, len(preview.Skills))
	for _, skill := range preview.Skills {
		expected[skill.Name] = filepath.Join(root.Path, skill.Name)
	}
	for _, target := range assessment.Targets {
		path, ok := expected[target.DisplayName]
		if !ok || !strings.EqualFold(filepath.Clean(target.Path), filepath.Clean(path)) {
			return errors.New("project assessment target path is not backend-authoritative")
		}
	}
	return nil
}

// enforceProjectAssessmentGate is called by every mutating installation path.
// It refreshes ignored-finding decoration so an old UI result cannot grant a
// stale authorization.
func (m *Manager) enforceProjectAssessmentGate(preview model.InstallPreview) (model.ProjectAssessment, error) {
	return m.enforceProjectAssessmentGateWithRisk(preview, false)
}

// enforceProjectAssessmentGateWithRisk is used only by a source-group apply
// after a persisted human risk decision.  It still verifies the complete
// assessment and all technical target/digest checks; the explicit allowance
// only changes the risk gate from a hard stop to a reviewable decision.
func (m *Manager) enforceProjectAssessmentGateWithRisk(preview model.InstallPreview, allowBlockingRisk bool) (model.ProjectAssessment, error) {
	assessment, err := m.assessInstallPreview(preview, true)
	if err != nil {
		return model.ProjectAssessment{}, err
	}
	switch assessment.Gate {
	case model.AssessmentGateReady, model.AssessmentGateAttention:
		return assessment, nil
	case model.AssessmentGateBlocked:
		if allowBlockingRisk && (assessment.HighestRisk == model.RiskHigh || assessment.HighestRisk == model.RiskCritical) {
			return assessment, nil
		}
		return model.ProjectAssessment{}, errors.New("project assessment blocked installation")
	case model.AssessmentGateIncomplete:
		return model.ProjectAssessment{}, errors.New("project assessment is incomplete")
	default:
		return model.ProjectAssessment{}, errors.New("project assessment returned an unknown gate")
	}
}

func (m *Manager) assessInstallPreview(preview model.InstallPreview, persist bool) (model.ProjectAssessment, error) {
	if err := m.verifyInstallPreviewMetadata(preview, preview.ID); err != nil {
		return model.ProjectAssessment{}, err
	}
	ignored, err := m.store.IgnoredFindings()
	if err != nil {
		return model.ProjectAssessment{}, err
	}
	preview.Scan = m.decorateScan(preview.Scan, ignored)
	inventory, err := inventoryAssessmentMarkers(preview.StagingPath)
	if err != nil {
		return model.ProjectAssessment{}, err
	}
	classification, evidence := classifyAssessment(preview, inventory)
	checks, gate, summary, enhanced, enhancedReason := m.assessmentChecks(preview, inventory, classification)
	root, err := m.resolveRoot(preview.TargetRootID)
	if err != nil {
		return model.ProjectAssessment{}, err
	}
	targets := make([]model.InstallTargetPreview, 0, len(preview.Skills))
	for _, skill := range preview.Skills {
		if !validMutableSkillName(skill.Name) {
			return model.ProjectAssessment{}, fmt.Errorf("invalid Skill target: %s", skill.Name)
		}
		targets = append(targets, model.InstallTargetPreview{
			Kind: "codex-skill", DisplayName: skill.Name,
			Path: filepath.Join(root.Path, skill.Name), Supported: true,
			PermissionIDs: []string{model.AssistedInstallPermissionSkillsWrite}, Reversible: true,
		})
	}
	now := time.Now().UTC()
	assessment := model.ProjectAssessment{
		ID:           "assessment-" + now.Format("20060102T150405.000000000"),
		SourcePlanID: preview.ID, Repository: preview.Repository,
		Classification: classification, ClassificationEvidence: evidence,
		Checks: checks, Gate: gate, Summary: summary,
		HighestRisk: preview.Scan.ActiveHighestSeverity,
		Coverage: model.ProjectAssessmentCoverage{
			FilesInventoried: inventory.files, FilesScanned: preview.Scan.FilesScanned,
			EvidenceLimited: inventory.limited,
		},
		Targets: targets, EnhancedScanRecommended: enhanced,
		EnhancedScanReason: enhancedReason, SourceDigest: preview.PreviewDigest,
		CreatedAt: now, ExpiresAt: preview.ExpiresAt,
	}
	if err := sealProjectAssessment(&assessment); err != nil {
		return model.ProjectAssessment{}, err
	}
	if persist {
		if err := saveProjectAssessment(m.Config.Paths.DataRoot, assessment); err != nil {
			return model.ProjectAssessment{}, err
		}
	}
	return assessment, nil
}

func (m *Manager) assessmentChecks(preview model.InstallPreview, inventory assessmentInventory, classification string) ([]model.LayeredSecurityCheck, string, string, bool, string) {
	checks := []model.LayeredSecurityCheck{
		{ID: "source-identity", Layer: "intake", Requirement: model.AssessmentRequirementRequired, Status: model.AssessmentCheckPassed, Title: "Source identity", Summary: "The source is bound to a managed installation preview.", Provider: "local"},
		{ID: "safe-staging", Layer: "intake", Requirement: model.AssessmentRequirementRequired, Status: model.AssessmentCheckPassed, Title: "Managed staging", Summary: "Assessment uses a managed, digest-bound source snapshot.", Provider: "local"},
	}
	docStatus := model.AssessmentCheckPassed
	docSummary := "Project documentation was discovered for installation review."
	if len(inventory.documentation) == 0 {
		docStatus = model.AssessmentCheckAttention
		docSummary = "No README or installation document was discovered."
	}
	checks = append(checks, model.LayeredSecurityCheck{ID: "documentation", Layer: "understanding", Requirement: model.AssessmentRequirementRequired, Status: docStatus, Title: "Project documentation", Summary: docSummary, Provider: "local", EvidenceFiles: inventory.documentation})
	checks = append(checks, model.LayeredSecurityCheck{ID: "skill-discovery", Layer: "understanding", Requirement: model.AssessmentRequirementRequired, Status: model.AssessmentCheckPassed, Title: "Codex Skill discovery", Summary: fmt.Sprintf("%d valid Skill target(s) were discovered.", len(preview.Skills)), Provider: "local", EvidenceFiles: skillEvidence(preview.Skills)})

	riskStatus := model.AssessmentCheckPassed
	riskSummary := "The built-in scanner found no active blocking risk."
	gate := model.AssessmentGateReady
	switch preview.Scan.ActiveHighestSeverity {
	case model.RiskCritical, model.RiskHigh:
		riskStatus, gate = model.AssessmentCheckBlocked, model.AssessmentGateBlocked
		riskSummary = "Active High or Critical local findings block installation."
	case model.RiskMedium, model.RiskLow:
		riskStatus, gate = model.AssessmentCheckAttention, model.AssessmentGateAttention
		riskSummary = "The local scanner found active findings that require review."
	case model.RiskInfo, "":
	default:
		riskStatus, gate = model.AssessmentCheckBlocked, model.AssessmentGateBlocked
		riskSummary = "The local scanner returned an unknown severity and failed closed."
	}
	checks = append(checks, model.LayeredSecurityCheck{ID: "local-scan", Layer: "baseline", Requirement: model.AssessmentRequirementRequired, Status: riskStatus, Title: "Built-in security scan", Summary: riskSummary, Provider: "builtin-scanner"})
	checks = append(checks, model.LayeredSecurityCheck{ID: "target-recovery", Layer: "install", Requirement: model.AssessmentRequirementRequired, Status: model.AssessmentCheckPassed, Title: "Target and recovery", Summary: "Skill targets are explicit and installation is transaction-journaled with backup recovery.", Provider: "local"})

	enhanced := classification != "skill" || len(inventory.triggerMarkers) > 0 || len(inventory.documentation) == 0 || inventory.limited
	enhancedReason := ""
	if enhanced {
		enhancedReason = "Repository complexity or incomplete local evidence warrants an explicit semantic project scan."
		checks = append(checks, model.LayeredSecurityCheck{ID: "enhanced-project-scan", Layer: "semantic", Requirement: model.AssessmentRequirementTriggered, Status: model.AssessmentCheckPending, Title: "Enhanced project scan", Summary: "Available after explicit consent; it performs no installation.", Reason: enhancedReason, Provider: "codex", EvidenceFiles: inventory.triggerMarkers})
		if gate == model.AssessmentGateReady {
			gate = model.AssessmentGateAttention
		}
	}
	checks = append(checks, model.LayeredSecurityCheck{ID: "deep-providers", Layer: "deep", Requirement: model.AssessmentRequirementOptional, Status: model.AssessmentCheckPending, Title: "Optional deep checks", Summary: "External SAST, container, SBOM, and dynamic providers are not run automatically.", Provider: "optional"})
	if inventory.limited && gate != model.AssessmentGateBlocked {
		gate = model.AssessmentGateIncomplete
	}
	if len(preview.Skills) == 0 && gate != model.AssessmentGateBlocked {
		gate = model.AssessmentGateIncomplete
	}
	summary := map[string]string{
		model.AssessmentGateReady:      "Required local checks passed.",
		model.AssessmentGateAttention:  "Local checks completed; review the highlighted items before installation.",
		model.AssessmentGateBlocked:    "Installation is blocked by the local security policy.",
		model.AssessmentGateIncomplete: "Required local coverage is incomplete.",
	}[gate]
	return checks, gate, summary, enhanced, enhancedReason
}

func inventoryAssessmentMarkers(root string) (assessmentInventory, error) {
	value := assessmentInventory{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("assessment rejects linked path: %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		value.files++
		if value.files > maxAssessmentFiles {
			value.limited = true
			return fs.SkipAll
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		lower := strings.ToLower(relative)
		base := strings.ToLower(filepath.Base(relative))
		if strings.HasPrefix(base, "readme") || strings.HasPrefix(base, "install") || strings.Contains(lower, "getting-started") {
			appendEvidence(&value.documentation, relative)
		}
		if lower == ".codex-plugin/plugin.json" {
			appendEvidence(&value.pluginMarkers, relative)
		}
		switch base {
		case "go.mod", "package.json", "pyproject.toml", "cargo.toml", "requirements.txt":
			appendEvidence(&value.projectMarkers, relative)
		case "dockerfile", "docker-compose.yml", "docker-compose.yaml", "install.ps1", "install.sh", "setup.exe":
			appendEvidence(&value.triggerMarkers, relative)
		}
		if strings.HasSuffix(lower, ".tf") || strings.HasSuffix(lower, ".exe") || strings.HasSuffix(lower, ".dll") {
			appendEvidence(&value.triggerMarkers, relative)
		}
		return nil
	})
	return value, err
}

func appendEvidence(values *[]string, value string) {
	if len(*values) < maxAssessmentEvidence {
		*values = append(*values, value)
	}
}

func classifyAssessment(preview model.InstallPreview, inventory assessmentInventory) (string, []string) {
	evidence := append([]string(nil), inventory.pluginMarkers...)
	evidence = append(evidence, inventory.projectMarkers...)
	evidence = append(evidence, skillEvidence(preview.Skills)...)
	sort.Strings(evidence)
	hasSkill, hasPlugin, hasProject := len(preview.Skills) > 0, len(inventory.pluginMarkers) > 0, len(inventory.projectMarkers) > 0 || len(inventory.triggerMarkers) > 0
	switch {
	case hasSkill && (hasPlugin || hasProject):
		return "mixed", boundedEvidence(evidence)
	case hasPlugin:
		return "plugin", boundedEvidence(evidence)
	case hasSkill:
		return "skill", boundedEvidence(evidence)
	case hasProject && len(inventory.triggerMarkers) > 0:
		return "application", boundedEvidence(evidence)
	case hasProject:
		return "library", boundedEvidence(evidence)
	default:
		return "unknown", boundedEvidence(evidence)
	}
}

func skillEvidence(skills []model.CandidateSkill) []string {
	values := make([]string, 0, len(skills))
	for _, skill := range skills {
		appendEvidence(&values, filepath.ToSlash(filepath.Join(skill.SourcePath, "SKILL.md")))
	}
	return values
}

func boundedEvidence(values []string) []string {
	if len(values) > maxAssessmentEvidence {
		return values[:maxAssessmentEvidence]
	}
	return values
}

func projectAssessmentDigest(value model.ProjectAssessment) (string, error) {
	value.AssessmentDigest = ""
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func sealProjectAssessment(value *model.ProjectAssessment) error {
	digest, err := projectAssessmentDigest(*value)
	if err != nil {
		return err
	}
	value.AssessmentDigest = digest
	return verifyProjectAssessment(*value)
}

func verifyProjectAssessment(value model.ProjectAssessment) error {
	if !validAssessmentID(value.ID) || !strings.HasPrefix(value.SourcePlanID, "plan-") {
		return errors.New("invalid project assessment identity")
	}
	if value.CreatedAt.IsZero() || value.ExpiresAt.IsZero() || !value.ExpiresAt.After(value.CreatedAt) {
		return errors.New("invalid project assessment timestamps")
	}
	if !validHexDigest(value.SourceDigest, sha256.Size) || !validHexDigest(value.AssessmentDigest, sha256.Size) {
		return errors.New("invalid project assessment digest shape")
	}
	switch value.Repository.Provider {
	case "github":
		if !immutableGitHubSHA.MatchString(value.Repository.CommitSHA) {
			return errors.New("assessment GitHub source is not pinned to a full commit SHA")
		}
	case "local":
	default:
		return errors.New("unknown project assessment source provider")
	}
	switch value.Classification {
	case "skill", "plugin", "application", "library", "mixed", "unknown":
	default:
		return errors.New("unknown project classification")
	}
	if !validRiskSeverity(value.HighestRisk) {
		return errors.New("unknown project assessment risk severity")
	}
	if value.Coverage.FilesInventoried < 0 || value.Coverage.FilesScanned < 0 {
		return errors.New("invalid project assessment coverage")
	}
	switch value.Gate {
	case model.AssessmentGateReady, model.AssessmentGateAttention, model.AssessmentGateBlocked, model.AssessmentGateIncomplete:
	default:
		return errors.New("unknown project assessment gate")
	}
	if len(value.Checks) == 0 {
		return errors.New("project assessment has no checks")
	}
	seenChecks := map[string]bool{}
	for _, check := range value.Checks {
		if strings.TrimSpace(check.ID) == "" || seenChecks[check.ID] || strings.TrimSpace(check.Provider) == "" {
			return errors.New("invalid or duplicate project assessment check")
		}
		seenChecks[check.ID] = true
		switch check.Requirement {
		case model.AssessmentRequirementRequired, model.AssessmentRequirementTriggered, model.AssessmentRequirementOptional:
		default:
			return errors.New("unknown project assessment requirement")
		}
		switch check.Status {
		case model.AssessmentCheckPassed, model.AssessmentCheckAttention, model.AssessmentCheckBlocked, model.AssessmentCheckPending, model.AssessmentCheckNotApplicable:
		default:
			return errors.New("unknown project assessment check status")
		}
		if check.Requirement == model.AssessmentRequirementRequired {
			if value.Gate == model.AssessmentGateReady && check.Status != model.AssessmentCheckPassed && check.Status != model.AssessmentCheckNotApplicable {
				return errors.New("ready project assessment contains an unresolved required check")
			}
			if (value.Gate == model.AssessmentGateReady || value.Gate == model.AssessmentGateAttention) &&
				(check.Status == model.AssessmentCheckBlocked || check.Status == model.AssessmentCheckPending) {
				return errors.New("permissive project assessment contains a blocked required check")
			}
		}
		for _, evidence := range check.EvidenceFiles {
			clean := filepath.Clean(filepath.FromSlash(evidence))
			if evidence == "" || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
				return errors.New("project assessment contains unsafe evidence path")
			}
		}
	}
	if isBlockingRiskSeverity(value.HighestRisk) && value.Gate != model.AssessmentGateBlocked {
		return errors.New("blocking risk is inconsistent with project assessment gate")
	}
	for _, target := range value.Targets {
		if target.Kind != "codex-skill" || !target.Supported || !target.Reversible ||
			!validMutableSkillName(target.DisplayName) || !filepath.IsAbs(target.Path) ||
			!strings.EqualFold(filepath.Base(target.Path), target.DisplayName) ||
			len(target.PermissionIDs) != 1 || target.PermissionIDs[0] != model.AssistedInstallPermissionSkillsWrite {
			return errors.New("unknown or unsafe project assessment target")
		}
	}
	expected, err := projectAssessmentDigest(value)
	if err != nil {
		return err
	}
	if value.AssessmentDigest == "" || !strings.EqualFold(value.AssessmentDigest, expected) {
		return errors.New("project assessment integrity digest mismatch")
	}
	return nil
}

func assessmentRoot(dataRoot string) string { return filepath.Join(dataRoot, "project-assessments") }

func saveProjectAssessment(dataRoot string, value model.ProjectAssessment) error {
	if err := verifyProjectAssessment(value); err != nil {
		return err
	}
	root := assessmentRoot(dataRoot)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, value.ID+".json"), data, 0o600)
}

func loadProjectAssessment(dataRoot, id string) (model.ProjectAssessment, error) {
	if !validAssessmentID(id) {
		return model.ProjectAssessment{}, errors.New("invalid project assessment ID")
	}
	data, err := os.ReadFile(filepath.Join(assessmentRoot(dataRoot), id+".json"))
	if err != nil {
		return model.ProjectAssessment{}, err
	}
	var value model.ProjectAssessment
	if err := json.Unmarshal(data, &value); err != nil {
		return model.ProjectAssessment{}, err
	}
	if value.ID != id {
		return model.ProjectAssessment{}, errors.New("project assessment identity mismatch")
	}
	if err := verifyProjectAssessment(value); err != nil {
		return model.ProjectAssessment{}, err
	}
	return value, nil
}

func latestProjectAssessmentForSource(dataRoot, sourcePlanID string) (model.ProjectAssessment, error) {
	entries, err := os.ReadDir(assessmentRoot(dataRoot))
	if err != nil {
		return model.ProjectAssessment{}, err
	}
	var selected model.ProjectAssessment
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		value, loadErr := loadProjectAssessment(dataRoot, strings.TrimSuffix(entry.Name(), ".json"))
		if loadErr == nil && value.SourcePlanID == sourcePlanID && value.CreatedAt.After(selected.CreatedAt) {
			selected = value
		}
	}
	if selected.ID == "" {
		return model.ProjectAssessment{}, errors.New("project assessment was not found")
	}
	return selected, nil
}

func validAssessmentID(value string) bool {
	if filepath.Base(value) != value || !strings.HasPrefix(value, "assessment-") || len(value) > 96 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
			continue
		}
		return false
	}
	return true
}

func validHexDigest(value string, expectedBytes int) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == expectedBytes
}
