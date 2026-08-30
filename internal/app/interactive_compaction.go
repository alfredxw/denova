package app

import (
	"context"
	"fmt"
	"log/slog"

	agentcompaction "denova/internal/agents/context/compaction"
	agentstructural "denova/internal/agents/context/structural"
	agentrun "denova/internal/agents/run"
	compactionapp "denova/internal/app/compaction"
)

func (s *InteractiveAppService) executeInteractiveContextCompaction(
	ctx context.Context,
	storyID, branchID, requestedCommandID string,
) (agentcompaction.Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	s.admission.Lock()
	defer s.admission.Unlock()
	store := s.store()
	if store == nil {
		return agentcompaction.Result{}, ErrNoWorkspace
	}
	storyContext, err := store.StoryContext(storyID, branchID)
	if err != nil {
		return agentcompaction.Result{}, err
	}
	branchID = storyContext.Snapshot.BranchID
	fence, err := s.drainInteractiveBinding(ctx, storyID, branchID)
	if err != nil {
		return agentcompaction.Result{}, err
	}
	storyContext, err = store.StoryContext(storyID, branchID)
	if err != nil {
		return agentcompaction.Result{}, err
	}
	expectedParent := storyContext.Meta.Branches[branchID].Head
	commandID, err := compactionapp.ResolveCommandID(
		requestedCommandID,
		agentstructural.CommandID(
			"game-compact", fence.workspace, storyID, branchID, expectedParent,
		),
	)
	if err != nil {
		return agentcompaction.Result{}, err
	}
	result, err := fence.chat.ExecuteStructuralOperation(ctx, agentstructural.Spec{
		CommandID: commandID,
		Action:    agentstructural.Compact,
		Ref:       agentrun.ContextCompactionRef{Force: true},
		Options: agentrun.Options{
			AgentKind: agentrun.AgentKindInteractiveStory, ProjectID: fence.projectID, Workspace: fence.workspace,
			StoryID: storyID, BranchID: branchID, Mode: "interactive",
		},
	})
	if err != nil {
		return result.Compaction, err
	}
	if !result.Compaction.Triggered {
		return result.Compaction, fmt.Errorf("没有可压缩的互动上下文 / No interactive context is available for compaction")
	}
	slog.InfoContext(ctx, "[interactive-agent] manual Agent Session compaction completed",
		"workspace", fence.workspace, "story_id", storyID, "branch_id", branchID,
		"revision", result.Compaction.Epoch, "source_messages", result.Compaction.SourceMessageCount,
	)
	return result.Compaction, nil
}

func (s *InteractiveAppService) executeInteractiveContextCompactionRemoval(
	ctx context.Context,
	storyID, branchID, requestedCommandID string,
) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	s.admission.Lock()
	defer s.admission.Unlock()
	store := s.store()
	if store == nil {
		return false, ErrNoWorkspace
	}
	storyContext, err := store.StoryContext(storyID, branchID)
	if err != nil {
		return false, err
	}
	branchID = storyContext.Snapshot.BranchID
	fence, err := s.drainInteractiveBinding(ctx, storyID, branchID)
	if err != nil {
		return false, err
	}
	storyContext, err = store.StoryContext(storyID, branchID)
	if err != nil {
		return false, err
	}
	expectedParent := storyContext.Meta.Branches[branchID].Head
	commandID, err := compactionapp.ResolveCommandID(
		requestedCommandID,
		agentstructural.CommandID(
			"game-remove-compaction", fence.workspace, storyID, branchID, expectedParent,
		),
	)
	if err != nil {
		return false, err
	}
	result, err := fence.chat.ExecuteStructuralOperation(ctx, agentstructural.Spec{
		CommandID: commandID,
		Action:    agentstructural.Remove,
		Options: agentrun.Options{
			AgentKind: agentrun.AgentKindInteractiveStory, ProjectID: fence.projectID, Workspace: fence.workspace,
			StoryID: storyID, BranchID: branchID, Mode: "interactive",
		},
	})
	return result.Removed, err
}
