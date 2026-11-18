import { useState } from "preact/hooks";
import {
  Badge,
  Button,
  CopyField,
  FeedbackBanner,
  VerdictHero,
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

  // Setup step calculation
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

  const handleRuntimeAction = (action: "start" | "stop" | "restart" | "fallback-cpu") =>
    runTracked(
      { key: `overview:${action}`, scope: "overview", lang: uiLang },
      async () => {
        if (action === "fallback-cpu") {
          await api("/api/v1/runtime/fallback-cpu", {});
          toast(
            en
              ? "Started CPU fallback task."
              : "已启动 CPU 回退准备任务。",
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
            ? `Inference verified in ${elapsed} ms: "${out}"`
            : `本地推理校验通过（耗时 ${elapsed} ms）：「${out}」`,
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

  // M-VD: Copy full Qingyi connection JSON (compatible with Qingyi ServiceSection one-click paste!)
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
            ? "Copied Qingyi connection JSON. Paste it directly in Qingyi Settings → Translation Service!"
            : "已复制轻译连接配置 JSON，可前往轻译扩展「翻译服务」点击一键粘贴！",
          "success",
        );
      },
    );

  return (
    <section class="panel mg-section">
      {/* M-S01 & M-R4: Recovery & Eviction alerts */}
      {data.recovered_from_backup ? (
        <div class="feedback-banner" data-tone="warning" role="status">
          {en
            ? "Primary state.json was damaged on last exit and was automatically restored from state.json.bak."
            : "检测到上次异常退出导致主状态文件受损，已自动从安全备份 state.json.bak 恢复配置与资产列表。"}
        </div>
      ) : null}
      {data.evicted_deployments && data.evicted_deployments.length > 0 ? (
        <div class="feedback-banner" data-tone="info" role="status">
          {en
            ? `Idle deployment(s) automatically unloaded to free VRAM/RAM: ${data.evicted_deployments.join(", ")}`
            : `为保障新模型加载显存/内存充足，已自动卸载空闲实例：${data.evicted_deployments.join("、")}`}
        </div>
      ) : null}

      <VerdictHero
        eyebrow={en ? "DEFAULT RUNTIME OVERVIEW" : "本地推理总览"}
        tone={badgeTone}
        title={
          !hasEngine || !hasModel
            ? en
              ? "Set up your local translation model in one minute"
              : "一分钟完成本地翻译模型部署，按需唤醒、空闲自动释放"
            : isRunning
              ? en
                ? `${st.model || data.settings.model} is ready for requests`
                : `${st.model || data.settings.model} 正在运行，随时响应翻译请求`
              : isTransitioning
                ? en
                  ? `Starting ${st.model || data.settings.model} (${formatStage(locale, st.stage) || formatRuntimeState(locale, st.state)})…`
                  : `正在启动 ${st.model || data.settings.model}（${formatStage(locale, st.stage) || formatRuntimeState(locale, st.state)}）…`
                : en
                  ? `${data.settings.model} is installed and standing by`
                  : `${data.settings.model} 已安装就绪（${data.settings.auto_start ? "收到请求时自动按需启动" : "手动启动模式"}）`
        }
        description={
          st.degraded_cpu
            ? en
              ? "Currently running in CPU fallback mode after GPU failure."
              : "当前已回退至 CPU 模式稳定运行。"
            : en
              ? `Endpoint: ${data.endpoint} · Active deployments: ${data.deployments.filter((d) => d.status.state === "running").length}/${data.deployments.length}`
              : `本机监听：${data.endpoint} · 运行中实例：${data.deployments.filter((d) => d.status.state === "running").length} / ${data.deployments.length}`
        }
        badge={
          <Badge tone={badgeTone}>
            {!hasEngine || !hasModel
              ? t(locale, "state_uninstalled")
              : formatRuntimeState(locale, st.state)}
          </Badge>
        }
        metrics={[
          {
            label: en ? "Selected Model" : "默认模型",
            value: st.model || data.settings.model || t(locale, "unknown"),
          },
          {
            label: en ? "Backend" : "加速后端",
            value: formatBackend(locale, st.backend || data.selected_backend),
          },
          {
            label: en ? "Total / Active Requests" : "累计请求 / 进行中",
            value: `${st.requests ?? 0} / ${st.active ?? 0}`,
          },
          {
            label: en ? "Last Test Latency" : "最近测试耗时",
            value: testSample ? `${testSample.latencyMs} ms` : "—",
          },
        ]}
        actions={
          <>
            {!hasEngine || !hasModel ? (
              <Button variant="primary" onClick={() => onNavigate("models")}>
                {en
                  ? "1. Download Runtime & Model →"
                  : "前往下载推理运行时与模型 →"}
              </Button>
            ) : (
              <>
                {!isRunning && !isTransitioning ? (
                  <Button
                    variant="primary"
                    busy={store.isBusy("overview:start")}
                    onClick={() => void handleRuntimeAction("start")}
                  >
                    {isCooldown
                      ? en
                        ? "Retry now (exit cooldown)"
                        : "立即重试启动"
                      : en
                        ? "Start model"
                        : "启动默认模型"}
                  </Button>
                ) : null}
                {/* M-14: Allow immediate Stop during starting/loading as well as running! */}
                {isRunning || isTransitioning ? (
                  <Button
                    variant="danger"
                    busy={store.isBusy("overview:stop")}
                    onClick={() => void handleRuntimeAction("stop")}
                  >
                    {isTransitioning
                      ? en
                        ? "Abort loading"
                        : "中止加载并停止"
                      : en
                        ? "Stop"
                        : "停止运行"}
                  </Button>
                ) : null}
                {isRunning ? (
                  <Button
                    busy={store.isBusy("overview:restart")}
                    onClick={() => void handleRuntimeAction("restart")}
                  >
                    {en ? "Restart" : "重启"}
                  </Button>
                ) : null}
                <Button
                  busy={store.isBusy("overview:test")}
                  disabled={!hasEngine || !hasModel}
                  onClick={handleQuickTest}
                >
                  {en ? "Test translation" : "测试翻译推理"}
                </Button>
                {(isFailed || isCooldown) &&
                data.selected_backend !== "cpu" ? (
                  <Button
                    busy={store.isBusy("overview:fallback-cpu")}
                    onClick={() => void handleRuntimeAction("fallback-cpu")}
                  >
                    {en ? "Fallback to CPU" : "改用 CPU 运行"}
                  </Button>
                ) : null}
              </>
            )}
            <Button onClick={() => onNavigate("translate")}>
              {en ? "Open Desktop Translator" : "打开粘贴与截图翻译"}
            </Button>
            {isRunning ? (
              <Button
                variant="ghost"
                onClick={async () => {
                  if (isDesktopMode) {
                    await nativeCommand("open-ui", "");
                  } else {
                    window.open("/ui", "_blank", "noopener,noreferrer");
                  }
                }}
              >
                {en ? "Open mistral.rs WebUI ↗" : "打开官方聊天界面 ↗"}
              </Button>
            ) : null}
          </>
        }
      />

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
                  ? `(Auto-cooldown until ${new Date(st.cooldown_until).toLocaleTimeString()})`
                  : `（冷却保护至 ${new Date(st.cooldown_until).toLocaleTimeString()}，也可点击上方立即重试或改用 CPU）`}
              </span>
            ) : null}
          </div>
          {st.detail || st.last_failure?.log_tail ? (
            <details>
              <summary>{en ? "Engine log output" : "展开引擎错误输出"}</summary>
              <pre>{st.last_failure?.log_tail || st.detail}</pre>
            </details>
          ) : null}
        </div>
      ) : null}

      {/* Quick Setup Guide when fresh */}
      {setupStep < 4 ? (
        <fieldset>
          <legend>{en ? "Quick Setup Checklist" : "快速上手步骤"}</legend>
          <div class="grid-2">
            <div class="card" style={{ margin: 0 }}>
              <strong>
                {hasEngine ? "✓ " : "1. "}
                {en ? "Install mistral.rs Runtime" : "安装推理运行时引擎"}
              </strong>
              <p class="hint">
                {hasEngine
                  ? en
                    ? `Installed (${data.settings.engine_version || "ready"})`
                    : `已就绪（${data.settings.engine_version || "已安装"}）`
                  : en
                    ? "Auto-matches CUDA, Metal, or CPU for this computer."
                    : "根据显卡驱动自动推荐 CUDA、Metal 或 CPU 预编译包。"}
              </p>
            </div>
            <div class="card" style={{ margin: 0 }}>
              <strong>
                {hasModel ? "✓ " : "2. "}
                {en ? "Download Translation Weights" : "下载轻量翻译模型权重"}
              </strong>
              <p class="hint">
                {hasModel
                  ? en
                    ? `Active: ${data.settings.model}`
                    : `当前已选：${data.settings.model}`
                  : en
                    ? "HY-MT1.5-1.8B Q4_K_M (~1.1 GiB) runs smoothly on CPU or GPU."
                    : "推荐 HY-MT1.5-1.8B Q4_K_M（约 1.1 GiB），无独显也能流畅运行。"}
              </p>
            </div>
          </div>
        </fieldset>
      ) : null}

      {/* M-VD: One-click Qingyi Extension Connection Card */}
      <fieldset>
        <legend>
          {en
            ? "Connect Qingyi Browser Extension"
            : "一键连接「轻译 Qingyi」浏览器扩展"}
        </legend>
        <p class="hint">
          {en
            ? "Copy the connection JSON below and click 'Paste mistral-gui config' in Qingyi Settings → Translation Service."
            : "点击下方「一键复制轻译连接配置」，然后在浏览器轻译设置页「翻译服务」中点击「一键粘贴 mistral-gui 配置」即可自动填好地址、Token 与模型。"}
        </p>
        <div class="grid-2">
          <CopyField
            label={en ? "Local Manager URL" : "本地管理器地址"}
            value={data.endpoint}
            lang={uiLang}
          />
          <CopyField
            label={en ? "Default Model ID" : "当前模型标识"}
            value={data.settings.model}
            lang={uiLang}
          />
        </div>
        <div class="actions">
          <Button
            variant="primary"
            busy={store.isBusy("overview:copy-qy")}
            onClick={handleCopyQingyiJson}
          >
            {en
              ? "Copy Qingyi connection JSON (incl. Token)"
              : "一键复制轻译连接配置 JSON（含 Token）"}
          </Button>
          <Button onClick={() => onNavigate("connection")}>
            {en ? "Manage Token & API examples" : "查看与轮换 Token / API 示例"}
          </Button>
        </div>
        {data.dictionary_sync.revisions > 0 ||
        data.dictionary_sync.pack_entries > 0 ? (
          <p class="hint" style={{ marginTop: "10px" }}>
            {en
              ? `Synced from Qingyi: ${data.dictionary_sync.revisions} user revision(s), ${data.dictionary_sync.pack_entries} dictionary entry(ies).`
              : `已接收轻译同步词库：人工修订 ${data.dictionary_sync.revisions} 条 · 词库词条 ${data.dictionary_sync.pack_entries} 条（桌面翻译自动优先命中）。`}
          </p>
        ) : null}
        {data.temp_storage.bytes > 0 ? (
          <p class="hint" style={{ marginTop: "6px" }}>
            {en
              ? `Resumable download cache (.part/.staging): ${formatBytes(data.temp_storage.bytes, uiLang)}`
              : `当前存在未完成下载暂存（${formatBytes(data.temp_storage.bytes, uiLang)}），可在「更新与设置」中按需清理。`}
          </p>
        ) : null}
      </fieldset>
    </section>
  );
}
