package app

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/book"
	"denova/internal/session"
)

// automationWorkspaceSnapshot binds asynchronous trigger evaluation and any
// resulting automation run to one workspace runtime. The referenced services
// remain valid after App switches to another workspace.
//
// The snapshot is always passed as a parameter (or returned from a constructor)
// — it is never stored as a field on AutomationAppService. This keeps the
// service a thin facade over the live App and avoids the "two-headed" pattern
// where one type conditionally behaves like two different objects.
type automationWorkspaceSnapshot struct {
	workspace    string
	novaDir      string
	cfg          config.Config
	bookState    *book.State
	bookService  *book.Service
	sessionStore *session.Store
	chatService  *agent.ChatService
}

// runtimeSnapshot builds a snapshot from the currently-active App runtime. It
// acquires the read lock once, copies every value it needs, releases the lock,
// then loads layered settings outside the lock — mirroring the
// LoreAppService.loreImageRuntimeSnapshot pattern.
func (s *AutomationAppService) runtimeSnapshot() (*automationWorkspaceSnapshot, error) {
	a := s.app
	a.mu.RLock()
	if a.workspace == "" {
		a.mu.RUnlock()
		return nil, ErrNoWorkspace
	}
	if a.cfg == nil {
		a.mu.RUnlock()
		return nil, fmt.Errorf("运行配置未初始化")
	}
	cfg := *a.cfg
	workspace := a.workspace
	novaDir := cfg.DataDir()
	bookState := a.bookState
	bookService := a.bookService
	sessionStore := a.sessionStore
	chatService := a.chatService
	a.mu.RUnlock()

	cfg.Workspace = workspace
	applyAutomationLayeredConfig(&cfg, novaDir, workspace)
	return &automationWorkspaceSnapshot{
		workspace:    workspace,
		novaDir:      novaDir,
		cfg:          cfg,
		bookState:    bookState,
		bookService:  bookService,
		sessionStore: sessionStore,
		chatService:  chatService,
	}, nil
}

// applyAutomationLayeredConfig loads layered settings for the given workspace and
// merges them into cfg in place. It is the single place where on-disk user and
// workspace settings are folded into an automation runtime config, so every
// snapshot (live workspace or cross-workspace target) resolves models, tools,
// and iteration limits consistently.
func applyAutomationLayeredConfig(cfg *config.Config, novaDir, workspace string) {
	if cfg == nil {
		return
	}
	if layered, err := config.LoadLayeredWithStartupConfig(novaDir, workspace); err == nil {
		applyLayeredSettingsToConfig(cfg, layered)
	} else {
		log.Printf("[automation] load layered settings failed workspace=%s err=%v", workspace, err)
	}
}

// automationSnapshotLocked must be called while the caller holds app.mu for
// reading or writing. It deliberately does not reacquire the RWMutex: a nested
// read lock can deadlock when a workspace switch is already waiting to write.
func (a *App) automationSnapshotLocked() *automationWorkspaceSnapshot {
	workspace := strings.TrimSpace(a.workspace)
	if workspace == "" {
		return nil
	}
	cfg := config.Config{Workspace: workspace}
	if a.cfg != nil {
		cfg = *a.cfg
		cfg.Workspace = workspace
	}
	return &automationWorkspaceSnapshot{
		workspace:    workspace,
		novaDir:      cfg.DataDir(),
		cfg:          cfg,
		bookState:    a.bookState,
		bookService:  a.bookService,
		sessionStore: a.sessionStore,
		chatService:  a.chatService,
	}
}

func (a *App) automationSnapshot() *automationWorkspaceSnapshot {
	a.mu.RLock()
	snap := a.automationSnapshotLocked()
	a.mu.RUnlock()
	if snap == nil {
		return nil
	}
	applyAutomationLayeredConfig(&snap.cfg, snap.novaDir, snap.workspace)
	return snap
}

func canonicalAutomationWorkspace(workspace string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return ""
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return filepath.Clean(workspace)
	}
	if canonical, err := filepath.EvalSymlinks(abs); err == nil {
		return filepath.Clean(canonical)
	}
	return filepath.Clean(abs)
}
