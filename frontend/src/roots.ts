/**
 * Frontend root contract.
 *
 * The desktop API is moving from a single Skills path to named roots.  The
 * UI keeps the new field names (`rootId`, `rootKind`, `rootName`, `roots`,
 * `defaultRootId`) at its boundary and accepts the v0.10 `id`/`path` shape
 * while the Go API is rolled forward.  Nothing in this module performs a
 * filesystem operation; it only normalizes display data.
 */

export type RootKind = "codex" | "agents" | "custom" | "system" | string;

export interface RootContract {
  rootId: string;
  rootKind: RootKind;
  rootName: string;
  path: string;
  enabled: boolean;
  skillCount?: number;
}

export interface RootPayload {
  rootId?: string;
  rootKind?: RootKind;
  rootName?: string;
  id?: string;
  kind?: RootKind;
  name?: string;
  path?: string;
  enabled?: boolean;
  skillCount?: number;
}

export interface RootContractPayload {
  roots?: RootPayload[];
  skillRoots?: RootPayload[];
  defaultRootId?: string;
  paths?: { skillsRoot?: string };
}

export interface NormalizedRootContract {
  roots: RootContract[];
  defaultRootId: string;
}

function inferKind(id: string, path: string): RootKind {
  const value = `${id} ${path}`.toLowerCase();
  if (value.includes("agent")) return "agents";
  if (value.includes("system")) return "system";
  if (value.includes("codex")) return "codex";
  return "custom";
}

function fallbackName(kind: RootKind, id: string): string {
  if (kind === "agents") return "Agents Skills";
  if (kind === "system") return "System Skills";
  if (kind === "codex") return "Codex Skills";
  return id || "Skills root";
}

/** Normalize both the v0.11 contract and the legacy config payload. */
export function normalizeRootContract(payload: RootContractPayload | null | undefined): NormalizedRootContract {
  const source = payload?.roots?.length ? payload.roots : payload?.skillRoots;
  const legacyPath = payload?.paths?.skillsRoot ?? "";
  const values = source?.length ? source : legacyPath ? [{ id: "codex-default", path: legacyPath, enabled: true }] : [];
  const roots = values.map((value, index): RootContract => {
    const rootId = value.rootId ?? value.id ?? `root-${index + 1}`;
    const path = value.path ?? "";
    const rootKind = value.rootKind ?? value.kind ?? inferKind(rootId, path);
    return {
      rootId,
      rootKind,
      rootName: value.rootName ?? value.name ?? fallbackName(rootKind, rootId),
      path,
      enabled: value.enabled !== false,
      ...(typeof value.skillCount === "number" ? { skillCount: value.skillCount } : {})
    };
  }).filter(root => root.enabled || root.path);
  const preferred = payload?.defaultRootId && roots.some(root => root.rootId === payload.defaultRootId)
    ? payload.defaultRootId
    : roots.find(root => root.rootKind === "codex")?.rootId ?? roots[0]?.rootId ?? "";
  return { roots, defaultRootId: preferred };
}

export function rootKindLabel(kind: RootKind, locale: "zh-CN" | "en-US"): string {
  if (locale === "en-US") {
    return kind === "agents" ? "Agents" : kind === "system" ? "System" : kind === "codex" ? "Codex" : "Custom";
  }
  return kind === "agents" ? "Agents 根目录" : kind === "system" ? "系统根目录" : kind === "codex" ? "Codex 根目录" : "自定义根目录";
}

export function rootIdentity(rootId: string, skillName: string): string {
  return `${rootId}::${skillName}`;
}

export function matchesRoot(rootId: string | undefined, selectedRootId: string, fallbackRootId = "codex-default"): boolean {
  return selectedRootId === "all" || (rootId ?? fallbackRootId) === selectedRootId;
}
