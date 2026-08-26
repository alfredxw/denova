package toolartifact

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

func TestWorkspaceScopeRemovalIsOwnedAndIsolated(t *testing.T) {
	workspace := t.TempDir()
	storyA := mustWorkspaceScope(t, "game",
		WorkspaceScopeOwner{Kind: "story", ID: "story/../../a"},
	)
	branchA := mustWorkspaceScope(t, "game",
		WorkspaceScopeOwner{Kind: "story", ID: "story/../../a"},
		WorkspaceScopeOwner{Kind: "branch", ID: "main"},
	)
	branchB := mustWorkspaceScope(t, "game",
		WorkspaceScopeOwner{Kind: "story", ID: "story/../../a"},
		WorkspaceScopeOwner{Kind: "branch", ID: "alternate"},
	)
	otherStoryBranch := mustWorkspaceScope(t, "game",
		WorkspaceScopeOwner{Kind: "story", ID: "story-b"},
		WorkspaceScopeOwner{Kind: "branch", ID: "main"},
	)

	branchAPath := writeScopedArtifact(t, workspace, branchA, "call-a", "branch a")
	branchBPath := writeScopedArtifact(t, workspace, branchB, "call-b", "branch b")
	otherStoryPath := writeScopedArtifact(t, workspace, otherStoryBranch, "call-other", "other story")
	branchARoot, err := WorkspaceScopeRoot(workspace, branchA)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(branchARoot, "../../a") || !strings.Contains(filepath.ToSlash(branchARoot), "/artifacts/game/story-") ||
		!strings.Contains(filepath.ToSlash(branchARoot), "/branch-") {
		t.Fatalf("scope root is not stable and opaque: %q", branchARoot)
	}

	if err := RemoveWorkspaceScope(workspace, branchA); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(branchARoot); !os.IsNotExist(err) {
		t.Fatalf("removed branch scope still exists: %v", err)
	}
	assertArtifactContent(t, branchBPath, "branch b")
	assertArtifactContent(t, otherStoryPath, "other story")
	if _, err := os.Stat(branchAPath); !os.IsNotExist(err) {
		t.Fatalf("removed branch artifact still exists: %v", err)
	}

	if err := RemoveWorkspaceScope(workspace, storyA); err != nil {
		t.Fatal(err)
	}
	storyRoot, err := WorkspaceScopeRoot(workspace, storyA)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(storyRoot); !os.IsNotExist(err) {
		t.Fatalf("removed story scope still exists: %v", err)
	}
	assertArtifactContent(t, otherStoryPath, "other story")
}

func TestWorkspaceScopeRemovalRejectsSymlinkTarget(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	outsidePath := filepath.Join(outside, "keep.txt")
	if err := os.WriteFile(outsidePath, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	scope := mustWorkspaceScope(t, "game", WorkspaceScopeOwner{Kind: "story", ID: "unsafe"})
	scopeRoot, err := WorkspaceScopeRoot(workspace, scope)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(scopeRoot), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, scopeRoot); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	if err := RemoveWorkspaceScope(workspace, scope); err == nil {
		t.Fatal("scope removal accepted a symlink target")
	}
	assertArtifactContent(t, outsidePath, "keep")
}

func TestWorkspaceScopeRequiresValidatedOwnedPath(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		namespace string
		owners    []WorkspaceScopeOwner
	}{
		{name: "missing owner", namespace: "game"},
		{name: "unsafe namespace", namespace: "../game", owners: []WorkspaceScopeOwner{{Kind: "story", ID: "a"}}},
		{name: "unsafe kind", namespace: "game", owners: []WorkspaceScopeOwner{{Kind: "../story", ID: "a"}}},
		{name: "missing ID", namespace: "game", owners: []WorkspaceScopeOwner{{Kind: "story"}}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := NewWorkspaceScope(testCase.namespace, testCase.owners...); err == nil {
				t.Fatal("invalid workspace scope was accepted")
			}
		})
	}
}

func mustWorkspaceScope(t *testing.T, namespace string, owners ...WorkspaceScopeOwner) WorkspaceScope {
	t.Helper()
	scope, err := NewWorkspaceScope(namespace, owners...)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

func writeScopedArtifact(t *testing.T, workspace string, scope WorkspaceScope, callID, content string) string {
	t.Helper()
	store, err := NewWorkspaceScopeStore(workspace, scope)
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

func assertArtifactContent(t *testing.T, path, expected string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil || string(content) != expected {
		t.Fatalf("artifact %q content=%q err=%v", path, content, err)
	}
}
