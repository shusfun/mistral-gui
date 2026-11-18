import { useEffect, useState } from "preact/hooks";
import {
  Badge,
  Button,
  DataList,
  FeedbackBanner,
  Select,
  StateBlock,
  TextInput,
  VerdictHero,
  confirmAction,
  feedbackStore,
  formatBytes,
  resolveError,
  runTracked,
  toast,
  useFeedbackStore,
} from "../shared/design-system";
import { api, isDesktopMode, nativeCommand } from "../api";
import {
  formatAssetSource,
  formatBackend,
  t,
  toUiLang,
  type ManagerLocale,
} from "../i18n";
import type {
  CatalogAsset,
  DeviceInfoResponse,
  RecommendationResponse,
  StatusResponse,
} from "../types";

export function ModelsPage({
  data,
  installedModels,
  deviceInfo,
  locale,
  onRefresh,
}: {
  data: StatusResponse;
  installedModels: CatalogAsset[];
  deviceInfo: DeviceInfoResponse | null;
  locale: ManagerLocale;
  onRefresh: () => Promise<void>;
}) {
  const store = useFeedbackStore();
  const uiLang = toUiLang(locale);
  const en = uiLang === "en";

  const [selectedBackend, setSelectedBackend] = useState<string>(
    data.settings.backend || "auto",
  );
  const [repositories, setRepositories] = useState<
    { id: string; name?: string; description?: string; format?: string }[]
  >([]);
  const [selectedRepo, setSelectedRepo] = useState("");
  const [customRepo, setCustomRepo] = useState("");
  const [engineAsset, setEngineAsset] = useState<CatalogAsset | null>(null);
  const [catalogModels, setCatalogModels] = useState<CatalogAsset[]>([]);
  const [assetErrors, setAssetErrors] = useState<Record<string, string>>({});
  const [loadingAssets, setLoadingAssets] = useState(false);
  const [recMap, setRecMap] = useState<Record<string, RecommendationResponse>>(
    {},
  );

  const activeRepo = customRepo.trim() || selectedRepo;

  // M-01 & M-25: Pass selectedBackend directly in query so user doesn't have to click Save Backend first; surface structured error + Retry on failure!
  const fetchAssets = async (refresh = false, backendArg = selectedBackend, repoArg = activeRepo) => {
    setLoadingAssets(true);
    try {
      const params = new URLSearchParams();
      if (backendArg) params.set("backend", backendArg);
      if (repoArg) params.set("repository", repoArg);
      if (refresh) params.set("refresh", "1");

      const res = await api<{
        engine?: CatalogAsset;
        models?: CatalogAsset[];
        errors?: Record<string, string>;
      }>(`/api/v1/assets?${params.toString()}`);

      setEngineAsset(res.engine || null);
      setCatalogModels(res.models || []);
      setAssetErrors(res.errors || {});
      if (refresh) {
        toast(
          en ? "Refreshed upstream asset list." : "已刷新上游可用运行时与模型列表。",
          "success",
        );
      }
    } catch (err) {
      const resolved = resolveError(err, uiLang);
      setAssetErrors({ models: resolved.code || "networkError" });
    } finally {
      setLoadingAssets(false);
    }
  };

  useEffect(() => {
    void (async () => {
      try {
        const cat = await api<{
          repositories: {
            id: string;
            name?: string;
            description?: string;
            format?: string;
          }[];
        }>("/api/v1/catalog");
        const repos = cat.repositories || [];
        setRepositories(repos);
        const first = repos[0]?.id || "";
        setSelectedRepo(first);
        await fetchAssets(false, selectedBackend, first);
      } catch {
        await fetchAssets(false, selectedBackend, "");
      }
    })();
  }, []);

  // Compute hardware fit badge ("suitable" | "tight" | "unsuitable") from device available memory / GPU VRAM (Prompt Item 13)
  const evaluateFit = (asset: CatalogAsset) => {
    let budgetBytes = deviceInfo?.available_bytes || deviceInfo?.memory_bytes || 0;
    if (deviceInfo?.gpu_details && deviceInfo.gpu_details.length > 0) {
      let maxVram = 0;
      for (const g of deviceInfo.gpu_details) {
        if (g.total_bytes > maxVram) maxVram = g.total_bytes;
      }
      if (maxVram > budgetBytes) budgetBytes = maxVram;
    }
    if (!budgetBytes) return null;
    const baseWeight =
      asset.format === "safetensors" ? asset.size / 3 : asset.size;
    const estimated = baseWeight * 1.25 + 512 * 1024 * 1024;
    if (estimated <= budgetBytes * 0.78) {
      return { tone: "success" as const, label: t(locale, "suitable"), estimated };
    }
    if (estimated <= budgetBytes * 1.05) {
      return { tone: "warning" as const, label: t(locale, "tight"), estimated };
    }
    return { tone: "error" as const, label: t(locale, "unsuitable"), estimated };
  };

  // M-01 & M-45: Install engine sends selectedBackend directly and provides immediate banner + toast feedback!
  const handleInstallEngine = () =>
    runTracked(
      { key: "models:install-engine", scope: "models", lang: uiLang },
      async () => {
        await api("/api/v1/assets/install", {
          kind: "engine",
          id: engineAsset?.id || "",
          backend: selectedBackend,
        });
        await onRefresh();
        feedbackStore.setBanner("models", {
          tone: "info",
          message: en
            ? `Downloading ${selectedBackend.toUpperCase()} runtime in background (track progress in Activity)…`
            : `已开始下载 ${selectedBackend.toUpperCase()} 推理运行时（可在「任务记录」查看实时进度）…`,
        });
      },
    );

  const handleSaveBackend = () =>
    runTracked(
      { key: "models:save-backend", scope: "models", lang: uiLang },
      async () => {
        await api("/api/v1/settings", { backend: selectedBackend });
        await onRefresh();
        toast(
          en
            ? `Saved default backend: ${selectedBackend.toUpperCase()}.`
            : `已保存默认加速后端：${formatBackend(locale, selectedBackend)}。`,
          "success",
        );
      },
    );

  const handleInstallModel = (m: CatalogAsset) =>
    runTracked(
      { key: `models:dl:${m.id}`, scope: "models", lang: uiLang },
      async () => {
        await api("/api/v1/assets/install", {
          kind: "model",
          id: m.id,
          repository: activeRepo,
        });
        await onRefresh();
        feedbackStore.setBanner("models", {
          tone: "info",
          message: en
            ? `Queued download for ${m.id} (resumable, verified by SHA-256).`
            : `已加入下载任务：${m.id}（支持断点续传与 SHA-256 完整性校验）。`,
        });
      },
    );

  // M-24: Hardware recommendation works on uninstalled catalog models via size & format query params!
  const handlePreviewRecommend = (m: CatalogAsset) =>
    runTracked(
      { key: `models:rec:${m.id}`, scope: "models", lang: uiLang },
      async () => {
        const q = new URLSearchParams({
          model: m.id,
          version: m.version || "",
          size: String(m.size || 0),
          format: m.format || "gguf",
        });
        const rec = await api<RecommendationResponse>(
          `/api/v1/recommend?${q.toString()}`,
        );
        setRecMap((prev) => ({ ...prev, [m.id]: rec }));
      },
    );

  const handleSelectDefaultModel = (m: CatalogAsset) =>
    runTracked(
      { key: `models:use:${m.id}:${m.version}`, scope: "models", lang: uiLang },
      async () => {
        await api("/api/v1/settings", {
          model: m.id,
          model_version: m.version,
        });
        await onRefresh();
        toast(
          en
            ? `Set ${m.id} as default model.`
            : `已将 ${m.id} 设为默认实例模型。`,
          "success",
        );
      },
    );

  const handleDeleteModel = async (m: CatalogAsset) => {
    const ok = await confirmAction({
      message: en
        ? `Delete downloaded model weights "${m.id}" (${formatBytes(m.size, uiLang)}) from disk?`
        : `确定从磁盘删除已下载的模型权重「${m.id}」（释放约 ${formatBytes(m.size, uiLang)}）吗？`,
      danger: true,
      lang: uiLang,
    });
    if (!ok) return;
    await runTracked(
      { key: `models:del:${m.id}:${m.version}`, scope: "models", lang: uiLang },
      async () => {
        await api("/api/v1/models/delete", {
          id: m.id,
          version: m.version,
        });
        await onRefresh();
        toast(en ? `Deleted ${m.id}.` : `已删除模型 ${m.id}。`, "success");
      },
    );
  };

  const handleImportLocal = (kind: "import-gguf" | "import-directory") =>
    runTracked(
      { key: `models:${kind}`, scope: "models", lang: uiLang },
      async () => {
        await nativeCommand(kind);
        await onRefresh();
        toast(
          en
            ? "Started local model verification & import."
            : "已启动本地模型校验与导入任务。",
          "success",
        );
      },
    );

  return (
    <section class="panel mg-section">
      <VerdictHero
        eyebrow={en ? "RUNTIME ENGINE & MODEL WEIGHTS" : "推理运行时与模型仓库"}
        title={
          en
            ? `${installedModels.length} installed model(s) · Engine ${data.settings.engine ? data.settings.engine_version || "Ready" : "Not installed"}`
            : `已安装 ${installedModels.length} 个本地模型 · 推理引擎 ${data.settings.engine ? data.settings.engine_version || "已就绪" : "尚未安装"}`
        }
        description={
          en
            ? "Runtime binaries and GGUF/safetensors weights are stored separately with SHA-256 verification and resumable downloads."
            : "推理引擎与 GGUF / safetensors 权重分开存放，全部下载均校验 SHA-256 摘要并支持断点续传。"
        }
      />

      <FeedbackBanner scope="models" lang={uiLang} />

      {/* Section 1: mistral.rs Runtime Engine */}
      <fieldset>
        <legend>{en ? "1. mistral.rs Inference Engine" : "1. mistral.rs 推理运行时"}</legend>
        <div class="grid-2">
          <Select
            label={en ? "Target Acceleration Backend" : "选择硬件加速后端"}
            value={selectedBackend}
            options={[
              { value: "auto", label: formatBackend(locale, "auto") },
              { value: "cpu", label: formatBackend(locale, "cpu") },
              { value: "cuda", label: formatBackend(locale, "cuda") },
              { value: "metal", label: formatBackend(locale, "metal") },
            ]}
            onValueChange={(v) => {
              setSelectedBackend(v);
              void fetchAssets(false, v, activeRepo);
            }}
          />
          <div style={{ display: "flex", alignItems: "end", gap: "8px", flexWrap: "wrap" }}>
            <Button
              busy={loadingAssets}
              onClick={() => void fetchAssets(true, selectedBackend, activeRepo)}
            >
              {en ? "Check upstream releases" : "检查可用资产"}
            </Button>
            <Button
              variant="primary"
              busy={store.isBusy("models:install-engine")}
              onClick={handleInstallEngine}
            >
              {data.settings.engine
                ? en
                  ? "Update / Reinstall Runtime"
                  : "更新或重装此后端运行时"
                : en
                  ? "Download Runtime"
                  : "下载并安装运行时"}
            </Button>
            <Button
              busy={store.isBusy("models:save-backend")}
              onClick={handleSaveBackend}
            >
              {en ? "Save as default backend" : "设为默认后端"}
            </Button>
            {data.settings.previous_engine ? (
              <Button
                onClick={() =>
                  void runTracked(
                    { key: "models:rb-engine", scope: "models", lang: uiLang },
                    async () => {
                      await api("/api/v1/runtime/rollback", {});
                      await onRefresh();
                      toast(
                        en ? "Rolled back runtime." : "已切回上一版运行时。",
                        "success",
                      );
                    },
                  )
                }
              >
                {en ? "Roll back runtime" : "切回上一版运行时"}
              </Button>
            ) : null}
          </div>
        </div>

        {assetErrors.engine ? (
          <div class="feedback-banner" data-tone="error" style={{ marginTop: "12px" }}>
            <span>{resolveError(assetErrors.engine, uiLang).message}</span>
          </div>
        ) : engineAsset ? (
          <p class="hint" style={{ marginTop: "10px" }}>
            {en
              ? `Available: ${engineAsset.id} (${engineAsset.version}, ${formatBytes(engineAsset.size, uiLang)}, ${formatAssetSource(locale, engineAsset.source)})`
              : `匹配发行包：${engineAsset.id}（版本 ${engineAsset.version} · ${formatBytes(engineAsset.size, uiLang)} · 来源：${formatAssetSource(locale, engineAsset.source)}）`}
          </p>
        ) : null}
      </fieldset>

      {/* Section 2: Installed Models */}
      <fieldset>
        <legend>{en ? "2. Installed Local Models" : "2. 本机已安装模型"}</legend>
        {isDesktopMode ? (
          <div class="actions" style={{ marginTop: 0, marginBottom: "12px" }}>
            <Button
              busy={store.isBusy("models:import-gguf")}
              onClick={() => void handleImportLocal("import-gguf")}
            >
              {en ? "Import local .gguf file…" : "导入本地 .gguf 文件…"}
            </Button>
            <Button
              busy={store.isBusy("models:import-directory")}
              onClick={() => void handleImportLocal("import-directory")}
            >
              {en ? "Import safetensors folder…" : "导入本地 safetensors 目录…"}
            </Button>
          </div>
        ) : null}

        <DataList
          items={installedModels}
          getKey={(m) => `${m.id}@${m.version}`}
          emptyMessage={
            en
              ? "No models installed yet. Choose a recommended model below to download."
              : "尚未安装本地模型，可在下方「可下载模型目录」一键下载。"
          }
          lang={uiLang}
          renderItem={(m) => {
            const isDefaultSelected =
              data.settings.model === m.id &&
              (!data.settings.model_version ||
                data.settings.model_version === m.version);
            const fit = evaluateFit(m);
            return (
              <>
                <div class="list-row-main">
                  <div style={{ display: "flex", gap: "8px", alignItems: "center", flexWrap: "wrap" }}>
                    <strong>{m.id}</strong>
                    {isDefaultSelected ? (
                      <Badge tone="success">{en ? "Current Default" : "当前默认"}</Badge>
                    ) : null}
                    {fit ? <Badge tone={fit.tone}>{fit.label}</Badge> : null}
                  </div>
                  <p class="hint">
                    {[
                      formatBytes(m.size, uiLang),
                      (m.format || "gguf").toUpperCase(),
                      `${en ? "Rev" : "版本"}: ${m.version.slice(0, 10)}`,
                      formatAssetSource(locale, m.source),
                    ]
                      .filter(Boolean)
                      .join(" · ")}
                  </p>
                </div>
                <div class="actions">
                  {!isDefaultSelected ? (
                    <Button
                      variant="primary"
                      busy={store.isBusy(`models:use:${m.id}:${m.version}`)}
                      onClick={() => void handleSelectDefaultModel(m)}
                    >
                      {en ? "Use as default" : "设为默认"}
                    </Button>
                  ) : null}
                  <Button
                    variant="danger"
                    busy={store.isBusy(`models:del:${m.id}:${m.version}`)}
                    onClick={() => void handleDeleteModel(m)}
                  >
                    {en ? "Delete" : "删除"}
                  </Button>
                </div>
              </>
            );
          }}
        />
      </fieldset>

      {/* Section 3: Curated & Custom Hugging Face Catalog */}
      <fieldset>
        <legend>
          {en
            ? "3. Curated Translation & Multilingual Model Catalog"
            : "3. 精选翻译与多语模型目录（支持自定义 Hugging Face 仓库）"}
        </legend>
        <div class="grid-2" style={{ marginBottom: "12px" }}>
          <Select
            label={en ? "Featured Repository" : "精选模型仓库"}
            value={selectedRepo}
            options={repositories.map((r) => ({
              value: r.id,
              label: r.description ? `${r.id} — ${r.description}` : r.id,
            }))}
            onValueChange={(v) => {
              setSelectedRepo(v);
              setCustomRepo("");
              void fetchAssets(false, selectedBackend, v);
            }}
          />
          <div style={{ display: "flex", gap: "8px", alignItems: "end" }}>
            <div style={{ flex: 1 }}>
              <TextInput
                label={en ? "Or custom Hugging Face repo (owner/repo)" : "或输入自定义仓库（owner/repo）"}
                placeholder="Qwen/Qwen3-4B-GGUF"
                value={customRepo}
                onValueInput={(v) => setCustomRepo(v)}
              />
            </div>
            <Button
              busy={loadingAssets}
              onClick={() => void fetchAssets(true, selectedBackend, activeRepo)}
            >
              {en ? "Load" : "读取列表"}
            </Button>
          </div>
        </div>

        {/* M-25: Explicit failure state with Retry when upstream catalog fetch fails */}
        {assetErrors.models ? (
          <StateBlock
            kind="failed"
            title={en ? "Could not load model catalog" : "未能读取上游模型列表"}
            message={resolveError(assetErrors.models, uiLang).message}
            detail={assetErrors.models}
            onRetry={() => void fetchAssets(true, selectedBackend, activeRepo)}
            lang={uiLang}
          />
        ) : loadingAssets && catalogModels.length === 0 ? (
          <StateBlock
            kind="loading"
            message={
              en
                ? "Fetching repository files & SHA-256 digests…"
                : "正在读取模型仓库文件大小与校验摘要…"
            }
            lang={uiLang}
          />
        ) : (
          <DataList
            items={catalogModels}
            getKey={(m) => m.id}
            emptyMessage={
              en
                ? "No compatible GGUF/safetensors weights found in this repository."
                : "该仓库中未找到受支持的量化或权重文件。"
            }
            lang={uiLang}
            renderItem={(m) => {
              const alreadyInstalled = installedModels.some(
                (inM) => inM.id === m.id && inM.version === m.version,
              );
              const fit = evaluateFit(m);
              const rec = recMap[m.id];

              return (
                <div style={{ width: "100%" }}>
                  <div class="split-head">
                    <div class="list-row-main">
                      <div style={{ display: "flex", gap: "8px", alignItems: "center", flexWrap: "wrap" }}>
                        <strong>{m.id}</strong>
                        {m.recommended ? (
                          <Badge tone="info">{en ? "Recommended" : "推荐"}</Badge>
                        ) : null}
                        {fit ? <Badge tone={fit.tone}>{fit.label}</Badge> : null}
                        {alreadyInstalled ? (
                          <Badge tone="success">{en ? "Installed" : "已安装"}</Badge>
                        ) : null}
                      </div>
                      <p class="hint">
                        {[
                          formatBytes(m.size, uiLang),
                          (m.format || "gguf").toUpperCase(),
                          fit
                            ? `${en ? "Est. RAM/VRAM" : "预计内存占用"} ~${formatBytes(fit.estimated, uiLang)}`
                            : "",
                        ]
                          .filter(Boolean)
                          .join(" · ")}
                      </p>
                      {rec ? (
                        <p class="hint" style={{ color: "var(--accent)", marginTop: "4px" }}>
                          {en
                            ? `Hardware check: Recommended ${rec.backend.toUpperCase()}, Context ${rec.tuning.context || 4096}, Est. footprint ${formatBytes(rec.estimated_bytes, uiLang)}`
                            : `本机评估：推荐 ${rec.backend.toUpperCase()} 后端 · 上下文 ${rec.tuning.context || 4096} · 预计占用 ${formatBytes(rec.estimated_bytes, uiLang)}`}
                        </p>
                      ) : null}
                    </div>
                    <div class="actions">
                      <Button
                        busy={store.isBusy(`models:rec:${m.id}`)}
                        onClick={() => void handlePreviewRecommend(m)}
                      >
                        {en ? "Evaluate fit" : "评估硬件适配"}
                      </Button>
                      <Button
                        variant={alreadyInstalled ? "secondary" : "primary"}
                        busy={store.isBusy(`models:dl:${m.id}`)}
                        onClick={() => void handleInstallModel(m)}
                      >
                        {alreadyInstalled
                          ? en
                            ? "Re-verify / Repair"
                            : "校验或重下"
                          : en
                            ? "Download"
                            : "下载模型"}
                      </Button>
                    </div>
                  </div>
                </div>
              );
            }}
          />
        )}
      </fieldset>
    </section>
  );
}
