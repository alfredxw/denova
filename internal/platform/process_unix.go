//go:build !windows

package platform

import (
	"os"
	"os/exec"
	"syscall"
)

func configureBackendProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
func ownBackendProcess(process *os.Process) (func(), error) {
	return func() {
		_ = syscall.Kill(-process.Pid, syscall.SIGKILL)
	}, nil
}
