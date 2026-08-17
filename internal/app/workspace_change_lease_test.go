package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"denova/config"
	"denova/internal/book"
	workspacechange "denova/internal/workspace/change"
)

func TestWorkspaceFileMutationDefersAutomaticGitVersion(t *testing.T) {
	workspace := t.TempDir()
	path := "chapters/ch01.md"
	absolutePath := filepath.Join(workspace, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
		t.Fatalf("create chapter directory: %v", err)
	}
	if err := os.WriteFile(absolutePath, []byte("first"), 0o644); err != nil {
		t.Fatalf("write chapter: %v", err)
	}
	changeService, err := workspacechange.ForWorkspace(workspace)
	if err != nil {
		t.Fatalf("create change service: %v", err)
	}
	_, baseRevision, err := changeService.ReadFile(path)
	if err != nil {
		t.Fatalf("read base revision: %v", err)
	}
	application := &App{
		cfg: &config.Config{
			VersionTimedEnabled:         true,
			VersionTimedIntervalMinutes: 10,
		},
		workspace:      workspace,
		versionService: book.NewVersionService(workspace, filepath.Join(t.TempDir(), "repository")),
	}
	t.Cleanup(application.Close)

	_, err = application.WithWorkspaceChangeMutation(
		context.Background(),
		workspace,
		func(service *workspacechange.Service) (WorkspaceChangeMutationHooks, error) {
			result, saveErr := service.SaveFile(context.Background(), path, "second", baseRevision)
			if saveErr != nil {
				return WorkspaceChangeMutationHooks{}, saveErr
			}
			if !result.Changed {
				t.Fatal("save unexpectedly reported no change")
			}
			return WorkspaceChangeMutationHooks{ScheduleAutoVersion: true}, nil
		},
	)
	if err != nil {
		t.Fatalf("save workspace file: %v", err)
	}

	history, err := application.VersionHistory(context.Background(), 10)
	if err != nil {
		t.Fatalf("read version history: %v", err)
	}
	if len(history) != 0 {
		t.Fatalf("file save must not synchronously create a Git version: %#v", history)
	}
}

func TestWorkspaceFileSaveLeaseBlocksTransitionAndCancelsPostMutationHooks(t *testing.T) {
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
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseMutation) }) }
	defer release()
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
					ScheduleAutoVersion: true,
					AutomationSource:    "test_workspace_change",
					Paths:               []string{path},
				}, nil
			},
		)
		mutationDone <- struct {
			workspace string
			err       error
		}{workspace: canonicalWorkspace, err: err}
	}()
	<-mutationEntered

	transitionStarted := make(chan struct{})
	transitionDone := make(chan error, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				transitionDone <- fmt.Errorf("workspace transition goroutine panic: %v", recovered)
			}
		}()
		_, scopes, _, transitionErr := application.beginWorkspaceTransitionTo(nextWorkspace)
		close(transitionStarted)
		if transitionErr != nil {
			transitionDone <- transitionErr
			return
		}
		if transitionErr = waitLifecycleScopes(context.Background(), scopes); transitionErr != nil {
			transitionDone <- transitionErr
			return
		}
		application.mu.Lock()
		application.workspace = nextWorkspace
		transitionErr = application.replaceWorkspaceScopeLocked(nextWorkspace)
		application.mu.Unlock()
		application.endWorkspaceTransition()
		transitionDone <- transitionErr
	}()
	<-transitionStarted

	select {
	case err := <-transitionDone:
		t.Fatalf("workspace transition completed before the admitted mutation exited: %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	release()
	result := <-mutationDone
	if !errors.Is(result.err, context.Canceled) {
		t.Fatalf("fenced mutation error=%v, want context cancellation", result.err)
	}
	select {
	case err := <-transitionDone:
		if err != nil {
			t.Fatalf("workspace transition failed: %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("workspace transition did not resume after the mutation lease was released")
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
