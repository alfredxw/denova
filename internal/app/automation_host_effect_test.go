package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"denova/config"
	agents "denova/internal/agents"
	"denova/internal/automation"
	runstate "github.com/alfredxw/denova/agent/runtime"
)

func TestAutomationHostEffectBridgeSurvivesRunMaterializationGap(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	workspace := filepath.Join(root, "workspace")
	application := &App{
		cfg: &config.Config{NovaDir: dataDir}, workspace: workspace,
		automationEffectWake: make(chan struct{}, 1),
	}
	application.ensureServices()
	store := automation.NewStore(dataDir, workspace)
	taskDef, err := store.Create(automation.Task{
		Scope: automation.ScopeWorkspace, Enabled: true, Name: "host effect owner", Template: automation.TemplateReview,
	})
	if err != nil {
		t.Fatal(err)
	}

	committed := agents.CommittedToolMutation{
		EffectID: "host-effect-materialization-gap", RuntimeOperation: "operation-1", RuntimeCycle: 1,
		ToolCallID: "tool-call-1",
		Origin: agents.ToolMutationOrigin{
			AgentKind: agents.AgentKindAutomation, TaskID: "run-materialization-gap",
			AutomationTaskID: taskDef.ID, Workspace: workspace, Mode: "automation",
		},
		Mutation: agents.ToolMutation{
			ToolName: "write", ToolCallID: "tool-call-1", Workspace: workspace, Target: "chapters/late.md",
		},
	}
	if err := application.reconcileHarnessHostEffect(context.Background(), committed); err != nil {
		t.Fatal(err)
	}
	globalStore := automation.NewStore(dataDir, "")
	obligations, err := globalStore.ListHostEffects()
	if err != nil || len(obligations) != 1 || obligations[0].ID != string(committed.EffectID) {
		t.Fatalf("materialization-gap obligation = %#v err=%v", obligations, err)
	}

	conflict := committed
	conflict.Mutation.Target = "chapters/conflict.md"
	if err := application.reconcileHarnessHostEffect(context.Background(), conflict); !errors.Is(err, automation.ErrHostEffectConflict) {
		t.Fatalf("same effect identity with different mutation error = %v, want ErrHostEffectConflict", err)
	}

	run := automation.RunRecord{
		ID: committed.Origin.TaskID, TaskID: taskDef.ID, Scope: taskDef.Scope, Workspace: workspace,
		Trigger: automation.TriggerManual, Status: automation.RunStatusSuccess,
		RuntimeCommandID: "automation-run:run-materialization-gap", RuntimeOperationID: string(committed.RuntimeOperation), RuntimeReceiptCursor: 1,
		CompletionEffectsOperationID: string(committed.RuntimeOperation), CompletionEffectsCompleted: true,
	}
	if _, err := store.AppendRun(taskDef.CatalogID, run); err != nil {
		t.Fatal(err)
	}
	transferred, err := application.automation().reconcilePersistedHostEffect(context.Background(), obligations[0])
	if err != nil || !transferred {
		t.Fatalf("transfer after run materialization transferred=%t err=%v", transferred, err)
	}
	if obligations, err = globalStore.ListHostEffects(); err != nil || len(obligations) != 0 {
		t.Fatalf("transferred generic outbox = %#v err=%v", obligations, err)
	}
	_, persisted, err := store.GetRunByID(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !persisted.CompletionEffectsPending || persisted.CompletionEffectsCompleted ||
		len(persisted.CompletionMutationPaths) != 1 || persisted.CompletionMutationPaths[0] != "chapters/late.md" ||
		len(persisted.CompletionMutationEffectIDs) != 1 || persisted.CompletionMutationEffectIDs[0] != string(committed.EffectID) {
		t.Fatalf("Automation run did not own the exact host effect: %#v", persisted)
	}

	// Crash after the application outbox transfers/acks but before Runtime
	// records its own ack: redelivery reuses the exact run receipt and clears the
	// generic obligation without duplicating the completion plan.
	if err := application.reconcileHarnessHostEffect(context.Background(), committed); err != nil {
		t.Fatal(err)
	}
	if obligations, err = globalStore.ListHostEffects(); err != nil || len(obligations) != 0 {
		t.Fatalf("exact redelivery remained pending: %#v err=%v", obligations, err)
	}
	_, replayed, err := store.GetRunByID(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(replayed.CompletionMutationPaths) != 1 || len(replayed.CompletionMutationEffectIDs) != 1 {
		t.Fatalf("exact redelivery duplicated run plan: %#v", replayed)
	}
}

func TestAutomationSuccessorFenceRejectsTransferFailureAndRecoversWithoutChangingOperation(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	application := &App{
		cfg: &config.Config{NovaDir: dataDir, Workspace: workspace}, workspace: workspace,
		automationEffectWake: make(chan struct{}, 1),
	}
	application.ensureServices()
	t.Cleanup(application.Close)
	service := application.automation()
	store := automation.NewStore(dataDir, workspace)
	taskDef, err := store.Create(automation.Task{
		Scope: automation.ScopeWorkspace, Enabled: true, Name: "failed run effect fence", Template: automation.TemplateReview,
	})
	if err != nil {
		t.Fatal(err)
	}
	run := automation.RunRecord{
		ID: "failed-run-effect-fence", TaskID: taskDef.ID, SessionID: automationRunSessionID("failed-run-effect-fence"),
		Scope: taskDef.Scope, Workspace: workspace, Trigger: automation.TriggerManual,
		RootRuntimeCommandID: automationRunAgentCommandID("failed-run-effect-fence"), RootRuntimeOperationID: "operation-1", RootRuntimeReceiptCursor: 1,
		RuntimeCommandID: automationRunAgentCommandID("failed-run-effect-fence"), RuntimeOperationID: "operation-1", RuntimeReceiptCursor: 1,
		Status: automation.RunStatusFailed, CompletionEffectsOperationID: "operation-1", CompletionEffectsCompleted: true,
	}
	if _, err := store.AppendRun(taskDef.CatalogID, run); err != nil {
		t.Fatal(err)
	}

	transferFailure := errors.New("injected durable transfer failure")
	service.hostEffectTransfer = func(context.Context, automation.HostEffectObligation, admittedToolMutationPayload) (bool, error) {
		return false, transferFailure
	}
	committed := agents.CommittedToolMutation{
		EffectID: "host-effect-before-failed-successor", RuntimeOperation: "operation-1", RuntimeCycle: 1,
		ToolCallID: "tool-call-before-failed-successor",
		Origin: agents.ToolMutationOrigin{
			AgentKind: agents.AgentKindAutomation, TaskID: run.ID, AutomationTaskID: taskDef.ID,
			SessionID: run.SessionID, Workspace: workspace, Mode: "automation",
		},
		Mutation: agents.ToolMutation{
			ToolName: "write", ToolCallID: "tool-call-before-failed-successor",
			Workspace: workspace, Target: "notes/not-a-chapter.md",
		},
	}
	if err := application.reconcileHarnessHostEffect(context.Background(), committed); err != nil {
		t.Fatal(err)
	}
	snap := &automationWorkspaceSnapshot{workspace: workspace, novaDir: dataDir}
	if _, _, err := service.fenceAutomationRunSuccessor(context.Background(), snap, taskDef, run); !errors.Is(err, transferFailure) {
		t.Fatalf("successor fence error = %v, want transfer failure", err)
	}
	_, blocked, err := store.GetRunByID(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if blocked.RuntimeOperationID != "operation-1" || blocked.PendingRuntimeCommandID != "" {
		t.Fatalf("failed fence advanced successor identity: %#v", blocked)
	}

	service.hostEffectTransfer = nil
	_, recovered, err := service.fenceAutomationRunSuccessor(context.Background(), snap, taskDef, blocked)
	if err != nil {
		t.Fatalf("recovered successor fence: %v", err)
	}
	if recovered.RuntimeOperationID != "operation-1" || recovered.PendingRuntimeCommandID != "" ||
		recovered.CompletionEffectsPending || !recovered.CompletionEffectsCompleted {
		t.Fatalf("recovered fence receipt = %#v", recovered)
	}
	obligations, err := automation.NewStore(dataDir, "").ListHostEffects()
	if err != nil || len(obligations) != 0 {
		t.Fatalf("global obligations after recovery = %#v err=%v", obligations, err)
	}
}

func TestCommittedAutomationHostEffectEnablesOneTriggerPassAcrossRedelivery(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	application := &App{
		cfg: &config.Config{NovaDir: dataDir, Workspace: workspace}, workspace: workspace,
		automationEffectWake: make(chan struct{}, 1),
	}
	application.ensureServices()
	t.Cleanup(application.Close)
	service := application.automation()
	store := automation.NewStore(dataDir, workspace)
	taskDef, err := store.Create(automation.Task{
		Scope: automation.ScopeWorkspace, Enabled: true, Name: "exact receipt trigger owner", Template: automation.TemplateReview,
	})
	if err != nil {
		t.Fatal(err)
	}
	run := automation.RunRecord{
		ID: "exact-receipt-trigger-run", TaskID: taskDef.ID, SessionID: automationRunSessionID("exact-receipt-trigger-run"),
		Scope: taskDef.Scope, Workspace: workspace, Trigger: automation.TriggerManual,
		RootRuntimeCommandID: automationRunAgentCommandID("exact-receipt-trigger-run"), RootRuntimeOperationID: "operation-1", RootRuntimeReceiptCursor: 1,
		RuntimeCommandID: automationRunAgentCommandID("exact-receipt-trigger-run"), RuntimeOperationID: "operation-1", RuntimeReceiptCursor: 1,
		Status: automation.RunStatusSuccess, CompletionEffectsOperationID: "operation-1", CompletionEffectsCompleted: true,
	}
	if _, err := store.AppendRun(taskDef.CatalogID, run); err != nil {
		t.Fatal(err)
	}

	committed := agents.CommittedToolMutation{
		EffectID: "host-effect-exact-output-receipt", RuntimeOperation: "operation-1", RuntimeCycle: 1,
		ToolCallID: "tool-call-exact-output-receipt",
		Origin: agents.ToolMutationOrigin{
			AgentKind: agents.AgentKindAutomation, TaskID: run.ID, AutomationTaskID: taskDef.ID,
			SessionID: run.SessionID, Workspace: workspace, Mode: "automation",
		},
		Mutation: agents.ToolMutation{
			ToolName: "write", ToolCallID: "tool-call-exact-output-receipt",
			Workspace: workspace, Target: "chapters/one.md",
		},
	}
	if err := application.reconcileHarnessHostEffect(context.Background(), committed); err != nil {
		t.Fatal(err)
	}
	_, admittedRun, err := store.GetRunByID(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan struct{}, 2)
	passes := 0
	application.automationTriggers.processOverride = func(context.Context, *AutomationAppService, *automationWorkspaceSnapshot, string) error {
		passes++
		return nil
	}
	application.automationTriggers.afterRun = func(string) { finished <- struct{}{} }
	snap := &automationWorkspaceSnapshot{workspace: workspace, novaDir: dataDir}
	if _, err := service.completeAutomationRunEffects(context.Background(), snap, taskDef, admittedRun); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for exact-receipt trigger pass")
	}
	_, completed, err := store.GetRunByID(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if passes != 1 || completed.CompletionEffectsPending || !completed.CompletionEffectsCompleted {
		t.Fatalf("exact-receipt trigger passes=%d run=%#v", passes, completed)
	}

	// Runtime may redeliver after the host admission succeeded but before its
	// own acknowledgement was durable. The exact EffectID must not reopen or
	// execute the already-receipted trigger plan.
	if err := application.reconcileHarnessHostEffect(context.Background(), committed); err != nil {
		t.Fatal(err)
	}
	if _, err := service.completeAutomationRunEffects(context.Background(), snap, taskDef, completed); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
		t.Fatal("exact HostEffect redelivery executed a second trigger pass")
	case <-time.After(50 * time.Millisecond):
	}
	if passes != 1 {
		t.Fatalf("exact HostEffect redelivery passes=%d, want 1", passes)
	}
}

func TestDecodeAdmittedToolMutationRejectsIncompleteIdentity(t *testing.T) {
	_, err := decodeAdmittedToolMutation(automation.HostEffectObligation{
		ID: "incomplete", Kind: string(runstate.HostEffectToolMutationCommitted),
		Payload: []byte(`{"version":1}`),
	})
	if err == nil {
		t.Fatal("incomplete admitted host effect was accepted")
	}
}
