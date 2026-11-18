package update

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type metadataTransport func(*http.Request) (*http.Response, error)

func (f metadataTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSignedSourcesFallbackAndRejection(t *testing.T) {
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	manifest, _ := json.Marshal(Manifest{Version: "v0.3.1", ExpiresAt: time.Now().Add(time.Hour).Format(time.RFC3339)})
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(key, manifest))
	for _, mode := range []string{"eof", "timeout", "503", "signature", "expired", "all-failed", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			calls := map[string]int{}
			trace := "00-11111111111111111111111111111111-2222222222222222-01"
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("traceparent") != trace {
					t.Error("trace missing")
				}
				if strings.HasSuffix(r.URL.Path, ".sig") {
					io.WriteString(w, sig)
				} else {
					w.Write(manifest)
				}
			}))
			defer server.Close()
			base := server.Client().Transport
			client := &http.Client{Transport: metadataTransport(func(r *http.Request) (*http.Response, error) {
				calls[r.URL.Host]++
				if r.URL.Host == "official.test" || mode == "all-failed" {
					switch mode {
					case "timeout":
						return nil, context.DeadlineExceeded
					case "503":
						return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
					case "signature":
						return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}")), Header: http.Header{}}, nil
					case "expired":
						old, _ := json.Marshal(Manifest{Version: "v0.3.1", ExpiresAt: time.Now().Add(-time.Hour).Format(time.RFC3339)})
						body := string(old)
						if strings.HasSuffix(r.URL.Path, ".sig") {
							body = base64.StdEncoding.EncodeToString(ed25519.Sign(key, old))
						}
						return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
					default:
						return nil, io.EOF
					}
				}
				return base.RoundTrip(r)
			})}
			ctx, cancel := context.WithCancel(WithTrace(context.Background(), trace, nil))
			defer cancel()
			if mode == "cancel" {
				cancel()
			}
			m, err := FetchSignedSources(ctx, client, "https://official.test/manifest", base64.StdEncoding.EncodeToString(pub), []string{server.URL})
			if mode == "all-failed" || mode == "cancel" {
				if err == nil {
					t.Fatal("failure accepted")
				}
				if mode == "cancel" && len(calls) != 0 {
					t.Fatal("cancel made requests")
				}
				return
			}
			if err != nil || m.Version != "v0.3.1" {
				t.Fatal(m, err)
			}
			if calls["official.test"] != 2 {
				t.Fatal("expected two requests (retry or manifest/signature)", calls)
			}
		})
	}
}

func TestVerifiedSourcesRejectInvalidSignature(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: metadataTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("bad")), Header: http.Header{}}, nil
	})}
	_, err := FetchVerifiedSources(context.Background(), client, "https://official.test/a.sig", []string{"https://mirror.test"}, 4096, func([]byte) error { return errors.New("update_signature_invalid") })
	if err == nil || calls != 2 {
		t.Fatal(err, calls)
	}
}
