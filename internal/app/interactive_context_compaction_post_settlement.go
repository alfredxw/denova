package app

import (
	"context"
	agentstructural "denova/internal/agents/context/structural"
	"fmt"
	"strings"

	"denova/config"
	agentcompaction "denova/internal/agents/context/compaction"
	agentrun "denova/internal/agents/run"
	"denova/internal/interactive"
)

type preparedInteractiveContextCompaction struct {
	Result          agentcompaction.Result
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
	settledOperationID agentrun.OperationID,
	options agentrun.Options,
) (*agentstructural.Spec, error) {
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
		Result          agentcompaction.Result
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
		ID:                   recordID,
		CompactionCheckpoint: agentcompaction.NewCheckpoint(config.AgentKindInteractiveStory, prepared.Result),
		SourceTurnCount:      prepared.SourceTurnCount,
		ExpectedParentID:     &expectedParent,
	}
	event.TriggerReason = reason
	options.StoryID = c.storyID
	options.BranchID = branchID
	ref := agentrun.ContextCompactionRef{
		Source: "story.turn_events", Purpose: "persist an automatic bounded model-history checkpoint after turn settlement",
		Resource: c.storyID + "/" + branchID, ExpectedRevision: contextStoryRevision(expectedParent),
	}
	binding := storyContextStructuralBinding(options.Workspace, c.storyID, branchID)
	plan, err := newContextStructuralRestorePlan(
		agentstructural.DomainStory, agentstructural.Compact, binding, ref, recordID,
		agentstructural.Result{Compaction: prepared.Result}, event,
	)
	if err != nil {
		return nil, err
	}
	operation := fixedContextStructuralOperation(plan,
		func(context.Context) (agentstructural.Receipt, error) {
			committed, err := c.store.AppendContextCompaction(c.storyID, branchID, event)
			if err != nil {
				return agentstructural.Receipt{}, err
			}
			if !sameStoryContextCompactionMutation(committed, event) {
				return agentstructural.Receipt{}, fmt.Errorf("canonical post-settlement Story compaction differs from frozen mutation")
			}
			return agentstructural.Receipt{Revision: "story-head:" + committed.ID}, nil
		},
		func(context.Context) (agentstructural.Receipt, bool, error) {
			current, err := c.store.StoryContext(c.storyID, branchID)
			if err != nil {
				return agentstructural.Receipt{}, false, err
			}
			if current.Snapshot.ContextCompaction == nil || current.Snapshot.ContextCompaction.ID != recordID {
				return agentstructural.Receipt{}, false, nil
			}
			committed := *current.Snapshot.ContextCompaction
			if !sameStoryContextCompactionMutation(committed, event) {
				return agentstructural.Receipt{}, false, fmt.Errorf("canonical post-settlement Story compaction conflicts with frozen mutation")
			}
			return agentstructural.Receipt{Revision: "story-head:" + committed.ID}, true, nil
		})
	return &agentstructural.Spec{
		CommandID: commandID, Action: agentstructural.Compact,
		Ref: ref, Options: options, Operation: operation, RestorePlan: &plan,
	}, nil
}
