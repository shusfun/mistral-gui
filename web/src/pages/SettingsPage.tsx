import { useEffect, useState } from "preact/hooks";
import {
  Badge,
  Button,
  FeedbackBanner,
  IconDownload,
  IconRefresh,
  IconTrash,
  SegmentedControl,
  SettingGroup,
  SettingRow,
  Switch,
  NumberStepper,
  TextInput,
  Textarea,
  applyTheme,
  confirmAction,
  feedbackStore,
  formatBytes,
  runTracked,
  toast,
  useFeedbackStore,
} from "../shared/design-system";
import { ApiError, api, isDesktopMode, nativeCommand, traceUiSave } from "../api";
import { t, toUiLang, type ManagerLocale } from "../i18n";
import type { StatusResponse } from "../types";

import { parseUpdateCheck, type UpdateCheckView } from "../update-check";
export type { UpdateCheckView } from "../update-check";

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
  onRefresh: (trace?: string) => Promise<void>;
  onLocaleSaved: (loc: ManagerLocale) => void;
  onThemePreview: (theme: "system" | "light" | "dark") => void;
  onManagerQuit: () => void;
}) {
  const store = useFeedbackStore();
  const uiLang = toUiLang(locale);
  const en = uiLang === "en";

  const cfg = data.settings;

  // M-23: Fresh manual/tray update check takes priority over historical update-status.json
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
            ? `Update available: v${trayUpdateCheck.result.manifest?.version?.replace(/^v/, "")}`
            : `发现新版本 v${trayUpdateCheck.result.manifest?.version?.replace(/^v/, "")}`
          : en
            ? `You are on the latest version (${data.version}).`
            : `当前已是最新版本（${data.version}）。`,
      });
    }
  }, [trayUpdateCheck]);

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
        const res = parseUpdateCheck(
          await api<unknown>("/api/v1/update/check", {}),
        );
        setFreshCheck(res);
        feedbackStore.setBanner("updates-check", {
          tone: res.available ? "info" : "success",
          message: res.available
            ? en
              ? `New version v${res.manifest?.version?.replace(/^v/, "")} is available (current: ${data.version}).`
              : `发现新版本 v${res.manifest?.version?.replace(/^v/, "")}（当前 ${data.version}），可点击安装更新。`
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
              ? "Downloading and verifying the update. You will be asked before restarting."
              : "正在下载并校验更新包，准备完成后会询问是否重启。",
          });
        } catch (err) {
          if (
            err instanceof ApiError &&
            err.message === "active_requests_running"
          ) {
            const ok = await confirmAction({
              message: en
                ? "Inference requests are currently running. Force update and restart anyway?"
                : "当前仍有进行中的推理请求，确定强制中断并更新重启吗？",
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

  // M-39: Optimistic theme preview locked against SSE flicker
  const handleSavePreferences = () =>
    runTracked(
      { key: "updates:save-prefs", scope: "updates-prefs", lang: uiLang },
      () => traceUiSave(async trace => {
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
        }, { trace });
        onLocaleSaved(langChoice);
        await onRefresh(trace);
        toast(en ? "Preferences saved." : "运行与界面偏好已保存。", "success");
      }),
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

  // M-44: Quit halts background SSE/polling and transitions to full-page stopped state
  const handleQuitManager = async () => {
    const ok = await confirmAction({
      title: en ? "Quit mistral" : "彻底退出本地管理器",
      message: en
        ? "Stop all running inference engines and quit mistral?"
        : "确定停止所有运行中的模型引擎并彻底退出 mistral 吗？",
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
      <div class="mg-page-head">
        <div class="mg-page-title-group">
          <h1 class="mg-page-title">{t(locale, "navUpdates")}</h1>
          <Badge tone={freshCheck?.available ? "info" : "neutral"}>
            {freshCheck?.available
              ? `v${freshCheck.manifest?.version?.replace(/^v/, "")}`
              : data.version}
          </Badge>
          <span class="hint">{data.platform}</span>
        </div>

        <div class="actions" style={{ margin: 0 }}>
          <Button
            size="sm"
            icon={<IconRefresh size={13} />}
            busy={store.isBusy("updates:check")}
            onClick={handleCheckUpdate}
          >
            {en ? "Check Updates" : "检查新版本"}
          </Button>
          {freshCheck?.available ? (
            <Button
              size="sm"
              variant="primary"
              icon={<IconDownload size={13} />}
              busy={store.isBusy("updates:install")}
              onClick={() => void handleInstallUpdate(false)}
            >
              {en
                ? `Install v${freshCheck.manifest?.version?.replace(/^v/, "")}`
                : `下载安装 v${freshCheck.manifest?.version?.replace(/^v/, "")}`}
            </Button>
          ) : null}
        </div>
      </div>

      <FeedbackBanner scope="updates-check" lang={uiLang} />

      {!freshCheck && data.update_result ? (
        <div class="feedback-banner" data-tone="neutral">
          {en
            ? `Last update status: ${JSON.stringify(data.update_result)}`
            : `上次更新记录：${JSON.stringify(data.update_result)}`}
        </div>
      ) : null}

      {freshCheck?.manifest?.notes ? (
        <SettingGroup title={en ? "Release Notes" : "版本说明"} padded>
          <pre style={{ whiteSpace: "pre-wrap", margin: 0 }}>
            {freshCheck.manifest.notes}
          </pre>
        </SettingGroup>
      ) : null}

      <FeedbackBanner scope="updates-prefs" lang={uiLang} />

      <SettingGroup
        title={en ? "Appearance & Runtime Preferences" : "外观与运行偏好"}
        actions={
          <>
            <Button
              variant="primary"
              size="sm"
              busy={store.isBusy("updates:save-prefs")}
              onClick={handleSavePreferences}
            >
              {en ? "Save Preferences" : "保存偏好"}
            </Button>
            <Button
              variant="danger"
              size="sm"
              busy={store.isBusy("updates:quit")}
              onClick={() => void handleQuitManager()}
            >
              {en ? "Quit Manager" : "彻底退出管理器"}
            </Button>
          </>
        }
      >
        <SettingRow title={en ? "Theme" : "外观主题"}>
          <SegmentedControl
            value={theme}
            onChange={(next) => {
              setTheme(next);
              onThemePreview(next);
              applyTheme(next);
            }}
            options={[
              { value: "system", label: en ? "System" : "跟随系统" },
              { value: "light", label: en ? "Light" : "浅色" },
              { value: "dark", label: en ? "Dark" : "深色" },
            ]}
          />
        </SettingRow>

        <SettingRow title={en ? "Interface Language" : "界面语言"}>
          <SegmentedControl
            value={langChoice}
            onChange={(next) => setLangChoice(next)}
            options={[
              { value: "zh-CN", label: "简体中文" },
              { value: "en-US", label: "English" },
            ]}
          />
        </SettingRow>

        <SettingRow
          title={en ? "Screenshot Hotkey" : "全局截图翻译快捷键"}
          description={en ? "Windows global shortcut" : "Windows 桌面全局热键"}
          controlWidth="medium"
        >
          <TextInput
            placeholder="Alt+Shift+A"
            value={screenshotHotkey}
            onValueInput={(v) => setScreenshotHotkey(v)}
          />
        </SettingRow>

        <SettingRow
          title={en ? "Default Idle Auto-Stop (minutes)" : "默认空闲卸载（分钟）"}
          description={en ? "0 disables auto-unload" : "填 0 关闭自动卸载"}
          controlWidth="narrow"
        >
          <NumberStepper
            en={en} name={en ? "Default idle auto-stop" : "默认空闲卸载"} unit={en ? "min" : "分钟"}
            min={0}
            max={1440}
            value={idleMinutes}
            onValueInput={(v) => setIdleMinutes(Number(v) || 0)}
          />
        </SettingRow>

        <SettingRow
          title={en ? "Max Running Deployments" : "并发运行实例数上限"}
          description={
            en
              ? "0 uses automatic memory guard"
              : "填 0 按可用内存自动保护"
          }
          controlWidth="narrow"
        >
          <NumberStepper
            en={en} name={en ? "Max running deployments" : "并发运行实例数上限"}
            min={0}
            max={32}
            value={maxRunning}
            onValueInput={(v) => setMaxRunning(Number(v) || 0)}
          />
        </SettingRow>

        <SettingRow
          title={en ? "Auto-Start Default Model" : "请求到达时自动唤醒默认模型"}
        >
          <Switch
            checked={autoStart}
            onChange={(c) => setAutoStart(c)}
          />
        </SettingRow>

        <SettingRow
          title={en ? "Launch Minimized at Login" : "开机登录时静默启动到系统托盘"}
        >
          <Switch
            checked={launchAtLogin}
            onChange={(c) => setLaunchAtLogin(c)}
          />
        </SettingRow>

        <SettingRow
          title={
            en
              ? "HTTPS Download Mirror Prefixes"
              : "HTTPS 下载镜像前缀（每行一个）"
          }
          vertical
        >
          <Textarea
            rows={2}
            placeholder="https://hf-mirror.com/"
            value={mirrorsText}
            onValueInput={(v) => setMirrorsText(v)}
          />
        </SettingRow>

        <SettingRow
          title={
            en
              ? "Additional Allowed Extension IDs"
              : "额外允许跨域访问的浏览器扩展 ID（每行一个）"
          }
          vertical
        >
          <Textarea
            rows={2}
            placeholder="epficbaecbjclndbkkkdoliihpnbdejm"
            value={allowedExtText}
            onValueInput={(v) => setAllowedExtText(v)}
          />
        </SettingRow>
      </SettingGroup>

      <FeedbackBanner scope="updates-cloud" lang={uiLang} />

      <SettingGroup
        title={
          en
            ? "Cloud Translation Fallback (Desktop)"
            : "云端回退服务（用于粘贴与截图翻译）"
        }
        actions={
          <>
            <Button
              size="sm"
              variant="primary"
              busy={store.isBusy("updates:save-cloud")}
              onClick={handleSaveCloudSettings}
            >
              {en ? "Save Cloud Config" : "保存云端配置"}
            </Button>
            {data.has_cloud_key ? (
              <Button
                size="sm"
                variant="ghost"
                onClick={() => {
                  setClearCloudKey(true);
                  setCloudKeyInput("");
                  toast(
                    en
                      ? "Click 'Save Cloud Config' to remove stored key."
                      : "点击「保存云端配置」确认清除已存 Key。",
                    "info",
                  );
                }}
              >
                {en ? "Clear Key" : "清除已存 Key"}
              </Button>
            ) : null}
          </>
        }
      >
        <SettingRow
          title={en ? "Cloud API URL" : "云端 API 地址（OpenAI 兼容）"}
          controlWidth="wide"
        >
          <TextInput
            value={cloudUrl}
            onValueInput={(v) => setCloudUrl(v)}
          />
        </SettingRow>

        <SettingRow
          title={en ? "Cloud Model Name" : "云端模型名称"}
          controlWidth="wide"
        >
          <TextInput
            value={cloudModel}
            onValueInput={(v) => setCloudModel(v)}
          />
        </SettingRow>

        <SettingRow
          title={en ? "Cloud API Key" : "云端 API Key"}
          description={
            data.has_cloud_key
              ? en
                ? "Encrypted via DPAPI — leave blank to keep"
                : "已通过 DPAPI 加密保存，留空不修改"
              : en
                ? "Encrypted locally via OS DPAPI"
                : "在本机通过系统 DPAPI 加密保存"
          }
          controlWidth="wide"
        >
          <TextInput
            type="password"
            placeholder={data.has_cloud_key ? "••••••••" : "sk-..."}
            autocomplete="off"
            value={cloudKeyInput}
            onValueInput={(v) => {
              setCloudKeyInput(v);
              setClearCloudKey(false);
            }}
          />
        </SettingRow>
      </SettingGroup>

      <FeedbackBanner scope="updates-storage" lang={uiLang} />

      <SettingGroup
        title={
          en
            ? "Local OCR, Synced Dictionary & Storage"
            : "本地 OCR、同步词库与暂存空间"
        }
      >
        <SettingRow
          title={
            en
              ? `Local OCR Runtime (${data.ocr.engine})`
              : `本地离线 OCR 识别引擎（${data.ocr.engine}）`
          }
          description={
            data.ocr.installed
              ? en
                ? `${formatBytes(data.ocr.disk_bytes, uiLang)} on disk · ${data.ocr.running ? "Warm" : "Idle"} · ${data.ocr.license}`
                : `占用 ${formatBytes(data.ocr.disk_bytes, uiLang)} · ${data.ocr.running ? "热备中" : "空闲未占内存"} · ${data.ocr.license}`
              : en
                ? `Not installed (~${formatBytes(data.ocr.download_bytes, uiLang)} download, on-demand for image/screenshot OCR)`
                : `未安装（约 ${formatBytes(data.ocr.download_bytes, uiLang)}，仅图片/截图翻译按需使用）`
          }
        >
          {!data.ocr.installed ? (
            <Button
              size="sm"
              icon={<IconDownload size={13} />}
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
              {en ? "Download OCR" : "下载安装"}
            </Button>
          ) : (
            <Button
              size="sm"
              variant="danger"
              icon={<IconTrash size={12} />}
              busy={store.isBusy("updates:ocr-del")}
              onClick={async () => {
                const ok = await confirmAction({
                  message: en
                    ? "Remove local OCR runtime files from disk?"
                    : "确定从本机移除本地 OCR 运行时文件吗？",
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
                      en
                        ? "Removed local OCR runtime."
                        : "已删除本地 OCR 运行时。",
                      "success",
                    );
                  },
                );
              }}
            >
              {en ? "Remove" : "卸载"}
            </Button>
          )}
        </SettingRow>

        <SettingRow
          title={
            en
              ? "Synced Qingyi Dictionary Copy"
              : "轻译单向同步词库副本"
          }
          description={
            en
              ? `${data.dictionary_sync.revisions} revision(s), ${data.dictionary_sync.pack_entries} entry(ies)`
              : `人工修订 ${data.dictionary_sync.revisions} 条 · 词库词条 ${data.dictionary_sync.pack_entries} 条`
          }
        >
          <Button
            size="sm"
            variant="danger"
            disabled={
              data.dictionary_sync.revisions === 0 &&
              data.dictionary_sync.pack_entries === 0
            }
            busy={store.isBusy("updates:dict-clear")}
            onClick={async () => {
              const ok = await confirmAction({
                message: en
                  ? "Clear synced dictionary copy in mistral?"
                  : "确定清空本机保存的同步词库与修订副本吗？",
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
            {en ? "Clear Copy" : "清空副本"}
          </Button>
        </SettingRow>

        <SettingRow
          title={
            en
              ? "Resumable Download Cache (.part / .staging)"
              : "未完成的下载暂存文件（.part / .staging）"
          }
          description={
            en
              ? `${data.temp_storage.files} file(s) · ${formatBytes(data.temp_storage.bytes, uiLang)}`
              : `共 ${data.temp_storage.files} 个文件 · 占用 ${formatBytes(data.temp_storage.bytes, uiLang)}`
          }
        >
          <Button
            size="sm"
            variant="danger"
            disabled={data.temp_storage.files === 0}
            busy={store.isBusy("updates:cleanup-temp")}
            onClick={() => void handleCleanupTemp()}
          >
            {en ? "Clean Cache" : "清理暂存"}
          </Button>
        </SettingRow>
      </SettingGroup>
    </section>
  );
}
