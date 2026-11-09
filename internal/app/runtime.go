package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
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
}

type startupOutput struct {
	app     *App
	process *engineProcess
}

func (w startupOutput) Write(b []byte) (int, error) {
	w.app.mu.Lock()
	defer w.app.mu.Unlock()
	select {
	case <-w.process.ready:
		return len(b), nil
	default:
	}
	w.process.startupLog = strings.ReplaceAll(w.process.startupLog+string(b), w.app.config.Token, "[redacted]")
	if len(w.process.startupLog) > 8192 {
		w.process.startupLog = w.process.startupLog[len(w.process.startupLog)-8192:]
	}
	return len(b), nil
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
	a.mu.Lock()
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
		if in, ok := a.modelLocked(model, a.config.ModelVersion); ok {
			modelPath = in.Path
			revision = in.Version + ":" + in.SHA256
		}
		if path == "" || modelPath == "" {
			a.state.State = "failed"
			a.state.Error = "install_engine_and_model_first"
			a.mu.Unlock()
			a.lifecycle.Unlock()
			return errors.New("install_engine_and_model_first")
		}
		if _, err := os.Stat(path); err != nil {
			a.state.State, a.state.Error, a.state.Detail = "failed", "engine_missing", err.Error()
			a.mu.Unlock()
			a.lifecycle.Unlock()
			return errors.New("engine_missing")
		}
		if _, err := os.Stat(modelPath); err != nil {
			a.state.State, a.state.Error, a.state.Detail = "failed", "model_not_installed", err.Error()
			a.mu.Unlock()
			a.lifecycle.Unlock()
			return errors.New("model_not_installed")
		}
		a.mu.Unlock()
		port, err := freePort()
		if err != nil {
			a.lifecycle.Unlock()
			return err
		}
		a.mu.Lock()
		tuning := a.config.Tuning
		in, _ := a.modelLocked(model, a.config.ModelVersion)
		a.mu.Unlock()
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
				// modelPath is the first shard; files retain their repository-relative paths.
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
		// Do not inherit unrelated application credentials into the engine.
		cmd.Env = engineEnvironment(backend, a.dataDir)
		p = &engineProcess{cmd: cmd, done: make(chan struct{}), ready: make(chan struct{}), target: fmt.Sprintf("http://127.0.0.1:%d", port), generation: randomToken()[:16]}
		output := startupOutput{app: a, process: p}
		cmd.Stdout, cmd.Stderr = output, output
		if err = cmd.Start(); err != nil {
			a.mu.Lock()
			a.state.State, a.state.Error, a.state.Detail = "failed", "engine_start_failed", err.Error()
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
		a.mu.Lock()
		a.engine = p
		a.state.State = "loading"
		a.state.Backend = backend
		a.state.DegradedCPU = backend == "cpu" && a.state.DegradedCPU
		a.state.Model = model
		a.state.Revision = revision
		a.state.Error = ""
		a.state.Detail = ""
		a.state.UpdatedAt = time.Now().UTC()
		a.mu.Unlock()
		slog.Info("runtime.start", "traceId", traceRoot(trace), "generation", p.generation, "backend", backend, "model", model)
		go func() {
			defer release()
			err := cmd.Wait()
			a.mu.Lock()
			p.err = err
			unexpected := a.engine == p && a.state.State != "stopping"
			if a.engine == p {
				a.engine = nil
				if a.state.State != "stopping" {
					a.state.State = "failed"
					if a.state.Error == "" {
						a.state.Error = "engine_exited"
					}
					a.state.Detail = p.startupLog
					if err != nil {
						a.state.Detail = strings.TrimSpace(a.state.Detail + "\n" + err.Error())
					} else if a.state.Detail == "" {
						a.state.Detail = "engine exited before being stopped"
					}
				}
				a.state.UpdatedAt = time.Now().UTC()
			}
			close(p.done)
			a.mu.Unlock()
			slog.Info("runtime.exit", "traceId", traceRoot(trace), "generation", p.generation, "failed", unexpected)
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
	for {
		select {
		case <-p.done:
			return
		case <-timer.C:
			a.mu.Lock()
			if a.engine == p {
				a.state.Error = "model_load_timeout"
				a.state.State = "failed"
				_ = p.cmd.Process.Kill()
			}
			a.mu.Unlock()
			return
		case <-ticker.C:
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
				if resp.StatusCode != 200 || decodeErr != nil || len(models.Data) == 0 {
					continue
				}
				a.mu.Lock()
				if a.engine == p {
					p.modelID = models.Data[0].ID
					p.startupLog = ""
					a.state.State = "running"
					a.state.UpdatedAt = time.Now().UTC()
					a.lastUse = time.Now()
					close(p.ready)
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
func (a *App) proxy(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/v1/") {
		fail(w, 404, "endpoint_unknown")
		return
	}
	a.mu.Lock()
	auto := a.config.AutoStart && r.Header.Get("X-Mistral-Auto-Start") != "0"
	running := a.engine != nil
	if !running && !strings.HasPrefix(r.URL.Path, "/v1/") && r.URL.Path != "/ui" && !strings.HasPrefix(r.URL.Path, "/ui/") {
		a.mu.Unlock()
		fail(w, 404, "endpoint_unknown")
		return
	}
	a.state.Active++
	a.lastUse = time.Now()
	a.mu.Unlock()
	defer func() { a.mu.Lock(); a.state.Active--; a.lastUse = time.Now(); a.mu.Unlock() }()
	if !running && !auto {
		fail(w, 503, "runtime_stopped")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	if err := a.ensureEngine(ctx, r.Header.Get("traceparent")); err != nil {
		fail(w, 503, err.Error())
		return
	}
	a.mu.Lock()
	p := a.engine
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
		// Upstream WebUI uses origin-relative URLs; proxy all non-manager paths as well.
		return nil
	}
	proxy.FlushInterval = -1
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) { fail(w, 502, "engine_connection_failed") }
	proxy.ServeHTTP(w, r)
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
