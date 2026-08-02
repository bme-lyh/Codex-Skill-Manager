package model

import "time"

const (
	AssessmentGateReady      = "ready"
	AssessmentGateAttention  = "attention"
	AssessmentGateBlocked    = "blocked"
	AssessmentGateIncomplete = "incomplete"

	AssessmentRequirementRequired  = "required"
	AssessmentRequirementTriggered = "triggered"
	AssessmentRequirementOptional  = "optional"

	AssessmentCheckPassed        = "passed"
	AssessmentCheckAttention     = "attention"
	AssessmentCheckBlocked       = "blocked"
	AssessmentCheckPending       = "pending"
	AssessmentCheckNotApplicable = "not-applicable"
)

// LayeredSecurityCheck is a backend-derived security decision. Evidence is
// bounded to project-relative paths and must never contain file contents.
type LayeredSecurityCheck struct {
	ID            string   `json:"id"`
	Layer         string   `json:"layer"`
	Requirement   string   `json:"requirement"`
	Status        string   `json:"status"`
	Title         string   `json:"title"`
	Summary       string   `json:"summary"`
	Reason        string   `json:"reason,omitempty"`
	Provider      string   `json:"provider"`
	EvidenceFiles []string `json:"evidenceFiles,omitempty"`
}

// InstallTargetPreview describes a locally allowlisted mutation target. Model
// output cannot create or change one of these values.
type InstallTargetPreview struct {
	Kind          string   `json:"kind"`
	DisplayName   string   `json:"displayName"`
	Path          string   `json:"path"`
	Supported     bool     `json:"supported"`
	Reason        string   `json:"reason,omitempty"`
	PermissionIDs []string `json:"permissionIds,omitempty"`
	Reversible    bool     `json:"reversible"`
}

type ProjectAssessmentCoverage struct {
	FilesInventoried int  `json:"filesInventoried"`
	FilesScanned     int  `json:"filesScanned"`
	EvidenceLimited  bool `json:"evidenceLimited"`
}

// ProjectAssessment is the persisted, deterministic local gate for both the
// standard and planned installation paths. It performs no network activity.
type ProjectAssessment struct {
	ID                      string                    `json:"id"`
	SourcePlanID            string                    `json:"sourcePlanId"`
	Repository              Repository                `json:"repository"`
	Classification          string                    `json:"classification"`
	ClassificationEvidence  []string                  `json:"classificationEvidence,omitempty"`
	Checks                  []LayeredSecurityCheck    `json:"checks"`
	Gate                    string                    `json:"gate"`
	Summary                 string                    `json:"summary"`
	HighestRisk             RiskSeverity              `json:"highestRisk"`
	Coverage                ProjectAssessmentCoverage `json:"coverage"`
	Targets                 []InstallTargetPreview    `json:"targets"`
	EnhancedScanRecommended bool                      `json:"enhancedScanRecommended"`
	EnhancedScanReason      string                    `json:"enhancedScanReason,omitempty"`
	SourceDigest            string                    `json:"sourceDigest"`
	AssessmentDigest        string                    `json:"assessmentDigest"`
	CreatedAt               time.Time                 `json:"createdAt"`
	ExpiresAt               time.Time                 `json:"expiresAt"`
}
