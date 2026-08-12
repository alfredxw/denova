package app

import (
	"context"
	"fmt"

	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	appagentruntime "denova/internal/app/agentruntime"
	apptask "denova/internal/app/task"
)

var ErrNoActiveAgentOperation = appagentruntime.ErrNoActiveOperation

type ChatAgentCommand = appagentruntime.Command

// SubmitChatAgentCommand adapts a transport command to the active writing
// binding. Workspace/session identity is captured from App state and never
// accepted from the client.
func (a *App) SubmitChatAgentCommand(ctx context.Context, command ChatAgentCommand) (agentrun.CommandReceipt, error) {
	return a.chat().SubmitAgentCommand(ctx, command)
}

func (s *ChatAppService) SubmitAgentCommand(ctx context.Context, command ChatAgentCommand) (agentrun.CommandReceipt, error) {
	return s.submitAgentCommand(ctx, command)
}

// SubmitChatAgentCommandForSession prevents a control intended for the
// visible Session from being delivered to a different foreground runtime.
func (a *App) SubmitChatAgentCommandForSession(ctx context.Context, sessionID string, command ChatAgentCommand) (agentrun.CommandReceipt, error) {
	return a.chat().SubmitAgentCommandForSession(ctx, sessionID, command)
}

func (s *ChatAppService) SubmitAgentCommandForSession(ctx context.Context, sessionID string, command ChatAgentCommand) (agentrun.CommandReceipt, error) {
	s.admission.RLock()
	defer s.admission.RUnlock()
	if err := s.confirmSelectedSessionID(sessionID); err != nil {
		return agentrun.CommandReceipt{}, err
	}
	return s.submitAgentCommand(ctx, command)
}

func (s *ChatAppService) submitAgentCommand(ctx context.Context, command ChatAgentCommand) (agentrun.CommandReceipt, error) {
	if command.Kind == agentexecution.CommandAbort || command.Kind == agentexecution.CommandSteerQueued || command.Kind == agentexecution.CommandCancelQueued {
		runtime, task, err := s.activeCommandRuntime()
		if err != nil {
			return agentrun.CommandReceipt{}, err
		}
		return runtime.executionRuntime.SubmitCommand(ctx, agentexecution.CommandRequest{
			Kind: command.Kind, CommandID: command.CommandID,
			OperationID: command.OperationID, TargetCommandID: command.TargetCommandID, Reason: command.Reason,
			Options: agentrun.Options{
				AgentKind: agentrun.AgentKindIDE, TaskID: task.ID(),
				StateRoot: runtime.projectState,
				SessionID: runtime.sess.ID, Workspace: runtime.workspace, Mode: "ide",
			},
		})
	}
	if command.Kind != agentexecution.CommandSteer && command.Kind != agentexecution.CommandFollowUp && command.Kind != agentexecution.CommandNextTurn {
		return agentrun.CommandReceipt{}, fmt.Errorf("%w: unsupported writing command %q", agentrun.ErrInvalidCommand, command.Kind)
	}
	activeRuntime, task, err := s.activeCommandRuntime()
	if err != nil {
		return agentrun.CommandReceipt{}, err
	}
	return activeRuntime.executionRuntime.SubmitCommand(ctx, agentexecution.CommandRequest{
		Kind: command.Kind, CommandID: command.CommandID,
		OperationID: command.OperationID, AfterOperationID: command.OperationID,
		Request: command.Input, Emit: task.Emit,
		Options: agentrun.Options{
			AgentKind: agentrun.AgentKindIDE,
			StateRoot: activeRuntime.projectState,
			TaskID:    task.ID(),
			SessionID: activeRuntime.sess.ID,
			Workspace: activeRuntime.workspace,
			Mode:      "ide",
		},
	})
}

func (s *ChatAppService) confirmActiveCommandRuntime(expected ideChatRuntime, task *apptask.Task) error {
	current, currentTask, err := s.activeCommandRuntime()
	if err != nil {
		return err
	}
	if currentTask != task || current.workspace != expected.workspace || current.sess != expected.sess || current.state != expected.state || current.executionRuntime != expected.executionRuntime {
		return ErrAgentContextChanged
	}
	return nil
}

func (s *ChatAppService) activeCommandRuntime() (ideChatRuntime, *apptask.Task, error) {
	if s == nil || s.app == nil {
		return ideChatRuntime{}, nil, ErrNoWorkspace
	}
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.workspaceTransition {
		return ideChatRuntime{}, nil, ErrWorkspaceTransition
	}
	if a.session == nil || a.bookState == nil || a.executionRuntime == nil || a.cfg == nil {
		return ideChatRuntime{}, nil, ErrNoWorkspace
	}
	run := a.activeWritingRun
	if run == nil || run.task == nil || run.task.Finished() {
		return ideChatRuntime{}, nil, ErrNoActiveAgentOperation
	}
	return run.runtime, run.task, nil
}
