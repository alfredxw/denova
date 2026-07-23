package app

import (
	"context"
	"fmt"

	"denova/config"
	"denova/internal/agent"
	runstate "denova/internal/agent/runtime"
	"denova/internal/interactive"
)

type preparedInteractiveContextCompaction struct {
	Result          agent.ContextCompactionResult
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
	settledOperationID runstate.OperationID,
	options agent.RunOptions,
) (*agent.ContextStructuralSpec, error) {
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
		Result          agent.ContextCompactionResult
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
	event := interactive.ContextCompactionEvent{
		ID: recordID, AgentKind: config.AgentKindInteractiveStory,
		Epoch: prepared.Result.Epoch, Summary: prepared.Result.Summary,
		SourceTurnCount: prepared.SourceTurnCount, RetainedTurns: prepared.Result.RetainedTurns,
		TokensBefore: prepared.Result.TokensBefore, TokensAfter: prepared.Result.TokensAfter,
		TargetRatio: prepared.Result.TargetRatio, ContextWindowTokens: prepared.Result.ContextWindowTokens,
		Strategy: prepared.Result.Strategy, Threshold: prepared.Result.Threshold,
		Reason: "context_usage_threshold", Phase: prepared.Result.Phase,
		ExpectedParentID: &expectedParent,
	}
	options.StoryID = c.storyID
	options.BranchID = branchID
	ref := runstate.ContextCompactionRef{
		Source: "story.turn_events", Purpose: "persist an automatic bounded model-history checkpoint after turn settlement",
		Resource: c.storyID + "/" + branchID, ExpectedRevision: contextStoryRevision(expectedParent),
	}
	binding, err := storyContextStructuralBinding(options.Workspace, c.storyID, branchID)
	if err != nil {
		return nil, err
	}
	plan, err := newContextStructuralRestorePlan(
		agent.ContextStructuralDomainStory, agent.ContextStructuralCompact, binding, ref, recordID,
		agent.ContextStructuralResult{Compaction: prepared.Result}, event,
	)
	if err != nil {
		return nil, err
	}
	operation := fixedContextStructuralOperation(plan,
		func(context.Context) (agent.ContextStructuralReceipt, error) {
			committed, err := c.store.AppendContextCompaction(c.storyID, branchID, event)
			if err != nil {
				return agent.ContextStructuralReceipt{}, err
			}
			if !sameStoryContextCompactionMutation(committed, event) {
				return agent.ContextStructuralReceipt{}, fmt.Errorf("canonical post-settlement Story compaction differs from frozen mutation")
			}
			return agent.ContextStructuralReceipt{Revision: "story-head:" + committed.ID}, nil
		},
		func(context.Context) (agent.ContextStructuralReceipt, bool, error) {
			current, err := c.store.StoryContext(c.storyID, branchID)
			if err != nil {
				return agent.ContextStructuralReceipt{}, false, err
			}
			if current.Snapshot.ContextCompaction == nil || current.Snapshot.ContextCompaction.ID != recordID {
				return agent.ContextStructuralReceipt{}, false, nil
			}
			committed := *current.Snapshot.ContextCompaction
			if !sameStoryContextCompactionMutation(committed, event) {
				return agent.ContextStructuralReceipt{}, false, fmt.Errorf("canonical post-settlement Story compaction conflicts with frozen mutation")
			}
			return agent.ContextStructuralReceipt{Revision: "story-head:" + committed.ID}, true, nil
		})
	return &agent.ContextStructuralSpec{
		CommandID: commandID, Action: agent.ContextStructuralCompact,
		Ref: ref, Options: options, Operation: operation, RestorePlan: &plan,
	}, nil
}
