package interactive

import (
	"bytes"
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"denova/internal/localfs"
)

func TestReadSceneAtTurnPreservesDiscardedVersionAndBranchAncestry(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	story, err := store.CreateStory(CreateStoryRequest{Title: "Recorded scenes"})
	if err != nil {
		t.Fatal(err)
	}
	appendTurn := func(branch, text string, affection, count int) TurnEvent {
		t.Helper()
		turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: branch, Narrative: text, ActorOps: []ActorStateOp{
			{Op: "set", ActorID: "protagonist", FieldID: "affection", Value: affection},
			{Op: "set", ActorID: "protagonist", FieldID: "inventory", Value: map[string]any{"medicine": count}},
			{Op: "set", ActorID: "protagonist", FieldID: "memory", Value: []string{text}},
		}})
		if err != nil {
			t.Fatal(err)
		}
		return turn
	}
	first := appendTurn("main", "Opening", 1, 0)
	original := appendTurn("main", "Original promise", 7, 1)
	originalSnapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RewindToTurnParent(story.ID, RewindTurnRequest{BranchID: "main", TurnID: original.ID}); err != nil {
		t.Fatal(err)
	}
	alternative := appendTurn("main", "Different promise", 3, 9)
	appendTurn("main", "Future on alternative", 99, 0)
	branch, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: first.ID, Title: "Other branch"})
	if err != nil {
		t.Fatal(err)
	}
	appendTurn(branch.ID, "Other branch future", 50, 5)
	before, err := store.Snapshot(story.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = NewStore(workspace)
	t.Cleanup(func() { _ = store.Close() })
	journalBefore, err := os.ReadFile(store.storyPath(story.ID))
	if err != nil {
		t.Fatal(err)
	}
	scene, err := store.ReadSceneAtTurn(context.Background(), story.ID, "main", original.ID, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if scene.Turn.ID != original.ID || scene.Turn.Narrative != "Original promise" || scene.PreviousTurn == nil || scene.PreviousTurn.ID != first.ID || scene.State.SourceRevision != original.ID {
		t.Fatalf("wrong recorded scene: %+v", scene)
	}
	if !reflect.DeepEqual(scene.State.State, originalSnapshot.State) {
		t.Fatalf("old version state differs from its original checkpoint: got=%#v want=%#v", scene.State.State, originalSnapshot.State)
	}
	if _, err := store.ReadSceneAtTurn(context.Background(), story.ID, branch.ID, alternative.ID, alternative.ID); !errors.Is(err, ErrStorySceneNotFound) {
		t.Fatalf("parent future accepted on child: %v", err)
	}
	inherited, err := store.ReadSceneAtTurn(context.Background(), story.ID, branch.ID, first.ID, first.ID)
	if err != nil || inherited.PreviousTurn != nil {
		t.Fatalf("inherited opening = %+v %v", inherited, err)
	}
	if _, err := store.ReadSceneAtTurn(context.Background(), story.ID, "missing", first.ID, first.ID); !errors.Is(err, ErrStorySceneNotFound) {
		t.Fatalf("missing branch error = %v", err)
	}
	if _, err := store.ReadSceneAtTurn(context.Background(), story.ID, "main", "missing", "missing"); !errors.Is(err, ErrStorySceneNotFound) {
		t.Fatalf("missing turn error = %v", err)
	}
	journalAfter, err := os.ReadFile(store.storyPath(story.ID))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(journalBefore, journalAfter) {
		t.Fatal("scene read changed the canonical journal")
	}
	after, err := store.Snapshot(story.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("scene read changed the current continuation")
	}
}

func TestReadSceneAtTurnRejectsChangedNarrativeRevision(t *testing.T) {
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "Edited scene"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", Narrative: "Original text"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateTurnNarrative(story.ID, UpdateTurnNarrativeRequest{BranchID: "main", TurnID: turn.ID, Narrative: "Corrected text"}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadSceneAtTurn(context.Background(), story.ID, "main", turn.ID, turn.ID); !errors.Is(err, ErrStorySceneRevisionConflict) {
		t.Fatalf("stale prose revision accepted: %v", err)
	}
	revision := TurnNarrativeRevision(*snapshot.CurrentTurn)
	scene, err := store.ReadSceneAtTurn(context.Background(), story.ID, "main", turn.ID, revision)
	if err != nil || scene.Turn.Narrative != "Corrected text" || scene.State.SourceRevision != revision {
		t.Fatalf("edited scene mismatch: %+v %v", scene, err)
	}
}

func TestReadSceneAtTurnDoesNotRepairTornJournalAndHonorsCancellation(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	story, err := store.CreateStory(CreateStoryRequest{Title: "Read-only scene"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", Narrative: "Existing prose"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	journalPath := store.storyPath(story.ID)
	file, err := os.OpenFile(journalPath, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"incomplete":`); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	store = NewStore(workspace)
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.ReadSceneAtTurn(context.Background(), story.ID, "main", turn.ID, turn.ID); err == nil {
		t.Fatal("incomplete journal accepted")
	}
	after, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("read-only scene repaired the journal")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.ReadSceneAtTurn(ctx, story.ID, "main", turn.ID, turn.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled scene read = %v", err)
	}
}

func TestReadSceneAtTurnCancelsWhileWriterOwnsNativeLease(t *testing.T) {
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "Scene lease contention"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", Narrative: "Existing prose"})
	if err != nil {
		t.Fatal(err)
	}
	release, err := localfs.AcquireLease(context.Background(), store.storyPath(story.ID)+".mutation.lock")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	// A writer normally owns both locks. The independent reader must not first
	// enter an uninterruptible Store mutex wait before attempting the OS lease.
	store.mu.Lock()
	defer store.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	if _, err := store.ReadSceneAtTurn(ctx, story.ID, "main", turn.ID, turn.ID); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("scene lease contention did not preserve cancellation: %v", err)
	}
}

func TestReadSceneAtTurnLongJournalKeepsOnlySelectedPublicTurns(t *testing.T) {
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "Long scene archive"})
	if err != nil {
		t.Fatal(err)
	}
	opening, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", Narrative: "Opening"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", Narrative: "Selected prose", Thinking: "PRIVATE_SELECTED", ModelContextMessages: []ModelContextMessage{{Role: "tool", ToolName: "lookup", ToolCallID: "selected", Content: "PRIVATE_SELECTED_CONTEXT"}}})
	if err != nil {
		t.Fatal(err)
	}
	requests := make([]AppendTurnRequest, 384)
	for index := range requests {
		requests[index] = AppendTurnRequest{
			Narrative:            strings.Repeat("Unrelated future prose. ", 20),
			Thinking:             strings.Repeat("PRIVATE_REASONING ", 128),
			ModelContextMessages: []ModelContextMessage{{Role: "tool", ToolName: "lookup", ToolCallID: "future", Content: strings.Repeat("PRIVATE_CONTEXT ", 256)}},
		}
	}
	appendStoryTurns(t, store, story.ID, "main", requests)
	before, err := os.ReadFile(store.storyPath(story.ID))
	if err != nil {
		t.Fatal(err)
	}
	if len(before) < 2*1024*1024 {
		t.Fatalf("long journal fixture too small: %d bytes", len(before))
	}
	if !bytes.Contains(before, []byte("PRIVATE_SELECTED_CONTEXT")) || !bytes.Contains(before, []byte("PRIVATE_CONTEXT")) {
		t.Fatal("long journal fixture omitted native private tool context")
	}
	scene, err := store.ReadSceneAtTurn(context.Background(), story.ID, "main", target.ID, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if scene.Turn.ID != target.ID || scene.Turn.Narrative != "Selected prose" || scene.PreviousTurn == nil || scene.PreviousTurn.ID != opening.ID {
		t.Fatalf("wrong selected scene: %+v", scene)
	}
	if scene.Turn.Thinking != "" || len(scene.Turn.ModelContextMessages) != 0 || len(scene.Turn.DisplayEvents) != 0 || len(scene.Turn.ProviderContinuation) != 0 {
		t.Fatal("scene projection retained private model context")
	}
	// Cancel after scanning has started, rather than supplying an already
	// cancelled context. ReadRange and per-event decoding both honor this ctx.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := store.ReadSceneAtTurn(ctx, story.ID, "main", target.ID, target.ID); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("long journal read did not honor in-flight cancellation: %v", err)
	}
	after, err := os.ReadFile(store.storyPath(story.ID))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("long or cancelled scene read changed canonical journal")
	}
	t.Logf("projected two public turns from %d-turn, %d-byte native journal", len(requests)+2, len(before))
}
