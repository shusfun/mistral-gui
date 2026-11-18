package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
)

func TestDictionarySyncPriorityAndDesktopTranslateRouting(t *testing.T) {
	dir := t.TempDir()
	a, err := New("127.0.0.1:0", dir)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	var cloudHits atomic.Int32
	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cloudHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"云端译文结果"}}]}`))
	}))
	defer cloudSrv.Close()

	a.mu.Lock()
	a.config.AutoStart = false
	a.config.CloudAPIURL = cloudSrv.URL + "/v1/chat/completions"
	a.config.CloudAPIKey = "sk-test-cloud"
	a.config.CloudModel = "Qwen/Qwen3-8B"
	a.mu.Unlock()

	h := a.Handler()
	doAuth := func(method, path string, payload any) *httptest.ResponseRecorder {
		var body []byte
		if payload != nil {
			body, _ = json.Marshal(payload)
		}
		req := httptest.NewRequest(method, "http://127.0.0.1:19820"+path, bytes.NewReader(body))
		req.Host = "127.0.0.1:19820"
		req.Header.Set("Authorization", "Bearer "+a.token())
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	// 1. Sync user revisions & dictionary packs
	syncResp := doAuth("POST", "/api/v1/dictionary/sync", map[string]any{
		"revisions": []map[string]any{
			{
				"source":      "Hello world",
				"translation": "人工修订问候",
				"sourceLang":  "en",
				"targetLang":  "zh-CN",
				"domain":      "*",
				"revision":    2,
				"time":        100,
			},
		},
		"packs": []map[string]any{
			{
				"id":         "glossary-core",
				"version":    1,
				"priority":   10,
				"sourceLang": "auto",
				"targetLang": "zh-CN",
				"domain":     "*",
				"entries": []map[string]string{
					{"source": "Hello world", "translation": "词库问候"},
					{"source": "Zero-copy buffer", "translation": "零拷贝缓冲区"},
				},
			},
		},
	})
	if syncResp.Code != 200 {
		t.Fatalf("dictionary sync failed: %d %s", syncResp.Code, syncResp.Body.String())
	}

	// 2. Verify revision beats dictionary, and local-only NEVER calls CloudAPIURL when local runtime is stopped!
	localOnlyResp := doAuth("POST", "/api/v1/translate", map[string]any{
		"text":        "Hello world\nZero-copy buffer\nUncached paragraph",
		"routing":     "local-only",
		"target_lang": "zh-CN",
	})
	if localOnlyResp.Code != 200 {
		t.Fatalf("expected partial 200 from revision/dict hits, got %d: %s", localOnlyResp.Code, localOnlyResp.Body.String())
	}
	if cloudHits.Load() != 0 {
		t.Fatalf("strict local-only routing contacted cloud server %d times!", cloudHits.Load())
	}

	var trOut TranslateResponse
	if err := json.Unmarshal(localOnlyResp.Body.Bytes(), &trOut); err != nil {
		t.Fatal(err)
	}
	if len(trOut.Segments) != 3 {
		t.Fatalf("expected 3 segments, got %d", len(trOut.Segments))
	}
	if trOut.Segments[0].Translation != "人工修订问候" || trOut.Segments[0].Channel != "revision" {
		t.Fatalf("segment 0 expected revision hit, got %+v", trOut.Segments[0])
	}
	if trOut.Segments[1].Translation != "零拷贝缓冲区" || trOut.Segments[1].Channel != "dictionary" {
		t.Fatalf("segment 1 expected dictionary hit, got %+v", trOut.Segments[1])
	}
	if trOut.Segments[2].Error != "runtime_stopped" {
		t.Fatalf("segment 2 expected runtime_stopped error in local-only, got %+v", trOut.Segments[2])
	}

	// 3. Verify local-first falls back to cloud cleanly for uncached segment and reports fallback_reason
	localFirstResp := doAuth("POST", "/api/v1/translate", map[string]any{
		"text":        "Uncached paragraph",
		"routing":     "local-first",
		"target_lang": "zh-CN",
	})
	if localFirstResp.Code != 200 {
		t.Fatalf("local-first fallback failed: %d %s", localFirstResp.Code, localFirstResp.Body.String())
	}
	if err := json.Unmarshal(localFirstResp.Body.Bytes(), &trOut); err != nil {
		t.Fatal(err)
	}
	if trOut.FallbackReason != "runtime_stopped" || trOut.Segments[0].Channel != "cloud" || trOut.Segments[0].Translation != "云端译文结果" {
		t.Fatalf("unexpected local-first response: %+v", trOut)
	}

	// 4. Clear synced dictionary
	clearResp := doAuth("POST", "/api/v1/dictionary/clear", map[string]any{})
	if clearResp.Code != 200 {
		t.Fatalf("dictionary clear failed: %d", clearResp.Code)
	}
	st := a.ensureDictionaryStore().Status()
	if st["revisions"].(int) != 0 || st["pack_entries"].(int) != 0 {
		t.Fatalf("expected empty dictionary after clear, got %+v", st)
	}
}

func TestOCRWorkerLifecycleAndPrivacyConfirmation(t *testing.T) {
	// Build tiny Go mock OCR HTTP worker executable to verify OCRSupervisor supervision & protocol
	workerSrc := filepath.Join(t.TempDir(), "mock_ocr.go")
	workerCode := `package main
import (
	"net/http"
	"os"
)
func main() {
	port := os.Getenv("MISTRAL_GUI_OCR_PORT")
	tok := os.Getenv("MISTRAL_GUI_OCR_TOKEN")
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("{\"ok\":true}"))
	})
	mux.HandleFunc("/ocr", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-OCR-Token") != tok {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"lines\":[{\"text\":\"Mock OCR Line\",\"score\":0.99}]}"))
	})
	_ = http.ListenAndServe("127.0.0.1:"+port, mux)
}
`
	if err := os.WriteFile(workerSrc, []byte(workerCode), 0600); err != nil {
		t.Fatal(err)
	}
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	workerBin := filepath.Join(t.TempDir(), "mock_ocr"+ext)
	buildCmd := exec.Command("go", "build", "-o", workerBin, workerSrc)
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build mock OCR worker: %v\n%s", err, string(out))
	}

	dir := t.TempDir()
	a, err := New("127.0.0.1:0", dir)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	sup := a.ensureOCRSupervisor()
	if err := os.MkdirAll(sup.dir, 0700); err != nil {
		t.Fatal(err)
	}
	sup.customExecPath = workerBin

	// Sync a dictionary entry for "Mock OCR Line" so translation completes locally
	_ = a.ensureDictionaryStore()
	a.dictionary.mu.Lock()
	a.dictionary.data.Revisions[dictRevKey("Mock OCR Line", "auto", "zh-CN")] = SyncedRevision{
		Source:      "Mock OCR Line",
		Translation: "模拟图片识别译文",
		SourceLang:  "auto",
		TargetLang:  "zh-CN",
	}
	a.dictionary.mu.Unlock()

	h := a.Handler()
	samplePNG := base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\nmock"))

	// 1. Without CloudImageConfirmed in local-first mode, returns 409 cloud_image_confirmation_required
	req1, _ := json.Marshal(map[string]any{
		"image_base64": samplePNG,
		"routing":      "local-first",
	})
	r1 := httptest.NewRequest("POST", "http://127.0.0.1:19820/api/v1/translate", bytes.NewReader(req1))
	r1.Host = "127.0.0.1:19820"
	r1.Header.Set("Authorization", "Bearer "+a.token())
	w1 := httptest.NewRecorder()
	h.ServeHTTP(w1, r1)
	if w1.Code != 409 {
		t.Fatalf("expected 409 cloud_image_confirmation_required, got %d: %s", w1.Code, w1.Body.String())
	}

	// 2. In local-only mode (or with confirm_cloud_image: true), OCR runs & translates cleanly!
	req2, _ := json.Marshal(map[string]any{
		"image_base64": samplePNG,
		"routing":      "local-only",
	})
	r2 := httptest.NewRequest("POST", "http://127.0.0.1:19820/api/v1/translate", bytes.NewReader(req2))
	r2.Host = "127.0.0.1:19820"
	r2.Header.Set("Authorization", "Bearer "+a.token())
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, r2)
	if w2.Code != 200 {
		t.Fatalf("expected 200 OCR translate, got %d: %s", w2.Code, w2.Body.String())
	}
	var out TranslateResponse
	if err := json.Unmarshal(w2.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.OCRText != "Mock OCR Line" || len(out.Segments) != 1 || out.Segments[0].Translation != "模拟图片识别译文" {
		t.Fatalf("unexpected OCR translation output: %+v", out)
	}

	// 3. Delete OCR stops worker and cleans directory
	sup.customExecPath = ""
	if err := sup.Delete(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sup.dir); !os.IsNotExist(err) {
		t.Fatalf("expected OCR directory removed, got err=%v", err)
	}
}

func TestEmbeddedManageWorkbenchAssetsServeCleanly(t *testing.T) {
	a, err := New("127.0.0.1:0", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	h := a.Handler()
	r := httptest.NewRequest("GET", "http://127.0.0.1:19820/manage", nil)
	r.Host = "127.0.0.1:19820"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("GET /manage returned %d", w.Code)
	}
	html := w.Body.String()
	entries, err := web.ReadDir("static/assets")
	if err != nil || len(entries) == 0 {
		t.Fatalf("expected embedded static/assets files, err=%v", err)
	}
	for _, e := range entries {
		assetPath := "/manage/assets/" + e.Name()
		if !bytes.Contains([]byte(html), []byte(assetPath)) {
			t.Fatalf("index.html did not reference %s:\n%s", assetPath, html)
		}
		ar := httptest.NewRequest("GET", "http://127.0.0.1:19820"+assetPath, nil)
		ar.Host = "127.0.0.1:19820"
		aw := httptest.NewRecorder()
		h.ServeHTTP(aw, ar)
		if aw.Code != 200 || aw.Body.Len() == 0 {
			t.Fatalf("GET %s returned %d (len=%d)", assetPath, aw.Code, aw.Body.Len())
		}
	}
}
