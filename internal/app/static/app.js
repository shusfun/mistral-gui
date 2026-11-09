const $ = (id) => document.getElementById(id);
const messages = {
  "zh-CN": {
    localWorkspace: "你的本地模型工作台",
    overview: "概览",
    models: "运行时与模型",
    device: "设备与后端",
    connection: "API 与诊断",
    updates: "更新与设置",
    connecting: "连接中",
    connected: "已连接",
    disconnected: "连接中断",
    gettingStarted: "开始使用",
    welcomeTitle: "让模型在需要时运行。",
    welcomeBody:
      "只安装一次运行时和模型，使用时启动，空闲时释放资源。聊天由官方 WebUI 提供。",
    step1: "选择后端",
    step2: "安装运行时",
    step3: "下载模型",
    step4: "启动并测试",
    setup: "配置运行时与模型",
    runtime: "模型运行状态",
    start: "启动",
    stop: "停止",
    restart: "重启",
    openUI: "打开官方 WebUI ↗",
    test: "测试推理",
    requests: "累计请求",
    active: "当前请求",
    testLatency: "测试耗时",
    engine: "mistral.rs 运行时",
    officialFirst: "优先使用官方 Release。官方缺包时才下载补充 CI 资产。",
    backend: "后端",
    saveBackend: "保存后端",
    checkAssets: "检查可用资产",
    installEngine: "下载运行时",
    rollbackEngine: "切回上一运行时",
    modelLibrary: "模型库",
    modelHint: "权重与运行时分开存储，停止运行不会删除文件。",
    availableModel: "可下载模型",
    downloadModel: "下载模型",
    installedModels: "已安装模型",
    emptyModels: "尚未安装模型",
    selectModel: "设为当前",
    deleteModel: "删除",
    deviceHint: "驱动和后端是否可用以本机检测为准，未取得的指标显示为未知。",
    detect: "检测设备",
    endpoint: "OpenAI 兼容地址",
    copyConfig: "复制 Qingyi 配置",
    revealToken: "显示 / 隐藏 Token",
    rotateToken: "重新生成 Token",
    export: "导出诊断",
    tokenHint: "Token 只保存在本机。重新生成后，需要重新配置客户端。",
    appUpdate: "管理器更新",
    updateHint:
      "管理器、mistral.rs 运行时和模型分别更新。管理器更新校验签名，失败自动回退。",
    checkUpdate: "检查更新",
    installUpdate: "下载并安装更新",
    preferences: "运行偏好",
    autoStart: "API 请求时自动启动已安装模型",
    idle: "空闲停止（分钟，0 表示关闭）",
    mirrors: "HTTPS 下载镜像前缀（每行一个）",
    save: "保存",
    tasks: "任务记录",
    taskHint: "刷新后保留，失败可重试",
    emptyTasks: "暂无后台任务",
    connect: "连接管理器",
    loginHint: "请从桌面快捷方式打开，或输入本机管理器 Token。",
    cancel: "取消",
    retry: "重试",
    official: "官方 Release",
    supplement: "补充 CI 资产",
    unknown: "未知",
    available: "可用",
    unavailable: "不可用",
    saved: "设置已保存",
    copied: "已复制",
    confirmDelete: "删除模型文件？",
    confirmRotate: "重新生成 Token 后，客户端需要重新配置。继续？",
    stopped: "已停止",
    starting: "启动中",
    loading: "加载模型",
    running: "运行中",
    stopping: "停止中",
    failed: "失败",
    success: "已完成",
    cancelled: "已取消",
    interrupted: "已中断",
    install: "安装",
    update: "更新",
    checking: "正在检查…",
    noUpdate: "当前已是最新版",
    install_engine_and_model_first: "请先安装运行时和模型。",
    supplement_repository_required: "此平台官方暂无资产，尚未配置补充仓库。",
    update_repository_unconfigured: "此构建未配置更新仓库，正式发行时配置。",
    update_key_unconfigured: "此构建未配置更新签名公钥。",
    invalid_token: "Token 无效，请重新连接。",
    stop_before_switch: "请先停止模型，再切换模型或后端。",
    model_in_use: "模型正在使用，请先停止。",
    previous_engine_missing: "没有可回滚的运行时。",
    manager_restarted: "管理器已重启，可重新发起任务以续传。",
    engine_exited: "引擎已退出；请核对运行时与模型兼容性。",
    model_load_timeout: "模型加载超时，已停止引擎。",
    nvidia_driver_unavailable: "未检测到可用 NVIDIA 驱动。",
    networkError: "连接失败，请检查管理器和网络。",
  },
  "en-US": {
    localWorkspace: "Your local model workspace",
    overview: "Overview",
    models: "Runtime & models",
    device: "Device & backend",
    connection: "API & diagnostics",
    updates: "Updates & settings",
    connecting: "Connecting",
    connected: "Connected",
    disconnected: "Disconnected",
    gettingStarted: "GET STARTED",
    welcomeTitle: "Run models when you need them.",
    welcomeBody:
      "Install a runtime and model once. Start on demand and release resources when idle. Chat in the official WebUI.",
    step1: "Choose backend",
    step2: "Install runtime",
    step3: "Download model",
    step4: "Start and test",
    setup: "Set up runtime & models",
    runtime: "Model runtime",
    start: "Start",
    stop: "Stop",
    restart: "Restart",
    openUI: "Open official WebUI ↗",
    test: "Test inference",
    requests: "Total requests",
    active: "Active requests",
    testLatency: "Test latency",
    engine: "mistral.rs runtime",
    officialFirst:
      "Official Releases come first. Supplemental CI assets fill missing targets only.",
    backend: "Backend",
    saveBackend: "Save backend",
    checkAssets: "Check available assets",
    installEngine: "Download runtime",
    rollbackEngine: "Restore previous runtime",
    modelLibrary: "Model library",
    modelHint:
      "Weights and runtimes are stored separately. Stopping a model keeps its files.",
    availableModel: "Available models",
    downloadModel: "Download model",
    installedModels: "Installed models",
    emptyModels: "No models installed",
    selectModel: "Set current",
    deleteModel: "Delete",
    deviceHint:
      "Backend support is detected on this device. Unavailable measurements are shown as unknown.",
    detect: "Detect device",
    endpoint: "OpenAI-compatible endpoint",
    copyConfig: "Copy Qingyi configuration",
    revealToken: "Show / hide token",
    rotateToken: "Regenerate token",
    export: "Export diagnostics",
    tokenHint:
      "The token stays on this device. Update clients after regenerating it.",
    appUpdate: "Manager updates",
    updateHint:
      "The manager, mistral.rs runtime, and model weights update separately. Manager updates verify signatures and roll back on failure.",
    checkUpdate: "Check for updates",
    installUpdate: "Download & install update",
    preferences: "Runtime preferences",
    autoStart: "Start an installed model on API requests",
    idle: "Stop after idle minutes (0 disables)",
    mirrors: "HTTPS download mirror prefixes (one per line)",
    save: "Save",
    tasks: "Task history",
    taskHint: "Retained across refresh; failed tasks can be retried",
    emptyTasks: "No background tasks",
    connect: "Connect",
    loginHint: "Open from the desktop shortcut, or enter this manager's token.",
    cancel: "Cancel",
    retry: "Retry",
    official: "Official Release",
    supplement: "Supplemental CI",
    unknown: "Unknown",
    available: "Available",
    unavailable: "Unavailable",
    saved: "Settings saved",
    copied: "Copied",
    confirmDelete: "Delete the model file?",
    confirmRotate:
      "Clients must be reconfigured after regenerating the token. Continue?",
    stopped: "Stopped",
    starting: "Starting",
    loading: "Loading model",
    running: "Running",
    stopping: "Stopping",
    failed: "Failed",
    success: "Complete",
    cancelled: "Cancelled",
    interrupted: "Interrupted",
    install: "Install",
    update: "Update",
    checking: "Checking…",
    noUpdate: "You're up to date",
    install_engine_and_model_first: "Install a runtime and model first.",
    supplement_repository_required:
      "No official asset exists for this target and no supplemental repository is configured.",
    update_repository_unconfigured:
      "No update repository is configured in this development build.",
    update_key_unconfigured: "No update signing public key is configured.",
    invalid_token: "Invalid token. Reconnect to the manager.",
    stop_before_switch: "Stop the model before switching the model or backend.",
    model_in_use: "Stop the model before deleting it.",
    previous_engine_missing: "No previous runtime is available.",
    manager_restarted:
      "The manager restarted. Retry the task to resume the download.",
    engine_exited: "The engine exited. Check runtime and model compatibility.",
    model_load_timeout: "Model loading timed out. The engine was stopped.",
    nvidia_driver_unavailable: "No usable NVIDIA driver was detected.",
    networkError: "Connection failed. Check the manager and network.",
  },
};
messages["zh-CN"].downloading = "下载中";
messages["en-US"].downloading = "Downloading";
messages["zh-CN"].verifying = "校验中";
messages["en-US"].verifying = "Verifying";
messages["zh-CN"].uninstalled = "未安装";
messages["en-US"].uninstalled = "Not installed";
messages["zh-CN"].installed = "已安装";
messages["en-US"].installed = "Installed";
messages["zh-CN"].quit = "退出管理器";
messages["en-US"].quit = "Quit manager";
const extra = {
  model: ["模型", "Model"],
  custom: ["自定义来源", "Custom source"],
  community: ["社区来源", "Community source"],
  model_existing_corrupt: [
    "已安装文件损坏，请停止相关部署，删除此版本后重新安装。",
    "Installed files are corrupt. Stop deployments, delete this version and reinstall.",
  ],
  loading_memory_budget_exceeded: [
    "加载及量化峰值内存可能超出预算",
    "Loading and quantization may exceed the memory budget",
  ],
  copyToken: ["复制 Token", "Copy token"],
  vcHint: [
    "mistral.rs 官方 Windows 资产需要 Microsoft Visual C++ 运行库；此设备尚未检测到。",
    "The official mistral.rs Windows runtime requires Microsoft Visual C++ Redistributable, which was not detected on this device.",
  ],
  vcDownload: ["从 Microsoft 下载", "Download from Microsoft"],
  setupDetect: ["检测设备", "Detect device"],
  setupChooseModel: ["选择模型", "Choose model"],
  setupDownload: ["下载并校验", "Download & verify"],
  setupConnect: ["连接使用", "Connect"],
  setupReady: [
    "测试通过，可以打开官方 WebUI 或在 API 页面复制 Qingyi 配置。",
    "Test passed. Open the official WebUI or copy Qingyi settings from API & diagnostics.",
  ],
  setupReview: [
    "先检查设备，按需调整后端和模型；下载任务可取消，刷新后可以继续。",
    "Check your device and choose a backend and model. Downloads can be cancelled and resumed after refresh.",
  ],
  setupBusy: [
    "后台任务正在进行，进度和恢复操作见任务记录。",
    "A task is running. Progress and recovery actions are in task history.",
  ],
  setupCheck: ["检查运行时来源与模型", "Check runtime source & models"],
  fallback_cpu: ["改用 CPU 恢复", "Recover using CPU"],
  degraded_cpu: ["降级到 CPU", "Using CPU fallback"],
  logs: ["管理器日志", "Manager logs"],
  refreshLogs: ["刷新日志", "Refresh logs"],
  empty_response: [
    "推理未返回有效译文，请检查模型。",
    "Inference returned no text. Check the selected model.",
  ],
  engine_missing: [
    "运行时文件已丢失，请重新下载。",
    "Runtime files are missing. Download the runtime again.",
  ],
  engine_start_failed: [
    "无法启动引擎，请查看详细错误并检查运行时依赖。",
    "The engine could not start. Check runtime dependencies and the details below.",
  ],
  task_in_progress: [
    "已有任务进行中，请等待完成或先取消。",
    "Another task is in progress. Wait for it or cancel it first.",
  ],
  installation_in_progress: [
    "正在安装，请等待完成或取消后重试。",
    "An installation is running. Wait or cancel it before retrying.",
  ],
  runtime_stopped: [
    "模型已停止，请先启动或开启按需启动。",
    "The model is stopped. Start it or enable on-demand startup.",
  ],
  invalid_idle_minutes: [
    "空闲停止时间须为 0 至 1440 分钟。",
    "Idle timeout must be between 0 and 1440 minutes.",
  ],
  invalid_mirror: [
    "镜像须为 HTTPS 地址前缀，不含账号、查询参数或片段。",
    "Use an HTTPS mirror prefix without credentials, a query or a fragment.",
  ],
  model_not_installed: [
    "该模型修订尚未安装，请下载或选择已安装版本。",
    "This model revision is not installed. Download it or choose an installed revision.",
  ],
  settings_write_failed: [
    "无法保存设置，请检查数据目录权限与剩余空间。",
    "Settings could not be saved. Check data directory permissions and free space.",
  ],
  official_digest_missing: [
    "上游没有提供可信摘要，已停止安装。请检查官方资产。",
    "Upstream did not provide a trusted digest. Installation stopped. Check the official asset.",
  ],
  asset_missing: [
    "此版本没有适用于所选平台和后端的资产。",
    "No asset matches this platform and backend in this release.",
  ],
  update_signature_invalid: [
    "更新签名无效，已拒绝安装，请重新获取官方更新。",
    "Update signature is invalid. Installation was refused. Fetch the official update again.",
  ],
  already_current: [
    "当前版本无需更新。",
    "This version does not need an update.",
  ],
  engine_connection_failed: [
    "引擎连接中断，请查看状态并重新启动模型。",
    "The engine connection failed. Check its status and restart the model.",
  ],
  rollbackModel: ["切回上一模型", "Restore previous model"],
  installedEngines: ["已安装运行时", "Installed runtimes"],
  previous_model_missing: [
    "没有可回滚的模型。",
    "No previous model is installed.",
  ],
  download_connection_failed: [
    "下载连接失败，可重试或配置镜像。",
    "Download connection failed. Retry or configure a mirror.",
  ],
  download_interrupted: [
    "下载中断，重试将从断点继续。",
    "Download interrupted. Retry to resume.",
  ],
  download_http_error: [
    "下载服务器返回错误，可重试或配置镜像。",
    "The download server returned an error. Retry or configure a mirror.",
  ],
  download_idle_timeout: [
    "下载长时间无响应，可重试或配置镜像。",
    "Download stalled. Retry or configure a mirror.",
  ],
  digest_mismatch: [
    "文件摘要不符，已丢弃，请重试。",
    "Digest mismatch. Corrupt data was discarded. Retry.",
  ],
  operation_failed: [
    "操作失败，详细原因如下。",
    "Operation failed. See details below.",
  ],
  rolled_back: [
    "更新失败，已自动切回上一版本。",
    "The update failed. The previous version was restored.",
  ],
  complete: ["更新完成。", "Update complete."],
  rollback_failed: [
    "自动回滚失败，请重新运行安装包。",
    "Rollback failed. Run the installer again.",
  ],
};
for (const [k, [zh, en]] of Object.entries(extra)) {
  messages["zh-CN"][k] = zh;
  messages["en-US"][k] = en;
}
let lang = localStorage.getItem("mistral-language") || "",
  current,
  assets,
  page = "overview",
  stream,
  firstLanguage = !lang,
  updateAvailable = false;
let detected = false;
const pendingButtons = new Set();
const desktopMode =
  location.hostname === "wails.localhost" ||
  location.protocol === "wails:" ||
  new URLSearchParams(location.search).get("desktop") === "1";
const nativeCall = desktopMode
  ? (await import("/wails/runtime.js")).Call
  : null;
let managerEndpoint = location.origin;
const deploymentTests = new Map();
function nativeCommand(kind, value = "") {
  return nativeCall.ByName(
    "github.com/shusfun/mistral-gui/internal/desktop.Service.Command",
    kind,
    value,
    "00-" +
      crypto.randomUUID().replaceAll("-", "") +
      "-" +
      crypto.randomUUID().replaceAll("-", "").slice(0, 16) +
      "-01",
  );
}
async function confirmNative(kind, message) {
  return desktopMode ? await nativeCommand(kind) : window.confirm(message);
}
async function actionError(error) {
  notice(error.message);
  if (desktopMode) await nativeCommand("error", error.message);
}
const t = (k) => messages[lang || "zh-CN"][k] || k;
const errorText = (code) =>
  messages[lang || "zh-CN"][code] || t("operation_failed") + " (" + code + ")";
function language(l) {
  lang = l;
  localStorage.setItem("mistral-language", l);
  document.documentElement.lang = l;
  $("language").value = l;
  for (const e of document.querySelectorAll("[data-t]"))
    e.textContent = t(e.dataset.t);
  $("page-title").textContent = t(page);
  if (current) render(current);
}
function notice(s) {
  $("notice").textContent = s;
}
async function api(path, body, method) {
  const trace =
    "00-" +
    crypto.randomUUID().replaceAll("-", "") +
    "-" +
    crypto.randomUUID().replaceAll("-", "").slice(0, 16) +
    "-01";
  const r = await fetch(path, {
    method: method || (body === undefined ? "GET" : "POST"),
    headers: {
      traceparent: trace,
      ...(body === undefined ? {} : { "Content-Type": "application/json" }),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  let d;
  try {
    d = await r.json();
  } catch {
    d = {};
  }
  if (!r.ok) {
    const code = d.error?.code || "networkError";
    if (r.status === 401 && !$("login-dialog").open)
      $("login-dialog").showModal();
    throw Error(errorText(code));
  }
  return d;
}
function bind(id, fn) {
  $(id).onclick = async () => {
    const b = $(id);
    pendingButtons.add(id);
    b.disabled = true;
    notice("");
    try {
      await fn();
    } catch (e) {
      await actionError(e);
    } finally {
      pendingButtons.delete(id);
      b.disabled = false;
      if (current) renderControls(current);
    }
  };
}
function go(name) {
  page = name;
  for (const e of document.querySelectorAll(".page")) e.hidden = e.id !== name;
  for (const e of document.querySelectorAll("[data-page]"))
    e.classList.toggle("selected", e.dataset.page === name);
  $("page-title").textContent = t(name);
  location.hash = name;
}
document
  .querySelectorAll("[data-page],[data-go]")
  .forEach((e) => (e.onclick = () => go(e.dataset.page || e.dataset.go)));
function renderControls(d) {
  const busy = ["starting", "loading", "stopping"].includes(d.status.state),
    running = d.status.state === "running";
  $("start").disabled = busy || running;
  $("stop").disabled = d.status.state === "stopped";
  $("restart").disabled = busy;
  $("open-ui").disabled = !running;
  $("test").disabled = !running;
  $("install-update").disabled = !updateAvailable;
  $("fallback-cpu").hidden =
    d.status.state !== "failed" ||
    !["cuda", "metal"].includes(d.status.backend || d.settings.backend);
  for (const id of pendingButtons) $(id).disabled = true;
}
function render(d) {
  current = d;
  renderDeployments(d.deployments || []);
  const state =
    d.status.state === "running" && d.status.degraded_cpu
      ? "degraded_cpu"
      : d.status.state === "stopped" &&
          (!d.settings.engine || !d.model_revision)
        ? "uninstalled"
        : d.status.state;
  $("runtime-state").textContent = t(state);
  $("runtime-state").className =
    "badge " +
    (d.status.state === "running"
      ? "good"
      : d.status.state === "failed"
        ? "bad"
        : "");
  $("current-model").textContent = d.status.model || d.settings.model || "—";
  $("current-backend").textContent = [
    d.status.backend || d.settings.backend,
    d.settings.engine_version,
  ]
    .filter(Boolean)
    .join(" · ");
  $("runtime-error").textContent = d.status.error
    ? errorText(d.status.error) +
      (d.status.detail ? "\n" + d.status.detail : "")
    : "";
  $("requests").textContent = d.status.requests;
  $("active").textContent = d.status.active;
  $("platform").textContent = d.platform;
  $("version").textContent = d.version;
  renderControls(d);
  renderTasks(d.operations || []);
  renderSetup(d);
}
function setupKey(d) {
  return d.model_revision + ":" + d.settings.engine;
}
function setupStep(d) {
  if (
    localStorage.getItem("mistral-setup-tested") === setupKey(d) &&
    d.model_revision
  )
    return 6;
  if (!detected) return 0;
  if (!assets && !d.settings.engine) return 1;
  if (!d.settings.model) return 2;
  if (!d.settings.engine || !d.model_revision) return 3;
  if (d.status.state !== "running") return 4;
  return 5;
}
function renderSetup(d) {
  const step = setupStep(d),
    busy = (d.operations || []).some((o) => o.state === "running"),
    labels = [
      "detect",
      "setupCheck",
      "setupChooseModel",
      !d.settings.engine ? "installEngine" : "downloadModel",
      "start",
      "test",
      "openUI",
    ];
  $("welcome").hidden = step === 6;
  for (const [i, e] of [...$("steps").children].entries()) {
    e.classList.toggle("done", i < step);
    e.classList.toggle("current", i === step);
    if (i === step) e.setAttribute("aria-current", "step");
    else e.removeAttribute("aria-current");
  }
  $("setup-next").textContent = t(labels[step]);
  $("setup-next").disabled = busy || pendingButtons.has("setup-next");
  $("setup-detail").textContent = t(
    busy ? "setupBusy" : step === 6 ? "setupReady" : "setupReview",
  );
}
bind("setup-next", async () => {
  const step = setupStep(current);
  if (step === 0) {
    await detectDevice();
    return;
  }
  if (step === 1) {
    go("models");
    $("refresh-assets").click();
    return;
  }
  if (step === 2) {
    go("models");
    return;
  }
  if (step === 3) {
    go("models");
    $(current.settings.engine ? "install-model" : "install-engine").click();
    return;
  }
  if (step === 4) {
    $("start").click();
    return;
  }
  if (step === 5) {
    $("test").click();
    return;
  }
  $("open-ui").click();
});
function renderTasks(ops) {
  const root = $("tasks");
  if (!ops.length) {
    root.textContent = t("emptyTasks");
    return;
  }
  if (!root.children.length) root.textContent = "";
  const ordered = [...ops].reverse();
  for (const [index, op] of ordered.entries()) {
    let row = root.querySelector('[data-id="' + op.id + '"]');
    if (!row) {
      row = document.createElement("div");
      row.className = "row";
      row.dataset.id = op.id;
      const head = document.createElement("div");
      head.className = "task-head";
      head.append(
        document.createElement("strong"),
        document.createElement("span"),
      );
      const detail = document.createElement("div");
      detail.className = "task-step";
      const button = document.createElement("button");
      button.className = "secondary";
      row.append(head, document.createElement("progress"), detail, button);
    }
    row.querySelector("strong").textContent = t(op.kind);
    const state = row.querySelector("span");
    state.className = "badge " + (op.state === "failed" ? "bad" : "");
    state.textContent = t(op.state);
    const progress = row.querySelector("progress");
    progress.max = op.total || 1;
    if (op.total) progress.value = op.progress;
    else if (op.state === "success") progress.value = 1;
    else progress.removeAttribute("value");
    row.querySelector(".task-step").textContent = [
      op.error
        ? errorText(op.error) + (op.detail ? " · " + op.detail : "")
        : [
            t(op.step),
            op.source,
            op.total
              ? (op.progress / 1048576).toFixed(1) +
                " / " +
                (op.total / 1048576).toFixed(1) +
                " MiB"
              : "",
          ]
            .filter(Boolean)
            .join(" · "),
      op.trace_id ? op.trace_id.slice(3, 35) : "",
    ]
      .filter(Boolean)
      .join(" · ");
    const b = row.querySelector("button"),
      canRetry =
        op.state !== "success" &&
        ["start", "restart", "install"].includes(op.kind);
    b.hidden = op.state !== "running" && !canRetry;
    b.textContent = t(op.state === "running" ? "cancel" : "retry");
    b.onclick = () =>
      api(
        op.state === "running"
          ? "/api/v1/tasks/cancel"
          : op.kind === "install"
            ? "/api/v1/assets/install"
            : "/api/v1/runtime/start",
        op.state === "running"
          ? { id: op.id }
          : op.kind === "install"
            ? { kind: op.asset_kind, id: op.asset_id }
            : { id: op.deployment_id || "" },
      ).catch((e) => notice(e.message));
    if (root.children[index] !== row)
      root.insertBefore(row, root.children[index] || null);
  }
  for (const row of [...root.children])
    if (!ordered.some((op) => op.id === row.dataset.id)) row.remove();
}
async function load() {
  const d = await api("/api/v1/status");
  managerEndpoint = d.endpoint || location.origin;
  if (firstLanguage && d.settings.language) {
    firstLanguage = false;
    language(d.settings.language);
  }
  render(d);
  if (!$("model-repositories").children.length) {
    const catalog = await api("/api/v1/catalog");
    for (const repo of catalog.repositories) {
      const o = document.createElement("option");
      o.value = repo.id;
      $("model-repositories").append(o);
    }
  }
  $("backend").value = d.settings.backend;
  $("idle").value = d.settings.idle_minutes;
  $("auto-start").checked = d.settings.auto_start;
  $("mirrors").value = (d.settings.mirrors || []).join("\n");
  $("connection-state").textContent = t("connected");
  $("connection-state").className = "badge good";
  const data = await api("/api/v1/models");
  await populateInstalled(data);
  const root = $("installed-models");
  root.replaceChildren();
  for (const m of data.models || []) {
    if (m.kind !== "model") continue;
    const row = document.createElement("div");
    row.className = "row";
    const title = document.createElement("strong");
    title.textContent = m.name || m.id;
    row.append(title);
    const meta = document.createElement("p");
    meta.className = "muted";
    meta.textContent =
      t(m.source) +
      " · " +
      (m.size / 1048576).toFixed(0) +
      " MiB · " +
      m.version.slice(0, 12);
    row.append(meta);
    for (const [label, action] of [
      [
        "selectModel",
        () =>
          api("/api/v1/settings", { model: m.id, model_version: m.version }),
      ],
      [
        "deleteModel",
        async () => {
          if (await confirmNative("confirm-delete", t("confirmDelete")))
            await api("/api/v1/models/delete", {
              id: m.id,
              version: m.version,
            });
        },
      ],
    ]) {
      const b = document.createElement("button");
      b.className = "secondary";
      b.textContent = t(label);
      b.onclick = async () => {
        try {
          await action();
          await load();
        } catch (e) {
          notice(e.message);
        }
      };
      row.append(b);
    }
    root.append(row);
  }
  if (!root.children.length) root.textContent = t("emptyModels");
  const engines = $("installed-engines");
  engines.replaceChildren();
  for (const m of data.models || []) {
    if (m.kind !== "engine") continue;
    const row = document.createElement("div");
    row.className = "row";
    const title = document.createElement("p");
    title.textContent = m.version + " · " + m.backend + " · " + t(m.source);
    const b = document.createElement("button");
    b.className = "secondary";
    b.textContent = t("selectModel");
    b.disabled = m.path === d.settings.engine;
    b.onclick = async () => {
      try {
        await api("/api/v1/runtime/select", {
          id: m.id,
          version: m.version,
          backend: m.backend,
        });
        await load();
      } catch (e) {
        notice(e.message);
      }
    };
    row.append(title, b);
    engines.append(row);
  }
  if (!engines.children.length) engines.textContent = t("uninstalled");
  if (d.update_result)
    $("update-info").textContent =
      t(d.update_result.state) +
      (d.update_result.error && d.update_result.error !== "<nil>"
        ? " · " + d.update_result.error
        : "");
  const tok = await api("/api/v1/token");
  $("token").value = tok.token;
  $("endpoint").value = managerEndpoint + "/v1/chat/completions";
  $("api-example").textContent =
    "curl " +
    managerEndpoint +
    "/v1/chat/completions\n  -H 'Authorization: Bearer YOUR_TOKEN'\n  -H 'Content-Type: application/json'\n  -d " +
    JSON.stringify({
      model: d.settings.model,
      messages: [{ role: "user", content: "Hello" }],
    });
  if (!stream) {
    stream = new EventSource("/api/v1/events");
    stream.addEventListener("status", (e) => {
      const v = JSON.parse(e.data);
      const completed = (v.operations || []).some(
        (op) =>
          op.state !== "running" &&
          current.operations?.some(
            (old) => old.id === op.id && old.state === "running",
          ),
      );
      render({ ...current, status: v.status, operations: v.operations });
      $("connection-state").textContent = t("connected");
      if (completed) load().catch((e) => notice(e.message));
    });
    stream.onerror = () => {
      $("connection-state").textContent = t("disconnected");
      $("connection-state").className = "badge bad";
    };
  }
}
bind("start", () => api("/api/v1/runtime/start", {}));
bind("stop", () => api("/api/v1/runtime/stop", {}));
bind("restart", () => api("/api/v1/runtime/restart", {}));
bind("rollback-engine", async () => {
  await api("/api/v1/runtime/rollback", {});
  await load();
});
bind("fallback-cpu", () => api("/api/v1/runtime/fallback-cpu", {}));
bind("save-backend", async () => {
  await api("/api/v1/settings", { backend: $("backend").value });
  notice(t("saved"));
  await load();
});
async function refreshAssets(refresh = false) {
  $("engine-info").textContent = t("checking");
  assets = await api(
    "/api/v1/assets?repository=" +
      encodeURIComponent($("model-repository").value) +
      (refresh ? "&refresh=1" : ""),
  );
  $("engine-info").textContent = assets.errors?.engine
    ? errorText(assets.errors.engine)
    : assets.engine.name +
      " · " +
      assets.engine.version +
      " · " +
      t(assets.engine.source) +
      " · " +
      (assets.engine.size / 1048576).toFixed(1) +
      " MiB";
  if (assets.errors?.models) notice(errorText(assets.errors.models));
  filterCatalog();
}
bind("refresh-assets", () => refreshAssets());
bind("install-engine", () =>
  api("/api/v1/assets/install", { kind: "engine", id: "" }),
);
bind("install-model", () =>
  api("/api/v1/assets/install", {
    kind: "model",
    id: $("available-model").value,
    repository: $("model-repository").value,
  }),
);
bind("rollback-model", async () => {
  await api("/api/v1/models/rollback", {});
  await load();
});
bind("save-settings", async () => {
  await api("/api/v1/settings", {
    auto_start: $("auto-start").checked,
    idle_minutes: Number($("idle").value),
    mirrors: $("mirrors")
      .value.split("\n")
      .map((s) => s.trim())
      .filter(Boolean),
  });
  notice(t("saved"));
});
bind("open-ui", () => {
  if (desktopMode) return nativeCommand("open-ui");
  window.open("/ui", "_blank", "noopener");
  return Promise.resolve();
});
bind("test", async () => {
  const start = performance.now(),
    key = setupKey(current),
    d = await api("/v1/chat/completions", {
      model: current.settings.model,
      messages: [{ role: "user", content: "Translate into Chinese: Hello." }],
      max_tokens: 32,
      stream: false,
    });
  const text = d.choices?.[0]?.message?.content;
  if (typeof text !== "string" || !text.trim())
    throw Error(t("empty_response"));
  $("test-result").textContent = text;
  $("test-latency").textContent =
    ((performance.now() - start) / 1000).toFixed(2) + " s";
  localStorage.setItem("mistral-setup-tested", key);
  render(current);
});
async function detectDevice() {
  const d = await api("/api/v1/devices"),
    dl = $("device-info");
  detected = true;
  $("vc-help").hidden = d.vc_runtime !== false;
  dl.replaceChildren();
  for (const [key, value] of Object.entries(d)) {
    const dt = document.createElement("dt"),
      dd = document.createElement("dd");
    dt.textContent = key;
    dd.textContent =
      typeof value === "boolean"
        ? t(value ? "available" : "unavailable")
        : Array.isArray(value)
          ? value.join(" · ") || "—"
          : t(String(value));
    dl.append(dt, dd);
  }
  if (current) renderSetup(current);
}
bind("detect", detectDevice);
bind("load-logs", async () => {
  const d = await api("/api/v1/diagnostics");
  $("log-output").textContent = d.logs || "—";
});
bind("copy-config", async () => {
  await navigator.clipboard.writeText(
    JSON.stringify(
      {
        localUrl: managerEndpoint,
        localToken: $("token").value,
        localModel: $("api-instance").value || current.settings.model,
        localBackend: "auto",
        routing: "local-first",
        localAutoStart: true,
      },
      null,
      2,
    ),
  );
  notice(t("copied"));
});
bind("copy-token", async () => {
  await navigator.clipboard.writeText($("token").value);
  notice(t("copied"));
});
bind("reveal-token", () => {
  $("token").type = $("token").type === "password" ? "text" : "password";
  return Promise.resolve();
});
bind("rotate-token", async () => {
  if (await confirmNative("confirm-rotate", t("confirmRotate"))) {
    const d = await api("/api/v1/token/rotate", {});
    $("token").value = d.token;
  }
});
bind("export", async () => {
  if (desktopMode) return nativeCommand("export");
  const d = await api("/api/v1/diagnostics"),
    a = document.createElement("a"),
    url = URL.createObjectURL(
      new Blob([JSON.stringify(d, null, 2)], { type: "application/json" }),
    );
  a.href = url;
  a.download = "mistral-gui-diagnostics.json";
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
});
bind("check-update", async () => {
  const d = await api("/api/v1/update/check", {});
  updateAvailable = d.available;
  $("update-info").textContent = d.available
    ? [d.manifest.version, d.manifest.notes].filter(Boolean).join(" · ")
    : t("noUpdate");
  $("install-update").disabled = !d.available;
});
bind("install-update", () =>
  desktopMode ? nativeCommand("update") : api("/api/v1/update/install", {}),
);
bind("quit", async () => {
  if (desktopMode) return nativeCommand("quit");
  await api("/api/v1/manager/quit", {});
  stream?.close();
  notice(t("stopped"));
});
$("language").onchange = () => {
  language($("language").value);
  api("/api/v1/settings", { language: lang })
    .then(load)
    .catch(() => {});
};
for (const b of document.querySelectorAll("[data-language]"))
  b.onclick = () => {
    firstLanguage = false;
    language(b.dataset.language);
    $("language-dialog").hidden = true;
    api("/api/v1/settings", { language: lang })
      .then(load)
      .catch(() => {});
  };
bind("login", async () => {
  const r = await fetch("/api/v1/session", {
    method: "POST",
    headers: { Authorization: "Bearer " + $("login-token").value },
  });
  if (!r.ok) {
    $("login-error").textContent = t("invalid_token");
    return;
  }
  $("login-token").value = "";
  $("login-dialog").close();
  await load();
});
async function boot() {
  const params = new URLSearchParams(location.hash.slice(1));
  const token = params.get("token");
  if (token) {
    history.replaceState(null, "", "/manage");
    await fetch("/api/v1/session", {
      method: "POST",
      headers: { Authorization: "Bearer " + token },
    });
    if (params.has("ui")) {
      location.replace("/ui?instance=" + encodeURIComponent(params.get("ui")));
      return;
    }
  }
  language(lang || "zh-CN");
  if (
    ["overview", "models", "device", "connection", "updates"].includes(
      location.hash.slice(1),
    )
  )
    go(location.hash.slice(1));
  try {
    await load();
    if (firstLanguage) $("language-dialog").hidden = false;
  } catch (e) {
    if (!desktopMode && !$("login-dialog").open) $("login-dialog").showModal();
    else notice(e.message);
  }
}
const moreMessages = {
  deployments: ["部署实例", "Deployments"],
  deploymentHint: [
    "每个实例独立启停。数量由你决定，资源提示不会限制数量。",
    "Each instance starts and stops independently. Resource estimates do not limit instance count.",
  ],
  editDeployment: ["创建／编辑部署", "Create / edit deployment"],
  deploymentID: ["部署 ID", "Deployment ID"],
  contextLength: ["上下文长度", "Context length"],
  sequenceCount: ["并发序列", "Concurrent sequences"],
  gpuLayers: ["GPU 层数（0 自动）", "GPU layers (0 = auto)"],
  isq: ["加载时量化（safetensors）", "In-situ quantization (safetensors)"],
  recommend: ["自动推荐", "Recommend settings"],
  saveDeployment: ["保存部署", "Save deployment"],
  catalog: ["模型目录", "Model catalog"],
  repository: ["模型仓库", "Model repository"],
  search: ["搜索", "Search"],
  format: ["格式", "Format"],
  quantization: ["量化筛选", "Quantization filter"],
  refreshCatalog: ["刷新仓库文件", "Refresh repository files"],
  importGGUF: ["导入 GGUF", "Import GGUF"],
  importDirectory: ["导入 safetensors 目录", "Import safetensors directory"],
  customRepositoryHint: [
    "可填写自定义仓库，需与当前引擎兼容；不会执行仓库中的 Python 代码。",
    "Custom repositories must be compatible with the engine. Repository Python code is never executed.",
  ],
  edit: ["编辑", "Edit"],
  removeDeployment: ["删除部署配置", "Remove deployment"],
  memory_budget_exceeded: [
    "估算内存超出当前可用内存；可手动调整并继续。",
    "Estimated memory exceeds current available memory. You can adjust settings and continue.",
  ],
  memory_available_unknown: [
    "无法确定当前可用内存。",
    "Available memory could not be determined.",
  ],
  local: ["本地导入", "Local import"],
};
for (const [k, [zh, en]] of Object.entries(moreMessages)) {
  messages["zh-CN"][k] = zh;
  messages["en-US"][k] = en;
}
let recommendation;
function filterCatalog() {
  const select = $("available-model");
  select.replaceChildren();
  const query = $("model-search").value.toLowerCase(),
    format = $("model-format").value,
    quant = $("model-quantization").value.toLowerCase();
  for (const m of assets?.models || []) {
    if (
      (query && !m.name.toLowerCase().includes(query)) ||
      (format && m.format !== format) ||
      (quant && !m.name.toLowerCase().includes(quant))
    )
      continue;
    const opt = document.createElement("option");
    opt.value = m.id;
    opt.textContent =
      m.name + " · " + (m.size / 1073741824).toFixed(2) + " GiB";
    select.append(opt);
  }
}
for (const id of ["model-search", "model-format", "model-quantization"])
  $(id).oninput = filterCatalog;
bind("load-catalog", () => refreshAssets(true));
for (const kind of ["import-gguf", "import-directory"]) {
  $(kind).hidden = !desktopMode;
  bind(kind, async () => {
    await nativeCommand(kind);
    await load();
  });
}
function editDeployment(d) {
  $("deployment-id").value = d.id;
  $("deployment-model").value = d.model + "@" + d.model_version;
  $("deployment-backend").value = d.backend;
  $("deployment-context").value = d.tuning?.context || 4096;
  $("deployment-sequences").value = d.tuning?.sequences || 1;
  $("deployment-gpu").value = d.tuning?.gpu_layers || 0;
  $("deployment-isq").value = d.tuning?.isq || "";
  $("deployment-util").value = d.tuning?.gpu_memory_utilization || 0;
  $("deployment-kv").value = d.tuning?.kv_cache || "";
  $("deployment-batch").value = d.tuning?.batch_tokens || 0;
  $("deployment-idle").value = d.idle_minutes;
  $("deployment-auto").checked = d.auto_start;
  recommendation = undefined;
  deploymentCapabilities().catch((e) => notice(e.message));
}
let deploymentsKey = "";
function renderDeployments(items) {
  const key = JSON.stringify([lang, items, [...deploymentTests]]);
  if (key === deploymentsKey) return;
  deploymentsKey = key;
  const root = $("deployments"),
    selection = $("api-instance").value;
  root.replaceChildren();
  $("api-instance").replaceChildren();
  for (const d of items) {
    const option = document.createElement("option");
    option.value = d.id;
    option.textContent = d.id + " · " + t(d.status.state);
    $("api-instance").append(option);
    const row = document.createElement("div");
    row.className = "row";
    const title = document.createElement("strong");
    title.textContent = d.id + " · " + t(d.status.state) + " · " + d.backend;
    const detail = document.createElement("p");
    detail.className = "muted";
    detail.textContent = [
      d.model,
      d.path,
      d.status.error ? errorText(d.status.error) : "",
      d.status.detail,
      deploymentTests.get(d.id),
    ]
      .filter(Boolean)
      .join(" · ");
    row.append(title, detail);
    for (const action of [
      "start",
      "stop",
      "restart",
      "test",
      "openUI",
      "edit",
      "rollbackModel",
      "removeDeployment",
    ]) {
      if (
        (action === "edit" ||
          action === "removeDeployment" ||
          action === "rollbackModel") &&
        d.id === current.settings.model
      )
        continue;
      if (action === "rollbackModel" && !d.previous_model) continue;
      const b = document.createElement("button");
      b.className = "secondary";
      b.textContent = t(action);
      b.onclick = async () => {
        b.disabled = true;
        try {
          if (action === "test") {
            const start = performance.now();
            const result = await api("/v1/chat/completions", {
              model: d.id,
              messages: [
                { role: "user", content: "Translate into Chinese: Hello." },
              ],
              max_tokens: 48,
              stream: false,
              enable_thinking: false,
            });
            const content = result.choices?.[0]?.message?.content;
            if (!content?.trim()) throw Error(t("empty_response"));
            deploymentTests.set(
              d.id,
              content +
                " · " +
                ((performance.now() - start) / 1000).toFixed(2) +
                " s",
            );
          } else if (action === "edit") editDeployment(d);
          else if (action === "rollbackModel")
            await api("/api/v1/deployments/rollback", { id: d.id });
          else if (action === "removeDeployment") {
            if (await confirmNative("confirm-delete", t("confirmDelete")))
              await api("/api/v1/deployments/delete", { id: d.id });
          } else if (action === "openUI") {
            if (desktopMode) await nativeCommand("open-ui", d.id);
            else
              window.open(
                "/ui?instance=" + encodeURIComponent(d.id),
                "_blank",
                "noopener",
              );
          } else await api("/api/v1/runtime/" + action, { id: d.id });
          await load();
        } catch (e) {
          await actionError(e);
        } finally {
          b.disabled = false;
        }
      };
      row.append(b);
    }
    root.append(row);
  }
  if ([...$("api-instance").options].some((o) => o.value === selection))
    $("api-instance").value = selection;
}
async function populateInstalled(data) {
  data ||= await api("/api/v1/models");
  const selected = $("deployment-model").value;
  $("deployment-model").replaceChildren();
  for (const m of data.models || []) {
    if (m.kind !== "model") continue;
    const o = document.createElement("option");
    o.value = m.id + "@" + m.version;
    o.textContent = (m.name || m.id) + " · " + m.version.slice(0, 8);
    o.dataset.format = m.format || "gguf";
    $("deployment-model").append(o);
  }
  if ([...$("deployment-model").options].some((o) => o.value === selected))
    $("deployment-model").value = selected;
}
bind("recommend-deployment", async () => {
  const [model, version] = $("deployment-model").value.split("@");
  recommendation = await api(
    "/api/v1/recommend?model=" +
      encodeURIComponent(model) +
      "&version=" +
      encodeURIComponent(version),
  );
  $("deployment-backend").value = recommendation.backend;
  await deploymentCapabilities();
  $("deployment-context").value = recommendation.tuning.context;
  $("deployment-sequences").value = recommendation.tuning.sequences;
  $("deployment-isq").value = recommendation.tuning.isq || "";
  $("recommendation").textContent =
    (recommendation.estimated_bytes / 1073741824).toFixed(2) +
    " GiB · " +
    recommendation.warnings.map(t).join(" · ");
});
bind("save-deployment", async () => {
  if (!recommendation) {
    const [model, version] = $("deployment-model").value.split("@");
    recommendation = await api(
      "/api/v1/recommend?model=" +
        encodeURIComponent(model) +
        "&version=" +
        encodeURIComponent(version),
    );
  }
  if (
    recommendation?.warnings.length &&
    !(await confirmNative(
      "warning-memory",
      recommendation.warnings.map(t).join("\n"),
    ))
  )
    return;
  const [model, model_version] = $("deployment-model").value.split("@");
  await api("/api/v1/deployments", {
    id: $("deployment-id").value.trim(),
    model,
    model_version,
    backend: $("deployment-backend").value,
    auto_start: $("deployment-auto").checked,
    idle_minutes: Number($("deployment-idle").value),
    tuning: {
      context: Number($("deployment-context").value),
      sequences: Number($("deployment-sequences").value),
      gpu_layers: $("deployment-gpu").disabled
        ? 0
        : Number($("deployment-gpu").value),
      isq: $("deployment-isq").disabled ? "" : $("deployment-isq").value,
      gpu_memory_utilization: $("deployment-util").disabled
        ? 0
        : Number($("deployment-util").value),
      kv_cache: $("deployment-kv").disabled
        ? ""
        : $("deployment-kv").value.trim(),
      batch_tokens: $("deployment-batch").disabled
        ? 0
        : Number($("deployment-batch").value),
    },
  });
  notice(t("saved"));
  await load();
});
$("deployment-model").onchange = () => {
  $("deployment-isq").disabled =
    $("deployment-model").selectedOptions[0]?.dataset.format !== "safetensors";
  if ($("deployment-isq").disabled) $("deployment-isq").value = "";
  recommendation = undefined;
  deploymentCapabilities().catch((e) => notice(e.message));
};
async function deploymentCapabilities() {
  const d = await api(
    "/api/v1/capabilities?backend=" +
      encodeURIComponent($("deployment-backend").value),
  );
  for (const [key, id] of Object.entries({
    gpu_layers: "deployment-gpu",
    isq: "deployment-isq",
    gpu_memory_utilization: "deployment-util",
    kv_cache: "deployment-kv",
    batch_tokens: "deployment-batch",
  })) {
    const control = $(id);
    control.closest("label").hidden = !d.features[key];
    control.disabled = !d.features[key];
  }
  if (
    $("deployment-model").selectedOptions[0]?.dataset.format !== "safetensors"
  ) {
    $("deployment-isq").disabled = true;
    $("deployment-isq").closest("label").hidden = true;
    $("deployment-isq").value = "";
  }
}
for (const [key, id, label, type, value] of [
  [
    "gpu_memory_utilization",
    "deployment-util",
    "gpuUtilization",
    "number",
    "0",
  ],
  ["kv_cache", "deployment-kv", "kvCache", "text", ""],
  ["batch_tokens", "deployment-batch", "batchTokens", "number", "0"],
]) {
  const field = document.createElement("label"),
    span = document.createElement("span"),
    control = document.createElement("input");
  span.dataset.t = label;
  control.id = id;
  control.type = type;
  control.value = value;
  if (type === "number") {
    control.min = "0";
    control.step = key === "gpu_memory_utilization" ? "0.05" : "1";
  }
  field.append(span, control);
  $("deployment-gpu").closest(".form-grid").append(field);
}
for (const [k, zh, en] of [
  [
    "gpuUtilization",
    "GPU 内存占用上限比例（0 自动）",
    "GPU memory utilization (0 = auto)",
  ],
  ["kvCache", "KV cache 容量（如 512MiB）", "KV cache capacity (e.g. 512MiB)"],
  ["batchTokens", "批处理 Token 数（0 自动）", "Batch tokens (0 = auto)"],
]) {
  messages["zh-CN"][k] = zh;
  messages["en-US"][k] = en;
}
$("deployment-backend").onchange = () =>
  deploymentCapabilities().catch((e) => notice(e.message));
boot()
  .then(async () => {
    const d = await api("/api/v1/catalog");
    for (const repo of d.repositories) {
      const o = document.createElement("option");
      o.value = repo.id;
      $("model-repositories").append(o);
    }
    await populateInstalled();
    await deploymentCapabilities();
    if (desktopMode) await nativeCommand("ready");
  })
  .catch((e) => notice(e.message));
setInterval(async () => {
  if ($("login-dialog").open) return;
  try {
    const d = await api("/api/v1/status");
    render(d);
  } catch {}
}, 10000);
