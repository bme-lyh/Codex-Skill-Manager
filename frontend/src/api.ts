import type {
  AdoptionPreview,
  AssistedInstallPlan,
  AssistedInstallProgress,
  AssistedInstallResult,
  CodexCLIStatus,
  CodexReviewProgress,
  Dashboard,
  Finding,
  CodexProjectScanResult,
  InstallPreview,
  GroupSecurityReport,
  SourceAnalysis,
  SourceTrustAudit,
  SourceTrustPolicy,
  ProjectAssessment,
  RiskCluster,
  ScanReport,
  Severity,
  Transaction,
  UpdateCheckResult
} from "./types";

/** Opaque, digest-bound acknowledgement returned by the manager after the
 * user reviews a Codex installation plan.  The renderer sends only `id` when
 * applying; selections and digests are reloaded and verified by Go. */
export interface InstallConfirmation {
  id: string;
  planId: string;
  sourcePlanId: string;
  sourceDigest: string;
  reportDigest: string;
  assessmentDigest: string;
  planDigest: string;
  targetRootId: string;
  selectedSkills: string[];
  permissionIds: string[];
  highRiskAccepted: boolean;
  createdAt: string;
  expiresAt: string;
  usedAt?: string;
  digest: string;
}

export interface LinkedSource {
  skillName: string;
  rootId?: string;
  provider: string;
  sourceAssociation?: string;
  repository: string;
  sourceUrl: string;
  sourcePath: string;
  requestedRef?: string;
  resolvedCommit?: string;
}
import {
  demoAssistedInstallPlan,
  demoAssistedInstallProgress,
  demoAssistedInstallResult,
  demoCodexStatus,
  demoConfig,
  demoDashboard,
  demoInstallPreview,
  demoQuarantine,
  demoScanReport
} from "./demo";
import { demoLocaleFromLocation, localizeDemo } from "./demoLocale";
import { normalizeRootContract } from "./roots";

type Backend = {
  GetDashboard(): Promise<Dashboard>;
  BootstrapCurrentSkills(): Promise<void>;
  PrepareAdoption(names: string[], rootId: string): Promise<AdoptionPreview>;
  ApplyAdoption(plan: string, names: string[], rootId: string): Promise<Transaction>;
  ApplyAdoptionBestEffort?(plan: string, names: string[], rootId: string): Promise<Transaction>;
  CreateGroup(name: string, rootId: string): Promise<Transaction>;
  RenameGroup(id: string, name: string, rootId: string): Promise<Transaction>;
  ReorderGroups(ids: string[], rootId: string): Promise<Transaction>;
  MoveSkillsToGroup(names: string[], groupId: string, rootId: string): Promise<Transaction>;
  PrepareGitHub(url: string, ref: string, rootId: string): Promise<InstallPreview>;
  PrepareLocal(path: string, rootId: string): Promise<InstallPreview>;
  LinkLocalSource?(skillName: string, url: string, ref: string, rootId: string): Promise<LinkedSource>;
  AssessInstallSource?(sourcePlanId: string): Promise<ProjectAssessment>;
  GetProjectAssessment?(reference: string): Promise<ProjectAssessment>;
  ApplyInstall(plan: string, skills: string[], acceptHighRisk: boolean, rootId: string): Promise<Transaction>;
  ApplyInstallBestEffort?(plan: string, skills: string[], acceptHighRisk: boolean, rootId: string): Promise<Transaction>;
  ApplyGroupInstall?(plan: string, skills: string[], acceptRisk: boolean, rootId: string): Promise<Transaction>;
  ApplySourceGroupInstall?(plan: string, acceptRisk: boolean, rootId: string): Promise<Transaction>;
  ApplyGroupUpdate?(plan: string, skills: string[], acceptRisk: boolean, rootId: string): Promise<Transaction>;
  ApproveGroupRisk?(plan: string, reason: string): Promise<Transaction>;
  ApproveGroupSecurity?(groupId: string, rootId: string, reason: string): Promise<Transaction>;
  SetSourceTrust?(repository: string, reason: string): Promise<Transaction>;
  RevokeSourceTrust?(repository: string, reason: string): Promise<Transaction>;
  GetSourceTrustPolicy?(repository: string): Promise<SourceTrustPolicy>;
  GetSourceTrustPolicies?(): Promise<SourceTrustPolicy[]>;
  GetSourceTrustAudit?(repository: string, limit: number): Promise<SourceTrustAudit[]>;
  AuditSkill(name: string, rootId: string): Promise<ScanReport>;
  AuditSkills(names: string[], rootId: string): Promise<ScanReport>;
  RunGroupSecurityCheck?(groupId: string, rootId: string): Promise<GroupSecurityReport>;
  GetGroupSecurityReport?(id: string): Promise<GroupSecurityReport>;
  GetSourceAnalysis?(id: string): Promise<SourceAnalysis>;
  GetOrCreateSourceGroupAnalysis?(plan: string): Promise<SourceAnalysis>;
  GetSourceAnalyses?(limit: number): Promise<SourceAnalysis[]>;
  GetGroupOperations?(limit: number): Promise<any[]>;
  GetScanReport(id: string, rootId: string): Promise<ScanReport>;
  SetFindingIgnored(finding: Finding, ignored: boolean, reason: string): Promise<boolean>;
  SetRiskClusterIgnored(cluster: RiskCluster, ignored: boolean, reason: string, confirmDeterministic: boolean): Promise<boolean>;
  SetRiskClustersIgnored(clusters: RiskCluster[], ignored: boolean, reason: string): Promise<boolean>;
  CheckUpdates(): Promise<UpdateCheckResult>;
  CheckUpdatesSelected(groupIds: string[], force: boolean): Promise<UpdateCheckResult>;
  PrepareUpdate(groupId: string, rootId: string): Promise<InstallPreview>;
  PrepareGroupUpdate?(groupId: string, rootId: string): Promise<InstallPreview>;
  QuarantineSkills(names: string[], rootId: string): Promise<Transaction>;
  RestoreSkill(name: string, transaction: string, rootId: string): Promise<Transaction>;
  Rollback(transaction: string): Promise<Transaction>;
  GetConfig(): Promise<any>;
  SaveConfig(config: any): Promise<void>;
  ConfigureSchedule(enabled: boolean, frequency: string, at: string): Promise<void>;
  SaveGitHubToken(token: string, username: string): Promise<void>;
  ValidateGitHubCredentials(): Promise<Record<string, any>>;
  GetCodexCLIStatus(): Promise<CodexCLIStatus>;
  ReviewScanWithCodex(report: ScanReport, skillNames: string[]): Promise<ScanReport>;
  ScanProjectWithCodex?(sourcePlanId: string): Promise<CodexProjectScanResult>;
  AnalyzeInstallFromProjectScan?(projectScanId: string): Promise<AssistedInstallPlan>;
  ConfirmCodexInstall?(
    planId: string,
    skills: string[],
    permissionIds: string[],
    acceptHighRisk: boolean,
    rootId: string
  ): Promise<InstallConfirmation>;
  ApplyConfirmedAssistedInstall?(
    confirmationId: string,
    projectRoot: string,
    rootId: string
  ): Promise<AssistedInstallResult>;
  GetProjectScan?(reference: string): Promise<CodexProjectScanResult>;
  ApplyAssistedInstall?(
    planId: string,
    skills: string[],
    permissionIds: string[],
    projectRoot: string,
    rootId: string
  ): Promise<AssistedInstallResult>;
  GetAssistedInstallPlan?(planId: string): Promise<AssistedInstallPlan>;
  GetAssistedInstallProgress?(referenceId: string): Promise<AssistedInstallProgress>;
  CancelAssistedInstall?(referenceId: string): Promise<void>;
  GetDiagnostics(): Promise<Record<string, any>>;
  ListQuarantine(rootId: string): Promise<Array<{ skill: string; rootId: string; transactionId: string; path: string }>>;
};

const browserDemoLocale = demoLocaleFromLocation();
const localizedDemo = <T>(value: T): T => localizeDemo(value, browserDemoLocale);
const apiError = (zhCN: string, enUS: string): Error =>
  new Error(browserDemoLocale === "en-US" ? enUS : zhCN);
const disconnectedError = (): Error => apiError("桌面后端尚未连接", "The desktop backend is not connected");
const demo: Dashboard = localizedDemo(demoDashboard);

function backend(): Backend | undefined {
  return (window as any).go?.main?.App;
}

function normalizeDashboard(value: Dashboard): Dashboard {
  const rootContract = normalizeRootContract({
    roots: (value as Dashboard & { roots?: import("./roots").RootPayload[] })?.roots,
    defaultRootId: (value as Dashboard & { defaultRootId?: string })?.defaultRootId,
  });
  return {
    ...demo,
    ...value,
    skills: value?.skills ?? [],
    groups: value?.groups ?? [],
    sourceGroups: value?.sourceGroups?.length ? value.sourceGroups : value?.groups ?? [],
    relations: value?.relations ?? [],
    recentReports: value?.recentReports ?? [],
    recentHistory: value?.recentHistory ?? [],
    updateStatuses: value?.updateStatuses ?? [],
    roots: rootContract.roots,
    defaultRootId: rootContract.defaultRootId
  };
}

function normalizeScan(value: ScanReport): ScanReport {
  const findings = (value?.findings ?? []).map(finding => ({
    ...finding,
    severity: normalizeSeverity(finding.severity)
  }));
	const clusters = (value?.clusters ?? []).map(cluster => ({
		...cluster,
		severity: normalizeSeverity(cluster.severity),
		sampleFindings: (cluster.sampleFindings ?? []).map(finding => ({
			...finding,
			severity: normalizeSeverity(finding.severity)
		}))
	}));
  const codexReview = value?.codexReview ? {
    ...value.codexReview,
    reviews: value.codexReview.reviews ?? [],
    skillReviews: value.codexReview.skillReviews ?? [],
    batches: value.codexReview.batches ?? [],
    totalSkills: value.codexReview.totalSkills ?? value.codexReview.skillReviews?.length ?? 0,
    durationMillis: value.codexReview.durationMillis ?? 0
  } : undefined;
  return {
    ...value,
    findings,
    codexReview,
    clusters,
    skills: value?.skills ?? [],
    activeHighestSeverity: normalizeSeverity(value?.activeHighestSeverity ?? value?.highestSeverity),
    activeFindingCount: value?.activeFindingCount ?? findings.filter(f => !f.ignored).length,
    ignoredFindingCount: value?.ignoredFindingCount ?? findings.filter(f => f.ignored).length
  };
}

const severityValues = new Set<Severity>(["informational", "low", "medium", "high", "critical"]);

function normalizeSeverity(value: unknown): Severity {
  return severityValues.has(value as Severity) ? value as Severity : "critical";
}

function normalizeAssistedPlan(value: AssistedInstallPlan): AssistedInstallPlan {
  return {
    ...value,
    repository: value?.repository ?? demoAssistedInstallPlan.repository,
    requirements: value?.requirements ?? [],
    steps: value?.steps ?? [],
    permissions: value?.permissions ?? [],
    warnings: value?.warnings ?? [],
    contextFileCount: value?.contextFileCount ?? 0,
    needsProjectRoot: value?.needsProjectRoot ?? false,
    skills: value?.skills ?? [],
    scan: value?.scan ? normalizeScan(value.scan) : undefined
  };
}

function normalizeProjectScan(value: CodexProjectScanResult): CodexProjectScanResult {
  return {
    ...value,
    repository: value?.repository ?? demoInstallPreview.repository,
    summary: value?.summary ?? "",
    security: {
      verdict: value?.security?.verdict ?? "insufficient-context",
      summary: value?.security?.summary ?? "",
      confidence: value?.security?.confidence ?? 0,
      localHighestRisk: normalizeSeverity(value?.security?.localHighestRisk),
      localFindingCount: value?.security?.localFindingCount ?? 0,
      concerns: value?.security?.concerns ?? []
    },
    installationMethods: value?.installationMethods ?? [],
    contextFileCount: value?.contextFileCount ?? 0,
    summaryFileCount: value?.summaryFileCount ?? 0,
    deepAnalysisFileCount: value?.deepAnalysisFileCount ?? 0,
    omittedFileCount: value?.omittedFileCount ?? 0,
    redactedFileCount: value?.redactedFileCount ?? 0,
    truncatedFileCount: value?.truncatedFileCount ?? 0,
    focusFiles: value?.focusFiles ?? [],
    contextDigest: value?.contextDigest ?? "",
    scanDigest: value?.scanDigest ?? ""
  };
}

const assessmentGates = new Set(["ready", "attention", "blocked", "incomplete"]);
const assessmentRequirements = new Set(["required", "triggered", "optional"]);
const assessmentStatuses = new Set(["passed", "attention", "blocked", "pending", "not-applicable"]);

function normalizeAssessment(value: ProjectAssessment): ProjectAssessment {
  const knownGate = assessmentGates.has(value?.gate) ? value.gate : "incomplete";
  return {
    ...value,
    repository: value?.repository ?? demoInstallPreview.repository,
    classification: value?.classification ?? "unknown",
    classificationEvidence: value?.classificationEvidence ?? [],
    gate: knownGate,
    summary: knownGate === value?.gate ? value?.summary ?? "" : "The backend returned an unknown assessment state. Installation is unavailable.",
    highestRisk: normalizeSeverity(value?.highestRisk),
    coverage: {
      filesInventoried: value?.coverage?.filesInventoried ?? 0,
      filesScanned: value?.coverage?.filesScanned ?? 0,
      evidenceLimited: value?.coverage?.evidenceLimited ?? true
    },
    checks: (value?.checks ?? []).map(check => ({
      ...check,
      requirement: (assessmentRequirements.has(check.requirement) ? check.requirement : "required") as ProjectAssessment["checks"][number]["requirement"],
      status: (assessmentStatuses.has(check.status) ? check.status : "blocked") as ProjectAssessment["checks"][number]["status"],
      evidenceFiles: check.evidenceFiles ?? []
    })),
    targets: (value?.targets ?? []).map(target => ({
      ...target,
      supported: target.kind === "codex-skill" && target.supported === true,
      permissionIds: target.permissionIds ?? []
    })),
    enhancedScanRecommended: value?.enhancedScanRecommended ?? false,
    sourceDigest: value?.sourceDigest ?? "",
    assessmentDigest: value?.assessmentDigest ?? ""
  };
}

function demoAssessment(sourcePlanId: string): ProjectAssessment {
  const now = new Date();
  return normalizeAssessment({
    id: "assessment-demo",
    sourcePlanId,
    repository: demoInstallPreview.repository,
    classification: "skill",
    classificationEvidence: ["SKILL.md"],
    gate: "ready",
    summary: "Required local checks passed.",
    highestRisk: demoInstallPreview.scan.activeHighestSeverity,
    coverage: { filesInventoried: 1, filesScanned: 1, evidenceLimited: false },
    checks: [{
      id: "local-scan", layer: "baseline", requirement: "required", status: "passed",
      title: "Built-in security scan", summary: "No active blocking risk was found.",
      provider: "builtin-scanner", evidenceFiles: []
    }],
    targets: demoInstallPreview.skills.map(skill => ({
      kind: "codex-skill", displayName: skill.name, path: skill.name,
      supported: true, permissionIds: ["skills-write"], reversible: true
    })),
    enhancedScanRecommended: false,
    sourceDigest: "demo-source",
    assessmentDigest: "demo-assessment",
    createdAt: now.toISOString(),
    expiresAt: new Date(now.getTime() + 86400000).toISOString()
  });
}

function demoProjectScan(sourcePlanId: string): CodexProjectScanResult {
  return normalizeProjectScan({
    id: "demo-project-scan",
    sourcePlanId,
    status: "completed",
    repository: demoInstallPreview.repository,
    summary: "This repository publishes Skills and keeps optional integration work separate from the Skill copy.",
    security: {
      verdict: "mostly-contextual",
      summary: "Local checks found no open high-risk warning in this preview; review the focused files before authorizing installation.",
      confidence: 0.72,
      localHighestRisk: demoInstallPreview.scan.activeHighestSeverity,
      localFindingCount: demoInstallPreview.scan.activeFindingCount,
      concerns: []
    },
    installationMethods: [{
      kind: "skills-only",
      title: "Install the source group",
      description: "Copy every valid Skill in the source group through one manager transaction.",
      supported: true,
      required: true,
      evidenceFiles: ["README.md"]
    }],
    contextMode: "demo",
    contextFileCount: 0,
    summaryFileCount: 0,
    deepAnalysisFileCount: 0,
    omittedFileCount: 0,
    redactedFileCount: 0,
    truncatedFileCount: 0,
    focusFiles: [],
    contextDigest: "",
    scanDigest: "",
    startedAt: new Date().toISOString(),
    expiresAt: new Date(Date.now() + 86400000).toISOString()
  });
}

function normalizeAssistedProgress(value: AssistedInstallProgress): AssistedInstallProgress {
  return {
    ...value,
    referenceId: value?.referenceId ?? value?.runId ?? "",
    runId: value?.runId ?? value?.referenceId ?? "",
    sequence: value?.sequence ?? 0,
    phase: value?.phase ?? "queued",
    message: value?.message ?? "",
    completedSteps: value?.completedSteps ?? 0,
    totalSteps: value?.totalSteps ?? value?.steps?.length ?? 0,
    activityCount: value?.activityCount ?? 0,
    steps: value?.steps ?? [],
    terminal: value?.terminal ?? false
  };
}

export const api = {
  dashboard: async () => {
    const b = backend();
    return b ? normalizeDashboard(await b.GetDashboard()) : normalizeDashboard(demo);
  },
  bootstrap: async () => backend()?.BootstrapCurrentSkills(),
  prepareAdoption: async (names: string[], rootId: string) => {
    const b = backend();
    if (!b) throw disconnectedError();
    return b.PrepareAdoption(names, rootId);
  },
  applyAdoption: async (plan: string, names: string[], rootId: string) => {
    const b = backend();
    if (!b) throw disconnectedError();
    if (typeof b.ApplyAdoptionBestEffort === "function" && names.length > 1) {
      return b.ApplyAdoptionBestEffort(plan, names, rootId);
    }
    return b.ApplyAdoption(plan, names, rootId);
  },
  createGroup: async (name: string, rootId: string) => {
    const b = backend();
    if (!b) throw disconnectedError();
    return b.CreateGroup(name, rootId);
  },
  renameGroup: async (id: string, name: string, rootId: string) => {
    const b = backend();
    if (!b) throw disconnectedError();
    return b.RenameGroup(id, name, rootId);
  },
  reorderGroups: async (ids: string[], rootId: string) => {
    const b = backend();
    if (!b) throw disconnectedError();
    return b.ReorderGroups(ids, rootId);
  },
  moveSkills: async (names: string[], groupId: string, rootId: string) => {
    const b = backend();
    if (!b) throw disconnectedError();
    return b.MoveSkillsToGroup(names, groupId, rootId);
  },
  prepareGitHub: async (url: string, ref = "", rootId = "codex-default") => {
    const b = backend();
    if (!b) return localizedDemo(demoInstallPreview);
    return b.PrepareGitHub(url, ref, rootId);
  },
  prepareLocal: async (path: string, rootId = "codex-default") => {
    const b = backend();
    if (!b) return localizedDemo(demoInstallPreview);
    return b.PrepareLocal(path, rootId);
  },
  linkLocalSource: async (skillName: string, url: string, ref = "", rootId = "codex-default"): Promise<LinkedSource> => {
    const b = backend();
    if (!b || typeof b.LinkLocalSource !== "function") {
      throw apiError("当前桌面后端不支持关联远程来源，请升级应用", "This desktop backend does not support linking a remote source. Update the app.");
    }
    return b.LinkLocalSource(skillName, url, ref, rootId);
  },
  assessSource: async (sourcePlanId: string): Promise<ProjectAssessment> => {
    const b = backend();
    if (!b) return localizedDemo(demoAssessment(sourcePlanId));
    if (typeof b.AssessInstallSource !== "function") {
      throw apiError("当前桌面后端不支持分层本地评估，请升级应用", "This desktop backend does not support layered local assessment. Update the app.");
    }
    return normalizeAssessment(await b.AssessInstallSource(sourcePlanId));
  },
  getAssessment: async (reference: string): Promise<ProjectAssessment> => {
    const b = backend();
    if (!b) return localizedDemo(demoAssessment(reference));
    if (typeof b.GetProjectAssessment !== "function") {
      throw apiError("当前桌面后端不支持恢复分层评估，请重新分析来源", "This desktop backend cannot restore the layered assessment. Assess the source again.");
    }
    return normalizeAssessment(await b.GetProjectAssessment(reference));
  },
  apply: async (plan: string, skills: string[], accept: boolean, rootId: string) => {
    const b = backend();
    if (!b) throw disconnectedError();
    if (typeof b.ApplyGroupInstall === "function") {
      return b.ApplyGroupInstall(plan, skills, accept, rootId);
    }
    if (typeof b.ApplyInstallBestEffort === "function" && skills.length > 1) {
      return b.ApplyInstallBestEffort(plan, skills, accept, rootId);
    }
    return b.ApplyInstall(plan, skills, accept, rootId);
  },
  applySourceGroup: async (plan: string, accept: boolean, rootId: string) => {
    const b = backend();
    if (!b) throw disconnectedError();
    if (typeof b.ApplySourceGroupInstall === "function") return b.ApplySourceGroupInstall(plan, accept, rootId);
    throw apiError("当前版本不支持整组安装，请更新应用", "This desktop backend does not support group installation. Update the app.");
  },
  applyGroupUpdate: async (plan: string, skills: string[], accept: boolean, rootId: string) => {
    const b = backend();
    if (!b) throw disconnectedError();
    if (typeof b.ApplyGroupUpdate === "function") return b.ApplyGroupUpdate(plan, skills, accept, rootId);
    return b.ApplyInstall(plan, skills, accept, rootId);
  },
  approveGroupRisk: async (plan: string, reason = "") => {
    const b = backend();
    if (!b || typeof b.ApproveGroupRisk !== "function") throw apiError("当前版本不支持分组风险通过，请更新应用", "This desktop backend does not support group risk approval. Update the app.");
    return b.ApproveGroupRisk(plan, reason);
  },
  approveGroupSecurity: async (groupId: string, rootId: string, reason = "") => {
    const b = backend();
    if (!b || typeof b.ApproveGroupSecurity !== "function") throw apiError("当前版本不支持分组安全通过，请更新应用", "This desktop backend does not support group security approval. Update the app.");
    return b.ApproveGroupSecurity(groupId, rootId, reason);
  },
  audit: async (name: string, rootId: string) => {
    const b = backend();
    if (!b) return localizedDemo(demoScanReport);
    return normalizeScan(await b.AuditSkill(name, rootId));
  },
  auditSkills: async (names: string[], rootId: string) => {
    const b = backend();
    if (!b) return localizedDemo(demoScanReport);
    return normalizeScan(await b.AuditSkills(names, rootId));
  },
  auditGroup: async (groupId: string, rootId: string): Promise<GroupSecurityReport> => {
    const b = backend();
    if (!b || typeof b.RunGroupSecurityCheck !== "function") throw apiError("当前版本不支持分组安全检查，请更新应用", "This desktop backend does not support group security checks. Update the app.");
    return b.RunGroupSecurityCheck(groupId, rootId);
  },
  getOrCreateSourceGroupAnalysis: async (plan: string): Promise<SourceAnalysis> => {
    const b = backend();
    if (!b || typeof b.GetOrCreateSourceGroupAnalysis !== "function") throw apiError("当前版本不支持来源分析，请更新应用", "This desktop backend does not support reusable source analysis. Update the app.");
    return b.GetOrCreateSourceGroupAnalysis(plan);
  },
  sourceTrust: async (repository: string): Promise<SourceTrustPolicy> => {
    const b = backend();
    if (!b || typeof b.GetSourceTrustPolicy !== "function") throw apiError("当前版本不支持来源信任设置，请更新应用", "This desktop backend does not support source trust policies. Update the app.");
    return b.GetSourceTrustPolicy(repository);
  },
  setSourceTrust: async (repository: string, reason = "") => {
    const b = backend();
    if (!b || typeof b.SetSourceTrust !== "function") throw apiError("当前版本不支持来源信任设置，请更新应用", "This desktop backend does not support source trust policies. Update the app.");
    return b.SetSourceTrust(repository, reason);
  },
  revokeSourceTrust: async (repository: string, reason = "") => {
    const b = backend();
    if (!b || typeof b.RevokeSourceTrust !== "function") throw apiError("当前版本不支持撤销来源信任，请更新应用", "This desktop backend does not support source trust policies. Update the app.");
    return b.RevokeSourceTrust(repository, reason);
  },
  report: async (id: string, rootId = "") => {
    const b = backend();
    if (!b) return normalizeScan(demoScanReport);
    return normalizeScan(await b.GetScanReport(id, rootId));
  },
  setFindingIgnored: async (finding: Finding, ignored: boolean, reason = "") => {
    const b = backend();
    if (!b) throw disconnectedError();
    return b.SetFindingIgnored(finding, ignored, reason);
  },
  setRiskClusterIgnored: async (cluster: RiskCluster, ignored: boolean, reason = "", confirmDeterministic = false) => {
    const b = backend();
    if (!b) throw disconnectedError();
    return b.SetRiskClusterIgnored(cluster, ignored, reason, confirmDeterministic);
  },
  setRiskClustersIgnored: async (clusters: RiskCluster[], ignored: boolean, reason = "") => {
    const b = backend();
    if (!b) throw disconnectedError();
    return b.SetRiskClustersIgnored(clusters, ignored, reason);
  },
  check: async () => {
    const b = backend();
    if (!b) return { checkedAt: demo.lastUpdateCheck ?? "", statuses: demo.updateStatuses };
    return b.CheckUpdates();
  },
  checkSelected: async (groupIds: string[], force = false) => {
    const b = backend();
    if (!b) {
      return {
        checkedAt: demo.lastUpdateCheck ?? "",
        statuses: demo.updateStatuses.filter(status => groupIds.includes(status.groupId))
      };
    }
    return b.CheckUpdatesSelected(groupIds, force);
  },
  prepareUpdate: async (groupId: string, rootId: string) => {
    const b = backend();
    if (!b) throw disconnectedError();
    if (typeof b.PrepareGroupUpdate === "function") return b.PrepareGroupUpdate(groupId, rootId);
    return b.PrepareUpdate(groupId, rootId);
  },
  prepareGroupUpdate: async (groupId: string, rootId: string) => {
    const b = backend();
    if (!b) throw disconnectedError();
    if (typeof b.PrepareGroupUpdate === "function") return b.PrepareGroupUpdate(groupId, rootId);
    return b.PrepareUpdate(groupId, rootId);
  },
  quarantine: async (names: string[], rootId: string) => {
    const b = backend();
    if (!b) throw disconnectedError();
    return b.QuarantineSkills(names, rootId);
  },
  restore: async (name: string, tx: string, rootId: string) => {
    const b = backend();
    if (!b) throw disconnectedError();
    return b.RestoreSkill(name, tx, rootId);
  },
  rollback: async (tx: string) => {
    const b = backend();
    if (!b) throw disconnectedError();
    return b.Rollback(tx);
  },
  config: async () => {
    const b = backend();
    const value = (b ? await b.GetConfig() : { ...localizedDemo(demoConfig), locale: browserDemoLocale }) ?? {};
    const roots = normalizeRootContract(value);
    return { ...value, roots: roots.roots, defaultRootId: roots.defaultRootId };
  },
  saveConfig: async (cfg: any) => backend()?.SaveConfig(cfg),
  schedule: async (enabled: boolean, frequency: string, at: string) =>
    backend()?.ConfigureSchedule(enabled, frequency, at),
  saveGitHubToken: async (token: string, username: string) => {
    const b = backend();
    if (!b) throw disconnectedError();
    return b.SaveGitHubToken(token, username);
  },
  validateGitHub: async () => {
    const b = backend();
    if (!b) throw disconnectedError();
    return b.ValidateGitHubCredentials();
  },
  codexStatus: async (): Promise<CodexCLIStatus> => {
    const b = backend();
    if (!b) return localizedDemo(demoCodexStatus);
    return b.GetCodexCLIStatus();
  },
  reviewWithCodex: async (report: ScanReport, skillNames: string[] = []) => {
    const b = backend();
    if (!b) throw disconnectedError();
    return normalizeScan(await b.ReviewScanWithCodex(report, skillNames));
  },
  scanProject: async (sourcePlanId: string): Promise<CodexProjectScanResult> => {
    const b = backend();
    if (!b) return localizedDemo(demoProjectScan(sourcePlanId));
    if (typeof b.ScanProjectWithCodex !== "function") {
      throw apiError("当前桌面后端不支持可复用的 Codex 项目扫描，请升级应用", "This desktop backend does not support reusable Codex project scans. Update the app.");
    }
    return normalizeProjectScan(await b.ScanProjectWithCodex(sourcePlanId));
  },
  createAssistedPlanFromScan: async (projectScanId: string): Promise<AssistedInstallPlan> => {
    const b = backend();
    if (!b) return normalizeAssistedPlan(localizedDemo(demoAssistedInstallPlan));
    if (typeof b.AnalyzeInstallFromProjectScan !== "function") {
      throw apiError("当前桌面后端不支持在项目扫描后生成 Codex 安装计划，请升级应用", "This desktop backend cannot create a Codex installation plan from the project scan. Update the app.");
    }
    return normalizeAssistedPlan(await b.AnalyzeInstallFromProjectScan(projectScanId));
  },
  confirmCodexInstall: async (
    planId: string,
    skills: string[],
    permissionIds: string[],
    acceptHighRisk: boolean,
    rootId: string
  ): Promise<InstallConfirmation> => {
    const b = backend();
    if (!b) throw disconnectedError();
    if (typeof b.ConfirmCodexInstall !== "function") {
      throw apiError(
        "当前桌面后端不支持一次性 Codex 安装确认，请升级应用",
        "This desktop backend does not support one-time Codex installation confirmation. Update the app."
      );
    }
    return b.ConfirmCodexInstall(planId, skills, permissionIds, acceptHighRisk, rootId);
  },
  getProjectScan: async (reference: string): Promise<CodexProjectScanResult> => {
    const b = backend();
    if (!b) return localizedDemo(demoProjectScan(reference));
    if (typeof b.GetProjectScan !== "function") {
      throw apiError("当前桌面后端不支持恢复 Codex 项目扫描，请升级应用", "This desktop backend cannot restore the Codex project scan. Update the app.");
    }
    return normalizeProjectScan(await b.GetProjectScan(reference));
  },
  applyAssisted: async (
    planId: string,
    skills: string[],
    permissionIds: string[],
    projectRoot = "",
    rootId: string
  ): Promise<AssistedInstallResult> => {
    const b = backend();
    if (!b) return localizedDemo({
      ...demoAssistedInstallResult,
      plan: normalizeAssistedPlan(demoAssistedInstallResult.plan),
      progress: demoAssistedInstallResult.progress
        ? normalizeAssistedProgress(demoAssistedInstallResult.progress)
        : undefined
    });
    if (typeof b.ApplyAssistedInstall !== "function") {
      throw apiError("当前桌面后端不支持执行计划安装，请升级应用或切换到标准安装", "This desktop backend cannot run planned installation. Update the app or use standard installation.");
    }
    const result = await b.ApplyAssistedInstall(planId, skills, permissionIds, projectRoot, rootId);
    return {
      ...result,
      plan: normalizeAssistedPlan(result.plan),
      progress: result.progress ? normalizeAssistedProgress(result.progress) : undefined
    };
  },
  applyConfirmedAssisted: async (
    confirmationId: string,
    projectRoot = "",
    rootId: string
  ): Promise<AssistedInstallResult> => {
    const b = backend();
    if (!b) throw disconnectedError();
    if (typeof b.ApplyConfirmedAssistedInstall !== "function") {
      throw apiError(
        "当前桌面后端不支持受确认绑定的安装，请升级应用",
        "This desktop backend does not support confirmation-bound installation. Update the app."
      );
    }
    const result = await b.ApplyConfirmedAssistedInstall(confirmationId, projectRoot, rootId);
    return {
      ...result,
      plan: normalizeAssistedPlan(result.plan),
      progress: result.progress ? normalizeAssistedProgress(result.progress) : undefined
    };
  },
  getAssistedPlan: async (planId: string): Promise<AssistedInstallPlan> => {
    const b = backend();
    if (!b) return normalizeAssistedPlan(localizedDemo({ ...demoAssistedInstallPlan, id: planId || demoAssistedInstallPlan.id }));
    if (typeof b.GetAssistedInstallPlan !== "function") {
      throw apiError("当前桌面后端不支持恢复计划安装", "This desktop backend cannot restore the planned installation.");
    }
    return normalizeAssistedPlan(await b.GetAssistedInstallPlan(planId));
  },
  getAssistedProgress: async (referenceId: string): Promise<AssistedInstallProgress> => {
    const b = backend();
    if (!b) return normalizeAssistedProgress(localizedDemo({
      ...demoAssistedInstallProgress,
      referenceId,
      runId: "",
      sequence: 0,
      phase: "ready",
      message: "",
      completedSteps: 0,
      activityCount: 0,
      terminal: false,
      steps: demoAssistedInstallProgress.steps.map(step => ({
        ...step,
        status: step.id === "initialize-project-index" ? "manual-pending" : "queued"
      }))
    }));
    if (typeof b.GetAssistedInstallProgress !== "function") {
      throw apiError("当前桌面后端不支持恢复计划安装进度", "This desktop backend cannot restore planned installation progress.");
    }
    return normalizeAssistedProgress(await b.GetAssistedInstallProgress(referenceId));
  },
  cancelAssisted: async (referenceId: string): Promise<void> => {
    const b = backend();
    if (!b) return;
    if (typeof b.CancelAssistedInstall !== "function") {
      throw apiError("当前桌面后端不支持取消计划安装", "This desktop backend cannot cancel planned installation.");
    }
    await b.CancelAssistedInstall(referenceId);
  },
  onCodexReviewProgress: (handler: (progress: CodexReviewProgress) => void): (() => void) => {
    const wailsRuntime = (window as any).runtime;
    if (typeof wailsRuntime?.EventsOn !== "function") return () => undefined;
    const unsubscribe = wailsRuntime.EventsOn("codex-review-progress", handler);
    return typeof unsubscribe === "function" ? unsubscribe : () => undefined;
  },
  onAssistedInstallProgress: (handler: (progress: AssistedInstallProgress) => void): (() => void) => {
    const wailsRuntime = (window as any).runtime;
    if (typeof wailsRuntime?.EventsOn !== "function") return () => undefined;
    const unsubscribe = wailsRuntime.EventsOn("assisted-install-progress", (progress: AssistedInstallProgress) =>
      handler(normalizeAssistedProgress(progress)));
    return typeof unsubscribe === "function" ? unsubscribe : () => undefined;
  },
  diagnostics: async () => {
    const b = backend();
    return b ? b.GetDiagnostics() : {
      version: "UI preview",
      skillsRootExists: true,
      dataRootExists: true,
      configPath: "D:\\CodexSkillManager\\data\\config.yaml"
    };
  },
  quarantineList: async (rootId: string) => {
    const b = backend();
    return b ? (await b.ListQuarantine(rootId)) ?? [] : localizedDemo(demoQuarantine).map(item => ({ ...item, rootId }));
  }
};
