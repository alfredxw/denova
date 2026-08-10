package app

import (
	"context"
	"fmt"
	"strings"

	agentchat "denova/internal/agents/chat"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	apptask "denova/internal/app/task"
)

// InteractiveAgentCommand targets one exact game operation. Workspace and
// durable binding identity are always derived from the active App runtime.
type InteractiveAgentCommand struct {
	Kind            agentexecution.CommandKind
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
	if command.Kind == agentexecution.CommandAbort || command.Kind == agentexecution.CommandSteerQueued || command.Kind == agentexecution.CommandCancelQueued {
		return target.executionRuntime.SubmitCommand(ctx, agentexecution.CommandRequest{
			Kind: command.Kind, CommandID: command.CommandID,
			OperationID: command.OperationID, TargetCommandID: command.TargetCommandID, Reason: command.Reason,
			Options: options,
		})
	}
	if command.Kind != agentexecution.CommandFollowUp {
		return agentrun.CommandReceipt{}, fmt.Errorf("%w: unsupported game command %q", agentrun.ErrInvalidCommand, command.Kind)
	}
	return target.executionRuntime.SubmitCommand(ctx, agentexecution.CommandRequest{
		Kind: command.Kind, CommandID: command.CommandID,
		OperationID: command.OperationID, AfterOperationID: command.OperationID,
		Request: command.Input, Emit: target.task.Emit,
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
	task             *apptask.Task
	info             InteractiveTaskInfo
	executionRuntime *agentexecution.Runtime
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
	if a.workspace == "" || a.executionRuntime == nil || a.interactive == nil {
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
	return interactiveAgentCommandTarget{task: run.task, info: run.info, executionRuntime: a.executionRuntime}, nil
}

func (s *InteractiveAppService) confirmActiveAgentCommandTarget(expected interactiveAgentCommandTarget) error {
	current, err := s.activeAgentCommandTarget(expected.info.StoryID, expected.info.BranchID)
	if err != nil {
		return err
	}
	if current.task != expected.task || current.executionRuntime != expected.executionRuntime || current.info.Workspace != expected.info.Workspace {
		return ErrAgentContextChanged
	}
	return nil
}
