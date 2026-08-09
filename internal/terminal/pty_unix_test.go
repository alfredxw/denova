//go:build !windows

package terminal

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestSessionRootOwnsPTYForegroundProcessGroup(t *testing.T) {
	manager := newTestManager(t)
	session, err := manager.Create(Spec{Command: "/bin/sh", Cwd: t.TempDir()})
	if err != nil {
		t.Fatalf("create interactive shell session: %v", err)
	}

	pid := session.cmd.Process.Pid
	sid, err := unix.Getsid(pid)
	if err != nil {
		t.Fatalf("get shell session id: %v", err)
	}
	if sid != pid {
		t.Fatalf("shell inherited host session: pid=%d sid=%d", pid, sid)
	}
	pgid, err := unix.Getpgid(pid)
	if err != nil {
		t.Fatalf("get shell process group: %v", err)
	}
	foreground, err := unix.IoctlGetInt(int(session.pty.Fd()), unix.TIOCGPGRP)
	if err != nil {
		t.Fatalf("get PTY foreground process group: %v", err)
	}
	if foreground != pgid {
		t.Fatalf("shell does not own PTY foreground: shell_pgid=%d foreground_pgid=%d", pgid, foreground)
	}
}
