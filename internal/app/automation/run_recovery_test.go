package automationapp

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	agentrun "denova/internal/agents/run"
	"denova/internal/automation"
)

// Exact accepted-input, terminal, successor, and abort recovery now belongs to
// the public Agent Session actor and is exercised through its production
// Interface. Automation tests retain only the product invariant here: an
// observation failure may not synthesize a terminal RunRecord while the public
// projection still proves that operation active.
func TestAutomationRecoveryFailureCannotFinalizeAnActiveProjection(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	novaDir := filepath.Join(root, "nova")
	store := automation.NewStore(novaDir, workspace)
	taskDef, err := store.Create(automation.TaskDefinition{Scope: automation.ScopeWorkspace, Name: "active recovery", Template: automation.TemplateReview})
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
	service.runtimeProjector = func(context.Context, *automationWorkspaceSnapshot, automation.Task, automation.RunRecord) (agentrun.RuntimeStatus, error) {
		return agentrun.RuntimeStatus{
			Cursor: 9, Phase: agentrun.RunPhaseRunning,
			ActiveCommandID: agentrun.CommandID(run.RuntimeCommandID), ActiveOperation: agentrun.OperationID(run.RuntimeOperationID),
			ActiveReceiptCursor: 1,
		}, nil
	}
	snapshot := &automationWorkspaceSnapshot{workspace: workspace, novaDir: novaDir}
	finalized, err := service.finalizeRecoveredAutomationRun(
		context.Background(), snapshot, taskDef, run,
		agentrun.Outcome{Status: agentrun.OutcomeFailed, Error: errors.New("observer failed")},
	)
	if err == nil {
		t.Fatal("failed observer finalized a still-active runtime projection")
	}
	if finalized.Status != automation.RunStatusRunning || !finalized.RuntimeRecoveryRequired || finalized.RuntimeReceiptCursor != 1 {
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
