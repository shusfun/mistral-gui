package update

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os/exec"
	"syscall"
)

func hideWindow(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
func atomicReplaceTarget(staged, target string) error {
	from, err := windows.UTF16PtrFromString(staged)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
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
