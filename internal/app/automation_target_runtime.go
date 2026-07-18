package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/automation"
	"denova/internal/book"
	"denova/internal/session"
)

// automationForTarget resolves an execution context without changing the
// workspace selected in the UI. Inactive books are loaded lazily only when a
// task needs to run or its trigger needs evaluation.
func (s *AutomationAppService) automationForTarget(ctx context.Context, target automation.ExecutionTarget) (*AutomationAppService, error) {
	if s == nil || s.app == nil {
		return nil, fmt.Errorf("automation app is unavailable")
	}
	if target.Kind == automation.TargetKindUser {
		return s.globalAutomationRuntime()
	}
	workspace := canonicalAutomationWorkspace(target.Workspace)
	if workspace == "" {
		return nil, fmt.Errorf("automation workspace target is required")
	}
	if current := s.app.automationSnapshot(); current != nil && canonicalAutomationWorkspace(current.workspace()) == workspace {
		return current, nil
	}

	s.app.mu.RLock()
	baseCfg := config.Config{}
	if s.app.cfg != nil {
		baseCfg = *s.app.cfg
	}
	chatService := s.app.chatService
	s.app.mu.RUnlock()
	baseCfg.Workspace = workspace
	if layered, err := config.LoadLayeredWithStartupConfig(baseCfg.NovaDir, workspace); err == nil {
		applyLayeredSettingsToConfig(&baseCfg, layered)
	}
	state := book.NewState(workspace)
	if err := state.InitWorkspace(); err != nil {
		return nil, fmt.Errorf("initialize automation workspace %s: %w", workspace, err)
	}
	sessionStore, err := session.NewStore(state.SessionDir())
	if err != nil {
		return nil, fmt.Errorf("open automation sessions for %s: %w", workspace, err)
	}
	if chatService == nil {
		chatService = agent.NewChatService()
	}
	return &AutomationAppService{
		app: s.app,
		snapshot: &automationWorkspaceSnapshot{
			workspace:    workspace,
			novaDir:      baseCfg.NovaDir,
			cfg:          baseCfg,
			bookState:    state,
			bookService:  book.NewService(workspace),
			sessionStore: sessionStore,
			chatService:  chatService,
		},
	}, nil
}

func (s *AutomationAppService) globalAutomationRuntime() (*AutomationAppService, error) {
	s.app.mu.RLock()
	baseCfg := config.Config{}
	if s.app.cfg != nil {
		baseCfg = *s.app.cfg
	}
	chatService := s.app.chatService
	s.app.mu.RUnlock()
	baseCfg.Workspace = ""
	novaDir := strings.TrimSpace(baseCfg.NovaDir)
	if novaDir == "" {
		return nil, fmt.Errorf("user data directory is required for global automation")
	}
	sessionStore, err := session.NewStore(filepath.Join(novaDir, "automations", "sessions"))
	if err != nil {
		return nil, fmt.Errorf("open global automation sessions: %w", err)
	}
	if chatService == nil {
		chatService = agent.NewChatService()
	}
	return &AutomationAppService{
		app: s.app,
		snapshot: &automationWorkspaceSnapshot{
			novaDir:      novaDir,
			cfg:          baseCfg,
			sessionStore: sessionStore,
			chatService:  chatService,
		},
	}, nil
}

func (s *AutomationAppService) automationForTask(ctx context.Context, task automation.Task) (*AutomationAppService, error) {
	return s.automationForTarget(ctx, task.Target)
}
