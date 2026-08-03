import { useI18n } from "../i18n";
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
  onSelect: (groupId: NavigationGroupId) => void;
};

export function Sidebar({
  activeGroupId,
  badges = {},
  className,
  onSelect
}: SidebarProps) {
  const { t } = useI18n();
  const activeGroup = getNavigationGroup(activeGroupId);
  const sidebarClassName = ["sidebar", className].filter(Boolean).join(" ");

  return (
    <aside className={sidebarClassName}>
      <div className="brand" title="Codex Skill Manager">
        <div className="brand-copy"><span>CODEX</span><strong>Skill Manager</strong></div>
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
              <span>{t(group.label.zhCN, group.label.enUS)}</span>
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
