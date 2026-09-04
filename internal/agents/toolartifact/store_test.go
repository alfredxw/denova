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

func TestStateStorePublishesPortablePathInsideStateScope(t *testing.T) {
	stateRoot := t.TempDir()
	store, err := NewStateStore(stateRoot, "story/../../sensitive-title")
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
	if filepath.IsAbs(reference.ReadablePath) || strings.Contains(reference.ReadablePath, "\\") {
		t.Fatalf("artifact reference is not portable: %q", reference.ReadablePath)
	}
	if strings.Contains(reference.ReadablePath, "sensitive-title") || strings.Contains(reference.ReadablePath, "../") ||
		!strings.HasPrefix(reference.ReadablePath, "artifacts/scope-") || !reference.Complete ||
		reference.Purpose != agent.ToolArtifactPurposeCompleteModelOutput {
		t.Fatalf("unsafe or incomplete artifact reference: %#v", reference)
	}
	runtimePath, err := store.ResolveToolArtifactPath(context.Background(), reference.ReadablePath)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(runtimePath)
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

func TestStateStoreDefaultsUnspecifiedArtifactPurposeToAttachment(t *testing.T) {
	store, err := NewStateStore(t.TempDir(), "attachment-scope")
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

func TestStateStoreReferenceSurvivesDataRootMove(t *testing.T) {
	parent := t.TempDir()
	oldRoot := filepath.Join(parent, "old-root")
	newRoot := filepath.Join(parent, "new-root")
	if err := os.MkdirAll(oldRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := NewStateStore(oldRoot, "portable-session")
	if err != nil {
		t.Fatal(err)
	}
	request := agent.ToolArtifactRequest{
		ToolName: "read", ToolCallID: "portable-call", Purpose: agent.ToolArtifactPurposeCompleteModelOutput,
		MIMEType: "text/plain", Extension: "txt",
	}
	writer, err := store.BeginToolArtifact(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("portable output")); err != nil {
		t.Fatal(err)
	}
	reference, err := writer.Commit()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(oldRoot, newRoot); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewStateStore(newRoot, "portable-session")
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.VerifyToolArtifact(context.Background(), reference, request); err != nil {
		t.Fatalf("moved store rejected durable reference: %v", err)
	}
	resolved, err := reopened.ResolveToolArtifactPath(context.Background(), reference.ReadablePath)
	if err != nil {
		t.Fatal(err)
	}
	canonicalNewRoot, err := filepath.EvalSymlinks(newRoot)
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(canonicalNewRoot, resolved)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(filepath.ToSlash(reference.ReadablePath), filepath.ToSlash(parent)) ||
		relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		t.Fatalf("reference did not rebase: durable=%q runtime=%q", reference.ReadablePath, resolved)
	}
	content, err := os.ReadFile(resolved)
	if err != nil || string(content) != "portable output" {
		t.Fatalf("moved artifact content=%q err=%v", content, err)
	}
}

func TestStateStoreSeparatesArtifactPurposesForOneToolCall(t *testing.T) {
	store, err := NewStateStore(t.TempDir(), "purpose-scope")
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

func TestStateStoreConcurrentReplayIsIdempotent(t *testing.T) {
	store, err := NewStateStore(t.TempDir(), "story-session-1")
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

func TestStateStoreRepairsOwnedArtifactPermissionsWithoutChangingParents(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose POSIX permission bits")
	}
	stateRoot := t.TempDir()
	if err := os.Chmod(stateRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := NewStateStore(stateRoot, "legacy-permission-scope")
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
	assertPathPermissions(t, stateRoot, 0o755)
	assertPathPermissions(t, artifactParent, 0o755)
	if _, err := writer.Write([]byte("legacy output")); err != nil {
		t.Fatal(err)
	}
	reference, err := writer.Commit()
	if err != nil {
		t.Fatal(err)
	}
	runtimePath, err := store.ResolveToolArtifactPath(context.Background(), reference.ReadablePath)
	if err != nil {
		t.Fatal(err)
	}
	assertPathPermissions(t, runtimePath, 0o600)

	// Simulate a legacy scope and artifact, then exercise the idempotent replay
	// path. Begin repairs only the owned leaf; Commit repairs the matched final
	// artifact after verifying its immutable identity and content.
	if err := os.Chmod(artifactRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(runtimePath, 0o644); err != nil {
		t.Fatal(err)
	}
	replayWriter, err := store.BeginToolArtifact(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	assertPathPermissions(t, artifactRoot, 0o700)
	assertPathPermissions(t, runtimePath, 0o644)
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
	assertPathPermissions(t, runtimePath, 0o600)
	assertPathPermissions(t, stateRoot, 0o755)
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
