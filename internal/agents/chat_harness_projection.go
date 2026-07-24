package agents

import (
	"context"
	"errors"
	"fmt"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

// ErrRuntimeProjectionUnavailable means this ChatService is nil or was not
// constructed with the mandatory durable command harness.
var ErrRuntimeProjectionUnavailable = errors.New("durable agent runtime projection is unavailable")

// RuntimeStatusProjection returns a bounded point-in-time display projection.
// It cannot carry durable messages and is never used as model context.
func (s *ChatService) RuntimeStatusProjection(ctx context.Context, options RunOptions) (RuntimeStatus, error) {
	if s == nil || s.harness == nil || s.harness.runtime == nil {
		return RuntimeStatus{}, ErrRuntimeProjectionUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	binding, err := harnessBindingForOptions(options)
	if err != nil {
		return RuntimeStatus{}, fmt.Errorf("derive agent runtime projection binding: %w", err)
	}
	status, err := s.harness.runtime.Project(ctx, binding)
	if err != nil {
		return RuntimeStatus{}, fmt.Errorf("project agent runtime status: %w", err)
	}
	projected, err := runtimeStatusFromSnapshot(status)
	if err != nil {
		return RuntimeStatus{}, err
	}
	return projected, nil
}

func (s *ChatService) closeRuntimeBindings(ctx context.Context, selector runstate.BindingSelector) error {
	if s == nil || s.harness == nil || s.harness.runtime == nil {
		return ErrRuntimeProjectionUnavailable
	}
	return s.harness.runtime.CloseBindings(ctx, selector)
}

// CloseWorkspaceBindings evicts every durable Agent binding rooted in one
// workspace after the app has drained that workspace generation.
func (s *ChatService) CloseWorkspaceBindings(ctx context.Context, workspace string) error {
	selector, err := runtimeWorkspaceBindingSelector(workspace)
	if err != nil {
		return err
	}
	return s.closeRuntimeBindings(ctx, selector)
}

// CloseSessionBindings evicts one session-backed Agent binding.
func (s *ChatService) CloseSessionBindings(ctx context.Context, agentKind, workspace, sessionID string) error {
	selector, err := runtimeSessionBindingSelector(agentKind, workspace, sessionID)
	if err != nil {
		return err
	}
	return s.closeRuntimeBindings(ctx, selector)
}

// CloseStoryBindings evicts every Agent binding for an exact story scope.
func (s *ChatService) CloseStoryBindings(ctx context.Context, workspace, storyID, branchID string) error {
	selector, err := runtimeStoryBindingSelector(workspace, storyID, branchID)
	if err != nil {
		return err
	}
	return s.closeRuntimeBindings(ctx, selector)
}
