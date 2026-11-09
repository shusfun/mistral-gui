//go:build !windows

package desktop

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func detach(c *exec.Cmd)        { c.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
func processAlive(pid int) bool { return syscall.Kill(pid, 0) == nil }
func processIsTarget(pid int, target string) bool {
	b, e := exec.Command("/bin/ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	return e == nil && strings.TrimSpace(string(b)) == target
}
