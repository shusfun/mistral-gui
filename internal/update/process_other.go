//go:build !windows

package update

import (
	"errors"
	"os/exec"
	"syscall"
	"time"
)

func hideWindow(c *exec.Cmd) {}
func waitParent(pid int) error {
	for i := 0; i < 300; i++ {
		if e := syscall.Kill(pid, 0); errors.Is(e, syscall.ESRCH) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("old_manager_exit_timeout")
}
