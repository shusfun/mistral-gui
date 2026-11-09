package app

import (
	"context"
	"net/http"
)

func (a *App) fallbackCPU(w http.ResponseWriter, r *http.Request) {
	a.taskStart.Lock()
	defer a.taskStart.Unlock()
	a.mu.Lock()
	for _, op := range a.operations {
		if op.State == "running" {
			a.mu.Unlock()
			fail(w, 409, "task_in_progress")
			return
		}
	}
	a.mu.Unlock()
	trace := r.Header.Get("traceparent")
	op := a.operation("fallback_cpu", trace, func(ctx context.Context, id string) error {
		a.stopEngine()
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
		if a.config.Engine != cpu.Path {
			a.config.PreviousEngine = a.config.Engine
		}
		a.config.Engine, a.config.EngineVersion, a.config.Backend = cpu.Path, cpu.Version, "cpu"
		a.state.DegradedCPU = true
		a.mu.Unlock()
		if err := a.save(); err != nil {
			return err
		}
		err := a.ensureEngine(ctx, trace)
		if ctx.Err() != nil {
			a.stopEngine()
		}
		return err
	})
	writeJSON(w, 202, op)
}
