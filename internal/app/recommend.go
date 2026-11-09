package app

import (
	"context"
	"encoding/csv"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type GPU struct {
	Name      string `json:"name"`
	Driver    string `json:"driver"`
	Total     uint64 `json:"total_bytes"`
	Available uint64 `json:"available_bytes"`
}
type Device struct {
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	Cores     int    `json:"cpu_cores"`
	Total     uint64 `json:"memory_total_bytes"`
	Available uint64 `json:"memory_available_bytes"`
	Unified   bool   `json:"unified_memory"`
	Metal     bool   `json:"metal"`
	CUDA      bool   `json:"cuda"`
	GPUs      []GPU  `json:"gpus"`
}

func detectDevice(ctx context.Context) Device {
	total, available := memoryInfo()
	d := Device{OS: runtime.GOOS, Arch: runtime.GOARCH, Cores: runtime.NumCPU(), Total: total, Available: available, Unified: runtime.GOOS == "darwin" && runtime.GOARCH == "arm64", GPUs: []GPU{}}
	if d.Unified {
		cmd := exec.CommandContext(ctx, "/usr/sbin/system_profiler", "SPDisplaysDataType")
		b, e := cmd.Output()
		d.Metal = e == nil && strings.Contains(strings.ToLower(string(b)), "metal")
	}
	cmd := exec.CommandContext(ctx, nvidiaSMI(), "--query-gpu=name,driver_version,memory.total,memory.used", "--format=csv,noheader,nounits")
	setProcess(cmd)
	if b, e := cmd.Output(); e == nil {
		rows, _ := csv.NewReader(strings.NewReader(string(b))).ReadAll()
		for _, row := range rows {
			if len(row) != 4 {
				continue
			}
			all, _ := strconv.ParseUint(strings.TrimSpace(row[2]), 10, 64)
			used, _ := strconv.ParseUint(strings.TrimSpace(row[3]), 10, 64)
			free := uint64(0)
			if all > used {
				free = all - used
			}
			d.GPUs = append(d.GPUs, GPU{strings.TrimSpace(row[0]), strings.TrimSpace(row[1]), all << 20, free << 20})
		}
		d.CUDA = len(d.GPUs) > 0
	}
	return d
}
func (a *App) recommend(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	device := detectDevice(ctx)
	id := r.URL.Query().Get("model")
	a.mu.Lock()
	model, found := a.modelLocked(id, r.URL.Query().Get("version"))
	if id == "" {
		model, found = a.modelLocked(a.config.Model, a.config.ModelVersion)
	}
	a.mu.Unlock()
	if !found {
		fail(w, 404, "model_not_installed")
		return
	}
	backend := "cpu"
	if device.Metal {
		backend = "metal"
	}
	if device.CUDA {
		backend = "cuda"
	}
	tuning := Tuning{Context: 4096, Sequences: 1}
	weight := uint64(model.Size)
	if model.Format == "safetensors" {
		tuning.ISQ = "Q4K"
		weight = weight / 3
	}
	required := weight + weight/4 + (512 << 20)
	peak := required
	if model.Format == "safetensors" {
		peak += uint64(model.Size)
	}
	budget := device.Available
	if backend == "cuda" {
		var largest uint64
		for _, g := range device.GPUs {
			if g.Available > largest {
				largest = g.Available
			}
		}
		if required > largest {
			backend = "cpu"
		} else {
			budget = largest
		}
	}
	warnings := []string{}
	if model.Format == "safetensors" && device.Available > 0 && peak > device.Available {
		warnings = append(warnings, "loading_memory_budget_exceeded")
	}
	if budget == 0 {
		warnings = append(warnings, "memory_available_unknown")
	} else if required > budget {
		tuning.Context = 2048
		warnings = append(warnings, "memory_budget_exceeded")
	}
	writeJSON(w, 200, map[string]any{"device": device, "backend": backend, "tuning": tuning, "estimated_bytes": required, "estimated_loading_bytes": peak, "warnings": warnings, "advisory": true})
}
