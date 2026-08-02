package projectfiles

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	projectdomain "denova/internal/project"
	workspacechange "denova/internal/workspace/change"
)

func TestServiceListsDirectoriesLazilyAndKeepsIgnoredEntriesExplicit(t *testing.T) {
	service, projectID, workspace := projectFilesTestService(t)
	mustWriteProjectFile(t, workspace, "src/main.ts", "export const value = 1\n")
	mustWriteProjectFile(t, workspace, "node_modules/pkg/index.js", "module.exports = 1\n")
	mustWriteProjectFile(t, workspace, ".hidden", "secret")

	root, err := service.ListDirectory(context.Background(), projectID, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(root.Entries) != 1 || root.Entries[0].Path != "src" || root.Entries[0].Type != EntryDirectory {
		t.Fatalf("unexpected filtered root: %#v", root.Entries)
	}
	if root.Entries[0].Ignored {
		t.Fatal("ordinary source directory was marked ignored")
	}

	complete, err := service.ListDirectory(context.Background(), projectID, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(complete.Entries) != 2 || complete.Entries[0].Path != "node_modules" || !complete.Entries[0].Ignored {
		t.Fatalf("ignored directory was not surfaced explicitly: %#v", complete.Entries)
	}

	source, err := service.ListDirectory(context.Background(), projectID, "src", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(source.Entries) != 1 || source.Entries[0].Path != "src/main.ts" {
		t.Fatalf("unexpected lazy child listing: %#v", source.Entries)
	}
}

func TestServiceReadsAndSavesWithProjectScopedRevision(t *testing.T) {
	service, projectID, workspace := projectFilesTestService(t)
	mustWriteProjectFile(t, workspace, "src/main.ts", "before\n")

	document, err := service.ReadFile(context.Background(), projectID, "src/main.ts")
	if err != nil {
		t.Fatal(err)
	}
	if document.Content != "before\n" || document.Kind != DocumentText || !document.Editable {
		t.Fatalf("unexpected document: %#v", document)
	}
	saved, err := service.SaveFile(context.Background(), projectID, SaveRequest{
		Path: "src/main.ts", Content: "after\n", BaseRevision: document.Revision,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !saved.Changed || saved.Revision == document.Revision {
		t.Fatalf("unexpected save result: %#v", saved)
	}

	_, err = service.SaveFile(context.Background(), projectID, SaveRequest{
		Path: "src/main.ts", Content: "stale\n", BaseRevision: document.Revision,
	})
	var changeErr *workspacechange.Error
	if err == nil || !errors.As(err, &changeErr) || changeErr.Code != workspacechange.ErrorCodeRevisionConflict {
		t.Fatalf("expected revision conflict, got %v", err)
	}
}

func TestServicePreviewsOnlySafeRasterAssets(t *testing.T) {
	service, projectID, workspace := projectFilesTestService(t)
	mustWriteProjectFile(t, workspace, "cover.png", "\x89PNG\r\n\x1a\n")
	mustWriteProjectFile(t, workspace, "diagram.svg", `<svg xmlns="http://www.w3.org/2000/svg"></svg>`)

	image, err := service.ReadFile(context.Background(), projectID, "cover.png")
	if err != nil {
		t.Fatal(err)
	}
	if image.Kind != DocumentImage || image.Editable {
		t.Fatalf("unexpected raster document: %#v", image)
	}
	if _, contentType, err := service.ReadAsset(context.Background(), projectID, "cover.png"); err != nil || contentType != "image/png" {
		t.Fatalf("read raster asset: content_type=%q err=%v", contentType, err)
	}

	vector, err := service.ReadFile(context.Background(), projectID, "diagram.svg")
	if err != nil {
		t.Fatal(err)
	}
	if vector.Kind != DocumentText || !vector.Editable {
		t.Fatalf("SVG should open as source instead of an active same-origin asset: %#v", vector)
	}
	if _, _, err := service.ReadAsset(context.Background(), projectID, "diagram.svg"); err == nil {
		t.Fatal("expected SVG asset preview to be rejected")
	}
}

func TestServiceAppliesOperationsWithPartialSuccess(t *testing.T) {
	service, projectID, workspace := projectFilesTestService(t)
	mustWriteProjectFile(t, workspace, "existing.txt", "existing")

	results, err := service.ApplyOperations(context.Background(), projectID, []Operation{
		{ID: "create", Kind: OperationCreate, Path: "created.txt", Type: "file", Content: "created"},
		{ID: "duplicate", Kind: OperationCreate, Path: "existing.txt", Type: "file"},
		{ID: "rename", Kind: OperationRename, Path: "created.txt", NewName: "renamed.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 || !results[0].OK || results[1].OK || results[1].Code != "target_exists" || !results[2].OK {
		t.Fatalf("unexpected operation results: %#v", results)
	}
	content, readErr := os.ReadFile(filepath.Join(workspace, "renamed.txt"))
	if readErr != nil || string(content) != "created" {
		t.Fatalf("successful neighbours were not retained: content=%q err=%v", content, readErr)
	}
}

func TestServiceRejectsMutationsThroughSymlinks(t *testing.T) {
	service, projectID, workspace := projectFilesTestService(t)
	external := filepath.Join(t.TempDir(), "external")
	if err := os.MkdirAll(external, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(workspace, "linked")); err != nil {
		t.Skipf("create symlink: %v", err)
	}

	results, err := service.ApplyOperations(context.Background(), projectID, []Operation{
		{Kind: OperationCreate, Path: "linked/outside.txt", Type: "file", Content: "escaped"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].OK || results[0].Code != "symlink_path" {
		t.Fatalf("unexpected symlink result: %#v", results)
	}
	if _, err := os.Stat(filepath.Join(external, "outside.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("operation escaped the project root: %v", err)
	}
}

func projectFilesTestService(t *testing.T) (*Service, string, string) {
	t.Helper()
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	registry := projectdomain.NewRegistry(filepath.Join(root, "denova"))
	record, err := registry.Add(workspace, projectdomain.TypeGeneral, "Project")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.EnsureState(record); err != nil {
		t.Fatal(err)
	}
	return NewService(registry), record.ID, workspace
}

func mustWriteProjectFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
