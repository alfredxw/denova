package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"denova/config"
	"denova/internal/agentruntime"
	"denova/internal/automation"
)

func TestAutomationAbortReplaysPersistedReceiptAfterRestart(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{NovaDir: root, Workspace: workspace}
	application, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	taskDef, err := application.CreateAutomation(automation.Task{
		Scope: automation.ScopeWorkspace, Name: "abort replay", Template: automation.TemplateReview,
	})
	if err != nil {
		application.Close()
		t.Fatal(err)
	}
	const runID = "run-abort-replay"
	const operationID = agentruntime.OperationID("operation-abort-replay")
	const commandID = "abort-command-replay"
	run := automation.RunRecord{
		ID: runID, TaskID: taskDef.ID, SessionID: automationRunSessionID(runID),
		Scope: taskDef.Scope, Workspace: application.Workspace(), Trigger: automation.TriggerManual,
		RuntimeCommandID: automationRunAgentCommandID(runID), RuntimeOperationID: string(operationID),
		RuntimeReceiptCursor: 1, Status: automation.RunStatusAborted,
		StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC(),
	}
	if _, err := automation.NewStore(root, application.Workspace()).AppendRun(taskDef.ID, run); err != nil {
		application.Close()
		t.Fatal(err)
	}

	ref := agentruntime.BindingRef{
		Kind: agentruntime.BindingAutomation, Profile: agentruntime.ProfileAutomation,
		Workspace: run.Workspace, SessionID: run.SessionID, TaskID: taskDef.ID,
	}
	key, err := json.Marshal(ref)
	if err != nil {
		application.Close()
		t.Fatal(err)
	}
	journalStore, err := agentruntime.NewFileJournalStore(filepath.Join(root, "agent-runtime"))
	if err != nil {
		application.Close()
		t.Fatal(err)
	}
	journal, err := journalStore.OpenJournal(context.Background(), string(key))
	if err != nil {
		application.Close()
		t.Fatal(err)
	}
	abort := agentruntime.Abort{ID: commandID, OperationID: operationID, Reason: "user_requested"}
	abortFingerprint, err := agentruntime.CommandFingerprint(abort)
	if err != nil {
		application.Close()
		t.Fatal(err)
	}
	_, err = journal.Append(context.Background(), 0, []agentruntime.EventPayload{
		agentruntime.CommandAcceptedEvent{CommandID: agentruntime.CommandID(automationRunAgentCommandID(runID)), CommandKind: "start_turn", OperationID: operationID, Fingerprint: "seed-start"},
		agentruntime.OperationStartedEvent{OperationID: operationID},
		agentruntime.CommandAcceptedEvent{CommandID: commandID, CommandKind: "abort", OperationID: operationID, Fingerprint: abortFingerprint},
		agentruntime.AbortRequestedEvent{OperationID: operationID, Reason: abort.Reason},
		agentruntime.OperationSettledEvent{OperationID: operationID, Status: agentruntime.OperationAborted, Reason: abort.Reason},
	})
	if closeErr := journal.Close(); err != nil || closeErr != nil {
		application.Close()
		t.Fatalf("seed abort journal: append=%v close=%v", err, closeErr)
	}
	application.Close()

	reopened, err := New(context.Background(), &config.Config{NovaDir: root, Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Close)
	receipt, err := reopened.AbortAutomationRunCommand(context.Background(), runID, commandID, operationID, abort.Reason)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Replayed || receipt.CommandID != commandID || receipt.OperationID != operationID || receipt.Cursor != 3 {
		t.Fatalf("replayed abort receipt = %#v", receipt)
	}
}
