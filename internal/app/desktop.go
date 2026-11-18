package app

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"runtime"
	"time"
)

func (a *App) OpenBrowser() {
	code := a.issueExchangeCode()
	target := "http://" + a.addr + "/manage#exchange=" + code
	if err := openURL(target); err != nil {
		slog.Warn("browser.open_failed", "error", err)
	}
}

func openURL(target string) error {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	case "darwin":
		c = exec.Command("open", target)
	case "linux":
		if _, err := exec.LookPath("xdg-open"); err != nil {
			return errors.New("platform_browser_open_unavailable")
		}
		c = exec.Command("xdg-open", target)
	default:
		return errors.New("platform_unsupported")
	}
	setProcess(c)
	return c.Start()
}

func verifyAndNotifyInstance(client *http.Client, candidateAddr, expectedInstance, token string, quit bool) error {
	healthReq, err := http.NewRequest("GET", "http://"+candidateAddr+"/healthz", nil)
	if err != nil {
		return err
	}
	hResp, err := client.Do(healthReq)
	if err != nil {
		return err
	}
	var health struct {
		Service  string `json:"service"`
		Instance string `json:"instance"`
	}
	decodeErr := json.NewDecoder(io.LimitReader(hResp.Body, 4096)).Decode(&health)
	_ = hResp.Body.Close()
	// Verify instance identity BEFORE sending the Bearer token (M-S12).
	if decodeErr != nil || health.Service != "mistral-gui" || health.Instance == "" {
		return errors.New("manager_identity_mismatch")
	}
	if expectedInstance != "" && health.Instance != expectedInstance {
		return errors.New("manager_identity_mismatch")
	}

	path := "/api/v1/manager/show"
	if quit {
		path = "/api/v1/manager/quit"
	}
	req, err := http.NewRequest("POST", "http://"+candidateAddr+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 202 {
		return errors.New("manager_connection_failed")
	}
	return nil
}

// ConnectExisting verifies /healthz instance identity before sending credentials,
// and discovers non-default ports via instance.json (M-S12, M-S20, M-S28).
func ConnectExisting(addr, dir string, quit bool) error {
	resolvedDir, err := resolveDataDir(dir)
	if err != nil {
		return err
	}
	persisted, err := loadPersistedFile(resolvedDir + "/state.json")
	if err != nil {
		// Fall back to .bak if primary state is mid-recovery
		persisted, err = loadPersistedFile(resolvedDir + "/state.json.bak")
		if err != nil {
			return err
		}
	}
	if persisted.Config.Token == "" {
		return errors.New("invalid_token")
	}

	rec, recErr := readInstanceRecord(resolvedDir)
	expectedInstance := ""
	if recErr == nil {
		expectedInstance = rec.Instance
	}

	candidates := []string{}
	if recErr == nil && rec.Addr != "" {
		candidates = append(candidates, rec.Addr)
	}
	if addr != "" && (len(candidates) == 0 || candidates[0] != addr) {
		candidates = append(candidates, addr)
	}

	c := &http.Client{Timeout: 3 * time.Second}
	var lastErr error
	for _, cand := range candidates {
		// When falling back to user-supplied addr without matching rec.Addr, still require expectedInstance if known.
		if e := verifyAndNotifyInstance(c, cand, expectedInstance, persisted.Config.Token, quit); e == nil {
			return nil
		} else {
			lastErr = e
		}
	}
	if lastErr != nil {
		return lastErr
	}
	return errors.New("manager_connection_failed")
}

func (a *App) SetLanguage(l string) error {
	if l != "zh-CN" && l != "en-US" {
		return errors.New("invalid_language")
	}
	a.mu.Lock()
	a.config.Language = l
	a.mu.Unlock()
	return a.save()
}
