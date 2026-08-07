package app

import (
	"fmt"
	"testing"

	"denova/config"
)

func TestTerminalConfigFromAppConfigMapsWholeCommandRegistry(t *testing.T) {
	commands := make([]config.TerminalCommandSettings, 48)
	for index := range commands {
		commands[index] = config.TerminalCommandSettings{
			ID:      fmt.Sprintf("cli-%d", index),
			Name:    fmt.Sprintf("CLI %d", index),
			Command: fmt.Sprintf("cli-%d --interactive", index),
			Enabled: index%2 == 0,
		}
	}

	resolved := terminalConfigFromAppConfig(&config.Config{
		TerminalEnabled:  true,
		TerminalCommands: commands,
	})
	if len(resolved.Commands) != len(commands) {
		t.Fatalf("terminal registry was truncated: got=%d want=%d", len(resolved.Commands), len(commands))
	}
	for index, command := range resolved.Commands {
		want := commands[index]
		if command.ID != want.ID || command.Name != want.Name || command.Command != want.Command || command.Enabled != want.Enabled {
			t.Fatalf("command %d = %#v, want %#v", index, command, want)
		}
	}
}

func TestTerminalConfigFromAppConfigPreservesExplicitlyEmptyCommandRegistry(t *testing.T) {
	resolved := terminalConfigFromAppConfig(&config.Config{
		TerminalEnabled:  true,
		TerminalCommands: []config.TerminalCommandSettings{},
	})
	if resolved.Commands == nil || len(resolved.Commands) != 0 {
		t.Fatalf("explicit empty terminal registry restored defaults: %#v", resolved.Commands)
	}
}
