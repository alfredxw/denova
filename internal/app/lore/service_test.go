package loreapp

import (
	"context"
	"errors"
	"testing"

	"denova/internal/app/task"
	booklore "denova/internal/book/lore"
)

var errTestProjectChanged = errors.New("Project changed")

type testHost struct {
	projectID    string
	workspace    string
	unregistered *task.Task
}

func (host *testHost) WithLoreStore(_ context.Context, projectID string, action func(*booklore.Store) error) (string, error) {
	if projectID != host.projectID {
		return "", errTestProjectChanged
	}
	if action == nil {
		return "", errors.New("lore action is nil")
	}
	if err := action(booklore.NewStore(host.workspace)); err != nil {
		return "", err
	}
	return host.workspace, nil
}

func (host *testHost) RegisterLoreTask(_ *task.Task, projectID string) (string, error) {
	if projectID != host.projectID {
		return "", errTestProjectChanged
	}
	return host.workspace, nil
}

func (host *testHost) UnregisterLoreTask(backgroundTask *task.Task) {
	host.unregistered = backgroundTask
}

func (*testHost) ClassifyLoreItems(context.Context, string, []booklore.ClassificationInput) ([]booklore.ClassificationSuggestion, error) {
	return nil, nil
}

func TestImageBatchRejectsCanceledTaskUntilWorkerExits(t *testing.T) {
	active, err := task.NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer active.RejectStart(errors.New("test cleanup"))
	active.Abort()
	const projectID = "project-a"
	service := &Service{
		host: &testHost{projectID: projectID, workspace: t.TempDir()},
		active: map[string]*activeImageTask{
			projectID: {task: active, projectID: projectID},
		},
	}

	_, err = service.StartImagesGenerateTask(context.Background(), projectID, ImagesGenerateRequest{ItemIDs: []string{"hero"}})
	if !errors.Is(err, ErrImageTaskRunning) {
		t.Fatalf("StartImagesGenerateTask error = %v, want %v", err, ErrImageTaskRunning)
	}
}

func TestAbortImagesGenerateTaskChecksProjectIdentity(t *testing.T) {
	const projectID = "project-a"
	workspace := t.TempDir()
	active, err := task.NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer active.RejectStart(errors.New("test cleanup"))
	service := &Service{
		host: &testHost{projectID: projectID, workspace: workspace},
		active: map[string]*activeImageTask{
			projectID: {task: active, projectID: projectID, workspace: workspace},
		},
	}

	if err := service.AbortImagesGenerateTask("project-b"); err != nil {
		t.Fatalf("another Project abort failed: %v", err)
	}
	if active.Snapshot().CancelRequested {
		t.Fatal("stale workspace abort canceled the active task")
	}
	if err := service.AbortImagesGenerateTask(projectID); err != nil {
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
	const projectID = "project-a"
	host := &testHost{projectID: projectID, workspace: t.TempDir()}
	service := &Service{host: host, active: map[string]*activeImageTask{
		projectID: {task: active, projectID: projectID, workspace: host.workspace},
	}}

	service.clearImageTask(active)

	if len(service.active) != 0 {
		t.Fatal("completed task remained active")
	}
	if host.unregistered != active {
		t.Fatal("completed task was not unregistered from its host")
	}
}
