//go:build !windows

package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func setLaunchAtLogin(enabled bool) error {
	if runtime.GOOS != "darwin" {
		if !enabled {
			return nil
		}
		return errors.New("autostart_failed")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return errors.New("autostart_failed")
	}
	dir := filepath.Join(home, "Library", "LaunchAgents")
	plistPath := filepath.Join(dir, "com.shusfun.mistral-gui.plist")
	if !enabled {
		if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
			return errors.New("autostart_failed")
		}
		return nil
	}
	exePath, err := os.Executable()
	if err != nil || exePath == "" {
		return errors.New("autostart_failed")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.New("autostart_failed")
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.shusfun.mistral-gui</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>--tray</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
</dict>
</plist>
`, exePath)
	if err := os.WriteFile(plistPath, []byte(plist), 0644); err != nil {
		return errors.New("autostart_failed")
	}
	return nil
}
