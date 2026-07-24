package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"denova/config"
	agents "denova/internal/agents"
	"denova/internal/automation"
	runstate "github.com/alfredxw/denova/agent/runtime"
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
	taskDef, err := application.CreateAutomation(automation.Task{
		Scope: automation.ScopeWorkspace, Enabled: true, Name: "capacity root", Template: automation.TemplateReview,
	})
	if err != nil {
		t.Fatal(err)
	}

	blocker, err := NewDeferredRegisteredTask(nil)
	if err != nil {
		t.Fatal(err)
	}
	application.activeTaskReplay.byteLimit = blocker.displayReplayRegistryCharge()
	reservation, err := application.activeTaskReplay.reserve(blocker)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		reservation.release()
		blocker.failBeforeStart(errors.New("test cleanup"))
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

	status, err := application.chatService.RuntimeStatusProjection(context.Background(), agents.RunOptions{
		AgentKind: agents.AgentKindAutomation, TaskID: runID, AutomationTaskID: taskDef.ID,
		SessionID: automationRunSessionID(runID), Workspace: taskDef.Target.Workspace, Mode: "automation",
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != runstate.PhaseIdle || status.ActiveOperation != "" || status.LastOperation != nil || len(status.Queue) != 0 {
		t.Fatalf("Runtime was mutated before Automation replay admission: %#v", status)
	}
}
