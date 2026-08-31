package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	projectdomain "denova/internal/project"
	workspacechange "denova/internal/workspace/change"
)

// ErrWorkspaceChanged means a mutation was submitted for a workspace that is
// no longer active. Callers must not silently redirect it to the new workspace.
var ErrWorkspaceChanged = errors.New("workspace changed during request")

// ReadWorkspaceFileWithRevision returns content, revision, and workspace from
// one runtime lease so a concurrent workspace switch cannot mix identities.
func (a *App) ReadWorkspaceFileWithRevision(path string) (content, revision, workspace string, err error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.workspace == "" || a.bookService == nil {
		return "", "", "", ErrNoWorkspace
	}
	content, revision, err = a.bookService.ReadFileWithRevision(path)
	if err != nil {
		return "", "", "", err
	}
	return content, revision, a.workspace, nil
}

// WorkspaceChangeService returns the shared durable change journal for the
// active workspace. Agent tools, review endpoints, and editor saves use the
// same instance so their read-modify-write transactions cannot race.
func (a *App) WorkspaceChangeService() (*workspacechange.Service, error) {
	a.mu.RLock()
	workspace := a.workspace
	stateRoot := ""
	if a.cfg != nil {
		stateRoot = a.cfg.ProjectStateDir
	}
	a.mu.RUnlock()
	if workspace == "" {
		return nil, ErrNoWorkspace
	}
	return workspaceChangeService(workspace, stateRoot)
}

// workspaceChangeService keeps the visible content root and user-owned ledger
// root explicit at every mutation boundary.
func workspaceChangeService(workspace, stateRoot string) (*workspacechange.Service, error) {
	if strings.TrimSpace(stateRoot) != "" {
		return workspacechange.ForWorkspaceAt(workspace, stateRoot)
	}
	return workspacechange.ForWorkspace(workspace)
}

// WithWorkspaceChangeService runs a mutation while holding a read lease on the
// active workspace. Workspace switches take the write lock, so a request can
// neither drift into a newly selected workspace nor outlive the identity check.
func (a *App) WithWorkspaceChangeService(
	expectedWorkspace string,
	action func(*workspacechange.Service) error,
) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	actualWorkspace := strings.TrimSpace(a.workspace)
	expectedWorkspace = strings.TrimSpace(expectedWorkspace)
	if actualWorkspace == "" {
		return ErrNoWorkspace
	}
	if expectedWorkspace == "" || filepath.Clean(expectedWorkspace) != filepath.Clean(actualWorkspace) {
		return fmt.Errorf("%w: expected=%q actual=%q", ErrWorkspaceChanged, expectedWorkspace, actualWorkspace)
	}
	stateRoot := ""
	if a.cfg != nil {
		stateRoot = a.cfg.ProjectStateDir
	}
	service, err := workspaceChangeService(actualWorkspace, stateRoot)
	if err != nil {
		return err
	}
	return action(service)
}

// WorkspaceChangeMutationHooks describes post-mutation work that must stay
// bound to the same workspace lease as the durable change operation.
type WorkspaceChangeMutationHooks struct {
	ScheduleAutoVersion bool
	AutomationSource    string
	Paths               []string
}

// WithWorkspaceChangeMutation runs a durable workspace mutation and its
// post-mutation hooks under one read lease. Versioning uses the captured
// version service, while automation receives an immutable snapshot of every
// workspace-scoped dependency it may need after this method returns.
func (a *App) WithWorkspaceChangeMutation(
	ctx context.Context,
	expectedWorkspace string,
	action func(*workspacechange.Service) (WorkspaceChangeMutationHooks, error),
) (string, error) {
	a.ensureServices()
	expectedWorkspace = strings.TrimSpace(expectedWorkspace)
	a.mu.RLock()
	actualWorkspace := strings.TrimSpace(a.workspace)
	a.mu.RUnlock()
	if actualWorkspace == "" {
		return "", ErrNoWorkspace
	}
	if expectedWorkspace == "" || lifecycleWorkspaceKey(expectedWorkspace) != lifecycleWorkspaceKey(actualWorkspace) {
		return "", fmt.Errorf("%w: expected=%q actual=%q", ErrWorkspaceChanged, expectedWorkspace, actualWorkspace)
	}
	operation, err := a.acquireWorkspaceOperation(ctx, actualWorkspace, true)
	if err != nil {
		return "", err
	}
	defer operation.Release()

	a.mu.RLock()
	if lifecycleWorkspaceKey(a.workspace) != lifecycleWorkspaceKey(actualWorkspace) {
		current := a.workspace
		a.mu.RUnlock()
		return "", fmt.Errorf("%w: expected=%q actual=%q", ErrWorkspaceChanged, actualWorkspace, current)
	}
	versionService := a.versionService
	settings := versionAutoSettingsForConfig(a.cfg)
	stateRoot := ""
	if a.cfg != nil {
		stateRoot = a.cfg.ProjectStateDir
	}
	a.mu.RUnlock()

	service, err := workspaceChangeService(actualWorkspace, stateRoot)
	if err != nil {
		return "", err
	}
	hooks, err := action(service)
	if err != nil {
		return "", err
	}
	if err := operation.Context().Err(); err != nil {
		return "", err
	}
	if hooks.ScheduleAutoVersion {
		scheduleAutoVersion(versionService, settings)
	}
	if strings.TrimSpace(hooks.AutomationSource) != "" && len(hooks.Paths) > 0 {
		a.Automation().CheckTriggersAfterWorkspaceMutation(operation.Context(), hooks.AutomationSource, hooks.Paths)
	}
	return actualWorkspace, nil
}

// WithProjectChangeService runs one read/review operation against an explicit
// stable Project identity. It never consults or changes the foreground Book.
func (a *App) WithProjectChangeService(
	ctx context.Context,
	projectID string,
	action func(*workspacechange.Service) error,
) (projectdomain.Layout, error) {
	operation, err := a.AcquireProjectOperation(ctx, projectID)
	if err != nil {
		return projectdomain.Layout{}, err
	}
	defer operation.Release()
	layout := operation.Layout()
	service, err := workspaceChangeService(layout.ContentRoot, layout.StateRoot)
	if err != nil {
		return projectdomain.Layout{}, err
	}
	if err := action(service); err != nil {
		return projectdomain.Layout{}, err
	}
	if err := operation.Context().Err(); err != nil {
		return projectdomain.Layout{}, err
	}
	return layout, nil
}

// WithProjectChangeMutation keeps the durable change, version scheduling, and
// automation trigger admission bound to one Project generation.
func (a *App) WithProjectChangeMutation(
	ctx context.Context,
	projectID string,
	action func(*workspacechange.Service) (WorkspaceChangeMutationHooks, error),
) (projectdomain.Layout, error) {
	a.ensureServices()
	operation, err := a.AcquireProjectOperation(ctx, projectID)
	if err != nil {
		return projectdomain.Layout{}, err
	}
	defer operation.Release()
	layout := operation.Layout()
	service, err := workspaceChangeService(layout.ContentRoot, layout.StateRoot)
	if err != nil {
		return projectdomain.Layout{}, err
	}
	hooks, err := action(service)
	if err != nil {
		return projectdomain.Layout{}, err
	}
	if err := operation.Context().Err(); err != nil {
		return projectdomain.Layout{}, err
	}
	if hooks.ScheduleAutoVersion {
		if err := a.ProjectFiles().ScheduleAutoVersion(layout.ProjectID); err != nil {
			return projectdomain.Layout{}, err
		}
	}
	if strings.TrimSpace(hooks.AutomationSource) != "" && len(hooks.Paths) > 0 {
		a.Automation().CheckTriggersAfterProjectMutation(
			operation.Context(), layout.ProjectID, hooks.AutomationSource, hooks.Paths,
		)
	}
	return layout, nil
}
