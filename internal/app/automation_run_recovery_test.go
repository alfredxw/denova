package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"denova/config"
	agents "denova/internal/agents"
	"denova/internal/automation"
	runstate "github.com/alfredxw/denova/agent/runtime"
)

func TestAutomationColdAcceptedRunStaysRecoveryRequiredUntilExplicitAbort(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	dataDir := filepath.Join(root, "nova")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	store := automation.NewStore(dataDir, workspace)
	taskDef, err := store.Create(automation.Task{
		Scope: automation.ScopeWorkspace, Enabled: true, Name: "cold recovery", Template: automation.TemplateReview,
		WriteMode: automation.WriteModeReadOnly, WriteScope: automation.WriteScopeNone,
		Triggers: []automation.TriggerDefinition{{
			ID: "schedule", Type: automation.TriggerTypeSchedule, Enabled: true,
			NotifyPolicy: automation.NotifyPolicySilent,
			Schedule:     automation.Schedule{Kind: automation.ScheduleDaily, Hour: 9},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	const runID = "cold-accepted-run"
	const operationID = runstate.OperationID("cold-accepted-operation")
	commandID := automationRunAgentCommandID(runID)
	evidence := []automation.TriggerEvidence{{Source: "schedule", Title: "daily", Snippet: "0 9 * * *"}}
	run := automation.RunRecord{
		ID: runID, TaskID: taskDef.ID, SessionID: automationRunSessionID(runID),
		Scope: taskDef.Scope, Workspace: workspace, Trigger: automation.TriggerSchedule,
		TriggerEvidence: evidence, Status: automation.RunStatusRunning, StartedAt: time.Now().UTC(),
		RootRuntimeCommandID: commandID, RootRuntimeOperationID: string(operationID), RootRuntimeReceiptCursor: 1,
		RuntimeCommandID: commandID, RuntimeOperationID: string(operationID), RuntimeReceiptCursor: 1,
	}
	if _, err := store.AppendRun(taskDef.ID, run); err != nil {
		t.Fatal(err)
	}
	match := automation.TriggerMatch{
		TaskID: taskDef.ID, TriggerID: "schedule", Title: "due", Summary: "due",
		Evidence: evidence, Fingerprint: "cold-schedule-fingerprint",
	}
	decision := automation.SemanticEvaluation{Matched: true, Confidence: 1, Reason: "due", Title: "due"}
	evaluation := automation.TriggerEvaluationRecord{
		ID: "cold-evaluation", IntentHash: "cold-intent", Status: automation.TriggerEvaluationStatusDecided,
		Scope: taskDef.Scope, Workspace: workspace, TaskID: taskDef.ID, TriggerID: "schedule",
		TriggerType: automation.TriggerTypeSchedule, ObservationFingerprint: match.Fingerprint,
		Decision: &decision, Match: &match,
		Action:    &automation.TriggerActionPlan{ID: "cold-action", ActionPolicy: automation.ActionPolicyAutoRun, NotifyPolicy: automation.NotifyPolicySilent, RunID: runID},
		ClaimedAt: time.Now().UTC(), DecidedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if _, err := store.UpdateTriggerState(taskDef.ID, "schedule", automation.TriggerState{Evaluation: &evaluation}); err != nil {
		t.Fatal(err)
	}
	seedAutomationRuntimeJournal(t, dataDir, taskDef, run, []runstate.EventPayload{
		runstate.CommandAcceptedEvent{CommandID: runstate.CommandID(commandID), CommandKind: "start_turn", OperationID: operationID, Fingerprint: "cold-start"},
		runstate.OperationStartedEvent{OperationID: operationID},
		runstate.UserMessageCommittedEvent{Message: runstate.Message{
			ID: "cold-accepted-user", Role: runstate.RoleUser, Content: "run automation",
			Input: runstate.UserInput{Text: "run automation"}, Operation: operationID,
		}},
		runstate.CycleStartedEvent{OperationID: operationID, Cycle: 1, SnapshotID: "cold-accepted-snapshot"},
		runstate.DomainCommitIntentAcceptedEvent{
			Identity: runstate.DomainCommitIdentity{
				CommandID: runstate.CommandID(commandID), OperationID: operationID,
				Cycle: 1, Stage: runstate.DomainCommitInput,
			},
			Hash: "cold-accepted-input",
		},
		runstate.DomainCommitReceiptEvent{
			Identity: runstate.DomainCommitIdentity{
				CommandID: runstate.CommandID(commandID), OperationID: operationID,
				Cycle: 1, Stage: runstate.DomainCommitInput,
			},
			Hash: "cold-accepted-input", Revision: "1",
		},
	})

	application, err := New(context.Background(), &config.Config{NovaDir: dataDir, Workspace: workspace, OpenAIModel: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	active := waitForAutomationActiveRun(t, application, runID)
	if !active.Run.RuntimeRecoveryRequired || active.Run.RuntimeOperationID != string(operationID) || active.Run.RuntimeCommandID != commandID {
		t.Fatalf("cold active projection = %#v", active)
	}
	if _, err := application.CheckAutomationTriggers(context.Background(), taskDef.ID); err != nil {
		t.Fatal(err)
	}
	stillDecided, err := store.Get(taskDef.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := stillDecided.TriggerState["schedule"].Evaluation; got == nil || got.Status != automation.TriggerEvaluationStatusDecided {
		t.Fatalf("uncertain runtime completed trigger evaluation: %#v", got)
	}

	if _, err := application.AbortAutomationRunCommand(context.Background(), runID, "abort-cold-accepted", agents.OperationID(operationID), "user_requested"); err != nil {
		t.Fatal(err)
	}
	settled := waitForAutomationRunStatus(t, store, runID, automation.RunStatusAborted)
	if settled.RuntimeRecoveryRequired {
		t.Fatalf("aborted run retained recovery obligation: %#v", settled)
	}
	if _, err := application.CheckAutomationTriggers(context.Background(), taskDef.ID); err != nil {
		t.Fatal(err)
	}
	completed, err := store.Get(taskDef.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := completed.TriggerState["schedule"].Evaluation; got == nil || got.Status != automation.TriggerEvaluationStatusCompleted {
		t.Fatalf("terminal reconciliation did not complete trigger action: %#v", got)
	}
}

func TestAutomationStartupScanFinalizesTerminalRuntimeEffectsExactlyOnce(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	dataDir := filepath.Join(root, "nova")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	store := automation.NewStore(dataDir, workspace)
	taskDef, err := store.Create(automation.Task{
		Scope: automation.ScopeWorkspace, Enabled: true, Name: "terminal recovery", Template: automation.TemplateReview,
		WriteMode: automation.WriteModeConfirmWrite, WriteScope: automation.WriteScopeFile,
	})
	if err != nil {
		t.Fatal(err)
	}
	const runID = "terminal-missing-append"
	const operationID = runstate.OperationID("terminal-operation")
	commandID := automationRunAgentCommandID(runID)
	run := automation.RunRecord{
		ID: runID, TaskID: taskDef.ID, SessionID: automationRunSessionID(runID),
		Scope: taskDef.Scope, Workspace: workspace, Trigger: automation.TriggerSchedule,
		Status: automation.RunStatusRunning, StartedAt: time.Now().UTC(),
		RootRuntimeCommandID: commandID, RootRuntimeOperationID: string(operationID), RootRuntimeReceiptCursor: 1,
		RuntimeCommandID: commandID, RuntimeOperationID: string(operationID), RuntimeReceiptCursor: 1,
	}
	if _, err := store.AppendRun(taskDef.ID, run); err != nil {
		t.Fatal(err)
	}
	seedAutomationRuntimeJournal(t, dataDir, taskDef, run, []runstate.EventPayload{
		runstate.CommandAcceptedEvent{CommandID: runstate.CommandID(commandID), CommandKind: "start_turn", OperationID: operationID, Fingerprint: "terminal-start"},
		runstate.OperationStartedEvent{OperationID: operationID},
		runstate.OperationSettledEvent{OperationID: operationID, Status: runstate.OperationSucceeded},
	})

	openAndWait := func() {
		application, openErr := New(context.Background(), &config.Config{NovaDir: dataDir, Workspace: workspace, OpenAIModel: "test-model"})
		if openErr != nil {
			t.Fatal(openErr)
		}
		waitForAutomationRunStatus(t, store, runID, automation.RunStatusSuccess)
		application.Close()
	}
	openAndWait()
	openAndWait()
	_, persisted, err := store.GetRunByID(runID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.CompletionEffectsPending || !persisted.CompletionEffectsCompleted {
		t.Fatalf("terminal effects receipt = %#v", persisted)
	}
	inbox, err := store.ListInbox()
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox) != 1 || inbox[0].SourceRunID != runID {
		t.Fatalf("terminal effects were not exactly-once: %#v", inbox)
	}
}

func TestAutomationColdFollowUpPublishesAndAbortsCurrentOperation(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	dataDir := filepath.Join(root, "nova")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	store := automation.NewStore(dataDir, workspace)
	taskDef, err := store.Create(automation.Task{
		Scope: automation.ScopeWorkspace, Enabled: true, Name: "recover follow-up", Template: automation.TemplateReview,
		WriteMode: automation.WriteModeReadOnly, WriteScope: automation.WriteScopeNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	const runID = "cold-follow-up-run"
	const rootOperationID = runstate.OperationID("cold-follow-up-root-operation")
	const currentOperationID = runstate.OperationID("cold-follow-up-current-operation")
	rootCommandID := automationRunAgentCommandID(runID)
	const currentCommandID = "cold-follow-up-command"
	run := automation.RunRecord{
		ID: runID, TaskID: taskDef.ID, SessionID: automationRunSessionID(runID),
		Scope: taskDef.Scope, Workspace: workspace, Trigger: automation.TriggerManual,
		Status: automation.RunStatusRunning, StartedAt: time.Now().UTC(),
		RootRuntimeCommandID: rootCommandID, RootRuntimeOperationID: string(rootOperationID), RootRuntimeReceiptCursor: 1,
		RuntimeCommandID: currentCommandID, RuntimeOperationID: string(currentOperationID), RuntimeReceiptCursor: 8,
	}
	if _, err := store.AppendRun(taskDef.ID, run); err != nil {
		t.Fatal(err)
	}
	seedAutomationRuntimeJournal(t, dataDir, taskDef, run, []runstate.EventPayload{
		runstate.CommandAcceptedEvent{CommandID: runstate.CommandID(rootCommandID), CommandKind: "start_turn", OperationID: rootOperationID, Fingerprint: "root-start"},
		runstate.OperationStartedEvent{OperationID: rootOperationID},
		runstate.UserMessageCommittedEvent{Message: runstate.Message{
			ID: "follow-up-root-user", Role: runstate.RoleUser, Content: "run automation",
			Input: runstate.UserInput{Text: "run automation"}, Operation: rootOperationID,
		}},
		runstate.CycleStartedEvent{OperationID: rootOperationID, Cycle: 1, SnapshotID: "follow-up-root-snapshot"},
		runstate.DomainCommitIntentAcceptedEvent{
			Identity: runstate.DomainCommitIdentity{CommandID: runstate.CommandID(rootCommandID), OperationID: rootOperationID, Cycle: 1, Stage: runstate.DomainCommitInput},
			Hash:     "follow-up-root-input",
		},
		runstate.DomainCommitReceiptEvent{
			Identity: runstate.DomainCommitIdentity{CommandID: runstate.CommandID(rootCommandID), OperationID: rootOperationID, Cycle: 1, Stage: runstate.DomainCommitInput},
			Hash:     "follow-up-root-input", Revision: "1",
		},
		runstate.OperationSettledEvent{OperationID: rootOperationID, Status: runstate.OperationSucceeded},
		runstate.CommandAcceptedEvent{CommandID: currentCommandID, CommandKind: "start_turn", OperationID: currentOperationID, Fingerprint: "follow-up-start"},
		runstate.OperationStartedEvent{OperationID: currentOperationID},
		runstate.UserMessageCommittedEvent{Message: runstate.Message{
			ID: "follow-up-current-user", Role: runstate.RoleUser, Content: "continue automation",
			Input: runstate.UserInput{Text: "continue automation"}, Operation: currentOperationID,
		}},
		runstate.CycleStartedEvent{OperationID: currentOperationID, Cycle: 1, SnapshotID: "follow-up-current-snapshot"},
		runstate.DomainCommitIntentAcceptedEvent{
			Identity: runstate.DomainCommitIdentity{CommandID: currentCommandID, OperationID: currentOperationID, Cycle: 1, Stage: runstate.DomainCommitInput},
			Hash:     "follow-up-current-input",
		},
		runstate.DomainCommitReceiptEvent{
			Identity: runstate.DomainCommitIdentity{CommandID: currentCommandID, OperationID: currentOperationID, Cycle: 1, Stage: runstate.DomainCommitInput},
			Hash:     "follow-up-current-input", Revision: "2",
		},
	})

	application, err := New(context.Background(), &config.Config{NovaDir: dataDir, Workspace: workspace, OpenAIModel: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	active := waitForAutomationActiveRun(t, application, runID)
	if !active.Run.RuntimeRecoveryRequired || active.Run.RuntimeCommandID != currentCommandID || active.Run.RuntimeOperationID != string(currentOperationID) {
		t.Fatalf("follow-up active projection = %#v", active)
	}
	if active.Run.RootRuntimeCommandID != rootCommandID || active.Run.RootRuntimeOperationID != string(rootOperationID) {
		t.Fatalf("follow-up changed immutable root receipt: %#v", active.Run)
	}
	if _, err := application.AbortAutomationRunCommand(context.Background(), runID, "abort-stale-root", agents.OperationID(rootOperationID), "stale"); !errors.Is(err, agents.ErrStaleOperation) {
		t.Fatalf("root-operation abort error = %v, want stale operation", err)
	}
	receipt, err := application.AbortAutomationRunCommand(context.Background(), runID, "abort-current-follow-up", agents.OperationID(currentOperationID), "user_requested")
	if err != nil {
		t.Fatal(err)
	}
	if receipt.CommandID != "abort-current-follow-up" || receipt.OperationID != agents.OperationID(currentOperationID) || receipt.Cursor == 0 {
		t.Fatalf("follow-up abort receipt = %#v", receipt)
	}
	settled := waitForAutomationRunStatus(t, store, runID, automation.RunStatusAborted)
	if settled.RootRuntimeCommandID != rootCommandID || settled.RootRuntimeOperationID != string(rootOperationID) ||
		settled.RuntimeCommandID != currentCommandID || settled.RuntimeOperationID != string(currentOperationID) || settled.RuntimeRecoveryRequired {
		t.Fatalf("follow-up terminal receipts = %#v", settled)
	}
}

func TestAutomationPendingFollowUpIntentRecoversActiveSuccessorAfterCrash(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	dataDir := filepath.Join(root, "nova")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	store := automation.NewStore(dataDir, workspace)
	taskDef, err := store.Create(automation.Task{
		Scope: automation.ScopeWorkspace, Enabled: true, Name: "pending successor", Template: automation.TemplateReview,
		WriteMode: automation.WriteModeReadOnly, WriteScope: automation.WriteScopeNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	const runID = "pending-follow-up-run"
	const rootOperationID = runstate.OperationID("pending-root-operation")
	const successorOperationID = runstate.OperationID("pending-successor-operation")
	rootCommandID := automationRunAgentCommandID(runID)
	const successorCommandID = "pending-successor-command"
	run := automation.RunRecord{
		ID: runID, TaskID: taskDef.ID, SessionID: automationRunSessionID(runID),
		Scope: taskDef.Scope, Workspace: workspace, Trigger: automation.TriggerManual,
		Status: automation.RunStatusSuccess, StartedAt: time.Now().UTC().Add(-time.Minute), FinishedAt: time.Now().UTC(),
		RootRuntimeCommandID: rootCommandID, RootRuntimeOperationID: string(rootOperationID), RootRuntimeReceiptCursor: 1,
		RuntimeCommandID: rootCommandID, RuntimeOperationID: string(rootOperationID), RuntimeReceiptCursor: 4,
		PendingRuntimeCommandID: successorCommandID, PendingRuntimeIntentHash: "pending-successor-intent",
		PendingRuntimeCommandFingerprint: "pending-successor",
		CompletionEffectsCompleted:       true,
	}
	if _, err := store.AppendRun(taskDef.CatalogID, run); err != nil {
		t.Fatal(err)
	}
	seedAutomationRuntimeJournal(t, dataDir, taskDef, run, []runstate.EventPayload{
		runstate.CommandAcceptedEvent{CommandID: runstate.CommandID(rootCommandID), CommandKind: "start_turn", OperationID: rootOperationID, Fingerprint: "pending-root"},
		runstate.OperationStartedEvent{OperationID: rootOperationID},
		runstate.OperationSettledEvent{OperationID: rootOperationID, Status: runstate.OperationSucceeded},
		runstate.CommandAcceptedEvent{CommandID: successorCommandID, CommandKind: "start_turn", OperationID: successorOperationID, Fingerprint: "pending-successor"},
		runstate.OperationStartedEvent{OperationID: successorOperationID},
		runstate.UserMessageCommittedEvent{Message: runstate.Message{
			ID: "pending-successor-user", Role: runstate.RoleUser, Content: "continue automation",
			Input: runstate.UserInput{Text: "continue automation"}, Operation: successorOperationID,
		}},
		runstate.CycleStartedEvent{OperationID: successorOperationID, Cycle: 1, SnapshotID: "pending-successor-snapshot"},
		runstate.DomainCommitIntentAcceptedEvent{
			Identity: runstate.DomainCommitIdentity{CommandID: successorCommandID, OperationID: successorOperationID, Cycle: 1, Stage: runstate.DomainCommitInput},
			Hash:     "pending-successor-input",
		},
		runstate.DomainCommitReceiptEvent{
			Identity: runstate.DomainCommitIdentity{CommandID: successorCommandID, OperationID: successorOperationID, Cycle: 1, Stage: runstate.DomainCommitInput},
			Hash:     "pending-successor-input", Revision: "2",
		},
	})

	application, err := New(context.Background(), &config.Config{NovaDir: dataDir, Workspace: workspace, OpenAIModel: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	active := waitForAutomationActiveRun(t, application, runID)
	if active.Run.PendingRuntimeCommandID != "" || active.Run.RuntimeCommandID != successorCommandID ||
		active.Run.RuntimeOperationID != string(successorOperationID) || !active.Run.RuntimeRecoveryRequired {
		t.Fatalf("pending successor was not promoted: %#v", active.Run)
	}
	if active.Run.RootRuntimeCommandID != rootCommandID || active.Run.RootRuntimeOperationID != string(rootOperationID) || active.Run.RootRuntimeReceiptCursor != 1 {
		t.Fatalf("successor recovery changed root receipt: %#v", active.Run)
	}
	if _, err := application.AbortAutomationRunCommand(context.Background(), runID, "abort-pending-successor", agents.OperationID(successorOperationID), "user_requested"); err != nil {
		t.Fatal(err)
	}
	settled := waitForAutomationRunStatus(t, store, runID, automation.RunStatusAborted)
	if settled.RuntimeOperationID != string(successorOperationID) || settled.RootRuntimeOperationID != string(rootOperationID) {
		t.Fatalf("pending successor abort targeted wrong operation: %#v", settled)
	}
}

func TestAutomationRecoveryFailureCannotFinalizeAnActiveProjection(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	novaDir := filepath.Join(root, "nova")
	store := automation.NewStore(novaDir, workspace)
	taskDef, err := store.Create(automation.Task{Scope: automation.ScopeWorkspace, Name: "active recovery", Template: automation.TemplateReview})
	if err != nil {
		t.Fatal(err)
	}
	run := automation.RunRecord{
		ID: "active-recovery-run", TaskID: taskDef.ID, SessionID: automationRunSessionID("active-recovery-run"),
		Scope: taskDef.Scope, Workspace: workspace, Trigger: automation.TriggerManual,
		RootRuntimeCommandID: automationRunAgentCommandID("active-recovery-run"), RootRuntimeOperationID: "active-recovery-operation", RootRuntimeReceiptCursor: 1,
		RuntimeCommandID: automationRunAgentCommandID("active-recovery-run"), RuntimeOperationID: "active-recovery-operation", RuntimeReceiptCursor: 1,
		RuntimeRecoveryRequired: true, Status: automation.RunStatusRunning,
	}
	if _, err := store.AppendRun(taskDef.CatalogID, run); err != nil {
		t.Fatal(err)
	}
	application := &App{}
	application.ensureServices()
	t.Cleanup(application.Close)
	service := application.automation()
	service.runtimeProjector = func(context.Context, *automationWorkspaceSnapshot, automation.Task, automation.RunRecord) (agents.RuntimeStatus, error) {
		return agents.RuntimeStatus{
			Cursor: 9, Phase: agents.RunPhaseRunning,
			ActiveCommandID: agents.CommandID(run.RuntimeCommandID), ActiveOperation: agents.OperationID(run.RuntimeOperationID),
		}, nil
	}
	snapshot := &automationWorkspaceSnapshot{workspace: workspace, novaDir: novaDir}
	finalized, err := service.finalizeRecoveredAutomationRun(
		context.Background(), snapshot, taskDef, run,
		agents.RunOutcome{Status: agents.RunOutcomeFailed, Error: errors.New("observer failed")},
	)
	if err == nil {
		t.Fatal("failed observer finalized a still-active runtime projection")
	}
	if finalized.Status != automation.RunStatusRunning || !finalized.RuntimeRecoveryRequired || finalized.RuntimeReceiptCursor != 9 {
		t.Fatalf("active recovery obligation was lost: %#v", finalized)
	}
	_, persisted, err := store.GetRunByID(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != automation.RunStatusRunning || !persisted.RuntimeRecoveryRequired || !persisted.FinishedAt.IsZero() {
		t.Fatalf("active projection was synthesized terminal: %#v", persisted)
	}
}

func seedAutomationRuntimeJournal(t *testing.T, dataDir string, taskDef automation.Task, run automation.RunRecord, events []runstate.EventPayload) {
	t.Helper()
	ref := automationRuntimeBindingForTest(run.Workspace, run.SessionID, taskDef.ID)
	key, err := json.Marshal(ref)
	if err != nil {
		t.Fatal(err)
	}
	journalStore, err := runstate.NewFileJournalStore(filepath.Join(dataDir, "agent-runtime"))
	if err != nil {
		t.Fatal(err)
	}
	journal, err := journalStore.OpenJournal(context.Background(), string(key))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Append(context.Background(), 0, events); err != nil {
		_ = journal.Close()
		t.Fatal(err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
}

func waitForAutomationActiveRun(t *testing.T, application *App, runID string) automation.ActiveRun {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		for _, active := range application.ActiveAutomationRuns() {
			if active.Run.ID == runID {
				return active
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("automation run %s did not become active", runID)
		}
		time.Sleep(time.Millisecond)
	}
}

func waitForAutomationRunStatus(t *testing.T, store *automation.Store, runID, status string) automation.RunRecord {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		_, run, err := store.GetRunByID(runID)
		if err == nil && run.Status == status {
			return run
		}
		if time.Now().After(deadline) {
			t.Fatalf("automation run %s did not settle as %s: run=%#v err=%v", runID, status, run, err)
		}
		time.Sleep(time.Millisecond)
	}
}
