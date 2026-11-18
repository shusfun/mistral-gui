import { useEffect, useState } from "preact/hooks";
import {
  Badge,
  Button,
  DataList,
  FeedbackBanner,
  IconDownload,
  IconRefresh,
  IconTrash,
  IconUpload,
  Select,
  SettingGroup,
  SettingRow,
  StateBlock,
  TextInput,
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

  // M-01 & M-25: Pass selectedBackend directly in query; surface structured error + Retry on failure
  const fetchAssets = async (
    refresh = false,
    backendArg = selectedBackend,
    repoArg = activeRepo,
  ) => {
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
          en
            ? "Refreshed upstream asset list."
            : "已刷新上游可用运行时与模型列表。",
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

  const evaluateFit = (asset: CatalogAsset) => {
    let budgetBytes =
      deviceInfo?.available_bytes || deviceInfo?.memory_bytes || 0;
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
      return {
        tone: "success" as const,
        label: t(locale, "suitable"),
        estimated,
      };
    }
    if (estimated <= budgetBytes * 1.05) {
      return {
        tone: "warning" as const,
        label: t(locale, "tight"),
        estimated,
      };
    }
    return {
      tone: "error" as const,
      label: t(locale, "unsuitable"),
      estimated,
    };
  };

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
            ? `Downloading ${selectedBackend.toUpperCase()} runtime in background…`
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
            : `已加入下载任务：${m.id}（支持断点续传与 SHA-256 校验）。`,
        });
      },
    );

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
      <div class="mg-page-head">
        <div class="mg-page-title-group">
          <h1 class="mg-page-title">{t(locale, "navModels")}</h1>
          <Badge tone={data.settings.engine ? "success" : "neutral"}>
            {data.settings.engine
              ? `${en ? "Engine" : "引擎"}: ${data.settings.engine_version || (en ? "Ready" : "已安装")}`
              : en
                ? "Engine Not Installed"
                : "引擎未安装"}
          </Badge>
          <Badge tone="neutral">
            {en
              ? `${installedModels.length} Installed`
              : `已装 ${installedModels.length} 个模型`}
          </Badge>
        </div>

        <Button
          size="sm"
          icon={<IconRefresh size={13} />}
          busy={loadingAssets}
          onClick={() => void fetchAssets(true, selectedBackend, activeRepo)}
        >
          {en ? "Check Releases" : "刷新上游列表"}
        </Button>
      </div>

      <FeedbackBanner scope="models" lang={uiLang} />

      <SettingGroup
        title={en ? "mistral.rs Inference Runtime" : "mistral.rs 推理运行时"}
        description={
          engineAsset
            ? en
              ? `Matched package: ${engineAsset.id} (${engineAsset.version}, ${formatBytes(engineAsset.size, uiLang)}, ${formatAssetSource(locale, engineAsset.source)})`
              : `匹配发行包：${engineAsset.id}（${engineAsset.version} · ${formatBytes(engineAsset.size, uiLang)} · ${formatAssetSource(locale, engineAsset.source)}）`
            : undefined
        }
      >
        <SettingRow
          title={en ? "Target Acceleration Backend" : "硬件加速后端"}
          controlWidth="medium"
        >
          <Select
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
        </SettingRow>

        <SettingRow
          title={en ? "Runtime Package Actions" : "运行时安装与切换"}
          description={
            assetErrors.engine
              ? resolveError(assetErrors.engine, uiLang).message
              : en
                ? "Verified by SHA-256 digest with instant previous-version rollback"
                : "自动校验 SHA-256 摘要，支持一键切回上一版本"
          }
        >
          <div class="actions" style={{ margin: 0 }}>
            <Button
              size="sm"
              variant="primary"
              icon={<IconDownload size={13} />}
              busy={store.isBusy("models:install-engine")}
              onClick={handleInstallEngine}
            >
              {data.settings.engine
                ? en
                  ? "Update / Reinstall"
                  : "更新或重装运行时"
                : en
                  ? "Download Runtime"
                  : "下载并安装运行时"}
            </Button>
            <Button
              size="sm"
              busy={store.isBusy("models:save-backend")}
              onClick={handleSaveBackend}
            >
              {en ? "Set Default Backend" : "设为默认后端"}
            </Button>
            {data.settings.previous_engine ? (
              <Button
                size="sm"
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
                {en ? "Roll Back" : "切回上一版"}
              </Button>
            ) : null}
          </div>
        </SettingRow>
      </SettingGroup>

      <SettingGroup
        title={en ? "Installed Local Models" : "已安装本地模型"}
        padded
        actions={
          isDesktopMode ? (
            <>
              <Button
                size="sm"
                icon={<IconUpload size={13} />}
                busy={store.isBusy("models:import-gguf")}
                onClick={() => void handleImportLocal("import-gguf")}
              >
                {en ? "Import .gguf" : "导入 .gguf 文件"}
              </Button>
              <Button
                size="sm"
                icon={<IconUpload size={13} />}
                busy={store.isBusy("models:import-directory")}
                onClick={() => void handleImportLocal("import-directory")}
              >
                {en ? "Import Folder" : "导入 safetensors 目录"}
              </Button>
            </>
          ) : undefined
        }
      >
        <DataList
          items={installedModels}
          getKey={(m) => `${m.id}@${m.version}`}
          emptyMessage={
            en
              ? "No models installed yet. Choose a recommended model below to download."
              : "尚未安装本地模型，可在下方模型目录一键下载。"
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
                  <div
                    style={{
                      display: "flex",
                      gap: "8px",
                      alignItems: "center",
                      flexWrap: "wrap",
                    }}
                  >
                    <strong>{m.id}</strong>
                    {isDefaultSelected ? (
                      <Badge tone="success">
                        {en ? "Default" : "当前默认"}
                      </Badge>
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
                <div class="actions" style={{ margin: 0 }}>
                  {!isDefaultSelected ? (
                    <Button
                      size="sm"
                      variant="primary"
                      busy={store.isBusy(`models:use:${m.id}:${m.version}`)}
                      onClick={() => void handleSelectDefaultModel(m)}
                    >
                      {en ? "Set Default" : "设为默认"}
                    </Button>
                  ) : null}
                  <Button
                    size="sm"
                    variant="danger"
                    icon={<IconTrash size={12} />}
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
      </SettingGroup>

      <SettingGroup
        title={en ? "Model Catalog" : "可下载模型目录（支持自定义 Hugging Face 仓库）"}
        padded
      >
        <div class="grid-2" style={{ marginBottom: "14px" }}>
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
                label={
                  en
                    ? "Custom Hugging Face Repo (owner/repo)"
                    : "自定义仓库（owner/repo）"
                }
                placeholder="Qwen/Qwen3-4B-GGUF"
                value={customRepo}
                onValueInput={(v) => setCustomRepo(v)}
              />
            </div>
            <Button
              busy={loadingAssets}
              onClick={() => void fetchAssets(true, selectedBackend, activeRepo)}
            >
              {en ? "Load" : "读取"}
            </Button>
          </div>
        </div>

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
                : "正在读取仓库文件体积与校验摘要…"
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
                : "该仓库中未找到兼容的量化或权重文件。"
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
                      <div
                        style={{
                          display: "flex",
                          gap: "8px",
                          alignItems: "center",
                          flexWrap: "wrap",
                        }}
                      >
                        <strong>{m.id}</strong>
                        {m.recommended ? (
                          <Badge tone="info">
                            {en ? "Recommended" : "推荐"}
                          </Badge>
                        ) : null}
                        {fit ? (
                          <Badge tone={fit.tone}>{fit.label}</Badge>
                        ) : null}
                        {alreadyInstalled ? (
                          <Badge tone="success">
                            {en ? "Installed" : "已安装"}
                          </Badge>
                        ) : null}
                      </div>
                      <p class="hint">
                        {[
                          formatBytes(m.size, uiLang),
                          (m.format || "gguf").toUpperCase(),
                          fit
                            ? `${en ? "Est. RAM/VRAM" : "预计占用"} ~${formatBytes(fit.estimated, uiLang)}`
                            : "",
                        ]
                          .filter(Boolean)
                          .join(" · ")}
                      </p>
                      {rec ? (
                        <p
                          class="hint"
                          style={{
                            color: "var(--color-accent)",
                            marginTop: "4px",
                          }}
                        >
                          {en
                            ? `Recommended ${rec.backend.toUpperCase()} · Context ${rec.tuning.context || 4096} · Est. ${formatBytes(rec.estimated_bytes, uiLang)}`
                            : `本机评估：推荐 ${rec.backend.toUpperCase()} · 上下文 ${rec.tuning.context || 4096} · 预计 ${formatBytes(rec.estimated_bytes, uiLang)}`}
                        </p>
                      ) : null}
                    </div>
                    <div class="actions" style={{ margin: 0 }}>
                      <Button
                        size="sm"
                        busy={store.isBusy(`models:rec:${m.id}`)}
                        onClick={() => void handlePreviewRecommend(m)}
                      >
                        {en ? "Evaluate Fit" : "评估适配"}
                      </Button>
                      <Button
                        size="sm"
                        variant={alreadyInstalled ? "secondary" : "primary"}
                        icon={<IconDownload size={13} />}
                        busy={store.isBusy(`models:dl:${m.id}`)}
                        onClick={() => void handleInstallModel(m)}
                      >
                        {alreadyInstalled
                          ? en
                            ? "Verify / Repair"
                            : "校验重下"
                          : en
                            ? "Download"
                            : "下载"}
                      </Button>
                    </div>
                  </div>
                </div>
              );
            }}
          />
        )}
      </SettingGroup>
    </section>
  );
}
