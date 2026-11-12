package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/shusfun/mistral-gui/internal/catalog"
	"github.com/shusfun/mistral-gui/internal/download"
)

// PrepareRequest is persisted with the task so retries use the same model revision.
type PrepareRequest struct {
	Deployment
	Recommend bool `json:"recommend,omitempty"`
	Restart   bool `json:"restart,omitempty"`
}

func (a *App) acquireResources(ctx context.Context) error {
	a.mu.Lock()
	if a.resourceQueue == nil {
		a.resourceQueue = make(chan struct{}, 1)
	}
	queue := a.resourceQueue
	a.mu.Unlock()
	select {
	case queue <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-a.closed:
		return errors.New("manager_stopped")
	}
}
func (a *App) releaseResources() { <-a.resourceQueue }

func (a *App) preparing(id string) bool {
	for _, op := range a.operationSnapshot() {
		if op.Kind == "prepare" && op.DeploymentID == id && op.State == "running" {
			return true
		}
	}
	return false
}

func (a *App) prepareDeployment(w http.ResponseWriter, r *http.Request) {
	var v PrepareRequest
	if decode(r, &v) != nil {
		fail(w, 400, "invalid_json")
		return
	}
	a.taskStart.Lock()
	defer a.taskStart.Unlock()
	var existing *Deployment
	a.mu.Lock()
	defaultID := a.config.Model
	a.mu.Unlock()
	for _, d := range a.deploymentViews() {
		if d.ID == v.ID || (v.ID == "default" && d.ID == defaultID) {
			copy := d.Deployment
			existing = &copy
			break
		}
	}
	if v.Model == "" {
		if existing == nil {
			fail(w, 404, "deployment_unknown")
			return
		}
		backend := v.Backend
		v.Deployment = *existing
		if backend != "" {
			v.Backend = backend
		}
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,95}$`).MatchString(v.ID) || v.ID == "default" || v.ID == "new" {
		fail(w, 400, "invalid_deployment_id")
		return
	}
	if v.Backend == "" {
		v.Backend = "auto"
	}
	if v.Backend != "auto" && v.Backend != "cpu" && v.Backend != "cuda" && v.Backend != "metal" {
		fail(w, 400, "invalid_backend")
		return
	}
	if v.IdleMinutes < 0 || v.IdleMinutes > 1440 {
		fail(w, 400, "invalid_idle_minutes")
		return
	}
	if err := validateTuning(v.Tuning); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if v.Repository != "" && !catalog.ValidRepository(v.Repository) {
		fail(w, 400, "invalid_repository")
		return
	}
	encoded, _ := json.Marshal(v)
	key := sha256.Sum256(encoded)
	requestKey := hex.EncodeToString(key[:])
	if v.ID == defaultID && v.Model != defaultID {
		fail(w, 409, "deployment_id_reserved")
		return
	}
	for _, op := range a.operationSnapshot() {
		if op.Kind == "prepare" && op.DeploymentID == v.ID && op.State == "running" {
			if op.PrepareKey == requestKey {
				writeJSON(w, 202, op)
			} else {
				fail(w, 409, "deployment_busy")
			}
			return
		}
	}
	if child, err := a.resolveDeployment(v.ID); err == nil {
		child.mu.Lock()
		running := child.engine != nil
		child.mu.Unlock()
		if running && !v.Restart {
			if existing != nil && reflect.DeepEqual(*existing, v.Deployment) {
				writeJSON(w, 200, map[string]bool{"ready": true})
				return
			}
			fail(w, 409, "stop_before_switch")
			return
		}
	}
	// Reserve a new ID before returning so the UI can immediately show its task.
	if existing == nil {
		a.mu.Lock()
		a.config.Deployments = append(a.config.Deployments, v.Deployment)
		a.deployments[v.ID] = a.newDeployment(v.Deployment)
		a.mu.Unlock()
		if err := a.save(); err != nil {
			fail(w, 500, "settings_write_failed")
			return
		}
	}
	ready := make(chan struct{})
	op := a.operation("prepare", r.Header.Get("traceparent"), func(ctx context.Context, id string) error {
		<-ready
		return a.prepare(ctx, id, v)
	})
	op.DeploymentID, op.Prepare, op.PrepareKey, op.Step = v.ID, &v, requestKey, "queued"
	a.changeOp(op.ID, func(o *Operation) { o.DeploymentID, o.Prepare, o.PrepareKey, o.Step = v.ID, &v, requestKey, "queued" })
	_ = a.save()
	close(ready)
	writeJSON(w, 202, op)
}

func childSpan(trace string) string {
	if !validTraceparent(trace) {
		return "00-" + randomToken()[:32] + "-" + randomToken()[:16] + "-01"
	}
	return trace[:36] + randomToken()[:16] + trace[52:]
}
func (a *App) prepareStage(ctx context.Context, id, phase string, run func(string) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	trace := a.operationTrace(id)
	span := childSpan(trace)
	a.changeOp(id, func(o *Operation) { o.SpanTrace = span; o.Step = phase; o.Progress = 0; o.Total = 0; o.Detail = "" })
	started := time.Now()
	slog.Info("deployment.stage.start", "traceId", traceRoot(trace), "spanId", span[36:52], "parentSpanId", trace[36:52], "operation", id, "stage", phase)
	err := run(span)
	outcome := "success"
	if err != nil {
		outcome = "failed"
	}
	slog.Info("deployment.stage.end", "traceId", traceRoot(trace), "spanId", span[36:52], "operation", id, "stage", phase, "outcome", outcome, "durationMs", time.Since(started).Milliseconds())
	if err != nil {
		return err
	}
	return ctx.Err()
}

func (a *App) prepare(ctx context.Context, id string, v PrepareRequest) error {
	var model Installed
	var engine Installed
	// The queue covers verification and installation, preventing shared staging races.
	acquired := false
	err := a.prepareStage(ctx, id, "queued", func(string) error { err := a.acquireResources(ctx); acquired = err == nil; return err })
	if err != nil {
		if acquired {
			a.releaseResources()
		}
		return err
	}
	err = func() error {
		defer a.releaseResources()
		if err := a.prepareStage(ctx, id, "preparing_model", func(string) error {
			a.mu.Lock()
			found, ok := a.modelLocked(v.Model, v.Version)
			a.mu.Unlock()
			if ok {
				if err := verifyInstalledModel(found); err != nil {
					return fmt.Errorf("model_file_invalid: %w", err)
				}
				model = found
				return nil
			}
			assets, err := a.catalogModels(ctx, v.Repository, false)
			if err != nil {
				return err
			}
			var asset catalog.Asset
			for _, item := range assets {
				if item.ID == v.Model && (v.Version == "" || v.Version == item.Version) {
					asset = item
					break
				}
			}
			if asset.ID == "" {
				return errors.New("model_unknown")
			}
			v.Version = asset.Version
			a.changeOp(id, func(o *Operation) { copy := v; o.Prepare = &copy; o.AssetKind = "model"; o.AssetID = asset.ID })
			if err = a.installResource(ctx, id, asset); err != nil {
				return err
			}
			a.mu.Lock()
			model, ok = a.modelLocked(v.Model, v.Version)
			a.mu.Unlock()
			if !ok {
				return errors.New("model_not_installed")
			}
			return verifyInstalledModel(model)
		}); err != nil {
			return err
		}
		v.Version = model.Version
		if err := a.prepareStage(ctx, id, "preparing_engine", func(string) error {
			if v.Backend == "auto" || v.Recommend {
				detectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				recommendation := recommendModel(detectDevice(detectCtx), model)
				cancel()
				if v.Backend == "auto" {
					v.Backend = recommendation.Backend
				}
				if v.Recommend {
					v.Tuning = recommendation.Tuning
				}
			}
			a.changeOp(id, func(o *Operation) { copy := v; o.Prepare = &copy })
			for _, in := range a.installedSnapshot() {
				if in.Kind == "engine" && in.Backend == v.Backend {
					if _, err := os.Stat(in.Path); err == nil {
						engine = in
					}
				}
			}
			if engine.Path != "" {
				return nil
			}
			asset, err := a.engineAsset(ctx, v.Backend)
			if err != nil {
				return err
			}
			a.changeOp(id, func(o *Operation) { o.AssetKind = "engine"; o.AssetID = asset.ID })
			if err = a.installResource(ctx, id, asset); err != nil {
				return err
			}
			for _, in := range a.installedSnapshot() {
				if in.Kind == "engine" && in.Backend == v.Backend && in.Version == asset.Version {
					engine = in
				}
			}
			if engine.Path == "" {
				return errors.New("engine_missing")
			}
			return nil
		}); err != nil {
			return err
		}
		return nil
	}()
	if err != nil {
		return err
	}
	child, err := a.resolveDeployment(v.ID)
	if err != nil {
		return err
	}
	if err = a.prepareStage(ctx, id, "saving_deployment", func(string) error {
		if v.Restart {
			child.stopEngine()
		}
		child.lifecycle.Lock()
		defer child.lifecycle.Unlock()
		child.mu.Lock()
		if child.engine != nil {
			child.mu.Unlock()
			return errors.New("stop_before_switch")
		}
		child.config.Model, child.config.ModelVersion, child.config.Backend = v.Model, v.Version, v.Backend
		child.config.Tuning, child.config.AutoStart, child.config.IdleMinutes = v.Tuning, v.AutoStart, v.IdleMinutes
		child.config.Engine, child.config.EngineVersion = engine.Path, engine.Version
		child.mu.Unlock()
		a.mu.Lock()
		for i, old := range a.config.Deployments {
			if old.ID == v.ID {
				v.PreviousModel, v.PreviousVersion = old.PreviousModel, old.PreviousVersion
				if old.Model != v.Model || old.Version != v.Version {
					v.PreviousModel, v.PreviousVersion = old.Model, old.Version
				}
				a.config.Deployments[i] = v.Deployment
			}
		}
		a.mu.Unlock()
		return a.save()
	}); err != nil {
		return err
	}
	started := false
	defer func() {
		if ctx.Err() != nil && started {
			child.stopEngine()
		}
	}()
	err = a.prepareStage(ctx, id, "starting", func(trace string) error {
		started = true
		return child.ensureEngine(ctx, trace)
	})
	if err != nil {
		return err
	}
	return a.prepareStage(ctx, id, "ready", func(string) error {
		if child.stateSnapshot().State != "running" {
			return errors.New("engine_not_ready")
		}
		return nil
	})
}

func verifyInstalledModel(in Installed) error {
	if len(in.Files) == 0 {
		return download.VerifyFile(in.Path, in.SHA256, "")
	}
	root := in.Path
	if in.Format != "safetensors" {
		root = filepath.Dir(in.Path)
		for range strings.Split(in.Files[0].Path, "/")[1:] {
			root = filepath.Dir(root)
		}
	}
	return verifyModelFiles(root, in.Files)
}
