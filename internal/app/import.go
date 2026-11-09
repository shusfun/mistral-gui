package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/shusfun/mistral-gui/internal/catalog"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func (a *App) ImportModel(source, trace string) (Operation, error) {
	info, e := os.Stat(source)
	if e != nil {
		return Operation{}, e
	}
	if !info.IsDir() && !strings.HasSuffix(strings.ToLower(source), ".gguf") {
		return Operation{}, errors.New("model_format_unsupported")
	}
	return a.operation("import", trace, func(ctx context.Context, id string) error {
		format := "gguf"
		root := filepath.Dir(source)
		var files []string
		if info.IsDir() {
			format = "safetensors"
			root = source
			e := filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
				if e != nil {
					return e
				}
				if d.Type()&os.ModeSymlink != 0 {
					return errors.New("model_symlink_unsupported")
				}
				if !d.IsDir() {
					ext := strings.ToLower(filepath.Ext(p))
					if ext == ".safetensors" || ext == ".json" || ext == ".model" || ext == ".txt" || ext == ".jinja" {
						files = append(files, p)
					}
				}
				return nil
			})
			if e != nil {
				return e
			}
			for _, name := range []string{"config.json"} {
				if _, e := os.Stat(filepath.Join(root, name)); e != nil {
					return errors.New("model_support_files_missing")
				}
			}
			if _, e := os.Stat(filepath.Join(root, "tokenizer.json")); e != nil {
				if _, e = os.Stat(filepath.Join(root, "tokenizer.model")); e != nil {
					return errors.New("model_support_files_missing")
				}
			}
		} else {
			files = []string{source}
		}
		asset := catalog.Asset{ID: catalog.ModelID(source, format), Kind: "model", Name: filepath.Base(source), Format: format, Source: "local", Backend: "any"}
		var manifest []catalog.ModelFile
		hasWeight := format == "gguf"
		for _, p := range files {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			f, e := os.Open(p)
			if e != nil {
				return e
			}
			h := sha256.New()
			n, e := io.Copy(h, f)
			f.Close()
			if e != nil {
				return e
			}
			rel, _ := filepath.Rel(root, p)
			manifest = append(manifest, catalog.ModelFile{Path: filepath.ToSlash(rel), Size: n, SHA256: hex.EncodeToString(h.Sum(nil))})
			asset.Size += n
			if strings.HasSuffix(p, ".safetensors") {
				hasWeight = true
			}
		}
		if !hasWeight {
			return errors.New("model_weights_missing")
		}
		b, _ := json.Marshal(manifest)
		sum := sha256.Sum256(b)
		asset.Version = hex.EncodeToString(sum[:])
		asset.SHA256 = asset.Version
		asset.Files = manifest
		dest := filepath.Join(a.dataDir, "models", asset.ID, asset.Version)
		stage := dest + ".importing"
		if e = os.MkdirAll(stage, 0700); e != nil {
			return e
		}
		for i, p := range files {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			target := filepath.Join(stage, filepath.FromSlash(manifest[i].Path))
			if e = os.MkdirAll(filepath.Dir(target), 0700); e != nil {
				return e
			}
			in, e := os.Open(p)
			if e != nil {
				return e
			}
			out, e := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
			if e != nil {
				in.Close()
				return e
			}
			_, e = io.Copy(out, in)
			in.Close()
			closeErr := out.Close()
			if e != nil {
				return e
			}
			if closeErr != nil {
				return closeErr
			}
			a.UpdateProgress(id, int64(i+1), int64(len(files)), "importing")
		}
		if e = verifyModelFiles(stage, manifest); e != nil {
			return e
		}
		if _, e = os.Stat(dest); e == nil {
			if e = verifyModelFiles(dest, manifest); e != nil {
				return e
			}
			_ = os.RemoveAll(stage)
		} else if e = os.Rename(stage, dest); e != nil {
			return e
		}
		if format == "gguf" {
			dest = filepath.Join(dest, filepath.Base(source))
		}
		a.mu.Lock()
		for _, in := range a.installed {
			if in.Path == dest {
				a.mu.Unlock()
				return nil
			}
		}
		a.installed = append(a.installed, Installed{asset, dest})
		a.mu.Unlock()
		return a.save()
	}), nil
}
