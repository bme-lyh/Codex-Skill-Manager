export type Severity = "informational" | "low" | "medium" | "high" | "critical";

export interface Relation {
  from: string;
  to: string;
  type: string;
  confidence: number;
  evidence?: string;
}

export interface Skill {
  name: string;
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
}

export interface Group {
  id: string;
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
}

export interface CodexClusterReview {
  clusterId: string;
  verdict: string;
  effectiveSeverity: Severity;
  confidence: number;
  rationale: string;
  recommendation: string;
}

export interface CodexReviewResult {
  id: string;
  status: "running" | "completed" | "failed";
  summary: string;
  overallVerdict: string;
  model: string;
  reasoningEffort: string;
  contextMode?: string;
  contextFileCount?: number;
  startedAt: string;
  completedAt?: string;
  reviews: CodexClusterReview[];
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
}

export interface Transaction {
  id: string;
  type: string;
  status: string;
  targets: string[];
  startedAt: string;
  completedAt?: string;
  error?: string;
}

export interface UpdateStatus {
  groupId: string;
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
}

export interface Candidate {
  name: string;
  description: string;
  sourcePath: string;
}

export interface InstallPreview {
  id: string;
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
  createdAt: string;
  expiresAt: string;
}

export interface AdoptionPreview {
  id: string;
  skills: Skill[];
  sources: Array<{
    skillName: string;
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
