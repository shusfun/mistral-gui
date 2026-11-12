package desktop

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/shusfun/mistral-gui/internal/app"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/icons"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
	"github.com/wailsapp/wails/v3/pkg/updater"
)

type Service struct {
	manager           *app.App
	host              *application.App
	window            *application.WebviewWindow
	mu                sync.Mutex
	ready             bool
	updating          bool
	version           string
	refreshMenus      func()
	notifier          *notifications.NotificationService
	notified          map[string]bool
	notificationOrder []string
	confirmations     map[string]chan bool
}

func (s *Service) RefreshMenus() {
	if s.refreshMenus != nil {
		s.refreshMenus()
	}
}

func (s *Service) text(zh, en string) string {
	if s.manager.Language() == "en-US" {
		return en
	}
	return zh
}
func (s *Service) Show()       { s.window.Show(); s.window.Restore(); s.window.Focus() }
func (s *Service) Ready() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.ready }
func (s *Service) Command(kind, value, trace string) (any, error) {
	s.manager.NativeTrace(kind, trace)
	switch kind {
	case "ready":
		s.mu.Lock()
		s.ready = true
		s.mu.Unlock()
		return true, nil
	case "confirm-result":
		var reply struct {
			ID       string `json:"id"`
			Accepted bool   `json:"accepted"`
		}
		if json.Unmarshal([]byte(value), &reply) != nil {
			return nil, errors.New("invalid_confirmation")
		}
		s.mu.Lock()
		pending := s.confirmations[reply.ID]
		s.mu.Unlock()
		if pending != nil {
			select {
			case pending <- reply.Accepted:
			default:
			}
		}
		return nil, nil
	case "task-notification":
		return s.notifyTask(value)
	case "export":
		path, e := s.host.Dialog.SaveFile().SetFilename("mistral-gui-diagnostics.json").AddFilter("JSON", "*.json").AttachToWindow(s.window).PromptForSingleSelection()
		if e != nil || path == "" {
			return nil, e
		}
		req := httptest.NewRequest("GET", "http://"+s.manager.Address()+"/api/v1/diagnostics", nil)
		req.Header.Set("traceparent", trace)
		w := httptest.NewRecorder()
		s.manager.DesktopHandler().ServeHTTP(w, req)
		if w.Code != 200 {
			return nil, errors.New("diagnostics_failed")
		}
		return nil, os.WriteFile(path, w.Body.Bytes(), 0600)
	case "open-ui":
		u := "http://" + s.manager.Address() + "/ui"
		if value != "" {
			u += "?instance=" + value
		}
		s.manager.OpenOfficialUI(u)
		return nil, nil
	case "import-gguf", "import-directory":
		dialog := s.host.Dialog.OpenFile().AttachToWindow(s.window)
		if kind == "import-directory" {
			dialog.CanChooseDirectories(true).CanChooseFiles(false)
		} else {
			dialog.CanChooseFiles(true).AddFilter("GGUF", "*.gguf")
		}
		path, e := dialog.PromptForSingleSelection()
		if e != nil || path == "" {
			return nil, e
		}
		return s.manager.ImportModel(path, trace)
	case "update":
		return s.manager.DesktopUpdate(trace), nil
	case "quit":
		s.manager.Close()
		s.host.Quit()
		return nil, nil
	default:
		return nil, errors.New("desktop_command_unknown")
	}
}
func Run(manager *app.App, hidden bool) error {
	s := &Service{manager: manager, notifier: notifications.New(), notified: map[string]bool{}, confirmations: map[string]chan bool{}}
	s.host = application.New(application.Options{Name: "mistral-gui", Description: "Local model manager", Services: []application.Service{application.NewService(s), application.NewService(s.notifier)}, Assets: application.AssetOptions{Handler: manager.DesktopHandler()}, Mac: application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: false}, OnShutdown: manager.Close})
	s.window = s.host.Window.NewWithOptions(application.WebviewWindowOptions{Title: "mistral-gui", Width: 1121, Height: 821, MinWidth: 861, MinHeight: 621, Frameless: runtime.GOOS != "darwin", Windows: application.WindowsWindow{NonClientRegionSupport: true, WebView2CompositionHosting: true}, Mac: application.MacWindow{TitleBar: application.MacTitleBarHidden, InvisibleTitleBarHeight: 47}, URL: "/manage?desktop=1", Hidden: hidden})
	s.window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) { s.window.Hide(); e.Cancel() })
	s.host.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(e *application.ApplicationEvent) { s.Show() })
	tray := s.host.SystemTray.New()
	tray.SetTooltip("mistral-gui")
	if runtime.GOOS == "darwin" {
		tray.SetTemplateIcon(icons.SystrayMacTemplate)
	} else {
		tray.SetIcon(icons.SystrayLight)
	}
	buildMenu := func() *application.Menu {
		menu := s.host.NewMenu()
		menu.Add(s.text("显示窗口", "Show window")).OnClick(func(*application.Context) { s.Show() })
		menu.Add(s.text("检查更新", "Check for updates")).OnClick(func(*application.Context) {
			s.Show()
			s.window.SetURL("/manage?desktop=1#updates")
			go func() {
				_, e := s.CheckUpdate(context.Background())
				if e != nil {
					slog.Error("update.check", "error", e)
				}
			}()
		})
		menu.AddSeparator()
		menu.Add(s.text("退出", "Quit")).OnClick(func(*application.Context) { manager.Close(); s.host.Quit() })
		return menu
	}
	tray.OnClick(s.Show)
	s.refreshMenus = func() {
		tray.SetMenu(buildMenu())
		menu := s.host.NewMenu()
		if runtime.GOOS == "darwin" {
			menu.AddRole(application.AppMenu)
			menu.AddRole(application.EditMenu)
			menu.AddRole(application.WindowMenu)
		}
		managerMenu := menu.AddSubmenu(s.text("管理器", "Manager"))
		managerMenu.Add(s.text("显示窗口", "Show window")).OnClick(func(*application.Context) { s.Show() })
		managerMenu.Add(s.text("检查更新", "Check for updates")).OnClick(func(*application.Context) { s.Show(); s.window.SetURL("/manage?desktop=1#updates") })
		managerMenu.Add(s.text("退出", "Quit")).OnClick(func(*application.Context) { manager.Close(); s.host.Quit() })
		s.host.Menu.Set(menu)
	}
	s.notifier.OnNotificationResponse(func(notifications.NotificationResult) { s.Show(); s.window.SetURL("/manage?desktop=1#activity") })
	s.RefreshMenus()
	manager.AttachDesktop(s)
	if app.UpdateRepository != "" && app.UpdatePublicKey != "" {
		pub, e := base64.StdEncoding.DecodeString(app.UpdatePublicKey)
		if e != nil {
			return e
		}
		p, e := newProvider(manager)
		if e != nil {
			return e
		}
		if e = s.host.Updater.Init(updater.Config{CurrentVersion: strings.TrimPrefix(app.Version, "v"), Providers: []updater.Provider{p}, PublicKey: pub, Window: updater.WindowNone}); e != nil {
			return e
		}
	}
	go func() { <-manager.Closed(); s.host.Quit() }()
	go func() {
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-manager.Closed():
				return
			case <-ticker.C:
				if _, e := s.CheckUpdate(context.Background()); e != nil {
					slog.Debug("update.check", "error", e)
				}
			}
		}
	}()
	return s.host.Run()
}
func (s *Service) CheckUpdate(ctx context.Context) (any, error) {
	r, e := s.host.Updater.Check(ctx)
	if e != nil {
		return nil, e
	}
	if r == nil {
		return map[string]any{"available": false}, nil
	}
	s.mu.Lock()
	s.version = r.Version
	s.mu.Unlock()
	return map[string]any{"available": true, "manifest": map[string]any{"version": r.Version, "notes": r.Notes}}, nil
}
func (s *Service) InstallUpdate(ctx context.Context, trace, id string) error {
	ctx = withDownloadTrace(ctx, trace)
	s.mu.Lock()
	if s.updating {
		s.mu.Unlock()
		return errors.New("update_in_progress")
	}
	s.updating = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.updating = false; s.mu.Unlock() }()
	if _, e := s.CheckUpdate(ctx); e != nil {
		return e
	}
	off := s.host.Event.On(updater.EventDownloadProgress, func(e *application.CustomEvent) {
		if p, ok := e.Data.(updater.Progress); ok {
			s.manager.UpdateProgress(id, p.Written, p.Total, "downloading")
		}
	})
	defer off()
	if e := s.host.Updater.DownloadAndInstall(ctx); e != nil {
		return e
	}
	s.manager.UpdateProgress(id, 1, 1, "ready")
	if !s.confirm(ctx, trace, s.text("更新已准备好。立即停止模型并重启？", "The update is ready. Stop models and restart now?")) {
		return nil
	}
	s.mu.Lock()
	version := s.version
	s.mu.Unlock()
	transaction, e := prepareGuard(s.manager, version, trace, !s.window.IsVisible())
	if e != nil {
		return e
	}
	if e = s.host.Updater.Restart(ctx); e != nil {
		cancelGuard(transaction)
		return e
	}
	return nil
}

func (s *Service) notifyTask(id string) (bool, error) {
	op, ok := s.manager.CompletedOperation(id)
	if !ok || (op.Kind != "install" && op.Kind != "start" && op.Kind != "restart" && op.Kind != "update" && op.Kind != "prepare") {
		return false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if sent, found := s.notified[id]; found {
		return sent, nil
	}
	if len(s.notificationOrder) >= 200 {
		delete(s.notified, s.notificationOrder[0])
		s.notificationOrder = s.notificationOrder[1:]
	}
	s.notificationOrder = append(s.notificationOrder, id)
	if s.window.IsFocused() && s.window.IsVisible() {
		s.notified[id] = false
		s.manager.NativeTrace("notification.foreground", op.TraceID)
		return false, nil
	}
	allowed, err := s.notifier.CheckNotificationAuthorization()
	if err != nil || !allowed {
		s.notified[id] = true
		return true, nil
	}
	body := s.text("任务已完成，可返回管理器查看。", "Task completed. Open the manager to review.")
	if op.State == "failed" {
		body = s.text("任务失败，请返回管理器查看原因。", "Task failed. Open the manager for details.")
	}
	s.notified[id] = true
	err = s.notifier.SendNotification(notifications.NotificationOptions{ID: op.ID, Title: "mistral-gui", Body: body})
	if err == nil {
		s.manager.NativeTrace("notification.native", op.TraceID)
		s.notified[id] = true
	}
	return true, err
}

func (s *Service) TaskCompleted(id string) {
	if _, err := s.notifyTask(id); err != nil {
		slog.Warn("notification.failed", "reason", "NATIVE_NOTIFICATION_UNAVAILABLE")
	}
}

func (s *Service) confirm(ctx context.Context, trace, message string) bool {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return false
	}
	id := hex.EncodeToString(nonce[:])
	answer := make(chan bool, 1)
	s.mu.Lock()
	s.confirmations[id] = answer
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.confirmations, id); s.mu.Unlock() }()
	s.Show()
	s.host.Event.Emit("desktop:confirm", map[string]string{"id": id, "message": message, "trace": trace})
	select {
	case accepted := <-answer:
		return accepted
	case <-ctx.Done():
		return false
	}
}
