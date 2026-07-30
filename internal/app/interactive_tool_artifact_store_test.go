package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

func TestInteractiveConversationPublishesReadableWorkspaceArtifact(t *testing.T) {
	workspace := t.TempDir()
	conversation := &interactiveConversation{workspace: workspace, storyID: "story/unsafe", branchID: "main"}
	store := conversation.ToolArtifactStore()
	if store == nil {
		t.Fatal("interactive artifact store is unavailable")
	}

	writer, err := store.BeginToolArtifact(context.Background(), agent.ToolArtifactRequest{
		ToolName: "read", ToolCallID: "call/unsafe", MIMEType: "text/plain", Extension: "log",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("complete game result")); err != nil {
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
	relative, err := filepath.Rel(canonicalWorkspace, reference.ReadablePath)
	if err != nil || relative == "." || strings.HasPrefix(relative, "..") {
		t.Fatalf("artifact escaped workspace: path=%q relative=%q err=%v", reference.ReadablePath, relative, err)
	}
	rooted := "/" + filepath.ToSlash(relative)
	if !strings.Contains(rooted, "/artifacts/game/story-") || !strings.Contains(rooted, "/branch-") ||
		strings.Contains(rooted, "story/unsafe") {
		t.Fatalf("artifact lacks stable game ownership scope: %q", rooted)
	}
	content, err := os.ReadFile(reference.ReadablePath)
	if err != nil || string(content) != "complete game result" {
		t.Fatalf("read published artifact: content=%q err=%v", content, err)
	}
}
