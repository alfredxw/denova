package projectfiles

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	projectdomain "denova/internal/project"
	workspacechange "denova/internal/workspace/change"
)

func TestServiceResolvesBatchedBranchesAndSingleChildChains(t *testing.T) {
	service, projectID, workspace := projectFilesTestService(t)
	mustWriteProjectFile(t, workspace, "src/components/editor/main.ts", "export const value = 1\n")
	mustWriteProjectFile(t, workspace, "node_modules/pkg/index.js", "module.exports = 1\n")
	mustWriteProjectFile(t, workspace, ".env.example", "TOKEN=\n")

	resolved, err := service.ResolveTree(context.Background(), projectID, TreeResolveRequest{
		Targets:                      []TreeResolveTarget{{ID: "root", Path: ""}, {ID: "source", Path: "src"}},
		FollowSingleChildDirectories: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Results) != 2 || !resolved.Results[0].OK || !resolved.Results[1].OK {
		t.Fatalf("unexpected resolve results: %#v", resolved.Results)
	}
	root := resolved.Results[0].Directories
	if len(root) != 4 || len(root[0].Entries) != 1 || root[0].Entries[0].Path != "src" || root[3].Entries[0].Path != "src/components/editor/main.ts" {
		t.Fatalf("unexpected filtered root: %#v", root)
	}
	source := resolved.Results[1].Directories
	if len(source) != 3 || source[0].Path != "src" || source[1].Path != "src/components" || source[2].Path != "src/components/editor" {
		t.Fatalf("single-child directories were not resolved together: %#v", source)
	}
	if source[2].Entries[0].Path != "src/components/editor/main.ts" {
		t.Fatalf("unexpected chain leaf: %#v", source[2].Entries)
	}

	complete, err := service.ResolveTree(context.Background(), projectID, TreeResolveRequest{
		Targets:        []TreeResolveTarget{{Path: ""}},
		IncludeIgnored: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	entries := complete.Results[0].Directories[0].Entries
	if len(entries) != 2 || entries[0].Path != "node_modules" || !entries[0].Ignored {
		t.Fatalf("ignored directory was not surfaced explicitly: %#v", entries)
	}
	if hidden, err := service.ResolveTree(context.Background(), projectID, TreeResolveRequest{
		Targets: []TreeResolveTarget{{Path: ".git"}},
	}); err != nil || hidden.Results[0].Code != "invalid_path" {
		t.Fatalf("hidden target should remain outside the file API: response=%#v err=%v", hidden, err)
	}
}

func TestServiceRecursivelyResolvesOrdinaryTreeWithinConfiguredLimit(t *testing.T) {
	service, projectID, workspace := projectFilesTestService(t)
	mustWriteProjectFile(t, workspace, "a/nested/one.md", "one")
	mustWriteProjectFile(t, workspace, "a/two.md", "two")
	mustWriteProjectFile(t, workspace, "b/three.md", "three")

	resolved, err := service.ResolveTree(context.Background(), projectID, TreeResolveRequest{
		Targets:   []TreeResolveTarget{{Path: ""}},
		Recursive: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Results) != 1 || !resolved.Results[0].OK {
		t.Fatalf("unexpected recursive resolve result: %#v", resolved.Results)
	}
	directories := resolved.Results[0].Directories
	paths := make([]string, 0, len(directories))
	for _, directory := range directories {
		paths = append(paths, directory.Path)
		if directory.ChildrenState != DirectoryChildrenComplete || directory.Continuation != "" {
			t.Fatalf("ordinary recursive tree should be complete: %#v", directory)
		}
	}
	if want := []string{"", "a", "b", "a/nested"}; !slices.Equal(paths, want) {
		t.Fatalf("recursive directory order = %v, want %v", paths, want)
	}
}

func TestServiceFallsBackToUnresolvedBranchesAfterConfiguredTreeLimit(t *testing.T) {
	service, projectID, workspace := projectFilesTestServiceWithOptions(t, WithTreeEntryLimit(3))
	mustWriteProjectFile(t, workspace, "a/one.md", "one")
	mustWriteProjectFile(t, workspace, "a/two.md", "two")
	mustWriteProjectFile(t, workspace, "b/three.md", "three")

	resolved, err := service.ResolveTree(context.Background(), projectID, TreeResolveRequest{
		Targets:   []TreeResolveTarget{{Path: ""}},
		Recursive: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	directories := resolved.Results[0].Directories
	if len(directories) != 2 || directories[0].Path != "" || directories[1].Path != "a" {
		t.Fatalf("tree limit should leave later branches unresolved: %#v", directories)
	}
	if directories[1].ChildrenState != DirectoryChildrenPartial || len(directories[1].Entries) != 1 || directories[1].Continuation == "" {
		t.Fatalf("tree limit should retain the existing paged fallback: %#v", directories[1])
	}

	branch, err := service.ResolveTree(context.Background(), projectID, TreeResolveRequest{
		Targets:   []TreeResolveTarget{{Path: "b"}},
		Recursive: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !branch.Results[0].OK || len(branch.Results[0].Directories) != 1 || branch.Results[0].Directories[0].Entries[0].Path != "b/three.md" {
		t.Fatalf("unresolved branch should remain loadable on demand: %#v", branch.Results)
	}
}

func TestServicePaginatesLargeDirectoriesAndRejectsStaleContinuations(t *testing.T) {
	service, projectID, workspace := projectFilesTestService(t)
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		mustWriteProjectFile(t, workspace, name, name)
	}

	first, err := service.ResolveTree(context.Background(), projectID, TreeResolveRequest{
		Targets:     []TreeResolveTarget{{Path: ""}},
		EntryBudget: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	page := first.Results[0].Directories[0]
	if page.ChildrenState != DirectoryChildrenPartial || len(page.Entries) != 2 || page.Continuation == "" {
		t.Fatalf("unexpected first page: %#v", page)
	}
	second, err := service.ResolveTree(context.Background(), projectID, TreeResolveRequest{
		Targets:     []TreeResolveTarget{{Path: "", Cursor: page.Continuation}},
		EntryBudget: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	lastPage := second.Results[0].Directories[0]
	if lastPage.ChildrenState != DirectoryChildrenComplete || len(lastPage.Entries) != 1 || lastPage.Entries[0].Path != "c.txt" {
		t.Fatalf("unexpected final page: %#v", lastPage)
	}

	mustWriteProjectFile(t, workspace, "d.txt", "d")
	stale, err := service.ResolveTree(context.Background(), projectID, TreeResolveRequest{
		Targets: []TreeResolveTarget{{Path: "", Cursor: page.Continuation}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if stale.Results[0].OK || stale.Results[0].Code != "cursor_stale" {
		t.Fatalf("expected a target-scoped stale cursor result, got %#v", stale.Results[0])
	}
}

func TestServicePaginatesWritingChaptersInNaturalOrder(t *testing.T) {
	service, projectID, workspace := projectFilesTestService(t)
	for _, name := range []string{
		"第十一章-潮声.md",
		"第一章-开局.md",
		"序章.md",
		"第十章-交锋.md",
	} {
		mustWriteProjectFile(t, workspace, name, name)
	}

	first, err := service.ResolveTree(context.Background(), projectID, TreeResolveRequest{
		Targets:     []TreeResolveTarget{{Path: ""}},
		EntryBudget: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	firstPage := first.Results[0].Directories[0]
	if got, want := []string{firstPage.Entries[0].Name, firstPage.Entries[1].Name}, []string{"序章.md", "第一章-开局.md"}; !slices.Equal(got, want) {
		t.Fatalf("first writing page order = %v, want %v", got, want)
	}

	second, err := service.ResolveTree(context.Background(), projectID, TreeResolveRequest{
		Targets:     []TreeResolveTarget{{Path: "", Cursor: firstPage.Continuation}},
		EntryBudget: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	secondPage := second.Results[0].Directories[0]
	if got, want := []string{secondPage.Entries[0].Name, secondPage.Entries[1].Name}, []string{"第十章-交锋.md", "第十一章-潮声.md"}; !slices.Equal(got, want) {
		t.Fatalf("second writing page order = %v, want %v", got, want)
	}
}

func TestServiceKeepsValidTreeTargetsWhenANeighbourFails(t *testing.T) {
	service, projectID, workspace := projectFilesTestService(t)
	mustWriteProjectFile(t, workspace, "main.ts", "export {}\n")
	resolved, err := service.ResolveTree(context.Background(), projectID, TreeResolveRequest{
		Targets: []TreeResolveTarget{{ID: "invalid", Path: "../outside"}, {ID: "root", Path: ""}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Results) != 2 || resolved.Results[0].OK || resolved.Results[0].Code != "invalid_path" || !resolved.Results[1].OK {
		t.Fatalf("unexpected partial resolve results: %#v", resolved.Results)
	}
}

func TestServiceReportsTargetsBeyondTheEntryBudget(t *testing.T) {
	service, projectID, workspace := projectFilesTestService(t)
	mustWriteProjectFile(t, workspace, "a/file.txt", "a")
	mustWriteProjectFile(t, workspace, "b/file.txt", "b")

	resolved, err := service.ResolveTree(context.Background(), projectID, TreeResolveRequest{
		Targets:     []TreeResolveTarget{{Path: "a"}, {Path: "b"}},
		EntryBudget: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Results) != 2 || !resolved.Results[0].OK || resolved.Results[1].OK || resolved.Results[1].Code != "budget_exhausted" {
		t.Fatalf("unexpected budgeted results: %#v", resolved.Results)
	}
}

func TestServiceBoundsAutomaticSingleDirectoryChains(t *testing.T) {
	service, projectID, workspace := projectFilesTestService(t)
	path := ""
	for index := 0; index < maximumResolvedDirChain+8; index++ {
		path = filepath.Join(path, fmt.Sprintf("dir-%03d", index))
	}
	mustWriteProjectFile(t, workspace, filepath.Join(path, "leaf.txt"), "leaf")

	resolved, err := service.ResolveTree(context.Background(), projectID, TreeResolveRequest{
		Targets:                      []TreeResolveTarget{{Path: ""}},
		FollowSingleChildDirectories: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	directories := resolved.Results[0].Directories
	if len(directories) != maximumResolvedDirChain {
		t.Fatalf("resolved directory chain length = %d, want %d", len(directories), maximumResolvedDirChain)
	}
	last := directories[len(directories)-1]
	if len(last.Entries) != 1 || last.Entries[0].Type != EntryDirectory {
		t.Fatalf("bounded chain should leave its next directory expandable: %#v", last)
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

func TestServiceCanManageASymlinkWithoutFollowingIt(t *testing.T) {
	service, projectID, workspace := projectFilesTestService(t)
	external := filepath.Join(t.TempDir(), "external")
	if err := os.MkdirAll(external, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(external, "keep.txt")
	if err := os.WriteFile(target, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(workspace, "linked")
	if err := os.Symlink(external, link); err != nil {
		t.Skipf("create symlink: %v", err)
	}

	results, err := service.ApplyOperations(context.Background(), projectID, []Operation{
		{Kind: OperationRename, Path: "linked", NewName: "renamed"},
		{Kind: OperationMove, Path: "renamed", To: "folder/moved"},
		{Kind: OperationDelete, Path: "folder/moved"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 || !results[0].OK || !results[1].OK || !results[2].OK {
		t.Fatalf("unexpected symlink operation results: %#v", results)
	}
	content, readErr := os.ReadFile(target)
	if readErr != nil || string(content) != "keep" {
		t.Fatalf("symlink target was changed: content=%q err=%v", content, readErr)
	}
	if _, statErr := os.Lstat(filepath.Join(workspace, "folder", "moved")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("symlink leaf was not deleted: %v", statErr)
	}
}

func projectFilesTestService(t *testing.T) (*Service, string, string) {
	return projectFilesTestServiceWithOptions(t)
}

func projectFilesTestServiceWithOptions(t *testing.T, options ...ServiceOption) (*Service, string, string) {
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
	return NewService(registry, options...), record.ID, workspace
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
