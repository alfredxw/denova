package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	agentsession "github.com/alfredxw/denova/agent/session"
	sessionfile "github.com/alfredxw/denova/agent/session/file"
)

type persistentMemoryStore struct{ agentsession.Store }

type fixedCompactionManager struct{}

func (fixedCompactionManager) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "compaction.fixed-test", Version: 1}
}

func (fixedCompactionManager) Plan(_ context.Context, request CompactionPlanRequest) (CompactionPlan, error) {
	if !request.Force || len(request.Messages) < 2 {
		return CompactionPlan{Action: CompactionNone}, nil
	}
	return CompactionPlan{Action: CompactionCreate, SourceFrom: 0, SourceTo: 2}, nil
}

func (fixedCompactionManager) Compact(context.Context, CompactionCompactRequest) (CompactionCheckpoint, error) {
	return CompactionCheckpoint{Summary: "summary of the first turn", TokenEstimate: 6}, nil
}

type contextDataCompactionManager struct{}

func (contextDataCompactionManager) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "compaction.context-data-test", Version: 1}
}

func (contextDataCompactionManager) Plan(_ context.Context, request CompactionPlanRequest) (CompactionPlan, error) {
	if !request.Force || len(request.Messages) < 2 {
		return CompactionPlan{Action: CompactionNone}, nil
	}
	return CompactionPlan{Action: CompactionCreate, SourceFrom: 0, SourceTo: 2}, nil
}

func (contextDataCompactionManager) Compact(context.Context, CompactionCompactRequest) (CompactionCheckpoint, error) {
	return CompactionCheckpoint{
		Summary: "summary with host cursor", TokenEstimate: 8,
		ContextData: &HostData{Type: "test.host-context", Version: 1, Data: []byte(`{"cursor":2}`)},
	}, nil
}

type recordingContextSource struct {
	mu     sync.Mutex
	states []*CompactionState
}

type automaticContextDataCompactionManager struct {
	mu                 sync.Mutex
	sawFullHostContext bool
}

func (*automaticContextDataCompactionManager) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "compaction.same-cycle-host-context-test", Version: 1}
}

func (*automaticContextDataCompactionManager) Plan(_ context.Context, request CompactionPlanRequest) (CompactionPlan, error) {
	if len(request.Messages) < 2 {
		return CompactionPlan{Action: CompactionNone}, nil
	}
	return CompactionPlan{Action: CompactionCreate, SourceFrom: 0, SourceTo: 2}, nil
}

func (manager *automaticContextDataCompactionManager) Compact(_ context.Context, request CompactionCompactRequest) (CompactionCheckpoint, error) {
	manager.mu.Lock()
	for _, message := range request.ModelRequest {
		if message != nil && strings.Contains(message.Content, "FULL_HOST_CONTEXT") {
			manager.sawFullHostContext = true
		}
	}
	manager.mu.Unlock()
	return CompactionCheckpoint{
		Summary: "automatic host summary", TokenEstimate: 5,
		ContextData: &HostData{Type: "test.host-context", Version: 1, Data: []byte(`{"cursor":2}`)},
	}, nil
}

func (manager *automaticContextDataCompactionManager) sawFullContext() bool {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.sawFullHostContext
}

type projectingContextSource struct{}

func (projectingContextSource) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "context.same-cycle-host-context-test", Version: 1}
}

func (projectingContextSource) Materialize(_ context.Context, request ContextRequest) ([]ContextFragment, error) {
	content := "FULL_HOST_CONTEXT"
	revision := "full"
	if request.Compaction != nil && request.Compaction.ContextData != nil {
		content = "BOUNDED_HOST_CONTEXT"
		revision = request.Compaction.ID
	}
	return []ContextFragment{{
		Source: "test.host", Purpose: "verify same-cycle host projection", Resource: "host-state",
		Revision: revision, Placement: ContextLeadingMessage, Content: content, HardLimit: 64 << 10,
	}}, nil
}

func (*recordingContextSource) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "context.compaction-observer-test", Version: 1}
}

func (source *recordingContextSource) Materialize(_ context.Context, request ContextRequest) ([]ContextFragment, error) {
	source.mu.Lock()
	source.states = append(source.states, cloneCompactionState(request.Compaction))
	source.mu.Unlock()
	return nil, nil
}

func (source *recordingContextSource) sawContextData(data string) bool {
	source.mu.Lock()
	defer source.mu.Unlock()
	for _, state := range source.states {
		if state != nil && state.ContextData != nil && string(state.ContextData.Data) == data {
			return true
		}
	}
	return false
}

type lifecycleModel struct {
	mu        sync.Mutex
	responses []*Message
	inputs    [][]*Message
}

func TestPermissionInteractionIsDurableBeforeConcreteToolStarts(t *testing.T) {
	var executed atomic.Bool
	tool, err := InferTool("mutate", "mutate a protected resource", func(context.Context, struct{}) (string, error) {
		executed.Store(true)
		return "changed", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	toolset := StaticToolsIdentified(CapabilityIdentity{Kind: "tools.permission-test", Version: 1}, ToolDefinition{
		Tool: tool,
		Descriptor: ToolDescriptor{
			Source: ToolSourceWrite, Execution: ToolExecutionWorkspaceExclusive,
			MutationScope: ToolMutationWorkspace, PostCheck: ToolPostCheckWorkspaceChange,
			Recovery: ToolRecoveryIdempotent, ResultProjection: ToolResultBoundedModelContext,
			ResultRetention: ToolResultProtected, Steering: SteeringFinishCurrent, MaxResultBytes: 64 << 10,
		},
	})
	model := &lifecycleModel{responses: []*Message{
		AssistantMessage("", []ToolCall{{ID: "provider-call", Type: "function", Function: FunctionCall{Name: "mutate", Arguments: `{}`}}}),
		AssistantMessage("done", nil),
	}}
	owner, err := New(context.Background(), Definition{Model: model, Tools: toolset})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	run, err := owner.Run(context.Background(), Text("change it"))
	if err != nil {
		t.Fatal(err)
	}
	var request InteractionRequest
	for event := range run.Events() {
		if interaction, ok := event.Payload.(InteractionRequested); ok {
			request = interaction.Request
			break
		}
	}
	if request.Kind != InteractionPermission || request.Permission == nil {
		t.Fatalf("permission request = %#v", request)
	}
	if executed.Load() {
		t.Fatal("concrete tool started before permission resolution")
	}
	if err := run.Respond(context.Background(), request.ID, InteractionResponse{Permission: PermissionAllowOnce}); err != nil {
		t.Fatal(err)
	}
	if result, err := run.Wait(context.Background()); err != nil || result.Status != ResultCompleted {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if !executed.Load() {
		t.Fatal("allowed concrete tool did not run")
	}
}

func TestNestedAgentEventsRemainDisplayOnlyAndToolEventsRetainLiveDetails(t *testing.T) {
	tool, err := InferTool("delegate", "delegate work", func(ctx context.Context, input struct {
		Prompt string `json:"prompt"`
	}) (ToolResult, error) {
		EmitEvent(ctx, &AgentEvent{
			AgentName: "child",
			RunPath:   []RunStep{NewRunStep("root"), NewRunStep("child")},
			Output: &AgentOutput{MessageOutput: &MessageVariant{
				Message: AssistantMessage("child display", nil), Role: Assistant,
			}},
		})
		return TextToolResult("delegated result"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	definition := ToolDefinition{Tool: tool, Descriptor: ToolDescriptor{
		Source: ToolSourceOther, Execution: ToolExecutionChild,
		MutationScope: ToolMutationNone, PostCheck: ToolPostCheckNone,
		Recovery: ToolRecoveryReadOnly, ResultProjection: ToolResultBoundedModelContext,
		ResultRetention: ToolResultProtected, Steering: SteeringFinishCurrent, MaxResultBytes: 64 << 10,
	}}
	model := &lifecycleModel{responses: []*Message{
		AssistantMessage("", []ToolCall{{
			ID: "delegate-call", Type: "function",
			Function: FunctionCall{Name: "delegate", Arguments: `{"prompt":"research"}`},
		}}),
		AssistantMessage("root final", nil),
	}}
	owner, err := New(context.Background(), Definition{
		Name: "root", Model: model,
		Tools: StaticToolsIdentified(CapabilityIdentity{Kind: "tools.nested-display-test", Version: 1}, definition),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	run, err := owner.Run(context.Background(), Text("delegate this"))
	if err != nil {
		t.Fatal(err)
	}
	if result, err := run.Wait(context.Background()); err != nil || result.Status != ResultCompleted {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	var nested AssistantDelta
	var started ToolStarted
	var finished ToolFinished
	for event := range run.Events() {
		switch payload := event.Payload.(type) {
		case AssistantDelta:
			if payload.Source.Name == "child" {
				nested = payload
			}
		case ToolStarted:
			started = payload
		case ToolFinished:
			finished = payload
		}
	}
	if nested.Delta != "child display" || !nested.DisplayOnly || len(nested.Source.Path) != 2 {
		t.Fatalf("nested delta=%#v", nested)
	}
	if started.Name != "delegate" || string(started.Arguments) != `{"prompt":"research"}` {
		t.Fatalf("tool start=%#v", started)
	}
	if finished.Name != "delegate" || finished.Result != "delegated result" || finished.IsError {
		t.Fatalf("tool finish=%#v", finished)
	}
	calls := model.calls()
	if len(calls) != 2 || len(calls[1]) != 3 {
		t.Fatalf("model transcript=%#v", calls)
	}
	for _, message := range calls[1] {
		if message != nil && message.Content == "child display" {
			t.Fatalf("nested display message leaked into root transcript: %#v", calls[1])
		}
	}
}

func TestToolCanRequestSuccessfulCompletionAtTheCompletedBatchBoundary(t *testing.T) {
	tool, err := InferTool("submit", "submit the completed result", func(ctx context.Context, _ struct{}) (string, error) {
		if !RequestCompletionAfterTools(ctx) {
			return "", errors.New("run completion controller is unavailable")
		}
		return "submitted", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &lifecycleModel{responses: []*Message{
		AssistantMessage("final narrative", []ToolCall{{
			ID: "submit-call", Type: "function", Function: FunctionCall{Name: "submit", Arguments: `{}`},
		}}),
	}}
	owner, err := New(context.Background(), Definition{
		Name: "submitter", Model: model,
		Tools: StaticToolsIdentified(CapabilityIdentity{Kind: "tools.completion-test", Version: 1}, ToolDefinition{
			Tool: tool,
			Descriptor: ToolDescriptor{
				Source: ToolSourceOther, Execution: ToolExecutionSessionExclusive,
				MutationScope: ToolMutationSession, PostCheck: ToolPostCheckSessionState,
				Recovery: ToolRecoveryIdempotent, ResultProjection: ToolResultBoundedModelContext,
				ResultRetention: ToolResultProtected, Steering: SteeringFinishCurrent, MaxResultBytes: 64 << 10,
			},
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	run, err := owner.Run(context.Background(), Text("finish"))
	if err != nil {
		t.Fatal(err)
	}
	if result, err := run.Wait(context.Background()); err != nil || result.Status != ResultCompleted {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	var final AssistantFinal
	for event := range run.Events() {
		if payload, ok := event.Payload.(AssistantFinal); ok {
			final = payload
		}
	}
	if final.Content != "final narrative" {
		t.Fatalf("final assistant = %#v", final)
	}
}

type gatedLifecycleModel struct {
	calls     chan []*Message
	responses chan *Message
}

func newGatedLifecycleModel() *gatedLifecycleModel {
	return &gatedLifecycleModel{calls: make(chan []*Message, 4), responses: make(chan *Message, 4)}
}

func (model *gatedLifecycleModel) Generate(ctx context.Context, input []*Message, _ ...ModelOption) (*Message, error) {
	return model.next(ctx, input)
}

func (model *gatedLifecycleModel) Stream(ctx context.Context, input []*Message, _ ...ModelOption) (*StreamReader[*Message], error) {
	message, err := model.next(ctx, input)
	if err != nil {
		return nil, err
	}
	return StreamReaderFromArray([]*Message{message}), nil
}

func (model *gatedLifecycleModel) next(ctx context.Context, input []*Message) (*Message, error) {
	select {
	case model.calls <- cloneMessages(input):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case response := <-model.responses:
		return CloneMessage(response), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (model *lifecycleModel) Generate(_ context.Context, input []*Message, _ ...ModelOption) (*Message, error) {
	return model.next(input)
}

func (model *lifecycleModel) Stream(_ context.Context, input []*Message, _ ...ModelOption) (*StreamReader[*Message], error) {
	message, err := model.next(input)
	if err != nil {
		return nil, err
	}
	return StreamReaderFromArray([]*Message{message}), nil
}

func (model *lifecycleModel) next(input []*Message) (*Message, error) {
	model.mu.Lock()
	defer model.mu.Unlock()
	model.inputs = append(model.inputs, cloneMessages(input))
	if len(model.responses) == 0 {
		return nil, errors.New("lifecycle model exhausted")
	}
	message := CloneMessage(model.responses[0])
	model.responses = model.responses[1:]
	return message, nil
}

func (model *lifecycleModel) calls() [][]*Message {
	model.mu.Lock()
	defer model.mu.Unlock()
	result := make([][]*Message, len(model.inputs))
	for index := range model.inputs {
		result[index] = cloneMessages(model.inputs[index])
	}
	return result
}

func TestAgentNamedSessionRetainsProviderNeutralTranscript(t *testing.T) {
	model := &lifecycleModel{responses: []*Message{
		AssistantMessage("first answer", nil),
		AssistantMessage("second answer", nil),
	}}
	owner, err := New(context.Background(), Definition{
		Key: "test-agent", Name: "test", Instructions: "answer exactly", Model: model,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })

	session, err := owner.Session(context.Background(), NamedSession("main"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := session.Run(context.Background(), Input{Text: "first question", IdempotencyKey: "first-command"})
	if err != nil {
		t.Fatal(err)
	}
	firstReceipt := first.Receipt()
	if firstReceipt.CommandID != "first-command" || firstReceipt.RunID != first.ID() || firstReceipt.Cursor == 0 || firstReceipt.Replayed {
		t.Fatalf("first durable receipt = %#v", firstReceipt)
	}
	firstResult, err := first.Wait(context.Background())
	if err != nil || firstResult.Status != ResultCompleted {
		t.Fatalf("first result = %#v, err = %v", firstResult, err)
	}

	// An exact command replay returns the already-settled public Run without a
	// second model call.
	replayed, err := session.Run(context.Background(), Input{Text: "first question", IdempotencyKey: "first-command"})
	if err != nil {
		t.Fatal(err)
	}
	replayedReceipt := replayed.Receipt()
	if replayedReceipt.CommandID != firstReceipt.CommandID || replayedReceipt.RunID != firstReceipt.RunID ||
		replayedReceipt.Cursor != firstReceipt.Cursor || !replayedReceipt.Replayed {
		t.Fatalf("replayed durable receipt = %#v, want %#v", replayedReceipt, firstReceipt)
	}
	replayedResult, err := replayed.Wait(context.Background())
	if err != nil || replayedResult.Status != ResultCompleted || replayed.ID() != first.ID() {
		t.Fatalf("replayed result = %#v id=%q, err = %v", replayedResult, replayed.ID(), err)
	}

	second, err := session.Run(context.Background(), Input{Text: "second question", IdempotencyKey: "second-command"})
	if err != nil {
		t.Fatal(err)
	}
	secondResult, err := second.Wait(context.Background())
	if err != nil || secondResult.Status != ResultCompleted {
		t.Fatalf("second result = %#v, err = %v", secondResult, err)
	}

	calls := model.calls()
	if len(calls) != 2 {
		t.Fatalf("model calls = %d, want 2", len(calls))
	}
	got := calls[1]
	if len(got) != 4 || got[0].Role != System || got[0].Content != "answer exactly" ||
		got[1].Role != User || got[1].Content != "first question" ||
		got[2].Role != Assistant || got[2].Content != "first answer" ||
		got[3].Role != User || got[3].Content != "second question" {
		t.Fatalf("second model transcript = %#v", got)
	}
}

func TestSessionCompactionRemoveAndClearPreserveRawHistorySemantics(t *testing.T) {
	model := &lifecycleModel{responses: []*Message{
		AssistantMessage("answer one", nil), AssistantMessage("answer two", nil),
		AssistantMessage("answer three", nil), AssistantMessage("answer four", nil),
		AssistantMessage("answer five", nil),
	}}
	owner, err := New(context.Background(), Definition{Model: model, Compaction: fixedCompactionManager{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("compaction"))
	if err != nil {
		t.Fatal(err)
	}
	for index, text := range []string{"question one", "question two"} {
		run, runErr := session.Run(context.Background(), Input{Text: text, IdempotencyKey: fmt.Sprintf("before-%d", index)})
		if runErr != nil {
			t.Fatal(runErr)
		}
		if _, waitErr := run.Wait(context.Background()); waitErr != nil {
			t.Fatal(waitErr)
		}
	}
	compacted, err := session.Compact(context.Background(), CompactionRequest{Force: true, IdempotencyKey: "compact-test"})
	if err != nil || !compacted.Changed || compacted.State.Summary != "summary of the first turn" {
		t.Fatalf("compact result=%#v err=%v", compacted, err)
	}
	third, err := session.Run(context.Background(), Input{Text: "question three", IdempotencyKey: "after-compact"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := third.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	calls := model.calls()
	thirdInput := calls[2]
	if len(thirdInput) != 4 || thirdInput[0].Role != System || !strings.Contains(thirdInput[0].Content, "summary of the first turn") ||
		thirdInput[1].Content != "question two" || thirdInput[2].Content != "answer two" || thirdInput[3].Content != "question three" {
		t.Fatalf("compacted model input=%#v", thirdInput)
	}
	removed, err := session.RemoveCompaction(context.Background(), CompactionRemoveRequest{
		ID: compacted.State.ID, ExpectedRevision: compacted.State.Revision, IdempotencyKey: "remove-test",
	})
	if err != nil || !removed {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	fourth, err := session.Run(context.Background(), Input{Text: "question four", IdempotencyKey: "after-remove"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fourth.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	calls = model.calls()
	if len(calls[3]) != 7 || calls[3][0].Content != "question one" || calls[3][6].Content != "question four" {
		t.Fatalf("restored raw history=%#v", calls[3])
	}
	if err := session.Clear(context.Background()); err != nil {
		t.Fatal(err)
	}
	fifth, err := session.Run(context.Background(), Input{Text: "question five", IdempotencyKey: "after-clear"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fifth.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	calls = model.calls()
	if len(calls[4]) != 1 || calls[4][0].Content != "question five" {
		t.Fatalf("cleared model history=%#v", calls[4])
	}
}

func TestAutomaticCompactionRematerializesHostContextBeforeSameCycleModelCall(t *testing.T) {
	model := &lifecycleModel{responses: []*Message{
		AssistantMessage("answer one", nil), AssistantMessage("answer two", nil),
	}}
	manager := &automaticContextDataCompactionManager{}
	owner, err := New(context.Background(), Definition{
		Model: model, Context: projectingContextSource{}, Compaction: manager,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("same-cycle-host-compaction"))
	if err != nil {
		t.Fatal(err)
	}
	for index, text := range []string{"question one", "question two"} {
		run, runErr := session.Run(context.Background(), Input{
			Text: text, IdempotencyKey: fmt.Sprintf("same-cycle-%d", index),
		})
		if runErr != nil {
			t.Fatal(runErr)
		}
		if _, waitErr := run.Wait(context.Background()); waitErr != nil {
			t.Fatal(waitErr)
		}
	}
	if !manager.sawFullContext() {
		t.Fatal("Compaction manager did not summarize the exact pre-compaction host context")
	}
	calls := model.calls()
	if len(calls) != 2 {
		t.Fatalf("model calls=%d", len(calls))
	}
	var second strings.Builder
	for _, message := range calls[1] {
		if message != nil {
			second.WriteString(message.Content)
			second.WriteByte('\n')
		}
	}
	if !strings.Contains(second.String(), "BOUNDED_HOST_CONTEXT") || strings.Contains(second.String(), "FULL_HOST_CONTEXT") {
		t.Fatalf("same-cycle model context was not rematerialized: %#v", calls[1])
	}
}

func TestAgentRunOwnsAndClosesTemporarySession(t *testing.T) {
	model := &lifecycleModel{responses: []*Message{AssistantMessage("done", nil)}}
	owner, err := New(context.Background(), Definition{Model: model})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })

	run, err := owner.Run(context.Background(), Text("work"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.Wait(context.Background())
	if err != nil || result.Status != ResultCompleted {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
	if err := run.session.usable(); !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("temporary Session remained usable: %v", err)
	}
}

func TestDurableAgentRejectsUnidentifiedCapabilities(t *testing.T) {
	model := &lifecycleModel{responses: []*Message{AssistantMessage("unused", nil)}}
	store := &persistentMemoryStore{Store: agentsession.Memory()}
	owner, err := New(context.Background(), Definition{Model: model}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("durable"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), Text("work"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.Wait(context.Background()); err == nil {
		t.Fatal("durable Run accepted a Model without stable identity")
	}
}

func TestDurableAgentRequiresExplicitRetryIdentity(t *testing.T) {
	model := &lifecycleModel{responses: []*Message{AssistantMessage("unused", nil)}}
	store := &persistentMemoryStore{Store: agentsession.Memory()}
	owner, err := New(context.Background(), Definition{
		Model: model, ModelIdentity: CapabilityIdentity{Kind: "model.retry-test", Version: 1},
		Execution: ExecutionPolicy{Retry: &RetryConfig{MaxRetries: 1}},
	}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("retry-without-identity"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), Text("work"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.Wait(context.Background()); err == nil || !strings.Contains(err.Error(), "Retry capability identity is incomplete") {
		t.Fatalf("durable retry error = %v", err)
	}
}

func TestDurableAgentAcceptsStableRetryIdentityWithoutHashingClosures(t *testing.T) {
	model := &lifecycleModel{responses: []*Message{AssistantMessage("done", nil)}}
	store := &persistentMemoryStore{Store: agentsession.Memory()}
	owner, err := New(context.Background(), Definition{
		Model: model, ModelIdentity: CapabilityIdentity{Kind: "model.retry-test", Version: 1},
		Execution: ExecutionPolicy{
			Retry: &RetryConfig{
				MaxRetries:  1,
				IsRetryable: func(context.Context, error) bool { return false },
			},
			RetryIdentity: CapabilityIdentity{Kind: "retry.denova-test", Version: 1, ConfigHash: "one-attempt"},
		},
	}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("retry-with-identity"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), Text("work"))
	if err != nil {
		t.Fatal(err)
	}
	if result, err := run.Wait(context.Background()); err != nil || result.Status != ResultCompleted {
		t.Fatalf("result=%#v error=%v", result, err)
	}
}

func TestSessionSnapshotReturnsCurrentProjectionWithoutSubscription(t *testing.T) {
	model := &lifecycleModel{responses: []*Message{AssistantMessage("done", nil)}}
	owner, err := New(context.Background(), Definition{Model: model})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("snapshot"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), Input{Text: "work", IdempotencyKey: "snapshot-run"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	snapshot, err := session.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Key.ID != "snapshot" || snapshot.Cursor == 0 || snapshot.ActiveRunID != "" || len(snapshot.RecentRuns) == 0 {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	latest := snapshot.RecentRuns[len(snapshot.RecentRuns)-1]
	if latest.ID != run.ID() || latest.Status != ResultCompleted {
		t.Fatalf("latest run=%#v, run id=%q", latest, run.ID())
	}
}

func TestAgentRestoresNamedSessionFromFileStore(t *testing.T) {
	store, err := sessionfile.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	identity := CapabilityIdentity{Kind: "model.lifecycle-test", Version: 1, ConfigHash: "stable"}
	firstModel := &lifecycleModel{responses: []*Message{AssistantMessage("persisted answer", nil)}}
	firstOwner, err := New(context.Background(), Definition{
		Key: "durable-test", Model: firstModel, ModelIdentity: identity,
	}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	firstSession, err := firstOwner.Session(context.Background(), NamedSession("persistent"))
	if err != nil {
		t.Fatal(err)
	}
	firstRun, err := firstSession.Run(context.Background(), Input{Text: "remember this", IdempotencyKey: "persist-one"})
	if err != nil {
		t.Fatal(err)
	}
	if result, err := firstRun.Wait(context.Background()); err != nil || result.Status != ResultCompleted {
		t.Fatalf("first durable result=%#v err=%v", result, err)
	}
	if err := firstOwner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	secondModel := &lifecycleModel{responses: []*Message{AssistantMessage("restored answer", nil)}}
	secondOwner, err := New(context.Background(), Definition{
		Key: "durable-test", Model: secondModel, ModelIdentity: identity,
	}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = secondOwner.Close(context.Background()) })
	secondSession, err := secondOwner.Session(context.Background(), NamedSession("persistent"))
	if err != nil {
		t.Fatal(err)
	}
	secondRun, err := secondSession.Run(context.Background(), Input{Text: "what did I say?", IdempotencyKey: "persist-two"})
	if err != nil {
		t.Fatal(err)
	}
	if result, err := secondRun.Wait(context.Background()); err != nil || result.Status != ResultCompleted {
		t.Fatalf("second durable result=%#v err=%v", result, err)
	}
	calls := secondModel.calls()
	if len(calls) != 1 || len(calls[0]) != 3 || calls[0][0].Content != "remember this" ||
		calls[0][1].Content != "persisted answer" || calls[0][2].Content != "what did I say?" {
		t.Fatalf("restored model transcript=%#v", calls)
	}
}

func TestAgentRestoresCompactionContextDataAndPassesItToContextSource(t *testing.T) {
	store, err := sessionfile.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	identity := CapabilityIdentity{Kind: "model.compaction-context-test", Version: 1}
	firstContext := &recordingContextSource{}
	firstOwner, err := New(context.Background(), Definition{
		Key: "compaction-context-test", Model: &lifecycleModel{responses: []*Message{
			AssistantMessage("answer one", nil), AssistantMessage("answer two", nil),
		}},
		ModelIdentity: identity, Context: firstContext, Compaction: contextDataCompactionManager{},
	}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	firstSession, err := firstOwner.Session(context.Background(), NamedSession("compaction-context"))
	if err != nil {
		t.Fatal(err)
	}
	for index, text := range []string{"question one", "question two"} {
		run, runErr := firstSession.Run(context.Background(), Input{Text: text, IdempotencyKey: fmt.Sprintf("context-before-%d", index)})
		if runErr != nil {
			t.Fatal(runErr)
		}
		if _, waitErr := run.Wait(context.Background()); waitErr != nil {
			t.Fatal(waitErr)
		}
	}
	compacted, err := firstSession.Compact(context.Background(), CompactionRequest{
		Force: true, IdempotencyKey: "context-compact",
	})
	if err != nil || !compacted.Changed || compacted.State.ContextData == nil ||
		string(compacted.State.ContextData.Data) != `{"cursor":2}` {
		t.Fatalf("compaction result=%#v err=%v", compacted, err)
	}
	if err := firstOwner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	secondContext := &recordingContextSource{}
	secondOwner, err := New(context.Background(), Definition{
		Key: "compaction-context-test", Model: &lifecycleModel{responses: []*Message{AssistantMessage("answer three", nil)}},
		ModelIdentity: identity, Context: secondContext, Compaction: contextDataCompactionManager{},
	}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = secondOwner.Close(context.Background()) })
	secondSession, err := secondOwner.Session(context.Background(), NamedSession("compaction-context"))
	if err != nil {
		t.Fatal(err)
	}
	third, err := secondSession.Run(context.Background(), Input{Text: "question three", IdempotencyKey: "context-after-reopen"})
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := third.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
		t.Fatalf("restored run=%#v err=%v", result, waitErr)
	}
	if !secondContext.sawContextData(`{"cursor":2}`) {
		t.Fatal("restored ContextSource did not receive durable Compaction ContextData")
	}
}

func TestCompactionContextDataValidationRejectsInvalidAndOversizedPayloads(t *testing.T) {
	invalid := &HostData{Type: "test", Version: 1, Data: []byte(`{"broken"`)}
	if err := validateCompactionContextData(invalid); err == nil || !strings.Contains(err.Error(), "valid JSON") {
		t.Fatalf("invalid JSON error=%v", err)
	}
	oversized := &HostData{
		Type: "test", Version: 1,
		Data: append(append([]byte{'"'}, make([]byte, maxCompactionContextDataBytes)...), '"'),
	}
	for index := 1; index < len(oversized.Data)-1; index++ {
		oversized.Data[index] = 'x'
	}
	if err := validateCompactionContextData(oversized); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized ContextData error=%v", err)
	}
}

func TestRunSteerPreservesAcceptedInputAcrossPreemption(t *testing.T) {
	model := newGatedLifecycleModel()
	owner, err := New(context.Background(), Definition{Model: model})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("steer"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), Input{Text: "original", IdempotencyKey: "steer-start"})
	if err != nil {
		t.Fatal(err)
	}
	firstCall := <-model.calls
	if len(firstCall) != 1 || firstCall[0].Content != "original" {
		t.Fatalf("first call=%#v", firstCall)
	}
	steerReceipt, err := run.Steer(context.Background(), Input{Text: "correction", IdempotencyKey: "steer-correction"})
	if err != nil {
		t.Fatal(err)
	}
	if steerReceipt.CommandID != "steer-correction" || steerReceipt.RunID != run.ID() || steerReceipt.Cursor == 0 {
		t.Fatalf("steer receipt = %#v", steerReceipt)
	}
	// Safe steering waits for the in-flight provider call, discards its answer,
	// then starts a new cycle with both accepted user inputs.
	model.responses <- AssistantMessage("discarded", nil)
	secondCall := <-model.calls
	if len(secondCall) != 2 || secondCall[0].Content != "original" || secondCall[1].Content != "correction" {
		t.Fatalf("steered call=%#v", secondCall)
	}
	model.responses <- AssistantMessage("corrected", nil)
	if result, err := run.Wait(context.Background()); err != nil || result.Status != ResultCompleted {
		t.Fatalf("steered result=%#v err=%v", result, err)
	}
}

func TestQueuedFollowUpCanBeAbortedThroughItsOwnHandle(t *testing.T) {
	model := newGatedLifecycleModel()
	owner, err := New(context.Background(), Definition{Model: model})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("queue"))
	if err != nil {
		t.Fatal(err)
	}
	parent, err := session.Run(context.Background(), Input{Text: "parent", IdempotencyKey: "queue-parent"})
	if err != nil {
		t.Fatal(err)
	}
	<-model.calls
	queued, err := parent.FollowUp(context.Background(), Input{Text: "later", IdempotencyKey: "queue-child"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queued.Abort(context.Background(), AbortRequest{Reason: "no longer needed"}); err != nil {
		t.Fatal(err)
	}
	if result, err := queued.Wait(context.Background()); err != nil || result.Status != ResultAborted {
		t.Fatalf("queued result=%#v err=%v", result, err)
	}
	model.responses <- AssistantMessage("parent done", nil)
	if result, err := parent.Wait(context.Background()); err != nil || result.Status != ResultCompleted {
		t.Fatalf("parent result=%#v err=%v", result, err)
	}
	select {
	case unexpected := <-model.calls:
		t.Fatalf("cancelled queued Run reached model: %#v", unexpected)
	default:
	}
}

func TestQueuedInputCanInterruptTheCurrentRunWithoutBecomingAnotherRun(t *testing.T) {
	model := newGatedLifecycleModel()
	owner, err := New(context.Background(), Definition{Model: model})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("queued-input"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), Input{Text: "parent", IdempotencyKey: "queued-input-parent"})
	if err != nil {
		t.Fatal(err)
	}
	<-model.calls
	queued, err := run.Queue(context.Background(), Input{Text: "interrupt with this", IdempotencyKey: "queued-input-child"})
	if err != nil {
		t.Fatal(err)
	}
	if queued.ID() != "queued-input-child" {
		t.Fatalf("queued input id = %q", queued.ID())
	}
	if receipt := queued.Receipt(); receipt.CommandID != queued.ID() || receipt.RunID != run.ID() || receipt.Cursor == 0 {
		t.Fatalf("queued input receipt = %#v", receipt)
	}
	restored, found, err := run.Queued(context.Background(), queued.ID())
	if err != nil || !found {
		t.Fatalf("restore queued input found=%v err=%v", found, err)
	}
	interruptReceipt, err := restored.Interrupt(context.Background(), QueueControlRequest{IdempotencyKey: "interrupt-queued-input"})
	if err != nil {
		t.Fatal(err)
	}
	if interruptReceipt.CommandID != "interrupt-queued-input" || interruptReceipt.RunID != run.ID() || interruptReceipt.Cursor == 0 {
		t.Fatalf("interrupt receipt = %#v", interruptReceipt)
	}
	model.responses <- AssistantMessage("discarded", nil)
	second := <-model.calls
	if len(second) != 2 || second[0].Content != "parent" || second[1].Content != "interrupt with this" {
		t.Fatalf("interrupted model context = %#v", second)
	}
	model.responses <- AssistantMessage("updated", nil)
	if result, err := run.Wait(context.Background()); err != nil || result.Status != ResultCompleted {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestSessionAttachRunRestoresQueuedAndSettledHandles(t *testing.T) {
	model := newGatedLifecycleModel()
	owner, err := New(context.Background(), Definition{Model: model})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("attach"))
	if err != nil {
		t.Fatal(err)
	}
	parent, err := session.Run(context.Background(), Input{Text: "parent", IdempotencyKey: "attach-parent"})
	if err != nil {
		t.Fatal(err)
	}
	<-model.calls
	queued, err := parent.FollowUp(context.Background(), Input{Text: "queued", IdempotencyKey: "attach-child"})
	if err != nil {
		t.Fatal(err)
	}
	attached, found, err := session.AttachRun(context.Background(), queued.ID())
	if err != nil || !found {
		t.Fatalf("attach queued found=%v err=%v", found, err)
	}
	if _, err := attached.Abort(context.Background(), AbortRequest{Reason: "cancel attached queue"}); err != nil {
		t.Fatal(err)
	}
	if result, err := attached.Wait(context.Background()); err != nil || result.Status != ResultAborted {
		t.Fatalf("attached queued result=%#v err=%v", result, err)
	}
	if result, err := queued.Wait(context.Background()); err != nil || result.Status != ResultAborted {
		t.Fatalf("original queued result=%#v err=%v", result, err)
	}
	model.responses <- AssistantMessage("done", nil)
	if result, err := parent.Wait(context.Background()); err != nil || result.Status != ResultCompleted {
		t.Fatalf("parent result=%#v err=%v", result, err)
	}
	replayed, found, err := session.AttachRun(context.Background(), parent.ID())
	if err != nil || !found {
		t.Fatalf("attach settled found=%v err=%v", found, err)
	}
	if result, err := replayed.Wait(context.Background()); err != nil || result.Status != ResultCompleted {
		t.Fatalf("replayed settled result=%#v err=%v", result, err)
	}
}
