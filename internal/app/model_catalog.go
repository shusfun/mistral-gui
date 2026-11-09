package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/shusfun/mistral-gui/internal/catalog"
	"github.com/shusfun/mistral-gui/internal/download"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (a *App) catalogModels(ctx context.Context, repo string, refresh bool) ([]catalog.Asset, error) {
	if repo == "" {
		repo = catalog.ModelRepo
	}
	if !catalog.ValidRepository(repo) {
		return nil, errors.New("invalid_repository")
	}
	dest := filepath.Join(a.dataDir, "catalog", catalog.ModelID(repo, "catalog")+".json")
	if !refresh {
		if info, e := os.Stat(dest); e == nil && time.Since(info.ModTime()) < 24*time.Hour {
			var assets []catalog.Asset
			if b, e := os.ReadFile(dest); e == nil && json.Unmarshal(b, &assets) == nil {
				return assets, nil
			}
		}
	}
	assets, e := a.catalog.RepositoryModels(ctx, repo)
	if e != nil {
		return nil, e
	}
	b, _ := json.Marshal(assets)
	if e = os.MkdirAll(filepath.Dir(dest), 0700); e != nil {
		return nil, e
	}
	if e = os.WriteFile(dest+".tmp", b, 0600); e != nil {
		return nil, e
	}
	if e = os.Rename(dest+".tmp", dest); e != nil {
		return nil, e
	}
	return assets, nil
}
func (a *App) installModel(ctx context.Context, op string, asset catalog.Asset) error {
	if len(asset.Files) == 0 {
		return errors.New("model_files_missing")
	}
	root := filepath.Join(a.dataDir, "models", asset.ID, asset.Version)
	stage := root + ".staging"
	if e := os.MkdirAll(stage, 0700); e != nil {
		return e
	}
	var done int64
	a.mu.Lock()
	mirrors := append([]string(nil), a.config.Mirrors...)
	a.mu.Unlock()
	for _, file := range asset.Files {
		if !catalog.SafeModelPath(file.Path) {
			return errors.New("invalid_model_path")
		}
		urls := []string{file.URL}
		for _, m := range mirrors {
			urls = append(urls, strings.TrimRight(m, "/")+"/"+file.URL)
		}
		e := download.Download(ctx, download.Request{URLs: urls, Dest: filepath.Join(stage, filepath.FromSlash(file.Path)), SHA256: file.SHA256, GitSHA1: file.GitSHA1, MaxSize: file.Size, Headers: http.Header{"Traceparent": []string{a.operationTrace(op)}}, OnProgress: func(p download.Progress) {
			a.changeOp(op, func(o *Operation) {
				o.Progress = done + p.Done
				o.Total = asset.Size
				o.Step = "downloading"
				o.Source = p.Source
				o.Detail = file.Path
			})
		}})
		if e != nil {
			return e
		}
		done += file.Size
	}
	if asset.Format == "safetensors" {
		if b, e := os.ReadFile(filepath.Join(stage, "model.safetensors.index.json")); e == nil {
			var index struct {
				Weights map[string]string `json:"weight_map"`
			}
			if json.Unmarshal(b, &index) != nil {
				return errors.New("model_index_invalid")
			}
			for _, p := range index.Weights {
				if !catalog.SafeModelPath(p) {
					return errors.New("model_index_invalid")
				}
				if _, e = os.Stat(filepath.Join(stage, p)); e != nil {
					return fmt.Errorf("model_shard_missing: %s", p)
				}
			}
		}
	}
	a.changeOp(op, func(o *Operation) { o.Step = "verifying" })
	if _, e := os.Stat(root); e == nil {
		if e = verifyModelFiles(root, asset.Files); e != nil {
			return fmt.Errorf("model_existing_corrupt: %w", e)
		}
		_ = os.RemoveAll(stage)
	} else if e = os.Rename(stage, root); e != nil {
		return e
	}
	modelPath := root
	if asset.Format == "gguf" {
		modelPath = filepath.Join(root, filepath.FromSlash(asset.Files[0].Path))
	}
	a.mu.Lock()
	exists := false
	for _, in := range a.installed {
		if in.Path == modelPath {
			exists = true
		}
	}
	if !exists {
		a.installed = append(a.installed, Installed{asset, modelPath})
	}
	if a.engine == nil {
		if asset.ID == a.config.Model || a.config.Engine == "" {
			a.selectModelLocked(Installed{asset, modelPath})
		}
	}
	a.mu.Unlock()
	return a.save()
}

func verifyModelFiles(root string, files []catalog.ModelFile) error {
	for _, f := range files {
		if !catalog.SafeModelPath(f.Path) {
			return errors.New("invalid_model_path")
		}
		if e := download.VerifyFile(filepath.Join(root, filepath.FromSlash(f.Path)), f.SHA256, f.GitSHA1); e != nil {
			return fmt.Errorf("model_file_invalid: %s: %w", f.Path, e)
		}
	}
	if b, e := os.ReadFile(filepath.Join(root, "model.safetensors.index.json")); e == nil {
		var index struct {
			Weights map[string]string `json:"weight_map"`
		}
		if json.Unmarshal(b, &index) != nil || len(index.Weights) == 0 {
			return errors.New("model_index_invalid")
		}
		for _, p := range index.Weights {
			if !catalog.SafeModelPath(p) {
				return errors.New("model_index_invalid")
			}
			if _, e = os.Stat(filepath.Join(root, filepath.FromSlash(p))); e != nil {
				return fmt.Errorf("model_shard_missing: %s", p)
			}
		}
	}
	return nil
}
