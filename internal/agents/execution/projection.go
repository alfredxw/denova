package execution

import (
	"context"
	"denova/internal/agents/run"
	"errors"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

// ErrRuntimeProjectionUnavailable means this Service is nil or was not
// constructed with the mandatory durable command harness.
var ErrRuntimeProjectionUnavailable = errors.New("durable agent runtime projection is unavailable")

// RuntimeStatusProjection returns a bounded point-in-time display projection.
// It cannot carry durable messages and is never used as model context.
func (s *Runtime) RuntimeStatusProjection(ctx context.Context, options agentrun.Options) (agentrun.RuntimeStatus, error) {
	if s == nil || s.public == nil {
		return agentrun.RuntimeStatus{}, ErrRuntimeProjectionUnavailable
	}
	return s.public.status(ctx, options)
}

func (s *Runtime) closeRuntimeBindings(ctx context.Context, selector runstate.BindingSelector) error {
	if s == nil || s.public == nil {
		return ErrRuntimeProjectionUnavailable
	}
	return s.public.closeSessions(ctx, selector)
}

// CloseWorkspaceBindings evicts every durable Agent binding rooted in one
// workspace after the app has drained that workspace generation.
func (s *Runtime) CloseWorkspaceBindings(ctx context.Context, workspace string) error {
	selector, err := agentrun.WorkspaceBindingSelector(workspace)
	if err != nil {
		return err
	}
	return s.closeRuntimeBindings(ctx, selector)
}

// CloseForegroundWorkspaceBindings evicts bindings owned by the foreground
// workspace runtime while deliberately preserving user-level AgentChat runs.
// AgentChat reuses the IDE implementation, but its distinct durable profile is
// allowed to keep running when the title-bar book selection changes.
func (s *Runtime) CloseForegroundWorkspaceBindings(ctx context.Context, workspace string) error {
	selectors, err := agentrun.ForegroundWorkspaceBindingSelectors(workspace)
	if err != nil {
		return err
	}
	for _, selector := range selectors {
		if err := s.closeRuntimeBindings(ctx, selector); err != nil {
			return err
		}
	}
	return nil
}

// CloseAgentChatSessionBindings evicts one user-level project conversation.
func (s *Runtime) CloseAgentChatSessionBindings(ctx context.Context, workspace, sessionID string) error {
	selector, err := agentrun.AgentChatSessionBindingSelector(workspace, sessionID)
	if err != nil {
		return err
	}
	return s.closeRuntimeBindings(ctx, selector)
}

// CloseProjectSessionBindings evicts one stable Project conversation. The
// project ID remains the durable owner even when its content path is relinked.
func (s *Runtime) CloseProjectSessionBindings(ctx context.Context, projectID, sessionID string) error {
	selector, err := agentrun.ProjectSessionBindingSelector(projectID, sessionID)
	if err != nil {
		return err
	}
	return s.closeRuntimeBindings(ctx, selector)
}

// CloseProjectBindings evicts all AgentChat runtime actors owned by a Project.
func (s *Runtime) CloseProjectBindings(ctx context.Context, projectID string) error {
	selector, err := agentrun.ProjectBindingSelector(projectID)
	if err != nil {
		return err
	}
	return s.closeRuntimeBindings(ctx, selector)
}

// CloseSessionBindings evicts one session-backed Agent binding.
func (s *Runtime) CloseSessionBindings(ctx context.Context, agentKind, workspace, sessionID string) error {
	selector, err := agentrun.SessionBindingSelector(agentKind, workspace, sessionID)
	if err != nil {
		return err
	}
	return s.closeRuntimeBindings(ctx, selector)
}

// CloseStoryBindings evicts every Agent binding for an exact story scope.
func (s *Runtime) CloseStoryBindings(ctx context.Context, workspace, storyID, branchID string) error {
	selector, err := agentrun.StoryBindingSelector(workspace, storyID, branchID)
	if err != nil {
		return err
	}
	selector.Profile = string(ProfileGame)
	return s.closeRuntimeBindings(ctx, selector)
}
