package desktop

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/shusfun/mistral-gui/internal/app"
	"github.com/shusfun/mistral-gui/internal/update"
	"github.com/wailsapp/wails/v3/pkg/updater"
)

type updateTestHost struct{}
type rewriteRoundTripper struct {
	base *url.URL
	next http.RoundTripper
}

func (t rewriteRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	u := *r.URL
	u.Scheme = t.base.Scheme
	u.Host = t.base.Host
	r.URL = &u
	return t.next.RoundTrip(r)
}

func (updateTestHost) Emit(string, ...any) bool         { return false }
func (updateTestHost) OnEvent(string, func(any)) func() { return func() {} }
func (updateTestHost) OpenWindow(updater.WindowOptions) updater.WindowHandle {
	panic("HTML update window must remain disabled")
}
func (updateTestHost) Quit() {}

func TestResumableProviderWithWailsSignatureVerification(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	var archive bytes.Buffer
	z := zip.NewWriter(&archive)
	f, _ := z.Create("mistral-gui.exe")
	_, _ = f.Write([]byte("new executable"))
	_ = z.Close()
	digest := sha256.Sum256(archive.Bytes())
	for _, mode := range []string{"valid", "wrong-key", "missing-signature", "corrupt", "cancel", "old-manifest"} {
		t.Run(mode, func(t *testing.T) {
			assetURL := "https://github.com/owner/manager/releases/download/v0.2.0/" + updateName("windows", "amd64")
			m := update.Manifest{Version: "v0.2.0", Assets: []update.Asset{{OS: "windows", Arch: "amd64", Kind: "wails-update", URL: assetURL, SHA256: hex.EncodeToString(digest[:]), Size: int64(archive.Len())}}}
			if mode == "old-manifest" {
				m.Assets[0].Kind = "update"
			}
			manifest, _ := json.Marshal(m)
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/manifest":
					w.Write(manifest)
				case r.URL.Path == "/manifest.sig":
					fmt.Fprint(w, base64.StdEncoding.EncodeToString(ed25519.Sign(priv, manifest)))
				case strings.HasSuffix(r.URL.Path, ".zip.sig"):
					if mode == "missing-signature" {
						w.WriteHeader(404)
						return
					}
					key := priv
					if mode == "wrong-key" {
						_, key, _ = ed25519.GenerateKey(rand.Reader)
					}
					fmt.Fprint(w, base64.StdEncoding.EncodeToString(ed25519.Sign(key, digest[:])))
				case strings.HasSuffix(r.URL.Path, ".zip"):
					if mode == "corrupt" {
						_, _ = w.Write(bytes.Repeat([]byte("x"), archive.Len()))
						return
					}
					_, _ = w.Write(archive.Bytes())
				default:
					t.Error("unexpected metadata path", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer s.Close()
			base, _ := url.Parse(s.URL)
			client := s.Client()
			client.Transport = rewriteRoundTripper{base, client.Transport}
			manager, e := app.New("127.0.0.1:0", t.TempDir())
			if e != nil {
				t.Fatal(e)
			}
			defer manager.Close()
			encodedKey := base64.StdEncoding.EncodeToString(pub)
			p := &provider{manager: manager, client: client, repository: "owner/manager", publicKey: encodedKey, fetchManifest: func(ctx context.Context) (update.Manifest, error) {
				return update.FetchSignedSources(ctx, client, s.URL+"/manifest", encodedKey, nil)
			}}
			u := updater.New(updateTestHost{})
			if e = u.Init(updater.Config{CurrentVersion: "0.1.1", Providers: []updater.Provider{p}, PublicKey: pub, Window: updater.WindowNone}); e != nil {
				t.Fatal(e)
			}
			traceCtx := update.WithTrace(context.Background(), "00-33333333333333333333333333333333-4444444444444444-01", nil)
			_, e = u.Check(traceCtx)
			if mode == "missing-signature" || mode == "wrong-key" || mode == "old-manifest" {
				if e == nil {
					t.Fatal("missing signature accepted")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			ctx, cancel := context.WithCancel(traceCtx)
			defer cancel()
			if mode == "cancel" {
				cancel()
			}
			e = u.DownloadAndInstall(ctx)
			if mode == "valid" {
				if e != nil || u.State() != updater.StateReady {
					t.Fatal(e, u.State())
				}
			} else if e == nil {
				t.Fatal("unsafe update accepted", mode)
			}
		})
	}
}
