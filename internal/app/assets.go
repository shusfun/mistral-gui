package app

import (
	"context"
	"errors"
	archiveutil "github.com/shusfun/mistral-gui/internal/archive"
	"github.com/shusfun/mistral-gui/internal/catalog"
	"github.com/shusfun/mistral-gui/internal/download"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func (a *App) install(ctx context.Context, id string, asset catalog.Asset) error {
	if err := a.acquireResources(ctx); err != nil {
		return err
	}
	defer a.releaseResources()
	return a.installResource(ctx, id, asset)
}
func (a *App) installResource(ctx context.Context, id string, asset catalog.Asset) error {
	if asset.Kind == "model" && len(asset.Files) > 0 {
		return a.installModel(ctx, id, asset)
	}
	if strings.ContainsAny(asset.ID, `/\:`) || strings.ContainsAny(asset.Version, `/\:`) {
		return errors.New("invalid_asset")
	}
	a.mu.Lock()
	mirrors := append([]string(nil), a.config.Mirrors...)
	a.mu.Unlock()
	urls := []string{asset.URL}
	for _, m := range mirrors {
		urls = append(urls, strings.TrimRight(m, "/")+"/"+asset.URL)
	}
	dest := filepath.Join(a.dataDir, "downloads", asset.Version+"-"+asset.ID)
	err := download.Download(ctx, download.Request{Headers: http.Header{"Traceparent": []string{a.operationTrace(id)}}, URLs: urls, Dest: dest, SHA256: asset.SHA256, MaxSize: asset.Size, Idle: 60 * time.Second,
		OnProgress: func(p download.Progress) {
			a.changeOp(id, func(o *Operation) {
				o.Progress = p.Done
				o.Total = p.Total
				o.Source = p.Source
				o.Step = "downloading"
			})
		},
		OnStep: func(s string) {
			a.changeOp(id, func(o *Operation) {
				o.Step = "downloading"
				if strings.Contains(s, "校验") {
					o.Step = "verifying"
				}
			})
		},
	})
	if err != nil {
		return err
	}
	path := dest
	if asset.Kind == "engine" {
		dir := filepath.Join(a.dataDir, "engines", asset.Version, asset.Backend)
		staging := dir + ".staging"
		if err = os.RemoveAll(staging); err != nil {
			return err
		}
		if err = archiveutil.Extract(dest, staging, 2<<30); err != nil {
			_ = os.RemoveAll(staging)
			return err
		}
		name := "mistralrs"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		err = filepath.WalkDir(staging, func(p string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if !d.IsDir() && d.Name() == name {
				rel, _ := filepath.Rel(staging, p)
				path = filepath.Join(dir, rel)
			}
			return nil
		})
		if err != nil || path == dest {
			return errors.New("engine_binary_missing")
		}
		if _, err = os.Stat(dir); err == nil {
			_ = os.RemoveAll(staging)
		} else if err = os.Rename(staging, dir); err != nil {
			return err
		}
	} else {
		dir := filepath.Join(a.dataDir, "models", asset.Version)
		if err = os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		path = filepath.Join(dir, asset.ID)
		if err = os.Rename(dest, path); err != nil {
			return err
		}
	}
	a.lifecycle.Lock()
	defer a.lifecycle.Unlock()
	a.mu.Lock()
	exists := false
	for _, in := range a.installed {
		if in.Path == path {
			exists = true
		}
	}
	if !exists {
		a.installed = append(a.installed, Installed{asset, path})
	}
	a.mu.Unlock()
	return a.save()
}
func (a *App) engineAsset(ctx context.Context, backend string) (catalog.Asset, error) {
	if backend == "auto" {
		backend = "cpu"
		if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
			backend = "metal"
		}
	}
	if backend != "cpu" && backend != "cuda" && backend != "metal" {
		return catalog.Asset{}, errors.New("backend_invalid")
	}
	return a.catalog.SelectEngine(ctx, SupplementRepository, "", runtime.GOOS, runtime.GOARCH, backend)
}
func (a *App) installedSnapshot() []Installed {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]Installed(nil), a.installed...)
}
func (a *App) deleteModel(id, version string) error {
	a.lifecycle.Lock()
	defer a.lifecycle.Unlock()
	for _, child := range a.children() {
		child.lifecycle.Lock()
		defer child.lifecycle.Unlock()
		child.mu.Lock()
		used := child.engine != nil && child.state.Model == id
		child.mu.Unlock()
		if used {
			return errors.New("model_in_use")
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.engine != nil && a.state.Model == id {
		return errors.New("model_in_use")
	}
	for i, in := range a.installed {
		if in.Kind == "model" && in.ID == id && (version == "" || in.Version == version) {
			remove := os.Remove
			if in.Format == "safetensors" {
				remove = os.RemoveAll
			}
			if err := remove(in.Path); err != nil {
				return err
			}
			a.installed = append(a.installed[:i], a.installed[i+1:]...)
			return nil
		}
	}
	return errors.New("model_not_installed")
}
func (a *App) exportDiagnostics() any {
	a.mu.Lock()
	defer a.mu.Unlock()
	logs := ""
	if f, e := os.Open(filepath.Join(a.dataDir, "manager.log")); e == nil {
		defer f.Close()
		info, _ := f.Stat()
		if info != nil && info.Size() > 64<<10 {
			_, _ = f.Seek(-(64 << 10), 2)
		}
		b := make([]byte, 64<<10)
		n, _ := f.Read(b)
		logs = strings.ReplaceAll(string(b[:n]), a.config.Token, "[redacted]")
	}
	return map[string]any{"version": Version, "platform": runtime.GOOS + "/" + runtime.GOARCH, "status": a.state, "operations": a.operations, "logs": logs}
}
