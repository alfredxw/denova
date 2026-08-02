package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	agentchat "denova/internal/agents/chat"
	agentrun "denova/internal/agents/run"
	reviewapp "denova/internal/app/review"
	"denova/internal/workspace/documentreview"
)

func (service *ChatAppService) resolveReviewFeedback(ctx context.Context, runtime ideChatRuntime, request *agentchat.ChatRequest) error {
	return reviewapp.Resolve(ctx, reviewRuntime(runtime), request)
}

func (service *ChatAppService) consumeResolvedReviewFeedback(ctx context.Context, runtime ideChatRuntime, request agentchat.ChatRequest) error {
	return reviewapp.Consume(ctx, reviewRuntime(runtime), request)
}

func (service *ChatAppService) bindReviewFeedbackInputCommit(options agentrun.Options, runtime ideChatRuntime, request agentchat.ChatRequest) agentrun.Options {
	return reviewapp.BindInputCommit(options, reviewRuntime(runtime), request)
}

func reviewRuntime(runtime ideChatRuntime) reviewapp.Runtime {
	sessionID := ""
	if runtime.sess != nil {
		sessionID = runtime.sess.ID
	}
	return reviewapp.Runtime{
		Workspace: runtime.workspace, StateRoot: runtime.projectState, SessionID: sessionID,
		DocumentsEnabled: runtime.state != nil, BookService: runtime.bookService,
	}
}

// WithDocumentReviewService keeps workspace identity, resource resolution, and
// the comment ledger under one runtime lease.
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
	resolver := reviewapp.NewTargetResolver(actualWorkspace, a.bookService)
	if err := action(service, resolver); err != nil {
		return "", err
	}
	return actualWorkspace, nil
}
