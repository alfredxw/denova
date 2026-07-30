package app

import (
	"context"
	"log"
	"strings"

	"denova/config"
	"denova/internal/agents/session"
	"denova/internal/book"
)

// versionCreateRuntime is the immutable identity and dependency set for one
// manual version creation. Summary inference and the final snapshot commit
// must never re-resolve adapters from the live App.
type versionCreateRuntime struct {
	operation      *appOperation
	workspace      string
	cfgRef         *config.Config
	cfg            config.Config
	bookService    *book.Service
	versionService *book.VersionService
	sessionStore   *session.Store
	settings       book.VersionAutoSettings
}

func (s *WorkspaceRuntimeManager) acquireVersionCreateRuntime(ctx context.Context) (*versionCreateRuntime, error) {
	if s == nil || s.app == nil {
		return nil, ErrNoWorkspace
	}
	a := s.app
	a.mu.RLock()
	workspace := strings.TrimSpace(a.workspace)
	cfgRef := a.cfg
	bookService := a.bookService
	versionService := a.versionService
	sessionStore := a.sessionStore
	if workspace == "" || cfgRef == nil || bookService == nil || versionService == nil {
		a.mu.RUnlock()
		return nil, ErrNoWorkspace
	}
	runtime := &versionCreateRuntime{
		workspace: workspace, cfgRef: cfgRef, cfg: *cfgRef,
		bookService: bookService, versionService: versionService,
		sessionStore: sessionStore, settings: versionAutoSettingsForConfig(cfgRef),
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
	projectConfigPath := config.ProjectConfigPath(runtime.cfg.ProjectStateDir)
	if layered, loadErr := config.LoadLayeredWithStartupConfigAt(runtime.cfg.DataDir(), workspace, projectConfigPath); loadErr == nil {
		applyLayeredSettingsToConfig(&runtime.cfg, layered)
	} else {
		log.Printf("[versions] 加载分层配置用于版本说明失败 workspace=%s err=%v", workspace, loadErr)
	}
	return runtime, nil
}

func (r *versionCreateRuntime) Context() context.Context {
	if r == nil || r.operation == nil {
		return context.Background()
	}
	return r.operation.Context()
}

func (r *versionCreateRuntime) Release() {
	if r != nil && r.operation != nil {
		r.operation.Release()
	}
}

func (r *versionCreateRuntime) matches(a *App) bool {
	if r == nil || a == nil {
		return false
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return lifecycleWorkspaceKey(a.workspace) == lifecycleWorkspaceKey(r.workspace) &&
		a.cfg == r.cfgRef && a.bookService == r.bookService &&
		a.versionService == r.versionService && a.sessionStore == r.sessionStore
}
