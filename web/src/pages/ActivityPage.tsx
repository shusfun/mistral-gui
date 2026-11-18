import {
  Badge,
  Button,
  DataList,
  FeedbackBanner,
  IconRefresh,
  IconSquare,
  SettingGroup,
  formatBytes,
  resolveError,
  runTracked,
  toast,
  useFeedbackStore,
} from "../shared/design-system";
import { api, isDesktopMode, nativeCommand } from "../api";
import {
  formatOpKind,
  formatRuntimeState,
  formatStep,
  t,
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
    (a, b) =>
      new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime(),
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

  // M-29 & M-30: Per-task busy guard + full repository & prepare parameters on Retry
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
          if (isDesktopMode) await nativeCommand("update");
          else await api("/api/v1/update/install", {});
        }
        await onRefresh();
        toast(en ? "Retrying task…" : "已重新发起任务…", "info");
      },
    );

  // M-10 & M-28: Format file-count progress vs byte progress cleanly
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
      <div class="mg-page-head">
        <div class="mg-page-title-group">
          <h1 class="mg-page-title">{t(locale, "navActivity")}</h1>
          <Badge tone={runningCount > 0 ? "info" : "neutral"}>
            {runningCount > 0
              ? en
                ? `${runningCount} Running`
                : `${runningCount} 项运行中`
              : en
                ? `${sorted.length} Total`
                : `共 ${sorted.length} 条`}
          </Badge>
        </div>
        <Button
          size="sm"
          icon={<IconRefresh size={13} />}
          onClick={() => void onRefresh()}
        >
          {en ? "Refresh" : "刷新"}
        </Button>
      </div>

      <FeedbackBanner scope="activity" lang={uiLang} />

      <SettingGroup
        description={
          en
            ? "Interrupted downloads keep verified .part chunks and resume automatically on retry."
            : "中断或取消的下载会保留已完成分片（.part），点击重试即可断点续传。"
        }
        padded
      >
        <DataList
          items={sorted}
          getKey={(op) => op.id}
          emptyMessage={
            en ? "No background tasks yet." : "暂无后台下载、导入或更新记录。"
          }
          lang={uiLang}
          renderItem={(op) => {
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
                ? Math.min(
                    100,
                    Math.max(2, Math.round((op.progress / op.total) * 100)),
                  )
                : 0;

            return (
              <div style={{ width: "100%" }}>
                <div class="split-head">
                  <div class="list-row-main">
                    <div
                      style={{
                        display: "flex",
                        gap: "8px",
                        alignItems: "center",
                        flexWrap: "wrap",
                      }}
                    >
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
                        op.repository
                          ? `${en ? "Repo" : "仓库"}: ${op.repository}`
                          : "",
                        op.detail || "",
                        new Date(op.updated_at).toLocaleString(),
                      ]
                        .filter(Boolean)
                        .join(" · ")}
                    </p>
                  </div>

                  <div class="actions" style={{ margin: 0 }}>
                    {op.state === "running" ? (
                      <Button
                        size="sm"
                        variant="danger"
                        icon={<IconSquare size={12} />}
                        busy={store.isBusy(`op:cancel:${op.id}`)}
                        onClick={() => void handleCancelOp(op)}
                      >
                        {en ? "Cancel" : "取消"}
                      </Button>
                    ) : null}
                    {canRetry(op) ? (
                      <Button
                        size="sm"
                        variant="primary"
                        icon={<IconRefresh size={13} />}
                        busy={store.isBusy(`op:retry:${op.id}`)}
                        disabled={runningCount > 0}
                        onClick={() => void handleRetryOp(op)}
                      >
                        {en ? "Retry" : "重试"}
                      </Button>
                    ) : null}
                  </div>
                </div>

                {op.state === "running" ? (
                  <div class="mg-progress-track">
                    <div
                      class={`mg-progress-fill${op.total <= 0 ? " indeterminate" : ""}`}
                      style={op.total > 0 ? { width: `${pct}%` } : undefined}
                    />
                  </div>
                ) : null}

                {op.state === "cancelled" ? (
                  <p class="hint" style={{ marginTop: "6px" }}>
                    {en
                      ? "Cancelled. Previously installed assets remain intact."
                      : "已取消，先前安装的模型与运行时保持完整。"}
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
                      <details style={{ marginTop: "6px" }}>
                        <summary>
                          <span>{en ? "Technical details" : "展开原始原因"}</span>
                        </summary>
                        <div class="ds-disclosure-body">
                          <pre>{op.error}</pre>
                        </div>
                      </details>
                    ) : null}
                  </div>
                ) : null}
              </div>
            );
          }}
        />
      </SettingGroup>
    </section>
  );
}
