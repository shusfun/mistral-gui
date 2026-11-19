import { confirmAction } from "./shared/design-system";

export class ApiError extends Error {
  status: number;
  raw?: string;
  constructor(code: string, status = 0, raw?: string) {
    super(code);
    this.status = status;
    this.raw = raw;
  }
}

export const isDesktopMode =
  typeof window !== "undefined" &&
  new URLSearchParams(window.location.search).get("desktop") === "1";

export const newTrace = (): string =>
  "00-" +
  crypto.randomUUID().replace(/-/g, "") +
  "-" +
  crypto.randomUUID().replace(/-/g, "").slice(0, 16) +
  "-01";

export const childTrace = (trace: string): string =>
  trace.slice(0, 36) +
  crypto.randomUUID().replace(/-/g, "").slice(0, 16) +
  "-01";

interface BufferedUiEvent {
  traceparent: string;
  phase: string;
  outcome: string;
  duration_ms: number;
}

const bufferedEvents: BufferedUiEvent[] = [];
let flushTimer: ReturnType<typeof setTimeout> | undefined;

export function recordUiTrace(
  trace: string,
  phase: string,
  outcome: string,
  durationMs = 0,
) {
  if (bufferedEvents.length >= 100) bufferedEvents.shift();
  bufferedEvents.push({
    traceparent: trace,
    phase,
    outcome,
    duration_ms: Math.max(0, Math.round(durationMs)),
  });
  if (!flushTimer) {
    flushTimer = setTimeout(flushUiTraces, 1200);
  }
}

export function flushUiTraces() {
  clearTimeout(flushTimer);
  flushTimer = undefined;
  const events = bufferedEvents.splice(0);
  if (!events.length) return;
  void fetch("/api/v1/ui-events", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-Mistral-UI": "1",
      traceparent: newTrace(),
    },
    body: JSON.stringify({ events }),
    keepalive: true,
  }).catch(() => {});
}

if (typeof window !== "undefined") {
  window.addEventListener("pagehide", flushUiTraces);
}

let onAuthRequiredCallback: (() => void) | null = null;
export async function traceUiSave(action: (trace: string) => Promise<void>) {
  const trace = newTrace();
  const started = performance.now();
  recordUiTrace(trace, "ui.start", "started");
  try {
    await action(trace);
    recordUiTrace(trace, "ui.end", "success", performance.now() - started);
  } catch (error) {
    recordUiTrace(trace, "ui.end", "failed", performance.now() - started);
    throw error;
  }
}
export function onAuthRequired(cb: () => void) {
  onAuthRequiredCallback = cb;
}

// M-04, M-05, M-07, M-S13: Unified API helper sending X-Mistral-UI: 1, distinguishing 401 from network/5xx
export async function api<T = any>(
  path: string,
  body?: unknown,
  options: {
    method?: string;
    signal?: AbortSignal;
    trace?: string;
    tokenOverride?: string;
    suppressAuthTrigger?: boolean;
  } = {},
): Promise<T> {
  const rootTrace = options.trace || newTrace();
  const span = childTrace(rootTrace);
  const started = performance.now();
  recordUiTrace(span, "request.start", "started");

  const headers: Record<string, string> = {
    traceparent: span,
    "X-Mistral-UI": "1",
  };
  if (body !== undefined) {
    headers["Content-Type"] = "application/json";
  }
  if (options.tokenOverride) {
    headers["Authorization"] = `Bearer ${options.tokenOverride}`;
  }

  let response: Response;
  try {
    response = await fetch(path, {
      method: options.method || (body === undefined ? "GET" : "POST"),
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: options.signal,
    });
  } catch (err: any) {
    if (err?.name === "AbortError" || options.signal?.aborted) {
      recordUiTrace(span, "request.end", "cancelled", performance.now() - started);
      throw new ApiError("cancelled", 0);
    }
    recordUiTrace(span, "request.failed", "failed", performance.now() - started);
    throw new ApiError("networkError", 0, err?.message);
  }

  let data: any = null;
  const text = await response.text();
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      recordUiTrace(span, "request.end", "failed", performance.now() - started);
      throw new ApiError("invalid_response", response.status);
    }
  }

  recordUiTrace(
    span,
    "request.end",
    response.ok ? "success" : "failed",
    performance.now() - started,
  );

  if (!response.ok) {
    const code =
      (typeof data?.error === "string" ? data.error : data?.error?.code) ||
      (response.status === 401 ? "invalid_token" : `HTTP_${response.status}`);
    // M-04: ONLY trigger auth modal on HTTP 401, never on 500/503/network failures!
    if (response.status === 401 && !options.suppressAuthTrigger) {
      onAuthRequiredCallback?.();
    }
    throw new ApiError(code, response.status, data?.error?.detail);
  }

  return data as T;
}

// M-S02: Exchange one-time #exchange=<code> fragment into HttpOnly SameSite=Strict session cookie and scrub URL
export async function consumeUrlExchangeCode(): Promise<string | null> {
  if (typeof window === "undefined") return null;
  const rawHash = window.location.hash.replace(/^#/, "");
  if (!rawHash.includes("exchange=")) return null;

  const params = new URLSearchParams(rawHash);
  const code = params.get("exchange");
  const uiTarget = params.get("ui");
  if (!code) return null;

  // Immediately scrub code from location bar and browser history
  const nextHash = uiTarget
    ? `#instances/${encodeURIComponent(uiTarget)}`
    : "#overview";
  history.replaceState(
    null,
    "",
    window.location.pathname + window.location.search + nextHash,
  );

  await api(
    "/api/v1/session/exchange",
    { code },
    { suppressAuthTrigger: true },
  );
  return nextHash;
}

// --- Wails v3 Native Bridge (M-08, M-22, M-VC) ---
let wailsRuntimePromise: Promise<any> | null = null;

export async function getWailsRuntime(): Promise<any | null> {
  if (!isDesktopMode) return null;
  if (!wailsRuntimePromise) {
    const wailsModulePath = "/wails/runtime.js";
    wailsRuntimePromise = import(/* @vite-ignore */ wailsModulePath).catch(
      () => null,
    );
  }
  return wailsRuntimePromise;
}

function normalizeNativeError(err: unknown): Error {
  const raw =
    err instanceof Error
      ? err.message
      : typeof err === "string"
        ? err
        : JSON.stringify(err || "");
  if (!raw || /cancel|dialog_cancelled|user_cancelled/i.test(raw)) {
    return new ApiError("dialog_cancelled", 0);
  }
  if (raw.includes("Init has not been called")) {
    return new ApiError("update_repository_unconfigured", 0, raw);
  }
  // Extract snake_case error code returned by Go desktop.Service.Command
  const m = /([a-z][a-z0-9_]{2,60})\s*$/.exec(raw.trim());
  if (m) {
    return new ApiError(m[1], 0, raw);
  }
  return new ApiError(raw.trim() || "operation_failed", 0, raw);
}

export async function nativeCommand<T = any>(
  kind: string,
  value = "",
  trace = newTrace(),
): Promise<T> {
  const rt = await getWailsRuntime();
  if (!rt?.Call?.ByName) {
    throw new ApiError("desktop_action_required", 0);
  }
  const span = childTrace(trace);
  recordUiTrace(span, "native.start", "started");
  try {
    const res = await rt.Call.ByName(
      "github.com/shusfun/mistral-gui/internal/desktop.Service.Command",
      kind,
      value,
      span,
    );
    recordUiTrace(span, "native.end", "success");
    return res as T;
  } catch (err) {
    recordUiTrace(span, "native.end", "failed");
    throw normalizeNativeError(err);
  }
}

let desktopBridgeInitialized = false;

export async function initDesktopBridge(
  platform: string,
  handlers: {
    onUpdateChecked?: (payload: { result?: any; error?: string }) => void;
    onScreenshotCaptured?: (payload: {
      image_base64?: string;
      trace?: string;
      width?: number;
      height?: number;
      error?: string;
    }) => void;
  },
) {
  if (!isDesktopMode) return;
  document.documentElement.classList.add("desktop");
  if (platform.startsWith("darwin")) {
    document.documentElement.classList.add("mac");
  }
  const rt = await getWailsRuntime();
  if (!rt || desktopBridgeInitialized) return;
  desktopBridgeInitialized = true;

  rt.Events?.On?.("desktop:confirm", async (event: any) => {
    const data = event?.data || event;
    if (!data?.id) return;
    const accepted = await confirmAction({
      message: String(data.message || ""),
    });
    await nativeCommand(
      "confirm-result",
      JSON.stringify({ id: data.id, accepted }),
      data.trace,
    ).catch(() => {});
  });

  rt.Events?.On?.("desktop:update-checked", (event: any) => {
    const data = event?.data || event || {};
    handlers.onUpdateChecked?.(data);
  });

  const handleScreenshot = (data: unknown) => {
    if (!data || typeof data !== "object") return;
    const { image_base64, trace, error } = data as Record<string, unknown>;
    handlers.onScreenshotCaptured?.({
      image_base64: typeof image_base64 === "string" ? image_base64 : undefined,
      trace:
        typeof trace === "string" && /^00-[0-9a-f]{32}-[0-9a-f]{16}-01$/.test(trace)
          ? trace
          : undefined,
      error: typeof error === "string" ? error : undefined,
    });
  };
  rt.Events?.On?.("desktop:screenshot-captured", (event: any) => {
    handleScreenshot(event?.data || event);
  });

  await nativeCommand<unknown>("ready").then(handleScreenshot).catch(() => {});
}
