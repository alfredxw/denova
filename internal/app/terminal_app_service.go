package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"denova/config"
	"denova/internal/terminal"
)

// terminalConfigFromAppConfig translates the layered settings into terminal runtime config.
func terminalConfigFromAppConfig(cfg *config.Config) terminal.Config {
	resolved := terminal.DefaultConfig()
	if cfg == nil {
		return resolved
	}
	resolved.Enabled = cfg.TerminalEnabled
	resolved.Shell = strings.TrimSpace(cfg.TerminalShell)
	if len(cfg.TerminalCommands) > 0 {
		resolved.Commands = make([]terminal.CommandProfile, 0, len(cfg.TerminalCommands))
		for _, command := range cfg.TerminalCommands {
			resolved.Commands = append(resolved.Commands, terminal.CommandProfile{
				ID: command.ID, Name: command.Name, Command: command.Command, Enabled: command.Enabled,
			})
		}
	}
	if cfg.TerminalMaxSessions > 0 {
		resolved.MaxSessions = cfg.TerminalMaxSessions
	}
	if cfg.TerminalScrollbackKB > 0 {
		resolved.ScrollbackBytes = cfg.TerminalScrollbackKB * 1024
	}
	return resolved
}

// Terminals exposes the terminal session manager to the HTTP layer. Configuration is synced on
// each call so toggles and limits saved in Settings take effect without a backend restart.
func (a *App) Terminals() *terminal.Manager {
	if a == nil || a.terminals == nil {
		return nil
	}
	a.mu.RLock()
	cfg := a.cfg
	a.mu.RUnlock()
	a.terminals.SetConfig(terminalConfigFromAppConfig(cfg))
	return a.terminals
}

// TerminalDefaultCwd returns the default working directory for new terminals: the current book
// workspace, then the user home directory, finally the backend process directory.
func (a *App) TerminalDefaultCwd() string {
	if a == nil {
		return ""
	}
	a.mu.RLock()
	workspace := a.workspace
	a.mu.RUnlock()
	if dir := usableDirectory(workspace); dir != "" {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil {
		if dir := usableDirectory(home); dir != "" {
			return dir
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[app/terminal_app_service.go] resolve process cwd failed: %v", err))
		return ""
	}
	return cwd
}

func usableDirectory(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return ""
	}
	return path
}
