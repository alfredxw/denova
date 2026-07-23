package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"

	"denova/internal/agentruntime"
)

func TestChatHarnessRunsLegacyRequestThroughDurableLane(t *testing.T) {
	service, err := newHarnessChatService(
		context.Background(),
		DefaultLoopPolicy(),
		agentruntime.NewMemoryJournalStore(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())

	conversation := &runControlConversation{}
	outcome := service.RunWithOptions(
		context.Background(),
		newRunControlTestRunner(t, &runControlFixedModel{message: schema.AssistantMessage("durable answer", nil)}, true),
		conversation,
		nil,
		ChatRequest{CommandID: "legacy-durable-lane", Message: "write"},
		RunOptions{AgentKind: AgentKindIDE, RootAgentName: "run-control-test", Workspace: "/book", SessionID: "session-1"},
		nil,
	)
	if outcome.Status != RunOutcomeCompleted || outcome.Content != "durable answer" {
		t.Fatalf("outcome = %#v", outcome)
	}

	binding, err := harnessBindingForOptions(RunOptions{AgentKind: AgentKindIDE, Workspace: "/book", SessionID: "session-1"})
	if err != nil {
		t.Fatal(err)
	}
	harness, err := service.harness.runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := harness.ObserveFromNow(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if observation.Snapshot.Phase != agentruntime.PhaseIdle || len(observation.Snapshot.Messages) != 2 {
		t.Fatalf("durable snapshot = %#v", observation.Snapshot)
	}
}

func TestChatHarnessCallerCancellationUsesTypedAbort(t *testing.T) {
	service, err := newHarnessChatService(
		context.Background(),
		DefaultLoopPolicy(),
		agentruntime.NewMemoryJournalStore(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())

	model := newRunControlTwoPhaseModel("partial", "thinking")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan RunOutcome, 1)
	runOutcomeTestGoroutine(done, "caller-cancellation run", func() RunOutcome {
		return service.RunWithOptions(
			ctx,
			newRunControlTwoPhaseRunner(t, model),
			&runControlConversation{},
			nil,
			ChatRequest{CommandID: "caller-cancellation", Message: "write"},
			RunOptions{AgentKind: AgentKindIDE, RootAgentName: "run-control-test", Workspace: "/book", SessionID: "session-abort"},
			nil,
		)
	})
	waitHarnessEngineSignal(t, model.blocked, "second model call")
	cancel()
	close(model.release)

	select {
	case outcome := <-done:
		if outcome.Status != RunOutcomeAborted || outcome.Error != nil {
			t.Fatalf("cancel outcome = %#v", outcome)
		}
	case <-time.After(time.Second):
		t.Fatal("typed abort did not settle")
	}
}

func TestChatHarnessStartReturnsDurableAcceptanceBeforeWait(t *testing.T) {
	service, err := newHarnessChatService(
		context.Background(),
		DefaultLoopPolicy(),
		agentruntime.NewMemoryJournalStore(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())

	model := newRunControlTwoPhaseModel("partial", "thinking")
	options := RunOptions{AgentKind: AgentKindIDE, RootAgentName: "run-control-test", Workspace: "/book", SessionID: "session-start-wait"}
	accepted, err := service.StartWithOptions(
		context.Background(),
		newRunControlTwoPhaseRunner(t, model),
		&runControlConversation{},
		nil,
		ChatRequest{CommandID: "start-before-wait", Message: "write"},
		options,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Receipt().OperationID == "" || accepted.Receipt().CommandID == "" {
		t.Fatalf("StartWithOptions returned without a durable receipt: %#v", accepted.Receipt())
	}
	waitHarnessEngineSignal(t, model.blocked, "second model call")
	projection, err := service.RuntimeStatusProjection(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if projection.ActiveOperation != accepted.Receipt().OperationID || projection.Phase != agentruntime.PhaseRunning {
		t.Fatalf("runtime projection was not active after acceptance: %#v receipt=%#v", projection, accepted.Receipt())
	}

	done := make(chan RunOutcome, 1)
	runOutcomeTestGoroutine(done, "accepted run wait", func() RunOutcome {
		return accepted.Wait(context.Background())
	})
	close(model.release)
	select {
	case outcome := <-done:
		if outcome.Status != RunOutcomeCompleted || !strings.Contains(outcome.Content, "partial") {
			t.Fatalf("wait outcome = %#v", outcome)
		}
	case <-time.After(time.Second):
		t.Fatal("accepted run did not settle")
	}
}

type postSettlementHarnessConversation struct {
	runControlConversation
	operation   *recordingContextStructuralOperation
	providerErr error
	calls       int
}

func (c *postSettlementHarnessConversation) PostSettlementContextStructuralSpec(
	_ context.Context,
	settledOperationID agentruntime.OperationID,
	options RunOptions,
) (*ContextStructuralSpec, error) {
	c.calls++
	if c.providerErr != nil {
		return nil, c.providerErr
	}
	binding, err := harnessBindingForOptions(options)
	if err != nil {
		return nil, err
	}
	bindingRef, err := agentruntime.BindingReference(binding)
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
	hash, err := ContextStructuralIntentHash(
		ContextStructuralCompact,
		bindingRef,
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
	plan := ContextStructuralRestorePlan{
		Version:    ContextStructuralRestorePlanVersion,
		Domain:     ContextStructuralDomainSession,
		Action:     ContextStructuralCompact,
		Commit:     true,
		IntentHash: hash,
		RecordID:   recordID,
		Result:     result,
		Mutation:   mutation,
	}
	return &ContextStructuralSpec{
		CommandID: "post-settlement-" + string(settledOperationID),
		Action:    ContextStructuralCompact,
		Ref: agentruntime.ContextCompactionRef{
			Source: "test.history", Purpose: "verify post-settlement ordering", Resource: options.SessionID,
			ExpectedRevision: "test-revision:1",
		},
		Options:   options,
		Operation: c.operation, RestorePlan: &plan,
	}, nil
}

func TestAcceptedRunCommitsPreparedCompactionBeforeReturning(t *testing.T) {
	service, err := newHarnessChatService(context.Background(), DefaultLoopPolicy(), agentruntime.NewMemoryJournalStore())
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())

	operation := &recordingContextStructuralOperation{
		result:  ContextStructuralResult{Compaction: ContextCompactionResult{Triggered: true, Epoch: 2}},
		receipt: ContextStructuralReceipt{Revision: "context:2"},
	}
	conversation := &postSettlementHarnessConversation{operation: operation}
	events := make([]Event, 0, 2)
	outcome := service.RunWithOptions(
		context.Background(),
		newRunControlTestRunner(t, &runControlFixedModel{message: schema.AssistantMessage("durable answer", nil)}, true),
		conversation,
		nil,
		ChatRequest{CommandID: "post-settlement-compaction", Message: "write"},
		RunOptions{AgentKind: AgentKindIDE, RootAgentName: "run-control-test", Workspace: "/book", SessionID: "session-post-settlement"},
		func(event Event) { events = append(events, event) },
	)
	if outcome.Status != RunOutcomeCompleted {
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

func TestAcceptedRunReportsCompactionFailureWithoutReversingCommittedTurn(t *testing.T) {
	service, err := newHarnessChatService(context.Background(), DefaultLoopPolicy(), agentruntime.NewMemoryJournalStore())
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close(context.Background())

	conversation := &postSettlementHarnessConversation{providerErr: errors.New("prepare checkpoint failed")}
	events := make([]Event, 0, 2)
	outcome := service.RunWithOptions(
		context.Background(),
		newRunControlTestRunner(t, &runControlFixedModel{message: schema.AssistantMessage("committed answer", nil)}, true),
		conversation,
		nil,
		ChatRequest{CommandID: "post-settlement-compaction-failure", Message: "write"},
		RunOptions{AgentKind: AgentKindIDE, RootAgentName: "run-control-test", Workspace: "/book", SessionID: "session-post-settlement-failure"},
		func(event Event) { events = append(events, event) },
	)
	if outcome.Status != RunOutcomeCompleted || outcome.Content != "committed answer" {
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

func TestHarnessBindingForProfiles(t *testing.T) {
	tests := []struct {
		name    string
		options RunOptions
		want    agentruntime.BindingRef
	}{
		{
			name:    "writing",
			options: RunOptions{AgentKind: AgentKindIDE, Workspace: "/book", SessionID: "s"},
			want:    agentruntime.BindingRef{Kind: agentruntime.BindingWriting, Profile: agentruntime.ProfileWriting, Workspace: "/book", SessionID: "s"},
		},
		{
			name:    "game",
			options: RunOptions{AgentKind: AgentKindInteractiveStory, Workspace: "/book", StoryID: "story", BranchID: "main"},
			want:    agentruntime.BindingRef{Kind: agentruntime.BindingGame, Profile: agentruntime.ProfileGame, Workspace: "/book", StoryID: "story", BranchID: "main"},
		},
		{
			name:    "automation_global",
			options: RunOptions{AgentKind: AgentKindAutomation, SessionID: "run", TaskID: "task"},
			want:    agentruntime.BindingRef{Kind: agentruntime.BindingAutomation, Profile: agentruntime.ProfileAutomation, SessionID: "run", TaskID: "task"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binding, err := harnessBindingForOptions(tt.options)
			if err != nil {
				t.Fatal(err)
			}
			if got := bindingRefForTest(binding); got != tt.want {
				t.Fatalf("binding = %#v, want %#v", got, tt.want)
			}
		})
	}

	if _, err := harnessBindingForOptions(RunOptions{AgentKind: AgentKindIDE, Workspace: "/book"}); !errors.Is(err, agentruntime.ErrInvalidBinding) {
		t.Fatalf("missing session error = %v", err)
	}
}

func bindingRefForTest(binding agentruntime.Binding) agentruntime.BindingRef {
	switch binding := binding.(type) {
	case agentruntime.WritingBinding:
		return agentruntime.BindingRef{Kind: agentruntime.BindingWriting, Profile: profileOr(binding.Profile, agentruntime.ProfileWriting), Workspace: binding.Workspace, SessionID: binding.SessionID, StoryID: binding.StoryID, BranchID: binding.BranchID}
	case agentruntime.GameBinding:
		return agentruntime.BindingRef{Kind: agentruntime.BindingGame, Profile: profileOr(binding.Profile, agentruntime.ProfileGame), Workspace: binding.Workspace, SessionID: binding.SessionID, StoryID: binding.StoryID, BranchID: binding.BranchID}
	case agentruntime.AutomationBinding:
		return agentruntime.BindingRef{Kind: agentruntime.BindingAutomation, Profile: profileOr(binding.Profile, agentruntime.ProfileAutomation), Workspace: binding.Workspace, SessionID: binding.SessionID, TaskID: binding.TaskID}
	default:
		return agentruntime.BindingRef{}
	}
}

func profileOr(profile, fallback agentruntime.AgentProfile) agentruntime.AgentProfile {
	if profile != "" {
		return profile
	}
	return fallback
}
