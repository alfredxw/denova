//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package tools

import (
	"os"
	"os/exec"
	"syscall"
)

func configureProcessTree(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func terminateProcessTree(process *os.Process) {
	if process != nil {
		_ = syscall.Kill(-process.Pid, syscall.SIGKILL)
	}
}

func cleanupProcessTree(process *os.Process) { terminateProcessTree(process) }
