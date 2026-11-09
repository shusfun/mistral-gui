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

func (a *App) manifest(ctx context.Context) (update.Manifest, error) {
	if UpdateRepository == "" {
		return update.Manifest{}, fmt.Errorf("update_repository_unconfigured")
	}
	return update.FetchSigned(ctx, &http.Client{Timeout: 30 * time.Second}, "https://github.com/"+UpdateRepository+"/releases/latest/download/release-manifest.json", UpdatePublicKey)
}
func (a *App) updateCheck(w http.ResponseWriter, r *http.Request) {
	m, e := a.manifest(r.Context())
	if e != nil {
		fail(w, 502, e.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"current": Version, "available": update.IsNewer(Version, m.Version), "manifest": m})
}
func (a *App) updateInstall(w http.ResponseWriter, r *http.Request) {
	a.taskStart.Lock()
	defer a.taskStart.Unlock()
	a.mu.Lock()
	for _, o := range a.operations {
		if o.State == "running" {
			a.mu.Unlock()
			fail(w, 409, "task_in_progress")
			return
		}
	}
	a.mu.Unlock()
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
		suffix := ".update.zip"
		if runtime.GOOS == "darwin" {
			suffix = ".update.tar.zst"
		}
		dest := filepath.Join(a.dataDir, "updates", m.Version+suffix)
		a.mu.Lock()
		urls := []string{asset.URL}
		for _, mirror := range a.config.Mirrors {
			urls = append(urls, strings.TrimRight(mirror, "/")+"/"+asset.URL)
		}
		a.mu.Unlock()
		e = download.Download(ctx, download.Request{Headers: http.Header{"Traceparent": []string{a.operationTrace(id)}}, URLs: urls, Dest: dest, SHA256: asset.SHA256, MaxSize: asset.Size, OnProgress: func(p download.Progress) {
			a.changeOp(id, func(o *Operation) { o.Progress = p.Done; o.Total = p.Total })
		}})
		if e != nil {
			return e
		}
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
			return e
		}
		staged := filepath.Join(unpack, "mistral-gui.exe")
		if runtime.GOOS == "darwin" {
			staged = filepath.Join(unpack, "mistral-gui.app")
		}
		if e = os.Rename(staged, stage); e != nil {
			return e
		}
		helperName := "mistral-gui-updater"
		if runtime.GOOS == "windows" {
			helperName += ".exe"
		}
		helperSrc := filepath.Join(unpack, helperName)
		helper := filepath.Join(a.dataDir, "updates", id+"-"+helperName)
		b, e := os.ReadFile(helperSrc)
		if e != nil {
			return e
		}
		if e = os.WriteFile(helper, b, 0700); e != nil {
			return e
		}
		req := update.ApplyRequest{ParentPID: os.Getpid(), PreviousInstance: a.instance(), Target: target, Staged: stage, Executable: exe, Args: []string{"--addr", a.addr, "--data-dir", a.dataDir, "--no-open"}, HealthURL: "http://" + a.addr + "/healthz", Version: m.Version, StateFile: filepath.Join(a.dataDir, "update-status.json"), Traceparent: a.operationTrace(id)}
		b, _ = json.Marshal(req)
		request := filepath.Join(a.dataDir, "updates", "apply.json")
		if e = os.WriteFile(request, b, 0600); e != nil {
			return e
		}
		c := exec.Command(helper, "--request", request)
		setProcess(c)
		if e = c.Start(); e != nil {
			return e
		}
		go func() { time.Sleep(time.Second); a.Close(); os.Exit(0) }()
		return nil
	})
	writeJSON(w, 202, op)
}
