package interactiveapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"

	"denova/internal/agents/toolresult"
	"denova/internal/interactive"

	agent "github.com/alfredxw/denova/agent"
)

const gameTranscriptSourceKind = "denova.game.story_transcript"

// CanonicalTranscript projects the exact branch history that public Agent must
// own before admitting the next turn. It performs no model/tool preparation;
// Session.SyncTranscript validates and atomically fences this snapshot against
// the durable source identity and revision.
func (c *Conversation) CanonicalTranscript(ctx context.Context) (agent.TranscriptSyncRequest, error) {
	if c == nil || c.store == nil {
		return agent.TranscriptSyncRequest{}, fmt.Errorf("interactive canonical transcript is unavailable")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return agent.TranscriptSyncRequest{}, err
		}
	}
	storyContext, err := c.storyContextForCycle()
	if err != nil {
		return agent.TranscriptSyncRequest{}, err
	}
	turnCount := SnapshotTurnCount(storyContext.Snapshot)
	history, err := c.store.ReadModelHistory(c.storyID, interactive.StoryModelHistoryQuery{
		BranchID: storyContext.Snapshot.BranchID, StartTurn: 0, EndTurn: turnCount,
	})
	if err != nil {
		return agent.TranscriptSyncRequest{}, err
	}
	// Product checkpoints and cleanup are Agent capabilities now. Import the
	// complete unmodified canonical branch so future cleanup/compaction targets
	// stable raw message indices instead of a second Story-store projection.
	projection, err := BuildModelContextProjection(
		history, nil, storyContext.Snapshot,
		canonicalToolContextPolicy(c.ToolResultContextPolicy()), c.AgentCycleIdentitySnapshot(),
	)
	if err != nil {
		return agent.TranscriptSyncRequest{}, err
	}
	branchID := strings.TrimSpace(storyContext.Snapshot.BranchID)
	sourceRevision := storyContext.Snapshot.ContextRevision
	historicalView := c.regenerateTargetSnapshot() != ""
	if historicalView {
		// Regeneration temporarily imports the selected turn's parent while the
		// canonical branch still points at the old version. Base the monotonic
		// source clock on the live branch revision and reserve the low bit for
		// that historical view. Once the replacement commits, the next ordinary
		// projection advances to the following even revision.
		live, liveErr := c.store.StoryContext(c.storyID, branchID)
		if liveErr != nil {
			return agent.TranscriptSyncRequest{}, liveErr
		}
		sourceRevision = live.Snapshot.ContextRevision
	}
	if sourceRevision > (math.MaxUint64-1)/2 {
		return agent.TranscriptSyncRequest{}, fmt.Errorf("interactive canonical transcript revision overflow")
	}
	sourceRevision *= 2
	if historicalView {
		sourceRevision++
	}
	identityDigest := sha256.Sum256([]byte(c.storyID + "\x00" + branchID))
	source := agent.CapabilityIdentity{
		Kind: gameTranscriptSourceKind, Version: 2,
		ConfigHash: hex.EncodeToString(identityDigest[:]),
	}
	return agent.TranscriptSyncRequest{
		Source: source, SourceRevision: sourceRevision,
		Messages: projection.Messages,
	}, nil
}

func canonicalToolContextPolicy(policy toolresult.ContextPolicy) toolresult.ContextPolicy {
	// Product visibility preferences never erase canonical raw history. The
	// model-call middleware applies Enabled on a per-request projection, while
	// Cleanup/Compaction and remove/rebuild continue to address the complete
	// validated tool batch stored by public Agent.
	policy.Enabled = true
	return policy
}
