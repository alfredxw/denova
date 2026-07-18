package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"denova/config"
	"denova/internal/automation"
)

func TestAutomationsListsTasksFromInactiveRegisteredWorkspaces(t *testing.T) {
	root := t.TempDir()
	novaDir := filepath.Join(root, "user")
	workspaceA := createAutomationTestWorkspace(t, filepath.Join(novaDir, "projects", "book-a"))
	workspaceB := createAutomationTestWorkspace(t, filepath.Join(novaDir, "projects", "book-b"))
	registry := NewBookRegistry(novaDir)
	if err := registry.Touch(workspaceA); err != nil {
		t.Fatalf("register workspace A: %v", err)
	}
	if err := registry.Touch(workspaceB); err != nil {
		t.Fatalf("register workspace B: %v", err)
	}

	taskA, err := automation.NewStore(novaDir, workspaceA).Create(automation.Task{
		Scope: automation.ScopeWorkspace, Name: "Task A", Template: automation.TemplateReview,
	})
	if err != nil {
		t.Fatalf("create task A: %v", err)
	}
	taskB, err := automation.NewStore(novaDir, workspaceB).Create(automation.Task{
		Scope: automation.ScopeWorkspace, Name: "Task B", Template: automation.TemplateReview,
	})
	if err != nil {
		t.Fatalf("create task B: %v", err)
	}

	application := &App{
		cfg:          &config.Config{NovaDir: novaDir, Workspace: workspaceA},
		workspace:    workspaceA,
		bookRegistry: registry,
	}
	application.ensureServices()
	t.Cleanup(application.Close)

	tasks, err := application.Automations()
	if err != nil {
		t.Fatalf("list automations: %v", err)
	}
	seen := map[string]bool{}
	for _, task := range tasks {
		seen[task.ID] = true
	}
	if !seen[taskA.ID] || !seen[taskB.ID] {
		t.Fatalf("global automation list omitted a registered workspace task: %#v", seen)
	}
}

func TestSchedulerEvaluatesDueTasksInInactiveWorkspace(t *testing.T) {
	root := t.TempDir()
	novaDir := filepath.Join(root, "user")
	workspaceA := createAutomationTestWorkspace(t, filepath.Join(novaDir, "projects", "book-a"))
	workspaceB := createAutomationTestWorkspace(t, filepath.Join(novaDir, "projects", "book-b"))
	registry := NewBookRegistry(novaDir)
	if err := registry.Touch(workspaceA); err != nil {
		t.Fatal(err)
	}
	if err := registry.Touch(workspaceB); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Minute)
	_, err := automation.NewStore(novaDir, workspaceB).Create(automation.Task{
		Scope:      automation.ScopeWorkspace,
		Enabled:    true,
		Name:       "Inactive workspace schedule",
		Template:   automation.TemplateReview,
		WriteMode:  automation.WriteModeReadOnly,
		WriteScope: automation.WriteScopeNone,
		Triggers: []automation.TriggerDefinition{{
			ID:           "schedule",
			Type:         automation.TriggerTypeSchedule,
			Enabled:      true,
			ActionPolicy: automation.ActionPolicyNotifyOnly,
			NotifyPolicy: automation.NotifyPolicyInbox,
			Schedule: automation.Schedule{
				Kind: automation.ScheduleDaily,
				Hour: now.Hour(), Minute: now.Minute(),
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	application := &App{
		cfg:          &config.Config{NovaDir: novaDir, Workspace: workspaceA},
		workspace:    workspaceA,
		bookRegistry: registry,
		chatService:  nil,
	}
	application.ensureServices()
	t.Cleanup(application.Close)

	application.RunDueAutomations(context.Background(), now)
	items, err := application.AutomationInbox()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	runID := ""
	for _, item := range items {
		if item.Title == "Inactive workspace schedule scheduled trigger" {
			found = canonicalAutomationWorkspace(item.Workspace) == canonicalAutomationWorkspace(workspaceB)
			runID = item.RunID
		}
	}
	if !found {
		t.Fatalf("inactive workspace schedule did not reach global inbox: %#v", items)
	}
	abortAutomationTestRun(application, runID)
}

func TestCheckAutomationTriggersUsesCatalogIDForInactiveWorkspace(t *testing.T) {
	root := t.TempDir()
	novaDir := filepath.Join(root, "user")
	workspaceA := createAutomationTestWorkspace(t, filepath.Join(novaDir, "projects", "book-a"))
	workspaceB := createAutomationTestWorkspace(t, filepath.Join(novaDir, "projects", "book-b"))
	registry := NewBookRegistry(novaDir)
	if err := registry.Touch(workspaceA); err != nil {
		t.Fatal(err)
	}
	if err := registry.Touch(workspaceB); err != nil {
		t.Fatal(err)
	}
	task, err := automation.NewStore(novaDir, workspaceB).Create(automation.Task{
		Scope:      automation.ScopeWorkspace,
		Enabled:    true,
		Name:       "Inactive workspace manual check",
		Template:   automation.TemplateReview,
		WriteMode:  automation.WriteModeReadOnly,
		WriteScope: automation.WriteScopeNone,
		Triggers: []automation.TriggerDefinition{{
			ID:           "schedule",
			Type:         automation.TriggerTypeSchedule,
			Enabled:      true,
			ActionPolicy: automation.ActionPolicyNotifyOnly,
			NotifyPolicy: automation.NotifyPolicyInbox,
			Schedule: automation.Schedule{
				Kind: automation.ScheduleEveryHours, EveryHours: 1,
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	application := &App{
		cfg:          &config.Config{NovaDir: novaDir, Workspace: workspaceA},
		workspace:    workspaceA,
		bookRegistry: registry,
	}
	application.ensureServices()
	t.Cleanup(application.Close)

	items, err := application.CheckAutomationTriggers(context.Background(), task.CatalogID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].TaskID != task.ID || canonicalAutomationWorkspace(items[0].Workspace) != canonicalAutomationWorkspace(workspaceB) {
		t.Fatalf("catalog trigger check returned %#v", items)
	}
	abortAutomationTestRun(application, items[0].RunID)
}

func abortAutomationTestRun(application *App, runID string) {
	if runID == "" {
		return
	}
	active, _, ok := application.ActiveAutomationTaskByRunID(runID)
	if !ok {
		return
	}
	_, finished := active.Subscribe()
	active.Abort()
	for range finished {
	}
}

func createAutomationTestWorkspace(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(path, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
