package app

import (
	"context"
	"errors"
	"fmt"

	"denova/internal/agent"
	runstate "denova/internal/agent/runtime"
)

var ErrNoActiveAgentOperation = errors.New("no active agent operation")

type ChatAgentCommand struct {
	Kind        agent.AgentCommandKind
	CommandID   string
	OperationID runstate.OperationID
	Reason      string
	Input       agent.ChatRequest
}

// SubmitChatAgentCommand adapts a transport command to the active writing
// binding. Workspace/session identity is captured from App state and never
// accepted from the client.
func (a *App) SubmitChatAgentCommand(ctx context.Context, command ChatAgentCommand) (runstate.Receipt, error) {
	return a.chat().SubmitAgentCommand(ctx, command)
}

func (s *ChatAppService) SubmitAgentCommand(ctx context.Context, command ChatAgentCommand) (runstate.Receipt, error) {
	if command.Kind == agent.AgentCommandAbort {
		runtime, task, err := s.activeCommandRuntime()
		if err != nil {
			return runstate.Receipt{}, err
		}
		return runtime.chatService.SubmitCommand(ctx, agent.AgentCommandSpec{
			Kind: command.Kind, CommandID: command.CommandID,
			OperationID: command.OperationID, Reason: command.Reason,
			Options: agent.RunOptions{
				AgentKind: agent.AgentKindIDE, TaskID: task.ID(),
				SessionID: runtime.sess.ID, Workspace: runtime.workspace, Mode: "ide",
			},
		})
	}
	if command.Kind != agent.AgentCommandSteer && command.Kind != agent.AgentCommandFollowUp && command.Kind != agent.AgentCommandNextTurn {
		return runstate.Receipt{}, fmt.Errorf("%w: unsupported writing command %q", runstate.ErrInvalidCommand, command.Kind)
	}
	activeRuntime, task, err := s.activeCommandRuntime()
	if err != nil {
		return runstate.Receipt{}, err
	}
	prepare := func(prepareCtx context.Context) (agent.HarnessTurnExecution, error) {
		if err := s.confirmActiveCommandRuntime(activeRuntime, task); err != nil {
			return agent.HarnessTurnExecution{}, err
		}
		execution, runtime, err := s.prepareWritingHarnessTurn(prepareCtx, command.Input, task.ID())
		if err != nil {
			return agent.HarnessTurnExecution{}, err
		}
		if runtime.workspace != activeRuntime.workspace || runtime.sess != activeRuntime.sess || runtime.chatService != activeRuntime.chatService {
			return agent.HarnessTurnExecution{}, ErrAgentContextChanged
		}
		if err := s.confirmActiveCommandRuntime(activeRuntime, task); err != nil {
			return agent.HarnessTurnExecution{}, err
		}
		return execution, nil
	}
	return activeRuntime.chatService.SubmitCommand(ctx, agent.AgentCommandSpec{
		Kind: command.Kind, CommandID: command.CommandID,
		OperationID: command.OperationID, AfterOperationID: command.OperationID,
		Request: command.Input, Emit: task.emit, Prepare: prepare,
		Options: agent.RunOptions{
			AgentKind: agent.AgentKindIDE,
			TaskID:    task.ID(),
			SessionID: activeRuntime.sess.ID,
			Workspace: activeRuntime.workspace,
			Mode:      "ide",
		},
	})
}

func (s *ChatAppService) confirmActiveCommandRuntime(expected ideChatRuntime, task *Task) error {
	current, currentTask, err := s.activeCommandRuntime()
	if err != nil {
		return err
	}
	if currentTask != task || current.workspace != expected.workspace || current.sess != expected.sess || current.state != expected.state || current.chatService != expected.chatService {
		return ErrAgentContextChanged
	}
	return nil
}

func (s *ChatAppService) activeCommandRuntime() (ideChatRuntime, *Task, error) {
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
