package handlers

import (
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"denova/internal/terminal"
)

type blockedTerminalSocket struct {
	closed       chan struct{}
	writeStarted chan struct{}
	closeOnce    sync.Once
	writeOnce    sync.Once
}

func newBlockedTerminalSocket() *blockedTerminalSocket {
	return &blockedTerminalSocket{
		closed:       make(chan struct{}),
		writeStarted: make(chan struct{}),
	}
}

func (c *blockedTerminalSocket) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func (c *blockedTerminalSocket) ReadMessage() (int, []byte, error) {
	<-c.closed
	return 0, nil, errors.New("socket closed")
}

func (c *blockedTerminalSocket) SetWriteDeadline(time.Time) error { return nil }

func (c *blockedTerminalSocket) WriteMessage(int, []byte) error {
	c.writeOnce.Do(func() { close(c.writeStarted) })
	<-c.closed
	return errors.New("socket closed")
}

func TestServeTerminalSocketDisconnectsLaggingSubscriberAndKeepsScrollback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("terminal handler test targets unix pty")
	}
	manager := terminal.NewManager(terminal.Config{
		Enabled:         true,
		MaxSessions:     1,
		ScrollbackBytes: 64 * 1024,
	})
	t.Cleanup(manager.CloseAll)
	session, err := manager.Create(terminal.Spec{
		Command: "/bin/sh",
		Args: []string{
			"-c",
			"read ready; head -c 8388608 /dev/zero; sleep 5",
		},
		Cwd: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("create terminal session: %v", err)
	}

	conn := newBlockedTerminalSocket()
	served := make(chan struct{})
	go func() {
		serveTerminalSocketWithSubscriberQueue(session, conn, 1)
		close(served)
	}()

	select {
	case <-conn.writeStarted:
	case <-time.After(time.Second):
		t.Fatal("terminal socket writer did not start")
	}
	if err := session.Write([]byte("ready\n")); err != nil {
		t.Fatalf("release terminal output: %v", err)
	}

	select {
	case <-conn.closed:
	case <-time.After(time.Second):
		_ = conn.Close()
		t.Fatal("lagging terminal subscriber did not close its WebSocket")
	}
	select {
	case <-served:
	case <-time.After(time.Second):
		t.Fatal("terminal socket relay did not stop after disconnect")
	}

	history, _, detach := session.Attach(1)
	defer detach()
	if len(history) == 0 {
		t.Fatal("expected a re-attached client to receive retained scrollback")
	}
}
