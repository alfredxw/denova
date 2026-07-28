package terminal

import (
	"slices"
	"testing"
)

func TestTerminalProcessEnvOwnsColorCapabilities(t *testing.T) {
	env := terminalProcessEnv(
		[]string{"PATH=/usr/bin", "TERM=dumb", "COLORTERM=", "NO_COLOR=1", "LANG=en_US.UTF-8"},
		[]string{"TERM=xterm-256color", "COLORTERM=truecolor"},
	)

	want := []string{"PATH=/usr/bin", "LANG=en_US.UTF-8", "TERM=xterm-256color", "COLORTERM=truecolor"}
	if !slices.Equal(env, want) {
		t.Fatalf("terminalProcessEnv() = %#v, want %#v", env, want)
	}
}

func TestTerminalProcessEnvCannotReintroduceNoColor(t *testing.T) {
	env := terminalProcessEnv([]string{"PATH=/usr/bin"}, []string{"NO_COLOR=1", "TERM=xterm-256color"})

	if slices.Contains(env, "NO_COLOR=1") {
		t.Fatalf("terminalProcessEnv() leaked NO_COLOR: %#v", env)
	}
}
