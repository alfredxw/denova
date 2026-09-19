package automation

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDeliveryReceiptDoesNotPersistExecutionOrOwnRecovery(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "user"), filepath.Join(root, "workspace"))
	task, err := store.Create(TaskDefinition{Scope: ScopeWorkspace, Name: "Delivery", Template: TemplateReview})
	if err != nil {
		t.Fatal(err)
	}
	pending := RunRecord{ID: "delivery", TaskID: task.ID, SessionID: "conversation", TurnID: "command", Scope: task.Scope, Workspace: task.Target.Workspace, Trigger: TriggerManual, DeliveryStatus: DeliveryPending,
		Input: &RunInput{Message: "original task"}}
	if _, err := store.AppendRun(task.CatalogID, pending); err != nil {
		t.Fatal(err)
	}
	stale, found, err := store.readDurableRunObligation(task.Scope, pending.ID)
	if err != nil || !found {
		t.Fatalf("pending obligation: %v %v", found, err)
	}
	accepted := pending
	accepted.DeliveryStatus = DeliveryAccepted
	accepted.RootRuntimeCommandID, accepted.RootRuntimeOperationID, accepted.RootRuntimeReceiptCursor = "command", "operation", 1
	accepted.Status, accepted.Summary, accepted.Error = RunStatusFailed, "display only", "display only"
	accepted.RuntimeRecoveryRequired = true
	if _, err := store.AppendRun(task.CatalogID, accepted); err != nil {
		t.Fatal(err)
	}
	_, persisted, err := store.GetRunByID(pending.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.DeliveryStatus != DeliveryAccepted || persisted.Status != "" || persisted.Summary != "" || persisted.Error != "" || RunHasDurableObligation(persisted) {
		t.Fatalf("execution leaked into trigger storage: %+v", persisted)
	}
	if _, err := store.AppendRun(task.CatalogID, pending); !errors.Is(err, ErrRunIdentityConflict) {
		t.Fatalf("accepted delivery regressed: %v", err)
	}
	changed := accepted
	changed.RootRuntimeOperationID = "other"
	if _, err := store.AppendRun(task.CatalogID, changed); !errors.Is(err, ErrRunIdentityConflict) {
		t.Fatalf("receipt replaced: %v", err)
	}
	changed = accepted
	changed.Input = &RunInput{Message: "changed task"}
	if _, err := store.AppendRun(task.CatalogID, changed); !errors.Is(err, ErrRunIdentityConflict) {
		t.Fatalf("input replaced: %v", err)
	}
	// Crash after accepted history is committed but before the hot copy clears.
	if err := store.writeDurableRunObligation(task.Scope, stale); err != nil {
		t.Fatal(err)
	}
	obligations, err := store.ListDurableObligations()
	if err != nil || len(obligations) != 0 {
		t.Fatalf("stale pending copy resurrected delivery: %+v %v", obligations, err)
	}
	if err := store.Delete(task.CatalogID); err != nil {
		t.Fatalf("accepted trigger still controls Agent lifecycle: %v", err)
	}
}

func TestAdoptingLegacyRunBacksUpAndRetainsPendingEffects(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "user"), filepath.Join(root, "workspace"))
	task, err := store.Create(TaskDefinition{Scope: ScopeWorkspace, Name: "Legacy delivery", Template: TemplateReview})
	if err != nil {
		t.Fatal(err)
	}
	legacy := RunRecord{ID: "legacy", TaskID: task.ID, Scope: task.Scope, Workspace: task.Target.Workspace, Trigger: TriggerManual, Status: RunStatusRunning,
		RuntimeCommandID: "command", RuntimeOperationID: "operation", RuntimeReceiptCursor: 1,
		CompletionEffectsPending: true, CompletionEffectsOperationID: "operation", CompletionMutationPaths: []string{"chapters/one.md"}, CompletionMutationEffectIDs: []string{"effect-one"}}
	if _, err := store.AppendRun(task.CatalogID, legacy); err != nil {
		t.Fatal(err)
	}
	adopted := legacy
	adopted.DeliveryStatus = DeliveryAccepted
	adopted.RootRuntimeCommandID, adopted.RootRuntimeOperationID, adopted.RootRuntimeReceiptCursor = "command", "operation", 1
	if _, err := store.AppendRun(task.CatalogID, adopted); err != nil {
		t.Fatal(err)
	}
	path, err := store.durableRunPath(task.Scope, legacy.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path + ".v1.bak")
	if err != nil {
		t.Fatal(err)
	}
	backup, err := decodeDurableRun(path, data)
	if err != nil || backup.Run.Status != RunStatusRunning || backup.Run.RuntimeOperationID != "operation" {
		t.Fatalf("legacy backup: %+v %v", backup, err)
	}
	obligations, err := store.ListDurableObligations()
	if err != nil || len(obligations) != 1 || len(obligations[0].Run.CompletionMutationEffectIDs) != 1 {
		t.Fatalf("legacy effects lost: %+v %v", obligations, err)
	}
	settled := obligations[0].Run
	settled.CompletionEffectsPending = false
	settled.CompletionEffectsCompleted = true
	if _, err := store.AppendRun(task.CatalogID, settled); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendRun(task.CatalogID, adopted); err != nil {
		t.Fatal(err)
	}
	obligations, err = store.ListDurableObligations()
	if err != nil || len(obligations) != 0 {
		t.Fatalf("stale writer reopened legacy effects: %+v %v", obligations, err)
	}
}
