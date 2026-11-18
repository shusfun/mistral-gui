package app

import (
	"context"
	"net/http"
)

// fallbackCPU switches either the default deployment or a specific deployment ID to the CPU runtime (M-R4, M-R2).
func (a *App) fallbackCPU(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if r.ContentLength > 0 && decode(r, &req) != nil {
		fail(w, 400, "invalid_json")
		return
	}
	target, err := a.resolveDeployment(req.ID)
	if err != nil {
		fail(w, 404, err.Error())
		return
	}
	a.taskStart.Lock()
	defer a.taskStart.Unlock()
	for _, op := range a.operationSnapshot() {
		if op.State == "running" {
			fail(w, 409, "task_in_progress")
			return
		}
	}
	trace := r.Header.Get("traceparent")
	op := a.operation("fallback_cpu", trace, func(ctx context.Context, id string) error {
		target.stopEngine()
		a.mu.Lock()
		var cpu Installed
		for _, in := range a.installed {
			if in.Kind == "engine" && in.Backend == "cpu" {
				cpu = in
			}
		}
		a.mu.Unlock()
		if cpu.Path == "" {
			asset, err := a.engineAsset(ctx, "cpu")
			if err != nil {
				return err
			}
			if err = a.install(ctx, id, asset); err != nil {
				return err
			}
			a.mu.Lock()
			for _, in := range a.installed {
				if in.Kind == "engine" && in.Backend == "cpu" {
					cpu = in
				}
			}
			a.mu.Unlock()
		}
		a.mu.Lock()
		if target == a {
			if a.config.Engine != cpu.Path {
				a.config.PreviousEngine = a.config.Engine
			}
			a.config.Engine, a.config.EngineVersion, a.config.Backend = cpu.Path, cpu.Version, "cpu"
		} else {
			for i := range a.config.Deployments {
				if a.config.Deployments[i].ID == target.deploymentID {
					a.config.Deployments[i].Backend = "cpu"
				}
			}
		}
		a.mu.Unlock()

		target.mu.Lock()
		target.config.Engine, target.config.EngineVersion, target.config.Backend = cpu.Path, cpu.Version, "cpu"
		target.state.DegradedCPU = true
		target.mu.Unlock()
		target.resetCooldown()

		if err := a.save(); err != nil {
			return err
		}
		err := target.ensureEngine(ctx, trace)
		if ctx.Err() != nil {
			target.stopEngine()
		}
		return err
	})
	if req.ID != "" {
		a.changeOp(op.ID, func(o *Operation) { o.DeploymentID = req.ID })
	}
	writeJSON(w, 202, op)
}
