//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package tools

import (
	"os"
	"os/exec"
)

func configureProcessTree(*exec.Cmd) {}

func terminateProcessTree(process *os.Process) {
	if process != nil {
		_ = process.Kill()
	}
}

func cleanupProcessTree(process *os.Process) { terminateProcessTree(process) }
