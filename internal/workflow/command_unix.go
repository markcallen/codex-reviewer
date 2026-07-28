//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly || solaris

package workflow

import (
	"os/exec"
	"syscall"
	"time"
)

func configureShellCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		pgid := -cmd.Process.Pid
		if err := syscall.Kill(pgid, syscall.SIGINT); err != nil && err != syscall.ESRCH {
			return err
		}
		go func() {
			time.Sleep(2 * time.Second)
			_ = syscall.Kill(pgid, syscall.SIGKILL)
		}()
		return nil
	}
	cmd.WaitDelay = 3 * time.Second
}
