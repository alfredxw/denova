package app

import (
	"fmt"
	"path/filepath"
	"strings"

	workspacechange "denova/internal/workspace/change"
	"denova/internal/workspace/documentreview"
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
	action func(*workspacechange.Service, *documentreview.Service, documentreview.SnapshotResolver) error,
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
		stateRoot := ""
		if a.cfg != nil {
			stateRoot = a.cfg.ProjectStateDir
		}
		changes, err = workspaceChangeService(actualWorkspace, stateRoot)
		if err != nil {
			return err
		}
	}
	if scope.documents {
		if a.bookService == nil {
			return ErrNoWorkspace
		}
		documents, err = documentreview.ForWorkspace(actualWorkspace)
		if err != nil {
			return err
		}
	}
	return action(changes, documents, newDocumentReviewTargetResolver(actualWorkspace, a.bookService))
}

// withRuntimeReviewFeedbackServices resolves ledgers from an explicitly
// captured runtime. AgentChat projects are independent from the foreground
// Writing selection, so they must never consult App.workspace here.
func withRuntimeReviewFeedbackServices(
	runtime ideChatRuntime,
	scope reviewFeedbackServiceScope,
	action func(*workspacechange.Service, *documentreview.Service, documentreview.SnapshotResolver) error,
) error {
	workspace := strings.TrimSpace(runtime.workspace)
	if workspace == "" {
		return ErrNoWorkspace
	}
	var changes *workspacechange.Service
	var documents *documentreview.Service
	var err error
	if scope.workspaceChanges {
		if strings.TrimSpace(runtime.projectState) != "" {
			changes, err = workspacechange.ForWorkspaceAt(workspace, runtime.projectState)
		} else {
			changes, err = workspacechange.ForWorkspace(workspace)
		}
		if err != nil {
			return err
		}
	}
	if scope.documents {
		if runtime.state == nil || runtime.bookService == nil {
			return invalidReviewFeedbackError("document review is available only in Book projects", nil)
		}
		documents, err = documentreview.ForWorkspace(workspace)
		if err != nil {
			return err
		}
	}
	return action(changes, documents, newDocumentReviewTargetResolver(workspace, runtime.bookService))
}
