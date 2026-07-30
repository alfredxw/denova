package app

import (
	"context"
	"fmt"
	"strings"

	"denova/config"
	agents "denova/internal/agents"
	"denova/internal/interactive"
)

type preparedInteractiveContextCompaction struct {
	Result          agents.ContextCompactionResult
	SourceTurnCount int
}

func (c *interactiveConversation) stagePreparedInteractiveCompaction(prepared preparedInteractiveContextCompaction) {
	if c == nil || !prepared.Result.Triggered {
		return
	}
	c.mu.Lock()
	copy := prepared
	c.pendingCompaction = &copy
	c.mu.Unlock()
}

func (c *interactiveConversation) PostSettlementContextStructuralSpec(
	ctx context.Context,
	settledOperationID agents.OperationID,
	options agents.RunOptions,
) (*agents.ContextStructuralSpec, error) {
	if c == nil || c.store == nil {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	prepared := c.pendingCompaction
	c.pendingCompaction = nil
	c.mu.Unlock()
	if prepared == nil || !prepared.Result.Triggered {
		return nil, nil
	}
	storyCtx, err := c.store.StoryContext(c.storyID, c.branchID)
	if err != nil {
		return nil, err
	}
	branchID := storyCtx.Snapshot.BranchID
	branch, ok := storyCtx.Meta.Branches[branchID]
	if !ok {
		return nil, fmt.Errorf("interactive branch metadata is missing: %s", branchID)
	}
	expectedParent := branch.Head
	preparedHash, err := contextStructuralValueHash(struct {
		Result          agents.ContextCompactionResult
		SourceTurnCount int
	}{prepared.Result, prepared.SourceTurnCount})
	if err != nil {
		return nil, err
	}
	commandID := contextStructuralCommandID(
		"game-auto-context-compaction", string(settledOperationID), c.storyID, branchID,
		fmt.Sprint(prepared.SourceTurnCount), preparedHash,
	)
	recordID := contextStructuralRecordID("cc", commandID)
	reason := strings.TrimSpace(prepared.Result.TriggerReason)
	if reason == "" {
		reason = "context_usage_threshold"
	}
	event := interactive.ContextCompactionEvent{
		ID: recordID, AgentKind: config.AgentKindInteractiveStory,
		Epoch: prepared.Result.Epoch, Summary: prepared.Result.Summary,
		SourceTurnCount: prepared.SourceTurnCount, RetainedTurns: prepared.Result.RetainedTurns,
		TokensBefore: prepared.Result.TokensBefore, TokensAfter: prepared.Result.TokensAfter,
		TargetRatio: prepared.Result.TargetRatio, ContextWindowTokens: prepared.Result.ContextWindowTokens,
		Strategy: prepared.Result.Strategy, Threshold: prepared.Result.Threshold,
		Reason: reason, Phase: prepared.Result.Phase,
		CandidateFingerprint: prepared.Result.CandidateFingerprint,
		CandidateGeneration:  prepared.Result.CandidateGeneration,
		ExpectedParentID:     &expectedParent,
	}
	options.StoryID = c.storyID
	options.BranchID = branchID
	ref := agents.ContextCompactionRef{
		Source: "story.turn_events", Purpose: "persist an automatic bounded model-history checkpoint after turn settlement",
		Resource: c.storyID + "/" + branchID, ExpectedRevision: contextStoryRevision(expectedParent),
	}
	binding := storyContextStructuralBinding(options.Workspace, c.storyID, branchID)
	plan, err := newContextStructuralRestorePlan(
		agents.ContextStructuralDomainStory, agents.ContextStructuralCompact, binding, ref, recordID,
		agents.ContextStructuralResult{Compaction: prepared.Result}, event,
	)
	if err != nil {
		return nil, err
	}
	operation := fixedContextStructuralOperation(plan,
		func(context.Context) (agents.ContextStructuralReceipt, error) {
			committed, err := c.store.AppendContextCompaction(c.storyID, branchID, event)
			if err != nil {
				return agents.ContextStructuralReceipt{}, err
			}
			if !sameStoryContextCompactionMutation(committed, event) {
				return agents.ContextStructuralReceipt{}, fmt.Errorf("canonical post-settlement Story compaction differs from frozen mutation")
			}
			return agents.ContextStructuralReceipt{Revision: "story-head:" + committed.ID}, nil
		},
		func(context.Context) (agents.ContextStructuralReceipt, bool, error) {
			current, err := c.store.StoryContext(c.storyID, branchID)
			if err != nil {
				return agents.ContextStructuralReceipt{}, false, err
			}
			if current.Snapshot.ContextCompaction == nil || current.Snapshot.ContextCompaction.ID != recordID {
				return agents.ContextStructuralReceipt{}, false, nil
			}
			committed := *current.Snapshot.ContextCompaction
			if !sameStoryContextCompactionMutation(committed, event) {
				return agents.ContextStructuralReceipt{}, false, fmt.Errorf("canonical post-settlement Story compaction conflicts with frozen mutation")
			}
			return agents.ContextStructuralReceipt{Revision: "story-head:" + committed.ID}, true, nil
		})
	return &agents.ContextStructuralSpec{
		CommandID: commandID, Action: agents.ContextStructuralCompact,
		Ref: ref, Options: options, Operation: operation, RestorePlan: &plan,
	}, nil
}
