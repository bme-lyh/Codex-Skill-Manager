import { AlertTriangle, CheckCircle2, ChevronDown, CircleAlert, OctagonX, ShieldCheck } from "lucide-react";
import { useI18n } from "../../i18n";
import type { Translate } from "../../i18n";
import type { AssessmentRequirement, ProjectAssessment } from "../../types";

const requirementOrder: AssessmentRequirement[] = ["required", "triggered", "optional"];

export function AssessmentView({ assessment }: { assessment: ProjectAssessment }) {
  const { t } = useI18n();
  const tone = assessment.gate;
  const Icon = tone === "ready" ? CheckCircle2 : tone === "attention" ? AlertTriangle : tone === "blocked" ? OctagonX : CircleAlert;
  const title = tone === "ready" ? t("可以安装", "Ready to install") :
    tone === "attention" ? t("安装前需要确认", "Review before installing") :
      tone === "blocked" ? t("已阻止安装", "Installation blocked") :
        t("检查尚未完成", "Assessment incomplete");
  return <section className={`assessment-view ${tone}`} aria-live="polite">
    <div className="assessment-outcome">
      <Icon size={24} />
      <div><span className="eyebrow">{t("本地分层安全检查", "Local layered security assessment")}</span>
        <h3>{title}</h3><p>{assessment.summary}</p></div>
      <span className="assessment-kind">{projectKindLabel(assessment.classification, t)}</span>
    </div>
    <div className="assessment-metrics">
      <span><strong>{assessment.coverage.filesInventoried}</strong>{t("已清点文件", " inventoried")}</span>
      <span><strong>{assessment.coverage.filesScanned}</strong>{t("已扫描文件", " scanned")}</span>
      <span><strong>{assessment.targets.filter(target => target.supported).length}</strong>{t("可安装目标", " supported targets")}</span>
    </div>
    {assessment.coverage.evidenceLimited && <div className="assessment-limit"><AlertTriangle size={16} />
      {t("仓库超过本地证据预算，结果按未完成处理。", "The repository exceeded the local evidence budget, so the result is incomplete.")}</div>}
    <div className="assessment-groups">
      {requirementOrder.map(requirement => {
        const checks = assessment.checks.filter(check => check.requirement === requirement);
        if (!checks.length) return null;
        return <details key={requirement} open={requirement !== "optional"}>
          <summary><ShieldCheck size={16} />{requirementLabel(requirement, t)}<span>{checks.length}</span><ChevronDown size={15} /></summary>
          <div className="assessment-checks">{checks.map(check => <div key={check.id} className={`assessment-check ${check.status}`}>
            <span className="assessment-check-dot" /><div><strong>{check.title}</strong><p>{check.summary}</p>
              {check.reason && <small>{check.reason}</small>}
              {!!check.evidenceFiles.length && <details className="assessment-evidence"><summary>{t("查看依据", "View evidence")}</summary>
                <ul>{check.evidenceFiles.map(file => <li key={file}><code>{file}</code></li>)}</ul></details>}
            </div></div>)}</div>
        </details>;
      })}
    </div>
    {!!assessment.targets.length && <div className="assessment-targets"><strong>{t("安装目标", "Install targets")}</strong>
      {assessment.targets.map(target => <div key={`${target.kind}:${target.displayName}`} className={target.supported ? "supported" : "unsupported"}>
        <span>{target.displayName}</span><code>{target.path}</code>
        <small>{target.supported
          ? target.reversible ? t("支持安装 · 可回滚", "Supported · reversible") : t("支持安装 · 不可回滚", "Supported · not reversible")
          : target.reason || t("当前不支持自动安装", "Automatic installation is not supported")}</small>
      </div>)}</div>}
  </section>;
}

function requirementLabel(requirement: AssessmentRequirement, t: Translate) {
  if (requirement === "required") return t("必选检查", "Required checks");
  if (requirement === "triggered") return t("已触发的附加检查", "Triggered checks");
  return t("可选深度检查", "Optional deep checks");
}

function projectKindLabel(kind: string, t: Translate) {
  const values: Record<string, [string, string]> = {
    skill: ["Skill 仓库", "Skill repository"], mixed: ["混合仓库", "Mixed repository"],
    plugin: ["Codex 插件", "Codex plugin"], application: ["应用项目", "Application"],
    library: ["代码库", "Library"], unknown: ["待确认项目", "Unclassified project"]
  };
  const value = values[kind] ?? values.unknown;
  return t(value[0], value[1]);
}
