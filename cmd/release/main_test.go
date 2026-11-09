package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	archiveutil "github.com/shusfun/mistral-gui/internal/archive"
	"github.com/shusfun/mistral-gui/internal/update"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestGeneratedReleaseIsAcceptedByUpdater(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"mistral-gui-windows-x64-setup.exe", "mistral-gui-macos-x64.dmg", "mistral-gui-macos-arm64.dmg", "mistral-gui-windows-x64.update.zip", "mistral-gui-macos-x64.update.tar.zst", "mistral-gui-macos-arm64.update.tar.zst"} {
		if e := os.WriteFile(filepath.Join(dir, name), []byte("fixture "+name), 0600); e != nil {
			t.Fatal(e)
		}
	}
	pub, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	t.Setenv("UPDATE_SIGNING_KEY", base64.StdEncoding.EncodeToString(key))
	t.Setenv("UPDATE_PUBLIC_KEY", base64.StdEncoding.EncodeToString(pub))
	if e = manifest(dir, "v1.2.3", "owner/manager"); e != nil {
		t.Fatal(e)
	}
	s := httptest.NewTLSServer(http.FileServer(http.Dir(dir)))
	defer s.Close()
	m, e := update.FetchSigned(context.Background(), s.Client(), s.URL+"/release-manifest.json", base64.StdEncoding.EncodeToString(pub))
	if e != nil || len(m.Assets) != 3 {
		t.Fatalf("%+v %v", m, e)
	}
	for _, a := range m.Assets {
		if e = update.Verify(filepath.Join(dir, filepath.Base(a.URL)), a.SHA256); e != nil {
			t.Fatal(e)
		}
	}
	t.Setenv("UPDATE_PUBLIC_KEY", "wrong")
	if e = manifest(dir, "v1.2.3", "owner/manager"); e == nil {
		t.Fatal("mismatched key accepted")
	}
}
func TestWindowsUpdateArchiveLayout(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"mistral-gui.exe", "mistral-gui-updater.exe"} {
		_ = os.WriteFile(filepath.Join(dir, name), []byte(name), 0700)
	}
	if e := pack(dir, "windows", "amd64"); e != nil {
		t.Fatal(e)
	}
	dest := t.TempDir()
	if e := archiveutil.Extract(filepath.Join(dir, "mistral-gui-windows-x64.update.zip"), dest, 1000); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(dest, "mistral-gui-updater.exe")); e != nil {
		t.Fatal(e)
	}
}
