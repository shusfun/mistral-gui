package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

func (a *App) OpenBrowser() {
	target := "http://" + a.addr + "/manage#token=" + a.token()
	openURL(target)
}
func openURL(target string) {
	var c *exec.Cmd
	if runtime.GOOS == "windows" {
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	} else {
		c = exec.Command("open", target)
	}
	setProcess(c)
	_ = c.Start()
}

// ConnectExisting never mutates an active manager's task state.
func ConnectExisting(addr, dir string, quit bool) error {
	if dir == "" {
		base, e := os.UserConfigDir()
		if e != nil {
			return e
		}
		dir = filepath.Join(base, "mistral-gui")
	}
	b, e := os.ReadFile(filepath.Join(dir, "state.json"))
	if e != nil {
		return e
	}
	var state struct {
		Config Config `json:"config"`
	}
	if e = json.Unmarshal(b, &state); e != nil {
		return e
	}
	c := http.Client{Timeout: 3 * time.Second}
	req, e := http.NewRequest("GET", "http://"+addr+"/api/v1/status", nil)
	if e != nil {
		return e
	}
	if quit {
		req.Method = "POST"
		req.URL.Path = "/api/v1/manager/quit"
	}
	req.Header.Set("Authorization", "Bearer "+state.Config.Token)
	resp, e := c.Do(req)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 202 {
		return errors.New("manager_connection_failed")
	}
	if !quit {
		openURL("http://" + addr + "/manage#token=" + state.Config.Token)
	}
	return nil
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
