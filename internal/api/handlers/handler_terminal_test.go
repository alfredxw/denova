package handlers

import (
	"encoding/json"
	"errors"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"denova/internal/terminal"
	"github.com/hertz-contrib/websocket"
)

type blockedTerminalSocket struct {
	closed       chan struct{}
	writeStarted chan struct{}
	closeOnce    sync.Once
	writeOnce    sync.Once
}

type recordedTerminalFrame struct {
	messageType int
	payload     []byte
}

type recordingTerminalSocket struct {
	closed    chan struct{}
	frames    chan recordedTerminalFrame
	closeOnce sync.Once
}

func newRecordingTerminalSocket() *recordingTerminalSocket {
	return &recordingTerminalSocket{
		closed: make(chan struct{}),
		frames: make(chan recordedTerminalFrame, 32),
	}
}

func (c *recordingTerminalSocket) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func (c *recordingTerminalSocket) ReadMessage() (int, []byte, error) {
	<-c.closed
	return 0, nil, errors.New("socket closed")
}

func (c *recordingTerminalSocket) SetWriteDeadline(time.Time) error { return nil }

func (c *recordingTerminalSocket) WriteMessage(messageType int, payload []byte) error {
	frame := recordedTerminalFrame{messageType: messageType, payload: append([]byte(nil), payload...)}
	select {
	case <-c.closed:
		return errors.New("socket closed")
	case c.frames <- frame:
		return nil
	}
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

func TestServeTerminalSocketSendsExitAfterFinalOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("terminal handler test targets unix pty")
	}
	manager := terminal.NewManager(terminal.Config{
		Enabled:         true,
		MaxSessions:     1,
		ScrollbackBytes: 4096,
	})
	t.Cleanup(manager.CloseAll)
	session, err := manager.Create(terminal.Spec{
		Command: "/bin/sh",
		Args:    []string{"-c", "read ready; printf final-output"},
		Cwd:     t.TempDir(),
	})
	if err != nil {
		t.Fatalf("create terminal session: %v", err)
	}

	conn := newRecordingTerminalSocket()
	served := make(chan struct{})
	go func() {
		serveTerminalSocket(session, conn)
		close(served)
	}()

	deadline := time.After(2 * time.Second)
	for {
		select {
		case frame := <-conn.frames:
			if frame.messageType != websocket.TextMessage {
				continue
			}
			var control map[string]any
			if err := json.Unmarshal(frame.payload, &control); err != nil {
				t.Fatalf("decode ready frame: %v", err)
			}
			if control["type"] == "ready" {
				if err := session.Write([]byte("ready\n")); err != nil {
					t.Fatalf("release terminal process: %v", err)
				}
				goto waitForExit
			}
		case <-deadline:
			_ = conn.Close()
			t.Fatal("terminal socket did not become ready")
		}
	}

waitForExit:
	var output strings.Builder
	deadline = time.After(2 * time.Second)
	for {
		select {
		case frame := <-conn.frames:
			if frame.messageType == websocket.BinaryMessage {
				output.Write(frame.payload)
				continue
			}
			if frame.messageType != websocket.TextMessage {
				continue
			}
			var control map[string]any
			if err := json.Unmarshal(frame.payload, &control); err != nil {
				t.Fatalf("decode exit frame: %v", err)
			}
			if control["type"] != "exit" {
				continue
			}
			if !strings.Contains(output.String(), "final-output") {
				t.Fatalf("exit frame arrived before final output: %q", output.String())
			}
			_ = conn.Close()
			select {
			case <-served:
			case <-time.After(time.Second):
				t.Fatal("terminal socket relay did not stop after client close")
			}
			return
		case <-deadline:
			_ = conn.Close()
			t.Fatal("terminal socket did not send an exit frame")
		}
	}
}
