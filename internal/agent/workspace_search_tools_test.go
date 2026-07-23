package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alfredxw/denova/adk"
)

func TestWorkspaceListAndGlobStayInsideWorkspace(t *testing.T) {
	workspace := createWorkspaceSearchFixture(t)
	backend := newTestAgentFilesystemBackend(t, workspace)
	listTool, err := newWorkspaceListTool(backend)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := listTool.(adk.InvokableTool).InvokableRun(context.Background(), `{"path":"chapters"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listed, "chapters/ch01.md") || strings.Contains(listed, "notes.txt") {
		t.Fatalf("unexpected ls result: %q", listed)
	}

	globTool, err := newWorkspaceGlobTool(backend)
	if err != nil {
		t.Fatal(err)
	}
	matched, err := globTool.(adk.InvokableTool).InvokableRun(context.Background(), `{"pattern":"**/*.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(matched, "chapters/ch01.md") || strings.Contains(matched, "notes.txt") {
		t.Fatalf("unexpected glob result: %q", matched)
	}
	outside := t.TempDir()
	if _, err := listTool.(adk.InvokableTool).InvokableRun(context.Background(), `{"path":"`+outside+`"}`); err == nil {
		t.Fatal("ls must reject paths outside the workspace")
	}
}

func TestWorkspaceGrepSupportsContentFilesAndCountModes(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep is not installed")
	}
	workspace := createWorkspaceSearchFixture(t)
	backend := newTestAgentFilesystemBackend(t, workspace)
	base, err := newWorkspaceGrepTool(backend)
	if err != nil {
		t.Fatal(err)
	}
	grep := base.(adk.InvokableTool)

	content, err := grep.InvokableRun(context.Background(), `{"pattern":"dragon","path":"chapters","output_mode":"content"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "chapters/ch01.md:2:dragon wakes") {
		t.Fatalf("unexpected grep content result: %q", content)
	}
	files, err := grep.InvokableRun(context.Background(), `{"pattern":"dragon","output_mode":"files_with_matches"}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(files) != "chapters/ch01.md" {
		t.Fatalf("unexpected grep files result: %q", files)
	}
	counts, err := grep.InvokableRun(context.Background(), `{"pattern":"dragon","output_mode":"count"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(counts, "chapters/ch01.md:1") {
		t.Fatalf("unexpected grep count result: %q", counts)
	}
}

func TestWorkspaceGrepHonorsHeadLimitWithoutReturningProcessError(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep is not installed")
	}
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "many.txt"), []byte(strings.Repeat("match\n", 100)), 0o644); err != nil {
		t.Fatal(err)
	}
	tool, err := newWorkspaceGrepTool(newTestAgentFilesystemBackend(t, workspace))
	if err != nil {
		t.Fatal(err)
	}
	result, err := tool.(adk.InvokableTool).InvokableRun(context.Background(), `{"pattern":"match","output_mode":"content","head_limit":2}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(result, "many.txt:") != 2 || !strings.Contains(result, "workspace result truncated") {
		t.Fatalf("head-limited grep result = %q", result)
	}
}

func TestDisabledExecuteToolFailsClosed(t *testing.T) {
	base, err := newWorkspaceExecuteTool(nil)
	if err != nil {
		t.Fatal(err)
	}
	streamable := base.(adk.StreamableTool)
	if _, err := streamable.StreamableRun(context.Background(), `{"command":"echo unsafe"}`); err == nil || !strings.Contains(err.Error(), "capability is disabled") {
		t.Fatalf("disabled execute should fail closed, got %v", err)
	}
}

func TestWorkspaceResultCollectorKeepsHardContextLimit(t *testing.T) {
	collector := newWorkspaceResultCollector()
	if collector.Add(strings.Repeat("x", workspaceSearchMaxResultBytes)) {
		t.Fatal("oversized search entry should trigger bounded truncation")
	}
	result := collector.Result("empty")
	if len(result) > workspaceSearchMaxResultBytes || !strings.Contains(result, "1 MiB safety limit") {
		t.Fatalf("bounded search result bytes=%d result=%q", len(result), result)
	}
}

func createWorkspaceSearchFixture(t *testing.T) string {
	t.Helper()
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "chapters", "ch01.md"), []byte("opening\ndragon wakes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "notes.txt"), []byte("quiet notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return workspace
}
