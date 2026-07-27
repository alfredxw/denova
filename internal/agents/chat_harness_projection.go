package agents

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

// CloseForegroundWorkspaceBindings evicts bindings owned by the foreground
// workspace runtime while deliberately preserving user-level AgentChat runs.
// AgentChat reuses the IDE implementation, but its distinct durable profile is
// allowed to keep running when the title-bar book selection changes.
func (s *ChatService) CloseForegroundWorkspaceBindings(ctx context.Context, workspace string) error {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return runstate.ErrInvalidBinding
	}
	profiles := []struct {
		kind    string
		profile string
	}{
		{runtimeBindingKindWriting, runtimeBindingProfileWriting},
		{runtimeBindingKindWriting, runtimeBindingProfileConfigManager},
		{runtimeBindingKindWriting, runtimeBindingProfileImage},
		{runtimeBindingKindGame, runtimeBindingProfileGame},
		{runtimeBindingKindGame, runtimeBindingProfileDirector},
		{runtimeBindingKindAutomation, runtimeBindingProfileAutomation},
	}
	for _, candidate := range profiles {
		if err := s.closeRuntimeBindings(ctx, runstate.BindingSelector{
			Kind: candidate.kind, Profile: candidate.profile,
			Labels: map[string]string{runtimeBindingLabelWorkspace: workspace},
		}); err != nil {
			return err
		}
	}
	return nil
}

// CloseAgentChatSessionBindings evicts one user-level project conversation.
func (s *ChatService) CloseAgentChatSessionBindings(ctx context.Context, workspace, sessionID string) error {
	workspace = strings.TrimSpace(workspace)
	sessionID = strings.TrimSpace(sessionID)
	if workspace == "" || sessionID == "" {
		return runstate.ErrInvalidBinding
	}
	return s.closeRuntimeBindings(ctx, runstate.BindingSelector{
		Kind: runtimeBindingKindWriting, Profile: runtimeBindingProfileAgentChat, Key: sessionID,
		Labels: map[string]string{
			runtimeBindingLabelWorkspace: workspace,
			runtimeBindingLabelSession:   sessionID,
		},
	})
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
