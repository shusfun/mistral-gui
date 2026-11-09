//go:build windows

package desktop

import (
	"golang.org/x/sys/windows"
	"os/exec"
	"strings"
	"syscall"
)

func detach(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000 | 0x00000008}
}
func processAlive(pid int) bool {
	p, e := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if e != nil {
		return false
	}
	defer windows.CloseHandle(p)
	event, e := windows.WaitForSingleObject(p, 0)
	return e == nil && event == uint32(windows.WAIT_TIMEOUT)
}
func processIsTarget(pid int, target string) bool {
	p, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if e != nil {
		return false
	}
	defer windows.CloseHandle(p)
	b := make([]uint16, 32768)
	n := uint32(len(b))
	if windows.QueryFullProcessImageName(p, 0, &b[0], &n) != nil {
		return false
	}
	return strings.EqualFold(windows.UTF16ToString(b[:n]), target)
}
