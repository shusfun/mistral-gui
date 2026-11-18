import { useEffect, useRef, useState } from "preact/hooks";
import {
  Badge,
  Button,
  CopyField,
  DataList,
  Disclosure,
  FeedbackBanner,
  Select,
  Switch,
  TextInput,
  VerdictHero,
  confirmAction,
  createRequestGate,
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
  formatStep,
  toUiLang,
  type ManagerLocale,
} from "../i18n";
import type {
  CapabilitiesResponse,
  CatalogAsset,
  DeploymentView,
  OperationItem,
  RecommendationResponse,
  StatusResponse,
} from "../types";

export function DeploymentsPage({
  data,
  subPath,
  installedModels,
  locale,
  onRefresh,
  onNavigateHash,
}: {
  data: StatusResponse;
  subPath: string; // "" | "new" | "<deploymentId>"
  installedModels: CatalogAsset[];
  locale: ManagerLocale;
  onRefresh: () => Promise<void>;
  onNavigateHash: (hash: string) => void;
}) {
  const store = useFeedbackStore();
  const uiLang = toUiLang(locale);
  const en = uiLang === "en";

  // List filter & search state (M-38, M-42)
  const [searchQuery, setSearchQuery] = useState("");
  const [statusFilter, setStatusFilter] = useState("all");

  // Per-deployment test result map & log drawer state (M-03, M-16, M-R3)
  const [testResults, setTestResults] = useState<
    Record<string, { latencyMs: number; sample: string }>
  >({});
  const [engineLogs, setEngineLogs] = useState<Record<string, string>>({});

  // Detail / Editor form state
  const isCreateMode = subPath === "new";
  const editingDep = !isCreateMode && subPath
    ? data.deployments.find((d) => d.id === subPath) || null
    : null;

  const [formId, setFormId] = useState("");
  const [formModelKey, setFormModelKey] = useState(""); // "<id>@<version>"
  const [formBackend, setFormBackend] = useState("auto");
  const [formAutoStart, setFormAutoStart] = useState(true);
  const [formIdleMinutes, setFormIdleMinutes] = useState(10);
  const [formRecommend, setFormRecommend] = useState(true);
  const [formContext, setFormContext] = useState(4096);
  const [formSequences, setFormSequences] = useState(1);
  const [formGpuLayers, setFormGpuLayers] = useState(0);
  const [formGpuUtil, setFormGpuUtil] = useState(0);
  const [formKvCache, setFormKvCache] = useState("");
  const [formIsq, setFormIsq] = useState("");
  const [formBatchTokens, setFormBatchTokens] = useState(0);
  const [capabilities, setCapabilities] = useState<CapabilitiesResponse | null>(
    null,
  );
  const [recommendation, setRecommendation] =
    useState<RecommendationResponse | null>(null);

  // M-40: Monotonic request gates so rapid backend/model switches never apply stale responses!
  const capGateRef = useRef(createRequestGate());
  const recGateRef = useRef(createRequestGate());

  const loadCapabilities = async (backend: string) => {
    const ticket = capGateRef.current.next();
    try {
      const res = await api<CapabilitiesResponse>(
        `/api/v1/capabilities?backend=${encodeURIComponent(backend)}`,
        undefined,
        { signal: ticket.signal },
      );
      if (ticket.isCurrent()) {
        setCapabilities(res);
      }
    } catch {
      // ignore aborted or transient capability check
    }
  };

  // Sync editor form when navigating into "new" or "<id>" (M-VA: preselect matching installed revision even when d.model_version was empty)
  useEffect(() => {
    if (!isCreateMode && !editingDep) return;
    if (isCreateMode) {
      const firstModel = installedModels[0];
      setFormId("");
      setFormModelKey(
        firstModel ? `${firstModel.id}@${firstModel.version}` : "",
      );
      setFormBackend(data.settings.backend || "auto");
      setFormAutoStart(true);
      setFormIdleMinutes(10);
      setFormRecommend(true);
      setFormContext(4096);
      setFormSequences(1);
      setFormGpuLayers(0);
      setFormGpuUtil(0);
      setFormKvCache("");
      setFormIsq("");
      setFormBatchTokens(0);
      setRecommendation(null);
      void loadCapabilities(data.settings.backend || "auto");
      return;
    }
    if (editingDep) {
      setFormId(editingDep.id);
      const matchedInstalled =
        installedModels.find(
          (m) =>
            m.id === editingDep.model &&
            (!editingDep.model_version || m.version === editingDep.model_version),
        ) || null;
      setFormModelKey(
        matchedInstalled
          ? `${matchedInstalled.id}@${matchedInstalled.version}`
          : editingDep.model
            ? `${editingDep.model}@${editingDep.model_version || ""}`
            : "",
      );
      setFormBackend(editingDep.backend || "auto");
      setFormAutoStart(editingDep.auto_start);
      setFormIdleMinutes(editingDep.idle_minutes ?? 10);
      setFormRecommend(false);
      setFormContext(editingDep.tuning?.context || 0);
      setFormSequences(editingDep.tuning?.sequences || 0);
      setFormGpuLayers(editingDep.tuning?.gpu_layers || 0);
      setFormGpuUtil(editingDep.tuning?.gpu_utilization || 0);
      setFormKvCache(editingDep.tuning?.kv_cache || "");
      setFormIsq(editingDep.tuning?.isq || "");
      setFormBatchTokens(editingDep.tuning?.batch_tokens || 0);
      setRecommendation(null);
      void loadCapabilities(editingDep.backend || "auto");
    }
  }, [subPath, editingDep?.id, installedModels.length]);

  const activePrepareOpFor = (depId: string): OperationItem | undefined =>
    (data.operations || []).find(
      (op) =>
        op.state === "running" &&
        (op.deployment_id === depId ||
          (depId === "default" && !op.deployment_id && op.kind === "prepare")),
    );

  // M-13: Real-time deployment runtime state ALWAYS takes precedence over historical failed/completed tasks; only running prepare task overlays step!
  const effectiveStatusBadge = (d: DeploymentView) => {
    const activeOp = activePrepareOpFor(d.id);
    const st = d.status.state;
    if (st === "running") {
      return { tone: "success" as const, text: formatRuntimeState(locale, "running") };
    }
    if (st === "starting" || st === "loading") {
      return {
        tone: "info" as const,
        text: formatStage(locale, d.status.stage) || formatRuntimeState(locale, st),
      };
    }
    if (activeOp) {
      return {
        tone: "info" as const,
        text: formatStep(locale, activeOp.step) || formatRuntimeState(locale, "loading"),
      };
    }
    if (st === "failed" || st === "cooldown") {
      return { tone: "error" as const, text: formatRuntimeState(locale, st) };
    }
    if (st === "stopping") {
      return { tone: "warning" as const, text: formatRuntimeState(locale, st) };
    }
    return { tone: "neutral" as const, text: formatRuntimeState(locale, st || "stopped") };
  };

  const handleDeploymentRuntime = (
    d: DeploymentView,
    action: "start" | "stop" | "restart" | "fallback-cpu",
  ) =>
    runTracked(
      { key: `dep:${d.id}:${action}`, scope: "deployments", lang: uiLang },
      async () => {
        const q = d.default ? "" : `?model=${encodeURIComponent(d.id)}`;
        await api(`/api/v1/runtime/${action}${q}`, {});
        if (action === "start" || action === "restart") {
          // Clear stale test sample on state transition (M-16)
          setTestResults((prev) => {
            const copy = { ...prev };
            delete copy[d.id];
            return copy;
          });
        }
        await onRefresh();
      },
    );

  const handleTestDeployment = (d: DeploymentView) =>
    runTracked(
      { key: `dep:${d.id}:test`, scope: "deployments", lang: uiLang },
      async () => {
        const started = performance.now();
        const res = await api<{ segments: { translation?: string }[] }>(
          "/api/v1/translate",
          {
            text: "Fast local translation.",
            routing: "local-only",
            model: d.default ? "" : d.id,
            target_lang: "zh-CN",
          },
        );
        const latencyMs = Math.round(performance.now() - started);
        const sample = res.segments?.[0]?.translation || "OK";
        setTestResults((prev) => ({
          ...prev,
          [d.id]: { latencyMs, sample },
        }));
        toast(
          en
            ? `${d.id} responded in ${latencyMs} ms: ${sample}`
            : `实例 ${d.id} 推理通过（${latencyMs} ms）：${sample}`,
          "success",
        );
        await onRefresh();
      },
    );

  // M-02 & M-26: Accurate confirmation clarifying model weights stay intact, and guard running state!
  const handleDeleteDeployment = async (d: DeploymentView) => {
    if (d.default) return;
    if (
      d.status.state === "running" ||
      d.status.state === "starting" ||
      d.status.state === "loading"
    ) {
      feedbackStore.reportError(
        "deployments",
        new Error("stop_before_switch"),
        uiLang,
      );
      return;
    }
    const ok = await confirmAction({
      title: en ? "Delete Deployment Configuration" : "删除部署实例配置",
      message: en
        ? `Remove deployment "${d.id}"? Downloaded model weights (${d.model}) and runtime binaries will NOT be deleted and remain available in Models & Runtime.`
        : `确定移除部署实例「${d.id}」吗？此操作仅删除该部署参数配置，已下载的模型权重（${d.model}）与推理运行时文件均会完整保留在「模型与运行时」库中。`,
      danger: true,
      lang: uiLang,
    });
    if (!ok) return;

    await runTracked(
      { key: `dep:${d.id}:delete`, scope: "deployments", lang: uiLang },
      async () => {
        await api("/api/v1/deployments/delete", { id: d.id });
        await onRefresh();
        if (subPath === d.id) {
          onNavigateHash("#instances");
        }
        toast(
          en ? `Removed deployment ${d.id}.` : `已移除部署配置 ${d.id}。`,
          "success",
        );
      },
    );
  };

  const handleRollbackDeployment = async (d: DeploymentView) => {
    if (!d.previous_model) return;
    const ok = await confirmAction({
      message: en
        ? `Roll deployment "${d.id}" back to model "${d.previous_model}"?`
        : `确定将实例「${d.id}」切回上一模型「${d.previous_model}」吗？`,
      lang: uiLang,
    });
    if (!ok) return;
    await runTracked(
      { key: `dep:${d.id}:rollback`, scope: "deployments", lang: uiLang },
      async () => {
        if (d.default) {
          await api("/api/v1/models/rollback", {});
        } else {
          await api("/api/v1/deployments/rollback", { id: d.id });
        }
        await onRefresh();
        toast(
          en ? "Rolled back to previous model." : "已切回上一模型配置。",
          "success",
        );
      },
    );
  };

  const handleLoadEngineLogs = (d: DeploymentView) =>
    runTracked(
      { key: `dep:${d.id}:logs`, scope: "deployments", lang: uiLang },
      async () => {
        const q = d.default ? "" : `?id=${encodeURIComponent(d.id)}`;
        const res = await api<{ logs: string }>(`/api/v1/deployments/logs${q}`);
        setEngineLogs((prev) => ({
          ...prev,
          [d.id]: res.logs || (en ? "(No engine output yet)" : "（暂无引擎运行输出）"),
        }));
      },
    );

  const handleRecommend = () =>
    runTracked(
      { key: "dep-form:recommend", scope: "dep-editor", lang: uiLang },
      async () => {
        const [mId, mVer] = formModelKey.split("@");
        if (!mId) throw new Error("chooseModel");
        const ticket = recGateRef.current.next();
        const rec = await api<RecommendationResponse>(
          `/api/v1/recommend?model=${encodeURIComponent(mId)}&version=${encodeURIComponent(mVer || "")}`,
          undefined,
          { signal: ticket.signal },
        );
        if (!ticket.isCurrent()) return;
        setRecommendation(rec);
        setFormBackend(rec.backend);
        await loadCapabilities(rec.backend);
        if (rec.tuning.context) setFormContext(rec.tuning.context);
        if (rec.tuning.sequences) setFormSequences(rec.tuning.sequences);
        if (rec.tuning.isq !== undefined) setFormIsq(rec.tuning.isq);
        toast(
          en
            ? `Applied recommended ${rec.backend.toUpperCase()} configuration.`
            : `已填入针对本机硬件推荐的 ${rec.backend.toUpperCase()} 参数。`,
          "success",
        );
      },
    );

  // M-41 & M-VE: Explicit client-side validation + single atomic prepare/save flow
  const handleSaveDeployment = (deployAndStart: boolean) =>
    runTracked(
      {
        key: deployAndStart ? "dep-form:prepare" : "dep-form:save",
        scope: "dep-editor",
        lang: uiLang,
      },
      async () => {
        const isDefault = editingDep?.default === true;
        const cleanId = isDefault ? "default" : formId.trim();
        if (!isDefault) {
          if (
            !cleanId ||
            cleanId === "default" ||
            cleanId === "new" ||
            !/^[A-Za-z0-9][A-Za-z0-9._-]{0,95}$/.test(cleanId)
          ) {
            throw new Error(
              cleanId === "default" || cleanId === "new"
                ? "deployment_id_reserved"
                : "invalid_deployment_id",
            );
          }
        }
        const [model, model_version] = formModelKey.split("@");
        if (!model) throw new Error("chooseModel");

        if (formKvCache.trim() && !/^\d+(?:MiB|GiB|tokens)$/.test(formKvCache.trim())) {
          throw new Error("invalid_kv_cache");
        }
        if (formIdleMinutes < 0 || formIdleMinutes > 1440) {
          throw new Error("invalid_idle_minutes");
        }

        const existingRunning =
          editingDep &&
          (editingDep.status.state === "running" ||
            editingDep.status.state === "loading");
        if (existingRunning && deployAndStart) {
          const ok = await confirmAction({
            message: en
              ? `Deployment "${cleanId}" is currently running. Restart it with the new configuration?`
              : `部署实例「${cleanId}」正在运行，是否重启并应用新配置？`,
            lang: uiLang,
          });
          if (!ok) return;
        }

        const tuning = {
          context: Number(formContext) || 0,
          sequences: Number(formSequences) || 0,
          gpu_layers: Number(formGpuLayers) || 0,
          gpu_utilization: Number(formGpuUtil) || 0,
          kv_cache: formKvCache.trim(),
          isq: formIsq,
          batch_tokens: Number(formBatchTokens) || 0,
        };

        const matchedAsset = installedModels.find(
          (m) => m.id === model && (!model_version || m.version === model_version),
        );

        if (deployAndStart) {
          await api("/api/v1/deployments/prepare", {
            id: isDefault ? "" : cleanId,
            model,
            model_version: model_version || "",
            repository: matchedAsset?.repository || "",
            backend: formBackend,
            recommend: formRecommend,
            start: true,
            restart: Boolean(existingRunning),
            auto_start: formAutoStart,
            idle_minutes: Number(formIdleMinutes) || 0,
            tuning,
          });
          toast(
            en
              ? `Deployment ${cleanId} preparation & startup queued.`
              : `已提交实例 ${cleanId} 的准备与启动任务。`,
            "success",
          );
        } else if (isDefault) {
          await api("/api/v1/settings", {
            model,
            model_version: model_version || "",
            backend: formBackend,
            auto_start: formAutoStart,
            idle_minutes: Number(formIdleMinutes) || 0,
          });
          toast(en ? "Saved default deployment." : "已保存默认实例配置。", "success");
        } else {
          await api("/api/v1/deployments", {
            id: cleanId,
            model,
            model_version: model_version || "",
            backend: formBackend,
            auto_start: formAutoStart,
            idle_minutes: Number(formIdleMinutes) || 0,
            tuning,
          });
          toast(
            en ? `Saved deployment ${cleanId}.` : `已保存部署实例 ${cleanId}。`,
            "success",
          );
        }
        await onRefresh();
        onNavigateHash("#instances");
      },
    );

  // --- Detail / Create View ---
  if (isCreateMode || editingDep) {
    const isDefault = editingDep?.default === true;
    const isRunningNow =
      editingDep &&
      (editingDep.status.state === "running" ||
        editingDep.status.state === "starting" ||
        editingDep.status.state === "loading");
    const cap = capabilities?.tuning || {};

    return (
      <section class="panel mg-section">
        <div class="split-head" style={{ marginBottom: "14px" }}>
          <div>
            <p class="eyebrow">
              {isCreateMode
                ? en
                  ? "NEW DEPLOYMENT"
                  : "新建独立部署实例"
                : en
                  ? "CONFIGURE DEPLOYMENT"
                  : "配置部署实例"}
            </p>
            <h2 style={{ margin: 0 }}>
              {isCreateMode
                ? en
                  ? "Create Deployment Instance"
                  : "新建模型部署实例"
                : `${editingDep!.id}${isDefault ? (en ? " (Default)" : "（默认实例）") : ""}`}
            </h2>
          </div>
          <Button variant="ghost" onClick={() => onNavigateHash("#instances")}>
            {en ? "← Back to Deployments" : "← 返回部署列表"}
          </Button>
        </div>

        <FeedbackBanner scope="dep-editor" lang={uiLang} />

        {isRunningNow ? (
          <div class="feedback-banner" data-tone="info" role="status">
            {en
              ? "This deployment is currently running. Use 'Deploy & Restart' to apply changes seamlessly, or stop it first before saving static settings."
              : "该部署当前正在运行中：点击「部署并重启生效」可平滑重启应用新参数，或先停止实例后再仅保存配置。"}
          </div>
        ) : null}

        <fieldset>
          <legend>{en ? "Identity & Model" : "实例标识与模型选择"}</legend>
          <div class="grid-2">
            <TextInput
              label={en ? "Deployment ID" : "部署实例 ID（供 API / 轻译指定调用）"}
              placeholder="translation-fast"
              disabled={!isCreateMode}
              value={isDefault ? "default" : formId}
              hint={
                isCreateMode
                  ? en
                    ? "Letters, digits, '.', '_', or '-' (cannot be 'default' or 'new')."
                    : "以字母或数字开头，可含 . _ -（不可使用保留名 default 或 new）。"
                  : undefined
              }
              onValueInput={(v) => setFormId(v)}
            />
            <Select
              label={en ? "Installed Model" : "选择已安装模型"}
              value={formModelKey}
              options={
                installedModels.length > 0
                  ? installedModels.map((m) => ({
                      value: `${m.id}@${m.version}`,
                      label: `${m.id} (${formatBytes(m.size, uiLang)} · ${m.version.slice(0, 8)})`,
                    }))
                  : [
                      {
                        value: formModelKey || "",
                        label:
                          formModelKey ||
                          (en
                            ? "No models installed yet — download on Models page"
                            : "尚未安装模型（请先前往模型与运行时页下载）"),
                      },
                    ]
              }
              onValueChange={(v) => setFormModelKey(v)}
            />
          </div>

          <div class="grid-3" style={{ marginTop: "12px" }}>
            <Select
              label={en ? "Acceleration Backend" : "推理加速后端"}
              value={formBackend}
              options={[
                { value: "auto", label: formatBackend(locale, "auto") },
                { value: "cpu", label: formatBackend(locale, "cpu") },
                { value: "cuda", label: formatBackend(locale, "cuda") },
                { value: "metal", label: formatBackend(locale, "metal") },
              ]}
              onValueChange={(v) => {
                setFormBackend(v);
                void loadCapabilities(v);
              }}
            />
            <TextInput
              label={en ? "Idle auto-stop (minutes)" : "空闲自动卸载（分钟，0 为常驻）"}
              type="number"
              min={0}
              max={1440}
              value={formIdleMinutes}
              onValueInput={(v) => setFormIdleMinutes(Number(v) || 0)}
            />
            <div style={{ display: "flex", alignItems: "end" }}>
              <Button
                busy={store.isBusy("dep-form:recommend")}
                disabled={!formModelKey}
                onClick={handleRecommend}
              >
                {en ? "Auto-recommend for hardware" : "按本机硬件自动推荐参数"}
              </Button>
            </div>
          </div>

          <div style={{ marginTop: "10px" }}>
            <Switch
              checked={formAutoStart}
              onChange={(c) => setFormAutoStart(c)}
              label={
                en
                  ? "Auto-start on incoming translation / API request"
                  : "收到轻译或 API 请求时自动按需唤醒此实例"
              }
            />
            <Switch
              checked={formRecommend}
              onChange={(c) => setFormRecommend(c)}
              label={
                en
                  ? "Apply hardware memory guardrails during Deploy & Start"
                  : "部署并启动时自动根据可用显存/内存校准参数"
              }
            />
          </div>

          {recommendation ? (
            <div class="card">
              <strong>
                {en
                  ? `Recommended: ${recommendation.backend.toUpperCase()} · Est. runtime footprint ${formatBytes(recommendation.estimated_bytes, uiLang)}`
                  : `硬件推荐：${recommendation.backend.toUpperCase()} · 预计常驻占用 ${formatBytes(recommendation.estimated_bytes, uiLang)}（加载峰值 ${formatBytes(recommendation.estimated_loading_bytes, uiLang)}）`}
              </strong>
              {recommendation.warnings.length > 0 ? (
                <p class="hint">
                  {recommendation.warnings
                    .map((w) => resolveError(w, uiLang).message)
                    .join(" ")}
                </p>
              ) : null}
            </div>
          ) : null}
        </fieldset>

        {!isDefault ? (
          <Disclosure
            summary={
              en
                ? "Advanced Inference Tuning (Context, KV Cache, ISQ)"
                : "高级推理调优（上下文长度、KV Cache、加载时量化 ISQ）"
            }
          >
            <div class="grid-3" style={{ marginTop: "12px" }}>
              <TextInput
                label={en ? "Max Context Tokens" : "最大上下文长度（0 跟随引擎）"}
                type="number"
                min={0}
                max={1048576}
                value={formContext}
                onValueInput={(v) => setFormContext(Number(v) || 0)}
              />
              <TextInput
                label={en ? "Max Concurrent Sequences" : "最大并发序列数"}
                type="number"
                min={0}
                max={4096}
                value={formSequences}
                onValueInput={(v) => setFormSequences(Number(v) || 0)}
              />
              <Select
                label={en ? "In-Situ Quantization (ISQ)" : "safetensors 加载量化（ISQ）"}
                value={formIsq}
                options={[
                  { value: "", label: en ? "None (GGUF native)" : "不启用（GGUF 原生量化）" },
                  { value: "Q4K", label: "Q4K" },
                  { value: "Q6K", label: "Q6K" },
                  { value: "Q8_0", label: "Q8_0" },
                ]}
                onValueChange={(v) => setFormIsq(v)}
              />
              <TextInput
                label={en ? "GPU Offload Layers" : "GPU 加载层数（0 自动）"}
                type="number"
                min={0}
                disabled={cap.gpu_layers === false || formBackend === "cpu"}
                value={formGpuLayers}
                onValueInput={(v) => setFormGpuLayers(Number(v) || 0)}
              />
              <TextInput
                label={en ? "GPU Memory Utilization (0–1)" : "显存占用上限比例（0–1）"}
                type="number"
                min={0}
                max={1}
                step={0.05}
                disabled={cap.gpu_utilization === false || formBackend === "cpu"}
                value={formGpuUtil}
                onValueInput={(v) => setFormGpuUtil(Number(v) || 0)}
              />
              <TextInput
                label={en ? "KV Cache Limit" : "KV Cache 上限（如 512MiB / 4GiB）"}
                placeholder="512MiB"
                disabled={cap.kv_cache === false || formBackend === "cpu"}
                value={formKvCache}
                onValueInput={(v) => setFormKvCache(v)}
              />
            </div>
          </Disclosure>
        ) : null}

        <div class="actions">
          <Button
            variant="primary"
            busy={store.isBusy("dep-form:prepare")}
            onClick={() => void handleSaveDeployment(true)}
          >
            {isRunningNow
              ? en
                ? "Deploy & Restart"
                : "部署并重启生效"
              : en
                ? "Deploy & Start"
                : "保存并立即启动"}
          </Button>
          <Button
            busy={store.isBusy("dep-form:save")}
            disabled={Boolean(isRunningNow)}
            onClick={() => void handleSaveDeployment(false)}
          >
            {en ? "Save configuration only" : "仅保存配置（不启动）"}
          </Button>
          <Button variant="ghost" onClick={() => onNavigateHash("#instances")}>
            {en ? "Cancel" : "取消"}
          </Button>
        </div>
      </section>
    );
  }

  // --- Deployments List View ---
  // M-38: Scoped search on ID, model, backend ONLY (not hidden JSON/stacktraces!) + complete state filter
  const qLower = searchQuery.trim().toLowerCase();
  const filteredDeployments = data.deployments.filter((d) => {
    if (statusFilter !== "all" && d.status.state !== statusFilter) {
      return false;
    }
    if (!qLower) return true;
    return (
      d.id.toLowerCase().includes(qLower) ||
      (d.model || "").toLowerCase().includes(qLower) ||
      (d.backend || "").toLowerCase().includes(qLower)
    );
  });

  return (
    <section class="panel mg-section">
      {/* M-37: Button label matches exact navigation target ("新建部署实例" -> #instances/new) */}
      <VerdictHero
        eyebrow={en ? "MULTI-MODEL DEPLOYMENTS" : "多模型部署实例管理"}
        title={
          en
            ? `${data.deployments.length} configured deployment(s)`
            : `共配置 ${data.deployments.length} 个模型部署实例`
        }
        description={
          en
            ? "Each deployment binds a model, acceleration backend, and idle policy, addressable by its Deployment ID from Qingyi or OpenAI clients."
            : "每个部署独立绑定模型版本、加速后端与空闲策略；轻译扩展或任意 OpenAI 客户端均可通过部署 ID 直接按需调用。"
        }
        actions={
          <>
            <Button
              variant="primary"
              onClick={() => onNavigateHash("#instances/new")}
            >
              {en ? "+ New Deployment" : "+ 新建部署实例"}
            </Button>
            <Button onClick={() => onNavigateHash("#models")}>
              {en ? "Browse & Download Models" : "前往模型库下载更多模型"}
            </Button>
          </>
        }
      />

      <FeedbackBanner scope="deployments" lang={uiLang} />

      {/* M-42 & M-38: Full-width search bar + complete status filter options */}
      <div class="mg-filter-bar">
        <TextInput
          type="search"
          label={en ? "Search by ID, model, or backend" : "搜索部署 ID、模型名称或后端"}
          placeholder={en ? "Filter deployments…" : "输入部署 ID 或模型关键词筛选…"}
          value={searchQuery}
          onValueInput={(v) => setSearchQuery(v)}
        />
        <Select
          label={en ? "Runtime Status" : "状态筛选"}
          value={statusFilter}
          options={[
            { value: "all", label: en ? "All states" : "全部状态" },
            { value: "running", label: formatRuntimeState(locale, "running") },
            { value: "loading", label: formatRuntimeState(locale, "loading") },
            { value: "starting", label: formatRuntimeState(locale, "starting") },
            { value: "stopped", label: formatRuntimeState(locale, "stopped") },
            { value: "stopping", label: formatRuntimeState(locale, "stopping") },
            { value: "failed", label: formatRuntimeState(locale, "failed") },
            { value: "cooldown", label: formatRuntimeState(locale, "cooldown") },
          ]}
          onValueChange={(v) => setStatusFilter(v)}
        />
      </div>

      <DataList
        items={filteredDeployments}
        getKey={(d) => d.id}
        emptyMessage={
          en
            ? "No deployments match the current filter."
            : "没有匹配当前筛选条件的部署实例。"
        }
        lang={uiLang}
        renderItem={(d) => {
          const badge = effectiveStatusBadge(d);
          const st = d.status.state;
          const isRunning = st === "running";
          const isTransitioning = st === "starting" || st === "loading";
          const isBusyRow =
            store.isBusy(`dep:${d.id}:start`) ||
            store.isBusy(`dep:${d.id}:stop`) ||
            store.isBusy(`dep:${d.id}:restart`) ||
            store.isBusy(`dep:${d.id}:delete`) ||
            store.isBusy(`dep:${d.id}:rollback`);
          const sample = testResults[d.id];

          return (
            <div style={{ width: "100%" }}>
              <div class="split-head">
                <div>
                  <div style={{ display: "flex", gap: "8px", alignItems: "center", flexWrap: "wrap" }}>
                    <strong>{d.id}</strong>
                    {d.default ? (
                      <Badge tone="neutral">{en ? "Default" : "默认"}</Badge>
                    ) : null}
                    <Badge tone={badge.tone}>{badge.text}</Badge>
                  </div>
                  <p class="hint" style={{ margin: "4px 0 0" }}>
                    {[
                      d.model || "—",
                      formatBackend(locale, d.status.backend || d.backend),
                      d.auto_start
                        ? en
                          ? "Auto-start"
                          : "按需自启"
                        : en
                          ? "Manual start"
                          : "手动启动",
                      d.idle_minutes > 0
                        ? en
                          ? `Idle ${d.idle_minutes}m`
                          : `空闲 ${d.idle_minutes} 分钟释放`
                        : en
                          ? "Keep warm"
                          : "常驻不卸载",
                      `${en ? "Requests" : "请求"}: ${d.status.requests ?? 0}`,
                    ].join(" · ")}
                  </p>
                </div>

                <div class="actions" style={{ marginTop: 0 }}>
                  {!isRunning && !isTransitioning ? (
                    <Button
                      variant="primary"
                      busy={store.isBusy(`dep:${d.id}:start`)}
                      disabled={isBusyRow}
                      onClick={() => void handleDeploymentRuntime(d, "start")}
                    >
                      {st === "cooldown"
                        ? en
                          ? "Retry now"
                          : "立即重试"
                        : en
                          ? "Start"
                          : "启动"}
                    </Button>
                  ) : null}
                  {/* M-14: Stop button available during starting, loading, AND running! */}
                  {isRunning || isTransitioning ? (
                    <Button
                      variant="danger"
                      busy={store.isBusy(`dep:${d.id}:stop`)}
                      onClick={() => void handleDeploymentRuntime(d, "stop")}
                    >
                      {isTransitioning
                        ? en
                          ? "Abort load"
                          : "中止加载"
                        : en
                          ? "Stop"
                          : "停止"}
                    </Button>
                  ) : null}
                  {isRunning ? (
                    <Button
                      busy={store.isBusy(`dep:${d.id}:restart`)}
                      disabled={isBusyRow}
                      onClick={() => void handleDeploymentRuntime(d, "restart")}
                    >
                      {en ? "Restart" : "重启"}
                    </Button>
                  ) : null}
                  <Button
                    busy={store.isBusy(`dep:${d.id}:test`)}
                    onClick={() => void handleTestDeployment(d)}
                  >
                    {en ? "Test" : "测试"}
                  </Button>
                  <Button
                    onClick={() =>
                      onNavigateHash(`#instances/${encodeURIComponent(d.id)}`)
                    }
                  >
                    {en ? "Configure" : "配置"}
                  </Button>
                  {d.previous_model ? (
                    <Button
                      busy={store.isBusy(`dep:${d.id}:rollback`)}
                      disabled={isRunning || isTransitioning || isBusyRow}
                      title={
                        isRunning || isTransitioning
                          ? resolveError("stop_before_switch", uiLang).message
                          : undefined
                      }
                      onClick={() => void handleRollbackDeployment(d)}
                    >
                      {en ? "Roll back model" : "切回上一模型"}
                    </Button>
                  ) : null}
                  {!d.default ? (
                    <Button
                      variant="danger"
                      busy={store.isBusy(`dep:${d.id}:delete`)}
                      disabled={isRunning || isTransitioning || isBusyRow}
                      title={
                        isRunning || isTransitioning
                          ? resolveError("stop_before_switch", uiLang).message
                          : undefined
                      }
                      onClick={() => void handleDeleteDeployment(d)}
                    >
                      {en ? "Delete" : "移除"}
                    </Button>
                  ) : null}
                </div>
              </div>

              {/* M-16: Current runtime error ALWAYS beats stale historical test sample! */}
              {d.status.error ? (
                <div
                  class="feedback-banner"
                  data-tone="error"
                  style={{ marginTop: "10px", marginBottom: 0 }}
                >
                  <div>
                    <span>{resolveError(d.status.error, uiLang).message}</span>
                    {d.backend !== "cpu" ? (
                      <>
                        {" "}
                        <Button
                          variant="ghost"
                          busy={store.isBusy(`dep:${d.id}:fallback-cpu`)}
                          onClick={() =>
                            void handleDeploymentRuntime(d, "fallback-cpu")
                          }
                        >
                          {en ? "Fallback to CPU" : "改用 CPU"}
                        </Button>
                      </>
                    ) : null}
                  </div>
                  {d.status.last_failure?.log_tail || d.status.detail ? (
                    <details>
                      <summary>{en ? "Crash log tail" : "展开最后日志输出"}</summary>
                      <pre>
                        {d.status.last_failure?.log_tail || d.status.detail}
                      </pre>
                    </details>
                  ) : null}
                </div>
              ) : sample ? (
                <p class="hint" style={{ marginTop: "8px", color: "var(--accent)" }}>
                  {en
                    ? `✓ Verified (${sample.latencyMs} ms): "${sample.sample}"`
                    : `✓ 推理测试通过（${sample.latencyMs} ms）：「${sample.sample}」`}
                </p>
              ) : null}

              {/* M-03 & M-R3: Stable Disclosure per deployment card for Qingyi ID copy & live ring-buffer logs */}
              <Disclosure
                summary={
                  en
                    ? "Connection ID, WebUI & Engine Logs"
                    : "展开调用标识、WebUI 入口与引擎运行日志"
                }
              >
                <div style={{ marginTop: "10px", display: "grid", gap: "10px" }}>
                  <CopyField
                    label={
                      en
                        ? "Qingyi Local Model / Deployment ID"
                        : "填入轻译「本地模型」字段的标识"
                    }
                    value={d.default ? d.model : d.id}
                    lang={uiLang}
                  />
                  <div class="actions" style={{ marginTop: 0 }}>
                    <Button
                      busy={store.isBusy(`dep:${d.id}:logs`)}
                      onClick={() => void handleLoadEngineLogs(d)}
                    >
                      {en ? "Refresh engine logs" : "读取引擎最近日志（64 KiB 环形缓冲）"}
                    </Button>
                    {isRunning ? (
                      <Button
                        variant="ghost"
                        onClick={async () => {
                          const target = d.default ? "" : d.id;
                          if (isDesktopMode) {
                            await nativeCommand("open-ui", target);
                          } else {
                            window.open(
                              target ? `/ui?instance=${encodeURIComponent(target)}` : "/ui",
                              "_blank",
                              "noopener,noreferrer",
                            );
                          }
                        }}
                      >
                        {en ? "Open WebUI ↗" : "打开此实例 WebUI ↗"}
                      </Button>
                    ) : null}
                  </div>
                  {engineLogs[d.id] ? (
                    <pre style={{ maxHeight: "220px", overflow: "auto", margin: 0 }}>
                      {engineLogs[d.id]}
                    </pre>
                  ) : null}
                </div>
              </Disclosure>
            </div>
          );
        }}
      />
    </section>
  );
}
