//go:build windows

package restart

import (
	"os"
	"os/exec"
)

// ReplaceProcess starts a replacement process and exits the current process.
func ReplaceProcess(invocation Invocation) error {
	args := []string(nil)
	if len(invocation.Args) > 1 {
		args = invocation.Args[1:]
	}
	cmd := exec.Command(invocation.Executable, args...)
	cmd.Env = invocation.Env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
