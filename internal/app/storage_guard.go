package app

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DataDirLock guards single-instance ownership of a data directory across ports (M-S10).
type DataDirLock struct {
	release func()
	once    sync.Once
}

func (l *DataDirLock) Release() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		if l.release != nil {
			l.release()
		}
	})
}

func resolveDataDir(dir string) (string, error) {
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(base, "mistral-gui")
	}
	return filepath.Abs(dir)
}

func AcquireDataDirLock(dir string) (*DataDirLock, error) {
	resolved, err := resolveDataDir(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(resolved, 0700); err != nil {
		return nil, err
	}
	rel, err := lockFileExclusive(filepath.Join(resolved, "instance.lock"))
	if err != nil {
		return nil, err
	}
	return &DataDirLock{release: rel}, nil
}

type InstanceRecord struct {
	Addr      string    `json:"addr"`
	Instance  string    `json:"instance"`
	PID       int       `json:"pid"`
	UpdatedAt time.Time `json:"updated_at"`
}

func writeInstanceRecord(dir, addr, instanceID string) error {
	rec := InstanceRecord{
		Addr:      addr,
		Instance:  instanceID,
		PID:       os.Getpid(),
		UpdatedAt: time.Now().UTC(),
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	target := filepath.Join(dir, "instance.json")
	if err := os.WriteFile(target+".tmp", b, 0600); err != nil {
		return err
	}
	return os.Rename(target+".tmp", target)
}

func readInstanceRecord(dir string) (InstanceRecord, error) {
	var rec InstanceRecord
	b, err := os.ReadFile(filepath.Join(dir, "instance.json"))
	if err != nil {
		return rec, err
	}
	err = json.Unmarshal(b, &rec)
	return rec, err
}

const diskSafetyHeadroom = 256 << 20 // 256 MiB reserve

// checkDiskSpace verifies free space in dataDir before large downloads, archive extractions, or imports (M-S04).
func checkDiskSpace(dir string, requiredBytes int64) error {
	if requiredBytes <= 0 {
		return nil
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	avail, err := diskFreeBytes(dir)
	if err != nil || avail == 0 {
		return nil
	}
	needed := uint64(requiredBytes) + diskSafetyHeadroom
	if avail < needed {
		return fmt.Errorf("insufficient_disk_space: need %d bytes, available %d bytes", needed, avail)
	}
	return nil
}

type TempStorageSummary struct {
	PartialBytes int64 `json:"partial_bytes"`
	PartialCount int   `json:"partial_count"`
	StagingBytes int64 `json:"staging_bytes"`
	StagingCount int   `json:"staging_count"`
	TotalBytes   int64 `json:"total_bytes"`
}

func dirSize(path string) int64 {
	var sum int64
	_ = filepath.WalkDir(path, func(_ string, d fs.DirEntry, e error) error {
		if e != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			sum += info.Size()
		}
		return nil
	})
	return sum
}

// cleanupStartupStaging removes abandoned .importing and .staging directories while keeping resumable .part files (M-S05).
func cleanupStartupStaging(dataDir string) {
	for _, sub := range []string{"models", "engines", "ocr", "updates"} {
		root := filepath.Join(dataDir, sub)
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
			if e != nil || p == root {
				return nil
			}
			name := d.Name()
			if d.IsDir() && (strings.HasSuffix(name, ".staging") || strings.HasSuffix(name, ".importing")) {
				_ = os.RemoveAll(p)
				return filepath.SkipDir
			}
			return nil
		})
	}
}

func scanTempStorage(dataDir string) TempStorageSummary {
	var out TempStorageSummary
	for _, sub := range []string{"downloads", "models", "engines", "ocr", "updates"} {
		root := filepath.Join(dataDir, sub)
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
			if e != nil || p == root {
				return nil
			}
			name := d.Name()
			if d.IsDir() && (strings.HasSuffix(name, ".staging") || strings.HasSuffix(name, ".importing")) {
				out.StagingCount++
				out.StagingBytes += dirSize(p)
				return filepath.SkipDir
			}
			if !d.IsDir() && strings.HasSuffix(name, ".part") {
				if info, err := d.Info(); err == nil {
					out.PartialCount++
					out.PartialBytes += info.Size()
				}
			}
			return nil
		})
	}
	out.TotalBytes = out.PartialBytes + out.StagingBytes
	return out
}

func cleanupTempStorage(dataDir string, includePartials bool) int64 {
	var freed int64
	for _, sub := range []string{"downloads", "models", "engines", "ocr", "updates"} {
		root := filepath.Join(dataDir, sub)
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
			if e != nil || p == root {
				return nil
			}
			name := d.Name()
			if d.IsDir() && (strings.HasSuffix(name, ".staging") || strings.HasSuffix(name, ".importing")) {
				sz := dirSize(p)
				if os.RemoveAll(p) == nil {
					freed += sz
				}
				return filepath.SkipDir
			}
			if includePartials && !d.IsDir() && strings.HasSuffix(name, ".part") {
				if info, err := d.Info(); err == nil {
					sz := info.Size()
					if os.Remove(p) == nil {
						freed += sz
					}
				}
			}
			return nil
		})
	}
	return freed
}

func (a *App) storageTemp(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, scanTempStorage(a.dataDir))
}

func (a *App) storageCleanup(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	for _, op := range a.operations {
		if op.State == "running" && (op.Kind == "install" || op.Kind == "import" || op.Kind == "prepare" || op.Kind == "update") {
			a.mu.Unlock()
			fail(w, 409, "task_in_progress")
			return
		}
	}
	a.mu.Unlock()
	var req struct {
		IncludePartials *bool `json:"include_partials"`
	}
	if r.ContentLength > 0 && decode(r, &req) != nil {
		fail(w, 400, "invalid_json")
		return
	}
	inc := true
	if req.IncludePartials != nil {
		inc = *req.IncludePartials
	}
	freed := cleanupTempStorage(a.dataDir, inc)
	writeJSON(w, 200, map[string]any{
		"ok":          true,
		"freed_bytes": freed,
		"temp":        scanTempStorage(a.dataDir),
	})
}

type exchangeTicket struct {
	expiresAt time.Time
}

// issueExchangeCode creates a single-use 60-second code so long-lived Tokens never appear in URLs or process args (M-S02).
func (a *App) issueExchangeCode() string {
	code := randomToken()
	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.exchangeCodes == nil {
		a.exchangeCodes = map[string]exchangeTicket{}
	}
	for k, v := range a.exchangeCodes {
		if now.After(v.expiresAt) {
			delete(a.exchangeCodes, k)
		}
	}
	if len(a.exchangeCodes) >= 16 {
		for k := range a.exchangeCodes {
			delete(a.exchangeCodes, k)
			break
		}
	}
	a.exchangeCodes[code] = exchangeTicket{expiresAt: now.Add(60 * time.Second)}
	return code
}

func (a *App) consumeExchangeCode(code string) bool {
	if len(code) != 64 {
		return false
	}
	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	ticket, ok := a.exchangeCodes[code]
	if !ok {
		return false
	}
	delete(a.exchangeCodes, code)
	return !now.After(ticket.expiresAt)
}

func (a *App) sessionExchange(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if decode(r, &body) != nil || !a.consumeExchangeCode(body.Code) {
		fail(w, 401, "invalid_exchange_code")
		return
	}
	a.setCookie(w)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
