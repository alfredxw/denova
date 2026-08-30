package execution

import (
	"context"
	"denova/internal/agents/run"
	"errors"

	agent "github.com/alfredxw/denova/agent"
)

// ErrRuntimeProjectionUnavailable means this Service has no live Agent owner.
var ErrRuntimeProjectionUnavailable = errors.New("agent runtime projection is unavailable")

// RuntimeStatusProjection returns a bounded point-in-time display projection.
// It cannot carry durable messages and is never used as model context.
func (s *Runtime) RuntimeStatusProjection(ctx context.Context, options agentrun.Options) (agentrun.RuntimeStatus, error) {
	if s == nil || s.public == nil {
		return agentrun.RuntimeStatus{}, ErrRuntimeProjectionUnavailable
	}
	return s.public.status(ctx, options)
}

// Goal returns the public Agent-owned Goal state for one exact Denova binding.
func (s *Runtime) Goal(ctx context.Context, options agentrun.Options) (agent.GoalState, bool, error) {
	if s == nil || s.public == nil {
		return agent.GoalState{}, false, ErrRuntimeProjectionUnavailable
	}
	return s.public.goal(ctx, options)
}

// UpdateGoal applies one revisioned mutation through the public Session
// capability. Product stores never mirror this state.
func (s *Runtime) UpdateGoal(ctx context.Context, options agentrun.Options, mutation agent.GoalMutation) (agent.GoalState, error) {
	if s == nil || s.public == nil {
		return agent.GoalState{}, ErrRuntimeProjectionUnavailable
	}
	return s.public.updateGoal(ctx, options, mutation)
}

// ClearSession resets the public transcript and clear-scoped capabilities for
// one exact product binding while preserving the Session identity and Goal.
func (s *Runtime) ClearSession(ctx context.Context, options agentrun.Options) error {
	if s == nil || s.public == nil {
		return ErrRuntimeProjectionUnavailable
	}
	return s.public.clearSession(ctx, options)
}

func (s *Runtime) ResolveInteraction(
	ctx context.Context,
	options agentrun.Options,
	interactionID string,
	response agent.InteractionResponse,
) (agent.InteractionRequest, agent.InteractionResolution, error) {
	if s == nil || s.public == nil {
		return agent.InteractionRequest{}, agent.InteractionResolution{}, ErrRuntimeProjectionUnavailable
	}
	return s.public.resolveInteraction(ctx, options, interactionID, response)
}

func (s *Runtime) closeRuntimeBindings(ctx context.Context, selector agent.SessionSelector) error {
	if s == nil || s.public == nil {
		return ErrRuntimeProjectionUnavailable
	}
	return s.public.closeSessions(ctx, selector)
}

func (s *Runtime) deleteRuntimeBindings(ctx context.Context, selector agent.SessionSelector) error {
	if s == nil || s.public == nil {
		return ErrRuntimeProjectionUnavailable
	}
	return s.public.deleteSessions(ctx, selector)
}

// CloseForegroundProjectBindings evicts bindings owned by the foreground
// Project runtime while deliberately preserving user-level AgentChat runs.
// AgentChat reuses the IDE implementation, but its distinct durable profile is
// allowed to keep running when the title-bar book selection changes.
func (s *Runtime) CloseForegroundProjectBindings(ctx context.Context, projectID string) error {
	selectors, err := agentrun.ForegroundProjectBindingSelectors(projectID)
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

// CloseProjectSessionBindings evicts one stable Project conversation. The
// project ID remains the durable owner even when its content path is relinked.
func (s *Runtime) CloseProjectSessionBindings(ctx context.Context, projectID, sessionID string) error {
	selector, err := agentrun.ProjectSessionBindingSelector(projectID, sessionID)
	if err != nil {
		return err
	}
	return s.closeRuntimeBindings(ctx, selector)
}

func (s *Runtime) DeleteProjectSessionBindings(ctx context.Context, projectID, sessionID string) error {
	selector, err := agentrun.ProjectSessionBindingSelector(projectID, sessionID)
	if err != nil {
		return err
	}
	return s.deleteRuntimeBindings(ctx, selector)
}

// CloseProjectBindings evicts all runtime actors owned by a Project.
func (s *Runtime) CloseProjectBindings(ctx context.Context, projectID string) error {
	selector, err := agentrun.ProjectBindingSelector(projectID)
	if err != nil {
		return err
	}
	return s.closeRuntimeBindings(ctx, selector)
}

func (s *Runtime) DeleteProjectBindings(ctx context.Context, projectID string) error {
	selector, err := agentrun.ProjectBindingSelector(projectID)
	if err != nil {
		return err
	}
	return s.deleteRuntimeBindings(ctx, selector)
}

// CloseSessionBindings evicts one session-backed Agent binding.
func (s *Runtime) CloseSessionBindings(ctx context.Context, agentKind, projectID, sessionID string) error {
	selector, err := agentrun.SessionBindingSelector(agentKind, projectID, sessionID)
	if err != nil {
		return err
	}
	return s.closeRuntimeBindings(ctx, selector)
}

func (s *Runtime) DeleteSessionBindings(ctx context.Context, agentKind, projectID, sessionID string) error {
	selector, err := agentrun.SessionBindingSelector(agentKind, projectID, sessionID)
	if err != nil {
		return err
	}
	return s.deleteRuntimeBindings(ctx, selector)
}

// CloseStoryBindings evicts every Agent binding for an exact story scope.
func (s *Runtime) CloseStoryBindings(ctx context.Context, projectID, storyID, branchID string) error {
	return s.forEachStorySelector(projectID, storyID, branchID, func(selector agent.SessionSelector) error {
		return s.closeRuntimeBindings(ctx, selector)
	})
}

// DeleteStoryBindings permanently removes every public game/director Session
// in the selected story or branch scope.
func (s *Runtime) DeleteStoryBindings(ctx context.Context, projectID, storyID, branchID string) error {
	return s.forEachStorySelector(projectID, storyID, branchID, func(selector agent.SessionSelector) error {
		return s.deleteRuntimeBindings(ctx, selector)
	})
}

func (s *Runtime) forEachStorySelector(projectID, storyID, branchID string, apply func(agent.SessionSelector) error) error {
	base, err := agentrun.StoryBindingSelector(projectID, storyID, branchID)
	if err != nil {
		return err
	}
	for _, kind := range []string{agentrun.AgentKindInteractiveStory} {
		profile, err := agentrun.BindingSelector(kind, "")
		if err != nil {
			return err
		}
		selector := base
		selector.Namespace = profile.Namespace
		if err := apply(selector); err != nil {
			return err
		}
	}
	return nil
}
