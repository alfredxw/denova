//go:build !windows

package terminal

import (
	"os/exec"
	"syscall"

	"github.com/charmbracelet/x/xpty"
)

// prepareCommandForPTY gives the shell its own session and makes the PTY its controlling
// terminal. Merely wiring stdin/stdout to the slave leaves the child attached to Denova's host
// terminal; an interactive program can then stop the shell with SIGTTOU when it hands foreground
// control back on exit, leaving the browser tab connected to a permanently paused process group.
func prepareCommandForPTY(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
	}
}

// preparePTYAfterStart closes the server-side slave handle once the child owns it, so the
// master returns EOF/EIO when the process exits and the output pump can finish cleanly.
func preparePTYAfterStart(terminal xpty.Pty) {
	if unixPTY, ok := terminal.(*xpty.UnixPty); ok {
		_ = unixPTY.Slave().Close()
	}
}

// finishPTYAfterWait is a no-op on Unix; the pty is released by Session.Close.
func finishPTYAfterWait(xpty.Pty) {}
