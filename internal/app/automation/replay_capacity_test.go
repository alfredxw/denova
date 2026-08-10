package automationapp

import (
	"context"
	agentrun "denova/internal/agents/run"
	apptask "denova/internal/app/task"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"denova/config"
	"denova/internal/automation"
)

func TestAutomationRootReplayCapacityRejectsBeforeRunOrRuntimeAdmission(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	application, err := New(context.Background(), &config.Config{
		OpenAIModel: "test-model", NovaDir: dataDir, Workspace: workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	taskDef, err := application.CreateAutomation(automation.TaskDefinition{
		Scope: automation.ScopeWorkspace, Enabled: true, Name: "capacity root", Template: automation.TemplateReview,
	})
	if err != nil {
		t.Fatal(err)
	}

	blocker, err := apptask.NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	application.activeTaskReplay.Configure(apptask.ReplayAdmissionLimits{MaxBytes: blocker.DisplayReplayCharge()})
	reservation, err := application.activeTaskReplay.Reserve(blocker)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		reservation.Release()
		blocker.RejectStart(errors.New("test cleanup"))
	})

	const commandID = "automation-capacity-pre-admission"
	runID, err := automationManualRunID(taskDef.CatalogID, commandID)
	if err != nil {
		t.Fatal(err)
	}
	started, run, err := application.StartAutomationTaskCommand(context.Background(), taskDef.CatalogID, commandID, nil)
	if started != nil || run.ID != "" || !errors.Is(err, ErrAgentReplayCapacity) {
		t.Fatalf("capacity start = task=%v run=%#v err=%v, want empty pre-admission rejection", started, run, err)
	}
	if _, _, err := application.automation().storeAllWorkspaces().GetRunByID(runID); !errors.Is(err, automation.ErrRunNotFound) {
		t.Fatalf("capacity rejection persisted a run ledger entry: %v", err)
	}

	status, err := application.executionRuntime.RuntimeStatusProjection(context.Background(), agentrun.Options{
		AgentKind: agentrun.AgentKindAutomation, TaskID: runID, AutomationTaskID: taskDef.ID,
		SessionID: automationRunSessionID(runID), Workspace: taskDef.Target.Workspace, Mode: "automation",
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != agentrun.RunPhaseIdle || status.ActiveOperation != "" || status.LastOperation != nil || len(status.Queue) != 0 {
		t.Fatalf("Runtime was mutated before Automation replay admission: %#v", status)
	}
}
