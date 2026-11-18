package app

import (
	"context"
	"encoding/json"
	"fmt"
	archiveutil "github.com/shusfun/mistral-gui/internal/archive"
	"github.com/shusfun/mistral-gui/internal/download"
	"github.com/shusfun/mistral-gui/internal/update"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func (a *App) isUpdating() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.updating
}

func (a *App) setUpdating(v bool) {
	a.mu.Lock()
	a.updating = v
	a.mu.Unlock()
}

func (a *App) manifest(ctx context.Context) (update.Manifest, error) {
	if UpdateRepository == "" {
		return update.Manifest{}, fmt.Errorf("update_repository_unconfigured")
	}
	m, err := update.FetchSigned(ctx, &http.Client{Timeout: 30 * time.Second}, "https://github.com/"+UpdateRepository+"/releases/latest/download/release-manifest.json", UpdatePublicKey)
	if err != nil {
		return m, err
	}
	// Anti-replay watermark: reject signed manifests older than any version already observed (M-S21).
	a.mu.Lock()
	highest := a.config.HighestSeenVersion
	a.mu.Unlock()
	if highest != "" && update.IsNewer(m.Version, highest) {
		return update.Manifest{}, fmt.Errorf("manifest_replay_rejected")
	}
	if highest == "" || update.IsNewer(highest, m.Version) {
		a.mu.Lock()
		a.config.HighestSeenVersion = m.Version
		a.mu.Unlock()
		_ = a.save()
	}
	return m, nil
}

func (a *App) updateCheck(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	desktop := a.desktop
	a.mu.Unlock()
	// Clear stale update-status.json when user explicitly triggers a fresh update check (M-23).
	_ = os.Remove(filepath.Join(a.dataDir, "update-status.json"))
	if desktop != nil {
		v, e := desktop.CheckUpdate(r.Context())
		if e != nil {
			fail(w, 502, e.Error())
			return
		}
		writeJSON(w, 200, v)
		return
	}
	m, e := a.manifest(r.Context())
	if e != nil {
		fail(w, 502, e.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"current": Version, "available": update.IsNewer(Version, m.Version), "manifest": m})
}

func pruneOldUpdatePackages(updatesDir, keepFile string) {
	entries, err := os.ReadDir(updatesDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == keepFile || name == "recovery" || name == "apply.json" {
			continue
		}
		if strings.HasSuffix(name, ".update.zip") || strings.HasSuffix(name, ".update.tar.zst") || strings.Contains(name, "-mistral-gui-updater") {
			_ = os.RemoveAll(filepath.Join(updatesDir, name))
		}
	}
}

func (a *App) updateInstall(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	desktop := a.desktop
	a.mu.Unlock()
	if desktop != nil {
		fail(w, 409, "desktop_action_required")
		return
	}
	var reqBody struct {
		Force bool `json:"force"`
	}
	if r.ContentLength > 0 && decode(r, &reqBody) != nil {
		fail(w, 400, "invalid_json")
		return
	}
	a.taskStart.Lock()
	defer a.taskStart.Unlock()
	for _, o := range a.operationSnapshot() {
		if o.State == "running" {
			fail(w, 409, "task_in_progress")
			return
		}
	}
	// Protect active translation requests unless force: true is explicitly confirmed (M-S07).
	if active := a.activeRequestCount(); active > 0 && !reqBody.Force {
		writeJSON(w, 409, map[string]any{
			"error": map[string]any{
				"code":            "active_requests_running",
				"active_requests": active,
			},
		})
		return
	}
	op := a.operation("update", r.Header.Get("traceparent"), func(ctx context.Context, id string) error {
		m, e := a.manifest(ctx)
		if e != nil {
			return e
		}
		if !update.IsNewer(Version, m.Version) {
			return fmt.Errorf("already_current")
		}
		var asset update.Asset
		for _, as := range m.Assets {
			if as.OS == runtime.GOOS && as.Arch == runtime.GOARCH && as.Kind == "update" {
				asset = as
			}
		}
		if asset.URL == "" {
			return fmt.Errorf("update_asset_missing")
		}
		if e = checkDiskSpace(a.dataDir, asset.Size*3); e != nil {
			return e
		}
		suffix := ".update.zip"
		if runtime.GOOS == "darwin" {
			suffix = ".update.tar.zst"
		}
		updatesDir := filepath.Join(a.dataDir, "updates")
		keepName := m.Version + suffix
		dest := filepath.Join(updatesDir, keepName)
		a.mu.Lock()
		urls := []string{asset.URL}
		for _, mirror := range a.config.Mirrors {
			urls = append(urls, strings.TrimRight(mirror, "/")+"/"+asset.URL)
		}
		a.mu.Unlock()
		e = download.Download(ctx, download.Request{Headers: http.Header{"Traceparent": []string{a.operationTrace(id)}}, URLs: urls, Dest: dest, SHA256: asset.SHA256, MaxSize: asset.Size, OnProgress: func(p download.Progress) {
			a.changeOp(id, func(o *Operation) { o.Progress = p.Done; o.Total = p.Total; o.Step = "downloading" })
		}})
		if e != nil {
			return e
		}
		pruneOldUpdatePackages(updatesDir, keepName)
		exe, e := os.Executable()
		if e != nil {
			return e
		}
		target := exe
		if runtime.GOOS == "darwin" {
			i := strings.Index(exe, ".app/")
			if i < 0 {
				return fmt.Errorf("macos_app_required")
			}
			target = exe[:i+4]
		}
		stage := target + ".next"
		if e = os.RemoveAll(stage); e != nil {
			return e
		}
		unpack := filepath.Join(filepath.Dir(target), ".mistral-gui-unpack")
		if e = os.RemoveAll(unpack); e != nil {
			return e
		}
		if e = archiveutil.Extract(dest, unpack, 512<<20); e != nil {
			_ = os.RemoveAll(unpack)
			return e
		}
		staged := filepath.Join(unpack, "mistral-gui.exe")
		if runtime.GOOS == "darwin" {
			staged = filepath.Join(unpack, "mistral-gui.app")
		}
		if e = os.Rename(staged, stage); e != nil {
			_ = os.RemoveAll(unpack)
			return e
		}
		helperName := "mistral-gui-updater"
		if runtime.GOOS == "windows" {
			helperName += ".exe"
		}
		helperSrc := filepath.Join(unpack, helperName)
		helper := filepath.Join(updatesDir, id+"-"+helperName)
		b, e := os.ReadFile(helperSrc)
		_ = os.RemoveAll(unpack)
		if e != nil {
			return e
		}
		if e = os.WriteFile(helper, b, 0700); e != nil {
			return e
		}
		req := update.ApplyRequest{ParentPID: os.Getpid(), PreviousInstance: a.instance(), Target: target, Staged: stage, Executable: exe, Args: []string{"--addr", a.addr, "--data-dir", a.dataDir, "--no-open"}, HealthURL: "http://" + a.addr + "/healthz", Version: m.Version, StateFile: filepath.Join(a.dataDir, "update-status.json"), Traceparent: a.operationTrace(id)}
		b, _ = json.Marshal(req)
		request := filepath.Join(updatesDir, "apply.json")
		if e = os.WriteFile(request, b, 0600); e != nil {
			return e
		}
		a.setUpdating(true)
		c := exec.Command(helper, "--request", request)
		setProcess(c)
		if e = c.Start(); e != nil {
			a.setUpdating(false)
			return e
		}
		go func() { time.Sleep(time.Second); a.Close(); os.Exit(0) }()
		return nil
	})
	writeJSON(w, 202, op)
}
