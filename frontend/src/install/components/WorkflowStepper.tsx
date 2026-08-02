import { Check } from "lucide-react";
import { useI18n } from "../../i18n";

export type InstallWorkflowStage = "source" | "assess" | "review" | "apply";

const stages: Array<{ id: InstallWorkflowStage; zh: string; en: string }> = [
  { id: "source", zh: "来源", en: "Source" },
  { id: "assess", zh: "理解与计划", en: "Understand & plan" },
  { id: "review", zh: "检查与确认", en: "Check & confirm" },
  { id: "apply", zh: "安装与结果", en: "Install & result" }
];

export function WorkflowStepper({ current }: { current: InstallWorkflowStage }) {
  const { t } = useI18n();
  const currentIndex = stages.findIndex(stage => stage.id === current);
  return <ol className="workflow-stepper" aria-label={t("安装进度", "Installation progress")}>
    {stages.map((stage, index) => {
      const complete = index < currentIndex;
      const active = index === currentIndex;
      return <li key={stage.id} className={complete ? "complete" : active ? "active" : ""}
        aria-current={active ? "step" : undefined}>
        <span className="workflow-step-index">{complete ? <Check size={13} /> : index + 1}</span>
        <span>{t(stage.zh, stage.en)}</span>
      </li>;
    })}
  </ol>;
}
