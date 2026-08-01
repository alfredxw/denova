package app

import (
	"denova/internal/book/lore"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// WithLoreStore binds a short lore read or mutation to the workspace identity
// supplied by the client. Workspace switches take the write lock and therefore
// cannot redirect an in-flight edit into another book.
func (a *App) WithLoreStore(expectedWorkspace string, action func(*lore.Store) error) (string, error) {
	if action == nil {
		return "", errors.New("lore action is nil")
	}
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
	if err := action(lore.NewStore(actualWorkspace)); err != nil {
		return "", err
	}
	return actualWorkspace, nil
}

// ValidateWorkspaceIdentity is for operations that acquire their own durable
// workspace runtime after request validation (for example image generation).
func (a *App) ValidateWorkspaceIdentity(expectedWorkspace string) error {
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
	return nil
}
