//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly && !solaris

package workflow

import "os/exec"

func configureShellCommand(cmd *exec.Cmd) {}
