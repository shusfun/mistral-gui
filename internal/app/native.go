package app

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

type networkConnContextKey struct{}

func markNetworkConnContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, networkConnContextKey{}, true)
}

func isNetworkConnRequest(r *http.Request) bool {
	v, _ := r.Context().Value(networkConnContextKey{}).(bool)
	return v
}

type Desktop interface {
	Ready() bool
	Show()
	RefreshMenus()
	CheckUpdate(context.Context) (any, error)
	InstallUpdate(context.Context, string, string) error
}

func (a *App) AttachDesktop(d Desktop) { a.mu.Lock(); a.desktop = d; a.mu.Unlock() }
func (a *App) Closed() <-chan struct{} { return a.closed }
func (a *App) Address() string         { return a.addr }
func (a *App) DataDir() string         { return a.dataDir }
func (a *App) Language() string        { a.mu.Lock(); defer a.mu.Unlock(); return a.config.Language }
func (a *App) ScreenshotHotkey() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.config.ScreenshotHotkey
}
func (a *App) DesktopUpdate(trace string) Operation {
	return a.operation("update", trace, func(ctx context.Context, id string) error { return a.desktop.InstallUpdate(ctx, trace, id) })
}
func (a *App) UpdateProgress(id string, done, total int64, step string) {
	a.changeOp(id, func(o *Operation) { o.Progress = done; o.Total = total; o.Step = step })
}
func (a *App) NativeTrace(kind, trace string) {
	slog.Info("desktop.action", "traceId", traceRoot(trace), "action", kind)
}
func (a *App) Mirrors() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.config.Mirrors...)
}
func (a *App) OpenOfficialUI(target string) {
	u, _ := url.Parse(target)
	id := ""
	if u != nil {
		id = u.Query().Get("instance")
	}
	code := a.issueExchangeCode()
	_ = openURL("http://" + a.addr + "/manage#exchange=" + code + "&ui=" + url.QueryEscape(id))
}

// DesktopHandler is strictly for in-process Wails WebView asset routing (never mount on a TCP listener!).
// Any request arriving over a network socket (marked by Server.ConnContext) is rejected with 403 (M-S25).
func (a *App) DesktopHandler() http.Handler {
	handler := a.Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isNetworkConnRequest(r) {
			fail(w, 403, "desktop_inprocess_only")
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/manage") && !strings.HasPrefix(r.URL.Path, "/api/v1/") && r.URL.Path != "/healthz" && r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		r = r.Clone(r.Context())
		r.Header = r.Header.Clone()
		r.Host = a.addr
		r.Header.Del("Origin")
		r.Header.Set("Authorization", "Bearer "+a.token())
		handler.ServeHTTP(w, r)
	})
}

func (a *App) CompletedOperation(id string) (Operation, bool) {
	a.mu.Lock()
	for _, op := range a.operations {
		if op.ID == id {
			a.mu.Unlock()
			return op, op.State == "success" || op.State == "failed" || op.State == "cancelled"
		}
	}
	a.mu.Unlock()
	for _, child := range a.children() {
		if op, ok := child.CompletedOperation(id); ok {
			return op, true
		}
	}
	return Operation{}, false
}

func (a *App) notifyCompletion(id string) {
	root := a
	if a.parent != nil {
		root = a.parent
	}
	root.mu.Lock()
	d := root.desktop
	root.mu.Unlock()
	if sink, ok := d.(interface{ TaskCompleted(string) }); ok {
		sink.TaskCompleted(id)
	}
}

func (a *App) cancelOperation(id string) bool {
	a.mu.Lock()
	cancel := a.cancels[id]
	a.mu.Unlock()
	if cancel != nil {
		slog.Info("operation.cancel", "traceId", traceRoot(a.operationTrace(id)), "operation", id)
		cancel()
		return true
	}
	for _, child := range a.children() {
		if child.cancelOperation(id) {
			return true
		}
	}
	return false
}
