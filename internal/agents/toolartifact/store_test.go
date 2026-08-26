package toolartifact

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

func TestWorkspaceStorePublishesInsideWorkspaceScope(t *testing.T) {
	workspace := t.TempDir()
	store, err := NewWorkspaceStore(workspace, "story/../../sensitive-title")
	if err != nil {
		t.Fatal(err)
	}
	request := agent.ToolArtifactRequest{
		ToolName: "read", ToolCallID: "call/../../42", Purpose: agent.ToolArtifactPurposeCompleteModelOutput,
		MIMEType: "text/plain; charset=utf-8", Extension: "log",
	}
	writer, err := store.BeginToolArtifact(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("complete game tool result")); err != nil {
		t.Fatal(err)
	}
	reference, err := writer.Commit()
	if err != nil {
		t.Fatal(err)
	}
	canonicalWorkspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(canonicalWorkspace, filepath.FromSlash(reference.ReadablePath))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		t.Fatalf("artifact escaped workspace: path=%q relative=%q err=%v", reference.ReadablePath, relative, err)
	}
	if strings.Contains(reference.ReadablePath, "sensitive-title") || strings.Contains(reference.ReadablePath, "../") ||
		!strings.Contains(filepath.ToSlash(relative), "/artifacts/scope-") || !reference.Complete ||
		reference.Purpose != agent.ToolArtifactPurposeCompleteModelOutput {
		t.Fatalf("unsafe or incomplete artifact reference: %#v", reference)
	}
	content, err := os.ReadFile(reference.ReadablePath)
	if err != nil || string(content) != "complete game tool result" {
		t.Fatalf("artifact content=%q err=%v", content, err)
	}
	if err := store.VerifyToolArtifact(context.Background(), reference, request); err != nil {
		t.Fatalf("store rejected its own artifact: %v", err)
	}
	forged := reference
	forged.ID = "forged"
	if err := store.VerifyToolArtifact(context.Background(), forged, request); err == nil {
		t.Fatal("store accepted a forged artifact identity")
	}
}

func TestWorkspaceStoreDefaultsUnspecifiedArtifactPurposeToAttachment(t *testing.T) {
	store, err := NewWorkspaceStore(t.TempDir(), "attachment-scope")
	if err != nil {
		t.Fatal(err)
	}
	writer, err := store.BeginToolArtifact(context.Background(), agent.ToolArtifactRequest{
		ToolName: "render", ToolCallID: "attachment-call", MIMEType: "image/png", Extension: "png",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("attachment")); err != nil {
		t.Fatal(err)
	}
	reference, err := writer.Commit()
	if err != nil {
		t.Fatal(err)
	}
	if reference.Purpose != agent.ToolArtifactPurposeAttachment {
		t.Fatalf("default artifact purpose = %q", reference.Purpose)
	}
}

func TestWorkspaceStoreSeparatesArtifactPurposesForOneToolCall(t *testing.T) {
	store, err := NewWorkspaceStore(t.TempDir(), "purpose-scope")
	if err != nil {
		t.Fatal(err)
	}
	commit := func(purpose agent.ToolArtifactPurpose, content string) agent.ToolArtifactRef {
		writer, beginErr := store.BeginToolArtifact(context.Background(), agent.ToolArtifactRequest{
			ToolName: "render", ToolCallID: "same-call", Purpose: purpose,
			MIMEType: "text/plain", Extension: "txt",
		})
		if beginErr != nil {
			t.Fatal(beginErr)
		}
		if _, writeErr := writer.Write([]byte(content)); writeErr != nil {
			t.Fatal(writeErr)
		}
		reference, commitErr := writer.Commit()
		if commitErr != nil {
			t.Fatal(commitErr)
		}
		return reference
	}
	attachment := commit(agent.ToolArtifactPurposeAttachment, "attachment")
	output := commit(agent.ToolArtifactPurposeCompleteModelOutput, "complete output")
	raw := commit(agent.ToolArtifactPurposeCompleteToolOutput, "raw output")
	if attachment.ID == output.ID || attachment.ID == raw.ID || output.ID == raw.ID ||
		attachment.ReadablePath == output.ReadablePath || output.ReadablePath == raw.ReadablePath ||
		attachment.Purpose != agent.ToolArtifactPurposeAttachment ||
		output.Purpose != agent.ToolArtifactPurposeCompleteModelOutput ||
		raw.Purpose != agent.ToolArtifactPurposeCompleteToolOutput {
		t.Fatalf("artifact purposes collided: attachment=%#v output=%#v raw=%#v", attachment, output, raw)
	}
}

func TestBoundedStoreRejectsArtifactRootOutsideBoundary(t *testing.T) {
	boundary := t.TempDir()
	outside := t.TempDir()
	if _, err := NewBoundedStore(boundary, filepath.Join(outside, "artifacts")); err == nil {
		t.Fatal("bounded store accepted a root outside its boundary")
	}
}

func TestWorkspaceStoreConcurrentReplayIsIdempotent(t *testing.T) {
	store, err := NewWorkspaceStore(t.TempDir(), "story-session-1")
	if err != nil {
		t.Fatal(err)
	}
	const writers = 8
	references := make(chan agent.ToolArtifactRef, writers)
	errors := make(chan error, writers)
	var group sync.WaitGroup
	for range writers {
		group.Add(1)
		go func() {
			defer group.Done()
			writer, beginErr := store.BeginToolArtifact(context.Background(), agent.ToolArtifactRequest{
				ToolName: "read", ToolCallID: "same-call", Purpose: agent.ToolArtifactPurposeCompleteModelOutput,
				MIMEType: "text/plain", Extension: "txt",
			})
			if beginErr != nil {
				errors <- beginErr
				return
			}
			if _, writeErr := writer.Write([]byte("same output")); writeErr != nil {
				_ = writer.Abort()
				errors <- writeErr
				return
			}
			reference, commitErr := writer.Commit()
			if commitErr != nil {
				errors <- commitErr
				return
			}
			references <- reference
		}()
	}
	group.Wait()
	close(references)
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	var first agent.ToolArtifactRef
	count := 0
	for reference := range references {
		if count == 0 {
			first = reference
		} else if reference.ID != first.ID || reference.ReadablePath != first.ReadablePath || reference.SHA256 != first.SHA256 {
			t.Fatalf("concurrent replay diverged: first=%#v next=%#v", first, reference)
		}
		count++
	}
	if count != writers {
		t.Fatalf("committed references = %d, want %d", count, writers)
	}
}

func TestWorkspaceStoreRepairsOwnedArtifactPermissionsWithoutChangingParents(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose POSIX permission bits")
	}
	workspace := t.TempDir()
	if err := os.Chmod(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := NewWorkspaceStore(workspace, "legacy-permission-scope")
	if err != nil {
		t.Fatal(err)
	}
	artifactRoot := filepath.Join(store.boundaryRoot, filepath.FromSlash(store.artifactRoot))
	artifactParent := filepath.Dir(artifactRoot)
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	// MkdirAll is affected by the process umask; force the legacy modes that
	// this regression test is intended to repair or preserve.
	if err := os.Chmod(artifactParent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(artifactRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	request := agent.ToolArtifactRequest{
		ToolName: "read", ToolCallID: "legacy-permission-call",
		Purpose:  agent.ToolArtifactPurposeCompleteToolOutput,
		MIMEType: "text/plain", Extension: "txt",
	}
	writer, err := store.BeginToolArtifact(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	assertPathPermissions(t, artifactRoot, 0o700)
	assertPathPermissions(t, workspace, 0o755)
	assertPathPermissions(t, artifactParent, 0o755)
	if _, err := writer.Write([]byte("legacy output")); err != nil {
		t.Fatal(err)
	}
	reference, err := writer.Commit()
	if err != nil {
		t.Fatal(err)
	}
	assertPathPermissions(t, reference.ReadablePath, 0o600)

	// Simulate a legacy scope and artifact, then exercise the idempotent replay
	// path. Begin repairs only the owned leaf; Commit repairs the matched final
	// artifact after verifying its immutable identity and content.
	if err := os.Chmod(artifactRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(reference.ReadablePath, 0o644); err != nil {
		t.Fatal(err)
	}
	replayWriter, err := store.BeginToolArtifact(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	assertPathPermissions(t, artifactRoot, 0o700)
	assertPathPermissions(t, reference.ReadablePath, 0o644)
	if _, err := replayWriter.Write([]byte("legacy output")); err != nil {
		t.Fatal(err)
	}
	replayed, err := replayWriter.Commit()
	if err != nil {
		t.Fatal(err)
	}
	if replayed.ID != reference.ID || replayed.ReadablePath != reference.ReadablePath {
		t.Fatalf("replayed artifact changed identity: first=%#v replay=%#v", reference, replayed)
	}
	assertPathPermissions(t, replayed.ReadablePath, 0o600)
	assertPathPermissions(t, workspace, 0o755)
	assertPathPermissions(t, artifactParent, 0o755)
}

func assertPathPermissions(t *testing.T, path string, expected os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if actual := info.Mode().Perm(); actual != expected {
		t.Fatalf("permissions for %q = %#o, want %#o", path, actual, expected)
	}
}
