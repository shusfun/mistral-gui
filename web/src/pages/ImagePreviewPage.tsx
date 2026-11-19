import { useEffect, useLayoutEffect, useRef, useState } from "preact/hooks";
import { flushUiTraces, getWailsRuntime, nativeCommand, recordUiTrace } from "../api";
import {
  ImageTranslation,
  type ImageOverlay,
  type ImageView,
} from "../components/ImageTranslation";
import {
  applyTheme,
  Button,
  IconMinus,
  IconPlus,
  SegmentedControl,
} from "../shared/design-system";

interface PreviewSnapshot {
  src: string;
  overlays: ImageOverlay[];
  view: ImageView;
  theme: string;
  lang: string;
  revision: number;
  trace: string;
}

interface PreviewImage extends PreviewSnapshot {
  width: number;
  height: number;
}

interface Point {
  x: number;
  y: number;
}

const center: Point = { x: 0, y: 0 };
const zoomStep = 1.25;
const maxZoom = 8;
const limit = (value: number, min: number, max: number) => Math.min(max, Math.max(min, value));

export function ImagePreviewPage() {
  const [image, setImage] = useState<PreviewImage>();
  const [failure, setFailure] = useState<string>();

  useEffect(() => {
    let disposed = false;
    let requested = 0;
    let revision = 0;
    let unsubscribe: (() => void) | undefined;
    const refresh = async () => {
      const request = ++requested;
      let snapshot: PreviewSnapshot | undefined;
      try {
        snapshot = await nativeCommand<PreviewSnapshot>("read-image-preview");
        if (disposed || request !== requested || snapshot.revision <= revision) return;
        const decoded = new Image();
        decoded.src = snapshot.src;
        await decoded.decode();
        if (disposed || request !== requested) return;
        if (decoded.naturalWidth * decoded.naturalHeight > 32 * 1024 * 1024) throw new Error();
        revision = snapshot.revision;
        applyTheme(snapshot.theme);
        document.documentElement.lang = snapshot.lang === "en" ? "en" : "zh-CN";
        setFailure(undefined);
        setImage({ ...snapshot, width: decoded.naturalWidth, height: decoded.naturalHeight });
      } catch {
        if (disposed || request !== requested) return;
        if (snapshot) {
          recordUiTrace(snapshot.trace, "image.preview.render", "failed");
          flushUiTraces();
        }
        setImage(undefined);
        setFailure(snapshot?.lang || document.documentElement.lang);
      }
    };
    void getWailsRuntime().then((runtime) => {
      if (disposed) return;
      unsubscribe = runtime?.Events?.On?.("desktop:image-preview-changed", () => void refresh());
      void refresh();
    });
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        void getWailsRuntime().then((runtime) => runtime?.Window?.Close?.());
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => {
      disposed = true;
      unsubscribe?.();
      window.removeEventListener("keydown", onKeyDown);
    };
  }, []);

  if (image) return <ImageViewer key={image.revision} image={image} />;
  if (!failure) return null;
  return (
    <main class="mg-viewer-message">
      <div role="alert">
        {failure.startsWith("en") ? "Could not display the image." : "无法显示图片。"}
      </div>
    </main>
  );
}

function ImageViewer({ image }: { image: PreviewImage }) {
  const viewport = useRef<HTMLDivElement>(null);
  const gesture = useRef<{ pointer: number; origin: Point; offset: Point } | undefined>(undefined);
  const [size, setSize] = useState({ width: 0, height: 0 });
  const [camera, setCamera] = useState<{ zoom: number | null; offset: Point }>({ zoom: null, offset: center });
  const [dragging, setDragging] = useState(false);
  const [view, setView] = useState(image.view);
  const en = image.lang === "en";
  const fit = Math.min(1, size.width / image.width, size.height / image.height);
  const artworkScale = Math.max(fit, Math.min(1, 2048 / Math.max(image.width, image.height)));
  const zoom = camera.zoom ?? fit;
  const minimum = Math.min(fit, 0.1);

  const constrain = (offset: Point, scale: number) => {
    const horizontal = Math.max(0, (image.width * scale - size.width) / 2);
    const vertical = Math.max(0, (image.height * scale - size.height) / 2);
    return { x: limit(offset.x, -horizontal, horizontal), y: limit(offset.y, -vertical, vertical) };
  };

  const offset = constrain(camera.offset, zoom);
  const canPan = image.width * zoom > size.width || image.height * zoom > size.height;
  const zoomTo = (scale: (previous: number) => number, anchor = center) => {
    if (!fit) return;
    setCamera((current) => {
      const previous = current.zoom ?? fit;
      const next = limit(scale(previous), minimum, maxZoom);
      const ratio = next / previous;
      const position = constrain(current.offset, previous);
      return { zoom: next, offset: constrain({
        x: anchor.x + (position.x - anchor.x) * ratio,
        y: anchor.y + (position.y - anchor.y) * ratio,
      }, next) };
    });
  };

  useLayoutEffect(() => {
    const element = viewport.current;
    if (!element) return;
    const measure = () => setSize({ width: element.clientWidth, height: element.clientHeight });
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    recordUiTrace(image.trace, "image.preview.render", "success");
    flushUiTraces();
  }, [image.trace]);

  useEffect(() => {
    const element = viewport.current;
    if (!element) return;
    const wheel = (event: WheelEvent) => {
      if (!event.deltaY) return;
      event.preventDefault();
      const rect = element.getBoundingClientRect();
      const distance = event.deltaY * (event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? size.height : 1);
      const factor = Math.exp(limit(-distance * 0.002, -0.7, 0.7));
      zoomTo((previous) => previous * factor, {
        x: event.clientX - rect.left - size.width / 2,
        y: event.clientY - rect.top - size.height / 2,
      });
    };
    element.addEventListener("wheel", wheel, { passive: false });
    return () => element.removeEventListener("wheel", wheel);
  }, [fit, size]);

  const zoomOut = en ? "Zoom out" : "缩小";
  const zoomIn = en ? "Zoom in" : "放大";
  return (
    <main class="mg-viewer">
      <header class="mg-viewer-toolbar">
        <SegmentedControl<ImageView>
          value={view}
          onChange={setView}
          ariaLabel={en ? "Image view" : "图片预览模式"}
          options={[
            { value: "original", label: en ? "Original" : "原图" },
            { value: "translated", label: en ? "Translated" : "译图", disabled: !image.overlays.length },
          ]}
        />
        <div class="mg-viewer-zoom">
          <Button size="sm" variant="ghost" aria-pressed={camera.zoom === null} onClick={() => setCamera({ zoom: null, offset: center })}>
            {en ? "Fit" : "适应窗口"}
          </Button>
          <Button size="sm" variant="ghost" aria-pressed={camera.zoom === 1} onClick={() => zoomTo(() => 1)}>
            {en ? "Actual size" : "实际大小"}
          </Button>
          <Button
            class="mg-preview-icon-button"
            size="sm"
            variant="ghost"
            icon={<IconMinus size={15} />}
            aria-label={zoomOut}
            title={zoomOut}
            disabled={!fit || zoom <= minimum}
            onClick={() => zoomTo((previous) => previous / zoomStep)}
          />
          <output>{`${Math.round(zoom * 100)}%`}</output>
          <Button
            class="mg-preview-icon-button"
            size="sm"
            variant="ghost"
            icon={<IconPlus size={15} />}
            aria-label={zoomIn}
            title={zoomIn}
            disabled={!fit || zoom >= maxZoom}
            onClick={() => zoomTo((previous) => previous * zoomStep)}
          />
        </div>
      </header>
      <div
        ref={viewport}
        class="mg-viewer-viewport"
        data-pan={dragging ? "dragging" : canPan ? "available" : undefined}
        onPointerDown={(event) => {
          if (event.button !== 0 || !event.isPrimary || !canPan) return;
          event.currentTarget.setPointerCapture(event.pointerId);
          gesture.current = { pointer: event.pointerId, origin: { x: event.clientX, y: event.clientY }, offset };
          setDragging(true);
        }}
        onPointerMove={(event) => {
          const drag = gesture.current;
          if (drag?.pointer !== event.pointerId) return;
          setCamera((current) => ({ ...current, offset: constrain({
            x: drag.offset.x + event.clientX - drag.origin.x,
            y: drag.offset.y + event.clientY - drag.origin.y,
          }, zoom) }));
        }}
        onLostPointerCapture={(event) => {
          if (gesture.current?.pointer !== event.pointerId) return;
          gesture.current = undefined;
          setDragging(false);
        }}
      >
        {fit > 0 && (
          <div class="mg-viewer-image" style={{
            width: image.width * artworkScale,
            height: image.height * artworkScale,
            transform: `translate(${offset.x}px, ${offset.y}px) translate(-50%, -50%) scale(${zoom / artworkScale})`,
          }}>
            <img src={image.src} alt={en ? "Previewed image" : "预览图片"} draggable={false} />
            {view === "translated" && image.overlays.map((overlay, index) => (
              <ImageTranslation key={index} {...overlay} referenceScale={artworkScale / fit} />
            ))}
          </div>
        )}
      </div>
    </main>
  );
}
