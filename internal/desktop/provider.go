package desktop

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/shusfun/mistral-gui/internal/app"
	"github.com/shusfun/mistral-gui/internal/download"
	"github.com/wailsapp/wails/v3/pkg/updater"
	gh "github.com/wailsapp/wails/v3/pkg/updater/providers/github"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type provider struct {
	official *gh.Provider
	manager  *app.App
}

type downloadTraceKey struct{}

func withDownloadTrace(ctx context.Context, trace string) context.Context {
	return context.WithValue(ctx, downloadTraceKey{}, trace)
}

func updateName(platform, arch string) string {
	label := "x64"
	if arch == "arm64" {
		label = "arm64"
	}
	if platform == "windows" {
		return "mistral-gui-windows-" + label + ".wails.update.zip"
	}
	return "mistral-gui-macos-" + label + ".update.tar.gz"
}
func newProvider(a *app.App) (*provider, error) {
	p, e := gh.New(gh.Config{Repository: app.UpdateRepository, ChecksumAsset: "SHA256SUMS", AssetMatcher: func(r updater.CheckRequest, assets []gh.ReleaseAsset) int {
		for i, asset := range assets {
			if asset.Name == updateName(r.Platform, r.Arch) {
				return i
			}
		}
		return -1
	}})
	if e != nil {
		return nil, e
	}
	return &provider{p, a}, nil
}
func (p *provider) Name() string { return "github-signed-resumable" }
func (p *provider) Check(ctx context.Context, r updater.CheckRequest) (*updater.Release, error) {
	rel, e := p.official.Check(ctx, r)
	if e != nil || rel == nil {
		return rel, e
	}
	if rel.Verification == nil || len(rel.Verification.Digest) != 32 {
		return nil, errors.New("update_digest_missing")
	}
	raw, ok := rel.Metadata["github.asset.url"].(string)
	if !ok {
		return nil, errors.New("update_asset_missing")
	}
	req, e := http.NewRequestWithContext(ctx, "GET", raw+".sig", nil)
	if e != nil {
		return nil, e
	}
	if trace, ok := ctx.Value(downloadTraceKey{}).(string); ok {
		req.Header.Set("traceparent", trace)
	}
	client := http.Client{Timeout: 30 * time.Second}
	resp, e := client.Do(req)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("update_signature_http_%d", resp.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if e != nil {
		return nil, e
	}
	sig, e := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if e != nil || len(sig) != 64 {
		return nil, errors.New("update_signature_invalid")
	}
	rel.Verification.Signature = sig
	rel.Verification.SignatureAlgo = "ed25519"
	return rel, nil
}
func (p *provider) Download(ctx context.Context, r *updater.Release, dst io.Writer, progress func(int64, int64)) error {
	raw, ok := r.Metadata["github.asset.url"].(string)
	if !ok {
		return errors.New("update_asset_missing")
	}
	urls := []string{raw}
	for _, m := range p.manager.Mirrors() {
		urls = append(urls, strings.TrimRight(m, "/")+"/"+raw)
	}
	dest := filepath.Join(p.manager.DataDir(), "updates", "wails", r.Version, filepath.Base(r.Artifact.Filename))
	headers := http.Header{}
	if trace, ok := ctx.Value(downloadTraceKey{}).(string); ok {
		headers.Set("traceparent", trace)
	}
	e := download.Download(ctx, download.Request{Headers: headers, URLs: urls, Dest: dest, SHA256: hex.EncodeToString(r.Verification.Digest), MaxSize: r.Artifact.Size, OnProgress: func(v download.Progress) { progress(v.Done, v.Total) }})
	if e != nil {
		return e
	}
	f, e := os.Open(dest)
	if e != nil {
		return e
	}
	defer f.Close()
	_, e = io.Copy(dst, f)
	return e
}
