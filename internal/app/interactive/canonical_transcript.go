package interactiveapp

import (
	"context"
	agentrun "denova/internal/agents/run"
	"fmt"

	"denova/internal/agents/toolresult"
	"denova/internal/interactive"

	agent "github.com/alfredxw/denova/agent"
)

// CanonicalMessages projects the exact Story branch model history. Story JSONL
// is the sole durable conversation lane; Agent keeps only an in-memory copy.
func (c *Conversation) CanonicalMessages(ctx context.Context) ([]*agent.Message, error) {
	if c == nil || c.store == nil {
		return nil, fmt.Errorf("interactive canonical transcript is unavailable")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	storyContext, err := c.storyContextForCycle()
	if err != nil {
		return nil, err
	}
	return c.canonicalMessagesForSnapshot(storyContext.Snapshot)
}

func (c *Conversation) canonicalMessagesForSnapshot(snapshot interactive.Snapshot) ([]*agent.Message, error) {
	turnCount := SnapshotTurnCount(snapshot)
	history, err := c.store.ReadModelHistory(c.storyID, interactive.StoryModelHistoryQuery{
		BranchID: snapshot.BranchID, StartTurn: 0, EndTurn: turnCount,
	})
	if err != nil {
		return nil, err
	}
	// Import the complete canonical branch so incremental Compaction targets
	// stable raw message indices instead of a second Story-store projection.
	projection, err := buildModelContextProjection(
		history, nil, snapshot,
		canonicalToolContextPolicy(c.ToolResultContextPolicy()), agentrun.CycleIdentity{},
		func(input interactive.PlayerInputAcceptedEvent) *agent.Message {
			return agent.UserMessageWithAttachments(input.Text, input.Attachments)
		},
	)
	if err != nil {
		return nil, err
	}
	return projection.Messages, nil
}

func canonicalToolContextPolicy(policy toolresult.ContextPolicy) toolresult.ContextPolicy {
	// Product visibility preferences never erase canonical raw history. The
	// model-call middleware applies Enabled on a per-request projection, while
	// Compaction and remove/rebuild continue to address the complete
	// validated tool batch stored by public Agent.
	policy.Enabled = true
	return policy
}
