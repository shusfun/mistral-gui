package app

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNewCreatesTokenAndState(t *testing.T) {
	d := t.TempDir()
	a, err := New("127.0.0.1:0", filepath.Join(d, "data"))
	if err != nil {
		t.Fatal(err)
	}
	if a.token() == "" {
		t.Fatal("missing token")
	}
	if _, err := os.Stat(filepath.Join(d, "data", "state.json")); err != nil {
		t.Fatal(err)
	}
}

func TestHealth(t *testing.T) {
	a, err := New("127.0.0.1:0", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRecorder()
	a.health(r, httptest.NewRequest("GET", "/healthz", nil))
	if r.Code != 200 || r.Body.String() == "" {
		t.Fatalf("health=%d %s", r.Code, r.Body.String())
	}
}
