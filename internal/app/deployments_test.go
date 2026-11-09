package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestIndependentDeploymentsCoalesceRouteAndPersist(t *testing.T) {
	a := fixtureApp(t)
	for _, id := range []string{"translation", "chat"} {
		body := `{"id":"` + id + `","model":"fixture.gguf","backend":"cpu","auto_start":true,"idle_minutes":10,"tuning":{"context":2048,"sequences":1}}`
		if w := request(t, a, "POST", "/api/v1/deployments", body, true); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	var wg sync.WaitGroup
	for _, id := range []string{"translation", "chat"} {
		for range 8 {
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				child, e := a.resolveDeployment(id)
				if e != nil {
					t.Error(e)
					return
				}
				if e = child.ensureEngine(context.Background(), "00-11111111111111111111111111111111-2222222222222222-01"); e != nil {
					t.Error(e)
				}
			}(id)
		}
	}
	wg.Wait()
	b, e := os.ReadFile(filepath.Join(a.dataDir, "hf-cache", "starts"))
	if e != nil || strings.Count(string(b), "start\n") != 2 {
		t.Fatalf("expected one process per instance: %q %v", b, e)
	}
	for _, id := range []string{"translation", "chat"} {
		w := request(t, a, "POST", "/v1/chat/completions", `{"model":"`+id+`","messages":[{"role":"user","content":"Hi"}]}`, true)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "你好") {
			t.Fatal(w.Code, w.Body.String())
		}
		w = request(t, a, "GET", "/api/v1/status?model="+id, "", true)
		var status struct {
			Deployments []DeploymentView `json:"deployments"`
		}
		if json.Unmarshal(w.Body.Bytes(), &status) != nil || len(status.Deployments) != 1 || status.Deployments[0].ID != id {
			t.Fatal(w.Body.String())
		}
	}
	if w := request(t, a, "POST", "/api/v1/models/delete", `{"id":"fixture.gguf"}`, true); w.Code != 409 {
		t.Fatal("deleted active instance model")
	}
	first, _ := a.resolveDeployment("translation")
	second, _ := a.resolveDeployment("chat")
	first.stopEngine()
	if second.stateSnapshot().State != "running" {
		t.Fatal("stopping one instance affected another")
	}
	second.mu.Lock()
	second.lastUse = time.Now().Add(-11 * time.Minute)
	second.mu.Unlock()
	second.stop(true)
	if second.stateSnapshot().State != "stopped" {
		t.Fatal("idle instance did not stop")
	}
	other, e := New(a.addr, a.dataDir)
	if e != nil {
		t.Fatal(e)
	}
	defer other.Close()
	if len(other.children()) != 2 {
		t.Fatal("deployments not persisted")
	}
	if w := request(t, a, "POST", "/v1/chat/completions", `{"model":"missing","messages":[]}`, true); w.Code != 404 {
		t.Fatal("unknown model fell through to default")
	}
	if w := request(t, a, "POST", "/api/v1/deployments", `{"id":"bad","model":"fixture.gguf","backend":"cpu","tuning":{"context":-1}}`, true); w.Code != 400 {
		t.Fatal("invalid tuning accepted")
	}
	if w := request(t, a, "POST", "/api/v1/deployments", `{}`, false); w.Code != 401 {
		t.Fatal("unauthorized deployment accepted")
	}
}
