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
	mux.HandleFunc("POST /api/v1/session/exchange", a.sessionExchange)
	mux.HandleFunc("POST /api/v1/manager/quit", a.auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 202, map[string]bool{"ok": true})
		go a.Close()
	}))
	mux.HandleFunc("POST /api/v1/ui-events", a.auth(a.uiEvents))
	mux.HandleFunc("GET /api/v1/status", a.auth(a.status))
	mux.HandleFunc("POST /api/v1/manager/show", a.auth(func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		d := a.desktop
		a.mu.Unlock()
		if d != nil {
			d.Show()
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("GET /api/v1/recommend", a.auth(a.recommend))
	mux.HandleFunc("GET /api/v1/capabilities", a.auth(a.capabilities))
	mux.HandleFunc("GET /api/v1/models", a.auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"models": a.installedSnapshot(), "deployments": a.deploymentViews()})
	}))
	mux.HandleFunc("POST /api/v1/deployments", a.auth(a.putDeployment))
	mux.HandleFunc("POST /api/v1/deployments/prepare", a.auth(a.prepareDeployment))
	mux.HandleFunc("POST /api/v1/deployments/delete", a.auth(a.deleteDeployment))
	mux.HandleFunc("POST /api/v1/deployments/rollback", a.auth(a.rollbackDeployment))
	mux.HandleFunc("GET /api/v1/deployments/logs", a.auth(a.deploymentLogs))
	mux.HandleFunc("GET /api/v1/storage/temp", a.auth(a.storageTemp))
	mux.HandleFunc("POST /api/v1/storage/cleanup", a.auth(a.storageCleanup))
	mux.HandleFunc("POST /api/v1/dictionary/sync", a.auth(a.dictionarySync))
	mux.HandleFunc("POST /api/v1/dictionary/clear", a.auth(a.dictionaryClear))
	mux.HandleFunc("GET /api/v1/dictionary/status", a.auth(a.dictionaryStatus))
	mux.HandleFunc("GET /api/v1/ocr/status", a.auth(a.ocrStatus))
	mux.HandleFunc("POST /api/v1/ocr/install", a.auth(a.ocrInstall))
	mux.HandleFunc("POST /api/v1/ocr/delete", a.auth(a.ocrDelete))
	mux.HandleFunc("POST /api/v1/translate", a.auth(a.translateDesktop))
	mux.HandleFunc("GET /api/v1/logs", a.auth(a.managerLogs))
	mux.HandleFunc("GET /api/v1/assets", a.auth(a.assetList))
	mux.HandleFunc("GET /api/v1/catalog", a.auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"repositories": catalog.Repositories})
	}))
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
	mux.HandleFunc("POST /api/v1/runtime/start", a.auth(a.runtimeAction("start")))
	mux.HandleFunc("POST /api/v1/runtime/stop", a.auth(a.runtimeAction("stop")))
	mux.HandleFunc("POST /api/v1/runtime/fallback-cpu", a.auth(a.fallbackCPU))
	mux.HandleFunc("POST /api/v1/runtime/restart", a.auth(a.runtimeAction("restart")))
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
		writeJSON(w, 200, map[string]bool{"ok": a.cancelOperation(v.ID)})
	}))
	mux.HandleFunc("POST /api/v1/update/check", a.auth(a.updateCheck))
	mux.HandleFunc("POST /api/v1/update/install", a.auth(a.updateInstall))
	mux.HandleFunc("/", a.auth(a.routeProxy))
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
			allowed := false
			if err == nil {
				if u.Scheme == "chrome-extension" {
					allowed = a.allowedExtensionID(u.Host)
				} else if u.Host == r.Host && (u.Scheme == "http" || u.Scheme == "https") {
					allowed = true
				}
			}
			if !allowed {
				fail(w, 403, "origin_rejected")
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, traceparent, X-Request-ID, X-Mistral-Auto-Start, X-Mistral-Async-Ready, X-Mistral-UI")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, traceparent, X-Mistral-Model-Revision, Retry-After")
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

var defaultAllowedExtensions = map[string]bool{
	"epficbaecbjclndbkkkdoliihpnbdejm": true,
	"edhbbapfabbclogflaljpghhfmkopohn": true,
}

func (a *App) allowedExtensionID(extID string) bool {
	id := strings.ToLower(strings.TrimSpace(extID))
	if defaultAllowedExtensions[id] {
		return true
	}
	root := a.root()
	root.mu.Lock()
	defer root.mu.Unlock()
	for _, custom := range root.config.AllowedExtensions {
		if strings.ToLower(strings.TrimSpace(custom)) == id {
			return true
		}
	}
	return false
}

func (a *App) authorized(r *http.Request) bool {
	expected := a.token()
	if expected == "" {
		return false
	}
	token := ""
	usedCookie := false
	if value := r.Header.Get("Authorization"); strings.HasPrefix(value, "Bearer ") {
		token = strings.TrimPrefix(value, "Bearer ")
	}
	if token == "" {
		if c, e := r.Cookie("mistral_gui_session"); e == nil && c.Value != "" {
			token = c.Value
			usedCookie = true
		}
	}
	if token == "" {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
		return false
	}
	// Protect Cookie-authenticated state-changing requests against cross-port localhost CSRF (M-S13).
	if usedCookie && r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
		if r.Header.Get("X-Mistral-UI") == "1" || r.Header.Get("Sec-Fetch-Site") == "same-origin" {
			return true
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			if u, err := url.Parse(origin); err == nil && u.Host == r.Host && (u.Scheme == "http" || u.Scheme == "https") {
				return true
			}
		}
		return false
	}
	return true
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
	if a.parent != nil {
		installed := a.parent.installedSnapshot()
		a.mu.Lock()
		a.installed = installed
		a.mu.Unlock()
	}
	if id := r.URL.Query().Get("model"); id != "" && a.parent == nil {
		child, e := a.resolveDeployment(id)
		if e != nil {
			fail(w, 404, e.Error())
			return
		}
		if child != a {
			child.status(w, r)
			return
		}
	}
	a.mu.Lock()
	c := a.config
	hasCloudKey := c.CloudAPIKey != ""
	c.Token = ""
	c.CloudAPIKey = ""
	s := a.state
	recovered := a.recoveredFromBackup
	evicted := append([]string(nil), a.evictedDeployments...)
	a.mu.Unlock()
	if s.CooldownUntil != nil && !time.Now().UTC().Before(*s.CooldownUntil) && s.State == "cooldown" {
		s.State = "failed"
		s.CooldownUntil = nil
	}
	ops := a.operationSnapshot()
	revision := ""
	selectedBackend := c.Backend
	for _, in := range a.installedSnapshot() {
		if in.Kind == "engine" && in.Path == c.Engine && c.Backend == "auto" {
			selectedBackend = in.Backend
		}
		if in.Kind == "model" && in.ID == c.Model && (c.ModelVersion == "" || c.ModelVersion == in.Version) {
			revision = in.Version + ":" + in.SHA256
			if c.ModelVersion == "" {
				c.ModelVersion = in.Version
			}
		}
	}
	var updateResult map[string]any
	if b, e := os.ReadFile(filepath.Join(a.dataDir, "update-status.json")); e == nil {
		_ = json.Unmarshal(b, &updateResult)
	}
	views := a.deploymentViews()
	fingerprint := ""
	for _, v := range views {
		if v.Model == c.Model {
			fingerprint = v.Fingerprint
			break
		}
	}
	writeJSON(w, 200, map[string]any{
		"status":                s,
		"settings":              c,
		"has_cloud_key":         hasCloudKey,
		"recovered_from_backup": recovered,
		"evicted_deployments":   evicted,
		"temp_storage":          scanTempStorage(a.dataDir),
		"dictionary_sync":       a.root().ensureDictionaryStore().Status(),
		"ocr":                   a.root().ensureOCRSupervisor().Status(),
		"operations":            ops,
		"platform":              runtime.GOOS + "/" + runtime.GOARCH,
		"version":               Version,
		"model_revision":        revision,
		"deployment_revision":   fingerprint,
		"deployments":           views,
		"endpoint":              "http://" + a.addr,
		"selected_backend":      selectedBackend,
		"update_result":         updateResult,
	})
}
func (a *App) settings(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Theme                 *string   `json:"theme"`
		Language              *string   `json:"language"`
		Backend               *string   `json:"backend"`
		Model                 *string   `json:"model"`
		ModelVersion          *string   `json:"model_version"`
		AutoStart             *bool     `json:"auto_start"`
		IdleMinutes           *int      `json:"idle_minutes"`
		Mirrors               *[]string `json:"mirrors"`
		AllowedExtensions     *[]string `json:"allowed_extensions"`
		MaxRunningDeployments *int      `json:"max_running_deployments"`
		CloudAPIURL           *string   `json:"cloud_api_url"`
		CloudAPIKey           *string   `json:"cloud_api_key"`
		ClearCloudKey         *bool     `json:"clear_cloud_key"`
		CloudModel            *string   `json:"cloud_model"`
		TranslateRouting      *string   `json:"translate_routing"`
		ScreenshotHotkey      *string   `json:"screenshot_hotkey"`
		LaunchAtLogin         *bool     `json:"launch_at_login"`
		CloudImageConfirmed   *bool     `json:"cloud_image_confirmed"`
	}
	if err := decode(r, &v); err != nil {
		fail(w, 400, "invalid_json")
		return
	}
	if v.Theme != nil && *v.Theme != "system" && *v.Theme != "light" && *v.Theme != "dark" {
		fail(w, 400, "invalid_theme")
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
	if v.MaxRunningDeployments != nil && (*v.MaxRunningDeployments < 0 || *v.MaxRunningDeployments > 32) {
		fail(w, 400, "invalid_tuning")
		return
	}
	if v.TranslateRouting != nil && *v.TranslateRouting != "local-first" && *v.TranslateRouting != "local-only" && *v.TranslateRouting != "cloud-only" {
		fail(w, 400, "invalid_routing")
		return
	}
	if v.CloudAPIURL != nil && strings.TrimSpace(*v.CloudAPIURL) != "" {
		u, e := url.Parse(strings.TrimSpace(*v.CloudAPIURL))
		if e != nil || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost"))) || u.Host == "" || u.User != nil {
			fail(w, 400, "invalid_endpoint")
			return
		}
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
	resetCD := false
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
		resetCD = true
	}
	if v.Theme != nil {
		a.config.Theme = *v.Theme
	}
	if v.Language != nil {
		a.config.Language = *v.Language
	}
	if v.Backend != nil {
		a.config.Backend = *v.Backend
		a.state.DegradedCPU = false
		resetCD = true
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
	if v.AllowedExtensions != nil {
		a.config.AllowedExtensions = *v.AllowedExtensions
	}
	if v.MaxRunningDeployments != nil {
		a.config.MaxRunningDeployments = *v.MaxRunningDeployments
	}
	if v.CloudAPIURL != nil {
		a.config.CloudAPIURL = strings.TrimSpace(*v.CloudAPIURL)
	}
	if v.ClearCloudKey != nil && *v.ClearCloudKey {
		a.config.CloudAPIKey = ""
	} else if v.CloudAPIKey != nil && strings.TrimSpace(*v.CloudAPIKey) != "" {
		a.config.CloudAPIKey = strings.TrimSpace(*v.CloudAPIKey)
	}
	if v.CloudModel != nil {
		a.config.CloudModel = strings.TrimSpace(*v.CloudModel)
	}
	if v.TranslateRouting != nil {
		a.config.TranslateRouting = *v.TranslateRouting
	}
	if v.ScreenshotHotkey != nil {
		a.config.ScreenshotHotkey = strings.TrimSpace(*v.ScreenshotHotkey)
	}
	if v.LaunchAtLogin != nil {
		if err := setLaunchAtLogin(*v.LaunchAtLogin); err != nil {
			a.mu.Unlock()
			fail(w, 500, err.Error())
			return
		}
		a.config.LaunchAtLogin = *v.LaunchAtLogin
	}
	if v.CloudImageConfirmed != nil {
		a.config.CloudImageConfirmed = *v.CloudImageConfirmed
	}
	a.mu.Unlock()
	if resetCD {
		a.resetCooldown()
	}
	if err := a.save(); err != nil {
		fail(w, 500, "settings_write_failed")
		return
	}
	if v.Language != nil {
		a.mu.Lock()
		desktop := a.desktop
		a.mu.Unlock()
		if desktop != nil {
			desktop.RefreshMenus()
		}
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
		b, _ := json.Marshal(map[string]any{"status": a.stateSnapshot(), "operations": a.operationSnapshot(), "deployments": a.deploymentViews()})
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
	if qBackend := r.URL.Query().Get("backend"); qBackend != "" {
		backend = qBackend
	}
	type modelResult struct {
		models []catalog.Asset
		err    error
	}
	result := make(chan modelResult, 1)
	go func() {
		models, e := a.catalogModels(ctx, r.URL.Query().Get("repository"), r.URL.Query().Get("refresh") == "1")
		result <- modelResult{models, e}
	}()
	engine, e := a.engineAsset(ctx, backend)
	mr := <-result
	models, me := mr.models, mr.err
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
		Kind       string `json:"kind"`
		ID         string `json:"id"`
		Repository string `json:"repository"`
		Backend    string `json:"backend"`
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
	if v.Backend != "" {
		backend = v.Backend
	}
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
		a.changeOp(id, func(o *Operation) {
			o.AssetKind = v.Kind
			o.AssetID = v.ID
			o.Repository = v.Repository
			o.Step = "checking"
		})
		var asset catalog.Asset
		var err error
		if v.Kind == "engine" {
			asset, err = a.engineAsset(ctx, backend)
		} else {
			var all []catalog.Asset
			all, err = a.catalogModels(ctx, v.Repository, false)
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

func (a *App) managerLogs(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	tok := a.config.Token
	ck := a.config.CloudAPIKey
	a.mu.Unlock()
	logs := ""
	if f, e := os.Open(filepath.Join(a.dataDir, "manager.log")); e == nil {
		defer f.Close()
		info, _ := f.Stat()
		if info != nil && info.Size() > 64<<10 {
			_, _ = f.Seek(-(64 << 10), 2)
		}
		b := make([]byte, 64<<10)
		n, _ := f.Read(b)
		logs = string(b[:n])
		if tok != "" {
			logs = strings.ReplaceAll(logs, tok, "[redacted]")
		}
		if ck != "" {
			logs = strings.ReplaceAll(logs, ck, "[redacted]")
		}
	}
	writeJSON(w, 200, map[string]string{"logs": logs})
}
func (a *App) devices(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, nvidiaSMI(), "--query-gpu=name,driver_version,memory.total,memory.used", "--format=csv,noheader,nounits")
	setProcess(c)
	b, err := c.Output()
	devSummary := detectDevice(ctx)
	info := map[string]any{
		"os":              runtime.GOOS,
		"arch":            runtime.GOARCH,
		"cpu":             true,
		"metal":           runtime.GOOS == "darwin" && runtime.GOARCH == "arm64",
		"cuda":            err == nil,
		"memory":          "unknown",
		"memory_bytes":    devSummary.Total,
		"available_bytes": devSummary.Available,
		"gpus":            []any{},
		"gpu_details":     devSummary.GPUs,
	}
	info["cpu_cores"] = runtime.NumCPU()
	if runtime.GOOS == "windows" {
		info["vc_runtime"] = vcRuntimeAvailable()
	}
	if devSummary.Total > 0 {
		info["memory"] = fmt.Sprintf("%.1f GiB", float64(devSummary.Total)/(1<<30))
	}
	if devSummary.Available > 0 {
		info["memory_available"] = fmt.Sprintf("%.1f GiB", float64(devSummary.Available)/(1<<30))
	}
	if err == nil && strings.TrimSpace(string(b)) != "" {
		info["gpus"] = strings.Split(strings.TrimSpace(string(b)), "\n")
	} else {
		info["cuda_reason"] = "nvidia_driver_unavailable"
		if len(devSummary.GPUs) > 0 {
			lines := make([]string, 0, len(devSummary.GPUs))
			for _, g := range devSummary.GPUs {
				lines = append(lines, fmt.Sprintf("%s, %s, %d, 0", g.Name, g.Driver, g.Total>>20))
			}
			info["gpus"] = lines
		}
	}
	writeJSON(w, 200, info)
}
