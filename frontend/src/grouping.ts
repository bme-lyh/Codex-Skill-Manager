import type { AppLocale } from "./i18n";
import type { Candidate, Group, LocalizedText, Skill } from "./types";

/** Resolve persisted bilingual fields without making older payloads invalid. */
export function localizedValue(
  value: unknown,
  locale: AppLocale,
  fallback = ""
): string {
  if (typeof value === "string") return value || fallback;
  if (!value || typeof value !== "object") return fallback;
  const record = value as Record<string, unknown>;
  const preferred = locale === "en-US"
    ? ["enUS", "en-US", "en", "english", "English"]
    : ["zhCN", "zh-CN", "zh", "chinese", "Chinese"];
  const alternate = locale === "en-US"
    ? ["zhCN", "zh-CN", "zh"]
    : ["enUS", "en-US", "en"];
  for (const key of [...preferred, ...alternate, "value", "text", "name", "title"]) {
    const candidate = record[key];
    if (typeof candidate === "string" && candidate.trim()) return candidate;
  }
  return fallback;
}

export function localizedText(value: LocalizedText | undefined, locale: AppLocale, fallback = "") {
  return localizedValue(value, locale, fallback);
}

export function groupLocalizedName(group: Partial<Group> | undefined, locale: AppLocale, fallback = "") {
  if (!group) return fallback;
  return localizedValue(
    group.localizedName ?? {
      zhCN: group.nameZhCN,
      enUS: group.nameEnUS,
    },
    locale,
    group.name || fallback
  );
}

export function candidateLocalizedName(candidate: Partial<Candidate>, locale: AppLocale) {
  return localizedValue(candidate.localizedName, locale, candidate.name || "");
}

export function candidateLocalizedDescription(candidate: Partial<Candidate>, locale: AppLocale) {
  return localizedValue(candidate.localizedDescription, locale, candidate.description || "");
}

export function candidateGroupId(candidate: Partial<Candidate>, fallback = "source") {
  return candidate.sourceGroupId || candidate.groupId || fallback;
}

export function candidateGroupName(
  candidate: Partial<Candidate>,
  locale: AppLocale,
  fallback = ""
) {
  const name = localizedValue(
    candidate.localizedSourceGroupName ?? candidate.localizedGroupName ?? {
      zhCN: candidate.sourceGroupNameZhCN || candidate.groupNameZhCN,
      enUS: candidate.sourceGroupNameEnUS || candidate.groupNameEnUS,
    },
    locale,
    candidate.sourceGroupName || candidate.groupName || fallback
  );
  return name || fallback;
}

export function sourceGroupIdForSkill(skill: Partial<Skill>, fallback = "source") {
  return skill.sourceGroupId || skill.groupId || fallback;
}

export function sourceGroupNameForSkill(skill: Partial<Skill>, locale: AppLocale, fallback = "") {
  return localizedValue(
    skill.localizedSourceGroupName ?? skill.localizedGroupName ?? {
      zhCN: skill.sourceGroupNameZhCN || skill.groupNameZhCN,
      enUS: skill.sourceGroupNameEnUS || skill.groupNameEnUS,
    },
    locale,
    skill.sourceGroupName || skill.groupName || fallback
  );
}

export interface CandidateGroup {
  id: string;
  name: string;
  source?: Group;
  candidates: Candidate[];
}

/** Group candidate Skills by immutable source identity. */
export function groupCandidates(
  candidates: Candidate[],
  sourceGroups: Group[] = [],
  fallbackName = "Source group"
): CandidateGroup[] {
  const byId = new Map(sourceGroups.map(group => [group.sourceGroupId || group.id, group]));
  const groups = new Map<string, CandidateGroup>();
  for (const candidate of candidates) {
    const id = candidateGroupId(candidate, sourceGroups.length === 1
      ? sourceGroups[0].sourceGroupId || sourceGroups[0].id
      : "source");
    const source = byId.get(id);
    const group = groups.get(id) ?? {
      id,
      name: candidate.sourceGroupName || candidate.groupName || source?.name || fallbackName,
      source,
      candidates: []
    };
    if (!group.name || group.name === fallbackName) {
      group.name = candidate.sourceGroupName || candidate.groupName || source?.name || fallbackName;
    }
    group.candidates.push(candidate);
    groups.set(id, group);
  }
  // Preserve backend group ordering, including groups whose candidates carry
  // no explicit source metadata in a legacy response.
  const ordered = [...groups.values()];
  ordered.sort((left, right) => {
    const leftPosition = left.source?.position ?? Number.MAX_SAFE_INTEGER;
    const rightPosition = right.source?.position ?? Number.MAX_SAFE_INTEGER;
    return leftPosition - rightPosition || left.name.localeCompare(right.name);
  });
  return ordered;
}

export function allCandidateNames(groups: CandidateGroup[]) {
  return groups.flatMap(group => group.candidates.map(candidate => candidate.name));
}

/** Expand any legacy partial list to complete source-group selections. */
export function normalizeGroupSelection(
  selected: string[],
  groups: CandidateGroup[]
) {
  const selectedSet = new Set(selected);
  return groups.flatMap(group => {
    const names = group.candidates.map(candidate => candidate.name);
    return names.some(name => selectedSet.has(name)) ? names : [];
  });
}
