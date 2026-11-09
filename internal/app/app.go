package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/shusfun/mistral-gui/internal/catalog"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

var Version = "0.1.0-dev"
var UpdateRepository = ""
var SupplementRepository = ""
var SupplementTagPrefix = "runtime-"
var UpdatePublicKey = ""

type Config struct {
	Token                string   `json:"token"`
	Language             string   `json:"language"`
	Backend              string   `json:"backend"`
	Model                string   `json:"model"`
	ModelVersion         string   `json:"model_version,omitempty"`
	PreviousModel        string   `json:"previous_model"`
	PreviousModelVersion string   `json:"previous_model_version,omitempty"`
	AutoStart            bool     `json:"auto_start"`
	IdleMinutes          int      `json:"idle_minutes"`
	Mirrors              []string `json:"mirrors"`
	Engine               string   `json:"engine"`
	PreviousEngine       string   `json:"previous_engine"`
	EngineVersion        string   `json:"engine_version"`
}
type State struct {
	State       string    `json:"state"`
	Backend     string    `json:"backend"`
	Model       string    `json:"model"`
	Revision    string    `json:"revision"`
	Error       string    `json:"error,omitempty"`
	Detail      string    `json:"detail,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
	Active      int       `json:"active"`
	Requests    int       `json:"requests"`
	DegradedCPU bool      `json:"degraded_cpu,omitempty"`
}
type Operation struct {
	Detail    string    `json:"detail,omitempty"`
	AssetKind string    `json:"asset_kind,omitempty"`
	AssetID   string    `json:"asset_id,omitempty"`
	Source    string    `json:"source,omitempty"`
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	State     string    `json:"state"`
	Progress  int64     `json:"progress"`
	Total     int64     `json:"total"`
	Step      string    `json:"step"`
	Error     string    `json:"error,omitempty"`
	TraceID   string    `json:"trace_id"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Installed struct {
	catalog.Asset
	Path string `json:"path"`
}
type App struct {
	mu                       sync.Mutex
	persist                  sync.Mutex
	lifecycle                sync.Mutex
	taskStart                sync.Mutex
	addr, dataDir, statePath string
	config                   Config
	state                    State
	installed                []Installed
	operations               []Operation
	cancels                  map[string]context.CancelFunc
	engine                   *engineProcess
	lastUse                  time.Time
	closed                   chan struct{}
	closeOnce                sync.Once
	instanceID               string
	client                   *http.Client
	catalog                  catalog.Client
}

func randomToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func New(addr, dir string) (*App, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return nil, errors.New("loopback_required")
	}
	if dir == "" {
		dir, err = os.UserConfigDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(dir, "mistral-gui")
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	a := &App{addr: addr, dataDir: dir, statePath: filepath.Join(dir, "state.json"), cancels: map[string]context.CancelFunc{}, closed: make(chan struct{}), client: &http.Client{}, state: State{State: "stopped", UpdatedAt: time.Now().UTC()}}
	a.catalog = catalog.Client{HTTP: &http.Client{Timeout: 30 * time.Second}, SupplementTagPrefix: SupplementTagPrefix}
	a.instanceID = randomToken()[:16]
	a.config = Config{Token: randomToken(), Backend: "auto", Model: catalog.DefaultModel, AutoStart: true, IdleMinutes: 10}
	b, err := os.ReadFile(a.statePath)
	if err == nil {
		var d struct {
			Config     Config      `json:"config"`
			Installed  []Installed `json:"installed"`
			Operations []Operation `json:"operations"`
		}
		if err = json.Unmarshal(b, &d); err != nil {
			return nil, err
		}
		a.config = d.Config
		a.installed = d.Installed
		a.operations = d.Operations
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if a.config.Token == "" {
		a.config.Token = randomToken()
	}
	for i := range a.operations {
		if a.operations[i].State == "running" {
			a.operations[i].State = "interrupted"
			a.operations[i].Error = "manager_restarted"
		}
	}
	if err = a.save(); err != nil {
		return nil, err
	}
	return a, nil
}
func (a *App) save() error {
	a.persist.Lock()
	defer a.persist.Unlock()
	a.mu.Lock()
	b, err := json.MarshalIndent(struct {
		Config     Config      `json:"config"`
		Installed  []Installed `json:"installed"`
		Operations []Operation `json:"operations"`
	}{a.config, a.installed, a.operations}, "", "  ")
	a.mu.Unlock()
	if err != nil {
		return err
	}
	tmp := a.statePath + ".tmp"
	if err = os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, a.statePath)
}
func (a *App) token() string        { a.mu.Lock(); defer a.mu.Unlock(); return a.config.Token }
func (a *App) stateSnapshot() State { a.mu.Lock(); defer a.mu.Unlock(); return a.state }
func (a *App) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"ok": true, "service": "mistral-gui", "version": Version, "instance": a.instance()})
}
func (a *App) instance() string { return a.instanceID }
func (a *App) Serve() error {
	l, err := net.Listen("tcp", a.addr)
	if err != nil {
		return err
	}
	return a.ServeListener(l)
}
func (a *App) ServeListener(l net.Listener) error {
	srv := &http.Server{Handler: a.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-a.closed
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()
	go a.idleLoop()
	slog.Info("mistral-gui listening", "addr", l.Addr().String(), "platform", runtime.GOOS+"/"+runtime.GOARCH)
	err := srv.Serve(l)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
func (a *App) Close() {
	a.closeOnce.Do(func() {
		close(a.closed)
		a.stopEngine()
		a.mu.Lock()
		for _, c := range a.cancels {
			c()
		}
		a.mu.Unlock()
		_ = a.save()
	})
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code}})
}
func decode(r *http.Request, v any) error {
	d := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	d.DisallowUnknownFields()
	return d.Decode(v)
}
func (a *App) operation(kind, trace string, run func(context.Context, string) error) Operation {
	ctx, cancel := context.WithCancel(context.Background())
	op := Operation{ID: randomToken()[:16], Kind: kind, State: "running", TraceID: trace, UpdatedAt: time.Now().UTC()}
	a.mu.Lock()
	a.operations = append(a.operations, op)
	a.cancels[op.ID] = cancel
	if len(a.operations) > 50 {
		a.operations = a.operations[len(a.operations)-50:]
	}
	a.mu.Unlock()
	_ = a.save()
	go func() {
		err := run(ctx, op.ID)
		a.changeOp(op.ID, func(o *Operation) {
			o.State = "success"
			if err != nil {
				o.State = "failed"
				o.Error = err.Error()
				code, _, _ := strings.Cut(o.Error, ":")
				if regexp.MustCompile(`^[a-z0-9_]+$`).MatchString(code) {
					o.Error = code
				} else {
					o.Error = "operation_failed"
				}
				if o.Error != err.Error() {
					o.Detail = err.Error()
				}
				if ctx.Err() != nil {
					o.State = "cancelled"
				}
			}
		})
		a.mu.Lock()
		delete(a.cancels, op.ID)
		a.mu.Unlock()
		cancel()
		_ = a.save()
	}()
	return op
}
func (a *App) changeOp(id string, f func(*Operation)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.operations {
		if a.operations[i].ID == id {
			f(&a.operations[i])
			a.operations[i].UpdatedAt = time.Now().UTC()
		}
	}
}
func (a *App) operationTrace(id string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, op := range a.operations {
		if op.ID == id {
			return op.TraceID
		}
	}
	return ""
}
func (a *App) String() string { return fmt.Sprintf("mistral-gui %s", Version) }
