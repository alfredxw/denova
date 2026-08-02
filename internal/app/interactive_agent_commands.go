package app

import (
	"context"
	"fmt"
	"strings"

	agentchat "denova/internal/agents/chat"
	agentharness "denova/internal/agents/harness"
	agentrun "denova/internal/agents/run"
	apptask "denova/internal/app/task"
)

// InteractiveAgentCommand targets one exact game operation. Workspace and
// durable binding identity are always derived from the active App runtime.
type InteractiveAgentCommand struct {
	Kind            agentharness.CommandKind
	CommandID       string
	OperationID     agentrun.OperationID
	TargetCommandID agentrun.CommandID
	StoryID         string
	BranchID        string
	Reason          string
	Input           agentchat.ChatRequest
}

func (a *App) SubmitInteractiveAgentCommand(ctx context.Context, command InteractiveAgentCommand) (agentrun.CommandReceipt, error) {
	return a.interactiveService().SubmitAgentCommand(ctx, command)
}

func (s *InteractiveAppService) SubmitAgentCommand(ctx context.Context, command InteractiveAgentCommand) (agentrun.CommandReceipt, error) {
	target, err := s.activeAgentCommandTarget(command.StoryID, command.BranchID)
	if err != nil {
		return agentrun.CommandReceipt{}, err
	}
	options := interactiveAgentCommandOptions(target)
	if command.Kind == agentharness.CommandAbort || command.Kind == agentharness.CommandSteerQueued || command.Kind == agentharness.CommandCancelQueued {
		return target.chatService.SubmitCommand(ctx, agentharness.CommandSpec{
			Kind: command.Kind, CommandID: command.CommandID,
			OperationID: command.OperationID, TargetCommandID: command.TargetCommandID, Reason: command.Reason,
			Options: options,
		})
	}
	if command.Kind != agentharness.CommandFollowUp {
		return agentrun.CommandReceipt{}, fmt.Errorf("%w: unsupported game command %q", agentrun.ErrInvalidCommand, command.Kind)
	}
	prepare := func(prepareCtx context.Context) (agentharness.TurnExecution, error) {
		if err := s.confirmActiveAgentCommandTarget(target); err != nil {
			return agentharness.TurnExecution{}, err
		}
		cycle, err := s.prepareInteractiveAgentCycle(prepareCtx, interactiveAgentCycleRequest{
			StoryID: target.info.StoryID, BranchID: target.info.BranchID,
			Message: command.Input.Message, StyleScenes: command.Input.StyleScenes, Locale: command.Input.Locale,
		})
		if err != nil {
			return agentharness.TurnExecution{}, err
		}
		if cycle.workspace != target.info.Workspace || cycle.storyID != target.info.StoryID || cycle.branchID != target.info.BranchID || cycle.chatService != target.chatService {
			return agentharness.TurnExecution{}, ErrAgentContextChanged
		}
		if err := s.confirmActiveAgentCommandTarget(target); err != nil {
			return agentharness.TurnExecution{}, err
		}
		cycle.bindCommit(target.task.Emit)
		return agentharness.TurnExecution{
			Runner: cycle.runner, Conversation: cycle.conversation,
			BookService: cycle.bookService, Request: cycle.request,
			Options: cycle.options(target.task.ID()),
		}, nil
	}
	return target.chatService.SubmitCommand(ctx, agentharness.CommandSpec{
		Kind: command.Kind, CommandID: command.CommandID,
		OperationID: command.OperationID, AfterOperationID: command.OperationID,
		Request: command.Input, Emit: target.task.Emit, Prepare: prepare,
		Options: options,
	})
}

func interactiveAgentCommandOptions(target interactiveAgentCommandTarget) agentrun.Options {
	return agentrun.Options{
		AgentKind: agentrun.AgentKindInteractiveStory, TaskID: target.task.ID(),
		StoryID: target.info.StoryID, BranchID: target.info.BranchID,
		Workspace: target.info.Workspace, Mode: "interactive",
	}
}

type interactiveAgentCommandTarget struct {
	task        *apptask.Task
	info        InteractiveTaskInfo
	chatService *agentharness.Service
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

func (s *InteractiveAppService) confirmActiveAgentCommandTarget(expected interactiveAgentCommandTarget) error {
	current, err := s.activeAgentCommandTarget(expected.info.StoryID, expected.info.BranchID)
	if err != nil {
		return err
	}
	if current.task != expected.task || current.chatService != expected.chatService || current.info.Workspace != expected.info.Workspace {
		return ErrAgentContextChanged
	}
	return nil
}
