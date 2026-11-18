import {
  Badge,
  Button,
  DataList,
  FeedbackBanner,
  IconRefresh,
  MetricGrid,
  SettingGroup,
  SettingRow,
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
        ? "x86_64"
        : arch === "arm64"
          ? "ARM64"
          : arch || "";
    return `${osLabel} · ${archLabel}`;
  };

  const gpuItems = deviceInfo?.gpu_details?.length
    ? deviceInfo.gpu_details.map((g, idx) => ({
        key: `${g.name}-${idx}`,
        title: g.name,
        subtitle: `${g.driver} · ${en ? "VRAM" : "显存"}: ${g.total_bytes > 0 ? formatBytes(g.total_bytes, uiLang) : t(locale, "unknown")}${g.available_bytes > 0 ? ` (${en ? "Free" : "可用"} ${formatBytes(g.available_bytes, uiLang)})` : ""}`,
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
      <div class="mg-page-head">
        <div class="mg-page-title-group">
          <h1 class="mg-page-title">{t(locale, "navDevice")}</h1>
          <Badge
            tone={
              deviceInfo?.cuda || deviceInfo?.metal ? "success" : "neutral"
            }
          >
            {deviceInfo?.cuda
              ? "CUDA"
              : deviceInfo?.metal
                ? "Metal"
                : "CPU"}
          </Badge>
        </div>
        <Button
          size="sm"
          icon={<IconRefresh size={13} />}
          busy={store.isBusy("device:detect")}
          onClick={() =>
            void runTracked(
              { key: "device:detect", scope: "device", lang: uiLang },
              onDetect,
            )
          }
        >
          {en ? "Re-detect" : "重新检测"}
        </Button>
      </div>

      <FeedbackBanner scope="device" lang={uiLang} />

      <MetricGrid
        items={[
          {
            label: en ? "Platform" : "系统与架构",
            value: formatOsArch(deviceInfo?.os, deviceInfo?.arch),
          },
          {
            label: en ? "Logical CPU" : "逻辑核心数",
            value: deviceInfo?.cpu_cores
              ? en
                ? `${deviceInfo.cpu_cores} cores`
                : `${deviceInfo.cpu_cores} 核`
              : t(locale, "unknown"),
          },
          {
            label: en ? "Total RAM" : "系统总内存",
            value:
              deviceInfo?.memory_bytes && deviceInfo.memory_bytes > 0
                ? formatBytes(deviceInfo.memory_bytes, uiLang)
                : t(locale, "unknown"),
          },
          {
            label: en ? "Available RAM" : "当前可用内存",
            value:
              deviceInfo?.available_bytes && deviceInfo.available_bytes > 0
                ? formatBytes(deviceInfo.available_bytes, uiLang)
                : t(locale, "unknown"),
          },
        ]}
      />

      <SettingGroup title={en ? "Backend Compatibility" : "加速后端可用性"}>
        <SettingRow
          title="CPU"
          description={
            en
              ? "Universal x86_64 and ARM64 execution"
              : "通用处理器执行，无需额外显卡驱动"
          }
        >
          <Badge tone="success">{t(locale, "available")}</Badge>
        </SettingRow>

        <SettingRow
          title="NVIDIA CUDA"
          description={
            deviceInfo?.cuda
              ? en
                ? "NVIDIA driver detected"
                : "已检测到可用 NVIDIA 显卡驱动"
              : deviceInfo?.cuda_reason
                ? resolveError(deviceInfo.cuda_reason, uiLang).message
                : t(locale, "unavailable")
          }
        >
          <Badge tone={deviceInfo?.cuda ? "success" : "neutral"}>
            {deviceInfo?.cuda
              ? t(locale, "available")
              : t(locale, "unavailable")}
          </Badge>
        </SettingRow>

        {deviceInfo?.os === "darwin" ? (
          <SettingRow
            title="Apple Metal"
            description={
              en
                ? "Apple Silicon unified memory GPU acceleration"
                : "Apple Silicon 统一内存 GPU 加速"
            }
          >
            <Badge tone={deviceInfo.metal ? "success" : "neutral"}>
              {deviceInfo.metal
                ? t(locale, "available")
                : t(locale, "unavailable")}
            </Badge>
          </SettingRow>
        ) : null}

        {deviceInfo?.os === "windows" ? (
          <SettingRow
            title={
              en
                ? "Microsoft VC++ Redistributable (x64)"
                : "Microsoft VC++ 运行库（vcruntime140）"
            }
            description={
              deviceInfo.vc_runtime
                ? en
                  ? "Verified in System32"
                  : "系统已安装所需运行库"
                : en
                  ? "Missing vcruntime140.dll — install Microsoft VC++ x64 Redistributable if startup fails"
                  : "未检测到 vcruntime140.dll，若引擎无法启动请安装微软 VC++ x64 运行库"
            }
          >
            <Badge tone={deviceInfo.vc_runtime ? "success" : "warning"}>
              {deviceInfo.vc_runtime
                ? t(locale, "available")
                : t(locale, "unavailable")}
            </Badge>
          </SettingRow>
        ) : null}
      </SettingGroup>

      <SettingGroup
        title={en ? "Graphics Adapters" : "图形显卡适配器（DXGI / 驱动）"}
        padded
      >
        <DataList
          items={gpuItems}
          getKey={(g) => g.key}
          emptyMessage={
            en
              ? "No dedicated graphics adapters reported."
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
      </SettingGroup>
    </section>
  );
}
