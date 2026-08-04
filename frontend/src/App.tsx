import { useEffect, useRef, useState } from "react";
import {
  ArchiveRestore, CheckCircle2, ChevronRight, CircleAlert, Clock3,
  FolderGit2, Gauge, History,
  Link2, ListRestart, LoaderCircle, Pencil, Plus, RefreshCw, RotateCcw, Search,
  Settings, ShieldCheck, ShieldAlert, Trash2, X, GripVertical, CheckSquare2,
  ArrowUpCircle, KeyRound, Stethoscope, Sparkles, Languages, Sun, Moon, Monitor
} from "lucide-react";
import { api } from "./api";
import { isPackagedFullContextMode } from "./codexContext";
import { I18nProvider, normalizeLocale, translate, uiCopy, useI18n } from "./i18n";
import { InstallDialog } from "./install/InstallDialog";
import { SectionTabs } from "./shell/SectionTabs";
import { Sidebar } from "./shell/Sidebar";
import { matchesRoot, normalizeRootContract, rootKindLabel } from "./roots";
import type { RootContract } from "./roots";
import { Loading, OperationBanner } from "./shell/AppChrome";
import type { Operation } from "./shell/AppChrome";
import { applyTheme, normalizeTheme } from "./theme";
import type { AppTheme } from "./theme";
import type { NavigationGroupId, NavigationTabId } from "./shell/navigation";
import type { AppLocale, Translate } from "./i18n";
import type { AdoptionPreview, CodexCLIStatus, CodexReviewProgress, Dashboard, Finding, Group, InstallPreview, RiskCluster, ScanReport, Skill, UpdateStatus } from "./types";

type Page = "overview" | "skills" | "groups" | "updates" | "security" | "history" | "quarantine" | "reports" | "settings";
type RunOperation = <T>(label: string, task: () => Promise<T>, successDetail?: string) => Promise<T | undefined>;

const pageTitles: Record<Page, { zhCN: string; enUS: string }> = {
  overview: { zhCN: uiCopy.home[0], enUS: uiCopy.home[1] },
  skills: { zhCN: uiCopy.skills[0], enUS: uiCopy.skills[1] },
  groups: { zhCN: uiCopy.groups[0], enUS: uiCopy.groups[1] },
  updates: { zhCN: uiCopy.updates[0], enUS: uiCopy.updates[1] },
  security: { zhCN: uiCopy.security[0], enUS: uiCopy.security[1] },
  history: { zhCN: uiCopy.history[0], enUS: uiCopy.history[1] },
  quarantine: { zhCN: uiCopy.quarantine[0], enUS: uiCopy.quarantine[1] },
  reports: { zhCN: uiCopy.reports[0], enUS: uiCopy.reports[1] },
  settings: { zhCN: uiCopy.settings[0], enUS: uiCopy.settings[1] }
};

const pageGroups: Record<Page, NavigationGroupId> = {
  overview: "home", skills: "assets", groups: "assets", updates: "activity",
  security: "security", history: "activity", quarantine: "activity", reports: "activity", settings: "settings"
};

const defaultPages: Record<NavigationGroupId, Page> = {
  home: "overview", assets: "skills", security: "security", activity: "updates", settings: "settings"
};

const tabPages: Record<NavigationTabId, Page> = {
  skills: "skills", groups: "groups", updates: "updates", history: "history", quarantine: "quarantine", reports: "reports"
};

const tabByPage: Partial<Record<Page, NavigationTabId>> = {
  skills: "skills", groups: "groups", updates: "updates", history: "history", quarantine: "quarantine", reports: "reports"
};

function useCodexProgress() {
  const [progress, setProgress] = useState<CodexReviewProgress | null>(null);
  useEffect(() => api.onCodexReviewProgress(next => setProgress(current => {
    if (!current) return next;
    if (current.reviewId === next.reviewId && next.sequence <= current.sequence) return current;
    if (current.reviewId !== next.reviewId &&
      new Date(next.startedAt).getTime() < new Date(current.startedAt).getTime()) return current;
    return next;
  })), []);
  return { progress, clearProgress: () => setProgress(null) };
}

export default function App() {
  const [locale, setLocale] = useState<AppLocale>("zh-CN");
  const [theme, setTheme] = useState<AppTheme>("system");
  const [rootContract, setRootContract] = useState<{ roots: RootContract[]; defaultRootId: string }>({ roots: [], defaultRootId: "" });
  useEffect(() => {
    void api.config().then(cfg => {
      setLocale(normalizeLocale(cfg?.locale));
      setTheme(normalizeTheme(cfg?.theme));
      setRootContract(normalizeRootContract(cfg));
    }).catch(() => undefined);
  }, []);
  useEffect(() => { applyTheme(theme); }, [theme]);
  return <I18nProvider locale={locale}><AppShell locale={locale} setLocale={setLocale} theme={theme} setTheme={setTheme} rootContract={rootContract} /></I18nProvider>;
}

function AppShell({ locale, setLocale, theme, setTheme, rootContract }: {
  locale: AppLocale;
  setLocale: (locale: AppLocale) => void;
  theme: AppTheme;
  setTheme: (theme: AppTheme) => void;
  rootContract: { roots: RootContract[]; defaultRootId: string };
}) {
  const { t } = useI18n();
  const [page, setPage] = useState<Page>("overview");
  const [data, setData] = useState<Dashboard | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [installOpen, setInstallOpen] = useState(false);
  const [selected, setSelected] = useState<string[]>([]);
  const [operation, setOperation] = useState<Operation | null>(null);
  const operationInFlight = useRef<{ label: string } | null>(null);
  const dashboardRequest = useRef(0);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(() => {
    try { return window.localStorage.getItem("csm.sidebar.collapsed") === "1"; } catch { return false; }
  });
  const [rootFilter, setRootFilter] = useState("all");

  useEffect(() => {
    try { window.localStorage.setItem("csm.sidebar.collapsed", sidebarCollapsed ? "1" : "0"); } catch { /* optional preference */ }
  }, [sidebarCollapsed]);

  const loadDashboard = async (throwOnError: boolean) => {
    const requestId = ++dashboardRequest.current;
    setLoading(true);
    try {
      const next = await api.dashboard();
      if (requestId !== dashboardRequest.current) return;
      setData(next);
      setError("");
    } catch (e: any) {
      if (requestId !== dashboardRequest.current) return;
      setError(e?.message ?? String(e));
      if (throwOnError) throw e;
    } finally {
      if (requestId === dashboardRequest.current) setLoading(false);
    }
  };
  const refresh = () => loadDashboard(false);
  const refreshStrict = () => loadDashboard(true);

  useEffect(() => { void refresh(); }, []);

  const runOperation: RunOperation = async (label, task, successDetail) => {
    const active = operationInFlight.current;
    if (active) {
      setError(t(
        `已有操作正在进行：${active.label}。请等待完成后再开始其他操作。`,
        `Another operation is running: ${active.label}. Wait for it to finish before starting another.`
      ));
      return undefined;
    }
    operationInFlight.current = { label };
    setOperation({ label, detail: t("正在处理，请稍候…", "Working…"), status: "running" });
    try {
      const value = await task();
      setOperation({ label, detail: successDetail ?? t("操作已完成", "Completed"), status: "success" });
      window.setTimeout(() => setOperation(current => current?.label === label && current.status === "success" ? null : current), 2600);
      return value;
    } catch (e: any) {
      const message = e?.message ?? String(e);
      setError(message);
      setOperation({ label, detail: message, status: "error" });
      return undefined;
    } finally {
      if (operationInFlight.current?.label === label) operationInFlight.current = null;
    }
  };

  const activeGroupId = pageGroups[page];
  const activeTabId = tabByPage[page];
  const navigateToGroup = (groupId: NavigationGroupId) => setPage(defaultPages[groupId]);
  const navigateToTab = (tabId: NavigationTabId) => setPage(tabPages[tabId]);
  const title = t(pageTitles[page].zhCN, pageTitles[page].enUS);
  const roots: RootContract[] = data?.roots?.length ? data.roots : rootContract.roots;
  const defaultRootId = data?.defaultRootId || rootContract.defaultRootId || roots[0]?.rootId || "";
  return (
    <div className={`shell${sidebarCollapsed ? " sidebar-collapsed" : ""}`}>
      <Sidebar activeGroupId={activeGroupId} badges={{ securityRiskCount: data?.riskCount ?? 0 }}
        collapsed={sidebarCollapsed} onToggle={() => setSidebarCollapsed(value => !value)} onSelect={navigateToGroup} />

      <main>
        <header>
          <h1>{title}</h1>
          <div className="header-actions">
            {roots.length > 0 && <label className="root-filter">
              <select aria-label={t("按根目录筛选", "Filter by root")} value={rootFilter} onChange={event => setRootFilter(event.target.value)}>
                <option value="all">{t("全部根目录", "All roots")}</option>
                {roots.map(root => <option key={root.rootId} value={root.rootId}>{root.rootName}</option>)}
              </select>
            </label>}
            <button className="ghost" disabled={loading || operation?.status === "running"} onClick={() => void runOperation(t("刷新 Skills 清单", "Refresh Skills"), refresh, t("清单已刷新", "Skills refreshed"))}>
              <RefreshCw size={17} className={loading ? "spin" : ""} />{loading ? t("刷新中…", "Refreshing…") : t("刷新", "Refresh")}
            </button>
            <button className="primary" disabled={operation?.status === "running"} onClick={() => setInstallOpen(true)}><Plus size={17} />{t("添加项目", "Add project")}</button>
          </div>
        </header>
        {activeTabId && <div className="section-tabs-wrap">
          <SectionTabs groupId={activeGroupId} activeTabId={activeTabId} onSelect={navigateToTab} />
        </div>}
        {error && <div className="error-banner" role="alert"><CircleAlert size={18} aria-hidden="true" />{error}<button type="button" aria-label={t("关闭错误提示", "Dismiss error")} onClick={() => setError("")}>×</button></div>}
        {operation && <OperationBanner operation={operation} dismiss={() => setOperation(null)} />}
        {loading && !data ? <Loading /> : data ? (
          <div className="content">
            {page === "overview" && <Overview data={data} onNavigate={setPage} />}
            {page === "skills" && <SkillsPage data={data} selected={selected} setSelected={setSelected} refresh={refresh} runOperation={runOperation} rootFilter={rootFilter} roots={roots} />}
            {page === "groups" && <GroupsPage data={data} refresh={refresh} runOperation={runOperation} defaultRootId={defaultRootId} />}
            {page === "updates" && <UpdatesPage data={data} refresh={refresh} runOperation={runOperation} />}
            <div className="persistent-page" hidden={page !== "security"}>
              <SecurityPage data={data} refresh={refresh} runOperation={runOperation} rootFilter={rootFilter} defaultRootId={defaultRootId} />
            </div>
            {page === "history" && <HistoryPage data={data} refresh={refresh} runOperation={runOperation} />}
            {page === "quarantine" && <QuarantinePage refresh={refresh} runOperation={runOperation} rootId={rootFilter === "all" ? defaultRootId : rootFilter} />}
            {page === "reports" && <ReportsPage data={data} />}
            {page === "settings" && <SettingsPage locale={locale} setLocale={setLocale} theme={theme} setTheme={setTheme} refresh={refresh} runOperation={runOperation} />}
          </div>
        ) : null}
      </main>
      {installOpen && <InstallDialog close={() => setInstallOpen(false)} refresh={refreshStrict}
        roots={roots} defaultRootId={defaultRootId}
        openSettings={() => { setInstallOpen(false); navigateToGroup("settings"); }} />}
    </div>
  );
}

function Overview({ data, onNavigate }: { data: Dashboard; onNavigate: (p: Page) => void }) {
  const { t } = useI18n();
  const cards = [
    { label: t("已管理", "Managed"), value: data.managedCount, detail: t("已记录来源和版本", "Source and version recorded"), icon: CheckCircle2, tone: "teal" },
    { label: t("未管理", "Unmanaged"), value: data.unmanagedCount, detail: t("等待确认来源", "Source needs confirmation"), icon: Link2, tone: "amber" },
    { label: t("系统 Skills", "System Skills"), value: data.systemCount, detail: t("由 Codex 管理", "Managed by Codex"), icon: ShieldCheck, tone: "blue" },
    { label: t("待处理报告", "Open reports"), value: data.riskCount, detail: t("需要人工处理", "Needs review"), icon: ShieldAlert, tone: "red" }
  ];
  return <>
    <section className="hero">
      <div><h2>{t("Skills 管理", "Skill management")}</h2>
        <p>{t("查看来源、版本、安全状态和最近操作。", "View sources, versions, security status, and recent actions.")}</p>
      </div>
      <button onClick={() => onNavigate("updates")}><RefreshCw size={19} />{t("检查更新", "Check updates")}</button>
    </section>
    <div className="stats">
      {cards.map(c => { const Icon = c.icon; return <article key={c.label}>
        <div className={`icon ${c.tone}`}><Icon size={20} /></div>
        <div><span>{c.label}</span><strong>{c.value}</strong><small>{c.detail}</small></div>
      </article>; })}
    </div>
    <div className="grid-two">
      <section className="panel">
        <PanelHead title={t("分组", "Groups")} subtitle={t("按来源分组，也支持手动调整", "Organized by source, with manual adjustment")} action={t("查看分组", "View groups")} onClick={() => onNavigate("groups")} />
        <div className="group-list">{data.groups.slice(0, 5).map(g => <GroupRow key={g.id} group={g} />)}</div>
      </section>
      <section className="panel">
        <PanelHead title={t("最近操作", "Recent actions")} subtitle={t("安装、更新和回滚记录", "Install, update, and rollback history")} action={t("查看历史", "View history")} onClick={() => onNavigate("history")} />
        <Timeline data={data.recentHistory.slice(0, 5)} />
      </section>
    </div>
  </>;
}

function PanelHead({ title, subtitle, action, onClick }: { title: string; subtitle: string; action?: string; onClick?: () => void }) {
  return <div className="panel-head"><div><h3>{title}</h3><p>{subtitle}</p></div>
    {action && <button onClick={onClick}>{action}<ChevronRight size={15} /></button>}</div>;
}

function displayGroupName(name: string, locale: AppLocale): string {
  if (!name) return name;
  const exact: Record<string, string> = {
    "Codex 系统 Skills": "Codex System Skills",
    "系统 Skills": "System Skills",
    "未分组": "Ungrouped"
  };
  if (locale === "en-US" && exact[name]) return exact[name];
  if (locale === "en-US" && name.startsWith("本地 · ")) return `Local · ${name.slice(5)}`;
  return name;
}

function transactionTypeLabel(value: string, locale: AppLocale): string {
  const labels: Record<string, [string, string]> = {
    install: ["安装", "Install"], update: ["更新", "Update"], manage: ["管理", "Manage"], adopt: ["管理", "Manage"],
    quarantine: ["移至隔离区", "Quarantine"], rollback: ["回滚", "Rollback"], restore: ["恢复", "Restore"],
    "assisted-install": ["计划安装", "Planned installation"],
    "rollback-assisted-install": ["回滚计划安装", "Roll back planned installation"],
    "group-layout": ["调整分组布局", "Change group layout"], "group-create": ["新建分组", "Create group"],
    "group-rename": ["重命名分组", "Rename group"], "group-move": ["移动 Skill", "Move Skill"],
    "group-reorder": ["调整分组顺序", "Reorder groups"]
  };
  const label = labels[value];
  return label ? translate(locale, label[0], label[1]) : value;
}

function transactionStatusLabel(value: string, locale: AppLocale): string {
  const labels: Record<string, [string, string]> = {
    completed: ["已完成", "Completed"], failed: ["失败", "Failed"],
    running: ["进行中", "Running"], planned: ["已计划", "Planned"],
    partial: ["部分完成", "Partial"], cancelled: ["已取消", "Cancelled"],
    interrupted: ["已中断", "Interrupted"], "rolled-back": ["已回滚", "Rolled back"],
    "manual-pending": ["待人工处理", "Manual action pending"], skipped: ["已跳过", "Skipped"]
  };
  const label = labels[value];
  return label ? translate(locale, label[0], label[1]) : value;
}

function skillStatusLabel(value: string, locale: AppLocale): string {
  const labels: Record<string, [string, string]> = {
    "not-scanned": ["未扫描", "Not scanned"], "未扫描": ["未扫描", "Not scanned"],
    unknown: ["未知", "Unknown"], "未知": ["未知", "Unknown"], "安全": ["安全", "Safe"],
    "有新版本": ["有新版本", "Update available"], "最新": ["最新", "Up to date"],
    "本地来源": ["本地来源", "Local source"], "未管理": ["未管理", "Unmanaged"], "系统维护": ["系统维护", "System-managed"]
  };
  const matched = value.match(/^(\d+) 个待核查$/);
  if (matched) return translate(locale, value, `${matched[1]} pending review`);
  const label = labels[value];
  return label ? translate(locale, label[0], label[1]) : value;
}

function reasoningEffortLabel(value: string, t: Translate): string {
  const labels: Record<string, [string, string]> = {
    minimal: ["最低", "minimal"], low: ["低", "low"], medium: ["中", "medium"],
    high: ["高", "high"], xhigh: ["超高", "extra high"], max: ["最高", "max"], ultra: ["极高", "ultra"]
  };
  const label = labels[value];
  return label ? t(label[0], label[1]) : value;
}

const reasoningOrder = ["minimal", "low", "medium", "high", "xhigh", "max", "ultra"];

function availableReasoningLevels(models: CodexCLIStatus["models"], model: string, current: string) {
  const candidates = (models ?? []).filter(item => model === "default" || item.slug === model);
  const values = candidates.flatMap(item => item.reasoningLevels?.map(level => level.effort) ?? []);
  if (!values.length) values.push(current || "xhigh");
  if (current && !values.includes(current)) values.push(current);
  return [...new Set(values)].sort((a, b) => {
    const left = reasoningOrder.indexOf(a);
    const right = reasoningOrder.indexOf(b);
    return (left < 0 ? reasoningOrder.length : left) - (right < 0 ? reasoningOrder.length : right);
  });
}

function GroupRow({ group }: { group: Group }) {
  const { locale } = useI18n();
  return <div className="group-row"><div className="repo-icon"><FolderGit2 size={19} /></div>
    <div className="grow"><strong>{displayGroupName(group.name, locale)}</strong><span>{group.provider === "github" ? group.repository : group.provider}</span></div>
    <div className="skill-stack">{group.skillNames.slice(0, 3).map(n => <i key={n}>{n.slice(0, 1).toUpperCase()}</i>)}</div>
    <b>{group.skillNames.length}</b><small>Skills</small></div>;
}

function Timeline({ data }: { data: Dashboard["recentHistory"] }) {
  const { t, locale, formatDate, join } = useI18n();
  if (!data.length) return <Empty text={t("暂无最近操作", "No recent actions")} />;
  return <div className="timeline">{data.map(tx => <div key={tx.id}><span className={tx.status} />
    <div><strong>{transactionTypeLabel(tx.type, locale)} · {join(tx.targets.map(target => displayGroupName(target, locale))) || "-"}</strong><small>{formatDate(tx.startedAt)}</small></div>
    <em>{transactionStatusLabel(tx.status, locale)}</em></div>)}</div>;
}

function SkillsPage({ data, selected, setSelected, refresh, runOperation, rootFilter, roots }: {
  data: Dashboard;
  selected: string[];
  setSelected: (s: string[]) => void;
  refresh: () => Promise<void>;
  runOperation: RunOperation;
  rootFilter: string;
  roots: RootContract[];
}) {
  const { t, locale } = useI18n();
  const [query, setQuery] = useState("");
  const [working, setWorking] = useState(false);
  const [adoption, setAdoption] = useState<AdoptionPreview | null>(null);
  const filtered = data.skills.filter(s =>
    matchesRoot(s.rootId, rootFilter) &&
    (s.name + s.description + s.groupName + (s.rootName ?? "")).toLowerCase().includes(query.toLowerCase())
  );
  const selectable = filtered.filter(skill => !skill.system).map(skill => skill.name);
  const rootNameFor = (skill: Skill) => skill.rootName || roots.find(root => root.rootId === skill.rootId)?.rootName || skill.rootId || "";
  const rootBadgeFor = (skill: Skill) => {
    const root = roots.find(value => value.rootId === skill.rootId);
    return root ? `${rootNameFor(skill)} · ${rootKindLabel(root.rootKind, locale)}` : rootNameFor(skill);
  };
  const unmanagedSelected = selected.filter(name => data.skills.some(skill => skill.name === name && !skill.managed && !skill.system));
  const selectedRoots = Array.from(new Set(data.skills
    .filter(skill => selected.includes(skill.name) && !skill.system)
    .map(skill => skill.rootId || "codex-default")));
  const actionRootId = rootFilter !== "all" ? rootFilter : selectedRoots.length === 1 ? selectedRoots[0] : "";
  const toggle = (name: string) => setSelected(selected.includes(name) ? selected.filter(n => n !== name) : [...selected, name]);
  const selectAll = () => setSelected(Array.from(new Set([...selected, ...selectable])));
  const invert = () => setSelected(Array.from(new Set([
    ...selected.filter(name => !selectable.includes(name)),
    ...selectable.filter(name => !selected.includes(name))
  ])));
  const clear = () => setSelected([]);
  const remove = async () => {
    if (!selected.length || !confirm(t(
      `将 ${selected.length} 个 Skills 移动到隔离区？不会永久删除。`,
      `Move ${selected.length} Skill${selected.length === 1 ? "" : "s"} to quarantine? Nothing will be permanently deleted.`
    ))) return;
    setWorking(true);
    try {
      const result = await runOperation(t("移动 Skills 到隔离区", "Move Skills to quarantine"), () => api.quarantine(selected, actionRootId), t("已安全移入隔离区", "Moved to quarantine"));
      if (result) { setSelected([]); await refresh(); }
    } finally { setWorking(false); }
  };
  const analyzeAdoption = async () => {
    if (!unmanagedSelected.length) return;
    setWorking(true);
    try {
      const preview = await runOperation(t("分析未管理 Skills", "Analyze unmanaged Skills"), () => api.prepareAdoption(unmanagedSelected, actionRootId), t("分析完成，可以确认管理", "Analysis complete; review the management plan"));
      if (preview) setAdoption(preview);
    } finally { setWorking(false); }
  };
  const auditSelected = async () => {
    if (selected.length !== 1) return;
    setWorking(true);
    try {
      const report = await runOperation(t(`扫描 ${selected[0]}`, `Scan ${selected[0]}`), () => api.audit(selected[0], actionRootId), t("安全扫描已完成，可在安全中心查看", "Security scan complete; view it in Security"));
      if (report) await refresh();
    } finally { setWorking(false); }
  };
  return <><section className="panel full">
    <div className="toolbar skills-toolbar"><div className="search"><Search size={17} /><input value={query} onChange={e => setQuery(e.target.value)} placeholder={t("搜索名称、描述或分组…", "Search name, description, or group…")} /></div>
      <div className="selection-tools">
        <button className="ghost" onClick={selectAll} disabled={!selectable.length}><CheckSquare2 size={15} />{t("全选当前", "Select all")}</button>
        <button className="ghost" onClick={invert} disabled={!selectable.length}><ListRestart size={15} />{t("反选当前", "Invert selection")}</button>
        <button className="ghost" onClick={clear} disabled={!selected.length}><X size={15} />{t("清空", "Clear")}</button>
      </div>
      {selected.length > 0 && <div className="selection"><span>{t(`已选 ${selected.length}`, `${selected.length} selected`)}</span>
        {selected.length === 1 && <button className="ghost" onClick={auditSelected} disabled={working || !actionRootId}>
          <ShieldCheck size={16} />{t("扫描此 Skill", "Scan this Skill")}
        </button>}
        {unmanagedSelected.length > 0 && <button className="ghost adopt-button" onClick={analyzeAdoption} disabled={working || !actionRootId}>
          {working ? <LoaderCircle className="spin" size={16} /> : <ShieldCheck size={16} />}{t(`管理 ${unmanagedSelected.length} 个`, `Manage ${unmanagedSelected.length}`)}
        </button>}
        <button className="danger" onClick={remove} disabled={working || !actionRootId}><Trash2 size={16} />{working ? t("处理中…", "Working…") : t("移至隔离区", "Move to quarantine")}</button></div>}
    </div>
    <div className="table">
      <div className="tr th"><span /><span>Skill</span><span>{t("来源分组", "Source group")}</span><span>{t("状态", "Status")}</span><span>{t("版本", "Version")}</span></div>
      {filtered.map(skill => <div className="tr" key={skill.identity || `${skill.rootId ?? "default"}::${skill.name}`}>
        <input type="checkbox" disabled={skill.system} checked={selected.includes(skill.name)} onChange={() => toggle(skill.name)} />
        <DetailCell summary={<span className="skill-text"><strong>{skill.name}</strong><small>{skill.description}</small>
          {rootNameFor(skill) && <em className="root-badge">{rootBadgeFor(skill)}</em>}</span>}
          rows={[[t("名称", "Name"), skill.name], [t("根目录", "Root"), rootNameFor(skill) || t("默认", "Default")], [t("说明", "Description"), skill.description], [t("路径", "Path"), skill.path], [t("文件数量", "Files"), String(skill.files?.length ?? 0)]]} />
        <DetailCell summary={<span><b>{displayGroupName(skill.groupName, locale)}</b><small>{skill.sourceRepository || displayGroupName(skill.sourceGroupName, locale) || (skill.system ? "Codex" : t("本地", "Local"))}</small></span>}
          rows={[
            [t("当前分组", "Current group"), displayGroupName(skill.groupName, locale)],
            [t("真实来源分组", "Source group"), displayGroupName(skill.sourceGroupName, locale) || t("尚未识别", "Not identified")],
            [t("来源类型", "Source type"), skill.sourceProvider || "unknown"],
            [t("仓库", "Repository"), skill.sourceRepository || t("无", "None")],
            [t("仓库内路径", "Repository path"), skill.sourcePath || t("无", "None")],
            [t("识别依据", "Detection evidence"), skill.sourceEvidence || t("无", "None")],
            [t("识别置信度", "Confidence"), `${Math.round((skill.sourceConfidence || 0) * 100)}%`]
          ]} />
        <DetailCell summary={<Status skill={skill} />} rows={[
          [t("管理状态", "Management"), skill.system ? t("系统 Skill", "System Skill") : skill.managed ? t("已管理", "Managed") : t("未管理", "Unmanaged")],
          [t("本地修改", "Local changes"), skill.localModified ? t("检测到修改", "Modified") : t("未检测到修改", "No changes detected")],
          [t("安全状态", "Security"), skillStatusLabel(skill.securityStatus || "not-scanned", locale)],
          [t("更新状态", "Updates"), skillStatusLabel(skill.updateStatus || "unknown", locale)]
        ]} />
        <DetailCell summary={<code>{skill.installedCommit?.slice(0, 8) || "-"}</code>}
          rows={[[t("完整 Commit", "Full commit"), skill.installedCommit || t("尚未记录", "Not recorded")], [t("来源路径", "Source path"), skill.sourcePath || t("无", "None")]]} />
      </div>)}
    </div>
  </section>
    {adoption && <AdoptionDialog preview={adoption} close={() => setAdoption(null)} refresh={refresh}
      runOperation={runOperation} onCompleted={() => setSelected([])} />}
  </>;
}

function AdoptionDialog({ preview, close, refresh, runOperation, onCompleted }: {
  preview: AdoptionPreview; close: () => void; refresh: () => Promise<void>; runOperation: RunOperation; onCompleted: () => void;
}) {
  const { t, locale } = useI18n();
  const [selected, setSelected] = useState(preview.skills.map(skill => skill.name));
  const [working, setWorking] = useState(false);
  const [reviewing, setReviewing] = useState("");
  const [scan, setScan] = useState(preview.scan);
  const toggleCluster = async (cluster: RiskCluster) => {
		const decision = requestRiskDecision(cluster, !cluster.ignored, t);
		if (!decision) return;
    setReviewing(cluster.id);
    try {
			await api.setRiskClusterIgnored(cluster, !cluster.ignored, decision.reason, decision.confirmHighRisk);
			setScan(current => updateClusterState(current, cluster.id, !cluster.ignored, decision.reason));
    } finally {
      setReviewing("");
    }
  };
  const ignoreAll = async (clusters: RiskCluster[]) => {
		clusters = batchDismissibleClusters(clusters);
		if (!clusters.length) return;
    setReviewing("manual-batch");
    try {
      await api.setRiskClustersIgnored(clusters, true, "");
      setScan(current => updateClustersState(current, clusters, true, ""));
    } finally {
      setReviewing("");
    }
  };
  const apply = async () => {
    setWorking(true);
    try {
      const result = await runOperation(t("管理现有 Skills", "Manage existing Skills"),
        () => api.applyAdoption(preview.id, selected, preview.targetRootId || preview.skills[0]?.rootId || "codex-default"),
        t("已完成管理；失败项可单独重试", "Management finished; failed items can be retried"));
      if (result) { onCompleted(); await refresh(); close(); }
    } finally { setWorking(false); }
  };
  return <div className="modal-backdrop"><div className="modal adoption-modal">
    <div className="modal-head"><h2>{t("管理现有 Skills", "Manage existing Skills")}</h2><button onClick={close}><X /></button></div>
    <div className="notice"><ShieldCheck size={20} /><span><strong>{t("记录来源和分组", "Record sources and groups")}</strong>
      <small>{t("不会移动文件；无法识别 GitHub 来源时会创建本地分组。", "Files are not moved. A local group is created when no GitHub source can be identified.")}</small></span></div>
    <div className="candidate-list">{preview.skills.map(skill => <label key={skill.name}>
      <input type="checkbox" checked={selected.includes(skill.name)}
        onChange={() => setSelected(selected.includes(skill.name) ? selected.filter(name => name !== skill.name) : [...selected, skill.name])} />
      <span><strong>{skill.name}</strong><small>{skill.description}</small>
        {(() => { const source = preview.sources?.find(item => item.skillName === skill.name); return source ? <>
          <small className="source-detected">{t("自动分组", "Detected group")}：{displayGroupName(source.groupName, locale)} · {t("置信度", "Confidence")} {Math.round(source.confidence * 100)}%</small>
          <code>{source.repository || source.sourcePath}</code><small>{source.evidence}</small>
        </> : <code>{skill.path}</code>; })()}
      </span>
    </label>)}</div>
    <ScanSummary report={scan} compact />
    <FindingDetails report={scan} reviewing={reviewing} onToggle={toggleCluster} onIgnoreAll={ignoreAll} />
    <div className="modal-actions"><button className="ghost" onClick={close}>{t("取消", "Cancel")}</button>
      <button className="primary" disabled={working || reviewing !== "" || selected.length === 0} onClick={apply}>
        {working ? <LoaderCircle className="spin" size={17} /> : <CheckCircle2 size={17} />}{working ? t("正在管理…", "Managing…") : t("确认管理", "Confirm")}
      </button></div>
  </div></div>;
}

function Status({ skill }: { skill: Skill }) {
  const { t } = useI18n();
  if (skill.system) return <span className="badge blue">{t("系统", "System")}</span>;
  if (skill.localModified) return <span className="badge amber">{t("本地修改", "Modified")}</span>;
  if (!skill.managed) return <span className="badge gray">{t("未管理", "Unmanaged")}</span>;
  return <span className="badge green">{t("受保护", "Protected")}</span>;
}

function DetailCell({ summary, rows }: { summary: React.ReactNode; rows: Array<[string, string]> }) {
  return <div className="detail-cell" tabIndex={0}>
    {summary}
    <div className="detail-popover" role="tooltip">
      {rows.map(([label, value]) => <div key={label}><b>{label}</b><span>{value || "-"}</span></div>)}
    </div>
  </div>;
}

function GroupsPage({ data, refresh, runOperation, defaultRootId }: { data: Dashboard; refresh: () => Promise<void>; runOperation: RunOperation; defaultRootId: string }) {
  const { t, locale } = useI18n();
  const [active, setActive] = useState(data.groups[0]?.id ?? "");
  const [working, setWorking] = useState(false);
  const group = data.groups.find(g => g.id === active) ?? data.groups[0];
  useEffect(() => {
    if (!data.groups.some(item => item.id === active)) setActive(data.groups[0]?.id ?? "");
  }, [data.groups, active]);
  const create = async () => {
    const name = window.prompt(t("请输入新分组名称：", "Enter a new group name:"), "")?.trim();
    if (!name) return;
    setWorking(true);
    try {
      const result = await runOperation(t("新建管理分组", "Create group"), () => api.createGroup(name, group?.rootId || defaultRootId), t(`分组“${name}”已创建`, `Group “${name}” created`));
      if (result) await refresh();
    } finally { setWorking(false); }
  };
  const rename = async (target: Group) => {
    const name = window.prompt(t("请输入新的分组名称：", "Enter the new group name:"), target.name)?.trim();
    if (!name || name === target.name) return;
    setWorking(true);
    try {
      const result = await runOperation(t("重命名管理分组", "Rename group"), () => api.renameGroup(target.id, name, target.rootId || defaultRootId), t(`分组已改名为“${name}”`, `Group renamed to “${name}”`));
      if (result) await refresh();
    } finally { setWorking(false); }
  };
  const dropOnGroup = async (event: React.DragEvent, target: Group) => {
    event.preventDefault();
    const skillName = event.dataTransfer.getData("application/x-csm-skill");
    if (skillName) {
      const sourceRootId = event.dataTransfer.getData("application/x-csm-root");
      if (target.readOnly || !sourceRootId || sourceRootId !== (target.rootId || defaultRootId)) return;
      setWorking(true);
      try {
        const result = await runOperation(t("移动 Skill 到分组", "Move Skill to group"), () => api.moveSkills([skillName], target.id, target.rootId || defaultRootId), t(`${skillName} 已移入“${target.name}”`, `${skillName} moved to “${target.name}”`));
        if (result) { setActive(target.id); await refresh(); }
      } finally { setWorking(false); }
      return;
    }
    const draggedID = event.dataTransfer.getData("application/x-csm-group");
    if (!draggedID || draggedID === target.id || target.readOnly) return;
    const targetRootId = target.rootId || defaultRootId;
    const editable = data.groups.filter(item => !item.readOnly && (item.rootId || defaultRootId) === targetRootId);
    const from = editable.findIndex(item => item.id === draggedID);
    const to = editable.findIndex(item => item.id === target.id);
    if (from < 0 || to < 0) return;
    const reordered = [...editable];
    const [moved] = reordered.splice(from, 1);
    reordered.splice(to, 0, moved);
    setWorking(true);
    try {
      const result = await runOperation(t("调整分组顺序", "Reorder groups"), () => api.reorderGroups(reordered.map(item => item.id), targetRootId), t("分组顺序已保存", "Group order saved"));
      if (result) await refresh();
    } finally { setWorking(false); }
  };
  return <div className="groups-layout">
    <section className="panel group-nav">
      <div className="group-nav-head"><div><h3>{t("分组", "Groups")}</h3><p>{t("拖动排序或移动 Skill", "Drag to reorder groups or move Skills")}</p></div>
        <button className="icon-button" title={t("新建分组", "Create group")} disabled={working} onClick={create}><Plus size={17} /></button></div>
      {data.groups.map(g => <div key={g.id} className={`group-nav-item ${active === g.id ? "active" : ""}`}
        draggable={!g.readOnly && !working}
        onDragStart={event => event.dataTransfer.setData("application/x-csm-group", g.id)}
        onDragOver={event => { if (!g.readOnly) event.preventDefault(); }}
        onDrop={event => void dropOnGroup(event, g)}>
        {!g.readOnly ? <GripVertical className="drag-handle" size={16} /> : <ShieldCheck size={16} />}
        <button className="group-select" onClick={() => setActive(g.id)}>
          <span><strong>{displayGroupName(g.name, locale)}</strong><small>{g.skillNames.length} Skills · {g.manual ? t("手动分组", "Manual group") : t("来源分组", "Source group")}</small></span>
        </button>
        {!g.readOnly && <button className="group-rename" title={t("重命名", "Rename")} onClick={() => void rename(g)}><Pencil size={14} /></button>}
      </div>)}
    </section>
    <section className="panel relation-panel"><PanelHead title={group ? displayGroupName(group.name, locale) : t("分组详情", "Group details")} subtitle={t("拖动 Skill 可更换分组", "Drag a Skill to change its group")} />
      {group ? <>
        <div className="group-skill-list">{group.skillNames.length ? group.skillNames.map(name => {
          const skill = data.skills.find(item => item.name === name && (item.rootId || defaultRootId) === (group.rootId || defaultRootId));
          return <article key={name} draggable={!skill?.system && !working}
            onDragStart={event => {
              event.dataTransfer.setData("application/x-csm-skill", name);
              event.dataTransfer.setData("application/x-csm-root", skill?.rootId || defaultRootId);
            }}>
            <GripVertical size={15} /><div><strong>{name}</strong><small>{skill?.description || t("暂无说明", "No description")}</small></div>
            <span>{skill?.sourceGroupName && skill.sourceGroupName !== group.name ? `${t("来源", "Source")}: ${displayGroupName(skill.sourceGroupName, locale)}` : skill?.sourceProvider}</span>
          </article>;
        }) : <Empty text={t("这个分组暂时为空，可将 Skill 拖入", "This group is empty. Drag a Skill here.")} />}</div>
      </> : <Empty text={t("暂无分组", "No groups")} />}
    </section>
  </div>;
}

function UpdatesPage({ data, refresh, runOperation }: { data: Dashboard; refresh: () => Promise<void>; runOperation: RunOperation }) {
  const { t, locale, formatDate, join } = useI18n();
  const [statuses, setStatuses] = useState<UpdateStatus[]>(data.updateStatuses);
  const [working, setWorking] = useState(false);
  const [selectedGroups, setSelectedGroups] = useState<string[]>([]);
  const [previews, setPreviews] = useState<Array<{ group: Group; value: InstallPreview }>>([]);
  const [prepareFailures, setPrepareFailures] = useState<string[]>([]);
  useEffect(() => { setStatuses(data.updateStatuses); }, [data.updateStatuses]);
  const [clock, setClock] = useState(Date.now());
  useEffect(() => {
    const timer = window.setInterval(() => setClock(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, []);
  const check = async () => {
    setWorking(true);
    try {
      const checked = await runOperation(t("检查 GitHub 更新", "Check GitHub updates"), api.check, t("更新检查已完成", "Update check complete"));
      if (checked) { setStatuses(checked.statuses); await refresh(); }
    } finally { setWorking(false); }
  };
  const retry = async (group: Group) => {
    setWorking(true);
    try {
      const checked = await runOperation(
        t("重试 GitHub 更新检查", "Retry GitHub update check"),
        () => api.checkSelected([groupIdentity(group)], true),
        t("该来源已重新检查", "Source checked again")
      );
      if (checked) { setStatuses(checked.statuses); await refresh(); }
    } finally { setWorking(false); }
  };
  const linkRemote = async (group: Group) => {
    const skillName = group.skillNames[0];
    if (!skillName) return;
    const url = window.prompt(t("输入该 Skill 的 GitHub 仓库地址", "Enter the GitHub repository URL for this Skill"), "https://github.com/");
    if (!url?.trim() || url.trim() === "https://github.com/") return;
    const ref = window.prompt(t("输入分支或标签（可留空）", "Enter a branch or tag (optional)"), "main") ?? "";
    setWorking(true);
    try {
      const linked = await runOperation(
        t("关联远程来源", "Link remote source"),
        () => api.linkLocalSource(skillName, url.trim(), ref.trim(), group.rootId || "codex-default"),
        t("来源已关联；现在可以检查更新", "Source linked; updates can now be checked")
      );
      if (linked) await refresh();
    } finally { setWorking(false); }
  };
  const prepare = async (groups: Group[]) => {
    if (!groups.length) return;
    setWorking(true);
    try {
      const outcome = await runOperation(t(`准备 ${groups.length} 个来源的更新计划`, `Prepare update plans for ${groups.length} source${groups.length === 1 ? "" : "s"}`), async () => {
        const ready: Array<{ group: Group; value: InstallPreview }> = [];
        const failures: string[] = [];
        for (const group of groups) {
          try {
            ready.push({ group, value: await api.prepareUpdate(group.id, group.rootId || "codex-default") });
          } catch (error: any) {
            failures.push(`${group.name}：${error?.message ?? String(error)}`);
          }
        }
        return { ready, failures };
      }, t("更新范围已核对，安全扫描已完成", "Update scope checked and security scan complete"));
      if (outcome) {
        setPrepareFailures(outcome.failures);
        setPreviews(outcome.ready);
      }
    } finally { setWorking(false); }
  };
  const statusByGroup = new Map(statuses.map(status => [statusIdentity(status), status]));
  const statusForGroup = (group: Group) => statusByGroup.get(groupIdentity(group)) ?? statusByGroup.get(group.id);
  const updateGroups = data.sourceGroups.filter(group => group.provider !== "system");
  const availableGroups = updateGroups.filter(group => {
    const status = statusForGroup(group);
    return status?.status === "update-available" ||
      ((status?.status === "error" || status?.status === "rate-limited") && status.lastSuccessStatus === "update-available");
  });
  const selectableIds = availableGroups.map(groupIdentity);
  const selectAll = () => setSelectedGroups(selectableIds);
  const invert = () => setSelectedGroups(selectableIds.filter(id => !selectedGroups.includes(id)));
  const clear = () => setSelectedGroups([]);
  const selectedAvailableGroups = availableGroups.filter(group => selectedGroups.includes(groupIdentity(group)));
  const lastChecked = statuses.reduce<string | undefined>((latest, status) =>
    !latest || new Date(status.checkedAt) > new Date(latest) ? status.checkedAt : latest, data.lastUpdateCheck);
  return <section className="panel full">
    <div className="update-hero"><div className="round-icon"><RefreshCw size={28} /></div><div><h2>{t("检查更新", "Check for updates")}</h2>
      <p>{t("检查版本并选择要更新的 Skills。更新前会显示计划和安全结果。", "Check versions and choose which Skills to update. The plan and security results appear before changes are applied.")}</p>
      <small>{lastChecked ? t(`上次检查：${formatDate(lastChecked)}`, `Last checked: ${formatDate(lastChecked)}`) : t("尚未执行过更新检查", "No update check has been run")}</small></div>
      <button className="primary" onClick={check} disabled={working}>{working ? <LoaderCircle className="spin" size={17} /> : <RefreshCw size={17} />}{working ? t("正在检查…", "Checking…") : t("检查更新", "Check updates")}</button></div>
    {availableGroups.length > 0 && <div className="update-toolbar">
      <div><strong>{t("选择更新来源", "Select update sources")}</strong><span>{t("仅显示有新版本的来源；每个来源可单独回滚。", "Only sources with updates are selectable. Each source can be rolled back separately.")}</span></div>
      <div className="selection-tools">
        <button className="ghost" onClick={selectAll}><CheckSquare2 size={15} />{t("全选可更新", "Select all")}</button>
        <button className="ghost" onClick={invert}><ListRestart size={15} />{t("反选", "Invert")}</button>
        <button className="ghost" onClick={clear} disabled={!selectedGroups.length}><X size={15} />{t("清空", "Clear")}</button>
        <button className="primary" disabled={working || !selectedAvailableGroups.length} onClick={() => void prepare(selectedAvailableGroups)}>
          {working ? <LoaderCircle className="spin" size={16} /> : <ArrowUpCircle size={16} />}
          {t(`检查所选 ${selectedAvailableGroups.length} 个来源`, `Check ${selectedAvailableGroups.length} selected source${selectedAvailableGroups.length === 1 ? "" : "s"}`)}
        </button>
      </div>
    </div>}
    {prepareFailures.length > 0 && <div className="prepare-failures"><CircleAlert size={17} /><div><strong>{t("部分更新无法准备", "Some updates could not be prepared")}</strong>
      {prepareFailures.map(message => <small key={message}>{message}</small>)}</div><button onClick={() => setPrepareFailures([])}><X size={15} /></button></div>}
    <div className="update-list">{updateGroups.length === 0 ? <Empty text={t("暂无可检查的个人 Skill 来源", "No personal Skill sources can be checked")} /> : updateGroups.map(g => {
      const status = statusForGroup(g);
      const identity = groupIdentity(g);
      const presentation = updatePresentation(status, t);
      return <article key={identity} className={`update-item ${presentation.tone}`}>
        <input className="update-check" type="checkbox" disabled={!availableGroups.some(group => groupIdentity(group) === identity) || working}
          checked={selectedGroups.includes(identity)}
          onChange={() => setSelectedGroups(selectedGroups.includes(identity) ? selectedGroups.filter(id => id !== identity) : [...selectedGroups, identity])} />
        <div className={`update-state-icon ${presentation.tone}`}>{presentation.icon}</div>
        <div className="update-copy"><strong>{displayGroupName(g.name, locale)}</strong><span>{g.skillNames.length} Skills · {g.repository || g.provider}</span>
          <small>{status ? updateDetail(status, t, formatDate, join) : t("点击“检查更新”获取当前状态", "Select “Check updates” to get the current status")}</small>
          {status?.error && <small className="update-error">{status.error}</small>}
          {status?.status === "rate-limited" && status.retryAt && <small className="rate-limit-countdown">
            {t("GitHub 限额恢复倒计时", "GitHub rate limit resets in")}: {formatCountdown(new Date(status.retryAt).getTime() - clock, locale)}
          </small>}
        </div>
        <div className="update-actions"><span className={`badge ${presentation.badge}`}>{presentation.label}</span>
          {status?.status === "update-available" && <button className="ghost compact" disabled={working} onClick={() => void prepare([g])}>
            <ShieldCheck size={15} />{t("单独复核", "Review separately")}
          </button>}
          {status?.status === "unsupported" && g.skillNames.length > 0 &&
            <button className="ghost compact" disabled={working} onClick={() => void linkRemote(g)}>
              <Link2 size={15} />{t("关联远程来源", "Link source")}
            </button>}
          {(status?.status === "error" || status?.status === "rate-limited") &&
            <button className="ghost compact" disabled={working}
              onClick={() => void retry(g)}>
              <RefreshCw size={15} />{t("重新检查", "Check again")}
            </button>}
        </div>
      </article>;
    })}</div>
    {previews.length > 0 && <UpdateDialog items={previews} close={() => setPreviews([])}
      refresh={refresh} />}
  </section>;
}

function groupIdentity(group: Pick<Group, "id" | "rootId">): string {
  return `${group.rootId ?? ""}\x00${group.id}`;
}

function statusIdentity(status: Pick<UpdateStatus, "groupId" | "rootId">): string {
  let rootId = status.rootId ?? "";
  let groupId = status.groupId;
  const separator = groupId.indexOf("\x00");
  if (separator >= 0) {
    if (!rootId) rootId = groupId.slice(0, separator);
    groupId = groupId.slice(separator + 1);
  }
  return `${rootId}\x00${groupId}`;
}

function updatePresentation(status: UpdateStatus | undefined, t: Translate) {
  if (!status) return { label: t("尚未检查", "Not checked"), tone: "unknown", badge: "gray", icon: <Clock3 size={19} /> };
  if (status.status === "up-to-date") return { label: t("已是最新", "Up to date"), tone: "current", badge: "green", icon: <CheckCircle2 size={19} /> };
  if (status.status === "update-available") return { label: t("发现新版本", "Update available"), tone: "available", badge: "amber", icon: <ArrowUpCircle size={19} /> };
  if (status.status === "error") return { label: t("检查失败", "Check failed"), tone: "failed", badge: "red", icon: <CircleAlert size={19} /> };
  if (status.status === "rate-limited") return { label: t("GitHub 已限流", "GitHub rate limit reached"), tone: "failed", badge: "red", icon: <Clock3 size={19} /> };
  return { label: t("不支持在线更新", "Online updates unavailable"), tone: "unsupported", badge: "gray", icon: <Link2 size={19} /> };
}

function updateDetail(status: UpdateStatus, t: Translate, formatDate: (value: string | number | Date) => string, join: (values: string[]) => string) {
  const checked = formatDate(status.checkedAt);
  if (status.status === "update-available") {
    return t(`${status.outdatedSkills.length} 个 Skill 可更新：${join(status.outdatedSkills)} · 检查于 ${checked}`,
      `${status.outdatedSkills.length} Skill${status.outdatedSkills.length === 1 ? "" : "s"} can be updated: ${join(status.outdatedSkills)} · Checked ${checked}`);
  }
  if (status.status === "up-to-date") {
    return t(`本地与远端一致 · Commit ${status.remoteCommit?.slice(0, 10) || "未知"} · 检查于 ${checked}`,
      `Local and remote match · Commit ${status.remoteCommit?.slice(0, 10) || "unknown"} · Checked ${checked}`);
  }
  if (status.status === "unsupported") return t(`本地来源尚未关联 GitHub；可点击“关联远程来源” · 检查于 ${checked}`, `Local source is not linked to GitHub; use “Link source” · Checked ${checked}`);
  if (status.status === "rate-limited") {
    const previous = status.lastSuccessStatus === "update-available" ? t("上次成功检查发现新版本，仍可选中", "The last successful check found an update; it remains selectable") :
      status.lastSuccessStatus === "up-to-date" ? t("上次成功检查时已是最新", "The last successful check was up to date") : t("没有可用的历史成功状态", "No previous successful status");
    return t(`${previous} · 本次检查于 ${checked}`, `${previous} · Checked ${checked}`);
  }
  if (status.status === "error" && status.lastSuccessAt) {
    return t(`本次检查失败；保留 ${formatDate(status.lastSuccessAt)} 的成功状态`, `This check failed; keeping the successful status from ${formatDate(status.lastSuccessAt)}`);
  }
  return t(`未能取得远端状态 · 检查于 ${checked}`, `Remote status unavailable · Checked ${checked}`);
}

function formatCountdown(milliseconds: number, locale: AppLocale): string {
  if (milliseconds <= 0) return translate(locale, "可以重试", "ready to retry");
  const seconds = Math.ceil(milliseconds / 1000);
  const minutes = Math.floor(seconds / 60);
  const remainder = seconds % 60;
  return translate(locale, `${minutes}分${remainder.toString().padStart(2, "0")}秒`, `${minutes}m ${remainder.toString().padStart(2, "0")}s`);
}

function UpdateDialog({ items, close, refresh }: {
  items: Array<{ group: Group; value: InstallPreview }>; close: () => void; refresh: () => Promise<void>;
}) {
  const { t, locale } = useI18n();
  const [selected, setSelected] = useState<Record<string, string[]>>(() => Object.fromEntries(items.map(({ value }) => [
    value.id, value.skills.map(skill => skill.name)
  ])));
  const [scans, setScans] = useState<Record<string, ScanReport>>(() =>
    Object.fromEntries(items.map(({ value }) => [value.id, value.scan])));
  const [working, setWorking] = useState(false);
  const [reviewing, setReviewing] = useState("");
  const [codexWorking, setCodexWorking] = useState(false);
  const [progress, setProgress] = useState("");
  const [failures, setFailures] = useState<string[]>([]);
  const { progress: codexProgress, clearProgress } = useCodexProgress();
  const selectedCount = Object.values(selected).reduce((sum, names) => sum + names.length, 0);
  const hasBlockingWarnings = items.some(({ value }) =>
    ["critical", "high"].includes(scans[value.id].activeHighestSeverity) && (selected[value.id]?.length ?? 0) > 0);
  const toggleCluster = async (planID: string, cluster: RiskCluster) => {
		const decision = requestRiskDecision(cluster, !cluster.ignored, t);
		if (!decision) return;
    setReviewing(cluster.id);
    try {
			await api.setRiskClusterIgnored(cluster, !cluster.ignored, decision.reason, decision.confirmHighRisk);
      setScans(current => ({
        ...current,
				[planID]: updateClusterState(current[planID], cluster.id, !cluster.ignored, decision.reason)
      }));
    } catch (error: any) {
      setFailures(current => [`${t("风险核查记录失败", "Failed to record risk review")}: ${error?.message ?? String(error)}`, ...current]);
    } finally {
      setReviewing("");
    }
  };
  const ignoreAll = async (planID: string, clusters: RiskCluster[]) => {
		clusters = batchDismissibleClusters(clusters);
		if (!clusters.length) return;
    setReviewing("manual-batch");
    try {
      await api.setRiskClustersIgnored(clusters, true, "");
      setScans(current => ({ ...current, [planID]: updateClustersState(current[planID], clusters, true, "") }));
    } catch (error: any) {
      setFailures(current => [`${t("一键忽略失败", "Ignore-all failed")}: ${error?.message ?? String(error)}`, ...current]);
    } finally {
      setReviewing("");
    }
  };
  const codexReview = async (planID: string) => {
    setCodexWorking(true);
    clearProgress();
    try {
      const reviewed = await api.reviewWithCodex(scans[planID], selected[planID] ?? []);
      setScans(current => ({ ...current, [planID]: reviewed }));
    } catch (error: any) {
      setFailures(current => [`${t("Codex 风险复核失败", "Codex risk review failed")}: ${error?.message ?? String(error)}`, ...current]);
    } finally {
      setCodexWorking(false);
    }
  };
  const applyCodexSuggestions = async (planID: string, clusters: RiskCluster[]) => {
    const scan = scans[planID];
    if (!confirmCodexSuggestions(scan, clusters)) return;
    setReviewing("codex-batch");
    try {
      const reason = codexBatchReason(scan, clusters, locale);
      await api.setRiskClustersIgnored(clusters, true, reason);
      setScans(current => ({ ...current, [planID]: updateClustersState(scan, clusters, true, reason) }));
    } catch (error: any) {
      setFailures(current => [`${t("Codex 建议采纳失败", "Failed to apply Codex suggestions")}: ${error?.message ?? String(error)}`, ...current]);
    } finally {
      setReviewing("");
    }
  };
  const apply = async () => {
    if (!selectedCount) return;
    setWorking(true);
    setFailures([]);
    try {
      const errors: string[] = [];
      const succeeded: string[] = [];
      const targets = items.filter(item => (selected[item.value.id]?.length ?? 0) > 0);
      let attempted = 0;
      for (const { group, value } of targets) {
        attempted++;
        setProgress(t(`正在更新 ${displayGroupName(group.name, locale)}（${attempted}/${targets.length}）`,
          `Updating ${displayGroupName(group.name, locale)} (${attempted}/${targets.length})`));
        try {
					await api.apply(value.id, selected[value.id],
						(scans[value.id].clusters ?? []).some(cluster => cluster.severity === "high" && cluster.ignored),
						value.targetRootId || group.rootId || "codex-default");
          succeeded.push(value.id);
        } catch (error: any) {
          errors.push(`${displayGroupName(group.name, locale)}: ${error?.message ?? String(error)}`);
        }
      }
      if (succeeded.length) {
        setSelected(current => ({ ...current, ...Object.fromEntries(succeeded.map(id => [id, []])) }));
      }
      setProgress(t("正在重新核对更新状态…", "Rechecking update status…"));
      await api.check();
      await refresh();
      setFailures(errors);
      if (!errors.length) close();
    } catch (error: any) {
      setFailures([t(`状态核对失败：${error?.message ?? String(error)}。已完成的操作可在“历史与回滚”中确认。`,
        `Status check failed: ${error?.message ?? String(error)}. Completed operations remain available in History & Rollback.`)]);
    } finally { setWorking(false); setProgress(""); }
  };
  return <div className="modal-backdrop"><div className="modal update-modal batch-update-modal">
    <div className="modal-head"><div><h2>{t("选择要更新的 Skills", "Choose Skills to update")}</h2>
      <small>{t(`${items.length} 个来源 · 已选择 ${selectedCount} 个 Skills`,
        `${items.length} source${items.length === 1 ? "" : "s"} · ${selectedCount} Skills selected`)}</small></div><button onClick={close} disabled={working}><X /></button></div>
    <div className="update-plan-list">{items.map(({ group, value }) => {
      const names = selected[value.id] ?? [];
      const scan = scans[value.id];
      const blocking = ["critical", "high"].includes(scan.activeHighestSeverity);
      return <section className="update-plan" key={value.id}>
        <div className="repo-summary update-repo"><FolderGit2 size={24} /><div><strong>{displayGroupName(group.name, locale)}</strong>
          <span>{value.repository.resolvedRef} · Commit {value.repository.commitSha.slice(0, 12)} · {t(`仅扫描本次写入的 ${scan.filesScanned} 个文件`, `Scanned only the ${scan.filesScanned} files to be written`)}</span></div>
          <span className={`severity ${scan.activeHighestSeverity}`}>{severityLabel(scan.activeHighestSeverity, locale)}</span></div>
        <div className="candidate-tools">
          <button onClick={() => setSelected({ ...selected, [value.id]: value.skills.map(skill => skill.name) })}>{t("全选", "Select all")}</button>
          <button onClick={() => setSelected({ ...selected, [value.id]: value.skills.filter(skill => !names.includes(skill.name)).map(skill => skill.name) })}>{t("反选", "Invert")}</button>
          <button onClick={() => setSelected({ ...selected, [value.id]: [] })} disabled={!names.length}>{t("清空", "Clear")}</button>
        </div>
        <div className="candidate-list update-candidates">{value.skills.map(skill => <label key={skill.name}>
          <input type="checkbox" checked={names.includes(skill.name)}
            onChange={() => setSelected({ ...selected, [value.id]: names.includes(skill.name) ? names.filter(name => name !== skill.name) : [...names, skill.name] })} />
          <span><strong>{skill.name}</strong><small>{skill.description}</small><code>{skill.sourcePath}</code></span>
        </label>)}</div>
        <ScanSummary report={scan} compact />
        <FindingDetails report={scan} reviewing={reviewing} onToggle={cluster => toggleCluster(value.id, cluster)}
          onCodexReview={() => codexReview(value.id)} codexWorking={codexWorking}
          codexProgress={codexProgress?.reportId === scan.id ? codexProgress : null}
          onApplyCodexSuggestions={clusters => applyCodexSuggestions(value.id, clusters)}
          onIgnoreAll={clusters => ignoreAll(value.id, clusters)} />
        {blocking && <div className="error-banner inline"><CircleAlert size={17} />
          {t("仍有 High 或 Critical 风险。High 必须逐项确认，Critical 必须修复。", "High or Critical findings remain. Confirm High individually and fix Critical findings.")}</div>}
      </section>;
    })}</div>
    {progress && <div className="batch-progress"><LoaderCircle className="spin" size={17} /><span>{progress}</span></div>}
    {failures.length > 0 && <div className="prepare-failures"><CircleAlert size={17} /><div><strong>{t("部分来源更新失败", "Some sources failed to update")}</strong>
      {failures.map(message => <small key={message}>{message}</small>)}</div></div>}
    <div className="modal-actions"><button className="ghost" onClick={close}>{t("取消", "Cancel")}</button>
      <button className="primary" disabled={working || reviewing !== "" || hasBlockingWarnings || selectedCount === 0} onClick={apply}>
        {working ? <LoaderCircle className="spin" size={17} /> : <ArrowUpCircle size={17} />}
        {working ? t("正在更新…", "Updating…") : t(`更新选中的 ${selectedCount} 个`, `Update ${selectedCount} selected`)}
      </button></div>
  </div></div>;
}

function FindingDetails({ report, onToggle, reviewing = "", onCodexReview, codexWorking = false, codexProgress = null, onApplyCodexSuggestions, onIgnoreAll }: {
  report: ScanReport; onToggle?: (cluster: RiskCluster) => void | Promise<void>; reviewing?: string;
  onCodexReview?: () => void | Promise<void>; codexWorking?: boolean;
  codexProgress?: CodexReviewProgress | null;
  onApplyCodexSuggestions?: (clusters: RiskCluster[]) => void | Promise<void>;
  onIgnoreAll?: (clusters: RiskCluster[]) => void | Promise<void>;
}) {
  const { t } = useI18n();
  const clusters = [...(report.clusters ?? [])].sort((a, b) => {
    if (a.ignored !== b.ignored) return a.ignored ? 1 : -1;
    return severityRank(b.severity) - severityRank(a.severity);
  });
  const codexByCluster = new Map((report.codexReview?.reviews ?? []).map(review => [review.clusterId, review]));
  const suggestedClusters = codexSuggestedClusters(report);
  const activeClusters = clusters.filter(cluster => !cluster.ignored);
	const batchClusters = batchDismissibleClusters(activeClusters);
  return <>
    {clusters.length ? <RiskOverview report={report} /> :
      <div className="scan-clean"><CheckCircle2 size={16} />{t("本地规则未发现警告。可选用 Codex 复核完整上下文。",
        "Local rules found no warnings. You can optionally ask Codex to review the full context.")}</div>}
    {onIgnoreAll && batchClusters.length > 0 && <div className="manual-review-action"><div>
      <strong>{t("批量处理警告", "Review warnings in bulk")}</strong>
      <small>{t(`可批量忽略 ${batchClusters.length} 个 Medium 及以下警告；High 必须逐项确认，Critical 不可忽略。`,
        `Ignore ${batchClusters.length} eligible Medium-or-lower warnings; High requires individual confirmation and Critical cannot be ignored.`)}</small>
    </div><button className="primary compact" disabled={reviewing !== "" || codexWorking}
      onClick={() => void onIgnoreAll(batchClusters)}>
      {reviewing === "manual-batch" ? <LoaderCircle className="spin" size={14} /> : <CheckSquare2 size={14} />}
      {reviewing === "manual-batch" ? t("正在记录…", "Saving…") : t("忽略可处理项", "Ignore eligible")}
    </button></div>}
    {onCodexReview && <div className="codex-review-action"><div><strong>{t("Codex 复核", "Codex review")}</strong>
      <small>{t("按分组读取完整上下文，本地规则结果作为参考。",
        "Reads the full context by group and uses local rule results as supporting evidence.")}</small></div>
      <button className="ghost" disabled={codexWorking} onClick={() => void onCodexReview()}>
        {codexWorking ? <LoaderCircle className="spin" size={15} /> : <Sparkles size={15} />}
        {codexWorking ? t("Codex 正在复核…", "Codex is reviewing…") :
          report.codexReview ? t("重新复核", "Review again") : t("使用 Codex 复核", "Review with Codex")}
      </button></div>}
    {codexWorking && codexProgress && <CodexProgressCard progress={codexProgress} />}
    {report.codexReview && <div className={`codex-review-summary ${report.codexReview.status}`}>
      <Sparkles size={18} /><div><strong>{t("Codex 复核结果", "Codex review result")}</strong>
        <p>{report.codexReview.summary || report.codexReview.error}</p>
        <small>{report.codexReview.model === "default" ? t("Codex 默认模型", "Codex default model") : report.codexReview.model} ·
          {t("推理强度", "Reasoning effort")} {reasoningEffortLabel(report.codexReview.reasoningEffort, t)} ·
          {isPackagedFullContextMode(report.codexReview.contextMode)
            ? t(` 完整目录上下文（${report.codexReview.contextFileCount || 0} 个文件）`,
              ` full directory context (${report.codexReview.contextFileCount || 0} files)`)
            : t(" 规则摘要上下文", " rule-summary context")}</small>
        {onApplyCodexSuggestions && ["completed", "partial"].includes(report.codexReview.status) && suggestedClusters.length > 0 &&
          <button className="primary compact codex-apply-suggestions" disabled={reviewing !== ""}
            onClick={() => void onApplyCodexSuggestions(suggestedClusters)}>
            {reviewing === "codex-batch" ? <LoaderCircle className="spin" size={14} /> : <CheckSquare2 size={14} />}
            {reviewing === "codex-batch" ? t("正在记录…", "Saving…") :
              t(`一键采纳 ${suggestedClusters.length} 个建议`, `Apply ${suggestedClusters.length} suggestions`)}
          </button>}
        <small className="codex-baseline-note">{t("结果仅供参考，最终由人工决定。",
          "The result is advisory; the final decision remains yours.")}</small></div>
    </div>}
    {!!report.codexReview?.skillReviews?.length && <CodexSkillReviewList report={report} />}
    {clusters.length > 0 && <GroupedRiskDetails report={report} clusters={clusters}
      codexByCluster={codexByCluster} reviewing={reviewing} onToggle={onToggle} />}
  </>;
}

function GroupedRiskDetails({ report, clusters, codexByCluster, reviewing, onToggle }: {
  report: ScanReport;
  clusters: RiskCluster[];
  codexByCluster: Map<string, NonNullable<ScanReport["codexReview"]>["reviews"][number]>;
  reviewing: string;
  onToggle?: (cluster: RiskCluster) => void | Promise<void>;
}) {
  const { t, locale } = useI18n();
  const summaries = report.skills?.length ? report.skills : Array.from(new Map(clusters.map(cluster => {
    const skillName = cluster.skillName || t("未识别 Skill", "Unidentified Skill");
    return [skillName, {
      skillName, sourcePath: skillName, groupId: cluster.groupId || "ungrouped",
      groupName: cluster.groupName || t("未分组", "Ungrouped"), filesScanned: 0,
      highestSeverity: cluster.severity, activeFindingCount: 0, ignoredFindingCount: 0
    }];
  })).values());
  const groups = new Map<string, { name: string; skills: typeof summaries }>();
  for (const skill of summaries) {
    const key = skill.groupId || skill.groupName || "ungrouped";
    const group = groups.get(key) ?? { name: skill.groupName || t("未分组", "Ungrouped"), skills: [] };
    group.skills.push(skill);
    groups.set(key, group);
  }
  return <div className="grouped-risks">
    {[...groups.entries()].map(([groupId, group]) => {
      const groupClusters = clusters.filter(cluster => (cluster.groupId || "ungrouped") === groupId ||
        (!cluster.groupId && group.skills.some(skill => skill.skillName === cluster.skillName)));
      const active = groupClusters.filter(cluster => !cluster.ignored).length;
      return <details key={groupId} className="risk-group">
        <summary className="risk-group-summary"><div><strong>{displayGroupName(group.name, locale)}</strong>
          <small>{t(`${group.skills.length} 个 Skill · ${active} 个待处理警告 · ${groupClusters.length - active} 个已忽略`,
            `${group.skills.length} Skills · ${active} open · ${groupClusters.length - active} ignored`)}</small></div>
          <span className={`badge ${active ? "red" : "green"}`}>{active ? t("需要核查", "Review needed") : t("已处理", "Resolved")}</span></summary>
        <div className="risk-group-skills">{group.skills.map(skill => {
          const skillClusters = groupClusters.filter(cluster => cluster.skillName === skill.skillName ||
            (!cluster.skillName && group.skills.length === 1));
          const skillActive = skillClusters.filter(cluster => !cluster.ignored).length;
          return <details key={`${groupId}:${skill.skillName}`} className="risk-skill">
            <summary><span><strong>{skill.skillName}</strong><small>{skill.sourcePath}</small></span>
              <span>{t(`${skillActive} 待处理 / ${skillClusters.length} 总计`, `${skillActive} open / ${skillClusters.length} total`)}</span></summary>
            <div className="finding-details">{skillClusters.length ? skillClusters.map(cluster => {
              const codex = codexByCluster.get(cluster.id);
              return <article key={cluster.id} className={cluster.ignored ? "ignored" : ""}>
                <span className={`severity ${cluster.ignored ? "ignored" : cluster.severity}`}>
                  {cluster.ignored ? t("已忽略", "Ignored") : severityLabel(cluster.severity, locale)}
                </span>
                <div><strong>{cluster.title}{cluster.deterministic && <em className="hard-baseline">{t("确定性规则", "Deterministic rule")}</em>}</strong>
                  <small>{cluster.ruleId} · {fileClassLabel(cluster.fileClass, locale)} · {t(`${cluster.findingCount} 条命中 · ${cluster.affectedFiles.length} 个文件`,
                    `${cluster.findingCount} matches · ${cluster.affectedFiles.length} files`)}</small>
                  {cluster.sampleFindings[0] && <p>{cluster.sampleFindings[0].explanation}</p>}
                  <details className="cluster-evidence"><summary>{t("查看代表性证据与文件", "View representative evidence and files")}</summary>
                    {cluster.sampleFindings.map(finding => <code key={finding.fingerprint}>
                      {finding.file}:{finding.line || 1}　{finding.evidence || t("文件级风险", "File-level risk")}
                    </code>)}
                  </details>
                  {codex && <div className="codex-cluster"><Sparkles size={14} /><span><b>Codex: {codexVerdictLabel(codex.verdict, locale)}</b>
                    <small>{codex.rationale}</small><small>{codex.recommendation}</small></span></div>}
                  {cluster.ignoreReason && <p className="ignore-reason"><b>{t("人工核查记录：", "Manual review record: ")}</b>{cluster.ignoreReason}</p>}</div>
				{onToggle && (cluster.ignored || cluster.severity !== "critical") && <button className="ghost finding-review" disabled={reviewing !== ""}
                  onClick={() => void onToggle(cluster)}>
                  {reviewing === cluster.id ? <LoaderCircle className="spin" size={14} /> :
                    cluster.ignored ? <RotateCcw size={14} /> : <ShieldCheck size={14} />}
					{cluster.ignored ? t("恢复", "Restore") : cluster.severity === "high" ? t("审阅并接受", "Review and accept") : t("忽略", "Ignore")}
				</button>}
				{onToggle && !cluster.ignored && cluster.severity === "critical" &&
					<span className="hard-baseline">{t("不可忽略", "Cannot ignore")}</span>}
              </article>;
            }) : <Empty text={t("这个 Skill 没有规则警告", "This Skill has no rule warnings")} />}</div>
          </details>;
        })}</div>
      </details>;
    })}
  </div>;
}

function CodexProgressCard({ progress }: { progress: CodexReviewProgress }) {
  const { t, locale } = useI18n();
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, []);
  const elapsed = Math.max(0, Math.floor((now - new Date(progress.startedAt).getTime()) / 1000));
  const percent = progress.totalSkills > 0
    ? Math.min(100, Math.round(progress.completedSkills / progress.totalSkills * 100))
    : 0;
  return <div className="codex-progress-card" role="status" aria-live="polite">
    <div className="codex-progress-head">
      <span><LoaderCircle className="spin" size={16} /><strong>{progress.message}</strong></span>
      <small>{formatElapsed(elapsed, locale)}</small>
    </div>
    <div className="codex-progress-track"><i style={{ width: `${percent}%` }} /></div>
    <div className="codex-progress-meta">
      <span>{progress.completedSkills}/{progress.totalSkills || "?"} Skills</span>
      <span>{progress.completedBatch}/{progress.batchCount || "?"} {t("分组", "groups")}</span>
      {!!progress.contextChunkCount && <span>{t(`上下文 ${progress.contextChunkIndex || 0}/${progress.contextChunkCount} 块`, `Context ${progress.contextChunkIndex || 0}/${progress.contextChunkCount} chunks`)}
        {progress.contextChunkAttempt ? ` · ${t(`第 ${progress.contextChunkAttempt} 次`, `attempt ${progress.contextChunkAttempt}`)}` : ""}</span>}
      {!!progress.contextChunkFiles && <span>{progress.contextChunkFiles} {t("个文件", "files")}</span>}
      <span>{t(`${progress.activityCount} 次分析活动`, `${progress.activityCount} analysis events`)}</span>
    </div>
    {!!progress.activeBatches?.length ? <div className="codex-progress-batches">
      {progress.activeBatches.map(batch => <div key={batch.index}><small>{batch.groupName
        ? displayGroupName(batch.groupName, locale) : t(`分组 ${batch.index}`, `Group ${batch.index}`)}</small>
        <span>{batch.skillNames.join(locale === "en-US" ? ", " : "、")}</span></div>)}
    </div> : !!progress.activeSkills.length && <div className="codex-progress-skills">
      <small>{t("当前 Skills", "Current Skills")}</small>{progress.activeSkills.map(name => <span key={name}>{name}</span>)}
    </div>}
  </div>;
}

function CodexSkillReviewList({ report }: { report: ScanReport }) {
  const { t, locale } = useI18n();
  const reviews = report.codexReview?.skillReviews ?? [];
  const groupBySkill = new Map((report.skills ?? []).map(skill => [skill.skillName, skill.groupName || t("未分组", "Ungrouped")]));
  const groups = new Map<string, typeof reviews>();
  for (const review of reviews) {
    const group = groupBySkill.get(review.skillName) || t("未分组", "Ungrouped");
    groups.set(group, [...(groups.get(group) ?? []), review]);
  }
  return <details className="codex-skill-results">
    <summary>{t(`按分组查看 ${reviews.length} 个 Skill 的 Codex 结论`,
      `View Codex conclusions for ${reviews.length} Skills by group`)}</summary>
    {[...groups.entries()].map(([group, groupReviews]) => <section key={group} className="codex-result-group">
      <h4>{displayGroupName(group, locale)}<small>{groupReviews.length} Skills</small></h4>
      <div className="codex-skill-list">{groupReviews.map(review => <article key={`${review.skillName}:${review.sourcePath}`}
      className={`codex-skill-review ${review.verdict}`}>
      <div className="codex-skill-head"><div><strong>{review.skillName}</strong><code>{review.sourcePath}</code></div>
        <span className={`codex-verdict ${review.verdict}`}>{codexSkillVerdictLabel(review.verdict, locale)}</span></div>
      <p>{review.summary}</p>
      <small>{t("置信度", "Confidence")} {Math.round((review.confidence || 0) * 100)}% ·
        {t(` Skill 目录文件 ${review.contextFileCount || 0} 个 · 关联警告 ${review.clusterIds?.length || 0} 个`,
          ` ${review.contextFileCount || 0} files in Skill directory · ${review.clusterIds?.length || 0} related warnings`)}</small>
      {review.error && <div className="codex-skill-error">{review.error}</div>}
      {!!review.concerns?.length && <div className="codex-concern-list">{review.concerns.map((concern, index) =>
        <div key={`${concern.title}:${index}`}><span className={`severity ${concern.severity}`}>{severityLabel(concern.severity, locale)}</span>
          <div><strong>{concern.title}</strong><p>{concern.rationale}</p>
            <small>{concern.recommendation}</small>
            {!!concern.evidenceFiles?.length && <code>{concern.evidenceFiles.join(" · ")}</code>}</div>
        </div>)}</div>}
    </article>)}</div></section>)}
  </details>;
}

function formatElapsed(seconds: number, locale: AppLocale): string {
  const minutes = Math.floor(seconds / 60);
  const remainder = seconds % 60;
  return locale === "en-US"
    ? minutes ? `${minutes}m ${remainder.toString().padStart(2, "0")}s` : `${remainder}s`
    : minutes ? `${minutes} 分 ${remainder.toString().padStart(2, "0")} 秒` : `${remainder} 秒`;
}

function codexSkillVerdictLabel(verdict: string, locale: AppLocale): string {
  switch (verdict) {
    case "no-material-risk": return translate(locale, "未见明确风险", "No material risk");
    case "mostly-contextual": return translate(locale, "主要为上下文内容", "Mostly contextual");
    case "review-required": return translate(locale, "建议人工关注", "Manual review advised");
    case "high-risk": return translate(locale, "高风险", "High risk");
    case "insufficient-context": return translate(locale, "上下文不足", "Insufficient context");
    default: return verdict || translate(locale, "未知", "Unknown");
  }
}

function RiskOverview({ report }: { report: ScanReport }) {
  const { t } = useI18n();
  const skills = report.skills ?? [];
  const groupCount = new Set(skills.map(skill => skill.groupId)).size;
  const affectedSkills = new Set((report.clusters ?? [])
    .filter(cluster => !cluster.ignored).map(cluster => cluster.skillName)).size;
  return <div className="risk-overview" aria-label={t("风险概述", "Risk overview")}>
    <div><span>{t("分组", "Groups")}</span><strong>{groupCount}</strong><small>{t("本次检查", "This scan")}</small></div>
    <div><span>Skills</span><strong>{skills.length}</strong><small>{t(`${affectedSkills} 个需要关注`, `${affectedSkills} need attention`)}</small></div>
    <div><span>{t("待处理", "Open")}</span><strong>{report.activeFindingCount}</strong><small>{t("按 Skill 归类", "Grouped by Skill")}</small></div>
    <div><span>{t("已忽略", "Ignored")}</span><strong>{report.ignoredFindingCount}</strong><small>{t("人工决定", "Manual decision")}</small></div>
  </div>;
}

function SecurityPage({ data, refresh, runOperation, rootFilter, defaultRootId }: {
  data: Dashboard; refresh: () => Promise<void>; runOperation: RunOperation; rootFilter: string; defaultRootId: string;
}) {
  const { t, locale, formatDate } = useI18n();
  const [report, setReport] = useState<ScanReport | null>(data.recentReports[0] ?? null);
  const [working, setWorking] = useState(false);
  const [codexWorking, setCodexWorking] = useState(false);
  const [reviewing, setReviewing] = useState("");
  const activeRootId = rootFilter === "all" ? defaultRootId : rootFilter;
  const selectable = data.skills.filter(skill => !skill.system && (skill.rootId || "codex-default") === activeRootId);
  const recommendedNames = () => selectable.filter(skill =>
    !isSecurityCurrent(skill)).map(skill => skill.name);
  const [selectedSkills, setSelectedSkills] = useState<Set<string>>(() => new Set(recommendedNames()));
  const { progress: codexProgress, clearProgress } = useCodexProgress();
  useEffect(() => {
    setSelectedSkills(new Set(recommendedNames()));
  }, [activeRootId]);
  useEffect(() => {
    const latest = data.recentReports[0] ?? null;
    if (!working && !codexWorking && latest?.id !== report?.id) setReport(latest);
  }, [data.recentReports, working, codexWorking, report?.id]);
  const audit = async () => {
    const names = [...selectedSkills];
    if (!names.length) return;
    setWorking(true);
    try {
      const scanned = await runOperation(
        t(`扫描 ${names.length} 个 Skills`, `Scan ${names.length} Skills`), () => api.auditSkills(names, activeRootId), t("安全扫描已完成", "Security scan completed")
      );
      if (scanned) {
        setReport(scanned);
        setSelectedSkills(new Set());
        await refresh();
      }
    } finally { setWorking(false); }
  };
  const toggleSkill = (name: string) => setSelectedSkills(current => {
    const next = new Set(current);
    if (next.has(name)) next.delete(name); else next.add(name);
    return next;
  });
  const selectAll = () => setSelectedSkills(new Set(selectable.map(skill => skill.name)));
  const invertSelection = () => setSelectedSkills(new Set(selectable
    .filter(skill => !selectedSkills.has(skill.name)).map(skill => skill.name)));
  const selectRecommended = () => setSelectedSkills(new Set(recommendedNames()));
  const toggleIgnore = async (cluster: RiskCluster) => {
		const decision = requestRiskDecision(cluster, !cluster.ignored, t);
		if (!decision) return;
    setReviewing(cluster.id);
    const changed = await runOperation(
      cluster.ignored ? t("恢复警告", "Restore warning") : t("记录人工决定", "Record manual decision"),
			() => api.setRiskClusterIgnored(cluster, !cluster.ignored, decision.reason, decision.confirmHighRisk),
      cluster.ignored ? t("警告已恢复", "Warning restored") : t("警告已忽略", "Warning ignored")
    );
    setReviewing("");
    if (!changed) return;
		setReport(current => current ? updateClusterState(current, cluster.id, !cluster.ignored, decision.reason) : current);
    await refresh();
  };
  const ignoreAll = async (clusters: RiskCluster[]) => {
		clusters = batchDismissibleClusters(clusters);
		if (!report || !clusters.length) return;
    setReviewing("manual-batch");
    const changed = await runOperation(
			t("批量忽略可处理警告", "Ignore eligible warnings in bulk"),
      () => api.setRiskClustersIgnored(clusters, true, ""),
      t(`已忽略 ${clusters.length} 个待处理警告`, `Ignored ${clusters.length} open warnings`)
    );
    setReviewing("");
    if (!changed) return;
    setReport(current => current ? updateClustersState(current, clusters, true, "") : current);
    await refresh();
  };
  const reviewWithCodex = async () => {
    if (!report) return;
    setCodexWorking(true);
    clearProgress();
    try {
      const reviewed = await runOperation(
        t("Codex 风险复核", "Codex risk review"),
        () => api.reviewWithCodex(report, (report.skills ?? []).map(skill => skill.skillName)),
        t("Codex 风险复核已完成", "Codex risk review completed")
      );
      if (reviewed) { setReport(reviewed); await refresh(); }
    } finally { setCodexWorking(false); }
  };
  const applyCodexSuggestions = async (clusters: RiskCluster[]) => {
    if (!report || !confirmCodexSuggestions(report, clusters)) return;
    setReviewing("codex-batch");
    try {
      const reason = codexBatchReason(report, clusters, locale);
      await api.setRiskClustersIgnored(clusters, true, reason);
      setReport(updateClustersState(report, clusters, true, reason));
      await refresh();
    } finally {
      setReviewing("");
    }
  };
  const groups = data.groups.map(group => ({
    ...group,
    skills: selectable.filter(skill => skill.groupId === group.id)
  })).filter(group => group.skills.length > 0);
  return <div className="security-grid">
    <section className="panel security-summary"><div className="shield"><ShieldCheck size={42} /></div><h2>{t("本地安全扫描", "Local security scan")}</h2>
      <p>{t("检查提示注入、凭据访问、命令执行、网络请求、批量删除和混淆内容。",
        "Checks for prompt injection, credential access, command execution, network requests, bulk deletion, and obfuscation.")}</p>
      <button className="primary" onClick={audit} disabled={working || codexWorking || selectedSkills.size === 0}>
        {working ? <LoaderCircle className="spin" size={17} /> : <ShieldCheck size={17} />}
        {working ? t("正在扫描…", "Scanning…") : t(`扫描选中的 ${selectedSkills.size} 个`, `Scan ${selectedSkills.size} selected`)}
      </button>
      <small>{t("仅在本地读取文件；Codex 复核可在扫描结果中启动。",
        "Files are read locally only. Codex review can be started from the scan results.")}</small></section>
    <section className="panel security-queue"><PanelHead title={t("选择要检查的 Skills", "Choose Skills to scan")}
      subtitle={t("默认选择未检查或内容已变化的 Skills。", "Skills not yet scanned or changed since their last scan are selected by default.")} />
      <div className="selection-tools">
        <button className="ghost compact" onClick={selectRecommended}>{t("恢复推荐", "Use recommended")}</button>
        <button className="ghost compact" onClick={selectAll}>{t("全选", "Select all")}</button>
        <button className="ghost compact" onClick={invertSelection}>{t("反选", "Invert")}</button>
        <button className="ghost compact" onClick={() => setSelectedSkills(new Set())}>{t("清空", "Clear")}</button>
        <small>{t(`已选 ${selectedSkills.size}/${selectable.length}`, `${selectedSkills.size}/${selectable.length} selected`)}</small>
      </div>
      <div className="security-skill-groups">{groups.map(group => {
        const selectedCount = group.skills.filter(skill => selectedSkills.has(skill.name)).length;
        return <details key={group.id} open={selectedCount > 0}>
          <summary><span><strong>{displayGroupName(group.name, locale)}</strong><small>{group.skills.length} Skills</small></span>
            <b>{t(`${selectedCount} 个已选`, `${selectedCount} selected`)}</b></summary>
          <div>{group.skills.map(skill => <label key={skill.name} className="security-skill-option">
            <input type="checkbox" checked={selectedSkills.has(skill.name)} onChange={() => toggleSkill(skill.name)} />
            <span><strong>{skill.name}</strong><small>
              {isSecurityCurrent(skill)
                ? t(`已检查${skill.lastSecurityScan ? ` · ${formatDate(skill.lastSecurityScan)}` : ""}`,
                  `Scanned${skill.lastSecurityScan ? ` · ${formatDate(skill.lastSecurityScan)}` : ""}`)
                : skill.securityChanged ? t("内容已变化，需要重新检查", "Content changed; scan again") : t("尚未检查", "Not scanned")}
            </small></span>
            <em className={isSecurityCurrent(skill) ? "checked" : "pending"}>
              {isSecurityCurrent(skill) ? t("可跳过", "Can skip") : t("建议检查", "Recommended")}
            </em>
          </label>)}</div>
        </details>;
      })}</div>
    </section>
    <section className="panel security-results"><PanelHead title={t("最近扫描结果", "Latest scan result")} subtitle={report
      ? t(`${report.filesScanned} 个文件 · ${report.activeFindingCount} 个待处理 · ${report.ignoredFindingCount} 个已忽略`,
        `${report.filesScanned} files · ${report.activeFindingCount} open · ${report.ignoredFindingCount} ignored`)
      : t("尚未扫描", "Not scanned yet")} />
      {!report ? <Empty text={t("运行一次本地安全扫描", "Run a local security scan")} /> : <>
        <ScanSummary report={report} />
        <div className="security-report-details">
          <FindingDetails report={report} reviewing={reviewing} onToggle={toggleIgnore}
            onCodexReview={reviewWithCodex} codexWorking={codexWorking}
            codexProgress={codexProgress?.reportId === report.id ? codexProgress : null}
            onApplyCodexSuggestions={applyCodexSuggestions} onIgnoreAll={ignoreAll} />
        </div>
      </>}
    </section>
  </div>;
}

function isSecurityCurrent(skill: Skill): boolean {
  return !skill.securityChanged && (skill.securityStatus === "checked" || skill.securityStatus === "安全");
}

function ScanSummary({ report, compact = false }: { report: ScanReport; compact?: boolean }) {
  const { t, locale } = useI18n();
  const groupCount = new Set((report.skills ?? []).map(skill => skill.groupId)).size;
  const skillCount = report.skills?.length ?? 0;
  return <div className={`scan-summary ${compact ? "compact" : ""}`}>
    <span className={`severity ${report.activeHighestSeverity}`}>{severityLabel(report.activeHighestSeverity, locale)}</span>
    <div><strong>{report.activeFindingCount === 0 ? t("没有待处理警告", "No open warnings") :
      t(`${report.activeFindingCount} 个警告需要处理`, `${report.activeFindingCount} warnings need review`)}</strong>
      <small>{t(`检查 ${groupCount || "未知"} 个分组、${skillCount || "未知"} 个 Skill；${report.ignoredFindingCount} 个警告已处理。`,
        `Scanned ${groupCount || "unknown"} groups and ${skillCount || "unknown"} Skills; ${report.ignoredFindingCount} warnings resolved.`)}</small></div>
  </div>;
}

function updateClusterState(report: ScanReport, clusterId: string, ignored: boolean, reason: string): ScanReport {
  const findings = report.findings.map(finding => finding.clusterId === clusterId
    ? { ...finding, ignored, ignoreReason: ignored ? reason : "" } : finding);
  const clusters = report.clusters.map(cluster => cluster.id === clusterId
    ? {
      ...cluster, ignored, ignoreReason: ignored ? reason : "",
      sampleFindings: cluster.sampleFindings.map(finding => ({ ...finding, ignored, ignoreReason: ignored ? reason : "" }))
    } : cluster);
  const active = clusters.filter(cluster => !cluster.ignored);
  return {
    ...report,
    findings,
    clusters,
    activeFindingCount: active.length,
    ignoredFindingCount: clusters.length - active.length,
    activeHighestSeverity: highestSeverity(active)
  };
}

function updateClustersState(report: ScanReport, clusters: RiskCluster[], ignored: boolean, reason: string): ScanReport {
  const ids = new Set(clusters.map(cluster => cluster.id));
  let next = report;
  for (const cluster of report.clusters) {
    if (ids.has(cluster.id)) next = updateClusterState(next, cluster.id, ignored, reason);
  }
  return next;
}

function highestSeverity(findings: Array<{ severity: Finding["severity"] }>): ScanReport["activeHighestSeverity"] {
  const order: ScanReport["activeHighestSeverity"][] = ["informational", "low", "medium", "high", "critical"];
  let highest: ScanReport["activeHighestSeverity"] = "informational";
  for (const finding of findings) {
    if (order.indexOf(finding.severity) > order.indexOf(highest)) highest = finding.severity;
  }
  return highest;
}

function severityRank(severity: Finding["severity"]): number {
  return ["informational", "low", "medium", "high", "critical"].indexOf(severity);
}

function fileClassLabel(fileClass: string, locale: AppLocale): string {
  const labels: Record<string, [string, string]> = {
    instruction: ["Skill 指令", "Skill instructions"], runtime: ["运行代码", "Runtime code"], test: ["测试/夹具", "Tests/fixtures"],
    documentation: ["文档/示例", "Documentation/examples"], asset: ["资源文件", "Assets"]
  };
  const value = labels[fileClass];
  return value ? translate(locale, value[0], value[1]) : fileClass;
}

function codexVerdictLabel(verdict: string, locale: AppLocale): string {
  const labels: Record<string, [string, string]> = {
    "confirmed-risk": ["确认存在风险", "Confirmed risk"],
    "context-dependent": ["取决于上下文", "Context dependent"],
    "documentation-or-example": ["文档或示例", "Documentation or example"],
    "false-positive": ["很可能误报", "Likely false positive"],
    "insufficient-context": ["上下文不足", "Insufficient context"],
    "manual-override-required": ["必须人工决定", "Manual decision required"]
  };
  const value = labels[verdict];
  return value ? translate(locale, value[0], value[1]) : verdict;
}

function codexSuggestedClusters(report: ScanReport): RiskCluster[] {
  if (!report.codexReview || !["completed", "partial"].includes(report.codexReview.status)) return [];
  const reviews = new Map(report.codexReview.reviews.map(review => [review.clusterId, review]));
  return (report.clusters ?? []).filter(cluster => {
		if (cluster.ignored || cluster.severity === "high" || cluster.severity === "critical") return false;
    const verdict = reviews.get(cluster.id)?.verdict;
    return verdict === "false-positive" || verdict === "documentation-or-example" ||
      verdict === "manual-override-required";
  });
}

function batchDismissibleClusters(clusters: RiskCluster[]): RiskCluster[] {
	return clusters.filter(cluster => cluster.severity !== "high" && cluster.severity !== "critical");
}

function requestRiskDecision(cluster: RiskCluster, ignored: boolean, t: Translate): {
	reason: string;
	confirmHighRisk: boolean;
} | null {
	if (!ignored) return { reason: "", confirmHighRisk: false };
	if (cluster.severity === "critical") {
		window.alert(t(
			"Critical 风险不可忽略。请修复风险内容或更换来源后重新检查。",
			"Critical risk cannot be ignored. Fix the content or replace the source, then reassess."
		));
		return null;
	}
	if (cluster.severity !== "high") return { reason: "", confirmHighRisk: false };
	if (!window.confirm(t(
		"确认已理解此 High 风险，并仅接受当前风险簇？",
		"Confirm that you understand this High risk and accept only this cluster?"
	))) return null;
	return {
		reason: t("人工审核确认（无需填写理由）", "Human review confirmed"),
		confirmHighRisk: true
	};
}

function codexBatchReason(report: ScanReport, clusters: RiskCluster[], locale: AppLocale): string {
  const model = report.codexReview?.model || "Codex";
  return translate(locale, `人工采纳 ${model} 的复核建议（${clusters.length} 个警告）`,
    `Manually accepted ${model} review suggestions (${clusters.length} warnings)`);
}

function confirmCodexSuggestions(_report: ScanReport, clusters: RiskCluster[]): boolean {
  return clusters.length > 0;
}

function severityLabel(severity: ScanReport["activeHighestSeverity"], locale: AppLocale) {
  const labels = {
    informational: ["提示", "Info"], low: ["低风险", "Low"], medium: ["中风险", "Medium"],
    high: ["高风险", "High"], critical: ["严重风险", "Critical"]
  } as const;
  return translate(locale, labels[severity][0], labels[severity][1]);
}

function HistoryPage({ data, refresh, runOperation }: { data: Dashboard; refresh: () => Promise<void>; runOperation: RunOperation }) {
  const { t, locale, formatDate, join } = useI18n();
  const [workingId, setWorkingId] = useState("");
  const rollback = async (id: string) => {
    const original = data.recentHistory.find(tx => tx.id === id);
    const detail = original?.type.startsWith("group-")
      ? t("只会恢复分组布局，不会移动或修改 Skill 文件。", "Only the group layout will be restored; Skill files will not be moved or changed.")
      : original?.type === "assisted-install"
        ? t("会按事务记录恢复 Skills、受管工具和 Codex MCP 配置；目标已被修改时会安全停止并提示人工处理。",
          "The transaction journal will restore Skills, managed tools, and Codex MCP configuration. Recovery stops safely if a target changed.")
      : original?.type === "manage" || original?.type === "adopt"
        ? t("只会恢复来源记录，不会移动 Skill 文件。", "Only source records will be restored; Skill files will not be moved.")
        : t("当前版本会先移动到隔离区。", "The current version will be moved to quarantine first.");
    if (!confirm(t(`回滚操作 ${id}？${detail}`, `Roll back operation ${id}? ${detail}`))) return;
    setWorkingId(id);
    try {
      const result = await runOperation(t("回滚操作", "Roll back operation"), () => api.rollback(id), t("回滚已完成", "Rollback completed"));
      if (result) await refresh();
    } finally { setWorkingId(""); }
  };
  return <section className="panel full"><PanelHead title={t("操作历史", "Operation history")} subtitle={t("查看操作状态并执行回滚", "View operation status and roll back completed changes")} />
    <div className="history-list">{data.recentHistory.length === 0 ? <Empty text={t("暂无操作", "No operations")} /> : data.recentHistory.map(tx => {
      const recoverableAssisted = tx.type === "assisted-install" && tx.status !== "running" &&
        !!tx.recoveryStatus && tx.recoveryStatus !== "completed";
      const recoverableStandard = tx.status === "completed" &&
        (tx.type === "install" || tx.type === "adopt" || tx.type === "manage" || tx.type.startsWith("group-"));
      const badgeClass = tx.status === "completed" ? "green" : tx.status === "partial" ? "amber" : "red";
      return <article key={tx.id}>
      <div className={`tx-icon ${tx.status}`}><Clock3 size={19} /></div><div className="grow"><strong>{transactionTypeLabel(tx.type, locale)}</strong><span>{join(tx.targets) || "-"}</span>
        <small>{formatDate(tx.startedAt)} · {tx.id}</small>
        {tx.type === "assisted-install" && !!tx.steps?.length && <details className="assisted-history-details">
          <summary>{t("查看安装步骤", "View installation steps")}</summary>
          <ol>{tx.steps.map(step => <li key={step.id} className={step.status}>
            <span>{step.title}</span><em>{transactionStatusLabel(step.status, locale)}</em>
            {step.error && <small>{step.error}</small>}
          </li>)}</ol>
        </details>}
      </div><span className={`badge ${badgeClass}`}>{transactionStatusLabel(tx.status, locale)}</span>
      {(recoverableAssisted || recoverableStandard) && <button className="icon-button" disabled={!!workingId} title={t("回滚", "Roll back")} onClick={() => rollback(tx.id)}>
        {workingId === tx.id ? <LoaderCircle className="spin" size={17} /> : <RotateCcw size={17} />}</button>}
    </article>;
    })}</div>
  </section>;
}

function QuarantinePage({ refresh, runOperation, rootId }: { refresh: () => Promise<void>; runOperation: RunOperation; rootId: string }) {
  const { t } = useI18n();
  const [items, setItems] = useState<Array<{ skill: string; rootId: string; transactionId: string; path: string }>>([]);
  const [working, setWorking] = useState("");
  useEffect(() => { void api.quarantineList(rootId).then(setItems); }, [rootId]);
  const restore = async (skill: string, tx: string) => {
    setWorking(skill + tx);
    try {
      const result = await runOperation(t(`恢复 ${skill}`, `Restore ${skill}`), () => api.restore(skill, tx, rootId), t("Skill 已恢复", "Skill restored"));
      if (result) { setItems(await api.quarantineList(rootId)); await refresh(); }
    } finally { setWorking(""); }
  };
  return <section className="panel full"><PanelHead title={t("隔离区", "Quarantine")} subtitle={t("查看已移除的 Skills，并选择恢复", "View removed Skills and restore them when needed")} />
    <div className="history-list">{items.length === 0 ? <Empty text={t("隔离区为空", "Quarantine is empty")} /> : items.map(item => <article key={item.rootId + item.skill + item.transactionId}>
      <div className="tx-icon"><ArchiveRestore size={19} /></div><div className="grow"><strong>{item.skill}</strong><span>{item.transactionId}</span><small>{item.path}</small></div>
      <button className="ghost" disabled={!!working} onClick={() => restore(item.skill, item.transactionId)}>
        {working === item.skill + item.transactionId ? <LoaderCircle className="spin" size={16} /> : <RotateCcw size={16} />}
        {working === item.skill + item.transactionId ? t("恢复中…", "Restoring…") : t("恢复", "Restore")}</button></article>)}</div>
  </section>;
}

function ReportsPage({ data }: { data: Dashboard }) {
  const { t, locale, formatDate } = useI18n();
  const [selected, setSelected] = useState<ScanReport | null>(null);
  const [loading, setLoading] = useState(false);
  const requestId = useRef(0);
  const openReport = async (report: ScanReport) => {
    const currentRequest = ++requestId.current;
    setSelected(report);
    setLoading(true);
    try {
      const detail = await api.report(report.id, report.rootId ?? "");
      if (currentRequest === requestId.current) setSelected(detail);
    } catch {
      // The dashboard already contains a complete report in v0.11; keep it
      // visible when an older backend does not expose the detail endpoint.
    } finally {
      if (currentRequest === requestId.current) setLoading(false);
    }
  };
  return <section className="panel full"><PanelHead title={t("扫描报告", "Scan reports")} subtitle={t("选择一份报告查看风险详情", "Select a report to inspect its findings")} />
    <div className="history-list">{data.recentReports.length === 0 ? <Empty text={t("暂无报告", "No reports")} /> : data.recentReports.map(r => {
      const active = selected?.id === r.id && (selected.rootId ?? "") === (r.rootId ?? "");
      return <article key={`${r.rootId ?? ""}:${r.id}`} className={`report-row ${active ? "selected" : ""}`} role="button" tabIndex={0}
        aria-controls="report-detail"
        aria-expanded={active} onClick={() => void openReport(r)}
        onKeyDown={event => { if (event.key === "Enter" || event.key === " ") { event.preventDefault(); void openReport(r); } }}>
        <div className={`tx-icon ${r.activeHighestSeverity}`}><ShieldCheck size={19} /></div><div className="grow"><strong>{r.id}</strong><span>{r.target}</span>
          <small>{formatDate(r.completedAt)} · {t(`${r.filesScanned} 文件 · ${r.ignoredFindingCount} 已忽略`,
            `${r.filesScanned} files · ${r.ignoredFindingCount} ignored`)}</small></div>
        <span className={`severity ${r.activeHighestSeverity}`}>{severityLabel(r.activeHighestSeverity, locale)}</span>
        <ChevronRight size={17} aria-hidden="true" />
      </article>;
    })}</div>
    {selected && <section id="report-detail" className="report-detail" aria-live="polite">
      <div className="report-detail-head"><div><h3>{selected.id}</h3><p>{selected.target} · {formatDate(selected.completedAt)}</p></div>
        <button className="icon-button" aria-label={t("关闭报告详情", "Close report details")} onClick={() => setSelected(null)}><X size={16} /></button></div>
      {loading && <div className="report-detail-loading"><LoaderCircle className="spin" size={16} />{t("正在读取完整报告…", "Loading full report…")}</div>}
      <div className="scan-summary"><ShieldCheck size={18} /><div><strong>{t("扫描结果", "Scan result")}</strong>
        <small>{selected.filesScanned} {t("个文件", "files")} · {selected.activeFindingCount} {t("个活动风险", "active findings")} · {selected.ignoredFindingCount} {t("个已忽略", "ignored")}</small></div>
        <span className={`severity ${selected.activeHighestSeverity}`}>{severityLabel(selected.activeHighestSeverity, locale)}</span></div>
      <FindingDetails report={selected} />
      {selected.codexReview && <CodexSkillReviewList report={selected} />}
    </section>}
  </section>;
}

function SettingsPage({ locale, setLocale, theme, setTheme, refresh, runOperation }: {
  locale: AppLocale; setLocale: (locale: AppLocale) => void;
  theme: AppTheme; setTheme: (theme: AppTheme) => void;
  refresh: () => Promise<void>; runOperation: RunOperation;
}) {
  const { t, formatDate } = useI18n();
  const [cfg, setCfg] = useState<any>(null);
  const [diagnostics, setDiagnostics] = useState<Record<string, any> | null>(null);
  const [githubUser, setGitHubUser] = useState("");
  const [githubToken, setGitHubToken] = useState("");
  const [githubStatus, setGitHubStatus] = useState<Record<string, any> | null>(null);
  const [codexStatus, setCodexStatus] = useState<CodexCLIStatus | null>(null);
  const [saved, setSaved] = useState(false);
  const [scheduling, setScheduling] = useState(false);
  useEffect(() => {
    void api.config().then(value => {
      const normalized = normalizeLocale(value?.locale);
      setCfg({ ...value, locale: normalized, theme: normalizeTheme(value?.theme) });
    });
    void api.diagnostics().then(setDiagnostics);
    let active = true;
    let checking = false;
    let lastChecked = 0;
    const refreshCodexStatus = () => {
      const now = Date.now();
      if (checking || now - lastChecked < 1500) return;
      checking = true;
      lastChecked = now;
      void api.codexStatus().then(status => {
        if (active) setCodexStatus(status);
      }).catch(() => undefined).finally(() => {
        checking = false;
      });
    };
    refreshCodexStatus();
    window.addEventListener("focus", refreshCodexStatus);
    const onVisibilityChange = () => {
      if (document.visibilityState === "visible") refreshCodexStatus();
    };
    document.addEventListener("visibilitychange", onVisibilityChange);
    return () => {
      active = false;
      window.removeEventListener("focus", refreshCodexStatus);
      document.removeEventListener("visibilitychange", onVisibilityChange);
    };
  }, []);
  if (!cfg) return <Loading />;
  const codexModels = codexStatus?.authenticated ? (codexStatus.models ?? []) : [];
  const configuredModel = cfg.codexReview.model || "gpt-5.6-luna";
  const configuredModelMissing = configuredModel !== "default" &&
    !codexModels.some(model => model.slug === configuredModel);
  const configuredEffort = cfg.codexReview.reasoningEffort || "xhigh";
  const reasoningLevels = availableReasoningLevels(codexModels, configuredModel, configuredEffort);
  const pathFields = [
    [t("Skills 根目录", "Skills root"), "skillsRoot"], [t("数据目录", "Data directory"), "dataRoot"],
    [t("操作日志", "Operation logs"), "logsRoot"], [t("扫描与操作报告", "Scan and operation reports"), "reportsRoot"],
    [t("备份目录", "Backups"), "backupsRoot"], [t("隔离区", "Quarantine"), "quarantineRoot"],
    [t("缓存目录", "Cache"), "cacheRoot"], [t("暂存目录", "Staging"), "stagingRoot"]
  ];
  const save = async () => {
    const result = await runOperation(t("保存应用设置", "Save application settings"), async () => {
      await api.saveConfig({ ...cfg, locale });
      return true;
    }, t("设置已保存，路径变更需重启应用", "Settings saved. Restart the application after changing paths."));
    if (result) { setSaved(true); setTimeout(() => setSaved(false), 1800); }
  };
  const changeLocale = async (nextLocale: AppLocale) => {
    setLocale(nextLocale);
    const nextCfg = { ...cfg, locale: nextLocale };
    setCfg(nextCfg);
    await runOperation(
      translate(nextLocale, "切换界面语言", "Change interface language"),
      async () => { await api.saveConfig(nextCfg); return true; },
      translate(nextLocale, "界面语言已保存", "Interface language saved")
    );
  };
  const changeTheme = async (nextTheme: AppTheme) => {
    setTheme(nextTheme);
    const nextCfg = { ...cfg, theme: nextTheme };
    setCfg(nextCfg);
    await runOperation(
      translate(locale, "切换外观", "Change appearance"),
      async () => { await api.saveConfig(nextCfg); return true; },
      translate(locale, "外观已保存", "Appearance saved")
    );
  };
  const schedule = async () => {
    setScheduling(true);
    try {
      await runOperation(t("配置定时更新检查", "Configure scheduled update checks"), async () => {
        await api.schedule(cfg.schedule.enabled, cfg.schedule.frequency, cfg.schedule.time);
        return true;
      }, cfg.schedule.enabled ? t("定时检查已启用", "Scheduled checks enabled") : t("定时检查已关闭", "Scheduled checks disabled"));
    } finally { setScheduling(false); }
  };
  const saveToken = async () => {
    if (!githubToken.trim()) return;
    const result = await runOperation(t("保存 GitHub 凭据", "Save GitHub credentials"), async () => {
      await api.saveGitHubToken(githubToken.trim(), githubUser.trim());
      return true;
    }, t("GitHub 凭据已保存到 Windows 凭据管理器", "GitHub credentials saved to Windows Credential Manager"));
    if (result) {
      setGitHubToken("");
      const status = await runOperation(t("验证 GitHub 凭据", "Validate GitHub credentials"), api.validateGitHub, t("GitHub 凭据验证完成", "GitHub credential validation completed"));
      if (status) setGitHubStatus(status);
    }
  };
  const validateGitHub = async () => {
    const status = await runOperation(t("验证 GitHub 凭据", "Validate GitHub credentials"), api.validateGitHub, t("GitHub 凭据验证完成", "GitHub credential validation completed"));
    if (status) setGitHubStatus(status);
  };
  const validateCodex = async () => {
    const status = await runOperation(t("检查 Codex CLI", "Check Codex CLI"), api.codexStatus, t("Codex CLI 检查完成", "Codex CLI check completed"));
    if (status) setCodexStatus(status);
  };
  const changeCodexModel = (nextModel: string) => {
    const catalogEntry = codexModels.find(model => model.slug === nextModel);
    const supported = catalogEntry?.reasoningLevels?.map(level => level.effort) ?? [];
    const nextEffort = catalogEntry?.defaultReasoningLevel && supported.includes(catalogEntry.defaultReasoningLevel)
      ? catalogEntry.defaultReasoningLevel
      : availableReasoningLevels(codexModels, nextModel, configuredEffort)[0] ?? configuredEffort;
    setCfg({ ...cfg, codexReview: { ...cfg.codexReview, model: nextModel, reasoningEffort: nextEffort } });
  };
  const bootstrap = async () => {
    if (!confirm(t("自动识别并管理当前已知的历史 Skills？此操作不会替换或移动 Skill 文件。",
      "Automatically identify and manage known existing Skills? Skill files will not be replaced or moved."))) return;
    const result = await runOperation(t("自动管理已知 Skills", "Manage known Skills automatically"), async () => {
      await api.bootstrap();
      return true;
    }, t("已知 Skills 的来源记录已更新", "Source records for known Skills were updated"));
    if (result) await refresh();
  };
  const rerunDiagnostics = async () => {
    const result = await runOperation(t("运行环境诊断", "Run environment diagnostics"), api.diagnostics, t("环境诊断已完成", "Environment diagnostics completed"));
    if (result) setDiagnostics(result);
  };
  return <div className="settings-grid">
    <section className="panel language-settings"><PanelHead title={t("语言", "Language")} subtitle={t("选择应用界面语言", "Choose the application interface language")} />
      <div className="language-options" role="group" aria-label={t("界面语言", "Interface language")}>
        <button className={locale === "zh-CN" ? "active" : ""} onClick={() => void changeLocale("zh-CN")}>
          <Languages size={18} /><span><strong>简体中文</strong><small>Chinese (Simplified)</small></span>
          {locale === "zh-CN" && <CheckCircle2 size={17} />}
        </button>
        <button className={locale === "en-US" ? "active" : ""} onClick={() => void changeLocale("en-US")}>
          <Languages size={18} /><span><strong>English</strong><small>{t("英语", "English")}</small></span>
          {locale === "en-US" && <CheckCircle2 size={17} />}
        </button>
      </div>
      <small className="language-hint">{t("选择后立即切换并自动保存。", "The interface switches immediately and saves automatically.")}</small>
      <div className="theme-options" role="group" aria-label={t("颜色模式", "Color mode")}>
        {([
          ["system", Monitor, t("跟随系统", "System")],
          ["light", Sun, t("浅色", "Light")],
          ["dark", Moon, t("深色", "Dark")]
        ] as const).map(([value, Icon, label]) => <button key={value} className={theme === value ? "active" : ""}
          onClick={() => void changeTheme(value)} aria-pressed={theme === value}>
          <Icon size={18} /><span><strong>{label}</strong><small>{value === "system" ? t("自动匹配 Windows", "Use Windows setting") : value === "light" ? t("保持明亮背景", "Keep light surfaces") : t("降低夜间亮度", "Reduce night glare")}</small></span>
          {theme === value && <CheckCircle2 size={17} />}
        </button>)}
      </div>
    </section>
    <section className="panel settings-paths"><PanelHead title={t("存储位置", "Storage locations")} subtitle={t("设置 Skills、数据、日志和备份目录", "Set directories for Skills, data, logs, and backups")} />
      <div className="form">{pathFields.map(([label, key]) => <label key={key}><span>{label}</span>
        <input value={cfg.paths[key]} onChange={e => setCfg({ ...cfg, paths: { ...cfg.paths, [key]: e.target.value } })} /></label>)}</div>
      <div className="settings-save"><small>{t("路径变更保存后需重启应用", "Restart the application after saving path changes")}</small>
        <button className="primary compact" onClick={save}>{saved ? <CheckCircle2 size={15} /> : <Settings size={15} />}
          {saved ? t("已保存", "Saved") : t("保存设置", "Save settings")}</button></div>
    </section>
    <section className="panel"><PanelHead title={t("定时检查", "Scheduled checks")} subtitle={t("设置检查时间；不会自动更新", "Set a check time; updates are never installed automatically")} />
      <div className="schedule-form"><label className="switch-row"><span><strong>{t("启用计划任务", "Enable scheduled task")}</strong><small>{t("通过 Windows 任务计划程序运行", "Runs through Windows Task Scheduler")}</small></span>
        <input type="checkbox" checked={cfg.schedule.enabled} onChange={e => setCfg({ ...cfg, schedule: { ...cfg.schedule, enabled: e.target.checked } })} /></label>
        <label><span>{t("频率", "Frequency")}</span><select value={cfg.schedule.frequency} onChange={e => setCfg({ ...cfg, schedule: { ...cfg.schedule, frequency: e.target.value } })}><option value="daily">{t("每天", "Daily")}</option><option value="weekly">{t("每周", "Weekly")}</option></select></label>
        <label><span>{t("时间", "Time")}</span><input type="time" value={cfg.schedule.time} onChange={e => setCfg({ ...cfg, schedule: { ...cfg.schedule, time: e.target.value } })} /></label>
        <button className="ghost" disabled={scheduling} onClick={schedule}>
          {scheduling ? <LoaderCircle className="spin" size={16} /> : <Clock3 size={16} />}{scheduling ? t("正在应用…", "Applying…") : t("应用计划任务", "Apply schedule")}</button>
      </div>
    </section>
    <section className="panel"><PanelHead title={t("GitHub 连接", "GitHub connection")} subtitle={t("访问公共和私有仓库；凭据保存在 Windows 凭据管理器", "Access public and private repositories. Credentials are stored in Windows Credential Manager.")} />
      <div className="credential-form">
        <label><span>{t("用户名（可选）", "Username (optional)")}</span><input value={githubUser} onChange={e => setGitHubUser(e.target.value)} placeholder={t("GitHub 用户名", "GitHub username")} /></label>
        <label><span>Personal Access Token</span><input type="password" value={githubToken} onChange={e => setGitHubToken(e.target.value)} placeholder={t("输入后不会再次显示", "Hidden after saving")} /></label>
        <div className="credential-actions">
          <button className="ghost" disabled={!githubToken.trim()} onClick={saveToken}><KeyRound size={16} />{t("保存并验证", "Save and validate")}</button>
          <button className="ghost" onClick={validateGitHub}><ShieldCheck size={16} />{t("验证当前凭据", "Validate current credentials")}</button>
        </div>
        {githubStatus && <div className={`integration-status ${githubStatus.authenticated ? "ok" : "warning"}`}>
          <strong>{githubStatus.authenticated ? t(`已认证：${githubStatus.login}`, `Authenticated: ${githubStatus.login}`) : t("未认证", "Not authenticated")}</strong>
          <small>{githubStatus.authenticated
            ? t("GitHub 凭据有效", "GitHub credentials are valid")
            : !githubStatus.configured
              ? t("未配置 GitHub 凭据；公共请求将使用共享 IP 额度",
                "No GitHub credential is configured; public requests use the shared IP rate limit")
              : githubStatus.message}</small>
          {!!githubStatus.limit && <small>{t("API 剩余额度", "API requests remaining")}：{githubStatus.remaining}/{githubStatus.limit}
            {githubStatus.resetAt ? t(` · 重置于 ${formatDate(githubStatus.resetAt)}`, ` · Resets ${formatDate(githubStatus.resetAt)}`) : ""}</small>}
        </div>}
      </div>
    </section>
    <section className="panel codex-settings"><PanelHead title={t("Codex 风险复核", "Codex risk review")} subtitle={t("可选功能；默认关闭，结果需人工确认", "Optional and off by default; results require manual confirmation")} />
      <div className="schedule-form">
        <label className="switch-row"><span><strong>{t("启用 Codex CLI 复核", "Enable Codex CLI review")}</strong>
          <small>{t("使用当前 CLI 登录状态，按分组复核完整上下文", "Uses the current CLI login and reviews full context by group")}</small></span>
          <input type="checkbox" checked={cfg.codexReview.enabled}
            onChange={e => setCfg({ ...cfg, codexReview: { ...cfg.codexReview, enabled: e.target.checked } })} /></label>
        <label><span>{t("CLI 路径", "CLI path")}</span><input value={cfg.codexReview.cliPath || ""}
          onChange={e => setCfg({ ...cfg, codexReview: { ...cfg.codexReview, cliPath: e.target.value } })}
          placeholder={t("留空自动查找独立 codex.exe", "Leave blank to find standalone codex.exe automatically")} /></label>
        <label><span>{t("模型", "Model")}</span><select value={configuredModel}
          disabled={!codexStatus?.authenticated}
          onChange={e => changeCodexModel(e.target.value)}>
          <option value="default">{t("跟随 Codex 默认模型（推荐）", "Use Codex default model (recommended)")}</option>
          {configuredModelMissing && <option value={configuredModel}>{t(`${configuredModel}（当前配置，模型目录中未返回）`,
            `${configuredModel} (configured, not returned by model catalog)`)}</option>}
          {codexModels.map(model => <option key={model.slug} value={model.slug} title={model.description}>
            {model.displayName}{model.displayName === model.slug ? "" : ` · ${model.slug}`}
          </option>)}
        </select></label>
        <label><span>{t("推理强度", "Reasoning effort")}</span><select value={configuredEffort}
          onChange={e => setCfg({ ...cfg, codexReview: { ...cfg.codexReview, reasoningEffort: e.target.value } })}>
          {reasoningLevels.map(level => <option key={level} value={level}>{reasoningEffortLabel(level, t)}{level === "xhigh" && configuredModel === "gpt-5.6-luna" ? t("（默认）", " (default)") : ""}</option>)}
        </select></label>
        <div className="codex-batch-settings">
          <label><span>{t("同时复核的分组", "Groups reviewed in parallel")}</span><input type="number" min="1" max="4"
            value={cfg.codexReview.maxParallelBatches || 1}
            onChange={e => setCfg({ ...cfg, codexReview: { ...cfg.codexReview, maxParallelBatches: Number(e.target.value) } })} /></label>
        </div>
        <small className="codex-batch-hint">{t("同组 Skills 一起复核。建议保持串行，避免 CLI 冲突或限流。",
          "Skills in the same group are reviewed together. Serial review is recommended to avoid CLI conflicts or rate limits.")}</small>
        <div className="credential-actions codex-actions">
          <button className="ghost" onClick={validateCodex}><Stethoscope size={16} />{t("检查 CLI 与登录状态", "Check CLI and login")}</button>
          <button className="primary compact" onClick={save}><Settings size={15} />{t("保存复核设置", "Save review settings")}</button>
        </div>
        {codexStatus && <div className={`integration-status ${codexStatus.authenticated && codexStatus.compatible ? "ok" : "warning"}`}>
          <strong>{!codexStatus.available ? t("Codex CLI 不可用", "Codex CLI unavailable") :
            !codexStatus.authenticated ? t("Codex CLI 尚未登录", "Codex CLI is not logged in") :
              !codexStatus.compatible ? t("Codex CLI 已登录，但复核能力不兼容", "Codex CLI is logged in but incompatible with review") :
                t("Codex CLI 已登录且兼容", "Codex CLI is logged in and compatible")}</strong>
          {codexStatus.version && <small>{codexStatus.version}</small>}
          {!codexStatus.available && <small>{t(
            "请安装独立 Codex CLI，或在上方指定可执行文件路径。",
            "Install the standalone Codex CLI or specify its executable path above."
          )}</small>}
          {codexStatus.available && !codexStatus.authenticated && <small>{t(
            "请先在独立 Codex CLI 中完成登录。",
            "Sign in with the standalone Codex CLI first."
          )}</small>}
          {codexStatus.authenticated && !codexStatus.compatible && <small>{t(
            "请更新 Codex CLI，或检查当前自定义路径。",
            "Update Codex CLI or check the configured executable path."
          )}</small>}
          {codexStatus.authenticated && !!codexStatus.models?.length &&
            <small>{t(`已从当前 CLI 载入 ${codexStatus.models.length} 个可选模型。`,
              `Loaded ${codexStatus.models.length} available models from the current CLI.`)}</small>}
          {codexStatus.modelCatalogError && <small>{t("模型列表暂不可用：", "Model list unavailable: ")}{codexStatus.modelCatalogError}</small>}
          {!!codexStatus.missingCapabilities?.length &&
            <small>{t("缺少能力：", "Missing capabilities: ")}{codexStatus.missingCapabilities.join(locale === "en-US" ? ", " : "、")}</small>}
          {codexStatus.checkedAt && <small>{t("上次检查：", "Last checked: ")}{formatDate(codexStatus.checkedAt)}</small>}
          {codexStatus.path && <code title={codexStatus.path}>{codexStatus.path}</code>}
        </div>}
      </div>
    </section>
    <section className="panel"><PanelHead title={t("诊断", "Diagnostics")} subtitle={t("检查应用版本、目录和 CLI 环境", "Check the application version, directories, and CLI environment")} />
      <div className="diagnostics">
        <div className="diagnostic-version"><Sparkles size={19} /><span><small>{t("应用版本", "Application version")}</small><strong>{diagnostics?.version || t("读取中…", "Loading…")}</strong></span></div>
        {diagnostics && <div className="diagnostic-grid">
          <span>{t("Skills 目录", "Skills directory")}<b className={diagnostics.skillsRootExists ? "ok" : "bad"}>{diagnostics.skillsRootExists ? t("正常", "Ready") : t("不可用", "Unavailable")}</b></span>
          <span>{t("数据目录", "Data directory")}<b className={diagnostics.dataRootExists ? "ok" : "bad"}>{diagnostics.dataRootExists ? t("正常", "Ready") : t("不可用", "Unavailable")}</b></span>
          <code title={diagnostics.configPath}>{diagnostics.configPath}</code>
        </div>}
        <div className="diagnostic-actions">
          <button className="ghost" onClick={rerunDiagnostics}><Stethoscope size={16} />{t("重新诊断", "Run diagnostics again")}</button>
          <button className="ghost" onClick={bootstrap}><Sparkles size={16} />{t("自动管理已知 Skills", "Manage known Skills")}</button>
        </div>
      </div>
    </section>
  </div>;
}

function Empty({ text }: { text: string }) {
  return <div className="empty"><Gauge size={28} /><span>{text}</span></div>;
}
