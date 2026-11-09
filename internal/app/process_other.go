//go:build !windows

package app

import (
	"io"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func setProcess(c *exec.Cmd)       { c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
func nvidiaSMI() string            { return "nvidia-smi" }
func terminateProcess(c *exec.Cmd) { _ = syscall.Kill(-c.Process.Pid, syscall.SIGTERM) }
func vcRuntimeAvailable() bool     { return true }
func memoryInfo() (uint64, uint64) {
	b, e := exec.Command("/usr/sbin/sysctl", "-n", "hw.memsize").Output()
	if e != nil {
		return 0, 0
	}
	n, _ := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
	return n, 0
}
func ownProcess(c *exec.Cmd) (func(), error) {
	// The pipe closes even after an abrupt manager exit. The OS shell is present on macOS.
	watch := exec.Command("/bin/sh", "-c", `if ! read -r completed; then kill -TERM -"$1"; fi`, "mistral-gui-watch", strconv.Itoa(c.Process.Pid))
	input, e := watch.StdinPipe()
	if e != nil {
		return nil, e
	}
	if e = watch.Start(); e != nil {
		input.Close()
		return nil, e
	}
	go func() { _ = watch.Wait() }()
	return func() { _, _ = io.WriteString(input, "done\n"); _ = input.Close() }, nil
}
