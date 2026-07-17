package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"denova/config"
	"denova/internal/book"
	"denova/internal/workspacechange"
)

func TestWorkspaceTreeMutationWaitsForSharedExclusiveLease(t *testing.T) {
	workspace := t.TempDir()
	application := newWorkspaceMutationTestApp(workspace)
	changeService, err := workspacechange.ForWorkspace(workspace)
	if err != nil {
		t.Fatalf("create workspace change service: %v", err)
	}

	releaseLease := holdWorkspaceMutationLease(t, changeService)
	attempted := make(chan struct{})
	mutationDone := make(chan error, 1)
	go func() {
		close(attempted)
		mutationDone <- application.CreateWorkspaceItem(context.Background(), "chapters/ch01.md", "file", "draft")
	}()
	<-attempted

	assertWorkspaceMutationBlocked(t, mutationDone)
	releaseLease()
	if err := waitForWorkspaceMutation(t, mutationDone); err != nil {
		t.Fatalf("create workspace item: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(workspace, "chapters", "ch01.md"))
	if err != nil || string(content) != "draft" {
		t.Fatalf("created content=%q err=%v", string(content), err)
	}
}

func TestVersionRestoreWaitsForSharedExclusiveLease(t *testing.T) {
	workspace := t.TempDir()
	application := newWorkspaceMutationTestApp(workspace)
	if err := application.BookService().Create("draft.md", "file", "first"); err != nil {
		t.Fatalf("create draft: %v", err)
	}
	initial, err := application.CreateVersion(context.Background(), "initial")
	if err != nil || initial.Version == nil {
		t.Fatalf("create initial version: result=%#v err=%v", initial, err)
	}
	if err := application.BookService().WriteFile("draft.md", "second"); err != nil {
		t.Fatalf("update draft: %v", err)
	}

	changeService, err := workspacechange.ForWorkspace(workspace)
	if err != nil {
		t.Fatalf("create workspace change service: %v", err)
	}
	releaseLease := holdWorkspaceMutationLease(t, changeService)
	attempted := make(chan struct{})
	restoreDone := make(chan error, 1)
	go func() {
		close(attempted)
		_, restoreErr := application.RestoreVersion(context.Background(), initial.Version.ID, []string{"draft.md"})
		restoreDone <- restoreErr
	}()
	<-attempted

	assertWorkspaceMutationBlocked(t, restoreDone)
	releaseLease()
	if err := waitForWorkspaceMutation(t, restoreDone); err != nil {
		t.Fatalf("restore version: %v", err)
	}
	content, err := application.BookService().ReadFile("draft.md")
	if err != nil || content != "first" {
		t.Fatalf("restored content=%q err=%v", content, err)
	}
}

func TestCreateVersionWaitsForSharedExclusiveLease(t *testing.T) {
	workspace := t.TempDir()
	application := newWorkspaceMutationTestApp(workspace)
	if err := application.BookService().Create("draft.md", "file", "first"); err != nil {
		t.Fatalf("create draft: %v", err)
	}
	changeService, err := workspacechange.ForWorkspace(workspace)
	if err != nil {
		t.Fatalf("create workspace change service: %v", err)
	}

	releaseLease := holdWorkspaceMutationLease(t, changeService)
	attempted := make(chan struct{})
	versionDone := make(chan error, 1)
	go func() {
		close(attempted)
		_, createErr := application.CreateVersion(context.Background(), "initial")
		versionDone <- createErr
	}()
	<-attempted

	assertWorkspaceMutationBlocked(t, versionDone)
	releaseLease()
	if err := waitForWorkspaceMutation(t, versionDone); err != nil {
		t.Fatalf("create version: %v", err)
	}
	history, err := application.VersionHistory(context.Background(), 1)
	if err != nil || len(history) != 1 || history[0].Message != "initial" {
		t.Fatalf("version history=%#v err=%v", history, err)
	}
}

func newWorkspaceMutationTestApp(workspace string) *App {
	return &App{
		cfg:            &config.Config{VersionTimedEnabled: false},
		workspace:      workspace,
		bookService:    book.NewService(workspace),
		versionService: book.NewVersionService(workspace),
	}
}

func holdWorkspaceMutationLease(t *testing.T, service *workspacechange.Service) func() {
	t.Helper()
	leaseHeld := make(chan struct{})
	release := make(chan struct{})
	leaseDone := make(chan error, 1)
	go func() {
		leaseDone <- service.WithExclusiveWorkspace(context.Background(), func() error {
			close(leaseHeld)
			<-release
			return nil
		})
	}()
	<-leaseHeld
	return func() {
		close(release)
		if err := waitForWorkspaceMutation(t, leaseDone); err != nil {
			t.Fatalf("held workspace lease: %v", err)
		}
	}
}

func assertWorkspaceMutationBlocked(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		t.Fatalf("workspace mutation completed before the exclusive lease was released: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
}

func waitForWorkspaceMutation(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(500 * time.Millisecond):
		t.Fatal("workspace mutation did not resume after the exclusive lease was released")
		return nil
	}
}
