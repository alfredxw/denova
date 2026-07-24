package app

import (
	"context"
	"fmt"
	"strings"

	agents "denova/internal/agents"
)

// InteractiveAgentCommand contains only the selected game resource and typed
// command payload. Workspace and durable binding identity are always derived
// from the active App runtime.
type InteractiveAgentCommand struct {
	Kind        agents.AgentCommandKind
	CommandID   string
	OperationID agents.OperationID
	StoryID     string
	BranchID    string
	Reason      string
	Input       agents.ChatRequest
}

func (a *App) SubmitInteractiveAgentCommand(ctx context.Context, command InteractiveAgentCommand) (agents.CommandReceipt, error) {
	return a.interactiveService().SubmitAgentCommand(ctx, command)
}

func (s *InteractiveAppService) SubmitAgentCommand(ctx context.Context, command InteractiveAgentCommand) (agents.CommandReceipt, error) {
	target, err := s.activeAgentCommandTarget(command.StoryID, command.BranchID)
	if err != nil {
		return agents.CommandReceipt{}, err
	}
	if command.Kind == agents.AgentCommandAbort {
		return target.chatService.SubmitCommand(ctx, agents.AgentCommandSpec{
			Kind: command.Kind, CommandID: command.CommandID,
			OperationID: command.OperationID, Reason: command.Reason,
			Options: agents.RunOptions{
				AgentKind: agents.AgentKindInteractiveStory, TaskID: target.task.ID(),
				StoryID: target.info.StoryID, BranchID: target.info.BranchID,
				Workspace: target.info.Workspace, Mode: "interactive",
			},
		})
	}
	if command.Kind != agents.AgentCommandSteer && command.Kind != agents.AgentCommandFollowUp && command.Kind != agents.AgentCommandNextTurn {
		return agents.CommandReceipt{}, fmt.Errorf("%w: unsupported game command %q", agents.ErrInvalidCommand, command.Kind)
	}

	prepare := func(prepareCtx context.Context) (agents.HarnessTurnExecution, error) {
		if err := s.confirmActiveAgentCommandTarget(target); err != nil {
			return agents.HarnessTurnExecution{}, err
		}
		cycle, err := s.prepareInteractiveAgentCycle(prepareCtx, interactiveAgentCycleRequest{
			StoryID: target.info.StoryID, BranchID: target.info.BranchID,
			Message: command.Input.Message, StyleScenes: command.Input.StyleScenes,
			Locale: command.Input.Locale,
		})
		if err != nil {
			return agents.HarnessTurnExecution{}, err
		}
		if err := s.confirmActiveAgentCommandTarget(target); err != nil {
			return agents.HarnessTurnExecution{}, err
		}
		cycle.bindCommit(target.task.emit)
		return agents.HarnessTurnExecution{
			Runner: cycle.runner, Conversation: cycle.conversation,
			BookService: cycle.bookService, Request: cycle.request,
			Options: cycle.options(target.task.ID()),
		}, nil
	}
	return target.chatService.SubmitCommand(ctx, agents.AgentCommandSpec{
		Kind: command.Kind, CommandID: command.CommandID,
		OperationID: command.OperationID, AfterOperationID: command.OperationID,
		Request: command.Input, Emit: target.task.emit, Prepare: prepare,
		Options: agents.RunOptions{
			AgentKind: agents.AgentKindInteractiveStory, TaskID: target.task.ID(),
			StoryID: target.info.StoryID, BranchID: target.info.BranchID,
			Workspace: target.info.Workspace, Mode: "interactive",
		},
	})
}

type interactiveAgentCommandTarget struct {
	task        *Task
	info        InteractiveTaskInfo
	chatService *agents.ChatService
}

func (s *InteractiveAppService) activeAgentCommandTarget(storyID, branchID string) (interactiveAgentCommandTarget, error) {
	if s == nil || s.app == nil {
		return interactiveAgentCommandTarget{}, ErrNoWorkspace
	}
	storyID = strings.TrimSpace(storyID)
	branchID = strings.TrimSpace(branchID)
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.workspaceTransition {
		return interactiveAgentCommandTarget{}, ErrWorkspaceTransition
	}
	if a.workspace == "" || a.chatService == nil || a.interactive == nil {
		return interactiveAgentCommandTarget{}, ErrNoWorkspace
	}
	run := a.activeInteractiveRun
	if run == nil || run.task == nil || run.task.Finished() || run.info.Workspace != a.workspace {
		return interactiveAgentCommandTarget{}, ErrNoActiveAgentOperation
	}
	if storyID == "" || run.info.StoryID != storyID {
		return interactiveAgentCommandTarget{}, ErrNoActiveAgentOperation
	}
	if branchID != "" && run.info.BranchID != branchID {
		return interactiveAgentCommandTarget{}, ErrNoActiveAgentOperation
	}
	return interactiveAgentCommandTarget{task: run.task, info: run.info, chatService: a.chatService}, nil
}

func (s *InteractiveAppService) confirmActiveAgentCommandTarget(target interactiveAgentCommandTarget) error {
	if s == nil || s.app == nil {
		return ErrNoWorkspace
	}
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	run := a.activeInteractiveRun
	if a.workspaceTransition || run == nil || run.task != target.task || run.task.Finished() ||
		run.info.Workspace != target.info.Workspace || run.info.StoryID != target.info.StoryID ||
		run.info.BranchID != target.info.BranchID || a.workspace != target.info.Workspace {
		return ErrNoActiveAgentOperation
	}
	return nil
}
