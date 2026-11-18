import { useEffect, useRef, useState } from "preact/hooks";
import {
  Badge,
  Button,
  FeedbackBanner,
  Select,
  Textarea,
  VerdictHero,
  confirmAction,
  feedbackStore,
  formatBytes,
  resolveError,
  runTracked,
  toast,
  useFeedbackStore,
} from "../shared/design-system";
import sharedRules from "../shared/translation-rules.json";
import { ApiError, api, isDesktopMode, nativeCommand } from "../api";
import { toUiLang, type ManagerLocale } from "../i18n";
import type {
  DesktopTranslateResponse,
  RouteId,
  StatusResponse,
} from "../types";

interface CropBox {
  x0: number;
  y0: number;
  x1: number;
  y1: number;
}

export function TranslatePage({
  data,
  locale,
  pendingScreenshot,
  onClearPendingScreenshot,
  onRefresh,
  onNavigate,
}: {
  data: StatusResponse;
  locale: ManagerLocale;
  pendingScreenshot: string | null;
  onClearPendingScreenshot: () => void;
  onRefresh: () => Promise<void>;
  onNavigate: (route: RouteId) => void;
}) {
  const store = useFeedbackStore();
  const uiLang = toUiLang(locale);
  const en = uiLang === "en";

  const fileInputRef = useRef<HTMLInputElement>(null);
  const imgRef = useRef<HTMLImageElement>(null);

  const [sourceText, setSourceText] = useState("");
  const [imageDataUrl, setImageDataUrl] = useState<string | null>(null);
  const [cropBox, setCropBox] = useState<CropBox | null>(null);
  const [draggingCrop, setDraggingCrop] = useState(false);

  const [routing, setRouting] = useState<
    "local-first" | "local-only" | "cloud-only"
  >(data.settings.translate_routing || "local-first");
  const [targetLang, setTargetLang] = useState("zh-CN");
  const [selectedDeployment, setSelectedDeployment] = useState("");
  const [result, setResult] = useState<DesktopTranslateResponse | null>(null);

  useEffect(() => {
    if (pendingScreenshot) {
      setImageDataUrl(pendingScreenshot);
      setCropBox(null);
      setResult(null);
      onClearPendingScreenshot();
    } else if (isDesktopMode) {
      void nativeCommand<{ image_base64?: string } | null>("consume-screenshot")
        .then((shot) => {
          if (shot?.image_base64) {
            setImageDataUrl(shot.image_base64);
            setCropBox(null);
            setResult(null);
          }
        })
        .catch(() => {});
    }
  }, [pendingScreenshot]);

  const readFileAsDataUrl = (file: File) => {
    const reader = new FileReader();
    reader.onload = () => {
      if (typeof reader.result === "string") {
        setImageDataUrl(reader.result);
        setCropBox(null);
      }
    };
    reader.readAsDataURL(file);
  };

  const handlePaste = (e: ClipboardEvent) => {
    const items = e.clipboardData?.items;
    if (!items) return;
    for (const item of Array.from(items)) {
      if (item.type.startsWith("image/")) {
        const f = item.getAsFile();
        if (f) {
          e.preventDefault();
          readFileAsDataUrl(f);
          toast(
            en ? "Image pasted from clipboard." : "已粘贴剪贴板图片，可直接拖选区域或点击识别翻译。",
            "info",
          );
          return;
        }
      }
    }
  };

  const extractCroppedBase64 = (): string | undefined => {
    if (!imageDataUrl) return undefined;
    const img = imgRef.current;
    if (!img || !cropBox) return imageDataUrl;
    const rect = img.getBoundingClientRect();
    const minX = Math.min(cropBox.x0, cropBox.x1);
    const minY = Math.min(cropBox.y0, cropBox.y1);
    const w = Math.abs(cropBox.x1 - cropBox.x0);
    const h = Math.abs(cropBox.y1 - cropBox.y0);
    if (w < 12 || h < 12 || rect.width <= 0 || rect.height <= 0) {
      return imageDataUrl;
    }
    const scaleX = img.naturalWidth / rect.width;
    const scaleY = img.naturalHeight / rect.height;
    const canvas = document.createElement("canvas");
    canvas.width = Math.max(1, Math.round(w * scaleX));
    canvas.height = Math.max(1, Math.round(h * scaleY));
    const ctx = canvas.getContext("2d");
    if (!ctx) return imageDataUrl;
    ctx.drawImage(
      img,
      Math.round(minX * scaleX),
      Math.round(minY * scaleY),
      canvas.width,
      canvas.height,
      0,
      0,
      canvas.width,
      canvas.height,
    );
    return canvas.toDataURL("image/png");
  };

  const handleCaptureScreen = () =>
    runTracked(
      { key: "translate:capture", scope: "translate", lang: uiLang },
      async () => {
        const shot = await nativeCommand<{ image_base64: string }>(
          "capture-screen",
        );
        if (shot?.image_base64) {
          setImageDataUrl(shot.image_base64);
          setCropBox(null);
          setResult(null);
          toast(
            en
              ? "Captured desktop screen. Drag on the image to select a region, or click Translate!"
              : "已捕获当前屏幕！可在图片上拖拽框选目标区域，再点击开始翻译。",
            "info",
          );
        }
      },
    );

  const handleInstallOcr = () =>
    runTracked(
      { key: "translate:ocr-install", scope: "translate", lang: uiLang },
      async () => {
        const ok = await confirmAction({
          message: en
            ? `Download and install the local RapidOCR (PP-OCRv4 ONNX) runtime (~${formatBytes(data.ocr.download_bytes, uiLang)}) into your mistral-gui data folder?`
            : `确认下载并安装本地离线 OCR 识别引擎（RapidOCR PP-OCRv4，下载体积约 ${formatBytes(data.ocr.download_bytes, uiLang)}）吗？文件仅存放在本机数据目录，随时可一键删除。`,
          lang: uiLang,
        });
        if (!ok) return;
        await api("/api/v1/ocr/install", {});
        await onRefresh();
        feedbackStore.setBanner("translate", {
          tone: "info",
          message: en
            ? "Downloading & verifying local OCR runtime in background (view progress in Activity)…"
            : "已开始在后台下载并校验本地 OCR 运行时，可在「任务记录」查看实时进度。",
        });
      },
    );

  const runTranslation = async (confirmCloudImage = false) => {
    await runTracked(
      { key: "translate:run", scope: "translate", lang: uiLang },
      async () => {
        const finalImg = extractCroppedBase64();
        if (!finalImg && !sourceText.trim()) {
          feedbackStore.setBanner("translate", {
            tone: "warning",
            message: en
              ? "Paste text or provide an image/screenshot first."
              : "请先输入或粘贴需要翻译的文字，或粘贴/截取图片。",
          });
          return;
        }
        if (finalImg && !data.ocr.installed) {
          await handleInstallOcr();
          return;
        }

        try {
          const res = await api<DesktopTranslateResponse>("/api/v1/translate", {
            text: finalImg ? "" : sourceText,
            image_base64: finalImg || "",
            source_lang: "auto",
            target_lang: targetLang,
            routing,
            model: selectedDeployment,
            confirm_cloud_image: confirmCloudImage,
          });
          setResult(res);
          if (res.ocr_text && !sourceText.trim()) {
            setSourceText(res.ocr_text);
          }
          if (res.fallback_reason) {
            const fb = resolveError(res.fallback_reason, uiLang);
            feedbackStore.setBanner("translate", {
              tone: "info",
              message: en
                ? `Local model unavailable (${fb.message}); translated via configured cloud fallback.`
                : `本地模型暂不可用（${fb.message}），本次已通过云端回退完成翻译。`,
            });
          } else {
            feedbackStore.clearBanner("translate");
          }
          await onRefresh();
        } catch (err) {
          if (
            err instanceof ApiError &&
            err.message === "cloud_image_confirmation_required"
          ) {
            const ok = await confirmAction({
              title: en ? "Cloud Routing Privacy Notice" : "云端回退隐私确认",
              message: en
                ? "Image OCR runs locally on your device, and current routing ('local-first' / 'cloud-only') may send the extracted text to your configured cloud API if local inference is unavailable. Allow this?"
                : "图片 OCR 文字识别始终在本机离线完成；但当前所选策略（本地优先 / 仅云端）在本地模型不可用时会将识别出的文字发送至你配置的云端服务。确认允许吗？（若不希望离开本机，可切换为「仅本地」。）",
              lang: uiLang,
            });
            if (ok) {
              await runTranslation(true);
            }
            return;
          }
          throw err;
        }
      },
    );
  };

  const fullTranslatedText = (result?.segments || [])
    .map((s) => s.translation || s.source)
    .join("\n\n");

  const channelLabel = (ch?: string) => {
    switch (ch) {
      case "revision":
        return en ? "User Revision" : "人工修订命中";
      case "dictionary":
        return en ? "Dictionary Pack" : "离线词库命中";
      case "local":
        return en ? "Local Model" : "本地模型推理";
      case "cloud":
        return en ? "Cloud API" : "云端模型服务";
      default:
        return ch || "—";
    }
  };

  return (
    <section
      class="panel mg-section"
      onPaste={(e) => handlePaste(e as unknown as ClipboardEvent)}
      onDragOver={(e) => e.preventDefault()}
      onDrop={(e) => {
        e.preventDefault();
        const file = e.dataTransfer?.files?.[0];
        if (file && file.type.startsWith("image/")) {
          readFileAsDataUrl(file);
        }
      }}
    >
      <VerdictHero
        eyebrow={en ? "DESKTOP & SCREENSHOT TRANSLATOR" : "桌面文本、图片与截图翻译"}
        title={
          en
            ? "Translate clipboard text, images, or screen regions offline-first"
            : "粘贴文本、拖入图片或一键框选屏幕截图，本地离线优先翻译"
        }
        description={
          en
            ? `Synced dictionary: ${data.dictionary_sync.revisions} revision(s) · ${data.dictionary_sync.pack_entries} pack entry(ies) · OCR: ${data.ocr.installed ? "Installed" : "On-demand download"}`
            : `已共享轻译词库：人工修订 ${data.dictionary_sync.revisions} 条 · 词库 ${data.dictionary_sync.pack_entries} 条 · 本地 OCR：${data.ocr.installed ? "已安装就绪" : "首次使用时按需下载"}`
        }
        badge={
          <Badge tone={routing === "local-only" ? "success" : "neutral"}>
            {routing === "local-only"
              ? en
                ? "100% Local-only"
                : "纯本地不联网"
              : routing === "local-first"
                ? en
                  ? "Local-first"
                  : "本地优先"
                : en
                  ? "Cloud-only"
                  : "仅云端"}
          </Badge>
        }
        actions={
          <>
            {isDesktopMode && data.ocr.platform_capture ? (
              <Button
                variant="primary"
                busy={store.isBusy("translate:capture")}
                onClick={handleCaptureScreen}
              >
                {en ? "Capture Screen & Crop" : "截取屏幕区域翻译"}
              </Button>
            ) : null}
            <input
              ref={fileInputRef}
              type="file"
              accept="image/png,image/jpeg,image/webp"
              style={{ display: "none" }}
              onChange={(e) => {
                const f = (e.currentTarget as HTMLInputElement).files?.[0];
                if (f) readFileAsDataUrl(f);
                if (fileInputRef.current) fileInputRef.current.value = "";
              }}
            />
            <Button onClick={() => fileInputRef.current?.click()}>
              {en ? "Choose image (or Ctrl+V)…" : "选择图片文件（支持 Ctrl+V 粘贴）…"}
            </Button>
            {!data.ocr.installed ? (
              <Button
                busy={store.isBusy("translate:ocr-install")}
                onClick={handleInstallOcr}
              >
                {en
                  ? `Download Local OCR (~${formatBytes(data.ocr.download_bytes, uiLang)})`
                  : `下载本地 OCR 引擎（约 ${formatBytes(data.ocr.download_bytes, uiLang)}）`}
              </Button>
            ) : null}
            {routing !== "local-only" && !data.has_cloud_key ? (
              <Button variant="ghost" onClick={() => onNavigate("updates")}>
                {en ? "Configure Cloud Fallback" : "配置云端回退服务"}
              </Button>
            ) : null}
          </>
        }
      />

      <FeedbackBanner scope="translate" lang={uiLang} />

      <div class="grid-3" style={{ marginBottom: "14px" }}>
        <Select
          label={en ? "Routing Policy" : "调用策略"}
          value={routing}
          options={[
            {
              value: "local-first",
              label: en
                ? "Local-first (cloud fallback)"
                : "本地优先（不可用时回退云端）",
            },
            {
              value: "local-only",
              label: en
                ? "Local-only (never leaves device)"
                : "仅本地（绝不发送外部请求）",
            },
            {
              value: "cloud-only",
              label: en ? "Cloud-only" : "仅云端服务",
            },
          ]}
          onValueChange={(v) => setRouting(v as any)}
        />
        <Select
          label={en ? "Target Language" : "目标语言"}
          value={targetLang}
          options={sharedRules.targetLanguages.map((item) => ({
            value: item.code,
            label: en ? `${item.enLabel} (${item.code})` : `${item.zhLabel}（${item.code}）`,
          }))}
          onValueChange={(v) => setTargetLang(v)}
        />
        <Select
          label={en ? "Local Deployment" : "指定本地部署实例"}
          value={selectedDeployment}
          options={[
            {
              value: "",
              label: en
                ? `Default (${data.settings.model})`
                : `默认实例（${data.settings.model}）`,
            },
            ...data.deployments
              .filter((d) => !d.default)
              .map((d) => ({
                value: d.id,
                label: `${d.id} (${d.model})`,
              })),
          ]}
          onValueChange={(v) => setSelectedDeployment(v)}
        />
      </div>

      {imageDataUrl ? (
        <div class="card" style={{ marginBottom: "14px" }}>
          <div class="split-head" style={{ marginBottom: "8px" }}>
            <strong>
              {en
                ? "Image / Screenshot Region (drag to crop target area)"
                : "待识别图片 / 屏幕截图（按住鼠标左键拖拽可框选局部区域）"}
            </strong>
            <div class="actions" style={{ marginTop: 0 }}>
              {cropBox ? (
                <Button variant="ghost" onClick={() => setCropBox(null)}>
                  {en ? "Full image" : "取消框选（识别整图）"}
                </Button>
              ) : null}
              <Button
                variant="danger"
                onClick={() => {
                  setImageDataUrl(null);
                  setCropBox(null);
                }}
              >
                {en ? "Remove image" : "移除图片"}
              </Button>
            </div>
          </div>
          <div
            class="mg-crop-stage"
            onPointerDown={(e) => {
              const rect = (
                e.currentTarget as HTMLElement
              ).getBoundingClientRect();
              const x = e.clientX - rect.left;
              const y = e.clientY - rect.top;
              (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
              setDraggingCrop(true);
              setCropBox({ x0: x, y0: y, x1: x, y1: y });
            }}
            onPointerMove={(e) => {
              if (!draggingCrop) return;
              const rect = (
                e.currentTarget as HTMLElement
              ).getBoundingClientRect();
              const x = Math.max(0, Math.min(rect.width, e.clientX - rect.left));
              const y = Math.max(0, Math.min(rect.height, e.clientY - rect.top));
              setCropBox((prev) => (prev ? { ...prev, x1: x, y1: y } : null));
            }}
            onPointerUp={() => setDraggingCrop(false)}
          >
            <img ref={imgRef} src={imageDataUrl} alt="OCR source" />
            {cropBox &&
            Math.abs(cropBox.x1 - cropBox.x0) >= 8 &&
            Math.abs(cropBox.y1 - cropBox.y0) >= 8 ? (
              <div
                class="mg-crop-rect"
                style={{
                  left: `${Math.min(cropBox.x0, cropBox.x1)}px`,
                  top: `${Math.min(cropBox.y0, cropBox.y1)}px`,
                  width: `${Math.abs(cropBox.x1 - cropBox.x0)}px`,
                  height: `${Math.abs(cropBox.y1 - cropBox.y0)}px`,
                }}
              />
            ) : null}
          </div>
        </div>
      ) : null}

      <div class="grid-2">
        <div>
          <Textarea
            label={en ? "Source Text (or OCR output)" : "原文输入（或图片识别出的文字）"}
            rows={8}
            placeholder={
              en
                ? "Paste paragraphs from PDF, Word, IDE, or press Ctrl+V to paste a screenshot…"
                : "粘贴来自 PDF、Word、终端或聊天软件的段落，或直接按 Ctrl+V 粘贴截图…"
            }
            value={sourceText}
            onValueInput={(v) => setSourceText(v)}
          />
          <div class="actions">
            <Button
              variant="primary"
              busy={store.isBusy("translate:run")}
              onClick={() => void runTranslation(false)}
            >
              {imageDataUrl
                ? en
                  ? "Recognize (OCR) & Translate"
                  : "识别图片文字并翻译"
                : en
                  ? "Translate"
                  : "开始翻译"}
            </Button>
            <Button
              variant="ghost"
              onClick={() => {
                setSourceText("");
                setImageDataUrl(null);
                setCropBox(null);
                setResult(null);
              }}
            >
              {en ? "Clear" : "清空"}
            </Button>
          </div>
        </div>

        <div>
          <Textarea
            label={
              result?.ocr_ms
                ? en
                  ? `Translation (OCR took ${result.ocr_ms} ms)`
                  : `译文结果（本地 OCR 耗时 ${result.ocr_ms} ms）`
                : en
                  ? "Translation Output"
                  : "译文结果"
            }
            rows={8}
            readOnly
            placeholder={
              en
                ? "Translated paragraphs will appear here…"
                : "译文将显示在此处，并标注每段命中的翻译通道…"
            }
            value={fullTranslatedText}
          />
          <div class="actions">
            <Button
              disabled={!fullTranslatedText}
              onClick={async () => {
                await navigator.clipboard.writeText(fullTranslatedText);
                toast(en ? "Translation copied." : "已复制全部译文。", "success");
              }}
            >
              {en ? "Copy translation" : "复制译文"}
            </Button>
          </div>
        </div>
      </div>

      {result && result.segments.length > 0 ? (
        <fieldset>
          <legend>
            {en ? "Paragraph Attribution" : "逐段来源与通道明细"}
          </legend>
          <div class="rows">
            {result.segments.map((seg, i) => (
              <div key={i} class="list-row">
                <div class="list-row-main">
                  <p class="hint" style={{ margin: "0 0 4px" }}>
                    {seg.source}
                  </p>
                  {seg.translation ? (
                    <strong>{seg.translation}</strong>
                  ) : (
                    <span class="field-error">
                      {resolveError(seg.error || "operation_failed", uiLang).message}
                    </span>
                  )}
                </div>
                <Badge
                  tone={
                    seg.channel === "revision" || seg.channel === "dictionary"
                      ? "success"
                      : seg.error
                        ? "error"
                        : "neutral"
                  }
                >
                  {seg.error
                    ? en
                      ? "Failed"
                      : "未完成"
                    : channelLabel(seg.channel)}
                </Badge>
              </div>
            ))}
          </div>
        </fieldset>
      ) : null}
    </section>
  );
}
