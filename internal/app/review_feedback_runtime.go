package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"denova/internal/documentreview"
	"denova/internal/workspacechange"
)

type reviewFeedbackServiceScope struct {
	workspaceChanges bool
	documents        bool
}

// withReviewFeedbackServices holds one workspace lease while a mixed feedback
// batch validates, consumes, or compensates both independent review ledgers.
func (a *App) withReviewFeedbackServices(
	expectedWorkspace string,
	scope reviewFeedbackServiceScope,
	action func(*workspacechange.Service, *documentreview.Service) error,
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

	var changes *workspacechange.Service
	var documents *documentreview.Service
	var err error
	if scope.workspaceChanges {
		changes, err = workspacechange.ForWorkspace(actualWorkspace)
		if err != nil {
			return err
		}
	}
	if scope.documents {
		documents, err = documentreview.ForWorkspace(actualWorkspace)
		if err != nil {
			return err
		}
	}
	return action(changes, documents)
}
