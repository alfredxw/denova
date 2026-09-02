package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupUnreleasedAgentTranscriptsMovesDirectoryOnce(t *testing.T) {
	dataDir := t.TempDir()
	source := filepath.Join(dataDir, "agent-transcripts")
	if err := os.MkdirAll(filepath.Join(source, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "nested", "session.jsonl"), []byte("unreleased\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	backup, err := backupUnreleasedAgentTranscripts(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(backup, filepath.Join(dataDir, "backups")+string(filepath.Separator)) {
		t.Fatalf("backup escaped data directory: %s", backup)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("unreleased source still exists: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(backup, "nested", "session.jsonl"))
	if err != nil || string(data) != "unreleased\n" {
		t.Fatalf("backup data=%q err=%v", data, err)
	}

	again, err := backupUnreleasedAgentTranscripts(dataDir)
	if err != nil || again != "" {
		t.Fatalf("idempotent backup=%q err=%v", again, err)
	}
}

func TestBackupUnreleasedAgentTranscriptsRejectsNonDirectory(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "agent-transcripts"), []byte("invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := backupUnreleasedAgentTranscripts(dataDir); err == nil {
		t.Fatal("expected non-directory source to fail")
	}
}
