package app

import (
	"context"
	apptask "denova/internal/app/task"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"denova/internal/automation"
)

func TestAutomationInboxConfirmationRetriesFailureBeforeRuntimeAcceptance(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	novaDir := filepath.Join(root, "nova")
	store := automation.NewStore(novaDir, workspace)
	task, err := store.Create(automation.Task{Scope: automation.ScopeWorkspace, Name: "Confirm once", Template: automation.TemplateReview})
	if err != nil {
		t.Fatalf("Create task failed: %v", err)
	}
	item, err := store.CreateInboxItem(automation.TriggerInboxItem{
		TaskID: task.ID, TriggerID: "chapter-1", Scope: automation.ScopeWorkspace, Workspace: workspace,
		Status: automation.InboxStatusPending, ActionPolicy: automation.ActionPolicyConfirm, NotifyPolicy: automation.NotifyPolicyInbox,
		Title: "Confirm", Summary: "Start exactly once", Fingerprint: "confirm-once",
	})
	if err != nil {
		t.Fatalf("Create inbox failed: %v", err)
	}
	service := (&App{}).automation()
	snap := &automationWorkspaceSnapshot{workspace: workspace, novaDir: novaDir}
	crashErr := errors.New("simulated crash after run start")
	started := map[string]int{}
	firstStarter := func(_ context.Context, taskID, trigger, sourceRunID, runID string, evidence []automation.TriggerEvidence) (*apptask.Task, automation.RunRecord, error) {
		started[runID]++
		return nil, automation.RunRecord{
			ID: runID, TaskID: taskID, Trigger: trigger, SourceRunID: sourceRunID, TriggerEvidence: evidence, Status: automation.RunStatusFailed,
		}, crashErr
	}
	if _, err := service.confirmInboxItemWithStarter(context.Background(), store, snap, item.ID, firstStarter); !errors.Is(err, crashErr) {
		t.Fatalf("first confirmation error=%v, want simulated crash", err)
	}
	claimed, err := store.GetInboxItem(item.ID)
	if err != nil || claimed.Status != automation.InboxStatusPending || claimed.RunID == "" {
		t.Fatalf("durable claim after crash=%#v err=%v", claimed, err)
	}

	restarted := automation.NewStore(novaDir, workspace)
	secondStarter := func(_ context.Context, taskID, trigger, sourceRunID, runID string, evidence []automation.TriggerEvidence) (*apptask.Task, automation.RunRecord, error) {
		started[runID]++
		if runID != claimed.RunID {
			t.Fatalf("retry run id=%q want=%q", runID, claimed.RunID)
		}
		return nil, automation.RunRecord{
			ID: runID, TaskID: taskID, Trigger: trigger, SourceRunID: sourceRunID, TriggerEvidence: evidence, Status: automation.RunStatusRunning,
			RuntimeCommandID: automationRunAgentCommandID(runID), RuntimeOperationID: "operation-" + runID, RuntimeReceiptCursor: 1,
		}, nil
	}
	result, err := service.confirmInboxItemWithStarter(context.Background(), restarted, snap, item.ID, secondStarter)
	if err != nil {
		t.Fatalf("replayed confirmation failed: %v", err)
	}
	if result.Item.Status != automation.InboxStatusConfirmed || result.Item.RunID != claimed.RunID || result.Run == nil || result.Run.ID != claimed.RunID {
		t.Fatalf("replayed confirmation result=%#v", result)
	}
	if len(started) != 1 || started[claimed.RunID] != 2 {
		t.Fatalf("confirmation allocated multiple run identities: %#v", started)
	}
}

func TestAutomationInboxConfirmationRequiresDurableRuntimeReceipt(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	novaDir := filepath.Join(root, "nova")
	store := automation.NewStore(novaDir, workspace)
	task, err := store.Create(automation.Task{Scope: automation.ScopeWorkspace, Name: "Confirm durably", Template: automation.TemplateReview})
	if err != nil {
		t.Fatalf("Create task failed: %v", err)
	}
	item, err := store.CreateInboxItem(automation.TriggerInboxItem{
		TaskID: task.ID, TriggerID: "chapter-1", Scope: automation.ScopeWorkspace, Workspace: workspace,
		Status: automation.InboxStatusPending, ActionPolicy: automation.ActionPolicyConfirm, NotifyPolicy: automation.NotifyPolicyInbox,
		Title: "Confirm", Summary: "Require a runtime receipt", Fingerprint: "confirm-durably",
	})
	if err != nil {
		t.Fatalf("Create inbox failed: %v", err)
	}

	service := (&App{}).automation()
	_, err = service.confirmInboxItemWithStarter(
		context.Background(),
		store,
		&automationWorkspaceSnapshot{workspace: workspace, novaDir: novaDir},
		item.ID,
		func(_ context.Context, taskID, trigger, sourceRunID, runID string, evidence []automation.TriggerEvidence) (*apptask.Task, automation.RunRecord, error) {
			return nil, automation.RunRecord{
				ID: runID, TaskID: taskID, Trigger: trigger, SourceRunID: sourceRunID,
				TriggerEvidence: evidence, Status: automation.RunStatusRunning,
			}, nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "durable root receipt") {
		t.Fatalf("confirmation error=%v, want missing durable receipt", err)
	}
	persisted, err := store.GetInboxItem(item.ID)
	if err != nil {
		t.Fatalf("GetInboxItem failed: %v", err)
	}
	if persisted.Status != automation.InboxStatusPending || persisted.RunID == "" {
		t.Fatalf("inbox completed without a durable receipt: %#v", persisted)
	}
}
