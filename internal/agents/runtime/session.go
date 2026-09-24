package agentruntime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"denova/config"
	"denova/internal/agents/conversationconfig"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/runtime/external"
	"denova/internal/agents/session"
	agent "github.com/alfredxw/denova/agent"
)

// Session is the application control boundary for a bound product conversation.
// It holds no additional state: commands and Goal mutations go to their owner.
// Product turn preparation/commit stay with writing and game; protocol details
// and context maintenance stay with the selected runtime adapter.
type Session struct {
	options    agentrun.Options
	native     *agentexecution.Runtime
	control    *ExternalController
	operations *external.Service
	journal    *session.Session
	engines    *Engines
	selection  config.RuntimeSelection
}

func NativeSession(runtime *agentexecution.Runtime, options agentrun.Options) *Session {
	return &Session{native: runtime, options: options}
}

func (engines *Engines) ConversationSession(runtime *agentexecution.Runtime, options agentrun.Options, journal *session.Session, selection config.RuntimeSelection) (*Session, error) {
	var bound *Session
	if selection.Kind == config.RuntimeNative {
		bound = NativeSession(runtime, options)
	} else {
		state, err := SessionState(options, journal)
		if err != nil {
			return nil, err
		}
		bound, err = engines.ExternalSession(options, state, journal)
		if err != nil {
			return nil, err
		}
	}
	bound.engines, bound.journal, bound.selection = engines, journal, selection
	return bound, nil
}

func (engines *Engines) ExternalSession(options agentrun.Options, state ProductState, journal *session.Session) (*Session, error) {
	control, err := engines.ExternalControl(options, state)
	if err != nil {
		return nil, err
	}
	return &Session{options: options, control: control, operations: &engines.Operations, journal: journal}, nil
}

func (bound *Session) Status(ctx context.Context) (status agentrun.RuntimeStatus, err error) {
	defer func() {
		if err != nil && !errors.Is(err, agentexecution.ErrRuntimeProjectionUnavailable) {
			slog.WarnContext(ctx, "Agent session status unavailable", "agent_kind", bound.options.AgentKind, "project_id", bound.options.ProjectID,
				"session_id", bound.options.SessionID, "story_id", bound.options.StoryID, "branch_id", bound.options.BranchID, "error", err)
		}
	}()
	if bound.control == nil {
		return bound.native.RuntimeStatusProjection(ctx, bound.options)
	}
	status, err = bound.control.Status(ctx)
	if err == nil && bound.journal != nil {
		var unit agentrun.RuntimeStatus
		unit, _, err = bound.operations.Status(ctx, bound.options.ProjectID, bound.journal)
		status.PendingInteractions, status.OpenToolCalls = unit.PendingInteractions, unit.OpenToolCalls
	}
	return status, err
}

func (bound *Session) Submit(ctx context.Context, command Command, emit func(agentrun.Event)) (agentrun.CommandReceipt, error) {
	request := agentexecution.CommandRequest{
		Kind: command.Kind, CommandID: command.CommandID, OperationID: command.OperationID,
		TargetCommandID: command.TargetCommandID, Reason: command.Reason, Options: bound.options,
	}
	switch command.Kind {
	case agentexecution.CommandAbort, agentexecution.CommandSuspend, agentexecution.CommandSteerQueued, agentexecution.CommandCancelQueued:
	case agentexecution.CommandSteer, agentexecution.CommandFollowUp, agentexecution.CommandNextTurn:
		request.AfterOperationID, request.Request, request.Emit = command.OperationID, command.Input, emit
	default:
		return agentrun.CommandReceipt{}, fmt.Errorf("%w: unsupported session command %q", agentrun.ErrInvalidCommand, command.Kind)
	}
	if bound.control != nil {
		return bound.control.Submit(ctx, command)
	}
	return bound.native.SubmitCommand(ctx, request)
}

func (bound *Session) Goal(ctx context.Context) (agent.GoalState, bool, error) {
	if !supportsGoal(bound.options.AgentKind) {
		return agent.GoalState{}, false, conversationconfig.ErrRuntimeCapabilityUnsupported
	}
	if bound.control != nil {
		return bound.control.store.Goal(ctx)
	}
	return bound.native.Goal(ctx, bound.options)
}

func (bound *Session) UpdateGoal(ctx context.Context, mutation agent.GoalMutation) (agent.GoalState, error) {
	if !supportsGoal(bound.options.AgentKind) {
		return agent.GoalState{}, conversationconfig.ErrRuntimeCapabilityUnsupported
	}
	// Goal mutations share the same configuration fence as turn admission.
	// Holding an old Session handle must not mutate a newly selected runtime.
	if bound.engines != nil && bound.journal != nil {
		release, err := bound.engines.AdmitExecution(ctx, bound.journal, &bound.selection)
		if err != nil {
			return agent.GoalState{}, err
		}
		defer release()
	}
	if bound.control != nil {
		return bound.control.store.UpdateGoal(ctx, mutation)
	}
	return bound.native.UpdateGoal(ctx, bound.options, mutation)
}

// GoalMutation is shared by every UI surface; revision checking is performed by
// the selected state owner, never by a stale frontend projection.
func GoalMutation(action, objective string, revision uint64) (agent.GoalMutation, error) {
	mutation := agent.GoalMutation{ExpectedRevision: revision}
	switch action {
	case "set":
		mutation.Kind, mutation.Objective = agent.GoalSet, objective
	case "pause":
		mutation.Kind = agent.GoalPause
	case "resume":
		mutation.Kind = agent.GoalResume
	case "clear":
		mutation.Kind = agent.GoalClear
	default:
		return agent.GoalMutation{}, fmt.Errorf("unsupported goal action %q", action)
	}
	return mutation, nil
}
