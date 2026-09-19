//go:build !windows

package claude

import "os/exec"

func configureProcess(cmd *exec.Cmd) {}
