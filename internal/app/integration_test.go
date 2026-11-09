package app

import (
	"context"
	"encoding/json"
	"github.com/shusfun/mistral-gui/internal/catalog"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

var fixtureEngine string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mistral-engine-fixture-")
	if err != nil {
		panic(err)
	}
	fixtureEngine = filepath.Join(dir, "mistralrs")
	if runtime.GOOS == "windows" {
		fixtureEngine += ".exe"
	}
	c := exec.Command("go", "build", "-o", fixtureEngine, "./testdata/engine")
	if b, e := c.CombinedOutput(); e != nil {
		panic(string(b) + e.Error())
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
func fixtureApp(t *testing.T) *App {
	t.Helper()
	a, e := New("127.0.0.1:19820", t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(a.Close)
	model := filepath.Join(a.dataDir, "model.gguf")
	if e = os.WriteFile(model, []byte("fixture"), 0600); e != nil {
		t.Fatal(e)
	}
	a.config.Backend = "cpu"
	a.config.Engine = fixtureEngine
	a.config.Model = "fixture.gguf"
	a.installed = []Installed{{Asset: catalog.Asset{Kind: "engine", Backend: "cpu"}, Path: fixtureEngine}, {Asset: catalog.Asset{Kind: "model", ID: "fixture.gguf", Version: "rev", SHA256: strings.Repeat("a", 64)}, Path: model}}
	return a
}
func request(t *testing.T, a *App, method, path, body string, authenticated bool) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "http://127.0.0.1:19820"+path, strings.NewReader(body))
	if authenticated {
		r.Header.Set("Authorization", "Bearer "+a.token())
	}
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	return w
}
func TestModelRevisionSelectionRollbackAndDelete(t *testing.T) {
	a := fixtureApp(t)
	second := filepath.Join(a.dataDir, "model-v2.gguf")
	if err := os.WriteFile(second, []byte("fixture-v2"), 0600); err != nil {
		t.Fatal(err)
	}
	m := a.installed[1]
	m.Version, m.Path, m.SHA256 = "rev2", second, strings.Repeat("b", 64)
	a.installed = append(a.installed, m)
	if w := request(t, a, "POST", "/api/v1/settings", `{"model":"fixture.gguf","model_version":"rev"}`, true); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if a.config.PreviousModelVersion != "rev2" {
		t.Fatal("previous revision was not retained")
	}
	if err := a.ensureEngine(context.Background(), "revision-test"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(a.stateSnapshot().Revision, "rev:") {
		t.Fatal("wrong loaded revision")
	}
	if w := request(t, a, "POST", "/api/v1/settings", `{"model":"fixture.gguf","model_version":"rev2"}`, true); w.Code != 409 {
		t.Fatal("live revision switched")
	}
	a.stopEngine()
	if w := request(t, a, "POST", "/api/v1/models/rollback", `{}`, true); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if a.config.ModelVersion != "rev2" {
		t.Fatal("wrong rollback revision")
	}
	if w := request(t, a, "POST", "/api/v1/models/delete", `{"id":"fixture.gguf","version":"rev"}`, true); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if _, err := os.Stat(second); err != nil {
		t.Fatal("deleted another revision", err)
	}
	if _, ok := a.modelLocked("fixture.gguf", "rev"); ok {
		t.Fatal("deleted revision retained")
	}
}

func TestExplicitCPURecovery(t *testing.T) {
	a := fixtureApp(t)
	a.config.Backend = "cuda"
	a.state.State, a.state.Backend, a.state.Error = "failed", "cuda", "engine_exited"
	w := request(t, a, "POST", "/api/v1/runtime/fallback-cpu", `{}`, true)
	if w.Code != 202 {
		t.Fatal(w.Body.String())
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		s := a.stateSnapshot()
		if s.State == "running" {
			if s.Backend != "cpu" || !s.DegradedCPU {
				t.Fatal(s)
			}
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("CPU recovery did not start", a.stateSnapshot())
}
func TestAuthenticationPersistenceAndSettings(t *testing.T) {
	a := fixtureApp(t)
	for _, path := range []string{"/.well-known/appspecific/com.chrome.devtools.json", "/api/v1/unknown", "/robots.txt"} {
		if w := request(t, a, "GET", path, "", true); w.Code != 404 {
			t.Fatalf("unexpected fallback: %s %d", path, w.Code)
		}
	}
	if a.engine != nil {
		t.Fatal("unrelated browser resource started the engine")
	}
	for _, path := range []string{"/api/v1/status", "/api/v1/models", "/api/v1/token", "/v1/models", "/api/v1/events"} {
		if w := request(t, a, "GET", path, "", false); w.Code != 401 {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	for _, path := range []string{"/manage", "/api/v1/status", "/api/v1/diagnostics", "/healthz"} {
		w := request(t, a, "GET", path, "", true)
		if w.Code != 200 || strings.Contains(w.Body.String(), a.token()) {
			t.Fatalf("secret leak or bad response %s: %d", path, w.Code)
		}
	}
	r := httptest.NewRequest("POST", "http://127.0.0.1:19820/api/v1/runtime/start", strings.NewReader("{}"))
	r.Header.Set("Origin", "https://evil.example")
	r.Header.Set("Authorization", "Bearer "+a.token())
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
	r.Header.Del("Origin")
	r.Host = "evil.example"
	w = httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
	w = request(t, a, "POST", "/api/v1/settings", `{"backend":"cuda","model":"missing"}`, true)
	if w.Code != 400 || a.config.Backend != "cpu" {
		t.Fatal("invalid settings mutated config")
	}
	w = request(t, a, "POST", "/api/v1/settings", `{"language":"en-US","idle_minutes":12}`, true)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	b, e := New(a.addr, a.dataDir)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	if b.config.Language != "en-US" || b.config.IdleMinutes != 12 || b.token() != a.token() {
		t.Fatal("settings not persisted")
	}
	w = request(t, a, "POST", "/api/v1/token/rotate", `{}`, true)
	if w.Code != 200 || !strings.Contains(w.Header().Get("Set-Cookie"), "HttpOnly") {
		t.Fatal("session rotation")
	}
}
func TestConcurrentOnDemandStreamingAndIdle(t *testing.T) {
	a := fixtureApp(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := request(t, a, "POST", "/v1/chat/completions", `{"model":"fixture.gguf","messages":[],"stream":true}`, true)
			if w.Code != 200 || !strings.Contains(w.Body.String(), "[DONE]") {
				t.Errorf("proxy: %d %s", w.Code, w.Body.String())
			}
			if w.Header().Get("X-Fixture-Trace") != w.Header().Get("traceparent") || w.Header().Get("X-Mistral-Model-Revision") == "" {
				t.Error("propagation missing")
			}
		}()
	}
	wg.Wait()
	b, e := os.ReadFile(filepath.Join(a.dataDir, "hf-cache", "starts"))
	if e != nil || string(b) != "start\n" {
		t.Fatalf("multiple startups: %q %v", b, e)
	}
	if w := request(t, a, "GET", "/ui", "", true); w.Code != 200 || w.Body.String() != "upstream UI fixture" {
		t.Fatal("WebUI proxy")
	}
	a.mu.Lock()
	a.lastUse = time.Now().Add(-11 * time.Minute)
	a.state.Active = 1
	a.mu.Unlock()
	a.stop(true)
	if a.stateSnapshot().State != "running" {
		t.Fatal("stopped active request")
	}
	a.mu.Lock()
	a.state.Active = 0
	a.mu.Unlock()
	a.stop(true)
	if a.stateSnapshot().State != "stopped" {
		t.Fatal("idle stop failed")
	}
	a.config.AutoStart = false
	if w := request(t, a, "POST", "/v1/chat/completions", `{}`, true); w.Code != 503 {
		t.Fatal("disabled auto start")
	}
}
func TestCrashAndBackendMismatch(t *testing.T) {
	a := fixtureApp(t)
	a.config.Backend = "cuda"
	if e := a.ensureEngine(context.Background(), "fixture-trace"); e == nil {
		t.Fatal("CPU engine accepted as CUDA")
	}
	a.config.Backend = "cpu"
	_ = os.WriteFile(a.installed[1].Path, []byte("crash"), 0600)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if e := a.ensureEngine(ctx, "fixture-trace"); e == nil {
		t.Fatal("crash accepted")
	}
	deadline := time.Now().Add(time.Second)
	for !strings.Contains(a.stateSnapshot().Detail, "unsupported GGUF") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if a.stateSnapshot().State != "failed" || !strings.Contains(a.stateSnapshot().Detail, "unsupported GGUF") {
		t.Fatalf("missing startup reason: %+v", a.stateSnapshot())
	}
}
func TestEventsAndInterruptedTask(t *testing.T) {
	a := fixtureApp(t)
	a.operations = []Operation{{ID: "fixture", Kind: "install", State: "running"}}
	_ = a.save()
	b, e := New(a.addr, a.dataDir)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	if b.operations[0].State != "interrupted" {
		t.Fatal("lost task")
	}
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/v1/events", nil)
	req.Header.Set("Authorization", "Bearer "+a.token())
	resp, e := server.Client().Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer resp.Body.Close()
	data := make([]byte, 1024)
	n, e := resp.Body.Read(data)
	if e != nil && e != io.EOF {
		t.Fatal(e)
	}
	if !strings.Contains(string(data[:n]), "event: status\ndata: ") || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatal("bad SSE")
	}
	cancel()
	var status map[string]any
	w := request(t, a, "GET", "/api/v1/status", "", true)
	if json.Unmarshal(w.Body.Bytes(), &status) != nil || !validTraceparent(w.Header().Get("traceparent")) {
		t.Fatal("invalid status")
	}
}

func TestEngineFailureCapturesStdoutAndExitStatus(t *testing.T) {
	for _, test := range []struct{ model, want string }{
		{"stdout-crash", "CUDA initialization failed: fixture"},
		{"silent-crash", "exit status 5"},
	} {
		t.Run(test.model, func(t *testing.T) {
			a := fixtureApp(t)
			if err := os.WriteFile(a.installed[1].Path, []byte(test.model), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := a.ensureEngine(ctx, "failure-test"); err == nil {
				t.Fatal("failed engine accepted")
			}
			if s := a.stateSnapshot(); s.State != "failed" || !strings.Contains(s.Detail, test.want) {
				t.Fatalf("missing failure reason: %+v", s)
			}
		})
	}
}
