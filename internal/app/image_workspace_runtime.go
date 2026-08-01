package app

import (
	"context"
	agentharness "denova/internal/agents/harness"
	"fmt"
	"strings"

	"denova/config"
	"denova/internal/agents/session"
	"denova/internal/book"
	"denova/internal/interactive"
)

// imageWorkspaceRuntime pins every image path to one exact workspace
// generation. Callers use only these captured adapters while operation is
// alive; workspace switch/close cancels Context and waits for Release.
type imageWorkspaceRuntime struct {
	operation    *appOperation
	workspace    string
	cfg          config.Config
	bookState    *book.State
	bookService  *book.Service
	interactive  *interactive.Store
	sessionStore *session.Store
	chatService  *agentharness.Service
}

func (r *imageWorkspaceRuntime) Context() context.Context {
	if r == nil || r.operation == nil {
		return context.Background()
	}
	return r.operation.Context()
}

func (r *imageWorkspaceRuntime) Release() {
	if r != nil && r.operation != nil {
		r.operation.Release()
	}
}

func (s *ImageAppService) acquireWorkspaceRuntime(ctx context.Context) (*imageWorkspaceRuntime, error) {
	return s.acquireWorkspaceRuntimeFor(ctx, "")
}

// acquireWorkspaceRuntimeFor atomically binds a UI request to the workspace
// identity sent by its caller. An empty expectedWorkspace keeps the internal
// current-workspace behavior used by non-HTTP call sites.
func (s *ImageAppService) acquireWorkspaceRuntimeFor(ctx context.Context, expectedWorkspace string) (*imageWorkspaceRuntime, error) {
	if s == nil || s.app == nil {
		return nil, ErrNoWorkspace
	}
	a := s.app
	a.mu.RLock()
	workspace := strings.TrimSpace(a.workspace)
	if workspace == "" || a.bookService == nil || a.cfg == nil {
		a.mu.RUnlock()
		return nil, ErrNoWorkspace
	}
	if strings.TrimSpace(expectedWorkspace) != "" && lifecycleWorkspaceKey(expectedWorkspace) != lifecycleWorkspaceKey(workspace) {
		a.mu.RUnlock()
		return nil, fmt.Errorf("%w: expected=%q actual=%q", ErrWorkspaceChanged, expectedWorkspace, workspace)
	}
	runtime := &imageWorkspaceRuntime{
		workspace: workspace, cfg: *a.cfg, bookState: a.bookState,
		bookService: a.bookService, interactive: a.interactive,
		sessionStore: a.sessionStore, chatService: a.chatService,
	}
	a.mu.RUnlock()

	operation, err := a.acquireWorkspaceOperation(ctx, workspace, true)
	if err != nil {
		return nil, err
	}
	runtime.operation = operation
	if !runtime.matches(a) {
		runtime.Release()
		return nil, ErrWorkspaceChanged
	}
	runtime.cfg.Workspace = workspace
	return runtime, nil
}

func (r *imageWorkspaceRuntime) matches(a *App) bool {
	if r == nil || a == nil {
		return false
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return lifecycleWorkspaceKey(a.workspace) == lifecycleWorkspaceKey(r.workspace) &&
		a.bookState == r.bookState && a.bookService == r.bookService &&
		a.interactive == r.interactive && a.sessionStore == r.sessionStore &&
		a.chatService == r.chatService
}

func (r *imageWorkspaceRuntime) requireAgentAdapters() error {
	if r == nil || r.bookState == nil || r.bookService == nil || r.chatService == nil {
		return fmt.Errorf("image Agent workspace runtime is incomplete")
	}
	return nil
}
