package app

import (
	"log"
	"os"
	"path/filepath"
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
		log.Printf("[app/terminal_app_service.go] resolve process cwd failed: %v", err)
		return ""
	}
	return cwd
}

// ResolveTerminalWorkspace binds a Workspace terminal tab to its own registered
// project. Empty input retains the foreground terminal behavior used outside
// AgentChat; explicit project input never changes App.workspace.
func (a *App) ResolveTerminalWorkspace(requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		return a.canonicalAgentChatWorkspace(requested)
	}
	if a == nil {
		return "", ErrNoWorkspace
	}
	a.mu.RLock()
	workspace := a.workspace
	a.mu.RUnlock()
	return workspace, nil
}

// ResolveTerminalProject resolves the stable owner and content directory for
// an AgentChat terminal without changing the foreground Book.
func (a *App) ResolveTerminalProject(projectID, legacyWorkspace string) (string, string, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID != "" {
		record, layout, err := a.resolveProject(projectID, true)
		if err != nil {
			return "", "", err
		}
		return record.ID, layout.ContentRoot, nil
	}
	legacyWorkspace = strings.TrimSpace(legacyWorkspace)
	if legacyWorkspace != "" {
		record, layout, err := a.resolveProjectByWorkspace(legacyWorkspace)
		if err != nil {
			return "", "", err
		}
		return record.ID, layout.ContentRoot, nil
	}
	workspace, err := a.ResolveTerminalWorkspace("")
	return "", workspace, err
}

// ResolveTerminalCwd validates the working directory sent by the frontend and falls back to the
// default when it is invalid or missing. The terminal already lets the user run arbitrary
// commands, so this only checks that the directory exists rather than sandboxing it.
func (a *App) ResolveTerminalCwd(requested string) string {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return a.TerminalDefaultCwd()
	}
	absolute, err := filepath.Abs(requested)
	if err != nil {
		log.Printf("[app/terminal_app_service.go] resolve terminal cwd failed path=%q err=%v", requested, err)
		return a.TerminalDefaultCwd()
	}
	if dir := usableDirectory(absolute); dir != "" {
		return dir
	}
	log.Printf("[app/terminal_app_service.go] terminal cwd not usable, falling back path=%q", absolute)
	return a.TerminalDefaultCwd()
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
