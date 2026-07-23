package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"denova/config"
	runstate "denova/internal/agent/runtime"
	"denova/internal/automation"
)

func TestAutomationManualRunIDIsStableAndScoped(t *testing.T) {
	first, err := automationManualRunID("task-a", "command-a")
	if err != nil {
		t.Fatal(err)
	}
	replay, err := automationManualRunID("task-a", "command-a")
	if err != nil {
		t.Fatal(err)
	}
	otherTask, err := automationManualRunID("task-b", "command-a")
	if err != nil {
		t.Fatal(err)
	}
	if first != replay || first == otherTask {
		t.Fatalf("ids first=%q replay=%q other_task=%q", first, replay, otherTask)
	}
	if len(first) != len("run-command-")+32 {
		t.Fatalf("bounded id length = %d", len(first))
	}
}

func TestAutomationManualRunIDUsesCatalogScopeForImportedIDs(t *testing.T) {
	const compactID = "imported-task"
	commandID := "manual-command"
	locatorA := automation.CatalogTaskID(automation.ScopeWorkspace, filepath.Join(t.TempDir(), "book-a"), compactID)
	locatorB := automation.CatalogTaskID(automation.ScopeWorkspace, filepath.Join(t.TempDir(), "book-b"), compactID)
	runA, err := automationManualRunID(locatorA, commandID)
	if err != nil {
		t.Fatal(err)
	}
	runB, err := automationManualRunID(locatorB, commandID)
	if err != nil {
		t.Fatal(err)
	}
	if runA == runB {
		t.Fatalf("same imported task ID produced colliding manual runs: %q", runA)
	}
}

func TestAutomationManualRunIDRequiresCallerIdentity(t *testing.T) {
	if _, err := automationManualRunID("task", ""); !errors.Is(err, ErrAgentCommandIDRequired) {
		t.Fatalf("error = %v, want ErrAgentCommandIDRequired", err)
	}
	if _, err := automationManualRunID("task", strings.Repeat("x", 4097)); !errors.Is(err, runstate.ErrInvalidCommand) {
		t.Fatalf("oversized error = %v, want ErrInvalidCommand", err)
	}
}

func TestAutomationManualCommandCanonicalizesTaskAliasBeforeAdmission(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	dataDir := filepath.Join(root, "nova")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	store := automation.NewStore(dataDir, workspace)
	taskDef, err := store.Create(automation.Task{
		Scope: automation.ScopeWorkspace, Enabled: true, Name: "canonical manual command", Template: automation.TemplateReview,
	})
	if err != nil {
		t.Fatal(err)
	}
	const commandID = "manual-canonical-command"
	runID, err := automationManualRunID(taskDef.CatalogID, commandID)
	if err != nil {
		t.Fatal(err)
	}
	rootCommandID := automationRunAgentCommandID(runID)
	run := automation.RunRecord{
		ID: runID, TaskID: taskDef.ID, SessionID: automationRunSessionID(runID),
		Scope: taskDef.Scope, Workspace: workspace, Trigger: automation.TriggerManual,
		RootRuntimeCommandID: rootCommandID, RootRuntimeOperationID: "manual-root-operation", RootRuntimeReceiptCursor: 1,
		RuntimeCommandID: rootCommandID, RuntimeOperationID: "manual-root-operation", RuntimeReceiptCursor: 1,
		Status: automation.RunStatusSuccess, CompletionEffectsCompleted: true,
	}
	if _, err := store.AppendRun(taskDef.ID, run); err != nil {
		t.Fatal(err)
	}
	seedAutomationRuntimeJournal(t, dataDir, taskDef, run, []runstate.EventPayload{
		runstate.CommandAcceptedEvent{CommandID: runstate.CommandID(rootCommandID), CommandKind: "start_turn", OperationID: "manual-root-operation", Fingerprint: "manual-root"},
		runstate.OperationStartedEvent{OperationID: "manual-root-operation"},
		runstate.OperationSettledEvent{OperationID: "manual-root-operation", Status: runstate.OperationSucceeded},
	})

	application, err := New(context.Background(), &config.Config{NovaDir: dataDir, Workspace: workspace, OpenAIModel: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	firstTask, first, err := application.StartAutomationTaskCommand(context.Background(), taskDef.ID, commandID, nil)
	if err != nil {
		application.Close()
		t.Fatal(err)
	}
	secondTask, second, err := application.StartAutomationTaskCommand(context.Background(), taskDef.CatalogID, commandID, nil)
	if err != nil {
		application.Close()
		t.Fatal(err)
	}
	if first.ID != runID || second.ID != runID || first.ID != second.ID {
		application.Close()
		t.Fatalf("alias admissions first=%#v second=%#v", first, second)
	}
	<-firstTask.Done()
	<-secondTask.Done()
	application.Close()

	ref := runstate.BindingRef{
		Kind: runstate.BindingAutomation, Profile: runstate.ProfileAutomation,
		Workspace: workspace, SessionID: run.SessionID, TaskID: taskDef.ID,
	}
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
	events, err := journal.Load(context.Background())
	if closeErr := journal.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	accepted := 0
	for _, event := range events {
		if payload, ok := event.Payload.(runstate.CommandAcceptedEvent); ok && payload.CommandKind == "start_turn" {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatalf("canonical task aliases admitted %d StartTurn commands, want 1", accepted)
	}
}

func TestAutomationCompletedFollowUpReplaysFromDurableIntentAfterRestart(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	dataDir := filepath.Join(root, "nova")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	store := automation.NewStore(dataDir, workspace)
	taskDef, err := store.Create(automation.Task{
		Scope: automation.ScopeWorkspace, Enabled: true, Name: "follow-up replay", Template: automation.TemplateReview,
	})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := newAutomationFollowUpIdentity("completed-follow-up", "follow-up-command", "continue exactly")
	if err != nil {
		t.Fatal(err)
	}
	run := automation.RunRecord{
		ID: identity.runID, TaskID: taskDef.ID, SessionID: automationRunSessionID(identity.runID),
		Scope: taskDef.Scope, Workspace: workspace, Trigger: automation.TriggerManual,
		RootRuntimeCommandID: automationRunAgentCommandID(identity.runID), RootRuntimeOperationID: "root-operation", RootRuntimeReceiptCursor: 1,
		RuntimeCommandID: identity.commandID, RuntimeOperationID: "follow-up-operation", RuntimeReceiptCursor: 5,
		RuntimeIntentHash: identity.fingerprint, RuntimeCommandFingerprint: "runtime-follow-up-fingerprint",
		Status: automation.RunStatusSuccess, CompletionEffectsCompleted: true,
	}
	if _, err := store.AppendRun(taskDef.CatalogID, run); err != nil {
		t.Fatal(err)
	}

	application, err := New(context.Background(), &config.Config{NovaDir: dataDir, Workspace: workspace, OpenAIModel: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	defer application.Close()
	replayedTask, replayed, err := application.ContinueAutomationRun(context.Background(), run.ID, identity.commandID, identity.message)
	if err != nil {
		t.Fatalf("exact durable follow-up replay failed: %v", err)
	}
	if replayedTask == nil || replayed.RuntimeOperationID != run.RuntimeOperationID || replayed.Status != automation.RunStatusSuccess {
		t.Fatalf("durable replay task=%v run=%#v", replayedTask, replayed)
	}
	<-replayedTask.Done()
	if conflictingTask, _, err := application.ContinueAutomationRun(context.Background(), run.ID, identity.commandID, "different payload"); !errors.Is(err, ErrAgentCommandConflict) || conflictingTask != nil {
		t.Fatalf("same command with different payload task=%v err=%v", conflictingTask, err)
	}
}
