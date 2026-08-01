package app

import (
	"context"
	agentrun "denova/internal/agents/run"
	apptask "denova/internal/app/task"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"denova/internal/automation"
)

func TestActiveAutomationRegistryScopesSameIDsByCanonicalWorkspace(t *testing.T) {
	root := t.TempDir()
	workspaceA := filepath.Join(root, "a")
	workspaceB := filepath.Join(root, "b")
	application := &App{workspace: workspaceA}
	application.ensureServices()
	serviceA := automationRegistryTestService(application)
	serviceB := automationRegistryTestService(application)
	snapA := automationRegistryTestSnapshot(workspaceA)
	snapB := automationRegistryTestSnapshot(workspaceB)

	runA := automation.RunRecord{ID: "same-run", TaskID: "same-task", Workspace: workspaceA, Status: automation.RunStatusRunning}
	runB := automation.RunRecord{ID: "same-run", TaskID: "same-task", Workspace: workspaceB, Status: automation.RunStatusRunning}
	claimA, owner, err := serviceA.reserveActiveAutomationRun(context.Background(), snapA, runA.TaskID, runA)
	if err != nil || !owner {
		t.Fatalf("reserve workspace A owner=%v err=%v", owner, err)
	}
	claimB, owner, err := serviceB.reserveActiveAutomationRun(context.Background(), snapB, runB.TaskID, runB)
	if err != nil || !owner {
		t.Fatalf("reserve workspace B owner=%v err=%v", owner, err)
	}
	release := make(chan struct{})
	taskA := blockingAutomationRegistryTask(release)
	taskB := blockingAutomationRegistryTask(release)
	if err := serviceA.activateAutomationClaim(claimA, taskA); err != nil {
		t.Fatalf("activate workspace A claim: %v", err)
	}
	if err := serviceB.activateAutomationClaim(claimB, taskB); err != nil {
		t.Fatalf("activate workspace B claim: %v", err)
	}

	if runs := serviceA.activeAutomationRuns(snapA); len(runs) != 1 || runs[0].Run.Workspace != workspaceA {
		t.Fatalf("workspace A active runs = %#v", runs)
	}
	if runs := serviceB.activeAutomationRuns(snapB); len(runs) != 1 || runs[0].Run.Workspace != workspaceB {
		t.Fatalf("workspace B active runs = %#v", runs)
	}
	if runs := serviceA.activeAutomationRuns(nil); len(runs) != 2 {
		t.Fatalf("user-level active runs = %#v, want both workspaces", runs)
	}
	if task, run, ok := serviceA.activeAutomationTaskByRunID(snapA, "same-run"); !ok || task != taskA || run.Workspace != workspaceA {
		t.Fatalf("workspace A lookup task=%p run=%#v ok=%v", task, run, ok)
	}
	if task, run, ok := serviceB.activeAutomationTaskByRunID(snapB, "same-run"); !ok || task != taskB || run.Workspace != workspaceB {
		t.Fatalf("workspace B lookup task=%p run=%#v ok=%v", task, run, ok)
	}
	close(release)
	<-taskA.Done()
	<-taskB.Done()
	serviceA.clearActiveAutomationTask(snapA, runA.TaskID, runA.ID)
	serviceB.clearActiveAutomationTask(snapB, runB.TaskID, runB.ID)
	application.unregisterWorkspaceTask(taskA)
	application.unregisterWorkspaceTask(taskB)
}

func TestActiveAutomationRunsIncludesGlobalAndSelectedWorkspaceOnly(t *testing.T) {
	root := t.TempDir()
	workspaceA := filepath.Join(root, "a")
	workspaceB := filepath.Join(root, "b")
	application := &App{workspace: workspaceA}
	application.ensureServices()
	service := automationRegistryTestService(application)

	fixtures := []struct {
		snap *automationWorkspaceSnapshot
		run  automation.RunRecord
	}{
		{snap: automationRegistryTestSnapshot(""), run: automation.RunRecord{ID: "global-run", TaskID: "global-task", Status: automation.RunStatusRunning}},
		{snap: automationRegistryTestSnapshot(workspaceA), run: automation.RunRecord{ID: "workspace-a-run", TaskID: "workspace-a-task", Workspace: workspaceA, Status: automation.RunStatusRunning}},
		{snap: automationRegistryTestSnapshot(workspaceB), run: automation.RunRecord{ID: "workspace-b-run", TaskID: "workspace-b-task", Workspace: workspaceB, Status: automation.RunStatusRunning}},
	}
	release := make(chan struct{})
	tasks := make([]*apptask.Task, 0, len(fixtures))
	for _, fixture := range fixtures {
		claim, owner, err := service.reserveActiveAutomationRun(context.Background(), fixture.snap, fixture.run.TaskID, fixture.run)
		if err != nil || !owner {
			t.Fatalf("reserve %s owner=%v err=%v", fixture.run.ID, owner, err)
		}
		task := blockingAutomationRegistryTask(release)
		if err := service.activateAutomationClaim(claim, task); err != nil {
			t.Fatalf("activate %s: %v", fixture.run.ID, err)
		}
		tasks = append(tasks, task)
	}

	runs := service.activeAutomationRuns(automationRegistryTestSnapshot(workspaceA))
	if len(runs) != 2 || !activeAutomationRunIDsEqual(runs, "global-run", "workspace-a-run") {
		t.Fatalf("selected workspace runs = %#v", runs)
	}
	application.mu.Lock()
	application.workspace = ""
	application.mu.Unlock()
	runs = service.ActiveAutomationRuns()
	if len(runs) != 1 || runs[0].Run.ID != "global-run" {
		t.Fatalf("workspace-less public runs = %#v", runs)
	}

	close(release)
	for index, task := range tasks {
		<-task.Done()
		service.clearActiveAutomationTask(fixtures[index].snap, fixtures[index].run.TaskID, fixtures[index].run.ID)
		application.unregisterWorkspaceTask(task)
	}
}

func activeAutomationRunIDsEqual(runs []automation.ActiveRun, expected ...string) bool {
	actual := make(map[string]bool, len(runs))
	for _, run := range runs {
		actual[run.Run.ID] = true
	}
	if len(actual) != len(expected) {
		return false
	}
	for _, id := range expected {
		if !actual[id] {
			return false
		}
	}
	return true
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
	service := automationRegistryTestService(application)
	aliasService := automationRegistryTestService(application)
	snap := automationRegistryTestSnapshot(workspace)
	aliasSnap := automationRegistryTestSnapshot(alias)
	firstRun := automation.RunRecord{ID: "first", TaskID: "shared", Workspace: workspace, Status: automation.RunStatusRunning}
	claim, owner, err := service.reserveActiveAutomationRun(context.Background(), snap, firstRun.TaskID, firstRun)
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
		defer func() {
			if recovered := recover(); recovered != nil {
				second <- result{err: fmt.Errorf("concurrent automation reservation panic: %v", recovered)}
			}
		}()
		candidate := automation.RunRecord{ID: "second", TaskID: "shared", Workspace: alias, Status: automation.RunStatusRunning}
		attached, owns, reserveErr := aliasService.reserveActiveAutomationRun(ctx, aliasSnap, candidate.TaskID, candidate)
		second <- result{claim: attached, owner: owns, err: reserveErr}
	}()

	release := make(chan struct{})
	task := blockingAutomationRegistryTask(release)
	if err := service.activateAutomationClaim(claim, task); err != nil {
		t.Fatalf("activate first claim: %v", err)
	}
	got := <-second
	if got.err != nil || got.owner || got.claim != claim || got.claim.task != task || got.claim.run.ID != "first" {
		t.Fatalf("second reservation = %#v owner=%v err=%v", got.claim, got.owner, got.err)
	}
	close(release)
	<-task.Done()
	service.clearActiveAutomationTask(snap, firstRun.TaskID, firstRun.ID)
	application.unregisterWorkspaceTask(task)
}

func TestAutomationClaimCannotActivateAfterAppClose(t *testing.T) {
	application := &App{}
	application.ensureServices()
	application.mu.Lock()
	if err := application.initializeLifecycleLocked(); err != nil {
		application.mu.Unlock()
		t.Fatal(err)
	}
	application.mu.Unlock()
	service := automationRegistryTestService(application)
	snap := automationRegistryTestSnapshot("")
	run := automation.RunRecord{ID: "global-run", TaskID: "global-task", Status: automation.RunStatusRunning}
	claim, owner, err := service.reserveActiveAutomationRun(context.Background(), snap, run.TaskID, run)
	if err != nil || !owner {
		t.Fatalf("reserve global claim owner=%v err=%v", owner, err)
	}

	application.Close()
	release := make(chan struct{})
	task := blockingAutomationRegistryTask(release)
	if err := service.activateAutomationClaim(claim, task); err == nil {
		t.Fatal("claim activated after App.Close fenced root admission")
	}
	application.mu.RLock()
	_, active := application.activeAutomationTasks[automationTaskRegistryKey("", run.TaskID)]
	_, registered := application.workspaceTasks[task]
	application.mu.RUnlock()
	if active || registered {
		t.Fatalf("closed App published ownerless task active=%t registered=%t", active, registered)
	}
	service.releaseAutomationClaim(claim)
	close(release)
	<-task.Done()
}

func TestGlobalAutomationClaimOwnsRootLeaseUntilTaskExit(t *testing.T) {
	application := &App{}
	application.ensureServices()
	service := automationRegistryTestService(application)
	snap := automationRegistryTestSnapshot("")
	run := automation.RunRecord{ID: "global-run", TaskID: "global-task", Status: automation.RunStatusRunning}
	claim, owner, err := service.reserveActiveAutomationRun(context.Background(), snap, run.TaskID, run)
	if err != nil || !owner {
		t.Fatalf("reserve global claim owner=%v err=%v", owner, err)
	}
	task := apptask.New(func(ctx context.Context, task *apptask.Task, _ func(agentrun.Event)) {
		defer application.unregisterWorkspaceTask(task)
		defer service.clearActiveAutomationTask(snap, run.TaskID, run.ID)
		<-ctx.Done()
	})
	if err := service.activateAutomationClaim(claim, task); err != nil {
		t.Fatalf("activate global claim: %v", err)
	}

	closed := make(chan error, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				closed <- fmt.Errorf("App.Close panic: %v", recovered)
			}
		}()
		application.Close()
		closed <- nil
	}()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("App.Close did not cancel and drain the root-owned automation task")
	}
	if snapshot := task.Snapshot(); !snapshot.Finished || snapshot.Status != apptask.Aborted {
		t.Fatalf("global task snapshot after Close = %#v", snapshot)
	}
}

func automationRegistryTestService(application *App) *AutomationAppService {
	return &AutomationAppService{app: application}
}

func automationRegistryTestSnapshot(workspace string) *automationWorkspaceSnapshot {
	return &automationWorkspaceSnapshot{workspace: workspace}
}

func blockingAutomationRegistryTask(release <-chan struct{}) *apptask.Task {
	return apptask.New(func(ctx context.Context, _ *apptask.Task, _ func(agentrun.Event)) {
		select {
		case <-release:
		case <-ctx.Done():
		}
	})
}
