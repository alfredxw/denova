//go:build !windows

package grok

import "os/exec"

func configureProcess(cmd *exec.Cmd) {}
