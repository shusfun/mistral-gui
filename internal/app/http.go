package app

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"github.com/shusfun/mistral-gui/internal/catalog"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/manage", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("GET /manage", a.manage)
	mux.HandleFunc("GET /manage/", a.manage)
	mux.HandleFunc("POST /api/v1/session", a.session)
	mux.HandleFunc("POST /api/v1/manager/quit", a.auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 202, map[string]bool{"ok": true})
		go a.Close()
	}))
	mux.HandleFunc("GET /api/v1/status", a.auth(a.status))
	mux.HandleFunc("GET /api/v1/models", a.auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"models": a.installedSnapshot()})
	}))
	mux.HandleFunc("GET /api/v1/assets", a.auth(a.assetList))
	mux.HandleFunc("POST /api/v1/assets/install", a.auth(a.installAsset))
	mux.HandleFunc("POST /api/v1/models/delete", a.auth(func(w http.ResponseWriter, r *http.Request) {
		var v struct {
			ID      string `json:"id"`
			Version string `json:"version"`
		}
		if decode(r, &v) != nil {
			fail(w, 400, "invalid_json")
			return
		}
		if err := a.deleteModel(v.ID, v.Version); err != nil {
			fail(w, 409, err.Error())
			return
		}
		_ = a.save()
		writeJSON(w, 200, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("POST /api/v1/models/rollback", a.auth(func(w http.ResponseWriter, r *http.Request) {
		a.lifecycle.Lock()
		defer a.lifecycle.Unlock()
		a.mu.Lock()
		if a.engine != nil {
			a.mu.Unlock()
			fail(w, 409, "stop_before_switch")
			return
		}
		_, found := a.modelLocked(a.config.PreviousModel, a.config.PreviousModelVersion)
		if !found {
			a.mu.Unlock()
			fail(w, 409, "previous_model_missing")
			return
		}
		a.config.Model, a.config.PreviousModel = a.config.PreviousModel, a.config.Model
		a.config.ModelVersion, a.config.PreviousModelVersion = a.config.PreviousModelVersion, a.config.ModelVersion
		a.mu.Unlock()
		if a.save() != nil {
			fail(w, 500, "settings_write_failed")
			return
		}
		a.status(w, r)
	}))
	mux.HandleFunc("POST /api/v1/runtime/select", a.auth(func(w http.ResponseWriter, r *http.Request) {
		var v struct {
			ID      string `json:"id"`
			Version string `json:"version"`
			Backend string `json:"backend"`
		}
		if decode(r, &v) != nil {
			fail(w, 400, "invalid_json")
			return
		}
		a.lifecycle.Lock()
		defer a.lifecycle.Unlock()
		a.mu.Lock()
		if a.engine != nil {
			a.mu.Unlock()
			fail(w, 409, "stop_before_switch")
			return
		}
		found := false
		for _, in := range a.installed {
			if in.Kind == "engine" && in.ID == v.ID && in.Version == v.Version && in.Backend == v.Backend {
				if a.config.Engine != in.Path {
					a.config.PreviousEngine = a.config.Engine
				}
				a.config.Engine = in.Path
				a.config.Backend = in.Backend
				a.config.EngineVersion = in.Version
				found = true
				break
			}
		}
		a.mu.Unlock()
		if !found {
			fail(w, 404, "engine_missing")
			return
		}
		if a.save() != nil {
			fail(w, 500, "settings_write_failed")
			return
		}
		a.status(w, r)
	}))
	mux.HandleFunc("POST /api/v1/runtime/start", a.auth(func(w http.ResponseWriter, r *http.Request) {
		trace := r.Header.Get("traceparent")
		op := a.operation("start", trace, func(ctx context.Context, id string) error {
			err := a.ensureEngine(ctx, trace)
			if ctx.Err() != nil {
				a.stopEngine()
			}
			return err
		})
		writeJSON(w, 202, op)
	}))
	mux.HandleFunc("POST /api/v1/runtime/stop", a.auth(func(w http.ResponseWriter, r *http.Request) { a.stopEngine(); writeJSON(w, 200, a.stateSnapshot()) }))
	mux.HandleFunc("POST /api/v1/runtime/fallback-cpu", a.auth(a.fallbackCPU))
	mux.HandleFunc("POST /api/v1/runtime/restart", a.auth(func(w http.ResponseWriter, r *http.Request) {
		trace := r.Header.Get("traceparent")
		op := a.operation("restart", trace, func(ctx context.Context, id string) error { a.stopEngine(); return a.ensureEngine(ctx, trace) })
		writeJSON(w, 202, op)
	}))
	mux.HandleFunc("POST /api/v1/runtime/rollback", a.auth(func(w http.ResponseWriter, r *http.Request) {
		a.stopEngine()
		a.mu.Lock()
		if a.config.PreviousEngine == "" {
			a.mu.Unlock()
			fail(w, 409, "previous_engine_missing")
			return
		}
		a.config.Engine, a.config.PreviousEngine = a.config.PreviousEngine, a.config.Engine
		for _, in := range a.installed {
			if in.Path == a.config.Engine {
				a.config.EngineVersion = in.Version
				a.config.Backend = in.Backend
			}
		}
		a.mu.Unlock()
		_ = a.save()
		writeJSON(w, 200, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("POST /api/v1/settings", a.auth(a.settings))
	mux.HandleFunc("POST /api/v1/token/rotate", a.auth(func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		a.config.Token = randomToken()
		a.mu.Unlock()
		if err := a.save(); err != nil {
			fail(w, 500, "settings_write_failed")
			return
		}
		a.setCookie(w)
		writeJSON(w, 200, map[string]string{"token": a.token()})
	}))
	mux.HandleFunc("GET /api/v1/token", a.auth(func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"token": a.token()}) }))
	mux.HandleFunc("GET /api/v1/devices", a.auth(a.devices))
	mux.HandleFunc("GET /api/v1/diagnostics", a.auth(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="mistral-gui-diagnostics.json"`)
		writeJSON(w, 200, a.exportDiagnostics())
	}))
	mux.HandleFunc("GET /api/v1/events", a.auth(a.events))
	mux.HandleFunc("POST /api/v1/tasks/cancel", a.auth(func(w http.ResponseWriter, r *http.Request) {
		var v struct {
			ID string `json:"id"`
		}
		if decode(r, &v) != nil {
			fail(w, 400, "invalid_json")
			return
		}
		a.mu.Lock()
		c := a.cancels[v.ID]
		a.mu.Unlock()
		if c != nil {
			c()
		}
		writeJSON(w, 200, map[string]bool{"ok": c != nil})
	}))
	mux.HandleFunc("POST /api/v1/update/check", a.auth(a.updateCheck))
	mux.HandleFunc("POST /api/v1/update/install", a.auth(a.updateInstall))
	mux.HandleFunc("/", a.auth(a.proxy))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trace := r.Header.Get("traceparent")
		if !validTraceparent(trace) {
			trace = "00-" + randomToken()[:32] + "-" + randomToken()[:16] + "-01"
		}
		r.Header.Set("traceparent", trace)
		w.Header().Set("traceparent", trace)
		requestID := randomToken()[:16]
		w.Header().Set("X-Request-ID", requestID)
		rw := &recordingWriter{ResponseWriter: w}
		w = rw
		started := time.Now()
		defer func() {
			slog.Info("http.request", "traceId", trace[3:35], "spanId", trace[36:52], "requestId", requestID, "method", r.Method, "path", r.URL.Path, "status", rw.status, "durationMs", time.Since(started).Milliseconds())
		}()
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			fail(w, 403, "host_rejected")
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "" {
			u, err := url.Parse(origin)
			allowed := err == nil && (u.Scheme == "chrome-extension" || u.Host == r.Host && (u.Scheme == "http" || u.Scheme == "https"))
			if !allowed {
				fail(w, 403, "origin_rejected")
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, traceparent, X-Request-ID, X-Mistral-Auto-Start")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, traceparent, X-Mistral-Model-Revision")
		}
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
func validTraceparent(s string) bool {
	if len(s) != 55 || s[:3] != "00-" || s[35] != '-' || s[52] != '-' {
		return false
	}
	for _, c := range s[3:35] + s[36:52] + s[53:] {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return s[3:35] != strings.Repeat("0", 32) && s[36:52] != strings.Repeat("0", 16)
}
func (a *App) authorized(r *http.Request) bool {
	token := ""
	if value := r.Header.Get("Authorization"); strings.HasPrefix(value, "Bearer ") {
		token = strings.TrimPrefix(value, "Bearer ")
	}
	if token == "" {
		if c, e := r.Cookie("mistral_gui_session"); e == nil {
			token = c.Value
		}
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(a.token())) == 1
}
func (a *App) auth(f http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.authorized(r) {
			fail(w, 401, "invalid_token")
			return
		}
		f(w, r)
	}
}
func (a *App) setCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: "mistral_gui_session", Value: a.token(), Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
}
func (a *App) session(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(r) {
		fail(w, 401, "invalid_token")
		return
	}
	a.setCookie(w)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (a *App) status(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	c := a.config
	c.Token = ""
	s := a.state
	ops := append([]Operation(nil), a.operations...)
	a.mu.Unlock()
	revision := ""
	selectedBackend := c.Backend
	for _, in := range a.installedSnapshot() {
		if in.Kind == "engine" && in.Path == c.Engine {
			selectedBackend = in.Backend
		}
		if in.Kind == "model" && in.ID == c.Model && (c.ModelVersion == "" || c.ModelVersion == in.Version) {
			revision = in.Version + ":" + in.SHA256
		}
	}
	var updateResult map[string]any
	if b, e := os.ReadFile(filepath.Join(a.dataDir, "update-status.json")); e == nil {
		_ = json.Unmarshal(b, &updateResult)
	}
	writeJSON(w, 200, map[string]any{"status": s, "settings": c, "operations": ops, "platform": runtime.GOOS + "/" + runtime.GOARCH, "version": Version, "model_revision": revision, "selected_backend": selectedBackend, "update_result": updateResult})
}
func (a *App) settings(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Language     *string   `json:"language"`
		Backend      *string   `json:"backend"`
		Model        *string   `json:"model"`
		ModelVersion *string   `json:"model_version"`
		AutoStart    *bool     `json:"auto_start"`
		IdleMinutes  *int      `json:"idle_minutes"`
		Mirrors      *[]string `json:"mirrors"`
	}
	if err := decode(r, &v); err != nil {
		fail(w, 400, "invalid_json")
		return
	}
	if v.Language != nil && *v.Language != "zh-CN" && *v.Language != "en-US" {
		fail(w, 400, "invalid_language")
		return
	}
	if v.Backend != nil && *v.Backend != "cpu" && *v.Backend != "auto" && *v.Backend != "cuda" && *v.Backend != "metal" {
		fail(w, 400, "invalid_backend")
		return
	}
	if v.IdleMinutes != nil && (*v.IdleMinutes < 0 || *v.IdleMinutes > 1440) {
		fail(w, 400, "invalid_idle_minutes")
		return
	}
	if v.Mirrors != nil {
		for _, m := range *v.Mirrors {
			u, e := url.Parse(m)
			if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
				fail(w, 400, "invalid_mirror")
				return
			}
		}
	}
	a.lifecycle.Lock()
	defer a.lifecycle.Unlock()
	a.mu.Lock()
	if a.engine != nil && (v.Backend != nil && *v.Backend != a.config.Backend || v.Model != nil && *v.Model != a.config.Model || v.ModelVersion != nil && *v.ModelVersion != a.config.ModelVersion) {
		a.mu.Unlock()
		fail(w, 409, "stop_before_switch")
		return
	}
	if v.Model != nil {
		version := ""
		if v.ModelVersion != nil {
			version = *v.ModelVersion
		}
		selected, ok := a.modelLocked(*v.Model, version)
		if !ok {
			a.mu.Unlock()
			fail(w, 400, "model_not_installed")
			return
		}
		a.selectModelLocked(selected)
	}
	if v.Language != nil {
		a.config.Language = *v.Language
	}
	if v.Backend != nil {
		a.config.Backend = *v.Backend
		a.state.DegradedCPU = false
	}
	if v.AutoStart != nil {
		a.config.AutoStart = *v.AutoStart
	}
	if v.IdleMinutes != nil {
		a.config.IdleMinutes = *v.IdleMinutes
	}
	if v.Mirrors != nil {
		a.config.Mirrors = *v.Mirrors
	}
	a.mu.Unlock()
	if err := a.save(); err != nil {
		fail(w, 500, "settings_write_failed")
		return
	}
	a.status(w, r)
}
func (a *App) events(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	f, ok := w.(http.Flusher)
	if !ok {
		return
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		a.mu.Lock()
		b, _ := json.Marshal(map[string]any{"status": a.state, "operations": a.operations})
		a.mu.Unlock()
		if _, err := fmt.Fprintf(w, "event: status\ndata: %s\n\n", b); err != nil {
			return
		}
		f.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-a.closed:
			return
		case <-tick.C:
		}
	}
}
func (a *App) assetList(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()
	a.mu.Lock()
	backend := a.config.Backend
	a.mu.Unlock()
	engine, e := a.engineAsset(ctx, backend)
	models, me := a.catalog.Models(ctx)
	errs := map[string]string{}
	if e != nil {
		errs["engine"] = e.Error()
	}
	if me != nil {
		errs["models"] = me.Error()
	}
	writeJSON(w, 200, map[string]any{"engine": engine, "models": models, "errors": errs})
}
func (a *App) installAsset(w http.ResponseWriter, r *http.Request) {
	a.taskStart.Lock()
	defer a.taskStart.Unlock()
	var v struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
	}
	if decode(r, &v) != nil {
		fail(w, 400, "invalid_json")
		return
	}
	if v.Kind != "engine" && v.Kind != "model" {
		fail(w, 400, "invalid_kind")
		return
	}
	a.mu.Lock()
	backend := a.config.Backend
	for _, op := range a.operations {
		if (op.Kind == "install" || op.Kind == "update" || op.Kind == "fallback_cpu") && op.State == "running" {
			a.mu.Unlock()
			fail(w, 409, "installation_in_progress")
			return
		}
	}
	a.mu.Unlock()
	trace := r.Header.Get("traceparent")
	op := a.operation("install", trace, func(ctx context.Context, id string) error {
		a.changeOp(id, func(o *Operation) { o.AssetKind = v.Kind; o.AssetID = v.ID; o.Step = "checking" })
		var asset catalog.Asset
		var err error
		if v.Kind == "engine" {
			asset, err = a.engineAsset(ctx, backend)
		} else {
			var all []catalog.Asset
			all, err = a.catalog.Models(ctx)
			for _, m := range all {
				if m.ID == v.ID {
					asset = m
				}
			}
			if err == nil && asset.ID == "" {
				return fmt.Errorf("model_unknown")
			}
		}
		if err != nil {
			return err
		}
		return a.install(ctx, id, asset)
	})
	writeJSON(w, 202, op)
}
func (a *App) devices(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, nvidiaSMI(), "--query-gpu=name,driver_version,memory.total,memory.used", "--format=csv,noheader,nounits")
	setProcess(c)
	b, err := c.Output()
	info := map[string]any{"os": runtime.GOOS, "arch": runtime.GOARCH, "cpu": true, "metal": runtime.GOOS == "darwin" && runtime.GOARCH == "arm64", "cuda": err == nil, "memory": "unknown", "gpus": []any{}}
	info["cpu_cores"] = runtime.NumCPU()
	if runtime.GOOS == "windows" {
		info["vc_runtime"] = vcRuntimeAvailable()
	}
	total, available := memoryInfo()
	if total > 0 {
		info["memory"] = fmt.Sprintf("%.1f GiB", float64(total)/(1<<30))
	}
	if available > 0 {
		info["memory_available"] = fmt.Sprintf("%.1f GiB", float64(available)/(1<<30))
	}
	if err == nil {
		info["gpus"] = strings.Split(strings.TrimSpace(string(b)), "\n")
	} else {
		info["cuda_reason"] = "nvidia_driver_unavailable"
	}
	writeJSON(w, 200, info)
}
