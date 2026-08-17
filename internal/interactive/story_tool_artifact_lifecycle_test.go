package interactive

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/agents/toolartifact"
)

func TestStoryAndEmptyBranchDeletionRemoveOnlyOwnedArtifacts(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	storyA, err := store.CreateStory(CreateStoryRequest{Title: "artifact story A", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(storyA.ID, AppendTurnRequest{BranchID: "main", User: "go", Narrative: "went"})
	if err != nil {
		t.Fatal(err)
	}
	branchA, err := store.CreateBranch(storyA.ID, CreateBranchRequest{ParentEventID: turn.ID, Title: "empty"})
	if err != nil {
		t.Fatal(err)
	}
	storyB, err := store.CreateStory(CreateStoryRequest{Title: "artifact story B", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}

	storyAMain := writeStoryArtifact(t, workspace, storyA.ID, "main", "call-a-main", "story a main")
	storyABranch := writeStoryArtifact(t, workspace, storyA.ID, branchA.ID, "call-a-branch", "story a branch")
	storyBMain := writeStoryArtifact(t, workspace, storyB.ID, "main", "call-b-main", "story b main")
	branchRoot := filepath.Dir(storyABranch)

	if err := store.DeleteBranch(storyA.ID, branchA.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(branchRoot); !os.IsNotExist(err) {
		t.Fatalf("deleted branch artifact root still exists: %v", err)
	}
	assertStoryArtifact(t, storyAMain, "story a main")
	assertStoryArtifact(t, storyBMain, "story b main")

	storyScope, err := storyToolArtifactScope(storyA.ID)
	if err != nil {
		t.Fatal(err)
	}
	storyRoot, err := toolartifact.WorkspaceScopeRoot(workspace, storyScope)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteStory(storyA.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(storyRoot); !os.IsNotExist(err) {
		t.Fatalf("deleted story artifact root still exists: %v", err)
	}
	assertStoryArtifact(t, storyBMain, "story b main")
}

func TestNonEmptyBranchDeleteKeepsOwnedArtifacts(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	story, err := store.CreateStory(CreateStoryRequest{Title: "protected artifacts", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "go", Narrative: "went"})
	if err != nil {
		t.Fatal(err)
	}
	branch, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: parent.ID, Title: "not empty"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: branch.ID, User: "continue", Narrative: "continued"}); err != nil {
		t.Fatal(err)
	}
	artifactPath := writeStoryArtifact(t, workspace, story.ID, branch.ID, "call-kept", "keep me")

	if err := store.DeleteBranch(story.ID, branch.ID); err == nil {
		t.Fatal("non-empty branch deletion unexpectedly succeeded")
	}
	assertStoryArtifact(t, artifactPath, "keep me")
}

func TestArtifactGarbageCollectionFailureCannotUndoCanonicalDeletion(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	story, err := store.CreateStory(CreateStoryRequest{Title: "post-commit gc", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "go", Narrative: "went"})
	if err != nil {
		t.Fatal(err)
	}
	branch, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: turn.ID, Title: "empty"})
	if err != nil {
		t.Fatal(err)
	}
	branchArtifact := writeStoryArtifact(t, workspace, story.ID, branch.ID, "call-gc", "temporary")
	branchRoot := filepath.Dir(branchArtifact)
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(outside, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("preserved"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(branchRoot); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, branchRoot); err != nil {
		t.Fatal(err)
	}

	// The hostile replacement makes bounded artifact GC fail. The branch
	// archive is nevertheless canonical and the symlink target is untouched.
	if err := store.DeleteBranch(story.ID, branch.ID); err != nil {
		t.Fatalf("committed branch deletion reported GC failure: %v", err)
	}
	branches, err := store.Branches(story.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range branches {
		if candidate.ID == branch.ID {
			t.Fatalf("archived branch remained reachable: %#v", candidate)
		}
	}
	assertStoryArtifact(t, sentinel, "preserved")
}

func writeStoryArtifact(t *testing.T, workspace, storyID, branchID, callID, content string) string {
	t.Helper()
	store, err := NewStoryToolArtifactStore(workspace, storyID, branchID)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := store.BeginToolArtifact(context.Background(), agent.ToolArtifactRequest{
		ToolName: "read", ToolCallID: callID, Purpose: agent.ToolArtifactPurposeCompleteModelOutput,
		MIMEType: "text/plain", Extension: "txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	reference, err := writer.Commit()
	if err != nil {
		t.Fatal(err)
	}
	return reference.ReadablePath
}

func assertStoryArtifact(t *testing.T, path, expected string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil || string(content) != expected {
		t.Fatalf("artifact %q content=%q err=%v", path, content, err)
	}
}
