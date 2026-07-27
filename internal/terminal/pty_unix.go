//go:build !windows

package terminal

import "github.com/charmbracelet/x/xpty"

// preparePTYAfterStart closes the server-side slave handle once the child owns it, so the
// master returns EOF/EIO when the process exits and the output pump can finish cleanly.
func preparePTYAfterStart(terminal xpty.Pty) {
	if unixPTY, ok := terminal.(*xpty.UnixPty); ok {
		_ = unixPTY.Slave().Close()
	}
}

// finishPTYAfterWait is a no-op on Unix; the pty is released by Session.Close.
func finishPTYAfterWait(xpty.Pty) {}
