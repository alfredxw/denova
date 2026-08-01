package change

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanTextEditsRejectsUnboundedFilesAndReplacementSets(t *testing.T) {
	if matches := literalMatches(strings.Repeat("a", 100_000), "a", 2); len(matches) != 2 {
		t.Fatalf("bounded uniqueness probe returned %d matches", len(matches))
	}
	if _, _, err := planTextEdits("large.txt", strings.Repeat("x", maxWorkspaceMutationFileBytes+1), []TextEdit{{
		OldString: "x", NewString: "y",
	}}, false); err == nil {
		t.Fatal("oversized base file was accepted")
	}
	if _, _, err := planTextEdits("many.txt", strings.Repeat("a", maxWorkspaceMutationReplacements+1), []TextEdit{{
		OldString: "a", NewString: "b", ReplaceAll: true,
	}}, false); err == nil || !strings.Contains(err.Error(), "replacement limit") {
		t.Fatalf("unbounded replace_all error = %v", err)
	}
	if _, _, err := planTextEdits("expanded.txt", strings.Repeat("a", 9_000), []TextEdit{{
		OldString: "a", NewString: strings.Repeat("b", 2_000), ReplaceAll: true,
	}}, false); err == nil || !strings.Contains(err.Error(), "file limit") {
		t.Fatalf("oversized replacement result error = %v", err)
	}
}

func TestWorkspaceChangeReadRejectsOversizedRegularFile(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "large.txt")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxWorkspaceMutationFileBytes + 1); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	service, err := NewService(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.ReadFile("large.txt"); err == nil || !strings.Contains(err.Error(), "mutation limit") {
		t.Fatalf("oversized workspace file error = %v", err)
	}
	if _, err := service.ReplaceFile(context.Background(), ReplaceFileRequest{
		Path: "new.txt", BaseRevision: "missing", Content: strings.Repeat("x", maxWorkspaceMutationFileBytes+1),
	}); err == nil {
		t.Fatal("oversized replacement was accepted")
	}
}

func TestWorkspaceChangeFailsClosedWhenWorkspacePathIsReplaced(t *testing.T) {
	container := t.TempDir()
	workspace := filepath.Join(container, "workspace")
	if err := os.Mkdir(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, workspace, "chapter.txt", "original")
	service, err := NewService(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(workspace, filepath.Join(container, "moved")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, workspace, "chapter.txt", "replacement")
	if _, _, err := service.ReadFile("chapter.txt"); err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("replacement workspace was accepted: %v", err)
	}
}
