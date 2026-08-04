package model

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const Version = "0.14.0"
const SourcesLockSchemaVersion = 2

// Skill root identifiers are part of the persisted identity of a Skill.  A
// name is only unique within a root; callers must use the qualified identity
// when addressing a Skill across roots.
const (
	RootIDCodexDefault = "codex-default"
	RootIDAgents       = "agents"
	DefaultRootID      = RootIDCodexDefault
)

const (
	AssistedInstallStepInstallSkills     = "install-skills"
	AssistedInstallStepManagedPythonTool = "managed-python-tool"
	AssistedInstallStepConfigureCodexMCP = "configure-codex-mcp"
	AssistedInstallStepManual            = "manual"

	AssistedInstallPermissionSkillsWrite       = "skills-write"
	AssistedInstallPermissionPyPIWheelLock     = "pypi-wheel-lock"
	AssistedInstallPermissionManagedToolWrite  = "managed-tool-write"
	AssistedInstallPermissionManagedToolRun    = "managed-tool-run"
	AssistedInstallPermissionManagedNativeCode = "managed-native-code"
	AssistedInstallPermissionCodexMCPConfig    = "codex-mcp-config"

	// AssistedInstallPermissionPyPINetwork is a deprecated source-compatible
	// alias. Applying the immutable Wheel lock is offline and grants no network.
	AssistedInstallPermissionPyPINetwork = AssistedInstallPermissionPyPIWheelLock
)

type RiskSeverity string

const (
	RiskInfo     RiskSeverity = "informational"
	RiskLow      RiskSeverity = "low"
	RiskMedium   RiskSeverity = "medium"
	RiskHigh     RiskSeverity = "high"
	RiskCritical RiskSeverity = "critical"
)

// Source association is deliberately separate from Provider.  Provider
// describes how a record was obtained (for example, github or local), while
// SourceAssociation describes whether an installed/local Skill has a
// verifiable remote identity.  Keeping the values as strings preserves the
// JSON contract used by older clients and makes an absent value safe to read
// as unknown/unlinked.
const (
	SourceAssociationRemote   = "remote"
	SourceAssociationLocal    = "local"
	SourceAssociationUnlinked = "unlinked"

	// SourceAssociationLinked is retained as a concise compatibility alias for
	// callers that only distinguish linked versus unlinked sources.  Remote is
	// the canonical value because local snapshots are not remote associations.
	SourceAssociationLinked = SourceAssociationRemote
)

const (
	ScanStatusPassed   = "passed"
	ScanStatusFindings = "findings"
	ScanStatusFailed   = "failed"
)

// NormalizeSourceAssociation returns a stable association value for a source
// record.  Older locks and inventory payloads may omit the field, so provider
// is used only as a conservative fallback; unknown providers remain
// unlinked rather than being treated as remotely trusted.
func NormalizeSourceAssociation(provider, association string) string {
	value := strings.ToLower(strings.TrimSpace(association))
	switch value {
	case SourceAssociationRemote, "remote-linked", "github", "linked":
		return SourceAssociationRemote
	case SourceAssociationLocal, "local-linked":
		return SourceAssociationLocal
	case SourceAssociationUnlinked, "unknown", "none":
		return SourceAssociationUnlinked
	}
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "github", "git", "remote":
		return SourceAssociationRemote
	case "local":
		// A local provider without an explicit association is an unmanaged or
		// legacy local Skill.  Keep it visibly unlinked; an explicitly recorded
		// local association still reaches the branch above.
		return SourceAssociationUnlinked
	default:
		return SourceAssociationUnlinked
	}
}

// IsRemoteSourceAssociation reports whether a record has a remote source
// association.  It intentionally accepts both the canonical value and the
// legacy linked spelling so callers can safely inspect old payloads.
func IsRemoteSourceAssociation(provider, association string) bool {
	return NormalizeSourceAssociation(provider, association) == SourceAssociationRemote
}

// IsImmutableCommitSHA validates the full 160-bit hexadecimal commit form
// required for remote source locks.  Prefixes, abbreviated SHAs, refs, and
// mixed metadata are rejected.  The check is kept in model so every reader
// and writer can enforce the same immutable-ref rule without importing a
// higher-level manager package.
func IsImmutableCommitSHA(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 40 {
		return false
	}
	for _, char := range value {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return false
		}
	}
	return true
}

// IsImmutableGitHubCommit is a descriptive alias used by source-provider
// callers.  It intentionally has the same strict full-SHA semantics.
func IsImmutableGitHubCommit(value string) bool { return IsImmutableCommitSHA(value) }

// IsImmutableRef is a short compatibility alias for callers that use ref
// terminology rather than commit terminology.
func IsImmutableRef(value string) bool { return IsImmutableCommitSHA(value) }

// CanonicalCommitSHA normalizes a full commit SHA for lock persistence.  An
// empty value remains empty for legacy/local records; a non-empty mutable ref
// is rejected so it cannot silently become an apparently immutable lock.
func CanonicalCommitSHA(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if !IsImmutableCommitSHA(value) {
		return "", fmt.Errorf("commit is not a full immutable SHA")
	}
	return strings.ToLower(value), nil
}

// CanonicalGitHubRepository normalizes an owner/repository identity for
// repository-wide trust policy.  It accepts the common HTTPS/SSH URL forms
// but persists only the lower-case owner/repository key; query, fragment,
// nested paths and empty components are rejected.
func CanonicalGitHubRepository(value string) (string, error) {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return "", errors.New("GitHub repository is required")
	}
	if strings.HasPrefix(strings.ToLower(raw), "git@github.com:") {
		raw = raw[len("git@github.com:"):]
	} else if strings.HasPrefix(strings.ToLower(raw), "ssh://git@github.com/") {
		raw = raw[len("ssh://git@github.com/"):]
	} else if strings.HasPrefix(strings.ToLower(raw), "https://github.com/") {
		raw = raw[len("https://github.com/"):]
	} else if strings.HasPrefix(strings.ToLower(raw), "http://github.com/") {
		raw = raw[len("http://github.com/"):]
	}
	raw = strings.TrimSuffix(raw, ".git")
	if strings.ContainsAny(raw, "?#\\") {
		return "", errors.New("GitHub repository must not include query, fragment, or backslash")
	}
	parts := strings.Split(strings.Trim(raw, "/"), "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", errors.New("GitHub repository must use owner/repository form")
	}
	for _, part := range parts {
		for _, r := range part {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
				continue
			}
			return "", errors.New("GitHub repository contains an invalid character")
		}
		if part == "." || part == ".." {
			return "", errors.New("GitHub repository contains an invalid component")
		}
	}
	return strings.ToLower(parts[0] + "/" + parts[1]), nil
}

// ValidateImmutableRef applies the remote-provider lock rule.  Local sources
// are content-bound by their file hashes and therefore do not require a Git
// commit; a remote provider must carry a full commit SHA whenever a revision is
// supplied or when the caller asks for a remote lock.
func ValidateImmutableRef(provider, commit string) error {
	if strings.EqualFold(strings.TrimSpace(provider), "github") {
		if !IsImmutableCommitSHA(commit) {
			return fmt.Errorf("github source must use a full immutable commit SHA")
		}
		return nil
	}
	// Local providers bind content through their tree/file hashes and may use
	// a non-Git digest in a provider-specific field.  Do not reject that
	// metadata here; only GitHub refs require the 40-character commit form.
	return nil
}

type Paths struct {
	SkillsRoot     string `json:"skillsRoot" yaml:"skillsRoot"`
	DataRoot       string `json:"dataRoot" yaml:"dataRoot"`
	LogsRoot       string `json:"logsRoot" yaml:"logsRoot"`
	ReportsRoot    string `json:"reportsRoot" yaml:"reportsRoot"`
	BackupsRoot    string `json:"backupsRoot" yaml:"backupsRoot"`
	QuarantineRoot string `json:"quarantineRoot" yaml:"quarantineRoot"`
	CacheRoot      string `json:"cacheRoot" yaml:"cacheRoot"`
	StagingRoot    string `json:"stagingRoot" yaml:"stagingRoot"`
}

// SkillRoot describes one Codex-compatible Skills directory.  Enabled roots
// are discovered when present, but are deliberately not created by config
// loading.  A writer must explicitly create its target root before mutation.
type SkillRoot struct {
	ID        string `json:"id" yaml:"id"`
	Name      string `json:"name,omitempty" yaml:"name,omitempty"`
	Kind      string `json:"kind,omitempty" yaml:"kind,omitempty"`
	Path      string `json:"path" yaml:"path"`
	Enabled   bool   `json:"enabled" yaml:"enabled"`
	ReadOnly  bool   `json:"readOnly,omitempty" yaml:"readOnly,omitempty"`
	SystemDir string `json:"systemDir,omitempty" yaml:"systemDir,omitempty"`
}

// Root is retained as a source-compatible alias for integrations that use the
// shorter name in their API payloads.
type Root = SkillRoot

type RootConfig = SkillRoot
type SkillRootConfig = SkillRoot

type Schedule struct {
	Enabled   bool   `json:"enabled" yaml:"enabled"`
	Frequency string `json:"frequency" yaml:"frequency"`
	Time      string `json:"time" yaml:"time"`
}

type CodexReviewConfig struct {
	Enabled            bool   `json:"enabled" yaml:"enabled"`
	CLIPath            string `json:"cliPath,omitempty" yaml:"cliPath,omitempty"`
	Model              string `json:"model" yaml:"model"`
	ReasoningEffort    string `json:"reasoningEffort" yaml:"reasoningEffort"`
	TimeoutSeconds     int    `json:"timeoutSeconds" yaml:"timeoutSeconds"`
	MaxSamplePerRisk   int    `json:"maxSamplePerRisk" yaml:"maxSamplePerRisk"`
	MaxParallelBatches int    `json:"maxParallelBatches" yaml:"maxParallelBatches"`
	OutputLocale       string `json:"-" yaml:"-"`
}

type Config struct {
	SchemaVersion int         `json:"schemaVersion" yaml:"schemaVersion"`
	Paths         Paths       `json:"paths" yaml:"paths"`
	SkillRoots    []SkillRoot `json:"skillRoots,omitempty" yaml:"skillRoots,omitempty"`
	// Roots is an API spelling accepted by schema v2 readers.  Save emits only
	// SkillRoots; Load mirrors it into both fields for callers during the v0.11
	// transition.
	Roots         []SkillRoot       `json:"roots,omitempty" yaml:"roots,omitempty"`
	DefaultRootID string            `json:"defaultRootId" yaml:"defaultRootId"`
	Schedule      Schedule          `json:"schedule" yaml:"schedule"`
	Locale        string            `json:"locale" yaml:"locale"`
	Theme         string            `json:"theme" yaml:"theme"`
	GitHubHost    string            `json:"githubHost" yaml:"githubHost"`
	AllowOwners   []string          `json:"allowOwners,omitempty" yaml:"allowOwners,omitempty"`
	MaxFileBytes  int64             `json:"maxFileBytes" yaml:"maxFileBytes"`
	MaxFiles      int               `json:"maxFiles" yaml:"maxFiles"`
	CodexReview   CodexReviewConfig `json:"codexReview" yaml:"codexReview"`
}

type FileRecord struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	Kind   string `json:"kind"`
}

type Skill struct {
	Name            string `json:"name"`
	RootID          string `json:"rootId,omitempty"`
	Identity        string `json:"identity,omitempty"`
	Description     string `json:"description"`
	Path            string `json:"path"`
	GroupID         string `json:"groupId"`
	GroupName       string `json:"groupName"`
	SourceGroupID   string `json:"sourceGroupId"`
	SourceGroupName string `json:"sourceGroupName"`
	SourceProvider  string `json:"sourceProvider"`
	// SourceAssociation is remote, local, or unlinked.  It is independent of
	// SourceProvider so an unmanaged local Skill cannot be mistaken for a
	// remotely verified source merely because it has a local path.
	SourceAssociation string       `json:"sourceAssociation,omitempty"`
	SourceConfidence  float64      `json:"sourceConfidence"`
	SourceEvidence    string       `json:"sourceEvidence,omitempty"`
	Managed           bool         `json:"managed"`
	System            bool         `json:"system"`
	LocalModified     bool         `json:"localModified"`
	SecurityStatus    string       `json:"securityStatus"`
	UpdateStatus      string       `json:"updateStatus"`
	InstalledCommit   string       `json:"installedCommit,omitempty"`
	SourceRepository  string       `json:"sourceRepository,omitempty"`
	SourcePath        string       `json:"sourcePath,omitempty"`
	Files             []FileRecord `json:"files,omitempty"`
	Dependencies      []string     `json:"dependencies,omitempty"`
	Relationships     []Relation   `json:"relationships,omitempty"`
	LastChecked       *time.Time   `json:"lastChecked,omitempty"`
	LastSecurityScan  *time.Time   `json:"lastSecurityScan,omitempty"`
	SecurityChanged   bool         `json:"securityChanged"`
}

type Group struct {
	ID         string   `json:"id"`
	RootID     string   `json:"rootId,omitempty"`
	Name       string   `json:"name"`
	Provider   string   `json:"provider"`
	Repository string   `json:"repository,omitempty"`
	ReadOnly   bool     `json:"readOnly"`
	Manual     bool     `json:"manual"`
	Position   int      `json:"position"`
	SkillNames []string `json:"skillNames"`
	Status     string   `json:"status"`
}

// Source-group status values are intentionally shared by install, update, and
// security views. Skill-level diagnostics must not introduce another primary
// state machine.
const (
	GroupStatusUnknown          = "unknown"
	GroupStatusPreparing        = "preparing"
	GroupStatusAnalyzing        = "analyzing"
	GroupStatusSecurityChecking = "security-checking"
	GroupStatusAwaitingApproval = "awaiting-approval"
	GroupStatusInstalling       = "installing"
	GroupStatusChecking         = "checking"
	GroupStatusCompleted        = "completed"
	GroupStatusPartial          = "partial"
	GroupStatusFailed           = "failed"
	GroupStatusRecoveryRequired = "recovery-required"
	GroupStatusUpdateAvailable  = "update-available"
	GroupStatusUpToDate         = "up-to-date"
	GroupStatusRateLimited      = "rate-limited"
	GroupStatusUnsupported      = "unsupported"
)

func ValidGroupStatus(value string) bool {
	switch strings.TrimSpace(value) {
	case GroupStatusUnknown, GroupStatusPreparing, GroupStatusAnalyzing, GroupStatusSecurityChecking,
		GroupStatusAwaitingApproval, GroupStatusInstalling, GroupStatusChecking, GroupStatusCompleted,
		GroupStatusPartial, GroupStatusFailed, GroupStatusRecoveryRequired, GroupStatusUpdateAvailable,
		GroupStatusUpToDate, GroupStatusRateLimited, GroupStatusUnsupported:
		return true
	default:
		return false
	}
}

type DetectedSource struct {
	SkillName         string  `json:"skillName"`
	RootID            string  `json:"rootId,omitempty"`
	Provider          string  `json:"provider"`
	SourceAssociation string  `json:"sourceAssociation,omitempty"`
	Repository        string  `json:"repository"`
	SourceURL         string  `json:"sourceUrl"`
	RequestedRef      string  `json:"requestedRef,omitempty"`
	ResolvedCommit    string  `json:"resolvedCommit,omitempty"`
	SourcePath        string  `json:"sourcePath"`
	GroupID           string  `json:"groupId"`
	GroupName         string  `json:"groupName"`
	Confidence        float64 `json:"confidence"`
	Evidence          string  `json:"evidence"`
}

type GroupPreference struct {
	ID       string `json:"id"`
	RootID   string `json:"rootId,omitempty"`
	Name     string `json:"name"`
	Position int    `json:"position"`
	Manual   bool   `json:"manual"`
}

type SkillGroupAssignment struct {
	SkillName string `json:"skillName"`
	RootID    string `json:"rootId,omitempty"`
	GroupID   string `json:"groupId"`
	Position  int    `json:"position"`
}

type GroupLayoutState struct {
	Groups      []GroupPreference      `json:"groups"`
	Assignments []SkillGroupAssignment `json:"assignments"`
}

type Relation struct {
	From       string  `json:"from"`
	To         string  `json:"to"`
	RootID     string  `json:"rootId,omitempty"`
	Type       string  `json:"type"`
	Confidence float64 `json:"confidence"`
	Evidence   string  `json:"evidence,omitempty"`
}

type Finding struct {
	RuleID         string       `json:"ruleId"`
	Title          string       `json:"title"`
	Severity       RiskSeverity `json:"severity"`
	Confidence     float64      `json:"confidence"`
	File           string       `json:"file"`
	Line           int          `json:"line"`
	Evidence       string       `json:"evidence"`
	Explanation    string       `json:"explanation"`
	Recommendation string       `json:"recommendedAction"`
	Fingerprint    string       `json:"fingerprint,omitempty"`
	Ignored        bool         `json:"ignored"`
	IgnoreReason   string       `json:"ignoreReason,omitempty"`
	FileClass      string       `json:"fileClass"`
	Category       string       `json:"category"`
	ClusterID      string       `json:"clusterId"`
	Deterministic  bool         `json:"deterministic"`
	SkillName      string       `json:"skillName,omitempty"`
	RootID         string       `json:"rootId,omitempty"`
	GroupID        string       `json:"groupId,omitempty"`
	GroupName      string       `json:"groupName,omitempty"`
}

type RiskCluster struct {
	ID             string       `json:"id"`
	RuleID         string       `json:"ruleId"`
	Title          string       `json:"title"`
	Severity       RiskSeverity `json:"severity"`
	Category       string       `json:"category"`
	FileClass      string       `json:"fileClass"`
	Deterministic  bool         `json:"deterministic"`
	FindingCount   int          `json:"findingCount"`
	AffectedFiles  []string     `json:"affectedFiles"`
	Fingerprints   []string     `json:"fingerprints"`
	SampleFindings []Finding    `json:"sampleFindings"`
	Ignored        bool         `json:"ignored"`
	IgnoreReason   string       `json:"ignoreReason,omitempty"`
	SkillName      string       `json:"skillName,omitempty"`
	GroupID        string       `json:"groupId,omitempty"`
	GroupName      string       `json:"groupName,omitempty"`
}

type CodexClusterReview struct {
	ClusterID         string       `json:"clusterId"`
	Verdict           string       `json:"verdict"`
	EffectiveSeverity RiskSeverity `json:"effectiveSeverity"`
	Confidence        float64      `json:"confidence"`
	Rationale         string       `json:"rationale"`
	Recommendation    string       `json:"recommendation"`
}

type CodexConcern struct {
	Title          string       `json:"title"`
	Severity       RiskSeverity `json:"severity"`
	Confidence     float64      `json:"confidence"`
	EvidenceFiles  []string     `json:"evidenceFiles"`
	Rationale      string       `json:"rationale"`
	Recommendation string       `json:"recommendation"`
}

type CodexSkillReview struct {
	SkillName        string               `json:"skillName"`
	SourcePath       string               `json:"sourcePath"`
	Status           string               `json:"status"`
	Verdict          string               `json:"verdict"`
	Summary          string               `json:"summary"`
	Confidence       float64              `json:"confidence"`
	ContextFileCount int                  `json:"contextFileCount"`
	ClusterIDs       []string             `json:"clusterIds"`
	Concerns         []CodexConcern       `json:"concerns"`
	ClusterReviews   []CodexClusterReview `json:"clusterReviews"`
	Error            string               `json:"error,omitempty"`
}

type CodexReviewBatch struct {
	Index      int      `json:"index"`
	GroupID    string   `json:"groupId"`
	GroupName  string   `json:"groupName"`
	Status     string   `json:"status"`
	Attempts   int      `json:"attempts"`
	SkillNames []string `json:"skillNames"`
	// Counts are optional because older persisted Codex reviews only recorded
	// scheduling state.  When present they make a batch result consumable
	// without reconstructing findings from the whole report.
	FindingCount       int          `json:"findingCount,omitempty"`
	ActiveFindingCount int          `json:"activeFindingCount,omitempty"`
	HighestSeverity    RiskSeverity `json:"highestSeverity,omitempty"`
	StartedAt          time.Time    `json:"startedAt,omitempty"`
	CompletedAt        time.Time    `json:"completedAt,omitempty"`
	Error              string       `json:"error,omitempty"`
}

type CodexReviewProgress struct {
	ReviewID            string             `json:"reviewId"`
	ReportID            string             `json:"reportId"`
	Sequence            uint64             `json:"sequence"`
	Phase               string             `json:"phase"`
	Message             string             `json:"message"`
	BatchCount          int                `json:"batchCount"`
	CompletedBatch      int                `json:"completedBatch"`
	TotalSkills         int                `json:"totalSkills"`
	CompletedSkills     int                `json:"completedSkills"`
	ActiveSkills        []string           `json:"activeSkills"`
	ActiveBatches       []CodexActiveBatch `json:"activeBatches"`
	ActivityCount       int                `json:"activityCount"`
	ContextChunkIndex   int                `json:"contextChunkIndex,omitempty"`
	ContextChunkCount   int                `json:"contextChunkCount,omitempty"`
	ContextChunkAttempt int                `json:"contextChunkAttempt,omitempty"`
	ContextChunkFiles   int                `json:"contextChunkFiles,omitempty"`
	StartedAt           time.Time          `json:"startedAt"`
	UpdatedAt           time.Time          `json:"updatedAt"`
}

type CodexActiveBatch struct {
	Index      int      `json:"index"`
	GroupID    string   `json:"groupId"`
	GroupName  string   `json:"groupName"`
	SkillNames []string `json:"skillNames"`
}

type CodexReviewResult struct {
	ID               string               `json:"id"`
	Status           string               `json:"status"`
	Summary          string               `json:"summary"`
	OverallVerdict   string               `json:"overallVerdict"`
	Model            string               `json:"model"`
	ReasoningEffort  string               `json:"reasoningEffort"`
	ContextMode      string               `json:"contextMode,omitempty"`
	ContextFileCount int                  `json:"contextFileCount,omitempty"`
	StartedAt        time.Time            `json:"startedAt"`
	CompletedAt      time.Time            `json:"completedAt,omitempty"`
	Reviews          []CodexClusterReview `json:"reviews"`
	SkillReviews     []CodexSkillReview   `json:"skillReviews"`
	Batches          []CodexReviewBatch   `json:"batches"`
	TotalSkills      int                  `json:"totalSkills"`
	DurationMillis   int64                `json:"durationMillis"`
	Error            string               `json:"error,omitempty"`
}

type ScanReport struct {
	ID                    string             `json:"id"`
	Target                string             `json:"target"`
	RootID                string             `json:"rootId,omitempty"`
	StartedAt             time.Time          `json:"startedAt"`
	CompletedAt           time.Time          `json:"completedAt"`
	HighestSeverity       RiskSeverity       `json:"highestSeverity"`
	ActiveHighestSeverity RiskSeverity       `json:"activeHighestSeverity"`
	Findings              []Finding          `json:"findings"`
	FilesScanned          int                `json:"filesScanned"`
	FilesSkipped          int                `json:"filesSkipped,omitempty"`
	ActiveFindingCount    int                `json:"activeFindingCount"`
	IgnoredFindingCount   int                `json:"ignoredFindingCount"`
	Status                string             `json:"status"`
	Error                 string             `json:"error,omitempty"`
	ScannerVersion        string             `json:"scannerVersion"`
	RuleCounts            map[string]int     `json:"ruleCounts"`
	CategoryCounts        map[string]int     `json:"categoryCounts"`
	Clusters              []RiskCluster      `json:"clusters"`
	CodexReview           *CodexReviewResult `json:"codexReview,omitempty"`
	Skills                []ScanSkillSummary `json:"skills"`
}

type ScanSkillSummary struct {
	SkillName           string       `json:"skillName"`
	RootID              string       `json:"rootId,omitempty"`
	SourcePath          string       `json:"sourcePath"`
	GroupID             string       `json:"groupId"`
	GroupName           string       `json:"groupName"`
	FilesScanned        int          `json:"filesScanned"`
	FindingCount        int          `json:"findingCount,omitempty"`
	Error               string       `json:"error,omitempty"`
	HighestSeverity     RiskSeverity `json:"highestSeverity"`
	ActiveFindingCount  int          `json:"activeFindingCount"`
	IgnoredFindingCount int          `json:"ignoredFindingCount"`
}

type SkillSecurityState struct {
	SkillName   string    `json:"skillName"`
	RootID      string    `json:"rootId,omitempty"`
	ContentHash string    `json:"contentHash"`
	ReportID    string    `json:"reportId"`
	CheckedAt   time.Time `json:"checkedAt"`
}

type PackageLock struct {
	RootID            string `json:"rootId,omitempty"`
	Provider          string `json:"provider"`
	SourceAssociation string `json:"sourceAssociation,omitempty"`
	Repository        string `json:"repository,omitempty"`
	GroupName         string `json:"groupName,omitempty"`
	SourceURL         string `json:"sourceUrl,omitempty"`
	RequestedRef      string `json:"requestedRef,omitempty"`
	// ResolvedRef is the immutable ref returned by a provider.  ResolvedCommit
	// remains the canonical v1/v2 field and is preferred when both exist.
	ResolvedRef    string               `json:"resolvedRef,omitempty"`
	ResolvedCommit string               `json:"resolvedCommit,omitempty"`
	InstalledAt    time.Time            `json:"installedAt"`
	UpdatedAt      time.Time            `json:"updatedAt"`
	Skills         map[string]SkillLock `json:"skills"`
}

type SkillLock struct {
	RootID            string            `json:"rootId,omitempty"`
	SourceAssociation string            `json:"sourceAssociation,omitempty"`
	SourcePath        string            `json:"sourcePath"`
	LocalPath         string            `json:"localPath"`
	ResolvedCommit    string            `json:"resolvedCommit,omitempty"`
	ResolvedRef       string            `json:"resolvedRef,omitempty"`
	TreeHash          string            `json:"treeHash,omitempty"`
	Files             map[string]string `json:"files"`
	Pinned            bool              `json:"pinned"`
	LastScanReport    string            `json:"lastScanReport,omitempty"`
}

type SourcesLock struct {
	SchemaVersion int                    `json:"schemaVersion"`
	Packages      map[string]PackageLock `json:"packages"`
}

// QualifiedSkillIdentity is the canonical key used by inventory, state, and
// manager callers.  It intentionally keeps the separator out of ordinary
// Skill names by using a NUL byte, while the human-readable form is exposed by
// SkillIdentity.
func QualifiedSkillIdentity(rootID, name string) string {
	return rootID + "\x00" + name
}

func SkillIdentity(rootID, name string) string {
	if rootID == "" {
		return name
	}
	return rootID + ":" + name
}

// QualifiedPackageID namespaces source-lock package IDs by root.  Existing
// v1 package IDs are migrated to this form without touching installed files.
func QualifiedPackageID(rootID, packageID string) string {
	if rootID == "" {
		return packageID
	}
	return rootID + "\x00" + packageID
}

func DefaultSkillRoots(codexPath, agentsPath string) []SkillRoot {
	return []SkillRoot{
		{ID: RootIDCodexDefault, Name: "Codex Skills", Kind: "codex", Path: codexPath, Enabled: true, SystemDir: ".system"},
		{ID: RootIDAgents, Name: "Agents Skills", Kind: "agents", Path: agentsPath, Enabled: true, SystemDir: ".system"},
	}
}

func IsSystemSkillName(name string) bool {
	return len(name) > 0 && strings.EqualFold(name, ".system")
}

func RootSystemDir(root SkillRoot) string {
	if strings.TrimSpace(root.SystemDir) == "" {
		return ".system"
	}
	return root.SystemDir
}

func IsSystemSkillPath(root SkillRoot, target string) bool {
	base := filepath.Clean(root.Path)
	candidate := filepath.Clean(target)
	relative, err := filepath.Rel(base, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return false
	}
	first := relative
	if index := strings.IndexRune(relative, filepath.Separator); index >= 0 {
		first = relative[:index]
	}
	return strings.EqualFold(first, RootSystemDir(root))
}

type Repository struct {
	Provider          string    `json:"provider"`
	SourceAssociation string    `json:"sourceAssociation,omitempty"`
	Owner             string    `json:"owner"`
	Name              string    `json:"name"`
	FullName          string    `json:"fullName"`
	Private           bool      `json:"private"`
	DefaultBranch     string    `json:"defaultBranch"`
	License           string    `json:"license,omitempty"`
	UpdatedAt         time.Time `json:"updatedAt"`
	ResolvedRef       string    `json:"resolvedRef"`
	CommitSHA         string    `json:"commitSha"`
	SourcePath        string    `json:"sourcePath,omitempty"`
	LocalPath         string    `json:"localPath,omitempty"`
}

type CandidateSkill struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	SourcePath  string       `json:"sourcePath"`
	Files       []FileRecord `json:"files"`
}

type InstallPreview struct {
	ID           string `json:"id"`
	TargetRootID string `json:"targetRootId,omitempty"`
	// SourceGroupID and SourceGroupName identify the update/security authority
	// represented by this preview.  They are additive fields: older previews
	// remain valid and callers may derive the values from Repository.
	SourceGroupID   string           `json:"sourceGroupId,omitempty"`
	SourceGroupName string           `json:"sourceGroupName,omitempty"`
	Repository      Repository       `json:"repository"`
	Skills          []CandidateSkill `json:"skills"`
	Scan            ScanReport       `json:"scan"`
	StagingPath     string           `json:"stagingPath"`
	PreviewDigest   string           `json:"previewDigest"`
	CreatedAt       time.Time        `json:"createdAt"`
	ExpiresAt       time.Time        `json:"expiresAt"`
}

// LocalizedText keeps reusable group records consumable by both the English
// and Chinese clients without forcing a locale into persisted state.  Empty
// values are allowed so a provider can supply only one language; readers
// should fall back to the populated value.
type LocalizedText struct {
	En string `json:"en,omitempty"`
	Zh string `json:"zh,omitempty"`
}

func (t LocalizedText) Empty() bool {
	return strings.TrimSpace(t.En) == "" && strings.TrimSpace(t.Zh) == ""
}

// Text returns the requested locale with a deterministic fallback.  Locale
// matching is intentionally prefix based so values such as zh-CN and en-US
// work with the compact persisted representation.
func (t LocalizedText) Text(locale string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "zh") {
		if strings.TrimSpace(t.Zh) != "" {
			return t.Zh
		}
		return t.En
	}
	if strings.TrimSpace(t.En) != "" {
		return t.En
	}
	return t.Zh
}

// SourceTrustPolicy is repository-wide policy keyed by canonical GitHub
// owner/repository (for example, "openai/codex-skills").  Trust is an
// explicit human policy signal and never replaces local hash, path, scanner,
// or immutable-ref checks.
type SourceTrustPolicy struct {
	Repository string     `json:"repository"`
	Provider   string     `json:"provider"`
	Trusted    bool       `json:"trusted"`
	Reason     string     `json:"reason,omitempty"`
	SetAt      time.Time  `json:"setAt,omitempty"`
	UpdatedAt  time.Time  `json:"updatedAt"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
}

// SourceTrustAudit is append-only evidence for every trust set/revoke action.
// TransactionID links the policy decision to the normal journal when a
// manager mutation is used, while Actor remains optional for compatibility
// with older callers that do not identify a principal.
type SourceTrustAudit struct {
	ID            int64     `json:"id,omitempty"`
	Repository    string    `json:"repository"`
	Action        string    `json:"action"`
	Trusted       bool      `json:"trusted"`
	Reason        string    `json:"reason,omitempty"`
	TransactionID string    `json:"transactionId,omitempty"`
	Actor         string    `json:"actor,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
}

// GroupSkillSecurity is the stable per-Skill projection embedded in a group
// security record.  Findings and clusters remain available through the
// existing ScanReport fields; this summary lets consumers render a group
// without reconstructing ownership from paths.
type GroupSkillSecurity struct {
	SkillName          string       `json:"skillName"`
	RootID             string       `json:"rootId,omitempty"`
	Status             string       `json:"status"`
	HighestSeverity    RiskSeverity `json:"highestSeverity"`
	ActiveFindingCount int          `json:"activeFindingCount"`
	FindingCount       int          `json:"findingCount"`
	ReportID           string       `json:"reportId,omitempty"`
	Error              string       `json:"error,omitempty"`
}

// GroupSecurityReport is a reusable, source-group authoritative security
// result.  It is deliberately compatible with ScanReport rather than
// replacing it: ScanReport remains the detailed scanner contract and this
// record captures the group-level status and bilingual summary.
type GroupSecurityReport struct {
	ID                    string               `json:"id"`
	RootID                string               `json:"rootId,omitempty"`
	GroupID               string               `json:"groupId"`
	GroupName             string               `json:"groupName"`
	Provider              string               `json:"provider,omitempty"`
	Repository            string               `json:"repository,omitempty"`
	CommitSHA             string               `json:"commitSha,omitempty"`
	Status                string               `json:"status"`
	HighestSeverity       RiskSeverity         `json:"highestSeverity"`
	ActiveHighestSeverity RiskSeverity         `json:"activeHighestSeverity"`
	Summary               LocalizedText        `json:"summary"`
	Skills                []GroupSkillSecurity `json:"skills"`
	Findings              []Finding            `json:"findings"`
	Clusters              []RiskCluster        `json:"clusters"`
	ScanReportID          string               `json:"scanReportId,omitempty"`
	CreatedAt             time.Time            `json:"createdAt"`
	CompletedAt           time.Time            `json:"completedAt,omitempty"`
	Error                 string               `json:"error,omitempty"`
}

// SourceAnalysis is the persisted read-only analysis envelope for one source
// group.  PlanID is optional because analysis may be generated before an
// installation/update plan exists.
type SourceAnalysis struct {
	ID             string              `json:"id"`
	RootID         string              `json:"rootId,omitempty"`
	GroupID        string              `json:"groupId"`
	GroupName      string              `json:"groupName"`
	Provider       string              `json:"provider,omitempty"`
	Repository     string              `json:"repository,omitempty"`
	CommitSHA      string              `json:"commitSha,omitempty"`
	Status         string              `json:"status"`
	Summary        LocalizedText       `json:"summary"`
	Skills         []string            `json:"skills"`
	Security       GroupSecurityReport `json:"security"`
	ScanReportID   string              `json:"scanReportId,omitempty"`
	PlanID         string              `json:"planId,omitempty"`
	ContextDigest  string              `json:"contextDigest,omitempty"`
	AnalysisDigest string              `json:"analysisDigest,omitempty"`
	CreatedAt      time.Time           `json:"createdAt"`
	CompletedAt    time.Time           `json:"completedAt,omitempty"`
	ExpiresAt      time.Time           `json:"expiresAt,omitempty"`
	Error          string              `json:"error,omitempty"`
}

// GroupOperationStep is internal per-Skill diagnostics/recovery metadata.
// The parent GroupOperation/Transaction remains the authoritative status.
type GroupOperationStep struct {
	ID             string    `json:"id"`
	SkillName      string    `json:"skillName"`
	Status         string    `json:"status"`
	TransactionID  string    `json:"transactionId,omitempty"`
	Error          string    `json:"error,omitempty"`
	RecoveryStatus string    `json:"recoveryStatus,omitempty"`
	BackupPaths    []string  `json:"backupPaths,omitempty"`
	StartedAt      time.Time `json:"startedAt,omitempty"`
	CompletedAt    time.Time `json:"completedAt,omitempty"`
}

// GroupOperation is a reusable plan/result envelope for source-group
// install/update/security actions.  TargetSkills and ValidSkills are both
// persisted so applying a group operation can prove that no partial target
// set was submitted.
type GroupOperation struct {
	ID                  string               `json:"id"`
	ParentTransactionID string               `json:"parentTransactionId,omitempty"`
	RootID              string               `json:"rootId,omitempty"`
	GroupID             string               `json:"groupId"`
	GroupName           string               `json:"groupName"`
	Kind                string               `json:"kind"`
	Status              string               `json:"status"`
	TargetSkills        []string             `json:"targetSkills"`
	ValidSkills         []string             `json:"validSkills"`
	Steps               []GroupOperationStep `json:"steps"`
	AnalysisID          string               `json:"analysisId,omitempty"`
	SecurityReportID    string               `json:"securityReportId,omitempty"`
	PlanID              string               `json:"planId,omitempty"`
	Error               string               `json:"error,omitempty"`
	StartedAt           time.Time            `json:"startedAt"`
	CompletedAt         time.Time            `json:"completedAt,omitempty"`
	RecoveryStatus      string               `json:"recoveryStatus,omitempty"`
}

// CodexProjectSecurity is the read-only security portion of a project scan.
// It is advisory evidence only; local scanner findings and the manager's
// approval gates remain authoritative for installation.
type CodexProjectSecurity struct {
	Verdict           string         `json:"verdict"`
	Summary           string         `json:"summary"`
	Confidence        float64        `json:"confidence"`
	LocalHighestRisk  RiskSeverity   `json:"localHighestRisk"`
	LocalFindingCount int            `json:"localFindingCount"`
	Concerns          []CodexConcern `json:"concerns"`
}

type CodexProjectInstallMethod struct {
	Kind          string   `json:"kind"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Supported     bool     `json:"supported"`
	Required      bool     `json:"required"`
	EvidenceFiles []string `json:"evidenceFiles"`
}

// CodexProjectScanResult is a reusable, read-only project overview. The
// result records coverage so callers can distinguish a conclusion from a
// complete guarantee when files were summarized, redacted, or omitted.
type CodexProjectScanResult struct {
	ID                    string                      `json:"id"`
	SourcePlanID          string                      `json:"sourcePlanId"`
	Status                string                      `json:"status"`
	Repository            Repository                  `json:"repository"`
	Summary               string                      `json:"summary"`
	Security              CodexProjectSecurity        `json:"security"`
	InstallationMethods   []CodexProjectInstallMethod `json:"installationMethods"`
	ContextMode           string                      `json:"contextMode"`
	ContextFileCount      int                         `json:"contextFileCount"`
	SummaryFileCount      int                         `json:"summaryFileCount"`
	DeepAnalysisFileCount int                         `json:"deepAnalysisFileCount"`
	OmittedFileCount      int                         `json:"omittedFileCount"`
	RedactedFileCount     int                         `json:"redactedFileCount"`
	TruncatedFileCount    int                         `json:"truncatedFileCount"`
	FocusFiles            []string                    `json:"focusFiles"`
	ContextDigest         string                      `json:"contextDigest"`
	ScanDigest            string                      `json:"scanDigest"`
	Model                 string                      `json:"model"`
	ReasoningEffort       string                      `json:"reasoningEffort"`
	StartedAt             time.Time                   `json:"startedAt"`
	CompletedAt           time.Time                   `json:"completedAt,omitempty"`
	ExpiresAt             time.Time                   `json:"expiresAt"`
	Error                 string                      `json:"error,omitempty"`
}

type AssistedInstallRequirement struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Description string `json:"description"`
	VersionSpec string `json:"versionSpec,omitempty"`
	Status      string `json:"status,omitempty"`
	Required    bool   `json:"required"`
}

// AssistedPythonWheelLock is the immutable, approval-time identity of one
// Wheel in a managed tool's complete dependency closure. Native Wheels require
// a separate high-risk permission.
type AssistedPythonWheelLock struct {
	Name     string   `json:"name"`
	Version  string   `json:"version"`
	Filename string   `json:"filename"`
	SHA256   string   `json:"sha256"`
	Native   bool     `json:"native,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

type AssistedInstallStep struct {
	ID                 string                    `json:"id"`
	Kind               string                    `json:"kind"`
	Title              string                    `json:"title"`
	Description        string                    `json:"description"`
	Status             string                    `json:"status"`
	Required           bool                      `json:"required"`
	Supported          bool                      `json:"supported"`
	SkillNames         []string                  `json:"skillNames,omitempty"`
	PythonPackage      string                    `json:"pythonPackage,omitempty"`
	VersionSpec        string                    `json:"versionSpec,omitempty"`
	PythonWheels       []AssistedPythonWheelLock `json:"pythonWheels,omitempty"`
	Entrypoint         string                    `json:"entrypoint,omitempty"`
	MCPServerName      string                    `json:"mcpServerName,omitempty"`
	MCPArgs            []string                  `json:"mcpArgs,omitempty"`
	PermissionIDs      []string                  `json:"permissionIds,omitempty"`
	Reversible         bool                      `json:"reversible"`
	Recovery           string                    `json:"recovery,omitempty"`
	TargetPath         string                    `json:"targetPath,omitempty"`
	BackupPath         string                    `json:"backupPath,omitempty"`
	ManifestPath       string                    `json:"manifestPath,omitempty"`
	ChildTransactionID string                    `json:"childTransactionId,omitempty"`
	OutputHashes       map[string]string         `json:"outputHashes,omitempty"`
	OriginalMissing    bool                      `json:"originalMissing,omitempty"`
	AppliedHash        string                    `json:"appliedHash,omitempty"`
	Error              string                    `json:"error,omitempty"`
	StartedAt          *time.Time                `json:"startedAt,omitempty"`
	CompletedAt        *time.Time                `json:"completedAt,omitempty"`
}

type AssistedInstallPermission struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Risk        string   `json:"risk"`
	Required    bool     `json:"required"`
	Approved    bool     `json:"approved"`
	Targets     []string `json:"targets,omitempty"`
}

type AssistedInstallPlan struct {
	ID                string                       `json:"id"`
	SourcePlanID      string                       `json:"sourcePlanId"`
	TargetRootID      string                       `json:"targetRootId"`
	ProjectScanID     string                       `json:"projectScanId,omitempty"`
	ProjectScanDigest string                       `json:"projectScanDigest,omitempty"`
	Status            string                       `json:"status"`
	TransactionID     string                       `json:"transactionId,omitempty"`
	RecoveryStatus    string                       `json:"recoveryStatus,omitempty"`
	Repository        Repository                   `json:"repository"`
	Summary           string                       `json:"summary"`
	Approach          string                       `json:"approach"`
	Complexity        string                       `json:"complexity"`
	Requirements      []AssistedInstallRequirement `json:"requirements"`
	Steps             []AssistedInstallStep        `json:"steps"`
	Permissions       []AssistedInstallPermission  `json:"permissions"`
	Warnings          []string                     `json:"warnings"`
	Skills            []CandidateSkill             `json:"skills,omitempty"`
	SelectedSkills    []string                     `json:"selectedSkills,omitempty"`
	Scan              ScanReport                   `json:"scan"`
	NeedsProjectRoot  bool                         `json:"needsProjectRoot"`
	ProjectRootReason string                       `json:"projectRootReason,omitempty"`
	ProjectRoot       string                       `json:"projectRoot,omitempty"`
	CodexModel        string                       `json:"codexModel"`
	ReasoningEffort   string                       `json:"reasoningEffort"`
	OutputLocale      string                       `json:"outputLocale,omitempty"`
	ContextFileCount  int                          `json:"contextFileCount"`
	ContextDigest     string                       `json:"contextDigest"`
	PlanDigest        string                       `json:"planDigest"`
	ConfigFingerprint string                       `json:"configFingerprint,omitempty"`
	CreatedAt         time.Time                    `json:"createdAt"`
	ExpiresAt         time.Time                    `json:"expiresAt"`
}

type AssistedInstallProgressStep struct {
	ID          string     `json:"id"`
	Kind        string     `json:"kind"`
	Title       string     `json:"title"`
	Status      string     `json:"status"`
	Message     string     `json:"message,omitempty"`
	Error       string     `json:"error,omitempty"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

type AssistedInstallProgress struct {
	ReferenceID    string                        `json:"referenceId"`
	RunID          string                        `json:"runId"`
	Sequence       uint64                        `json:"sequence"`
	Phase          string                        `json:"phase"`
	Message        string                        `json:"message"`
	CurrentStepID  string                        `json:"currentStepId,omitempty"`
	CompletedSteps int                           `json:"completedSteps"`
	TotalSteps     int                           `json:"totalSteps"`
	ActivityCount  int                           `json:"activityCount"`
	Steps          []AssistedInstallProgressStep `json:"steps"`
	StartedAt      time.Time                     `json:"startedAt"`
	UpdatedAt      time.Time                     `json:"updatedAt"`
	CompletedAt    *time.Time                    `json:"completedAt,omitempty"`
	Terminal       bool                          `json:"terminal"`
	Error          string                        `json:"error,omitempty"`
}

type AssistedInstallResult struct {
	ReferenceID string                   `json:"referenceId,omitempty"`
	RunID       string                   `json:"runId,omitempty"`
	Plan        AssistedInstallPlan      `json:"plan"`
	Transaction Transaction              `json:"transaction"`
	Progress    *AssistedInstallProgress `json:"progress,omitempty"`
}

type AdoptionPreview struct {
	ID            string           `json:"id"`
	TargetRootID  string           `json:"targetRootId,omitempty"`
	PreviewDigest string           `json:"previewDigest"`
	Skills        []Skill          `json:"skills"`
	Sources       []DetectedSource `json:"sources"`
	Scan          ScanReport       `json:"scan"`
	CreatedAt     time.Time        `json:"createdAt"`
	ExpiresAt     time.Time        `json:"expiresAt"`
}

type UpdateStatus struct {
	GroupID                 string            `json:"groupId"`
	RootID                  string            `json:"rootId,omitempty"`
	GroupName               string            `json:"groupName"`
	Provider                string            `json:"provider"`
	Repository              string            `json:"repository,omitempty"`
	Status                  string            `json:"status"`
	CurrentCommits          map[string]string `json:"currentCommits,omitempty"`
	RemoteCommit            string            `json:"remoteCommit,omitempty"`
	OutdatedSkills          []string          `json:"outdatedSkills"`
	CheckedAt               time.Time         `json:"checkedAt"`
	Error                   string            `json:"error,omitempty"`
	LastSuccessStatus       string            `json:"lastSuccessStatus,omitempty"`
	LastSuccessAt           *time.Time        `json:"lastSuccessAt,omitempty"`
	LastSuccessRemoteCommit string            `json:"lastSuccessRemoteCommit,omitempty"`
	RetryAt                 *time.Time        `json:"retryAt,omitempty"`
	RateLimitRemaining      int               `json:"rateLimitRemaining,omitempty"`
	RateLimitLimit          int               `json:"rateLimitLimit,omitempty"`
	FromCache               bool              `json:"fromCache"`
}

type UpdateCheckResult struct {
	CheckedAt time.Time      `json:"checkedAt"`
	Statuses  []UpdateStatus `json:"statuses"`
}

type GitHubCredentialStatus struct {
	Configured    bool       `json:"configured"`
	Authenticated bool       `json:"authenticated"`
	Source        string     `json:"source"`
	Login         string     `json:"login,omitempty"`
	Limit         int        `json:"limit,omitempty"`
	Remaining     int        `json:"remaining,omitempty"`
	ResetAt       *time.Time `json:"resetAt,omitempty"`
	Message       string     `json:"message"`
}

type CodexCLIStatus struct {
	Available           bool               `json:"available"`
	Authenticated       bool               `json:"authenticated"`
	Compatible          bool               `json:"compatible"`
	Path                string             `json:"path,omitempty"`
	Version             string             `json:"version,omitempty"`
	AuthStatus          string             `json:"authStatus,omitempty"`
	MissingCapabilities []string           `json:"missingCapabilities,omitempty"`
	Models              []CodexModelOption `json:"models,omitempty"`
	ModelCatalogError   string             `json:"modelCatalogError,omitempty"`
	CheckedAt           time.Time          `json:"checkedAt"`
	Error               string             `json:"error,omitempty"`
}

type CodexModelOption struct {
	Slug                  string                 `json:"slug"`
	DisplayName           string                 `json:"displayName"`
	Description           string                 `json:"description,omitempty"`
	DefaultReasoningLevel string                 `json:"defaultReasoningLevel,omitempty"`
	ReasoningLevels       []CodexReasoningOption `json:"reasoningLevels,omitempty"`
}

type CodexReasoningOption struct {
	Effort      string `json:"effort"`
	Description string `json:"description,omitempty"`
}

type Transaction struct {
	ID             string                `json:"id"`
	RootID         string                `json:"rootId,omitempty"`
	Type           string                `json:"type"`
	Status         string                `json:"status"`
	Targets        []string              `json:"targets"`
	GroupID        string                `json:"groupId,omitempty"`
	GroupName      string                `json:"groupName,omitempty"`
	OperationID    string                `json:"operationId,omitempty"`
	StartedAt      time.Time             `json:"startedAt"`
	CompletedAt    time.Time             `json:"completedAt,omitempty"`
	BackupPaths    []string              `json:"backupPaths,omitempty"`
	Steps          []AssistedInstallStep `json:"steps,omitempty"`
	ProjectRoot    string                `json:"projectRoot,omitempty"`
	RecoveryStatus string                `json:"recoveryStatus,omitempty"`
	Error          string                `json:"error,omitempty"`
	ParentID       string                `json:"parentId,omitempty"`
	ItemResults    []BatchItemResult     `json:"itemResults,omitempty"`
}

// BatchItemResult keeps best-effort batch operations useful even when one
// target fails. Each item points at its own journaled transaction so callers
// can retry or recover only the failed target.
type BatchItemResult struct {
	Target        string `json:"target"`
	Status        string `json:"status"`
	TransactionID string `json:"transactionId,omitempty"`
	Error         string `json:"error,omitempty"`
}

type Dashboard struct {
	Skills          []Skill        `json:"skills"`
	Roots           []SkillRoot    `json:"roots"`
	DefaultRootID   string         `json:"defaultRootId"`
	Groups          []Group        `json:"groups"`
	SourceGroups    []Group        `json:"sourceGroups"`
	Relations       []Relation     `json:"relations"`
	RecentReports   []ScanReport   `json:"recentReports"`
	RecentHistory   []Transaction  `json:"recentHistory"`
	UpdateStatuses  []UpdateStatus `json:"updateStatuses"`
	LastUpdateCheck *time.Time     `json:"lastUpdateCheck,omitempty"`
	ManagedCount    int            `json:"managedCount"`
	UnmanagedCount  int            `json:"unmanagedCount"`
	SystemCount     int            `json:"systemCount"`
	RiskCount       int            `json:"riskCount"`
	UpdateCount     int            `json:"updateCount"`
}
