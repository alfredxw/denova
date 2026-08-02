package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"denova/internal/app/task"
	booklore "denova/internal/book/lore"
)

// loreHost keeps workspace identity and task ownership in the composition
// root while the lore package owns all catalog behavior.
type loreHost struct {
	app *App
}

func (host loreHost) WithLoreStore(expectedWorkspace string, action func(*booklore.Store) error) (string, error) {
	if action == nil {
		return "", errors.New("lore action is nil")
	}
	app := host.app
	if app == nil {
		return "", ErrNoWorkspace
	}
	app.mu.RLock()
	defer app.mu.RUnlock()

	actualWorkspace := strings.TrimSpace(app.workspace)
	expectedWorkspace = strings.TrimSpace(expectedWorkspace)
	if actualWorkspace == "" {
		return "", ErrNoWorkspace
	}
	if expectedWorkspace != "" && filepath.Clean(expectedWorkspace) != filepath.Clean(actualWorkspace) {
		return "", fmt.Errorf("%w: expected=%q actual=%q", ErrWorkspaceChanged, expectedWorkspace, actualWorkspace)
	}
	if err := action(booklore.NewStore(actualWorkspace)); err != nil {
		return "", err
	}
	return actualWorkspace, nil
}

func (host loreHost) ValidateLoreWorkspace(expectedWorkspace string) (string, error) {
	app := host.app
	if app == nil {
		return "", ErrNoWorkspace
	}
	app.mu.RLock()
	defer app.mu.RUnlock()
	actualWorkspace := strings.TrimSpace(app.workspace)
	expectedWorkspace = strings.TrimSpace(expectedWorkspace)
	if actualWorkspace == "" {
		return "", ErrNoWorkspace
	}
	if expectedWorkspace == "" || filepath.Clean(expectedWorkspace) != filepath.Clean(actualWorkspace) {
		return "", fmt.Errorf("%w: expected=%q actual=%q", ErrWorkspaceChanged, expectedWorkspace, actualWorkspace)
	}
	return actualWorkspace, nil
}

func (host loreHost) RegisterLoreTask(backgroundTask *task.Task, expectedWorkspace string) (string, error) {
	app := host.app
	if app == nil {
		return "", ErrNoWorkspace
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.workspaceTransition {
		return "", ErrWorkspaceTransition
	}
	workspace := strings.TrimSpace(app.workspace)
	if workspace == "" {
		return "", ErrNoWorkspace
	}
	if expectedWorkspace = strings.TrimSpace(expectedWorkspace); expectedWorkspace != "" && lifecycleWorkspaceKey(expectedWorkspace) != lifecycleWorkspaceKey(workspace) {
		return "", fmt.Errorf("%w: expected=%q actual=%q", ErrWorkspaceChanged, expectedWorkspace, workspace)
	}
	if err := app.registerWorkspaceTaskLocked(backgroundTask, workspace, true); err != nil {
		return "", err
	}
	return workspace, nil
}

func (host loreHost) UnregisterLoreTask(backgroundTask *task.Task) {
	if host.app != nil {
		host.app.unregisterWorkspaceTask(backgroundTask)
	}
}

func (host loreHost) ClassifyLoreItems(ctx context.Context, inputs []booklore.ClassificationInput) ([]booklore.ClassificationSuggestion, error) {
	if host.app == nil {
		return nil, ErrNoWorkspace
	}
	return host.app.ClassifyLoreItems(ctx, inputs)
}

// WithLoreStore binds a short lore read or mutation to the workspace identity
// supplied by the client. Workspace switches take the write lock and cannot
// redirect an in-flight edit into another book.
func (app *App) WithLoreStore(expectedWorkspace string, action func(*booklore.Store) error) (string, error) {
	if _, err := (loreHost{app: app}).ValidateLoreWorkspace(expectedWorkspace); err != nil {
		return "", err
	}
	return (loreHost{app: app}).WithLoreStore(expectedWorkspace, action)
}
