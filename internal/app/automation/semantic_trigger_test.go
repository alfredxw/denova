package automationapp

import (
	"context"
	apptask "denova/internal/app/task"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"denova/config"
	"denova/internal/automation"
	"denova/internal/book"
)

func TestDurableSemanticTriggerClaimsObservationBeforeModelCall(t *testing.T) {
	app, service, snap, store, task, trigger, stateKey := newDurableSemanticTriggerTest(t, automation.NotifyPolicyInbox)
	_ = app

	previousEvaluator := service.semanticEvaluator
	defer func() { service.semanticEvaluator = previousEvaluator }()
	evaluatorCalls := 0
	service.semanticEvaluator = func(_ context.Context, _ *config.Config, _ string) (string, error) {
		evaluatorCalls++
		saved, err := store.Get(task.ID)
		if err != nil {
			t.Fatalf("load claimed task during evaluator: %v", err)
		}
		record := saved.TriggerState[stateKey].Evaluation
		if record == nil || record.Status != automation.TriggerEvaluationStatusClaimed || record.Instruction == "" {
			t.Fatalf("model called before durable observation claim: %#v", record)
		}
		return `{"matched":false,"confidence":0.2,"reason":"not yet","title":"","evidence_refs":[]}`, nil
	}

	item, run, processed, err := service.processDurableSemanticTriggerWithStarter(
		context.Background(), snap, store, time.Now().UTC(), task, trigger, stateKey, nil,
	)
	if err != nil {
		t.Fatalf("processDurableSemanticTriggerWithStarter failed: %v", err)
	}
	if !processed || item.ID != "" || run.Run.ID != "" || evaluatorCalls != 1 {
		t.Fatalf("unmatched result item=%#v run=%#v processed=%v calls=%d", item, run, processed, evaluatorCalls)
	}
	saved, err := store.Get(task.ID)
	if err != nil {
		t.Fatalf("load completed task: %v", err)
	}
	if got := saved.TriggerState[stateKey].Evaluation; got == nil || got.Status != automation.TriggerEvaluationStatusCompleted || got.Decision == nil {
		t.Fatalf("unmatched evaluation did not cross durable completion barrier: %#v", got)
	}
}

func TestDurableSemanticTriggerResumesDecisionAcrossRunCrashWithoutDuplicates(t *testing.T) {
	_, service, snap, store, task, trigger, stateKey := newDurableSemanticTriggerTest(t, automation.NotifyPolicyInbox)

	previousEvaluator := service.semanticEvaluator
	defer func() { service.semanticEvaluator = previousEvaluator }()
	evaluatorCalls := 0
	service.semanticEvaluator = func(_ context.Context, _ *config.Config, _ string) (string, error) {
		evaluatorCalls++
		return `{"matched":true,"confidence":0.93,"reason":"explicit bounded change","title":"Semantic hit","evidence_refs":["chapters/ch01.md"]}`, nil
	}

	crashErr := errors.New("simulated process exit after run admission")
	createdRuns := map[string]automation.RunRecord{}
	startIDs := []string{}
	firstStart := true
	starter := func(_ context.Context, taskID, triggerName, runID string, evidence []automation.TriggerEvidence) (*apptask.Task, automation.RunRecord, error) {
		startIDs = append(startIDs, runID)
		run, exists := createdRuns[runID]
		if !exists {
			run = automation.RunRecord{ID: runID, TaskID: taskID, Scope: task.Scope, Workspace: snap.workspace, Trigger: triggerName, TriggerEvidence: evidence, Status: automation.RunStatusRunning}
			createdRuns[runID] = run
		}
		if firstStart {
			firstStart = false
			return nil, automation.RunRecord{}, crashErr
		}
		run.RuntimeCommandID = automationRunAgentCommandID(runID)
		run.RuntimeOperationID = "operation-" + runID
		run.RuntimeReceiptCursor = 1
		createdRuns[runID] = run
		return nil, run, nil
	}

	firstItem, _, processed, err := service.processDurableSemanticTriggerWithStarter(
		context.Background(), snap, store, time.Now().UTC(), task, trigger, stateKey, starter,
	)
	if !errors.Is(err, crashErr) || processed || firstItem.ID == "" {
		t.Fatalf("first crash result item=%#v processed=%v err=%v", firstItem, processed, err)
	}
	afterCrash, err := store.Get(task.ID)
	if err != nil {
		t.Fatalf("load after crash: %v", err)
	}
	if got := afterCrash.TriggerState[stateKey].Evaluation; got == nil || got.Status != automation.TriggerEvaluationStatusDecided || got.Action == nil {
		t.Fatalf("decision/action was not durable before side effects: %#v", got)
	}

	// A fresh service and Store model restart. The model must not run again;
	// the same inbox and run IDs are reconciled to completion.
	restartedService := NewService(service.host)
	restartedStore := automation.NewStore(snap.novaDir, snap.workspace)
	secondItem, secondRun, processed, err := restartedService.processDurableSemanticTriggerWithStarter(
		context.Background(), snap, restartedStore, time.Now().UTC(), task, trigger, stateKey, starter,
	)
	if err != nil || !processed {
		t.Fatalf("restart processing item=%#v run=%#v processed=%v err=%v", secondItem, secondRun, processed, err)
	}
	if evaluatorCalls != 1 || len(createdRuns) != 1 || len(startIDs) != 2 || startIDs[0] != startIDs[1] {
		t.Fatalf("retry duplicated semantic work calls=%d unique_runs=%d start_ids=%#v", evaluatorCalls, len(createdRuns), startIDs)
	}
	items, err := restartedStore.ListInbox()
	if err != nil {
		t.Fatalf("ListInbox failed: %v", err)
	}
	if len(items) != 1 || items[0].ID != firstItem.ID || items[0].RunID != secondRun.Run.ID {
		t.Fatalf("durable inbox reconciliation = %#v", items)
	}
	completed, err := restartedStore.Get(task.ID)
	if err != nil {
		t.Fatalf("load completed task: %v", err)
	}
	if got := completed.TriggerState[stateKey].Evaluation; got == nil || got.Status != automation.TriggerEvaluationStatusCompleted {
		t.Fatalf("restart did not complete evaluation: %#v", got)
	}

	if _, _, processed, err := restartedService.processDurableSemanticTriggerWithStarter(
		context.Background(), snap, restartedStore, time.Now().UTC(), task, trigger, stateKey, starter,
	); err != nil || processed || evaluatorCalls != 1 || len(startIDs) != 2 {
		t.Fatalf("completed replay processed=%v evaluator_calls=%d starts=%d err=%v", processed, evaluatorCalls, len(startIDs), err)
	}
}

func TestDurableSemanticSilentAutoRunNeverCreatesInbox(t *testing.T) {
	_, service, snap, store, task, trigger, stateKey := newDurableSemanticTriggerTest(t, automation.NotifyPolicySilent)

	previousEvaluator := service.semanticEvaluator
	defer func() { service.semanticEvaluator = previousEvaluator }()
	service.semanticEvaluator = func(_ context.Context, _ *config.Config, _ string) (string, error) {
		return `{"matched":true,"confidence":0.99,"reason":"matched","title":"Silent hit","evidence_refs":[]}`, nil
	}
	starts := 0
	starter := func(_ context.Context, taskID, triggerName, runID string, evidence []automation.TriggerEvidence) (*apptask.Task, automation.RunRecord, error) {
		starts++
		return nil, automation.RunRecord{
			ID: runID, TaskID: taskID, Scope: task.Scope, Workspace: snap.workspace, Trigger: triggerName, TriggerEvidence: evidence, Status: automation.RunStatusRunning,
			RuntimeCommandID: automationRunAgentCommandID(runID), RuntimeOperationID: "operation-" + runID, RuntimeReceiptCursor: 1,
		}, nil
	}
	item, run, processed, err := service.processDurableSemanticTriggerWithStarter(
		context.Background(), snap, store, time.Now().UTC(), task, trigger, stateKey, starter,
	)
	if err != nil || !processed || starts != 1 || item.ID != "" || run.Run.ID == "" {
		t.Fatalf("silent auto-run item=%#v run=%#v processed=%v starts=%d err=%v", item, run, processed, starts, err)
	}
	items, err := store.ListInbox()
	if err != nil {
		t.Fatalf("ListInbox failed: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("silent auto-run created inbox: %#v", items)
	}
}

func TestDurableScheduleTriggerResumesEffectBeforeCompletion(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	novaDir := filepath.Join(root, "nova")
	store := automation.NewStore(novaDir, workspace)
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	task, err := store.Create(automation.Task{
		Scope: automation.ScopeWorkspace, Enabled: true, Name: "Durable schedule", Template: automation.TemplateReview,
		Triggers: []automation.TriggerDefinition{{
			ID: "schedule", Type: automation.TriggerTypeSchedule, Enabled: true,
			NotifyPolicy: automation.NotifyPolicySilent,
			Schedule:     automation.Schedule{Kind: automation.ScheduleDaily, Hour: now.Hour(), Minute: now.Minute()},
		}},
	})
	if err != nil {
		t.Fatalf("Create task failed: %v", err)
	}
	trigger := task.Triggers[0]
	stateKey := trigger.ID
	service := (&App{}).automation()
	snap := &automationWorkspaceSnapshot{workspace: workspace, novaDir: novaDir}
	crashErr := errors.New("simulated crash after schedule effect")
	createdRuns := map[string]automation.RunRecord{}
	first := true
	starter := func(_ context.Context, taskID, triggerName, runID string, evidence []automation.TriggerEvidence) (*apptask.Task, automation.RunRecord, error) {
		run, ok := createdRuns[runID]
		if !ok {
			run = automation.RunRecord{ID: runID, TaskID: taskID, Scope: automation.ScopeWorkspace, Workspace: workspace, Trigger: triggerName, TriggerEvidence: evidence, Status: automation.RunStatusRunning}
			createdRuns[runID] = run
		}
		if first {
			first = false
			return nil, run, crashErr
		}
		run.RuntimeCommandID = automationRunAgentCommandID(runID)
		run.RuntimeOperationID = "operation-" + runID
		run.RuntimeReceiptCursor = 1
		createdRuns[runID] = run
		return nil, run, nil
	}
	if _, _, _, err := service.processDurableBuiltInTriggerWithStarter(context.Background(), snap, store, now, task, trigger, stateKey, starter); !errors.Is(err, crashErr) {
		t.Fatalf("first schedule processing error=%v", err)
	}
	pending, err := store.Get(task.ID)
	if err != nil || pending.TriggerState[stateKey].Evaluation == nil || pending.TriggerState[stateKey].Evaluation.Status != automation.TriggerEvaluationStatusDecided {
		t.Fatalf("pending schedule state=%#v err=%v", pending.TriggerState[stateKey], err)
	}

	restarted := automation.NewStore(novaDir, workspace)
	_, replayedRun, processed, err := service.processDurableBuiltInTriggerWithStarter(context.Background(), snap, restarted, now, task, trigger, stateKey, starter)
	if err != nil || !processed || replayedRun.Run.ID == "" {
		t.Fatalf("replayed schedule run=%#v processed=%t err=%v", replayedRun, processed, err)
	}
	if len(createdRuns) != 1 {
		t.Fatalf("schedule replay allocated duplicate run identities: %#v", createdRuns)
	}
	completed, err := restarted.Get(task.ID)
	if err != nil || completed.TriggerState[stateKey].Evaluation.Status != automation.TriggerEvaluationStatusCompleted || completed.TriggerState[stateKey].LastMatchedAt.IsZero() {
		t.Fatalf("completed schedule state=%#v err=%v", completed.TriggerState[stateKey], err)
	}
}

func newDurableSemanticTriggerTest(t *testing.T, notifyPolicy string) (*App, *AutomationAppService, *automationWorkspaceSnapshot, *automation.Store, automation.Task, automation.TriggerDefinition, string) {
	t.Helper()
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(filepath.Join(workspace, "chapters"), 0o755); err != nil {
		t.Fatalf("create chapters: %v", err)
	}
	writeTestChapter(t, workspace, 1)
	novaDir := filepath.Join(root, "nova")
	application := &App{
		cfg:         &config.Config{NovaDir: novaDir, Workspace: workspace},
		workspace:   workspace,
		bookService: book.NewService(workspace),
	}
	application.ensureServices()
	t.Cleanup(application.Close)
	task, err := application.CreateAutomation(automation.Task{
		Scope:    automation.ScopeWorkspace,
		Enabled:  true,
		Name:     "Durable semantic",
		Template: automation.TemplateReview,
		Triggers: []automation.TriggerDefinition{{
			ID:                "semantic_1",
			Type:              automation.TriggerTypeSemantic,
			Enabled:           true,
			NotifyPolicy:      notifyPolicy,
			SemanticCondition: "a bounded character state changed",
			ChapterBatchSize:  1,
		}},
	})
	if err != nil {
		t.Fatalf("CreateAutomation failed: %v", err)
	}
	service := application.automation()
	snap := &automationWorkspaceSnapshot{
		workspace:   workspace,
		novaDir:     novaDir,
		cfg:         config.Config{NovaDir: novaDir, Workspace: workspace},
		bookService: book.NewService(workspace),
	}
	store := automation.NewStore(novaDir, workspace)
	trigger := task.Triggers[0]
	return application, service, snap, store, task, trigger, service.triggerStateKey(snap, task, trigger)
}
