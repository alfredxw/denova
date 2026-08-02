package interactiveapp

import (
	"context"
	agentconversation "denova/internal/agents/conversation"
	"denova/internal/agents/toolresult"
	"errors"
	"fmt"
	"strings"
	"time"

	"denova/config"
	agents "denova/internal/agents"
	agentcontext "denova/internal/agents/context"
	agentcompaction "denova/internal/agents/context/compaction"
	agentstructural "denova/internal/agents/context/structural"
	agentrun "denova/internal/agents/run"
	"denova/internal/interactive"
)

var errInteractiveCleanupBlockedByPendingModelContext = errors.New("interactive tool-result cleanup is deferred while durable model-context batches are pending")

type preparedInteractiveToolResultCleanup struct {
	event interactive.ToolResultCleanupEvent
}

func (c *Conversation) ContextPressurePolicy(messages []*agents.Message) agentcontext.ContextPressurePolicy {
	if c == nil {
		return agentcontext.ContextPressurePolicy{}
	}
	policy := agentconversation.ResolvePressurePolicy(c.cfg, config.AgentKindInteractiveStory, messages)
	completionReserve, toolReserve := agentcompaction.EstimateProjectionReserves(c.cfg, config.AgentKindInteractiveStory, c.replyTargetChars)
	policy.ReservedTokens = completionReserve + toolReserve
	policy.CheckpointOutputReserve = max(policy.CheckpointOutputReserve, completionReserve)
	storyCtx, err := c.storyContextForCycle()
	if err != nil {
		// Cleanup is an optimization, never a reason to guess canonical indexes.
		// Match Writing mode and fail closed when the durable projection cannot
		// be read; hard pressure can still route to checkpoint compaction.
		policy.CleanupEnabled = false
		policy.ObservedPromptTokens = 0
		return policy
	}
	if len(storyCtx.Snapshot.PendingModelContextBatches) > 0 {
		// Pending side batches are durable but do not yet have completed-turn
		// indexes. A cleanup record addresses completed model history, so any
		// replacement would be ambiguous until the Turn atomically absorbs the
		// batch. Hard pressure still routes to checkpoint compaction.
		policy.CleanupEnabled = false
	}
	if promptTokens, cachedTokens, ok := latestInteractiveModelPromptUsage(storyCtx.Snapshot, config.AgentKindInteractiveStory); ok {
		policy = policy.ObservePromptUsage(promptTokens, cachedTokens)
	}
	return policy
}

func latestInteractiveModelPromptUsage(snapshot interactive.Snapshot, agentKind string) (promptTokens, cachedTokens int, ok bool) {
	agentKind = strings.TrimSpace(agentKind)
	for index := len(snapshot.TokenUsageEvents) - 1; index >= 0; index-- {
		event := snapshot.TokenUsageEvents[index]
		if agentKind != "" && strings.TrimSpace(event.AgentKind) != agentKind {
			continue
		}
		if !interactiveUsageFollowsStructuralContext(event.CreatedAt, snapshot) {
			return 0, 0, false
		}
		for callIndex := len(event.UsageCalls) - 1; callIndex >= 0; callIndex-- {
			call := event.UsageCalls[callIndex]
			if call.PromptTokens > 0 {
				return call.PromptTokens, min(call.PromptTokens, max(0, call.CachedPromptTokens)), true
			}
		}
		if event.PromptTokens > 0 && event.ModelCalls <= 1 {
			return event.PromptTokens, min(event.PromptTokens, max(0, event.CachedPromptTokens)), true
		}
		return 0, 0, false
	}
	return 0, 0, false
}

func interactiveUsageFollowsStructuralContext(createdAt string, snapshot interactive.Snapshot) bool {
	latestStructural := ""
	if snapshot.ContextCompaction != nil {
		latestStructural = maxRFC3339Timestamp(latestStructural, snapshot.ContextCompaction.Ts)
	}
	if snapshot.ContextCompactionRemoval != nil {
		latestStructural = maxRFC3339Timestamp(latestStructural, snapshot.ContextCompactionRemoval.Ts)
	}
	if snapshot.ToolResultCleanup != nil {
		latestStructural = maxRFC3339Timestamp(latestStructural, snapshot.ToolResultCleanup.Ts)
	}
	if strings.TrimSpace(latestStructural) == "" {
		return true
	}
	usageTime, usageErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(createdAt))
	structuralTime, structuralErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(latestStructural))
	return usageErr == nil && structuralErr == nil && usageTime.After(structuralTime)
}

func maxRFC3339Timestamp(left, right string) string {
	leftTime, leftErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(left))
	rightTime, rightErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(right))
	switch {
	case leftErr != nil:
		return strings.TrimSpace(right)
	case rightErr != nil:
		return strings.TrimSpace(left)
	case rightTime.After(leftTime):
		return strings.TrimSpace(right)
	default:
		return strings.TrimSpace(left)
	}
}

func (c *Conversation) StageToolResultCleanup(ctx context.Context, visible []*agents.Message, plan toolresult.CleanupPlan) error {
	if c == nil || c.store == nil || len(plan.Replacements) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	storyCtx, err := c.storyContextForCycle()
	if err != nil {
		return err
	}
	if len(storyCtx.Snapshot.PendingModelContextBatches) > 0 {
		return errInteractiveCleanupBlockedByPendingModelContext
	}
	modelHistory, activeCompaction, err := c.modelHistoryForCycle(storyCtx)
	if err != nil {
		return err
	}
	// Persist indexes against the same canonical settled projection used by
	// provider assembly. In particular, resolved interrupted inputs live at
	// their acceptance boundary rather than inside their later owner Turn.
	// The current cycle is excluded exactly as it is during model assembly; its
	// new Turn is appended after these stable indexes during settlement.
	projection, err := BuildModelContextProjection(
		modelHistory, activeCompaction, storyCtx.Snapshot, c.ToolResultContextPolicy(), c.AgentCycleIdentitySnapshot(),
	)
	if err != nil {
		return err
	}
	effective := projection.Messages
	resolved, err := toolresult.ResolveCleanupTargets(visible, effective, plan)
	if err != nil {
		return err
	}

	var existing []toolresult.PersistedReplacement
	previousReclaimed := 0
	if existingEvent := storyCtx.Snapshot.ToolResultCleanup; existingEvent != nil {
		for _, replacement := range existingEvent.Replacements {
			existing = append(existing, toolresult.PersistedReplacement{
				MessageIndex: replacement.MessageIndex, ToolCallID: replacement.ToolCallID, Placeholder: replacement.Placeholder,
			})
		}
		previousReclaimed = existingEvent.ReclaimedTokens
	}
	merged, err := toolresult.MergeCleanup(existing, resolved, 0, int64(len(effective)), previousReclaimed, plan.ReclaimedTokens)
	if err != nil {
		return err
	}
	ordered := make([]interactive.ToolResultReplacement, 0, len(merged.Replacements))
	for _, replacement := range merged.Replacements {
		ordered = append(ordered, interactive.ToolResultReplacement{
			MessageIndex: replacement.MessageIndex, ToolCallID: replacement.ToolCallID, Placeholder: replacement.Placeholder,
		})
	}
	event := interactive.ToolResultCleanupEvent{
		AgentKind:   config.AgentKindInteractiveStory,
		SourceStart: merged.SourceStart, SourceEnd: merged.SourceEnd, Replacements: ordered,
		ReclaimedTokens: merged.ReclaimedTokens, TriggeredAtUsage: agentcontext.EstimateTokens(visible, nil),
		EarliestChanged: merged.SourceStart, WarmSuffixTokens: plan.WarmSuffixTokens, RendererVersion: plan.RendererVersion,
	}
	c.mu.Lock()
	c.pendingCleanup = &preparedInteractiveToolResultCleanup{event: event}
	c.mu.Unlock()
	return nil
}

func (c *Conversation) CommitPostSettlementToolResultCleanup(ctx context.Context, settledOperationID agentrun.OperationID) error {
	if c == nil || c.store == nil {
		return nil
	}
	c.mu.Lock()
	prepared := c.pendingCleanup
	c.pendingCleanup = nil
	c.mu.Unlock()
	if prepared == nil || len(prepared.event.Replacements) == 0 {
		return nil
	}
	storyCtx, err := c.store.StoryContext(c.storyID, c.branchID)
	if err != nil {
		return err
	}
	branch, ok := storyCtx.Meta.Branches[storyCtx.Snapshot.BranchID]
	if !ok {
		return fmt.Errorf("interactive cleanup branch metadata is missing: %s", storyCtx.Snapshot.BranchID)
	}
	hash, err := agentstructural.ValueHash(prepared.event)
	if err != nil {
		return err
	}
	commandID := agentstructural.CommandID("auto-tool-result-cleanup", string(settledOperationID), c.storyID, storyCtx.Snapshot.BranchID, hash)
	event := prepared.event
	event.ID = agentstructural.RecordID("trc", commandID)
	expectedParent := strings.TrimSpace(branch.Head)
	event.ExpectedParentID = &expectedParent
	_, err = c.store.AppendToolResultCleanup(c.storyID, storyCtx.Snapshot.BranchID, event)
	return err
}

func (c *Conversation) DiscardStagedToolResultCleanup() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.pendingCleanup = nil
	c.mu.Unlock()
}

func applyInteractiveToolResultCleanup(messages []*agents.Message, event interactive.ToolResultCleanupEvent) []*agents.Message {
	plan := toolresult.CleanupPlan{RendererVersion: event.RendererVersion}
	for _, replacement := range event.Replacements {
		plan.Replacements = append(plan.Replacements, toolresult.CleanupReplacement{
			MessageIndex: int(replacement.MessageIndex), ToolCallID: replacement.ToolCallID, Placeholder: replacement.Placeholder,
		})
	}
	return toolresult.ApplyCleanupPlan(messages, plan)
}
