package update

import (
	"fmt"
	"os/exec"
	"syscall"
)

func hideWindow(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
func waitParent(pid int) error {
	h, e := syscall.OpenProcess(0x00100000, false, uint32(pid))
	if e != nil {
		return nil
	}
	defer syscall.CloseHandle(h)
	event, e := syscall.WaitForSingleObject(h, 30000)
	if e != nil {
		return e
	}
	if event != 0 {
		return fmt.Errorf("old_manager_exit_timeout")
	}
	return nil
}
