import {
  Badge,
  Button,
  DataList,
  FeedbackBanner,
  VerdictHero,
  formatBytes,
  resolveError,
  runTracked,
  useFeedbackStore,
} from "../shared/design-system";
import { t, toUiLang, type ManagerLocale } from "../i18n";
import type { DeviceInfoResponse } from "../types";

// M-34: All device fields & memory units localized cleanly without mixed raw English ("unknown" -> "未知")
export function DevicePage({
  deviceInfo,
  locale,
  onDetect,
}: {
  deviceInfo: DeviceInfoResponse | null;
  locale: ManagerLocale;
  onDetect: () => Promise<void>;
}) {
  const store = useFeedbackStore();
  const uiLang = toUiLang(locale);
  const en = uiLang === "en";

  const formatOsArch = (osName?: string, arch?: string) => {
    if (!osName) return t(locale, "unknown");
    const osLabel =
      osName === "windows"
        ? "Windows"
        : osName === "darwin"
          ? "macOS"
          : osName === "linux"
            ? "Linux"
            : osName;
    const archLabel =
      arch === "amd64"
        ? "x86_64 (64-bit)"
        : arch === "arm64"
          ? "ARM64 / Apple Silicon"
          : arch || "";
    return `${osLabel} · ${archLabel}`;
  };

  const gpuItems = deviceInfo?.gpu_details?.length
    ? deviceInfo.gpu_details.map((g, idx) => ({
        key: `${g.name}-${idx}`,
        title: g.name,
        subtitle: `${g.driver} · ${en ? "Total VRAM" : "显存容量"}: ${g.total_bytes > 0 ? formatBytes(g.total_bytes, uiLang) : t(locale, "unknown")}${g.available_bytes > 0 ? ` (${en ? "Available" : "当前可用"} ${formatBytes(g.available_bytes, uiLang)})` : ""}`,
      }))
    : (deviceInfo?.gpus || [])
        .filter((line) => line && line.trim())
        .map((line, idx) => ({
          key: `raw-${idx}`,
          title: line,
          subtitle: "",
        }));

  return (
    <section class="panel mg-section">
      <VerdictHero
        eyebrow={en ? "HARDWARE & ACCELERATION" : "本机硬件与加速后端检测"}
        title={
          deviceInfo
            ? deviceInfo.cuda
              ? en
                ? "NVIDIA CUDA GPU acceleration is available"
                : "检测到可用的 NVIDIA CUDA 硬件加速环境"
              : deviceInfo.metal
                ? en
                  ? "Apple Metal unified-memory acceleration is available"
                  : "检测到可用的 Apple Metal 统一内存硬件加速"
                : en
                  ? "Running in CPU inference mode (GGUF quantized models recommended)"
                  : "当前使用 CPU 通用推理（推荐使用 Q4_K_M 量化模型）"
            : en
              ? "Detecting hardware capabilities…"
              : "正在检测本机硬件与驱动环境…"
        }
        description={
          en
            ? "Detected via OS memory APIs, DXGI graphics adapters, and NVIDIA/Metal drivers."
            : "基于系统物理内存接口、Windows DXGI 显卡适配器枚举与 NVIDIA / Metal 驱动实测。"
        }
        actions={
          <Button
            variant="primary"
            busy={store.isBusy("device:detect")}
            onClick={() =>
              void runTracked(
                { key: "device:detect", scope: "device", lang: uiLang },
                onDetect,
              )
            }
          >
            {en ? "Re-detect hardware" : "重新检测硬件"}
          </Button>
        }
        metrics={[
          {
            label: en ? "Platform" : "操作系统与架构",
            value: formatOsArch(deviceInfo?.os, deviceInfo?.arch),
          },
          {
            label: en ? "Logical CPU Cores" : "逻辑 CPU 核心数",
            value: deviceInfo?.cpu_cores
              ? en
                ? `${deviceInfo.cpu_cores} cores`
                : `${deviceInfo.cpu_cores} 核`
              : t(locale, "unknown"),
          },
          {
            label: en ? "Total System RAM" : "系统总内存",
            value:
              deviceInfo?.memory_bytes && deviceInfo.memory_bytes > 0
                ? formatBytes(deviceInfo.memory_bytes, uiLang)
                : t(locale, "unknown"),
          },
          {
            label: en ? "Available System RAM" : "当前可用内存",
            value:
              deviceInfo?.available_bytes && deviceInfo.available_bytes > 0
                ? formatBytes(deviceInfo.available_bytes, uiLang)
                : t(locale, "unknown"),
          },
        ]}
      />

      <FeedbackBanner scope="device" lang={uiLang} />

      <fieldset>
        <legend>{en ? "Backend Compatibility" : "推理后端可用性"}</legend>
        <div class="rows">
          <div class="list-row">
            <div class="list-row-main">
              <strong>CPU</strong>
              <p class="hint">
                {en
                  ? "Always available on x86_64 and ARM64."
                  : "所有处理器架构通用支持，无需额外显卡驱动。"}
              </p>
            </div>
            <Badge tone="success">{t(locale, "available")}</Badge>
          </div>

          <div class="list-row">
            <div class="list-row-main">
              <strong>NVIDIA CUDA</strong>
              <p class="hint">
                {deviceInfo?.cuda
                  ? en
                    ? "NVIDIA driver detected via nvidia-smi."
                    : "已检测到 NVIDIA 显卡驱动，可使用 CUDA 加速推理。"
                  : deviceInfo?.cuda_reason
                    ? resolveError(deviceInfo.cuda_reason, uiLang).message
                    : t(locale, "unavailable")}
              </p>
            </div>
            <Badge tone={deviceInfo?.cuda ? "success" : "neutral"}>
              {deviceInfo?.cuda ? t(locale, "available") : t(locale, "unavailable")}
            </Badge>
          </div>

          {deviceInfo?.os === "darwin" ? (
            <div class="list-row">
              <div class="list-row-main">
                <strong>Apple Metal</strong>
                <p class="hint">
                  {en
                    ? "Apple Silicon unified memory GPU acceleration."
                    : "Apple Silicon 统一内存 GPU 加速。"}
                </p>
              </div>
              <Badge tone={deviceInfo.metal ? "success" : "neutral"}>
                {deviceInfo.metal ? t(locale, "available") : t(locale, "unavailable")}
              </Badge>
            </div>
          ) : null}

          {deviceInfo?.os === "windows" ? (
            <div class="list-row">
              <div class="list-row-main">
                <strong>
                  {en
                    ? "Microsoft VC++ Redistributable (x64)"
                    : "Microsoft VC++ 运行库（vcruntime140）"}
                </strong>
                <p class="hint">
                  {deviceInfo.vc_runtime
                    ? en
                      ? "Installed and verified in System32."
                      : "系统已安装所需的 VC++ 运行库。"
                    : en
                      ? "Missing vcruntime140.dll — install Microsoft Visual C++ x64 Redistributable if engine startup fails."
                      : "未检测到 vcruntime140.dll，若推理引擎无法启动请安装微软官方 VC++ x64 运行库。"}
                </p>
              </div>
              <Badge tone={deviceInfo.vc_runtime ? "success" : "warning"}>
                {deviceInfo.vc_runtime
                  ? t(locale, "available")
                  : t(locale, "unavailable")}
              </Badge>
            </div>
          ) : null}
        </div>
      </fieldset>

      <fieldset>
        <legend>
          {en
            ? "Detected Graphics Adapters (DXGI / Driver)"
            : "检测到的图形显卡适配器（DXGI / 显卡驱动）"}
        </legend>
        <DataList
          items={gpuItems}
          getKey={(g) => g.key}
          emptyMessage={
            en
              ? "No dedicated/hardware graphics adapters reported."
              : "未枚举到独立或硬件加速显卡适配器。"
          }
          lang={uiLang}
          renderItem={(g) => (
            <div class="list-row-main">
              <strong>{g.title}</strong>
              {g.subtitle ? <p class="hint">{g.subtitle}</p> : null}
            </div>
          )}
        />
      </fieldset>
    </section>
  );
}
