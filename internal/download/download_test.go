package download

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

func TestFileResumesAndVerifies(t *testing.T) {
	body := []byte("mistral-gui asset")
	sum := sha256.Sum256(body)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "bytes=5-" {
			t.Fatalf("range=%q", r.Header.Get("Range"))
		}
		w.Header().Set("Content-Range", "bytes 5-16/17")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(body[5:])
	}))
	defer s.Close()
	d := t.TempDir()
	dest := filepath.Join(d, "asset")
	if err := os.WriteFile(dest+".part", body[:5], 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Download(context.Background(), Request{HTTP: s.Client(), URLs: []string{s.URL}, Dest: dest, SHA256: hex.EncodeToString(sum[:])}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != string(body) {
		t.Fatalf("got=%q", got)
	}
}
