package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestThemePreservesRunningEngineAndExistingData(t *testing.T) {
	a := fixtureApp(t)
	token, model, engine := a.token(), a.config.Model, a.config.Engine
	if err := a.ensureEngine(context.Background(), "00-11111111111111111111111111111111-2222222222222222-01"); err != nil {
		t.Fatal(err)
	}
	for _, theme := range []string{"dark", "light", "system"} {
		w := request(t, a, "POST", "/api/v1/settings", `{"theme":"`+theme+`"}`, true)
		if w.Code != 200 || a.config.Theme != theme || a.stateSnapshot().State != "running" {
			t.Fatalf("theme update: %d %s", w.Code, w.Body.String())
		}
	}
	if a.token() != token || a.config.Model != model || a.config.Engine != engine {
		t.Fatal("appearance changed credentials or model selection")
	}
	for _, tc := range []struct {
		body       string
		authorized bool
		status     int
	}{{`{"theme":"invalid"}`, true, 400}, {`{"theme":"dark"}`, false, 401}} {
		if w := request(t, a, "POST", "/api/v1/settings", tc.body, tc.authorized); w.Code != tc.status {
			t.Fatalf("expected %d, got %d", tc.status, w.Code)
		}
	}
	// Simulate the previous persisted schema without a theme.
	path := filepath.Join(a.dataDir, "state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]any
	if err = json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	delete(state["config"].(map[string]any), "theme")
	data, _ = json.Marshal(state)
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	restored, err := New(a.addr, a.dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if restored.config.Theme != "system" || restored.token() != token || restored.config.Model != model || len(restored.installed) != len(a.installed) {
		t.Fatal("legacy state was not preserved")
	}
}

func TestUIEventsRequireAuthValidateAndPreserveHTTPTrace(t *testing.T) {
	a := fixtureApp(t)
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	trace := "00-11111111111111111111111111111111-2222222222222222-01"
	event := `{"traceparent":"` + trace + `","phase":"ui.end","outcome":"success","duration_ms":7}`
	for _, tc := range []struct {
		body       string
		authorized bool
		status     int
	}{
		{`{"events":[` + event + `]}`, true, 200},
		{`{"events":[` + event + `]}`, false, 401},
		{`{"events":[` + strings.Replace(event, "ui.end", "raw.secret", 1) + `]}`, true, 400},
		{`{"events":[` + strings.Replace(event, trace, "invalid", 1) + `]}`, true, 400},
		{`{"events":[],"token":"must-not-log"}`, true, 400},
		{`{"events":[` + strings.TrimSuffix(strings.Repeat(event+",", 101), ",") + `]}`, true, 400},
	} {
		req, err := http.NewRequest("POST", server.URL+"/api/v1/ui-events", bytes.NewBufferString(tc.body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("traceparent", trace)
		if tc.authorized {
			req.Header.Set("Authorization", "Bearer "+a.token())
		}
		response, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode != tc.status || response.Header.Get("traceparent") != trace {
			t.Fatalf("status=%d trace=%s", response.StatusCode, response.Header.Get("traceparent"))
		}
	}
}

func TestOperationsIncludeIndependentDeploymentTasks(t *testing.T) {
	a := fixtureApp(t)
	child := a.newDeployment(Deployment{ID: "translation", Model: a.config.Model, Backend: "cpu"})
	a.deployments["translation"] = child
	child.operations = []Operation{{ID: "child-task", Kind: "start", State: "success", TraceID: "00-11111111111111111111111111111111-2222222222222222-01"}}
	ops := a.operationSnapshot()
	if len(ops) != 1 || ops[0].DeploymentID != "translation" {
		t.Fatal("deployment operation missing from manager tasks")
	}
	if op, ok := a.CompletedOperation("child-task"); !ok || op.ID != "child-task" {
		t.Fatal("notification cannot resolve deployment task")
	}
}

func TestTaskCancellationReachesIndependentDeployment(t *testing.T) {
	a := fixtureApp(t)
	child := a.newDeployment(Deployment{ID: "translation", Model: a.config.Model, Backend: "cpu"})
	a.deployments["translation"] = child
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	child.cancels["child-task"] = cancel
	if w := request(t, a, "POST", "/api/v1/tasks/cancel", `{"id":"child-task"}`, false); w.Code != 401 {
		t.Fatal("anonymous cancellation accepted")
	}
	if ctx.Err() != nil {
		t.Fatal("unauthorized cancellation reached task")
	}
	if w := request(t, a, "POST", "/api/v1/tasks/cancel", `{"id":"child-task"}`, true); w.Code != 200 || !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatal("child cancellation failed")
	}
	if ctx.Err() == nil {
		t.Fatal("child task was not cancelled")
	}
}
