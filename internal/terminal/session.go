package terminal

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/xpty"
)

// ErrSessionExited reports that the session process already exited and cannot take input.
var ErrSessionExited = errors.New("terminal session exited")

// Spec describes how to launch one terminal session. The command and working directory are
// resolved by the HTTP/manager boundary from configuration and the current workspace; Session
// itself performs no path inference.
type Spec struct {
	// OwnerTabID is the stable frontend tab identity. It makes session creation idempotent and
	// lets the frontend distinguish reloadable sessions from processes whose tab no longer exists.
	OwnerTabID string
	// ProfileID records the resolved launch profile (shell / codex / claude / custom) for display
	// and tab restoration. Manager.ResolveLaunchProfile applies its startup semantics beforehand.
	ProfileID string
	Title     string
	Command   string
	Args      []string
	// StartupCommand is entered once into an interactive shell after it starts. Built-in
	// Codex/Claude profiles use this instead of replacing the shell process, so leaving the CLI
	// returns to the same workspace prompt. It is intentionally omitted from Info and logs.
	StartupCommand string
	Cwd            string
	// Env holds KEY=VALUE pairs appended on top of the current process environment.
	Env  []string
	Cols int
	Rows int
	// Workspace records the workspace bound at creation time so the frontend can group by project.
	Workspace string
}

// Info is the read-only session snapshot used by the list and create responses.
type Info struct {
	ID         string    `json:"id"`
	OwnerTabID string    `json:"owner_tab_id,omitempty"`
	ProfileID  string    `json:"profile_id"`
	Title      string    `json:"title"`
	Command    string    `json:"command"`
	Args       []string  `json:"args"`
	Cwd        string    `json:"cwd"`
	Workspace  string    `json:"workspace"`
	Cols       int       `json:"cols"`
	Rows       int       `json:"rows"`
	CreatedAt  time.Time `json:"created_at"`
	Attached   int       `json:"attached"`
	Exited     bool      `json:"exited"`
	ExitCode   int       `json:"exit_code"`
	ExitError  string    `json:"exit_error,omitempty"`
}

// subscriber is one attached WebSocket client. `out` is a bounded queue: a client that does
// not keep up is treated as lagging and disconnected, then restores the screen from scrollback
// after re-attaching. This keeps the server from buffering without bound.
type subscriber struct {
	out    chan []byte
	closed bool
}

// Session is a long-lived terminal process backed by a pty. Its lifetime is independent of the
// WebSocket: disconnecting only detaches, the process keeps running, and reopening the tab
// re-attaches and restores the recent output.
type Session struct {
	id        string
	token     string
	spec      Spec
	createdAt time.Time

	pty xpty.Pty
	cmd *exec.Cmd

	mu         sync.Mutex
	history    *scrollback
	subs       map[*subscriber]struct{}
	cols       int
	rows       int
	exited     bool
	exitCode   int
	exitErr    string
	closed     bool
	outputDone bool
	exitedCh   chan struct{}
	closeOnce  sync.Once
	onTerminal func(*Session)
}

func newSession(id, token string, spec Spec, scrollbackBytes int, onTerminal func(*Session)) (*Session, error) {
	cols, rows := normalizeSize(spec.Cols, spec.Rows)
	terminalPty, err := xpty.NewPty(cols, rows)
	if err != nil {
		return nil, fmt.Errorf("open pty: %w", err)
	}

	cmd := exec.Command(spec.Command, spec.Args...)
	cmd.Dir = spec.Cwd
	cmd.Env = terminalProcessEnv(os.Environ(), spec.Env)
	prepareCommandForPTY(cmd)

	if err := terminalPty.Start(cmd); err != nil {
		_ = terminalPty.Close()
		return nil, fmt.Errorf("start %q: %w", spec.Command, err)
	}
	preparePTYAfterStart(terminalPty)
	if startupCommand := strings.TrimSpace(spec.StartupCommand); startupCommand != "" {
		if _, err := terminalPty.Write([]byte(startupCommand + "\r")); err != nil {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			_ = cmd.Wait()
			_ = terminalPty.Close()
			return nil, fmt.Errorf("enter startup command: %w", err)
		}
	}

	s := &Session{
		id:         id,
		token:      token,
		spec:       spec,
		createdAt:  time.Now(),
		pty:        terminalPty,
		cmd:        cmd,
		history:    newScrollback(scrollbackBytes),
		subs:       map[*subscriber]struct{}{},
		cols:       cols,
		rows:       rows,
		exitedCh:   make(chan struct{}),
		onTerminal: onTerminal,
	}
	log.Printf("[terminal/session.go] session started id=%s profile=%s command=%q args=%v cwd=%q size=%dx%d",
		s.id, spec.ProfileID, spec.Command, spec.Args, spec.Cwd, cols, rows)

	go s.pumpOutput()
	go s.waitProcess()
	return s, nil
}

// ID returns the session identifier.
func (s *Session) ID() string { return s.id }

// OwnerTabID returns the immutable frontend tab identity that owns this process.
func (s *Session) OwnerTabID() string { return s.spec.OwnerTabID }

// Token returns the attach token. The create endpoint is already authenticated; the token keeps
// the WebSocket attach restricted to its creator even though browsers cannot send an
// Authorization header on WebSocket handshakes.
func (s *Session) Token() string { return s.token }

// Info returns a session snapshot.
func (s *Session) Info() Info {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Info{
		ID:         s.id,
		OwnerTabID: s.spec.OwnerTabID,
		ProfileID:  s.spec.ProfileID,
		Title:      s.spec.Title,
		Command:    s.spec.Command,
		Args:       append([]string(nil), s.spec.Args...),
		Cwd:        s.spec.Cwd,
		Workspace:  s.spec.Workspace,
		Cols:       s.cols,
		Rows:       s.rows,
		CreatedAt:  s.createdAt,
		Attached:   len(s.subs),
		Exited:     s.exited,
		ExitCode:   s.exitCode,
		ExitError:  s.exitErr,
	}
}

// Write forwards user input to the pty.
func (s *Session) Write(data []byte) error {
	s.mu.Lock()
	exited := s.exited || s.closed
	s.mu.Unlock()
	if exited {
		return ErrSessionExited
	}
	if _, err := s.pty.Write(data); err != nil {
		return fmt.Errorf("write pty: %w", err)
	}
	return nil
}

// Resize adjusts the pty window. With several clients attached the last resize wins.
func (s *Session) Resize(cols, rows int) error {
	cols, rows = normalizeSize(cols, rows)
	s.mu.Lock()
	if s.exited || s.closed {
		s.mu.Unlock()
		return ErrSessionExited
	}
	unchanged := s.cols == cols && s.rows == rows
	s.cols, s.rows = cols, rows
	s.mu.Unlock()
	if unchanged {
		return nil
	}
	if err := s.pty.Resize(cols, rows); err != nil {
		return fmt.Errorf("resize pty: %w", err)
	}
	return nil
}

// Attach registers a subscriber and returns the screen history, the output channel and a detach
// function. History and subscription are taken under one lock so a re-attach neither drops
// nor duplicates frames.
func (s *Session) Attach(queueSize int) (history []byte, out <-chan []byte, detach func()) {
	if queueSize <= 0 {
		queueSize = defaultSubscriberQueue
	}
	sub := &subscriber{out: make(chan []byte, queueSize)}
	s.mu.Lock()
	history = s.history.snapshot()
	if s.closed || (s.exited && s.outputDone) {
		s.mu.Unlock()
		close(sub.out)
		return history, sub.out, func() {}
	}
	s.subs[sub] = struct{}{}
	attached := len(s.subs)
	s.mu.Unlock()
	log.Printf("[terminal/session.go] client attached id=%s attached=%d history=%dB", s.id, attached, len(history))
	return history, sub.out, func() { s.detach(sub) }
}

// Exited returns a channel closed once the process exits, so attach loops can notice the end.
func (s *Session) Exited() <-chan struct{} { return s.exitedCh }

// ExitStatus returns the process exit information; only meaningful after Exited is closed.
func (s *Session) ExitStatus() (code int, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exitCode, s.exitErr
}

func (s *Session) detach(sub *subscriber) {
	s.mu.Lock()
	if _, ok := s.subs[sub]; ok {
		delete(s.subs, sub)
		if !sub.closed {
			sub.closed = true
			close(sub.out)
		}
	}
	attached := len(s.subs)
	s.mu.Unlock()
	log.Printf("[terminal/session.go] client detached id=%s attached=%d", s.id, attached)
}

// pumpOutput reads pty output continuously, appends it to scrollback and broadcasts it.
func (s *Session) pumpOutput() {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[terminal/session.go] output pump panic recovered id=%s err=%v", s.id, recovered)
		}
		s.finishOutput()
	}()
	buf := make([]byte, 32*1024)
	for {
		n, err := s.pty.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			s.broadcast(chunk)
		}
		if err != nil {
			// The pty returns EIO/EOF once the child exits; that is a normal wind-down, not an error.
			log.Printf("[terminal/session.go] output pump finished id=%s err=%v", s.id, err)
			return
		}
	}
}

// finishOutput closes attached output streams only after the process exit status is visible.
// This preserves the final PTY bytes and makes a closed subscription an unambiguous terminal
// completion signal rather than racing cmd.Wait.
func (s *Session) finishOutput() {
	s.mu.Lock()
	s.outputDone = true
	var subs []*subscriber
	if s.exited {
		subs = s.takeSubscribersLocked()
	}
	s.mu.Unlock()
	closeSubscribers(subs)
}

func (s *Session) broadcast(chunk []byte) {
	var lagging []*subscriber
	s.mu.Lock()
	s.history.append(chunk)
	for sub := range s.subs {
		if sub.closed {
			continue
		}
		select {
		case sub.out <- chunk:
		default:
			lagging = append(lagging, sub)
		}
	}
	for _, sub := range lagging {
		delete(s.subs, sub)
		sub.closed = true
		close(sub.out)
	}
	s.mu.Unlock()
	for _, sub := range lagging {
		log.Printf("[terminal/session.go] dropped lagging subscriber id=%s queue=%d", s.id, cap(sub.out))
	}
}

// waitProcess waits for the child process and records its exit status.
func (s *Session) waitProcess() {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[terminal/session.go] wait panic recovered id=%s err=%v", s.id, recovered)
		}
	}()
	err := s.cmd.Wait()
	code := 0
	message := ""
	var exitErr *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exitErr):
		code = exitErr.ExitCode()
	default:
		code = -1
		message = err.Error()
	}
	s.mu.Lock()
	s.exited = true
	s.exitCode = code
	s.exitErr = message
	// Attach must never observe exited=true before Exited is closed; the WebSocket relay uses
	// these two signals together to distinguish natural completion from a dropped subscriber.
	close(s.exitedCh)
	var subs []*subscriber
	if s.outputDone {
		subs = s.takeSubscribersLocked()
	}
	s.mu.Unlock()
	closeSubscribers(subs)
	log.Printf("[terminal/session.go] process exited id=%s code=%d err=%q", s.id, code, message)
	finishPTYAfterWait(s.pty)
	if s.onTerminal != nil {
		s.onTerminal(s)
	}
}

// Close terminates the process, releases the pty and drops all subscribers. Safe to call twice.
func (s *Session) Close() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		subs := s.takeSubscribersLocked()
		exited := s.exited
		s.mu.Unlock()

		if !exited && s.cmd.Process != nil {
			if err := s.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				log.Printf("[terminal/session.go] kill process failed id=%s err=%v", s.id, err)
			}
		}
		if err := s.pty.Close(); err != nil {
			log.Printf("[terminal/session.go] close pty failed id=%s err=%v", s.id, err)
		}
		closeSubscribers(subs)
		log.Printf("[terminal/session.go] session closed id=%s", s.id)
	})
}

// takeSubscribersLocked transfers ownership of every subscriber channel to the caller.
// Marking and removing them under the session mutex prevents detach, exit and Close from
// closing the same channel twice.
func (s *Session) takeSubscribersLocked() []*subscriber {
	subs := make([]*subscriber, 0, len(s.subs))
	for sub := range s.subs {
		if sub.closed {
			continue
		}
		sub.closed = true
		subs = append(subs, sub)
	}
	s.subs = map[*subscriber]struct{}{}
	return subs
}

func closeSubscribers(subs []*subscriber) {
	for _, sub := range subs {
		close(sub.out)
	}
}

func normalizeSize(cols, rows int) (int, int) {
	if cols < minCols || cols > maxCols {
		if cols <= 0 {
			cols = defaultCols
		} else if cols < minCols {
			cols = minCols
		} else {
			cols = maxCols
		}
	}
	if rows < minRows || rows > maxRows {
		if rows <= 0 {
			rows = defaultRows
		} else if rows < minRows {
			rows = minRows
		} else {
			rows = maxRows
		}
	}
	return cols, rows
}

// describeCommand builds a command description for logs and tab labels.
func describeCommand(command string, args []string) string {
	if len(args) == 0 {
		return command
	}
	return command + " " + strings.Join(args, " ")
}
