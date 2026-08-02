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
    label: { zhCN: "首页", enUS: "Home" },
    icon: LayoutDashboard
  },
  {
    id: "assets",
    label: { zhCN: "资产", enUS: "Assets" },
    icon: Boxes,
    tabs: [
      { id: "skills", label: { zhCN: "Skills", enUS: "Skills" }, icon: Boxes },
      { id: "groups", label: { zhCN: "分组", enUS: "Groups" }, icon: GitBranch }
    ]
  },
  {
    id: "security",
    label: { zhCN: "安全", enUS: "Security" },
    icon: ShieldCheck,
    badgeKey: "securityRiskCount"
  },
  {
    id: "activity",
    label: { zhCN: "活动", enUS: "Activity" },
    icon: Activity,
    tabs: [
      { id: "updates", label: { zhCN: "更新", enUS: "Updates" }, icon: RefreshCw },
      { id: "history", label: { zhCN: "历史与回滚", enUS: "History & Rollback" }, icon: History },
      { id: "quarantine", label: { zhCN: "隔离区", enUS: "Quarantine" }, icon: ArchiveRestore },
      { id: "reports", label: { zhCN: "报告", enUS: "Reports" }, icon: FileClock }
    ]
  },
  {
    id: "settings",
    label: { zhCN: "设置", enUS: "Settings" },
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
