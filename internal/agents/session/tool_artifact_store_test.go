package session

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

func TestSessionToolArtifactIsImmutableAndDeletedWithSession(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetOrCreate("keep"); err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("artifact-owner")
	if err != nil {
		t.Fatal(err)
	}
	artifactStore := sess.ToolArtifactStore()
	writer, err := artifactStore.BeginToolArtifact(context.Background(), agent.ToolArtifactRequest{
		ToolName: "bash", MIMEType: "text/plain; charset=utf-8", Extension: "log",
	})
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("complete output\nwith a tail\n")
	if _, err := writer.Write(content); err != nil {
		t.Fatal(err)
	}
	reference, err := writer.Commit()
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(content)
	if reference.ByteSize != int64(len(content)) || reference.SHA256 != hex.EncodeToString(digest[:]) || reference.MIMEType == "" {
		t.Fatalf("artifact reference = %#v", reference)
	}
	stored, err := os.ReadFile(reference.URI)
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) != string(content) {
		t.Fatalf("stored artifact = %q", stored)
	}
	if err := store.Delete("artifact-owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sessionToolArtifactDirectory(sess.filePath)); !os.IsNotExist(err) {
		t.Fatalf("artifact directory survived session deletion: %v", err)
	}
}

func TestRemoveSessionJournalCleansArtifactsAfterJournalIsAlreadyGone(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "gone.jsonl")
	artifactDirectory := sessionToolArtifactDirectory(path)
	if err := os.MkdirAll(artifactDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifactDirectory, "orphan.log"), []byte("recoverable orphan"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removeSessionJournal(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(artifactDirectory); !os.IsNotExist(err) {
		t.Fatalf("orphan artifact directory survived idempotent cleanup: %v", err)
	}
}
