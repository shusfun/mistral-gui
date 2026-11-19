import { useLayoutEffect, useRef } from "preact/hooks";

export interface ImageOverlay {
  box: [number, number, number, number];
  text: string;
  sourceLines: number;
}

export type ImageView = "original" | "translated";

export function ImageTranslation({
  box,
  text,
  sourceLines,
  referenceScale = 1,
}: ImageOverlay & { referenceScale?: number }) {
  const boxRef = useRef<HTMLDivElement>(null);
  const textRef = useRef<HTMLSpanElement>(null);

  useLayoutEffect(() => {
    const frame = boxRef.current;
    const label = textRef.current;
    if (!frame || !label) return;
    const fit = () => {
      const height = frame.clientHeight;
      const width = frame.clientWidth;
      const ceiling = Math.min(
        (height / sourceLines) * 0.75,
        parseFloat(getComputedStyle(frame).fontSize),
      );
      let low = 0;
      let high = ceiling;
      for (let attempt = 0; attempt < 8 && high - low > 0.05; attempt++) {
        const size = (low + high) / 2;
        label.style.fontSize = `${size}px`;
        if (label.scrollHeight <= height && label.scrollWidth <= width) {
          low = size;
        } else {
          high = size;
        }
      }
      label.style.fontSize = `${low}px`;
    };
    fit();
    const observer = new ResizeObserver(fit);
    observer.observe(frame);
    return () => observer.disconnect();
  }, [text, sourceLines, referenceScale]);

  return (
    <div
      class="mg-image-overlay-box"
      ref={boxRef}
      aria-hidden="true"
      style={{
        "--mg-overlay-reference-scale": referenceScale,
        left: `${box[0] * 100}%`,
        top: `${box[1] * 100}%`,
        width: `${(box[2] - box[0]) * 100}%`,
        height: `${(box[3] - box[1]) * 100}%`,
      }}
    >
      <span ref={textRef}>{text}</span>
    </div>
  );
}
