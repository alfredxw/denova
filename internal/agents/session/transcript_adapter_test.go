package session

import (
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

func TestTranscriptSnapshotProjectsOnlyModelHistory(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("public-contract")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessage("old")); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendDisplayEvent(DisplayEvent{Role: "thinking", Content: "display only"}); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendClearMarker(); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendContextMessage(agent.UserMessage("current")); err != nil {
		t.Fatal(err)
	}

	snapshot, err := sess.TranscriptSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Entries()) != 3 || len(snapshot.Messages()) != 2 {
		t.Fatalf("public transcript leaked product records: %#v", snapshot.Entries())
	}
	effective := snapshot.EffectiveMessages()
	if len(effective) != 1 || effective[0].Content != "current" {
		t.Fatalf("public effective transcript = %#v", effective)
	}
}
