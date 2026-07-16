package app

import (
	"context"
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
type automationWorkspaceSnapshot struct {
	workspace    string
	novaDir      string
	cfg          config.Config
	bookState    *book.State
	bookService  *book.Service
	sessionStore *session.Store
	chatService  *agent.ChatService
}

// automationSnapshotLocked must be called while the caller holds app.mu for
// reading or writing. It deliberately does not reacquire the RWMutex: a nested
// read lock can deadlock when a workspace switch is already waiting to write.
func (a *App) automationSnapshotLocked() *AutomationAppService {
	workspace := strings.TrimSpace(a.workspace)
	if workspace == "" {
		return nil
	}
	cfg := config.Config{Workspace: workspace}
	if a.cfg != nil {
		cfg = *a.cfg
		cfg.Workspace = workspace
	}
	return &AutomationAppService{
		app: a,
		snapshot: &automationWorkspaceSnapshot{
			workspace:    workspace,
			novaDir:      cfg.NovaDir,
			cfg:          cfg,
			bookState:    a.bookState,
			bookService:  a.bookService,
			sessionStore: a.sessionStore,
			chatService:  a.chatService,
		},
	}
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

func (a *App) automationSnapshot() *AutomationAppService {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.automationSnapshotLocked()
}

func (s *AutomationAppService) workspaceScoped() *AutomationAppService {
	if s == nil {
		return nil
	}
	if s.snapshot != nil {
		return s
	}
	return s.app.automationSnapshot()
}

func (s *AutomationAppService) configSnapshot() config.Config {
	if s.snapshot != nil {
		return s.snapshot.cfg
	}
	s.app.mu.RLock()
	defer s.app.mu.RUnlock()
	if s.app.cfg == nil {
		return config.Config{Workspace: s.app.workspace}
	}
	cfg := *s.app.cfg
	cfg.Workspace = s.app.workspace
	return cfg
}

func (s *AutomationAppService) bookService() *book.Service {
	if s.snapshot != nil {
		return s.snapshot.bookService
	}
	s.app.mu.RLock()
	defer s.app.mu.RUnlock()
	return s.app.bookService
}

func (s *AutomationAppService) sessionStore() *session.Store {
	if s.snapshot != nil {
		return s.snapshot.sessionStore
	}
	s.app.mu.RLock()
	defer s.app.mu.RUnlock()
	return s.app.sessionStore
}

func (s *AutomationAppService) chatService() *agent.ChatService {
	if s.snapshot != nil {
		return s.snapshot.chatService
	}
	s.app.mu.RLock()
	defer s.app.mu.RUnlock()
	return s.app.chatService
}

func (s *AutomationAppService) automationMutationCallback(source string) func(context.Context, []agent.ToolMutation, agent.PostRunVerification) {
	scoped := s.workspaceScoped()
	return func(ctx context.Context, mutations []agent.ToolMutation, _ agent.PostRunVerification) {
		if scoped == nil {
			return
		}
		paths := make([]string, 0, len(mutations))
		for _, mutation := range mutations {
			if mutation.Target != "" {
				paths = append(paths, mutation.Target)
			}
		}
		scoped.CheckTriggersAfterWorkspaceMutation(ctx, source, paths)
	}
}
