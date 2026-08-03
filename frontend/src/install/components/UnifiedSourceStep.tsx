import { FileCode2, FolderGit2, ShieldCheck } from "lucide-react";
import { useI18n } from "../../i18n";
import { RootSelector } from "../../shell/RootSelector";
import type { RootContract } from "../../roots";

export type SourceMethod = "github" | "local";

interface UnifiedSourceStepProps {
  sourceMethod: SourceMethod;
  source: string;
  requestedRef: string;
  busy: boolean;
  roots: RootContract[];
  rootId: string;
  setRootId: (value: string) => void;
  setSourceMethod: (value: SourceMethod) => void;
  setSource: (value: string) => void;
  setRequestedRef: (value: string) => void;
}

export function UnifiedSourceStep({ sourceMethod, source, requestedRef, busy, roots, rootId, setRootId, setSourceMethod, setSource, setRequestedRef }: UnifiedSourceStepProps) {
  const { t } = useI18n();
  return <div className="install-source-step unified-source-step">
    <div className="source-intro"><ShieldCheck size={24} aria-hidden="true" /><div>
      <span className="eyebrow">{t("统一安全流程", "Unified security workflow")}</span>
      <h3>{t("添加需要检查的项目", "Add a project to assess")}</h3>
      <p>{t("应用先读取项目并运行本地必选检查，再显示安装目标和可选检查。", "The app first reads the project and runs required local checks, then shows install targets and optional checks.")}</p>
    </div></div>
    {roots.length > 0 && <RootSelector roots={roots} value={rootId} onChange={setRootId} disabled={busy} />}
    <div className="install-source-tabs" aria-label={t("来源类型", "Source type")} role="group">
      <button type="button" aria-pressed={sourceMethod === "github"}
        className={sourceMethod === "github" ? "active" : ""} disabled={busy}
        onClick={() => setSourceMethod("github")}><FolderGit2 size={17} aria-hidden="true" />{t("GitHub 链接", "GitHub link")}</button>
      <button type="button" aria-pressed={sourceMethod === "local"}
        className={sourceMethod === "local" ? "active" : ""} disabled={busy}
        onClick={() => setSourceMethod("local")}><FileCode2 size={17} aria-hidden="true" />{t("本地目录", "Local directory")}</button>
    </div>
    <label className="install-field"><span>{sourceMethod === "github"
      ? t("GitHub 仓库、目录或 SKILL.md 链接", "GitHub repository, directory, or SKILL.md link")
      : t("包含一个或多个 Skills 的绝对路径", "Absolute path containing one or more Skills")}</span>
      <input autoFocus value={source} disabled={busy} onChange={event => setSource(event.target.value)}
        placeholder={sourceMethod === "github" ? "https://github.com/owner/repository" : "D:\\skills\\my-package"} /></label>
    {sourceMethod === "github" && <label className="install-field"><span>{t("分支、标签或 Commit（可选）", "Branch, tag, or commit (optional)")}</span>
      <input value={requestedRef} disabled={busy} onChange={event => setRequestedRef(event.target.value)}
        placeholder={t("留空时使用链接版本或默认分支", "Leave blank to use the linked version or default branch")} /></label>}
    <div className="install-safety-note" role="note"><ShieldCheck size={21} aria-hidden="true" /><div><strong>{t("本地检查优先", "Local checks first")}</strong>
      <p>{t("这一步只固定来源、创建受管快照并运行本地检查；不会调用 Codex、下载依赖或执行仓库脚本。", "This step only pins the source, creates a managed snapshot, and runs local checks. It does not call Codex, download dependencies, or execute repository scripts.")}</p></div></div>
  </div>;
}
