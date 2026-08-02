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
  ProjectAssessment,
  RiskCluster,
  ScanReport,
  Severity,
  Transaction,
  UpdateCheckResult
} from "./types";
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

type Backend = {
  GetDashboard(): Promise<Dashboard>;
  BootstrapCurrentSkills(): Promise<void>;
  PrepareAdoption(names: string[]): Promise<AdoptionPreview>;
  ApplyAdoption(plan: string, names: string[]): Promise<Transaction>;
  CreateGroup(name: string): Promise<Transaction>;
  RenameGroup(id: string, name: string): Promise<Transaction>;
  ReorderGroups(ids: string[]): Promise<Transaction>;
  MoveSkillsToGroup(names: string[], groupId: string): Promise<Transaction>;
  PrepareGitHub(url: string, ref: string): Promise<InstallPreview>;
  PrepareLocal(path: string): Promise<InstallPreview>;
  AssessInstallSource?(sourcePlanId: string): Promise<ProjectAssessment>;
  GetProjectAssessment?(reference: string): Promise<ProjectAssessment>;
  ApplyInstall(plan: string, skills: string[], acceptHighRisk: boolean): Promise<Transaction>;
  AuditSkill(name: string): Promise<ScanReport>;
  AuditSkills(names: string[]): Promise<ScanReport>;
  SetFindingIgnored(finding: Finding, ignored: boolean, reason: string): Promise<boolean>;
  SetRiskClusterIgnored(cluster: RiskCluster, ignored: boolean, reason: string, confirmDeterministic: boolean): Promise<boolean>;
  SetRiskClustersIgnored(clusters: RiskCluster[], ignored: boolean, reason: string): Promise<boolean>;
  CheckUpdates(): Promise<UpdateCheckResult>;
  CheckUpdatesSelected(groupIds: string[], force: boolean): Promise<UpdateCheckResult>;
  PrepareUpdate(groupId: string): Promise<InstallPreview>;
  QuarantineSkills(names: string[]): Promise<Transaction>;
  RestoreSkill(name: string, transaction: string): Promise<Transaction>;
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
  GetProjectScan?(reference: string): Promise<CodexProjectScanResult>;
  ApplyAssistedInstall?(
    planId: string,
    skills: string[],
    permissionIds: string[],
    projectRoot: string
  ): Promise<AssistedInstallResult>;
  GetAssistedInstallPlan?(planId: string): Promise<AssistedInstallPlan>;
  GetAssistedInstallProgress?(referenceId: string): Promise<AssistedInstallProgress>;
  CancelAssistedInstall?(referenceId: string): Promise<void>;
  GetDiagnostics(): Promise<Record<string, any>>;
  ListQuarantine(): Promise<Array<{ skill: string; transactionId: string; path: string }>>;
};

const demo: Dashboard = demoDashboard;

function backend(): Backend | undefined {
  return (window as any).go?.main?.App;
}

function normalizeDashboard(value: Dashboard): Dashboard {
  return {
    ...demo,
    ...value,
    skills: value?.skills ?? [],
    groups: value?.groups ?? [],
    sourceGroups: value?.sourceGroups?.length ? value.sourceGroups : value?.groups ?? [],
    relations: value?.relations ?? [],
    recentReports: value?.recentReports ?? [],
    recentHistory: value?.recentHistory ?? [],
    updateStatuses: value?.updateStatuses ?? []
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
      title: "Install selected Skills",
      description: "Copy the selected Skills through the manager transaction.",
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
  prepareAdoption: async (names: string[]) => {
    const b = backend();
    if (!b) throw new Error("桌面后端尚未连接");
    return b.PrepareAdoption(names);
  },
  applyAdoption: async (plan: string, names: string[]) => {
    const b = backend();
    if (!b) throw new Error("桌面后端尚未连接");
    return b.ApplyAdoption(plan, names);
  },
  createGroup: async (name: string) => {
    const b = backend();
    if (!b) throw new Error("桌面后端尚未连接");
    return b.CreateGroup(name);
  },
  renameGroup: async (id: string, name: string) => {
    const b = backend();
    if (!b) throw new Error("桌面后端尚未连接");
    return b.RenameGroup(id, name);
  },
  reorderGroups: async (ids: string[]) => {
    const b = backend();
    if (!b) throw new Error("桌面后端尚未连接");
    return b.ReorderGroups(ids);
  },
  moveSkills: async (names: string[], groupId: string) => {
    const b = backend();
    if (!b) throw new Error("桌面后端尚未连接");
    return b.MoveSkillsToGroup(names, groupId);
  },
  prepareGitHub: async (url: string, ref = "") => {
    const b = backend();
    if (!b) return demoInstallPreview;
    return b.PrepareGitHub(url, ref);
  },
  prepareLocal: async (path: string) => {
    const b = backend();
    if (!b) return demoInstallPreview;
    return b.PrepareLocal(path);
  },
  assessSource: async (sourcePlanId: string): Promise<ProjectAssessment> => {
    const b = backend();
    if (!b) return demoAssessment(sourcePlanId);
    if (typeof b.AssessInstallSource !== "function") {
      throw new Error("当前桌面后端不支持分层本地评估，请升级应用");
    }
    return normalizeAssessment(await b.AssessInstallSource(sourcePlanId));
  },
  getAssessment: async (reference: string): Promise<ProjectAssessment> => {
    const b = backend();
    if (!b) return demoAssessment(reference);
    if (typeof b.GetProjectAssessment !== "function") {
      throw new Error("当前桌面后端不支持恢复分层评估，请重新分析来源");
    }
    return normalizeAssessment(await b.GetProjectAssessment(reference));
  },
  apply: async (plan: string, skills: string[], accept: boolean) => {
    const b = backend();
    if (!b) throw new Error("桌面后端尚未连接");
    return b.ApplyInstall(plan, skills, accept);
  },
  audit: async (name: string) => {
    const b = backend();
    if (!b) return demoScanReport;
    return normalizeScan(await b.AuditSkill(name));
  },
  auditSkills: async (names: string[]) => {
    const b = backend();
    if (!b) return demoScanReport;
    return normalizeScan(await b.AuditSkills(names));
  },
  setFindingIgnored: async (finding: Finding, ignored: boolean, reason = "") => {
    const b = backend();
    if (!b) throw new Error("桌面后端尚未连接");
    return b.SetFindingIgnored(finding, ignored, reason);
  },
  setRiskClusterIgnored: async (cluster: RiskCluster, ignored: boolean, reason = "", confirmDeterministic = false) => {
    const b = backend();
    if (!b) throw new Error("桌面后端尚未连接");
    return b.SetRiskClusterIgnored(cluster, ignored, reason, confirmDeterministic);
  },
  setRiskClustersIgnored: async (clusters: RiskCluster[], ignored: boolean, reason = "") => {
    const b = backend();
    if (!b) throw new Error("桌面后端尚未连接");
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
  prepareUpdate: async (groupId: string) => {
    const b = backend();
    if (!b) throw new Error("桌面后端尚未连接");
    return b.PrepareUpdate(groupId);
  },
  quarantine: async (names: string[]) => {
    const b = backend();
    if (!b) throw new Error("桌面后端尚未连接");
    return b.QuarantineSkills(names);
  },
  restore: async (name: string, tx: string) => {
    const b = backend();
    if (!b) throw new Error("桌面后端尚未连接");
    return b.RestoreSkill(name, tx);
  },
  rollback: async (tx: string) => {
    const b = backend();
    if (!b) throw new Error("桌面后端尚未连接");
    return b.Rollback(tx);
  },
  config: async () => {
    const b = backend();
    return b ? b.GetConfig() : demoConfig;
  },
  saveConfig: async (cfg: any) => backend()?.SaveConfig(cfg),
  schedule: async (enabled: boolean, frequency: string, at: string) =>
    backend()?.ConfigureSchedule(enabled, frequency, at),
  saveGitHubToken: async (token: string, username: string) => {
    const b = backend();
    if (!b) throw new Error("桌面后端尚未连接");
    return b.SaveGitHubToken(token, username);
  },
  validateGitHub: async () => {
    const b = backend();
    if (!b) throw new Error("桌面后端尚未连接");
    return b.ValidateGitHubCredentials();
  },
  codexStatus: async (): Promise<CodexCLIStatus> => {
    const b = backend();
    if (!b) return demoCodexStatus;
    return b.GetCodexCLIStatus();
  },
  reviewWithCodex: async (report: ScanReport, skillNames: string[] = []) => {
    const b = backend();
    if (!b) throw new Error("桌面后端尚未连接");
    return normalizeScan(await b.ReviewScanWithCodex(report, skillNames));
  },
  scanProject: async (sourcePlanId: string): Promise<CodexProjectScanResult> => {
    const b = backend();
    if (!b) return demoProjectScan(sourcePlanId);
    if (typeof b.ScanProjectWithCodex !== "function") {
      throw new Error("当前桌面后端不支持可复用的 Codex 项目扫描，请升级应用");
    }
    return normalizeProjectScan(await b.ScanProjectWithCodex(sourcePlanId));
  },
  createAssistedPlanFromScan: async (projectScanId: string): Promise<AssistedInstallPlan> => {
    const b = backend();
    if (!b) return normalizeAssistedPlan(demoAssistedInstallPlan);
    if (typeof b.AnalyzeInstallFromProjectScan !== "function") {
      throw new Error("当前桌面后端不支持在项目扫描后生成 Codex 安装计划，请升级应用");
    }
    return normalizeAssistedPlan(await b.AnalyzeInstallFromProjectScan(projectScanId));
  },
  getProjectScan: async (reference: string): Promise<CodexProjectScanResult> => {
    const b = backend();
    if (!b) return demoProjectScan(reference);
    if (typeof b.GetProjectScan !== "function") {
      throw new Error("当前桌面后端不支持恢复 Codex 项目扫描，请升级应用");
    }
    return normalizeProjectScan(await b.GetProjectScan(reference));
  },
  applyAssisted: async (
    planId: string,
    skills: string[],
    permissionIds: string[],
    projectRoot = ""
  ): Promise<AssistedInstallResult> => {
    const b = backend();
    if (!b) return {
      ...demoAssistedInstallResult,
      plan: normalizeAssistedPlan(demoAssistedInstallResult.plan),
      progress: demoAssistedInstallResult.progress
        ? normalizeAssistedProgress(demoAssistedInstallResult.progress)
        : undefined
    };
    if (typeof b.ApplyAssistedInstall !== "function") {
      throw new Error("当前桌面后端不支持执行 Codex 一键安装，请升级应用或切换到标准安装");
    }
    const result = await b.ApplyAssistedInstall(planId, skills, permissionIds, projectRoot);
    return {
      ...result,
      plan: normalizeAssistedPlan(result.plan),
      progress: result.progress ? normalizeAssistedProgress(result.progress) : undefined
    };
  },
  getAssistedPlan: async (planId: string): Promise<AssistedInstallPlan> => {
    const b = backend();
    if (!b) return normalizeAssistedPlan({ ...demoAssistedInstallPlan, id: planId || demoAssistedInstallPlan.id });
    if (typeof b.GetAssistedInstallPlan !== "function") {
      throw new Error("当前桌面后端不支持恢复 Codex 一键安装计划");
    }
    return normalizeAssistedPlan(await b.GetAssistedInstallPlan(planId));
  },
  getAssistedProgress: async (referenceId: string): Promise<AssistedInstallProgress> => {
    const b = backend();
    if (!b) return normalizeAssistedProgress({
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
    });
    if (typeof b.GetAssistedInstallProgress !== "function") {
      throw new Error("当前桌面后端不支持恢复 Codex 一键安装进度");
    }
    return normalizeAssistedProgress(await b.GetAssistedInstallProgress(referenceId));
  },
  cancelAssisted: async (referenceId: string): Promise<void> => {
    const b = backend();
    if (!b) return;
    if (typeof b.CancelAssistedInstall !== "function") {
      throw new Error("当前桌面后端不支持取消 Codex 一键安装");
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
  quarantineList: async () => {
    const b = backend();
    return b ? (await b.ListQuarantine()) ?? [] : demoQuarantine;
  }
};
