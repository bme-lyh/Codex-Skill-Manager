import {
  Activity,
  ArchiveRestore,
  Boxes,
  FileClock,
  GitBranch,
  History,
  LayoutDashboard,
  RefreshCw,
  Settings,
  ShieldCheck
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { uiCopy } from "../i18n";

export type LocalizedLabel = Readonly<{
  zhCN: string;
  enUS: string;
}>;

export type NavigationGroupId =
  | "home"
  | "assets"
  | "security"
  | "activity"
  | "settings";

export type NavigationTabId =
  | "skills"
  | "groups"
  | "updates"
  | "history"
  | "quarantine"
  | "reports";

export type NavigationBadgeKey = "securityRiskCount";

export type NavigationTab = Readonly<{
  id: NavigationTabId;
  label: LocalizedLabel;
  icon: LucideIcon;
}>;

export type NavigationGroup = Readonly<{
  id: NavigationGroupId;
  label: LocalizedLabel;
  icon: LucideIcon;
  badgeKey?: NavigationBadgeKey;
  tabs?: readonly NavigationTab[];
}>;

export const DEFAULT_NAVIGATION_GROUP_ID: NavigationGroupId = "home";

export const NAVIGATION_GROUPS: readonly NavigationGroup[] = [
  {
    id: "home",
    label: { zhCN: uiCopy.home[0], enUS: uiCopy.home[1] },
    icon: LayoutDashboard
  },
  {
    id: "assets",
    label: { zhCN: uiCopy.assets[0], enUS: uiCopy.assets[1] },
    icon: Boxes,
    tabs: [
      { id: "skills", label: { zhCN: uiCopy.skills[0], enUS: uiCopy.skills[1] }, icon: Boxes },
      { id: "groups", label: { zhCN: uiCopy.groups[0], enUS: uiCopy.groups[1] }, icon: GitBranch }
    ]
  },
  {
    id: "security",
    label: { zhCN: uiCopy.security[0], enUS: uiCopy.security[1] },
    icon: ShieldCheck,
    badgeKey: "securityRiskCount"
  },
  {
    id: "activity",
    label: { zhCN: uiCopy.activity[0], enUS: uiCopy.activity[1] },
    icon: Activity,
    tabs: [
      { id: "updates", label: { zhCN: uiCopy.updates[0], enUS: uiCopy.updates[1] }, icon: RefreshCw },
      { id: "history", label: { zhCN: uiCopy.history[0], enUS: uiCopy.history[1] }, icon: History },
      { id: "quarantine", label: { zhCN: uiCopy.quarantine[0], enUS: uiCopy.quarantine[1] }, icon: ArchiveRestore },
      { id: "reports", label: { zhCN: uiCopy.reports[0], enUS: uiCopy.reports[1] }, icon: FileClock }
    ]
  },
  {
    id: "settings",
    label: { zhCN: uiCopy.settings[0], enUS: uiCopy.settings[1] },
    icon: Settings
  }
];

export function getNavigationGroup(id: string | null | undefined): NavigationGroup {
  return NAVIGATION_GROUPS.find(group => group.id === id) ?? NAVIGATION_GROUPS[0];
}

export function getSectionTabs(id: string | null | undefined): readonly NavigationTab[] {
  return getNavigationGroup(id).tabs ?? [];
}

export function getDefaultSectionTabId(
  id: string | null | undefined
): NavigationTabId | undefined {
  return getSectionTabs(id)[0]?.id;
}

export function getSectionTab(
  groupId: string | null | undefined,
  tabId: string | null | undefined
): NavigationTab | undefined {
  const tabs = getSectionTabs(groupId);
  return tabs.find(tab => tab.id === tabId) ?? tabs[0];
}
