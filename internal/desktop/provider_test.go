package desktop

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shusfun/mistral-gui/internal/app"
	"github.com/wailsapp/wails/v3/pkg/updater"
	gh "github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

type updateTestHost struct{}

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
	for _, mode := range []string{"valid", "wrong-key", "missing-signature", "corrupt", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			var base string
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasPrefix(r.URL.Path, "/repos/"):
					fmt.Fprintf(w, `{"tag_name":"v0.2.0","assets":[{"name":"%s","url":"%s/update.zip","size":%d},{"name":"SHA256SUMS","url":"%s/SHA256SUMS"}]}`, updateName("windows", "amd64"), base, archive.Len(), base)
				case r.URL.Path == "/SHA256SUMS":
					fmt.Fprintf(w, "%x  %s\n", digest, updateName("windows", "amd64"))
				case r.URL.Path == "/update.zip.sig":
					if mode == "missing-signature" {
						w.WriteHeader(404)
						return
					}
					key := priv
					if mode == "wrong-key" {
						_, key, _ = ed25519.GenerateKey(rand.Reader)
					}
					fmt.Fprint(w, base64.StdEncoding.EncodeToString(ed25519.Sign(key, digest[:])))
				case r.URL.Path == "/update.zip":
					if mode == "corrupt" {
						_, _ = w.Write(bytes.Repeat([]byte("x"), archive.Len()))
						return
					}
					_, _ = w.Write(archive.Bytes())
				default:
					w.WriteHeader(404)
				}
			}))
			defer s.Close()
			base = s.URL
			official, e := gh.New(gh.Config{Repository: "owner/manager", BaseURL: s.URL, ChecksumAsset: "SHA256SUMS", AssetMatcher: func(updater.CheckRequest, []gh.ReleaseAsset) int { return 0 }})
			if e != nil {
				t.Fatal(e)
			}
			manager, e := app.New("127.0.0.1:0", t.TempDir())
			if e != nil {
				t.Fatal(e)
			}
			defer manager.Close()
			p := &provider{official: official, manager: manager}
			u := updater.New(updateTestHost{})
			if e = u.Init(updater.Config{CurrentVersion: "0.1.1", Providers: []updater.Provider{p}, PublicKey: pub, Window: updater.WindowNone}); e != nil {
				t.Fatal(e)
			}
			_, e = u.Check(context.Background())
			if mode == "missing-signature" {
				if e == nil {
					t.Fatal("missing signature accepted")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			ctx, cancel := context.WithCancel(context.Background())
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
