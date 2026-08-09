package automationapp

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"denova/config"
	"denova/internal/automation"
)

func TestAutomationTriggerCoordinatorDoesNotLoseEnqueueDuringIdleExit(t *testing.T) {
	workspace := t.TempDir()
	coordinator := newAutomationTriggerCoordinator()
	releaseIdle := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(releaseIdle) })
		coordinator.Close()
	})

	detached := make(chan struct{})
	secondRun := make(chan struct{})
	var idleCalls atomic.Int32
	var runCalls atomic.Int32
	coordinator.afterIdleDetach = func(string) {
		if idleCalls.Add(1) == 1 {
			close(detached)
			<-releaseIdle
		}
	}
	coordinator.afterRun = func(string) {
		if runCalls.Add(1) == 2 {
			close(secondRun)
		}
	}
	service := automationRegistryTestService(&App{})
	snapshot := &automationWorkspaceSnapshot{
		workspace: workspace,
		novaDir:   filepath.Join(workspace, "user"),
		cfg:       config.Config{Workspace: workspace, NovaDir: filepath.Join(workspace, "user")},
	}
	if !coordinator.Enqueue(service, snapshot, "first", []string{"chapters/one.md"}) {
		t.Fatal("first enqueue was rejected")
	}
	select {
	case <-detached:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("first worker did not reach the idle-detached barrier")
	}
	// The first worker has removed its entry but has not yet returned. The
	// enqueue must create a distinct worker that the first defer cannot erase.
	if !coordinator.Enqueue(service, snapshot, "second", []string{"chapters/two.md"}) {
		t.Fatal("second enqueue was rejected")
	}
	select {
	case <-secondRun:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("second enqueue was lost; process calls=%d", runCalls.Load())
	}
	releaseOnce.Do(func() { close(releaseIdle) })
	coordinator.Close()
	if got := runCalls.Load(); got != 2 {
		t.Fatalf("process calls = %d, want 2", got)
	}
}

func TestAutomationTriggerCoordinatorOwnsWorkspaceGenerationAndRejectsFencedEnqueue(t *testing.T) {
	workspace := t.TempDir()
	application := &App{workspace: workspace, cfg: &config.Config{Workspace: workspace, NovaDir: t.TempDir()}}
	application.ensureServices()
	coordinator := newAutomationTriggerCoordinator()
	releaseWorker := make(chan struct{})
	workerReachedBarrier := make(chan struct{})
	var barrierOnce sync.Once
	coordinator.afterRun = func(string) {
		barrierOnce.Do(func() { close(workerReachedBarrier) })
		<-releaseWorker
	}
	service := automationRegistryTestService(application)
	snapshot := &automationWorkspaceSnapshot{
		workspace: workspace,
		novaDir:   application.cfg.DataDir(),
		cfg:       *application.cfg,
	}
	if !coordinator.Enqueue(service, snapshot, "first", []string{"chapters/one.md"}) {
		t.Fatal("initial enqueue was rejected")
	}
	select {
	case <-workerReachedBarrier:
	case <-time.After(time.Second):
		t.Fatal("trigger worker did not reach lifecycle barrier")
	}

	_, scopes, _, err := application.beginWorkspaceTransitionTo(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if coordinator.Enqueue(service, snapshot, "after_fence", []string{"chapters/two.md"}) {
		t.Fatal("coordinator accepted work after workspace generation was fenced")
	}
	drained := make(chan error, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				drained <- fmt.Errorf("lifecycle drain panic: %v", recovered)
			}
		}()
		drained <- waitLifecycleScopes(context.Background(), scopes)
	}()
	select {
	case err := <-drained:
		t.Fatalf("workspace generation drained before worker exit: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(releaseWorker)
	select {
	case err := <-drained:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("workspace generation did not drain after trigger worker exit")
	}
	application.endWorkspaceTransition()
	coordinator.Close()
	application.Close()
}

func TestAutomationCompletionOutboxSurvivesCoordinatorFailureAndRetriesOnce(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	novaDir := filepath.Join(root, "nova")
	application := &App{workspace: workspace, cfg: &config.Config{Workspace: workspace, NovaDir: novaDir}}
	application.ensureServices()
	t.Cleanup(application.Close)
	service := application.automation()
	snapshot := &automationWorkspaceSnapshot{
		workspace: workspace,
		novaDir:   novaDir,
		cfg:       config.Config{Workspace: workspace, NovaDir: novaDir},
	}
	store := automation.NewStore(novaDir, workspace)
	task, err := store.Create(automation.Task{
		Scope: automation.ScopeWorkspace, Name: "durable effects", Template: automation.TemplateReview,
	})
	if err != nil {
		t.Fatal(err)
	}
	run := automation.RunRecord{
		ID: "durable-effects-run", TaskID: task.ID, SessionID: automationRunSessionID("durable-effects-run"),
		Scope: task.Scope, Workspace: workspace, Trigger: automation.TriggerManual,
		RootRuntimeCommandID: automationRunAgentCommandID("durable-effects-run"), RootRuntimeOperationID: "effects-operation", RootRuntimeReceiptCursor: 1,
		RuntimeCommandID: automationRunAgentCommandID("durable-effects-run"), RuntimeOperationID: "effects-operation", RuntimeReceiptCursor: 1,
		Status: automation.RunStatusSuccess, CompletionEffectsPending: true, CompletionEffectsOperationID: "effects-operation",
		CompletionMutationPaths: []string{"chapters/one.md"},
	}
	if _, err := store.AppendRun(task.CatalogID, run); err != nil {
		t.Fatal(err)
	}

	// A shutdown coordinator rejects admission and must leave the durable
	// outbox untouched.
	service.triggers.Close()
	if _, err := service.completeAutomationRunEffects(context.Background(), snapshot, task, run); err == nil {
		t.Fatal("closed coordinator admitted completion effects")
	}
	_, persisted, err := store.GetRunByID(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !persisted.CompletionEffectsPending || persisted.CompletionEffectsCompleted {
		t.Fatalf("shutdown cleared completion outbox: %#v", persisted)
	}

	coordinator := newAutomationTriggerCoordinator()
	service.triggers = coordinator
	passes := make(chan struct{}, 2)
	var attempts atomic.Int32
	coordinator.processOverride = func(context.Context, *AutomationAppService, *automationWorkspaceSnapshot, string) error {
		if attempts.Add(1) == 1 {
			return errors.New("injected trigger failure")
		}
		return nil
	}
	coordinator.afterRun = func(string) { passes <- struct{}{} }

	if _, err := service.completeAutomationRunEffects(context.Background(), snapshot, task, persisted); err != nil {
		t.Fatal(err)
	}
	select {
	case <-passes:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("failed completion-effects pass did not finish")
	}
	_, persisted, err = store.GetRunByID(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !persisted.CompletionEffectsPending || persisted.CompletionEffectsCompleted {
		t.Fatalf("failed trigger pass acknowledged completion outbox: %#v", persisted)
	}

	if _, err := service.completeAutomationRunEffects(context.Background(), snapshot, task, persisted); err != nil {
		t.Fatal(err)
	}
	select {
	case <-passes:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("retried completion-effects pass did not finish")
	}
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		_, persisted, err = store.GetRunByID(run.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !persisted.CompletionEffectsPending && persisted.CompletionEffectsCompleted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("successful retry did not acknowledge outbox: %#v", persisted)
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := service.completeAutomationRunEffects(context.Background(), snapshot, task, persisted); err != nil {
		t.Fatal(err)
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("completion effect process attempts = %d, want one failure and one retry", got)
	}
}

func TestAutomationCommittedMutationOutboxDrainsAfterFailedOrAbortedRestart(t *testing.T) {
	for _, status := range []string{automation.RunStatusFailed, automation.RunStatusAborted} {
		t.Run(status, func(t *testing.T) {
			root := t.TempDir()
			workspace := filepath.Join(root, "workspace")
			novaDir := filepath.Join(root, "nova")
			store := automation.NewStore(novaDir, workspace)
			task, err := store.Create(automation.Task{Scope: automation.ScopeWorkspace, Name: status, Template: automation.TemplateReview})
			if err != nil {
				t.Fatal(err)
			}
			run := automation.RunRecord{
				ID: "mutation-" + status, TaskID: task.ID, SessionID: automationRunSessionID("mutation-" + status),
				Scope: task.Scope, Workspace: workspace, Trigger: automation.TriggerManual,
				RootRuntimeCommandID: automationRunAgentCommandID("mutation-" + status), RootRuntimeOperationID: "operation-1", RootRuntimeReceiptCursor: 1,
				RuntimeCommandID: automationRunAgentCommandID("mutation-" + status), RuntimeOperationID: "operation-1", RuntimeReceiptCursor: 1,
				Status: status, CompletionEffectsPending: true, CompletionEffectsOperationID: "operation-1",
				CompletionMutationPaths: []string{"chapters/committed-before-terminal.md"},
			}
			if _, err := store.AppendRun(task.CatalogID, run); err != nil {
				t.Fatal(err)
			}

			// Recreate App/coordinator state to prove the mutation obligation is
			// independent of the process and of the model's terminal outcome.
			restarted := &App{workspace: workspace, cfg: &config.Config{Workspace: workspace, NovaDir: novaDir}}
			restarted.ensureServices()
			t.Cleanup(restarted.Close)
			finished := make(chan struct{}, 1)
			restarted.automationTriggers.processOverride = func(context.Context, *AutomationAppService, *automationWorkspaceSnapshot, string) error {
				return nil
			}
			restarted.automationTriggers.afterRun = func(string) { finished <- struct{}{} }
			snapshot := &automationWorkspaceSnapshot{
				workspace: workspace, novaDir: novaDir,
				cfg: config.Config{Workspace: workspace, NovaDir: novaDir},
			}
			_, persisted, err := store.GetRunByID(run.ID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := restarted.automation().completeAutomationRunEffects(context.Background(), snapshot, task, persisted); err != nil {
				t.Fatal(err)
			}
			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Fatal("committed mutation effect did not drain after restart")
			}
			deadline := time.Now().Add(time.Second)
			for {
				_, persisted, err = store.GetRunByID(run.ID)
				if err != nil {
					t.Fatal(err)
				}
				if persisted.CompletionEffectsCompleted && !persisted.CompletionEffectsPending {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("mutation obligation remained pending: %#v", persisted)
				}
				time.Sleep(time.Millisecond)
			}
			if len(persisted.CompletionMutationPaths) != 1 {
				t.Fatalf("acknowledgement erased committed mutation evidence: %#v", persisted)
			}
		})
	}
}
