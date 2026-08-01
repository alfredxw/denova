package toolruntime

import (
	"context"
	agentinteractive "denova/internal/agents/interactive"
	agentrun "denova/internal/agents/run"
	"testing"
)

func TestAgentWorkspaceChangeMetadataUsesStableRunIdentityWithoutLedger(t *testing.T) {
	observer := agentrun.NewObserverWithIdentity(nil, "", "task-run", "session-1", "review-thread-1")
	metadata := agentWorkspaceChangeMetadata(agentrun.ContextWithObserver(context.Background(), observer))

	if metadata.ChangeGroupID != "task-run" || metadata.RunID != "task-run" {
		t.Fatalf("stable run identity was lost without a ledger: %#v", metadata)
	}
	if metadata.SessionID != "session-1" || metadata.ReviewThreadID != "review-thread-1" {
		t.Fatalf("review linkage metadata was lost: %#v", metadata)
	}
}

func TestProjectInteractiveToolContextPreservesSourceTurn(t *testing.T) {
	projected := ProjectInteractiveContext(agentinteractive.InteractiveStoryToolContext{
		StoryID: "story-1", BranchID: "main", TurnID: "turn-source",
	})
	if projected.StoryID != "story-1" || projected.BranchID != "main" || projected.TurnID != "turn-source" {
		t.Fatalf("interactive tool context projection = %#v", projected)
	}
}
