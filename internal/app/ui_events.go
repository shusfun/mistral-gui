package app

import (
	"log/slog"
	"net/http"
)

type UIEvent struct {
	Traceparent string `json:"traceparent"`
	Phase       string `json:"phase"`
	Outcome     string `json:"outcome"`
	DurationMS  int64  `json:"duration_ms"`
}

// Accept only bounded stage metadata; payloads and credentials are never logged.
func (a *App) uiEvents(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Events []UIEvent `json:"events"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32768)
	if decode(r, &body) != nil || len(body.Events) > 100 {
		fail(w, 400, "invalid_ui_events")
		return
	}
	phases := map[string]bool{"ui.start": true, "ui.end": true, "ui.result": true, "request.start": true, "request.end": true, "request.failed": true, "native.start": true, "native.end": true}
	outcomes := map[string]bool{"started": true, "success": true, "failed": true, "cancelled": true}
	for _, e := range body.Events {
		if !validTraceparent(e.Traceparent) || !phases[e.Phase] || !outcomes[e.Outcome] || e.DurationMS < 0 || e.DurationMS > 86400000 {
			fail(w, 400, "invalid_ui_events")
			return
		}
	}
	for _, e := range body.Events {
		slog.Info(e.Phase, "traceId", traceRoot(e.Traceparent), "spanId", e.Traceparent[36:52], "outcome", e.Outcome, "durationMs", e.DurationMS)
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
