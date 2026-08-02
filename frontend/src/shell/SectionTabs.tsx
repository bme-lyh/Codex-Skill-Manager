import { useI18n } from "../i18n";
import {
  getNavigationGroup,
  getSectionTab,
  getSectionTabs
} from "./navigation";
import type { NavigationTabId } from "./navigation";

export type SectionTabsProps = {
  activeTabId?: string | null;
  className?: string;
  groupId?: string | null;
  onSelect: (tabId: NavigationTabId) => void;
};

export function SectionTabs({
  activeTabId,
  className,
  groupId,
  onSelect
}: SectionTabsProps) {
  const { t } = useI18n();
  const group = getNavigationGroup(groupId);
  const tabs = getSectionTabs(group.id);

  if (!tabs.length) return null;

  const activeTab = getSectionTab(group.id, activeTabId);
  const tabsClassName = ["tabs", "section-tabs", className].filter(Boolean).join(" ");

  return (
    <nav
      aria-label={t(`${group.label.zhCN}导航`, `${group.label.enUS} navigation`)}
      className={tabsClassName}
    >
      {tabs.map(tab => {
        const Icon = tab.icon;
        const isActive = activeTab?.id === tab.id;
        return (
          <button
            aria-current={isActive ? "page" : undefined}
            className={isActive ? "active" : ""}
            key={tab.id}
            onClick={() => onSelect(tab.id)}
            type="button"
          >
            <Icon aria-hidden="true" focusable="false" size={16} />
            <span>{t(tab.label.zhCN, tab.label.enUS)}</span>
          </button>
        );
      })}
    </nav>
  );
}
