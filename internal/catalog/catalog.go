package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const Upstream = "EricLBuehler/mistral.rs"
const ModelRepo = "tencent/HY-MT1.5-1.8B-GGUF"
const DefaultModel = "HY-MT1.5-1.8B-Q4_K_M.gguf"

type Asset struct {
	Repository string      `json:"repository,omitempty"`
	Format     string      `json:"format,omitempty"`
	Files      []ModelFile `json:"files,omitempty"`
	ID         string      `json:"id"`
	Kind       string      `json:"kind"`
	Version    string      `json:"version"`
	OS         string      `json:"os"`
	Arch       string      `json:"arch"`
	Backend    string      `json:"backend"`
	URL        string      `json:"url"`
	SHA256     string      `json:"sha256"`
	Size       int64       `json:"size"`
	Source     string      `json:"source"`
	Name       string      `json:"name"`
}
type Release struct {
	Tag    string `json:"tag_name"`
	Assets []struct {
		Name   string `json:"name"`
		URL    string `json:"browser_download_url"`
		Digest string `json:"digest"`
		Size   int64  `json:"size"`
	} `json:"assets"`
}
type Client struct {
	HTTP                *http.Client
	API                 string
	SupplementTagPrefix string
}

func (c Client) get(ctx context.Context, raw string, out any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "mistral-gui")
	req.Header.Set("Accept", "application/vnd.github+json")
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("metadata_http_%d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out)
}
func (c Client) Engine(ctx context.Context, repo, tag, goos, arch, backend string) (Asset, error) {
	api := c.API
	if api == "" {
		api = "https://api.github.com"
	}
	suffix := "latest"
	if tag != "" {
		suffix = "tags/" + tag
	}
	var r Release
	if err := c.get(ctx, api+"/repos/"+repo+"/releases/"+suffix, &r); err != nil {
		return Asset{}, err
	}
	target := ""
	switch goos + "/" + arch {
	case "windows/amd64":
		target = "x86_64-pc-windows-msvc"
	case "darwin/amd64":
		target = "x86_64-apple-darwin"
	case "darwin/arm64":
		target = "aarch64-apple-darwin"
	default:
		return Asset{}, errors.New("platform_unsupported")
	}
	baseName := "mistralrs-" + backend + "-" + target
	for _, a := range r.Assets {
		if a.Name == baseName+".zip" || a.Name == baseName+".tar.gz" || a.Name == baseName+".tar.zst" {
			sum := strings.TrimPrefix(a.Digest, "sha256:")
			if len(sum) != 64 {
				return Asset{}, errors.New("official_digest_missing")
			}
			source := "official"
			if repo != Upstream {
				source = "supplement"
			}
			return Asset{ID: a.Name, Name: a.Name, Kind: "engine", Version: r.Tag, OS: goos, Arch: arch, Backend: backend, URL: a.URL, SHA256: sum, Size: a.Size, Source: source}, nil
		}
	}
	return Asset{}, errors.New("asset_missing")
}

// SelectEngine never replaces an existing official asset with a supplemental build.
func (c Client) SelectEngine(ctx context.Context, supplement, tag, goos, arch, backend string) (Asset, error) {
	a, err := c.Engine(ctx, Upstream, tag, goos, arch, backend)
	if err == nil {
		return a, nil
	}
	if err.Error() != "asset_missing" {
		return Asset{}, err
	}
	if supplement == "" {
		return Asset{}, errors.New("supplement_repository_required")
	}
	if tag == "" {
		api := c.API
		if api == "" {
			api = "https://api.github.com"
		}
		var release Release
		if e := c.get(ctx, api+"/repos/"+Upstream+"/releases/latest", &release); e != nil {
			return Asset{}, e
		}
		tag = release.Tag
	}
	a, err = c.Engine(ctx, supplement, c.SupplementTagPrefix+tag, goos, arch, backend)
	if err == nil {
		a.Version = tag
	}
	return a, err
}
func (c Client) Models(ctx context.Context) ([]Asset, error) {
	var info struct {
		SHA string `json:"sha"`
	}
	if err := c.get(ctx, "https://huggingface.co/api/models/"+ModelRepo, &info); err != nil {
		return nil, err
	}
	var files []struct {
		Path string `json:"path"`
		Size int64  `json:"size"`
		LFS  *struct {
			OID string `json:"oid"`
		} `json:"lfs"`
	}
	if err := c.get(ctx, "https://huggingface.co/api/models/"+ModelRepo+"/tree/"+info.SHA, &files); err != nil {
		return nil, err
	}
	var out []Asset
	for _, f := range files {
		if strings.HasSuffix(f.Path, ".gguf") && f.LFS != nil && len(f.LFS.OID) == 64 {
			out = append(out, Asset{ID: f.Path, Name: f.Path, Kind: "model", Version: info.SHA, Backend: "any", URL: "https://huggingface.co/" + ModelRepo + "/resolve/" + info.SHA + "/" + f.Path + "?download=true", SHA256: f.LFS.OID, Size: f.Size, Source: "official"})
		}
	}
	return out, nil
}
