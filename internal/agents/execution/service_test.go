package execution

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentcompaction "denova/internal/agents/context/compaction"
	agentstructural "denova/internal/agents/context/structural"
	agentrun "denova/internal/agents/run"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func TestChatExecutionRunsLegacyRequestThroughDurableLane(t *testing.T) {
	service, err := newRuntime(
		context.Background(),
		agentrun.DefaultLoopPolicy(),
		runstate.NewMemoryJournalStore(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())

	conversation := &runControlConversation{}
	outcome := runCycle(service,
		context.Background(),
		newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("durable answer", nil)}, true),
		conversation,
		nil,
		agentchat.ChatRequest{CommandID: "legacy-durable-lane", Message: "write"},
		agentrun.Options{AgentKind: agentrun.AgentKindIDE, RootAgentName: "run-control-test", Workspace: "/book", SessionID: "session-1"},
		nil,
	)
	if outcome.Status != agentrun.OutcomeCompleted || outcome.Content != "durable answer" {
		t.Fatalf("outcome = %#v", outcome)
	}

	binding, err := agentrun.BindingForOptions(agentrun.Options{AgentKind: agentrun.AgentKindIDE, Workspace: "/book", SessionID: "session-1"})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := service.coordinator.runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := harness.ObserveFromNow(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if observation.Snapshot.Phase != runstate.PhaseIdle || len(observation.Snapshot.Messages) != 2 {
		t.Fatalf("durable snapshot = %#v", observation.Snapshot)
	}
}

func TestChatExecutionCallerCancellationUsesTypedAbort(t *testing.T) {
	service, err := newRuntime(
		context.Background(),
		agentrun.DefaultLoopPolicy(),
		runstate.NewMemoryJournalStore(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())

	model := newRunControlTwoPhaseModel("partial", "thinking")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan agentrun.Outcome, 1)
	runOutcomeTestGoroutine(done, "caller-cancellation run", func() agentrun.Outcome {
		return runCycle(service,
			ctx,
			newRunControlTwoPhaseRunner(t, model),
			&runControlConversation{},
			nil,
			agentchat.ChatRequest{CommandID: "caller-cancellation", Message: "write"},
			agentrun.Options{AgentKind: agentrun.AgentKindIDE, RootAgentName: "run-control-test", Workspace: "/book", SessionID: "session-abort"},
			nil,
		)
	})
	waitEngineSignal(t, model.blocked, "second model call")
	cancel()
	close(model.release)

	select {
	case outcome := <-done:
		if outcome.Status != agentrun.OutcomeAborted || outcome.Error != nil {
			t.Fatalf("cancel outcome = %#v", outcome)
		}
	case <-time.After(time.Second):
		t.Fatal("typed abort did not settle")
	}
}

func TestChatExecutionStartReturnsDurableAcceptanceBeforeWait(t *testing.T) {
	service, err := newRuntime(
		context.Background(),
		agentrun.DefaultLoopPolicy(),
		runstate.NewMemoryJournalStore(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())

	model := newRunControlTwoPhaseModel("partial", "thinking")
	options := agentrun.Options{AgentKind: agentrun.AgentKindIDE, RootAgentName: "run-control-test", Workspace: "/book", SessionID: "session-start-wait"}
	accepted, err := startCycle(service,
		context.Background(),
		newRunControlTwoPhaseRunner(t, model),
		&runControlConversation{},
		nil,
		agentchat.ChatRequest{CommandID: "start-before-wait", Message: "write"},
		options,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Receipt().OperationID == "" || accepted.Receipt().CommandID == "" {
		t.Fatalf("Start returned without a durable receipt: %#v", accepted.Receipt())
	}
	waitEngineSignal(t, model.blocked, "second model call")
	projection, err := service.RuntimeStatusProjection(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if projection.ActiveOperation != accepted.Receipt().OperationID || projection.Phase != agentrun.RunPhaseRunning {
		t.Fatalf("runtime projection was not active after acceptance: %#v receipt=%#v", projection, accepted.Receipt())
	}

	done := make(chan agentrun.Outcome, 1)
	runOutcomeTestGoroutine(done, "accepted run wait", func() agentrun.Outcome {
		return accepted.Wait(context.Background())
	})
	close(model.release)
	select {
	case outcome := <-done:
		if outcome.Status != agentrun.OutcomeCompleted || !strings.Contains(outcome.Content, "partial") {
			t.Fatalf("wait outcome = %#v", outcome)
		}
	case <-time.After(time.Second):
		t.Fatal("accepted run did not settle")
	}
}

type postSettlementConversation struct {
	runControlConversation
	operation   *recordingContextStructuralOperation
	providerErr error
	calls       int
}

func (c *postSettlementConversation) PostSettlementContextStructuralSpec(
	_ context.Context,
	settledOperationID agentrun.OperationID,
	options agentrun.Options,
) (*agentstructural.Spec, error) {
	c.calls++
	if c.providerErr != nil {
		return nil, c.providerErr
	}
	binding, err := agentrun.BindingForOptions(options)
	if err != nil {
		return nil, err
	}
	bindingRef, err := runstate.BindingReference(binding)
	if err != nil {
		return nil, err
	}
	recordID := "cc-post-settlement-" + string(settledOperationID)
	mutation, err := json.Marshal(struct {
		ID string `json:"id"`
	}{ID: recordID})
	if err != nil {
		return nil, err
	}
	productBinding, err := agentrun.ParseRuntimeBinding(bindingRef)
	if err != nil {
		return nil, err
	}
	hash, err := agentstructural.IntentHash(
		agentstructural.Compact,
		productBinding,
		"test-revision:1",
		recordID,
		mutation,
	)
	if err != nil {
		return nil, err
	}
	c.operation.mu.Lock()
	c.operation.hash = hash
	result := c.operation.result
	c.operation.mu.Unlock()
	plan := agentstructural.RestorePlan{
		Version:    agentstructural.RestorePlanVersion,
		Domain:     agentstructural.DomainSession,
		Action:     agentstructural.Compact,
		Commit:     true,
		IntentHash: hash,
		RecordID:   recordID,
		Result:     result,
		Mutation:   mutation,
	}
	return &agentstructural.Spec{
		CommandID: "post-settlement-" + string(settledOperationID),
		Action:    agentstructural.Compact,
		Ref: agentrun.ContextCompactionRef{
			Source: "test.history", Purpose: "verify post-settlement ordering", Resource: options.SessionID,
			ExpectedRevision: "test-revision:1",
		},
		Options:   options,
		Operation: c.operation, RestorePlan: &plan,
	}, nil
}

func TestStartedCycleCommitsPreparedCompactionBeforeReturning(t *testing.T) {
	service, err := newRuntime(context.Background(), agentrun.DefaultLoopPolicy(), runstate.NewMemoryJournalStore())
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())

	operation := &recordingContextStructuralOperation{
		result:  agentstructural.Result{Compaction: agentcompaction.Result{Triggered: true, Epoch: 2}},
		receipt: agentstructural.Receipt{Revision: "context:2"},
	}
	conversation := &postSettlementConversation{operation: operation}
	events := make([]agentrun.Event, 0, 2)
	outcome := runCycle(service,
		context.Background(),
		newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("durable answer", nil)}, true),
		conversation,
		nil,
		agentchat.ChatRequest{CommandID: "post-settlement-compaction", Message: "write"},
		agentrun.Options{AgentKind: agentrun.AgentKindIDE, RootAgentName: "run-control-test", Workspace: "/book", SessionID: "session-post-settlement"},
		func(event agentrun.Event) { events = append(events, event) },
	)
	if outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("outcome = %#v", outcome)
	}
	operation.mu.Lock()
	prepared, committed := operation.prepared, operation.committed
	operation.mu.Unlock()
	if conversation.calls != 1 || prepared != 1 || committed != 1 {
		t.Fatalf("post-settlement calls/provider/prepare/commit = %d/%d/%d", conversation.calls, prepared, committed)
	}
	if len(events) == 0 || events[len(events)-1].Type != "done" {
		t.Fatalf("done must be emitted after structural settlement: %#v", events)
	}
}

func TestStartedCycleReportsCompactionFailureWithoutReversingCommittedTurn(t *testing.T) {
	service, err := newRuntime(context.Background(), agentrun.DefaultLoopPolicy(), runstate.NewMemoryJournalStore())
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())

	conversation := &postSettlementConversation{providerErr: errors.New("prepare checkpoint failed")}
	events := make([]agentrun.Event, 0, 2)
	outcome := runCycle(service,
		context.Background(),
		newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("committed answer", nil)}, true),
		conversation,
		nil,
		agentchat.ChatRequest{CommandID: "post-settlement-compaction-failure", Message: "write"},
		agentrun.Options{AgentKind: agentrun.AgentKindIDE, RootAgentName: "run-control-test", Workspace: "/book", SessionID: "session-post-settlement-failure"},
		func(event agentrun.Event) { events = append(events, event) },
	)
	if outcome.Status != agentrun.OutcomeCompleted || outcome.Content != "committed answer" {
		t.Fatalf("compaction failure reversed the committed turn: %#v", outcome)
	}
	if len(events) < 2 || events[len(events)-2].Type != "context_compaction" || events[len(events)-1].Type != "done" {
		t.Fatalf("expected compaction failure diagnostic before done: %#v", events)
	}
	payload, ok := events[len(events)-2].Data.(map[string]any)
	if !ok || payload["status"] != "failed" || !strings.Contains(payload["error"].(string), "prepare checkpoint failed") {
		t.Fatalf("compaction failure payload = %#v", events[len(events)-2].Data)
	}
}

func TestExecutionBindingForProfiles(t *testing.T) {
	tests := []struct {
		name    string
		options agentrun.Options
		want    agentrun.RuntimeBinding
	}{
		{
			name:    "writing",
			options: agentrun.Options{AgentKind: agentrun.AgentKindIDE, Workspace: "/book", SessionID: "s"},
			want:    agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindIDE, Workspace: "/book", SessionID: "s"},
		},
		{
			name:    "agent_chat",
			options: agentrun.Options{AgentKind: agentrun.AgentKindIDE, Mode: "agent_chat", Workspace: "/book", SessionID: "s"},
			want:    agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindIDE, Mode: "agent_chat", Workspace: "/book", SessionID: "s"},
		},
		{
			name:    "game",
			options: agentrun.Options{AgentKind: agentrun.AgentKindInteractiveStory, Workspace: "/book", StoryID: "story", BranchID: "main"},
			want:    agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindInteractiveStory, Workspace: "/book", StoryID: "story", BranchID: "main"},
		},
		{
			name:    "automation_global",
			options: agentrun.Options{AgentKind: agentrun.AgentKindAutomation, SessionID: "run", TaskID: "task"},
			want:    agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindAutomation, SessionID: "run", TaskID: "task"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binding, err := agentrun.BindingForOptions(tt.options)
			if err != nil {
				t.Fatal(err)
			}
			want, err := tt.want.Ref()
			if err != nil {
				t.Fatal(err)
			}
			if !binding.Equal(want) {
				t.Fatalf("binding = %#v, want %#v", binding, want)
			}
		})
	}

	if _, err := agentrun.BindingForOptions(agentrun.Options{AgentKind: agentrun.AgentKindIDE, Workspace: "/book"}); !errors.Is(err, runstate.ErrInvalidBinding) {
		t.Fatalf("missing session error = %v", err)
	}
}
