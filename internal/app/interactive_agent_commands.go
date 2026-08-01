package app

import (
	"context"
	agentharness "denova/internal/agents/harness"
	agentrun "denova/internal/agents/run"
	apptask "denova/internal/app/task"
	"strings"
)

// InteractiveAgentAbort identifies the exact game operation to stop. Workspace
// and durable binding identity are always derived from the active App runtime.
type InteractiveAgentAbort struct {
	CommandID   string
	OperationID agentrun.OperationID
	StoryID     string
	BranchID    string
	Reason      string
}

func (a *App) SubmitInteractiveAgentAbort(ctx context.Context, command InteractiveAgentAbort) (agentrun.CommandReceipt, error) {
	return a.interactiveService().SubmitAgentAbort(ctx, command)
}

func (s *InteractiveAppService) SubmitAgentAbort(ctx context.Context, command InteractiveAgentAbort) (agentrun.CommandReceipt, error) {
	target, err := s.activeAgentCommandTarget(command.StoryID, command.BranchID)
	if err != nil {
		return agentrun.CommandReceipt{}, err
	}
	return target.chatService.SubmitCommand(ctx, agentharness.CommandSpec{
		Kind: agentharness.CommandAbort, CommandID: command.CommandID,
		OperationID: command.OperationID, Reason: command.Reason,
		Options: agentrun.Options{
			AgentKind: agentrun.AgentKindInteractiveStory, TaskID: target.task.ID(),
			StoryID: target.info.StoryID, BranchID: target.info.BranchID,
			Workspace: target.info.Workspace, Mode: "interactive",
		},
	})
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
