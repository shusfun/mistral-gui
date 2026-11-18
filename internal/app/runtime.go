package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type engineProcess struct {
	cmd        *exec.Cmd
	done       chan struct{}
	ready      chan struct{}
	err        error
	target     string
	generation string
	modelID    string
	startupLog string
	runtimeLog *ringLogBuffer
}

type startupOutput struct {
	app     *App
	process *engineProcess
}

func (w startupOutput) Write(b []byte) (int, error) {
	chunk := string(b)
	w.app.mu.Lock()
	tok := w.app.config.Token
	cloudKey := w.app.config.CloudAPIKey
	if w.app.parent != nil {
		w.app.mu.Unlock()
		w.app.parent.mu.Lock()
		tok = w.app.parent.config.Token
		cloudKey = w.app.parent.config.CloudAPIKey
		w.app.parent.mu.Unlock()
		w.app.mu.Lock()
	}
	if w.process.runtimeLog != nil {
		w.process.runtimeLog.Append(chunk, tok, cloudKey)
	}
	select {
	case <-w.process.ready:
		w.app.mu.Unlock()
		return len(b), nil
	default:
	}
	redacted := w.process.startupLog + chunk
	if tok != "" {
		redacted = strings.ReplaceAll(redacted, tok, "[redacted]")
	}
	if cloudKey != "" {
		redacted = strings.ReplaceAll(redacted, cloudKey, "[redacted]")
	}
	w.process.startupLog = redacted
	if len(w.process.startupLog) > 8192 {
		w.process.startupLog = w.process.startupLog[len(w.process.startupLog)-8192:]
	}
	w.app.mu.Unlock()
	return len(b), nil
}

func cooldownDurationForFailures(failures int) time.Duration {
	switch {
	case failures < 2:
		return 0
	case failures == 2:
		return 30 * time.Second
	case failures == 3:
		return 120 * time.Second
	default:
		return 300 * time.Second
	}
}

func (a *App) resetCooldown() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.FailureCount = 0
	a.state.CooldownUntil = nil
	if a.state.State == "cooldown" {
		a.state.State = "stopped"
		a.state.Error = ""
		a.state.Detail = ""
		a.state.UpdatedAt = time.Now().UTC()
	}
}

func (a *App) recordEngineFailureLocked(code, detail string, exitCode int, logTail string, countTowardsCooldown bool) {
	now := time.Now().UTC()
	if logTail == "" && a.runtimeLog != nil {
		logTail = a.runtimeLog.Tail(8192)
	}
	if countTowardsCooldown {
		a.state.FailureCount++
	}
	a.state.LastFailure = &FailureRecord{
		Code:     code,
		Detail:   detail,
		ExitCode: exitCode,
		LogTail:  logTail,
		Time:     now,
	}
	if countTowardsCooldown {
		if d := cooldownDurationForFailures(a.state.FailureCount); d > 0 {
			until := now.Add(d)
			a.state.CooldownUntil = &until
			a.state.State = "cooldown"
		} else {
			a.state.CooldownUntil = nil
			a.state.State = "failed"
		}
	} else {
		a.state.State = "failed"
	}
	a.state.Error = code
	a.state.Detail = detail
	a.state.Stage = ""
	a.state.StartedAt = nil
	a.state.UpdatedAt = now
}

func estimatedLoadMs(sizeBytes int64, backend string) int64 {
	mib := sizeBytes >> 20
	if mib <= 0 {
		mib = 512
	}
	if backend == "cpu" {
		est := mib * 28
		if est < 5000 {
			return 5000
		}
		if est > 240000 {
			return 240000
		}
		return est
	}
	est := mib * 14
	if est < 3500 {
		return 3500
	}
	if est > 180000 {
		return 180000
	}
	return est
}

func estimateModelVRAMBytes(model Installed, tuning Tuning) uint64 {
	weight := uint64(0)
	if model.Size > 0 {
		weight = uint64(model.Size)
	}
	if model.Format == "safetensors" {
		switch tuning.ISQ {
		case "Q4K", "":
			weight = weight / 3
		case "Q6K":
			weight = weight / 2
		}
	}
	return weight + weight/4 + (384 << 20)
}

func (a *App) canonicalID() string {
	if a.parent != nil && a.deploymentID != "" {
		return a.deploymentID
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.config.Model
}

// coordinateResourcesBeforeStart enforces MaxRunningDeployments and GPU VRAM headroom by stopping LRU idle instances (M-R4).
func (a *App) coordinateResourcesBeforeStart(ctx context.Context, target *App, backend string, model Installed, tuning Tuning) {
	root := a.root()
	all := append([]*App{root}, root.children()...)

	type candidate struct {
		app     *App
		id      string
		lastUse time.Time
	}

	collectRunning := func() (totalRunning int, idle []candidate) {
		for _, inst := range all {
			if inst == target {
				continue
			}
			inst.mu.Lock()
			running := inst.engine != nil && (inst.state.State == "running" || inst.state.State == "loading")
			canEvict := inst.engine != nil && inst.state.State == "running" && inst.state.Active == 0
			lu := inst.lastUse
			inst.mu.Unlock()
			if running {
				totalRunning++
			}
			if canEvict {
				idle = append(idle, candidate{app: inst, id: inst.canonicalID(), lastUse: lu})
			}
		}
		sort.Slice(idle, func(i, j int) bool { return idle[i].lastUse.Before(idle[j].lastUse) })
		return totalRunning, idle
	}

	recordEviction := func(id string) {
		root.mu.Lock()
		root.evictedDeployments = append(root.evictedDeployments, id)
		if len(root.evictedDeployments) > 10 {
			root.evictedDeployments = root.evictedDeployments[len(root.evictedDeployments)-10:]
		}
		root.mu.Unlock()
		slog.Info("runtime.evicted_idle_deployment", "victim", id, "for", target.canonicalID())
	}

	root.mu.Lock()
	maxRun := root.config.MaxRunningDeployments
	root.mu.Unlock()

	if maxRun > 0 {
		for range len(all) {
			totalRunning, idle := collectRunning()
			if totalRunning < maxRun || len(idle) == 0 {
				break
			}
			victim := idle[0]
			if !victim.app.stopIfInactive() {
				break
			}
			recordEviction(victim.id)
		}
	}

	if (backend == "cuda" || backend == "metal") && model.Size > 0 {
		required := estimateModelVRAMBytes(model, tuning)
		for range len(all) {
			_, idle := collectRunning()
			if len(idle) == 0 {
				break
			}
			detectCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			dev := detectDevice(detectCtx)
			cancel()
			var avail uint64
			if dev.Unified {
				avail = dev.Available
			} else {
				for _, g := range dev.GPUs {
					if g.Available > avail {
						avail = g.Available
					}
				}
			}
			if avail == 0 || avail >= required {
				break
			}
			victim := idle[0]
			if !victim.app.stopIfInactive() {
				break
			}
			recordEviction(victim.id)
		}
	}
}

func (a *App) ensureEngine(ctx context.Context, trace string) error {
	if a.parent != nil {
		installed := a.parent.installedSnapshot()
		a.mu.Lock()
		a.installed = installed
		a.mu.Unlock()
	}
	a.lifecycle.Lock()
	select {
	case <-a.closed:
		a.lifecycle.Unlock()
		return errors.New("manager_stopped")
	default:
	}
	if a.root().isUpdating() {
		a.lifecycle.Unlock()
		return errors.New("manager_updating")
	}

	a.mu.Lock()
	if a.engine == nil && a.state.CooldownUntil != nil {
		if time.Now().UTC().Before(*a.state.CooldownUntil) {
			a.state.State = "cooldown"
			a.mu.Unlock()
			a.lifecycle.Unlock()
			return errors.New("runtime_cooldown")
		}
		a.state.CooldownUntil = nil
	}
	p := a.engine
	if p == nil {
		backend := a.config.Backend
		if backend == "auto" {
			backend = "cpu"
			if a.config.Engine != "" {
				for _, in := range a.installed {
					if in.Path == a.config.Engine {
						backend = in.Backend
					}
				}
			}
		}
		path := a.config.Engine
		matched := false
		for _, in := range a.installed {
			if in.Kind == "engine" && in.Path == path && in.Backend == backend {
				matched = true
			}
		}
		if !matched {
			path = ""
			for _, in := range a.installed {
				if in.Kind == "engine" && in.Backend == backend {
					path = in.Path
				}
			}
		}
		model := a.config.Model
		var modelPath, revision string
		in, ok := a.modelLocked(model, a.config.ModelVersion)
		if ok {
			modelPath = in.Path
			revision = in.Version + ":" + in.SHA256
		}
		if path == "" || modelPath == "" {
			a.recordEngineFailureLocked("install_engine_and_model_first", "", 0, "", false)
			a.mu.Unlock()
			a.lifecycle.Unlock()
			return errors.New("install_engine_and_model_first")
		}
		if _, err := os.Stat(path); err != nil {
			a.recordEngineFailureLocked("engine_missing", err.Error(), 0, "", false)
			a.mu.Unlock()
			a.lifecycle.Unlock()
			return errors.New("engine_missing")
		}
		if _, err := os.Stat(modelPath); err != nil {
			a.recordEngineFailureLocked("model_not_installed", err.Error(), 0, "", false)
			a.mu.Unlock()
			a.lifecycle.Unlock()
			return errors.New("model_not_installed")
		}
		tuning := a.config.Tuning
		if a.runtimeLog == nil {
			a.runtimeLog = newRingLogBuffer(defaultRingLogCapacity)
		}
		rlog := a.runtimeLog
		a.mu.Unlock()

		// Coordinate concurrent deployments and GPU VRAM headroom before spawning process (M-R4).
		a.coordinateResourcesBeforeStart(ctx, a, backend, in, tuning)

		port, err := freePort()
		if err != nil {
			a.lifecycle.Unlock()
			return err
		}
		if tuning.Context == 0 {
			tuning.Context = 4096
		}
		if tuning.Sequences == 0 {
			tuning.Sequences = 4
		}
		args := []string{"serve", "--host", "127.0.0.1", "--port", strconv.Itoa(port), "--hf-cache", filepath.Join(a.dataDir, "hf-cache"), "--max-seqs", strconv.Itoa(tuning.Sequences), "--disable-access-log"}
		if in.Format == "safetensors" {
			overrides, _ := json.Marshal(map[string]int{"max_position_embeddings": tuning.Context})
			args = append(args, "--format", "plain", "-m", modelPath, "--hf-overrides", string(overrides))
			if tuning.ISQ != "" {
				args = append(args, "--isq", strings.ToLower(tuning.ISQ))
			}
		} else {
			args = append(args, "--max-model-len", strconv.Itoa(tuning.Context))
			if len(in.Files) > 1 {
				paths := make([]string, 0, len(in.Files))
				root := filepath.Dir(modelPath)
				for range strings.Split(in.Files[0].Path, "/")[1:] {
					root = filepath.Dir(root)
				}
				for _, file := range in.Files {
					paths = append(paths, filepath.Join(root, filepath.FromSlash(file.Path)))
				}
				modelPath = strings.Join(paths, ";")
			}
			args = append(args, "-f", modelPath)
		}
		if tuning.GPU > 0 && backend != "cpu" {
			args = append(args, "--device-layers", "0:"+strconv.Itoa(tuning.GPU))
		}
		if tuning.GPUUtil > 0 && backend != "cpu" {
			args = append(args, "--gpu-memory-utilization", strconv.FormatFloat(tuning.GPUUtil, 'f', 2, 64))
		}
		if tuning.KVCache != "" && backend != "cpu" {
			args = append(args, "--kv-cache", tuning.KVCache)
		}
		if tuning.BatchTokens > 0 {
			args = append(args, "--max-num-batched-tokens", strconv.Itoa(tuning.BatchTokens))
		}
		if strings.HasPrefix(model, "HY-MT") {
			args = append(args, "--tok-model-id", "tencent/HY-MT1.5-1.8B")
		}
		if backend == "cpu" {
			args = append(args, "--cpu")
		}
		cmd := exec.Command(path, args...)
		setProcess(cmd)
		cmd.Dir = filepath.Dir(path)
		cmd.Env = engineEnvironment(backend, a.dataDir)
		rlog.Append(fmt.Sprintf("\n--- start generation (%s %s) ---\n", backend, model))
		p = &engineProcess{
			cmd:        cmd,
			done:       make(chan struct{}),
			ready:      make(chan struct{}),
			target:     fmt.Sprintf("http://127.0.0.1:%d", port),
			generation: randomToken()[:16],
			runtimeLog: rlog,
		}
		output := startupOutput{app: a, process: p}
		cmd.Stdout, cmd.Stderr = output, output
		if err = cmd.Start(); err != nil {
			a.mu.Lock()
			a.recordEngineFailureLocked("engine_start_failed", err.Error(), -1, rlog.Tail(8192), true)
			a.mu.Unlock()
			a.lifecycle.Unlock()
			return err
		}
		release, err := ownProcess(cmd)
		if err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			a.lifecycle.Unlock()
			return fmt.Errorf("process_supervision_failed: %w", err)
		}
		now := time.Now().UTC()
		a.mu.Lock()
		a.engine = p
		a.state.State = "loading"
		a.state.Stage = "starting_process"
		a.state.StartedAt = &now
		a.state.EstimatedMs = estimatedLoadMs(in.Size, backend)
		a.state.Backend = backend
		a.state.DegradedCPU = backend == "cpu" && a.state.DegradedCPU
		a.state.Model = model
		a.state.Revision = revision
		a.state.Error = ""
		a.state.Detail = ""
		a.state.UpdatedAt = now
		a.mu.Unlock()
		slog.Info("runtime.start", "traceId", traceRoot(trace), "generation", p.generation, "backend", backend, "model", model)
		go func() {
			defer release()
			err := cmd.Wait()
			exitCode := 0
			if cmd.ProcessState != nil {
				exitCode = cmd.ProcessState.ExitCode()
			}
			logTail := rlog.Tail(8192)
			a.mu.Lock()
			p.err = err
			unexpected := a.engine == p && a.state.State != "stopping"
			if a.engine == p {
				a.engine = nil
				if unexpected {
					code := a.state.Error
					if code == "" {
						code = "engine_exited"
					}
					detail := logTail
					if detail == "" {
						detail = p.startupLog
					}
					if err != nil {
						detail = strings.TrimSpace(detail + "\n" + err.Error())
					} else if detail == "" {
						detail = "engine exited before being stopped"
					}
					a.recordEngineFailureLocked(code, detail, exitCode, logTail, true)
				} else {
					a.state.Stage = ""
					a.state.StartedAt = nil
					a.state.UpdatedAt = time.Now().UTC()
				}
			}
			close(p.done)
			a.mu.Unlock()
			slog.Info("runtime.exit", "traceId", traceRoot(trace), "generation", p.generation, "failed", unexpected, "exitCode", exitCode)
		}()
		go a.readiness(p, trace)
	} else {
		a.mu.Unlock()
	}
	a.lifecycle.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.done:
		a.mu.Lock()
		inCooldown := a.state.State == "cooldown"
		a.mu.Unlock()
		if inCooldown {
			return errors.New("runtime_cooldown")
		}
		return errors.New("engine_exited")
	case <-p.ready:
		return nil
	}
}

func (a *App) readiness(p *engineProcess, trace string) {
	timer := time.NewTimer(5 * time.Minute)
	defer timer.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	c := http.Client{Timeout: 2 * time.Second}
	polls := 0
	for {
		select {
		case <-p.done:
			return
		case <-timer.C:
			a.mu.Lock()
			if a.engine == p {
				a.state.Error = "model_load_timeout"
				_ = p.cmd.Process.Kill()
			}
			a.mu.Unlock()
			return
		case <-ticker.C:
			polls++
			if polls == 1 {
				a.mu.Lock()
				if a.engine == p && a.state.State == "loading" && a.state.Stage == "starting_process" {
					a.state.Stage = "loading_weights"
					a.state.UpdatedAt = time.Now().UTC()
				}
				a.mu.Unlock()
			}
			// Guard against port takeover if our child already exited before readiness completed (M-S29).
			select {
			case <-p.done:
				return
			default:
			}
			req, _ := http.NewRequest("GET", p.target+"/v1/models", nil)
			req.Header.Set("traceparent", trace)
			resp, err := c.Do(req)
			if err == nil {
				var models struct {
					Data []struct {
						ID string `json:"id"`
					} `json:"data"`
				}
				decodeErr := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&models)
				_ = resp.Body.Close()
				if resp.StatusCode != 200 || decodeErr != nil || len(models.Data) == 0 || models.Data[0].ID == "" {
					continue
				}
				select {
				case <-p.done:
					return
				default:
				}
				a.mu.Lock()
				if a.engine == p && p.cmd.ProcessState == nil {
					p.modelID = models.Data[0].ID
					p.startupLog = ""
					a.state.State = "running"
					a.state.Stage = "ready"
					a.state.StartedAt = nil
					a.state.Error = ""
					a.state.Detail = ""
					a.state.CooldownUntil = nil
					a.state.UpdatedAt = time.Now().UTC()
					a.lastUse = time.Now()
					close(p.ready)
					// Reset consecutive crash counter once the deployment stays up for 15s of stable operation.
					gen := p.generation
					go func() {
						select {
						case <-p.done:
						case <-time.After(15 * time.Second):
							a.mu.Lock()
							if a.engine != nil && a.engine.generation == gen && a.state.State == "running" {
								a.state.FailureCount = 0
							}
							a.mu.Unlock()
						}
					}()
				}
				a.mu.Unlock()
				slog.Info("runtime.ready", "traceId", traceRoot(trace), "generation", p.generation)
				return
			}
		}
	}
}

func traceRoot(trace string) string {
	if validTraceparent(trace) {
		return trace[3:35]
	}
	return trace
}

func (a *App) stopEngine() {
	a.stop(false)
}

func (a *App) stopIfInactive() bool {
	a.lifecycle.Lock()
	defer a.lifecycle.Unlock()
	a.mu.Lock()
	if a.engine == nil || a.state.Active > 0 {
		a.mu.Unlock()
		return false
	}
	p := a.engine
	a.state.State = "stopping"
	a.mu.Unlock()
	terminateProcess(p.cmd)
	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
		_ = p.cmd.Process.Kill()
		<-p.done
	}
	a.mu.Lock()
	a.engine = nil
	a.state.State = "stopped"
	a.state.Stage = ""
	a.state.StartedAt = nil
	a.state.Error = ""
	a.state.Detail = ""
	a.state.UpdatedAt = time.Now().UTC()
	a.mu.Unlock()
	return true
}

// stop transitions current runtime state to "stopped" without erasing LastFailure history (M-R5).
func (a *App) stop(onlyIdle bool) {
	a.lifecycle.Lock()
	defer a.lifecycle.Unlock()
	a.mu.Lock()
	if onlyIdle && !a.isIdle() {
		a.mu.Unlock()
		return
	}
	p := a.engine
	if p != nil {
		a.state.State = "stopping"
	}
	a.mu.Unlock()
	if p != nil {
		terminateProcess(p.cmd)
		select {
		case <-p.done:
		case <-time.After(5 * time.Second):
			_ = p.cmd.Process.Kill()
			<-p.done
		}
	}
	a.mu.Lock()
	a.engine = nil
	a.state.State = "stopped"
	a.state.Stage = ""
	a.state.StartedAt = nil
	a.state.Error = ""
	a.state.Detail = ""
	a.state.UpdatedAt = time.Now().UTC()
	a.mu.Unlock()
}

func (a *App) idleLoop() {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-a.closed:
			return
		case <-t.C:
			for _, child := range a.children() {
				child.stop(true)
			}
			a.mu.Lock()
			idle := a.isIdle()
			a.mu.Unlock()
			if idle {
				a.stop(true)
			}
		}
	}
}

func isProcessReady(p *engineProcess) bool {
	if p == nil {
		return false
	}
	select {
	case <-p.ready:
		return true
	default:
		return false
	}
}

func (a *App) writeCooldownError(w http.ResponseWriter) bool {
	a.mu.Lock()
	until := a.state.CooldownUntil
	last := a.state.LastFailure
	if until == nil || !time.Now().UTC().Before(*until) {
		a.mu.Unlock()
		return false
	}
	rem := int(math.Ceil(time.Until(*until).Seconds()))
	if rem < 1 {
		rem = 1
	}
	a.state.State = "cooldown"
	a.mu.Unlock()
	payload := map[string]any{
		"code":                "runtime_cooldown",
		"retry_after_seconds": rem,
	}
	if last != nil {
		payload["last_failure"] = last
	}
	w.Header().Set("Retry-After", strconv.Itoa(rem))
	writeJSON(w, 503, map[string]any{"error": payload})
	return true
}

func (a *App) proxy(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/v1/") {
		fail(w, 404, "endpoint_unknown")
		return
	}
	if a.root().isUpdating() {
		fail(w, 503, "manager_updating")
		return
	}
	a.mu.Lock()
	auto := a.config.AutoStart && r.Header.Get("X-Mistral-Auto-Start") != "0"
	p := a.engine
	running := p != nil
	ready := isProcessReady(p)
	if !running && !strings.HasPrefix(r.URL.Path, "/v1/") && r.URL.Path != "/ui" && !strings.HasPrefix(r.URL.Path, "/ui/") {
		a.mu.Unlock()
		fail(w, 404, "endpoint_unknown")
		return
	}
	// GET/HEAD requests never auto-start a stopped engine (M-S13).
	if !running && (r.Method == "GET" || r.Method == "HEAD") {
		a.mu.Unlock()
		fail(w, 503, "runtime_stopped")
		return
	}
	a.mu.Unlock()

	if !running && a.writeCooldownError(w) {
		return
	}
	if !running && !auto {
		fail(w, 503, "runtime_stopped")
		return
	}

	// Async cold-start readiness handshake for Qingyi (M-R1, M-R6):
	// return 503 runtime_loading with stage and timing so the client polls without burning request timeout.
	if r.Header.Get("X-Mistral-Async-Ready") == "1" && !ready {
		trace := r.Header.Get("traceparent")
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			_ = a.ensureEngine(bgCtx, trace)
		}()
		// Brief yield so ensureEngine sets initial stage or surfaces immediate config/cooldown errors.
		time.Sleep(40 * time.Millisecond)
		if a.writeCooldownError(w) {
			return
		}
		a.mu.Lock()
		st := a.state
		curReady := isProcessReady(a.engine)
		a.mu.Unlock()
		if !curReady {
			if st.State == "failed" && st.Error != "" {
				fail(w, 503, st.Error)
				return
			}
			stage := st.Stage
			if stage == "" {
				stage = "starting_process"
			}
			var elapsedMs int64
			if st.StartedAt != nil {
				elapsedMs = time.Since(*st.StartedAt).Milliseconds()
			}
			writeJSON(w, 503, map[string]any{
				"error": map[string]any{
					"code":         "runtime_loading",
					"stage":        stage,
					"elapsed_ms":   elapsedMs,
					"estimated_ms": st.EstimatedMs,
				},
			})
			return
		}
	}

	a.mu.Lock()
	a.state.Active++
	a.lastUse = time.Now()
	a.mu.Unlock()
	defer func() { a.mu.Lock(); a.state.Active--; a.lastUse = time.Now(); a.mu.Unlock() }()

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	if err := a.ensureEngine(ctx, r.Header.Get("traceparent")); err != nil {
		if err.Error() == "runtime_cooldown" && a.writeCooldownError(w) {
			return
		}
		fail(w, 503, err.Error())
		return
	}
	a.mu.Lock()
	p = a.engine
	if p == nil {
		a.mu.Unlock()
		fail(w, 503, "engine_exited")
		return
	}
	a.state.Requests++
	a.lastUse = time.Now()
	a.mu.Unlock()
	if r.URL.Path == "/v1/chat/completions" {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<20))
		var payload map[string]json.RawMessage
		if err != nil || json.Unmarshal(body, &payload) != nil {
			fail(w, 400, "invalid_json")
			return
		}
		var model string
		_ = json.Unmarshal(payload["model"], &model)
		if model == a.stateSnapshot().Model || model == a.deploymentID {
			payload["model"], _ = json.Marshal(p.modelID)
			body, _ = json.Marshal(payload)
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
	}
	u, _ := url.Parse(p.target)
	proxy := httputil.NewSingleHostReverseProxy(u)
	old := proxy.Director
	proxy.Director = func(out *http.Request) {
		old(out)
		out.Header.Del("Authorization")
		out.Header.Del("Cookie")
		out.Host = u.Host
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Set("X-Mistral-Model-Revision", a.stateSnapshot().Revision)
		return nil
	}
	proxy.FlushInterval = -1
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) { fail(w, 502, "engine_connection_failed") }
	proxy.ServeHTTP(w, r)
}

func (a *App) deploymentLogs(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	child, err := a.resolveDeployment(id)
	if err != nil {
		fail(w, 404, err.Error())
		return
	}
	child.mu.Lock()
	rlog := child.runtimeLog
	st := child.state
	child.mu.Unlock()
	text := ""
	if rlog != nil {
		text = rlog.String()
	}
	if r.URL.Query().Get("download") == "1" {
		safeName := id
		if safeName == "" {
			safeName = "default"
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-engine.log"`, safeName))
		w.WriteHeader(200)
		_, _ = w.Write([]byte(text))
		return
	}
	writeJSON(w, 200, map[string]any{
		"id":           child.canonicalID(),
		"logs":         text,
		"last_failure": st.LastFailure,
	})
}

func freePort() (int, error) {
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		return 0, e
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// Caller holds mu.
func (a *App) isIdle() bool {
	return a.config.IdleMinutes > 0 && a.engine != nil && a.state.State == "running" && a.state.Active == 0 && time.Since(a.lastUse) > time.Duration(a.config.IdleMinutes)*time.Minute
}
