package interactiveapp

import (
	"context"
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
	turnCount := SnapshotTurnCount(storyContext.Snapshot)
	history, err := c.store.ReadModelHistory(c.storyID, interactive.StoryModelHistoryQuery{
		BranchID: storyContext.Snapshot.BranchID, StartTurn: 0, EndTurn: turnCount,
	})
	if err != nil {
		return nil, err
	}
	// Product checkpoints and cleanup are Agent capabilities now. Import the
	// complete unmodified canonical branch so future cleanup/compaction targets
	// stable raw message indices instead of a second Story-store projection.
	projection, err := BuildModelContextProjection(
		history, nil, storyContext.Snapshot,
		canonicalToolContextPolicy(c.ToolResultContextPolicy()), c.AgentCycleIdentitySnapshot(),
	)
	if err != nil {
		return nil, err
	}
	return projection.Messages, nil
}

func canonicalToolContextPolicy(policy toolresult.ContextPolicy) toolresult.ContextPolicy {
	// Product visibility preferences never erase canonical raw history. The
	// model-call middleware applies Enabled on a per-request projection, while
	// Cleanup/Compaction and remove/rebuild continue to address the complete
	// validated tool batch stored by public Agent.
	policy.Enabled = true
	return policy
}
