package update

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSignedManifestAndTampering(t *testing.T) {
	pub, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	body, _ := json.Marshal(Manifest{Version: "v1.0.0", Assets: []Asset{{OS: "windows", Arch: "amd64", Kind: "update", URL: "https://github.com/owner/repo/releases/download/v1.0.0/m.update.zip", Size: 10, SHA256: strings.Repeat("a", 64)}}})
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(key, body))
	tamper := false
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".sig") {
			_, _ = w.Write([]byte(signature))
			return
		}
		if tamper {
			_, _ = w.Write(append(body, ' '))
		} else {
			_, _ = w.Write(body)
		}
	}))
	defer s.Close()
	m, e := FetchSigned(context.Background(), s.Client(), s.URL+"/release-manifest.json", base64.StdEncoding.EncodeToString(pub))
	if e != nil || m.Version != "v1.0.0" {
		t.Fatalf("%+v %v", m, e)
	}
	tamper = true
	if _, e = FetchSigned(context.Background(), s.Client(), s.URL+"/release-manifest.json", base64.StdEncoding.EncodeToString(pub)); e == nil || e.Error() != "update_signature_invalid" {
		t.Fatal("tampered manifest accepted", e)
	}
}
