import {
  Badge,
  Button,
  DataList,
  FeedbackBanner,
  VerdictHero,
  formatBytes,
  resolveError,
  runTracked,
  toast,
  useFeedbackStore,
} from "../shared/design-system";
import { api } from "../api";
import {
  formatOpKind,
  formatRuntimeState,
  formatStep,
  toUiLang,
  type ManagerLocale,
} from "../i18n";
import type { OperationItem } from "../types";

export function ActivityPage({
  operations,
  locale,
  onRefresh,
}: {
  operations: OperationItem[];
  locale: ManagerLocale;
  onRefresh: () => Promise<void>;
}) {
  const store = useFeedbackStore();
  const uiLang = toUiLang(locale);
  const en = uiLang === "en";

  const sorted = [...operations].sort(
    (a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime(),
  );
  const runningCount = sorted.filter((o) => o.state === "running").length;

  const handleCancelOp = (op: OperationItem) =>
    runTracked(
      { key: `op:cancel:${op.id}`, scope: "activity", lang: uiLang },
      async () => {
        await api("/api/v1/tasks/cancel", { id: op.id });
        await onRefresh();
        toast(en ? "Task cancelled." : "已取消后台任务。", "info");
      },
    );

  // M-29 & M-30: Per-task busy guard + full repository & prepare parameters on Retry!
  const handleRetryOp = (op: OperationItem) =>
    runTracked(
      { key: `op:retry:${op.id}`, scope: "activity", lang: uiLang },
      async () => {
        if (op.kind === "prepare" && op.prepare) {
          await api("/api/v1/deployments/prepare", op.prepare);
        } else if (op.kind === "fallback_cpu") {
          const q = op.deployment_id
            ? `?model=${encodeURIComponent(op.deployment_id)}`
            : "";
          await api(`/api/v1/runtime/fallback-cpu${q}`, {});
        } else if (op.kind === "ocr_install") {
          await api("/api/v1/ocr/install", {});
        } else if (op.kind === "install" && op.asset_kind) {
          await api("/api/v1/assets/install", {
            kind: op.asset_kind,
            id: op.asset_id || "",
            repository: op.repository || "",
          });
        } else if (op.kind === "update") {
          await api("/api/v1/update/install", {});
        }
        await onRefresh();
        toast(en ? "Retrying task…" : "已重新发起任务…", "info");
      },
    );

  // M-10 & M-28: Format file-count progress vs byte progress cleanly, and use indeterminate progress bar without "Unknown" when total == 0!
  const formatOpProgressText = (op: OperationItem): string => {
    const stepText = formatStep(locale, op.step);
    const isFileCount = op.kind === "import" || op.step === "importing";
    if (op.total > 0) {
      if (isFileCount) {
        return en
          ? `${stepText ? stepText + " · " : ""}${op.progress} / ${op.total} file(s)`
          : `${stepText ? stepText + " · " : ""}${op.progress} / ${op.total} 个文件`;
      }
      const pct = Math.min(100, Math.round((op.progress / op.total) * 100));
      return `${stepText ? stepText + " · " : ""}${formatBytes(op.progress, uiLang)} / ${formatBytes(op.total, uiLang)} (${pct}%)`;
    }
    if (op.state === "running") {
      return stepText || (en ? "Working…" : "正在处理…");
    }
    return stepText;
  };

  const canRetry = (op: OperationItem) =>
    (op.state === "failed" ||
      op.state === "cancelled" ||
      op.state === "interrupted") &&
    (op.kind === "prepare" ||
      op.kind === "install" ||
      op.kind === "fallback_cpu" ||
      op.kind === "ocr_install");

  return (
    <section class="panel mg-section">
      <VerdictHero
        eyebrow={en ? "BACKGROUND TASK HISTORY" : "后台下载与准备任务"}
        title={
          runningCount > 0
            ? en
              ? `${runningCount} active background task(s)`
              : `当前有 ${runningCount} 项后台任务正在运行`
            : en
              ? `${sorted.length} recorded task(s)`
              : `共记录 ${sorted.length} 项后台任务`
        }
        description={
          en
            ? "Interrupted downloads preserve verified .part files and resume automatically when retried."
            : "网络中断或取消的下载会保留已完成分片（.part），点击「重试」可直接断点续传。"
        }
        badge={
          <Badge tone={runningCount > 0 ? "info" : "neutral"}>
            {runningCount > 0
              ? en
                ? "Active"
                : "运行中"
              : en
                ? "Idle"
                : "空闲"}
          </Badge>
        }
      />

      <FeedbackBanner scope="activity" lang={uiLang} />

      <DataList
        items={sorted}
        getKey={(op) => op.id}
        emptyMessage={
          en ? "No background tasks yet." : "暂无后台下载、导入或更新记录。"
        }
        lang={uiLang}
        renderItem={(op) => {
          // M-09: Concise badge ("已取消" / "已完成" / "失败"), never stuffing full sentence into badge!
          const badgeTone =
            op.state === "running"
              ? "info"
              : op.state === "success"
                ? "success"
                : op.state === "failed"
                  ? "error"
                  : "neutral";
          const badgeText = formatRuntimeState(locale, op.state);
          const progressLabel = formatOpProgressText(op);
          const pct =
            op.total > 0
              ? Math.min(100, Math.max(2, Math.round((op.progress / op.total) * 100)))
              : 0;

          return (
            <div style={{ width: "100%" }}>
              <div class="split-head">
                <div class="list-row-main">
                  <div style={{ display: "flex", gap: "8px", alignItems: "center", flexWrap: "wrap" }}>
                    <strong>
                      {formatOpKind(locale, op.kind)}
                      {op.asset_id
                        ? ` · ${op.asset_id}`
                        : op.deployment_id
                          ? ` · ${op.deployment_id}`
                          : ""}
                    </strong>
                    <Badge tone={badgeTone}>{badgeText}</Badge>
                  </div>
                  <p class="hint" style={{ margin: "4px 0 0" }}>
                    {[
                      progressLabel,
                      op.repository ? `${en ? "Repo" : "仓库"}: ${op.repository}` : "",
                      op.detail || "",
                      new Date(op.updated_at).toLocaleString(),
                    ]
                      .filter(Boolean)
                      .join(" · ")}
                  </p>
                </div>

                <div class="actions">
                  {op.state === "running" ? (
                    <Button
                      variant="danger"
                      busy={store.isBusy(`op:cancel:${op.id}`)}
                      onClick={() => void handleCancelOp(op)}
                    >
                      {en ? "Cancel" : "取消"}
                    </Button>
                  ) : null}
                  {canRetry(op) ? (
                    <Button
                      variant="primary"
                      busy={store.isBusy(`op:retry:${op.id}`)}
                      disabled={runningCount > 0}
                      onClick={() => void handleRetryOp(op)}
                    >
                      {en ? "Retry" : "重试"}
                    </Button>
                  ) : null}
                </div>
              </div>

              {/* M-28: Determinate bar when total > 0, smooth indeterminate animation when total == 0 */}
              {op.state === "running" ? (
                <div class="mg-progress-track">
                  <div
                    class={`mg-progress-fill${op.total <= 0 ? " indeterminate" : ""}`}
                    style={op.total > 0 ? { width: `${pct}%` } : undefined}
                  />
                </div>
              ) : null}

              {/* M-09: Explanatory cancellation note vs error reason */}
              {op.state === "cancelled" ? (
                <p class="hint" style={{ marginTop: "6px" }}>
                  {en
                    ? "Task cancelled by user. Previously installed models and runtimes remain intact."
                    : "任务已取消，已验证安装的模型与运行时保持完整可用。"}
                </p>
              ) : op.error ? (
                <div
                  class="feedback-banner"
                  data-tone={op.state === "interrupted" ? "warning" : "error"}
                  style={{ marginTop: "8px", marginBottom: 0 }}
                >
                  <div>
                    <span>{resolveError(op.error, uiLang).message}</span>
                  </div>
                  {op.error.includes(":") ? (
                    <details>
                      <summary>{en ? "Technical details" : "展开原始原因"}</summary>
                      <pre>{op.error}</pre>
                    </details>
                  ) : null}
                </div>
              ) : null}
            </div>
          );
        }}
      />
    </section>
  );
}
