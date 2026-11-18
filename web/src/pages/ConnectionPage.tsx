import { useState } from "preact/hooks";
import {
  Badge,
  Button,
  CopyButton,
  CopyField,
  Disclosure,
  FeedbackBanner,
  IconCopy,
  IconDownload,
  IconEye,
  IconEyeOff,
  IconRefresh,
  Select,
  SettingGroup,
  confirmAction,
  runTracked,
  toast,
  useFeedbackStore,
} from "../shared/design-system";
import { api, isDesktopMode, nativeCommand } from "../api";
import { t, toUiLang, type ManagerLocale } from "../i18n";
import type { StatusResponse } from "../types";

export function ConnectionPage({
  data,
  locale,
  onRefresh,
}: {
  data: StatusResponse;
  locale: ManagerLocale;
  onRefresh: () => Promise<void>;
}) {
  const store = useFeedbackStore();
  const uiLang = toUiLang(locale);
  const en = uiLang === "en";

  const [tokenValue, setTokenValue] = useState("");
  const [revealed, setRevealed] = useState(false);
  const [selectedExampleModel, setSelectedExampleModel] = useState(
    data.settings.model || "default",
  );
  const [logsContent, setLogsContent] = useState<string | null>(null);
  const [logsOpen, setLogsOpen] = useState(false);

  const ensureToken = async (): Promise<string> => {
    if (tokenValue) return tokenValue;
    const res = await api<{ token: string }>("/api/v1/token");
    setTokenValue(res.token);
    return res.token;
  };

  const handleToggleReveal = () =>
    runTracked(
      { key: "conn:reveal", scope: "connection", lang: uiLang },
      async () => {
        if (!revealed) {
          await ensureToken();
        }
        setRevealed((r) => !r);
      },
    );

  // M-45: Immediate clear feedback on Token rotate and diagnostics export
  const handleRotateToken = async () => {
    const ok = await confirmAction({
      title: en ? "Rotate Local Access Token" : "轮换本机访问 Token",
      message: en
        ? "Generate a new random token? Connected clients (such as Qingyi browser extension) will need the updated token."
        : "确定重新生成访问 Token 吗？已配置的客户端（如轻译扩展）需同步更新为此新 Token。",
      danger: true,
      lang: uiLang,
    });
    if (!ok) return;
    await runTracked(
      { key: "conn:rotate", scope: "connection", lang: uiLang },
      async () => {
        const res = await api<{ token: string }>("/api/v1/token/rotate", {});
        setTokenValue(res.token);
        setRevealed(true);
        await onRefresh();
        toast(
          en
            ? "New token generated. Copy and update Qingyi settings."
            : "已生成新 Token，请复制并更新到轻译设置中。",
          "success",
        );
      },
    );
  };

  const handleCopyQingyiConfig = () =>
    runTracked(
      { key: "conn:copy-qy", scope: "connection", lang: uiLang },
      async () => {
        const tok = await ensureToken();
        const chosenDep = data.deployments.find(
          (d) =>
            d.id === selectedExampleModel || d.model === selectedExampleModel,
        );
        const bundle = {
          localUrl: data.endpoint,
          localToken: tok,
          localModel: chosenDep
            ? chosenDep.default
              ? chosenDep.model
              : chosenDep.id
            : selectedExampleModel,
          localBackend: chosenDep?.backend || data.settings.backend || "auto",
        };
        await navigator.clipboard.writeText(JSON.stringify(bundle, null, 2));
        toast(
          en
            ? "Copied Qingyi connection JSON to clipboard."
            : "已复制轻译一键连接 JSON（含 Token）。",
          "success",
        );
      },
    );

  const handleExportDiagnostics = () =>
    runTracked(
      { key: "conn:export", scope: "connection", lang: uiLang },
      async () => {
        if (isDesktopMode) {
          await nativeCommand("export");
          toast(
            en
              ? "Diagnostics file saved (tokens & keys redacted)."
              : "已保存脱敏诊断文件（所有密钥与 Token 已自动剔除）。",
            "success",
          );
          return;
        }
        const diag = await api<Record<string, unknown>>("/api/v1/diagnostics");
        const blob = new Blob([JSON.stringify(diag, null, 2)], {
          type: "application/json",
        });
        const url = URL.createObjectURL(blob);
        const a = document.createElement("a");
        a.href = url;
        a.download = `mistral-diagnostics-${new Date().toISOString().slice(0, 10)}.json`;
        a.click();
        setTimeout(() => URL.revokeObjectURL(url), 10000);
        toast(
          en
            ? "Exported redacted diagnostics JSON."
            : "已导出脱敏诊断 JSON。",
          "success",
        );
      },
    );

  // M-36: Clicking Refresh Logs automatically opens the Disclosure & shows explicit empty state
  const handleRefreshLogs = () =>
    runTracked(
      { key: "conn:logs", scope: "connection", lang: uiLang },
      async () => {
        const res = await api<{ logs: string }>("/api/v1/logs");
        setLogsContent(res.logs?.trim() ? res.logs : "");
        setLogsOpen(true);
      },
    );

  // M-17: Executable curl examples for BOTH Bash and Windows PowerShell
  const targetModelParam = selectedExampleModel || data.settings.model;
  const displayTok = revealed && tokenValue ? tokenValue : "<YOUR_LOCAL_TOKEN>";

  const bashExample = `curl -sS ${data.endpoint}/v1/chat/completions \\
  -H "Authorization: Bearer ${displayTok}" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"${targetModelParam}","messages":[{"role":"user","content":"Translate into Chinese: Hello world."}]}'`;

  const pwshExample = `$body = @{
  model = "${targetModelParam}"
  messages = @(@{ role = "user"; content = "Translate into Chinese: Hello world." })
} | ConvertTo-Json -Depth 4
Invoke-RestMethod -Method Post -Uri "${data.endpoint}/v1/chat/completions" \`
  -Headers @{ Authorization = "Bearer ${displayTok}" } \`
  -ContentType "application/json" -Body $body`;

  return (
    <section class="panel mg-section">
      <div class="mg-page-head">
        <div class="mg-page-title-group">
          <h1 class="mg-page-title">{t(locale, "navConnection")}</h1>
          <Badge tone="success">OpenAI v1</Badge>
          <span class="hint">{data.endpoint}</span>
        </div>
        <div class="actions" style={{ margin: 0 }}>
          <Button
            variant="primary"
            size="sm"
            icon={<IconCopy size={13} />}
            busy={store.isBusy("conn:copy-qy")}
            onClick={handleCopyQingyiConfig}
          >
            {en ? "Copy Qingyi JSON" : "一键复制轻译连接 JSON"}
          </Button>
          <Button
            size="sm"
            icon={<IconDownload size={13} />}
            busy={store.isBusy("conn:export")}
            onClick={handleExportDiagnostics}
          >
            {en ? "Export Diagnostics" : "导出脱敏诊断包"}
          </Button>
        </div>
      </div>

      <FeedbackBanner scope="connection" lang={uiLang} />

      <SettingGroup
        title={en ? "Endpoints & Access Token" : "接口地址与访问凭证"}
        description={
          en
            ? "Bound strictly to loopback (127.0.0.1) with DPAPI-encrypted Token protection."
            : "严格绑定本机回环地址（127.0.0.1），Token 经系统 DPAPI 加密存储。"
        }
        padded
      >
        <div class="grid-2">
          <CopyField
            label={en ? "Manager Base URL (for Qingyi)" : "本地管理器根地址（轻译填写此项）"}
            value={data.endpoint}
            lang={uiLang}
          />
          <CopyField
            label={en ? "OpenAI Chat Completions URL" : "OpenAI 兼容推理地址"}
            value={`${data.endpoint}/v1/chat/completions`}
            lang={uiLang}
          />
        </div>

        <div style={{ marginTop: "12px" }}>
          <CopyField
            label={en ? "Bearer Access Token" : "本机访问 Token"}
            value={revealed ? tokenValue : "••••••••••••••••••••••••••••••••"}
            masked={!revealed}
            copyValue={tokenValue || undefined}
            lang={uiLang}
          />
          <div class="actions" style={{ marginBottom: 0 }}>
            <Button
              size="sm"
              icon={revealed ? <IconEyeOff size={13} /> : <IconEye size={13} />}
              busy={store.isBusy("conn:reveal")}
              onClick={handleToggleReveal}
            >
              {revealed
                ? en
                  ? "Hide Token"
                  : "隐藏 Token"
                : en
                  ? "Reveal Token"
                  : "显示 Token"}
            </Button>
            <Button
              size="sm"
              icon={<IconCopy size={13} />}
              onClick={async () => {
                const tok = await ensureToken();
                await navigator.clipboard.writeText(tok);
                toast(en ? "Token copied." : "已复制访问 Token。", "success");
              }}
            >
              {en ? "Copy Token" : "复制 Token"}
            </Button>
            <Button
              size="sm"
              variant="danger"
              icon={<IconRefresh size={13} />}
              busy={store.isBusy("conn:rotate")}
              onClick={() => void handleRotateToken()}
            >
              {en ? "Rotate Token" : "重新生成 Token"}
            </Button>
          </div>
        </div>
      </SettingGroup>

      {/* M-17: Dynamic runnable CLI snippets */}
      <SettingGroup
        title={en ? "CLI Examples" : "命令行调用示例"}
        padded
      >
        <Select
          label={en ? "Target Deployment / Model" : "示例目标部署或模型"}
          value={selectedExampleModel}
          options={data.deployments.map((d) => ({
            value: d.default ? d.model : d.id,
            label: d.default
              ? `${d.model} (${en ? "Default" : "默认"})`
              : `${d.id} → ${d.model}`,
          }))}
          onValueChange={(v) => setSelectedExampleModel(v)}
        />
        <div class="grid-2" style={{ marginTop: "12px" }}>
          <div>
            <div class="split-head" style={{ marginBottom: "6px" }}>
              <strong style={{ fontSize: "var(--font-xs)" }}>Bash / curl</strong>
              <CopyButton text={bashExample} size="sm" variant="ghost" lang={uiLang} />
            </div>
            <pre style={{ overflowX: "auto", margin: 0 }}>{bashExample}</pre>
          </div>
          <div>
            <div class="split-head" style={{ marginBottom: "6px" }}>
              <strong style={{ fontSize: "var(--font-xs)" }}>PowerShell</strong>
              <CopyButton text={pwshExample} size="sm" variant="ghost" lang={uiLang} />
            </div>
            <pre style={{ overflowX: "auto", margin: 0 }}>{pwshExample}</pre>
          </div>
        </div>
      </SettingGroup>

      {/* M-36: Auto-expanding manager logs with clear empty state */}
      <SettingGroup
        title={en ? "Manager Logs (Redacted)" : "管理器运行日志（已脱敏）"}
        actions={
          <Button
            size="sm"
            icon={<IconRefresh size={13} />}
            busy={store.isBusy("conn:logs")}
            onClick={handleRefreshLogs}
          >
            {en ? "Load Recent Logs" : "读取最近日志"}
          </Button>
        }
        padded
      >
        <Disclosure
          open={logsOpen}
          onToggleOpen={(next) => setLogsOpen(next)}
          summary={
            en ? "manager.log tail (64 KiB)" : "查看 manager.log 尾部内容（64 KiB）"
          }
        >
          {logsContent === null ? (
            <p class="muted" style={{ margin: 0 }}>
              {en
                ? "Click 'Load Recent Logs' above to view."
                : "点击右上角「读取最近日志」加载记录。"}
            </p>
          ) : logsContent === "" ? (
            <p class="muted" style={{ margin: 0 }}>
              {en ? "No log entries recorded yet." : "暂无日志记录。"}
            </p>
          ) : (
            <pre style={{ maxHeight: "280px", overflow: "auto", margin: 0 }}>
              {logsContent}
            </pre>
          )}
        </Disclosure>
      </SettingGroup>
    </section>
  );
}
