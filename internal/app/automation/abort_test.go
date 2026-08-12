package automationapp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"denova/config"
	agentrun "denova/internal/agents/run"
	"denova/internal/automation"

	agent "github.com/alfredxw/denova/agent"
	sessionfile "github.com/alfredxw/denova/agent/session/file"
)

type automationAbortBlockingModel struct{ started chan struct{} }

func (model *automationAbortBlockingModel) Generate(ctx context.Context, _ []*agent.Message, _ ...agent.ModelOption) (*agent.Message, error) {
	select {
	case <-model.started:
	default:
		close(model.started)
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func (model *automationAbortBlockingModel) Stream(ctx context.Context, messages []*agent.Message, options ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	message, err := model.Generate(ctx, messages, options...)
	if err != nil {
		return nil, err
	}
	return agent.StreamReaderFromArray([]*agent.Message{message}), nil
}

func TestAutomationAbortReplaysPersistedReceiptAfterRestart(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{NovaDir: root, Workspace: workspace, OpenAIModel: "test-model"}
	application, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	taskDef, err := application.CreateAutomation(automation.TaskDefinition{
		Scope: automation.ScopeWorkspace, Name: "abort replay", Template: automation.TemplateReview,
	})
	if err != nil {
		application.Close()
		t.Fatal(err)
	}
	const runID = "run-abort-replay"
	const commandID = "abort-command-replay"
	ref := automationRuntimeBindingForTest(application.Workspace(), automationRunSessionID(runID), runID, taskDef.Target.ProjectID)
	key, err := ref.AgentSessionKey()
	if err != nil {
		application.Close()
		t.Fatal(err)
	}
	publicStore, err := sessionfile.New(filepath.Join(root, "agent-sessions"))
	if err != nil {
		application.Close()
		t.Fatal(err)
	}
	model := &automationAbortBlockingModel{started: make(chan struct{})}
	owner, err := agent.New(context.Background(), agent.Definition{
		Name: "automation-replay", Model: model,
		ModelIdentity: agent.CapabilityIdentity{Kind: "test.automation.abort-replay", Version: 1},
	}, agent.WithSessionStore(publicStore))
	if err != nil {
		application.Close()
		t.Fatal(err)
	}
	session, err := owner.Session(context.Background(), key)
	if err != nil {
		application.Close()
		t.Fatal(err)
	}
	publicRun, err := session.Run(context.Background(), agent.Input{
		Text: "run automation", IdempotencyKey: automationRunAgentCommandID(runID),
	})
	if err != nil {
		application.Close()
		t.Fatal(err)
	}
	select {
	case <-model.started:
	case <-time.After(time.Second):
		application.Close()
		t.Fatal("public Automation Run did not reach its model")
	}
	abortReceipt, err := publicRun.Abort(context.Background(), agent.AbortRequest{Reason: "user_requested", IdempotencyKey: commandID})
	if err != nil {
		application.Close()
		t.Fatal(err)
	}
	if result, waitErr := publicRun.Wait(context.Background()); waitErr != nil || result.Status != agent.ResultAborted {
		application.Close()
		t.Fatalf("public Abort settlement=%#v error=%v", result, waitErr)
	}
	if err := session.Close(context.Background()); err != nil {
		application.Close()
		t.Fatal(err)
	}
	if err := owner.Close(context.Background()); err != nil {
		application.Close()
		t.Fatal(err)
	}
	run := automation.RunRecord{
		ID: runID, TaskID: taskDef.ID, SessionID: automationRunSessionID(runID),
		ProjectID: taskDef.Target.ProjectID,
		Scope:     taskDef.Scope, Workspace: application.Workspace(), Trigger: automation.TriggerManual,
		RuntimeCommandID: automationRunAgentCommandID(runID), RuntimeOperationID: publicRun.ID(),
		RuntimeReceiptCursor: uint64(publicRun.Receipt().Cursor), Status: automation.RunStatusAborted,
		StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC(),
	}
	if application.cfg.ProjectID != run.ProjectID {
		projects, _ := application.Projects(false)
		t.Fatalf("active Project identity = %q, automation target = %q, projects=%#v", application.cfg.ProjectID, run.ProjectID, projects)
	}
	if _, err := application.automation().storeAllWorkspaces().AppendRun(automationTaskStoreID(taskDef), run); err != nil {
		application.Close()
		t.Fatal(err)
	}

	application.Close()

	reopened, err := New(context.Background(), &config.Config{NovaDir: root, Workspace: workspace, OpenAIModel: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Close)
	if reopened.cfg.ProjectID != run.ProjectID {
		t.Fatalf("reopened Project identity = %q, want %q", reopened.cfg.ProjectID, run.ProjectID)
	}
	reopenedRef := automationRuntimeBindingForTest(run.Workspace, run.SessionID, run.ID, reopened.cfg.ProjectID)
	if reopenedRef != ref {
		t.Fatalf("reopened runtime binding = %#v, seeded %#v", reopenedRef, ref)
	}
	receipt, err := reopened.AbortAutomationRunCommand(context.Background(), runID, commandID, agentrun.OperationID(publicRun.ID()), "user_requested")
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Replayed || receipt.CommandID != agentrun.CommandID(commandID) ||
		receipt.OperationID != agentrun.OperationID(publicRun.ID()) || receipt.Cursor != agentrun.Cursor(abortReceipt.Cursor) {
		t.Fatalf("replayed abort receipt = %#v", receipt)
	}
}
