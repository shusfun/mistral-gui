import { useState } from "preact/hooks";
import {
  Badge,
  Button,
  CopyField,
  FeedbackBanner,
  IconCopy,
  IconExternalLink,
  IconPlay,
  IconRefresh,
  IconSquare,
  IconTranslate,
  MetricGrid,
  SettingGroup,
  SettingRow,
  feedbackStore,
  formatBytes,
  resolveError,
  runTracked,
  toast,
  useFeedbackStore,
} from "../shared/design-system";
import { api, isDesktopMode, nativeCommand } from "../api";
import {
  formatBackend,
  formatRuntimeState,
  formatStage,
  t,
  toUiLang,
  type ManagerLocale,
} from "../i18n";
import type { RouteId, StatusResponse } from "../types";

export function OverviewPage({
  data,
  locale,
  onRefresh,
  onNavigate,
}: {
  data: StatusResponse;
  locale: ManagerLocale;
  onRefresh: () => Promise<void>;
  onNavigate: (route: RouteId) => void;
}) {
  const store = useFeedbackStore();
  const uiLang = toUiLang(locale);
  const en = uiLang === "en";
  const [tokenText, setTokenText] = useState("");
  const [testSample, setTestSample] = useState<{
    latencyMs: number;
    output: string;
  } | null>(null);

  const st = data.status;
  const hasEngine = Boolean(data.settings.engine);
  const hasModel = Boolean(data.model_revision);
  const isRunning = st.state === "running";
  const isTransitioning = st.state === "starting" || st.state === "loading";
  const isCooldown = st.state === "cooldown";
  const isFailed = st.state === "failed";

  const setupStep = !hasEngine
    ? 1
    : !hasModel
      ? 2
      : !isRunning
        ? 3
        : 4;

  const badgeTone = isRunning
    ? "success"
    : isFailed || isCooldown
      ? "error"
      : isTransitioning
        ? "info"
        : "neutral";

  const handleRuntimeAction = (
    action: "start" | "stop" | "restart" | "fallback-cpu",
  ) =>
    runTracked(
      { key: `overview:${action}`, scope: "overview", lang: uiLang },
      async () => {
        if (action === "fallback-cpu") {
          await api("/api/v1/runtime/fallback-cpu", {});
          toast(
            en ? "Started CPU fallback task." : "已启动 CPU 回退准备任务。",
            "info",
          );
        } else {
          await api(`/api/v1/runtime/${action}`, {});
        }
        await onRefresh();
      },
    );

  const handleQuickTest = () =>
    runTracked(
      { key: "overview:test", scope: "overview", lang: uiLang },
      async () => {
        const started = performance.now();
        const res = await api<{
          segments: { translation?: string }[];
        }>("/api/v1/translate", {
          text: "Local inference keeps your reading fast and private.",
          routing: "local-only",
          target_lang: "zh-CN",
        });
        const elapsed = Math.round(performance.now() - started);
        const out = res.segments?.[0]?.translation || "";
        setTestSample({ latencyMs: elapsed, output: out });
        feedbackStore.setBanner("overview", {
          tone: "success",
          message: en
            ? `Verified in ${elapsed} ms: "${out}"`
            : `本地推理校验通过（${elapsed} ms）：「${out}」`,
        });
        await onRefresh();
      },
    );

  const fetchTokenIfNeeded = async (): Promise<string> => {
    if (tokenText) return tokenText;
    const res = await api<{ token: string }>("/api/v1/token");
    setTokenText(res.token);
    return res.token;
  };

  // M-VD: Copy full Qingyi connection JSON
  const handleCopyQingyiJson = () =>
    runTracked(
      { key: "overview:copy-qy", scope: "overview", lang: uiLang },
      async () => {
        const tok = await fetchTokenIfNeeded();
        const bundle = {
          localUrl: data.endpoint,
          localToken: tok,
          localModel: data.settings.model,
          localBackend: data.settings.backend || "auto",
        };
        await navigator.clipboard.writeText(JSON.stringify(bundle, null, 2));
        toast(
          en
            ? "Copied Qingyi connection JSON."
            : "已复制轻译连接配置 JSON，可在轻译设置页一键粘贴。",
          "success",
        );
      },
    );

  const activeDeployments = data.deployments.filter(
    (d) => d.status.state === "running",
  ).length;

  return (
    <section class="panel mg-section">
      {/* M-S01 & M-R4: Recovery & Eviction alerts */}
      {data.recovered_from_backup ? (
        <div class="feedback-banner" data-tone="warning" role="status">
          {en
            ? "Primary state.json was restored automatically from state.json.bak."
            : "检测到主状态文件受损，已自动从安全备份 state.json.bak 恢复。"}
        </div>
      ) : null}
      {data.evicted_deployments && data.evicted_deployments.length > 0 ? (
        <div class="feedback-banner" data-tone="info" role="status">
          {en
            ? `Idle deployment(s) unloaded automatically to free memory: ${data.evicted_deployments.join(", ")}`
            : `为保障显存充足，已自动释放空闲实例：${data.evicted_deployments.join("、")}`}
        </div>
      ) : null}

      <div class="mg-page-head">
        <div class="mg-page-title-group">
          <h1 class="mg-page-title">{t(locale, "navOverview")}</h1>
          <Badge tone={badgeTone}>
            {!hasEngine || !hasModel
              ? t(locale, "state_uninstalled")
              : isTransitioning
                ? formatStage(locale, st.stage) ||
                  formatRuntimeState(locale, st.state)
                : formatRuntimeState(locale, st.state)}
          </Badge>
          {st.degraded_cpu ? (
            <Badge tone="warning">{en ? "CPU Fallback" : "CPU 回退运行"}</Badge>
          ) : null}
        </div>

        <div class="actions" style={{ margin: 0 }}>
          {!hasEngine || !hasModel ? (
            <Button variant="primary" onClick={() => onNavigate("models")}>
              {en ? "Download Runtime & Model" : "前往下载运行时与模型"}
            </Button>
          ) : (
            <>
              {!isRunning && !isTransitioning ? (
                <Button
                  variant="primary"
                  size="sm"
                  icon={<IconPlay size={13} />}
                  busy={store.isBusy("overview:start")}
                  onClick={() => void handleRuntimeAction("start")}
                >
                  {isCooldown
                    ? en
                      ? "Retry Now"
                      : "立即重试"
                    : en
                      ? "Start"
                      : "启动模型"}
                </Button>
              ) : null}
              {/* M-14: Allow immediate Stop during starting/loading as well as running */}
              {isRunning || isTransitioning ? (
                <Button
                  variant="danger"
                  size="sm"
                  icon={<IconSquare size={13} />}
                  busy={store.isBusy("overview:stop")}
                  onClick={() => void handleRuntimeAction("stop")}
                >
                  {isTransitioning
                    ? en
                      ? "Abort"
                      : "中止加载"
                    : en
                      ? "Stop"
                      : "停止"}
                </Button>
              ) : null}
              {isRunning ? (
                <Button
                  size="sm"
                  icon={<IconRefresh size={13} />}
                  busy={store.isBusy("overview:restart")}
                  onClick={() => void handleRuntimeAction("restart")}
                >
                  {en ? "Restart" : "重启"}
                </Button>
              ) : null}
              <Button
                size="sm"
                busy={store.isBusy("overview:test")}
                disabled={!hasEngine || !hasModel}
                onClick={handleQuickTest}
              >
                {en ? "Quick Test" : "快速自检"}
              </Button>
              {(isFailed || isCooldown) && data.selected_backend !== "cpu" ? (
                <Button
                  size="sm"
                  busy={store.isBusy("overview:fallback-cpu")}
                  onClick={() => void handleRuntimeAction("fallback-cpu")}
                >
                  {en ? "Use CPU" : "改用 CPU"}
                </Button>
              ) : null}
            </>
          )}
          <Button
            size="sm"
            icon={<IconTranslate size={14} />}
            onClick={() => onNavigate("translate")}
          >
            {en ? "Translator" : "打开翻译"}
          </Button>
          {isRunning ? (
            <Button
              variant="ghost"
              size="sm"
              icon={<IconExternalLink size={13} />}
              onClick={async () => {
                if (isDesktopMode) {
                  await nativeCommand("open-ui", "");
                } else {
                  window.open("/ui", "_blank", "noopener,noreferrer");
                }
              }}
            >
              WebUI
            </Button>
          ) : null}
        </div>
      </div>

      <FeedbackBanner scope="overview" lang={uiLang} />

      {/* Surface structured engine failure / crash cooldown reason (M-R2, M-R5) */}
      {st.error ? (
        <div class="feedback-banner" data-tone="error" role="alert">
          <div>
            <strong>{resolveError(st.error, uiLang).message}</strong>
            {st.cooldown_until ? (
              <span>
                {" "}
                {en
                  ? `(Cooldown until ${new Date(st.cooldown_until).toLocaleTimeString()})`
                  : `（冷却至 ${new Date(st.cooldown_until).toLocaleTimeString()}）`}
              </span>
            ) : null}
          </div>
          {st.detail || st.last_failure?.log_tail ? (
            <details style={{ marginTop: "8px" }}>
              <summary>
                <span>{en ? "Engine log output" : "展开引擎错误日志"}</span>
              </summary>
              <div class="ds-disclosure-body">
                <pre>{st.last_failure?.log_tail || st.detail}</pre>
              </div>
            </details>
          ) : null}
        </div>
      ) : null}

      <MetricGrid
        items={[
          {
            label: en ? "Default Model" : "默认模型",
            value: st.model || data.settings.model || t(locale, "unknown"),
          },
          {
            label: en ? "Backend" : "加速后端",
            value: formatBackend(locale, st.backend || data.selected_backend),
          },
          {
            label: en ? "Deployments / Requests" : "运行实例 / 累计请求",
            value: `${activeDeployments}/${data.deployments.length} · ${st.requests ?? 0}`,
          },
          {
            label: en ? "Test Latency" : "自检耗时",
            value: testSample ? `${testSample.latencyMs} ms` : "—",
          },
        ]}
      />

      {setupStep < 4 ? (
        <SettingGroup title={en ? "Setup Checklist" : "准备步骤"}>
          <SettingRow
            title={en ? "Inference Runtime (mistral.rs)" : "推理运行时引擎"}
            description={
              hasEngine
                ? en
                  ? `Installed (${data.settings.engine_version || "ready"})`
                  : `已安装（${data.settings.engine_version || "就绪"}）`
                : en
                  ? "Auto-matches CUDA, Metal, or CPU for your hardware"
                  : "自动匹配本机 CUDA、Metal 或 CPU 预编译发行包"
            }
          >
            <Badge tone={hasEngine ? "success" : "neutral"}>
              {hasEngine ? (en ? "Ready" : "已就绪") : en ? "Pending" : "待安装"}
            </Badge>
          </SettingRow>
          <SettingRow
            title={en ? "Translation Model Weights" : "翻译模型权重"}
            description={
              hasModel
                ? en
                  ? `Selected: ${data.settings.model}`
                  : `当前已选：${data.settings.model}`
                : en
                  ? "HY-MT1.5-1.8B Q4_K_M (~1.1 GiB) runs smoothly on CPU or GPU"
                  : "推荐 HY-MT1.5-1.8B Q4_K_M（约 1.1 GiB），无独显也可流畅运行"
            }
          >
            {hasModel ? (
              <Badge tone="success">{en ? "Ready" : "已就绪"}</Badge>
            ) : (
              <Button
                size="sm"
                variant="primary"
                onClick={() => onNavigate("models")}
              >
                {en ? "Download" : "去下载"}
              </Button>
            )}
          </SettingRow>
        </SettingGroup>
      ) : null}

      {/* M-VD: One-click Qingyi Extension Connection Card */}
      <SettingGroup
        title={
          en ? "Qingyi Browser Extension" : "连接「轻译 Qingyi」浏览器扩展"
        }
        padded
        actions={
          <>
            <Button
              variant="primary"
              size="sm"
              icon={<IconCopy size={13} />}
              busy={store.isBusy("overview:copy-qy")}
              onClick={handleCopyQingyiJson}
            >
              {en ? "Copy Connection JSON" : "一键复制连接配置（含 Token）"}
            </Button>
            <Button size="sm" onClick={() => onNavigate("connection")}>
              {en ? "Token & API" : "管理凭证与示例"}
            </Button>
          </>
        }
      >
        <div class="grid-2">
          <CopyField
            label={en ? "Local Manager URL" : "本地管理器地址"}
            value={data.endpoint}
            lang={uiLang}
          />
          <CopyField
            label={en ? "Default Model ID" : "当前默认模型"}
            value={data.settings.model}
            lang={uiLang}
          />
        </div>
        {data.dictionary_sync.revisions > 0 ||
        data.dictionary_sync.pack_entries > 0 ? (
          <p class="hint" style={{ marginTop: "10px", marginBottom: 0 }}>
            {en
              ? `Synced from Qingyi: ${data.dictionary_sync.revisions} revision(s), ${data.dictionary_sync.pack_entries} dictionary entry(ies).`
              : `已同步轻译词库：人工修订 ${data.dictionary_sync.revisions} 条 · 词库词条 ${data.dictionary_sync.pack_entries} 条。`}
          </p>
        ) : null}
        {data.temp_storage.bytes > 0 ? (
          <p class="hint" style={{ marginTop: "6px", marginBottom: 0 }}>
            {en
              ? `Resumable staging cache: ${formatBytes(data.temp_storage.bytes, uiLang)}`
              : `未完成下载暂存：${formatBytes(data.temp_storage.bytes, uiLang)}（可在「更新与设置」清理）。`}
          </p>
        ) : null}
      </SettingGroup>
    </section>
  );
}
