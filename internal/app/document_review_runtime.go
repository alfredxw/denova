package app

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"denova/internal/documentreview"
)

// WithDocumentReviewService keeps the workspace identity, resource resolver,
// and comment ledger under one runtime lease. This prevents a workspace switch
// from binding a comment to a snapshot from another book.
func (a *App) WithDocumentReviewService(
	expectedWorkspace string,
	action func(*documentreview.Service, documentreview.SnapshotResolver) error,
) (string, error) {
	if action == nil {
		return "", errors.New("document review action is nil")
	}
	a.mu.RLock()
	defer a.mu.RUnlock()

	actualWorkspace := strings.TrimSpace(a.workspace)
	expectedWorkspace = strings.TrimSpace(expectedWorkspace)
	if actualWorkspace == "" || a.bookService == nil {
		return "", ErrNoWorkspace
	}
	if expectedWorkspace == "" || filepath.Clean(expectedWorkspace) != filepath.Clean(actualWorkspace) {
		return "", fmt.Errorf("%w: expected=%q actual=%q", ErrWorkspaceChanged, expectedWorkspace, actualWorkspace)
	}
	service, err := documentreview.ForWorkspace(actualWorkspace)
	if err != nil {
		return "", err
	}
	resolver := newDocumentReviewTargetResolver(actualWorkspace, a.bookService)
	if err := action(service, resolver); err != nil {
		return "", err
	}
	return actualWorkspace, nil
}
