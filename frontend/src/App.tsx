import { useEffect, useState } from "react";
import {
  ArchiveRestore, Boxes, CheckCircle2, ChevronRight, CircleAlert, Clock3,
  Download, FileClock, FolderGit2, Gauge, GitBranch, History, LayoutDashboard,
  Link2, ListRestart, LoaderCircle, Pencil, Plus, RefreshCw, RotateCcw, Search,
  Settings, ShieldCheck, ShieldAlert, Trash2, X, GripVertical, CheckSquare2,
  ArrowUpCircle, KeyRound, Stethoscope, Sparkles
} from "lucide-react";
import { api } from "./api";
import type { AdoptionPreview, CodexCLIStatus, CodexReviewProgress, Dashboard, Finding, Group, InstallPreview, RiskCluster, ScanReport, Skill, UpdateStatus } from "./types";

type Page = "overview" | "skills" | "groups" | "updates" | "security" | "history" | "quarantine" | "reports" | "settings";
type Operation = { label: string; detail: string; status: "running" | "success" | "error" };
type RunOperation = <T>(label: string, task: () => Promise<T>, successDetail?: string) => Promise<T | undefined>;

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

const nav: Array<{ id: Page; label: string; icon: any }> = [
  { id: "overview", label: "概览", icon: LayoutDashboard },
  { id: "skills", label: "Skills", icon: Boxes },
  { id: "groups", label: "分组与关系", icon: GitBranch },
  { id: "updates", label: "更新中心", icon: RefreshCw },
  { id: "security", label: "安全中心", icon: ShieldCheck },
  { id: "history", label: "历史与回滚", icon: History },
  { id: "quarantine", label: "隔离区", icon: ArchiveRestore },
  { id: "reports", label: "报告", icon: FileClock },
  { id: "settings", label: "设置", icon: Settings }
];

export default function App() {
  const [page, setPage] = useState<Page>("overview");
  const [data, setData] = useState<Dashboard | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [installOpen, setInstallOpen] = useState(false);
  const [selected, setSelected] = useState<string[]>([]);
  const [operation, setOperation] = useState<Operation | null>(null);

  const refresh = async () => {
    setLoading(true);
    try {
      setData(await api.dashboard());
      setError("");
    } catch (e: any) {
      setError(e?.message ?? String(e));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { void refresh(); }, []);

  const runOperation: RunOperation = async (label, task, successDetail = "操作已完成") => {
    setOperation({ label, detail: "正在处理，请稍候…", status: "running" });
    try {
      const value = await task();
      setOperation({ label, detail: successDetail, status: "success" });
      window.setTimeout(() => setOperation(current => current?.label === label && current.status === "success" ? null : current), 2600);
      return value;
    } catch (e: any) {
      const message = e?.message ?? String(e);
      setError(message);
      setOperation({ label, detail: message, status: "error" });
      return undefined;
    }
  };

  const title = nav.find(item => item.id === page)?.label ?? "概览";
  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="brand" title="Codex Skill Manager">
          <div className="brand-copy"><span>CODEX</span><strong>Skill Manager</strong><small>LOCAL · SAFE · REVERSIBLE</small></div>
        </div>
        <nav>
          {nav.map(item => {
            const Icon = item.icon;
            return <button key={item.id} className={page === item.id ? "active" : ""} onClick={() => setPage(item.id)}>
              <Icon size={18} /><span>{item.label}</span>
              {item.id === "security" && data?.riskCount ? <em>{data.riskCount}</em> : null}
            </button>;
          })}
        </nav>
        <div className="sidebar-foot">
          <span className="status-dot" />
          <div><strong>本地保护已启用</strong><small>不会上传 Skill 内容</small></div>
        </div>
      </aside>

      <main>
        <header>
          <div><p className="eyebrow">CODEX SKILLS</p><h1>{title}</h1></div>
          <div className="header-actions">
            <button className="ghost" disabled={loading} onClick={() => void runOperation("刷新 Skills 清单", refresh, "清单已刷新")}>
              <RefreshCw size={17} className={loading ? "spin" : ""} />{loading ? "刷新中…" : "刷新"}
            </button>
            <button className="primary" onClick={() => setInstallOpen(true)}><Download size={17} />安装 Skill</button>
          </div>
        </header>
        {error && <div className="error-banner"><CircleAlert size={18} />{error}<button onClick={() => setError("")}>×</button></div>}
        {operation && <OperationBanner operation={operation} dismiss={() => setOperation(null)} />}
        {loading && !data ? <Loading /> : data ? (
          <div className="content">
            {page === "overview" && <Overview data={data} onNavigate={setPage} />}
            {page === "skills" && <SkillsPage data={data} selected={selected} setSelected={setSelected} refresh={refresh} runOperation={runOperation} />}
            {page === "groups" && <GroupsPage data={data} refresh={refresh} runOperation={runOperation} />}
            {page === "updates" && <UpdatesPage data={data} refresh={refresh} runOperation={runOperation} />}
            <div className="persistent-page" hidden={page !== "security"}>
              <SecurityPage data={data} refresh={refresh} runOperation={runOperation} />
            </div>
            {page === "history" && <HistoryPage data={data} refresh={refresh} runOperation={runOperation} />}
            {page === "quarantine" && <QuarantinePage refresh={refresh} runOperation={runOperation} />}
            {page === "reports" && <ReportsPage data={data} />}
            {page === "settings" && <SettingsPage refresh={refresh} runOperation={runOperation} />}
          </div>
        ) : null}
      </main>
      {installOpen && <InstallDialog close={() => setInstallOpen(false)} refresh={refresh} runOperation={runOperation} />}
    </div>
  );
}

function OperationBanner({ operation, dismiss }: { operation: Operation; dismiss: () => void }) {
  return <div className={`operation-banner ${operation.status}`}>
    {operation.status === "running" ? <LoaderCircle className="spin" size={19} /> :
      operation.status === "success" ? <CheckCircle2 size={19} /> : <CircleAlert size={19} />}
    <div><strong>{operation.label}</strong><span>{operation.detail}</span></div>
    {operation.status !== "running" && <button onClick={dismiss}><X size={16} /></button>}
    {operation.status === "running" && <i />}
  </div>;
}

function Loading() {
  return <div className="loading"><LoaderCircle className="spin" /><span>正在读取本地 Skill 清单…</span></div>;
}

function Overview({ data, onNavigate }: { data: Dashboard; onNavigate: (p: Page) => void }) {
  const cards = [
    { label: "已管理", value: data.managedCount, detail: "来源和版本已锁定", icon: CheckCircle2, tone: "teal" },
    { label: "未管理", value: data.unmanagedCount, detail: "建议确认来源", icon: Link2, tone: "amber" },
    { label: "系统 Skills", value: data.systemCount, detail: "由 Codex 维护", icon: ShieldCheck, tone: "blue" },
    { label: "高风险报告", value: data.riskCount, detail: "需要人工审查", icon: ShieldAlert, tone: "red" }
  ];
  return <>
    <section className="hero">
      <div><span className="pill">LOCAL FIRST</span><h2>你的 Skills，清楚、可控、可回滚。</h2>
        <p>统一管理来源、版本、安全检查和更新记录。任何修改都先生成计划。</p>
      </div>
      <button onClick={() => onNavigate("updates")}><RefreshCw size={19} />检查全部更新</button>
    </section>
    <div className="stats">
      {cards.map(c => { const Icon = c.icon; return <article key={c.label}>
        <div className={`icon ${c.tone}`}><Icon size={20} /></div>
        <div><span>{c.label}</span><strong>{c.value}</strong><small>{c.detail}</small></div>
      </article>; })}
    </div>
    <div className="grid-two">
      <section className="panel">
        <PanelHead title="管理分组" subtitle="自动识别来源，也可按需要手动整理" action="查看关系" onClick={() => onNavigate("groups")} />
        <div className="group-list">{data.groups.slice(0, 5).map(g => <GroupRow key={g.id} group={g} />)}</div>
      </section>
      <section className="panel">
        <PanelHead title="最近活动" subtitle="安装、更新和回滚事务" action="完整历史" onClick={() => onNavigate("history")} />
        <Timeline data={data.recentHistory.slice(0, 5)} />
      </section>
    </div>
  </>;
}

function PanelHead({ title, subtitle, action, onClick }: { title: string; subtitle: string; action?: string; onClick?: () => void }) {
  return <div className="panel-head"><div><h3>{title}</h3><p>{subtitle}</p></div>
    {action && <button onClick={onClick}>{action}<ChevronRight size={15} /></button>}</div>;
}

function GroupRow({ group }: { group: Group }) {
  return <div className="group-row"><div className="repo-icon"><FolderGit2 size={19} /></div>
    <div className="grow"><strong>{group.name}</strong><span>{group.provider === "github" ? group.repository : group.provider}</span></div>
    <div className="skill-stack">{group.skillNames.slice(0, 3).map(n => <i key={n}>{n.slice(0, 1).toUpperCase()}</i>)}</div>
    <b>{group.skillNames.length}</b><small>Skills</small></div>;
}

function Timeline({ data }: { data: Dashboard["recentHistory"] }) {
  if (!data.length) return <Empty text="暂无管理事务" />;
  return <div className="timeline">{data.map(tx => <div key={tx.id}><span className={tx.status} />
    <div><strong>{tx.type} · {tx.targets.join("、") || "—"}</strong><small>{new Date(tx.startedAt).toLocaleString("zh-CN")}</small></div>
    <em>{tx.status}</em></div>)}</div>;
}

function SkillsPage({ data, selected, setSelected, refresh, runOperation }: {
  data: Dashboard; selected: string[]; setSelected: (s: string[]) => void; refresh: () => Promise<void>; runOperation: RunOperation;
}) {
  const [query, setQuery] = useState("");
  const [working, setWorking] = useState(false);
  const [adoption, setAdoption] = useState<AdoptionPreview | null>(null);
  const filtered = data.skills.filter(s => (s.name + s.description + s.groupName).toLowerCase().includes(query.toLowerCase()));
  const selectable = filtered.filter(skill => !skill.system).map(skill => skill.name);
  const unmanagedSelected = selected.filter(name => data.skills.some(skill => skill.name === name && !skill.managed && !skill.system));
  const toggle = (name: string) => setSelected(selected.includes(name) ? selected.filter(n => n !== name) : [...selected, name]);
  const selectAll = () => setSelected(Array.from(new Set([...selected, ...selectable])));
  const invert = () => setSelected(Array.from(new Set([
    ...selected.filter(name => !selectable.includes(name)),
    ...selectable.filter(name => !selected.includes(name))
  ])));
  const clear = () => setSelected([]);
  const remove = async () => {
    if (!selected.length || !confirm(`将 ${selected.length} 个 Skills 移动到隔离区？不会永久删除。`)) return;
    setWorking(true);
    try {
      const result = await runOperation("移动 Skills 到隔离区", () => api.quarantine(selected), "已安全移入隔离区");
      if (result) { setSelected([]); await refresh(); }
    } finally { setWorking(false); }
  };
  const analyzeAdoption = async () => {
    if (!unmanagedSelected.length) return;
    setWorking(true);
    try {
      const preview = await runOperation("分析未管理 Skills", () => api.prepareAdoption(unmanagedSelected), "分析完成，可以确认管理");
      if (preview) setAdoption(preview);
    } finally { setWorking(false); }
  };
  const auditSelected = async () => {
    if (selected.length !== 1) return;
    setWorking(true);
    try {
      const report = await runOperation(`扫描 ${selected[0]}`, () => api.audit(selected[0]), "安全扫描已完成，可在安全中心查看");
      if (report) await refresh();
    } finally { setWorking(false); }
  };
  return <><section className="panel full">
    <div className="toolbar skills-toolbar"><div className="search"><Search size={17} /><input value={query} onChange={e => setQuery(e.target.value)} placeholder="搜索名称、描述或分组…" /></div>
      <div className="selection-tools">
        <button className="ghost" onClick={selectAll} disabled={!selectable.length}><CheckSquare2 size={15} />全选当前</button>
        <button className="ghost" onClick={invert} disabled={!selectable.length}><ListRestart size={15} />反选当前</button>
        <button className="ghost" onClick={clear} disabled={!selected.length}><X size={15} />清空</button>
      </div>
      {selected.length > 0 && <div className="selection"><span>已选 {selected.length}</span>
        {selected.length === 1 && <button className="ghost" onClick={auditSelected} disabled={working}>
          <ShieldCheck size={16} />扫描此 Skill
        </button>}
        {unmanagedSelected.length > 0 && <button className="ghost adopt-button" onClick={analyzeAdoption} disabled={working}>
          {working ? <LoaderCircle className="spin" size={16} /> : <ShieldCheck size={16} />}分析并管理 {unmanagedSelected.length} 个
        </button>}
        <button className="danger" onClick={remove} disabled={working}><Trash2 size={16} />{working ? "处理中…" : "移至隔离区"}</button></div>}
    </div>
    <div className="table">
      <div className="tr th"><span /><span>Skill</span><span>来源分组</span><span>状态</span><span>版本</span></div>
      {filtered.map(skill => <div className="tr" key={skill.name}>
        <input type="checkbox" disabled={skill.system} checked={selected.includes(skill.name)} onChange={() => toggle(skill.name)} />
        <DetailCell summary={<span className="skill-text"><strong>{skill.name}</strong><small>{skill.description}</small></span>}
          rows={[["名称", skill.name], ["说明", skill.description], ["路径", skill.path], ["文件数量", String(skill.files?.length ?? 0)]]} />
        <DetailCell summary={<span><b>{skill.groupName}</b><small>{skill.sourceRepository || skill.sourceGroupName || (skill.system ? "Codex" : "本地")}</small></span>}
          rows={[
            ["当前分组", skill.groupName],
            ["真实来源分组", skill.sourceGroupName || "尚未识别"],
            ["来源类型", skill.sourceProvider || "unknown"],
            ["仓库", skill.sourceRepository || "无"],
            ["仓库内路径", skill.sourcePath || "无"],
            ["识别依据", skill.sourceEvidence || "无"],
            ["识别置信度", `${Math.round((skill.sourceConfidence || 0) * 100)}%`]
          ]} />
        <DetailCell summary={<Status skill={skill} />} rows={[
          ["管理状态", skill.system ? "系统 Skill" : skill.managed ? "已管理" : "未管理"],
          ["本地修改", skill.localModified ? "检测到修改" : "未检测到修改"],
          ["安全状态", skill.securityStatus || "未扫描"],
          ["更新状态", skill.updateStatus || "未知"]
        ]} />
        <DetailCell summary={<code>{skill.installedCommit?.slice(0, 8) || "—"}</code>}
          rows={[["完整 Commit", skill.installedCommit || "尚未记录"], ["来源路径", skill.sourcePath || "无"]]} />
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
  const [selected, setSelected] = useState(preview.skills.map(skill => skill.name));
  const [working, setWorking] = useState(false);
  const [reviewing, setReviewing] = useState("");
  const [scan, setScan] = useState(preview.scan);
  const toggleCluster = async (cluster: RiskCluster) => {
    setReviewing(cluster.id);
    try {
      await api.setRiskClusterIgnored(cluster, !cluster.ignored, "", true);
      setScan(current => updateClusterState(current, cluster.id, !cluster.ignored, ""));
    } finally {
      setReviewing("");
    }
  };
  const ignoreAll = async (clusters: RiskCluster[]) => {
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
      const result = await runOperation("管理现有 Skills", () => api.applyAdoption(preview.id, selected), "已建立来源快照并完成管理");
      if (result) { onCompleted(); await refresh(); close(); }
    } finally { setWorking(false); }
  };
  return <div className="modal-backdrop"><div className="modal adoption-modal">
    <div className="modal-head"><div><p className="eyebrow">MANAGE EXISTING</p><h2>分析并管理</h2></div><button onClick={close}><X /></button></div>
    <div className="notice"><ShieldCheck size={20} /><span><strong>只建立管理快照，不移动 Skill 文件</strong>
      <small>工具会自动识别真实来源并分组；无法确认 GitHub 来源时，会建立独立本地分组。</small></span></div>
    <div className="candidate-list">{preview.skills.map(skill => <label key={skill.name}>
      <input type="checkbox" checked={selected.includes(skill.name)}
        onChange={() => setSelected(selected.includes(skill.name) ? selected.filter(name => name !== skill.name) : [...selected, skill.name])} />
      <span><strong>{skill.name}</strong><small>{skill.description}</small>
        {(() => { const source = preview.sources?.find(item => item.skillName === skill.name); return source ? <>
          <small className="source-detected">自动分组：{source.groupName} · 置信度 {Math.round(source.confidence * 100)}%</small>
          <code>{source.repository || source.sourcePath}</code><small>{source.evidence}</small>
        </> : <code>{skill.path}</code>; })()}
      </span>
    </label>)}</div>
    <ScanSummary report={scan} compact />
    <FindingDetails report={scan} reviewing={reviewing} onToggle={toggleCluster} onIgnoreAll={ignoreAll} />
    <div className="modal-actions"><button className="ghost" onClick={close}>取消</button>
      <button className="primary" disabled={working || reviewing !== "" || selected.length === 0} onClick={apply}>
        {working ? <LoaderCircle className="spin" size={17} /> : <CheckCircle2 size={17} />}{working ? "正在管理…" : "确认管理"}
      </button></div>
  </div></div>;
}

function Status({ skill }: { skill: Skill }) {
  if (skill.system) return <span className="badge blue">系统</span>;
  if (skill.localModified) return <span className="badge amber">本地修改</span>;
  if (!skill.managed) return <span className="badge gray">未管理</span>;
  return <span className="badge green">受保护</span>;
}

function DetailCell({ summary, rows }: { summary: React.ReactNode; rows: Array<[string, string]> }) {
  return <div className="detail-cell" tabIndex={0}>
    {summary}
    <div className="detail-popover" role="tooltip">
      {rows.map(([label, value]) => <div key={label}><b>{label}</b><span>{value || "—"}</span></div>)}
    </div>
  </div>;
}

function GroupsPage({ data, refresh, runOperation }: { data: Dashboard; refresh: () => Promise<void>; runOperation: RunOperation }) {
  const [active, setActive] = useState(data.groups[0]?.id ?? "");
  const [working, setWorking] = useState(false);
  const group = data.groups.find(g => g.id === active) ?? data.groups[0];
  useEffect(() => {
    if (!data.groups.some(item => item.id === active)) setActive(data.groups[0]?.id ?? "");
  }, [data.groups, active]);
  const create = async () => {
    const name = window.prompt("请输入新分组名称：", "")?.trim();
    if (!name) return;
    setWorking(true);
    try {
      const result = await runOperation("新建管理分组", () => api.createGroup(name), `分组“${name}”已创建`);
      if (result) await refresh();
    } finally { setWorking(false); }
  };
  const rename = async (target: Group) => {
    const name = window.prompt("请输入新的分组名称：", target.name)?.trim();
    if (!name || name === target.name) return;
    setWorking(true);
    try {
      const result = await runOperation("重命名管理分组", () => api.renameGroup(target.id, name), `分组已改名为“${name}”`);
      if (result) await refresh();
    } finally { setWorking(false); }
  };
  const dropOnGroup = async (event: React.DragEvent, target: Group) => {
    event.preventDefault();
    const skillName = event.dataTransfer.getData("application/x-csm-skill");
    if (skillName) {
      if (target.readOnly) return;
      setWorking(true);
      try {
        const result = await runOperation("移动 Skill 到分组", () => api.moveSkills([skillName], target.id), `${skillName} 已移入“${target.name}”`);
        if (result) { setActive(target.id); await refresh(); }
      } finally { setWorking(false); }
      return;
    }
    const draggedID = event.dataTransfer.getData("application/x-csm-group");
    if (!draggedID || draggedID === target.id || target.readOnly) return;
    const editable = data.groups.filter(item => !item.readOnly);
    const from = editable.findIndex(item => item.id === draggedID);
    const to = editable.findIndex(item => item.id === target.id);
    if (from < 0 || to < 0) return;
    const reordered = [...editable];
    const [moved] = reordered.splice(from, 1);
    reordered.splice(to, 0, moved);
    setWorking(true);
    try {
      const result = await runOperation("调整分组顺序", () => api.reorderGroups(reordered.map(item => item.id)), "分组顺序已保存");
      if (result) await refresh();
    } finally { setWorking(false); }
  };
  return <div className="groups-layout">
    <section className="panel group-nav">
      <div className="group-nav-head"><div><h3>管理分组</h3><p>拖动分组排序，拖动 Skill 更换分组</p></div>
        <button className="icon-button" title="新建分组" disabled={working} onClick={create}><Plus size={17} /></button></div>
      {data.groups.map(g => <div key={g.id} className={`group-nav-item ${active === g.id ? "active" : ""}`}
        draggable={!g.readOnly && !working}
        onDragStart={event => event.dataTransfer.setData("application/x-csm-group", g.id)}
        onDragOver={event => { if (!g.readOnly) event.preventDefault(); }}
        onDrop={event => void dropOnGroup(event, g)}>
        {!g.readOnly ? <GripVertical className="drag-handle" size={16} /> : <ShieldCheck size={16} />}
        <button className="group-select" onClick={() => setActive(g.id)}>
          <span><strong>{g.name}</strong><small>{g.skillNames.length} Skills · {g.manual ? "手动分组" : "来源分组"}</small></span>
        </button>
        {!g.readOnly && <button className="group-rename" title="重命名" onClick={() => void rename(g)}><Pencil size={14} /></button>}
      </div>)}
    </section>
    <section className="panel relation-panel"><PanelHead title={group?.name || "分组详情"} subtitle="Skill 可直接拖动到左侧其他分组" />
      {group ? <>
        <div className="group-skill-list">{group.skillNames.length ? group.skillNames.map(name => {
          const skill = data.skills.find(item => item.name === name);
          return <article key={name} draggable={!skill?.system && !working}
            onDragStart={event => event.dataTransfer.setData("application/x-csm-skill", name)}>
            <GripVertical size={15} /><div><strong>{name}</strong><small>{skill?.description || "暂无说明"}</small></div>
            <span>{skill?.sourceGroupName && skill.sourceGroupName !== group.name ? `来源：${skill.sourceGroupName}` : skill?.sourceProvider}</span>
          </article>;
        }) : <Empty text="这个分组暂时为空，可将 Skill 拖入" />}</div>
      </> : <Empty text="暂无分组" />}
    </section>
  </div>;
}

function UpdatesPage({ data, refresh, runOperation }: { data: Dashboard; refresh: () => Promise<void>; runOperation: RunOperation }) {
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
      const checked = await runOperation("检查 GitHub 更新", api.check, "更新检查已完成");
      if (checked) { setStatuses(checked.statuses); await refresh(); }
    } finally { setWorking(false); }
  };
  const retry = async (groupId: string) => {
    setWorking(true);
    try {
      const checked = await runOperation(
        "重试 GitHub 更新检查",
        () => api.checkSelected([groupId], true),
        "该来源已重新检查"
      );
      if (checked) { setStatuses(checked.statuses); await refresh(); }
    } finally { setWorking(false); }
  };
  const prepare = async (groups: Group[]) => {
    if (!groups.length) return;
    setWorking(true);
    try {
      const outcome = await runOperation(`准备 ${groups.length} 个来源的更新计划`, async () => {
        const ready: Array<{ group: Group; value: InstallPreview }> = [];
        const failures: string[] = [];
        for (const group of groups) {
          try {
            ready.push({ group, value: await api.prepareUpdate(group.id) });
          } catch (error: any) {
            failures.push(`${group.name}：${error?.message ?? String(error)}`);
          }
        }
        return { ready, failures };
      }, "更新范围已核对，安全扫描已完成");
      if (outcome) {
        setPrepareFailures(outcome.failures);
        setPreviews(outcome.ready);
      }
    } finally { setWorking(false); }
  };
  const statusByGroup = new Map(statuses.map(status => [status.groupId, status]));
  const updateGroups = data.sourceGroups.filter(group => group.provider !== "system");
  const availableGroups = updateGroups.filter(group => {
    const status = statusByGroup.get(group.id);
    return status?.status === "update-available" ||
      ((status?.status === "error" || status?.status === "rate-limited") && status.lastSuccessStatus === "update-available");
  });
  const selectableIds = availableGroups.map(group => group.id);
  const selectAll = () => setSelectedGroups(selectableIds);
  const invert = () => setSelectedGroups(selectableIds.filter(id => !selectedGroups.includes(id)));
  const clear = () => setSelectedGroups([]);
  const selectedAvailableGroups = availableGroups.filter(group => selectedGroups.includes(group.id));
  const lastChecked = statuses.reduce<string | undefined>((latest, status) =>
    !latest || new Date(status.checkedAt) > new Date(latest) ? status.checkedAt : latest, data.lastUpdateCheck);
  return <section className="panel full">
    <div className="update-hero"><div className="round-icon"><RefreshCw size={28} /></div><div><h2>先检查，再计划，最后更新</h2>
      <p>远端版本会解析为不可变 Commit。发现更新后，可选择单个或多个 Skills，审查安全结果再应用。</p>
      <small>{lastChecked ? `上次检查：${new Date(lastChecked).toLocaleString("zh-CN")}` : "尚未执行过更新检查"}</small></div>
      <button className="primary" onClick={check} disabled={working}>{working ? <LoaderCircle className="spin" size={17} /> : <RefreshCw size={17} />}{working ? "正在检查…" : "检查更新"}</button></div>
    {availableGroups.length > 0 && <div className="update-toolbar">
      <div><strong>批量更新</strong><span>仅选择已发现新版本的来源；每个来源都有独立备份和回滚事务。</span></div>
      <div className="selection-tools">
        <button className="ghost" onClick={selectAll}><CheckSquare2 size={15} />全选可更新</button>
        <button className="ghost" onClick={invert}><ListRestart size={15} />反选</button>
        <button className="ghost" onClick={clear} disabled={!selectedGroups.length}><X size={15} />清空</button>
        <button className="primary" disabled={working || !selectedAvailableGroups.length} onClick={() => void prepare(selectedAvailableGroups)}>
          {working ? <LoaderCircle className="spin" size={16} /> : <ArrowUpCircle size={16} />}
          审查所选 {selectedAvailableGroups.length} 个来源
        </button>
      </div>
    </div>}
    {prepareFailures.length > 0 && <div className="prepare-failures"><CircleAlert size={17} /><div><strong>部分更新计划未能准备</strong>
      {prepareFailures.map(message => <small key={message}>{message}</small>)}</div><button onClick={() => setPrepareFailures([])}><X size={15} /></button></div>}
    <div className="update-list">{updateGroups.length === 0 ? <Empty text="暂无可检查的个人 Skill 来源" /> : updateGroups.map(g => {
      const status = statusByGroup.get(g.id);
      const presentation = updatePresentation(status);
      return <article key={g.id} className={`update-item ${presentation.tone}`}>
        <input className="update-check" type="checkbox" disabled={!availableGroups.some(group => group.id === g.id) || working}
          checked={selectedGroups.includes(g.id)}
          onChange={() => setSelectedGroups(selectedGroups.includes(g.id) ? selectedGroups.filter(id => id !== g.id) : [...selectedGroups, g.id])} />
        <div className={`update-state-icon ${presentation.tone}`}>{presentation.icon}</div>
        <div className="update-copy"><strong>{g.name}</strong><span>{g.skillNames.length} Skills · {g.repository || g.provider}</span>
          <small>{status ? updateDetail(status) : "点击“检查更新”获取当前状态"}</small>
          {status?.error && <small className="update-error">{status.error}</small>}
          {status?.status === "rate-limited" && status.retryAt && <small className="rate-limit-countdown">
            GitHub 限额恢复倒计时：{formatCountdown(new Date(status.retryAt).getTime() - clock)}
          </small>}
        </div>
        <div className="update-actions"><span className={`badge ${presentation.badge}`}>{presentation.label}</span>
          {status?.status === "update-available" && <button className="ghost compact" disabled={working} onClick={() => void prepare([g])}>
            <ShieldCheck size={15} />单独审查
          </button>}
          {(status?.status === "error" || status?.status === "rate-limited") &&
            <button className="ghost compact" disabled={working}
              onClick={() => void retry(g.id)}>
              <RefreshCw size={15} />重新检查
            </button>}
        </div>
      </article>;
    })}</div>
    {previews.length > 0 && <UpdateDialog items={previews} close={() => setPreviews([])}
      refresh={refresh} />}
  </section>;
}

function updatePresentation(status?: UpdateStatus) {
  if (!status) return { label: "尚未检查", tone: "unknown", badge: "gray", icon: <Clock3 size={19} /> };
  if (status.status === "up-to-date") return { label: "已是最新", tone: "current", badge: "green", icon: <CheckCircle2 size={19} /> };
  if (status.status === "update-available") return { label: "发现新版本", tone: "available", badge: "amber", icon: <ArrowUpCircle size={19} /> };
  if (status.status === "error") return { label: "检查失败", tone: "failed", badge: "red", icon: <CircleAlert size={19} /> };
  if (status.status === "rate-limited") return { label: "GitHub 已限流", tone: "failed", badge: "red", icon: <Clock3 size={19} /> };
  return { label: "不支持在线更新", tone: "unsupported", badge: "gray", icon: <Link2 size={19} /> };
}

function updateDetail(status: UpdateStatus) {
  const checked = new Date(status.checkedAt).toLocaleString("zh-CN");
  if (status.status === "update-available") {
    return `${status.outdatedSkills.length} 个 Skill 可更新：${status.outdatedSkills.join("、")} · 检查于 ${checked}`;
  }
  if (status.status === "up-to-date") {
    return `本地与远端一致 · Commit ${status.remoteCommit?.slice(0, 10) || "未知"} · 检查于 ${checked}`;
  }
  if (status.status === "unsupported") return `本地来源没有可检查的 GitHub 版本 · 检查于 ${checked}`;
  if (status.status === "rate-limited") {
    const previous = status.lastSuccessStatus === "update-available" ? "上次成功检查发现新版本，仍可选中" :
      status.lastSuccessStatus === "up-to-date" ? "上次成功检查时已是最新" : "没有可用的历史成功状态";
    return `${previous} · 本次检查于 ${checked}`;
  }
  if (status.status === "error" && status.lastSuccessAt) {
    return `本次检查失败；保留 ${new Date(status.lastSuccessAt).toLocaleString("zh-CN")} 的成功状态`;
  }
  return `未能取得远端状态 · 检查于 ${checked}`;
}

function formatCountdown(milliseconds: number): string {
  if (milliseconds <= 0) return "可以重试";
  const seconds = Math.ceil(milliseconds / 1000);
  const minutes = Math.floor(seconds / 60);
  const remainder = seconds % 60;
  return `${minutes}分${remainder.toString().padStart(2, "0")}秒`;
}

function UpdateDialog({ items, close, refresh }: {
  items: Array<{ group: Group; value: InstallPreview }>; close: () => void; refresh: () => Promise<void>;
}) {
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
    setReviewing(cluster.id);
    try {
      await api.setRiskClusterIgnored(cluster, !cluster.ignored, "", true);
      setScans(current => ({
        ...current,
        [planID]: updateClusterState(current[planID], cluster.id, !cluster.ignored, "")
      }));
    } catch (error: any) {
      setFailures(current => [`风险核查记录失败：${error?.message ?? String(error)}`, ...current]);
    } finally {
      setReviewing("");
    }
  };
  const ignoreAll = async (planID: string, clusters: RiskCluster[]) => {
    if (!clusters.length) return;
    setReviewing("manual-batch");
    try {
      await api.setRiskClustersIgnored(clusters, true, "");
      setScans(current => ({ ...current, [planID]: updateClustersState(current[planID], clusters, true, "") }));
    } catch (error: any) {
      setFailures(current => [`一键忽略失败：${error?.message ?? String(error)}`, ...current]);
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
      setFailures(current => [`Codex 风险复核失败：${error?.message ?? String(error)}`, ...current]);
    } finally {
      setCodexWorking(false);
    }
  };
  const applyCodexSuggestions = async (planID: string, clusters: RiskCluster[]) => {
    const scan = scans[planID];
    if (!confirmCodexSuggestions(scan, clusters)) return;
    setReviewing("codex-batch");
    try {
      const reason = codexBatchReason(scan, clusters);
      await api.setRiskClustersIgnored(clusters, true, reason);
      setScans(current => ({ ...current, [planID]: updateClustersState(scan, clusters, true, reason) }));
    } catch (error: any) {
      setFailures(current => [`Codex 建议采纳失败：${error?.message ?? String(error)}`, ...current]);
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
        setProgress(`正在更新 ${group.name}（${attempted}/${targets.length}）`);
        try {
          await api.apply(value.id, selected[value.id], false);
          succeeded.push(value.id);
        } catch (error: any) {
          errors.push(`${group.name}：${error?.message ?? String(error)}`);
        }
      }
      if (succeeded.length) {
        setSelected(current => ({ ...current, ...Object.fromEntries(succeeded.map(id => [id, []])) }));
      }
      setProgress("正在重新核对更新状态…");
      await api.check();
      await refresh();
      setFailures(errors);
      if (!errors.length) close();
    } catch (error: any) {
      setFailures([`状态核对失败：${error?.message ?? String(error)}。已完成的事务可在“历史与回滚”中确认。`]);
    } finally { setWorking(false); setProgress(""); }
  };
  return <div className="modal-backdrop"><div className="modal update-modal batch-update-modal">
    <div className="modal-head"><div><p className="eyebrow">SAFE BATCH UPDATE</p><h2>审查并选择要更新的 Skills</h2>
      <small>{items.length} 个来源 · 已选择 {selectedCount} 个 Skills</small></div><button onClick={close} disabled={working}><X /></button></div>
    <div className="update-plan-list">{items.map(({ group, value }) => {
      const names = selected[value.id] ?? [];
      const scan = scans[value.id];
      const blocking = ["critical", "high"].includes(scan.activeHighestSeverity);
      return <section className="update-plan" key={value.id}>
        <div className="repo-summary update-repo"><FolderGit2 size={24} /><div><strong>{group.name}</strong>
          <span>{value.repository.resolvedRef} · Commit {value.repository.commitSha.slice(0, 12)} · 仅扫描本次写入的 {scan.filesScanned} 个文件</span></div>
          <span className={`severity ${scan.activeHighestSeverity}`}>{severityLabel(scan.activeHighestSeverity)}</span></div>
        <div className="candidate-tools">
          <button onClick={() => setSelected({ ...selected, [value.id]: value.skills.map(skill => skill.name) })}>全选</button>
          <button onClick={() => setSelected({ ...selected, [value.id]: value.skills.filter(skill => !names.includes(skill.name)).map(skill => skill.name) })}>反选</button>
          <button onClick={() => setSelected({ ...selected, [value.id]: [] })} disabled={!names.length}>清空</button>
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
          仍有未忽略的高风险或严重风险。可以直接使用“一键忽略全部警告”；处理完成前不会执行更新。</div>}
      </section>;
    })}</div>
    {progress && <div className="batch-progress"><LoaderCircle className="spin" size={17} /><span>{progress}</span></div>}
    {failures.length > 0 && <div className="prepare-failures"><CircleAlert size={17} /><div><strong>部分来源更新失败，已完成的来源仍可单独回滚</strong>
      {failures.map(message => <small key={message}>{message}</small>)}</div></div>}
    <div className="modal-actions"><button className="ghost" onClick={close}>取消</button>
      <button className="primary" disabled={working || reviewing !== "" || hasBlockingWarnings || selectedCount === 0} onClick={apply}>
        {working ? <LoaderCircle className="spin" size={17} /> : <ArrowUpCircle size={17} />}
        {working ? "正在更新…" : `更新选中的 ${selectedCount} 个`}
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
  const clusters = [...(report.clusters ?? [])].sort((a, b) => {
    if (a.ignored !== b.ignored) return a.ignored ? 1 : -1;
    return severityRank(b.severity) - severityRank(a.severity);
  });
  const codexByCluster = new Map((report.codexReview?.reviews ?? []).map(review => [review.clusterId, review]));
  const suggestedClusters = codexSuggestedClusters(report);
  const activeClusters = clusters.filter(cluster => !cluster.ignored);
  return <>
    {clusters.length ? <RiskOverview report={report} /> :
      <div className="scan-clean"><CheckCircle2 size={16} />本地规则未发现警告，仍可使用 Codex 做完整上下文复核</div>}
    {onIgnoreAll && activeClusters.length > 0 && <div className="manual-review-action"><div>
      <strong>人工决定优先</strong>
      <small>无需 Codex 复核，也无需填写原因；一次忽略当前报告中全部 {activeClusters.length} 个待处理风险簇。</small>
    </div><button className="primary compact" disabled={reviewing !== "" || codexWorking}
      onClick={() => void onIgnoreAll(activeClusters)}>
      {reviewing === "manual-batch" ? <LoaderCircle className="spin" size={14} /> : <CheckSquare2 size={14} />}
      {reviewing === "manual-batch" ? "正在记录…" : "一键忽略全部警告"}
    </button></div>}
    {onCodexReview && <div className="codex-review-action"><div><strong>需要快速归纳大量警告？</strong>
      <small>Codex 会按分组复核完整上下文；本地规则只提供简短概览。</small></div>
      <button className="ghost" disabled={codexWorking} onClick={() => void onCodexReview()}>
        {codexWorking ? <LoaderCircle className="spin" size={15} /> : <Sparkles size={15} />}
        {codexWorking ? "Codex 正在复核…" : report.codexReview ? "重新用 Codex 复核" : "使用 Codex 辅助复核"}
      </button></div>}
    {codexWorking && codexProgress && <CodexProgressCard progress={codexProgress} />}
    {report.codexReview && <div className={`codex-review-summary ${report.codexReview.status}`}>
      <Sparkles size={18} /><div><strong>Codex 辅助复核</strong>
        <p>{report.codexReview.summary || report.codexReview.error}</p>
        <small>{report.codexReview.model === "default" ? "Codex 默认先进模型" : report.codexReview.model} ·
          推理强度 {report.codexReview.reasoningEffort} ·
          {report.codexReview.contextMode === "full-target-read-only"
            ? ` 完整目录上下文（${report.codexReview.contextFileCount || 0} 个文件）`
            : " 规则摘要上下文"}</small>
        {onApplyCodexSuggestions && ["completed", "partial"].includes(report.codexReview.status) && suggestedClusters.length > 0 &&
          <button className="primary compact codex-apply-suggestions" disabled={reviewing !== ""}
            onClick={() => void onApplyCodexSuggestions(suggestedClusters)}>
            {reviewing === "codex-batch" ? <LoaderCircle className="spin" size={14} /> : <CheckSquare2 size={14} />}
            {reviewing === "codex-batch" ? "正在记录…" : `一键采纳 ${suggestedClusters.length} 个建议`}
          </button>}
        <small className="codex-baseline-note">Codex 结论仅供参考；所有级别最终均可由人工直接决定。</small></div>
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
  const summaries = report.skills?.length ? report.skills : Array.from(new Map(clusters.map(cluster => {
    const skillName = cluster.skillName || "未识别 Skill";
    return [skillName, {
      skillName, sourcePath: skillName, groupId: cluster.groupId || "ungrouped",
      groupName: cluster.groupName || "未分组", filesScanned: 0,
      highestSeverity: cluster.severity, activeFindingCount: 0, ignoredFindingCount: 0
    }];
  })).values());
  const groups = new Map<string, { name: string; skills: typeof summaries }>();
  for (const skill of summaries) {
    const key = skill.groupId || skill.groupName || "ungrouped";
    const group = groups.get(key) ?? { name: skill.groupName || "未分组", skills: [] };
    group.skills.push(skill);
    groups.set(key, group);
  }
  return <div className="grouped-risks">
    {[...groups.entries()].map(([groupId, group]) => {
      const groupClusters = clusters.filter(cluster => (cluster.groupId || "ungrouped") === groupId ||
        (!cluster.groupId && group.skills.some(skill => skill.skillName === cluster.skillName)));
      const active = groupClusters.filter(cluster => !cluster.ignored).length;
      return <details key={groupId} className="risk-group">
        <summary className="risk-group-summary"><div><strong>{group.name}</strong>
          <small>{group.skills.length} 个 Skill · {active} 个待处理警告 · {groupClusters.length - active} 个已忽略</small></div>
          <span className={`badge ${active ? "red" : "green"}`}>{active ? "需要核查" : "已处理"}</span></summary>
        <div className="risk-group-skills">{group.skills.map(skill => {
          const skillClusters = groupClusters.filter(cluster => cluster.skillName === skill.skillName ||
            (!cluster.skillName && group.skills.length === 1));
          const skillActive = skillClusters.filter(cluster => !cluster.ignored).length;
          return <details key={`${groupId}:${skill.skillName}`} className="risk-skill">
            <summary><span><strong>{skill.skillName}</strong><small>{skill.sourcePath}</small></span>
              <span>{skillActive} 待处理 / {skillClusters.length} 总计</span></summary>
            <div className="finding-details">{skillClusters.length ? skillClusters.map(cluster => {
              const codex = codexByCluster.get(cluster.id);
              return <article key={cluster.id} className={cluster.ignored ? "ignored" : ""}>
                <span className={`severity ${cluster.ignored ? "ignored" : cluster.severity}`}>
                  {cluster.ignored ? "已忽略" : severityLabel(cluster.severity)}
                </span>
                <div><strong>{cluster.title}{cluster.deterministic && <em className="hard-baseline">确定性规则</em>}</strong>
                  <small>{cluster.ruleId} · {fileClassLabel(cluster.fileClass)} · {cluster.findingCount} 条命中 · {cluster.affectedFiles.length} 个文件</small>
                  {cluster.sampleFindings[0] && <p>{cluster.sampleFindings[0].explanation}</p>}
                  <details className="cluster-evidence"><summary>查看代表性证据与文件</summary>
                    {cluster.sampleFindings.map(finding => <code key={finding.fingerprint}>
                      {finding.file}:{finding.line || 1}　{finding.evidence || "文件级风险"}
                    </code>)}
                  </details>
                  {codex && <div className="codex-cluster"><Sparkles size={14} /><span><b>Codex：{codexVerdictLabel(codex.verdict)}</b>
                    <small>{codex.rationale}</small><small>{codex.recommendation}</small></span></div>}
                  {cluster.ignoreReason && <p className="ignore-reason"><b>人工核查记录：</b>{cluster.ignoreReason}</p>}</div>
                {onToggle && <button className="ghost finding-review" disabled={reviewing !== ""}
                  onClick={() => void onToggle(cluster)}>
                  {reviewing === cluster.id ? <LoaderCircle className="spin" size={14} /> :
                    cluster.ignored ? <RotateCcw size={14} /> : <ShieldCheck size={14} />}
                  {cluster.ignored ? "恢复" : "忽略"}
                </button>}
              </article>;
            }) : <Empty text="这个 Skill 没有规则警告" />}</div>
          </details>;
        })}</div>
      </details>;
    })}
  </div>;
}

function CodexProgressCard({ progress }: { progress: CodexReviewProgress }) {
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
      <small>{formatElapsed(elapsed)}</small>
    </div>
    <div className="codex-progress-track"><i style={{ width: `${percent}%` }} /></div>
    <div className="codex-progress-meta">
      <span>{progress.completedSkills}/{progress.totalSkills || "?"} Skills</span>
      <span>{progress.completedBatch}/{progress.batchCount || "?"} 分组</span>
      <span>{progress.activityCount} 次分析活动</span>
    </div>
    {!!progress.activeBatches?.length ? <div className="codex-progress-batches">
      {progress.activeBatches.map(batch => <div key={batch.index}><small>{batch.groupName || `分组 ${batch.index}`}</small>
        <span>{batch.skillNames.join("、")}</span></div>)}
    </div> : !!progress.activeSkills.length && <div className="codex-progress-skills">
      <small>当前 Skills</small>{progress.activeSkills.map(name => <span key={name}>{name}</span>)}
    </div>}
  </div>;
}

function CodexSkillReviewList({ report }: { report: ScanReport }) {
  const reviews = report.codexReview?.skillReviews ?? [];
  const groupBySkill = new Map((report.skills ?? []).map(skill => [skill.skillName, skill.groupName || "未分组"]));
  const groups = new Map<string, typeof reviews>();
  for (const review of reviews) {
    const group = groupBySkill.get(review.skillName) || "未分组";
    groups.set(group, [...(groups.get(group) ?? []), review]);
  }
  return <details className="codex-skill-results">
    <summary>按分组查看 {reviews.length} 个 Skill 的 Codex 结论</summary>
    {[...groups.entries()].map(([group, groupReviews]) => <section key={group} className="codex-result-group">
      <h4>{group}<small>{groupReviews.length} 个 Skill</small></h4>
      <div className="codex-skill-list">{groupReviews.map(review => <article key={`${review.skillName}:${review.sourcePath}`}
      className={`codex-skill-review ${review.verdict}`}>
      <div className="codex-skill-head"><div><strong>{review.skillName}</strong><code>{review.sourcePath}</code></div>
        <span className={`codex-verdict ${review.verdict}`}>{codexSkillVerdictLabel(review.verdict)}</span></div>
      <p>{review.summary}</p>
      <small>置信度 {Math.round((review.confidence || 0) * 100)}% ·
        Skill 目录文件 {review.contextFileCount || 0} 个 · 风险簇 {review.clusterIds?.length || 0} 个</small>
      {review.error && <div className="codex-skill-error">{review.error}</div>}
      {!!review.concerns?.length && <div className="codex-concern-list">{review.concerns.map((concern, index) =>
        <div key={`${concern.title}:${index}`}><span className={`severity ${concern.severity}`}>{severityLabel(concern.severity)}</span>
          <div><strong>{concern.title}</strong><p>{concern.rationale}</p>
            <small>{concern.recommendation}</small>
            {!!concern.evidenceFiles?.length && <code>{concern.evidenceFiles.join(" · ")}</code>}</div>
        </div>)}</div>}
    </article>)}</div></section>)}
  </details>;
}

function formatElapsed(seconds: number): string {
  const minutes = Math.floor(seconds / 60);
  const remainder = seconds % 60;
  return minutes ? `${minutes} 分 ${remainder.toString().padStart(2, "0")} 秒` : `${remainder} 秒`;
}

function codexSkillVerdictLabel(verdict: string): string {
  switch (verdict) {
    case "no-material-risk": return "未见明确风险";
    case "mostly-contextual": return "主要为上下文内容";
    case "review-required": return "建议人工关注";
    case "high-risk": return "高风险";
    case "insufficient-context": return "上下文不足";
    default: return verdict || "未知";
  }
}

function RiskOverview({ report }: { report: ScanReport }) {
  const skills = report.skills ?? [];
  const groupCount = new Set(skills.map(skill => skill.groupId)).size;
  const affectedSkills = new Set((report.clusters ?? [])
    .filter(cluster => !cluster.ignored).map(cluster => cluster.skillName)).size;
  return <div className="risk-overview" aria-label="风险概述">
    <div><span>分组</span><strong>{groupCount}</strong><small>本次检查</small></div>
    <div><span>Skills</span><strong>{skills.length}</strong><small>{affectedSkills} 个需要关注</small></div>
    <div><span>待处理</span><strong>{report.activeFindingCount}</strong><small>按 Skill 归类</small></div>
    <div><span>已忽略</span><strong>{report.ignoredFindingCount}</strong><small>人工决定</small></div>
  </div>;
}

function SecurityPage({ data, refresh, runOperation }: { data: Dashboard; refresh: () => Promise<void>; runOperation: RunOperation }) {
  const [report, setReport] = useState<ScanReport | null>(data.recentReports[0] ?? null);
  const [working, setWorking] = useState(false);
  const [codexWorking, setCodexWorking] = useState(false);
  const [reviewing, setReviewing] = useState("");
  const selectable = data.skills.filter(skill => !skill.system);
  const recommendedNames = () => selectable.filter(skill =>
    !isSecurityCurrent(skill)).map(skill => skill.name);
  const [selectedSkills, setSelectedSkills] = useState<Set<string>>(() => new Set(recommendedNames()));
  const { progress: codexProgress, clearProgress } = useCodexProgress();
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
        `扫描 ${names.length} 个 Skills`, () => api.auditSkills(names), "安全扫描已完成"
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
    setReviewing(cluster.id);
    const changed = await runOperation(
      cluster.ignored ? "恢复风险簇" : "记录风险簇人工决定",
      () => api.setRiskClusterIgnored(cluster, !cluster.ignored, "", true),
      cluster.ignored ? "风险簇已恢复" : "风险簇已按人工决定忽略"
    );
    setReviewing("");
    if (!changed) return;
    setReport(current => current ? updateClusterState(current, cluster.id, !cluster.ignored, "") : current);
    await refresh();
  };
  const ignoreAll = async (clusters: RiskCluster[]) => {
    if (!report || !clusters.length) return;
    setReviewing("manual-batch");
    const changed = await runOperation(
      "一键忽略全部安全警告",
      () => api.setRiskClustersIgnored(clusters, true, ""),
      `已忽略 ${clusters.length} 个待处理风险簇`
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
        "Codex 辅助风险复核",
        () => api.reviewWithCodex(report, (report.skills ?? []).map(skill => skill.skillName)),
        "Codex 风险归纳已完成"
      );
      if (reviewed) { setReport(reviewed); await refresh(); }
    } finally { setCodexWorking(false); }
  };
  const applyCodexSuggestions = async (clusters: RiskCluster[]) => {
    if (!report || !confirmCodexSuggestions(report, clusters)) return;
    setReviewing("codex-batch");
    try {
      const reason = codexBatchReason(report, clusters);
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
    <section className="panel security-summary"><div className="shield"><ShieldCheck size={42} /></div><h2>本地安全基线</h2>
      <p>扫描提示注入、凭据访问、命令执行、网络外泄、批量删除和混淆载荷。</p>
      <button className="primary" onClick={audit} disabled={working || codexWorking || selectedSkills.size === 0}>
        {working ? <LoaderCircle className="spin" size={17} /> : <ShieldCheck size={17} />}
        {working ? "正在扫描…" : `扫描选中的 ${selectedSkills.size} 个`}
      </button>
      <small>扫描只读取本地文件，不上传内容；Codex 复核入口位于最近扫描结果中。</small></section>
    <section className="panel security-queue"><PanelHead title="选择要检查的 Skills"
      subtitle="没有检查记录或内容已变化的 Skill 会默认选中；已检查且未变化的 Skill 默认跳过。" />
      <div className="selection-tools">
        <button className="ghost compact" onClick={selectRecommended}>恢复推荐</button>
        <button className="ghost compact" onClick={selectAll}>全选</button>
        <button className="ghost compact" onClick={invertSelection}>反选</button>
        <button className="ghost compact" onClick={() => setSelectedSkills(new Set())}>清空</button>
        <small>已选 {selectedSkills.size}/{selectable.length}</small>
      </div>
      <div className="security-skill-groups">{groups.map(group => {
        const selectedCount = group.skills.filter(skill => selectedSkills.has(skill.name)).length;
        return <details key={group.id} open={selectedCount > 0}>
          <summary><span><strong>{group.name}</strong><small>{group.skills.length} 个 Skill</small></span>
            <b>{selectedCount} 个已选</b></summary>
          <div>{group.skills.map(skill => <label key={skill.name} className="security-skill-option">
            <input type="checkbox" checked={selectedSkills.has(skill.name)} onChange={() => toggleSkill(skill.name)} />
            <span><strong>{skill.name}</strong><small>
              {isSecurityCurrent(skill)
                ? `已检查${skill.lastSecurityScan ? ` · ${new Date(skill.lastSecurityScan).toLocaleString("zh-CN")}` : ""}`
                : skill.securityChanged ? "内容已变化，需要重新检查" : "尚未检查"}
            </small></span>
            <em className={isSecurityCurrent(skill) ? "checked" : "pending"}>
              {isSecurityCurrent(skill) ? "可跳过" : "建议检查"}
            </em>
          </label>)}</div>
        </details>;
      })}</div>
    </section>
    <section className="panel security-results"><PanelHead title="最近扫描结果" subtitle={report ? `${report.filesScanned} 个文件 · ${report.activeFindingCount} 个待处理 · ${report.ignoredFindingCount} 个已忽略` : "尚未扫描"} />
      {!report ? <Empty text="运行一次本地安全扫描" /> : <>
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
  const groupCount = new Set((report.skills ?? []).map(skill => skill.groupId)).size;
  const skillCount = report.skills?.length ?? 0;
  return <div className={`scan-summary ${compact ? "compact" : ""}`}>
    <span className={`severity ${report.activeHighestSeverity}`}>{severityLabel(report.activeHighestSeverity)}</span>
    <div><strong>{report.activeFindingCount === 0 ? "没有待处理警告" : `${report.activeFindingCount} 个警告需要处理`}</strong>
      <small>本次检查 {groupCount || "未知"} 个分组、{skillCount || "未知"} 个 Skill；已人工处理 {report.ignoredFindingCount} 个警告。</small></div>
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

function fileClassLabel(fileClass: string): string {
  return ({
    instruction: "Skill 指令", runtime: "运行代码", test: "测试/夹具",
    documentation: "文档/示例", asset: "资源文件"
  } as Record<string, string>)[fileClass] || fileClass;
}

function codexVerdictLabel(verdict: string): string {
  return ({
    "confirmed-risk": "确认存在风险",
    "context-dependent": "取决于上下文",
    "documentation-or-example": "文档或示例",
    "false-positive": "很可能误报",
    "insufficient-context": "上下文不足",
    "manual-override-required": "必须人工决定"
  } as Record<string, string>)[verdict] || verdict;
}

function codexSuggestedClusters(report: ScanReport): RiskCluster[] {
  if (!report.codexReview || !["completed", "partial"].includes(report.codexReview.status)) return [];
  const reviews = new Map(report.codexReview.reviews.map(review => [review.clusterId, review]));
  return (report.clusters ?? []).filter(cluster => {
    if (cluster.ignored) return false;
    const verdict = reviews.get(cluster.id)?.verdict;
    return verdict === "false-positive" || verdict === "documentation-or-example" ||
      verdict === "manual-override-required";
  });
}

function codexBatchReason(report: ScanReport, clusters: RiskCluster[]): string {
  const model = report.codexReview?.model || "Codex";
  return `人工一键采纳 ${model} 的完整上下文复核建议（${clusters.length} 个风险簇）`;
}

function confirmCodexSuggestions(_report: ScanReport, clusters: RiskCluster[]): boolean {
  return clusters.length > 0;
}

function severityLabel(severity: ScanReport["activeHighestSeverity"]) {
  return ({ informational: "提示", low: "低风险", medium: "中风险", high: "高风险", critical: "严重风险" } as const)[severity];
}

function HistoryPage({ data, refresh, runOperation }: { data: Dashboard; refresh: () => Promise<void>; runOperation: RunOperation }) {
  const [workingId, setWorkingId] = useState("");
  const rollback = async (id: string) => {
    const original = data.recentHistory.find(tx => tx.id === id);
    const detail = original?.type.startsWith("group-")
      ? "只会恢复分组布局，不会移动或修改 Skill 文件。"
      : original?.type === "manage" || original?.type === "adopt"
        ? "只会恢复来源记录，不会移动 Skill 文件。"
        : "当前版本会先移动到隔离区。";
    if (!confirm(`回滚事务 ${id}？${detail}`)) return;
    setWorkingId(id);
    try {
      const result = await runOperation("回滚管理事务", () => api.rollback(id), "回滚已完成");
      if (result) await refresh();
    } finally { setWorkingId(""); }
  };
  return <section className="panel full"><PanelHead title="事务历史" subtitle="每次修改都保留状态、报告和回滚依据" />
    <div className="history-list">{data.recentHistory.length === 0 ? <Empty text="暂无事务" /> : data.recentHistory.map(tx => <article key={tx.id}>
      <div className={`tx-icon ${tx.status}`}><Clock3 size={19} /></div><div className="grow"><strong>{tx.type}</strong><span>{tx.targets.join("、") || "—"}</span>
        <small>{new Date(tx.startedAt).toLocaleString("zh-CN")} · {tx.id}</small></div><span className={`badge ${tx.status === "completed" ? "green" : "red"}`}>{tx.status}</span>
      {tx.status === "completed" && (tx.type === "install" || tx.type === "adopt" || tx.type === "manage" || tx.type.startsWith("group-")) && <button className="icon-button" disabled={!!workingId} title="回滚" onClick={() => rollback(tx.id)}>
        {workingId === tx.id ? <LoaderCircle className="spin" size={17} /> : <RotateCcw size={17} />}</button>}
    </article>)}</div>
  </section>;
}

function QuarantinePage({ refresh, runOperation }: { refresh: () => Promise<void>; runOperation: RunOperation }) {
  const [items, setItems] = useState<Array<{ skill: string; transactionId: string; path: string }>>([]);
  const [working, setWorking] = useState("");
  useEffect(() => { void api.quarantineList().then(setItems); }, []);
  const restore = async (skill: string, tx: string) => {
    setWorking(skill + tx);
    try {
      const result = await runOperation(`恢复 ${skill}`, () => api.restore(skill, tx), "Skill 已恢复");
      if (result) { setItems(await api.quarantineList()); await refresh(); }
    } finally { setWorking(""); }
  };
  return <section className="panel full"><PanelHead title="隔离区" subtitle="卸载内容不会被永久删除，可随时恢复" />
    <div className="history-list">{items.length === 0 ? <Empty text="隔离区为空" /> : items.map(item => <article key={item.skill + item.transactionId}>
      <div className="tx-icon"><ArchiveRestore size={19} /></div><div className="grow"><strong>{item.skill}</strong><span>{item.transactionId}</span><small>{item.path}</small></div>
      <button className="ghost" disabled={!!working} onClick={() => restore(item.skill, item.transactionId)}>
        {working === item.skill + item.transactionId ? <LoaderCircle className="spin" size={16} /> : <RotateCcw size={16} />}
        {working === item.skill + item.transactionId ? "恢复中…" : "恢复"}</button></article>)}</div>
  </section>;
}

function ReportsPage({ data }: { data: Dashboard }) {
  return <section className="panel full"><PanelHead title="扫描报告" subtitle="报告同时保存为 Markdown 和 JSON" />
    <div className="history-list">{data.recentReports.length === 0 ? <Empty text="暂无报告" /> : data.recentReports.map(r => <article key={r.id}>
      <div className={`tx-icon ${r.activeHighestSeverity}`}><ShieldCheck size={19} /></div><div className="grow"><strong>{r.id}</strong><span>{r.target}</span>
        <small>{new Date(r.completedAt).toLocaleString("zh-CN")} · {r.filesScanned} 文件 · {r.ignoredFindingCount} 已忽略</small></div>
      <span className={`severity ${r.activeHighestSeverity}`}>{severityLabel(r.activeHighestSeverity)}</span></article>)}</div>
  </section>;
}

function SettingsPage({ refresh, runOperation }: { refresh: () => Promise<void>; runOperation: RunOperation }) {
  const [cfg, setCfg] = useState<any>(null);
  const [diagnostics, setDiagnostics] = useState<Record<string, any> | null>(null);
  const [githubUser, setGitHubUser] = useState("");
  const [githubToken, setGitHubToken] = useState("");
  const [githubStatus, setGitHubStatus] = useState<Record<string, any> | null>(null);
  const [codexStatus, setCodexStatus] = useState<CodexCLIStatus | null>(null);
  const [saved, setSaved] = useState(false);
  const [scheduling, setScheduling] = useState(false);
  useEffect(() => {
    void api.config().then(setCfg);
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
  const configuredModel = cfg.codexReview.model || "default";
  const configuredModelMissing = configuredModel !== "default" &&
    !codexModels.some(model => model.slug === configuredModel);
  const pathFields = [
    ["Skills 根目录", "skillsRoot"], ["数据目录", "dataRoot"], ["操作日志", "logsRoot"], ["扫描与事务报告", "reportsRoot"],
    ["备份目录", "backupsRoot"], ["隔离区", "quarantineRoot"], ["缓存目录", "cacheRoot"], ["暂存目录", "stagingRoot"]
  ];
  const save = async () => {
    const result = await runOperation("保存应用设置", async () => { await api.saveConfig(cfg); return true; }, "设置已保存，路径变更需重启应用");
    if (result) { setSaved(true); setTimeout(() => setSaved(false), 1800); }
  };
  const schedule = async () => {
    setScheduling(true);
    try {
      await runOperation("配置定时更新检查", async () => {
        await api.schedule(cfg.schedule.enabled, cfg.schedule.frequency, cfg.schedule.time);
        return true;
      }, cfg.schedule.enabled ? "定时检查已启用" : "定时检查已关闭");
    } finally { setScheduling(false); }
  };
  const saveToken = async () => {
    if (!githubToken.trim()) return;
    const result = await runOperation("保存 GitHub 凭据", async () => {
      await api.saveGitHubToken(githubToken.trim(), githubUser.trim());
      return true;
    }, "GitHub 凭据已保存到 Windows 凭据管理器");
    if (result) {
      setGitHubToken("");
      const status = await runOperation("验证 GitHub 凭据", api.validateGitHub, "GitHub 凭据验证完成");
      if (status) setGitHubStatus(status);
    }
  };
  const validateGitHub = async () => {
    const status = await runOperation("验证 GitHub 凭据", api.validateGitHub, "GitHub 凭据验证完成");
    if (status) setGitHubStatus(status);
  };
  const validateCodex = async () => {
    const status = await runOperation("检查 Codex CLI", api.codexStatus, "Codex CLI 检查完成");
    if (status) setCodexStatus(status);
  };
  const bootstrap = async () => {
    if (!confirm("自动识别并管理当前已知的历史 Skills？此操作不会替换或移动 Skill 文件。")) return;
    const result = await runOperation("自动管理已知 Skills", async () => {
      await api.bootstrap();
      return true;
    }, "已知 Skills 的来源记录已更新");
    if (result) await refresh();
  };
  const rerunDiagnostics = async () => {
    const result = await runOperation("运行环境诊断", api.diagnostics, "环境诊断已完成");
    if (result) setDiagnostics(result);
  };
  return <div className="settings-grid">
    <section className="panel settings-paths"><PanelHead title="存储位置" subtitle="所有目录均支持自定义绝对路径" />
      <div className="form">{pathFields.map(([label, key]) => <label key={key}><span>{label}</span>
        <input value={cfg.paths[key]} onChange={e => setCfg({ ...cfg, paths: { ...cfg.paths, [key]: e.target.value } })} /></label>)}</div>
      <div className="settings-save"><small>路径变更保存后需重启应用</small>
        <button className="primary compact" onClick={save}>{saved ? <CheckCircle2 size={15} /> : <Settings size={15} />}
          {saved ? "已保存" : "保存设置"}</button></div>
    </section>
    <section className="panel"><PanelHead title="定时检查" subtitle="只检查更新，不自动安装" />
      <div className="schedule-form"><label className="switch-row"><span><strong>启用计划任务</strong><small>通过 Windows 任务计划程序运行</small></span>
        <input type="checkbox" checked={cfg.schedule.enabled} onChange={e => setCfg({ ...cfg, schedule: { ...cfg.schedule, enabled: e.target.checked } })} /></label>
        <label><span>频率</span><select value={cfg.schedule.frequency} onChange={e => setCfg({ ...cfg, schedule: { ...cfg.schedule, frequency: e.target.value } })}><option value="daily">每天</option><option value="weekly">每周</option></select></label>
        <label><span>时间</span><input type="time" value={cfg.schedule.time} onChange={e => setCfg({ ...cfg, schedule: { ...cfg.schedule, time: e.target.value } })} /></label>
        <button className="ghost" disabled={scheduling} onClick={schedule}>
          {scheduling ? <LoaderCircle className="spin" size={16} /> : <Clock3 size={16} />}{scheduling ? "正在应用…" : "应用计划任务"}</button>
      </div>
    </section>
    <section className="panel"><PanelHead title="GitHub 凭据与限额" subtitle="公共和私有仓库共用；凭据仅保存在 Windows 本地" />
      <div className="credential-form">
        <label><span>用户名（可选）</span><input value={githubUser} onChange={e => setGitHubUser(e.target.value)} placeholder="GitHub 用户名" /></label>
        <label><span>Personal Access Token</span><input type="password" value={githubToken} onChange={e => setGitHubToken(e.target.value)} placeholder="输入后不会再次显示" /></label>
        <div className="credential-actions">
          <button className="ghost" disabled={!githubToken.trim()} onClick={saveToken}><KeyRound size={16} />保存并验证</button>
          <button className="ghost" onClick={validateGitHub}><ShieldCheck size={16} />验证当前凭据</button>
        </div>
        {githubStatus && <div className={`integration-status ${githubStatus.authenticated ? "ok" : "warning"}`}>
          <strong>{githubStatus.authenticated ? `已认证：${githubStatus.login}` : "未认证"}</strong>
          <small>{githubStatus.message}</small>
          {!!githubStatus.limit && <small>API 剩余额度：{githubStatus.remaining}/{githubStatus.limit}
            {githubStatus.resetAt ? ` · 重置于 ${new Date(githubStatus.resetAt).toLocaleString("zh-CN")}` : ""}</small>}
        </div>}
      </div>
    </section>
    <section className="panel codex-settings"><PanelHead title="Codex 辅助风险复核" subtitle="可选的第二阶段语义归纳；默认关闭，不替代人工决定" />
      <div className="schedule-form">
        <label className="switch-row"><span><strong>启用 Codex CLI 复核</strong>
          <small>复用独立 Codex CLI 的已登录状态，仅提交本地扫描审查包</small></span>
          <input type="checkbox" checked={cfg.codexReview.enabled}
            onChange={e => setCfg({ ...cfg, codexReview: { ...cfg.codexReview, enabled: e.target.checked } })} /></label>
        <label><span>CLI 路径</span><input value={cfg.codexReview.cliPath || ""}
          onChange={e => setCfg({ ...cfg, codexReview: { ...cfg.codexReview, cliPath: e.target.value } })}
          placeholder="留空自动查找独立 codex.exe" /></label>
        <label><span>模型</span><select value={configuredModel}
          disabled={!codexStatus?.authenticated}
          onChange={e => setCfg({ ...cfg, codexReview: { ...cfg.codexReview, model: e.target.value } })}>
          <option value="default">跟随 Codex 默认模型（推荐）</option>
          {configuredModelMissing && <option value={configuredModel}>{configuredModel}（当前配置，模型目录中未返回）</option>}
          {codexModels.map(model => <option key={model.slug} value={model.slug} title={model.description}>
            {model.displayName}{model.displayName === model.slug ? "" : ` · ${model.slug}`}
          </option>)}
        </select></label>
        <label><span>推理强度</span><select value={cfg.codexReview.reasoningEffort}
          onChange={e => setCfg({ ...cfg, codexReview: { ...cfg.codexReview, reasoningEffort: e.target.value } })}>
          <option value="minimal">最低</option><option value="low">低</option><option value="medium">中（推荐）</option>
          <option value="high">高</option><option value="xhigh">超高</option>
        </select></label>
        <div className="codex-batch-settings">
          <label><span>同时复核的分组</span><input type="number" min="1" max="4"
            value={cfg.codexReview.maxParallelBatches || 1}
            onChange={e => setCfg({ ...cfg, codexReview: { ...cfg.codexReview, maxParallelBatches: Number(e.target.value) } })} /></label>
        </div>
        <small className="codex-batch-hint">同一分组内的 Skill 始终一起复核。默认串行处理；提高并发可能造成 Codex CLI 模型刷新竞争或限流。</small>
        <div className="credential-actions codex-actions">
          <button className="ghost" onClick={validateCodex}><Stethoscope size={16} />检查 CLI 与登录状态</button>
          <button className="primary compact" onClick={save}><Settings size={15} />保存复核设置</button>
        </div>
        {codexStatus && <div className={`integration-status ${codexStatus.authenticated && codexStatus.compatible ? "ok" : "warning"}`}>
          <strong>{!codexStatus.available ? "Codex CLI 不可用" :
            !codexStatus.authenticated ? "Codex CLI 尚未登录" :
              !codexStatus.compatible ? "Codex CLI 已登录，但复核能力不兼容" : "Codex CLI 已登录且兼容"}</strong>
          {codexStatus.version && <small>{codexStatus.version}</small>}
          {codexStatus.error && <small>{codexStatus.error}</small>}
          {codexStatus.authenticated && !!codexStatus.models?.length &&
            <small>已从当前 CLI 载入 {codexStatus.models.length} 个可选模型。</small>}
          {codexStatus.modelCatalogError && <small>模型列表暂不可用：{codexStatus.modelCatalogError}</small>}
          {!!codexStatus.missingCapabilities?.length &&
            <small>缺少能力：{codexStatus.missingCapabilities.join("、")}</small>}
          {codexStatus.checkedAt && <small>上次检查：{new Date(codexStatus.checkedAt).toLocaleString("zh-CN")}</small>}
          {codexStatus.path && <code title={codexStatus.path}>{codexStatus.path}</code>}
        </div>}
      </div>
    </section>
    <section className="panel"><PanelHead title="工具与诊断" subtitle="对应 CLI 的 bootstrap、doctor 和 version" />
      <div className="diagnostics">
        <div className="diagnostic-version"><Sparkles size={19} /><span><small>应用版本</small><strong>{diagnostics?.version || "读取中…"}</strong></span></div>
        {diagnostics && <div className="diagnostic-grid">
          <span>Skills 目录<b className={diagnostics.skillsRootExists ? "ok" : "bad"}>{diagnostics.skillsRootExists ? "正常" : "不可用"}</b></span>
          <span>数据目录<b className={diagnostics.dataRootExists ? "ok" : "bad"}>{diagnostics.dataRootExists ? "正常" : "不可用"}</b></span>
          <code title={diagnostics.configPath}>{diagnostics.configPath}</code>
        </div>}
        <div className="diagnostic-actions">
          <button className="ghost" onClick={rerunDiagnostics}><Stethoscope size={16} />重新诊断</button>
          <button className="ghost" onClick={bootstrap}><Sparkles size={16} />自动管理已知 Skills</button>
        </div>
      </div>
    </section>
  </div>;
}

function InstallDialog({ close, refresh, runOperation }: { close: () => void; refresh: () => Promise<void>; runOperation: RunOperation }) {
  const [mode, setMode] = useState<"github" | "local">("github");
  const [source, setSource] = useState("");
  const [ref, setRef] = useState("");
  const [preview, setPreview] = useState<InstallPreview | null>(null);
  const [selected, setSelected] = useState<string[]>([]);
  const [working, setWorking] = useState(false);
  const [reviewing, setReviewing] = useState("");
  const [codexWorking, setCodexWorking] = useState(false);
  const [error, setError] = useState("");
  const { progress: codexProgress, clearProgress } = useCodexProgress();
  const analyze = async () => {
    setWorking(true); setError("");
    try {
      const p = await runOperation(
        mode === "github" ? "分析 GitHub Skill 来源" : "分析本地 Skill 目录",
        () => mode === "github" ? api.prepareGitHub(source, ref) : api.prepareLocal(source),
        "来源分析和安全扫描已完成"
      );
      if (p) { setPreview(p); setSelected(p.skills.map(s => s.name)); }
    } catch (e: any) { setError(e?.message ?? String(e)); } finally { setWorking(false); }
  };
  const apply = async () => {
    if (!preview || !selected.length) return;
    setWorking(true); setError("");
    try {
      const result = await runOperation("安装选中的 Skills",
        () => api.apply(preview.id, selected, false), "Skills 已安装并完成备份记录");
      if (result) { await refresh(); close(); }
    }
    catch (e: any) { setError(e?.message ?? String(e)); } finally { setWorking(false); }
  };
  const toggleCluster = async (cluster: RiskCluster) => {
    if (!preview) return;
    setReviewing(cluster.id);
    setError("");
    try {
      await api.setRiskClusterIgnored(cluster, !cluster.ignored, "", true);
      setPreview(current => current ? {
        ...current,
        scan: updateClusterState(current.scan, cluster.id, !cluster.ignored, "")
      } : current);
    } catch (e: any) {
      setError(e?.message ?? String(e));
    } finally {
      setReviewing("");
    }
  };
  const ignoreAll = async (clusters: RiskCluster[]) => {
    if (!preview || !clusters.length) return;
    setReviewing("manual-batch");
    setError("");
    try {
      await api.setRiskClustersIgnored(clusters, true, "");
      setPreview(current => current ? {
        ...current, scan: updateClustersState(current.scan, clusters, true, "")
      } : current);
    } catch (e: any) {
      setError(e?.message ?? String(e));
    } finally {
      setReviewing("");
    }
  };
  const codexReview = async () => {
    if (!preview) return;
    setCodexWorking(true);
    clearProgress();
    setError("");
    try {
      const reviewed = await api.reviewWithCodex(preview.scan, selected);
      setPreview(current => current ? { ...current, scan: reviewed } : current);
    } catch (e: any) {
      setError(e?.message ?? String(e));
    } finally {
      setCodexWorking(false);
    }
  };
  const applyCodexSuggestions = async (clusters: RiskCluster[]) => {
    if (!preview || !confirmCodexSuggestions(preview.scan, clusters)) return;
    setReviewing("codex-batch");
    setError("");
    try {
      const reason = codexBatchReason(preview.scan, clusters);
      await api.setRiskClustersIgnored(clusters, true, reason);
      setPreview(current => current ? {
        ...current, scan: updateClustersState(current.scan, clusters, true, reason)
      } : current);
    } catch (e: any) {
      setError(e?.message ?? String(e));
    } finally {
      setReviewing("");
    }
  };
  return <div className="modal-backdrop"><div className="modal">
    <div className="modal-head"><div><p className="eyebrow">SAFE INSTALL</p><h2>安装 Skill</h2></div><button onClick={close}><X /></button></div>
    {!preview ? <div className="install-source">
      <div className="tabs"><button className={mode === "github" ? "active" : ""} onClick={() => setMode("github")}><FolderGit2 size={17} />GitHub 链接</button>
        <button className={mode === "local" ? "active" : ""} onClick={() => setMode("local")}><Boxes size={17} />本地目录</button></div>
      <label><span>{mode === "github" ? "GitHub 仓库、目录或 SKILL.md 链接" : "包含一个或多个 Skills 的绝对路径"}</span>
        <input autoFocus value={source} onChange={e => setSource(e.target.value)} placeholder={mode === "github" ? "https://github.com/owner/repository" : "D:\\skills\\my-package"} /></label>
      {mode === "github" && <label><span>分支、标签或 Commit（可选）</span><input value={ref} onChange={e => setRef(e.target.value)} placeholder="留空时使用链接版本或默认分支" /></label>}
      <div className="notice"><ShieldCheck size={20} /><span><strong>先暂存和扫描</strong><small>分析阶段不会写入正式 Skills 目录，也不会执行仓库脚本。</small></span></div>
    </div> : <div className="preview">
      <div className="repo-summary"><FolderGit2 size={24} /><div><strong>{preview.repository.fullName}</strong><span>{preview.repository.resolvedRef} · {preview.repository.commitSha?.slice(0, 10)}</span></div>
        <span className={`severity ${preview.scan.activeHighestSeverity}`}>{severityLabel(preview.scan.activeHighestSeverity)}</span></div>
      <h3>发现 {preview.skills.length} 个 Skills</h3>
      <div className="candidate-list">{preview.skills.map(s => <label key={s.name}><input type="checkbox" checked={selected.includes(s.name)}
        onChange={() => setSelected(selected.includes(s.name) ? selected.filter(n => n !== s.name) : [...selected, s.name])} />
        <span><strong>{s.name}</strong><small>{s.description}</small><code>{s.sourcePath}</code></span></label>)}</div>
      <div className="notice"><ShieldAlert size={20} /><span><strong>{preview.scan.findings.length} 个原始安全发现，{preview.scan.activeFindingCount} 个待核查</strong>
        <small>所有级别均在下方概述，可由人工直接忽略单个风险簇或一键忽略全部；无需填写原因。</small></span></div>
      <FindingDetails report={preview.scan} reviewing={reviewing} onToggle={toggleCluster}
        onCodexReview={codexReview} codexWorking={codexWorking}
        codexProgress={codexProgress?.reportId === preview.scan.id ? codexProgress : null}
        onApplyCodexSuggestions={applyCodexSuggestions} onIgnoreAll={ignoreAll} />
    </div>}
    {error && <div className="error-banner"><CircleAlert size={17} />{error}</div>}
    <div className="modal-actions"><button className="ghost" onClick={preview ? () => setPreview(null) : close}>{preview ? "返回" : "取消"}</button>
      <button className="primary" disabled={working || (!preview && !source) || (!!preview && (!selected.length ||
        reviewing !== "" || ["critical", "high"].includes(preview.scan.activeHighestSeverity)))} onClick={preview ? apply : analyze}>
        {working ? <LoaderCircle className="spin" size={17} /> : preview ? <Download size={17} /> : <Search size={17} />}
        {working ? (preview ? "正在安装…" : "正在分析…") : preview ? "确认安装" : "分析来源"}</button></div>
  </div></div>;
}

function Empty({ text }: { text: string }) {
  return <div className="empty"><Gauge size={28} /><span>{text}</span></div>;
}
