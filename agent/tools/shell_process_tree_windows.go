//go:build windows

package tools

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
)

func configureProcessTree(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

func terminateProcessTree(process *os.Process) {
	if process == nil {
		return
	}
	command := exec.Command("taskkill.exe", "/T", "/F", "/PID", fmt.Sprintf("%d", process.Pid))
	command.Stdout, command.Stderr = io.Discard, io.Discard
	if command.Run() != nil {
		_ = process.Kill()
	}
}

func cleanupProcessTree(process *os.Process) { terminateProcessTree(process) }
