package update

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestApplyHealthAndRollback(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		t.Skip("Desktop update targets only")
	}
	buildDir := t.TempDir()
	binary := filepath.Join(buildDir, "manager.exe")
	c := exec.Command("go", "build", "-ldflags", "-X github.com/shusfun/mistral-gui/internal/app.Version=v9.0.0", "-o", binary, "../../cmd/mistral-gui")
	if b, e := c.CombinedOutput(); e != nil {
		t.Fatalf("%s %v", b, e)
	}
	bytes, e := os.ReadFile(binary)
	if e != nil {
		t.Fatal(e)
	}
	for _, success := range []bool{true, false} {
		t.Run(map[bool]string{true: "healthy", false: "corrupt_new_executable"}[success], func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "mistral-gui.exe")
			executable := target
			if runtime.GOOS == "darwin" {
				target = filepath.Join(dir, "mistral-gui.app")
				executable = filepath.Join(target, "Contents", "MacOS", "mistral-gui")
				if err := os.MkdirAll(filepath.Dir(executable), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(executable, bytes, 0700); err != nil {
				t.Fatal(err)
			}
			stage := bytes
			if !success {
				stage = []byte("corrupt fixture")
			}
			staged := target + ".next"
			if runtime.GOOS == "darwin" {
				staged = filepath.Join(staged, "Contents", "MacOS", "mistral-gui")
				if err := os.MkdirAll(filepath.Dir(staged), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(staged, stage, 0700); err != nil {
				t.Fatal(err)
			}
			l, e := net.Listen("tcp", "127.0.0.1:0")
			if e != nil {
				t.Fatal(e)
			}
			addr := l.Addr().String()
			_ = l.Close()
			data := filepath.Join(dir, "data")
			t.Cleanup(func() {
				b, _ := os.ReadFile(filepath.Join(data, "state.json"))
				var s struct{ Config struct{ Token string } }
				_ = json.Unmarshal(b, &s)
				req, _ := http.NewRequest("POST", "http://"+addr+"/api/v1/manager/quit", nil)
				req.Header.Set("Authorization", "Bearer "+s.Config.Token)
				client := http.Client{Timeout: time.Second}
				if resp, e := client.Do(req); e == nil {
					resp.Body.Close()
				}
				for i := 0; i < 50; i++ {
					if e := os.Remove(executable); e == nil {
						return
					}
					time.Sleep(100 * time.Millisecond)
				}
			})
			stateFile := filepath.Join(dir, "update-status.json")
			e = Apply(ApplyRequest{Target: target, Staged: target + ".next", Executable: executable, Args: []string{"--addr", addr, "--data-dir", data, "--no-open"}, HealthURL: "http://" + addr + "/healthz", Version: "v9.0.0", StateFile: stateFile})
			if success && e != nil || !success && e == nil {
				t.Fatalf("unexpected apply result: %v", e)
			}
			deadline := time.Now().Add(10 * time.Second)
			for {
				resp, err := http.Get("http://" + addr + "/healthz")
				if err == nil {
					b, _ := io.ReadAll(resp.Body)
					resp.Body.Close()
					if strings.Contains(string(b), "v9.0.0") {
						break
					}
				}
				if time.Now().After(deadline) {
					t.Fatal("restored manager did not start")
				}
				time.Sleep(100 * time.Millisecond)
			}
			b, _ := os.ReadFile(stateFile)
			want := "complete"
			if !success {
				want = "rolled_back"
			}
			if !strings.Contains(string(b), want) {
				t.Fatalf("missing state: %s", b)
			}
		})
	}
}
