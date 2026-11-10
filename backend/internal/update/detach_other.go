//go:build !windows && !js

package update

import (
	"os/exec"
	"syscall"
)

// detach: nový proces vo vlastnej relácii (neskončí so starým Fiškusom).
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
