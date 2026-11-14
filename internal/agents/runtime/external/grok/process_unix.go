//go:build !windows

package grok

import "os/exec"

func configureProcess(cmd *exec.Cmd) {}

// verifyCommandLine is a no-op outside Windows: other platforms size their
// argument list per-argument rather than as one bounded command line.
func verifyCommandLine(string, []string) error { return nil }