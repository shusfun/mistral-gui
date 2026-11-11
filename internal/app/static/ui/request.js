export const newTrace = () =>
  "00-" +
  crypto.randomUUID().replaceAll("-", "") +
  "-" +
  crypto.randomUUID().replaceAll("-", "").slice(0, 16) +
  "-01";
export const childTrace = (trace) =>
  trace.slice(0, 36) +
  crypto.randomUUID().replaceAll("-", "").slice(0, 16) +
  "-01";
let buffered = [],
  timer;
export function record(trace, phase, outcome, duration = 0) {
  if (buffered.length >= 100) buffered.shift();
  buffered.push({
    traceparent: trace,
    phase,
    outcome,
    duration_ms: Math.max(0, Math.round(duration)),
  });
  if (!timer) timer = setTimeout(flush, 1000);
}
function flush() {
  timer = undefined;
  const events = buffered.splice(0);
  if (events.length)
    void fetch("/api/v1/ui-events", {
      method: "POST",
      headers: { "Content-Type": "application/json", traceparent: newTrace() },
      body: JSON.stringify({ events }),
      keepalive: true,
    }).catch(() => {});
}
addEventListener("pagehide", flush);
export async function runOperation(name, action, trace = newTrace()) {
  const started = performance.now();
  record(trace, "ui.start", "started");
  try {
    const result = await action(trace);
    record(trace, "ui.end", "success", performance.now() - started);
    return result;
  } catch (error) {
    record(trace, "ui.end", "failed", performance.now() - started);
    throw error;
  }
}
export async function request(path, body, method, trace = newTrace()) {
  const span = childTrace(trace),
    started = performance.now();
  record(span, "request.start", "started");
  try {
    const response = await fetch(path, {
      method: method || (body === undefined ? "GET" : "POST"),
      headers: {
        traceparent: span,
        ...(body === undefined ? {} : { "Content-Type": "application/json" }),
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    let data;
    try {
      data = await response.json();
    } catch {
      throw Error("invalid_response");
    }
    record(
      span,
      "request.end",
      response.ok ? "success" : "failed",
      performance.now() - started,
    );
    if (!response.ok) {
      const error = Error(data.error?.code || "networkError");
      error.status = response.status;
      throw error;
    }
    return data;
  } catch (error) {
    record(span, "request.failed", "failed", performance.now() - started);
    throw error;
  }
}
