package model

import "time"

const Version = "0.8.0"

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
	SchemaVersion int               `json:"schemaVersion" yaml:"schemaVersion"`
	Paths         Paths             `json:"paths" yaml:"paths"`
	Schedule      Schedule          `json:"schedule" yaml:"schedule"`
	Locale        string            `json:"locale" yaml:"locale"`
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
	Name             string       `json:"name"`
	Description      string       `json:"description"`
	Path             string       `json:"path"`
	GroupID          string       `json:"groupId"`
	GroupName        string       `json:"groupName"`
	SourceGroupID    string       `json:"sourceGroupId"`
	SourceGroupName  string       `json:"sourceGroupName"`
	SourceProvider   string       `json:"sourceProvider"`
	SourceConfidence float64      `json:"sourceConfidence"`
	SourceEvidence   string       `json:"sourceEvidence,omitempty"`
	Managed          bool         `json:"managed"`
	System           bool         `json:"system"`
	LocalModified    bool         `json:"localModified"`
	SecurityStatus   string       `json:"securityStatus"`
	UpdateStatus     string       `json:"updateStatus"`
	InstalledCommit  string       `json:"installedCommit,omitempty"`
	SourceRepository string       `json:"sourceRepository,omitempty"`
	SourcePath       string       `json:"sourcePath,omitempty"`
	Files            []FileRecord `json:"files,omitempty"`
	Dependencies     []string     `json:"dependencies,omitempty"`
	Relationships    []Relation   `json:"relationships,omitempty"`
	LastChecked      *time.Time   `json:"lastChecked,omitempty"`
	LastSecurityScan *time.Time   `json:"lastSecurityScan,omitempty"`
	SecurityChanged  bool         `json:"securityChanged"`
}

type Group struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Provider   string   `json:"provider"`
	Repository string   `json:"repository,omitempty"`
	ReadOnly   bool     `json:"readOnly"`
	Manual     bool     `json:"manual"`
	Position   int      `json:"position"`
	SkillNames []string `json:"skillNames"`
	Status     string   `json:"status"`
}

type DetectedSource struct {
	SkillName    string  `json:"skillName"`
	Provider     string  `json:"provider"`
	Repository   string  `json:"repository"`
	SourceURL    string  `json:"sourceUrl"`
	RequestedRef string  `json:"requestedRef,omitempty"`
	SourcePath   string  `json:"sourcePath"`
	GroupID      string  `json:"groupId"`
	GroupName    string  `json:"groupName"`
	Confidence   float64 `json:"confidence"`
	Evidence     string  `json:"evidence"`
}

type GroupPreference struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Position int    `json:"position"`
	Manual   bool   `json:"manual"`
}

type SkillGroupAssignment struct {
	SkillName string `json:"skillName"`
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
	Index       int       `json:"index"`
	GroupID     string    `json:"groupId"`
	GroupName   string    `json:"groupName"`
	Status      string    `json:"status"`
	Attempts    int       `json:"attempts"`
	SkillNames  []string  `json:"skillNames"`
	StartedAt   time.Time `json:"startedAt,omitempty"`
	CompletedAt time.Time `json:"completedAt,omitempty"`
	Error       string    `json:"error,omitempty"`
}

type CodexReviewProgress struct {
	ReviewID        string             `json:"reviewId"`
	ReportID        string             `json:"reportId"`
	Sequence        uint64             `json:"sequence"`
	Phase           string             `json:"phase"`
	Message         string             `json:"message"`
	BatchCount      int                `json:"batchCount"`
	CompletedBatch  int                `json:"completedBatch"`
	TotalSkills     int                `json:"totalSkills"`
	CompletedSkills int                `json:"completedSkills"`
	ActiveSkills    []string           `json:"activeSkills"`
	ActiveBatches   []CodexActiveBatch `json:"activeBatches"`
	ActivityCount   int                `json:"activityCount"`
	StartedAt       time.Time          `json:"startedAt"`
	UpdatedAt       time.Time          `json:"updatedAt"`
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
	StartedAt             time.Time          `json:"startedAt"`
	CompletedAt           time.Time          `json:"completedAt"`
	HighestSeverity       RiskSeverity       `json:"highestSeverity"`
	ActiveHighestSeverity RiskSeverity       `json:"activeHighestSeverity"`
	Findings              []Finding          `json:"findings"`
	FilesScanned          int                `json:"filesScanned"`
	ActiveFindingCount    int                `json:"activeFindingCount"`
	IgnoredFindingCount   int                `json:"ignoredFindingCount"`
	Status                string             `json:"status"`
	ScannerVersion        string             `json:"scannerVersion"`
	Clusters              []RiskCluster      `json:"clusters"`
	CodexReview           *CodexReviewResult `json:"codexReview,omitempty"`
	Skills                []ScanSkillSummary `json:"skills"`
}

type ScanSkillSummary struct {
	SkillName           string       `json:"skillName"`
	SourcePath          string       `json:"sourcePath"`
	GroupID             string       `json:"groupId"`
	GroupName           string       `json:"groupName"`
	FilesScanned        int          `json:"filesScanned"`
	HighestSeverity     RiskSeverity `json:"highestSeverity"`
	ActiveFindingCount  int          `json:"activeFindingCount"`
	IgnoredFindingCount int          `json:"ignoredFindingCount"`
}

type SkillSecurityState struct {
	SkillName   string    `json:"skillName"`
	ContentHash string    `json:"contentHash"`
	ReportID    string    `json:"reportId"`
	CheckedAt   time.Time `json:"checkedAt"`
}

type PackageLock struct {
	Provider       string               `json:"provider"`
	Repository     string               `json:"repository,omitempty"`
	GroupName      string               `json:"groupName,omitempty"`
	SourceURL      string               `json:"sourceUrl,omitempty"`
	RequestedRef   string               `json:"requestedRef,omitempty"`
	ResolvedCommit string               `json:"resolvedCommit,omitempty"`
	InstalledAt    time.Time            `json:"installedAt"`
	UpdatedAt      time.Time            `json:"updatedAt"`
	Skills         map[string]SkillLock `json:"skills"`
}

type SkillLock struct {
	SourcePath     string            `json:"sourcePath"`
	LocalPath      string            `json:"localPath"`
	ResolvedCommit string            `json:"resolvedCommit,omitempty"`
	TreeHash       string            `json:"treeHash,omitempty"`
	Files          map[string]string `json:"files"`
	Pinned         bool              `json:"pinned"`
	LastScanReport string            `json:"lastScanReport,omitempty"`
}

type SourcesLock struct {
	SchemaVersion int                    `json:"schemaVersion"`
	Packages      map[string]PackageLock `json:"packages"`
}

type Repository struct {
	Provider      string    `json:"provider"`
	Owner         string    `json:"owner"`
	Name          string    `json:"name"`
	FullName      string    `json:"fullName"`
	Private       bool      `json:"private"`
	DefaultBranch string    `json:"defaultBranch"`
	License       string    `json:"license,omitempty"`
	UpdatedAt     time.Time `json:"updatedAt"`
	ResolvedRef   string    `json:"resolvedRef"`
	CommitSHA     string    `json:"commitSha"`
	SourcePath    string    `json:"sourcePath,omitempty"`
	LocalPath     string    `json:"localPath,omitempty"`
}

type CandidateSkill struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	SourcePath  string       `json:"sourcePath"`
	Files       []FileRecord `json:"files"`
}

type InstallPreview struct {
	ID          string           `json:"id"`
	Repository  Repository       `json:"repository"`
	Skills      []CandidateSkill `json:"skills"`
	Scan        ScanReport       `json:"scan"`
	StagingPath string           `json:"stagingPath"`
	CreatedAt   time.Time        `json:"createdAt"`
	ExpiresAt   time.Time        `json:"expiresAt"`
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
	ID        string           `json:"id"`
	Skills    []Skill          `json:"skills"`
	Sources   []DetectedSource `json:"sources"`
	Scan      ScanReport       `json:"scan"`
	CreatedAt time.Time        `json:"createdAt"`
	ExpiresAt time.Time        `json:"expiresAt"`
}

type UpdateStatus struct {
	GroupID                 string            `json:"groupId"`
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
	Type           string                `json:"type"`
	Status         string                `json:"status"`
	Targets        []string              `json:"targets"`
	StartedAt      time.Time             `json:"startedAt"`
	CompletedAt    time.Time             `json:"completedAt,omitempty"`
	BackupPaths    []string              `json:"backupPaths,omitempty"`
	Steps          []AssistedInstallStep `json:"steps,omitempty"`
	ProjectRoot    string                `json:"projectRoot,omitempty"`
	RecoveryStatus string                `json:"recoveryStatus,omitempty"`
	Error          string                `json:"error,omitempty"`
}

type Dashboard struct {
	Skills          []Skill        `json:"skills"`
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
