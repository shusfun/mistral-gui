import { useEffect, useState } from "preact/hooks";
import {
  Badge,
  Button,
  FeedbackBanner,
  Select,
  Switch,
  TextInput,
  Textarea,
  VerdictHero,
  applyTheme,
  confirmAction,
  feedbackStore,
  formatBytes,
  resolveError,
  runTracked,
  toast,
  useFeedbackStore,
} from "../shared/design-system";
import { ApiError, api, isDesktopMode, nativeCommand } from "../api";
import { toUiLang, type ManagerLocale } from "../i18n";
import type { StatusResponse } from "../types";

export interface UpdateCheckView {
  available?: boolean;
  version?: string;
  current?: string;
  notes?: string;
}

export function SettingsPage({
  data,
  locale,
  trayUpdateCheck,
  onRefresh,
  onLocaleSaved,
  onThemePreview,
  onManagerQuit,
}: {
  data: StatusResponse;
  locale: ManagerLocale;
  trayUpdateCheck: { result?: UpdateCheckView; error?: string } | null;
  onRefresh: () => Promise<void>;
  onLocaleSaved: (loc: ManagerLocale) => void;
  onThemePreview: (theme: "system" | "light" | "dark") => void;
  onManagerQuit: () => void;
}) {
  const store = useFeedbackStore();
  const uiLang = toUiLang(locale);
  const en = uiLang === "en";

  const cfg = data.settings;

  // M-23: Fresh manual/tray update check takes priority over historical update-status.json!
  const [freshCheck, setFreshCheck] = useState<UpdateCheckView | null>(null);

  useEffect(() => {
    if (!trayUpdateCheck) return;
    if (trayUpdateCheck.error) {
      feedbackStore.reportError(
        "updates-check",
        new Error(trayUpdateCheck.error),
        uiLang,
      );
    } else if (trayUpdateCheck.result) {
      setFreshCheck(trayUpdateCheck.result);
      feedbackStore.setBanner("updates-check", {
        tone: trayUpdateCheck.result.available ? "info" : "success",
        message: trayUpdateCheck.result.available
          ? en
            ? `Update available: v${trayUpdateCheck.result.version}`
            : `发现新版本 v${trayUpdateCheck.result.version}`
          : en
            ? `You are on the latest version (${data.version}).`
            : `当前已是最新版本（${data.version}）。`,
      });
    }
  }, [trayUpdateCheck]);

  // Preferences form state
  const [theme, setTheme] = useState<"system" | "light" | "dark">(
    cfg.theme || "system",
  );
  const [langChoice, setLangChoice] = useState<ManagerLocale>(locale);
  const [autoStart, setAutoStart] = useState(cfg.auto_start);
  const [idleMinutes, setIdleMinutes] = useState(cfg.idle_minutes ?? 10);
  const [maxRunning, setMaxRunning] = useState(
    cfg.max_running_deployments ?? 0,
  );
  const [launchAtLogin, setLaunchAtLogin] = useState(
    Boolean(cfg.launch_at_login),
  );
  const [screenshotHotkey, setScreenshotHotkey] = useState(
    cfg.screenshot_hotkey || "",
  );
  const [mirrorsText, setMirrorsText] = useState(
    (cfg.mirrors || []).join("\n"),
  );
  const [allowedExtText, setAllowedExtText] = useState(
    (cfg.allowed_extensions || []).join("\n"),
  );

  // Cloud fallback API settings (DPAPI encrypted in state.json)
  const [cloudUrl, setCloudUrl] = useState(
    cfg.cloud_api_url || "https://api.siliconflow.cn/v1/chat/completions",
  );
  const [cloudModel, setCloudModel] = useState(
    cfg.cloud_model || "tencent/Hunyuan-MT-7B",
  );
  const [cloudKeyInput, setCloudKeyInput] = useState("");
  const [clearCloudKey, setClearCloudKey] = useState(false);

  const handleCheckUpdate = () =>
    runTracked(
      { key: "updates:check", scope: "updates-check", lang: uiLang },
      async () => {
        const res = await api<UpdateCheckView>("/api/v1/update/check", {});
        setFreshCheck(res);
        feedbackStore.setBanner("updates-check", {
          tone: res.available ? "info" : "success",
          message: res.available
            ? en
              ? `New version v${res.version} is available (current: ${data.version}).`
              : `发现新版本 v${res.version}（当前 ${data.version}），可点击下方安装更新。`
            : en
              ? `Already up to date (${data.version}).`
              : `当前已是最新版本（${data.version}）。`,
        });
      },
    );

  const handleInstallUpdate = async (force = false) => {
    await runTracked(
      { key: "updates:install", scope: "updates-check", lang: uiLang },
      async () => {
        try {
          if (isDesktopMode && !force) {
            await nativeCommand("update");
          } else {
            await api("/api/v1/update/install", { force });
          }
          await onRefresh();
          feedbackStore.setBanner("updates-check", {
            tone: "info",
            message: en
              ? "Downloading & verifying signed update. Manager will restart automatically…"
              : "正在下载并校验数字签名更新包，完成后将自动重启并在失败时回退…",
          });
        } catch (err) {
          if (
            err instanceof ApiError &&
            err.message === "active_requests_running"
          ) {
            const ok = await confirmAction({
              message: en
                ? "Inference requests are currently running. Force update and restart anyway?"
                : "当前仍有进行中的推理请求，确定强制中断并执行更新重启吗？",
              danger: true,
              lang: uiLang,
            });
            if (ok) {
              await handleInstallUpdate(true);
            }
            return;
          }
          throw err;
        }
      },
    );
  };

  // M-39: Optimistic theme preview locked against 1s SSE flicker!
  const handleSavePreferences = () =>
    runTracked(
      { key: "updates:save-prefs", scope: "updates-prefs", lang: uiLang },
      async () => {
        const mirrors = mirrorsText
          .split(/\r?\n/)
          .map((s) => s.trim())
          .filter(Boolean);
        const allowed_extensions = allowedExtText
          .split(/\r?\n/)
          .map((s) => s.trim())
          .filter(Boolean);

        onThemePreview(theme);
        applyTheme(theme);

        await api("/api/v1/settings", {
          theme,
          language: langChoice,
          auto_start: autoStart,
          idle_minutes: Number(idleMinutes) || 0,
          max_running_deployments: Number(maxRunning) || 0,
          launch_at_login: launchAtLogin,
          screenshot_hotkey: screenshotHotkey.trim(),
          mirrors,
          allowed_extensions,
        });
        onLocaleSaved(langChoice);
        await onRefresh();
        toast(
          en ? "Preferences saved." : "运行与界面偏好已保存。",
          "success",
        );
      },
    );

  const handleSaveCloudSettings = () =>
    runTracked(
      { key: "updates:save-cloud", scope: "updates-cloud", lang: uiLang },
      async () => {
        await api("/api/v1/settings", {
          cloud_api_url: cloudUrl.trim(),
          cloud_model: cloudModel.trim(),
          ...(clearCloudKey
            ? { clear_cloud_key: true }
            : cloudKeyInput.trim()
              ? { cloud_api_key: cloudKeyInput.trim() }
              : {}),
        });
        setCloudKeyInput("");
        setClearCloudKey(false);
        await onRefresh();
        toast(
          en
            ? "Saved cloud fallback settings (encrypted locally)."
            : "已保存云端回退配置（API Key 经系统 DPAPI 加密存储）。",
          "success",
        );
      },
    );

  const handleCleanupTemp = async () => {
    const ok = await confirmAction({
      message: en
        ? `Delete ${data.temp_storage.files} unfinished .part / staging file(s) (${formatBytes(data.temp_storage.bytes, uiLang)})? Active models are unaffected.`
        : `确定清理 ${data.temp_storage.files} 个未完成的断点续传暂存文件（释放 ${formatBytes(data.temp_storage.bytes, uiLang)}）吗？已安装模型不受影响。`,
      danger: true,
      lang: uiLang,
    });
    if (!ok) return;
    await runTracked(
      { key: "updates:cleanup-temp", scope: "updates-storage", lang: uiLang },
      async () => {
        await api("/api/v1/storage/cleanup", {});
        await onRefresh();
        toast(
          en ? "Cleaned temporary staging files." : "已清理下载暂存文件。",
          "success",
        );
      },
    );
  };

  // M-44: Quit halts background SSE/polling and transitions to full-page stopped state!
  const handleQuitManager = async () => {
    const ok = await confirmAction({
      title: en ? "Quit mistral-gui" : "彻底退出本地管理器",
      message: en
        ? "Stop all running inference engines and quit mistral-gui?"
        : "确定停止所有运行中的模型引擎并彻底退出 mistral-gui 吗？",
      danger: true,
      lang: uiLang,
    });
    if (!ok) return;
    await runTracked(
      { key: "updates:quit", scope: "updates-prefs", lang: uiLang },
      async () => {
        if (isDesktopMode) {
          await nativeCommand("quit");
        } else {
          await api("/api/v1/manager/quit", {});
        }
        onManagerQuit();
      },
    );
  };

  return (
    <section class="panel mg-section">
      <VerdictHero
        eyebrow={en ? "UPDATES, OCR & PREFERENCES" : "自更新、OCR 引擎与系统设置"}
        title={`mistral-gui ${data.version}`}
        description={
          en
            ? `Platform: ${data.platform} · Ed25519-signed updates with automatic health-check rollback`
            : `运行平台：${data.platform} · 支持 Ed25519 数字签名校验与 60 秒健康自检自动回滚`
        }
        badge={
          <Badge tone={freshCheck?.available ? "info" : "neutral"}>
            {freshCheck?.available
              ? `v${freshCheck.version}`
              : data.version}
          </Badge>
        }
      />

      {/* 1. Manager Self-Update */}
      <fieldset>
        <legend>{en ? "1. Manager Self-Update" : "1. 管理器签名自更新"}</legend>
        <FeedbackBanner scope="updates-check" lang={uiLang} />

        {/* M-23: Only show historical update_result when no fresh check supersedes it */}
        {!freshCheck && data.update_result ? (
          <div class="card">
            <p class="hint" style={{ margin: 0 }}>
              {en
                ? `Last update transaction status: ${JSON.stringify(data.update_result)}`
                : `上次自更新记录：${JSON.stringify(data.update_result)}`}
            </p>
          </div>
        ) : null}

        <div class="actions">
          <Button
            variant="primary"
            busy={store.isBusy("updates:check")}
            onClick={handleCheckUpdate}
          >
            {en ? "Check for updates" : "检查新版本"}
          </Button>
          {freshCheck?.available ? (
            <Button
              busy={store.isBusy("updates:install")}
              onClick={() => void handleInstallUpdate(false)}
            >
              {en
                ? `Download & Install v${freshCheck.version}`
                : `下载并安装 v${freshCheck.version}`}
            </Button>
          ) : null}
        </div>
        {freshCheck?.notes ? (
          <pre style={{ whiteSpace: "pre-wrap", marginTop: "10px" }}>
            {freshCheck.notes}
          </pre>
        ) : null}
      </fieldset>

      {/* 2. Interface & Runtime Preferences */}
      <fieldset>
        <legend>{en ? "2. Interface & Runtime Preferences" : "2. 界面、自启与运行偏好"}</legend>
        <FeedbackBanner scope="updates-prefs" lang={uiLang} />

        <div class="grid-3">
          <Select
            label={en ? "Theme" : "外观主题"}
            value={theme}
            options={[
              { value: "system", label: en ? "Follow system" : "跟随系统" },
              { value: "light", label: en ? "Light" : "浅色模式" },
              { value: "dark", label: en ? "Dark" : "深色模式" },
            ]}
            onValueChange={(v) => {
              const next = v as "system" | "light" | "dark";
              setTheme(next);
              onThemePreview(next);
              applyTheme(next);
            }}
          />
          <Select
            label={en ? "Interface Language" : "界面语言"}
            value={langChoice}
            options={[
              { value: "zh-CN", label: "简体中文 (zh-CN)" },
              { value: "en-US", label: "English (en-US)" },
            ]}
            onValueChange={(v) => setLangChoice(v as ManagerLocale)}
          />
          <TextInput
            label={en ? "Screenshot Translate Hotkey (Windows)" : "全局截图翻译快捷键（Windows）"}
            placeholder="Alt+Shift+A"
            value={screenshotHotkey}
            onValueInput={(v) => setScreenshotHotkey(v)}
          />
        </div>

        <div class="grid-2" style={{ marginTop: "12px" }}>
          <TextInput
            label={en ? "Default idle auto-stop (minutes, 0=off)" : "默认空闲自动卸载（分钟，0 为关闭）"}
            type="number"
            min={0}
            max={1440}
            value={idleMinutes}
            onValueInput={(v) => setIdleMinutes(Number(v) || 0)}
          />
          <TextInput
            label={en ? "Max concurrent running deployments (0=auto VRAM guard)" : "同时运行实例数上限（0 表示按可用内存自动保护）"}
            type="number"
            min={0}
            max={32}
            value={maxRunning}
            onValueInput={(v) => setMaxRunning(Number(v) || 0)}
          />
        </div>

        <div style={{ marginTop: "10px" }}>
          <Switch
            checked={autoStart}
            onChange={(c) => setAutoStart(c)}
            label={
              en
                ? "Automatically start installed default model when API requests arrive"
                : "收到翻译或 API 请求时自动唤醒已安装的默认模型"
            }
          />
          <Switch
            checked={launchAtLogin}
            onChange={(c) => setLaunchAtLogin(c)}
            label={
              en
                ? "Start mistral-gui minimized in system tray at OS login"
                : "开机登录时自动在系统托盘静默启动 mistral-gui"
            }
          />
        </div>

        <div class="grid-2" style={{ marginTop: "12px" }}>
          <Textarea
            label={en ? "HTTPS Download Mirror Prefixes (one per line)" : "HTTPS 下载镜像前缀（每行一个，用于加速 Hugging Face / GitHub）"}
            rows={3}
            placeholder="https://hf-mirror.com/"
            value={mirrorsText}
            onValueInput={(v) => setMirrorsText(v)}
          />
          <Textarea
            label={en ? "Additional Allowed Extension IDs (optional)" : "额外允许跨域访问的浏览器扩展 ID（每行一个，默认内置轻译正式版）"}
            rows={3}
            placeholder="epficbaecbjclndbkkkdoliihpnbdejm"
            value={allowedExtText}
            onValueInput={(v) => setAllowedExtText(v)}
          />
        </div>

        <div class="actions">
          <Button
            variant="primary"
            busy={store.isBusy("updates:save-prefs")}
            onClick={handleSavePreferences}
          >
            {en ? "Save preferences" : "保存运行偏好"}
          </Button>
          <Button
            variant="danger"
            busy={store.isBusy("updates:quit")}
            onClick={() => void handleQuitManager()}
          >
            {en ? "Stop All & Quit Manager" : "停止所有引擎并退出管理器"}
          </Button>
        </div>
      </fieldset>

      {/* 3. Desktop Cloud Fallback API */}
      <fieldset>
        <legend>
          {en
            ? "3. Cloud Translation Fallback (for Desktop Paste & Screenshot Translate)"
            : "3. 桌面翻译云端回退服务配置（用于粘贴与截图翻译）"}
        </legend>
        <FeedbackBanner scope="updates-cloud" lang={uiLang} />
        <div class="grid-3">
          <TextInput
            label={en ? "Cloud API URL (OpenAI Chat Completions)" : "云端 API 地址（OpenAI 兼容）"}
            value={cloudUrl}
            onValueInput={(v) => setCloudUrl(v)}
          />
          <TextInput
            label={en ? "Cloud Model Name" : "云端模型名称"}
            value={cloudModel}
            onValueInput={(v) => setCloudModel(v)}
          />
          <TextInput
            type="password"
            label={
              data.has_cloud_key
                ? en
                  ? "Cloud API Key (saved — leave blank to keep)"
                  : "云端 API Key（已加密保存，留空不修改）"
                : en
                  ? "Cloud API Key"
                  : "云端 API Key"
            }
            placeholder={data.has_cloud_key ? "••••••••" : "sk-..."}
            autocomplete="off"
            value={cloudKeyInput}
            onValueInput={(v) => {
              setCloudKeyInput(v);
              setClearCloudKey(false);
            }}
          />
        </div>
        <div class="actions">
          <Button
            busy={store.isBusy("updates:save-cloud")}
            onClick={handleSaveCloudSettings}
          >
            {en ? "Save cloud fallback" : "保存云端配置"}
          </Button>
          {data.has_cloud_key ? (
            <Button
              variant="ghost"
              onClick={() => {
                setClearCloudKey(true);
                setCloudKeyInput("");
                toast(
                  en
                    ? "Click 'Save cloud fallback' to confirm removing stored key."
                    : "点击「保存云端配置」即可清除已保存的云端 Key。",
                  "info",
                );
              }}
            >
              {en ? "Clear stored key" : "清除已保存的云端 Key"}
            </Button>
          ) : null}
        </div>
      </fieldset>

      {/* 4. Local OCR Engine, Synced Dictionary & Temp Storage */}
      <fieldset>
        <legend>
          {en
            ? "4. Local OCR Engine, Synced Dictionary & Storage Cleanup"
            : "4. 本地 OCR 识别引擎、同步词库与暂存空间管理"}
        </legend>
        <FeedbackBanner scope="updates-storage" lang={uiLang} />
        <div class="rows">
          <div class="list-row">
            <div class="list-row-main">
              <strong>
                {en
                  ? `Local OCR Engine (${data.ocr.engine})`
                  : `本地离线 OCR 识别运行时（${data.ocr.engine}）`}
              </strong>
              <p class="hint">
                {data.ocr.installed
                  ? en
                    ? `Installed (${formatBytes(data.ocr.disk_bytes, uiLang)} on disk · ${data.ocr.running ? "Worker warm" : "Idle unloaded"}) · ${data.ocr.license}`
                    : `已安装（占用磁盘 ${formatBytes(data.ocr.disk_bytes, uiLang)} · ${data.ocr.running ? "识别进程热备中" : "空闲未占内存"}）· 许可：${data.ocr.license}`
                  : en
                    ? `Not installed (~${formatBytes(data.ocr.download_bytes, uiLang)} download). Only needed for image & screenshot translation.`
                    : `尚未安装（下载体积约 ${formatBytes(data.ocr.download_bytes, uiLang)}）。仅在使用图片或屏幕截图翻译时按需下载。`}
              </p>
            </div>
            <div class="actions">
              {!data.ocr.installed ? (
                <Button
                  busy={store.isBusy("updates:ocr-dl")}
                  onClick={() =>
                    void runTracked(
                      {
                        key: "updates:ocr-dl",
                        scope: "updates-storage",
                        lang: uiLang,
                      },
                      async () => {
                        await api("/api/v1/ocr/install", {});
                        await onRefresh();
                        toast(
                          en
                            ? "Started OCR runtime download."
                            : "已开始下载本地 OCR 运行时。",
                          "info",
                        );
                      },
                    )
                  }
                >
                  {en ? "Download OCR" : "下载安装 OCR"}
                </Button>
              ) : (
                <Button
                  variant="danger"
                  busy={store.isBusy("updates:ocr-del")}
                  onClick={async () => {
                    const ok = await confirmAction({
                      message: en
                        ? "Remove local OCR runtime files from disk?"
                        : "确定从本机删除本地 OCR 识别运行时文件吗？",
                      danger: true,
                      lang: uiLang,
                    });
                    if (!ok) return;
                    await runTracked(
                      {
                        key: "updates:ocr-del",
                        scope: "updates-storage",
                        lang: uiLang,
                      },
                      async () => {
                        await api("/api/v1/ocr/delete", {});
                        await onRefresh();
                        toast(
                          en ? "Removed local OCR runtime." : "已删除本地 OCR 运行时。",
                          "success",
                        );
                      },
                    );
                  }}
                >
                  {en ? "Remove OCR" : "卸载移除 OCR"}
                </Button>
              )}
            </div>
          </div>

          <div class="list-row">
            <div class="list-row-main">
              <strong>
                {en ? "Synced Qingyi Dictionary & Revisions" : "来自轻译扩展的单向同步词库副本"}
              </strong>
              <p class="hint">
                {en
                  ? `${data.dictionary_sync.revisions} revision(s), ${data.dictionary_sync.pack_entries} dictionary entry(ies)${data.dictionary_sync.synced_at ? ` · Last synced ${new Date(data.dictionary_sync.synced_at).toLocaleString()}` : ""}`
                  : `人工修订 ${data.dictionary_sync.revisions} 条 · 词库词条 ${data.dictionary_sync.pack_entries} 条${data.dictionary_sync.synced_at ? ` · 最近同步：${new Date(data.dictionary_sync.synced_at).toLocaleString()}` : ""}`}
              </p>
            </div>
            <div class="actions">
              <Button
                variant="danger"
                disabled={
                  data.dictionary_sync.revisions === 0 &&
                  data.dictionary_sync.pack_entries === 0
                }
                busy={store.isBusy("updates:dict-clear")}
                onClick={async () => {
                  const ok = await confirmAction({
                    message: en
                      ? "Clear synced dictionary copy in mistral-gui?"
                      : "确定清空 mistral-gui 本机保存的同步词库与修订副本吗？",
                    danger: true,
                    lang: uiLang,
                  });
                  if (!ok) return;
                  await runTracked(
                    {
                      key: "updates:dict-clear",
                      scope: "updates-storage",
                      lang: uiLang,
                    },
                    async () => {
                      await api("/api/v1/dictionary/clear", {});
                      await onRefresh();
                      toast(
                        en ? "Cleared synced dictionary." : "已清空同步词库副本。",
                        "success",
                      );
                    },
                  );
                }}
              >
                {en ? "Clear synced copy" : "清空同步副本"}
              </Button>
            </div>
          </div>

          <div class="list-row">
            <div class="list-row-main">
              <strong>
                {en
                  ? "Resumable Download Cache (.part / .staging)"
                  : "未完成的下载暂存文件（.part / .staging）"}
              </strong>
              <p class="hint">
                {en
                  ? `${data.temp_storage.files} file(s) · ${formatBytes(data.temp_storage.bytes, uiLang)}`
                  : `共 ${data.temp_storage.files} 个暂存文件 · 占用 ${formatBytes(data.temp_storage.bytes, uiLang)}`}
              </p>
            </div>
            <div class="actions">
              <Button
                variant="danger"
                disabled={data.temp_storage.files === 0}
                busy={store.isBusy("updates:cleanup-temp")}
                onClick={() => void handleCleanupTemp()}
              >
                {en ? "Clean temporary files" : "清理暂存文件"}
              </Button>
            </div>
          </div>
        </div>

        {/* Technical resolution note for M-31 */}
        <p class="hint" style={{ marginTop: "10px" }}>
          {en
            ? `Data directory resides on local user profile storage.`
            : `模型、配置与日志均独立存放于当前用户本机数据目录中，卸载程序时默认保留，可按需选择清理。`}
        </p>
      </fieldset>
    </section>
  );
}
