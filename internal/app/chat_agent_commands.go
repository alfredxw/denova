package app

import (
	"context"
	"errors"
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
	runtime, task, err := s.commandRuntime(ctx)
	if err != nil {
		return agentrun.CommandReceipt{}, err
	}
	taskID := ""
	var emit func(agentrun.Event)
	if task != nil {
		taskID, emit = task.ID(), task.Emit
	}
	if command.Kind == agentexecution.CommandAbort || command.Kind == agentexecution.CommandSuspend || command.Kind == agentexecution.CommandSteerQueued || command.Kind == agentexecution.CommandCancelQueued {
		return runtime.executionRuntime.SubmitCommand(ctx, agentexecution.CommandRequest{
			Kind: command.Kind, CommandID: command.CommandID,
			OperationID: command.OperationID, TargetCommandID: command.TargetCommandID, Reason: command.Reason,
			Options: runtime.agentOptions(taskID),
		})
	}
	if command.Kind != agentexecution.CommandSteer && command.Kind != agentexecution.CommandFollowUp && command.Kind != agentexecution.CommandNextTurn {
		return agentrun.CommandReceipt{}, fmt.Errorf("%w: unsupported writing command %q", agentrun.ErrInvalidCommand, command.Kind)
	}
	return runtime.executionRuntime.SubmitCommand(ctx, agentexecution.CommandRequest{
		Kind: command.Kind, CommandID: command.CommandID,
		OperationID: command.OperationID, AfterOperationID: command.OperationID,
		Request: command.Input, Emit: emit,
		Options: runtime.agentOptions(taskID),
	})
}

// A paused logical Run remains addressable after its display task closes.
func (s *ChatAppService) commandRuntime(ctx context.Context) (ideChatRuntime, *apptask.Task, error) {
	runtime, task, err := s.activeCommandRuntime()
	if !errors.Is(err, ErrNoActiveAgentOperation) {
		return runtime, task, err
	}
	a := s.app
	a.mu.RLock()
	if a.cfg == nil || a.session == nil || a.executionRuntime == nil || a.workspaceTransition {
		a.mu.RUnlock()
		return ideChatRuntime{}, nil, ErrAgentContextChanged
	}
	runtime = ideChatRuntime{app: a, projectID: a.cfg.ProjectID, projectStore: a.cfg.ProjectStoreDir,
		workspace: a.workspace, sess: a.session, state: a.bookState, executionRuntime: a.executionRuntime}
	a.mu.RUnlock()
	status, err := runtime.executionRuntime.RuntimeStatusProjection(ctx, runtime.agentOptions(""))
	if err != nil {
		return ideChatRuntime{}, nil, err
	}
	if status.Phase != agentrun.RunPhaseSuspended {
		return ideChatRuntime{}, nil, ErrNoActiveAgentOperation
	}
	return runtime, nil, nil
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
