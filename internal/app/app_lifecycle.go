package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"denova/internal/lifecycle"
)

// appOperation is an admitted unit of work. Its context is canceled when the
// owning App or workspace generation starts closing; Release is the resource
// barrier used by workspace switches and App.Close.
type appOperation struct {
	ctx   context.Context
	lease *lifecycle.Lease
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

func (a *App) initializeLifecycleLocked() error {
	if a == nil || a.closed {
		return lifecycle.ErrClosed
	}
	if a.rootScope == nil {
		a.rootScope = lifecycle.NewRoot("denova-app")
	}
	if a.workspaceScopes == nil {
		a.workspaceScopes = make(map[string]*lifecycle.Scope)
	}
	return nil
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

func (a *App) workspaceScopeLocked(workspace string) (*lifecycle.Scope, error) {
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
		return nil, lifecycle.ErrClosed
	}
	key := lifecycleWorkspaceKey(workspace)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil, lifecycle.ErrClosed
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
		if errors.Is(err, lifecycle.ErrClosing) || errors.Is(err, lifecycle.ErrClosed) {
			return nil, ErrWorkspaceTransition
		}
		return nil, err
	}
	return &appOperation{ctx: opCtx, lease: lease}, nil
}

func (a *App) acquireRootOperation(ctx context.Context) (*appOperation, error) {
	if a == nil {
		return nil, lifecycle.ErrClosed
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
func (a *App) fenceWorkspaceScopesLocked(workspaces ...string) []*lifecycle.Scope {
	seen := make(map[*lifecycle.Scope]struct{})
	result := make([]*lifecycle.Scope, 0, len(workspaces))
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

func waitLifecycleScopes(ctx context.Context, scopes []*lifecycle.Scope) error {
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
