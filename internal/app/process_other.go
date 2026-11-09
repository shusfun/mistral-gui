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
	vm, e := exec.Command("/usr/bin/vm_stat").Output()
	if e != nil {
		return n, 0
	}
	page := uint64(4096)
	var free uint64
	for _, line := range strings.Split(string(vm), "\n") {
		if strings.Contains(line, "page size of ") {
			fields := strings.Fields(line)
			for i, f := range fields {
				if f == "of" && i+1 < len(fields) {
					page, _ = strconv.ParseUint(fields[i+1], 10, 64)
				}
			}
		}
		for _, key := range []string{"Pages free:", "Pages inactive:", "Pages speculative:"} {
			if strings.HasPrefix(line, key) {
				v, _ := strconv.ParseUint(strings.Trim(strings.TrimPrefix(line, key), " .\t\r"), 10, 64)
				free += v
			}
		}
	}
	return n, free * page
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
