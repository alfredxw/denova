package conversationjournal

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFormatBackupPrecedesAppendAndSurvivesRetry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	original := writeLegacyJournal(t, path, 1, 2)
	identity := Identity{ID: "session-test", Generation: "generation-1"}
	journal, err := Open(context.Background(), path, identity, &countingProjection{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	backupPath := path + ".pre-resilience-v1.bak"
	// An unusable backup destination must reject the format-changing write.
	if err := os.Mkdir(backupPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.AppendWithBackup(context.Background(), Guard{Cursor: journal.Head().Cursor}, "resilience-v1", rawValue(t, 3)); err == nil {
		t.Fatal("append bypassed its required backup")
	}
	unchanged, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(unchanged, original) {
		t.Fatalf("failed upgrade changed the journal: %v", err)
	}
	if err := os.Remove(backupPath); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.AppendWithBackup(context.Background(), Guard{Cursor: journal.Head().Cursor}, "resilience-v1", rawValue(t, 3)); err != nil {
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	journal, err = Open(context.Background(), path, identity, &countingProjection{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.AppendWithBackup(context.Background(), Guard{Cursor: journal.Head().Cursor}, "resilience-v1", rawValue(t, 4)); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(backupPath)
	if err != nil || !bytes.Equal(backup, original) {
		t.Fatalf("format backup replaced original data: %v", err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
}
