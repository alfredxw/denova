package automationapp

import (
	"context"
	agentrun "denova/internal/agents/run"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"denova/internal/automation"
)

func TestAutomationRuntimeReceiptFindsOlderInitialOperationByStableCommand(t *testing.T) {
	run := automation.RunRecord{ID: "run-1"}
	status := agentrun.RuntimeStatus{
		LastOperation: &agentrun.OperationSummary{
			OperationID: "follow-up-operation", CommandID: "follow-up-command", Status: agentrun.OperationSucceeded,
		},
		RecentOperations: []agentrun.OperationSummary{
			{OperationID: "initial-operation", CommandID: agentrun.CommandID(automationRunAgentCommandID(run.ID)), Status: agentrun.OperationSucceeded},
			{OperationID: "follow-up-operation", CommandID: "follow-up-command", Status: agentrun.OperationSucceeded},
		},
	}
	match := automationRuntimeReceipt(status, run)
	if match.active || match.commandID != automationRunAgentCommandID(run.ID) || match.operationID != "initial-operation" || match.status != agentrun.OperationSucceeded {
		t.Fatalf("receipt command=%q operation=%q status=%q active=%v", match.commandID, match.operationID, match.status, match.active)
	}
}

func TestAutomationRootReceiptIsExactAndDoesNotOverwriteCurrent(t *testing.T) {
	run := automation.RunRecord{ID: "receipt-run"}
	root := agentrun.CommandReceipt{
		CommandID:   agentrun.CommandID(automationRunAgentCommandID(run.ID)),
		OperationID: "root-operation", Cursor: 3,
	}
	if err := applyAutomationRootReceipt(&run, root); err != nil {
		t.Fatal(err)
	}
	current := agentrun.CommandReceipt{CommandID: "follow-up", OperationID: "follow-up-operation", Cursor: 8}
	if err := applyAutomationCurrentReceipt(&run, current, "follow-up"); err != nil {
		t.Fatal(err)
	}
	if err := applyAutomationRootReceipt(&run, root); err != nil {
		t.Fatal(err)
	}
	if run.RuntimeCommandID != "follow-up" || run.RuntimeOperationID != "follow-up-operation" || run.RuntimeReceiptCursor != 8 {
		t.Fatalf("root replay overwrote current receipt: %#v", run)
	}
	changedRoot := root
	changedRoot.Cursor++
	if err := applyAutomationRootReceipt(&run, changedRoot); !errors.Is(err, automation.ErrRunIdentityConflict) {
		t.Fatalf("changed root error = %v, want identity conflict", err)
	}
}

func TestAutomationCurrentReceiptIsMonotonicAndSuccessorOnly(t *testing.T) {
	run := automation.RunRecord{
		ID: "receipt-run", RootRuntimeCommandID: automationRunAgentCommandID("receipt-run"),
		RootRuntimeOperationID: "root-operation", RootRuntimeReceiptCursor: 2,
		RuntimeCommandID: automationRunAgentCommandID("receipt-run"), RuntimeOperationID: "root-operation", RuntimeReceiptCursor: 2,
	}
	regressed := agentrun.CommandReceipt{
		CommandID: agentrun.CommandID(run.RuntimeCommandID), OperationID: agentrun.OperationID(run.RuntimeOperationID), Cursor: 1,
	}
	if err := applyAutomationCurrentReceipt(&run, regressed, run.RuntimeCommandID); !errors.Is(err, automation.ErrRunIdentityConflict) {
		t.Fatalf("regressed current error = %v, want identity conflict", err)
	}
	replaced := agentrun.CommandReceipt{CommandID: "follow-up", OperationID: "follow-up-operation", Cursor: 4}
	if err := applyAutomationCurrentReceipt(&run, replaced, "follow-up"); !errors.Is(err, automation.ErrRunIdentityConflict) {
		t.Fatalf("direct operation replacement error = %v, want identity conflict", err)
	}
	if err := advanceAutomationCurrentReceipt(&run, replaced, "follow-up"); err != nil {
		t.Fatal(err)
	}
	if run.RuntimeCommandID != "follow-up" || run.RuntimeOperationID != "follow-up-operation" || run.RuntimeReceiptCursor != 4 {
		t.Fatalf("successor receipt = %#v", run)
	}
	staleSuccessor := agentrun.CommandReceipt{CommandID: "another", OperationID: "another-operation", Cursor: 4}
	if err := advanceAutomationCurrentReceipt(&run, staleSuccessor, "another"); !errors.Is(err, automation.ErrRunIdentityConflict) {
		t.Fatalf("stale successor error = %v, want identity conflict", err)
	}
}

func TestAutomationFollowUpReceiptReplaysExactCurrentOperation(t *testing.T) {
	run := automation.RunRecord{
		ID:                     "follow-up-replay",
		RootRuntimeCommandID:   automationRunAgentCommandID("follow-up-replay"),
		RootRuntimeOperationID: "root-operation", RootRuntimeReceiptCursor: 1,
		RuntimeCommandID: "follow-up-command", RuntimeOperationID: "follow-up-operation",
		RuntimeReceiptCursor: 9, RuntimeCommandFingerprint: "runtime-fingerprint",
	}
	replayed := agentrun.CommandReceipt{
		CommandID: "follow-up-command", OperationID: "follow-up-operation", Cursor: 9, Replayed: true,
	}
	if err := applyAutomationFollowUpReceipt(&run, replayed, "follow-up-command", "runtime-fingerprint"); err != nil {
		t.Fatalf("exact current replay failed: %v", err)
	}
	if run.RuntimeOperationID != "follow-up-operation" || run.RuntimeReceiptCursor != 9 {
		t.Fatalf("exact replay changed current receipt: %#v", run)
	}
	if err := applyAutomationFollowUpReceipt(&run, replayed, "follow-up-command", "different-fingerprint"); !errors.Is(err, automation.ErrRunIdentityConflict) {
		t.Fatalf("different-payload replay error = %v, want ErrRunIdentityConflict", err)
	}
}

func TestAutomationPendingReconciliationRequiresExactRuntimeFingerprint(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	store := automation.NewStore(filepath.Join(root, "user"), workspace)
	task, err := store.Create(automation.TaskDefinition{Scope: automation.ScopeWorkspace, Name: "Fingerprint", Template: automation.TemplateReview})
	if err != nil {
		t.Fatal(err)
	}
	rootCommand := automationRunAgentCommandID("fingerprint-run")
	run := automation.RunRecord{
		ID: "fingerprint-run", TaskID: task.ID, SessionID: automationRunSessionID("fingerprint-run"),
		Scope: task.Scope, Workspace: workspace, Trigger: automation.TriggerManual,
		RootRuntimeCommandID: rootCommand, RootRuntimeOperationID: "root-operation", RootRuntimeReceiptCursor: 1,
		RuntimeCommandID: rootCommand, RuntimeOperationID: "root-operation", RuntimeReceiptCursor: 1,
		PendingRuntimeCommandID: "reused-command", PendingRuntimeIntentHash: "new-app-intent",
		PendingRuntimeCommandFingerprint: "new-runtime-fingerprint",
		Status:                           automation.RunStatusSuccess, CompletionEffectsCompleted: true, StartedAt: time.Now().UTC(),
	}
	if _, err := store.AppendRun(task.CatalogID, run); err != nil {
		t.Fatal(err)
	}
	service := (&App{}).automation()
	service.runtimeProjector = func(context.Context, *automationWorkspaceSnapshot, automation.Task, automation.RunRecord) (agentrun.RuntimeStatus, error) {
		return agentrun.RuntimeStatus{
			Cursor: 20,
			LastOperation: &agentrun.OperationSummary{
				CommandID: "reused-command", OperationID: "old-operation", Status: agentrun.OperationSucceeded,
				CommandFingerprint: "old-runtime-fingerprint", ReceiptCursor: 2,
			},
		}, nil
	}
	snap := &automationWorkspaceSnapshot{workspace: workspace, novaDir: filepath.Join(root, "user")}
	_, _, err = service.reconcileAutomationRunReceipt(context.Background(), snap, task, run)
	if !errors.Is(err, automation.ErrRunIdentityConflict) {
		t.Fatalf("mismatched pending fingerprint error = %v, want ErrRunIdentityConflict", err)
	}
	_, persisted, err := store.GetRunByID(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.PendingRuntimeCommandID != "" || persisted.RuntimeOperationID != "root-operation" || persisted.RuntimeSuccessorConflict == "" {
		t.Fatalf("mismatched runtime command was promoted or left pending: %#v", persisted)
	}
}

func TestAutomationPendingReconciliationClearsExactCurrentReplay(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	store := automation.NewStore(filepath.Join(root, "user"), workspace)
	task, err := store.Create(automation.TaskDefinition{Scope: automation.ScopeWorkspace, Name: "Replay", Template: automation.TemplateReview})
	if err != nil {
		t.Fatal(err)
	}
	rootCommand := automationRunAgentCommandID("current-replay-run")
	run := automation.RunRecord{
		ID: "current-replay-run", TaskID: task.ID, SessionID: automationRunSessionID("current-replay-run"),
		Scope: task.Scope, Workspace: workspace, Trigger: automation.TriggerManual,
		RootRuntimeCommandID: rootCommand, RootRuntimeOperationID: "root-operation", RootRuntimeReceiptCursor: 1,
		RuntimeCommandID: "follow-up-command", RuntimeOperationID: "follow-up-operation", RuntimeReceiptCursor: 9,
		RuntimeCommandFingerprint: "follow-up-runtime", RuntimeIntentHash: "follow-up-app-intent",
		PendingRuntimeCommandID: "follow-up-command", PendingRuntimeIntentHash: "follow-up-app-intent",
		PendingRuntimeCommandFingerprint: "follow-up-runtime",
		Status:                           automation.RunStatusSuccess, CompletionEffectsCompleted: true, StartedAt: time.Now().UTC(),
	}
	if _, err := store.AppendRun(task.CatalogID, run); err != nil {
		t.Fatal(err)
	}
	service := (&App{}).automation()
	service.runtimeProjector = func(context.Context, *automationWorkspaceSnapshot, automation.Task, automation.RunRecord) (agentrun.RuntimeStatus, error) {
		return agentrun.RuntimeStatus{
			Cursor: 30,
			LastOperation: &agentrun.OperationSummary{
				CommandID: "follow-up-command", OperationID: "follow-up-operation", Status: agentrun.OperationSucceeded,
				CommandFingerprint: "follow-up-runtime", ReceiptCursor: 9,
			},
		}, nil
	}
	snap := &automationWorkspaceSnapshot{workspace: workspace, novaDir: filepath.Join(root, "user")}
	reconciled, ok, err := service.reconcileAutomationRunReceipt(context.Background(), snap, task, run)
	if err != nil || !ok {
		t.Fatalf("exact current replay reconcile ok=%v err=%v", ok, err)
	}
	if reconciled.PendingRuntimeCommandID != "" || reconciled.RuntimeOperationID != "follow-up-operation" ||
		reconciled.RuntimeReceiptCursor != 9 || !reconciled.CompletionEffectsCompleted || reconciled.CompletionEffectsPending {
		t.Fatalf("exact current replay was not idempotent: %#v", reconciled)
	}
}
