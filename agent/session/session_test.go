package session

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/alfredxw/denova/agent"
)

func TestTranscriptClearFiltersWithoutDeletingHistory(t *testing.T) {
	handle, err := Open("story", NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := handle.Append(context.Background(), 0, agent.UserMessage("before clear"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = handle.Clear(context.Background(), snapshot.Revision)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = handle.Append(context.Background(), snapshot.Revision, agent.UserMessage("after clear"))
	if err != nil {
		t.Fatal(err)
	}

	if snapshot.Revision != 3 || len(snapshot.Entries()) != 3 {
		t.Fatalf("snapshot = %#v, want three append-only revisions", snapshot)
	}
	all := snapshot.Messages()
	effective := snapshot.EffectiveMessages()
	if len(all) != 2 || all[0].Content != "before clear" || all[1].Content != "after clear" {
		t.Fatalf("raw transcript = %#v", all)
	}
	if len(effective) != 1 || effective[0].Content != "after clear" {
		t.Fatalf("effective transcript = %#v", effective)
	}
}

func TestMemoryStoreRejectsStaleRevisionAndPreservesWinner(t *testing.T) {
	store := NewMemoryStore()
	handle, err := Open("shared", store)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := handle.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	var wait sync.WaitGroup
	wait.Add(2)
	errorsByWriter := make(chan error, 2)
	for _, content := range []string{"one", "two"} {
		content := content
		go func() {
			defer wait.Done()
			defer func() {
				if recovered := recover(); recovered != nil {
					errorsByWriter <- errors.New("writer panic")
				}
			}()
			_, appendErr := handle.Append(context.Background(), initial.Revision, agent.UserMessage(content))
			errorsByWriter <- appendErr
		}()
	}
	wait.Wait()
	close(errorsByWriter)

	successes, conflicts := 0, 0
	for appendErr := range errorsByWriter {
		switch {
		case appendErr == nil:
			successes++
		case errors.Is(appendErr, ErrRevisionConflict):
			conflicts++
		default:
			t.Fatalf("unexpected CAS error: %v", appendErr)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d, want one of each", successes, conflicts)
	}
	final, err := handle.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if final.Revision != 1 || len(final.Messages()) != 1 {
		t.Fatalf("final snapshot = %#v, want exactly one committed writer", final)
	}
}

func TestSnapshotsAndMutationsAreDeeplyIsolated(t *testing.T) {
	store := NewMemoryStore()
	message := agent.UserMessage("original")
	message.Extra = map[string]any{"nested": []any{"value"}}
	snapshot, err := store.CompareAndSwap(context.Background(), "isolated", 0, AppendMessage(message))
	if err != nil {
		t.Fatal(err)
	}
	message.Content = "caller mutation"
	message.Extra["nested"].([]any)[0] = "caller mutation"
	returned := snapshot.Messages()[0]
	returned.Content = "snapshot mutation"
	returned.Extra["nested"].([]any)[0] = "snapshot mutation"

	loaded, err := store.Load(context.Background(), "isolated")
	if err != nil {
		t.Fatal(err)
	}
	got := loaded.Messages()[0]
	if got.Content != "original" || got.Extra["nested"].([]any)[0] != "value" {
		t.Fatalf("stored message was mutated through caller alias: %#v", got)
	}
}

func TestSnapshotJSONRoundTripPreservesClearMarker(t *testing.T) {
	snapshot, err := Restore("persisted", 4, []Entry{
		{Revision: 1, Type: EntryMessage, Message: agent.UserMessage("old")},
		{Revision: 2, Type: EntryClear},
		{Revision: 4, Type: EntryMessage, Message: agent.AssistantMessage("current", nil)},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Snapshot
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ID != "persisted" || decoded.Revision != 4 || len(decoded.Messages()) != 2 {
		t.Fatalf("decoded snapshot = %#v", decoded)
	}
	effective := decoded.EffectiveMessages()
	if len(effective) != 1 || effective[0].Content != "current" {
		t.Fatalf("decoded effective transcript = %#v", effective)
	}
}

func TestSessionHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := NewMemoryStore()
	if _, err := store.Load(ctx, "canceled"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Load error = %v, want context cancellation", err)
	}
	if _, err := store.CompareAndSwap(ctx, "canceled", 0, AppendMessage(agent.UserMessage("ignored"))); !errors.Is(err, context.Canceled) {
		t.Fatalf("CompareAndSwap error = %v, want context cancellation", err)
	}
}
