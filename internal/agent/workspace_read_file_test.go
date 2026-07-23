package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	adk "github.com/alfredxw/denova/adk"
)

func TestWorkspaceReadFileToolReturnsPartialWindowWithoutRevision(t *testing.T) {
	content := "first\nsecond\nthird\nfourth"
	workspace := t.TempDir()
	path := filepath.Join(workspace, "story.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	base, err := newWorkspaceReadFileTool(newTestAgentFilesystemBackend(t, workspace))
	if err != nil {
		t.Fatal(err)
	}
	result, err := base.(adk.InvokableTool).InvokableRun(context.Background(), `{"file_path":"`+path+`","offset":2,"limit":1}`)
	if err != nil {
		t.Fatal(err)
	}
	metadataLine, body, ok := strings.Cut(result, "\n")
	if !ok {
		t.Fatalf("read result has no metadata line: %q", result)
	}
	var metadata workspaceReadFileMetadata
	if err := json.Unmarshal([]byte(metadataLine), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.Schema != workspaceReadFileResultSchema || metadata.Offset != 2 || metadata.Limit != 1 {
		t.Fatalf("unexpected read metadata: %#v", metadata)
	}
	var rawMetadata map[string]any
	if err := json.Unmarshal([]byte(metadataLine), &rawMetadata); err != nil {
		t.Fatal(err)
	}
	if _, ok := rawMetadata["revision"]; ok {
		t.Fatalf("read_file exposed internal revision: %s", metadataLine)
	}
	if _, ok := rawMetadata["revision_scope"]; ok {
		t.Fatalf("read_file exposed revision metadata: %s", metadataLine)
	}
	if !strings.Contains(body, "     2\tsecond") || strings.Contains(body, "first") || strings.Contains(body, "third") {
		t.Fatalf("partial cat-n selection mismatch: %q", body)
	}
}

func TestWorkspaceReadFileToolPreservesDefaultWindowSchema(t *testing.T) {
	base, err := newWorkspaceReadFileTool(newTestAgentFilesystemBackend(t))
	if err != nil {
		t.Fatal(err)
	}
	info, err := base.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	for _, property := range []string{`"file_path"`, `"offset"`, `"limit"`} {
		if !strings.Contains(string(raw), property) {
			t.Fatalf("read_file schema is missing %s: %s", property, raw)
		}
	}
}

func TestWorkspaceReadFileToolRejectsPathOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	base, err := newWorkspaceReadFileTool(newTestAgentFilesystemBackend(t, workspace))
	if err != nil {
		t.Fatal(err)
	}
	_, err = base.(adk.InvokableTool).InvokableRun(context.Background(), `{"file_path":"`+outside+`"}`)
	if err == nil || !strings.Contains(err.Error(), "outside the active workspace") {
		t.Fatalf("outside read should be rejected, got %v", err)
	}
}

func TestWorkspaceReadFileToolBoundsOneVeryLongLine(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "long.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", workspaceReadFileMaxSelectedBytes+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	base, err := newWorkspaceReadFileTool(newTestAgentFilesystemBackend(t, workspace))
	if err != nil {
		t.Fatal(err)
	}
	_, err = base.(adk.InvokableTool).InvokableRun(context.Background(), `{"file_path":"`+path+`"}`)
	if err == nil || !strings.Contains(err.Error(), "selected read_file window exceeds") {
		t.Fatalf("oversized selected line should be rejected, got %v", err)
	}
}

func TestWorkspaceReadFileToolRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows")
	}
	workspace := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(workspace, "escape.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	base, err := newWorkspaceReadFileTool(newTestAgentFilesystemBackend(t, workspace))
	if err != nil {
		t.Fatal(err)
	}
	_, err = base.(adk.InvokableTool).InvokableRun(context.Background(), `{"file_path":"`+link+`"}`)
	if err == nil {
		t.Fatal("workspace read must not follow a symlink outside the active workspace")
	}
}

func TestWorkspaceReadFileToolAcceptsWorkspaceRelativePath(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "chapters", "ch01.md"), []byte("opening"), 0o644); err != nil {
		t.Fatal(err)
	}
	base, err := newWorkspaceReadFileTool(newTestAgentFilesystemBackend(t, workspace))
	if err != nil {
		t.Fatal(err)
	}
	result, err := base.(adk.InvokableTool).InvokableRun(context.Background(), `{"file_path":"chapters/ch01.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "     1\topening") {
		t.Fatalf("unexpected relative read result: %q", result)
	}
}
