package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"denova/internal/workspacechange"
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
	workspace := a.Workspace()
	if workspace == "" {
		return nil, ErrNoWorkspace
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
	service, err := workspacechange.ForWorkspace(actualWorkspace)
	if err != nil {
		return err
	}
	return action(service)
}

// WorkspaceChangeMutationHooks describes post-mutation work that must stay
// bound to the same workspace lease as the durable change operation.
type WorkspaceChangeMutationHooks struct {
	CreateTimedVersion bool
	AutomationSource   string
	Paths              []string
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
	a.mu.RLock()
	defer a.mu.RUnlock()

	actualWorkspace := strings.TrimSpace(a.workspace)
	expectedWorkspace = strings.TrimSpace(expectedWorkspace)
	if actualWorkspace == "" {
		return "", ErrNoWorkspace
	}
	if expectedWorkspace == "" || filepath.Clean(expectedWorkspace) != filepath.Clean(actualWorkspace) {
		return "", fmt.Errorf("%w: expected=%q actual=%q", ErrWorkspaceChanged, expectedWorkspace, actualWorkspace)
	}
	service, err := workspacechange.ForWorkspace(actualWorkspace)
	if err != nil {
		return "", err
	}
	hooks, err := action(service)
	if err != nil {
		return "", err
	}
	if hooks.CreateTimedVersion {
		maybeCreateTimedVersion(a.versionService, versionAutoSettingsForConfig(a.cfg))
	}
	if strings.TrimSpace(hooks.AutomationSource) != "" && len(hooks.Paths) > 0 {
		if automation := a.automationSnapshotLocked(); automation != nil {
			automation.CheckTriggersAfterWorkspaceMutation(ctx, hooks.AutomationSource, hooks.Paths)
		}
	}
	return actualWorkspace, nil
}
