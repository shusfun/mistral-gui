package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFetchManifestAndVerify(t *testing.T) {
	body := []byte(`{"version":"1.2.3","assets":[{"os":"windows","arch":"amd64","kind":"update","url":"https://example/update.zip","sha256":"abc"}]}`)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(body) }))
	defer s.Close()
	m, err := FetchManifest(context.Background(), s.Client(), s.URL)
	if err != nil {
		t.Fatal(err)
	}
	if m.Version != "1.2.3" {
		t.Fatal(m.Version)
	}
	d := t.TempDir()
	p := filepath.Join(d, "x")
	b := []byte("x")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	if err := Verify(p, hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
}
