package desktop

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shusfun/mistral-gui/internal/app"
	"github.com/shusfun/mistral-gui/internal/download"
	"github.com/shusfun/mistral-gui/internal/update"
	"github.com/wailsapp/wails/v3/pkg/updater"
)

type provider struct {
	manager       *app.App
	fetchManifest func(context.Context) (update.Manifest, error)
	client        *http.Client
	repository    string
	publicKey     string
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
	return &provider{manager: a, fetchManifest: a.UpdateManifest, repository: app.UpdateRepository, publicKey: app.UpdatePublicKey}, nil
}
func (p *provider) Name() string { return "signed-manifest-resumable" }
func (p *provider) Check(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	m, err := p.fetchManifest(ctx)
	if err != nil {
		return nil, err
	}
	if !update.IsNewer(req.CurrentVersion, m.Version) {
		return nil, nil
	}
	if m.MinVersion != "" && update.IsNewer(req.CurrentVersion, m.MinVersion) {
		return nil, errors.New("update_requires_installer")
	}
	name := updateName(req.Platform, req.Arch)
	var asset *update.Asset
	for i := range m.Assets {
		a := &m.Assets[i]
		if a.OS == req.Platform && a.Arch == req.Arch && a.Kind == "wails-update" {
			if asset != nil {
				return nil, errors.New("update_asset_invalid")
			}
			asset = a
		}
	}
	if asset == nil {
		return nil, errors.New("update_asset_missing")
	}
	expected := "https://github.com/" + p.repository + "/releases/download/" + url.PathEscape(m.Version) + "/" + name
	if asset.URL != expected || asset.Size <= 0 {
		return nil, errors.New("update_asset_invalid")
	}
	digest, err := hex.DecodeString(asset.SHA256)
	if err != nil || len(digest) != 32 {
		return nil, errors.New("update_digest_missing")
	}
	pub, err := base64.StdEncoding.DecodeString(p.publicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return nil, errors.New("update_key_unconfigured")
	}
	var signature []byte
	_, err = update.FetchVerifiedSources(ctx, p.client, expected+".sig", p.manager.Mirrors(), 4096, func(b []byte) error {
		var err error
		signature, err = base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
		if err != nil || !ed25519.Verify(pub, digest, signature) {
			return errors.New("update_signature_invalid")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	filetype := "tar.gz"
	if req.Platform == "windows" {
		filetype = "zip"
	}
	return &updater.Release{Version: strings.TrimPrefix(m.Version, "v"), Notes: m.Notes,
		Artifact:     updater.Artifact{Filename: name, Filetype: filetype, Platform: req.Platform, Arch: req.Arch, Size: asset.Size},
		Verification: &updater.Verification{DigestAlgo: "sha256", Digest: digest, SignatureAlgo: "ed25519", Signature: signature},
		Metadata:     map[string]any{"artifact.url": expected}}, nil
}
func (p *provider) Download(ctx context.Context, r *updater.Release, dst io.Writer, progress func(int64, int64)) error {
	raw, ok := r.Metadata["artifact.url"].(string)
	if !ok {
		return errors.New("update_asset_missing")
	}
	dest := filepath.Join(p.manager.DataDir(), "updates", "wails", r.Version, filepath.Base(r.Artifact.Filename))
	headers := http.Header{}
	if trace := update.Trace(ctx); trace != "" {
		headers.Set("traceparent", trace)
	}
	source := ""
	err := download.Download(ctx, download.Request{KeepSourceOrder: true, OnSourceResult: func(host string, elapsed time.Duration, err error) {
		cause := ""
		if err != nil {
			cause = "download_failed"
			if errors.Is(err, download.ErrDigest) {
				cause = "digest_mismatch"
			}
			if ctx.Err() != nil {
				cause = "cancelled"
			}
		}
		slog.Info("update.download_source_result", "traceparent", update.Trace(ctx), "source", host, "durationMs", elapsed.Milliseconds(), "cause", cause)
	}, HTTP: p.client, Headers: headers, URLs: update.URLs(raw, p.manager.Mirrors()), Dest: dest, SHA256: hex.EncodeToString(r.Verification.Digest), MaxSize: r.Artifact.Size,
		OnStep: func(step string) {
			phase := "downloading"
			for _, prefix := range []string{"连接 ", "改用 "} {
				if strings.HasPrefix(step, prefix) {
					source = strings.TrimPrefix(step, prefix)
				}
			}
			if strings.Contains(step, "改用") {
				phase = "switching_source"
			}
			if strings.Contains(step, "校验") {
				phase = "verifying"
			}
			if step == "retrying" {
				phase = step
			}
			update.Phase(ctx, phase, source)
		},
		OnProgress: func(v download.Progress) { progress(v.Done, v.Total) }})
	if err != nil {
		return err
	}
	update.Phase(ctx, "verifying", "")
	f, err := os.Open(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(dst, f)
	return err
}
