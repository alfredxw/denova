package automation

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestMergeRunMutationEffectReopensOnlyExactTerminalOperation(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "user"), filepath.Join(root, "workspace"))
	task, err := store.Create(TaskDefinition{Scope: ScopeWorkspace, Name: "Late host effect", Template: TemplateReview})
	if err != nil {
		t.Fatal(err)
	}
	run := RunRecord{
		ID: "late-host-effect-run", TaskID: task.ID, Scope: task.Scope, Workspace: task.Target.Workspace,
		Trigger: TriggerManual, Status: RunStatusSuccess,
		RuntimeCommandID: "automation-run:late-host-effect-run", RuntimeOperationID: "operation-1", RuntimeReceiptCursor: 1,
		CompletionEffectsOperationID: "operation-1", CompletionEffectsCompleted: true,
	}
	if _, err := store.AppendRun(task.CatalogID, run); err != nil {
		t.Fatal(err)
	}

	merged, changed, err := store.MergeRunMutationEffect(
		context.Background(), run.ID, "operation-1", "effect-1", []string{" chapters/late.md ", "chapters/late.md"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || !merged.Run.CompletionEffectsPending || merged.Run.CompletionEffectsCompleted ||
		len(merged.Run.CompletionMutationPaths) != 1 || merged.Run.CompletionMutationPaths[0] != "chapters/late.md" ||
		len(merged.Run.CompletionMutationEffectIDs) != 1 || merged.Run.CompletionMutationEffectIDs[0] != "effect-1" {
		t.Fatalf("late effect was not transferred exactly: %#v changed=%t", merged.Run, changed)
	}
	replayed, changed, err := store.MergeRunMutationEffect(
		context.Background(), run.ID, "operation-1", "effect-1", []string{"chapters/different.md"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if changed || len(replayed.Run.CompletionMutationPaths) != 1 || replayed.Run.CompletionMutationPaths[0] != "chapters/late.md" {
		t.Fatalf("same effect replay changed the durable plan: %#v changed=%t", replayed.Run, changed)
	}
	if _, _, err := store.MergeRunMutationEffect(context.Background(), run.ID, "operation-2", "effect-wrong-op", []string{"chapters/wrong.md"}); !errors.Is(err, ErrRunIdentityConflict) {
		t.Fatalf("wrong operation merge error = %v, want ErrRunIdentityConflict", err)
	}

	settled := merged.Run
	settled.CompletionEffectsPending = false
	settled.CompletionEffectsCompleted = true
	if _, err := store.AppendRun(task.CatalogID, settled); err != nil {
		t.Fatal(err)
	}
	staleDirectMutation := settled
	staleDirectMutation.CompletionMutationPaths = append(staleDirectMutation.CompletionMutationPaths, "chapters/not-admitted.md")
	if _, err := store.AppendRun(task.CatalogID, staleDirectMutation); !errors.Is(err, ErrRunIdentityConflict) {
		t.Fatalf("ordinary append reopened effects: %v", err)
	}
	reopened, changed, err := store.MergeRunMutationEffect(
		context.Background(), run.ID, "operation-1", "effect-2", []string{"chapters/second.md"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || !reopened.Run.CompletionEffectsPending || reopened.Run.CompletionEffectsCompleted ||
		len(reopened.Run.CompletionMutationEffectIDs) != 2 || len(reopened.Run.CompletionMutationPaths) != 2 {
		t.Fatalf("second exact effect did not reopen terminal outbox: %#v", reopened.Run)
	}
}

func TestDurableRunRevisionOrdersStaleCleanupAndNewWriteAheadObligation(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "user"), filepath.Join(root, "workspace"))
	task, err := store.Create(TaskDefinition{Scope: ScopeWorkspace, Name: "Revision ordering", Template: TemplateReview})
	if err != nil {
		t.Fatal(err)
	}
	pending := RunRecord{
		ID: "revision-ordered-run", TaskID: task.ID, Scope: task.Scope, Workspace: task.Target.Workspace,
		Trigger: TriggerManual, Status: RunStatusRunning,
		RuntimeCommandID: "automation-run:revision-ordered-run", RuntimeOperationID: "operation-1", RuntimeReceiptCursor: 1,
		RuntimeRecoveryRequired: true,
	}
	if _, err := store.AppendRun(task.CatalogID, pending); err != nil {
		t.Fatal(err)
	}
	staleObligation, found, err := store.readDurableRunObligation(task.Scope, pending.ID)
	if err != nil || !found {
		t.Fatalf("read first obligation: found=%t err=%v", found, err)
	}

	settled := pending
	settled.Status = RunStatusFailed
	settled.RuntimeRecoveryRequired = false
	settled.CompletionEffectsCompleted = true
	if _, err := store.AppendRun(task.CatalogID, settled); err != nil {
		t.Fatal(err)
	}
	settledHistory, found, err := store.readDurableRun(task.Scope, pending.ID)
	if err != nil || !found || settledHistory.Revision <= staleObligation.Revision {
		t.Fatalf("settled history revision = %d stale=%d found=%t err=%v", settledHistory.Revision, staleObligation.Revision, found, err)
	}

	// Crash after full-history commit but before hot-file removal: the older hot
	// copy must not resurrect accepted work.
	if err := store.writeDurableRunObligation(task.Scope, staleObligation); err != nil {
		t.Fatal(err)
	}
	_, recovered, err := store.GetRunByID(pending.ID)
	if err != nil || recovered.Status != RunStatusFailed || recovered.RuntimeRecoveryRequired {
		t.Fatalf("stale hot copy won over settled history: run=%#v err=%v", recovered, err)
	}
	if obligations, err := store.ListDurableObligations(); err != nil || len(obligations) != 0 {
		t.Fatalf("stale hot copy entered recovery scan: %#v err=%v", obligations, err)
	}

	// Crash after a valid successor intent reaches the write-ahead file but
	// before full history: the newer revision must remain recoverable.
	newerObligation := settledHistory
	newerObligation.Revision++
	newerObligation.Run.PendingRuntimeCommandID = "follow-up-2"
	newerObligation.Run.PendingRuntimeIntentHash = "follow-up-2-intent"
	if err := store.writeDurableRunObligation(task.Scope, newerObligation); err != nil {
		t.Fatal(err)
	}
	_, recovered, err = store.GetRunByID(pending.ID)
	if err != nil || recovered.PendingRuntimeCommandID != "follow-up-2" {
		t.Fatalf("new write-ahead obligation lost: run=%#v err=%v", recovered, err)
	}
	obligations, err := store.ListDurableObligations()
	if err != nil || len(obligations) != 1 || obligations[0].Run.PendingRuntimeCommandID != "follow-up-2" {
		t.Fatalf("new write-ahead recovery scan = %#v err=%v", obligations, err)
	}
}

func TestFailedReceiptlessRunAllowsOnlyExplicitAdmissionRetry(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "user"), filepath.Join(root, "workspace"))
	task, err := store.Create(TaskDefinition{Scope: ScopeWorkspace, Name: "Admission retry", Template: TemplateReview})
	if err != nil {
		t.Fatal(err)
	}
	failed := RunRecord{
		ID: "receiptless-retry", TaskID: task.ID, Scope: task.Scope, Workspace: task.Target.Workspace,
		Trigger: TriggerManual, Status: RunStatusFailed, Error: "not accepted", CompletionEffectsCompleted: true,
	}
	if _, err := store.AppendRun(task.CatalogID, failed); err != nil {
		t.Fatal(err)
	}
	unsafe := failed
	unsafe.Status = RunStatusRunning
	unsafe.Error = ""
	if _, err := store.AppendRun(task.CatalogID, unsafe); !errors.Is(err, ErrRunIdentityConflict) {
		t.Fatalf("receiptless terminal run restarted without admission intent: %v", err)
	}
	retry := unsafe
	retry.RuntimeAdmissionPending = true
	updated, err := store.AppendRun(task.CatalogID, retry)
	if err != nil {
		t.Fatal(err)
	}
	if updated.LastRun == nil || updated.LastRun.Status != RunStatusRunning || !updated.LastRun.RuntimeAdmissionPending {
		t.Fatalf("explicit admission retry = %#v", updated.LastRun)
	}
	clearedWithoutProof := *updated.LastRun
	clearedWithoutProof.RuntimeAdmissionPending = false
	if _, err := store.AppendRun(task.CatalogID, clearedWithoutProof); !errors.Is(err, ErrRunIdentityConflict) {
		t.Fatalf("admission intent cleared without receipt or terminal proof: %v", err)
	}
}
