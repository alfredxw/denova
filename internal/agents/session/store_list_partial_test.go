package session

import (
	"os"
	"path/filepath"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

func TestListKeepsHealthySessionsWhenAnotherJournalIsUnreadable(t *testing.T) {
	const (
		healthyID = "interactive-story-st_001-healthy"
		brokenID  = "interactive-story-st_001-obsolete"
		prefix    = "interactive-story-st_001-"
	)
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	healthy, err := store.GetOrCreate(healthyID)
	if err != nil {
		t.Fatal(err)
	}
	if err := healthy.Append(agent.UserMessage("healthy conversation")); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	broken := []byte("{\"type\":\"session\",\"id\":\"interactive-story-st_001-obsolete\",\"created_at\":\"2026-01-01T00:00:00Z\"}\n{\"type\":\"obsolete_record\"}\n")
	if err := os.WriteFile(filepath.Join(dir, brokenID+".jsonl"), broken, 0o644); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	metas, err := reloaded.List(healthyID)
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 1 || metas[0].ID != healthyID || !metas[0].Active {
		t.Fatalf("healthy Session was hidden by an unrelated unreadable journal: %#v", metas)
	}
	prefixed, err := reloaded.ListByPrefix(prefix)
	if err != nil {
		t.Fatal(err)
	}
	if len(prefixed) != 1 || prefixed[0].ID != healthyID {
		t.Fatalf("healthy prefixed Session was hidden by an unrelated unreadable journal: %#v", prefixed)
	}
	if _, err := reloaded.Get(brokenID); err == nil {
		t.Fatal("direct access must still report the unreadable Session")
	}
}
