package interactive

import (
	"errors"
	"testing"

	agentcontext "denova/internal/agents/context"
)

func TestContextCompactionRejectsBranchHeadDrift(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "压缩 CAS", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	base, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "走进大厅", Narrative: "大厅很安静。"})
	if err != nil {
		t.Fatal(err)
	}
	expected := base.ID
	if _, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "继续前进", Narrative: "分支已经前进。"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendContextCompaction(story.ID, "main", ContextCompactionEvent{
		CompactionCheckpoint: agentcontext.CompactionCheckpoint{Summary: "基于旧上下文的摘要"},
		ExpectedParentID:     &expected,
	}); !errors.Is(err, ErrStoryContextRevisionConflict) {
		t.Fatalf("stale compaction error = %v, want %v", err, ErrStoryContextRevisionConflict)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ContextCompaction != nil || len(snapshot.Turns) != 2 {
		t.Fatalf("rejected compaction changed story: %+v", snapshot)
	}
}

func TestContextCompactionRemovalRejectsBranchHeadDrift(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "压缩撤销 CAS", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "观察", Narrative: "庭院里有一口井。"})
	if err != nil {
		t.Fatal(err)
	}
	expected := turn.ID
	compaction, err := store.AppendContextCompaction(story.ID, "main", ContextCompactionEvent{
		CompactionCheckpoint: agentcontext.CompactionCheckpoint{Summary: "庭院里有井"},
		ExpectedParentID:     &expected,
	})
	if err != nil {
		t.Fatal(err)
	}
	staleHead := compaction.ID
	if _, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "查看井口", Narrative: "井水很深。"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendContextCompactionRemoval(story.ID, "main", ContextCompactionRemovalEvent{
		CompactionID: compaction.ID, ExpectedParentID: &staleHead,
	}); !errors.Is(err, ErrStoryContextRevisionConflict) {
		t.Fatalf("stale removal error = %v, want %v", err, ErrStoryContextRevisionConflict)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ContextCompaction == nil || snapshot.ContextCompaction.ID != compaction.ID || snapshot.ContextCompactionRemoval != nil {
		t.Fatalf("rejected removal changed compaction state: %+v", snapshot)
	}
}

func TestContextCompactionStructuralCommitReconcilesExactEventID(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "压缩幂等", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "观察", Narrative: "钟楼仍在响。"})
	if err != nil {
		t.Fatal(err)
	}
	expected := turn.ID
	intent := ContextCompactionEvent{
		ID: "cc-command-1",
		CompactionCheckpoint: agentcontext.CompactionCheckpoint{
			AgentKind: "interactive_story", Epoch: 1, Summary: "钟楼持续鸣响",
			RetainedTurns: 2, TriggerReason: "manual", Phase: "manual",
		},
		SourceTurnCount: 1, ExpectedParentID: &expected,
	}
	first, err := store.AppendContextCompaction(story.ID, "main", intent)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.AppendContextCompaction(story.ID, "main", intent)
	if err != nil {
		t.Fatalf("exact compaction retry after branch-head advance failed: %v", err)
	}
	if second.ID != first.ID || second.ParentID != first.ParentID || second.CompactionCheckpoint != intent.CompactionCheckpoint {
		t.Fatalf("retry created a different compaction: first=%#v second=%#v", first, second)
	}
	conflictingReplay := intent
	conflictingReplay.RecoveryBand = 0.75
	if _, err := store.AppendContextCompaction(story.ID, "main", conflictingReplay); !errors.Is(err, ErrStoryContextRevisionConflict) {
		t.Fatalf("same event id with changed durable fields error = %v, want %v", err, ErrStoryContextRevisionConflict)
	}
	removeExpected := first.ID
	removeIntent := ContextCompactionRemovalEvent{
		ID: "ccr-command-1", AgentKind: "interactive_story", CompactionID: first.ID,
		SourceTurnCount: 1, Reason: "user_removed", ExpectedParentID: &removeExpected,
	}
	removed, err := store.AppendContextCompactionRemoval(story.ID, "main", removeIntent)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := store.AppendContextCompactionRemoval(story.ID, "main", removeIntent)
	if err != nil {
		t.Fatalf("exact removal retry after branch-head advance failed: %v", err)
	}
	if replayed.ID != removed.ID || replayed.ParentID != removed.ParentID {
		t.Fatalf("retry created a different removal: first=%#v second=%#v", removed, replayed)
	}
}
