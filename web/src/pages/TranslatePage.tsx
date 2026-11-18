import { useEffect, useMemo, useRef, useState } from "preact/hooks";
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
  SettingGroup,
  Textarea,
  confirmAction,
  feedbackStore,
  formatBytes,
  resolveError,
  runTracked,
  toast,
  useFeedbackStore,
} from "../shared/design-system";
import { rebuildParagraphs } from "../shared/paragraph-rebuild";
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

interface NormalizedCropRect {
  nx0: number;
  ny0: number;
  nx1: number;
  ny1: number;
}

type ResultDisplayMode = "bilingual" | "translation" | "overlay";

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

  // Source & translation strictly stay in component memory only (never in URL, history, or storage).
  const [sourceText, setSourceText] = useState("");
  const [rawInputBackup, setRawInputBackup] = useState<string | null>(null);
  const [preserveLines, setPreserveLines] = useState(false);
  const [displayMode, setDisplayMode] =
    useState<ResultDisplayMode>("bilingual");

  const [imageDataUrl, setImageDataUrl] = useState<string | null>(null);
  const [cropBox, setCropBox] = useState<CropBox | null>(null);
  const [lastTranslatedCrop, setLastTranslatedCrop] =
    useState<NormalizedCropRect>({ nx0: 0, ny0: 0, nx1: 1, ny1: 1 });
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
      setSourceText("");
      setRawInputBackup(null);
      setDisplayMode("bilingual");
      onClearPendingScreenshot();
    } else if (isDesktopMode) {
      void nativeCommand<{ image_base64?: string } | null>("consume-screenshot")
        .then((shot) => {
          if (shot?.image_base64) {
            setImageDataUrl(shot.image_base64);
            setCropBox(null);
            setResult(null);
            setSourceText("");
            setRawInputBackup(null);
            setDisplayMode("bilingual");
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
        setResult(null);
        setSourceText("");
        setRawInputBackup(null);
        setDisplayMode("bilingual");
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
            en
              ? "Image pasted. Drag to crop or click Translate."
              : "已粘贴图片，可拖拽框选区域或点击「识别并翻译」。",
            "info",
          );
          return;
        }
      }
    }
  };

  const extractCroppedImage = (): {
    base64?: string;
    normRect: NormalizedCropRect;
  } => {
    const fullRect: NormalizedCropRect = { nx0: 0, ny0: 0, nx1: 1, ny1: 1 };
    if (!imageDataUrl) return { normRect: fullRect };
    const img = imgRef.current;
    if (!img || !cropBox) return { base64: imageDataUrl, normRect: fullRect };
    const rect = img.getBoundingClientRect();
    const minX = Math.min(cropBox.x0, cropBox.x1);
    const minY = Math.min(cropBox.y0, cropBox.y1);
    const w = Math.abs(cropBox.x1 - cropBox.x0);
    const h = Math.abs(cropBox.y1 - cropBox.y0);
    if (w < 12 || h < 12 || rect.width <= 0 || rect.height <= 0) {
      return { base64: imageDataUrl, normRect: fullRect };
    }
    const scaleX = img.naturalWidth / rect.width;
    const scaleY = img.naturalHeight / rect.height;
    const canvas = document.createElement("canvas");
    canvas.width = Math.max(1, Math.round(w * scaleX));
    canvas.height = Math.max(1, Math.round(h * scaleY));
    const ctx = canvas.getContext("2d");
    if (!ctx) return { base64: imageDataUrl, normRect: fullRect };
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
    return {
      base64: canvas.toDataURL("image/png"),
      normRect: {
        nx0: minX / rect.width,
        ny0: minY / rect.height,
        nx1: (minX + w) / rect.width,
        ny1: (minY + h) / rect.height,
      },
    };
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
          setSourceText("");
          setRawInputBackup(null);
          setDisplayMode("bilingual");
          toast(
            en
              ? "Screen captured. Drag on image to select region or click Translate."
              : "已截取屏幕，可拖拽框选目标区域后点击识别翻译。",
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

  const executeTranslate = async (opts?: {
    confirmCloudImage?: boolean;
    useEditedTextOnly?: boolean;
    overridePreserveLines?: boolean;
    overrideText?: string;
  }) => {
    const confirmCloudImage = !!opts?.confirmCloudImage;
    const useEditedTextOnly = !!opts?.useEditedTextOnly;
    const nextPreserve = opts?.overridePreserveLines ?? preserveLines;
    const textCandidate = opts?.overrideText ?? sourceText;

    await runTracked(
      { key: "translate:run", scope: "translate", lang: uiLang },
      async () => {
        const cropped = useEditedTextOnly
          ? { base64: undefined, normRect: lastTranslatedCrop }
          : extractCroppedImage();
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

        try {
          if (!finalImg && rawInputBackup === null) {
            setRawInputBackup(textCandidate);
          }
          const res = await api<DesktopTranslateResponse>("/api/v1/translate", {
            text: finalImg ? "" : textCandidate,
            image_base64: finalImg || "",
            source_lang: "auto",
            target_lang: targetLang,
            routing,
            model: selectedDeployment,
            preserve_lines: nextPreserve,
            confirm_cloud_image: confirmCloudImage,
          });
          setResult(res);
          if (finalImg) {
            setLastTranslatedCrop(cropped.normRect);
            const nextSrc = nextPreserve
              ? res.ocr_raw_text || res.ocr_text || ""
              : res.ocr_text || "";
            setSourceText(nextSrc);
            setRawInputBackup(res.ocr_raw_text || res.ocr_text || null);
            if (displayMode === "overlay" && !res.segments.some((s) => s.box)) {
              setDisplayMode("bilingual");
            }
          } else {
            const sep = nextPreserve ? "\n" : "\n\n";
            const rebuiltJoined = (res.segments || [])
              .map((s) => s.source)
              .join(sep);
            if (rebuiltJoined) setSourceText(rebuiltJoined);
          }

          if (res.fallback_reason) {
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
          await onRefresh();
        } catch (err) {
          if (
            err instanceof ApiError &&
            err.message === "cloud_image_confirmation_required"
          ) {
            const ok = await confirmAction({
              title: en ? "Cloud Routing Privacy Notice" : "云端回退隐私确认",
              message: en
                ? "OCR runs locally, but current routing may send extracted text to your configured cloud API if local inference is unavailable. Allow?"
                : "图片识别在本机完成，但当前策略在本地模型不可用时会将识别文字发往云端服务。确认允许？（不想联网可切换为「仅本地」。）",
              lang: uiLang,
            });
            if (ok) {
              await executeTranslate({ ...opts, confirmCloudImage: true });
            }
            return;
          }
          throw err;
        }
      },
    );
  };

  const handleTogglePreserveLines = (next: boolean) => {
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
    .map((s) => s.translation || s.source)
    .join("\n\n");

  const hasBoxOverlaySupport =
    !!imageDataUrl && !!result?.segments.some((s) => !!s.box);

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
      ...(hasBoxOverlaySupport
        ? [{ value: "overlay" as const, label: en ? "Image Overlay" : "贴回原图" }]
        : []),
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
                onValueChange={(v) => setTargetLang(v)}
              />
            </div>
          </div>

          <SegmentedControl
            value={routing}
            onChange={(v) => setRouting(v)}
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
                    label: en
                      ? `Default (${data.settings.model})`
                      : `默认实例`,
                  },
                  ...data.deployments
                    .filter((d) => !d.default)
                    .map((d) => ({
                      value: d.id,
                      label: d.id,
                    })),
                ]}
                onValueChange={(v) => setSelectedDeployment(v)}
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

      <FeedbackBanner scope="translate" lang={uiLang} />

      {imageDataUrl ? (
        <>
          {/* Screenshot / Image Stage + Adjacent Floating Paragraph Comparison */}
          <div
            class={
              result && result.segments.length > 0
                ? "mg-image-workbench"
                : "mg-image-workbench single"
            }
          >
            <div class="mg-crop-card">
              <div class="mg-studio-head">
                <span class="hint">
                  {en
                    ? "Drag on image to crop target region, or translate full image"
                    : "按住左键拖拽可框选局部区域，或直接识别整张图片"}
                </span>
                <div class="actions" style={{ margin: 0 }}>
                  {cropBox ? (
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => setCropBox(null)}
                    >
                      {en ? "Full image" : "取消框选"}
                    </Button>
                  ) : null}
                  <Button
                    size="sm"
                    variant="primary"
                    busy={store.isBusy("translate:run")}
                    onClick={() => void executeTranslate()}
                  >
                    {en ? "OCR & Translate" : "识别并翻译"}
                  </Button>
                  <Button
                    size="sm"
                    variant="danger"
                    icon={<IconTrash size={13} />}
                    onClick={() => {
                      setImageDataUrl(null);
                      setCropBox(null);
                    }}
                  >
                    {en ? "Remove" : "移除图片"}
                  </Button>
                </div>
              </div>

              <div class="mg-crop-stage-wrap">
                <div
                  class="mg-crop-stage"
                  onPointerDown={(e) => {
                    if (
                      (e.target as HTMLElement)?.closest?.(
                        ".mg-image-overlay-box",
                      )
                    ) {
                      return;
                    }
                    const rect = (
                      e.currentTarget as HTMLElement
                    ).getBoundingClientRect();
                    const x = e.clientX - rect.left;
                    const y = e.clientY - rect.top;
                    (e.currentTarget as HTMLElement).setPointerCapture(
                      e.pointerId,
                    );
                    setDraggingCrop(true);
                    setCropBox({ x0: x, y0: y, x1: x, y1: y });
                  }}
                  onPointerMove={(e) => {
                    if (!draggingCrop) return;
                    const rect = (
                      e.currentTarget as HTMLElement
                    ).getBoundingClientRect();
                    const x = Math.max(
                      0,
                      Math.min(rect.width, e.clientX - rect.left),
                    );
                    const y = Math.max(
                      0,
                      Math.min(rect.height, e.clientY - rect.top),
                    );
                    setCropBox((prev) =>
                      prev ? { ...prev, x1: x, y1: y } : null,
                    );
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

                  {displayMode === "overlay" && result
                    ? result.segments.map((seg, idx) => {
                        if (!seg.box || !seg.translation) return null;
                        const cw =
                          lastTranslatedCrop.nx1 - lastTranslatedCrop.nx0 || 1;
                        const ch =
                          lastTranslatedCrop.ny1 - lastTranslatedCrop.ny0 || 1;
                        const leftPct =
                          (lastTranslatedCrop.nx0 + seg.box[0] * cw) * 100;
                        const topPct =
                          (lastTranslatedCrop.ny0 + seg.box[1] * ch) * 100;
                        const widthPct = Math.max(
                          8,
                          (seg.box[2] - seg.box[0]) * cw * 100,
                        );
                        const minHeightPct = Math.max(
                          3,
                          (seg.box[3] - seg.box[1]) * ch * 100,
                        );
                        return (
                          <div
                            key={idx}
                            class="mg-image-overlay-box"
                            style={{
                              left: `${leftPct}%`,
                              top: `${topPct}%`,
                              width: `${widthPct}%`,
                              minHeight: `${minHeightPct}%`,
                            }}
                          >
                            {seg.translation}
                          </div>
                        );
                      })
                    : null}
                </div>
              </div>
            </div>

            {result && result.segments.length > 0 ? (
              <div class="mg-float-card">
                <div class="mg-studio-head">{renderModeSwitcherAndCopy()}</div>
                {renderParagraphStream()}
              </div>
            ) : null}
          </div>

          <SettingGroup
            title={
              en
                ? "Reconstructed OCR Source (editable)"
                : "识别并合并后的原文（可修改后重新翻译）"
            }
            padded
            actions={
              <Button
                size="sm"
                variant="primary"
                busy={store.isBusy("translate:run")}
                onClick={() => void executeTranslate({ useEditedTextOnly: true })}
              >
                {en ? "Re-translate Text" : "按修改后原文重译"}
              </Button>
            }
          >
            <Textarea
              rows={4}
              placeholder={
                en
                  ? "Recognized paragraphs appear here after OCR…"
                  : "点击「识别并翻译」后，重建好的段落原文会显示在此处供核对修改…"
              }
              value={sourceText}
              onValueInput={(v) => {
                setSourceText(v);
                setRawInputBackup(v);
              }}
            />
          </SettingGroup>
        </>
      ) : (
        /* Seamless Dual-Pane Text Translator Studio */
        <div class="mg-studio-card">
          <div class="mg-studio-pane">
            <textarea
              class="mg-studio-textarea"
              placeholder={
                en
                  ? "Paste text from PDF, docs, or press Ctrl+V to paste an image… (Ctrl+Enter to translate)"
                  : "输入或粘贴来自 PDF、网页、文档的文字，或直接按 Ctrl+V 粘贴截图…\n（段内硬换行会自动智能合并，按 Ctrl+Enter 立即翻译）"
              }
              value={sourceText}
              onInput={(e) => {
                const v = (e.currentTarget as HTMLTextAreaElement).value;
                setSourceText(v);
                setRawInputBackup(v);
              }}
              onKeyDown={(e) => {
                if ((e.ctrlKey || e.metaKey) && e.key === "Enter") {
                  e.preventDefault();
                  void executeTranslate();
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
                    onClick={() => {
                      setSourceText("");
                      setRawInputBackup(null);
                      setResult(null);
                    }}
                  >
                    {en ? "Clear" : "清空"}
                  </Button>
                ) : null}
                <Button
                  variant="primary"
                  busy={store.isBusy("translate:run")}
                  onClick={() => void executeTranslate()}
                >
                  {en ? "Translate" : "翻译"}
                </Button>
              </div>
            </div>
          </div>

          <div class="mg-studio-pane">
            <div class="mg-studio-head">{renderModeSwitcherAndCopy()}</div>
            {result && result.segments.length > 0 ? (
              renderParagraphStream()
            ) : (
              <div class="mg-studio-empty">
                <IconTranslate size={24} />
                <span>
                  {en
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
