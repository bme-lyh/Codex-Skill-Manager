import type {
  AssistedInstallPlan,
  AssistedInstallProgress,
  AssistedInstallStep,
  ProjectAssessment
} from "../types";

export const ACTIVE_INSTALL_REFERENCE_VERSION = 1 as const;

export type ActiveInstallReferenceKind = "analysis" | "plan";

export interface ActiveInstallReference {
  version: typeof ACTIVE_INSTALL_REFERENCE_VERSION;
  kind: ActiveInstallReferenceKind;
  id: string;
}

export interface ParsedActiveInstallReference {
  reference: ActiveInstallReference;
  migrated: boolean;
}

export interface InstallIssueSignals {
  code: string;
  rawMessage: string;
  rawDetail: string;
  retryAt: string;
  rateLimited: boolean;
  githubForbidden: boolean;
  codexUnavailable: boolean;
  restartRequired: boolean;
  invalidInput: boolean;
  skillVariantConflict: boolean;
  suggestedSourceUrl: string;
}

export interface AssistedPlanDisposition {
  manualRequired: boolean;
  manualOnly: boolean;
  partial: boolean;
  supportedStepCount: number;
}

export function assessmentAllowsSelectedTargets(
  assessment: Pick<ProjectAssessment, "gate" | "targets"> | null,
  selectedNames: string[]
): boolean {
  if (!assessment || (assessment.gate !== "ready" && assessment.gate !== "attention")) {
    return false;
  }
  const supported = new Set(
    assessment.targets
      .filter(target => target.kind === "codex-skill" && target.supported)
      .map(target => target.displayName)
  );
  return selectedNames.length > 0 && selectedNames.every(name => supported.has(name));
}

export function createActiveInstallReference(
  kind: ActiveInstallReferenceKind,
  id: string
): ActiveInstallReference {
  return {
    version: ACTIVE_INSTALL_REFERENCE_VERSION,
    kind,
    id
  };
}

export function serializeActiveInstallReference(reference: ActiveInstallReference): string {
  return JSON.stringify(reference);
}

export function parseActiveInstallReference(raw: string | null): ParsedActiveInstallReference | null {
  if (!raw?.trim()) return null;
  const value = raw.trim();
  try {
    const parsed = JSON.parse(value) as unknown;
    if (typeof parsed === "string") return legacyActiveInstallReference(parsed);
    if (!parsed || typeof parsed !== "object") return null;
    const record = parsed as Record<string, unknown>;
    if ((record.kind !== "analysis" && record.kind !== "plan") ||
      typeof record.id !== "string" || !record.id.trim() ||
      (record.version !== undefined && record.version !== ACTIVE_INSTALL_REFERENCE_VERSION)) {
      return null;
    }
    return {
      reference: createActiveInstallReference(record.kind, record.id),
      migrated: record.version !== ACTIVE_INSTALL_REFERENCE_VERSION
    };
  } catch {
    return legacyActiveInstallReference(value);
  }
}

function legacyActiveInstallReference(value: string): ParsedActiveInstallReference | null {
  const id = value.trim();
  const kind = id.startsWith("assisted-plan-")
    ? "plan"
    : id.startsWith("plan-")
      ? "analysis"
      : null;
  if (!kind) return null;
  return {
    reference: createActiveInstallReference(kind, id),
    migrated: true
  };
}

export function mergeProgressSnapshot(
  current: AssistedInstallProgress | null,
  incoming: AssistedInstallProgress
): AssistedInstallProgress {
  const sameRun = !!current && (
    (current.runId && incoming.runId && current.runId === incoming.runId) ||
    (!current.runId && !incoming.runId && current.referenceId === incoming.referenceId)
  );
  if (sameRun && incoming.sequence <= current.sequence) return current;
  return incoming;
}

export function restoredSelectedSkills(
  plan: Pick<AssistedInstallPlan, "selectedSkills">
): string[] {
  return [...new Set((plan.selectedSkills ?? []).filter(name =>
    typeof name === "string" && name.trim().length > 0
  ))];
}

export function assistedPlanDisposition(
  plan: Pick<AssistedInstallPlan, "status" | "steps">
): AssistedPlanDisposition {
  const status = plan.status?.toLowerCase() ?? "";
  const requiredManualSteps = plan.steps.filter(step => isRequiredManualStep(step));
  const supportedStepCount = plan.steps.filter(step => step.supported).length;
  const manualRequired = status === "manual-required" || requiredManualSteps.length > 0;
  return {
    manualRequired,
    manualOnly: manualRequired && supportedStepCount === 0,
    partial: status === "partial" || manualRequired,
    supportedStepCount
  };
}

function isRequiredManualStep(step: AssistedInstallStep): boolean {
  return step.kind === "manual" && step.required;
}

export function classifyInstallIssue(error: unknown): InstallIssueSignals {
  const record = error && typeof error === "object" ? error as Record<string, unknown> : {};
  const rawMessage = typeof record.message === "string" ? record.message : String(error ?? "");
  const code = typeof record.code === "string" ? record.code : "";
  const rawDetail = typeof record.detail === "string" ? record.detail : "";
  const structuredRetryAt = typeof record.retryAt === "string" ? record.retryAt : "";
  const retryAt = structuredRetryAt ||
    rawMessage.match(/\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}(?::\d{2})?(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})?/)?.[0] ||
    "";
  const combined = `${code} ${rawMessage} ${rawDetail}`;
  const suggestedSourceUrl = combined.match(
    /suggested Codex source URL:\s*(https:\/\/github\.com\/[^\s;]+)/i
  )?.[1] ?? "";
  return {
    code,
    rawMessage,
    rawDetail,
    retryAt,
    rateLimited: /rate.?limit|限流|请求额度|API rate limit|remaining[^\n]*0/i.test(combined),
    githubForbidden: /GitHub[^\n]*(403|Forbidden)|403 Forbidden/i.test(combined),
    codexUnavailable: /Codex.*(?:not found|not logged|unavailable|unsupported|未找到|未登录|不可用|不支持)|CLI.*(?:not found|unavailable|未找到|不可用)/i
      .test(combined),
    restartRequired: /(?:plan|计划).*(?:expired|过期)|configuration changed after approval|配置.*(?:变化|变更)|context.*changed|上下文.*变化|digest mismatch|摘要不匹配|tamper/i
      .test(combined),
    invalidInput: /only https:\/\/github\.com|invalid GitHub repository URL|unsupported GitHub repository URL path|invalid local|无效.*(?:链接|目录|来源)/i
      .test(combined),
    skillVariantConflict: /multiple different Skills use the name|conflicting repository paths/i.test(combined),
    suggestedSourceUrl
  };
}

export function parseRetryTimestamp(value?: string): number {
  if (!value) return 0;
  const normalized = /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}/.test(value)
    ? value.replace(" ", "T")
    : value;
  const parsed = Date.parse(normalized);
  return Number.isFinite(parsed) ? parsed : 0;
}

export function retryWaitMilliseconds(retryAt: string | undefined, now: number): number {
  const retryTimestamp = parseRetryTimestamp(retryAt);
  return retryTimestamp ? Math.max(0, retryTimestamp - now) : 0;
}
