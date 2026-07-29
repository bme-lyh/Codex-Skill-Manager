import type { AdoptionPreview, CodexCLIStatus, Dashboard, Finding, InstallPreview, RiskCluster, ScanReport, Transaction, UpdateCheckResult } from "./types";
import {
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
  ApplyInstall(plan: string, skills: string[], acceptHighRisk: boolean): Promise<Transaction>;
  AuditSkill(name: string): Promise<ScanReport>;
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
  ReviewScanWithCodex(report: ScanReport): Promise<ScanReport>;
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
  const findings = value?.findings ?? [];
  return {
    ...value,
    findings,
    clusters: value?.clusters ?? [],
    activeHighestSeverity: value?.activeHighestSeverity ?? value?.highestSeverity ?? "informational",
    activeFindingCount: value?.activeFindingCount ?? findings.filter(f => !f.ignored).length,
    ignoredFindingCount: value?.ignoredFindingCount ?? findings.filter(f => f.ignored).length
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
  reviewWithCodex: async (report: ScanReport) => {
    const b = backend();
    if (!b) throw new Error("桌面后端尚未连接");
    return normalizeScan(await b.ReviewScanWithCodex(report));
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
