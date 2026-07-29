//go:build windows

package terminal

import (
	"os/exec"

	"github.com/charmbracelet/x/xpty"
)

// ConPTY owns the Windows process attachment and does not use Unix controlling-terminal flags.
func prepareCommandForPTY(*exec.Cmd) {}

// preparePTYAfterStart is a no-op on Windows (ConPTY).
func preparePTYAfterStart(xpty.Pty) {}

// finishPTYAfterWait closes the ConPTY, otherwise the output pump never sees the read end.
func finishPTYAfterWait(terminal xpty.Pty) { _ = terminal.Close() }
