package session

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentschema "github.com/alfredxw/denova/agent/schema"
)

func TestV080JournalMigrationFoldsLegacyPatchesDropsDiscardedAndDeduplicatesAssistant(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy-v080.jsonl")
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	doneStatus := "success"
	toolResult := "chapter-content"
	discarded := "discarded"
	finalPhase := DisplayPhaseFinal
	progressPhase := DisplayPhaseProgress

	rawLines := []any{
		sessionHeader{
			Type: "session", Version: 2, ID: "legacy-v080", Title: "Legacy Chat",
			CreatedAt: now, UpdatedAt: now,
		},
		messageRecord{
			Type: historyTypeMessage, CreatedAt: now,
			Message:         *agentschema.UserMessage("Check chapter 1"),
			MessageMetadata: MessageMetadata{RunID: "run-1", ContextRevision: 1},
		},
		displayRecord{
			Type: historyTypeDisplay, RecordID: "rec-prog",
			DisplayEvent: DisplayEvent{
				ID: "seg-prog", Role: "assistant", Content: "Reading chapter 1.",
				DisplayPhase: DisplayPhaseCandidate, RunID: "run-1", CreatedAt: now,
			},
		},
		displayRecord{
			Type: historyTypeDisplay, RecordID: "rec-tool",
			DisplayEvent: DisplayEvent{
				ID: "call-1", Role: "tool_call", Name: "read", Args: `{"path":`,
				Status: "running", RunID: "run-1", CreatedAt: now,
			},
		},
		displayPatchRecord{
			Type: historyTypeDisplayPatch, TargetRecordID: "rec-tool",
			ArgsAppend: `"ch1.md"}`, Status: &doneStatus, Result: &toolResult, CreatedAt: now,
		},
		displayRecord{
			Type: historyTypeDisplay, RecordID: "rec-retry",
			DisplayEvent: DisplayEvent{
				ID: "seg-retry", Role: "assistant", Content: "Broken draft",
				RunID: "run-1", CreatedAt: now,
			},
		},
		displayPatchRecord{
			Type: historyTypeDisplayPatch, TargetRecordID: "rec-retry",
			Status: &discarded, CreatedAt: now,
		},
		// Out-of-order streaming display prefix + canonical message + late tail patch.
		displayRecord{
			Type: historyTypeDisplay, RecordID: "rec-final",
			DisplayEvent: DisplayEvent{
				ID: "seg-final", Role: "assistant", Content: "Final ",
				DisplayPhase: DisplayPhaseCandidate, RunID: "run-1", CreatedAt: now,
			},
		},
		messageRecord{
			Type: historyTypeMessage, CreatedAt: now,
			Message:         *agentschema.AssistantMessage("Final answer.", nil),
			MessageMetadata: MessageMetadata{RunID: "run-1", ContextRevision: 2},
		},
		displayPatchRecord{
			Type: historyTypeDisplayPatch, TargetRecordID: "rec-final",
			ContentAppend: "answer.", DisplayPhase: &finalPhase, CreatedAt: now,
		},
		displayPatchRecord{
			Type: historyTypeDisplayPatch, TargetRecordID: "rec-prog",
			DisplayPhase: &progressPhase, CreatedAt: now,
		},
	}

	var original bytes.Buffer
	enc := json.NewEncoder(&original)
	for _, record := range rawLines {
		if err := enc.Encode(record); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(path, original.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	sess, err := store.Get("legacy-v080")
	if err != nil {
		t.Fatal(err)
	}

	// Backup must preserve original v0.8.0 bytes verbatim.
	backupBytes, err := os.ReadFile(path + v3MigrationBackupSuffix)
	if err != nil {
		t.Fatalf("missing v0.8.0 migration backup: %v", err)
	}
	if !bytes.Equal(backupBytes, original.Bytes()) {
		t.Fatal("v0.8.0 migration backup does not match original file bytes")
	}

	// Migrated journal must be v3 with zero legacy display_patch or discarded rows.
	migratedBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(migratedBytes), `"display_patch"`) {
		t.Fatalf("migrated v3 journal still contains display_patch: %s", migratedBytes)
	}
	if strings.Contains(string(migratedBytes), `"discarded"`) || strings.Contains(string(migratedBytes), "Broken draft") {
		t.Fatalf("migrated v3 journal retained discarded preview: %s", migratedBytes)
	}
	if !strings.Contains(string(migratedBytes), `"version":3`) {
		t.Fatalf("migrated journal header not upgraded to v3: %s", migratedBytes)
	}

	history := sess.History()
	if len(history) != 4 {
		t.Fatalf("migrated history len = %d, want 4 (user, progress, tool, final): %#v", len(history), history)
	}
	if history[0].Role != "user" || history[0].Content != "Check chapter 1" {
		t.Fatalf("history[0] = %#v", history[0])
	}
	if history[1].Role != "assistant" || history[1].Content != "Reading chapter 1." || history[1].DisplayPhase != DisplayPhaseProgress {
		t.Fatalf("history[1] = %#v", history[1])
	}
	if history[2].Role != "tool_call" || history[2].Args != `{"path":"ch1.md"}` || history[2].Status != "success" || history[2].Result != "chapter-content" {
		t.Fatalf("history[2] = %#v", history[2])
	}
	if history[3].Role != "assistant" || history[3].Content != "Final answer." || history[3].DisplayPhase != DisplayPhaseFinal {
		t.Fatalf("history[3] = %#v", history[3])
	}

	page, err := sess.ReadHistoryPage(context.Background(), -1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 4 || len(page.Entries) != 4 {
		t.Fatalf("paged history after v3 migration = %#v", page)
	}

	// Effective model context must remain intact.
	effective := sess.GetEffectiveMessages()
	if len(effective) != 2 || effective[0].Content != "Check chapter 1" || effective[1].Content != "Final answer." {
		t.Fatalf("effective messages changed after v3 migration: %#v", effective)
	}

	// Reopen must hit the v3 fast path without modifying the journal.
	modBefore, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	reopenedStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopenedStore.Close() })
	if _, err := reopenedStore.Get("legacy-v080"); err != nil {
		t.Fatal(err)
	}
	modAfter, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !modAfter.ModTime().Equal(modBefore.ModTime()) || modAfter.Size() != modBefore.Size() {
		t.Fatal("reopening an already-migrated v3 journal rewrote the file")
	}
}
