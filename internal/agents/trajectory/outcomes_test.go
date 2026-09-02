package trajectory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOutcomeStorePersistsInAgentsProjectStoreAndListsNewestFirst(t *testing.T) {
	stateRoot := t.TempDir()
	store, err := NewOutcomeStore(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Append(Outcome{ProjectID: "project-1", RunID: "run-1", Signal: OutcomePositive})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Append(Outcome{ProjectID: "project-1", SessionID: "session-1", Signal: OutcomeCorrection, Comment: "Keep the requested format."})
	if err != nil {
		t.Fatal(err)
	}

	items, err := store.List(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != second.ID || items[0].ID == first.ID {
		t.Fatalf("newest bounded outcomes = %#v", items)
	}
	if _, err := os.Stat(filepath.Join(stateRoot, "outcomes.jsonl")); err != nil {
		t.Fatalf("Agents Project outcome ledger missing: %v", err)
	}
}

func TestOutcomeStoreRejectsUnattributedFeedback(t *testing.T) {
	store, err := NewOutcomeStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(Outcome{Signal: OutcomeNegative}); err == nil {
		t.Fatal("outcome without a Run or Session was accepted")
	}
}
