package projectbook

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"denova/internal/book"
	projectdomain "denova/internal/project"
)

func TestServiceFollowsBookRelinkByStableProjectID(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	for _, workspace := range []string{first, second} {
		if err := book.NewState(workspace).InitWorkspace(); err != nil {
			t.Fatal(err)
		}
	}
	if err := book.NewService(first).WriteFile("chapters/first.md", "# First\n"); err != nil {
		t.Fatal(err)
	}
	if err := book.NewService(second).WriteFile("chapters/second.md", "# Second\n"); err != nil {
		t.Fatal(err)
	}

	registry := projectdomain.NewRegistry(filepath.Join(root, "denova"))
	record, err := registry.Add(first, projectdomain.TypeBook, "Book")
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(registry)

	before, err := service.Snapshot(context.Background(), record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if before.Workspace != record.WorkspacePath || len(before.Summary.Chapters) != 1 || before.Summary.Chapters[0].Path != "chapters/first.md" {
		t.Fatalf("unexpected snapshot before relink: %#v", before)
	}

	relinked, err := registry.Relink(record.ID, second)
	if err != nil {
		t.Fatal(err)
	}
	after, err := service.Snapshot(context.Background(), record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.ProjectID != record.ID || after.Workspace != relinked.WorkspacePath || len(after.Summary.Chapters) != 1 || after.Summary.Chapters[0].Path != "chapters/second.md" {
		t.Fatalf("stable Project ID did not follow the relinked Book: %#v", after)
	}
}

func TestServiceRejectsGeneralProjectBookResources(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "general")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	registry := projectdomain.NewRegistry(filepath.Join(root, "denova"))
	record, err := registry.Add(workspace, projectdomain.TypeGeneral, "General")
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewService(registry).Snapshot(context.Background(), record.ID)
	if !errors.Is(err, ErrBookProjectRequired) {
		t.Fatalf("Snapshot error = %v, want ErrBookProjectRequired", err)
	}
}
