package desktop

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/shusfun/mistral-gui/internal/app"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const transactionEnv = "MISTRAL_GUI_UPDATE_TRANSACTION"

var guardStartupTimeout = 120 * time.Second
var guardHealthTimeout = 60 * time.Second
var guardPollInterval = 500 * time.Millisecond

func launchPointer() string {
	dir, e := os.UserConfigDir()
	if e != nil {
		return ""
	}
	return filepath.Join(dir, "mistral-gui", "update-launch.json")
}

type transaction struct {
	ID         string   `json:"id"`
	Target     string   `json:"target"`
	Backup     string   `json:"backup"`
	Executable string   `json:"executable"`
	Dir        string   `json:"data_dir"`
	Addr       string   `json:"addr"`
	Args       []string `json:"args"`
	Version    string   `json:"version"`
	OldVersion string   `json:"old_version"`
	Trace      string   `json:"traceparent"`
	Hidden     bool     `json:"hidden"`
	Parent     int      `json:"parent"`
	PID        int      `json:"pid"`
	State      string   `json:"state"`
}

func writeTransaction(path string, t transaction) error {
	b, e := json.Marshal(t)
	if e != nil {
		return e
	}
	if e = os.WriteFile(path+".tmp", b, 0600); e != nil {
		return e
	}
	return os.Rename(path+".tmp", path)
}
func readTransaction(path string) (transaction, error) {
	var t transaction
	b, e := os.ReadFile(path)
	if e == nil {
		e = json.Unmarshal(b, &t)
	}
	if e == nil && (!filepath.IsAbs(t.Target) || !filepath.IsAbs(t.Dir) || filepath.Dir(path) != filepath.Join(t.Dir, "updates", "recovery", t.ID) || t.Backup != filepath.Join(filepath.Dir(path), filepath.Base(t.Target))) {
		e = errors.New("invalid_update_transaction")
	}
	return t, e
}
func copyTree(source, dest string) error {
	return filepath.WalkDir(source, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(source, p)
		if e != nil {
			return e
		}
		target := filepath.Join(dest, rel)
		info, e := d.Info()
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			link, e := os.Readlink(p)
			if e != nil {
				return e
			}
			return os.Symlink(link, target)
		}
		if d.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if e = os.MkdirAll(filepath.Dir(target), 0700); e != nil {
			return e
		}
		in, e := os.Open(p)
		if e != nil {
			return e
		}
		defer in.Close()
		out, e := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if e != nil {
			return e
		}
		_, e = io.Copy(out, in)
		closeErr := out.Close()
		if e != nil {
			return e
		}
		return closeErr
	})
}
func prepareGuard(a *app.App, version, trace string, hidden bool) (string, error) {
	exe, e := os.Executable()
	if e != nil {
		return "", e
	}
	target := exe
	if runtime.GOOS == "darwin" {
		i := strings.Index(exe, ".app/")
		if i < 0 {
			return "", errors.New("macos_app_required")
		}
		target = exe[:i+4]
	}
	id := fmt.Sprintf("%d-%d", time.Now().UnixNano(), os.Getpid())
	dir := filepath.Join(a.DataDir(), "updates", "recovery", id)
	if e = os.MkdirAll(dir, 0700); e != nil {
		return "", e
	}
	backup := filepath.Join(dir, filepath.Base(target))
	if e = copyTree(target, backup); e != nil {
		return "", e
	}
	t := transaction{ID: id, Target: target, Backup: backup, Executable: exe, Dir: a.DataDir(), Addr: a.Address(), Args: append([]string(nil), os.Args[1:]...), Version: version, OldVersion: app.Version, Trace: trace, Hidden: hidden, Parent: os.Getpid(), State: "pending"}
	path := filepath.Join(dir, "transaction.json")
	if e = writeTransaction(path, t); e != nil {
		return "", e
	}
	if pointer := launchPointer(); pointer != "" {
		if e = os.MkdirAll(filepath.Dir(pointer), 0700); e != nil {
			return "", e
		}
		if e = os.WriteFile(pointer, []byte(path), 0600); e != nil {
			return "", e
		}
	}
	helper := backup
	if runtime.GOOS == "darwin" {
		helper = filepath.Join(backup, "Contents", "MacOS", "mistral-gui")
	}
	cmd := exec.Command(helper, "--update-guard", path)
	detach(cmd)
	if e = cmd.Start(); e != nil {
		return "", e
	}
	_ = cmd.Process.Release()
	if e = os.Setenv(transactionEnv, path); e != nil {
		return "", e
	}
	return path, nil
}
func cancelGuard(path string) {
	t, e := readTransaction(path)
	if e == nil {
		t.State = "cancelled"
		_ = writeTransaction(path, t)
	}
	_ = os.Unsetenv(transactionEnv)
}

// RestoreLaunch runs before flags and before creating a WebView.
func RestoreLaunch() (bool, error) {
	path := os.Getenv(transactionEnv)
	if path == "" {
		if b, e := os.ReadFile(launchPointer()); e == nil {
			path = string(b)
		}
	}
	if path == "" {
		return false, nil
	}
	t, e := readTransaction(path)
	if e != nil {
		return false, e
	}
	exe, e := os.Executable()
	if e != nil {
		return false, e
	}
	if filepath.Clean(exe) != filepath.Clean(t.Executable) {
		if os.Getenv(transactionEnv) == "" {
			return false, nil
		}
		return false, errors.New("update_target_mismatch")
	}
	if t.State != "pending" && t.State != "restarting" {
		_ = os.Unsetenv(transactionEnv)
		if b, e := os.ReadFile(launchPointer()); e == nil && string(b) == path {
			_ = os.Remove(launchPointer())
		}
		return false, nil
	}
	os.Args = append([]string{exe}, t.Args...)
	t.PID = os.Getpid()
	t.State = "restarting"
	app.UpdateTransaction = t.ID
	if e = writeTransaction(path, t); e != nil {
		return false, e
	}
	return t.Hidden, nil
}
func RunGuard(path string) error {
	t, e := readTransaction(path)
	if e != nil {
		return e
	}
	deadline := time.Now().Add(guardStartupTimeout)
	var launched time.Time
	client := http.Client{Timeout: time.Second}
	for time.Now().Before(deadline) {
		current, e := readTransaction(path)
		if e != nil {
			return e
		}
		if current.State == "cancelled" {
			return nil
		}
		if current.PID > 0 && current.PID != t.Parent {
			t.PID = current.PID
			if launched.IsZero() {
				launched = time.Now()
			}
		}
		if !launched.IsZero() {
			req, _ := http.NewRequest("GET", "http://"+t.Addr+"/healthz", nil)
			req.Header.Set("traceparent", t.Trace)
			resp, e := client.Do(req)
			if e == nil {
				var h struct {
					OK          bool   `json:"ok"`
					PID         int    `json:"pid"`
					Version     string `json:"version"`
					Service     string `json:"service"`
					Instance    string `json:"instance"`
					Transaction string `json:"update_transaction"`
				}
				e = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&h)
				resp.Body.Close()
				if e == nil && resp.StatusCode == 200 && h.OK && h.PID == t.PID && h.Instance != "" && h.Transaction == t.ID && h.Service == "mistral-gui" && strings.TrimPrefix(h.Version, "v") == strings.TrimPrefix(t.Version, "v") {
					t.State = "complete"
					_ = writeTransaction(path, t)
					return guardResult(t, "complete", nil)
				}
			}
			if time.Since(launched) >= guardHealthTimeout {
				break
			}
		}
		time.Sleep(guardPollInterval)
	}
	if processAlive(t.Parent) {
		return guardResult(t, "failed", errors.New("old_manager_exit_timeout"))
	}
	if t.PID > 0 && processAlive(t.PID) && !processIsTarget(t.PID, t.Executable) {
		return guardResult(t, "rollback_failed", errors.New("update_process_identity_mismatch"))
	}
	if t.PID > 0 && t.PID != t.Parent && processIsTarget(t.PID, t.Executable) {
		process, e := os.FindProcess(t.PID)
		if e == nil {
			_ = process.Kill()
			_, _ = process.Wait()
		}
	}
	// The recovery copy is independent of the framework's temporary .bak.
	failed := t.Target + ".failed-" + t.ID
	for i := 0; i < 60; i++ {
		e = os.Rename(t.Target, failed)
		if e == nil || os.IsNotExist(e) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if e != nil && !os.IsNotExist(e) {
		return guardResult(t, "rollback_failed", e)
	}
	if e = copyTree(t.Backup, t.Target); e != nil {
		return guardResult(t, "rollback_failed", e)
	}
	t.State = "rolled_back"
	_ = writeTransaction(path, t)
	cmd := exec.Command(t.Executable, t.Args...)
	cmd.Env = withoutTransaction(os.Environ())
	detach(cmd)
	if e = cmd.Start(); e != nil {
		return guardResult(t, "rollback_failed", e)
	}
	_ = cmd.Process.Release()
	return guardResult(t, "rolled_back", errors.New("update_health_check_failed"))
}
func withoutTransaction(env []string) []string {
	out := make([]string, 0, len(env))
	for _, v := range env {
		if !strings.HasPrefix(v, transactionEnv+"=") && !strings.HasPrefix(v, "WAILS_UPDATER_HELPER_") {
			out = append(out, v)
		}
	}
	return out
}
func guardResult(t transaction, state string, cause error) error {
	t.State = state
	path := filepath.Join(t.Dir, "updates", "recovery", t.ID, "transaction.json")
	if e := writeTransaction(path, t); e != nil {
		return e
	}
	if b, e := os.ReadFile(launchPointer()); e == nil && string(b) == path {
		_ = os.Remove(launchPointer())
	}
	detail := ""
	if cause != nil {
		detail = cause.Error()
	}
	b, _ := json.Marshal(map[string]any{"state": state, "error": detail, "version": t.Version, "traceparent": t.Trace, "transaction": t.ID})
	return os.WriteFile(filepath.Join(t.Dir, "update-status.json"), b, 0600)
}
