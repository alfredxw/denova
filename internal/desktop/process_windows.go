package desktop

import (
	"os/exec"
	"syscall"
)

func hideChildWindow(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }
