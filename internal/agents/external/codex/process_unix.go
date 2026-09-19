//go:build !windows

package codex

import "os/exec"

func configureProcess(cmd *exec.Cmd) {}
