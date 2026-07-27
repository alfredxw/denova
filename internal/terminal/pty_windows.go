//go:build windows

package terminal

import "github.com/charmbracelet/x/xpty"

// preparePTYAfterStart is a no-op on Windows (ConPTY).
func preparePTYAfterStart(xpty.Pty) {}

// finishPTYAfterWait closes the ConPTY, otherwise the output pump never sees the read end.
func finishPTYAfterWait(terminal xpty.Pty) { _ = terminal.Close() }
