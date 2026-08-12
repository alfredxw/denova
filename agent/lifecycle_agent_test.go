package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agentsession "github.com/alfredxw/denova/agent/session"
	sessionfile "github.com/alfredxw/denova/agent/session/file"
)

type persistentMemoryStore struct{ agentsession.Store }

type fixedCompactionManager struct{}

func (fixedCompactionManager) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "compaction.fixed-test", Version: 1}
}

func (fixedCompactionManager) SummaryLimitBytes() int { return 64 << 10 }

func (fixedCompactionManager) Plan(_ context.Context, request CompactionPlanRequest) (CompactionPlan, error) {
	if !request.Force || len(request.Messages) < 2 {
		return CompactionPlan{Action: CompactionNone}, nil
	}
	return CompactionPlan{Action: CompactionCreate, SourceFrom: 0, SourceTo: 2,
		Validation: CompactionValidationPolicy{HardLimitBytes: 8 << 20}}, nil
}

func (fixedCompactionManager) Compact(context.Context, CompactionCompactRequest) (CompactionCheckpoint, error) {
	return CompactionCheckpoint{Summary: "summary of the first turn", TokenEstimate: 6}, nil
}

type contextDataCompactionManager struct{}

func (contextDataCompactionManager) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "compaction.context-data-test", Version: 1}
}

func (contextDataCompactionManager) SummaryLimitBytes() int { return 64 << 10 }

func (contextDataCompactionManager) Plan(_ context.Context, request CompactionPlanRequest) (CompactionPlan, error) {
	if !request.Force || len(request.Messages) < 2 {
		return CompactionPlan{Action: CompactionNone}, nil
	}
	return CompactionPlan{Action: CompactionCreate, SourceFrom: 0, SourceTo: 2,
		Validation: CompactionValidationPolicy{HardLimitBytes: 8 << 20}}, nil
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
	sawFinalSnapshot   bool
	cacheKey           string
}

func (*automaticContextDataCompactionManager) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "compaction.same-cycle-host-context-test", Version: 1}
}

func (*automaticContextDataCompactionManager) SummaryLimitBytes() int { return 64 << 10 }

func (*automaticContextDataCompactionManager) Plan(_ context.Context, request CompactionPlanRequest) (CompactionPlan, error) {
	if len(request.Messages) < 2 {
		return CompactionPlan{Action: CompactionNone}, nil
	}
	return CompactionPlan{Action: CompactionCreate, SourceFrom: 0, SourceTo: 2,
		Validation: CompactionValidationPolicy{HardLimitBytes: 8 << 20}}, nil
}

func (manager *automaticContextDataCompactionManager) Compact(_ context.Context, request CompactionCompactRequest) (CompactionCheckpoint, error) {
	manager.mu.Lock()
	for _, message := range request.ModelRequest {
		if message != nil && strings.Contains(message.Content, "FULL_HOST_CONTEXT") {
			manager.sawFullHostContext = true
		}
	}
	if request.ModelSnapshot != nil {
		manager.cacheKey = request.ModelSnapshot.ResolvedOptions().SessionKey
		for _, message := range request.ModelSnapshot.Messages() {
			if message != nil && strings.Contains(message.Content, "FINAL_MIDDLEWARE_CONTEXT") {
				manager.sawFinalSnapshot = true
			}
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

func (manager *automaticContextDataCompactionManager) sawMiddlewareSnapshot() bool {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.sawFinalSnapshot
}

func (manager *automaticContextDataCompactionManager) snapshotCacheKey() string {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.cacheKey
}

type appendFinalContextMiddleware struct{ BaseMiddleware }

func (*appendFinalContextMiddleware) BeforeModelCall(
	ctx context.Context,
	call *ModelCall,
	_ *ModelContext,
) (context.Context, *ModelCall, error) {
	next := *call
	next.Messages = append(cloneMessages(call.Messages), UserMessage("FINAL_MIDDLEWARE_CONTEXT"))
	return ctx, &next, nil
}

type maintenanceAwareCompactionManager struct {
	mu           sync.Mutex
	planCalls    int
	compactCalls int
}

type postToolCompactionManager struct {
	mu           sync.Mutex
	planCalls    int
	compactCalls int
}

func (*postToolCompactionManager) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "compaction.post-tool-model-seam-test", Version: 1}
}

func (*postToolCompactionManager) SummaryLimitBytes() int { return 64 << 10 }

func (manager *postToolCompactionManager) Plan(_ context.Context, request CompactionPlanRequest) (CompactionPlan, error) {
	manager.mu.Lock()
	manager.planCalls++
	manager.mu.Unlock()
	for _, message := range request.ModelRequest {
		if message != nil && message.Role == ToolRole && len(request.Messages) >= 2 {
			return CompactionPlan{
				Action: CompactionCreate, SourceFrom: 0, SourceTo: 2,
				Validation: CompactionValidationPolicy{HardLimitBytes: 8 << 20},
			}, nil
		}
	}
	return CompactionPlan{Action: CompactionNone, SkippedReason: "below_trigger"}, nil
}

func (manager *postToolCompactionManager) Compact(context.Context, CompactionCompactRequest) (CompactionCheckpoint, error) {
	manager.mu.Lock()
	manager.compactCalls++
	manager.mu.Unlock()
	return CompactionCheckpoint{Summary: "checkpoint selected at the post-tool model seam", TokenEstimate: 9}, nil
}

func (manager *postToolCompactionManager) calls() (int, int) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.planCalls, manager.compactCalls
}

type failingAutomaticCompactionManager struct {
	mu        sync.Mutex
	planCalls int
}

type hardOverflowCleanupManager struct{}

func (hardOverflowCleanupManager) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "cleanup.hard-overflow-lifecycle-test", Version: 1}
}

func (hardOverflowCleanupManager) Plan(_ context.Context, request CleanupPlanRequest) (CleanupPlan, error) {
	if len(request.Messages) < 4 {
		return CleanupPlan{Action: CleanupNone, Reason: "below_cleanup_threshold"}, nil
	}
	for index, message := range request.ModelRequest {
		if message == nil || message.Role != ToolRole {
			continue
		}
		return CleanupPlan{
			Action: CleanupCompact, Reason: "hard_overflow", Renderer: "cleanup.hard-overflow-test.v1",
			FallbackToCompaction: true,
			Replacements: []CleanupReplacement{{
				MessageIndex: index, ToolCallID: message.ToolCallID,
				Placeholder: "[recoverable result removed before checkpoint]", OriginalTokens: 2_000, PlaceholderTokens: 12,
			}},
			Metrics: CleanupMetrics{PressureBefore: 1.2, BodyPressureBefore: 1.2},
		}, nil
	}
	return CleanupPlan{Action: CleanupNone, Reason: "below_cleanup_threshold"}, nil
}

type hardOverflowCompactionManager struct {
	mu                   sync.Mutex
	compactCalls         int
	sawCleanupProjection bool
}

func (*hardOverflowCompactionManager) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "compaction.hard-overflow-lifecycle-test", Version: 1}
}

func (*hardOverflowCompactionManager) SummaryLimitBytes() int { return 64 << 10 }

func (manager *hardOverflowCompactionManager) Plan(_ context.Context, request CompactionPlanRequest) (CompactionPlan, error) {
	if len(request.Messages) < 4 {
		return CompactionPlan{Action: CompactionNone}, nil
	}
	return CompactionPlan{Action: CompactionCreate, SourceFrom: 0, SourceTo: len(request.Messages),
		Validation: CompactionValidationPolicy{HardLimitBytes: 8 << 20}}, nil
}

func (manager *hardOverflowCompactionManager) Compact(_ context.Context, request CompactionCompactRequest) (CompactionCheckpoint, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.compactCalls++
	for _, message := range request.ModelRequest {
		if message != nil && message.Role == ToolRole && message.Content == "[recoverable result removed before checkpoint]" {
			manager.sawCleanupProjection = true
		}
	}
	return CompactionCheckpoint{Summary: "bounded checkpoint after transient cleanup", TokenEstimate: 9}, nil
}

func (manager *hardOverflowCompactionManager) observed() (int, bool) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.compactCalls, manager.sawCleanupProjection
}

func (*failingAutomaticCompactionManager) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "compaction.failure-fuse-test", Version: 1}
}

func (*failingAutomaticCompactionManager) SummaryLimitBytes() int { return 64 << 10 }

func (manager *failingAutomaticCompactionManager) Plan(context.Context, CompactionPlanRequest) (CompactionPlan, error) {
	manager.mu.Lock()
	manager.planCalls++
	manager.mu.Unlock()
	return CompactionPlan{}, errors.New("checkpoint fork failed")
}

func (*failingAutomaticCompactionManager) Compact(context.Context, CompactionCompactRequest) (CompactionCheckpoint, error) {
	return CompactionCheckpoint{}, errors.New("unexpected Compact call")
}

func (manager *failingAutomaticCompactionManager) calls() int {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.planCalls
}

func (*maintenanceAwareCompactionManager) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "compaction.maintenance-order-test", Version: 1}
}

func (*maintenanceAwareCompactionManager) SummaryLimitBytes() int { return 64 << 10 }

func (manager *maintenanceAwareCompactionManager) Plan(_ context.Context, request CompactionPlanRequest) (CompactionPlan, error) {
	manager.mu.Lock()
	manager.planCalls++
	manager.mu.Unlock()
	if len(request.Messages) < 2 {
		return CompactionPlan{Action: CompactionNone}, nil
	}
	return CompactionPlan{Action: CompactionCreate, SourceFrom: 0, SourceTo: 2}, nil
}

func (manager *maintenanceAwareCompactionManager) Compact(context.Context, CompactionCompactRequest) (CompactionCheckpoint, error) {
	manager.mu.Lock()
	manager.compactCalls++
	manager.mu.Unlock()
	return CompactionCheckpoint{Summary: "unexpected checkpoint", TokenEstimate: 2}, nil
}

func (manager *maintenanceAwareCompactionManager) calls() (int, int) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.planCalls, manager.compactCalls
}

type maintenanceProjectionMiddleware struct{ BaseMiddleware }

func (*maintenanceProjectionMiddleware) BeforeModelCall(
	ctx context.Context,
	call *ModelCall,
	_ *ModelContext,
) (context.Context, *ModelCall, error) {
	if len(call.Messages) < 2 {
		return ctx, call, nil
	}
	next := *call
	next.Messages = []*Message{UserMessage("REVERSIBLE_CLEANUP_PROJECTION")}
	return ctx, &next, nil
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
	options   []*Options
}

type streamingToolInputLifecycleModel struct {
	mu          sync.Mutex
	streamCalls int
	first       *StreamReader[*Message]
	writer      *StreamWriter[*Message]
}

func newStreamingToolInputLifecycleModel() *streamingToolInputLifecycleModel {
	reader, writer := Pipe[*Message](-1)
	return &streamingToolInputLifecycleModel{first: reader, writer: writer}
}

func (model *streamingToolInputLifecycleModel) Generate(context.Context, []*Message, ...ModelOption) (*Message, error) {
	return nil, errors.New("streaming tool input test unexpectedly used Generate")
}

func (model *streamingToolInputLifecycleModel) Stream(context.Context, []*Message, ...ModelOption) (*StreamReader[*Message], error) {
	model.mu.Lock()
	defer model.mu.Unlock()
	model.streamCalls++
	if model.streamCalls == 1 {
		return model.first, nil
	}
	return StreamReaderFromArray([]*Message{AssistantMessage("done", nil)}), nil
}

func TestPublicRunStreamsToolInputBeforeToolExecutionStarts(t *testing.T) {
	model := newStreamingToolInputLifecycleModel()
	toolEntered := make(chan struct{})
	releaseTool := make(chan struct{})
	var enterOnce sync.Once
	var releaseOnce sync.Once
	t.Cleanup(func() {
		model.writer.Close()
		releaseOnce.Do(func() { close(releaseTool) })
	})
	tool, err := InferTool("read", "read a file", func(ctx context.Context, input struct {
		Path string `json:"path"`
	}) (string, error) {
		enterOnce.Do(func() { close(toolEntered) })
		select {
		case <-releaseTool:
			return input.Path, nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := New(context.Background(), Definition{
		Name: "streaming-tool-input", Model: model,
		Tools: mustStaticToolsIdentified(t,
			CapabilityIdentity{Kind: "tools.streaming-input-test", Version: 1},
			testToolDefinition(tool),
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	run, err := owner.Run(context.Background(), Text("read the draft"))
	if err != nil {
		t.Fatal(err)
	}

	nextEvent := func(match func(EventPayload) bool) EventPayload {
		t.Helper()
		timer := time.NewTimer(time.Second)
		defer timer.Stop()
		for {
			select {
			case event, ok := <-run.Events():
				if !ok {
					t.Fatal("public Run events closed before the expected event")
				}
				if match(event.Payload) {
					return event.Payload
				}
			case <-timer.C:
				t.Fatal("timed out waiting for the expected public Run event")
			}
		}
	}

	index := 0
	model.writer.Send(&Message{Role: Assistant, ToolCalls: []ToolCall{{
		Index: &index, ID: "provider-call", Type: "function",
		Function: FunctionCall{Name: "read", Arguments: `{"path":"`},
	}}}, nil)
	inputStarted := nextEvent(func(payload EventPayload) bool {
		_, ok := payload.(ToolInputStarted)
		return ok
	}).(ToolInputStarted)
	firstDelta := nextEvent(func(payload EventPayload) bool {
		_, ok := payload.(ToolInputDelta)
		return ok
	}).(ToolInputDelta)
	if inputStarted.CallID == "" || inputStarted.ProviderCallID != "provider-call" || inputStarted.Name != "read" ||
		inputStarted.Index != 0 || inputStarted.Descriptor == nil || inputStarted.Descriptor.Source != ToolSourceRead {
		t.Fatalf("tool input start = %#v", inputStarted)
	}
	if firstDelta.CallID != inputStarted.CallID || firstDelta.Delta != `{"path":"` {
		t.Fatalf("first tool input delta = %#v, start = %#v", firstDelta, inputStarted)
	}
	select {
	case <-toolEntered:
		t.Fatal("tool execution started before the model input stream completed")
	default:
	}

	model.writer.Send(&Message{ToolCalls: []ToolCall{{
		Index: &index, Function: FunctionCall{Arguments: `draft.md"}`},
	}}}, nil)
	secondDelta := nextEvent(func(payload EventPayload) bool {
		_, ok := payload.(ToolInputDelta)
		return ok
	}).(ToolInputDelta)
	if secondDelta.CallID != inputStarted.CallID || secondDelta.Delta != `draft.md"}` {
		t.Fatalf("second tool input delta = %#v, start = %#v", secondDelta, inputStarted)
	}
	select {
	case <-toolEntered:
		t.Fatal("tool execution started before the model input stream completed")
	default:
	}

	model.writer.Close()
	started := nextEvent(func(payload EventPayload) bool {
		_, ok := payload.(ToolStarted)
		return ok
	}).(ToolStarted)
	if started.CallID != inputStarted.CallID || started.ProviderCallID != "provider-call" || started.Name != "read" ||
		started.Index != 0 || started.Descriptor == nil || started.Descriptor.Source != ToolSourceRead ||
		string(started.Arguments) != `{"path":"draft.md"}` {
		t.Fatalf("tool execution start = %#v, input start = %#v", started, inputStarted)
	}
	select {
	case <-toolEntered:
	case <-time.After(time.Second):
		t.Fatal("tool did not execute after its model input stream completed")
	}
	releaseOnce.Do(func() { close(releaseTool) })
	if result, err := run.Wait(context.Background()); err != nil || result.Status != ResultCompleted {
		t.Fatalf("result=%#v err=%v", result, err)
	}
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
	toolset := mustStaticToolsIdentified(t, CapabilityIdentity{Kind: "tools.permission-test", Version: 1}, ToolDefinition{
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
	if request.Permission.Mode == "" || request.Permission.Risk == "" || request.Permission.RuleID == "" ||
		request.Permission.ArgsHash == "" || len(request.Permission.Options) != 2 || request.Permission.CanRemember {
		t.Fatalf("permission presentation lost audit/options metadata: %#v", request.Permission)
	}
	for _, option := range request.Permission.Options {
		if option.Label.Chinese == "" || option.Label.English == "" ||
			option.Description.Chinese == "" || option.Description.English == "" {
			t.Fatalf("permission option is not bilingual: %#v", option)
		}
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
		if err := ForwardNestedEvent(ctx, NestedEvent{
			Source:    EventSource{Name: "child", Path: []string{"root", "child"}, InvocationID: "child-session/child-run", InvocationType: "task"},
			SessionID: "child-session",
			Child:     Event{Cursor: 1, Durability: EphemeralEvent, RunID: "child-run", Payload: AssistantDelta{Delta: "child display"}},
		}); err != nil {
			return ToolResult{}, err
		}
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
		Tools: mustStaticToolsIdentified(t, CapabilityIdentity{Kind: "tools.nested-display-test", Version: 1}, definition),
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
	var nested NestedEvent
	var started ToolStarted
	var finished ToolFinished
	for event := range run.Events() {
		switch payload := event.Payload.(type) {
		case NestedEvent:
			if payload.Source.Name == "child" {
				nested = payload
			}
		case ToolStarted:
			started = payload
		case ToolFinished:
			finished = payload
		}
	}
	childDelta, ok := nested.Child.Payload.(AssistantDelta)
	if !ok || childDelta.Delta != "child display" || len(nested.Source.Path) != 2 || nested.SessionID != "child-session" {
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
		Tools: mustStaticToolsIdentified(t, CapabilityIdentity{Kind: "tools.completion-test", Version: 1}, ToolDefinition{
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

func (model *lifecycleModel) Generate(_ context.Context, input []*Message, options ...ModelOption) (*Message, error) {
	return model.next(input, options...)
}

func (model *lifecycleModel) Stream(_ context.Context, input []*Message, options ...ModelOption) (*StreamReader[*Message], error) {
	message, err := model.next(input, options...)
	if err != nil {
		return nil, err
	}
	return StreamReaderFromArray([]*Message{message}), nil
}

func (model *lifecycleModel) next(input []*Message, options ...ModelOption) (*Message, error) {
	model.mu.Lock()
	defer model.mu.Unlock()
	model.inputs = append(model.inputs, cloneMessages(input))
	model.options = append(model.options, GetCommonOptions(&Options{}, options...))
	if len(model.responses) == 0 {
		return nil, errors.New("lifecycle model exhausted")
	}
	message := CloneMessage(model.responses[0])
	model.responses = model.responses[1:]
	return message, nil
}

func (model *lifecycleModel) cacheKeys() []string {
	model.mu.Lock()
	defer model.mu.Unlock()
	keys := make([]string, len(model.options))
	for index, options := range model.options {
		keys[index] = options.SessionKey
	}
	return keys
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

func TestAgentBindsStableOpaqueProviderCacheKeyPerSession(t *testing.T) {
	model := &lifecycleModel{responses: []*Message{
		AssistantMessage("first", nil), AssistantMessage("second", nil), AssistantMessage("other", nil),
	}}
	owner, err := New(context.Background(), Definition{Model: model})
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(context.Background())
	first, err := owner.Session(context.Background(), SessionKey{
		Namespace: "private", ID: "session-one", Attributes: map[string]string{"workspace": "/secret/book"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for index, text := range []string{"one", "two"} {
		run, runErr := first.Run(context.Background(), Input{Text: text, IdempotencyKey: fmt.Sprintf("cache-first-%d", index)})
		if runErr != nil {
			t.Fatal(runErr)
		}
		if _, waitErr := run.Wait(context.Background()); waitErr != nil {
			t.Fatal(waitErr)
		}
	}
	second, err := owner.Session(context.Background(), SessionKey{Namespace: "private", ID: "session-two"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := second.Run(context.Background(), Input{Text: "other", IdempotencyKey: "cache-second"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	keys := model.cacheKeys()
	if len(keys) != 3 || keys[0] == "" || keys[0] != keys[1] || keys[0] == keys[2] {
		t.Fatalf("provider cache keys = %#v", keys)
	}
	for _, key := range keys {
		if strings.Contains(key, "session-") || strings.Contains(key, "/secret/book") {
			t.Fatalf("provider cache key exposed Session identity: %q", key)
		}
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
	questionOne := strings.TrimSpace(strings.Repeat("question one with substantial context ", 80))
	for index, text := range []string{questionOne, "question two"} {
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
	if len(calls[3]) != 7 || calls[3][0].Content != questionOne || calls[3][6].Content != "question four" {
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

func TestCompactionIncrementalSourceUsesCheckpointAndOnlyNewRawTail(t *testing.T) {
	messages := []*Message{
		UserMessage("old request"), AssistantMessage("old answer", nil),
		UserMessage("new request"), AssistantMessage("new answer", nil),
	}
	current := CompactionState{
		ID: "checkpoint-one", Revision: 1, Summary: "old checkpoint summary",
		ReplacementFrom: 0, ReplacementTo: 2,
	}
	source := compactionIncrementalSource(messages, CompactionPlan{
		Action: CompactionCreate, SourceFrom: 0, SourceTo: 3,
	}, current, true, 64<<10)
	if len(source) != 2 || source[0].Role != System || !strings.Contains(source[0].Content, current.Summary) ||
		source[1].Content != "new request" {
		t.Fatalf("incremental Compaction source = %#v", source)
	}
	for _, message := range source {
		if message.Content == "old request" || message.Content == "old answer" {
			t.Fatalf("hidden raw history leaked back into incremental source: %#v", source)
		}
	}
}

func TestAutomaticCompactionRematerializesHostContextBeforeSameCycleModelCall(t *testing.T) {
	model := &lifecycleModel{responses: []*Message{
		AssistantMessage("answer one", nil), AssistantMessage("answer two", nil),
	}}
	manager := &automaticContextDataCompactionManager{}
	owner, err := New(context.Background(), Definition{
		Model: model, Context: projectingContextSource{}, Compaction: manager,
		Middlewares: []Middleware{&appendFinalContextMiddleware{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("same-cycle-host-compaction"))
	if err != nil {
		t.Fatal(err)
	}
	for index, text := range []string{strings.Repeat("question one with host state ", 80), "question two"} {
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
	if !manager.sawMiddlewareSnapshot() {
		t.Fatal("Compaction manager did not receive the final post-middleware model snapshot")
	}
	calls := model.calls()
	cacheKeys := model.cacheKeys()
	if len(cacheKeys) != 2 || cacheKeys[1] == "" || manager.snapshotCacheKey() != cacheKeys[1] {
		t.Fatalf("automatic Compaction cache key=%q primary=%#v", manager.snapshotCacheKey(), cacheKeys)
	}
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

func TestMiddlewareProjectionCannotSkipFixedAutomaticCompaction(t *testing.T) {
	model := &lifecycleModel{responses: []*Message{
		AssistantMessage("answer one", nil), AssistantMessage("answer two", nil),
	}}
	manager := &maintenanceAwareCompactionManager{}
	owner, err := New(context.Background(), Definition{
		Model: model, Compaction: manager,
		Middlewares: []Middleware{&maintenanceProjectionMiddleware{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("maintenance-before-compaction"))
	if err != nil {
		t.Fatal(err)
	}
	for index, text := range []string{strings.TrimSpace(strings.Repeat("question one with host state ", 80)), "question two"} {
		run, runErr := session.Run(context.Background(), Input{
			Text: text, IdempotencyKey: fmt.Sprintf("maintenance-order-%d", index),
		})
		if runErr != nil {
			t.Fatal(runErr)
		}
		if _, waitErr := run.Wait(context.Background()); waitErr != nil {
			t.Fatal(waitErr)
		}
	}
	planCalls, compactCalls := manager.calls()
	if planCalls != 2 || compactCalls != 1 {
		t.Fatalf("fixed compaction calls after middleware projection: plan=%d compact=%d", planCalls, compactCalls)
	}
	calls := model.calls()
	if len(calls) != 2 || len(calls[1]) != 1 || calls[1][0].Content != "REVERSIBLE_CLEANUP_PROJECTION" {
		t.Fatalf("final cleanup projection = %#v", calls)
	}
}

func TestAutomaticCompactionFailureContinuesPrimaryModelAndOpensDurableFuse(t *testing.T) {
	model := &lifecycleModel{responses: []*Message{
		AssistantMessage("answer one", nil), AssistantMessage("answer two", nil),
		AssistantMessage("answer three", nil), AssistantMessage("answer four", nil),
	}}
	manager := &failingAutomaticCompactionManager{}
	owner, err := New(context.Background(), Definition{
		Model: model, Compaction: manager,
		Execution: ExecutionPolicy{MaxAutomaticCompactionFailures: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("automatic-compaction-failure-fuse"))
	if err != nil {
		t.Fatal(err)
	}
	failedEvents := 0
	skippedEvents := 0
	for index := range 4 {
		run, runErr := session.Run(context.Background(), Input{
			Text: fmt.Sprintf("question %d", index+1), IdempotencyKey: fmt.Sprintf("failure-fuse-%d", index),
		})
		if runErr != nil {
			t.Fatal(runErr)
		}
		result, waitErr := run.Wait(context.Background())
		if waitErr != nil || result.Status != ResultCompleted {
			t.Fatalf("run %d result=%#v err=%v", index, result, waitErr)
		}
		for event := range run.Events() {
			switch payload := event.Payload.(type) {
			case CompactionFailed:
				failedEvents++
				if payload.ConsecutiveFailures != index+1 || payload.FailureFuseOpen != (index == 2) {
					t.Fatalf("failure event %d = %#v", index, payload)
				}
			case CompactionSkipped:
				skippedEvents++
				if payload.Reason != "consecutive_failure_fuse" || !payload.FailureFuseOpen || payload.ConsecutiveFailures != 3 {
					t.Fatalf("skip event = %#v", payload)
				}
			}
		}
	}
	if manager.calls() != 3 || len(model.calls()) != 4 || failedEvents != 3 || skippedEvents != 1 {
		t.Fatalf("plan=%d model=%d failed=%d skipped=%d", manager.calls(), len(model.calls()), failedEvents, skippedEvents)
	}
}

func TestAdvanceCompactionHealthCountsOnlySameStructure(t *testing.T) {
	first := nextCompactionHealth(compactionHealthState{}, false, "structure-a", errors.New("failed"))
	same := nextCompactionHealth(first, true, "structure-a", errors.New("failed again"))
	changed := nextCompactionHealth(same, true, "structure-b", errors.New("different structure"))
	if first.ConsecutiveFailures != 1 || same.ConsecutiveFailures != 2 || changed.ConsecutiveFailures != 1 ||
		changed.Fingerprint != "structure-b" {
		t.Fatalf("Compaction health sequence first=%#v same=%#v changed=%#v", first, same, changed)
	}
}

func TestFailedCompactionForkIsNotRetriedAfterAToolCallInTheSameRun(t *testing.T) {
	model := &lifecycleModel{responses: []*Message{
		cleanupToolCall("after-compaction-failure"), AssistantMessage("done", nil),
	}}
	manager := &failingAutomaticCompactionManager{}
	owner, err := New(context.Background(), Definition{
		Model: model, Tools: cleanupLifecycleTools(t), Compaction: manager,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	run, err := owner.Run(context.Background(), Input{Text: "inspect", IdempotencyKey: "compaction-failure-one-run"})
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
		t.Fatalf("result=%#v err=%v", result, waitErr)
	}
	if manager.calls() != 1 || len(model.calls()) != 2 {
		t.Fatalf("Compaction plan calls=%d model calls=%d", manager.calls(), len(model.calls()))
	}
}

func TestContextMaintenanceRunsAtEveryModelSeamButMutatesOnlyOncePerRun(t *testing.T) {
	model := &lifecycleModel{responses: []*Message{
		AssistantMessage("seed answer", nil), cleanupToolCall("post-tool-pressure"), AssistantMessage("done", nil),
	}}
	manager := &postToolCompactionManager{}
	owner, err := New(context.Background(), Definition{
		Model: model, Tools: cleanupLifecycleTools(t), Compaction: manager,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("compaction-every-model-seam"))
	if err != nil {
		t.Fatal(err)
	}
	for index, text := range []string{strings.Repeat("seed context ", 200), "inspect with a tool"} {
		run, runErr := session.Run(context.Background(), Input{
			Text: text, IdempotencyKey: fmt.Sprintf("compaction-every-seam-%d", index),
		})
		if runErr != nil {
			t.Fatal(runErr)
		}
		if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
			t.Fatalf("run %d result=%#v err=%v", index, result, waitErr)
		}
	}
	planCalls, compactCalls := manager.calls()
	if planCalls != 3 || compactCalls != 1 {
		t.Fatalf("maintenance calls plan=%d compact=%d, want every seam and one mutation", planCalls, compactCalls)
	}
	if calls := model.calls(); len(calls) != 3 || calls[2][0].Role != System ||
		!strings.Contains(calls[2][0].Content, "post-tool model seam") {
		t.Fatalf("post-tool Compaction was not applied before the final provider call: %#v", calls)
	}
}

func TestContextCompactionHealthPersistsAcrossTranscriptRevisionsAndReload(t *testing.T) {
	store, err := sessionfile.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	definition := func(model BaseChatModel, manager CompactionManager) Definition {
		return Definition{
			Key: "compaction-health-reload", Model: model,
			ModelIdentity: CapabilityIdentity{Kind: "model.compaction-health-reload", Version: 1},
			Compaction:    manager, Execution: ExecutionPolicy{MaxAutomaticCompactionFailures: 3},
		}
	}
	firstManager := &failingAutomaticCompactionManager{}
	firstOwner, err := New(context.Background(), definition(
		&lifecycleModel{responses: []*Message{AssistantMessage("first", nil)}}, firstManager,
	), WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	firstSession, err := firstOwner.Session(context.Background(), NamedSession("compaction-health-reload"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := firstSession.Run(context.Background(), Input{Text: "one", IdempotencyKey: "health-reload-one"})
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := first.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
		t.Fatalf("first result=%#v err=%v", result, waitErr)
	}
	if err := firstOwner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	secondManager := &failingAutomaticCompactionManager{}
	secondOwner, err := New(context.Background(), definition(
		&lifecycleModel{responses: []*Message{AssistantMessage("second", nil)}}, secondManager,
	), WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = secondOwner.Close(context.Background()) })
	secondSession, err := secondOwner.Session(context.Background(), NamedSession("compaction-health-reload"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := secondSession.Run(context.Background(), Input{Text: "two", IdempotencyKey: "health-reload-two"})
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := second.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
		t.Fatalf("second result=%#v err=%v", result, waitErr)
	}
	wantFailures := 2
	found := false
	for event := range second.Events() {
		if payload, ok := event.Payload.(CompactionFailed); ok {
			found = true
			if payload.ConsecutiveFailures != wantFailures {
				t.Fatalf("reloaded Compaction health=%#v", payload)
			}
		}
	}
	if !found || firstManager.calls() != 1 || secondManager.calls() != 1 {
		t.Fatalf("reloaded failure found=%v calls=(%d,%d)", found, firstManager.calls(), secondManager.calls())
	}
}

func TestContextMaintenanceHardOverflowCleansToolResultsBeforeCompaction(t *testing.T) {
	model := &lifecycleModel{responses: []*Message{
		cleanupToolCall("large-result"), AssistantMessage("seed complete", nil), AssistantMessage("done", nil),
	}}
	compaction := &hardOverflowCompactionManager{}
	owner, err := New(context.Background(), Definition{
		Model: model, Tools: cleanupLifecycleTools(t), Cleanup: hardOverflowCleanupManager{}, Compaction: compaction,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("hard-overflow-cleanup-before-compaction"))
	if err != nil {
		t.Fatal(err)
	}
	for index, text := range []string{"seed", "continue"} {
		run, runErr := session.Run(context.Background(), Input{Text: text, IdempotencyKey: fmt.Sprintf("hard-overflow-%d", index)})
		if runErr != nil {
			t.Fatal(runErr)
		}
		if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
			t.Fatalf("run %d result=%#v err=%v", index, result, waitErr)
		}
	}
	compactCalls, sawProjection := compaction.observed()
	if compactCalls != 1 || !sawProjection {
		t.Fatalf("Compaction calls=%d saw transient Cleanup=%v", compactCalls, sawProjection)
	}
	if _, present, cleanupErr := session.Cleanup(context.Background()); cleanupErr != nil || present {
		t.Fatalf("transient Cleanup became durable present=%v err=%v", present, cleanupErr)
	}
	if _, present, compactionErr := session.compactionState(context.Background()); compactionErr != nil || !present {
		t.Fatalf("checkpoint missing present=%v err=%v", present, compactionErr)
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

func TestAgentUsesApplicationRunIDGenerator(t *testing.T) {
	model := &lifecycleModel{responses: []*Message{AssistantMessage("done", nil)}}
	var request RunIDRequest
	owner, err := New(
		context.Background(),
		Definition{Model: model},
		WithRunIDGenerator(func(value RunIDRequest) (string, error) {
			request = value
			return "host-run-42", nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })

	session, err := owner.Session(context.Background(), NamedSession("custom-identity"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), Text("work"))
	if err != nil {
		t.Fatal(err)
	}
	if run.ID() != "host-run-42" {
		t.Fatalf("run id = %q, want application identity", run.ID())
	}
	if request.Session.Namespace != agentsession.DefaultNamespace || request.Session.ID != "custom-identity" {
		t.Fatalf("run ID request = %#v", request)
	}
	if result, err := run.Wait(context.Background()); err != nil || result.Status != ResultCompleted {
		t.Fatalf("result = %#v, err = %v", result, err)
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
	for index, text := range []string{strings.TrimSpace(strings.Repeat("question one with durable host context ", 80)), "question two"} {
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

type blockingFirstDefinitionSource struct {
	started   chan struct{}
	canceled  chan struct{}
	calls     atomic.Int32
	model     BaseChatModel
	canonical CanonicalAdapter
}

func (source *blockingFirstDefinitionSource) Prepare(ctx context.Context, _ PrepareRequest) (Definition, error) {
	if source.calls.Add(1) == 1 {
		close(source.started)
		<-ctx.Done()
		close(source.canceled)
		return Definition{}, ctx.Err()
	}
	return Definition{Model: source.model, Canonical: source.canonical}, nil
}

func (source *blockingFirstDefinitionSource) CanonicalInput(context.Context, PrepareRequest) (CanonicalAdapter, error) {
	return source.canonical, nil
}

func TestRunControlInterruptsPreparationBeforeModelEffects(t *testing.T) {
	tests := []struct {
		name       string
		control    func(context.Context, *Run) error
		wantStatus ResultStatus
		wantCalls  int
	}{
		{
			name: "steer",
			control: func(ctx context.Context, run *Run) error {
				_, err := run.Steer(ctx, Input{Text: "correction", IdempotencyKey: "prepare-control-steer"})
				return err
			},
			wantStatus: ResultCompleted, wantCalls: 1,
		},
		{
			name: "abort",
			control: func(ctx context.Context, run *Run) error {
				_, err := run.Abort(ctx, AbortRequest{Reason: "stop preparation", IdempotencyKey: "prepare-control-abort"})
				return err
			},
			wantStatus: ResultAborted,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := &lifecycleModel{responses: []*Message{AssistantMessage("controlled answer", nil)}}
			canonical := &identityCanonicalAdapter{identity: CapabilityIdentity{
				Kind: "canonical.preparation-control-test", Version: 1,
			}}
			source := &blockingFirstDefinitionSource{
				started: make(chan struct{}), canceled: make(chan struct{}), model: model, canonical: canonical,
			}
			owner, err := New(context.Background(), source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = owner.Close(context.Background()) })
			session, err := owner.Session(context.Background(), NamedSession("preparation-control-"+test.name))
			if err != nil {
				t.Fatal(err)
			}
			run, err := session.Run(context.Background(), Input{Text: "original", IdempotencyKey: "prepare-control-start-" + test.name})
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-source.started:
			case <-time.After(time.Second):
				t.Fatal("Definition preparation did not start")
			}
			if err := test.control(context.Background(), run); err != nil {
				t.Fatal(err)
			}
			select {
			case <-source.canceled:
			case <-time.After(time.Second):
				t.Fatal("accepted control did not cancel Definition preparation")
			}
			result, err := run.Wait(context.Background())
			if err != nil || result.Status != test.wantStatus {
				t.Fatalf("controlled result=%#v err=%v", result, err)
			}
			if calls := len(model.calls()); calls != test.wantCalls {
				t.Fatalf("model calls=%d, want %d", calls, test.wantCalls)
			}
			wantCanonicalInputs := 1
			if test.name == "steer" {
				wantCanonicalInputs = 2
				calls := model.calls()
				if len(calls[0]) != 2 || calls[0][0].Content != "original" || calls[0][1].Content != "correction" {
					t.Fatalf("steered model input=%#v", calls[0])
				}
			}
			canonicalInputs := int(canonical.materializeCalls.Load())
			if canonicalInputs != wantCanonicalInputs {
				t.Fatalf("canonical input writes=%d, want exactly one per accepted input (%d)", canonicalInputs, wantCanonicalInputs)
			}
		})
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
