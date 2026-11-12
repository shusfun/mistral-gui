package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"sort"
	"time"
)

type Tuning struct {
	Context     int     `json:"context"`
	Sequences   int     `json:"sequences"`
	GPU         int     `json:"gpu_layers"`
	ISQ         string  `json:"isq,omitempty"`
	GPUUtil     float64 `json:"gpu_memory_utilization,omitempty"`
	KVCache     string  `json:"kv_cache,omitempty"`
	BatchTokens int     `json:"batch_tokens,omitempty"`
}
type Deployment struct {
	Repository      string `json:"repository,omitempty"`
	ID              string `json:"id"`
	Model           string `json:"model"`
	Version         string `json:"model_version,omitempty"`
	Backend         string `json:"backend"`
	AutoStart       bool   `json:"auto_start"`
	IdleMinutes     int    `json:"idle_minutes"`
	Tuning          Tuning `json:"tuning"`
	PreviousModel   string `json:"previous_model,omitempty"`
	PreviousVersion string `json:"previous_version,omitempty"`
}
type DeploymentView struct {
	Deployment
	Status      State  `json:"status"`
	Revision    string `json:"model_revision"`
	Fingerprint string `json:"deployment_revision"`
	Path        string `json:"path"`
	ModelName   string `json:"model_name"`
}

func (a *App) restoreDeployments() {
	for _, d := range a.config.Deployments {
		a.deployments[d.ID] = a.newDeployment(d)
	}
}
func (a *App) newDeployment(d Deployment) *App {
	c := a.config
	c.Deployments = nil
	c.Model = d.Model
	c.ModelVersion = d.Version
	c.Backend = d.Backend
	c.AutoStart = d.AutoStart
	c.IdleMinutes = d.IdleMinutes
	c.Tuning = d.Tuning
	return &App{parent: a, deploymentID: d.ID, addr: a.addr, dataDir: a.dataDir, config: c, installed: append([]Installed(nil), a.installed...), state: State{State: "stopped", UpdatedAt: time.Now().UTC()}, closed: a.closed, client: a.client, catalog: a.catalog, cancels: map[string]context.CancelFunc{}}
}
func (a *App) children() []*App {
	a.mu.Lock()
	defer a.mu.Unlock()
	result := make([]*App, 0, len(a.deployments))
	for _, child := range a.deployments {
		result = append(result, child)
	}
	return result
}
func (a *App) resolveDeployment(id string) (*App, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if id == "" || id == "default" || id == a.config.Model {
		return a, nil
	}
	if a.parent != nil && id == a.deploymentID {
		return a, nil
	}
	if child := a.deployments[id]; child != nil {
		return child, nil
	}
	return nil, errors.New("deployment_unknown")
}
func (a *App) deploymentViews() []DeploymentView {
	a.mu.Lock()
	defaults := Deployment{ID: a.config.Model, Model: a.config.Model, Version: a.config.ModelVersion, Backend: a.config.Backend, AutoStart: a.config.AutoStart, IdleMinutes: a.config.IdleMinutes, Tuning: a.config.Tuning}
	if a.parent != nil {
		defaults.ID = a.deploymentID
	}
	defs := append([]Deployment{defaults}, a.config.Deployments...)
	a.mu.Unlock()
	result := make([]DeploymentView, 0, len(defs))
	for _, d := range defs {
		child, e := a.resolveDeployment(d.ID)
		if e != nil {
			continue
		}
		a.mu.Lock()
		in, ok := a.modelLocked(d.Model, d.Version)
		a.mu.Unlock()
		child.mu.Lock()
		s := child.state
		child.mu.Unlock()
		revision, path, name := "", "", d.Model
		if ok {
			revision = in.Version + ":" + in.SHA256
			path = in.Path
			name = in.Name
			if in.Repository != "" {
				name = in.Repository
			}
		}
		b, _ := json.Marshal([]any{revision, d.Backend, d.Tuning})
		sum := sha256.Sum256(b)
		result = append(result, DeploymentView{Deployment: d, Status: s, Revision: revision, Fingerprint: hex.EncodeToString(sum[:]), Path: path, ModelName: name})
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}
func validateTuning(t Tuning) error {
	if t.GPUUtil < 0 || t.GPUUtil > 1 || t.BatchTokens < 0 || t.BatchTokens > 1048576 {
		return errors.New("invalid_tuning")
	}
	if t.KVCache != "" && !regexp.MustCompile(`^[1-9][0-9]{0,7}(MiB|GiB|tokens)$`).MatchString(t.KVCache) {
		return errors.New("invalid_kv_cache")
	}
	if t.Context < 0 || t.Context > 1048576 || t.Sequences < 0 || t.Sequences > 4096 || t.GPU < 0 || t.GPU > 100000 {
		return errors.New("invalid_tuning")
	}
	switch t.ISQ {
	case "", "Q4K", "Q6K", "Q8_0":
	default:
		return errors.New("invalid_isq")
	}
	return nil
}
func (a *App) putDeployment(w http.ResponseWriter, r *http.Request) {
	a.taskStart.Lock()
	defer a.taskStart.Unlock()
	var d Deployment
	if decode(r, &d) != nil {
		fail(w, 400, "invalid_json")
		return
	}
	if a.preparing(d.ID) {
		fail(w, 409, "deployment_busy")
		return
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,95}$`).MatchString(d.ID) || d.ID == "default" {
		fail(w, 400, "invalid_deployment_id")
		return
	}
	if d.Backend != "auto" && d.Backend != "cpu" && d.Backend != "cuda" && d.Backend != "metal" {
		fail(w, 400, "invalid_backend")
		return
	}
	if d.IdleMinutes < 0 || d.IdleMinutes > 1440 {
		fail(w, 400, "invalid_idle_minutes")
		return
	}
	if e := validateTuning(d.Tuning); e != nil {
		fail(w, 400, e.Error())
		return
	}
	a.lifecycle.Lock()
	defer a.lifecycle.Unlock()
	a.mu.Lock()
	if d.ID == a.config.Model {
		a.mu.Unlock()
		fail(w, 409, "deployment_id_reserved")
		return
	}
	in, ok := a.modelLocked(d.Model, d.Version)
	if !ok {
		a.mu.Unlock()
		fail(w, 400, "model_not_installed")
		return
	}
	d.Version = in.Version
	if child := a.deployments[d.ID]; child != nil {
		child.lifecycle.Lock()
		defer child.lifecycle.Unlock()
		child.mu.Lock()
		running := child.engine != nil
		child.mu.Unlock()
		if running {
			a.mu.Unlock()
			fail(w, 409, "stop_before_switch")
			return
		}
	}
	found := false
	for i, old := range a.config.Deployments {
		if old.ID == d.ID {
			d.PreviousModel, d.PreviousVersion = old.PreviousModel, old.PreviousVersion
			if old.Model != d.Model || old.Version != d.Version {
				d.PreviousModel, d.PreviousVersion = old.Model, old.Version
			}
			a.config.Deployments[i] = d
			found = true
		}
	}
	if !found {
		a.config.Deployments = append(a.config.Deployments, d)
	}
	a.deployments[d.ID] = a.newDeployment(d)
	a.mu.Unlock()
	if a.save() != nil {
		fail(w, 500, "settings_write_failed")
		return
	}
	writeJSON(w, 200, map[string]any{"deployments": a.deploymentViews()})
}

func (a *App) rollbackDeployment(w http.ResponseWriter, r *http.Request) {
	var v struct {
		ID string `json:"id"`
	}
	if decode(r, &v) != nil {
		fail(w, 400, "invalid_json")
		return
	}
	a.mu.Lock()
	var d Deployment
	for _, item := range a.config.Deployments {
		if item.ID == v.ID {
			d = item
			break
		}
	}
	a.mu.Unlock()
	if d.PreviousModel == "" {
		fail(w, 409, "previous_model_missing")
		return
	}
	d.Model, d.Version = d.PreviousModel, d.PreviousVersion
	b, _ := json.Marshal(d)
	next := r.Clone(r.Context())
	next.Body = io.NopCloser(bytes.NewReader(b))
	next.ContentLength = int64(len(b))
	a.putDeployment(w, next)
}
func (a *App) deleteDeployment(w http.ResponseWriter, r *http.Request) {
	a.taskStart.Lock()
	defer a.taskStart.Unlock()
	var v struct {
		ID string `json:"id"`
	}
	if decode(r, &v) != nil {
		fail(w, 400, "invalid_json")
		return
	}
	if a.preparing(v.ID) {
		fail(w, 409, "deployment_busy")
		return
	}
	a.lifecycle.Lock()
	defer a.lifecycle.Unlock()
	a.mu.Lock()
	child := a.deployments[v.ID]
	if child == nil {
		a.mu.Unlock()
		fail(w, 404, "deployment_unknown")
		return
	}
	child.lifecycle.Lock()
	defer child.lifecycle.Unlock()
	child.mu.Lock()
	running := child.engine != nil
	child.mu.Unlock()
	if running {
		a.mu.Unlock()
		fail(w, 409, "stop_before_switch")
		return
	}
	delete(a.deployments, v.ID)
	for i, d := range a.config.Deployments {
		if d.ID == v.ID {
			a.config.Deployments = append(a.config.Deployments[:i], a.config.Deployments[i+1:]...)
			break
		}
	}
	a.mu.Unlock()
	_ = a.save()
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (a *App) runtimeAction(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var v struct {
			ID    string `json:"id"`
			Model string `json:"model"`
		}
		if r.ContentLength != 0 && decode(r, &v) != nil {
			fail(w, 400, "invalid_json")
			return
		}
		if v.ID == "" {
			v.ID = v.Model
		}
		child, e := a.resolveDeployment(v.ID)
		if e != nil {
			fail(w, 404, e.Error())
			return
		}
		a.taskStart.Lock()
		defer a.taskStart.Unlock()
		canonical := v.ID
		if child == a {
			a.mu.Lock()
			canonical = a.config.Model
			a.mu.Unlock()
		}
		if a.preparing(canonical) {
			fail(w, 409, "deployment_busy")
			return
		}
		trace := r.Header.Get("traceparent")
		op := a.operation(kind, trace, func(ctx context.Context, id string) error {
			if kind == "stop" || kind == "restart" {
				child.stopEngine()
			}
			if kind != "stop" {
				return child.ensureEngine(ctx, trace)
			}
			return nil
		})
		op.DeploymentID = v.ID
		a.changeOp(op.ID, func(o *Operation) { o.DeploymentID = v.ID })
		writeJSON(w, 202, op)
	}
}
func (a *App) routeProxy(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/v1/models" {
		var models []map[string]any
		for _, d := range a.deploymentViews() {
			if d.Status.State == "running" {
				models = append(models, map[string]any{"id": d.ID, "object": "model", "owned_by": "local"})
			}
		}
		if models == nil {
			models = []map[string]any{}
		}
		writeJSON(w, 200, map[string]any{"object": "list", "data": models})
		return
	}
	id := r.URL.Query().Get("instance")
	if r.URL.Path == "/v1/chat/completions" {
		b, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<20))
		var p struct {
			Model string `json:"model"`
		}
		if e != nil || json.Unmarshal(b, &p) != nil {
			fail(w, 400, "invalid_json")
			return
		}
		id = p.Model
		r.Body = io.NopCloser(bytes.NewReader(b))
		r.ContentLength = int64(len(b))
	}
	if id == "" {
		if c, e := r.Cookie("mistral_gui_instance"); e == nil {
			id = c.Value
		}
	}
	child, e := a.resolveDeployment(id)
	if e != nil {
		fail(w, 404, e.Error())
		return
	}
	if r.URL.Query().Has("instance") {
		http.SetCookie(w, &http.Cookie{Name: "mistral_gui_instance", Value: id, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	}
	canonical := id
	if child == a {
		a.mu.Lock()
		canonical = a.config.Model
		a.mu.Unlock()
	}
	if a.preparing(canonical) {
		fail(w, 503, "deployment_busy")
		return
	}
	if child != a {
		installed := a.installedSnapshot()
		child.mu.Lock()
		child.installed = installed
		child.mu.Unlock()
	}
	child.proxy(w, r)
}
