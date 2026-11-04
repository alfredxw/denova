//go:build !windows

package desktop

import "os/exec"

func hideChildWindow(*exec.Cmd) {}
