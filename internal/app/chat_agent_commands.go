package app

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentharness "denova/internal/agents/harness"
	agentrun "denova/internal/agents/run"
	apptask "denova/internal/app/task"
	"errors"
	"fmt"
)

var ErrNoActiveAgentOperation = errors.New("no active agent operation")

type ChatAgentCommand struct {
	Kind            agentharness.CommandKind
	CommandID       string
	OperationID     agentrun.OperationID
	TargetCommandID agentrun.CommandID
	Reason          string
	Input           agentchat.ChatRequest
}

// SubmitChatAgentCommand adapts a transport command to the active writing
// binding. Workspace/session identity is captured from App state and never
// accepted from the client.
func (a *App) SubmitChatAgentCommand(ctx context.Context, command ChatAgentCommand) (agentrun.CommandReceipt, error) {
	return a.chat().SubmitAgentCommand(ctx, command)
}

func (s *ChatAppService) SubmitAgentCommand(ctx context.Context, command ChatAgentCommand) (agentrun.CommandReceipt, error) {
	if command.Kind == agentharness.CommandAbort || command.Kind == agentharness.CommandSteerQueued || command.Kind == agentharness.CommandCancelQueued {
		runtime, task, err := s.activeCommandRuntime()
		if err != nil {
			return agentrun.CommandReceipt{}, err
		}
		return runtime.chatService.SubmitCommand(ctx, agentharness.CommandSpec{
			Kind: command.Kind, CommandID: command.CommandID,
			OperationID: command.OperationID, TargetCommandID: command.TargetCommandID, Reason: command.Reason,
			Options: agentrun.Options{
				AgentKind: agentrun.AgentKindIDE, TaskID: task.ID(),
				StateRoot: runtime.projectState,
				SessionID: runtime.sess.ID, Workspace: runtime.workspace, Mode: "ide",
			},
		})
	}
	if command.Kind != agentharness.CommandSteer && command.Kind != agentharness.CommandFollowUp && command.Kind != agentharness.CommandNextTurn {
		return agentrun.CommandReceipt{}, fmt.Errorf("%w: unsupported writing command %q", agentrun.ErrInvalidCommand, command.Kind)
	}
	activeRuntime, task, err := s.activeCommandRuntime()
	if err != nil {
		return agentrun.CommandReceipt{}, err
	}
	prepare := func(prepareCtx context.Context) (agentharness.TurnExecution, error) {
		if err := s.confirmActiveCommandRuntime(activeRuntime, task); err != nil {
			return agentharness.TurnExecution{}, err
		}
		execution, runtime, err := s.prepareWritingHarnessTurn(prepareCtx, command.Input, task.ID())
		if err != nil {
			return agentharness.TurnExecution{}, err
		}
		if runtime.workspace != activeRuntime.workspace || runtime.sess != activeRuntime.sess || runtime.chatService != activeRuntime.chatService {
			return agentharness.TurnExecution{}, ErrAgentContextChanged
		}
		if err := s.confirmActiveCommandRuntime(activeRuntime, task); err != nil {
			return agentharness.TurnExecution{}, err
		}
		return execution, nil
	}
	return activeRuntime.chatService.SubmitCommand(ctx, agentharness.CommandSpec{
		Kind: command.Kind, CommandID: command.CommandID,
		OperationID: command.OperationID, AfterOperationID: command.OperationID,
		Request: command.Input, Emit: task.Emit, Prepare: prepare,
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
	if currentTask != task || current.workspace != expected.workspace || current.sess != expected.sess || current.state != expected.state || current.chatService != expected.chatService {
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
	if a.session == nil || a.bookState == nil || a.chatService == nil || a.cfg == nil {
		return ideChatRuntime{}, nil, ErrNoWorkspace
	}
	run := a.activeWritingRun
	if run == nil || run.task == nil || run.task.Finished() {
		return ideChatRuntime{}, nil, ErrNoActiveAgentOperation
	}
	return run.runtime, run.task, nil
}
