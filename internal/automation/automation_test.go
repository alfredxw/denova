package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestExecutionTargetMigratesReleasedWorkspaceIDToProjectID(t *testing.T) {
	var target ExecutionTarget
	if err := json.Unmarshal([]byte(`{"kind":"workspace","workspace_id":"project-legacy","workspace":"/books/legacy"}`), &target); err != nil {
		t.Fatal(err)
	}
	if target.ProjectID != "project-legacy" || target.Workspace != "/books/legacy" {
		t.Fatalf("released target was not migrated: %#v", target)
	}
	persisted, err := json.Marshal(target)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(persisted), "workspace_id") || !strings.Contains(string(persisted), `"project_id":"project-legacy"`) {
		t.Fatalf("current target persisted a non-canonical identity: %s", persisted)
	}

	conflicting := []byte(`{"kind":"workspace","project_id":"project-new","workspace_id":"project-old"}`)
	if err := json.Unmarshal(conflicting, &target); err == nil {
		t.Fatal("conflicting canonical and released target identities were accepted")
	}
}

func TestStoreUpdateIfRevisionRejectsStaleDefinition(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "user"), filepath.Join(root, "workspace"))
	created, err := store.Create(TaskDefinition{
		Scope:    ScopeWorkspace,
		Name:     "Review",
		Template: TemplateReview,
		Prompt:   "original",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if created.Revision == "" {
		t.Fatal("created task should expose a definition revision")
	}

	agent, err := store.Update(created.ID, Task{Prompt: "agent update"})
	if err != nil {
		t.Fatalf("agent Update failed: %v", err)
	}
	if agent.Revision == created.Revision {
		t.Fatalf("definition revision did not change: %q", agent.Revision)
	}

	_, err = store.UpdateIfRevision(created.ID, Task{Prompt: "stale editor"}, created.Revision)
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale UpdateIfRevision error = %v, want ErrRevisionConflict", err)
	}
	latest, getErr := store.Get(created.ID)
	if getErr != nil {
		t.Fatalf("Get failed: %v", getErr)
	}
	if latest.Prompt != "agent update" {
		t.Fatalf("stale update overwrote agent content: %q", latest.Prompt)
	}
}

func TestStoreUpdateIfRevisionPreservesSchedulerRuntimeState(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "user"), filepath.Join(root, "workspace"))
	created, err := store.Create(TaskDefinition{
		Scope:    ScopeWorkspace,
		Name:     "Review",
		Template: TemplateReview,
		Prompt:   "original",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	runtimeState := TriggerState{LastEvidenceFingerprint: "scheduler-new-state"}
	withRuntime, err := store.UpdateTriggerState(created.ID, "schedule", runtimeState)
	if err != nil {
		t.Fatalf("UpdateTriggerState failed: %v", err)
	}
	if withRuntime.Revision != created.Revision {
		t.Fatalf("runtime-only update changed definition revision: got %q want %q", withRuntime.Revision, created.Revision)
	}

	staleFullTask := created
	staleFullTask.Prompt = "definition update"
	staleFullTask.TriggerState = map[string]TriggerState{"schedule": {LastEvidenceFingerprint: "stale-state"}}
	updated, err := store.UpdateIfRevision(created.ID, staleFullTask, created.Revision)
	if err != nil {
		t.Fatalf("UpdateIfRevision failed: %v", err)
	}
	if got := updated.TriggerState["schedule"].LastEvidenceFingerprint; got != "scheduler-new-state" {
		t.Fatalf("definition update replayed stale scheduler state: %q", got)
	}
}

func TestStoreSeparatesUserAndWorkspaceTasks(t *testing.T) {
	root := t.TempDir()
	userDir := filepath.Join(root, "user")
	workspace := filepath.Join(root, "book")
	if err := os.MkdirAll(filepath.Join(workspace, ".nova"), 0o755); err != nil {
		t.Fatal(err)
	}
	store := NewStore(userDir, workspace)

	userTask, err := store.Create(TaskDefinition{Scope: ScopeUser, Name: "User task", Template: TemplateCustomPrompt})
	if err != nil {
		t.Fatalf("create user task: %v", err)
	}
	workspaceTask, err := store.Create(TaskDefinition{Scope: ScopeWorkspace, Name: "Workspace task", Template: TemplateReview})
	if err != nil {
		t.Fatalf("create workspace task: %v", err)
	}
	if _, err := os.Stat(filepath.Join(userDir, "automations", "tasks.json")); err != nil {
		t.Fatalf("user tasks not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".nova", "automations", "tasks.json")); err != nil {
		t.Fatalf("workspace tasks not written: %v", err)
	}

	tasks, err := store.List()
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("task count = %d, want 2 user-created tasks", len(tasks))
	}

	userOnly, err := NewStore(userDir, "").List()
	if err != nil {
		t.Fatalf("list user-only: %v", err)
	}
	if len(userOnly) != 1 || userOnly[0].ID != userTask.ID {
		t.Fatalf("user-only tasks = %#v, want %s", userOnly, userTask.ID)
	}
	if _, err := NewStore(userDir, "").Get(workspaceTask.ID); err == nil {
		t.Fatalf("workspace task should not be visible without workspace")
	}
}

func TestStoreListDoesNotCreateWorkspaceAutomationFile(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	store := NewStore(filepath.Join(root, "user"), workspace)
	tasks, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("task count = %d, want no implicit tasks", len(tasks))
	}
	path := filepath.Join(workspace, ".denova", "automations", "tasks.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("read-only List should not create %s: %v", path, err)
	}
}

func TestStoreGetRunByIDResolvesRunAcrossScopes(t *testing.T) {
	root := t.TempDir()
	userDir := filepath.Join(root, "user")
	workspace := filepath.Join(root, "workspace")
	store := NewStore(userDir, workspace)

	userTask, err := store.Create(TaskDefinition{Scope: ScopeUser, Name: "User task", Template: TemplateCustomPrompt})
	if err != nil {
		t.Fatalf("Create user task failed: %v", err)
	}
	userRun := RunRecord{ID: "run-user", TaskID: userTask.ID, Scope: ScopeUser, Trigger: TriggerManual, Status: RunStatusSuccess}
	if _, err := store.AppendRun(userTask.ID, userRun); err != nil {
		t.Fatalf("AppendRun user failed: %v", err)
	}

	workspaceTask, err := store.Create(TaskDefinition{Scope: ScopeWorkspace, Name: "Workspace task", Template: TemplateReview})
	if err != nil {
		t.Fatalf("Create workspace task failed: %v", err)
	}
	workspaceRun := RunRecord{ID: "run-workspace", TaskID: workspaceTask.ID, Scope: ScopeWorkspace, Trigger: TriggerManual, Status: RunStatusSuccess}
	if _, err := store.AppendRun(workspaceTask.ID, workspaceRun); err != nil {
		t.Fatalf("AppendRun workspace failed: %v", err)
	}

	for _, runID := range []string{"run-user", "run-workspace"} {
		if _, run, err := store.GetRunByID(runID); err != nil {
			t.Fatalf("GetRunByID(%q) failed: %v", runID, err)
		} else if run.ID != runID {
			t.Fatalf("GetRunByID(%q) returned run %q", runID, run.ID)
		}
	}
	if _, _, err := store.GetRunByID("run-missing"); err == nil {
		t.Fatal("GetRunByID for unknown run returned nil error")
	}
	if _, _, err := store.GetRunByID("  "); err == nil {
		t.Fatal("GetRunByID for empty run id returned nil error")
	}
}

func TestStoreDurableRunLedgerOutlivesRecentRunsProjection(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "user"), filepath.Join(root, "workspace"))
	task, err := store.Create(TaskDefinition{Scope: ScopeWorkspace, Name: "Durable ledger", Template: TemplateReview})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC().Add(-time.Hour)
	obligation := RunRecord{
		ID: "old-pending-outbox", TaskID: task.ID, Scope: task.Scope, Workspace: task.Target.Workspace,
		Trigger: TriggerManual, Status: RunStatusSuccess, StartedAt: base,
		CompletionEffectsPending: true,
	}
	if _, err := store.AppendRun(task.CatalogID, obligation); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < MaxRecentRuns+5; index++ {
		run := RunRecord{
			ID: fmt.Sprintf("settled-%02d", index), TaskID: task.ID, Scope: task.Scope,
			Workspace: task.Target.Workspace, Trigger: TriggerManual, Status: RunStatusSuccess,
			StartedAt: base.Add(time.Duration(index+1) * time.Minute), CompletionEffectsCompleted: true,
		}
		if _, err := store.AppendRun(task.CatalogID, run); err != nil {
			t.Fatalf("AppendRun %d failed: %v", index, err)
		}
	}

	projected, err := store.Get(task.CatalogID)
	if err != nil {
		t.Fatal(err)
	}
	if len(projected.RecentRuns) != MaxRecentRuns {
		t.Fatalf("recent projection length = %d, want %d", len(projected.RecentRuns), MaxRecentRuns)
	}
	for _, run := range projected.RecentRuns {
		if run.ID == obligation.ID {
			t.Fatal("old obligation unexpectedly remained in bounded RecentRuns projection")
		}
	}
	_, recovered, err := store.GetRunByID(obligation.ID)
	if err != nil {
		t.Fatalf("GetRunByID lost clipped obligation: %v", err)
	}
	if !recovered.CompletionEffectsPending {
		t.Fatalf("recovered obligation = %#v", recovered)
	}
	durable, err := store.ListDurableRuns()
	if err != nil {
		t.Fatal(err)
	}
	if len(durable) != MaxRecentRuns+6 {
		t.Fatalf("durable run count = %d, want %d", len(durable), MaxRecentRuns+6)
	}
	obligations, err := store.ListDurableObligations()
	if err != nil {
		t.Fatal(err)
	}
	if len(obligations) != 1 || obligations[0].Run.ID != obligation.ID {
		t.Fatalf("hot obligation scan = %#v, want only %s", obligations, obligation.ID)
	}
	recovered.CompletionEffectsPending = false
	recovered.CompletionEffectsCompleted = true
	if _, err := store.AppendRun(task.CatalogID, recovered); err != nil {
		t.Fatalf("settle obligation: %v", err)
	}
	obligations, err = store.ListDurableObligations()
	if err != nil {
		t.Fatal(err)
	}
	if len(obligations) != 0 {
		t.Fatalf("settled run remained in hot obligation scan: %#v", obligations)
	}
}

func TestStoreObligationScanIgnoresLegacySettledSuccess(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "user"), filepath.Join(root, "workspace"))
	task, err := store.Create(TaskDefinition{Scope: ScopeWorkspace, Name: "Legacy review", Template: TemplateReview})
	if err != nil {
		t.Fatal(err)
	}
	legacyRun := RunRecord{
		ID: "legacy-success", TaskID: task.ID, Scope: task.Scope, Workspace: task.Target.Workspace,
		Trigger: TriggerManual, Status: RunStatusSuccess, StartedAt: time.Now().UTC().Add(-time.Minute),
		FinishedAt: time.Now().UTC(),
	}
	task.LastRun = &legacyRun
	task.RecentRuns = []RunRecord{legacyRun}
	if _, err := store.Update(task.CatalogID, task); err != nil {
		t.Fatal(err)
	}

	obligations, err := store.ListDurableObligations()
	if err != nil {
		t.Fatal(err)
	}
	if len(obligations) != 0 {
		t.Fatalf("legacy settled success entered recovery scan: %#v", obligations)
	}
}

func TestStoreObligationScanDoesNotResurrectStaleTaskProjection(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "user"), filepath.Join(root, "workspace"))
	task, err := store.Create(TaskDefinition{Scope: ScopeWorkspace, Name: "Crash ordering", Template: TemplateReview})
	if err != nil {
		t.Fatal(err)
	}
	pending := RunRecord{
		ID: "settled-before-projection", TaskID: task.ID, Scope: task.Scope, Workspace: task.Target.Workspace,
		Trigger: TriggerManual, Status: RunStatusRunning, RuntimeRecoveryRequired: true,
	}
	if _, err := store.AppendRun(task.CatalogID, pending); err != nil {
		t.Fatal(err)
	}
	taskPath, err := store.pathForScope(ScopeWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	staleProjection, err := os.ReadFile(taskPath)
	if err != nil {
		t.Fatal(err)
	}
	settled := pending
	settled.Status = RunStatusFailed
	settled.RuntimeRecoveryRequired = false
	settled.CompletionEffectsCompleted = true
	if _, err := store.AppendRun(task.CatalogID, settled); err != nil {
		t.Fatal(err)
	}
	// Recreate the exact crash state after full-history commit and obligation
	// removal but before the final task projection write.
	if err := durableWriteJSON(taskPath, staleProjection, 0o644); err != nil {
		t.Fatal(err)
	}
	obligations, err := store.ListDurableObligations()
	if err != nil {
		t.Fatal(err)
	}
	if len(obligations) != 0 {
		t.Fatalf("stale RecentRuns resurrected settled obligation: %#v", obligations)
	}
	_, recovered, err := store.GetRunByID(pending.ID)
	if err != nil || recovered.Status != RunStatusFailed || recovered.RuntimeRecoveryRequired {
		t.Fatalf("full run ledger was not authoritative: run=%#v err=%v", recovered, err)
	}
}

func TestStoreDeleteRejectsLiveRunAndArchivesPendingEffects(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "user"), filepath.Join(root, "workspace"))
	liveTask, err := store.Create(TaskDefinition{Scope: ScopeWorkspace, Name: "Live", Template: TemplateReview})
	if err != nil {
		t.Fatal(err)
	}
	liveRun := RunRecord{
		ID: "live-run", TaskID: liveTask.ID, Scope: liveTask.Scope, Workspace: liveTask.Target.Workspace,
		Trigger: TriggerManual, Status: RunStatusRunning, RuntimeRecoveryRequired: true,
	}
	if _, err := store.AppendRun(liveTask.CatalogID, liveRun); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(liveTask.CatalogID); !errors.Is(err, ErrTaskHasActiveRun) {
		t.Fatalf("Delete live task error = %v, want ErrTaskHasActiveRun", err)
	}
	if task, err := store.Get(liveTask.CatalogID); err != nil || task.ArchivedAt != nil {
		t.Fatalf("live task changed after rejected delete: task=%#v err=%v", task, err)
	}

	outboxTask, err := store.Create(TaskDefinition{Scope: ScopeWorkspace, Name: "Outbox", Template: TemplateReview})
	if err != nil {
		t.Fatal(err)
	}
	outboxRun := RunRecord{
		ID: "outbox-run", TaskID: outboxTask.ID, Scope: outboxTask.Scope, Workspace: outboxTask.Target.Workspace,
		Trigger: TriggerManual, Status: RunStatusSuccess, CompletionEffectsPending: true,
	}
	if _, err := store.AppendRun(outboxTask.CatalogID, outboxRun); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(outboxTask.CatalogID); err != nil {
		t.Fatalf("Delete with pending completion effects failed: %v", err)
	}
	listed, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range listed {
		if task.CatalogID == outboxTask.CatalogID {
			t.Fatal("archived task remained visible in task catalog")
		}
	}
	archived, err := store.Get(outboxTask.CatalogID)
	if err != nil || archived.ArchivedAt == nil || archived.Enabled {
		t.Fatalf("archived tombstone = %#v err=%v", archived, err)
	}
	owner, recovered, err := store.GetRunByID(outboxRun.ID)
	if err != nil || owner.ArchivedAt == nil || !recovered.CompletionEffectsPending {
		t.Fatalf("pending outbox was not recoverable after delete: task=%#v run=%#v err=%v", owner, recovered, err)
	}
}

func TestStoreAppendRunUpdatesExistingRun(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	store := NewStore(filepath.Join(root, "nova"), workspace)
	task, err := store.Create(TaskDefinition{Scope: ScopeWorkspace, Name: "Review", Template: TemplateReview})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	first := RunRecord{ID: "run-1", TaskID: task.ID, Scope: ScopeWorkspace, Trigger: TriggerManual, Status: RunStatusSuccess, Summary: "first"}
	if _, err := store.AppendRun(task.ID, first); err != nil {
		t.Fatalf("AppendRun first failed: %v", err)
	}
	second := first
	second.Summary = "second"
	updated, err := store.AppendRun(task.ID, second)
	if err != nil {
		t.Fatalf("AppendRun second failed: %v", err)
	}
	if len(updated.RecentRuns) != 1 {
		t.Fatalf("recent runs = %#v, want one updated run", updated.RecentRuns)
	}
	if updated.RecentRuns[0].Summary != "second" || updated.LastRun == nil || updated.LastRun.Summary != "second" {
		t.Fatalf("run was not updated in place: %#v last=%#v", updated.RecentRuns, updated.LastRun)
	}
}

func TestStoreRejectsStaleOperationAfterSuccessorPromotion(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "nova"), filepath.Join(root, "workspace"))
	task, err := store.Create(TaskDefinition{Scope: ScopeWorkspace, Name: "successor CAS", Template: TemplateReview})
	if err != nil {
		t.Fatal(err)
	}
	rootCommandID := "automation-run:successor-run"
	rootRun := RunRecord{
		ID: "successor-run", TaskID: task.ID, SessionID: "successor-session", Scope: task.Scope,
		Workspace: task.Target.Workspace, Trigger: TriggerManual,
		RootRuntimeCommandID: rootCommandID, RootRuntimeOperationID: "operation-1", RootRuntimeReceiptCursor: 1,
		RuntimeCommandID: rootCommandID, RuntimeOperationID: "operation-1", RuntimeReceiptCursor: 3,
		Status: RunStatusSuccess, CompletionEffectsCompleted: true,
	}
	if _, err := store.AppendRun(task.CatalogID, rootRun); err != nil {
		t.Fatal(err)
	}
	staleCursor := rootRun
	staleCursor.RuntimeReceiptCursor = 2
	staleCursor.Summary = "same-operation terminal update"
	merged, err := store.AppendRun(task.CatalogID, staleCursor)
	if err != nil {
		t.Fatal(err)
	}
	if merged.LastRun == nil || merged.LastRun.RuntimeReceiptCursor != rootRun.RuntimeReceiptCursor {
		t.Fatalf("same-operation append regressed durable cursor: %#v", merged.LastRun)
	}
	pending := rootRun
	pending.PendingRuntimeCommandID = "follow-up-2"
	pending.PendingRuntimeIntentHash = "follow-up-2-intent"
	if _, err := store.AppendRun(task.CatalogID, pending); err != nil {
		t.Fatal(err)
	}
	successor := pending
	successor.RuntimeCommandID = "follow-up-2"
	successor.RuntimeOperationID = "operation-2"
	successor.RuntimeReceiptCursor = 7
	successor.PendingRuntimeCommandID = ""
	successor.PendingRuntimeIntentHash = ""
	successor.Status = RunStatusRunning
	successor.FinishedAt = time.Time{}
	if _, err := store.AppendRun(task.CatalogID, successor); err != nil {
		t.Fatal(err)
	}

	staleRootWriter := rootRun
	staleRootWriter.Status = RunStatusFailed
	staleRootWriter.Error = "late op1 failure"
	if _, err := store.AppendRun(task.CatalogID, staleRootWriter); !errors.Is(err, ErrRunIdentityConflict) {
		t.Fatalf("stale op1 append error = %v, want identity conflict", err)
	}
	_, persisted, err := store.GetRunByID(rootRun.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.RootRuntimeOperationID != "operation-1" || persisted.RuntimeOperationID != "operation-2" || persisted.Status != RunStatusRunning {
		t.Fatalf("stale op1 writer changed successor state: %#v", persisted)
	}

	terminal := successor
	terminal.Status = RunStatusSuccess
	terminal.FinishedAt = time.Now().UTC()
	if _, err := store.AppendRun(task.CatalogID, terminal); err != nil {
		t.Fatal(err)
	}
	staleRunning := successor
	if _, err := store.AppendRun(task.CatalogID, staleRunning); !errors.Is(err, ErrRunIdentityConflict) {
		t.Fatalf("terminal regression error = %v, want identity conflict", err)
	}
}

func TestStoreCompletionEffectsReceiptIsMonotonicAgainstStaleWriter(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "user"), filepath.Join(root, "workspace"))
	task, err := store.Create(TaskDefinition{Scope: ScopeWorkspace, Name: "Effects CAS", Template: TemplateReview})
	if err != nil {
		t.Fatal(err)
	}
	commandID := "automation-run:effects-cas"
	pending := RunRecord{
		ID: "effects-cas", TaskID: task.ID, Scope: task.Scope, Workspace: task.Target.Workspace,
		Trigger: TriggerManual, Status: RunStatusSuccess,
		RootRuntimeCommandID: commandID, RootRuntimeOperationID: "operation-1", RootRuntimeReceiptCursor: 1,
		RuntimeCommandID: commandID, RuntimeOperationID: "operation-1", RuntimeReceiptCursor: 1,
		CompletionEffectsPending: true, CompletionEffectsOperationID: "operation-1",
		CompletionMutationPaths: []string{"chapters/committed.md"},
	}
	if _, err := store.AppendRun(task.CatalogID, pending); err != nil {
		t.Fatal(err)
	}
	acknowledged := pending
	acknowledged.CompletionEffectsPending = false
	acknowledged.CompletionEffectsCompleted = true
	if _, err := store.AppendRun(task.CatalogID, acknowledged); err != nil {
		t.Fatal(err)
	}
	stale := pending
	stale.CompletionEffectsOperationID = ""
	stale.CompletionMutationPaths = nil
	updated, err := store.AppendRun(task.CatalogID, stale)
	if err != nil {
		t.Fatalf("stale append should merge acknowledged facts: %v", err)
	}
	if updated.LastRun == nil || !updated.LastRun.CompletionEffectsCompleted || updated.LastRun.CompletionEffectsPending ||
		updated.LastRun.CompletionEffectsOperationID != "operation-1" || len(updated.LastRun.CompletionMutationPaths) != 1 {
		t.Fatalf("stale writer regressed effects receipt: %#v", updated.LastRun)
	}
	lateMutation := acknowledged
	lateMutation.CompletionMutationPaths = append(lateMutation.CompletionMutationPaths, "chapters/unacknowledged.md")
	if _, err := store.AppendRun(task.CatalogID, lateMutation); !errors.Is(err, ErrRunIdentityConflict) {
		t.Fatalf("late mutation after effects ack error = %v, want ErrRunIdentityConflict", err)
	}
}

func TestTaskDefinitionRunAndEvaluationWritesShareFileLease(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	store := NewStore(filepath.Join(root, "nova"), workspace)
	created, err := store.Create(TaskDefinition{
		Scope: ScopeWorkspace, Enabled: true, Name: "before lease", Template: TemplateReview,
	})
	if err != nil {
		t.Fatal(err)
	}
	path, err := store.pathForScope(ScopeWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	release, err := acquireTaskStoreLease(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	released := false
	defer func() {
		if !released {
			_ = release()
		}
	}()

	definition := created
	definition.Name = "after lease"
	run := RunRecord{
		ID: "leased-run", TaskID: created.ID, Scope: ScopeWorkspace,
		Trigger: TriggerManual, Status: RunStatusRunning,
	}
	state := TriggerState{LastEvidenceFingerprint: "leased-evaluation"}
	started := make(chan struct{}, 3)
	results := make(chan error, 3)
	launch := func(operation func() error) {
		go func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					results <- fmt.Errorf("leased task mutation panic: %v", recovered)
				}
			}()
			started <- struct{}{}
			results <- operation()
		}()
	}
	launch(func() error {
		_, updateErr := store.UpdateIfRevision(created.ID, definition, created.Revision)
		return updateErr
	})
	launch(func() error {
		_, appendErr := store.AppendRun(created.ID, run)
		return appendErr
	})
	launch(func() error {
		_, stateErr := store.UpdateTriggerState(created.ID, "schedule", state)
		return stateErr
	})
	for range 3 {
		<-started
	}
	select {
	case mutationErr := <-results:
		t.Fatalf("task mutation escaped the shared file lease: %v", mutationErr)
	case <-time.After(15 * time.Millisecond):
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	released = true
	for range 3 {
		if mutationErr := <-results; mutationErr != nil {
			t.Fatal(mutationErr)
		}
	}

	latest, err := store.Get(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Name != "after lease" || latest.TriggerState["schedule"].LastEvidenceFingerprint != state.LastEvidenceFingerprint {
		t.Fatalf("definition/evaluation mutation was lost: %#v", latest)
	}
	if len(latest.RecentRuns) != 1 || latest.RecentRuns[0].ID != run.ID || latest.LastRun == nil || latest.LastRun.ID != run.ID {
		t.Fatalf("run receipt mutation was lost: recent=%#v last=%#v", latest.RecentRuns, latest.LastRun)
	}
}

func TestAutomationTaskStoreProcessHelper(t *testing.T) {
	if os.Getenv("DENOVA_AUTOMATION_STORE_HELPER") != "1" {
		return
	}
	userDir := os.Getenv("DENOVA_AUTOMATION_STORE_USER_DIR")
	workspace := os.Getenv("DENOVA_AUTOMATION_STORE_WORKSPACE")
	taskID := os.Getenv("DENOVA_AUTOMATION_STORE_TASK_ID")
	readyPath := os.Getenv("DENOVA_AUTOMATION_STORE_READY")
	if err := os.WriteFile(readyPath, []byte("ready"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewStore(userDir, workspace)
	switch os.Getenv("DENOVA_AUTOMATION_STORE_MODE") {
	case "definition":
		current, err := store.Get(taskID)
		if err != nil {
			t.Fatal(err)
		}
		current.Name = "updated by subprocess"
		if _, err := store.UpdateIfRevision(taskID, current, current.Revision); err != nil {
			t.Fatal(err)
		}
	case "run":
		current, err := store.Get(taskID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.AppendRun(taskID, RunRecord{
			ID: "subprocess-run", TaskID: current.ID, Scope: ScopeWorkspace, Workspace: workspace,
			Trigger: TriggerManual, Status: RunStatusSuccess,
		}); err != nil {
			t.Fatal(err)
		}
	case "evaluation":
		if _, err := store.UpdateTriggerState(taskID, "schedule", TriggerState{LastEvidenceFingerprint: "subprocess-evaluation"}); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unknown subprocess store mode %q", os.Getenv("DENOVA_AUTOMATION_STORE_MODE"))
	}
}

func TestTaskDefinitionRunAndEvaluationWritesShareProcessLease(t *testing.T) {
	root := t.TempDir()
	userDir := filepath.Join(root, "nova")
	workspace := filepath.Join(root, "workspace")
	store := NewStore(userDir, workspace)
	created, err := store.Create(TaskDefinition{Scope: ScopeWorkspace, Enabled: true, Name: "before subprocess", Template: TemplateReview})
	if err != nil {
		t.Fatal(err)
	}
	path, err := store.pathForScope(ScopeWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	release, err := acquireTaskStoreLease(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	released := false
	commands := make([]*exec.Cmd, 0, 3)
	defer func() {
		if !released {
			_ = release()
		}
		for _, command := range commands {
			if command.Process != nil && command.ProcessState == nil {
				_ = command.Process.Kill()
				_ = command.Wait()
			}
		}
	}()

	readyPaths := make([]string, 0, 3)
	for _, mode := range []string{"definition", "run", "evaluation"} {
		readyPath := filepath.Join(root, mode+".ready")
		command := exec.Command(os.Args[0], "-test.run=^TestAutomationTaskStoreProcessHelper$", "-test.count=1")
		command.Env = append(os.Environ(),
			"DENOVA_AUTOMATION_STORE_HELPER=1",
			"DENOVA_AUTOMATION_STORE_USER_DIR="+userDir,
			"DENOVA_AUTOMATION_STORE_WORKSPACE="+workspace,
			"DENOVA_AUTOMATION_STORE_TASK_ID="+created.CatalogID,
			"DENOVA_AUTOMATION_STORE_MODE="+mode,
			"DENOVA_AUTOMATION_STORE_READY="+readyPath,
		)
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		commands = append(commands, command)
		readyPaths = append(readyPaths, readyPath)
	}
	deadline := time.Now().Add(500 * time.Millisecond)
	for _, readyPath := range readyPaths {
		for {
			if _, err := os.Stat(readyPath); err == nil {
				break
			} else if !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
			if time.Now().After(deadline) {
				t.Fatalf("subprocess did not reach task-store lease: %s", readyPath)
			}
			time.Sleep(time.Millisecond)
		}
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	released = true
	for _, command := range commands {
		if err := command.Wait(); err != nil {
			t.Fatalf("task-store subprocess failed: %v", err)
		}
	}

	latest, err := store.Get(created.CatalogID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Name != "updated by subprocess" || latest.TriggerState["schedule"].LastEvidenceFingerprint != "subprocess-evaluation" {
		t.Fatalf("subprocess definition/evaluation write was lost: %#v", latest)
	}
	if len(latest.RecentRuns) != 1 || latest.RecentRuns[0].ID != "subprocess-run" {
		t.Fatalf("subprocess run write was lost: %#v", latest.RecentRuns)
	}
}

func TestStoreConcurrentUserScopeCreatesDoNotLoseTasks(t *testing.T) {
	root := t.TempDir()
	userDir := filepath.Join(root, "user")
	workspaces := []string{filepath.Join(root, "one"), filepath.Join(root, "two")}
	const count = 24
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			defer func() {
				if recovered := recover(); recovered != nil {
					errs <- fmt.Errorf("concurrent Create panic: %v", recovered)
				}
			}()
			_, err := NewStore(userDir, workspaces[index%len(workspaces)]).Create(TaskDefinition{
				Scope:    ScopeUser,
				Name:     "Concurrent user task",
				Template: TemplateCustomPrompt,
			})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Create failed: %v", err)
		}
	}
	tasks, err := NewStore(userDir, "").List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(tasks) != count {
		t.Fatalf("task count = %d, want %d", len(tasks), count)
	}
	data, err := os.ReadFile(filepath.Join(userDir, "automations", "tasks.json"))
	if err != nil {
		t.Fatalf("read tasks JSON: %v", err)
	}
	var persisted storeFile
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("tasks JSON is invalid after concurrent writes: %v", err)
	}
	if len(persisted.Tasks) != count {
		t.Fatalf("persisted task count = %d, want %d", len(persisted.Tasks), count)
	}
}

func TestStoreConcurrentAppendRunPreservesEveryRun(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	userDir := filepath.Join(root, "user")
	store := NewStore(userDir, workspace)
	task, err := store.Create(TaskDefinition{Scope: ScopeWorkspace, Name: "Review", Template: TemplateReview})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	const count = 12
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			defer func() {
				if recovered := recover(); recovered != nil {
					errs <- fmt.Errorf("concurrent AppendRun panic: %v", recovered)
				}
			}()
			_, err := NewStore(userDir, workspace).AppendRun(task.ID, RunRecord{
				ID:      fmt.Sprintf("run-%02d", index),
				TaskID:  task.ID,
				Scope:   ScopeWorkspace,
				Trigger: TriggerManual,
				Status:  RunStatusSuccess,
			})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent AppendRun failed: %v", err)
		}
	}
	updated, err := store.Get(task.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if len(updated.RecentRuns) != count {
		t.Fatalf("recent run count = %d, want %d", len(updated.RecentRuns), count)
	}
	seen := make(map[string]bool, count)
	for _, run := range updated.RecentRuns {
		seen[run.ID] = true
	}
	for i := 0; i < count; i++ {
		if !seen[fmt.Sprintf("run-%02d", i)] {
			t.Fatalf("run-%02d was lost: %#v", i, updated.RecentRuns)
		}
	}
}

func TestStoreConcurrentUserInboxWritesRemainWorkspaceScoped(t *testing.T) {
	root := t.TempDir()
	userDir := filepath.Join(root, "user")
	workspaces := []string{filepath.Join(root, "one"), filepath.Join(root, "two")}
	const count = 20
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			defer func() {
				if recovered := recover(); recovered != nil {
					errs <- fmt.Errorf("concurrent inbox write panic: %v", recovered)
				}
			}()
			_, err := NewStore(userDir, workspaces[index%2]).CreateInboxItem(TriggerInboxItem{
				TaskID:       "shared-user-task",
				TriggerID:    "batch",
				Scope:        ScopeUser,
				ActionPolicy: ActionPolicyConfirm,
				NotifyPolicy: NotifyPolicyInbox,
				Title:        fmt.Sprintf("Item %d", index),
				Fingerprint:  fmt.Sprintf("fp-%d", index),
			})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent CreateInboxItem failed: %v", err)
		}
	}
	for _, workspace := range workspaces {
		items, err := NewStore(userDir, workspace).ListInbox()
		if err != nil {
			t.Fatalf("ListInbox(%s) failed: %v", workspace, err)
		}
		if len(items) != count/2 {
			t.Fatalf("workspace %s inbox count = %d, want %d", workspace, len(items), count/2)
		}
		for _, item := range items {
			if canonicalStoreRoot(item.Workspace) != canonicalStoreRoot(workspace) {
				t.Fatalf("workspace %s received foreign inbox item: %#v", workspace, item)
			}
		}
	}
	data, err := os.ReadFile(filepath.Join(userDir, "automations", "inbox.json"))
	if err != nil {
		t.Fatalf("read inbox JSON: %v", err)
	}
	var persisted inboxFile
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("inbox JSON is invalid after concurrent writes: %v", err)
	}
	if len(persisted.Items) != count {
		t.Fatalf("persisted inbox count = %d, want %d", len(persisted.Items), count)
	}
}

func TestStoreRemovesPristineLegacyWorkspaceSeeds(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	store := NewStore(filepath.Join(root, "user"), workspace)
	now := time.Now().UTC()
	tasks := legacyDefaultWorkspaceAutomations(now)
	if got := taskByID(tasks, legacyReviewTaskID); got == nil || got.Prompt != legacyReviewPrompt || got.Prompt == DefaultReviewPrompt {
		t.Fatalf("legacy review seed must retain the exact historical prompt: %#v", got)
	}
	tasks[0].Prompt = "" // Version 1 seeds did not persist editable prompts.
	if err := store.writeScopeFile(ScopeWorkspace, storeFile{SeedVersion: 1, Tasks: tasks}); err != nil {
		t.Fatalf("write legacy seed file failed: %v", err)
	}

	migrated, err := store.List()
	if err != nil {
		t.Fatalf("List after legacy seed failed: %v", err)
	}
	if len(migrated) != 0 {
		t.Fatalf("pristine legacy seeds should be removed: %#v", migrated)
	}
}

func TestStorePreservesEditedOrUsedLegacyWorkspaceSeeds(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	store := NewStore(filepath.Join(root, "user"), workspace)
	now := time.Now().UTC()
	tasks := legacyDefaultWorkspaceAutomations(now)
	tasks[0].Prompt = "用户修改后的续写规则"
	tasks[0].UpdatedAt = now.Add(time.Minute)
	tasks[1].RecentRuns = []RunRecord{{ID: "run-1", TaskID: tasks[1].ID, Status: RunStatusSuccess}}
	if err := store.writeScopeFile(ScopeWorkspace, storeFile{SeedVersion: 2, Tasks: tasks}); err != nil {
		t.Fatalf("write legacy seed file failed: %v", err)
	}

	migrated, err := store.List()
	if err != nil {
		t.Fatalf("List after legacy seed failed: %v", err)
	}
	if len(migrated) != 2 {
		t.Fatalf("edited and used legacy tasks must be preserved: %#v", migrated)
	}
	if got := taskByID(migrated, legacyContinueWritingTaskID); got == nil || got.Prompt != "用户修改后的续写规则" {
		t.Fatalf("edited continue-writing task changed during migration: %#v", got)
	}
	if got := taskByID(migrated, legacyReviewTaskID); got == nil || len(got.RecentRuns) != 1 {
		t.Fatalf("used review task changed during migration: %#v", got)
	}
}

func TestStorePreservesSeedLikeTaskWithoutLegacyFileMarker(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	store := NewStore(filepath.Join(root, "user"), workspace)
	tasks := legacyDefaultWorkspaceAutomations(time.Now().UTC())[:1]
	if err := store.writeScopeFile(ScopeWorkspace, storeFile{Tasks: tasks}); err != nil {
		t.Fatalf("write unversioned task file failed: %v", err)
	}

	listed, err := store.List()
	if err != nil {
		t.Fatalf("List unversioned task file failed: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != legacyContinueWritingTaskID {
		t.Fatalf("unversioned user-owned task must be preserved: %#v", listed)
	}
}

func TestNormalizeScheduleBuildsCronShape(t *testing.T) {
	tests := []struct {
		name     string
		schedule Schedule
		wantCron string
	}{
		{"daily", Schedule{Kind: ScheduleDaily, Hour: 9, Minute: 30}, "30 9 * * *"},
		{"weekly", Schedule{Kind: ScheduleWeekly, Weekday: 2, Hour: 8, Minute: 5}, "5 8 * * 2"},
		{"monthly", Schedule{Kind: ScheduleMonthly, DayOfMonth: 12, Hour: 7, Minute: 0}, "0 7 12 * *"},
		{"every-hours", Schedule{Kind: ScheduleEveryHours, EveryHours: 6, Minute: 15}, "15 */6 * * *"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeSchedule(tt.schedule)
			if err != nil {
				t.Fatalf("NormalizeSchedule failed: %v", err)
			}
			if got.Cron != tt.wantCron {
				t.Fatalf("cron = %q, want %q", got.Cron, tt.wantCron)
			}
		})
	}
}

func TestDueHandlesStructuredSchedules(t *testing.T) {
	now := time.Date(2026, 6, 10, 10, 0, 0, 0, time.UTC)
	last := now.Add(-25 * time.Hour)
	task := Task{
		Enabled:  true,
		Schedule: Schedule{Kind: ScheduleDaily, Hour: 9, Minute: 0},
		LastRun:  &RunRecord{StartedAt: last},
	}
	if !Due(now, task) {
		t.Fatalf("daily task should be due")
	}
	task.Enabled = false
	if Due(now, task) {
		t.Fatalf("disabled task should not be due")
	}
	task.Enabled = true
	task.Schedule = Schedule{Kind: ScheduleManual}
	if Due(now, task) {
		t.Fatalf("manual task should not be due")
	}
}

func TestNormalizeTaskAcceptsContinueWritingTemplate(t *testing.T) {
	task, err := NormalizeTask(Task{Scope: ScopeWorkspace, Name: "Continue", Template: TemplateContinueWriting})
	if err != nil {
		t.Fatalf("NormalizeTask failed: %v", err)
	}
	if task.Template != TemplateContinueWriting {
		t.Fatalf("template = %q, want %q", task.Template, TemplateContinueWriting)
	}
}

func TestNormalizeTaskTrimsModelProfileID(t *testing.T) {
	task, err := NormalizeTask(Task{Scope: ScopeWorkspace, Name: "Profile", Template: TemplateReview, ModelProfileID: " fast "})
	if err != nil {
		t.Fatalf("NormalizeTask failed: %v", err)
	}
	if task.ModelProfileID != "fast" {
		t.Fatalf("model profile id = %q, want fast", task.ModelProfileID)
	}
}

func TestNormalizeTaskDefaultsAndPreservesSessionStrategy(t *testing.T) {
	perRun, err := NormalizeTask(Task{Template: TemplateCustomPrompt})
	if err != nil {
		t.Fatal(err)
	}
	if perRun.SessionStrategy != SessionStrategyPerRun {
		t.Fatalf("default session strategy = %q, want %q", perRun.SessionStrategy, SessionStrategyPerRun)
	}

	perTask, err := NormalizeTask(Task{Template: TemplateCustomPrompt, SessionStrategy: SessionStrategyPerTask})
	if err != nil {
		t.Fatal(err)
	}
	if perTask.SessionStrategy != SessionStrategyPerTask {
		t.Fatalf("explicit session strategy = %q, want %q", perTask.SessionStrategy, SessionStrategyPerTask)
	}
}

func TestNormalizeTaskAcceptsChapterBatchTrigger(t *testing.T) {
	task, err := NormalizeTask(Task{
		Scope:    ScopeWorkspace,
		Name:     "Batch review",
		Template: TemplateReview,
		Triggers: []TriggerDefinition{{
			Type:    TriggerTypeChapterBatch,
			Enabled: true,
		}},
	})
	if err != nil {
		t.Fatalf("NormalizeTask failed: %v", err)
	}
	if len(task.Triggers) != 1 {
		t.Fatalf("trigger count = %d, want 1", len(task.Triggers))
	}
	trigger := task.Triggers[0]
	if trigger.Type != TriggerTypeChapterBatch || trigger.ChapterBatchSize != 5 || trigger.NotifyPolicy != NotifyPolicyInbox {
		t.Fatalf("unexpected chapter batch trigger: %#v", trigger)
	}
}

func TestNormalizeTaskMigratesLegacyScheduleToTaskLevelSilentAutoRun(t *testing.T) {
	task, err := NormalizeTask(Task{
		Scope:    ScopeWorkspace,
		Name:     "Legacy schedule",
		Template: TemplateReview,
		Schedule: Schedule{Kind: ScheduleDaily, Hour: 10, Minute: 5},
	})
	if err != nil {
		t.Fatalf("NormalizeTask failed: %v", err)
	}
	if task.DefaultActionPolicy != ActionPolicyAutoRun {
		t.Fatalf("default action = %q, want auto_run", task.DefaultActionPolicy)
	}
	if len(task.Triggers) != 1 {
		t.Fatalf("trigger count = %d, want 1", len(task.Triggers))
	}
	trigger := task.Triggers[0]
	if trigger.Type != TriggerTypeSchedule || !trigger.Enabled {
		t.Fatalf("legacy trigger = %#v, want enabled schedule", trigger)
	}
	if trigger.ActionPolicy != "" || trigger.NotifyPolicy != NotifyPolicySilent {
		t.Fatalf("legacy trigger policy = %s/%s, want empty/silent", trigger.ActionPolicy, trigger.NotifyPolicy)
	}
}

func TestNormalizeTaskClearsLegacyTriggerActionAndUsesAutomaticRuns(t *testing.T) {
	task, err := NormalizeTask(Task{
		Scope:               ScopeWorkspace,
		Name:                "Saved legacy schedule",
		Template:            TemplateReview,
		DefaultActionPolicy: ActionPolicyConfirm,
		Triggers: []TriggerDefinition{{
			Type:         TriggerTypeSchedule,
			Enabled:      true,
			ActionPolicy: ActionPolicyAutoRun,
			NotifyPolicy: NotifyPolicySilent,
			Schedule:     Schedule{Kind: ScheduleDaily, Hour: 10, Minute: 5},
		}},
	})
	if err != nil {
		t.Fatalf("NormalizeTask failed: %v", err)
	}
	if task.DefaultActionPolicy != ActionPolicyAutoRun {
		t.Fatalf("default action = %q, want auto_run", task.DefaultActionPolicy)
	}
	if task.Triggers[0].ActionPolicy != "" {
		t.Fatalf("trigger action should be cleared, got %q", task.Triggers[0].ActionPolicy)
	}
}

func TestNormalizeTaskMigratesLegacyCharacterTriggerToSemantic(t *testing.T) {
	task, err := NormalizeTask(Task{
		Scope:    ScopeWorkspace,
		Name:     "Legacy semantic",
		Template: TemplateReview,
		Triggers: []TriggerDefinition{{
			Type:    "interactive_new_character",
			Enabled: true,
		}},
	})
	if err != nil {
		t.Fatalf("NormalizeTask failed: %v", err)
	}
	if len(task.Triggers) != 1 {
		t.Fatalf("trigger count = %d, want 1", len(task.Triggers))
	}
	trigger := task.Triggers[0]
	if trigger.Type != TriggerTypeSemantic || !strings.Contains(trigger.SemanticCondition, "important character") {
		t.Fatalf("legacy trigger not migrated to semantic: %#v", trigger)
	}
}

func TestEffectiveTriggerPolicies(t *testing.T) {
	task := Task{DefaultActionPolicy: ActionPolicyNotifyOnly}
	trigger := TriggerDefinition{Type: TriggerTypeSchedule, NotifyPolicy: NotifyPolicySilent}
	if got := EffectiveActionPolicy(task, trigger); got != ActionPolicyAutoRun {
		t.Fatalf("effective action = %q, want auto_run", got)
	}
	if got := EffectiveNotifyPolicy(task, trigger); got != NotifyPolicySilent {
		t.Fatalf("schedule notify = %q, want silent", got)
	}
	trigger.ActionPolicy = ActionPolicyConfirm
	if got := EffectiveActionPolicy(task, trigger); got != ActionPolicyAutoRun {
		t.Fatalf("trigger action override should be ignored, got %q", got)
	}
	task.DefaultActionPolicy = ActionPolicyConfirm
	if got := EffectiveNotifyPolicy(task, trigger); got != NotifyPolicySilent {
		t.Fatalf("task action metadata should not force inbox notify, got %q", got)
	}
}

func TestStoreInboxLifecycle(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "user"), filepath.Join(root, "workspace"))
	item, err := store.CreateInboxItem(TriggerInboxItem{
		TaskID:       "auto-1",
		TriggerID:    "schedule",
		Scope:        ScopeWorkspace,
		ActionPolicy: ActionPolicyConfirm,
		NotifyPolicy: NotifyPolicyInbox,
		Title:        "Review ready",
		Summary:      "A chapter is ready.",
		Fingerprint:  "fp-1",
	})
	if err != nil {
		t.Fatalf("CreateInboxItem failed: %v", err)
	}
	if item.ID == "" || item.Status != InboxStatusPending {
		t.Fatalf("unexpected item after create: %#v", item)
	}
	if _, ok, err := store.FindOpenInboxItem("auto-1", "schedule", "fp-1"); err != nil || !ok {
		t.Fatalf("FindOpenInboxItem ok=%v err=%v", ok, err)
	}
	read, err := store.MarkInboxItemRead(item.ID)
	if err != nil {
		t.Fatalf("MarkInboxItemRead failed: %v", err)
	}
	if read.ReadAt == nil {
		t.Fatalf("read_at should be set")
	}
	confirmed, err := store.ConfirmInboxItem(item.ID, "run-1")
	if err != nil {
		t.Fatalf("ConfirmInboxItem failed: %v", err)
	}
	if confirmed.Status != InboxStatusConfirmed || confirmed.RunID != "run-1" || confirmed.HandledAt == nil {
		t.Fatalf("unexpected confirmed item: %#v", confirmed)
	}
	if _, ok, err := store.FindOpenInboxItem("auto-1", "schedule", "fp-1"); err != nil || ok {
		t.Fatalf("confirmed item should not remain open ok=%v err=%v", ok, err)
	}
}

func TestStoreInboxSoftLimitNeverEvictsPendingObligations(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	store := NewStore(filepath.Join(root, "user"), workspace)
	oldest, err := store.CreateInboxItem(TriggerInboxItem{
		TaskID: "auto-old", TriggerID: "semantic", Scope: ScopeWorkspace, Workspace: workspace,
		Status: InboxStatusPending, ActionPolicy: ActionPolicyConfirm, NotifyPolicy: NotifyPolicyInbox,
		Title: "Old pending", Summary: "Must survive", Fingerprint: "old-pending",
	})
	if err != nil {
		t.Fatal(err)
	}
	expectedFingerprints := map[string]struct{}{oldest.Fingerprint: {}}
	// Persist a valid near-limit fixture in one write. Repeating more than one
	// hundred durable writes only benchmarks fsync; the six public creates below
	// still exercise the production read/bound/write path across the soft limit.
	seeded := make([]TriggerInboxItem, 0, MaxInboxItems)
	seeded = append(seeded, oldest)
	for index := 1; index < MaxInboxItems; index++ {
		item, normalizeErr := NormalizeInboxItem(TriggerInboxItem{
			TaskID: "auto-seed", TriggerID: "semantic", Scope: ScopeWorkspace, Workspace: workspace,
			Status: InboxStatusPending, ActionPolicy: ActionPolicyConfirm, NotifyPolicy: NotifyPolicyInbox,
			Title: "Seed pending", Summary: "Also actionable", Fingerprint: fmt.Sprintf("seed-pending-%03d", index),
		})
		if normalizeErr != nil {
			t.Fatalf("NormalizeInboxItem %d failed: %v", index, normalizeErr)
		}
		seeded = append(seeded, item)
		expectedFingerprints[item.Fingerprint] = struct{}{}
	}
	if err := store.writeInboxScope(ScopeWorkspace, seeded); err != nil {
		t.Fatalf("seed inbox projection: %v", err)
	}
	reloaded := NewStore(filepath.Join(root, "user"), workspace)
	for index := 0; index < 6; index++ {
		created, createErr := reloaded.CreateInboxItem(TriggerInboxItem{
			TaskID: "auto-new", TriggerID: "semantic", Scope: ScopeWorkspace, Workspace: workspace,
			Status: InboxStatusPending, ActionPolicy: ActionPolicyConfirm, NotifyPolicy: NotifyPolicyInbox,
			Title: "New pending", Summary: "Also actionable", Fingerprint: fmt.Sprintf("pending-%03d", index),
		})
		if createErr != nil {
			t.Fatalf("CreateInboxItem %d failed: %v", index, createErr)
		}
		expectedFingerprints[created.Fingerprint] = struct{}{}
	}
	items, err := reloaded.ListInbox()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != MaxInboxItems+6 {
		t.Fatalf("actionable inbox count = %d, want %d beyond soft limit", len(items), MaxInboxItems+6)
	}
	for _, item := range items {
		delete(expectedFingerprints, item.Fingerprint)
	}
	if len(expectedFingerprints) != 0 {
		t.Fatalf("pending obligations were evicted across soft limit: missing fingerprints=%v", expectedFingerprints)
	}
	if _, err := reloaded.GetInboxItem(oldest.ID); err != nil {
		t.Fatalf("GetInboxItem lost oldest pending obligation: %v", err)
	}
	confirmed, err := reloaded.ConfirmInboxItem(oldest.ID, "run-old-pending")
	if err != nil {
		t.Fatalf("ConfirmInboxItem lost oldest pending obligation: %v", err)
	}
	if confirmed.Status != InboxStatusConfirmed || confirmed.RunID != "run-old-pending" {
		t.Fatalf("confirmed item = %#v", confirmed)
	}
}

func TestTriggerContextBoundAndSemanticEvaluation(t *testing.T) {
	ctx := BoundedTriggerContext(TriggerContext{
		Source:  strings.Repeat("s", 300),
		Summary: strings.Repeat("一", 2000),
		Evidence: []TriggerEvidence{{
			Source:  "chapter",
			Title:   strings.Repeat("t", 500),
			Ref:     strings.Repeat("r", 500),
			Snippet: strings.Repeat("x", 2000),
		}},
	})
	if len([]rune(ctx.Source)) > 120 || len([]rune(ctx.Summary)) > 1000 {
		t.Fatalf("context source/summary not bounded: %#v", ctx)
	}
	if len(ctx.Evidence) != 1 || len([]rune(ctx.Evidence[0].Snippet)) > 1200 {
		t.Fatalf("evidence not bounded: %#v", ctx.Evidence)
	}
	eval, err := ParseSemanticEvaluation(`{"matched":true,"confidence":0.82,"reason":"ok","title":"Hit","evidence_refs":[" a ",""]}`)
	if err != nil {
		t.Fatalf("ParseSemanticEvaluation failed: %v", err)
	}
	if !eval.Matched || eval.Confidence != 0.82 || len(eval.EvidenceRefs) != 1 || eval.EvidenceRefs[0] != "a" {
		t.Fatalf("unexpected semantic eval: %#v", eval)
	}
}

func hasTask(tasks []Task, id string) bool {
	return taskByID(tasks, id) != nil
}

func taskByID(tasks []Task, id string) *Task {
	for i := range tasks {
		if tasks[i].ID == id {
			return &tasks[i]
		}
	}
	return nil
}
