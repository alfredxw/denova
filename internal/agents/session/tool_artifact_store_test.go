package session

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	agenttools "github.com/alfredxw/denova/agent/tools"
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
		ToolName: "bash", ToolCallID: "call-output", Purpose: agent.ToolArtifactPurposeCompleteModelOutput,
		MIMEType: "text/plain; charset=utf-8", Extension: "log",
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
	if reference.ByteSize != int64(len(content)) || reference.EstimatedBytes != int64(len(content)) ||
		reference.EstimatedTokens <= 0 || !reference.Complete || reference.ReadablePath == "" ||
		reference.ContentType == "" || reference.Purpose != agent.ToolArtifactPurposeCompleteModelOutput ||
		reference.SHA256 != hex.EncodeToString(digest[:]) || reference.MIMEType == "" {
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

func TestSessionToolArtifactCommitIsAtomicAndIdempotentPerCall(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("artifact-owner")
	if err != nil {
		t.Fatal(err)
	}
	artifactStore := sess.ToolArtifactStore()
	request := agent.ToolArtifactRequest{
		ToolName: "read", ToolCallID: "provider/call:42", Purpose: agent.ToolArtifactPurposeCompleteModelOutput,
		MIMEType: "text/plain; charset=utf-8", Extension: "txt",
	}
	commit := func(content string) (agent.ToolArtifactRef, error) {
		writer, beginErr := artifactStore.BeginToolArtifact(context.Background(), request)
		if beginErr != nil {
			return agent.ToolArtifactRef{}, beginErr
		}
		if _, writeErr := writer.Write([]byte(content)); writeErr != nil {
			_ = writer.Abort()
			return agent.ToolArtifactRef{}, writeErr
		}
		return writer.Commit()
	}

	first, err := commit("same complete output")
	if err != nil {
		t.Fatal(err)
	}
	second, err := commit("same complete output")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.ReadablePath != second.ReadablePath || first.SHA256 != second.SHA256 {
		t.Fatalf("idempotent replay changed artifact: first=%#v second=%#v", first, second)
	}
	if _, err := commit("conflicting replay output"); err == nil {
		t.Fatal("conflicting replay overwrote an existing call artifact")
	}
	stored, err := os.ReadFile(first.ReadablePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) != "same complete output" {
		t.Fatalf("atomic artifact changed after conflict: %q", stored)
	}
}

func TestSessionToolArtifactReadableThroughOrdinaryRead(t *testing.T) {
	workspaceRoot := t.TempDir()
	store, err := NewStore(filepath.Join(workspaceRoot, ".denova", "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("readable-artifact")
	if err != nil {
		t.Fatal(err)
	}
	writer, err := sess.ToolArtifactStore().BeginToolArtifact(context.Background(), agent.ToolArtifactRequest{
		ToolName: "web_fetch", ToolCallID: "call-readable", Purpose: agent.ToolArtifactPurposeCompleteModelOutput,
		MIMEType: "text/plain; charset=utf-8", Extension: "txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("artifact line one\nartifact line two\n")); err != nil {
		t.Fatal(err)
	}
	reference, err := writer.Commit()
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := agenttools.OpenWorkspace(workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := agenttools.LocalTextAdapter(workspace)
	if err != nil {
		t.Fatal(err)
	}
	readDefinition, err := agenttools.Read([]agenttools.ReadAdapter{adapter})
	if err != nil {
		t.Fatal(err)
	}
	arguments, err := json.Marshal(map[string]any{"path": reference.ReadablePath, "limit": 10})
	if err != nil {
		t.Fatal(err)
	}
	result, err := readDefinition.Tool.Run(context.Background(), string(arguments))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.ModelContent, "artifact line one") || !strings.Contains(result.ModelContent, "artifact line two") {
		t.Fatalf("ordinary read could not recover artifact: %q", result.ModelContent)
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
