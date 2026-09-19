package automationapp

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	apptask "denova/internal/app/task"
	"denova/internal/automation"
	projectdomain "denova/internal/project"
)

type deliveryTestHost struct {
	Host
	fail     bool
	calls    []ProjectConversationTurn
	accepted *deliveryTestTurn
}

type deliveryTestTurn struct {
	task    *apptask.Task
	receipt agentrun.CommandReceipt
	release chan struct{}
	starts  int
}

func (turn *deliveryTestTurn) Receipt() agentrun.CommandReceipt { return turn.receipt }
func (turn *deliveryTestTurn) Task() *apptask.Task              { return turn.task }
func (turn *deliveryTestTurn) Start() error {
	turn.starts++
	return turn.task.Start(func(ctx context.Context, _ *apptask.Task, _ func(agentrun.Event)) {
		select {
		case <-turn.release:
		case <-ctx.Done():
		}
	})
}
func (host *deliveryTestHost) AcceptProjectConversationTurn(ctx context.Context, input ProjectConversationTurn) (ProjectConversationExecution, error) {
	host.calls = append(host.calls, input)
	if host.fail {
		return nil, errors.New("temporary admission failure")
	}
	task, err := apptask.NewDeferredWithContext(ctx, nil)
	if err != nil {
		return nil, err
	}
	host.accepted = &deliveryTestTurn{task: task, receipt: agentrun.CommandReceipt{CommandID: agentrun.CommandID(input.CommandID), OperationID: "accepted-operation", Cursor: 1}, release: make(chan struct{})}
	return host.accepted, nil
}

func TestTriggerRetriesFrozenInputAndDoesNotOwnAcceptedWorker(t *testing.T) {
	root := t.TempDir()
	sessions, err := session.NewStore(filepath.Join(root, "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	snap := &automationWorkspaceSnapshot{projectID: "project-one", projectType: projectdomain.TypeBook, stateRoot: filepath.Join(root, "store"), workspace: filepath.Join(root, "project"), novaDir: root, sessionStore: sessions}
	store := storeForSnapshot(snap)
	task, err := store.Create(automation.TaskDefinition{Scope: automation.ScopeWorkspace, Target: automation.ExecutionTarget{Kind: automation.TargetKindWorkspace, ProjectID: snap.projectID}, Name: "Original name", Prompt: "Original prompt", Template: automation.TemplateCustomPrompt})
	if err != nil {
		t.Fatal(err)
	}
	host := &deliveryTestHost{fail: true}
	service := NewService(host)
	start := func() (*apptask.Task, automation.RunRecord, error) {
		return service.startTaskWithSourceRunID(t.Context(), snap, task.CatalogID, automation.TriggerManual, "", "one-delivery", nil)
	}
	if _, _, err := start(); err == nil {
		t.Fatal("expected admission failure")
	}
	_, pending, err := store.GetRunByID("one-delivery")
	if err != nil || pending.DeliveryStatus != automation.DeliveryPending || !strings.Contains(pending.Input.Message, "Original prompt") {
		t.Fatalf("pending input: %+v %v", pending, err)
	}
	task.Prompt = "Changed prompt"
	task.Name = "Changed name"
	if _, err := store.Update(task.CatalogID, task); err != nil {
		t.Fatal(err)
	}
	host.fail = false
	worker, accepted, err := start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { close(host.accepted.release); <-worker.Done() }()
	if accepted.DeliveryStatus != automation.DeliveryAccepted || host.accepted.starts != 1 || host.calls[1].Message != pending.Input.Message || host.calls[1].SessionTitle != "Original name" {
		t.Fatalf("handoff changed intent: %+v %+v", accepted, host.calls)
	}
	if err := service.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if worker.Finished() || worker.Snapshot().CancelRequested {
		t.Fatal("closing Automation controlled the Project Agent worker")
	}
	// Recreate the trigger service while the Agent owner is still executing.
	service = NewService(host)
	replayWorker, replay, err := start()
	if err != nil || replayWorker != nil || replay.RootRuntimeOperationID != accepted.RootRuntimeOperationID || len(host.calls) != 2 {
		t.Fatalf("replay admitted another Agent: %+v %v calls=%d", replay, err, len(host.calls))
	}
	obligations, err := store.ListDurableObligations()
	if err != nil || len(obligations) != 0 {
		t.Fatalf("accepted Agent retained trigger recovery: %+v %v", obligations, err)
	}
}
