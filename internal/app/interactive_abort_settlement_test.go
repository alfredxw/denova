package app

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"testing"

	"denova/internal/interactive"
)

func TestAbortSettlementIsLimitedToExactGameCommand(t *testing.T) {
	root := t.TempDir()
	store := interactive.NewStore(root)
	defer store.Close()
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Parallel accepted inputs"})
	if err != nil { t.Fatal(err) }
	before, err := store.Snapshot(story.ID, "main")
	if err != nil { t.Fatal(err) }
	makeInput := func(command string) string {
		t.Helper()
		digest := sha256.Sum256([]byte(command))
		identity := interactive.DomainCommitIdentity{
			CommandID: command,
			OperationID: "external-game-" + hex.EncodeToString(digest[:16]),
			Cycle: 1,
		}
		intent, err := interactive.NewPlayerInputIntent(identity, "main", "Input for "+command)
		if err != nil { t.Fatal(err) }
		accepted, err := store.CommitPlayerInput(story.ID, intent)
		if err != nil { t.Fatal(err) }
		return accepted.Event.ID
	}
	first := makeInput("first")
	second := makeInput("second")
	if err := settleAbortedExternalGameInput(store, story.ID, "main", "first"); err != nil { t.Fatal(err) }
	if err := settleAbortedExternalGameInput(store, story.ID, "main", "first"); err != nil { t.Fatalf("retry: %v", err) }
	cold := interactive.NewStore(root)
	defer cold.Close()
	after, err := cold.Snapshot(story.ID, "main")
	if err != nil { t.Fatal(err) }
	if after.TurnCount != before.TurnCount || !reflect.DeepEqual(after.State, before.State) {
		t.Fatal("aborting a command changed committed story state")
	}
	if len(after.PendingPlayerInputs) != 1 || after.PendingPlayerInputs[0].ID != second || after.PendingPlayerInputs[0].ID == first {
		t.Fatalf("abort settled another command: %+v", after.PendingPlayerInputs)
	}
}
