export type Severity = "informational" | "low" | "medium" | "high" | "critical";
export type Root = import("./roots").RootContract;

export interface Relation {
  from: string;
  to: string;
  type: string;
  confidence: number;
  evidence?: string;
}

export interface Skill {
  name: string;
  /** Root-qualified identity fields introduced by the v0.11 API contract. */
  rootId?: string;
  rootKind?: string;
  rootName?: string;
  identity?: string;
  description: string;
  path: string;
  groupId: string;
  groupName: string;
  sourceGroupId: string;
  sourceGroupName: string;
  sourceProvider: string;
  sourceConfidence: number;
  sourceEvidence?: string;
  managed: boolean;
  system: boolean;
  localModified: boolean;
  securityStatus: string;
  updateStatus: string;
  installedCommit?: string;
  sourceRepository?: string;
  sourcePath?: string;
  files?: Array<{ path: string; size: number; sha256: string; kind: string }>;
  lastSecurityScan?: string;
  securityChanged?: boolean;
}

export interface Group {
  id: string;
  rootId?: string;
  name: string;
  provider: string;
  repository?: string;
  readOnly: boolean;
  manual: boolean;
  position: number;
  skillNames: string[];
  status: string;
}

export interface Finding {
  ruleId: string;
  title: string;
  severity: Severity;
  confidence: number;
  file: string;
  line: number;
  evidence: string;
  explanation: string;
  recommendedAction: string;
  fingerprint: string;
  ignored: boolean;
  ignoreReason?: string;
  fileClass: "instruction" | "runtime" | "test" | "documentation" | "asset";
  category: string;
  clusterId: string;
  deterministic: boolean;
  skillName?: string;
  rootId?: string;
  groupId?: string;
  groupName?: string;
}

export interface RiskCluster {
  id: string;
  ruleId: string;
  title: string;
  severity: Severity;
  category: string;
  fileClass: string;
  deterministic: boolean;
  findingCount: number;
  affectedFiles: string[];
  fingerprints: string[];
  sampleFindings: Finding[];
  ignored: boolean;
  ignoreReason?: string;
  skillName?: string;
  rootId?: string;
  groupId?: string;
  groupName?: string;
}

export interface CodexClusterReview {
  clusterId: string;
  verdict: string;
  effectiveSeverity: Severity;
  confidence: number;
  rationale: string;
  recommendation: string;
}

export interface CodexConcern {
  title: string;
  severity: Severity;
  confidence: number;
  evidenceFiles: string[];
  rationale: string;
  recommendation: string;
}

export interface CodexSkillReview {
  skillName: string;
  sourcePath: string;
  status: "completed" | "failed";
  verdict: string;
  summary: string;
  confidence: number;
  contextFileCount: number;
  clusterIds: string[];
  concerns: CodexConcern[];
  clusterReviews: CodexClusterReview[];
  error?: string;
}

export interface CodexReviewBatch {
  index: number;
  groupId?: string;
  groupName?: string;
  status: "queued" | "running" | "completed" | "failed";
  attempts?: number;
  skillNames: string[];
  startedAt?: string;
  completedAt?: string;
  error?: string;
}

export interface CodexReviewProgress {
  reviewId: string;
  reportId: string;
  sequence: number;
  phase: "preparing" | "queued" | "reviewing" | "completed" | "partial" | "failed";
  message: string;
  batchCount: number;
  completedBatch: number;
  totalSkills: number;
  completedSkills: number;
  activeSkills: string[];
  activeBatches: Array<{ index: number; groupId?: string; groupName?: string; skillNames: string[] }>;
  activityCount: number;
  contextChunkIndex?: number;
  contextChunkCount?: number;
  contextChunkAttempt?: number;
  contextChunkFiles?: number;
  startedAt: string;
  updatedAt: string;
}

export interface CodexReviewResult {
  id: string;
  status: "running" | "completed" | "partial" | "failed";
  summary: string;
  overallVerdict: string;
  model: string;
  reasoningEffort: string;
  contextMode?: string;
  contextFileCount?: number;
  startedAt: string;
  completedAt?: string;
  reviews: CodexClusterReview[];
  skillReviews: CodexSkillReview[];
  batches: CodexReviewBatch[];
  totalSkills: number;
  durationMillis: number;
  error?: string;
}

export interface CodexCLIStatus {
  available: boolean;
  authenticated: boolean;
  compatible: boolean;
  path?: string;
  version?: string;
  authStatus?: string;
  missingCapabilities?: string[];
  models?: CodexModelOption[];
  modelCatalogError?: string;
  checkedAt: string;
  error?: string;
}

export interface CodexModelOption {
  slug: string;
  displayName: string;
  description?: string;
  defaultReasoningLevel?: string;
  reasoningLevels?: Array<{ effort: string; description?: string }>;
}

export interface ScanReport {
  id: string;
  target: string;
  rootId?: string;
  highestSeverity: Severity;
  activeHighestSeverity: Severity;
  findings: Finding[];
  filesScanned: number;
  activeFindingCount: number;
  ignoredFindingCount: number;
  status: string;
  completedAt: string;
  clusters: RiskCluster[];
  codexReview?: CodexReviewResult;
  skills?: ScanSkillSummary[];
}

export interface ScanSkillSummary {
  skillName: string;
  rootId?: string;
  sourcePath: string;
  groupId: string;
  groupName: string;
  filesScanned: number;
  findingCount?: number;
  error?: string;
  highestSeverity: Severity;
  activeFindingCount: number;
  ignoredFindingCount: number;
}

export interface Transaction {
  id: string;
  rootId?: string;
  type: string;
  status: string;
  targets: string[];
  startedAt: string;
  completedAt?: string;
  backupPaths?: string[];
  steps?: AssistedInstallStep[];
  projectRoot?: string;
  recoveryStatus?: string;
  error?: string;
  parentId?: string;
  itemResults?: Array<{
    target: string;
    status: string;
    transactionId?: string;
    error?: string;
  }>;
}

export interface UpdateStatus {
  groupId: string;
  rootId?: string;
  groupName: string;
  provider: string;
  repository?: string;
  status: "up-to-date" | "update-available" | "unsupported" | "error" | "rate-limited";
  currentCommits?: Record<string, string>;
  remoteCommit?: string;
  outdatedSkills: string[];
  checkedAt: string;
  error?: string;
  lastSuccessStatus?: "up-to-date" | "update-available";
  lastSuccessAt?: string;
  lastSuccessRemoteCommit?: string;
  retryAt?: string;
  rateLimitRemaining?: number;
  rateLimitLimit?: number;
  fromCache: boolean;
}

export interface UpdateCheckResult {
  checkedAt: string;
  statuses: UpdateStatus[];
}

export interface Dashboard {
  skills: Skill[];
  groups: Group[];
  sourceGroups: Group[];
  relations: Relation[];
  recentReports: ScanReport[];
  recentHistory: Transaction[];
  updateStatuses: UpdateStatus[];
  lastUpdateCheck?: string;
  managedCount: number;
  unmanagedCount: number;
  systemCount: number;
  riskCount: number;
  updateCount: number;
  /** Optional while older desktop builds are still in the field. */
  roots?: import("./roots").RootContract[];
  defaultRootId?: string;
}

export interface Candidate {
  name: string;
  description: string;
  sourcePath: string;
}

export interface InstallPreview {
  id: string;
  targetRootId?: string;
  repository: {
    provider: string;
    fullName: string;
    private: boolean;
    defaultBranch: string;
    license?: string;
    resolvedRef: string;
    commitSha: string;
  };
  skills: Candidate[];
  scan: ScanReport;
  previewDigest?: string;
  createdAt: string;
  expiresAt: string;
}

export type AssessmentGate = "ready" | "attention" | "blocked" | "incomplete";
export type AssessmentRequirement = "required" | "triggered" | "optional";
export type AssessmentCheckStatus = "passed" | "attention" | "blocked" | "pending" | "not-applicable";

export interface LayeredSecurityCheck {
  id: string;
  layer: string;
  requirement: AssessmentRequirement;
  status: AssessmentCheckStatus;
  title: string;
  summary: string;
  reason?: string;
  provider: string;
  evidenceFiles: string[];
}

export interface InstallTargetPreview {
  kind: "codex-skill" | string;
  rootId?: string;
  rootKind?: string;
  rootName?: string;
  displayName: string;
  path: string;
  supported: boolean;
  reason?: string;
  permissionIds: string[];
  reversible: boolean;
}

export interface ProjectAssessment {
  id: string;
  sourcePlanId: string;
  repository: InstallPreview["repository"];
  classification: "skill" | "plugin" | "application" | "library" | "mixed" | "unknown" | string;
  classificationEvidence: string[];
  checks: LayeredSecurityCheck[];
  gate: AssessmentGate;
  summary: string;
  highestRisk: Severity;
  coverage: {
    filesInventoried: number;
    filesScanned: number;
    evidenceLimited: boolean;
  };
  targets: InstallTargetPreview[];
  enhancedScanRecommended: boolean;
  enhancedScanReason?: string;
  sourceDigest: string;
  assessmentDigest: string;
  createdAt: string;
  expiresAt: string;
}

export interface CodexProjectSecurity {
  verdict: string;
  summary: string;
  confidence: number;
  localHighestRisk: Severity;
  localFindingCount: number;
  concerns: CodexConcern[];
}

export interface CodexProjectInstallMethod {
  kind: string;
  title: string;
  description: string;
  supported: boolean;
  required: boolean;
  evidenceFiles: string[];
}

export interface CodexProjectScanResult {
  id: string;
  sourcePlanId: string;
  status: string;
  repository: InstallPreview["repository"];
  summary: string;
  security: CodexProjectSecurity;
  installationMethods: CodexProjectInstallMethod[];
  contextMode: string;
  contextFileCount: number;
  summaryFileCount: number;
  deepAnalysisFileCount: number;
  omittedFileCount: number;
  redactedFileCount: number;
  truncatedFileCount: number;
  focusFiles: string[];
  contextDigest: string;
  scanDigest: string;
  model?: string;
  reasoningEffort?: string;
  startedAt: string;
  completedAt?: string;
  expiresAt: string;
  error?: string;
}

export type AssistedInstallStatus =
  | "analyzing"
  | "ready"
  | "awaiting-approval"
  | "running"
  | "completed"
  | "partial"
  | "failed"
  | "cancelled"
  | "interrupted";

export interface AssistedInstallRequirement {
  id?: string;
  kind?: string;
  name?: string;
  title?: string;
  description?: string;
  versionSpec?: string;
  status?: string;
  required?: boolean;
  satisfied?: boolean;
}

export interface AssistedInstallPermission {
  id: string;
  kind: string;
  title: string;
  description?: string;
  target?: string;
  targets?: string[];
  risk?: Severity | "standard";
  required: boolean;
  approved?: boolean;
  reversible?: boolean;
}

export interface AssistedPythonWheelLock {
  name: string;
  version: string;
  filename: string;
  sha256: string;
  native?: boolean;
  tags?: string[];
}

export interface AssistedInstallStep {
  id: string;
  kind: string;
  title: string;
  description: string;
  status: string;
  required: boolean;
  supported: boolean;
  skillNames?: string[];
  pythonPackage?: string;
  versionSpec?: string;
  pythonWheels?: AssistedPythonWheelLock[];
  entrypoint?: string;
  mcpServerName?: string;
  mcpArgs?: string[];
  permissionIds?: string[];
  reversible: boolean;
  recovery?: string;
  targetPath?: string;
  backupPath?: string;
  manifestPath?: string;
  childTransactionId?: string;
  outputHashes?: Record<string, string>;
  originalMissing?: boolean;
  appliedHash?: string;
  startedAt?: string;
  completedAt?: string;
  error?: string;
}

export interface AssistedInstallPlan {
  id: string;
  targetRootId?: string;
  sourcePlanId: string;
  projectScanId?: string;
  projectScanDigest?: string;
  status: AssistedInstallStatus | string;
  repository: InstallPreview["repository"];
  summary: string;
  approach: string;
  complexity: string;
  requirements: Array<string | AssistedInstallRequirement>;
  steps: AssistedInstallStep[];
  permissions: AssistedInstallPermission[];
  warnings: string[];
  needsProjectRoot: boolean;
  projectRootReason?: string;
  projectRoot?: string;
  codexModel?: string;
  reasoningEffort?: string;
  outputLocale?: string;
  contextFileCount: number;
  contextDigest?: string;
  planDigest?: string;
  configFingerprint?: string;
  createdAt: string;
  expiresAt: string;
  transactionId?: string;
  recoveryStatus?: string;
  selectedSkills?: string[];
  skills?: Candidate[];
  scan?: ScanReport;
}

export interface AssistedInstallProgressStep {
  id: string;
  title?: string;
  kind?: string;
  status: "queued" | "running" | "completed" | "failed" | "skipped" | "cancelled" | string;
  message?: string;
  startedAt?: string;
  completedAt?: string;
  error?: string;
}

export interface AssistedInstallProgress {
  referenceId: string;
  runId: string;
  sequence: number;
  phase: string;
  message: string;
  currentStepId?: string;
  completedSteps: number;
  totalSteps: number;
  activityCount: number;
  steps: AssistedInstallProgressStep[];
  startedAt: string;
  updatedAt: string;
  terminal: boolean;
  error?: string;
}

export interface AssistedInstallResult {
  plan: AssistedInstallPlan;
  transaction: Transaction;
  referenceId?: string;
  runId?: string;
  progress?: AssistedInstallProgress;
}

export interface AdoptionPreview {
  id: string;
  targetRootId?: string;
  skills: Skill[];
  sources: Array<{
    skillName: string;
    rootId?: string;
    provider: string;
    repository?: string;
    sourceUrl?: string;
    requestedRef?: string;
    sourcePath: string;
    groupId: string;
    groupName: string;
    confidence: number;
    evidence: string;
  }>;
  scan: ScanReport;
  createdAt: string;
  expiresAt: string;
}
