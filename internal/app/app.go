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
	"sort"
	"strings"
	"sync"
	"time"
)

var Version = "0.1.1-dev"
var UpdateRepository = ""
var SupplementRepository = ""
var SupplementTagPrefix = "runtime-"
var UpdatePublicKey = ""

var ErrUserCancelled = errors.New("user_cancelled")

type FailureRecord struct {
	Code     string    `json:"code"`
	Detail   string    `json:"detail,omitempty"`
	ExitCode int       `json:"exit_code,omitempty"`
	LogTail  string    `json:"log_tail,omitempty"`
	Time     time.Time `json:"time"`
}

type Config struct {
	Theme                 string       `json:"theme"`
	Deployments           []Deployment `json:"deployments,omitempty"`
	Tuning                Tuning       `json:"tuning"`
	Token                 string       `json:"token"`
	Language              string       `json:"language"`
	Backend               string       `json:"backend"`
	Model                 string       `json:"model"`
	ModelVersion          string       `json:"model_version,omitempty"`
	PreviousModel         string       `json:"previous_model"`
	PreviousModelVersion  string       `json:"previous_model_version,omitempty"`
	AutoStart             bool         `json:"auto_start"`
	IdleMinutes           int          `json:"idle_minutes"`
	Mirrors               []string     `json:"mirrors"`
	Engine                string       `json:"engine"`
	PreviousEngine        string       `json:"previous_engine"`
	EngineVersion         string       `json:"engine_version"`
	AllowedExtensions     []string     `json:"allowed_extensions,omitempty"`
	MaxRunningDeployments int          `json:"max_running_deployments,omitempty"`
	HighestSeenVersion    string       `json:"highest_seen_version,omitempty"`
	CloudAPIURL           string       `json:"cloud_api_url,omitempty"`
	CloudAPIKey           string       `json:"cloud_api_key,omitempty"`
	CloudModel            string       `json:"cloud_model,omitempty"`
	TranslateRouting      string       `json:"translate_routing,omitempty"`
	ScreenshotHotkey      string       `json:"screenshot_hotkey,omitempty"`
	LaunchAtLogin         bool         `json:"launch_at_login,omitempty"`
	CloudImageConfirmed   bool         `json:"cloud_image_confirmed,omitempty"`
}

type State struct {
	State         string         `json:"state"`
	Stage         string         `json:"stage,omitempty"`
	StartedAt     *time.Time     `json:"started_at,omitempty"`
	EstimatedMs   int64          `json:"estimated_ms,omitempty"`
	CooldownUntil *time.Time     `json:"cooldown_until,omitempty"`
	FailureCount  int            `json:"failure_count,omitempty"`
	LastFailure   *FailureRecord `json:"last_failure,omitempty"`
	Backend       string         `json:"backend"`
	Model         string         `json:"model"`
	Revision      string         `json:"revision"`
	Error         string         `json:"error,omitempty"`
	Detail        string         `json:"detail,omitempty"`
	UpdatedAt     time.Time      `json:"updated_at"`
	Active        int            `json:"active"`
	Requests      int            `json:"requests"`
	DegradedCPU   bool           `json:"degraded_cpu,omitempty"`
}

type Operation struct {
	Prepare      *PrepareRequest `json:"prepare,omitempty"`
	PrepareKey   string          `json:"prepare_key,omitempty"`
	SpanTrace    string          `json:"span_trace,omitempty"`
	DeploymentID string          `json:"deployment_id,omitempty"`
	Repository   string          `json:"repository,omitempty"`
	Detail       string          `json:"detail,omitempty"`
	AssetKind    string          `json:"asset_kind,omitempty"`
	AssetID      string          `json:"asset_id,omitempty"`
	Source       string          `json:"source,omitempty"`
	ID           string          `json:"id"`
	Kind         string          `json:"kind"`
	State        string          `json:"state"`
	Progress     int64           `json:"progress"`
	Total        int64           `json:"total"`
	Step         string          `json:"step"`
	Error        string          `json:"error,omitempty"`
	TraceID      string          `json:"trace_id"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type Installed struct {
	catalog.Asset
	Path string `json:"path"`
}

type persistedState struct {
	Config     Config      `json:"config"`
	Installed  []Installed `json:"installed"`
	Operations []Operation `json:"operations"`
}

type App struct {
	parent                   *App
	deploymentID             string
	deployments              map[string]*App
	desktop                  Desktop
	mu                       sync.Mutex
	persist                  sync.Mutex
	lifecycle                sync.Mutex
	taskStart                sync.Mutex
	resourceQueue            chan struct{}
	addr, dataDir, statePath string
	config                   Config
	state                    State
	installed                []Installed
	operations               []Operation
	cancels                  map[string]context.CancelFunc
	engine                   *engineProcess
	runtimeLog               *ringLogBuffer
	lastUse                  time.Time
	closed                   chan struct{}
	closeOnce                sync.Once
	instanceID               string
	client                   *http.Client
	catalog                  catalog.Client
	dataLock                 *DataDirLock
	exchangeCodes            map[string]exchangeTicket
	recoveredFromBackup      bool
	updating                 bool
	evictedDeployments       []string
	dictionary               *DictionaryStore
	ocr                      *OCRSupervisor
}

func randomToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func loadPersistedFile(path string) (persistedState, error) {
	var out persistedState
	b, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return out, errors.New("state_file_empty")
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return out, err
	}
	tok, err := unprotectSecret(out.Config.Token)
	if err != nil {
		return out, err
	}
	cloudKey, err := unprotectSecret(out.Config.CloudAPIKey)
	if err != nil {
		return out, err
	}
	out.Config.Token = tok
	out.Config.CloudAPIKey = cloudKey
	return out, nil
}

func New(addr, dir string) (*App, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return nil, errors.New("loopback_required")
	}
	dir, err = resolveDataDir(dir)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	a := &App{
		addr:          addr,
		dataDir:       dir,
		statePath:     filepath.Join(dir, "state.json"),
		cancels:       map[string]context.CancelFunc{},
		deployments:   map[string]*App{},
		exchangeCodes: map[string]exchangeTicket{},
		runtimeLog:    newRingLogBuffer(defaultRingLogCapacity),
		closed:        make(chan struct{}),
		client:        &http.Client{},
		state:         State{State: "stopped", UpdatedAt: time.Now().UTC()},
	}
	a.catalog = catalog.Client{HTTP: &http.Client{Timeout: 30 * time.Second}, SupplementTagPrefix: SupplementTagPrefix}
	a.instanceID = randomToken()[:16]
	a.resourceQueue = make(chan struct{}, 1)
	a.config = Config{Token: randomToken(), Backend: "auto", Model: catalog.DefaultModel, AutoStart: true, IdleMinutes: 10, TranslateRouting: "local-first"}

	d, err := loadPersistedFile(a.statePath)
	if err == nil {
		a.config = d.Config
		a.installed = d.Installed
		a.operations = d.Operations
	} else if !os.IsNotExist(err) {
		// Primary state.json is corrupt or truncated — automatically recover from state.json.bak (M-S01).
		bak, bakErr := loadPersistedFile(a.statePath + ".bak")
		if bakErr != nil {
			return nil, fmt.Errorf("state_corrupt: %w", err)
		}
		slog.Warn("state.recovered_from_backup", "path", a.statePath, "cause", err)
		a.config = bak.Config
		a.installed = bak.Installed
		a.operations = bak.Operations
		a.recoveredFromBackup = true
	}

	if a.config.Token == "" {
		a.config.Token = randomToken()
	}
	if a.config.Theme == "" {
		a.config.Theme = "system"
	}
	if a.config.TranslateRouting == "" {
		a.config.TranslateRouting = "local-first"
	}
	// Populate empty legacy ModelVersion when a single installed revision matches (M-VA).
	if a.config.ModelVersion == "" {
		if m, ok := a.modelLocked(a.config.Model, ""); ok {
			a.config.ModelVersion = m.Version
		}
	}
	a.restoreDeployments()
	for i := range a.operations {
		if a.operations[i].State == "running" {
			a.operations[i].State = "interrupted"
			a.operations[i].Error = "manager_restarted"
		}
	}
	cleanupStartupStaging(dir)
	if err = a.save(); err != nil {
		return nil, err
	}
	return a, nil
}

func writeSyncedFile(path string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func (a *App) save() error {
	if a.parent != nil {
		return nil
	}
	a.persist.Lock()
	defer a.persist.Unlock()
	a.mu.Lock()
	cfgCopy := a.config
	installedCopy := append([]Installed(nil), a.installed...)
	opsCopy := append([]Operation(nil), a.operations...)
	a.mu.Unlock()

	protTok, err := protectSecret(cfgCopy.Token)
	if err != nil {
		return err
	}
	protCloud, err := protectSecret(cfgCopy.CloudAPIKey)
	if err != nil {
		return err
	}
	cfgCopy.Token = protTok
	cfgCopy.CloudAPIKey = protCloud

	b, err := json.MarshalIndent(persistedState{
		Config:     cfgCopy,
		Installed:  installedCopy,
		Operations: opsCopy,
	}, "", "  ")
	if err != nil {
		return err
	}

	tmp := fmt.Sprintf("%s.%s.tmp", a.statePath, randomToken()[:12])
	defer func() { _ = os.Remove(tmp) }()
	if err = writeSyncedFile(tmp, b, 0600); err != nil {
		return err
	}
	// Preserve previous valid state.json as .bak before atomic replacement (M-S01).
	if prev, readErr := os.ReadFile(a.statePath); readErr == nil && len(strings.TrimSpace(string(prev))) > 0 {
		var check map[string]any
		if json.Unmarshal(prev, &check) == nil {
			bakTmp := fmt.Sprintf("%s.%s.bak.tmp", a.statePath, randomToken()[:12])
			if writeSyncedFile(bakTmp, prev, 0600) == nil {
				_ = os.Rename(bakTmp, a.statePath+".bak")
			}
			_ = os.Remove(bakTmp)
		}
	}
	return os.Rename(tmp, a.statePath)
}

func (a *App) root() *App {
	if a.parent != nil {
		return a.parent
	}
	return a
}

func (a *App) AttachLock(lock *DataDirLock) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.dataLock != nil && a.dataLock != lock {
		a.dataLock.Release()
	}
	a.dataLock = lock
}

func (a *App) token() string        { a.mu.Lock(); defer a.mu.Unlock(); return a.config.Token }
func (a *App) stateSnapshot() State { a.mu.Lock(); defer a.mu.Unlock(); return a.state }

var UpdateTransaction string

// health returns minimal liveness and instance identity when unauthenticated,
// omitting PID and update_transaction unless authorized or carrying matching X-Mistral-Update-Transaction (M-S15).
func (a *App) health(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	desktop := a.desktop
	a.mu.Unlock()
	ready := desktop == nil || desktop.Ready()
	status := 200
	if !ready {
		status = 503
	}
	resp := map[string]any{
		"ok":       ready,
		"service":  "mistral-gui",
		"version":  Version,
		"instance": a.instance(),
	}
	txHeader := r.Header.Get("X-Mistral-Update-Transaction")
	if a.authorized(r) || (UpdateTransaction != "" && txHeader != "" && txHeader == UpdateTransaction) {
		resp["pid"] = os.Getpid()
		resp["update_transaction"] = UpdateTransaction
	}
	writeJSON(w, status, resp)
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
	a.mu.Lock()
	hasLock := a.dataLock != nil
	a.mu.Unlock()
	if !hasLock {
		lock, err := AcquireDataDirLock(a.dataDir)
		if err != nil {
			_ = l.Close()
			return err
		}
		a.AttachLock(lock)
	}
	a.addr = l.Addr().String()
	_ = writeInstanceRecord(a.dataDir, a.addr, a.instanceID)

	srv := &http.Server{
		Handler:           a.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ConnContext: func(ctx context.Context, _ net.Conn) context.Context {
			return markNetworkConnContext(ctx)
		},
	}
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
		for _, child := range a.children() {
			child.stopEngine()
		}
		a.stopEngine()
		if a.ocr != nil {
			a.ocr.Stop()
		}
		a.mu.Lock()
		for _, c := range a.cancels {
			c()
		}
		lock := a.dataLock
		a.dataLock = nil
		a.mu.Unlock()
		_ = a.save()
		if lock != nil {
			lock.Release()
		}
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
		started := time.Now()
		slog.Info("operation.start", "traceId", traceRoot(trace), "operation", op.ID, "kind", kind)
		err := run(ctx, op.ID)
		slog.Info("operation.end", "traceId", traceRoot(trace), "operation", op.ID, "kind", kind, "durationMs", time.Since(started).Milliseconds(), "error", err)
		a.changeOp(op.ID, func(o *Operation) {
			o.State = "success"
			if err != nil {
				if errors.Is(err, ErrUserCancelled) || ctx.Err() != nil {
					o.State = "cancelled"
					o.Error = ""
					return
				}
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
			}
		})
		a.mu.Lock()
		delete(a.cancels, op.ID)
		a.mu.Unlock()
		cancel()
		_ = a.save()
		a.notifyCompletion(op.ID)
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
			if validTraceparent(op.SpanTrace) {
				return op.SpanTrace
			}
			return op.TraceID
		}
	}
	return ""
}

func (a *App) String() string { return fmt.Sprintf("mistral-gui %s", Version) }

func (a *App) activeRequestCount() int {
	a.mu.Lock()
	total := a.state.Active
	a.mu.Unlock()
	if a.parent == nil {
		for _, child := range a.children() {
			child.mu.Lock()
			total += child.state.Active
			child.mu.Unlock()
		}
	}
	return total
}

func (a *App) operationSnapshot() []Operation {
	a.mu.Lock()
	ops := append([]Operation(nil), a.operations...)
	a.mu.Unlock()
	if a.parent == nil {
		for _, child := range a.children() {
			child.mu.Lock()
			for _, op := range child.operations {
				op.DeploymentID = child.deploymentID
				ops = append(ops, op)
			}
			child.mu.Unlock()
		}
	}
	sort.SliceStable(ops, func(i, j int) bool { return ops[i].UpdatedAt.Before(ops[j].UpdatedAt) })
	if len(ops) > 200 {
		ops = ops[len(ops)-200:]
	}
	return ops
}
