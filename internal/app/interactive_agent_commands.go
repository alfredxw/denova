package app

import (
	"context"
	"strings"

	agents "denova/internal/agents"
)

// InteractiveAgentAbort identifies the exact game operation to stop. Workspace
// and durable binding identity are always derived from the active App runtime.
type InteractiveAgentAbort struct {
	CommandID   string
	OperationID agents.OperationID
	StoryID     string
	BranchID    string
	Reason      string
}

func (a *App) SubmitInteractiveAgentAbort(ctx context.Context, command InteractiveAgentAbort) (agents.CommandReceipt, error) {
	return a.interactiveService().SubmitAgentAbort(ctx, command)
}

func (s *InteractiveAppService) SubmitAgentAbort(ctx context.Context, command InteractiveAgentAbort) (agents.CommandReceipt, error) {
	target, err := s.activeAgentCommandTarget(command.StoryID, command.BranchID)
	if err != nil {
		return agents.CommandReceipt{}, err
	}
	return target.chatService.SubmitCommand(ctx, agents.AgentCommandSpec{
		Kind: agents.AgentCommandAbort, CommandID: command.CommandID,
		OperationID: command.OperationID, Reason: command.Reason,
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
