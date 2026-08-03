import { useI18n } from "../i18n";
import { PanelLeftClose, PanelLeftOpen } from "lucide-react";
import {
  getNavigationGroup,
  NAVIGATION_GROUPS
} from "./navigation";
import type {
  NavigationBadgeKey,
  NavigationGroupId
} from "./navigation";

export type SidebarProps = {
  activeGroupId?: string | null;
  badges?: Partial<Record<NavigationBadgeKey, number>>;
  className?: string;
  collapsed?: boolean;
  onToggle?: () => void;
  onSelect: (groupId: NavigationGroupId) => void;
};

export function Sidebar({
  activeGroupId,
  badges = {},
  className,
  collapsed = false,
  onToggle,
  onSelect
}: SidebarProps) {
  const { t } = useI18n();
  const activeGroup = getNavigationGroup(activeGroupId);
  const sidebarClassName = ["sidebar", collapsed ? "collapsed" : "", className].filter(Boolean).join(" ");

  return (
    <aside className={sidebarClassName}>
      <div className="brand" title="Codex Skill Manager">
        <div className="brand-copy"><span>CODEX</span><strong>Skill Manager</strong></div>
        {onToggle && <button className="sidebar-toggle" type="button" onClick={onToggle}
          aria-label={collapsed ? t("展开侧栏", "Expand sidebar") : t("折叠侧栏", "Collapse sidebar")}
          title={collapsed ? t("展开侧栏", "Expand sidebar") : t("折叠侧栏", "Collapse sidebar")}>
          {collapsed ? <PanelLeftOpen size={17} aria-hidden="true" /> : <PanelLeftClose size={17} aria-hidden="true" />}
        </button>}
      </div>
      <nav aria-label={t("主导航", "Primary navigation")}>
        {NAVIGATION_GROUPS.map(group => {
          const Icon = group.icon;
          const badge = group.badgeKey ? badges[group.badgeKey] : undefined;
          const isActive = activeGroup.id === group.id;
          return (
            <button
              aria-current={isActive ? "page" : undefined}
              className={isActive ? "active" : ""}
              key={group.id}
              onClick={() => onSelect(group.id)}
              type="button"
            >
              <Icon aria-hidden="true" focusable="false" size={18} />
              <span className="sidebar-label">{t(group.label.zhCN, group.label.enUS)}</span>
              {typeof badge === "number" && badge > 0 ? (
                <em aria-label={t(`${badge} 个待处理风险`, `${badge} security risk${badge === 1 ? "" : "s"}`)}>
                  {badge > 99 ? "99+" : badge}
                </em>
              ) : null}
            </button>
          );
        })}
      </nav>
      <div className="sidebar-foot" role="status">
        <span aria-hidden="true" className="status-dot" />
        <div>
          <strong>{t("本地模式", "Local mode")}</strong>
          <small>{t("Codex 复核需单独启用", "Codex review is opt-in")}</small>
        </div>
      </div>
    </aside>
  );
}
