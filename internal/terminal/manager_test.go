package terminal

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("terminal manager tests target unix pty")
	}
	manager := NewManager(Config{Enabled: true, MaxSessions: 2, ScrollbackBytes: 4096})
	t.Cleanup(manager.CloseAll)
	return manager
}

// collect reads output until the session exits or the deadline passes, returning what it saw.
func collect(t *testing.T, session *Session, out <-chan []byte, want string) string {
	t.Helper()
	deadline := time.After(2 * time.Second)
	var builder strings.Builder
	for {
		select {
		case chunk, ok := <-out:
			if !ok {
				return builder.String()
			}
			builder.Write(chunk)
			if want != "" && strings.Contains(builder.String(), want) {
				return builder.String()
			}
		case <-deadline:
			t.Fatalf("timed out waiting for terminal output, got %q", builder.String())
			return builder.String()
		}
	}
}

func TestManagerCreateRunsCommandAndStreamsOutput(t *testing.T) {
	manager := newTestManager(t)
	session, err := manager.Create(Spec{Command: "/bin/sh", Args: []string{"-c", "echo denova-terminal-ok"}, Cwd: t.TempDir()})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	_, out, detach := session.Attach(16)
	defer detach()

	got := collect(t, session, out, "denova-terminal-ok")
	if !strings.Contains(got, "denova-terminal-ok") {
		t.Fatalf("expected command output, got %q", got)
	}

	select {
	case <-session.Exited():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for process exit")
	}
	if code, message := session.ExitStatus(); code != 0 || message != "" {
		t.Fatalf("unexpected exit status code=%d err=%q", code, message)
	}
}

func TestAttachedSessionClosesOutputAfterFinalProcessBytes(t *testing.T) {
	manager := newTestManager(t)
	session, err := manager.Create(Spec{
		Command: "/bin/sh",
		Args:    []string{"-c", "printf final-output"},
		Cwd:     t.TempDir(),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	_, out, detach := session.Attach(16)
	defer detach()

	type outputResult struct {
		text string
	}
	result := make(chan outputResult, 1)
	go func() {
		var builder strings.Builder
		for chunk := range out {
			builder.Write(chunk)
		}
		result <- outputResult{text: builder.String()}
	}()

	select {
	case got := <-result:
		if !strings.Contains(got.text, "final-output") {
			t.Fatalf("final output was lost before subscription close: %q", got.text)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("attached output subscription stayed open after process exit")
	}
	select {
	case <-session.Exited():
	default:
		t.Fatal("output subscription closed before process exit became observable")
	}
}

func TestCLIStartupReturnsToWorkspaceShell(t *testing.T) {
	manager := NewManager(Config{
		Enabled: true,
		Shell:   "/bin/sh",
		Commands: []CommandProfile{
			{ID: "claude", Name: "Claude Code", Command: "/bin/sh -c 'printf cli-finished'", Enabled: true},
		},
		MaxSessions:     1,
		ScrollbackBytes: 4096,
	})
	t.Cleanup(manager.CloseAll)
	startupCommand, err := manager.ResolveStartupCommand("claude")
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	session, err := manager.Create(Spec{
		ProfileID:      "claude",
		StartupCommand: startupCommand,
		Cwd:            workspace,
	})
	if err != nil {
		t.Fatalf("create Claude profile session: %v", err)
	}
	_, out, detach := session.Attach(16)
	defer detach()

	if got := collect(t, session, out, "cli-finished"); !strings.Contains(got, "cli-finished") {
		t.Fatalf("startup command output = %q", got)
	}
	select {
	case <-session.Exited():
		t.Fatal("leaving the startup CLI exited the workspace shell")
	default:
	}
	if err := session.Write([]byte("pwd\r")); err != nil {
		t.Fatalf("write command after CLI exit: %v", err)
	}
	if got := collect(t, session, out, workspace); !strings.Contains(got, workspace) {
		t.Fatalf("CLI did not return to the workspace shell: output=%q workspace=%q", got, workspace)
	}
}

func TestSessionAttachReplaysScrollback(t *testing.T) {
	manager := newTestManager(t)
	session, err := manager.Create(Spec{Command: "/bin/sh", Args: []string{"-c", "echo replay-marker"}, Cwd: t.TempDir()})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	_, out, detach := session.Attach(16)
	collect(t, session, out, "replay-marker")
	detach()

	select {
	case <-session.Exited():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for process exit")
	}

	// The process already exited: re-attaching must yield the history without blocking.
	history, replayOut, replayDetach := session.Attach(16)
	defer replayDetach()
	if !strings.Contains(string(history), "replay-marker") {
		t.Fatalf("expected scrollback to contain earlier output, got %q", string(history))
	}
	select {
	case _, ok := <-replayOut:
		if ok {
			t.Fatal("expected closed output channel for exited session")
		}
	case <-time.After(time.Second):
		t.Fatal("expected exited session to close the output channel immediately")
	}
}

func TestSessionWriteFeedsStdin(t *testing.T) {
	manager := newTestManager(t)
	session, err := manager.Create(Spec{Command: "/bin/sh", Args: []string{"-c", "read line; echo got:$line"}, Cwd: t.TempDir()})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	_, out, detach := session.Attach(16)
	defer detach()

	if err := session.Write([]byte("ping\n")); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	got := collect(t, session, out, "got:ping")
	if !strings.Contains(got, "got:ping") {
		t.Fatalf("expected stdin echo, got %q", got)
	}
}

func TestManagerRejectsWhenDisabledOrOverLimit(t *testing.T) {
	manager := newTestManager(t)
	first, err := manager.Create(Spec{Command: "/bin/sh", Args: []string{"-c", "sleep 5"}, Cwd: t.TempDir()})
	if err != nil {
		t.Fatalf("create first session: %v", err)
	}
	second, err := manager.Create(Spec{Command: "/bin/sh", Args: []string{"-c", "sleep 5"}, Cwd: t.TempDir()})
	if err != nil {
		t.Fatalf("create second session: %v", err)
	}
	if _, err := manager.Create(Spec{Command: "/bin/sh", Args: []string{"-c", "sleep 5"}, Cwd: t.TempDir()}); err == nil {
		t.Fatal("expected the third session to hit the concurrency limit")
	}
	if got := len(manager.List()); got != 2 {
		t.Fatalf("expected 2 live sessions, got %d", got)
	}
	if err := manager.Close(first.ID()); err != nil {
		t.Fatalf("close first session: %v", err)
	}
	if _, err := manager.Get(first.ID()); err == nil {
		t.Fatal("expected closed session to be removed from the registry")
	}
	_ = second

	manager.SetConfig(Config{Enabled: false})
	if _, err := manager.Create(Spec{Command: "/bin/sh"}); err == nil {
		t.Fatal("expected disabled terminal to reject session creation")
	}
	if got := len(manager.List()); got != 0 {
		t.Fatalf("expected disabling terminal to close live sessions, got %d", got)
	}
}

func TestManagerCreateIsIdempotentForOneOwnerTab(t *testing.T) {
	manager := newTestManager(t)
	spec := Spec{
		OwnerTabID: "terminal-tab-1",
		Command:    "/bin/sh",
		Args:       []string{"-c", "sleep 5"},
		Cwd:        t.TempDir(),
	}
	type result struct {
		session *Session
		err     error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	for range 2 {
		go func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					results <- result{err: fmt.Errorf("create panic: %v", recovered)}
				}
			}()
			<-start
			session, err := manager.Create(spec)
			results <- result{session: session, err: err}
		}()
	}
	close(start)
	first := <-results
	second := <-results
	if first.err != nil || second.err != nil {
		t.Fatalf("concurrent create failed: first=%v second=%v", first.err, second.err)
	}
	if first.session.ID() != second.session.ID() {
		t.Fatalf("expected one session for one owner tab, got %s and %s", first.session.ID(), second.session.ID())
	}
	if got := len(manager.List()); got != 1 {
		t.Fatalf("expected one registered session, got %d", got)
	}
	if got := first.session.Info().OwnerTabID; got != spec.OwnerTabID {
		t.Fatalf("expected owner tab %q, got %q", spec.OwnerTabID, got)
	}
	if _, err := manager.Create(Spec{
		OwnerTabID: spec.OwnerTabID, ProjectID: "foreign-project",
		Command: "/bin/sh", Args: []string{"-c", "exit 0"}, Cwd: t.TempDir(),
	}); !errors.Is(err, ErrOwnerConflict) {
		t.Fatalf("cross-Project owner reuse error = %v, want ErrOwnerConflict", err)
	}

	oldID := first.session.ID()
	if err := manager.Close(oldID); err != nil {
		t.Fatalf("close first session: %v", err)
	}
	replacement, err := manager.Create(spec)
	if err != nil {
		t.Fatalf("create replacement: %v", err)
	}
	if replacement.ID() == oldID {
		t.Fatal("expected closing an owner session to allow a fresh replacement")
	}
}

func TestManagerCloseProjectRejectsRunningProcessesAndCleansExitedSessions(t *testing.T) {
	manager := newTestManager(t)
	live, err := manager.Create(Spec{
		ProjectID: "project-live", Command: "/bin/sh", Args: []string{"-c", "read line"}, Cwd: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	exited, err := manager.Create(Spec{
		ProjectID: "project-exited", Command: "/bin/sh", Args: []string{"-c", "exit 0"}, Cwd: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-exited.Exited():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for terminal exit")
	}

	if err := manager.CloseProject("project-live"); !errors.Is(err, ErrProjectSessionsActive) {
		t.Fatalf("CloseProject error = %v, want ErrProjectSessionsActive", err)
	}
	if _, err := manager.Get(live.ID()); err != nil {
		t.Fatalf("running Project session was removed: %v", err)
	}
	if err := manager.CloseProject("project-exited"); err != nil {
		t.Fatalf("clean exited Project sessions: %v", err)
	}
	if _, err := manager.Get(exited.ID()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("exited Project session remains registered: %v", err)
	}
}

func TestManagerResolveStartupCommand(t *testing.T) {
	manager := NewManager(Config{
		Enabled: true,
		Commands: []CommandProfile{
			{ID: "codex", Name: "Codex CLI", Command: `npx @openai/codex --profile "deep work"`, Enabled: true},
			{ID: "claude", Name: "Claude Code", Command: `"/Applications/Claude Code/claude" --resume`, Enabled: true},
			{ID: "aider", Name: "Aider", Command: "aider --model sonnet", Enabled: true},
			{ID: "disabled", Name: "Disabled", Command: "disabled", Enabled: false},
		},
		MaxSessions:     2,
		ScrollbackBytes: 4096,
	})

	startupCommand, err := manager.ResolveStartupCommand("codex")
	if err != nil {
		t.Fatal(err)
	}
	if startupCommand != `npx @openai/codex --profile "deep work"` {
		t.Fatalf("unexpected Codex startup command: %q", startupCommand)
	}

	startupCommand, err = manager.ResolveStartupCommand("claude")
	if err != nil {
		t.Fatal(err)
	}
	if startupCommand != `"/Applications/Claude Code/claude" --resume` {
		t.Fatalf("unexpected Claude startup command: %q", startupCommand)
	}

	startupCommand, err = manager.ResolveStartupCommand("aider")
	if err != nil || startupCommand != "aider --model sonnet" {
		t.Fatalf("configured profile should resolve by backend ID: command=%q err=%v", startupCommand, err)
	}
	if _, err = manager.ResolveStartupCommand("disabled"); !errors.Is(err, ErrInvalidProfile) {
		t.Fatalf("disabled profile should fail with ErrInvalidProfile, got %v", err)
	}

	startupCommand, err = manager.ResolveStartupCommand("shell")
	if err != nil || startupCommand != "" {
		t.Fatalf("shell should defer to the configured login shell: command=%q err=%v", startupCommand, err)
	}

	if _, err = manager.ResolveStartupCommand("unknown"); !errors.Is(err, ErrInvalidProfile) {
		t.Fatalf("unknown profile should fail with ErrInvalidProfile, got %v", err)
	}

	manager.SetConfig(Config{
		Enabled: true,
		Commands: []CommandProfile{
			{ID: "codex", Name: "Codex CLI", Command: `"unterminated`, Enabled: true},
		},
		MaxSessions: 2, ScrollbackBytes: 4096,
	})
	if _, err = manager.ResolveStartupCommand("codex"); !errors.Is(err, ErrInvalidLaunchCommand) {
		t.Fatalf("malformed command should fail with ErrInvalidLaunchCommand, got %v", err)
	}
}

func TestAvailableCommandsReturnsOnlySafeMenuMetadataInConfiguredOrder(t *testing.T) {
	manager := NewManager(Config{
		Enabled: true,
		Commands: []CommandProfile{
			{ID: "aider", Name: "Aider", Command: "aider --model sonnet", Enabled: true},
			{ID: "disabled", Name: "Disabled", Command: "disabled", Enabled: false},
			{ID: "aider", Name: "Duplicate", Command: "duplicate", Enabled: true},
			{ID: "goose", Name: "Goose", Command: "goose", Enabled: true},
		},
	})

	commands := manager.AvailableCommands()
	if len(commands) != 2 || commands[0].ID != "aider" || commands[1].ID != "goose" {
		t.Fatalf("available commands = %#v", commands)
	}
	if commands[0].Command != "" || commands[0].Enabled {
		t.Fatalf("API metadata leaked runtime command fields: %#v", commands[0])
	}
}

func TestScrollbackKeepsMostRecentBytes(t *testing.T) {
	history := newScrollback(8)
	history.append([]byte("abcdef"))
	history.append([]byte("ghij"))
	if got := string(history.snapshot()); got != "cdefghij" {
		t.Fatalf("expected the most recent 8 bytes, got %q", got)
	}
	history.append([]byte("0123456789"))
	if got := string(history.snapshot()); got != "23456789" {
		t.Fatalf("expected oversized chunk to be tail-truncated, got %q", got)
	}
}

func TestNormalizeSizeClampsToUsableRange(t *testing.T) {
	cases := []struct {
		cols, rows       int
		wantCol, wantRow int
	}{
		{0, 0, defaultCols, defaultRows},
		{-5, -5, defaultCols, defaultRows},
		{1, 0, minCols, defaultRows},
		{5000, 5000, maxCols, maxRows},
		{120, 40, 120, 40},
	}
	for _, tc := range cases {
		cols, rows := normalizeSize(tc.cols, tc.rows)
		if cols != tc.wantCol || rows != tc.wantRow {
			t.Fatalf("normalizeSize(%d,%d) = (%d,%d), want (%d,%d)", tc.cols, tc.rows, cols, rows, tc.wantCol, tc.wantRow)
		}
	}
}
