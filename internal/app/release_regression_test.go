package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReleaseTranslationLimitsAndSyncSnapshot(t *testing.T) {
	a, err := New("127.0.0.1:0", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	h := a.Handler()
	call := func(path string, payload any, auth bool) *httptest.ResponseRecorder {
		b, _ := json.Marshal(payload)
		r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:19820"+path, bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("traceparent", "00-11111111111111111111111111111111-2222222222222222-01")
		if auth {
			r.Header.Set("Authorization", "Bearer "+a.token())
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/api/v1/translate", "/api/v1/dictionary/sync"} {
		if w := call(path, map[string]any{}, false); w.Code != 401 {
			t.Fatalf("unauthorized %s: %d", path, w.Code)
		}
	}
	w := call("/api/v1/translate", map[string]any{"text": strings.Repeat("paragraph\n", 129)}, true)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "translation_input_limit") {
		t.Fatalf("silently truncated: %d %s", w.Code, w.Body.String())
	}
	rev := SyncedRevision{Source: "release smoke", Translation: "发布冒烟", SourceLang: "auto", TargetLang: "zh-CN", Time: 1}
	w = call("/api/v1/dictionary/sync", map[string]any{"revisions": []SyncedRevision{rev}, "revisionsSnapshot": true}, true)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = call("/api/v1/translate", map[string]any{"text": rev.Source, "routing": "local-only"}, true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), rev.Translation) {
		t.Fatalf("dictionary translate: %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("traceparent") != "00-11111111111111111111111111111111-2222222222222222-01" {
		t.Fatal("trace not propagated")
	}
	w = call("/api/v1/dictionary/sync", map[string]any{"revisions": []SyncedRevision{}, "revisionsSnapshot": true}, true)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if _, _, ok := a.ensureDictionaryStore().Lookup(rev.Source, "auto", "zh-CN"); ok {
		t.Fatal("deleted correction survived snapshot")
	}
}
