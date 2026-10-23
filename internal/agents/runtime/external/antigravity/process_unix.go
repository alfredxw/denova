//go:build !windows

package antigravity

import "os/exec"

func configureProcess(cmd *exec.Cmd) {}
