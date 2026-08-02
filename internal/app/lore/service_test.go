package loreapp

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"denova/internal/app/task"
	booklore "denova/internal/book/lore"
)

var errTestWorkspaceChanged = errors.New("workspace changed")

type testHost struct {
	workspace    string
	unregistered *task.Task
}

func (host *testHost) WithLoreStore(expected string, action func(*booklore.Store) error) (string, error) {
	if _, err := host.ValidateLoreWorkspaceAllowEmpty(expected); err != nil {
		return "", err
	}
	return host.workspace, action(booklore.NewStore(host.workspace))
}

func (host *testHost) ValidateLoreWorkspace(expected string) (string, error) {
	if expected == "" || filepath.Clean(expected) != filepath.Clean(host.workspace) {
		return "", errTestWorkspaceChanged
	}
	return host.workspace, nil
}

func (host *testHost) ValidateLoreWorkspaceAllowEmpty(expected string) (string, error) {
	if expected != "" && filepath.Clean(expected) != filepath.Clean(host.workspace) {
		return "", errTestWorkspaceChanged
	}
	return host.workspace, nil
}

func (host *testHost) RegisterLoreTask(backgroundTask *task.Task, expected string) (string, error) {
	return host.ValidateLoreWorkspaceAllowEmpty(expected)
}

func (host *testHost) UnregisterLoreTask(backgroundTask *task.Task) {
	host.unregistered = backgroundTask
}

func (*testHost) ClassifyLoreItems(context.Context, []booklore.ClassificationInput) ([]booklore.ClassificationSuggestion, error) {
	return nil, nil
}

func TestImageBatchRejectsCanceledTaskUntilWorkerExits(t *testing.T) {
	active, err := task.NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer active.RejectStart(errors.New("test cleanup"))
	active.Abort()
	service := &Service{
		host:   &testHost{workspace: t.TempDir()},
		active: &activeImageTask{task: active},
	}

	_, err = service.StartImagesGenerateTask(context.Background(), "", ImagesGenerateRequest{ItemIDs: []string{"hero"}})
	if !errors.Is(err, ErrImageTaskRunning) {
		t.Fatalf("StartImagesGenerateTask error = %v, want %v", err, ErrImageTaskRunning)
	}
}

func TestAbortImagesGenerateTaskChecksWorkspaceIdentity(t *testing.T) {
	workspace := t.TempDir()
	staleWorkspace := t.TempDir()
	active, err := task.NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer active.RejectStart(errors.New("test cleanup"))
	service := &Service{
		host:   &testHost{workspace: workspace},
		active: &activeImageTask{task: active, workspace: workspace},
	}

	if err := service.AbortImagesGenerateTask(staleWorkspace); !errors.Is(err, errTestWorkspaceChanged) {
		t.Fatalf("stale abort error = %v, want %v", err, errTestWorkspaceChanged)
	}
	if active.Snapshot().CancelRequested {
		t.Fatal("stale workspace abort canceled the active task")
	}
	if err := service.AbortImagesGenerateTask(workspace); err != nil {
		t.Fatalf("matching workspace abort failed: %v", err)
	}
	if !active.Snapshot().CancelRequested {
		t.Fatal("matching workspace abort did not cancel the active task")
	}
}

func TestClearImageTaskReleasesHostRegistrationAndOwnership(t *testing.T) {
	active, err := task.NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer active.RejectStart(errors.New("test cleanup"))
	host := &testHost{workspace: t.TempDir()}
	service := &Service{host: host, active: &activeImageTask{task: active, workspace: host.workspace}}

	service.clearImageTask(active)

	if service.active != nil {
		t.Fatal("completed task remained active")
	}
	if host.unregistered != active {
		t.Fatal("completed task was not unregistered from its host")
	}
}
