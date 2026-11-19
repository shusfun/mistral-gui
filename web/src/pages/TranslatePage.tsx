import {
  useEffect,
  useMemo,
  useRef,
  useState,
} from "preact/hooks";
import {
  Badge,
  Button,
  CopyButton,
  FeedbackBanner,
  IconCamera,
  IconDownload,
  IconImage,
  IconTranslate,
  IconTrash,
  SegmentedControl,
  Select,
  Textarea,
  confirmAction,
  feedbackStore,
  formatBytes,
  resolveError,
  runTracked,
  useFeedbackStore,
} from "../shared/design-system";
import { rebuildParagraphs } from "../shared/paragraph-rebuild";
import sharedRules from "../shared/translation-rules.json";
import {
  ApiError,
  api,
  isDesktopMode,
  nativeCommand,
  newTrace,
  recordUiTrace,
} from "../api";
import { toUiLang, type ManagerLocale } from "../i18n";
import {
  ImageTranslation,
  type ImageOverlay,
  type ImageView,
} from "../components/ImageTranslation";
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

interface NormalizedCropRect {
  nx0: number;
  ny0: number;
  nx1: number;
  ny1: number;
}

type ResultDisplayMode = "bilingual" | "translation";

interface TranslateOptions {
  useEditedTextOnly?: boolean;
  overridePreserveLines?: boolean;
  overrideText?: string;
  automatic?: boolean;
  decoded?: HTMLImageElement;
  confirmCloudImage?: boolean;
  previousResult?: DesktopTranslateResponse;
}

const fullImage: NormalizedCropRect = { nx0: 0, ny0: 0, nx1: 1, ny1: 1 };
const maxImageBytes = 16 * 1024 * 1024;
const maxImagePixels = 32 * 1024 * 1024;
const maxOcrSide = 4096;

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
  pendingScreenshot: { src: string; trace?: string } | null;
  onClearPendingScreenshot: () => void;
  onRefresh: () => Promise<void>;
  onNavigate: (route: RouteId) => void;
}) {
  const store = useFeedbackStore();
  const uiLang = toUiLang(locale);
  const en = uiLang === "en";

  const fileInputRef = useRef<HTMLInputElement>(null);
  const imageLoad = useRef(0);
  const activeTranslation = useRef<{
    key: string;
    controller: AbortController;
    requestedManually: boolean;
  }>(undefined);
  const autoTimer = useRef<number>(undefined);
  const composing = useRef(false);
  const confirmation = useRef<(() => void) | undefined>(undefined);
  const renderTrace = useRef<string | undefined>(undefined);
  const cropPointer = useRef<number | undefined>(undefined);

  // Source & translation strictly stay in component memory only (never in URL, history, or storage).
  const [sourceText, setSourceText] = useState("");
  const [rawInputBackup, setRawInputBackup] = useState<string | null>(null);
  const [preserveLines, setPreserveLines] = useState(false);
  const [displayMode, setDisplayMode] =
    useState<ResultDisplayMode>("bilingual");

  const [image, setImage] = useState<{
    src: string;
    aspect: number;
    trace: string;
    decoded: HTMLImageElement;
  } | null>(null);
  const [imageView, setImageView] = useState<ImageView>("translated");
  const imageDataUrl = image?.src;
  const [cropBox, setCropBox] = useState<CropBox | null>(null);
  const [lastTranslatedCrop, setLastTranslatedCrop] =
    useState<NormalizedCropRect>(fullImage);

  const [routing, setRouting] = useState<
    "local-first" | "local-only" | "cloud-only"
  >(data.settings.translate_routing || "local-first");
  const [targetLang, setTargetLang] = useState("zh-CN");
  const [selectedDeployment, setSelectedDeployment] = useState("");
  const [result, setResult] = useState<DesktopTranslateResponse | null>(null);
  const [translating, setTranslating] = useState(false);

  const clearAutoTimer = () => window.clearTimeout(autoTimer.current);

  const cancelTranslation = () => {
    clearAutoTimer();
    activeTranslation.current?.controller.abort();
    activeTranslation.current = undefined;
    confirmation.current = undefined;
    setTranslating(false);
    feedbackStore.clearBanner("translate");
  };

  const resetImageResult = () => {
    cancelTranslation();
    composing.current = false;
    cropPointer.current = undefined;
    setCropBox(null);
    setLastTranslatedCrop(fullImage);
    setResult(null);
    setSourceText("");
    setRawInputBackup(null);
    setImageView("translated");
  };

  const loadImage = async (
    read: () => Promise<string | undefined>,
    trace = newTrace(),
  ) => {
    const loadID = ++imageLoad.current;
    cancelTranslation();
    const started = performance.now();
    recordUiTrace(trace, "image.load", "started");
    try {
      const src = await read();
      if (!src || loadID !== imageLoad.current) {
        recordUiTrace(
          trace,
          "image.load",
          "cancelled",
          performance.now() - started,
        );
        return;
      }
      if (src.length > Math.ceil(maxImageBytes / 3) * 4 + 128) {
        throw new ApiError("ocr_image_too_large", 0);
      }
      const decoded = new Image();
      decoded.src = src;
      await decoded.decode();
      if (loadID !== imageLoad.current) {
        recordUiTrace(
          trace,
          "image.decode",
          "cancelled",
          performance.now() - started,
        );
        return;
      }
      if (decoded.naturalWidth * decoded.naturalHeight > maxImagePixels) {
        throw new ApiError("ocr_image_too_large", 0);
      }
      resetImageResult();
      setImage({
        src,
        aspect: decoded.naturalWidth / decoded.naturalHeight,
        trace,
        decoded,
      });
      recordUiTrace(
        trace,
        "image.decode",
        "success",
        performance.now() - started,
      );
    } catch (error) {
      recordUiTrace(trace, "image.load", "failed", performance.now() - started);
      if (loadID !== imageLoad.current) return;
      feedbackStore.setBanner("translate", {
        tone: "error",
        message:
          error instanceof DOMException
            ? en
              ? "Could not read this image."
              : "无法读取这张图片。"
            : resolveError(error, uiLang).message,
      });
    }
  };

  useEffect(() => {
    if (!pendingScreenshot) return;
    void loadImage(async () => pendingScreenshot.src, pendingScreenshot.trace);
    onClearPendingScreenshot();
  }, [pendingScreenshot]);

  useEffect(
    () => () => {
      imageLoad.current++;
      clearAutoTimer();
      activeTranslation.current?.controller.abort();
      confirmation.current = undefined;
      feedbackStore.clearBanner("translate");
    },
    [],
  );

  useEffect(() => {
    if (result && renderTrace.current) {
      const translated = result.segments.some((segment) => segment.translation);
      recordUiTrace(renderTrace.current, "translation.render", translated ? "success" : "failed");
    }
  }, [result]);

  const readFileAsDataUrl = (file: File) =>
    void loadImage(() => {
      if (file.size > maxImageBytes) {
        throw new ApiError("ocr_image_too_large", 0);
      }
      return new Promise<string>((resolve, reject) => {
        const reader = new FileReader();
        reader.onerror = () => reject(reader.error);
        reader.onload = () =>
          typeof reader.result === "string"
            ? resolve(reader.result)
            : reject(new DOMException());
        reader.readAsDataURL(file);
      });
    });

  const handlePaste = (e: ClipboardEvent) => {
    const items = e.clipboardData?.items;
    if (!items) return;
    for (const item of Array.from(items)) {
      if (item.type.startsWith("image/")) {
        const f = item.getAsFile();
        if (f) {
          e.preventDefault();
          readFileAsDataUrl(f);
          return;
        }
      }
    }
  };

  const extractCroppedImage = (decoded?: HTMLImageElement): {
    base64?: string;
    normRect: NormalizedCropRect;
  } => {
    if (!imageDataUrl) return { normRect: fullImage };
    const img = decoded ?? image?.decoded;
    if (!img || !img.naturalWidth) throw new ApiError("ocr_failed", 0);
    const crop = decoded ? null : cropBox;
    const normRect = crop
      ? {
          nx0: Math.min(crop.x0, crop.x1),
          ny0: Math.min(crop.y0, crop.y1),
          nx1: Math.max(crop.x0, crop.x1),
          ny1: Math.max(crop.y0, crop.y1),
        }
      : fullImage;
    const width = (normRect.nx1 - normRect.nx0) * img.naturalWidth;
    const height = (normRect.ny1 - normRect.ny0) * img.naturalHeight;
    if (
      !crop &&
      Math.max(width, height) <= maxOcrSide &&
      imageDataUrl.length < maxImageBytes
    ) {
      return { base64: imageDataUrl, normRect };
    }
    const scale = Math.min(1, maxOcrSide / Math.max(width, height));
    const canvas = document.createElement("canvas");
    canvas.width = Math.max(1, Math.round(width * scale));
    canvas.height = Math.max(1, Math.round(height * scale));
    const ctx = canvas.getContext("2d");
    if (!ctx) throw new ApiError("ocr_failed", 0);
    ctx.drawImage(
      img,
      normRect.nx0 * img.naturalWidth,
      normRect.ny0 * img.naturalHeight,
      width,
      height,
      0,
      0,
      canvas.width,
      canvas.height,
    );
    let base64 = canvas.toDataURL("image/png");
    if (base64.length >= maxImageBytes)
      base64 = canvas.toDataURL("image/jpeg", 0.9);
    return { base64, normRect };
  };

  const handleCaptureScreen = () =>
    runTracked(
      { key: "translate:capture", scope: "translate", lang: uiLang },
      async () => {
        const trace = newTrace();
        await loadImage(
          async () =>
            (
              await nativeCommand<{ image_base64: string }>(
                "capture-screen",
                "",
                trace,
              )
            ).image_base64,
          trace,
        );
      },
    );

  const handleInstallOcr = () =>
    runTracked(
      { key: "translate:ocr-install", scope: "translate", lang: uiLang },
      async () => {
        const ok = await confirmAction({
          message: en
            ? `Download and install local RapidOCR (~${formatBytes(data.ocr.download_bytes, uiLang)})?`
            : `确认下载并安装本地离线 OCR 引擎（RapidOCR PP-OCRv4，约 ${formatBytes(data.ocr.download_bytes, uiLang)}）？`,
          lang: uiLang,
        });
        if (!ok) return;
        await api("/api/v1/ocr/install", {});
        await onRefresh();
        feedbackStore.setBanner("translate", {
          tone: "info",
          message: en
            ? "Downloading local OCR runtime in background…"
            : "正在后台下载本地 OCR 运行时，可在「任务记录」查看进度。",
        });
      },
    );

  const executeTranslate = async (opts?: TranslateOptions) => {
    clearAutoTimer();
    if (opts?.automatic && (routing === "cloud-only" || (image && !data.ocr.installed))) {
      return;
    }
    const useEditedTextOnly = !!opts?.useEditedTextOnly;
    const nextPreserve = opts?.overridePreserveLines ?? preserveLines;
    const textCandidate = opts?.overrideText ?? sourceText;
    const crop = opts?.decoded ? null : cropBox;
    const key = JSON.stringify([
      image?.trace, useEditedTextOnly ? null : crop,
      !image || useEditedTextOnly ? textCandidate : null,
      nextPreserve, targetLang, routing, selectedDeployment, !!opts?.confirmCloudImage,
    ]);
    if (activeTranslation.current?.key === key) {
      if (!opts?.automatic) activeTranslation.current.requestedManually = true;
      return;
    }
    cancelTranslation();
    const controller = new AbortController();
    const request = { key, controller, requestedManually: !opts?.automatic };
    activeTranslation.current = request;
    const obsolete = () => activeTranslation.current !== request;
    const trace = opts?.decoded && image ? image.trace : newTrace();
    try {
      const cropped = useEditedTextOnly
        ? { base64: undefined, normRect: fullImage }
        : extractCroppedImage(opts?.decoded);
      const finalImg = cropped.base64;

      if (!finalImg && !textCandidate.trim()) {
        feedbackStore.setBanner("translate", {
          tone: "warning",
          message: en
            ? "Paste text or provide an image/screenshot first."
            : "请先输入或粘贴需要翻译的内容。",
        });
        return;
      }
      if (finalImg && !data.ocr.installed) {
        await handleInstallOcr();
        return;
      }
      setTranslating(true);
      const submit = (readyOnly: boolean) =>
        api<DesktopTranslateResponse>(
          "/api/v1/translate",
          {
            text: finalImg ? "" : textCandidate,
            image_base64: finalImg || "",
            source_lang: "auto",
            target_lang: targetLang,
            routing,
            model: selectedDeployment,
            preserve_lines: nextPreserve,
            ready_only: readyOnly,
            ocr_derived: !!image && useEditedTextOnly,
            confirm_cloud_image: !!opts?.confirmCloudImage,
          },
          { trace, signal: controller.signal },
        );
      recordUiTrace(trace, opts?.automatic ? "translation.automatic" : "translation.manual", "started");
      const res = await submit(!!opts?.automatic).catch((error) => {
        if (!opts?.automatic || !(error instanceof ApiError) ||
            error.message !== "translation_model_not_ready") throw error;
        return !obsolete() && request.requestedManually ? submit(false) : undefined;
      });
      if (!res || obsolete()) return;

      renderTrace.current = trace;
      const previous = opts?.previousResult;
      setResult(previous ? {
        ...res,
        segments: res.segments.map((segment, index) => {
          const earlier = previous.segments[index];
          return !segment.translation && earlier?.translation && earlier.source === segment.source
            ? earlier : segment;
        }),
      } : res);
      if (finalImg) {
        setLastTranslatedCrop(cropped.normRect);
        const nextSrc = nextPreserve
          ? res.ocr_raw_text || res.ocr_text || ""
          : res.ocr_text || "";
        setSourceText(nextSrc);
        setRawInputBackup(res.ocr_raw_text || res.ocr_text || null);
        setImageView("translated");
      } else if (image) {
        setImageView("original");
      }

      const firstFailure = res.segments.find((segment) => segment.error)?.error;
      if (res.cloud_confirmation_required) {
        const confirm = () => {
          if (confirmation.current !== confirm) return;
          void executeTranslate({
            ...opts,
            overrideText: textCandidate,
            automatic: false,
            confirmCloudImage: true,
            previousResult: res,
          });
        };
        confirmation.current = confirm;
        feedbackStore.setBanner("translate", {
          tone: "warning",
          message: en
            ? "Allow recognized text to be sent to your configured cloud service?"
            : "允许将需要云端翻译的识别文字发给已配置的云端服务吗？",
          actionLabel: en ? "Allow cloud translation" : "允许云端翻译",
          onAction: confirm,
        });
      } else if (firstFailure && !res.segments.some((segment) => segment.translation)) {
        feedbackStore.reportError("translate", firstFailure, uiLang);
      } else if (res.fallback_reason) {
        const fb = resolveError(res.fallback_reason, uiLang);
        feedbackStore.setBanner("translate", {
          tone: "info",
          message: en
            ? `Local model unavailable (${fb.message}); translated via cloud fallback.`
            : `本地模型暂不可用（${fb.message}），已回退云端完成翻译。`,
        });
      } else {
        feedbackStore.clearBanner("translate");
      }
      if (request.requestedManually) void onRefresh().catch(() => {});
    } catch (error) {
      if (!obsolete()) feedbackStore.reportError("translate", error, uiLang);
    } finally {
      if (!obsolete()) {
        activeTranslation.current = undefined;
        setTranslating(false);
      }
    }
  };

  useEffect(() => {
    if (image) void executeTranslate({ automatic: true, decoded: image.decoded });
  }, [image]);

  const updateText = (value: string, delay?: number) => {
    cancelTranslation();
    setResult(null);
    setSourceText(value);
    setRawInputBackup(value);
    if (!value.trim() || delay === undefined) return;
    const translateInput = () => void executeTranslate({ automatic: true, overrideText: value });
    if (delay === 0) translateInput();
    else autoTimer.current = window.setTimeout(translateInput, delay);
  };

  const handleTogglePreserveLines = (next: boolean) => {
    cancelTranslation();
    setPreserveLines(next);
    if (imageDataUrl && result) {
      void executeTranslate({ overridePreserveLines: next });
      return;
    }
    const baseRaw = rawInputBackup ?? sourceText;
    if (baseRaw.trim()) {
      const preview = rebuildParagraphs(baseRaw, { preserveLineBreaks: next });
      const sep = next ? "\n" : "\n\n";
      const nextSrc = preview.map((p) => p.merged).join(sep);
      setSourceText(nextSrc);
      if (result) {
        void executeTranslate({
          useEditedTextOnly: true,
          overridePreserveLines: next,
          overrideText: baseRaw,
        });
      }
    }
  };

  const previewParagraphCount = useMemo(() => {
    if (!sourceText.trim()) return 0;
    return rebuildParagraphs(sourceText, { preserveLineBreaks: preserveLines })
      .length;
  }, [sourceText, preserveLines]);

  const fullTranslatedText = (result?.segments || [])
    .map((segment) => segment.translation)
    .filter(Boolean)
    .join("\n\n");

  const imageTranslations: ImageOverlay[] = [];
  const { nx0, ny0, nx1, ny1 } = lastTranslatedCrop;
  for (const { box, source, raw_source, translation } of result?.segments ?? []) {
    if (
      !translation ||
      !box ||
      box.length !== 4 ||
      !box.every((value) => Number.isFinite(value) && value >= 0 && value <= 1) ||
      box[2] <= box[0] ||
      box[3] <= box[1]
    ) continue;
    imageTranslations.push({
      box: [
        nx0 + box[0] * (nx1 - nx0),
        ny0 + box[1] * (ny1 - ny0),
        nx0 + box[2] * (nx1 - nx0),
        ny0 + box[3] * (ny1 - ny0),
      ],
      text: translation,
      sourceLines: Math.max(1, (raw_source || source).split(/\r\n?|\n/).filter((line) => line.trim()).length),
    });
  }

  const previewLabel = en ? "Open in new window" : "独立预览";
  const openImagePreview = async () => {
    if (!image) return;
    const trace = newTrace();
    try {
      await nativeCommand("open-image-preview", JSON.stringify({
        src: image.src,
        overlays: imageTranslations,
        view: imageView,
        theme: document.documentElement.dataset.theme || "system",
        lang: uiLang,
      }), trace);
    } catch {
      feedbackStore.setBanner("translate", {
        tone: "error",
        message: en ? "Could not open the image preview." : "无法打开图片预览。",
      });
    }
  };

  const cropPoint = (event: PointerEvent, stage: HTMLDivElement) => {
    const rect = stage.getBoundingClientRect();
    const clamp = (value: number) => Math.max(0, Math.min(1, value));
    return {
      x: clamp((event.clientX - rect.left) / rect.width),
      y: clamp((event.clientY - rect.top) / rect.height),
    };
  };

  const channelLabel = (ch?: string) => {
    switch (ch) {
      case "revision":
        return en ? "Revision" : "修订";
      case "dictionary":
        return en ? "Dict" : "词库";
      case "local":
        return en ? "Local" : "本地";
      case "cloud":
        return en ? "Cloud" : "云端";
      default:
        return ch || "—";
    }
  };

  const renderModeSwitcherAndCopy = () => {
    const viewOpts: { value: ResultDisplayMode; label: string }[] = [
      { value: "bilingual", label: en ? "Bilingual" : "段落对照" },
      { value: "translation", label: en ? "Translation" : "只看译文" },
    ];
    return (
      <>
        <SegmentedControl
          value={displayMode}
          onChange={setDisplayMode}
          options={viewOpts}
          ariaLabel={en ? "Display mode" : "展示模式"}
        />
        <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
          {result?.ocr_ms ? (
            <span class="hint">OCR {result.ocr_ms}ms</span>
          ) : null}
          <CopyButton
            text={fullTranslatedText}
            label={en ? "Copy" : "复制译文"}
            size="sm"
            lang={uiLang}
          />
        </div>
      </>
    );
  };

  const renderParagraphStream = () => {
    if (!result || result.segments.length === 0) return null;
    return (
      <div class="mg-float-body">
        {result.segments.map((seg, idx) => (
          <div key={idx} class="mg-para-item">
            {displayMode === "bilingual" ? (
              <p class="mg-para-raw">{seg.raw_source || seg.source}</p>
            ) : null}
            {seg.translation ? (
              <div
                style={{
                  display: "flex",
                  justifyContent: "space-between",
                  alignItems: "flex-start",
                  gap: "10px",
                }}
              >
                <p class="mg-para-trans">{seg.translation}</p>
                <Badge
                  tone={
                    seg.channel === "revision" || seg.channel === "dictionary"
                      ? "success"
                      : "neutral"
                  }
                >
                  {channelLabel(seg.channel)}
                </Badge>
              </div>
            ) : (
              <span class="field-error">
                {resolveError(seg.error || "operation_failed", uiLang).message}
              </span>
            )}
          </div>
        ))}
      </div>
    );
  };

  return (
    <section
      class={image ? "panel mg-section mg-image-translate" : "panel mg-section"}
      onPaste={handlePaste}
      onDragOver={(e) => e.preventDefault()}
      onDrop={(e) => {
        e.preventDefault();
        const file = e.dataTransfer?.files?.[0];
        if (file && file.type.startsWith("image/")) {
          readFileAsDataUrl(file);
        }
      }}
    >
      {/* Sleek Unified Translator Control Bar */}
      <div class="mg-translator-bar">
        <div class="mg-translator-bar-left">
          <div class="mg-lang-pill">
            <span>{en ? "Auto" : "自动检测"}</span>
            <span>→</span>
            <div style={{ width: "136px" }}>
              <Select
                value={targetLang}
                options={sharedRules.targetLanguages.map((item) => ({
                  value: item.code,
                  label: en ? item.enLabel : item.zhLabel,
                }))}
                onValueChange={(value) => {
                  if (value === targetLang) return;
                  cancelTranslation();
                  setResult(null);
                  setTargetLang(value);
                }}
              />
            </div>
          </div>

          <SegmentedControl
            value={routing}
            onChange={(value) => {
              if (value === routing) return;
              cancelTranslation();
              setRouting(value);
            }}
            options={[
              { value: "local-first", label: en ? "Local-first" : "本地优先" },
              { value: "local-only", label: en ? "Local-only" : "仅本地" },
              { value: "cloud-only", label: en ? "Cloud" : "仅云端" },
            ]}
          />

          {data.deployments.length > 1 ? (
            <div style={{ width: "156px" }}>
              <Select
                value={selectedDeployment}
                options={[
                  {
                    value: "",
                    label: en ? `Default (${data.settings.model})` : `默认实例`,
                  },
                  ...data.deployments
                    .filter((d) => !d.default)
                    .map((d) => ({
                      value: d.id,
                      label: d.id,
                    })),
                ]}
                onValueChange={(value) => {
                  if (value === selectedDeployment) return;
                  cancelTranslation();
                  setSelectedDeployment(value);
                }}
              />
            </div>
          ) : null}

          <SegmentedControl
            value={preserveLines ? "lines" : "smart"}
            onChange={(m) => handleTogglePreserveLines(m === "lines")}
            options={[
              { value: "smart", label: en ? "Smart Merge" : "智能合并段落" },
              { value: "lines", label: en ? "Line-by-line" : "逐行翻译" },
            ]}
          />
        </div>

        <div class="mg-translator-bar-right">
          {isDesktopMode && data.ocr.platform_capture ? (
            <Button
              variant="primary"
              size="sm"
              icon={<IconCamera size={14} />}
              busy={store.isBusy("translate:capture")}
              onClick={handleCaptureScreen}
            >
              {en ? "Screenshot" : "截图翻译"}
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
          <Button
            size="sm"
            icon={<IconImage size={14} />}
            onClick={() => fileInputRef.current?.click()}
          >
            {en ? "Image" : "选择图片"}
          </Button>
          {!data.ocr.installed ? (
            <Button
              size="sm"
              icon={<IconDownload size={14} />}
              busy={store.isBusy("translate:ocr-install")}
              onClick={handleInstallOcr}
            >
              {en ? "Install OCR" : "安装 OCR"}
            </Button>
          ) : null}
          {routing !== "local-only" && !data.has_cloud_key ? (
            <Button
              size="sm"
              variant="ghost"
              onClick={() => onNavigate("updates")}
            >
              {en ? "Cloud Key" : "配置云端"}
            </Button>
          ) : null}
        </div>
      </div>

      {!image && <FeedbackBanner scope="translate" lang={uiLang} />}

      {image ? (
        <div class="mg-image-workbench">
          <div class="mg-image-preview">
            <div class="mg-studio-head">
              <SegmentedControl
                value={imageTranslations.length ? imageView : "original"}
                onChange={(view: ImageView) => setImageView(view)}
                options={[
                  { value: "original", label: en ? "Original" : "原图" },
                  {
                    value: "translated",
                    label: en ? "Translated" : "译图",
                    disabled: !imageTranslations.length,
                  },
                ]}
                ariaLabel={en ? "Image view" : "图片预览模式"}
              />
              <div class="actions mg-image-actions">
                {isDesktopMode && (
                  <Button size="sm" onClick={() => void openImagePreview()}>
                    {previewLabel}
                  </Button>
                )}
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={!cropBox}
                  onClick={() => {
                    cancelTranslation();
                    setCropBox(null);
                  }}
                >
                  {en ? "Clear crop" : "取消框选"}
                </Button>
                <Button
                  size="sm"
                  variant="primary"
                  icon={<IconTranslate size={14} />}
                  busy={translating && activeTranslation.current?.requestedManually}
                  onClick={() => void executeTranslate()}
                >
                  {en ? "OCR & Translate" : "识别并翻译"}
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  icon={<IconTrash size={14} />}
                  aria-label={en ? "Remove image" : "移除图片"}
                  title={en ? "Remove image" : "移除图片"}
                  onClick={() => {
                    imageLoad.current++;
                    resetImageResult();
                    setImage(null);
                  }}
                />
              </div>
            </div>
            <div class="mg-crop-stage-wrap">
              <div
                class="mg-crop-stage"
                style={{ "--mg-image-ratio": image.aspect }}
                onPointerDown={(event) => {
                  if (event.button !== 0 || !event.isPrimary) return;
                  cancelTranslation();
                  const point = cropPoint(event, event.currentTarget);
                  event.currentTarget.setPointerCapture(event.pointerId);
                  cropPointer.current = event.pointerId;
                  setCropBox({
                    x0: point.x,
                    y0: point.y,
                    x1: point.x,
                    y1: point.y,
                  });
                }}
                onPointerMove={(event) => {
                  if (cropPointer.current !== event.pointerId) return;
                  const point = cropPoint(event, event.currentTarget);
                  setCropBox(
                    (previous) =>
                      previous && { ...previous, x1: point.x, y1: point.y },
                  );
                }}
                onPointerUp={(event) => {
                  if (cropPointer.current !== event.pointerId) return;
                  cropPointer.current = undefined;
                  const point = cropPoint(event, event.currentTarget);
                  const rect = event.currentTarget.getBoundingClientRect();
                  setCropBox((previous) => {
                    if (
                      !previous ||
                      Math.abs(point.x - previous.x0) * rect.width < 8 ||
                      Math.abs(point.y - previous.y0) * rect.height < 8
                    )
                      return null;
                    return { ...previous, x1: point.x, y1: point.y };
                  });
                }}
                onLostPointerCapture={(event) => {
                  if (cropPointer.current === event.pointerId) {
                    cropPointer.current = undefined;
                    setCropBox(null);
                  }
                }}
              >
                <img
                  src={image.src}
                  alt={en ? "Image to translate" : "待翻译图片"}
                  draggable={false}
                />
                {imageView === "translated" &&
                  imageTranslations.map((overlay, index) => (
                    <ImageTranslation key={index} {...overlay} />
                  ))}
                {cropBox && (
                  <div
                    class="mg-crop-rect"
                    aria-hidden="true"
                    style={{
                      left: `${Math.min(cropBox.x0, cropBox.x1) * 100}%`,
                      top: `${Math.min(cropBox.y0, cropBox.y1) * 100}%`,
                      width: `${Math.abs(cropBox.x1 - cropBox.x0) * 100}%`,
                      height: `${Math.abs(cropBox.y1 - cropBox.y0) * 100}%`,
                    }}
                  />
                )}
              </div>
            </div>
          </div>

          <section
            class="mg-image-text-panel"
            aria-labelledby="mg-image-translation-title"
          >
            <div class="mg-studio-head">
              <h2 id="mg-image-translation-title">
                {en ? "Translation" : "译文"}
              </h2>
              <CopyButton
                text={fullTranslatedText}
                label={en ? "Copy all" : "复制全部译文"}
                size="sm"
                lang={uiLang}
              />
            </div>
            <div
              class="mg-image-translation-text"
              tabIndex={0}
              aria-labelledby="mg-image-translation-title"
              aria-busy={translating}
            >
              <FeedbackBanner scope="translate" lang={uiLang} inline />
              {fullTranslatedText || (!store.getBanner("translate") && (
                <span class="hint">
                  {translating ? (en ? "Translating…" : "正在翻译…") : en
                    ? "Translation appears here."
                    : "识别并翻译后，译文显示在这里。"}
                </span>
              ))}
              {fullTranslatedText && result?.segments.some((segment) => segment.error) && (
                <div class="hint">
                  {en
                    ? "Some paragraphs could not be translated."
                    : "部分段落翻译失败。"}
                </div>
              )}
            </div>
          </section>

          <section
            class="mg-image-text-panel"
            aria-labelledby="mg-image-source-title"
          >
            <div class="mg-studio-head">
              <h2 id="mg-image-source-title">
                {en ? "Recognized text" : "识别原文"}
              </h2>
              <Button
                size="sm"
                disabled={!sourceText.trim()}
                busy={translating && activeTranslation.current?.requestedManually}
                onClick={() =>
                  void executeTranslate({ useEditedTextOnly: true })
                }
              >
                {en ? "Re-translate" : "按修改后原文重译"}
              </Button>
            </div>
            <Textarea
              class="mg-image-source-text"
              aria-labelledby="mg-image-source-title"
              placeholder={
                en
                  ? "Recognized text appears here and can be edited."
                  : "识别原文显示在这里，可直接修改。"
              }
              value={sourceText}
              onValueInput={(value) => {
                cancelTranslation();
                setSourceText(value);
                setRawInputBackup(value);
              }}
            />
          </section>
        </div>
      ) : (
        /* Seamless Dual-Pane Text Translator Studio */
        <div class="mg-studio-card">
          <div class="mg-studio-pane">
            <textarea
              class="mg-studio-textarea"
              placeholder={
                en
                  ? "Type or paste text to translate when the local model is ready. Press Ctrl+Enter to translate now."
                  : "输入或粘贴文字，本地模型就绪时会自动翻译。按 Ctrl+Enter 可立即翻译。"
              }
              value={sourceText}
              onCompositionStart={() => {
                composing.current = true;
                cancelTranslation();
                setResult(null);
              }}
              onCompositionEnd={(event) => {
                composing.current = false;
                updateText(event.currentTarget.value, 350);
              }}
              onInput={(event) => {
                const delay = composing.current || event.isComposing
                  ? undefined
                  : event.inputType === "insertFromPaste" ? 0 : 350;
                updateText(event.currentTarget.value, delay);
              }}
              onKeyDown={(event) => {
                if (!composing.current && !event.isComposing &&
                    (event.ctrlKey || event.metaKey) && event.key === "Enter") {
                  event.preventDefault();
                  void executeTranslate({ overrideText: event.currentTarget.value });
                }
              }}
            />
            <div class="mg-studio-foot">
              <span class="hint">
                {sourceText.trim()
                  ? en
                    ? `${previewParagraphCount} paragraph(s) · ${sourceText.length} chars`
                    : `将按 ${previewParagraphCount} 段处理 · ${sourceText.length} 字符`
                  : en
                    ? `Synced ${data.dictionary_sync.pack_entries} dict · ${data.dictionary_sync.revisions} rev`
                    : `已共享词库 ${data.dictionary_sync.pack_entries} 条 · 修订 ${data.dictionary_sync.revisions} 条`}
              </span>
              <div class="actions" style={{ margin: 0 }}>
                {sourceText ? (
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => updateText("")}
                  >
                    {en ? "Clear" : "清空"}
                  </Button>
                ) : null}
                <Button
                  variant="primary"
                  busy={translating && activeTranslation.current?.requestedManually}
                  onClick={() => void executeTranslate()}
                >
                  {en ? "Translate" : "翻译"}
                </Button>
              </div>
            </div>
          </div>

          <div class="mg-studio-pane" aria-busy={translating}>
            <div class="mg-studio-head">{renderModeSwitcherAndCopy()}</div>
            {result && result.segments.length > 0 ? (
              renderParagraphStream()
            ) : (
              <div class="mg-studio-empty">
                <IconTranslate size={24} />
                <span>
                  {translating ? (en ? "Translating…" : "正在翻译…") : en
                    ? "Paragraph-by-paragraph bilingual comparison will appear here"
                    : "译文将在此按段落对照显示，支持切换「只看译文」一键复制"}
                </span>
              </div>
            )}
          </div>
        </div>
      )}
    </section>
  );
}
