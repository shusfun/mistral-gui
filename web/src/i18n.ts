import type { UiLang } from "./shared/design-system";

export type ManagerLocale = "zh-CN" | "en-US";

export function toUiLang(locale?: string | null): UiLang {
  return (locale || "").toLowerCase().startsWith("en") ? "en" : "zh-CN";
}

export function normalizeManagerLocale(raw?: string | null): ManagerLocale {
  return (raw || "").toLowerCase().startsWith("en") ? "en-US" : "zh-CN";
}

const DICT = {
  "zh-CN": {
    workspaceSubtitle: "本地模型与桌面翻译工作台",
    navOverview: "概览",
    navTranslate: "翻译工作台",
    navInstances: "部署实例",
    navModels: "模型与运行时",
    navDevice: "硬件与后端",
    navConnection: "API 与诊断",
    navActivity: "任务记录",
    navUpdates: "更新与设置",
    connected: "已连接",
    connecting: "连接中",
    unauthorized: "待鉴权",
    stoppedManager: "已退出",
    minimizeWindow: "最小化窗口",
    maximizeWindow: "最大化或还原窗口",
    closeWindow: "隐藏到托盘",
    chooseLangTitle: "选择界面语言 / Choose Interface Language",
    chooseLangDesc: "随时可在「更新与设置」中切换界面语言。",
    loginTitle: "连接到本机 mistral-gui",
    loginHint:
      "请从系统托盘打开（自动一次性免密登录），或粘贴本机管理器的访问 Token。",
    tokenLabel: "访问 Token",
    connectBtn: "连接管理器",
    cancelBtn: "取消",

    // Runtime & Operation States (M-12, M-32)
    state_stopped: "已停止",
    state_starting: "启动中",
    state_loading: "加载模型中",
    state_running: "运行中",
    state_stopping: "停止中",
    state_failed: "启动失败",
    state_cooldown: "崩溃冷却保护",
    state_uninstalled: "待安装资产",
    state_success: "已完成",
    state_cancelled: "已取消",
    state_interrupted: "重启中断",

    // Operation Steps (M-12)
    step_checking: "正在检查元数据",
    step_downloading: "正在下载",
    step_verifying: "正在校验完整性",
    step_extracting: "正在安全解压",
    step_importing: "正在导入模型文件",
    step_starting: "正在启动推理引擎",
    step_update_ready: "更新包校验就绪，正在切换重启",
    step_preparing: "正在准备运行时与模型",

    // Runtime Loading Stages (M-R1)
    stage_starting_process: "正在拉起推理子进程",
    stage_loading_weights: "正在加载模型权重到内存/显存",
    stage_warming_up: "正在建立推理服务监听",

    // Operation Kinds
    kind_install: "下载与安装资产",
    kind_import: "本地模型导入",
    kind_prepare: "部署准备与启动",
    kind_fallback_cpu: "回退切换至 CPU",
    kind_update: "管理器自更新",
    kind_ocr_install: "安装本地 OCR 识别引擎",

    // Backends & Sources (M-35)
    backend_auto: "自动选择（Auto）",
    backend_cpu: "CPU 通用推理",
    backend_cuda: "NVIDIA CUDA 加速",
    backend_metal: "Apple Metal 加速",
    source_official: "官方 Release",
    source_supplement: "签名补充构建",
    source_huggingface: "Hugging Face",
    source_local: "本地导入",

    // Units & Fallbacks (M-34)
    unknown: "未知",
    available: "可用",
    unavailable: "不可用",
    suitable: "流畅适配",
    tight: "内存偏紧",
    unsuitable: "超出可用内存",
  },
  "en-US": {
    workspaceSubtitle: "Local Model & Desktop Translation Workbench",
    navOverview: "Overview",
    navTranslate: "Translate",
    navInstances: "Deployments",
    navModels: "Models & Runtime",
    navDevice: "Device & Backend",
    navConnection: "API & Diagnostics",
    navActivity: "Activity",
    navUpdates: "Updates & Settings",
    connected: "Connected",
    connecting: "Connecting",
    unauthorized: "Locked",
    stoppedManager: "Stopped",
    minimizeWindow: "Minimize window",
    maximizeWindow: "Maximize or restore window",
    closeWindow: "Hide to system tray",
    chooseLangTitle: "Choose Interface Language / 选择界面语言",
    chooseLangDesc: "You can switch the interface language anytime in Updates & Settings.",
    loginTitle: "Connect to Local mistral-gui",
    loginHint:
      "Open from the system tray for automatic sign-in, or paste your local manager access token.",
    tokenLabel: "Access Token",
    connectBtn: "Connect",
    cancelBtn: "Cancel",

    state_stopped: "Stopped",
    state_starting: "Starting",
    state_loading: "Loading model",
    state_running: "Running",
    state_stopping: "Stopping",
    state_failed: "Failed",
    state_cooldown: "Crash cooldown",
    state_uninstalled: "Not installed",
    state_success: "Completed",
    state_cancelled: "Cancelled",
    state_interrupted: "Interrupted",

    step_checking: "Checking metadata",
    step_downloading: "Downloading",
    step_verifying: "Verifying digest",
    step_extracting: "Extracting archive",
    step_importing: "Importing model files",
    step_starting: "Starting engine",
    step_update_ready: "Update verified & restarting",
    step_preparing: "Preparing runtime & weights",

    stage_starting_process: "Spawning engine process",
    stage_loading_weights: "Loading weights into RAM/VRAM",
    stage_warming_up: "Warming up HTTP server",

    kind_install: "Asset Installation",
    kind_import: "Local Model Import",
    kind_prepare: "Deployment Prepare",
    kind_fallback_cpu: "Fallback to CPU",
    kind_update: "Manager Self-Update",
    kind_ocr_install: "Local OCR Engine Install",

    backend_auto: "Auto",
    backend_cpu: "CPU",
    backend_cuda: "NVIDIA CUDA",
    backend_metal: "Apple Metal",
    source_official: "Official Release",
    source_supplement: "Signed Supplement",
    source_huggingface: "Hugging Face",
    source_local: "Local Import",

    unknown: "Unknown",
    available: "Available",
    unavailable: "Unavailable",
    suitable: "Suitable",
    tight: "Tight memory",
    unsuitable: "Insufficient memory",
  },
} as const;

export type DictKey = keyof (typeof DICT)["zh-CN"];

export function t(locale: ManagerLocale, key: DictKey): string {
  return DICT[locale][key] || DICT["zh-CN"][key] || key;
}

export function formatRuntimeState(locale: ManagerLocale, state?: string): string {
  if (!state) return t(locale, "unknown");
  const k = `state_${state}` as DictKey;
  return k in DICT[locale] ? DICT[locale][k] : state;
}

export function formatStep(locale: ManagerLocale, step?: string): string {
  if (!step) return "";
  const k = `step_${step}` as DictKey;
  return k in DICT[locale] ? DICT[locale][k] : step;
}

export function formatStage(locale: ManagerLocale, stage?: string): string {
  if (!stage) return "";
  const k = `stage_${stage}` as DictKey;
  return k in DICT[locale] ? DICT[locale][k] : stage;
}

export function formatOpKind(locale: ManagerLocale, kind?: string): string {
  if (!kind) return "";
  const k = `kind_${kind}` as DictKey;
  return k in DICT[locale] ? DICT[locale][k] : kind;
}

export function formatBackend(locale: ManagerLocale, backend?: string): string {
  if (!backend) return t(locale, "unknown");
  const k = `backend_${backend.toLowerCase()}` as DictKey;
  return k in DICT[locale] ? DICT[locale][k] : backend.toUpperCase();
}

export function formatAssetSource(locale: ManagerLocale, source?: string): string {
  if (!source) return "";
  const k = `source_${source.toLowerCase()}` as DictKey;
  return k in DICT[locale] ? DICT[locale][k] : source;
}
