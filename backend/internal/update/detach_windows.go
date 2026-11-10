package update

import (
	"os/exec"
	"syscall"
)

// detach: nový proces bez konzoly a mimo skupiny starého Fiškusa.
func detach(cmd *exec.Cmd) {
	const createNewProcessGroup, detachedProcess = 0x00000200, 0x00000008
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup | detachedProcess}
}
