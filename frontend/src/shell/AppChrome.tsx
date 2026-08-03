import { CheckCircle2, CircleAlert, LoaderCircle, X } from "lucide-react";
import { useI18n } from "../i18n";

export type Operation = { label: string; detail: string; status: "running" | "success" | "error" };

export function OperationBanner({ operation, dismiss }: { operation: Operation; dismiss: () => void }) {
  const { t } = useI18n();
  const icon = operation.status === "running" ? <LoaderCircle className="spin" size={19} aria-hidden="true" /> :
    operation.status === "success" ? <CheckCircle2 size={19} aria-hidden="true" /> : <CircleAlert size={19} aria-hidden="true" />;
  return <div className={`operation-banner ${operation.status}`} role={operation.status === "error" ? "alert" : "status"}
    aria-live={operation.status === "error" ? "assertive" : "polite"}>
    {icon}
    <div><strong>{operation.label}</strong><span>{operation.detail}</span></div>
    {operation.status !== "running" && <button type="button" onClick={dismiss} aria-label={t("关闭提示", "Dismiss notification")}><X size={16} /></button>}
    {operation.status === "running" && <i aria-hidden="true" />}
  </div>;
}

export function Loading() {
  const { t } = useI18n();
  return <div className="loading" role="status" aria-live="polite">
    <LoaderCircle className="spin" aria-hidden="true" />
    <span>{t("正在读取本地 Skill 清单…", "Loading local Skills…")}</span>
  </div>;
}
