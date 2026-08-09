package conversation

import (
	"context"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/toolresult"
	"fmt"

	agent "github.com/alfredxw/denova/agent"

	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/session"
)

// PostSettlementToolResultCleanupProvider commits a deterministic, reversible
// context projection after the parent turn is durable. A missed cleanup is
// safe to retry on a future turn and therefore does not require a second
// recovery-paused actor operation like semantic checkpoint compaction does.
type PostSettlementToolResultCleanupProvider interface {
	CommitPostSettlementToolResultCleanup(context.Context, agentrun.OperationID) error
}

type preparedSessionToolResultCleanup struct {
	record session.ToolResultCleanupRecord
}

func (c *SessionConversation) ContextPressurePolicy(messages []*agent.Message) agentcontext.ContextPressurePolicy {
	if c == nil {
		return agentcontext.ContextPressurePolicy{}
	}
	policy := resolveContextPressurePolicy(c.cfg, c.agentKind, messages)
	if c.session != nil {
		if promptTokens, cachedTokens, ok := c.session.LatestModelPromptUsage(c.agentKind); ok {
			policy = policy.ObservePromptUsage(promptTokens, cachedTokens)
		}
	}
	return policy
}

func (c *SessionConversation) StageToolResultCleanup(ctx context.Context, visible []*agent.Message, plan toolresult.CleanupPlan) error {
	if c == nil || c.session == nil || len(plan.Replacements) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	snapshot, err := c.session.SnapshotContext(c.agentKind)
	if err != nil {
		return fmt.Errorf("snapshot cleanup canonical context: %w", err)
	}
	total := snapshot.Cursor.MessageCount
	start := snapshot.Cursor.ClearAfterIndex
	canonical, err := c.session.ReadMessageRange(ctx, start, total)
	if err != nil {
		return fmt.Errorf("resolve cleanup canonical range: %w", err)
	}
	resolved, err := toolresult.ResolveCleanupTargets(visible, canonical, plan)
	if err != nil {
		return err
	}

	var existing []toolresult.PersistedReplacement
	previousReclaimed := 0
	if existingRecord := snapshot.ToolResultCleanup; existingRecord != nil {
		for _, replacement := range existingRecord.Replacements {
			existing = append(existing, toolresult.PersistedReplacement{
				MessageIndex: replacement.MessageIndex, ToolCallID: replacement.ToolCallID, Placeholder: replacement.Placeholder,
			})
		}
		previousReclaimed = existingRecord.ReclaimedTokens
	}
	merged, err := toolresult.MergeCleanup(existing, resolved, start, int64(total), previousReclaimed, plan.ReclaimedTokens)
	if err != nil {
		return err
	}
	ordered := make([]session.ToolResultReplacement, 0, len(merged.Replacements))
	for _, replacement := range merged.Replacements {
		ordered = append(ordered, session.ToolResultReplacement{
			MessageIndex: replacement.MessageIndex, ToolCallID: replacement.ToolCallID, Placeholder: replacement.Placeholder,
		})
	}
	record := session.ToolResultCleanupRecord{
		AgentKind: c.agentKind, SourceStart: merged.SourceStart, SourceEnd: merged.SourceEnd,
		Replacements: ordered, ReclaimedTokens: merged.ReclaimedTokens,
		TriggeredAtUsage: agentcontext.EstimateTokens(visible, nil), EarliestChanged: merged.SourceStart,
		WarmSuffixTokens: plan.WarmSuffixTokens, RendererVersion: plan.RendererVersion,
	}
	c.cycleMu.Lock()
	c.pendingCleanup = &preparedSessionToolResultCleanup{record: record}
	c.cycleMu.Unlock()
	return nil
}

func (c *SessionConversation) CommitPostSettlementToolResultCleanup(ctx context.Context, settledOperationID agentrun.OperationID) error {
	if c == nil || c.session == nil {
		return nil
	}
	c.cycleMu.Lock()
	prepared := c.pendingCleanup
	c.pendingCleanup = nil
	c.cycleMu.Unlock()
	if prepared == nil || len(prepared.record.Replacements) == 0 {
		return nil
	}
	hash, err := postSettlementContextHash(prepared.record)
	if err != nil {
		return err
	}
	commandID := postSettlementContextCommandID("auto-tool-result-cleanup", string(settledOperationID), c.session.ID, hash)
	record := prepared.record
	record.ID = postSettlementContextRecordID("trc", commandID)
	_, err = c.session.AppendToolResultCleanupAtContext(ctx, c.session.ContextCursor(), record)
	return err
}

func (c *SessionConversation) DiscardStagedToolResultCleanup() {
	if c == nil {
		return
	}
	c.cycleMu.Lock()
	c.pendingCleanup = nil
	c.cycleMu.Unlock()
}
