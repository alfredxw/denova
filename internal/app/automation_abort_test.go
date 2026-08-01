package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	filejournal "github.com/alfredxw/denova/agent/runtime/filejournal"

	"denova/config"
	agentrun "denova/internal/agents/run"
	"denova/internal/automation"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func TestAutomationAbortReplaysPersistedReceiptAfterRestart(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{NovaDir: root, Workspace: workspace, OpenAIModel: "test-model"}
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
	const operationID = runstate.OperationID("operation-abort-replay")
	const commandID = "abort-command-replay"
	run := automation.RunRecord{
		ID: runID, TaskID: taskDef.ID, SessionID: automationRunSessionID(runID),
		ProjectID: taskDef.Target.WorkspaceID,
		Scope:     taskDef.Scope, Workspace: application.Workspace(), Trigger: automation.TriggerManual,
		RuntimeCommandID: automationRunAgentCommandID(runID), RuntimeOperationID: string(operationID),
		RuntimeReceiptCursor: 1, Status: automation.RunStatusAborted,
		StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC(),
	}
	if application.cfg.ProjectID != run.ProjectID {
		projects, _ := application.Projects(false)
		t.Fatalf("active Project identity = %q, automation target = %q, projects=%#v", application.cfg.ProjectID, run.ProjectID, projects)
	}
	if _, err := application.automation().storeAllWorkspaces().AppendRun(automationTaskStoreID(taskDef), run); err != nil {
		application.Close()
		t.Fatal(err)
	}

	ref := automationRuntimeBindingForTest(run.Workspace, run.SessionID, taskDef.ID, run.ProjectID)
	key, err := json.Marshal(ref)
	if err != nil {
		application.Close()
		t.Fatal(err)
	}
	journalStore, err := filejournal.NewStore(filepath.Join(root, "agent-runtime"))
	if err != nil {
		application.Close()
		t.Fatal(err)
	}
	journal, err := journalStore.OpenJournal(context.Background(), string(key))
	if err != nil {
		application.Close()
		t.Fatal(err)
	}
	abort := runstate.Abort{ID: commandID, OperationID: operationID, Reason: "user_requested"}
	abortFingerprint, err := runstate.CommandFingerprint(abort)
	if err != nil {
		application.Close()
		t.Fatal(err)
	}
	_, err = journal.Append(context.Background(), 0, []runstate.EventPayload{
		runstate.CommandAcceptedEvent{CommandID: runstate.CommandID(automationRunAgentCommandID(runID)), CommandKind: "start_turn", OperationID: operationID, Fingerprint: "seed-start"},
		runstate.OperationStartedEvent{OperationID: operationID},
		runstate.CommandAcceptedEvent{CommandID: commandID, CommandKind: "abort", OperationID: operationID, Fingerprint: abortFingerprint},
		runstate.AbortRequestedEvent{OperationID: operationID, Reason: abort.Reason},
		runstate.OperationSettledEvent{OperationID: operationID, Status: runstate.OperationAborted, Reason: abort.Reason},
	})
	if closeErr := journal.Close(); err != nil || closeErr != nil {
		application.Close()
		t.Fatalf("seed abort journal: append=%v close=%v", err, closeErr)
	}
	application.Close()

	reopened, err := New(context.Background(), &config.Config{NovaDir: root, Workspace: workspace, OpenAIModel: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Close)
	if reopened.cfg.ProjectID != run.ProjectID {
		t.Fatalf("reopened Project identity = %q, want %q", reopened.cfg.ProjectID, run.ProjectID)
	}
	reopenedRef := automationRuntimeBindingForTest(run.Workspace, run.SessionID, taskDef.ID, reopened.cfg.ProjectID)
	if !reopenedRef.Equal(ref) {
		t.Fatalf("reopened runtime binding = %#v, seeded %#v", reopenedRef, ref)
	}
	receipt, err := reopened.AbortAutomationRunCommand(context.Background(), runID, commandID, agentrun.OperationID(operationID), abort.Reason)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Replayed || receipt.CommandID != agentrun.CommandID(commandID) || receipt.OperationID != agentrun.OperationID(operationID) || receipt.Cursor != 3 {
		t.Fatalf("replayed abort receipt = %#v", receipt)
	}
}
