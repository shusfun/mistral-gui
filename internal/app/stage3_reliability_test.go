package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/shusfun/mistral-gui/internal/catalog"
)

func TestRuntimeAsyncLoadingCooldownLogsAndFailureSeparation(t *testing.T) {
	a := fixtureApp(t)

	// 1. M-R1 & M-R6: slow-load engine returns 503 runtime_loading on X-Mistral-Async-Ready: 1, then becomes ready
	if err := os.WriteFile(a.installed[1].Path, []byte("slow-load"), 0600); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "http://127.0.0.1:19820/v1/chat/completions", strings.NewReader(`{"model":"fixture.gguf","messages":[]}`))
	req.Header.Set("Authorization", "Bearer "+a.token())
	req.Header.Set("X-Mistral-Async-Ready", "1")
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, req)
	if w.Code != 503 || !strings.Contains(w.Body.String(), `"runtime_loading"`) {
		t.Fatalf("expected 503 runtime_loading, got %d %s", w.Code, w.Body.String())
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if a.stateSnapshot().State == "running" {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if a.stateSnapshot().State != "running" {
		t.Fatalf("slow-load engine never reached running: %+v", a.stateSnapshot())
	}
	a.stopEngine()

	// 2. M-R3 & M-R5: post-ready crash retains runtime stderr + exit code, and stopEngine() preserves LastFailure
	if err := os.WriteFile(a.installed[1].Path, []byte("crash-after-ready"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.ensureEngine(context.Background(), "00-11111111111111111111111111111111-2222222222222222-01"); err != nil {
		t.Fatal(err)
	}
	// Inject a secret token string into runtime log and verify redaction
	a.runtimeLog.Append("debug token="+a.token()+"\n", a.token())

	_ = request(t, a, "POST", "/v1/chat/completions", `{"model":"fixture.gguf","messages":[]}`, true)
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if a.stateSnapshot().LastFailure != nil && a.stateSnapshot().LastFailure.ExitCode == 7 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	snap := a.stateSnapshot()
	if snap.LastFailure == nil || snap.LastFailure.ExitCode != 7 || !strings.Contains(snap.LastFailure.LogTail, "CUDA runtime OOM during decode") {
		t.Fatalf("post-ready crash details missing: %+v", snap.LastFailure)
	}
	logResp := request(t, a, "GET", "/api/v1/deployments/logs", "", true)
	if logResp.Code != 200 || strings.Contains(logResp.Body.String(), a.token()) || !strings.Contains(logResp.Body.String(), "CUDA runtime OOM during decode") {
		t.Fatalf("unexpected deployment logs response: %d %s", logResp.Code, logResp.Body.String())
	}

	// M-R5: Stopping after crash sets State="stopped" while preserving LastFailure!
	a.stopEngine()
	afterStop := a.stateSnapshot()
	if afterStop.State != "stopped" || afterStop.LastFailure == nil || afterStop.LastFailure.ExitCode != 7 {
		t.Fatalf("stopEngine wiped LastFailure: %+v", afterStop)
	}

	// 3. M-R2: second consecutive crash triggers runtime_cooldown and blocks repeat startup until manual reset
	if err := os.WriteFile(a.installed[1].Path, []byte("crash"), 0600); err != nil {
		t.Fatal(err)
	}
	_ = a.ensureEngine(context.Background(), "00-11111111111111111111111111111111-2222222222222222-01")
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if a.stateSnapshot().CooldownUntil != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if a.stateSnapshot().CooldownUntil == nil {
		t.Fatalf("expected cooldown after 2 consecutive failures: %+v", a.stateSnapshot())
	}
	cdResp := request(t, a, "POST", "/v1/chat/completions", `{"model":"fixture.gguf","messages":[]}`, true)
	if cdResp.Code != 503 || !strings.Contains(cdResp.Body.String(), `"runtime_cooldown"`) {
		t.Fatalf("expected 503 runtime_cooldown, got %d %s", cdResp.Code, cdResp.Body.String())
	}

	// Restore valid model and verify manual /api/v1/runtime/start resets cooldown
	if err := os.WriteFile(a.installed[1].Path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	startResp := request(t, a, "POST", "/api/v1/runtime/start", `{}`, true)
	if startResp.Code != 202 {
		t.Fatalf("manual start failed: %d %s", startResp.Code, startResp.Body.String())
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if a.stateSnapshot().State == "running" {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if a.stateSnapshot().State != "running" {
		t.Fatalf("manual start did not clear cooldown and reach running: %+v", a.stateSnapshot())
	}
}

func TestMultiDeploymentEvictionAndNamedFallbackCPU(t *testing.T) {
	a := fixtureApp(t)
	a.config.MaxRunningDeployments = 1

	for _, id := range []string{"dep-a", "dep-b"} {
		body := `{"id":"` + id + `","model":"fixture.gguf","backend":"cpu","auto_start":true,"idle_minutes":10,"tuning":{"context":2048,"sequences":1}}`
		if w := request(t, a, "POST", "/api/v1/deployments", body, true); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	depA, _ := a.resolveDeployment("dep-a")
	depB, _ := a.resolveDeployment("dep-b")
	if err := depA.ensureEngine(context.Background(), "00-11111111111111111111111111111111-2222222222222222-01"); err != nil {
		t.Fatal(err)
	}
	if err := depB.ensureEngine(context.Background(), "00-11111111111111111111111111111111-2222222222222222-01"); err != nil {
		t.Fatal(err)
	}
	if depA.stateSnapshot().State != "stopped" || depB.stateSnapshot().State != "running" {
		t.Fatalf("expected dep-a evicted for dep-b: depA=%s depB=%s", depA.stateSnapshot().State, depB.stateSnapshot().State)
	}

	// M-R4: fallback-cpu works on named deployment dep-a too
	depA.mu.Lock()
	depA.config.Backend = "cuda"
	depA.mu.Unlock()
	w := request(t, a, "POST", "/api/v1/runtime/fallback-cpu", `{"id":"dep-a"}`, true)
	if w.Code != 202 {
		t.Fatalf("named fallback-cpu failed: %d %s", w.Code, w.Body.String())
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s := depA.stateSnapshot()
		if s.State == "running" && s.Backend == "cpu" && s.DegradedCPU {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("named deployment fallback-cpu did not recover: %+v", depA.stateSnapshot())
}

func TestSecurityBackupLockExchangeAndHealthHardening(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	a, err := New("127.0.0.1:19820", dir)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	// M-S20: on Windows, token is DPAPI-encrypted on disk, never stored in plaintext
	rawState, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" && strings.Contains(string(rawState), a.token()) {
		t.Fatal("plaintext manager token found in state.json on Windows")
	}

	// Trigger a second save so state.json.bak is populated, then corrupt state.json and verify M-S01 backup recovery
	_ = a.SetLanguage("zh-CN")
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte("{truncated"), 0600); err != nil {
		t.Fatal(err)
	}
	recovered, err := New("127.0.0.1:19820", dir)
	if err != nil {
		t.Fatalf("expected automatic .bak recovery, got: %v", err)
	}
	defer recovered.Close()
	if !recovered.recoveredFromBackup || recovered.token() != a.token() {
		t.Fatal("backup recovery flag or token mismatch")
	}

	// M-S10: Exclusive data-dir lock blocks concurrent acquisition
	lockDir := filepath.Join(t.TempDir(), "locked")
	l1, err := AcquireDataDirLock(lockDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireDataDirLock(lockDir); err == nil {
		l1.Release()
		t.Fatal("expected second AcquireDataDirLock to fail")
	}
	l1.Release()
	l2, err := AcquireDataDirLock(lockDir)
	if err != nil {
		t.Fatalf("expected lock re-acquisition after release, got %v", err)
	}
	l2.Release()

	// M-S12: ConnectExisting never leaks Bearer token to an imposter server
	leakedAuth := ""
	imposter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "" {
			leakedAuth = auth
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ok":true,"service":"other-app"}`))
	}))
	defer imposter.Close()
	imposterAddr := strings.TrimPrefix(imposter.URL, "http://")
	if err := ConnectExisting(imposterAddr, dir, false); err == nil {
		t.Fatal("ConnectExisting accepted imposter service")
	}
	if leakedAuth != "" {
		t.Fatalf("ConnectExisting leaked token to imposter: %s", leakedAuth)
	}

	// M-S02: Single-use exchange code sets cookie once and rejects replay
	code := a.issueExchangeCode()
	exReq := httptest.NewRequest("POST", "http://127.0.0.1:19820/api/v1/session/exchange", strings.NewReader(`{"code":"`+code+`"}`))
	exW := httptest.NewRecorder()
	a.Handler().ServeHTTP(exW, exReq)
	if exW.Code != 200 || !strings.Contains(exW.Header().Get("Set-Cookie"), "mistral_gui_session=") {
		t.Fatalf("exchange failed: %d %s", exW.Code, exW.Body.String())
	}
	replayW := httptest.NewRecorder()
	a.Handler().ServeHTTP(replayW, httptest.NewRequest("POST", "http://127.0.0.1:19820/api/v1/session/exchange", strings.NewReader(`{"code":"`+code+`"}`)))
	if replayW.Code != 401 {
		t.Fatalf("replayed exchange code accepted: %d", replayW.Code)
	}

	// M-S13: Cookie-authenticated POST without Origin / Sec-Fetch-Site / X-Mistral-UI is rejected
	csrfReq := httptest.NewRequest("POST", "http://127.0.0.1:19820/api/v1/settings", strings.NewReader(`{"theme":"dark"}`))
	csrfReq.AddCookie(&http.Cookie{Name: "mistral_gui_session", Value: a.token()})
	csrfW := httptest.NewRecorder()
	a.Handler().ServeHTTP(csrfW, csrfReq)
	if csrfW.Code != 401 {
		t.Fatalf("expected 401 on cross-site cookie POST, got %d", csrfW.Code)
	}
	csrfReq2 := httptest.NewRequest("POST", "http://127.0.0.1:19820/api/v1/settings", strings.NewReader(`{"theme":"dark"}`))
	csrfReq2.AddCookie(&http.Cookie{Name: "mistral_gui_session", Value: a.token()})
	csrfReq2.Header.Set("X-Mistral-UI", "1")
	csrfW2 := httptest.NewRecorder()
	a.Handler().ServeHTTP(csrfW2, csrfReq2)
	if csrfW2.Code != 200 {
		t.Fatalf("expected 200 with X-Mistral-UI header, got %d", csrfW2.Code)
	}

	// M-S14: CORS allows official Qingyi extension ID and rejects unknown chrome-extension IDs
	corsGood := httptest.NewRequest("OPTIONS", "http://127.0.0.1:19820/api/v1/status", nil)
	corsGood.Header.Set("Origin", "chrome-extension://epficbaecbjclndbkkkdoliihpnbdejm")
	corsGoodW := httptest.NewRecorder()
	a.Handler().ServeHTTP(corsGoodW, corsGood)
	if corsGoodW.Code != 204 {
		t.Fatalf("expected 204 for Qingyi extension origin, got %d", corsGoodW.Code)
	}
	corsBad := httptest.NewRequest("OPTIONS", "http://127.0.0.1:19820/api/v1/status", nil)
	corsBad.Header.Set("Origin", "chrome-extension://aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	corsBadW := httptest.NewRecorder()
	a.Handler().ServeHTTP(corsBadW, corsBad)
	if corsBadW.Code != 403 {
		t.Fatalf("expected 403 for unknown extension origin, got %d", corsBadW.Code)
	}

	// M-S15: unauthenticated /healthz omits pid and update_transaction
	hUnauth := request(t, a, "GET", "/healthz", "", false)
	var hMap map[string]any
	_ = json.Unmarshal(hUnauth.Body.Bytes(), &hMap)
	if _, hasPID := hMap["pid"]; hasPID {
		t.Fatalf("unauthenticated /healthz exposed pid: %v", hMap)
	}

	// M-S24 & M-S25: Windows reserved names rejected & DesktopHandler rejects network-tagged requests
	for _, unsafePath := range []string{"CON", "NUL.gguf", "sub/COM1.json", "LPT1/weights.safetensors"} {
		if catalog.SafeModelPath(unsafePath) {
			t.Fatalf("SafeModelPath accepted Windows reserved device path: %s", unsafePath)
		}
	}
	netReq := httptest.NewRequest("GET", "http://127.0.0.1:19820/api/v1/token", nil)
	netReq = netReq.WithContext(markNetworkConnContext(netReq.Context()))
	netW := httptest.NewRecorder()
	a.DesktopHandler().ServeHTTP(netW, netReq)
	if netW.Code != 403 {
		t.Fatalf("DesktopHandler accepted network request: %d", netW.Code)
	}
}
