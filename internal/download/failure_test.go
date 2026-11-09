package download

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDigestFailureTriesNextSourceAndNeverActivatesCorruption(t *testing.T) {
	body := []byte("verified asset")
	sum := sha256.Sum256(body)
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == "/bad" {
			_, _ = w.Write([]byte("corrupt"))
		} else {
			_, _ = w.Write(body)
		}
	}))
	defer s.Close()
	dest := filepath.Join(t.TempDir(), "asset")
	e := Download(context.Background(), Request{HTTP: s.Client(), URLs: []string{s.URL + "/bad", s.URL + "/good"}, Dest: dest, SHA256: hex.EncodeToString(sum[:]), MaxSize: 100})
	if e != nil || calls != 2 {
		t.Fatalf("%v calls=%d", e, calls)
	}
	if b, _ := os.ReadFile(dest); string(b) != string(body) {
		t.Fatal("corrupt asset activated")
	}
}
func TestCancellationPreservesPartAndIdleIsBounded(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("partial"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer s.Close()
	sum := sha256.Sum256([]byte("never complete"))
	dest := filepath.Join(t.TempDir(), "asset")
	ctx, cancel := context.WithCancel(context.Background())
	cancelTimer := time.AfterFunc(150*time.Millisecond, cancel)
	defer cancelTimer.Stop()
	defer cancel()
	e := Download(ctx, Request{HTTP: s.Client(), URLs: []string{s.URL}, Dest: dest, SHA256: hex.EncodeToString(sum[:]), OnProgress: func(p Progress) {
		if p.Done > 0 {
			cancel()
		}
	}})
	if !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if info, e := os.Stat(dest + ".part"); e != nil || info.Size() != 7 {
		t.Fatal("partial not retained", e)
	}
	started := time.Now()
	e = Download(context.Background(), Request{HTTP: s.Client(), URLs: []string{s.URL}, Dest: filepath.Join(t.TempDir(), "idle"), SHA256: hex.EncodeToString(sum[:]), Idle: 100 * time.Millisecond})
	if e == nil || time.Since(started) > 3*time.Second {
		t.Fatal("idle timeout failed", e)
	}
}
func TestTransientRetryKeepsConfiguredTLSAndResumes(t *testing.T) {
	body := []byte("verified retry with private CA")
	sum := sha256.Sum256(body)
	calls := 0
	var protocols []int
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		protocols = append(protocols, r.ProtoMajor)
		if calls == 1 {
			w.Header().Set("Content-Length", "29")
			_, _ = w.Write(body[:7])
			return
		}
		if r.Header.Get("Range") != "bytes=7-" {
			t.Errorf("lost resume offset: %s", r.Header.Get("Range"))
		}
		w.Header().Set("Content-Range", "bytes 7-28/29")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(body[7:])
	}))
	s.EnableHTTP2 = true
	s.StartTLS()
	defer s.Close()
	dest := filepath.Join(t.TempDir(), "asset")
	err := Download(context.Background(), Request{HTTP: s.Client(), URLs: []string{s.URL}, Dest: dest, SHA256: hex.EncodeToString(sum[:]), MaxSize: int64(len(body))})
	if err != nil || calls != 2 {
		t.Fatalf("retry failed: %v calls=%d", err, calls)
	}
	if len(protocols) != 2 || protocols[0] != 2 || protocols[1] != 1 {
		t.Fatalf("retry did not reconnect via HTTP/1.1: %v", protocols)
	}
}
