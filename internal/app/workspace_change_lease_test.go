package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"denova/internal/workspacechange"
)

func TestWorkspaceFileSaveLeaseBlocksWorkspaceSwitchThroughHooks(t *testing.T) {
	workspace := t.TempDir()
	nextWorkspace := t.TempDir()
	path := "chapters/ch01.md"
	for root, content := range map[string]string{workspace: "old workspace", nextWorkspace: "next workspace"} {
		absolutePath := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
			t.Fatalf("create chapter directory: %v", err)
		}
		if err := os.WriteFile(absolutePath, []byte(content), 0o644); err != nil {
			t.Fatalf("write chapter: %v", err)
		}
	}
	service, err := workspacechange.ForWorkspace(workspace)
	if err != nil {
		t.Fatalf("create change service: %v", err)
	}
	_, baseRevision, err := service.ReadFile(path)
	if err != nil {
		t.Fatalf("read base revision: %v", err)
	}
	application := &App{workspace: workspace}
	t.Cleanup(application.Close)
	mutationEntered := make(chan struct{})
	releaseMutation := make(chan struct{})
	mutationDone := make(chan struct {
		workspace string
		err       error
	}, 1)

	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				mutationDone <- struct {
					workspace string
					err       error
				}{err: fmt.Errorf("mutation goroutine panic: %v", recovered)}
			}
		}()
		canonicalWorkspace, err := application.WithWorkspaceChangeMutation(
			context.Background(),
			workspace,
			func(changeService *workspacechange.Service) (WorkspaceChangeMutationHooks, error) {
				close(mutationEntered)
				<-releaseMutation
				saveResult, saveErr := changeService.SaveFile(context.Background(), path, "saved in old workspace", baseRevision)
				if saveErr != nil {
					return WorkspaceChangeMutationHooks{}, saveErr
				}
				if !saveResult.Changed {
					return WorkspaceChangeMutationHooks{}, fmt.Errorf("save unexpectedly reported no change")
				}
				return WorkspaceChangeMutationHooks{
					CreateTimedVersion: true,
					AutomationSource:   "test_workspace_change",
					Paths:              []string{path},
				}, nil
			},
		)
		mutationDone <- struct {
			workspace string
			err       error
		}{workspace: canonicalWorkspace, err: err}
	}()
	<-mutationEntered

	switchAttempted := make(chan struct{})
	switchDone := make(chan struct{})
	switchPanic := make(chan any, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				switchPanic <- recovered
			}
			close(switchDone)
		}()
		close(switchAttempted)
		application.mu.Lock()
		application.workspace = nextWorkspace
		application.mu.Unlock()
	}()
	<-switchAttempted

	select {
	case <-switchDone:
		t.Fatal("workspace switch completed before the mutation and its hooks released the read lease")
	case <-time.After(25 * time.Millisecond):
	}

	close(releaseMutation)
	result := <-mutationDone
	if result.err != nil {
		t.Fatalf("mutation failed: %v", result.err)
	}
	if result.workspace != workspace {
		t.Fatalf("canonical workspace=%q want=%q", result.workspace, workspace)
	}
	select {
	case <-switchDone:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("workspace switch did not resume after the mutation lease was released")
	}
	select {
	case recovered := <-switchPanic:
		t.Fatalf("workspace switch goroutine panic: %v", recovered)
	default:
	}
	if application.Workspace() != nextWorkspace {
		t.Fatalf("current workspace=%q want=%q", application.Workspace(), nextWorkspace)
	}
	oldContent, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(path)))
	if err != nil || string(oldContent) != "saved in old workspace" {
		t.Fatalf("captured workspace save content=%q err=%v", string(oldContent), err)
	}
	nextContent, err := os.ReadFile(filepath.Join(nextWorkspace, filepath.FromSlash(path)))
	if err != nil || string(nextContent) != "next workspace" {
		t.Fatalf("new workspace must not receive stale save content=%q err=%v", string(nextContent), err)
	}
}
