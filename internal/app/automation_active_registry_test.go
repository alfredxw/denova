package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"denova/internal/agent"
	"denova/internal/automation"
)

func TestActiveAutomationRegistryScopesSameIDsByCanonicalWorkspace(t *testing.T) {
	root := t.TempDir()
	workspaceA := filepath.Join(root, "a")
	workspaceB := filepath.Join(root, "b")
	application := &App{workspace: workspaceA}
	application.ensureServices()
	serviceA := automationRegistryTestService(application, workspaceA)
	serviceB := automationRegistryTestService(application, workspaceB)

	runA := automation.RunRecord{ID: "same-run", TaskID: "same-task", Workspace: workspaceA, Status: automation.RunStatusRunning}
	runB := automation.RunRecord{ID: "same-run", TaskID: "same-task", Workspace: workspaceB, Status: automation.RunStatusRunning}
	claimA, owner, err := serviceA.reserveActiveAutomationRun(context.Background(), runA.TaskID, runA)
	if err != nil || !owner {
		t.Fatalf("reserve workspace A owner=%v err=%v", owner, err)
	}
	claimB, owner, err := serviceB.reserveActiveAutomationRun(context.Background(), runB.TaskID, runB)
	if err != nil || !owner {
		t.Fatalf("reserve workspace B owner=%v err=%v", owner, err)
	}
	release := make(chan struct{})
	taskA := blockingAutomationRegistryTask(release)
	taskB := blockingAutomationRegistryTask(release)
	if !serviceA.activateAutomationClaim(claimA, taskA) || !serviceB.activateAutomationClaim(claimB, taskB) {
		t.Fatal("activate claims failed")
	}

	if runs := serviceA.ActiveAutomationRuns(); len(runs) != 1 || runs[0].Run.Workspace != workspaceA {
		t.Fatalf("workspace A active runs = %#v", runs)
	}
	if runs := serviceB.ActiveAutomationRuns(); len(runs) != 1 || runs[0].Run.Workspace != workspaceB {
		t.Fatalf("workspace B active runs = %#v", runs)
	}
	if task, run, ok := serviceA.ActiveAutomationTaskByRunID("same-run"); !ok || task != taskA || run.Workspace != workspaceA {
		t.Fatalf("workspace A lookup task=%p run=%#v ok=%v", task, run, ok)
	}
	if task, run, ok := serviceB.ActiveAutomationTaskByRunID("same-run"); !ok || task != taskB || run.Workspace != workspaceB {
		t.Fatalf("workspace B lookup task=%p run=%#v ok=%v", task, run, ok)
	}
	close(release)
	serviceA.clearActiveAutomationTask(runA.TaskID, runA.ID)
	serviceB.clearActiveAutomationTask(runB.TaskID, runB.ID)
}

func TestActiveAutomationReservationAtomicallyAttachesConcurrentCaller(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "real")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(workspace, alias); err != nil {
		t.Fatal(err)
	}
	application := &App{workspace: workspace}
	application.ensureServices()
	service := automationRegistryTestService(application, workspace)
	aliasService := automationRegistryTestService(application, alias)
	firstRun := automation.RunRecord{ID: "first", TaskID: "shared", Workspace: workspace, Status: automation.RunStatusRunning}
	claim, owner, err := service.reserveActiveAutomationRun(context.Background(), firstRun.TaskID, firstRun)
	if err != nil || !owner {
		t.Fatalf("first reservation owner=%v err=%v", owner, err)
	}

	type result struct {
		claim *automationRunClaim
		owner bool
		err   error
	}
	second := make(chan result, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go func() {
		candidate := automation.RunRecord{ID: "second", TaskID: "shared", Workspace: alias, Status: automation.RunStatusRunning}
		attached, owns, reserveErr := aliasService.reserveActiveAutomationRun(ctx, candidate.TaskID, candidate)
		second <- result{claim: attached, owner: owns, err: reserveErr}
	}()

	release := make(chan struct{})
	task := blockingAutomationRegistryTask(release)
	if !service.activateAutomationClaim(claim, task) {
		t.Fatal("activate first claim failed")
	}
	got := <-second
	if got.err != nil || got.owner || got.claim != claim || got.claim.task != task || got.claim.run.ID != "first" {
		t.Fatalf("second reservation = %#v owner=%v err=%v", got.claim, got.owner, got.err)
	}
	close(release)
	service.clearActiveAutomationTask(firstRun.TaskID, firstRun.ID)
}

func automationRegistryTestService(application *App, workspace string) *AutomationAppService {
	return &AutomationAppService{
		app: application,
		snapshot: &automationWorkspaceSnapshot{
			workspace: workspace,
		},
	}
}

func blockingAutomationRegistryTask(release <-chan struct{}) *Task {
	return NewTask(func(ctx context.Context, _ *Task, _ func(agent.Event)) {
		select {
		case <-release:
		case <-ctx.Done():
		}
	})
}
