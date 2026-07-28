//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly && !solaris

package reviewer

import "os/exec"

func configureReviewCommand(cmd *exec.Cmd) {}
