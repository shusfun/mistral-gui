package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func prepareFixture(t *testing.T) *App {
	a := fixtureApp(t)
	sum := sha256.Sum256([]byte("fixture"))
	a.installed[1].SHA256 = hex.EncodeToString(sum[:])
	return a
}
func waitPrepare(t *testing.T, a *App, id string) Operation {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		for _, op := range a.operationSnapshot() {
			if op.ID == id && op.State != "running" {
				return op
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("deployment task timed out")
	return Operation{}
}
func submitPrepare(t *testing.T, a *App, body string) Operation {
	t.Helper()
	w := request(t, a, "POST", "/api/v1/deployments/prepare", body, true)
	if w.Code != 202 {
		t.Fatalf("prepare: %d %s", w.Code, w.Body.String())
	}
	var op Operation
	if err := json.Unmarshal(w.Body.Bytes(), &op); err != nil {
		t.Fatal(err)
	}
	return op
}
func TestPrepareDeploymentStartsRoutesAndPreservesOtherInstances(t *testing.T) {
	a := prepareFixture(t)
	if err := a.ensureEngine(context.Background(), "00-11111111111111111111111111111111-2222222222222222-01"); err != nil {
		t.Fatal(err)
	}
	op := submitPrepare(t, a, `{"id":"translation","model":"fixture.gguf","backend":"cpu","auto_start":true,"idle_minutes":10,"tuning":{"context":2048,"sequences":1}}`)
	finished := waitPrepare(t, a, op.ID)
	if finished.State != "success" || finished.Step != "ready" || finished.TraceID != op.TraceID || finished.DeploymentID != "translation" {
		t.Fatalf("task: %+v", finished)
	}
	w := request(t, a, "POST", "/v1/chat/completions", `{"model":"translation","messages":[{"role":"user","content":"Hello"}]}`, true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "你好") {
		t.Fatal(w.Code, w.Body.String())
	}
	if a.config.Model != "fixture.gguf" || a.stateSnapshot().State != "running" {
		t.Fatal("default instance changed")
	}
	child, _ := a.resolveDeployment("translation")
	child.stopEngine()
	if a.stateSnapshot().State != "running" {
		t.Fatal("another instance stopped")
	}
	other, err := New(a.addr, a.dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if len(other.children()) != 1 || other.config.Deployments[0].Version != "rev" {
		t.Fatal("revision not persisted")
	}
}
func TestPrepareQueueDeduplicatesCancelsAndRetries(t *testing.T) {
	a := prepareFixture(t)
	if err := a.acquireResources(context.Background()); err != nil {
		t.Fatal(err)
	}
	body := `{"id":"queued","model":"fixture.gguf","backend":"cpu"}`
	first := submitPrepare(t, a, body)
	second := submitPrepare(t, a, body)
	if first.ID != second.ID {
		t.Fatal("duplicate submission created another task")
	}
	a.changeOp(first.ID, func(op *Operation) { op.Prepare.Backend = "cuda" })
	third := submitPrepare(t, a, body)
	if first.ID != third.ID {
		t.Fatal("automatic recommendation changed duplicate request identity")
	}
	if w := request(t, a, "POST", "/api/v1/deployments/delete", `{"id":"queued"}`, true); w.Code != 409 {
		t.Fatal("deleted pending deployment")
	}
	a.cancelOperation(first.ID)
	if op := waitPrepare(t, a, first.ID); op.State != "cancelled" {
		t.Fatal(op)
	}
	a.releaseResources()
	retry := submitPrepare(t, a, body)
	if op := waitPrepare(t, a, retry.ID); op.State != "success" {
		t.Fatal(op)
	}
}
func TestPrepareRejectsUnauthorizedInvalidAndCorruptModels(t *testing.T) {
	a := prepareFixture(t)
	for _, tc := range []struct {
		body       string
		authorized bool
		status     int
	}{
		{`{"id":"test","model":"fixture.gguf"}`, false, 401},
		{`{"id":"../test","model":"fixture.gguf"}`, true, 400},
		{`{"id":"test","model":"fixture.gguf","tuning":{"context":-1}}`, true, 400},
	} {
		if w := request(t, a, "POST", "/api/v1/deployments/prepare", tc.body, tc.authorized); w.Code != tc.status {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if err := os.WriteFile(a.installed[1].Path, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	op := submitPrepare(t, a, `{"id":"bad","model":"fixture.gguf","backend":"cpu"}`)
	if result := waitPrepare(t, a, op.ID); result.State != "failed" || result.Error != "model_file_invalid" {
		t.Fatal(result)
	}
	child, _ := a.resolveDeployment("bad")
	if child.engine != nil {
		t.Fatal("started corrupt model")
	}
}
