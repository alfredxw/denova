package agents

import (
	"context"
	"testing"
)

func TestAgentWorkspaceChangeMetadataUsesStableRunIdentityWithoutLedger(t *testing.T) {
	observer := newRunObserverWithIdentity(nil, "", "task-run", "session-1", "review-thread-1")
	metadata := agentWorkspaceChangeMetadata(ContextWithRunObserver(context.Background(), observer))

	if metadata.ChangeGroupID != "task-run" || metadata.RunID != "task-run" {
		t.Fatalf("stable run identity was lost without a ledger: %#v", metadata)
	}
	if metadata.SessionID != "session-1" || metadata.ReviewThreadID != "review-thread-1" {
		t.Fatalf("review linkage metadata was lost: %#v", metadata)
	}
}
