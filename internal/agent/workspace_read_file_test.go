package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"

	"denova/internal/workspacechange"
)

func TestWorkspaceReadFileToolReturnsFullRevisionForPartialWindow(t *testing.T) {
	content := "first\nsecond\nthird\nfourth"
	path := writeTempFile(t, content)
	base, err := newWorkspaceReadFileTool(newTestAgentFilesystemBackend(t))
	if err != nil {
		t.Fatal(err)
	}
	result, err := base.(tool.InvokableTool).InvokableRun(context.Background(), `{"file_path":"`+path+`","offset":2,"limit":1}`)
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
	if metadata.Schema != workspaceReadFileResultSchema || metadata.RevisionScope != "full_file" {
		t.Fatalf("unexpected read metadata: %#v", metadata)
	}
	if metadata.Revision != workspacechange.Revision([]byte(content)) {
		t.Fatalf("partial read revision = %q, want full-file %q", metadata.Revision, workspacechange.Revision([]byte(content)))
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

func TestWorkspaceReadRevisionRejectsStaleAgentEdit(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "ideas.md")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	readTool, err := newWorkspaceReadFileTool(newTestAgentFilesystemBackend(t))
	if err != nil {
		t.Fatal(err)
	}
	result, err := readTool.(tool.InvokableTool).InvokableRun(context.Background(), `{"file_path":"`+path+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	metadataLine, _, ok := strings.Cut(result, "\n")
	if !ok {
		t.Fatalf("read result has no metadata line: %q", result)
	}
	var metadata workspaceReadFileMetadata
	if err := json.Unmarshal([]byte(metadataLine), &metadata); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("manual update"), 0o644); err != nil {
		t.Fatal(err)
	}
	service, err := workspacechange.NewService(workspace)
	if err != nil {
		t.Fatal(err)
	}
	editTool, err := newWorkspaceEditFileTool(service)
	if err != nil {
		t.Fatal(err)
	}
	_, err = editTool.(tool.InvokableTool).InvokableRun(context.Background(), `{"file_path":"`+path+`","base_revision":"`+metadata.Revision+`","edits":[{"old_string":"original","new_string":"agent update"}]}`)
	if err == nil {
		t.Fatal("stale read revision should reject edit_file")
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(content) != "manual update" {
		t.Fatalf("stale Agent edit overwrote manual content: %q", content)
	}
}

func TestWorkspaceReadFileToolRejectsPathOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outside := writeTempFile(t, "outside")
	base, err := newWorkspaceReadFileTool(newTestAgentFilesystemBackend(t, workspace), workspace)
	if err != nil {
		t.Fatal(err)
	}
	_, err = base.(tool.InvokableTool).InvokableRun(context.Background(), `{"file_path":"`+outside+`"}`)
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
	base, err := newWorkspaceReadFileTool(newTestAgentFilesystemBackend(t, workspace), workspace)
	if err != nil {
		t.Fatal(err)
	}
	_, err = base.(tool.InvokableTool).InvokableRun(context.Background(), `{"file_path":"`+path+`"}`)
	if err == nil || !strings.Contains(err.Error(), "selected read_file window exceeds") {
		t.Fatalf("oversized selected line should be rejected, got %v", err)
	}
}

func TestWorkspaceReadFileToolRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows")
	}
	workspace := t.TempDir()
	outside := writeTempFile(t, "outside")
	link := filepath.Join(workspace, "escape.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	base, err := newWorkspaceReadFileTool(newTestAgentFilesystemBackend(t, workspace), workspace)
	if err != nil {
		t.Fatal(err)
	}
	_, err = base.(tool.InvokableTool).InvokableRun(context.Background(), `{"file_path":"`+link+`"}`)
	if err == nil {
		t.Fatal("workspace read must not follow a symlink outside the active workspace")
	}
}
