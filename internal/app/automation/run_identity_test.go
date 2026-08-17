package automationapp

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentrun "denova/internal/agents/run"
	"denova/internal/automation"
	projectdomain "denova/internal/project"
)

func TestAutomationSessionStrategyOwnsStableConversationIdentity(t *testing.T) {
	task := automation.Task{ID: "task-local", CatalogID: "project-a:task-local"}
	perRunA := automationSessionID(task, "run-a")
	perRunB := automationSessionID(task, "run-b")
	if perRunA == perRunB || perRunA != automationRunSessionID("run-a") {
		t.Fatalf("per-run conversations A=%q B=%q", perRunA, perRunB)
	}

	task.SessionStrategy = automation.SessionStrategyPerTask
	perTaskA := automationSessionID(task, "run-a")
	perTaskB := automationSessionID(task, "run-b")
	if perTaskA != perTaskB || !strings.HasPrefix(perTaskA, "automation-task-") {
		t.Fatalf("per-task conversations A=%q B=%q", perTaskA, perTaskB)
	}

	run := (&Service{}).newRunRecord(&automationWorkspaceSnapshot{projectID: "project-a", workspace: "/books/a"}, automation.Task{
		ID: "task-local", Revision: "revision-7", SessionStrategy: automation.SessionStrategyPerTask,
	}, automation.TriggerSchedule)
	if run.ProjectID != "project-a" || run.TaskRevision != "revision-7" || run.SessionStrategy != automation.SessionStrategyPerTask || run.TurnID == "" {
		t.Fatalf("run admission snapshot = %#v", run)
	}
}

func TestAutomationDeterministicRunIDReplaysExactPersistedRun(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	novaDir := filepath.Join(root, "nova")
	application := &App{workspace: workspace}
	application.ensureServices()
	service := application.automation()
	store := automation.NewStore(novaDir, workspace)
	taskDef, err := store.Create(automation.TaskDefinition{Scope: automation.ScopeWorkspace, Name: "director action", Template: automation.TemplateReview})
	if err != nil {
		t.Fatal(err)
	}
	evidence := []automation.TriggerEvidence{{Source: " director ", Title: " Action ", Ref: " turn-1 ", Snippet: " inspect "}}
	existing := automation.RunRecord{
		ID: "director-action-1", TaskID: taskDef.ID, SessionID: automationRunSessionID("director-action-1"),
		Scope: taskDef.Scope, Workspace: workspace, Trigger: automation.TriggerManual, SourceRunID: "source-1",
		TriggerEvidence: boundedRunTriggerEvidence(evidence), Status: automation.RunStatusSuccess, Summary: "already committed",
	}
	if _, err := store.AppendRun(taskDef.ID, existing); err != nil {
		t.Fatal(err)
	}
	snap := &automationWorkspaceSnapshot{workspace: workspace, novaDir: novaDir}

	task, replay, err := service.startTaskWithSourceRunID(context.Background(), snap, taskDef.ID, automation.TriggerManual, "source-1", existing.ID, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if task != nil || replay.ID != existing.ID || replay.Summary != existing.Summary {
		t.Fatalf("exact replay = task=%v run=%+v, want persisted terminal run", task, replay)
	}
}

func TestAutomationDeterministicRunIDRejectsSemanticConflict(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	novaDir := filepath.Join(root, "nova")
	application := &App{workspace: workspace}
	application.ensureServices()
	service := application.automation()
	store := automation.NewStore(novaDir, workspace)
	taskDef, err := store.Create(automation.TaskDefinition{Scope: automation.ScopeWorkspace, Name: "director action", Template: automation.TemplateReview})
	if err != nil {
		t.Fatal(err)
	}
	existing := automation.RunRecord{
		ID: "director-action-conflict", TaskID: taskDef.ID, Scope: taskDef.Scope, Workspace: workspace,
		Trigger: automation.TriggerManual, SourceRunID: "source-1", Status: automation.RunStatusSuccess,
	}
	if _, err := store.AppendRun(taskDef.ID, existing); err != nil {
		t.Fatal(err)
	}
	snap := &automationWorkspaceSnapshot{workspace: workspace, novaDir: novaDir}

	task, run, err := service.startTaskWithSourceRunID(context.Background(), snap, taskDef.ID, automation.TriggerManual, "source-2", existing.ID, nil)
	if task != nil || run.ID != "" || !errors.Is(err, automation.ErrRunIdentityConflict) {
		t.Fatalf("conflicting replay = task=%v run=%+v err=%v", task, run, err)
	}
	_, persisted, lookupErr := store.GetRunByID(existing.ID)
	if lookupErr != nil {
		t.Fatal(lookupErr)
	}
	if persisted.SourceRunID != existing.SourceRunID || persisted.ID != existing.ID {
		t.Fatalf("conflicting replay mutated persisted run: %+v", persisted)
	}
}

func TestAutomationWriteConfirmationStartsFromCleanRunRecord(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	novaDir := filepath.Join(root, "nova")
	application := &App{workspace: workspace}
	application.ensureServices()
	service := application.automation()
	store := automation.NewStore(novaDir, workspace)
	taskDef, err := store.Create(automation.TaskDefinition{
		Scope: automation.ScopeWorkspace, Name: "confirm cleanly", Template: automation.TemplateReview,
	})
	if err != nil {
		t.Fatal(err)
	}
	finishedAt := time.Now().UTC().Add(-time.Minute)
	source := automation.RunRecord{
		ID: "source-terminal-run", TaskID: taskDef.ID, SessionID: automationRunSessionID("source-terminal-run"),
		Scope: taskDef.Scope, Workspace: workspace, Trigger: automation.TriggerManual,
		RootRuntimeCommandID: automationRunAgentCommandID("source-terminal-run"), RootRuntimeOperationID: "source-root", RootRuntimeReceiptCursor: 1,
		RuntimeCommandID: "source-follow-up", RuntimeOperationID: "source-current", RuntimeReceiptCursor: 9,
		RuntimeRecoveryRequired: true, CompletionEffectsPending: true,
		CompletionMutationPaths: []string{"chapters/source.md"}, Status: automation.RunStatusSuccess,
		StartedAt: finishedAt.Add(-time.Minute), FinishedAt: finishedAt, Summary: "source summary",
		Error: "source error", OutputPath: "reports/source.md",
		ToolManifest: []automation.ToolManifestItem{{Source: "source-tool", Allowed: true}},
	}
	if _, err := store.AppendRun(taskDef.ID, source); err != nil {
		t.Fatal(err)
	}

	evidence := []automation.TriggerEvidence{{Source: "confirmation", Title: "verify write", Snippet: "review source changes"}}
	_, confirmation, err := service.startTaskWithSourceRunID(
		context.Background(),
		&automationWorkspaceSnapshot{projectID: "project-test", projectType: projectdomain.TypeBook, workspace: workspace, novaDir: novaDir},
		taskDef.ID,
		automation.TriggerWriteConfirmation,
		source.ID,
		"clean-confirmation-run",
		evidence,
	)
	if !errors.Is(err, ErrNoWorkspace) {
		t.Fatalf("confirmation admission error = %v, want missing workspace runtime", err)
	}
	if confirmation.ID != "clean-confirmation-run" || confirmation.SourceRunID != source.ID || confirmation.Trigger != automation.TriggerWriteConfirmation {
		t.Fatalf("confirmation identity = %#v", confirmation)
	}
	if confirmation.RootRuntimeCommandID != "" || confirmation.RootRuntimeOperationID != "" || confirmation.RootRuntimeReceiptCursor != 0 ||
		confirmation.RuntimeCommandID != "" || confirmation.RuntimeOperationID != "" || confirmation.RuntimeReceiptCursor != 0 ||
		confirmation.RuntimeRecoveryRequired || confirmation.CompletionEffectsPending || !confirmation.CompletionEffectsCompleted ||
		len(confirmation.CompletionMutationPaths) != 0 || confirmation.Summary != "" || confirmation.OutputPath != "" || len(confirmation.ToolManifest) != 0 {
		t.Fatalf("confirmation inherited source execution state: %#v", confirmation)
	}
	if confirmation.Status != automation.RunStatusFailed || confirmation.FinishedAt.IsZero() || len(confirmation.TriggerEvidence) != 1 {
		t.Fatalf("confirmation local failure/evidence = %#v", confirmation)
	}
}

func TestAutomationPreAcceptanceRetryPreservesOriginalAdmissionTime(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	novaDir := filepath.Join(root, "nova")
	application := &App{workspace: workspace}
	application.ensureServices()
	t.Cleanup(application.Close)
	service := application.automation()
	store := automation.NewStore(novaDir, workspace)
	taskDef, err := store.Create(automation.TaskDefinition{Scope: automation.ScopeWorkspace, Name: "retry admission", Template: automation.TemplateReview})
	if err != nil {
		t.Fatal(err)
	}
	startedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	persisted := automation.RunRecord{
		ID: "preaccept-retry", TaskID: taskDef.ID, SessionID: "preaccept-session",
		Scope: taskDef.Scope, Workspace: workspace, Trigger: automation.TriggerManual,
		Status: automation.RunStatusFailed, StartedAt: startedAt, FinishedAt: startedAt.Add(time.Second),
		Error: "runner construction failed", CompletionEffectsCompleted: true,
	}
	if _, err := store.AppendRun(taskDef.CatalogID, persisted); err != nil {
		t.Fatal(err)
	}
	_, retried, err := service.startTaskWithSourceRunID(
		context.Background(),
		&automationWorkspaceSnapshot{projectID: "project-test", projectType: projectdomain.TypeBook, workspace: workspace, novaDir: novaDir},
		taskDef.CatalogID,
		automation.TriggerManual,
		"",
		persisted.ID,
		nil,
	)
	if !errors.Is(err, ErrNoWorkspace) {
		t.Fatalf("retry error = %v, want missing runtime snapshot", err)
	}
	if !retried.StartedAt.Equal(startedAt) || retried.SessionID != persisted.SessionID || retried.ID != persisted.ID {
		t.Fatalf("pre-acceptance retry changed durable admission identity: %#v", retried)
	}
}

func TestAutomationDeterministicRunIDNeverAttachesDifferentActiveIdentity(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	novaDir := filepath.Join(root, "nova")
	application := &App{workspace: workspace}
	application.ensureServices()
	t.Cleanup(application.Close)
	service := application.automation()
	store := automation.NewStore(novaDir, workspace)
	taskDef, err := store.Create(automation.TaskDefinition{Scope: automation.ScopeWorkspace, Name: "director action", Template: automation.TemplateReview})
	if err != nil {
		t.Fatal(err)
	}
	snap := &automationWorkspaceSnapshot{workspace: workspace, novaDir: novaDir}
	activeRun := automation.RunRecord{
		ID: "different-active-id", TaskID: taskDef.ID, Scope: taskDef.Scope, Workspace: workspace,
		Trigger: automation.TriggerManual, Status: automation.RunStatusRunning,
	}
	claim, owner, err := service.reserveActiveAutomationRun(context.Background(), snap, automationTaskStoreID(taskDef), activeRun)
	if err != nil || !owner {
		t.Fatalf("reserve active run owner=%t err=%v", owner, err)
	}
	release := make(chan struct{})
	activeTask := blockingAutomationRegistryTask(release)
	if err := service.activateAutomationClaim(claim, activeTask); err != nil {
		t.Fatalf("activate active run claim: %v", err)
	}

	task, run, err := service.startTaskWithSourceRunID(context.Background(), snap, taskDef.ID, automation.TriggerManual, "", "required-id", nil)
	if task != nil || run.ID != "" || !errors.Is(err, automation.ErrRunIdentityConflict) {
		t.Fatalf("deterministic start attached alternate identity task=%v run=%+v err=%v", task, run, err)
	}

	close(release)
	<-activeTask.Done()
	service.clearActiveAutomationTask(snap, automationTaskStoreID(taskDef), activeRun.ID)
	application.unregisterWorkspaceTask(activeTask)
}

func TestAutomationDeterministicRunRecoversAcceptedRuntimeBeforeRunRecord(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	novaDir := filepath.Join(root, "nova")
	application := &App{workspace: workspace}
	application.ensureServices()
	service := application.automation()
	store := automation.NewStore(novaDir, workspace)
	taskDef, err := store.Create(automation.TaskDefinition{Scope: automation.ScopeWorkspace, Name: "recover admission", Template: automation.TemplateReview})
	if err != nil {
		t.Fatal(err)
	}
	snap := &automationWorkspaceSnapshot{workspace: workspace, novaDir: novaDir}
	service.runtimeProjector = func(_ context.Context, _ *automationWorkspaceSnapshot, task automation.Task, run automation.RunRecord) (agentrun.RuntimeStatus, error) {
		return agentrun.RuntimeStatus{
			Binding: agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindAutomation, Workspace: workspace, SessionID: run.SessionID, TaskID: task.ID},
			Cursor:  7, Phase: agentrun.RunPhaseRunning,
			ActiveCommandID: agentrun.CommandID(automationRunAgentCommandID(run.ID)), ActiveOperation: "operation-accepted", ActiveCycle: 1,
			ActiveReceiptCursor: 7,
		}, nil
	}
	defer func() { service.runtimeProjector = nil }()

	candidate := automation.RunRecord{
		ID: "deterministic-admission", TaskID: taskDef.ID, SessionID: automationRunSessionID("deterministic-admission"),
		Scope: taskDef.Scope, Workspace: workspace, Trigger: automation.TriggerSchedule, Status: automation.RunStatusRunning,
	}
	recovered, ok, err := service.reconcileAutomationRunReceipt(context.Background(), snap, taskDef, candidate)
	if err != nil {
		t.Fatalf("reconcile accepted runtime failed: %v", err)
	}
	if !ok || recovered.ID != "deterministic-admission" || recovered.Status != automation.RunStatusRunning || !recovered.RuntimeRecoveryRequired {
		t.Fatalf("recovered ok=%v run=%#v", ok, recovered)
	}
	if recovered.RuntimeCommandID != automationRunAgentCommandID(recovered.ID) || recovered.RuntimeOperationID != "operation-accepted" || recovered.RuntimeReceiptCursor != 7 {
		t.Fatalf("recovered runtime receipt=%#v", recovered)
	}
	_, persisted, err := store.GetRunByID(recovered.ID)
	if err != nil {
		t.Fatalf("persisted recovered run lookup failed: %v", err)
	}
	if persisted.RuntimeOperationID != recovered.RuntimeOperationID || persisted.Status != automation.RunStatusRunning {
		t.Fatalf("persisted recovered run=%#v", persisted)
	}
}

func TestAutomationAdmissionIntentRecoversAcceptedRuntimeAfterReceiptWriteGap(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	novaDir := filepath.Join(root, "nova")
	store := automation.NewStore(novaDir, workspace)
	taskDef, err := store.Create(automation.TaskDefinition{Scope: automation.ScopeWorkspace, Name: "receipt gap", Template: automation.TemplateReview})
	if err != nil {
		t.Fatal(err)
	}
	run := automation.RunRecord{
		ID: "accepted-receipt-gap", TaskID: taskDef.ID, SessionID: automationRunSessionID("accepted-receipt-gap"),
		Scope: taskDef.Scope, Workspace: workspace, Trigger: automation.TriggerSchedule,
		Status: automation.RunStatusRunning, RuntimeAdmissionPending: true, StartedAt: time.Now().UTC(),
	}
	if _, err := store.AppendRun(taskDef.CatalogID, run); err != nil {
		t.Fatal(err)
	}
	service := (&App{}).automation()
	service.runtimeProjector = func(context.Context, *automationWorkspaceSnapshot, automation.Task, automation.RunRecord) (agentrun.RuntimeStatus, error) {
		return agentrun.RuntimeStatus{
			Phase: agentrun.RunPhaseRunning, Cursor: 7,
			ActiveCommandID:          agentrun.CommandID(automationRunAgentCommandID(run.ID)),
			ActiveCommandFingerprint: "accepted-fingerprint", ActiveReceiptCursor: 3,
			ActiveOperation: "accepted-operation", ActiveCycle: 1,
		}, nil
	}
	snap := &automationWorkspaceSnapshot{workspace: workspace, novaDir: novaDir}
	reconciled, ok, err := service.reconcileAutomationRunReceipt(context.Background(), snap, taskDef, run)
	if err != nil || !ok {
		t.Fatalf("receipt-gap reconciliation ok=%t err=%v", ok, err)
	}
	if reconciled.RuntimeAdmissionPending || !reconciled.RuntimeRecoveryRequired ||
		reconciled.RuntimeOperationID != "accepted-operation" || reconciled.RuntimeReceiptCursor != 3 {
		t.Fatalf("receipt-gap reconciliation = %#v", reconciled)
	}
	_, persisted, err := store.GetRunByID(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.RuntimeAdmissionPending || persisted.RuntimeOperationID != "accepted-operation" || !persisted.RuntimeRecoveryRequired {
		t.Fatalf("receipt-gap durable run = %#v", persisted)
	}
}

func TestAutomationAdmissionIntentSettlesWhenRuntimeProvesNoAcceptance(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	novaDir := filepath.Join(root, "nova")
	store := automation.NewStore(novaDir, workspace)
	taskDef, err := store.Create(automation.TaskDefinition{Scope: automation.ScopeWorkspace, Name: "unaccepted intent", Template: automation.TemplateReview})
	if err != nil {
		t.Fatal(err)
	}
	run := automation.RunRecord{
		ID: "unaccepted-intent", TaskID: taskDef.ID, SessionID: automationRunSessionID("unaccepted-intent"),
		Scope: taskDef.Scope, Workspace: workspace, Trigger: automation.TriggerSchedule,
		Status: automation.RunStatusRunning, RuntimeAdmissionPending: true, StartedAt: time.Now().UTC(),
	}
	if _, err := store.AppendRun(taskDef.CatalogID, run); err != nil {
		t.Fatal(err)
	}
	service := (&App{}).automation()
	service.runtimeProjector = func(context.Context, *automationWorkspaceSnapshot, automation.Task, automation.RunRecord) (agentrun.RuntimeStatus, error) {
		return agentrun.RuntimeStatus{Phase: agentrun.RunPhaseIdle}, nil
	}
	snap := &automationWorkspaceSnapshot{workspace: workspace, novaDir: novaDir}
	reconciled, ok, err := service.reconcileAutomationRunReceipt(context.Background(), snap, taskDef, run)
	if err != nil || ok {
		t.Fatalf("unaccepted reconciliation ok=%t err=%v", ok, err)
	}
	if reconciled.Status != automation.RunStatusFailed || reconciled.RuntimeAdmissionPending || !reconciled.CompletionEffectsCompleted {
		t.Fatalf("unaccepted intent did not settle safely: %#v", reconciled)
	}
	_, persisted, err := store.GetRunByID(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != automation.RunStatusFailed || persisted.RuntimeAdmissionPending {
		t.Fatalf("unaccepted durable run = %#v", persisted)
	}
	if obligations, err := store.ListDurableObligations(); err != nil || len(obligations) != 0 {
		t.Fatalf("settled unaccepted intent remained hot: %#v err=%v", obligations, err)
	}
}

func TestAutomationDeterministicRunReconcilesPersistedRunningFromJournal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	novaDir := filepath.Join(root, "nova")
	application := &App{workspace: workspace}
	application.ensureServices()
	service := application.automation()
	store := automation.NewStore(novaDir, workspace)
	taskDef, err := store.Create(automation.TaskDefinition{Scope: automation.ScopeWorkspace, Name: "reconcile terminal", Template: automation.TemplateReview})
	if err != nil {
		t.Fatal(err)
	}
	running := automation.RunRecord{
		ID: "deterministic-terminal", TaskID: taskDef.ID, SessionID: automationRunSessionID("deterministic-terminal"),
		Scope: taskDef.Scope, Workspace: workspace, Trigger: automation.TriggerSchedule, Status: automation.RunStatusRunning,
		RootRuntimeCommandID: automationRunAgentCommandID("deterministic-terminal"), RootRuntimeOperationID: "operation-terminal", RootRuntimeReceiptCursor: 3,
		RuntimeCommandID: automationRunAgentCommandID("deterministic-terminal"), RuntimeOperationID: "operation-terminal", RuntimeReceiptCursor: 3,
	}
	if _, err := store.AppendRun(taskDef.ID, running); err != nil {
		t.Fatal(err)
	}
	snap := &automationWorkspaceSnapshot{workspace: workspace, novaDir: novaDir}
	service.runtimeProjector = func(_ context.Context, _ *automationWorkspaceSnapshot, _ automation.Task, _ automation.RunRecord) (agentrun.RuntimeStatus, error) {
		return agentrun.RuntimeStatus{
			Cursor: 9, Phase: agentrun.RunPhaseIdle,
			LastOperation: &agentrun.OperationSummary{
				CommandID: agentrun.CommandID(automationRunAgentCommandID(running.ID)), OperationID: "operation-terminal",
				ReceiptCursor: 3, Status: agentrun.OperationSucceeded,
			},
		}, nil
	}
	defer func() { service.runtimeProjector = nil }()

	task, reconciled, err := service.startTaskWithSourceRunID(context.Background(), snap, taskDef.ID, automation.TriggerSchedule, "", running.ID, nil)
	if err != nil {
		t.Fatalf("terminal reconciliation failed: %v", err)
	}
	if task != nil || reconciled.Status != automation.RunStatusSuccess || reconciled.FinishedAt.IsZero() || reconciled.RuntimeReceiptCursor != 3 {
		t.Fatalf("reconciled task=%v run=%#v", task, reconciled)
	}
	_, persisted, err := store.GetRunByID(running.ID)
	if err != nil || persisted.Status != automation.RunStatusSuccess {
		t.Fatalf("persisted terminal run=%#v err=%v", persisted, err)
	}
}
