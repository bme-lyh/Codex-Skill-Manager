import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  AlertTriangle,
  ArrowLeft,
  Check,
  CheckCircle2,
  ChevronRight,
  CircleAlert,
  CircleDashed,
  Clock3,
  Download,
  FileCode2,
  FolderGit2,
  KeyRound,
  LoaderCircle,
  OctagonX,
  RefreshCw,
  RotateCcw,
  Search,
  Settings,
  ShieldCheck,
  Sparkles,
  Square,
  TerminalSquare,
  X
} from "lucide-react";
import { api } from "../api";
import { useI18n } from "../i18n";
import type { Translate } from "../i18n";
import type {
  AssistedInstallPermission,
  AssistedInstallPlan,
  AssistedInstallProgress,
  AssistedInstallProgressStep,
  AssistedInstallRequirement,
  AssistedInstallResult,
  AssistedInstallStep,
  Candidate,
  CodexProjectScanResult,
  InstallPreview,
  ProjectAssessment,
  RiskCluster,
  ScanReport,
  Severity,
  Transaction
} from "../types";
import {
  assessmentAllowsSelectedTargets,
  assistedPlanDisposition,
  classifyInstallIssue,
  createActiveInstallReference,
  mergeProgressSnapshot,
  parseActiveInstallReference,
  parseRetryTimestamp,
  restoredSelectedSkills,
  retryWaitMilliseconds,
  serializeActiveInstallReference
} from "./state";
import type { ActiveInstallReference } from "./state";
import { AssessmentView } from "./components/AssessmentView";
import { WorkflowStepper } from "./components/WorkflowStepper";
import type { InstallWorkflowStage } from "./components/WorkflowStepper";
import "./install.css";

const ACTIVE_PLAN_KEY = "csm.assisted-install.active-plan";
const INSTALL_DRAFT_KEY = "csm.install.draft";

type InstallMethod = "standard" | "assisted";
type SourceMethod = "github" | "local";
type BusyTask = "" | "restore" | "source" | "codex" | "standard" | "risk" | "assisted" | "cancel" | "rollback" | "refresh";
type RetryTask = "" | "restore" | "source" | "codex" | "standard" | "assisted" | "rollback" | "refresh";

interface Feedback {
  tone: "info" | "success" | "warning" | "error";
  title: string;
  message: string;
  detail?: string;
  rateLimited?: boolean;
  retryAt?: string;
  retryable?: boolean;
  settingsSuggested?: boolean;
  settingsKind?: "github" | "codex";
  restartRequired?: boolean;
  suggestedSourceUrl?: string;
}

interface InstallDraft {
  installMethod: InstallMethod;
  sourceMethod: SourceMethod;
  source: string;
  requestedRef: string;
}

interface InstallDialogProps {
  close: () => void;
  refresh: () => Promise<void>;
  openSettings?: () => void;
}

export function InstallDialog({ close, refresh, openSettings }: InstallDialogProps) {
  const { t } = useI18n();
  const initialDraft = useRef(readInstallDraft()).current;
  const [installMethod, setInstallMethod] = useState<InstallMethod>("standard");
  const [sourceMethod, setSourceMethod] = useState<SourceMethod>(initialDraft?.sourceMethod ?? "github");
  const [source, setSource] = useState(initialDraft?.source ?? "");
  const [requestedRef, setRequestedRef] = useState(initialDraft?.requestedRef ?? "");
  const [preview, setPreview] = useState<InstallPreview | null>(null);
  const [assessment, setAssessment] = useState<ProjectAssessment | null>(null);
  const [projectScan, setProjectScan] = useState<CodexProjectScanResult | null>(null);
  const [plan, setPlan] = useState<AssistedInstallPlan | null>(null);
  const [selectedSkills, setSelectedSkills] = useState<string[]>([]);
  const [selectedPermissions, setSelectedPermissions] = useState<string[]>([]);
  const [projectRoot, setProjectRoot] = useState("");
  const [progress, setProgress] = useState<AssistedInstallProgress | null>(null);
  const [result, setResult] = useState<AssistedInstallResult | null>(null);
  const [executionStarted, setExecutionStarted] = useState(false);
  const [standardResult, setStandardResult] = useState<Transaction | null>(null);
  const [busy, setBusy] = useState<BusyTask>("restore");
  const [feedback, setFeedback] = useState<Feedback | null>(null);
  const [retryTask, setRetryTask] = useState<RetryTask>("");
  const [rolledBack, setRolledBack] = useState(false);
  const retryWaitMs = useRetryCountdown(feedback?.retryAt);
  const rateLimitBlocked = !!feedback?.rateLimited && retryWaitMs > 0;
  const dialogRef = useRef<HTMLDivElement | null>(null);
  const restoreStarted = useRef(false);
  const analysisGeneration = useRef(0);
  const activeAnalysisReferenceRef = useRef("");
  const analysisStartedAtRef = useRef(0);
  const restoredAnalysisPollingRef = useRef(false);
  const planIdRef = useRef("");
  const progressRef = useRef<AssistedInstallProgress | null>(null);
  const executionStartedRef = useRef(false);
  const activeExecutionRunRef = useRef("");
  const executionStartedAtRef = useRef(0);

  const candidates = useMemo(
    () => preview?.skills?.length ? preview.skills : planCandidates(plan),
    [preview, plan]
  );
  const scan = preview?.scan ?? plan?.scan;
  const activeClusters = scan?.clusters?.filter(cluster => !cluster.ignored) ?? [];
  const hasBlockingWarnings = activeClusters.some(cluster => cluster.severity === "critical" || cluster.severity === "high");
  const assessmentAllowsInstall = useMemo(
    () => assessmentAllowsSelectedTargets(assessment, selectedSkills),
    [assessment, selectedSkills]
  );
  const requiredPermissionIds = useMemo(
    () => (plan?.permissions ?? []).filter(permission => permission.required).map(permission => permission.id),
    [plan]
  );
  const missingPermissionCount = requiredPermissionIds.filter(id => !selectedPermissions.includes(id)).length;
  const permissionDependencyIssue = useMemo(
    () => plan ? assistedPermissionDependencyIssue(plan.steps, selectedPermissions, t) : "",
    [plan, selectedPermissions, t]
  );
  const projectRootRequired = useMemo(
    () => !!plan?.needsProjectRoot && hasScheduledAssistedStep(
      plan.steps,
      "configure-codex-mcp",
      selectedPermissions
    ),
    [plan, selectedPermissions]
  );
  const executionProgress = progress && (executionStarted || !!plan?.transactionId || !!result) &&
    isExecutionProgress(progress) ? progress : null;
  const recoveredFailure = !!plan && isRecoveredFailure(plan);
  const recoveryProgress = useMemo(
    () => recoveredFailure && !executionProgress ? recoveredFailureProgress(plan, t) : null,
    [executionProgress, plan, recoveredFailure, t]
  );
  const displayProgress = executionProgress ?? recoveryProgress;
  const executionRunning = busy === "assisted" || busy === "cancel" ||
    (!!executionProgress && !executionProgress.terminal);
  const cannotClose = busy !== "" || executionRunning;
  const canHideInBackground = busy === "codex" || busy === "assisted" ||
    busy === "cancel" || executionRunning;
  const assistedRetryReady = !!plan && !executionRunning && !isRecoveredFailure(plan) &&
    ["ready", "failed", "cancelled"].includes(plan.status);
  const rollbackRetryReady = !!(result?.transaction?.id || plan?.transactionId) &&
    !rolledBack && plan?.recoveryStatus !== "completed";
  const retryEnabled = retryTask !== "" && busy === "" && !rateLimitBlocked && (
    retryTask === "assisted" ? assistedRetryReady :
      retryTask === "rollback" ? rollbackRetryReady :
        retryTask === "codex" ? !!preview || !!projectScan :
          retryTask === "source" ? !!source.trim() :
            retryTask === "standard" ? !!selectedSkills.length && !hasBlockingWarnings && assessmentAllowsInstall :
              true
  );

  const dismiss = useCallback(() => {
    clearInstallDraft();
    const status = plan?.status?.toLowerCase() ?? "";
    const preserveRecovery = !!plan && !rolledBack &&
      !["completed", "partial", "rolled-back"].includes(status);
    if (!preserveRecovery) clearActivePlan();
    close();
  }, [close, plan, rolledBack]);

  const hydrateRestoredPlan = useCallback((restored: AssistedInstallPlan) => {
    setInstallMethod("assisted");
    setPlan(restored);
    executionStartedRef.current = !!restored.transactionId;
    activeExecutionRunRef.current = restored.transactionId ?? "";
    executionStartedAtRef.current = 0;
    setExecutionStarted(!!restored.transactionId);
    setSelectedSkills(restoredSelectedSkills(restored));
    setSelectedPermissions(
      restored.permissions.filter(permission => permission.approved).map(permission => permission.id)
    );
    setProjectRoot(restored.projectRoot ?? "");
    if (restored.sourcePlanId) {
      void api.getAssessment(restored.sourcePlanId).then(setAssessment).catch(error => {
        setAssessment(null);
        setFeedback(issueFrom(error, t, t(
          "无法恢复必选分层评估。旧计划不可继续，请重新检查来源。",
          "The required layered assessment could not be restored. Recheck the source before continuing."
        )));
        setRetryTask("");
      });
    }
  }, []);

  useEffect(() => {
    saveInstallDraft({ installMethod, sourceMethod, source, requestedRef });
  }, [installMethod, requestedRef, source, sourceMethod]);

  useEffect(() => {
    planIdRef.current = plan?.id ?? "";
  }, [plan?.id]);

  useEffect(() => {
    progressRef.current = progress;
  }, [progress]);

  useEffect(() => {
    executionStartedRef.current = executionStarted;
  }, [executionStarted]);

  const mergeProgress = useCallback((incoming: AssistedInstallProgress) => {
    const expected = planIdRef.current;
    const incomingIDs = [incoming.referenceId, incoming.runId].filter(Boolean);
    if (expected && incomingIDs.length && !incomingIDs.includes(expected) &&
      progressRef.current && !incomingIDs.includes(progressRef.current.referenceId) &&
      !incomingIDs.includes(progressRef.current.runId)) return;
    if (executionStartedRef.current) {
      if (!isExecutionProgress(incoming)) return;
      const incomingStartedAt = Date.parse(incoming.startedAt);
      if (executionStartedAtRef.current && Number.isFinite(incomingStartedAt) &&
        incomingStartedAt + 50 < executionStartedAtRef.current) return;
      if (incoming.runId) {
        if (activeExecutionRunRef.current && activeExecutionRunRef.current !== incoming.runId) return;
        activeExecutionRunRef.current = incoming.runId;
      }
    } else if (activeAnalysisReferenceRef.current) {
      if (incoming.referenceId !== activeAnalysisReferenceRef.current) return;
      const incomingStartedAt = Date.parse(incoming.startedAt);
      if (analysisStartedAtRef.current && Number.isFinite(incomingStartedAt) &&
        incomingStartedAt + 50 < analysisStartedAtRef.current) return;
    }
    setProgress(current => mergeProgressSnapshot(current, incoming));
    if (executionStartedRef.current && incoming.terminal &&
      (incoming.phase === "completed" || incoming.phase === "partial") &&
      (!activeExecutionRunRef.current || activeExecutionRunRef.current === incoming.runId)) {
      clearActivePlan();
    }
  }, []);

  const syncProgress = useCallback(async (referenceId: string, quiet = true) => {
    if (!referenceId) return;
    try {
      const snapshot = await api.getAssistedProgress(referenceId);
      if (snapshot.referenceId || snapshot.runId || snapshot.sequence > 0) mergeProgress(snapshot);
    } catch (error) {
      if (!quiet) {
        setFeedback(issueFrom(error, t));
        setRetryTask("restore");
      }
    }
  }, [mergeProgress, t]);

  useEffect(() => api.onAssistedInstallProgress(mergeProgress), [mergeProgress]);

  useEffect(() => {
    if (restoreStarted.current) return;
    restoreStarted.current = true;
    const activeReference = readActiveInstallReference();
    if (!activeReference) {
      setBusy("");
      return;
    }
    void (async () => {
      let keepAnalysisBusy = false;
      setFeedback({
        tone: "info",
        title: t("正在恢复安装任务", "Restoring installation"),
        message: t("正在读取上次保存的计划与执行状态。", "Loading the last saved plan and execution status.")
      });
      try {
        const restoredPlan = await api.getAssistedPlan(activeReference.id);
        hydrateRestoredPlan(restoredPlan);
        setActiveInstallReference({ kind: "plan", id: restoredPlan.id });
        setFeedback(isRecoveredFailure(restoredPlan) ? {
          tone: "warning",
          title: t("检测到未完成的安装", "An unfinished installation was found"),
          message: restoredPlan.transactionId
            ? t("请先回滚已完成步骤，再重新分析或新建计划。旧计划不会被直接重试。",
              "Roll back completed steps before analyzing again or creating a new plan. The old plan will not be retried directly.")
            : t("该计划已中断且没有可用事务记录。请开始新的分析，不要直接重试旧计划。",
              "This plan was interrupted without an available transaction journal. Start a new analysis instead of retrying the old plan.")
        } : {
          tone: "success",
          title: t("已恢复安装计划", "Installation plan restored"),
          message: t("可继续审核权限或查看当前执行状态。", "Continue reviewing permissions or inspect the current execution state.")
        });
        await syncProgress(restoredPlan.id, true);
      } catch (error) {
        if (activeReference.kind === "analysis") {
          try {
            const restoredScan = await api.getProjectScan(activeReference.id);
            setInstallMethod("assisted");
            setProjectScan(restoredScan);
            activeAnalysisReferenceRef.current = restoredScan.sourcePlanId;
            analysisStartedAtRef.current = 0;
            setActiveInstallReference({ kind: "analysis", id: restoredScan.sourcePlanId });
            setFeedback({
              tone: "success",
              title: t("项目扫描已恢复", "Project scan restored"),
              message: t(
                "请先阅读项目概述、安全结论和安装方式，再决定是否授权生成安装计划。",
                "Review the overview, security conclusion, and installation methods before authorizing an installation plan."
              )
            });
            setRetryTask("");
            return;
          } catch {
            // The scan may still be running; restore its progress below.
          }
          try {
            const snapshot = await api.getAssistedProgress(activeReference.id);
            activeAnalysisReferenceRef.current = activeReference.id;
            analysisStartedAtRef.current = Date.parse(snapshot.startedAt) || 0;
            setInstallMethod("assisted");
            mergeProgress(snapshot);
            if (!snapshot.terminal) {
              setFeedback({
                tone: "info",
                title: t("Codex 分析仍在进行", "Codex analysis is still running"),
                message: t(
                  "已恢复后台分析进度。可以继续查看，也可以隐藏窗口后稍后再打开。",
                  "Background analysis progress was restored. Keep watching or hide the dialog and reopen it later."
                )
              });
              setRetryTask("");
              restoredAnalysisPollingRef.current = true;
              keepAnalysisBusy = true;
              setBusy("codex");
              return;
            }
            clearActivePlan();
            activeAnalysisReferenceRef.current = "";
            setFeedback({
              tone: snapshot.phase === "cancelled" ? "warning" : "error",
              title: t("Codex 分析未完成", "Codex analysis did not complete"),
              message: snapshot.message || t(
                "上次分析已中断，请重新分析来源。",
                "The previous analysis was interrupted. Analyze the source again."
              ),
              detail: snapshot.error
            });
            setRetryTask(source.trim() ? "source" : "");
            return;
          } catch {
            // Fall through to the structured restore error below.
          }
        }
        setFeedback(issueFrom(error, t, t(
          "无法恢复上次的安装计划。你可以重试，或开始新的安装。",
          "The previous installation plan could not be restored. Retry or start a new installation."
        )));
        setRetryTask("restore");
      } finally {
        if (!keepAnalysisBusy) setBusy("");
      }
    })();
  }, [hydrateRestoredPlan, mergeProgress, source, syncProgress, t]);

  useEffect(() => {
    if (!executionProgress || executionProgress.terminal) return;
    const referenceId = executionProgress.referenceId || executionProgress.runId || plan?.id || "";
    if (!referenceId) return;
    const timer = window.setInterval(() => void syncProgress(referenceId, true), 1800);
    const onVisibility = () => {
      if (document.visibilityState === "visible") void syncProgress(referenceId, true);
    };
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, [executionProgress?.referenceId, executionProgress?.runId, executionProgress?.terminal, plan?.id, syncProgress]);

  useEffect(() => {
    const referenceId = activeAnalysisReferenceRef.current;
    if (!restoredAnalysisPollingRef.current || !referenceId || plan || busy !== "codex") return;
    let disposed = false;
    const poll = async () => {
      let snapshot: AssistedInstallProgress | null = null;
      try {
        snapshot = await api.getAssistedProgress(referenceId);
        if (disposed) return;
        mergeProgress(snapshot);
      } catch {
        // The plan lookup below remains the authoritative completion check.
      }
      try {
        const restored = await api.getAssistedPlan(referenceId);
        if (disposed) return;
        restoredAnalysisPollingRef.current = false;
        activeAnalysisReferenceRef.current = "";
        analysisStartedAtRef.current = 0;
        hydrateRestoredPlan(restored);
        setActiveInstallReference({ kind: "plan", id: restored.id });
        setBusy("");
        setFeedback({
          tone: "success",
          title: t("安装计划已生成", "Installation plan ready"),
          message: t("后台分析已完成，请核对步骤和权限。", "Background analysis completed. Review the steps and permissions.")
        });
        setRetryTask("");
        return;
      } catch {
        try {
          const restoredScan = await api.getProjectScan(referenceId);
          if (disposed) return;
          restoredAnalysisPollingRef.current = false;
          activeAnalysisReferenceRef.current = restoredScan.sourcePlanId;
          analysisStartedAtRef.current = 0;
          setProjectScan(restoredScan);
          setActiveInstallReference({ kind: "analysis", id: restoredScan.sourcePlanId });
          setBusy("");
          setFeedback({
            tone: "success",
            title: t("项目扫描已完成", "Project scan complete"),
            message: t(
              "后台扫描已完成。请阅读结果，再决定是否授权生成安装计划。",
              "The background scan completed. Review it before authorizing an installation plan."
            )
          });
          setRetryTask("");
          return;
        } catch {
          if (!snapshot?.terminal || disposed) return;
        }
      }
      restoredAnalysisPollingRef.current = false;
      activeAnalysisReferenceRef.current = "";
      analysisStartedAtRef.current = 0;
      clearActivePlan();
      setBusy("");
      setFeedback({
        tone: snapshot?.phase === "cancelled" ? "warning" : "error",
        title: t("Codex 分析未完成", "Codex analysis did not complete"),
        message: snapshot?.message || t("请重新分析来源。", "Analyze the source again."),
        detail: snapshot?.error
      });
      setRetryTask(source.trim() ? "source" : "");
    };
    void poll();
    const timer = window.setInterval(() => void poll(), 1800);
    return () => {
      disposed = true;
      window.clearInterval(timer);
    };
  }, [busy, hydrateRestoredPlan, mergeProgress, plan, source, t]);

  useEffect(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const frame = window.requestAnimationFrame(() => {
      const dialog = dialogRef.current;
      if (!dialog) return;
      const preferred = dialog.querySelector<HTMLElement>(
        "[autofocus], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex='-1'])",
      );
      (preferred ?? dialog).focus();
    });
    return () => {
      window.cancelAnimationFrame(frame);
      if (previous?.isConnected) previous.focus();
    };
  }, []);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        if (!cannotClose) dismiss();
        return;
      }
      if (event.key !== "Tab" || !dialogRef.current) return;
      const focusable = Array.from(dialogRef.current.querySelectorAll<HTMLElement>(
        "button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), details > summary, [tabindex]:not([tabindex='-1'])"
      )).filter(element => !element.hasAttribute("hidden"));
      if (!focusable.length) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (!dialogRef.current.contains(document.activeElement)) {
        event.preventDefault();
        (event.shiftKey ? last : first).focus();
        return;
      }
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [cannotClose, dismiss]);

  useEffect(() => {
    if (!executionProgress?.terminal || result) return;
    setBusy(current => current === "cancel" || current === "assisted" ? "" : current);
    if (executionProgress.phase === "cancelled") {
      setFeedback({
        tone: "warning",
        title: t("执行已取消", "Execution cancelled"),
        message: executionProgress.message || t(
          "后端已完成安全停止；请检查恢复状态后决定是否重试。",
          "The backend stopped safely. Review the recovery status before retrying."
        ),
        detail: executionProgress.error
      });
      setRetryTask("assisted");
    } else if (executionProgress.error) {
      setFeedback({
        tone: "error",
        title: t("安装未完成", "Installation did not complete"),
        message: executionProgress.error,
        detail: executionProgress.error
      });
      setRetryTask("assisted");
    } else if (executionProgress.phase === "completed") {
      setFeedback({
        tone: "success",
        title: t("安装已完成", "Installation completed"),
        message: executionProgress.message || t("计划中的自动步骤已执行完毕。", "The automated plan steps have finished.")
      });
    }
  }, [executionProgress?.terminal, executionProgress?.error, executionProgress?.phase, executionProgress?.message, result, t]);

  const resetForNewSource = () => {
    if (cannotClose) return;
    analysisGeneration.current++;
    activeAnalysisReferenceRef.current = "";
    analysisStartedAtRef.current = 0;
    restoredAnalysisPollingRef.current = false;
    clearActivePlan();
    setPreview(null);
    setAssessment(null);
    setProjectScan(null);
    setPlan(null);
    setProgress(null);
    setResult(null);
    executionStartedRef.current = false;
    activeExecutionRunRef.current = "";
    executionStartedAtRef.current = 0;
    setExecutionStarted(false);
    setStandardResult(null);
    setSelectedSkills([]);
    setSelectedPermissions([]);
    setProjectRoot("");
    setFeedback(null);
    setRetryTask("");
    setRolledBack(false);
  };

  const scanWithCodex = async (sourcePlanId: string) => {
    setInstallMethod("assisted");
    const generation = ++analysisGeneration.current;
    activeAnalysisReferenceRef.current = sourcePlanId;
    analysisStartedAtRef.current = Date.now();
    restoredAnalysisPollingRef.current = false;
    progressRef.current = null;
    setProgress(null);
    executionStartedRef.current = false;
    activeExecutionRunRef.current = "";
    executionStartedAtRef.current = 0;
    setExecutionStarted(false);
    setActiveInstallReference({ kind: "analysis", id: sourcePlanId });
    setBusy("codex");
    setFeedback({
      tone: "info",
	  title: t("Codex 正在扫描项目", "Codex is scanning the project"),
      message: t(
	    "正在按“本地结果、文件摘要、重点文件分析”生成项目概述、安全结论和安装方式；此阶段不会生成或执行安装计划。",
	    "Building the overview, security conclusion, and installation methods from local results, file summaries, and focused analysis. No installation plan is generated or executed yet."
      )
    });
    setRetryTask("");
    try {
      const analyzed = await api.scanProject(sourcePlanId);
      if (generation !== analysisGeneration.current) return;
	  setProjectScan(analyzed);
      setSelectedPermissions([]);
      setFeedback({
        tone: "success",
	    title: t("项目扫描已完成", "Project scan complete"),
        message: t(
	      `已汇总 ${analyzed.summaryFileCount || 0} 个文件，并深度分析 ${analyzed.deepAnalysisFileCount || 0} 个重点文件。请先阅读结论，再决定是否授权生成安装计划。`,
	      `${analyzed.summaryFileCount || 0} files were summarized and ${analyzed.deepAnalysisFileCount || 0} focus files were analyzed in depth. Review the result before authorizing an installation plan.`
        )
      });
    } catch (error) {
      if (generation !== analysisGeneration.current) return;
      restoredAnalysisPollingRef.current = false;
      // Keep the typed analysis reference until the user explicitly dismisses
      // or retries. If the dialog was hidden, the persisted terminal progress
      // is then available when the task is reopened.
      const issue = issueFrom(error, t, t(
	    "Codex 未能完成项目扫描。已保留本地来源分析结果，可重试或切换到标准安装。",
	    "Codex could not complete the project scan. The local source analysis is preserved; retry or switch to standard installation."
      ));
      setFeedback(issue);
      setRetryTask(issue.retryable === false ? "" : "codex");
      throw error;
    } finally {
      if (generation === analysisGeneration.current) setBusy("");
    }
  };

  const createPlanFromProjectScan = async () => {
    if (!projectScan || !assessment || (assessment.gate !== "ready" && assessment.gate !== "attention")) {
      setFeedback({
        tone: "error",
        title: t("必选检查未通过", "Required assessment is not available"),
        message: t("请重新检查来源；未通过本地分层评估时不会调用 Codex 或访问 PyPI。",
          "Recheck the source. Codex and PyPI are unavailable until the local layered assessment passes."),
        retryable: false,
        restartRequired: true
      });
      return;
    }
    const generation = ++analysisGeneration.current;
    activeAnalysisReferenceRef.current = projectScan.sourcePlanId;
    analysisStartedAtRef.current = Date.now();
    restoredAnalysisPollingRef.current = false;
    progressRef.current = null;
    setProgress(null);
    setBusy("codex");
    setFeedback({
      tone: "info",
      title: t("正在制定安装计划", "Creating installation plan"),
      message: t(
        "你已同意继续。Codex 将以已验证的项目扫描为输入制定计划；本地管理器仍会限制动作类型并生成逐项权限。",
        "You approved continuing. Codex is using the verified project scan to propose a plan; the local manager still restricts action types and derives per-action permissions."
      )
    });
    setRetryTask("");
    try {
      const analyzed = await api.createAssistedPlanFromScan(projectScan.id);
      if (generation !== analysisGeneration.current) return;
      setPlan(analyzed);
      setActiveInstallReference({ kind: "plan", id: analyzed.id });
      const analyzedCandidates = planCandidates(analyzed);
      if (analyzedCandidates.length) {
        setSelectedSkills(current => current.length ? current : analyzedCandidates.map(skill => skill.name));
      }
      setSelectedPermissions([]);
      setFeedback({
        tone: "success",
        title: t("安装计划已生成", "Installation plan ready"),
        message: t(
          "请核对自动步骤、人工步骤和逐项权限后再执行。",
          "Review automatic steps, manual steps, and per-action permissions before execution."
        )
      });
    } catch (error) {
      if (generation !== analysisGeneration.current) return;
      const issue = issueFrom(error, t, t(
        "Codex 未能生成可靠计划。项目扫描结果仍然保留，可重试或切换到标准安装。",
        "Codex could not produce a reliable plan. The project scan is preserved; retry or switch to standard installation."
      ));
      setFeedback(issue);
      setRetryTask(issue.retryable === false ? "" : "codex");
    } finally {
      if (generation === analysisGeneration.current) setBusy("");
    }
  };

  const cancelCodexAnalysis = async () => {
    const referenceId = preview?.id || activeAnalysisReferenceRef.current;
    if (!referenceId) return;
    const cancelledGeneration = analysisGeneration.current;
    analysisGeneration.current++;
    setBusy("cancel");
    setFeedback({
      tone: "warning",
      title: t("正在取消 Codex 分析", "Cancelling Codex analysis"),
      message: t("取消后会保留本地来源分析和安全检查结果。", "Local source analysis and safety results will be preserved.")
    });
    try {
      await api.cancelAssisted(referenceId);
      if (analysisGeneration.current !== cancelledGeneration + 1) return;
      activeAnalysisReferenceRef.current = "";
      analysisStartedAtRef.current = 0;
      restoredAnalysisPollingRef.current = false;
      clearActivePlan();
      setFeedback({
        tone: "warning",
        title: t("Codex 分析已取消", "Codex analysis cancelled"),
        message: t("可重新分析，或直接切换到标准安装。", "Analyze again or switch directly to standard installation.")
      });
      setRetryTask("codex");
      setBusy("");
    } catch (error) {
      if (analysisGeneration.current !== cancelledGeneration + 1) return;
      activeAnalysisReferenceRef.current = "";
      analysisStartedAtRef.current = 0;
      restoredAnalysisPollingRef.current = false;
      setFeedback(issueFrom(error, t));
      setRetryTask("codex");
      setBusy("");
    }
  };

  const analyzeSourceUnified = async (
    sourceOverride?: string,
    sourceMethodOverride?: SourceMethod,
    requestedRefOverride?: string
  ) => {
    const effectiveSourceMethod = sourceMethodOverride ?? sourceMethod;
    const trimmed = (sourceOverride ?? source).trim();
    if (!trimmed) return;
    setInstallMethod("standard");
    setBusy("source");
    setFeedback({ tone: "info", title: t("正在准备来源", "Preparing source"),
      message: t("正在固定来源、创建受管快照并运行本地安全检查。", "Pinning the source, creating a managed snapshot, and running local safety checks.") });
    setRetryTask("");
    setPreview(null);
    setAssessment(null);
    setProjectScan(null);
    setPlan(null);
    setProgress(null);
    setResult(null);
    executionStartedRef.current = false;
    setExecutionStarted(false);
    setStandardResult(null);
    setSelectedPermissions([]);
    try {
      const analyzed = effectiveSourceMethod === "github"
        ? await api.prepareGitHub(trimmed, (requestedRefOverride ?? requestedRef).trim())
        : await api.prepareLocal(trimmed);
      setPreview(analyzed);
      setSelectedSkills(analyzed.skills.map(skill => skill.name));
      setFeedback({ tone: "info", title: t("正在执行必选检查", "Running required checks"),
        message: t("正在确认项目类型、覆盖范围、安装目标和恢复能力。", "Confirming project type, coverage, install targets, and recovery.") });
      const assessed = await api.assessSource(analyzed.id);
      setAssessment(assessed);
      setFeedback({
        tone: assessed.gate === "blocked" || assessed.gate === "incomplete" ? "warning" : "success",
        title: assessed.gate === "ready" ? t("必选检查已通过", "Required checks passed") :
          assessed.gate === "attention" ? t("检查完成，需要确认", "Assessment complete; review needed") :
            assessed.gate === "blocked" ? t("安全策略已阻止安装", "Security policy blocked installation") :
              t("检查尚未完成", "Assessment incomplete"),
        message: assessed.summary
      });
    } catch (error) {
      const issue = issueFrom(error, t);
      setFeedback(issue);
      setRetryTask(issue.retryable === false ? "" : "source");
    } finally {
      setBusy("");
    }
  };

  const useSuggestedSource = (url: string) => {
    if (!url || busy !== "") return;
    setSourceMethod("github");
    setSource(url);
    setRequestedRef("");
    void analyzeSourceUnified(url, "github", "");
  };

  const refreshWithinDialog = async (): Promise<boolean> => {
    try {
      await refresh();
      return true;
    } catch (error) {
      const issue = issueFrom(error, t);
      setFeedback({
        tone: "warning",
        title: t("操作已完成，但界面刷新失败", "The operation completed, but refresh failed"),
        message: t(
          "实际修改已经完成；安装窗口会保留结果。可重试刷新，不要重复执行安装。",
          "The change itself completed and its result is preserved here. Retry the refresh; do not repeat the installation."
        ),
        detail: issue.detail
      });
      setRetryTask("refresh");
      return false;
    }
  };

  const installStandard = async () => {
    if (!preview || !selectedSkills.length) return;
    setBusy("standard");
    setFeedback({
      tone: "info",
      title: t("正在安装 Skills", "Installing Skills"),
      message: t("正在写入选中的 Skills，并记录备份、来源和操作日志。", "Writing selected Skills and recording backups, provenance, and the operation journal.")
    });
    setRetryTask("");
    try {
		const transaction = await api.apply(preview.id, selectedSkills,
			(preview.scan.clusters ?? []).some(cluster => cluster.severity === "high" && cluster.ignored));
      setStandardResult(transaction);
      if (!await refreshWithinDialog()) return;
      setFeedback({
        tone: "success",
        title: t("安装完成", "Installation completed"),
        message: t(
          `已安装 ${selectedSkills.length} 个 Skills，可在“历史与回滚”中查看记录。`,
          `Installed ${selectedSkills.length} Skills. The record is available in History & Rollback.`
        )
      });
    } catch (error) {
      const issue = issueFrom(error, t);
      setFeedback(issue);
      setRetryTask(issue.retryable === false ? "" : "standard");
    } finally {
      setBusy("");
    }
  };

  const updateIgnoredClusters = (report: ScanReport, clusters: RiskCluster[], reason: string): ScanReport => {
    const ids = new Set(clusters.map(cluster => cluster.id));
    const nextClusters = report.clusters.map(cluster => ids.has(cluster.id)
      ? {
        ...cluster,
        ignored: true,
        ignoreReason: reason,
        sampleFindings: cluster.sampleFindings.map(finding => ({ ...finding, ignored: true, ignoreReason: reason }))
      }
      : cluster);
    const nextFindings = report.findings.map(finding => ids.has(finding.clusterId)
      ? { ...finding, ignored: true, ignoreReason: reason }
      : finding);
    const active = nextClusters.filter(cluster => !cluster.ignored);
    return {
      ...report,
      clusters: nextClusters,
      findings: nextFindings,
      activeFindingCount: active.reduce((total, cluster) => total + Math.max(cluster.findingCount, 1), 0),
      ignoredFindingCount: nextClusters.filter(cluster => cluster.ignored)
        .reduce((total, cluster) => total + Math.max(cluster.findingCount, 1), 0),
      activeHighestSeverity: highestSeverity(active)
    };
  };

  const ignoreClusters = async (clusters: RiskCluster[]) => {
    if (!clusters.length) return;
		if (clusters.some(cluster => cluster.severity === "critical")) {
			setFeedback({
				tone: "error",
				title: t("严重风险不可忽略", "Critical risk cannot be ignored"),
				message: t("Critical 属于强制安全底线。请更换来源或修复风险内容后重新检查。",
					"Critical findings are a mandatory safety boundary. Fix or replace the source, then reassess."),
				retryable: false
			});
			return;
		}
		const highRisk = clusters.filter(cluster => cluster.severity === "high");
		if (highRisk.length > 0 && clusters.length !== 1) {
			setFeedback({
				tone: "warning",
				title: t("高风险必须逐项确认", "High risk requires individual review"),
				message: t("请展开详情，逐个阅读并记录接受原因。批量操作不会接受 High 风险。",
					"Open the details and review each High finding individually. Batch actions cannot accept High risk."),
				retryable: false
			});
			return;
		}
		let reason = "";
		if (highRisk.length === 1) {
			reason = window.prompt(t(
				"请输入接受此 High 风险的具体原因（必填）：",
				"Enter the specific reason for accepting this High risk (required):"
			))?.trim() ?? "";
			if (!reason || !window.confirm(t(
				"确认已理解此 High 风险，并仅接受当前风险簇？",
				"Confirm that you understand this High risk and accept only this cluster?"
			))) return;
		}
    setBusy("risk");
    setFeedback({
      tone: "warning",
      title: t("正在记录人工决定", "Recording manual decision"),
      message: t(`正在忽略 ${clusters.length} 个警告组。`, `Ignoring ${clusters.length} warning groups.`)
    });
    try {
			if (highRisk.length === 1) {
				await api.setRiskClusterIgnored(highRisk[0], true, reason, true);
			} else {
				await api.setRiskClustersIgnored(clusters, true, reason);
			}
			const assessmentSource = preview?.id || assessment?.sourcePlanId;
			const refreshedAssessment = assessmentSource ? await api.assessSource(assessmentSource) : null;
      setPreview(current => current ? { ...current, scan: updateIgnoredClusters(current.scan, clusters, reason) } : current);
      setPlan(current => current?.scan ? { ...current, scan: updateIgnoredClusters(current.scan, clusters, reason) } : current);
			if (refreshedAssessment) setAssessment(refreshedAssessment);
      setFeedback({
        tone: "success",
        title: t("警告已忽略", "Warnings ignored"),
        message: t(
          "人工决定已保存。你仍可在安全中心查看原始命中。",
          "The manual decision was saved. Original findings remain available in Security."
        )
      });
    } catch (error) {
      setFeedback(issueFrom(error, t));
    } finally {
      setBusy("");
    }
  };

  const executeAssisted = async () => {
    if (!plan) return;
    const previousTransactionId = plan.transactionId || "";
    if (projectRootRequired && !projectRoot.trim()) {
      setFeedback({
        tone: "warning",
        title: t("需要项目目录", "Project directory required"),
        message: plan.projectRootReason || t(
          "请输入本次 MCP 配置要使用的项目目录。",
          "Enter the project directory to use for this MCP configuration."
        )
      });
      return;
    }
    if (missingPermissionCount > 0) {
      setFeedback({
        tone: "warning",
        title: t("尚未确认必要权限", "Required permissions are not confirmed"),
        message: t(
          `还需要确认 ${missingPermissionCount} 项必要权限。`,
          `${missingPermissionCount} required permissions still need confirmation.`
        )
      });
      return;
    }
    if (permissionDependencyIssue) {
      setFeedback({
        tone: "warning",
        title: t("权限组合不完整", "Permission combination is incomplete"),
        message: permissionDependencyIssue
      });
      return;
    }
    if (candidates.length && !selectedSkills.length) return;
    setBusy("assisted");
    setResult(null);
    setRolledBack(false);
    activeExecutionRunRef.current = "";
    executionStartedAtRef.current = Date.now();
    activeAnalysisReferenceRef.current = "";
    analysisStartedAtRef.current = 0;
    executionStartedRef.current = true;
    setExecutionStarted(true);
    setActiveInstallReference({ kind: "plan", id: plan.id });
    setFeedback({
      tone: "info",
      title: t("正在执行安装计划", "Executing installation plan"),
      message: t("每个步骤的状态会实时更新；如需停止，请使用“取消执行”。", "Step status updates in real time. Use “Cancel execution” if you need to stop.")
    });
    setRetryTask("");
    const startingProgress = initialProgress(plan);
    progressRef.current = startingProgress;
    setProgress(startingProgress);
    try {
      const completed = await api.applyAssisted(
        plan.id,
        selectedSkills,
        selectedPermissions,
        projectRoot.trim()
      );
      setResult(completed);
      setExecutionStarted(true);
      setPlan(completed.plan);
      if (completed.progress) {
        // This is the authoritative response to the exact apply request. The
        // freshness checks in mergeProgress are for asynchronous events and
        // restored snapshots; applying them here can incorrectly retain the
        // initial 0% state when backend and renderer clocks differ.
        activeExecutionRunRef.current = completed.progress.runId || completed.runId || "";
        progressRef.current = completed.progress;
        setProgress(completed.progress);
      } else {
        setProgress(current => completedProgress(completed.plan, current, t));
      }
      if (completed.transaction.status === "completed" || completed.transaction.status === "partial") clearActivePlan();
      else setActiveInstallReference({ kind: "plan", id: completed.plan.id });
      if (!await refreshWithinDialog()) return;
      setFeedback({
        tone: completed.transaction.status === "completed" ? "success" : "warning",
        title: completed.transaction.status === "completed"
          ? t("一键安装完成", "Assisted installation completed")
          : t("安装部分完成", "Installation partially completed"),
        message: completed.transaction.status === "completed"
          ? t("Skills、配置和验证步骤已完成。", "Skills, configuration, and verification steps completed.")
          : t("部分步骤未完成，请查看时间线和恢复建议。", "Some steps did not complete. Review the timeline and recovery guidance.")
      });
    } catch (error) {
      const referenceId = plan.id;
      let terminalProgress: AssistedInstallProgress | null = null;
      try {
        terminalProgress = await api.getAssistedProgress(referenceId);
      } catch {
        // The transaction journal and the original error remain available.
      }
      let latestPlan = plan;
      try {
        latestPlan = await api.getAssistedPlan(plan.id);
        setPlan(latestPlan);
      } catch {
        // Progress and the transaction journal remain the recovery source.
      }
      if (terminalProgress && latestPlan.recoveryStatus === "completed" && latestPlan.steps.length) {
        terminalProgress = reconcileProgressWithPlan(terminalProgress, latestPlan);
      }
      const snapshotStartedAt = terminalProgress ? Date.parse(terminalProgress.startedAt) : Number.NaN;
      const hasExecutionSnapshot = !!terminalProgress && isExecutionProgress(terminalProgress) &&
        terminalProgress.runId !== previousTransactionId &&
        (!executionStartedAtRef.current || !Number.isFinite(snapshotStartedAt) ||
          snapshotStartedAt + 50 >= executionStartedAtRef.current);
      const hasNewPersistedTransaction = !!latestPlan.transactionId &&
        latestPlan.transactionId !== previousTransactionId;
      if (terminalProgress && hasExecutionSnapshot) {
        mergeProgress(terminalProgress);
      } else if (hasNewPersistedTransaction) {
        executionStartedRef.current = true;
        activeExecutionRunRef.current = latestPlan.transactionId || "";
        executionStartedAtRef.current = 0;
        setExecutionStarted(true);
        const persisted = persistedPlanProgress(latestPlan, t);
        progressRef.current = persisted;
        setProgress(persisted);
      } else {
        executionStartedRef.current = false;
        activeExecutionRunRef.current = "";
        executionStartedAtRef.current = 0;
        setExecutionStarted(false);
        progressRef.current = null;
        setProgress(null);
      }
      const authoritativePhase = (hasExecutionSnapshot ? terminalProgress?.phase : latestPlan.status)?.toLowerCase() || "";
      if (authoritativePhase === "completed" || authoritativePhase === "partial") {
        clearActivePlan();
        setRetryTask("");
        if (!await refreshWithinDialog()) return;
        setFeedback({
          tone: authoritativePhase === "completed" ? "success" : "warning",
          title: authoritativePhase === "completed"
            ? t("一键安装完成", "Assisted installation completed")
            : t("自动步骤已完成", "Automatic steps completed"),
          message: terminalProgress?.message || (authoritativePhase === "completed" ? t(
            "后端已完成安装，界面已读取最新状态。",
            "The backend completed the installation and the interface now shows the latest state."
          ) : t(
            "受支持的步骤已经完成；计划中的人工步骤仍需按说明处理。",
            "Supported steps completed. Follow the plan for the remaining manual work."
          ))
        });
        return;
      }
      const cancelledIssue: Feedback | null = hasExecutionSnapshot && terminalProgress?.phase === "cancelled" ? {
        tone: "warning",
        title: t("执行已取消", "Execution cancelled"),
        message: terminalProgress.message || t(
          "后端已完成安全停止；请检查恢复状态后决定是否重试。",
          "The backend stopped safely. Review the recovery status before retrying."
        ),
        detail: terminalProgress.error
      } : null;
      const executionIssue = cancelledIssue ?? issueFrom(error, t, t(
        "执行已停止。已完成步骤会保留在时间线中，可重试或按恢复建议处理。",
        "Execution stopped. Completed steps remain in the timeline; retry or follow the recovery guidance."
      ));
      setFeedback(executionIssue);
      setRetryTask(executionIssue.retryable === false ? "" : "assisted");
    } finally {
      setBusy("");
    }
  };

  const cancelExecution = async () => {
    if (!plan) return;
    const referenceId = progress?.referenceId || progress?.runId || plan.id;
    setBusy("cancel");
    setFeedback({
      tone: "warning",
      title: t("正在请求取消", "Requesting cancellation"),
      message: t("当前步骤会在安全停止点结束，已完成步骤不会被直接删除。", "The current step will stop at a safe point. Completed steps will not be deleted.")
    });
    try {
      await api.cancelAssisted(referenceId);
      await syncProgress(referenceId, true);
      setFeedback({
        tone: "info",
        title: t("已发送取消请求", "Cancellation requested"),
        message: t(
          "正在等待当前步骤安全停止并恢复已完成的修改；收到后端终态前窗口会保持锁定。",
          "Waiting for the current step to stop safely and recover completed changes. The window stays locked until the backend reports a terminal state."
        )
      });
    } catch (error) {
      setFeedback(issueFrom(error, t));
      setBusy("assisted");
    }
  };

  const rollback = async () => {
    const transactionId = result?.transaction?.id || plan?.transactionId || "";
    if (!transactionId || rolledBack || plan?.recoveryStatus === "completed") return;
    if (!window.confirm(t(
      "回滚本次一键安装？应用会使用事务记录恢复可逆修改。",
      "Roll back this assisted installation? Reversible changes will be restored from the transaction journal."
    ))) return;
    setBusy("rollback");
    setRetryTask("");
    try {
      const rollbackTransaction = await api.rollback(transactionId);
      const recoveredSteps = rollbackTransaction.steps ?? [];
      setRolledBack(true);
      setPlan(current => current ? {
        ...current,
        status: "rolled-back",
        recoveryStatus: "completed",
        steps: recoveredSteps.length ? recoveredSteps : current.steps
      } : current);
      setProgress(current => ({
        ...(current ?? recoveredFailureProgress(plan!, t)),
        sequence: (current?.sequence ?? 0) + 1,
        phase: "rolled-back",
        message: t("已回滚完成步骤", "Completed steps rolled back"),
        completedSteps: recoveredSteps.length
          ? recoveredSteps.filter(step => ["rolled-back", "skipped"].includes(step.status)).length
          : current?.completedSteps ?? 0,
        totalSteps: recoveredSteps.length || current?.totalSteps || plan?.steps.length || 0,
        steps: recoveredSteps.length ? recoveredSteps.map(step => ({
          id: step.id,
          title: step.title,
          kind: step.kind,
          status: step.status,
          startedAt: step.startedAt,
          completedAt: step.completedAt,
          error: step.error
        })) : current?.steps ?? [],
        terminal: true,
        updatedAt: new Date().toISOString()
      }));
      clearActivePlan();
      setRetryTask("");
      if (!await refreshWithinDialog()) return;
      setFeedback({
        tone: "success",
        title: t("回滚完成", "Rollback completed"),
        message: t("可逆修改已按事务记录恢复。现在可以重新分析或新建计划。",
          "Reversible changes were restored from the transaction journal. You can now analyze again or create a new plan.")
      });
    } catch (error) {
      setFeedback(issueFrom(error, t));
      setRetryTask("rollback");
    } finally {
      setBusy("");
    }
  };

  const reviewBeforeRetry = () => {
    if (!plan || isRecoveredFailure(plan)) return;
    progressRef.current = null;
    setProgress(null);
    setResult(null);
    executionStartedRef.current = false;
    activeExecutionRunRef.current = "";
    executionStartedAtRef.current = 0;
    setExecutionStarted(false);
    setSelectedPermissions(
      plan.permissions.filter(permission => permission.approved).map(permission => permission.id)
    );
    setProjectRoot(plan.projectRoot ?? projectRoot);
    setActiveInstallReference({ kind: "plan", id: plan.id });
    setRetryTask("");
    setFeedback({
      tone: "info",
      title: t("请重新核对后重试", "Review before retrying"),
      message: t(
        "已恢复权限和项目目录。请重新核对 Skills、权限与剩余步骤，再开始执行。",
        "Permissions and the project directory were restored. Review the Skills, permissions, and remaining steps before running again."
      )
    });
  };

  const retry = () => {
    if (!retryEnabled) return;
    if (retryTask === "restore") {
      const activeReference = readActiveInstallReference();
      if (!activeReference) {
        resetForNewSource();
        return;
      }
      setBusy("restore");
      void api.getAssistedPlan(activeReference.id).then(restored => {
        hydrateRestoredPlan(restored);
        setActiveInstallReference({ kind: "plan", id: restored.id });
        return syncProgress(restored.id, true);
      }).then(() => {
        setFeedback({
          tone: "success",
          title: t("安装计划已恢复", "Installation plan restored"),
          message: t("可以继续处理。", "You can continue.")
        });
        setRetryTask("");
      }).catch(error => setFeedback(issueFrom(error, t))).finally(() => setBusy(""));
    } else if (retryTask === "source") void analyzeSourceUnified();
    else if (retryTask === "codex" && projectScan) void createPlanFromProjectScan();
    else if (retryTask === "codex" && preview) void scanWithCodex(preview.id).catch(() => undefined);
    else if (retryTask === "standard") void installStandard();
    else if (retryTask === "assisted") reviewBeforeRetry();
    else if (retryTask === "rollback") void rollback();
    else if (retryTask === "refresh") {
      setBusy("refresh");
      void refreshWithinDialog().then(refreshed => {
        if (refreshed) {
          setFeedback({
            tone: "success",
            title: t("界面已刷新", "Interface refreshed"),
            message: t("已重新读取最新的 Skills 和操作状态。", "The latest Skills and operation status have been loaded.")
          });
          setRetryTask("");
        }
      }).finally(() => setBusy(""));
    }
  };

  const switchToStandard = () => {
    if (cannotClose) return;
    analysisGeneration.current++;
    activeAnalysisReferenceRef.current = "";
    analysisStartedAtRef.current = 0;
    clearActivePlan();
    setInstallMethod("standard");
    setProjectScan(null);
    setPlan(null);
    setProgress(null);
    setResult(null);
    executionStartedRef.current = false;
    activeExecutionRunRef.current = "";
    executionStartedAtRef.current = 0;
    setExecutionStarted(false);
    setSelectedPermissions([]);
    setRetryTask("");
    setFeedback(preview ? {
      tone: "info",
      title: t("已切换到标准安装", "Switched to standard installation"),
      message: t(
        "已保留来源分析、安全检查和 Skill 选择，不会配置 MCP 或额外依赖。",
        "Source analysis, safety checks, and Skill selection were preserved. MCP and extra dependencies will not be configured."
      )
    } : null);
  };

  const renderBody = () => {
		const assessmentPanel = assessment
			? <AssessmentView assessment={assessment} />
			: preview ? <AssessmentView assessment={fallbackIncompleteAssessment(preview)} /> : null;
    if (busy === "restore" && !plan && !preview) {
      return <CenteredState icon={<LoaderCircle className="spin" />} title={t("正在恢复安装计划", "Restoring installation plan")}
        detail={t("请稍候。", "Please wait.")} />;
    }
    if (installMethod === "assisted" && busy === "codex" && progress && !plan) {
      return <>{assessmentPanel}<AnalysisProgressView progress={progress} /></>;
    }
    if (!preview && !plan) {
      return <UnifiedSourceStep
        sourceMethod={sourceMethod}
        source={source}
        requestedRef={requestedRef}
        busy={busy !== ""}
        setSourceMethod={setSourceMethod}
        setSource={setSource}
        setRequestedRef={setRequestedRef}
      />;
    }
    if (preview && !assessment && busy === "source") {
      return <CenteredState icon={<LoaderCircle className="spin" />} title={t("正在执行必选检查", "Running required checks")}
        detail={t("正在生成本地分层安全结论；不会调用 Codex 或下载依赖。", "Building the local layered security result without Codex or dependency downloads.")} />;
    }
    if (installMethod === "standard") {
      return <>{assessmentPanel}<StandardReview
        preview={preview}
        candidates={candidates}
        selected={selectedSkills}
        setSelected={setSelectedSkills}
        scan={scan}
        riskBusy={busy === "risk"}
        onIgnore={ignoreClusters}
        completed={!!standardResult}
      /></>;
    }
    if (projectScan && !plan) {
      return <>{assessmentPanel}<ProjectScanView scan={projectScan} /></>;
    }
    if (!plan) {
      if (busy === "codex" && progress) {
        return <>{assessmentPanel}<AnalysisProgressView progress={progress} /></>;
      }
      return <CenteredState icon={busy === "codex" ? <LoaderCircle className="spin" /> : <CircleAlert />}
        title={busy === "codex" ? t("Codex 正在生成安装计划", "Codex is generating the installation plan") :
          t("安装计划尚未生成", "Installation plan is not available")}
        detail={busy === "codex"
          ? t("正在按固定预算分层处理仓库上下文。", "Processing repository context in bounded layers.")
          : t("可重试 Codex 分析，或使用已完成的来源检查切换到标准安装。", "Retry Codex analysis or use the completed source check for standard installation.")} />;
    }
    if (displayProgress || result) {
      return <>{assessmentPanel}<ExecutionView plan={plan} progress={displayProgress ?? progress} result={result}
        rolledBack={rolledBack} /></>;
    }
    return <>{assessmentPanel}<AssistedPlanView
      plan={plan}
      candidates={candidates}
      selectedSkills={selectedSkills}
      setSelectedSkills={setSelectedSkills}
      selectedPermissions={selectedPermissions}
      setSelectedPermissions={setSelectedPermissions}
      permissionDependencyIssue={permissionDependencyIssue}
      projectRoot={projectRoot}
      projectRootRequired={projectRootRequired}
      setProjectRoot={setProjectRoot}
      scan={scan}
      riskBusy={busy === "risk"}
      onIgnore={ignoreClusters}
    /></>;
  };

  const renderActions = () => {
    if (busy === "restore" && !plan && !preview) {
      return <button type="button" className="ghost" disabled>{t("正在恢复…", "Restoring…")}</button>;
    }
    if (!plan && (busy === "codex" || busy === "cancel")) {
      return <button type="button" className="danger-button" disabled={busy === "cancel"}
        onClick={() => void cancelCodexAnalysis()}>
        {busy === "cancel" ? <LoaderCircle className="spin" size={17} /> : <Square size={15} />}
        {busy === "cancel" ? t("正在取消分析…", "Cancelling analysis…") : t("取消分析", "Cancel analysis")}
      </button>;
    }
    if (!preview && !plan) {
      return <>
        <button type="button" className="ghost" onClick={dismiss} disabled={cannotClose}>{t("取消", "Cancel")}</button>
        <button type="button" className="primary" onClick={() => void analyzeSourceUnified()}
          disabled={!source.trim() || busy !== "" || rateLimitBlocked}>
          {busy === "source" ? <LoaderCircle className="spin" size={17} /> : <Search size={17} />}
          {busy === "source" ? t("正在检查项目…", "Checking project…") : t("检查项目", "Check project")}
        </button>
      </>;
    }
    if (installMethod === "standard") {
      if (standardResult) {
        return <button type="button" className="primary" disabled={busy !== ""} onClick={dismiss}>
          <Check size={17} />{t("完成", "Done")}
        </button>;
      }
      return <>
        <button type="button" className="ghost" disabled={busy !== ""} onClick={resetForNewSource}><ArrowLeft size={16} />{t("返回", "Back")}</button>
        <button type="button" className="ghost" disabled={busy !== "" || !preview || !assessment || assessment.gate === "blocked" || assessment.gate === "incomplete"}
          onClick={() => {
            if (!preview) return;
            setInstallMethod("assisted");
            void scanWithCodex(preview.id).catch(() => undefined);
          }}>
          <Sparkles size={16} />{t("运行增强项目扫描", "Run enhanced project scan")}
        </button>
        <button type="button" className="primary" disabled={busy !== "" || !selectedSkills.length || hasBlockingWarnings || !assessmentAllowsInstall}
          onClick={() => void installStandard()}>
          {busy === "standard" ? <LoaderCircle className="spin" size={17} /> : <Download size={17} />}
          {busy === "standard" ? t("正在安装…", "Installing…") : t(`安装选中的 ${selectedSkills.length} 个`, `Install ${selectedSkills.length} selected`)}
        </button>
      </>;
    }
    if (projectScan && !plan) {
      return <>
        <button type="button" className="ghost" disabled={busy !== ""} onClick={resetForNewSource}>
          <ArrowLeft size={16} />{t("返回", "Back")}
        </button>
        <button type="button" className="ghost" disabled={busy !== "" || !preview} onClick={switchToStandard}>
          {t("切换到标准安装", "Switch to standard installation")}
        </button>
        <button type="button" className="primary" disabled={busy !== "" || !assessment ||
			(assessment.gate !== "ready" && assessment.gate !== "attention")}
          onClick={() => void createPlanFromProjectScan()}>
          <Sparkles size={17} />{t("继续生成计划（可能访问 PyPI）", "Continue to plan generation (may access PyPI)")}
        </button>
      </>;
    }
    if (!plan) {
      if (busy === "codex" || busy === "cancel") {
        return <button type="button" className="danger-button" disabled={busy === "cancel"}
          onClick={() => void cancelCodexAnalysis()}>
          {busy === "cancel" ? <LoaderCircle className="spin" size={17} /> : <Square size={15} />}
          {busy === "cancel" ? t("正在取消分析…", "Cancelling analysis…") : t("取消分析", "Cancel analysis")}
        </button>;
      }
      return <>
        <button type="button" className="ghost" disabled={busy !== ""} onClick={switchToStandard}>
          {t("切换到标准安装", "Switch to standard installation")}
        </button>
        <button type="button" className="primary" disabled={busy !== "" || !preview}
          onClick={() => preview && void scanWithCodex(preview.id).catch(() => undefined)}>
          <RefreshCw size={17} />{t("重试 Codex 分析", "Retry Codex analysis")}
        </button>
      </>;
    }
    if (executionRunning) {
      return <button type="button" className="danger-button" disabled={busy === "cancel"} onClick={() => void cancelExecution()}>
        {busy === "cancel" ? <LoaderCircle className="spin" size={17} /> : <Square size={15} />}
        {busy === "cancel" ? t("正在取消…", "Cancelling…") : t("取消执行", "Cancel execution")}
      </button>;
    }
    if (displayProgress?.terminal || result) {
      const failed = displayProgress?.phase === "failed" ||
        displayProgress?.phase === "cancelled" || displayProgress?.phase === "interrupted" || !!displayProgress?.error;
      const partial = displayProgress?.phase === "partial";
      const oldPlanNeedsRecovery = isRecoveredFailure(plan) && !rolledBack;
      const transactionId = result?.transaction?.id || plan.transactionId;
      const canRollback = !!transactionId && !rolledBack && plan.recoveryStatus !== "completed";
      const recoveredTerminal = failed && plan.recoveryStatus === "completed";
      return <>
        {canRollback && <button type="button" className="ghost" disabled={busy !== ""}
          onClick={() => void rollback()}><RotateCcw size={16} />{t("回滚已完成步骤", "Roll back completed steps")}</button>}
        {failed && !partial && !oldPlanNeedsRecovery && <button type="button" className="ghost" disabled={busy !== ""}
          onClick={reviewBeforeRetry}><RefreshCw size={16} />{t("核对后重试", "Review and retry")}</button>}
        {(rolledBack || recoveredTerminal || (oldPlanNeedsRecovery && !transactionId)) && <button type="button" className="ghost"
          disabled={busy !== ""} onClick={resetForNewSource}><Sparkles size={16} />{t("新建安装计划", "Create a new plan")}</button>}
        <button type="button" className="primary" disabled={busy !== ""} onClick={dismiss}>
          <Check size={17} />{t("完成", "Done")}
        </button>
      </>;
    }
    const permissionsReady = missingPermissionCount === 0;
    const rootReady = !projectRootRequired || !!projectRoot.trim();
    const { manualRequired, manualOnly } = assistedPlanDisposition(plan);
    return <>
      <button type="button" className="ghost" disabled={busy !== ""} onClick={resetForNewSource}>{t("重新开始", "Start over")}</button>
      <button type="button" className="ghost" disabled={busy !== ""} onClick={switchToStandard}>
        {t("切换到标准安装", "Switch to standard installation")}
      </button>
      <button type="button" className="primary" disabled={busy !== "" || !permissionsReady ||
        !!permissionDependencyIssue || !rootReady ||
		(candidates.length > 0 && !selectedSkills.length) || hasBlockingWarnings || manualOnly || !assessmentAllowsInstall}
        onClick={() => void executeAssisted()}>
        <ChevronRight size={17} />{manualOnly
          ? t("没有可自动执行的步骤", "No automatic steps are available")
          : manualRequired
            ? t("批准权限并执行自动步骤", "Approve and run automatic steps")
          : t("批准所选权限并执行", "Approve selected permissions and execute")}
      </button>
    </>;
  };

  return <div className="modal-backdrop install-backdrop">
    <div className="modal install-dialog" ref={dialogRef} role="dialog" aria-modal="true"
      aria-labelledby="install-dialog-title" tabIndex={-1}>
      <div className="modal-head install-dialog-head">
        <div>
          <h2 id="install-dialog-title">{t("检查并添加项目", "Check and add project")}</h2>
          <span>{t("先理解和检查，再确认任何系统变更", "Understand and assess before any system change")}</span>
        </div>
        <button type="button" onClick={canHideInBackground ? close : dismiss}
          disabled={cannotClose && !canHideInBackground}
          aria-label={canHideInBackground
            ? t("隐藏窗口，任务继续在后台运行", "Hide the dialog while the task continues in the background")
            : cannotClose
              ? t("当前操作完成前无法关闭", "The dialog cannot close until the current operation finishes")
              : t("关闭", "Close")}
          title={canHideInBackground
            ? t("隐藏窗口", "Hide dialog")
            : cannotClose
              ? t("请等待当前操作完成", "Wait for the current operation to finish")
              : t("关闭", "Close")}>
          <X />
        </button>
      </div>
      {feedback && <TaskFeedback feedback={feedback} retryTask={retryTask} retryEnabled={retryEnabled} onRetry={retry}
        retryWaitMs={retryWaitMs} onOpenSettings={openSettings} onUseSuggestedSource={useSuggestedSource} />}
      <div className="install-dialog-body">
        <WorkflowStepper current={currentWorkflowStage(preview, assessment, projectScan, plan, standardResult, result, displayProgress)} />
        {renderBody()}
      </div>
      <div className="modal-actions install-dialog-actions">{renderActions()}</div>
    </div>
  </div>;
}

function currentWorkflowStage(
  preview: InstallPreview | null,
  assessment: ProjectAssessment | null,
  projectScan: CodexProjectScanResult | null,
  plan: AssistedInstallPlan | null,
  standardResult: Transaction | null,
  result: AssistedInstallResult | null,
  progress: AssistedInstallProgress | null
): InstallWorkflowStage {
  if (standardResult || result || (progress && plan && isExecutionProgress(progress))) return "apply";
  if (assessment || projectScan || plan) return "review";
  if (preview) return "assess";
  return "source";
}

function fallbackIncompleteAssessment(preview: InstallPreview): ProjectAssessment {
  return {
    id: "assessment-unavailable",
    sourcePlanId: preview.id,
    repository: preview.repository,
    classification: "unknown",
    classificationEvidence: [],
    gate: "incomplete",
    summary: "The mandatory local assessment is unavailable. Start over before installing.",
    highestRisk: preview.scan.activeHighestSeverity,
    coverage: { filesInventoried: 0, filesScanned: preview.scan.filesScanned, evidenceLimited: true },
    checks: [{
      id: "assessment-unavailable", layer: "baseline", requirement: "required", status: "blocked",
      title: "Local assessment", summary: "The backend assessment result could not be loaded.",
      provider: "local", evidenceFiles: []
    }],
    targets: [],
    enhancedScanRecommended: false,
    sourceDigest: "",
    assessmentDigest: "",
    createdAt: preview.createdAt,
    expiresAt: preview.expiresAt
  };
}

function UnifiedSourceStep({ sourceMethod, source, requestedRef, busy, setSourceMethod, setSource, setRequestedRef }: {
  sourceMethod: SourceMethod;
  source: string;
  requestedRef: string;
  busy: boolean;
  setSourceMethod: (value: SourceMethod) => void;
  setSource: (value: string) => void;
  setRequestedRef: (value: string) => void;
}) {
  const { t } = useI18n();
  return <div className="install-source-step unified-source-step">
    <div className="source-intro"><ShieldCheck size={24} /><div>
      <span className="eyebrow">{t("统一安全流程", "Unified security workflow")}</span>
      <h3>{t("添加需要检查的项目", "Add a project to assess")}</h3>
      <p>{t("应用会先理解项目、执行本地必选检查，再向你展示安装目标和任何附加检查。",
        "The app first understands the project and runs required local checks, then shows install targets and any additional checks.")}</p>
    </div></div>
    <div className="install-source-tabs" aria-label={t("来源类型", "Source type")}>
      <button type="button" aria-pressed={sourceMethod === "github"}
        className={sourceMethod === "github" ? "active" : ""} disabled={busy}
        onClick={() => setSourceMethod("github")}><FolderGit2 size={17} />{t("GitHub 链接", "GitHub link")}</button>
      <button type="button" aria-pressed={sourceMethod === "local"}
        className={sourceMethod === "local" ? "active" : ""} disabled={busy}
        onClick={() => setSourceMethod("local")}><FileCode2 size={17} />{t("本地目录", "Local directory")}</button>
    </div>
    <label className="install-field"><span>{sourceMethod === "github"
      ? t("GitHub 仓库、目录或 SKILL.md 链接", "GitHub repository, directory, or SKILL.md link")
      : t("包含一个或多个 Skills 的绝对路径", "Absolute path containing one or more Skills")}</span>
      <input autoFocus value={source} disabled={busy} onChange={event => setSource(event.target.value)}
        placeholder={sourceMethod === "github" ? "https://github.com/owner/repository" : "D:\\skills\\my-package"} /></label>
    {sourceMethod === "github" && <label className="install-field"><span>{t("分支、标签或 Commit（可选）", "Branch, tag, or commit (optional)")}</span>
      <input value={requestedRef} disabled={busy} onChange={event => setRequestedRef(event.target.value)}
        placeholder={t("留空时使用链接版本或默认分支", "Leave blank to use the linked version or default branch")} /></label>}
    <div className="install-safety-note"><ShieldCheck size={21} /><div><strong>{t("本地检查优先", "Local checks first")}</strong>
      <p>{t("这一步只固定来源、创建受管快照并运行本地检查；不会调用 Codex、下载依赖或执行仓库脚本。",
        "This step only pins the source, creates a managed snapshot, and runs local checks. It does not call Codex, download dependencies, or execute repository scripts.")}</p></div></div>
  </div>;
}

function StandardReview({ preview, candidates, selected, setSelected, scan, riskBusy, onIgnore, completed }: {
  preview: InstallPreview | null;
  candidates: Candidate[];
  selected: string[];
  setSelected: (value: string[]) => void;
  scan?: ScanReport;
  riskBusy: boolean;
  onIgnore: (clusters: RiskCluster[]) => Promise<void>;
  completed: boolean;
}) {
  return <div className="install-review">
    <RepositorySummary preview={preview} />
    <SkillSelection candidates={candidates} selected={selected} setSelected={setSelected} disabled={completed} />
    {scan && <CompactRiskReview report={scan} busy={riskBusy} onIgnore={onIgnore} />}
  </div>;
}

function ProjectScanView({ scan }: { scan: CodexProjectScanResult }) {
  const { t, locale, formatDate } = useI18n();
  const security = scan.security;
  return <div className="assisted-plan">
    <div className="assisted-overview">
      <div className="assisted-overview-head">
        <div>
          <span className="eyebrow">{t("Codex 项目扫描", "Codex project scan")}</span>
          <h3>{scan.repository?.fullName || t("本地 Skill 项目", "Local Skill project")}</h3>
        </div>
        <span className={`complexity ${security.localHighestRisk}`}>
          {severityLabel(security.localHighestRisk, locale)}
        </span>
      </div>
      <p>{scan.summary || t("Codex 未提供项目概述。", "Codex did not provide a project summary.")}</p>
      <div className="approach">
        <ShieldCheck size={18} />
        <span>{security.summary || t("未提供安全结论。", "No security conclusion was provided.")}</span>
      </div>
      <div className="assisted-meta">
        <span><b>{t("安全结论", "Security verdict")}</b>{security.verdict}</span>
        <span><b>{t("置信度", "Confidence")}</b>{Math.round(security.confidence * 100)}%</span>
        <span><b>{t("本地发现", "Local findings")}</b>{security.localFindingCount}</span>
        <span><b>{t("上下文", "Context")}</b>{scan.contextFileCount} {t("个文件", "files")}</span>
        {scan.expiresAt && <span><b>{t("扫描有效期", "Scan expires")}</b>{formatDate(scan.expiresAt)}</span>}
      </div>
    </div>

    <PlanSection title={t("安全注意事项", "Security notes")} icon={<AlertTriangle size={19} />}
      subtitle={t("敏感内容仅保留元数据；结论区分合理功能与实际危害。", "Sensitive content is metadata-only; conclusions distinguish legitimate capability from harmful behavior.")}>
      {security.concerns.length ? <div className="plan-step-list">
        {security.concerns.map((concern, index) => <div className="plan-step" key={`${concern.title}-${index}`}>
          <span className={`risk-dot ${concern.severity}`} />
          <div>
            <strong>{concern.title}</strong>
            <p>{concern.rationale}</p>
            {concern.recommendation && <small>{concern.recommendation}</small>}
          </div>
        </div>)}
      </div> : <EmptyPlanText text={t("Codex 未提出额外安全注意事项。", "Codex raised no additional security notes.")} />}
    </PlanSection>

    <PlanSection title={t("识别到的安装方式", "Detected installation methods")} icon={<Download size={19} />}
      subtitle={t("这里只描述可能的方式，不执行命令、下载或安装。", "These are declarative options only; no command, download, or installation is performed.")}>
      <div className="plan-step-list">{scan.installationMethods.length ? scan.installationMethods.map((method, index) =>
        <div className="plan-step" key={`${method.kind}-${index}`}>
          {method.supported ? <CheckCircle2 size={18} /> : <CircleDashed size={18} />}
          <div>
            <strong>{method.title}</strong>
            <p>{method.description}</p>
            {!!method.evidenceFiles.length && <small>{method.evidenceFiles.join(", ")}</small>}
          </div>
        </div>) : <EmptyPlanText text={t("未识别到可声明的安装方式。", "No declarative installation method was identified.")} />}</div>
    </PlanSection>

    <div className="plan-warnings" role="status">
      <ShieldCheck size={20} /><div>
        <strong>{t("尚未授权安装", "Installation is not authorized")}</strong>
        <p>{t(
          `已摘要 ${scan.summaryFileCount} 个文件、深度分析 ${scan.deepAnalysisFileCount} 个重点文件；${scan.redactedFileCount} 个敏感文件已脱敏，${scan.truncatedFileCount} 个大文件已截断。只有点击“同意并生成安装计划”后才会进入下一阶段。`,
          `${scan.summaryFileCount} files were summarized and ${scan.deepAnalysisFileCount} focus files were analyzed; ${scan.redactedFileCount} sensitive files were redacted and ${scan.truncatedFileCount} large files were truncated. The next stage starts only after you approve creating a plan.`
        )}</p>
      </div>
    </div>
  </div>;
}

function AssistedPlanView({ plan, candidates, selectedSkills, setSelectedSkills, selectedPermissions, setSelectedPermissions,
  permissionDependencyIssue, projectRoot, projectRootRequired, setProjectRoot, scan, riskBusy, onIgnore }: {
  plan: AssistedInstallPlan;
  candidates: Candidate[];
  selectedSkills: string[];
  setSelectedSkills: (value: string[]) => void;
  selectedPermissions: string[];
  setSelectedPermissions: (value: string[]) => void;
  permissionDependencyIssue: string;
  projectRoot: string;
  projectRootRequired: boolean;
  setProjectRoot: (value: string) => void;
  scan?: ScanReport;
  riskBusy: boolean;
  onIgnore: (clusters: RiskCluster[]) => Promise<void>;
}) {
  const { t, formatDate } = useI18n();
  return <div className="assisted-plan">
    <div className="assisted-overview">
      <div className="assisted-overview-head">
        <div><span className="eyebrow">{t("Codex 分析概览", "Codex analysis overview")}</span>
          <h3>{plan.repository?.fullName || t("本地 Skill 包", "Local Skill package")}</h3></div>
        <span className={`complexity ${plan.complexity || "unknown"}`}>{complexityLabel(plan.complexity, t)}</span>
      </div>
      <p>{plan.summary || t("Codex 未提供概述。", "Codex did not provide a summary.")}</p>
      {plan.approach && <div className="approach"><Sparkles size={18} /><span>{plan.approach}</span></div>}
      <div className="assisted-meta">
        <span><b>{t("模型", "Model")}</b>{plan.codexModel || "Codex"}</span>
        <span><b>{t("推理强度", "Reasoning")}</b>{plan.reasoningEffort || t("默认", "Default")}</span>
        <span><b>{t("上下文", "Context")}</b>{t(`${plan.contextFileCount} 个文件`, `${plan.contextFileCount} files`)}</span>
        {plan.expiresAt && <span><b>{t("计划有效期", "Plan expires")}</b>{formatDate(plan.expiresAt)}</span>}
      </div>
    </div>

    {plan.status === "manual-required" && <div className="plan-warnings" role="status">
      <AlertTriangle size={20} /><div>
        <strong>{t("部分步骤需要手动完成", "Some steps require manual work")}</strong>
        <p>{t(
          "应用可以先执行并记录受支持的步骤；其余步骤不会自动运行，完成后会明确标记为待手动处理。",
          "The app can run and journal the supported steps first. Unsupported work is never run automatically and remains clearly marked for manual completion."
        )}</p>
      </div>
    </div>}

    {!!candidates.length && <SkillSelection candidates={candidates} selected={selectedSkills}
      setSelected={setSelectedSkills} />}

    <PlanSection title={t("环境要求", "Requirements")} icon={<TerminalSquare size={19} />}
      subtitle={t("执行前需要满足的工具和环境", "Tools and environment needed before execution")}>
      <div className="requirement-list">{plan.requirements.length ? plan.requirements.map((requirement, index) =>
        <RequirementRow key={requirementKey(requirement, index)} requirement={requirement} />) :
        <EmptyPlanText text={t("没有额外环境要求", "No additional environment requirements")} />}</div>
    </PlanSection>

    <PlanSection title={t("安装步骤", "Installation steps")} icon={<ChevronRight size={19} />}
      subtitle={t("只会自动执行受支持的结构化步骤", "Only supported structured steps are executed automatically")}>
      <div className="plan-step-list">{plan.steps.map((step, index) =>
        <PlanStepRow key={step.id || index} step={step} index={index} />)}</div>
    </PlanSection>

    <PlanSection title={t("权限确认", "Permission approval")} icon={<KeyRound size={19} />}
      subtitle={t("集中选择本计划可以使用的权限；必要权限必须逐项勾选", "Select permissions for this plan. Every required permission must be checked")}>
      <PermissionList permissions={plan.permissions} selected={selectedPermissions} setSelected={setSelectedPermissions} />
      {permissionDependencyIssue && <div className="permission-dependency-warning" role="status">
        <CircleAlert size={16} /><span>{permissionDependencyIssue}</span>
      </div>}
    </PlanSection>

    {projectRootRequired && <label className="install-field project-root-field">
      <span>{t("项目目录", "Project directory")}<em>{t("必填", "Required")}</em></span>
      <input value={projectRoot} onChange={event => setProjectRoot(event.target.value)}
        placeholder="D:\\projects\\my-project" />
      <p>{plan.projectRootReason || t("此配置需要明确的项目范围。", "This configuration needs an explicit project scope.")}</p>
    </label>}

    {!!plan.warnings.length && <div className="plan-warnings"><AlertTriangle size={20} /><div>
      <strong>{t("执行前请注意", "Before execution")}</strong>
      <ul>{plan.warnings.map((warning, index) => <li key={`${warning}-${index}`}>{warning}</li>)}</ul>
    </div></div>}

    {scan && <CompactRiskReview report={scan} busy={riskBusy} onIgnore={onIgnore} />}
  </div>;
}

function ExecutionView({ plan, progress, result, rolledBack }: {
  plan: AssistedInstallPlan;
  progress: AssistedInstallProgress | null;
  result: AssistedInstallResult | null;
  rolledBack: boolean;
}) {
  const { t, formatDate } = useI18n();
  const steps = progress?.steps?.length ? progress.steps : plan.steps.map(step => ({
    id: step.id,
    title: step.title,
    kind: step.kind,
    status: step.status,
    error: step.error
  }));
  const completed = progress?.completedSteps ?? steps.filter(step => step.status === "completed").length;
  const total = progress?.totalSteps || steps.length;
  const terminal = progress?.terminal ?? !!result;
  const partial = progress?.phase === "partial";
  const failed = progress?.phase === "failed" ||
    progress?.phase === "cancelled" || progress?.phase === "interrupted" || !!progress?.error;
  return <div className="execution-view">
    <div className={`execution-summary ${terminal ? rolledBack ? "rolled-back" : failed ? "failed" : partial ? "partial" : "completed" : "running"}`}
      role="status" aria-live="polite" aria-atomic="true">
      {terminal ? rolledBack ? <RotateCcw size={26} /> : failed ? <CircleAlert size={26} /> :
        partial ? <AlertTriangle size={26} /> : <CheckCircle2 size={26} /> : <LoaderCircle className="spin" size={26} />}
      <div><span className="eyebrow">{terminal ? t("执行结果", "Execution result") : t("执行进度", "Execution progress")}</span>
        <h3>{progress?.message || (terminal ? t("计划执行完毕", "Plan execution finished") : t("正在开始…", "Starting…"))}</h3>
        <p>{t(`已处理 ${completed}/${total} 个步骤`, `${completed}/${total} steps processed`)}
          {progress?.activityCount ? t(` · ${progress.activityCount} 条活动记录`, ` · ${progress.activityCount} activity records`) : ""}</p>
      </div>
      <strong>{total ? Math.min(100, Math.round(completed / total * 100)) : 0}%</strong>
    </div>
    <div className="execution-progress-track" role="progressbar"
      aria-label={t("安装完成进度", "Installation completion progress")}
      aria-valuemin={0} aria-valuemax={total} aria-valuenow={completed}>
      <i aria-hidden="true" style={{ width: `${total ? Math.min(100, completed / total * 100) : 0}%` }} />
    </div>
    <ol className="execution-timeline" aria-label={t("安装步骤状态", "Installation step status")}>
      {steps.map((step, index) => <TimelineStep key={step.id || index} step={step} index={index} />)}
    </ol>
    {progress?.error && <div className="execution-error"><CircleAlert size={18} /><span>{progress.error}</span></div>}
    {terminal && <div className="result-grid">
      <div><strong>{t("Skills", "Skills")}</strong>
        <p>{result?.transaction?.targets?.length
          ? result.transaction.targets.join(localeJoiner())
          : unique(plan.steps.flatMap(step => step.skillNames ?? [])).join(localeJoiner()) || t("请在 Skills 页面确认", "Confirm on the Skills page")}</p></div>
      <div><strong>{t("事务记录", "Transaction")}</strong>
        <p>{result?.transaction?.id || t("可在历史与回滚中查看", "Available in History & Rollback")}</p></div>
      <div><strong>{t("恢复状态", "Recovery")}</strong>
        <p>{rolledBack ? t("已回滚可逆修改", "Reversible changes rolled back") :
          failed ? t("可重试或按步骤建议恢复", "Retry or follow step recovery guidance") :
            partial ? t("自动步骤已完成；人工步骤仍待处理", "Automatic steps completed; manual work remains") :
            t("可从历史记录回滚", "Rollback is available from history")}</p></div>
    </div>}
    {(progress?.startedAt || progress?.updatedAt) && <div className="execution-time">
      {progress.startedAt && <span>{t("开始：", "Started: ")}{formatDate(progress.startedAt)}</span>}
      {progress.updatedAt && <span>{t("最近活动：", "Latest activity: ")}{formatDate(progress.updatedAt)}</span>}
    </div>}
  </div>;
}

function AnalysisProgressView({ progress }: { progress: AssistedInstallProgress }) {
  const { t } = useI18n();
  const total = progress.totalSteps || progress.steps.length || 3;
  const completed = Math.min(total, progress.completedSteps || 0);
  const isProjectScan = progress.steps.length > 0 &&
    !progress.steps.some(step => step.id === "dependency-lock");
  const steps = progress.steps.map(step => ({
    ...step,
    title: step.id === "inventory" ? t("仓库盘点", "Repository inventory") :
      step.id === "codex-analysis" ? t("Codex 分析", "Codex analysis") :
        step.id === "validation" ? t("本地验证", "Local validation") :
          step.id === "dependency-lock" ? t("依赖锁定", "Dependency lock") :
            step.id === "finalizing" && isProjectScan ? t("保存项目扫描", "Persist project scan") :
              step.id === "finalizing" ? t("保存计划", "Finalize plan") : step.title
  }));
  return <div className="analysis-progress-view">
    <div className="analysis-progress-head" role="status" aria-live="polite" aria-atomic="true">
      <LoaderCircle className="spin" size={25} />
      <div><span className="eyebrow">{isProjectScan
        ? t("项目扫描进度", "Project scan progress")
        : t("安装分析进度", "Installation analysis progress")}</span>
        <h3>{progress.message || (isProjectScan
          ? t("Codex 正在扫描项目", "Codex is scanning the project")
          : t("Codex 正在生成安装计划", "Codex is generating the installation plan"))}</h3>
        <p>{t(`已完成 ${completed}/${total} 个阶段`, `${completed}/${total} stages completed`)}
          {progress.activityCount ? t(` · ${progress.activityCount} 条活动更新`, ` · ${progress.activityCount} activity updates`) : ""}</p>
      </div>
      <strong>{total ? Math.min(100, Math.round(completed / total * 100)) : 0}%</strong>
    </div>
    <div className="execution-progress-track" role="progressbar"
      aria-label={t("安装分析完成进度", "Installation analysis progress")}
      aria-valuemin={0} aria-valuemax={total} aria-valuenow={completed}>
      <i aria-hidden="true" style={{ width: `${total ? Math.min(100, completed / total * 100) : 0}%` }} />
    </div>
    <ol className="execution-timeline analysis-timeline" aria-label={t("安装分析阶段", "Installation analysis stages")}>
      {steps.map((step, index) => <TimelineStep key={step.id || index} step={step} index={index} />)}
    </ol>
  </div>;
}

function RepositorySummary({ preview }: { preview: InstallPreview | null }) {
  const { t, formatDate } = useI18n();
  if (!preview) return null;
  return <div className="install-repository">
    <FolderGit2 size={24} />
    <div><strong>{preview.repository.fullName || t("本地 Skill 包", "Local Skill package")}</strong>
      <span>{preview.repository.resolvedRef || t("本地来源", "Local source")}
        {preview.repository.commitSha ? ` · ${preview.repository.commitSha.slice(0, 12)}` : ""}</span></div>
    {preview.expiresAt && <span>{t("有效至", "Expires")} {formatDate(preview.expiresAt)}</span>}
  </div>;
}

function SkillSelection({ candidates, selected, setSelected, disabled = false }: {
  candidates: Candidate[];
  selected: string[];
  setSelected: (value: string[]) => void;
  disabled?: boolean;
}) {
  const { t } = useI18n();
  const invert = () => setSelected(candidates.filter(skill => !selected.includes(skill.name)).map(skill => skill.name));
  return <section className="skill-selection">
    <div className="section-heading"><div><h3>{t("选择 Skills", "Choose Skills")}</h3>
      <p>{t(`发现 ${candidates.length} 个，已选择 ${selected.length} 个`, `${candidates.length} found, ${selected.length} selected`)}</p></div>
      <div className="selection-tools">
        <button type="button" disabled={disabled} onClick={() => setSelected(candidates.map(skill => skill.name))}>{t("全选", "All")}</button>
        <button type="button" disabled={disabled} onClick={invert}>{t("反选", "Invert")}</button>
        <button type="button" disabled={disabled || !selected.length} onClick={() => setSelected([])}>{t("清空", "Clear")}</button>
      </div>
    </div>
    <div className="install-skill-list">{candidates.map(skill => <label key={skill.name}>
      <input type="checkbox" disabled={disabled} checked={selected.includes(skill.name)}
        onChange={() => setSelected(selected.includes(skill.name)
          ? selected.filter(name => name !== skill.name)
          : [...selected, skill.name])} />
      <span><strong>{skill.name}</strong><span>{skill.description || t("暂无说明", "No description")}</span>
        {skill.sourcePath && <code>{skill.sourcePath}</code>}</span>
    </label>)}</div>
  </section>;
}

function CompactRiskReview({ report, busy, onIgnore }: {
  report: ScanReport;
  busy: boolean;
  onIgnore: (clusters: RiskCluster[]) => Promise<void>;
}) {
  const { t, locale } = useI18n();
  const active = [...(report.clusters ?? [])].filter(cluster => !cluster.ignored)
    .sort((a, b) => severityRank(b.severity) - severityRank(a.severity));
  const ignored = (report.clusters ?? []).filter(cluster => cluster.ignored);
	const batchDismissible = active.filter(cluster => cluster.severity !== "high" && cluster.severity !== "critical");
  const grouped = severityOrder.map(severity => ({
    severity,
    clusters: active.filter(cluster => cluster.severity === severity)
  })).filter(group => group.clusters.length);
  return <section className={`compact-risk ${active.length ? "has-warnings" : "clean"}`}>
    <div className="compact-risk-head">
      {active.length ? <CircleAlert size={21} /> : <ShieldCheck size={21} />}
      <div><h3>{active.length ? t(`${active.length} 个警告组待处理`, `${active.length} warning groups need review`) :
        t("没有待处理警告", "No open warnings")}</h3>
        <p>{t(`本地规则扫描了 ${report.filesScanned} 个文件；${ignored.length} 个警告组已忽略。`,
          `Local rules scanned ${report.filesScanned} files; ${ignored.length} warning groups are ignored.`)}</p></div>
      {batchDismissible.length > 0 && <button type="button" disabled={busy} onClick={() => void onIgnore(batchDismissible)}>
        {busy ? <LoaderCircle className="spin" size={16} /> : <Check size={16} />}
        {t(`忽略可处理项（${batchDismissible.length}）`, `Ignore eligible (${batchDismissible.length})`)}
      </button>}
    </div>
    {grouped.length > 0 && <details className="risk-details">
      <summary>{t("查看警告详情", "View warning details")}</summary>
      <div>{grouped.map(group => <section key={group.severity}>
        <h4><span className={`risk-dot ${group.severity}`} />{severityLabel(group.severity, locale)}
          <em>{group.clusters.length}</em></h4>
        {group.clusters.map(cluster => <article key={cluster.id}>
          <div><strong>{cluster.title}</strong><span>{cluster.ruleId} · {cluster.affectedFiles.length} {t("个文件", "files")}</span></div>
			{cluster.severity === "critical"
				? <span className="risk-policy-label critical">{t("不可忽略", "Cannot ignore")}</span>
				: <button type="button" disabled={busy} onClick={() => void onIgnore([cluster])}>
					{cluster.severity === "high" ? t("审阅并接受", "Review and accept") : t("忽略", "Ignore")}
				</button>}
        </article>)}
      </section>)}</div>
    </details>}
  </section>;
}

function PlanSection({ title, subtitle, icon, children }: {
  title: string;
  subtitle: string;
  icon: React.ReactNode;
  children: React.ReactNode;
}) {
  return <section className="plan-section">
    <div className="plan-section-head">{icon}<div><h3>{title}</h3><p>{subtitle}</p></div></div>
    {children}
  </section>;
}

function RequirementRow({ requirement }: { requirement: string | AssistedInstallRequirement }) {
  const { t } = useI18n();
  if (typeof requirement === "string") {
    return <div className="requirement-row"><CircleDashed size={17} /><span><strong>{requirement}</strong></span></div>;
  }
  const satisfied = requirement.satisfied || ["ready", "satisfied", "available", "completed"].includes(requirement.status?.toLowerCase() ?? "");
  return <div className={`requirement-row ${satisfied ? "satisfied" : requirement.required ? "required" : ""}`}>
    {satisfied ? <CheckCircle2 size={17} /> : <CircleDashed size={17} />}
    <span><strong>{requirement.title || requirement.name || t("未命名要求", "Unnamed requirement")}</strong>
      {requirement.description && <p>{requirement.description}</p>}
      {requirement.versionSpec && <p>{t("版本", "Version")}: {requirement.versionSpec}</p>}</span>
    <em>{satisfied ? t("已满足", "Ready") : requirement.required ? t("必要", "Required") : t("可选", "Optional")}</em>
  </div>;
}

function PlanStepRow({ step, index }: { step: AssistedInstallStep; index: number }) {
  const { t } = useI18n();
  const nativeWheelCount = step.pythonWheels?.filter(wheel => wheel.native).length ?? 0;
  return <article className={`plan-step ${step.supported ? "" : "manual"}`}>
    <span className="step-index">{index + 1}</span>
    <div><div className="plan-step-title"><strong>{step.title}</strong>
      <span>{step.supported ? step.required ? t("自动 · 必要", "Automatic · Required") : t("自动 · 可选", "Automatic · Optional") :
        t("需要人工完成", "Manual step")}</span></div>
      <p>{step.description}</p>
      <div className="plan-step-meta">
        {!!step.skillNames?.length && <span>{t("Skills", "Skills")}: {step.skillNames.join(", ")}</span>}
        {step.pythonPackage && <span>{t("依赖", "Dependency")}: {step.pythonPackage}{step.versionSpec || ""}</span>}
        {!!step.pythonWheels?.length && <span>{t(
          `已锁定 ${step.pythonWheels.length} 个 Wheel`,
          `${step.pythonWheels.length} Wheels locked`
        )}{nativeWheelCount ? t(` · ${nativeWheelCount} 个包含本机代码`, ` · ${nativeWheelCount} contain native code`) : ""}</span>}
        {step.mcpServerName && <span>MCP: {step.mcpServerName}</span>}
        {step.entrypoint && <span>{t("入口", "Entrypoint")}: <code>{step.entrypoint}</code></span>}
        {step.targetPath && <span>{t("目标", "Target")}: {step.targetPath}</span>}
        <span>{step.reversible ? t("可回滚", "Reversible") : t("不可自动回滚", "No automatic rollback")}</span>
      </div>
      {(step.pythonWheels?.length || step.mcpArgs?.length || step.recovery || step.error) && <details>
        <summary>{t("查看技术详情与恢复方式", "Technical details and recovery")}</summary>
        {!!step.pythonWheels?.length && <div className="wheel-lock-list">
          <strong>{t("批准时锁定的完整依赖", "Complete dependency lock at approval")}</strong>
          {step.pythonWheels.map(wheel => <div key={`${wheel.filename}-${wheel.sha256}`}>
            <span><b>{wheel.name}=={wheel.version}</b>
              {wheel.native && <em>{t("本机代码 · 高风险", "Native code · High risk")}</em>}</span>
            <code>{wheel.filename}</code>
            <small>SHA-256 {wheel.sha256}</small>
          </div>)}
        </div>}
        {!!step.mcpArgs?.length && <code>{step.mcpArgs.join(" ")}</code>}
        {step.recovery && <p>{step.recovery}</p>}
        {step.error && <p className="step-error">{step.error}</p>}
      </details>}
    </div>
  </article>;
}

function PermissionList({ permissions, selected, setSelected }: {
  permissions: AssistedInstallPermission[];
  selected: string[];
  setSelected: (value: string[]) => void;
}) {
  const { t } = useI18n();
  if (!permissions.length) return <EmptyPlanText text={t("该计划不需要额外权限", "This plan needs no additional permissions")} />;
  return <div className="permission-list">
    <div className="permission-tools">
      <button type="button" onClick={() => setSelected(permissions.map(permission => permission.id))}>{t("全部勾选", "Select all")}</button>
      <button type="button" onClick={() => setSelected([])} disabled={!selected.length}>{t("清空", "Clear")}</button>
    </div>
    {permissions.map(permission => {
      const risk = permissionRisk(permission);
      const content = localizedPermission(permission, t);
      return <label key={permission.id}>
      <input type="checkbox" checked={selected.includes(permission.id)}
        onChange={() => setSelected(selected.includes(permission.id)
          ? selected.filter(id => id !== permission.id)
          : [...selected, permission.id])} />
      <span><strong>{content.title}<em>{permission.required ? t("必要", "Required") : t("可选", "Optional")}</em></strong>
        {content.description && <p>{content.description}</p>}
        {(permission.target || permission.targets?.length) &&
          <code>{permission.target || permission.targets?.join(", ")}</code>}
      </span>
      <span className={`permission-risk ${risk}`}>{permissionRiskLabel(risk, t)}</span>
    </label>;
    })}
  </div>;
}

function localizedPermission(
  permission: AssistedInstallPermission,
  t: Translate
): { title: string; description: string } {
  switch (permission.id) {
    case "skills-write":
      return {
        title: t("安装 Skills", "Install Skills"),
        description: t(
          "把计划中明确列出的 Skills 写入目标目录，并创建事务备份。",
          "Write only the explicitly listed Skills with a transactional backup."
        )
      };
    case "pypi-wheel-lock":
      return {
        title: t("使用已锁定的 PyPI Wheel", "Use locked PyPI Wheels"),
        description: t(
          "完整依赖已在分析阶段从官方 PyPI 暂存；执行时只离线接受计划中的名称、版本、文件名和 SHA256。",
          "The complete dependency set was staged from official PyPI during analysis; execution is offline and accepts only the listed names, versions, filenames, and SHA256 values."
        )
      };
    case "managed-tool-write":
      return {
        title: t("创建受管工具", "Create managed tool"),
        description: t(
          "在应用数据目录中创建固定版本的 Python 环境。",
          "Create a versioned Python environment under the application data directory."
        )
      };
    case "managed-tool-run":
      return {
        title: t("运行受管工具", "Run managed tool"),
        description: t(
          "运行已批准的软件包入口，用于验证和 MCP 服务。",
          "Run the approved package entrypoint for verification and MCP service use."
        )
      };
    case "managed-native-code":
      return {
        title: t("运行本机代码（高风险）", "Run native code (high risk)"),
        description: t(
          "允许计划中列出的平台专用 Wheel；文件名、兼容标签和 SHA256 均已固定。",
          "Allow the listed platform-specific Wheels; every filename, compatibility tag, and SHA256 is fixed in the approved plan."
        )
      };
    case "codex-mcp-config":
      return {
        title: t("配置 Codex MCP", "Configure Codex MCP"),
        description: t(
          "备份 Codex 配置，并添加由本应用管理的 MCP 条目。",
          "Back up the Codex configuration and add the approved manager-owned MCP entry."
        )
      };
    default:
      return {
        title: permission.title || permission.id,
        description: permission.description || ""
      };
  }
}

function TimelineStep({ step, index }: { step: AssistedInstallProgressStep; index: number }) {
  const { t, formatDate } = useI18n();
  return <li className={`timeline-step ${step.status}`}>
    <span className="timeline-icon">{step.status === "completed" ? <Check size={16} /> :
      step.status === "rolled-back" ? <RotateCcw size={15} /> :
      step.status === "running" ? <LoaderCircle className="spin" size={16} /> :
        step.status === "failed" || step.status === "interrupted" ? <OctagonX size={16} /> :
          step.status === "manual-pending" ? <AlertTriangle size={15} /> :
          step.status === "cancelled" || step.status === "skipped" ? <Square size={13} /> : <Clock3 size={16} />}</span>
    <div><strong>{step.title || t(`步骤 ${index + 1}`, `Step ${index + 1}`)}</strong>
      <span>{step.message || stepStatusLabel(step.status, t)}</span>
      {step.error && <p>{step.error}</p>}
      {(step.startedAt || step.completedAt) && <small>
        {step.completedAt ? formatDate(step.completedAt) : step.startedAt ? formatDate(step.startedAt) : ""}
      </small>}</div>
  </li>;
}

function TaskFeedback({
  feedback,
  retryTask,
  retryEnabled,
  onRetry,
  retryWaitMs,
  onOpenSettings,
  onUseSuggestedSource
}: {
  feedback: Feedback;
  retryTask: RetryTask;
  retryEnabled: boolean;
  onRetry: () => void;
  retryWaitMs: number;
  onOpenSettings?: () => void;
  onUseSuggestedSource?: (url: string) => void;
}) {
  const { t } = useI18n();
  return <div className={`install-feedback ${feedback.tone}`}
    role={feedback.tone === "error" ? "alert" : "status"} aria-live={feedback.tone === "error" ? "assertive" : "polite"}>
    {feedback.tone === "success" ? <CheckCircle2 size={20} /> :
      feedback.tone === "error" ? <CircleAlert size={20} /> :
        feedback.tone === "warning" ? <AlertTriangle size={20} /> : <LoaderCircle className={feedback.title.includes("正在") ? "spin" : ""} size={20} />}
    <div><strong>{feedback.title}</strong><p>{feedback.message}</p>
      {feedback.detail && <details><summary>{t("技术详情", "Technical details")}</summary><pre>{feedback.detail}</pre></details>}
      {((retryTask && (retryEnabled || retryWaitMs > 0)) ||
        (feedback.settingsSuggested && onOpenSettings) ||
        (feedback.suggestedSourceUrl && onUseSuggestedSource)) && <div className="feedback-actions">
        {retryTask && (retryEnabled || retryWaitMs > 0) && <button type="button" onClick={onRetry} disabled={!retryEnabled}>
          <RefreshCw size={15} />{retryWaitMs > 0
            ? t(`${formatRetryWait(retryWaitMs)} 后可重试`, `Retry in ${formatRetryWait(retryWaitMs)}`)
            : t("重试", "Retry")}</button>}
        {feedback.settingsSuggested && onOpenSettings && <button type="button" onClick={onOpenSettings}>
          <Settings size={15} />{feedback.settingsKind === "github"
            ? t("检查 GitHub 凭据", "Check GitHub credentials")
            : t("打开 Codex 设置", "Open Codex settings")}</button>}
        {feedback.suggestedSourceUrl && onUseSuggestedSource &&
          <button type="button" onClick={() => onUseSuggestedSource(feedback.suggestedSourceUrl!)}>
            <FolderGit2 size={15} />{t("使用建议的 Codex 目录", "Use suggested Codex directory")}
          </button>}
      </div>}
    </div>
  </div>;
}

function CenteredState({ icon, title, detail }: { icon: React.ReactNode; title: string; detail: string }) {
  return <div className="install-centered-state">{icon}<h3>{title}</h3><p>{detail}</p></div>;
}

function EmptyPlanText({ text }: { text: string }) {
  return <div className="empty-plan-text"><CheckCircle2 size={17} />{text}</div>;
}

function issueFrom(error: unknown, t: Translate, fallback?: string): Feedback {
  const {
    code,
    rawMessage,
    rawDetail,
    retryAt: retryHint,
    rateLimited,
    githubForbidden,
    codexUnavailable: unavailable,
    restartRequired,
    invalidInput,
    skillVariantConflict,
    suggestedSourceUrl
  } = classifyInstallIssue(error);
  let title = t("操作失败", "Operation failed");
  let message = fallback || rawMessage || t("未收到可用的错误信息。", "No usable error information was returned.");
  if (skillVariantConflict) {
    title = t("发现同名 Skill 变体", "Multiple Skill variants found");
    message = suggestedSourceUrl
      ? t(
        "仓库同时提供了内容不同的同名 Skill。请选择具体版本；可直接使用应用识别出的 Codex 目录。",
        "The repository contains different Skills with the same name. Choose a specific variant or use the detected Codex directory."
      )
      : t(
        "仓库同时提供了内容不同的同名 Skill。请根据技术详情，将来源改为具体的仓库子目录。",
        "The repository contains different Skills with the same name. Use the paths in Technical details to select a repository subtree."
      );
  } else if (rateLimited) {
    title = t("GitHub 访问受限", "GitHub access is limited");
    message = retryHint
      ? t(`GitHub 请求额度暂时不可用，可在 ${retryHint} 后重试。输入内容和当前进度已保留。`,
        `GitHub requests are temporarily unavailable. Retry after ${retryHint}. Your input and progress were preserved.`)
      : t("GitHub 拒绝了当前请求，常见原因是未验证凭据或请求额度已用尽。输入内容和当前进度已保留。",
        "GitHub rejected the request, usually because credentials are not verified or the rate limit is exhausted. Your input and progress were preserved.");
  } else if (unavailable) {
    title = t("Codex 一键安装暂不可用", "Codex assisted installation is unavailable");
  } else if (githubForbidden) {
    title = t("GitHub 无法访问该来源", "GitHub source access was denied");
    message = t(
      "请检查仓库权限和 GitHub 凭据。输入内容和当前进度已保留。",
      "Check repository access and GitHub credentials. Your input and current progress were preserved."
    );
  } else if (restartRequired) {
    title = t("需要重新分析来源", "Source analysis must be repeated");
    message = t(
      "来源、计划或配置已发生变化。为避免使用过期授权，请返回并重新生成安装计划。",
      "The source, plan, or configuration changed. Go back and create a new installation plan instead of reusing stale approval."
    );
  }
  const serialized = safeSerialize(error);
  return {
    tone: "error",
    title,
    message,
    detail: [code ? `code: ${code}` : "", rawDetail, serialized !== rawMessage ? serialized : rawMessage]
      .filter(Boolean).join("\n"),
    rateLimited,
    retryAt: retryHint,
    retryable: !(restartRequired || invalidInput || skillVariantConflict),
    settingsSuggested: rateLimited || githubForbidden || unavailable,
    settingsKind: rateLimited || githubForbidden ? "github" : unavailable ? "codex" : undefined,
    restartRequired,
    suggestedSourceUrl
  };
}

function useRetryCountdown(retryAt?: string): number {
  const retryTimestamp = parseRetryTimestamp(retryAt);
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    setNow(Date.now());
    if (!retryTimestamp || retryTimestamp <= Date.now()) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [retryTimestamp]);
  return retryWaitMilliseconds(retryAt, now);
}

function formatRetryWait(milliseconds: number): string {
  const seconds = Math.max(0, Math.ceil(milliseconds / 1000));
  const minutes = Math.floor(seconds / 60);
  const remaining = seconds % 60;
  return minutes > 0 ? `${minutes}:${remaining.toString().padStart(2, "0")}` : `${remaining}s`;
}

function safeSerialize(value: unknown): string {
  if (value instanceof Error) return value.stack || value.message;
  if (typeof value === "string") return value;
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

function planCandidates(plan: AssistedInstallPlan | null): Candidate[] {
  if (!plan) return [];
  if (plan.skills?.length) return plan.skills;
  return unique(plan.steps.flatMap(step => step.skillNames ?? [])).map(name => ({
    name,
    description: "",
    sourcePath: ""
  }));
}

function hasScheduledAssistedStep(
  steps: AssistedInstallStep[],
  kind: string,
  selectedPermissions: string[]
): boolean {
  const approved = new Set(selectedPermissions);
  return steps.some(step =>
    step.kind === kind &&
    step.supported &&
    (step.permissionIds ?? []).every(id => approved.has(id))
  );
}

function assistedPermissionDependencyIssue(
  steps: AssistedInstallStep[],
  selectedPermissions: string[],
  t: Translate
): string {
  const approved = new Set(selectedPermissions);
  const scheduledTools = new Set<string>();
  for (const step of steps) {
    if (!step.supported || !(step.permissionIds ?? []).every(id => approved.has(id))) continue;
    if (step.kind === "managed-python-tool") {
      scheduledTools.add((step.entrypoint ?? "").toLowerCase());
    } else if (step.kind === "configure-codex-mcp" &&
      !scheduledTools.has((step.entrypoint ?? "").toLowerCase())) {
      return t(
        `“${step.title}”还需要勾选其前置受管工具的全部权限。`,
        `“${step.title}” also requires every permission for its preceding managed tool step.`
      );
    }
  }
  return "";
}

function initialProgress(plan: AssistedInstallPlan): AssistedInstallProgress {
  const now = new Date().toISOString();
  return {
    referenceId: plan.id,
    runId: "",
    sequence: 0,
    phase: "starting",
    message: "",
    completedSteps: 0,
    totalSteps: plan.steps.length,
    activityCount: 0,
    steps: plan.steps.map(step => ({
      id: step.id,
      title: step.title,
      kind: step.kind,
      status: step.supported ? "queued" : step.required ? "manual-pending" : "skipped",
      message: step.supported ? "" : step.description
    })),
    startedAt: now,
    updatedAt: now,
    terminal: false
  };
}

function completedProgress(
  plan: AssistedInstallPlan,
  current: AssistedInstallProgress | null,
  t: Translate
): AssistedInstallProgress {
  const now = new Date().toISOString();
  const { partial } = assistedPlanDisposition(plan);
  return {
    referenceId: current?.referenceId || plan.id,
    runId: current?.runId || "",
    sequence: (current?.sequence ?? 0) + 1,
    phase: partial ? "partial" : "completed",
    message: partial
      ? t("自动步骤已完成，人工步骤仍待处理", "Automatic steps completed; manual work remains")
      : t("一键安装已完成", "Assisted installation completed"),
    completedSteps: plan.steps.filter(step =>
      ["completed", "skipped", "manual-pending", "rolled-back"].includes(step.status) || step.supported
    ).length,
    totalSteps: plan.steps.length,
    activityCount: current?.activityCount ?? plan.steps.length,
    steps: plan.steps.map(step => ({
      id: step.id,
      title: step.title,
      kind: step.kind,
      status: step.supported ? "completed" : step.required ? "manual-pending" : "skipped"
    })),
    startedAt: current?.startedAt || now,
    updatedAt: now,
    terminal: true
  };
}

function isRecoveredFailure(plan: AssistedInstallPlan): boolean {
  const status = plan.status?.toLowerCase();
  const recovery = plan.recoveryStatus?.toLowerCase() ?? "";
  if (status === "partial" && recovery !== "required") return false;
  const alreadyRecovered = recovery.includes("rolled") || recovery.includes("restored") ||
    recovery === "completed";
  return !alreadyRecovered && ["interrupted", "failed", "partial", "cancelled"].includes(status);
}

function recoveredFailureProgress(plan: AssistedInstallPlan, t: Translate): AssistedInstallProgress {
  const now = new Date().toISOString();
  const completedSteps = plan.steps.filter(step => step.status === "completed").length;
  return {
    referenceId: plan.id,
    runId: "",
    sequence: 1,
    phase: plan.status || "interrupted",
    message: plan.transactionId
      ? t("安装已中断，请先回滚已完成步骤", "Installation was interrupted. Roll back completed steps first")
      : t("安装已中断，请新建计划", "Installation was interrupted. Create a new plan"),
    completedSteps,
    totalSteps: plan.steps.length,
    activityCount: completedSteps,
    steps: plan.steps.map(step => ({
      id: step.id,
      title: step.title,
      kind: step.kind,
      status: step.status || "queued",
      error: step.error
    })),
    startedAt: plan.createdAt || now,
    updatedAt: now,
    terminal: true
  };
}

function persistedPlanProgress(plan: AssistedInstallPlan, t: Translate): AssistedInstallProgress {
  const now = new Date().toISOString();
  const status = plan.status?.toLowerCase() || "interrupted";
  const recoveryCompleted = plan.recoveryStatus?.toLowerCase() === "completed";
  const completedSteps = plan.steps.filter(step =>
    ["completed", "skipped", "manual-pending", "rolled-back"].includes(step.status)
  ).length;
  const terminal = status !== "running";
  const message = status === "completed"
    ? t("一键安装已完成", "Assisted installation completed")
    : status === "partial"
      ? t("自动步骤已完成，人工步骤仍待处理", "Automatic steps completed; manual work remains")
    : status === "running"
      ? t("正在恢复执行状态", "Restoring execution state")
      : recoveryCompleted
        ? t("执行未完成，自动恢复已完成", "Execution did not complete; automatic recovery finished")
        : t("执行未完成，需要检查恢复状态", "Execution did not complete; review the recovery state");
  return {
    referenceId: plan.id,
    runId: plan.transactionId || "",
    sequence: 1,
    phase: status,
    message,
    completedSteps,
    totalSteps: plan.steps.length,
    activityCount: completedSteps,
    steps: plan.steps.map(step => ({
      id: step.id,
      title: step.title,
      kind: step.kind,
      status: step.status || "queued",
      error: step.error
    })),
    startedAt: plan.createdAt || now,
    updatedAt: now,
    terminal
  };
}

function reconcileProgressWithPlan(
  progress: AssistedInstallProgress,
  plan: AssistedInstallPlan
): AssistedInstallProgress {
  const terminalCount = plan.steps.filter(step =>
    ["completed", "skipped", "manual-pending", "rolled-back"].includes(step.status)
  ).length;
  return {
    ...progress,
    sequence: progress.sequence + 1,
    completedSteps: terminalCount,
    totalSteps: plan.steps.length,
    steps: plan.steps.map(step => ({
      id: step.id,
      title: step.title,
      kind: step.kind,
      status: step.status || "queued",
      startedAt: step.startedAt,
      completedAt: step.completedAt,
      error: step.error
    }))
  };
}

function isExecutionProgress(value: AssistedInstallProgress): boolean {
  return value.runId.startsWith("tx-") ||
    ["starting", "running", "executing", "recovering", "cancelling", "partial",
      "cancelled", "interrupted", "rolled-back"].includes(value.phase);
}

function requirementKey(requirement: string | AssistedInstallRequirement, index: number): string {
  return typeof requirement === "string" ? `${requirement}-${index}` :
    requirement.id || `${requirement.title || requirement.name || "requirement"}-${index}`;
}

function highestSeverity(clusters: RiskCluster[]): Severity {
  return clusters.reduce<Severity>((highest, cluster) =>
    severityRank(cluster.severity) > severityRank(highest) ? cluster.severity : highest, "informational");
}

const severityOrder: Severity[] = ["critical", "high", "medium", "low", "informational"];

function severityRank(severity: Severity): number {
  return ["informational", "low", "medium", "high", "critical"].indexOf(severity);
}

function severityLabel(severity: Severity, locale: string): string {
  const labels: Record<Severity, [string, string]> = {
    informational: ["提示", "Info"],
    low: ["低风险", "Low"],
    medium: ["中风险", "Medium"],
    high: ["高风险", "High"],
    critical: ["严重风险", "Critical"]
  };
  return locale === "en-US" ? labels[severity][1] : labels[severity][0];
}

function complexityLabel(complexity: string, t: Translate): string {
  const normalized = complexity?.toLowerCase();
  if (normalized === "low" || normalized === "simple") return t("简单", "Simple");
  if (normalized === "high" || normalized === "complex") return t("复杂", "Complex");
  if (normalized === "medium" || normalized === "moderate") return t("中等", "Moderate");
  return complexity || t("未标注", "Not specified");
}

function permissionRiskLabel(risk: AssistedInstallPermission["risk"], t: Translate): string {
  if (risk === "critical") return t("严重", "Critical");
  if (risk === "high") return t("高", "High");
  if (risk === "medium") return t("中", "Medium");
  if (risk === "low") return t("低", "Low");
  return t("标准", "Standard");
}

function permissionRisk(permission: AssistedInstallPermission): AssistedInstallPermission["risk"] {
  if (permission.risk) return permission.risk;
  if (permission.id === "managed-native-code" || permission.kind === "high-risk-process") return "high";
  return "standard";
}

function stepStatusLabel(status: string, t: Translate): string {
  if (status === "completed") return t("已完成", "Completed");
  if (status === "running") return t("正在执行", "Running");
  if (status === "failed") return t("失败", "Failed");
  if (status === "skipped") return t("已跳过", "Skipped");
  if (status === "cancelled") return t("已取消", "Cancelled");
  if (status === "interrupted") return t("已中断", "Interrupted");
  if (status === "rolled-back") return t("已回滚", "Rolled back");
  if (status === "manual-pending") return t("待手动处理", "Manual work pending");
  return t("等待执行", "Queued");
}

function unique(values: string[]): string[] {
  return [...new Set(values.filter(Boolean))];
}

function localeJoiner(): string {
  return document.documentElement.lang === "en-US" ? ", " : "、";
}

function saveInstallDraft(draft: InstallDraft): void {
  try {
    window.localStorage.setItem(INSTALL_DRAFT_KEY, JSON.stringify(draft));
  } catch {
    // The fields remain in component state while this dialog is open.
  }
}

function readInstallDraft(): InstallDraft | null {
  try {
    const raw = window.localStorage.getItem(INSTALL_DRAFT_KEY);
    if (!raw) return null;
    const value = JSON.parse(raw) as Partial<InstallDraft>;
    if ((value.installMethod !== "standard" && value.installMethod !== "assisted") ||
      (value.sourceMethod !== "github" && value.sourceMethod !== "local") ||
      typeof value.source !== "string" || typeof value.requestedRef !== "string") return null;
    return {
      installMethod: value.installMethod,
      sourceMethod: value.sourceMethod,
      source: value.source.slice(0, 4096),
      requestedRef: value.requestedRef.slice(0, 512)
    };
  } catch {
    return null;
  }
}

function clearInstallDraft(): void {
  try {
    window.localStorage.removeItem(INSTALL_DRAFT_KEY);
  } catch {
    // A stale non-secret draft can be overwritten the next time the dialog opens.
  }
}

function setActiveInstallReference(
  reference: Pick<ActiveInstallReference, "kind" | "id">
): void {
  try {
    window.localStorage.setItem(
      ACTIVE_PLAN_KEY,
      serializeActiveInstallReference(createActiveInstallReference(reference.kind, reference.id))
    );
  } catch {
    // Recovery remains available through the backend journal.
  }
}

function readActiveInstallReference(): ActiveInstallReference | null {
  try {
    const raw = window.localStorage.getItem(ACTIVE_PLAN_KEY);
    const parsed = parseActiveInstallReference(raw);
    if (!parsed) return null;
    if (parsed.migrated) {
      window.localStorage.setItem(
        ACTIVE_PLAN_KEY,
        serializeActiveInstallReference(parsed.reference)
      );
    }
    return parsed.reference;
  } catch {
    return null;
  }
}

function clearActivePlan(): void {
  try {
    window.localStorage.removeItem(ACTIVE_PLAN_KEY);
  } catch {
    // A stale browser key is harmless; the backend still validates plan expiry.
  }
}
