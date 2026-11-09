package desktop

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestGuardFixture(t *testing.T) {
	if os.Getenv("MISTRAL_GUARD_FIXTURE") != "1" {
		return
	}
	if path := os.Getenv("MISTRAL_GUARD_RECEIPT"); path != "" {
		_ = os.WriteFile(path, []byte("restarted"), 0600)
		os.Exit(0)
	}
	time.Sleep(30 * time.Second)
	os.Exit(0)
}

func TestGuardRestoresAfterUnhealthyProcessAndAcceptsHealthyIdentity(t *testing.T) {
	oldStart, oldHealth, oldPoll := guardStartupTimeout, guardHealthTimeout, guardPollInterval
	guardStartupTimeout = 2 * time.Second
	guardHealthTimeout = 80 * time.Millisecond
	guardPollInterval = 10 * time.Millisecond
	defer func() { guardStartupTimeout, guardHealthTimeout, guardPollInterval = oldStart, oldHealth, oldPoll }()
	for _, healthy := range []bool{true, false} {
		t.Run(map[bool]string{true: "healthy", false: "rollback"}[healthy], func(t *testing.T) {
			dir := t.TempDir()
			exe, e := os.Executable()
			if e != nil {
				t.Fatal(e)
			}
			target := filepath.Join(dir, "mistral-gui")
			if runtime.GOOS == "windows" {
				target += ".exe"
			}
			if e = copyTree(exe, target); e != nil {
				t.Fatal(e)
			}
			txdir := filepath.Join(dir, "updates", "recovery", "fixture")
			if e = os.MkdirAll(txdir, 0700); e != nil {
				t.Fatal(e)
			}
			backup := filepath.Join(txdir, filepath.Base(target))
			if e = copyTree(target, backup); e != nil {
				t.Fatal(e)
			}
			cmd := exec.Command(target, "-test.run=TestGuardFixture")
			cmd.Env = append(os.Environ(), "MISTRAL_GUARD_FIXTURE=1")
			if e = cmd.Start(); e != nil {
				t.Fatal(e)
			}
			defer cmd.Process.Kill()
			listener, e := net.Listen("tcp", "127.0.0.1:0")
			if e != nil {
				t.Fatal(e)
			}
			srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !healthy {
					w.WriteHeader(503)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "pid": cmd.Process.Pid, "service": "mistral-gui", "instance": "new-instance", "version": "0.2.0", "update_transaction": "fixture"})
			})}
			go srv.Serve(listener)
			defer srv.Close()
			tx := transaction{ID: "fixture", Target: target, Backup: backup, Executable: target, Dir: dir, Addr: listener.Addr().String(), Args: []string{"-test.run=TestGuardFixture"}, Version: "0.2.0", Parent: 2147480000, PID: cmd.Process.Pid, State: "restarting"}
			path := filepath.Join(txdir, "transaction.json")
			if e = writeTransaction(path, tx); e != nil {
				t.Fatal(e)
			}
			receipt := filepath.Join(dir, "restarted")
			t.Setenv("MISTRAL_GUARD_FIXTURE", "1")
			t.Setenv("MISTRAL_GUARD_RECEIPT", receipt)
			if e = RunGuard(path); e != nil {
				t.Fatal(e)
			}
			b, e := os.ReadFile(filepath.Join(dir, "update-status.json"))
			if e != nil {
				t.Fatal(e)
			}
			var result map[string]any
			_ = json.Unmarshal(b, &result)
			want := "rolled_back"
			if healthy {
				want = "complete"
			}
			if result["state"] != want {
				t.Fatal(string(b))
			}
			if !healthy {
				until := time.Now().Add(3 * time.Second)
				for time.Now().Before(until) {
					if _, e = os.Stat(receipt); e == nil {
						return
					}
					time.Sleep(10 * time.Millisecond)
				}
				t.Fatal("previous executable was not relaunched")
			}
		})
	}
}

func TestTransactionRejectsPathsOutsideRecoveryDirectory(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "transaction.json")
	b, _ := json.Marshal(transaction{ID: "x", Dir: dir, Target: filepath.Join(dir, "app"), Backup: filepath.Join(dir, "backup")})
	_ = os.WriteFile(p, b, 0600)
	if _, e := readTransaction(p); e == nil {
		t.Fatal("invalid recovery path accepted")
	}
}
