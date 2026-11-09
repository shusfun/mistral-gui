package app

import (
	"context"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

func (a *App) capabilities(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	engine := a.config.Engine
	backend := r.URL.Query().Get("backend")
	if backend != "" && backend != "auto" {
		engine = ""
		for _, in := range a.installed {
			if in.Kind == "engine" && in.Backend == backend {
				engine = in.Path
			}
		}
	}
	a.mu.Unlock()
	features := map[string]bool{}
	if engine != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, engine, "serve", "--help")
		setProcess(cmd)
		if b, e := cmd.Output(); e == nil {
			for name, flag := range map[string]string{"gpu_layers": "--device-layers", "gpu_memory_utilization": "--gpu-memory-utilization", "kv_cache": "--kv-cache", "batch_tokens": "--max-num-batched-tokens", "isq": "--isq"} {
				features[name] = strings.Contains(string(b), flag)
			}
		}
	}
	if backend == "cpu" {
		for _, key := range []string{"gpu_layers", "gpu_memory_utilization", "kv_cache"} {
			features[key] = false
		}
	}
	writeJSON(w, 200, map[string]any{"features": features, "engine": engine})
}
