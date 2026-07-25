package app

import (
	"context"
	"fmt"
	"log"

	"denova/config"
	agents "denova/internal/agents"
	"denova/internal/interactive"
)

func (s *InteractiveAppService) executeInteractiveContextCompaction(ctx context.Context, storyID, branchID, requestedCommandID string) (agents.ContextCompactionResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	s.admission.Lock()
	defer s.admission.Unlock()
	store, runtimeCfg, workspace, err := s.interactiveRuntimeConfig()
	if err != nil {
		return agents.ContextCompactionResult{}, err
	}
	storyCtx, err := store.StoryContext(storyID, branchID)
	if err != nil {
		return agents.ContextCompactionResult{}, err
	}
	if recovered, resumed, resumeErr := s.resumeStoryContextStructuralOperation(
		ctx, workspace, storyID, storyCtx.Snapshot.BranchID, agents.ContextStructuralCompact,
	); resumeErr != nil {
		return recovered.Compaction, resumeErr
	} else if resumed {
		return recovered.Compaction, nil
	}
	fence, err := s.drainInteractiveBinding(ctx, storyID, storyCtx.Snapshot.BranchID)
	if err != nil {
		return agents.ContextCompactionResult{}, err
	}
	storyCtx, err = store.StoryContext(storyID, storyCtx.Snapshot.BranchID)
	if err != nil {
		return agents.ContextCompactionResult{}, err
	}
	branchID = storyCtx.Snapshot.BranchID
	expectedParent := storyCtx.Meta.Branches[branchID].Head
	startTurn, endTurn, activeCompaction := interactiveModelHistoryRange(storyCtx.Snapshot)
	modelHistory, err := store.ReadModelHistory(storyID, interactive.StoryModelHistoryQuery{
		BranchID: branchID, StartTurn: startTurn, EndTurn: endTurn,
	})
	if err != nil {
		return agents.ContextCompactionResult{}, err
	}
	source, existingCheckpoint := interactiveCompactionModelHistorySource(modelHistory, activeCompaction)
	commandID, err := resolveContextStructuralCommandID(
		requestedCommandID,
		contextStructuralCommandID("game-compact", workspace, storyID, branchID, expectedParent),
	)
	if err != nil {
		return agents.ContextCompactionResult{}, err
	}
	recordID := contextStructuralRecordID("cc", commandID)
	if committed, found, findErr := store.ContextCompactionByID(storyID, recordID); findErr != nil {
		return agents.ContextCompactionResult{}, findErr
	} else if found {
		if committed.BranchID != branchID {
			return agents.ContextCompactionResult{}, fmt.Errorf("context compaction command belongs to another story branch")
		}
		return contextCompactionResultFromInteractive(committed), nil
	}
	if len(source) == 0 {
		return agents.ContextCompactionResult{Phase: "manual", SkippedReason: "empty_source"}, fmt.Errorf("没有可压缩的互动上下文")
	}
	epoch := 1
	if activeCompaction != nil {
		epoch = activeCompaction.Epoch + 1
	}
	_, prepared, err := agents.PrepareContextCompaction(ctx, &runtimeCfg, config.AgentKindInteractiveStory, agents.ContextCompactionInput{
		Messages: source, SourceMessages: source, Phase: "manual", Force: true,
		ExistingCheckpoint: existingCheckpoint, KeepLatestUser: true,
	}, epoch)
	if err != nil {
		return prepared, err
	}
	if !prepared.Triggered {
		return prepared, fmt.Errorf("没有可压缩的互动上下文")
	}
	event := interactiveCompactionEvent(recordID, expectedParent, modelHistory.EndTurn, prepared)
	ref := agents.ContextCompactionRef{
		Source: "story.turn_events", Purpose: "persist a bounded model-history checkpoint",
		Resource: storyID + "/" + branchID, ExpectedRevision: contextStoryRevision(expectedParent), Force: true,
	}
	binding := storyContextStructuralBinding(workspace, storyID, branchID)
	plan, err := newContextStructuralRestorePlan(
		agents.ContextStructuralDomainStory, agents.ContextStructuralCompact, binding, ref, recordID,
		agents.ContextStructuralResult{Compaction: prepared}, event,
	)
	if err != nil {
		return prepared, err
	}
	operation := fixedContextStructuralOperation(plan,
		func(context.Context) (agents.ContextStructuralReceipt, error) {
			a := s.app
			a.mu.Lock()
			defer a.mu.Unlock()
			if err := fence.validateLocked(a); err != nil || a.interactive != store {
				if err != nil {
					return agents.ContextStructuralReceipt{}, err
				}
				return agents.ContextStructuralReceipt{}, ErrAgentContextChanged
			}
			committed, err := store.AppendContextCompaction(storyID, branchID, event)
			if err != nil {
				return agents.ContextStructuralReceipt{}, err
			}
			if !sameStoryContextCompactionMutation(committed, event) {
				return agents.ContextStructuralReceipt{}, fmt.Errorf("canonical interactive compaction differs from frozen mutation")
			}
			return agents.ContextStructuralReceipt{Revision: "story-head:" + committed.ID}, nil
		},
		func(context.Context) (agents.ContextStructuralReceipt, bool, error) {
			current, err := store.StoryContext(storyID, branchID)
			if err != nil {
				return agents.ContextStructuralReceipt{}, false, err
			}
			if current.Snapshot.ContextCompaction == nil || current.Snapshot.ContextCompaction.ID != recordID {
				return agents.ContextStructuralReceipt{}, false, nil
			}
			committed := *current.Snapshot.ContextCompaction
			if !sameStoryContextCompactionMutation(committed, event) {
				return agents.ContextStructuralReceipt{}, false, fmt.Errorf("canonical interactive compaction conflicts with frozen mutation")
			}
			return agents.ContextStructuralReceipt{Revision: "story-head:" + committed.ID}, true, nil
		})
	result, err := fence.chat.ExecuteContextStructuralOperation(ctx, agents.ContextStructuralSpec{
		CommandID: commandID, Action: agents.ContextStructuralCompact,
		Ref: ref, Options: agents.RunOptions{AgentKind: agents.AgentKindInteractiveStory, Workspace: workspace, StoryID: storyID, BranchID: branchID, Mode: "interactive"},
		Operation: operation, RestorePlan: &plan,
	})
	if err != nil {
		return result.Compaction, err
	}
	if !result.Compaction.Triggered {
		return result.Compaction, fmt.Errorf("没有可压缩的互动上下文")
	}
	log.Printf("[interactive-agent] durable manual context compaction completed workspace=%s story_id=%s branch_id=%s epoch=%d source_turns=%d", workspace, storyID, branchID, result.Compaction.Epoch, modelHistory.EndTurn)
	return result.Compaction, nil
}

func (s *InteractiveAppService) executeInteractiveContextCompactionRemoval(ctx context.Context, storyID, branchID, requestedCommandID string) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	s.admission.Lock()
	defer s.admission.Unlock()
	store := s.store()
	if store == nil {
		return false, ErrNoWorkspace
	}
	storyCtx, err := store.StoryContext(storyID, branchID)
	if err != nil {
		return false, err
	}
	s.app.mu.RLock()
	workspace := s.app.workspace
	s.app.mu.RUnlock()
	if recovered, resumed, resumeErr := s.resumeStoryContextStructuralOperation(
		ctx, workspace, storyID, storyCtx.Snapshot.BranchID, agents.ContextStructuralRemove,
	); resumeErr != nil {
		return recovered.Removed, resumeErr
	} else if resumed {
		return recovered.Removed, nil
	}
	fence, err := s.drainInteractiveBinding(ctx, storyID, storyCtx.Snapshot.BranchID)
	if err != nil {
		return false, err
	}
	storyCtx, err = store.StoryContext(storyID, storyCtx.Snapshot.BranchID)
	if err != nil {
		return false, err
	}
	branchID = storyCtx.Snapshot.BranchID
	commandID := ""
	if requestedCommandID != "" {
		commandID, err = resolveContextStructuralCommandID(requestedCommandID, "")
		if err != nil {
			return false, err
		}
		if committed, found, findErr := store.ContextCompactionRemovalByID(storyID, contextStructuralRecordID("ccr", commandID)); findErr != nil {
			return false, findErr
		} else if found {
			if committed.BranchID != branchID {
				return false, fmt.Errorf("context compaction removal command belongs to another story branch")
			}
			return true, nil
		}
	}
	compaction := storyCtx.Snapshot.ContextCompaction
	active := compaction != nil
	if !active {
		return false, nil
	}
	expectedParent := storyCtx.Meta.Branches[branchID].Head
	compactionID := ""
	sourceTurns := 0
	if compaction != nil {
		compactionID = compaction.ID
		sourceTurns = compaction.SourceTurnCount
	}
	if commandID == "" {
		commandID, err = resolveContextStructuralCommandID(
			"", contextStructuralCommandID("game-remove-compaction", fence.workspace, storyID, branchID, expectedParent, compactionID),
		)
		if err != nil {
			return false, err
		}
	}
	recordID := contextStructuralRecordID("ccr", commandID)
	event := interactive.ContextCompactionRemovalEvent{
		ID: recordID, AgentKind: config.AgentKindInteractiveStory, CompactionID: compactionID,
		SourceTurnCount: sourceTurns, Reason: "user_removed", ExpectedParentID: &expectedParent,
	}
	ref := agents.ContextCompactionRef{
		Source: "story.context_compaction", Purpose: "restore canonical story turn history",
		Resource: storyID + "/" + branchID, ExpectedRevision: contextStoryRevision(expectedParent), CompactionID: compactionID,
	}
	binding := storyContextStructuralBinding(fence.workspace, storyID, branchID)
	plan, err := newContextStructuralRestorePlan(
		agents.ContextStructuralDomainStory, agents.ContextStructuralRemove, binding, ref, recordID,
		agents.ContextStructuralResult{Removed: true}, event,
	)
	if err != nil {
		return false, err
	}
	operation := fixedContextStructuralOperation(plan,
		func(context.Context) (agents.ContextStructuralReceipt, error) {
			a := s.app
			a.mu.Lock()
			defer a.mu.Unlock()
			if err := fence.validateLocked(a); err != nil || a.interactive != store {
				if err != nil {
					return agents.ContextStructuralReceipt{}, err
				}
				return agents.ContextStructuralReceipt{}, ErrAgentContextChanged
			}
			committed, err := store.AppendContextCompactionRemoval(storyID, branchID, event)
			if err != nil {
				return agents.ContextStructuralReceipt{}, err
			}
			if !sameStoryContextCompactionRemovalMutation(committed, event) {
				return agents.ContextStructuralReceipt{}, fmt.Errorf("canonical interactive compaction removal differs from frozen mutation")
			}
			return agents.ContextStructuralReceipt{Revision: "story-head:" + committed.ID}, nil
		},
		func(context.Context) (agents.ContextStructuralReceipt, bool, error) {
			current, err := store.StoryContext(storyID, branchID)
			if err != nil {
				return agents.ContextStructuralReceipt{}, false, err
			}
			if current.Snapshot.ContextCompactionRemoval == nil || current.Snapshot.ContextCompactionRemoval.ID != recordID {
				return agents.ContextStructuralReceipt{}, false, nil
			}
			committed := *current.Snapshot.ContextCompactionRemoval
			if !sameStoryContextCompactionRemovalMutation(committed, event) {
				return agents.ContextStructuralReceipt{}, false, fmt.Errorf("canonical interactive compaction removal conflicts with frozen mutation")
			}
			return agents.ContextStructuralReceipt{Revision: "story-head:" + recordID}, true, nil
		})
	result, err := fence.chat.ExecuteContextStructuralOperation(ctx, agents.ContextStructuralSpec{
		CommandID: commandID, Action: agents.ContextStructuralRemove,
		Ref: ref, Options: agents.RunOptions{AgentKind: agents.AgentKindInteractiveStory, Workspace: fence.workspace, StoryID: storyID, BranchID: branchID, Mode: "interactive"},
		Operation: operation, RestorePlan: &plan,
	})
	return result.Removed, err
}
