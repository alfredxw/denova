package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"denova/internal/concurrency"
	projectdomain "denova/internal/project"
	workspacechange "denova/internal/workspace/change"
)

// ErrForegroundProject protects the open Writing runtime from being detached
// behind its back. Selecting another Book is an explicit navigation action;
// Project management never performs that switch implicitly.
var ErrForegroundProject = errors.New("project is the foreground Writing workspace")

// appOperation is an admitted unit of work. Its context is canceled when the
// owning App or workspace generation starts closing; Release is the resource
// barrier used by workspace switches and App.Close.
type appOperation struct {
	ctx   context.Context
	lease *concurrency.Lease
}

func (o *appOperation) Context() context.Context {
	if o == nil || o.ctx == nil {
		return context.Background()
	}
	return o.ctx
}

func (o *appOperation) Release() {
	if o != nil && o.lease != nil {
		o.lease.Release()
	}
}

// ProjectOperation is one request's immutable Project identity plus the
// lifecycle lease that prevents archive or relink from changing its layout
// before the request completes.
type ProjectOperation struct {
	ctx    context.Context
	layout projectdomain.Layout
	lease  *concurrency.Lease
}

type projectOperationContextKey struct{}

func (operation *ProjectOperation) Context() context.Context {
	if operation == nil || operation.ctx == nil {
		return context.Background()
	}
	return operation.ctx
}

func (operation *ProjectOperation) Layout() projectdomain.Layout {
	if operation == nil {
		return projectdomain.Layout{}
	}
	return operation.layout
}

func (operation *ProjectOperation) Release() {
	if operation != nil && operation.lease != nil {
		operation.lease.Release()
	}
}

func (a *App) initializeLifecycleLocked() error {
	if a == nil || a.closed {
		return concurrency.ErrClosed
	}
	if a.rootScope == nil {
		a.rootScope = concurrency.NewRoot("denova-app")
	}
	if a.workspaceScopes == nil {
		a.workspaceScopes = make(map[string]*concurrency.Scope)
	}
	if a.projectScopes == nil {
		a.projectScopes = make(map[string]*concurrency.Scope)
	}
	if a.projectTransitions == nil {
		a.projectTransitions = make(map[string]struct{})
	}
	return nil
}

func (a *App) projectScopeLocked(projectID string) (*concurrency.Scope, error) {
	if err := a.initializeLifecycleLocked(); err != nil {
		return nil, err
	}
	projectID = strings.TrimSpace(projectID)
	if err := projectdomain.ValidateID(projectID); err != nil {
		return nil, err
	}
	if _, transitioning := a.projectTransitions[projectID]; transitioning {
		return nil, ErrWorkspaceTransition
	}
	if scope := a.projectScopes[projectID]; scope != nil {
		return scope, nil
	}
	a.projectScopeSequence++
	scope, err := a.rootScope.Child(fmt.Sprintf("project:%s:g%d", projectID, a.projectScopeSequence))
	if err != nil {
		return nil, err
	}
	a.projectScopes[projectID] = scope
	return scope, nil
}

// AcquireProjectOperation resolves one stable Project and admits work to that
// exact Project generation. Project paths are derived server-side and remain a
// display/storage detail rather than caller authority.
func (a *App) AcquireProjectOperation(ctx context.Context, projectID string) (*ProjectOperation, error) {
	if a == nil {
		return nil, concurrency.ErrClosed
	}
	projectID = strings.TrimSpace(projectID)
	if err := projectdomain.ValidateID(projectID); err != nil {
		return nil, err
	}
	if current, ok := ctx.Value(projectOperationContextKey{}).(*ProjectOperation); ok && current != nil && current.layout.ProjectID == projectID {
		if err := current.Context().Err(); err != nil {
			return nil, err
		}
		// The request-level owner retains the lifecycle lease. Nested application
		// services borrow its immutable scope and may release their handle without
		// shortening the outer request's admission.
		return &ProjectOperation{ctx: current.Context(), layout: current.layout}, nil
	}
	return a.acquireOwnedProjectOperation(ctx, projectID)
}

// acquireOwnedProjectOperation always admits a new lifecycle lease. Detached
// work must use this boundary because a request-scoped operation carried as a
// context value is only a borrow and cannot outlive its HTTP owner.
func (a *App) acquireOwnedProjectOperation(ctx context.Context, projectID string) (*ProjectOperation, error) {
	// Reject unknown identities before allocating a lifecycle scope. The layout
	// is resolved again after admission because a relink may begin between
	// these two steps; projectTransitions prevents admitting that stale view.
	if _, _, err := a.resolveProject(projectID, true); err != nil {
		return nil, err
	}
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil, concurrency.ErrClosed
	}
	scope, err := a.projectScopeLocked(projectID)
	if err != nil {
		a.mu.Unlock()
		return nil, err
	}
	operationContext, lease, err := scope.AcquireContext(ctx)
	a.mu.Unlock()
	if err != nil {
		if errors.Is(err, concurrency.ErrClosing) || errors.Is(err, concurrency.ErrClosed) {
			return nil, ErrWorkspaceTransition
		}
		return nil, err
	}
	_, layout, err := a.resolveProject(projectID, true)
	if err != nil {
		lease.Release()
		return nil, err
	}
	operation := &ProjectOperation{ctx: operationContext, layout: layout, lease: lease}
	operation.ctx = context.WithValue(operation.ctx, projectOperationContextKey{}, operation)
	return operation, nil
}

// beginProjectTransition fences and drains every operation admitted for one
// stable Project generation. The caller must always finish the transition so
// a failed relink/archive does not strand the Project in a closing state.
func (a *App) beginProjectTransition(ctx context.Context, projectID string) error {
	if a == nil {
		return concurrency.ErrClosed
	}
	projectID = strings.TrimSpace(projectID)
	if err := projectdomain.ValidateID(projectID); err != nil {
		return err
	}
	a.mu.Lock()
	if err := a.initializeLifecycleLocked(); err != nil {
		a.mu.Unlock()
		return err
	}
	if _, transitioning := a.projectTransitions[projectID]; transitioning {
		a.mu.Unlock()
		return ErrWorkspaceTransition
	}
	a.projectTransitions[projectID] = struct{}{}
	scope := a.projectScopes[projectID]
	if scope != nil {
		scope.BeginClose()
	}
	a.mu.Unlock()
	if scope == nil {
		return nil
	}
	if err := scope.Wait(ctx); err != nil {
		a.finishProjectTransition(projectID, true)
		return err
	}
	return nil
}

// finishProjectTransition installs a fresh generation for an active Project,
// or removes its lifecycle state after archive. It is safe after a failed
// transition and is intentionally idempotent from the caller's perspective.
func (a *App) finishProjectTransition(projectID string, active bool) {
	if a == nil {
		return
	}
	projectID = strings.TrimSpace(projectID)
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.projectScopes, projectID)
	delete(a.projectTransitions, projectID)
	if !active || a.closed {
		return
	}
	_, _ = a.projectScopeLocked(projectID)
}

// RelinkProject changes only the content directory behind a stable Project
// identity. Durable Project state stays at the existing state root.
func (a *App) RelinkProject(ctx context.Context, projectID, path string) (projectdomain.Record, error) {
	projectID = strings.TrimSpace(projectID)
	record, oldLayout, err := a.resolveProject(projectID, false)
	if err != nil {
		return projectdomain.Record{}, err
	}
	a.mu.RLock()
	foregroundProjectID := ""
	if a.cfg != nil {
		foregroundProjectID = strings.TrimSpace(a.cfg.ProjectID)
	}
	a.mu.RUnlock()
	if record.Type == projectdomain.TypeBook && foregroundProjectID == projectID {
		return projectdomain.Record{}, ErrForegroundProject
	}
	if err := a.beginProjectTransition(ctx, projectID); err != nil {
		return projectdomain.Record{}, err
	}
	defer func() { a.finishProjectTransition(projectID, true) }()

	if a.terminals != nil {
		if err := a.terminals.CloseProject(projectID); err != nil {
			return projectdomain.Record{}, err
		}
	}
	if err := a.AgentChat().CloseProject(ctx, projectID); err != nil {
		return projectdomain.Record{}, err
	}
	if a.workspaceFiles != nil {
		a.workspaceFiles.CloseProject(projectID)
	}
	a.ProjectFiles().CloseProject(projectID)
	if err := workspacechange.ForgetWorkspace(oldLayout.ContentRoot); err != nil {
		return projectdomain.Record{}, fmt.Errorf("close Project change journal before relink: %w", err)
	}
	relinked, err := a.projectRegistry.Relink(projectID, path)
	if err != nil {
		return projectdomain.Record{}, err
	}
	if _, err := a.projectRegistry.EnsureState(relinked); err != nil {
		// Registry records are user-owned durable data. Restore the old content
		// link if state preparation fails after the record was changed.
		_, rollbackErr := a.projectRegistry.Relink(projectID, oldLayout.ContentRoot)
		if rollbackErr != nil {
			return projectdomain.Record{}, fmt.Errorf("prepare relinked Project state: %w (rollback failed: %v)", err, rollbackErr)
		}
		return projectdomain.Record{}, fmt.Errorf("prepare relinked Project state: %w", err)
	}
	return relinked, nil
}

// ArchiveProject drains Project-owned work before hiding the registry record.
// User files and durable Project state are never deleted.
func (a *App) ArchiveProject(ctx context.Context, projectID string) (projectdomain.Record, error) {
	projectID = strings.TrimSpace(projectID)
	record, layout, err := a.resolveProject(projectID, false)
	if err != nil {
		return projectdomain.Record{}, err
	}
	a.mu.RLock()
	foregroundProjectID := ""
	if a.cfg != nil {
		foregroundProjectID = strings.TrimSpace(a.cfg.ProjectID)
	}
	a.mu.RUnlock()
	if record.Type == projectdomain.TypeBook && foregroundProjectID == projectID {
		return projectdomain.Record{}, ErrForegroundProject
	}
	if err := a.beginProjectTransition(ctx, projectID); err != nil {
		return projectdomain.Record{}, err
	}
	archived := false
	defer func() { a.finishProjectTransition(projectID, !archived) }()
	if a.terminals != nil {
		if err := a.terminals.CloseProject(projectID); err != nil {
			return projectdomain.Record{}, err
		}
	}
	if err := a.AgentChat().CloseProject(ctx, projectID); err != nil {
		return projectdomain.Record{}, err
	}
	if a.workspaceFiles != nil {
		a.workspaceFiles.CloseProject(projectID)
	}
	a.ProjectFiles().CloseProject(projectID)
	if err := workspacechange.ForgetWorkspace(layout.ContentRoot); err != nil {
		return projectdomain.Record{}, fmt.Errorf("close Project change journal before archive: %w", err)
	}
	archivedRecord, err := a.projectRegistry.Archive(projectID)
	if err != nil {
		return projectdomain.Record{}, err
	}
	archived = true
	return archivedRecord, nil
}

func lifecycleWorkspaceKey(workspace string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return ""
	}
	if abs, err := filepath.Abs(workspace); err == nil {
		workspace = abs
	}
	if canonical, err := filepath.EvalSymlinks(workspace); err == nil {
		workspace = canonical
	}
	return filepath.Clean(workspace)
}

func (a *App) workspaceScopeLocked(workspace string) (*concurrency.Scope, error) {
	if err := a.initializeLifecycleLocked(); err != nil {
		return nil, err
	}
	key := lifecycleWorkspaceKey(workspace)
	if key == "" {
		return nil, ErrNoWorkspace
	}
	if scope := a.workspaceScopes[key]; scope != nil {
		return scope, nil
	}
	// Scope identity is independent from the structural generation of the
	// currently installed workspace. Cross-workspace automation may create an
	// inactive scope and must not invalidate a current session/story fence.
	a.workspaceScopeSequence++
	scope, err := a.rootScope.Child(fmt.Sprintf("workspace:%s:g%d", key, a.workspaceScopeSequence))
	if err != nil {
		return nil, err
	}
	a.workspaceScopes[key] = scope
	return scope, nil
}

// acquireWorkspaceOperation atomically validates identity and admits work to
// that exact workspace generation. strictCurrent is used by UI operations;
// cross-book automation passes false but remains fenced when that target is
// being rebuilt.
func (a *App) acquireWorkspaceOperation(ctx context.Context, workspace string, strictCurrent bool) (*appOperation, error) {
	if a == nil {
		return nil, concurrency.ErrClosed
	}
	key := lifecycleWorkspaceKey(workspace)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil, concurrency.ErrClosed
	}
	if strictCurrent && lifecycleWorkspaceKey(a.workspace) != key {
		return nil, fmt.Errorf("%w: expected=%q actual=%q", ErrWorkspaceChanged, workspace, a.workspace)
	}
	if _, transitioning := a.workspaceTransitionTargets[key]; a.workspaceTransition && transitioning {
		return nil, ErrWorkspaceTransition
	}
	scope, err := a.workspaceScopeLocked(key)
	if err != nil {
		return nil, err
	}
	opCtx, lease, err := scope.AcquireContext(ctx)
	if err != nil {
		if errors.Is(err, concurrency.ErrClosing) || errors.Is(err, concurrency.ErrClosed) {
			return nil, ErrWorkspaceTransition
		}
		return nil, err
	}
	return &appOperation{ctx: opCtx, lease: lease}, nil
}

func (a *App) acquireRootOperation(ctx context.Context) (*appOperation, error) {
	if a == nil {
		return nil, concurrency.ErrClosed
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.initializeLifecycleLocked(); err != nil {
		return nil, err
	}
	opCtx, lease, err := a.rootScope.AcquireContext(ctx)
	if err != nil {
		return nil, err
	}
	return &appOperation{ctx: opCtx, lease: lease}, nil
}

// fenceWorkspaceScopesLocked fences known generations for the provided
// workspaces. The caller must hold App.mu and wait after releasing it.
func (a *App) fenceWorkspaceScopesLocked(workspaces ...string) []*concurrency.Scope {
	seen := make(map[*concurrency.Scope]struct{})
	result := make([]*concurrency.Scope, 0, len(workspaces))
	for _, workspace := range workspaces {
		key := lifecycleWorkspaceKey(workspace)
		if key == "" {
			continue
		}
		if scope := a.workspaceScopes[key]; scope != nil {
			if _, ok := seen[scope]; !ok {
				scope.BeginClose()
				seen[scope] = struct{}{}
				result = append(result, scope)
			}
		}
	}
	return result
}

func waitLifecycleScopes(ctx context.Context, scopes []*concurrency.Scope) error {
	for _, scope := range scopes {
		if scope == nil {
			continue
		}
		if err := scope.Wait(ctx); err != nil {
			return fmt.Errorf("wait for %s: %w", scope.Name(), err)
		}
	}
	return nil
}

func (a *App) replaceWorkspaceScopeLocked(workspace string) error {
	if err := a.initializeLifecycleLocked(); err != nil {
		return err
	}
	key := lifecycleWorkspaceKey(workspace)
	if key == "" {
		return ErrNoWorkspace
	}
	delete(a.workspaceScopes, key)
	if _, err := a.workspaceScopeLocked(key); err != nil {
		return err
	}
	// Only installing (or deliberately restoring) the current runtime creates
	// a new structural generation. This is the CAS token used by session and
	// story mutation fences.
	a.workspaceGeneration++
	return nil
}

func (a *App) restoreWorkspaceGenerationAfterFailedTransition(currentWorkspace string, attempted ...string) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, workspace := range append([]string{currentWorkspace}, attempted...) {
		delete(a.workspaceScopes, lifecycleWorkspaceKey(workspace))
	}
	if strings.TrimSpace(currentWorkspace) != "" && !a.closed {
		if err := a.replaceWorkspaceScopeLocked(currentWorkspace); err != nil {
			// The App root is the only reason replacement can fail here. Keep
			// the runtime fenced; callers will receive the original transition
			// error and App.Close remains safe.
			return
		}
	}
}

// closeWorkspaceRuntimeBindings evicts foreground-owned harness actors only
// after the App workspace generation has drained. Project-scoped AgentChat
// actors deliberately survive foreground Book changes.
func (a *App) closeWorkspaceRuntimeBindings(ctx context.Context, workspaces ...string) error {
	if a == nil || a.chatService == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(workspaces))
	for _, workspace := range workspaces {
		workspace = lifecycleWorkspaceKey(workspace)
		if strings.TrimSpace(workspace) == "" {
			continue
		}
		if _, exists := seen[workspace]; exists {
			continue
		}
		seen[workspace] = struct{}{}
		if err := a.chatService.CloseForegroundWorkspaceBindings(ctx, workspace); err != nil {
			return err
		}
	}
	return nil
}
