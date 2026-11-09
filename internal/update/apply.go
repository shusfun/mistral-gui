package update

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type ApplyRequest struct {
	ParentPID        int      `json:"parent_pid"`
	Target           string   `json:"target"`
	Staged           string   `json:"staged"`
	Executable       string   `json:"executable"`
	Args             []string `json:"args"`
	HealthURL        string   `json:"health_url"`
	Version          string   `json:"version"`
	PreviousInstance string   `json:"previous_instance"`
	StateFile        string   `json:"state_file"`
	Traceparent      string   `json:"traceparent,omitempty"`
}

// Apply executes from a copied helper outside the install tree.
// The caller has already authenticated and verified the signed release.
func Apply(r ApplyRequest) error {
	if err := validateRequest(r); err != nil {
		return err
	}
	if r.Traceparent == "" {
		var id [24]byte
		if _, err := rand.Read(id[:]); err != nil {
			return err
		}
		r.Traceparent = "00-" + hex.EncodeToString(id[:16]) + "-" + hex.EncodeToString(id[16:]) + "-01"
	}
	if r.ParentPID > 0 {
		if err := waitParent(r.ParentPID); err != nil {
			return err
		}
	}
	result := func(state string, e error) {
		detail := ""
		if e != nil {
			detail = e.Error()
		}
		b, _ := json.Marshal(map[string]any{"state": state, "error": detail, "version": r.Version, "traceparent": r.Traceparent, "trace_id": r.Traceparent[3:35]})
		_ = os.WriteFile(r.StateFile, b, 0600)
	}
	result("installing", nil)
	backup := r.Target + ".previous"
	if _, e := os.Stat(backup); e == nil {
		if e = os.RemoveAll(backup); e != nil {
			return e
		}
	}
	var err error
	for i := 0; i < 120; i++ {
		err = os.Rename(r.Target, backup)
		if err == nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if err != nil {
		result("failed", err)
		return err
	}
	rollback := func(cause error) error {
		_ = os.RemoveAll(r.Target)
		err := os.Rename(backup, r.Target)
		if err != nil {
			result("rollback_failed", err)
			return err
		}
		cmd := exec.Command(r.Executable, r.Args...)
		hideWindow(cmd)
		_ = cmd.Start()
		result("rolled_back", cause)
		return cause
	}
	if err = os.Rename(r.Staged, r.Target); err != nil {
		return rollback(err)
	}
	cmd := exec.Command(r.Executable, r.Args...)
	hideWindow(cmd)
	if err = cmd.Start(); err != nil {
		return rollback(err)
	}
	client := http.Client{Timeout: time.Second}
	for i := 0; i < 60; i++ {
		req, e := http.NewRequest("GET", r.HealthURL, nil)
		if e != nil {
			return rollback(e)
		}
		req.Header.Set("traceparent", r.Traceparent)
		resp, e := client.Do(req)
		if e == nil {
			var h struct {
				Version  string `json:"version"`
				Instance string `json:"instance"`
				OK       bool   `json:"ok"`
				Service  string `json:"service"`
			}
			e = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&h)
			resp.Body.Close()
			if resp.StatusCode == 200 && e == nil && h.OK && h.Service == "mistral-gui" && h.Version == r.Version && h.Instance != "" && h.Instance != r.PreviousInstance {
				result("complete", nil)
				return nil
			}
		}
		time.Sleep(time.Second)
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	return rollback(errors.New("update_health_check_failed"))
}
func validateRequest(r ApplyRequest) error {
	if r.Traceparent != "" && !validApplyTrace(r.Traceparent) {
		return errors.New("invalid_update_trace")
	}
	u, e := url.Parse(r.HealthURL)
	if e != nil || u.Scheme != "http" || u.Path != "/healthz" || u.User != nil || u.RawQuery != "" || net.ParseIP(u.Hostname()) == nil || !net.ParseIP(u.Hostname()).IsLoopback() {
		return errors.New("invalid_health_url")
	}
	base := filepath.Base(r.Target)
	if !filepath.IsAbs(r.Target) || r.Staged != r.Target+".next" || (base != "mistral-gui.exe" && base != "mistral-gui.app") {
		return errors.New("invalid_update_target")
	}
	exe, err := filepath.Rel(r.Target, r.Executable)
	if base == "mistral-gui.exe" && r.Executable != r.Target || base == "mistral-gui.app" && (err != nil || strings.HasPrefix(exe, "..") || exe != "Contents"+string(filepath.Separator)+"MacOS"+string(filepath.Separator)+"mistral-gui") {
		return errors.New("invalid_update_executable")
	}
	for _, path := range []string{r.Target, r.Staged} {
		if info, e := os.Lstat(path); e == nil && info.Mode()&os.ModeSymlink != 0 {
			return errors.New("update_link_rejected")
		}
	}
	return nil
}

func validApplyTrace(trace string) bool {
	if len(trace) != 55 || trace[:3] != "00-" || trace[35] != '-' || trace[52] != '-' {
		return false
	}
	for _, part := range []string{trace[3:35], trace[36:52], trace[53:]} {
		if _, err := hex.DecodeString(part); err != nil || part != strings.ToLower(part) {
			return false
		}
	}
	return trace[3:35] != strings.Repeat("0", 32) && trace[36:52] != strings.Repeat("0", 16)
}
