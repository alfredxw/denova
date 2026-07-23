package session

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestRefreshCanonicalReplaysOnlyAppendedJournalTail(t *testing.T) {
	dir := t.TempDir()
	writerStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	readerStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	writer, err := writerStore.GetOrCreate("incremental-refresh")
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Append(schema.UserMessage("first")); err != nil {
		t.Fatal(err)
	}
	reader, err := readerStore.GetOrCreate("incremental-refresh")
	if err != nil {
		t.Fatal(err)
	}
	initialSize := reader.journalSize

	if err := writer.Append(schema.AssistantMessage("second", nil)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Append(schema.UserMessage("third")); err != nil {
		t.Fatal(err)
	}
	if err := reader.RefreshCanonical(context.Background()); err != nil {
		t.Fatal(err)
	}

	if got := reader.MessageCountTotal(); got != 3 {
		t.Fatalf("message count = %d, want 3", got)
	}
	if got := reader.lastReplayRecords; got != 2 {
		t.Fatalf("tail replay records = %d, want 2", got)
	}
	if got, want := reader.lastReplayBytes, reader.journalSize-initialSize; got != want {
		t.Fatalf("tail replay bytes = %d, want appended delta %d", got, want)
	}
	if reader.lastReplayBytes >= reader.journalSize {
		t.Fatalf("tail refresh replayed the full journal: tail=%d journal=%d", reader.lastReplayBytes, reader.journalSize)
	}

	if err := reader.RefreshCanonical(context.Background()); err != nil {
		t.Fatal(err)
	}
	if reader.lastReplayBytes != 0 || reader.lastReplayRecords != 0 {
		t.Fatalf("unchanged refresh replayed data: bytes=%d records=%d", reader.lastReplayBytes, reader.lastReplayRecords)
	}
}

func TestRefreshCanonicalAppliesDisplayPatchToIsolatedSnapshot(t *testing.T) {
	dir := t.TempDir()
	writerStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	readerStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	writer, err := writerStore.GetOrCreate("display-refresh")
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.AppendDisplayEvent(DisplayEvent{
		ID: "call-1", Role: "tool_call", Name: "read_file", Status: "running",
		RunPath:    []string{"root", "researcher"},
		UsageCalls: []TokenUsageCall{{RequestedTools: []string{"read_file"}}},
	}); err != nil {
		t.Fatal(err)
	}
	reader, err := readerStore.GetOrCreate("display-refresh")
	if err != nil {
		t.Fatal(err)
	}
	previousDisplay := reader.records[0].display

	if err := writer.UpdateDisplayToolResult("call-1", "read_file", "success", "chapter"); err != nil {
		t.Fatal(err)
	}
	if err := reader.RefreshCanonical(context.Background()); err != nil {
		t.Fatal(err)
	}

	history := reader.History()
	if len(history) != 1 {
		t.Fatalf("history length = %d, want 1", len(history))
	}
	if history[0].Status != "success" || history[0].Result != "chapter" {
		t.Fatalf("display patch not applied: status=%q result=%q", history[0].Status, history[0].Result)
	}

	// The candidate replay must not retain nested slices from the prior
	// materialized state after it becomes canonical.
	previousDisplay.RunPath[0] = "mutated"
	previousDisplay.UsageCalls[0].RequestedTools[0] = "mutated"
	readerHistory := reader.History()
	if readerHistory[0].RunPath[0] != "root" || readerHistory[0].UsageCalls[0].RequestedTools[0] != "read_file" {
		t.Fatalf("incremental snapshot aliases prior state: %#v", readerHistory[0])
	}
}
