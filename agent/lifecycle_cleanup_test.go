package agent

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sessionfile "github.com/alfredxw/denova/agent/session/file"
)

type scriptedCleanupManager struct {
	mu                   sync.Mutex
	planCalls            int
	failWithHistory      bool
	fallbackToCompaction bool
}

type identityCleanupManager struct{ identity CapabilityIdentity }

func (manager identityCleanupManager) Identity() CapabilityIdentity { return manager.identity }

func (identityCleanupManager) Plan(context.Context, CleanupPlanRequest) (CleanupPlan, error) {
	return CleanupPlan{Action: CleanupNone, Reason: "below_cleanup_threshold"}, nil
}

type stablePrefixCapture struct {
	mu         sync.Mutex
	cleanup    int
	compaction int
}

func (capture *stablePrefixCapture) setCleanup(value int) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	capture.cleanup = value
}

func (capture *stablePrefixCapture) setCompaction(value int) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	capture.compaction = value
}

func (capture *stablePrefixCapture) values() (int, int) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return capture.cleanup, capture.compaction
}

type prefixCaptureCleanupManager struct{ capture *stablePrefixCapture }

func (prefixCaptureCleanupManager) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "cleanup.prefix-provenance-test", Version: 1}
}

func (manager prefixCaptureCleanupManager) Plan(_ context.Context, request CleanupPlanRequest) (CleanupPlan, error) {
	manager.capture.setCleanup(request.ModelInspection.StablePrefixMessages)
	return CleanupPlan{Action: CleanupNone, Reason: "below_cleanup_threshold"}, nil
}

type prefixCaptureCompactionManager struct{ capture *stablePrefixCapture }

func (prefixCaptureCompactionManager) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "compaction.prefix-provenance-test", Version: 1}
}

type cleanupBoundaryCompactionManager struct{}

func (cleanupBoundaryCompactionManager) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "compaction.cleanup-boundary-test", Version: 1}
}

func (cleanupBoundaryCompactionManager) SummaryLimitBytes() int { return 64 << 10 }

func (cleanupBoundaryCompactionManager) Plan(_ context.Context, request CompactionPlanRequest) (CompactionPlan, error) {
	if !request.Force || len(request.Messages) < 2 {
		return CompactionPlan{Action: CompactionNone}, nil
	}
	return CompactionPlan{
		Action: CompactionCreate, SourceFrom: 0, SourceTo: len(request.Messages),
		Validation: CompactionValidationPolicy{HardLimitBytes: 8 << 20},
	}, nil
}

func (cleanupBoundaryCompactionManager) Compact(context.Context, CompactionCompactRequest) (CompactionCheckpoint, error) {
	return CompactionCheckpoint{Summary: "checkpoint absorbed prior cleanup boundary", TokenEstimate: 8}, nil
}

func (prefixCaptureCompactionManager) SummaryLimitBytes() int { return 64 << 10 }

func (manager prefixCaptureCompactionManager) Plan(_ context.Context, request CompactionPlanRequest) (CompactionPlan, error) {
	if request.ModelSnapshot != nil {
		manager.capture.setCompaction(request.ModelSnapshot.StablePrefixMessages())
	}
	if !request.Force || len(request.Messages) < 2 {
		return CompactionPlan{Action: CompactionNone}, nil
	}
	return CompactionPlan{
		Action: CompactionCreate, SourceFrom: 0, SourceTo: 2,
		Validation: CompactionValidationPolicy{HardLimitBytes: 8 << 20},
	}, nil
}

func (prefixCaptureCompactionManager) Compact(context.Context, CompactionCompactRequest) (CompactionCheckpoint, error) {
	return CompactionCheckpoint{Summary: "bounded prefix checkpoint", TokenEstimate: 5}, nil
}

type userLeadingPrefixSource struct{}

func (userLeadingPrefixSource) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "context.user-leading-prefix-test", Version: 1}
}

func (userLeadingPrefixSource) Materialize(context.Context, ContextRequest) ([]ContextFragment, error) {
	return []ContextFragment{{
		Source: "test.leading", Purpose: "verify authenticated stable prefix", Resource: "stable-user-context",
		Placement: ContextLeadingMessage, Role: User, Rendering: ContextRenderVerbatim,
		Content: strings.Repeat("stable user context ", 80), HardLimit: 64 << 10,
	}}, nil
}

func (*scriptedCleanupManager) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "cleanup.lifecycle-test", Version: 1}
}

func (manager *scriptedCleanupManager) Plan(_ context.Context, request CleanupPlanRequest) (CleanupPlan, error) {
	manager.mu.Lock()
	manager.planCalls++
	manager.mu.Unlock()
	if manager.failWithHistory && len(request.Messages) >= 2 {
		return CleanupPlan{
			Action: CleanupProject, Reason: "test_stage_failure", Renderer: "cleanup.test.v1",
			FallbackToCompaction: manager.fallbackToCompaction,
			Metrics:              CleanupMetrics{PressureBefore: .9, BodyPressureBefore: .9},
		}, errors.New("cleanup projection preparation failed")
	}
	for index, message := range request.ModelRequest {
		if message == nil || message.Role != ToolRole || strings.HasPrefix(message.Content, "[cleaned ") {
			continue
		}
		return CleanupPlan{
			Action: CleanupProject, Reason: "recoverable_tool_result", Renderer: "cleanup.test.v1",
			Replacements: []CleanupReplacement{{
				MessageIndex: index, ToolCallID: message.ToolCallID,
				Placeholder:    fmt.Sprintf("[cleaned %s]", message.ToolCallID),
				OriginalTokens: 32, PlaceholderTokens: 4,
			}},
			Metrics: CleanupMetrics{EstimatedTokensBefore: 64, EstimatedTokensAfter: 36, ReclaimedTokens: 28},
		}, nil
	}
	return CleanupPlan{Action: CleanupNone, Reason: "below_cleanup_threshold"}, nil
}

func (manager *scriptedCleanupManager) calls() int {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.planCalls
}

type injectedToolContextMiddleware struct{ BaseMiddleware }

func (*injectedToolContextMiddleware) BeforeModelCall(
	ctx context.Context,
	call *ModelCall,
	_ *ModelContext,
) (context.Context, *ModelCall, error) {
	next := *call
	next.Messages = append(cloneMessages(call.Messages), &Message{
		Role: ToolRole, Content: "middleware-only tool context", ToolCallID: "injected-call", ToolName: "injected",
		ToolResult: &ToolResultSummary{Status: ToolResultSuccess, ResultRetention: ToolResultDeferred},
	})
	return ctx, &next, nil
}

type contextMaintenanceOrder struct {
	mu    sync.Mutex
	steps []string
}

func (order *contextMaintenanceOrder) append(step string) {
	order.mu.Lock()
	defer order.mu.Unlock()
	order.steps = append(order.steps, step)
}

func (order *contextMaintenanceOrder) snapshot() []string {
	order.mu.Lock()
	defer order.mu.Unlock()
	return append([]string(nil), order.steps...)
}

type reportingNormalizerMiddleware struct {
	BaseMiddleware
	order *contextMaintenanceOrder
}

func (middleware *reportingNormalizerMiddleware) BeforeModelCall(
	ctx context.Context,
	call *ModelCall,
	modelContext *ModelContext,
) (context.Context, *ModelCall, error) {
	middleware.order.append("normalizer")
	modelContext.ReportContextNormalization(ContextNormalizationMetrics{
		RepairCount: 1, MessagesBefore: len(call.Messages), MessagesAfter: len(call.Messages),
	})
	return ctx, call, nil
}

type orderingCleanupManager struct{ order *contextMaintenanceOrder }

func (orderingCleanupManager) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "cleanup.ordering-test", Version: 1}
}

func (manager orderingCleanupManager) Plan(context.Context, CleanupPlanRequest) (CleanupPlan, error) {
	manager.order.append("maintenance")
	return CleanupPlan{Action: CleanupNone, Reason: "below_cleanup_threshold"}, nil
}

type orderingModel struct{ order *contextMaintenanceOrder }

func (model orderingModel) Generate(context.Context, []*Message, ...ModelOption) (*Message, error) {
	model.order.append("provider")
	return AssistantMessage("done", nil), nil
}

func (model orderingModel) Stream(context.Context, []*Message, ...ModelOption) (*StreamReader[*Message], error) {
	model.order.append("provider")
	return StreamReaderFromArray([]*Message{AssistantMessage("done", nil)}), nil
}

type countingLifecycleModel struct{ calls atomic.Int32 }

func (model *countingLifecycleModel) Generate(context.Context, []*Message, ...ModelOption) (*Message, error) {
	model.calls.Add(1)
	return AssistantMessage("unexpected", nil), nil
}

func (model *countingLifecycleModel) Stream(context.Context, []*Message, ...ModelOption) (*StreamReader[*Message], error) {
	model.calls.Add(1)
	return StreamReaderFromArray([]*Message{AssistantMessage("unexpected", nil)}), nil
}

func cleanupLifecycleTools(t testing.TB) Toolset {
	t.Helper()
	tool, err := InferTool("read", "read recoverable context", func(context.Context, struct{}) (string, error) {
		return "recoverable payload", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return mustStaticToolsIdentified(t,
		CapabilityIdentity{Kind: "tools.cleanup-lifecycle-test", Version: 1},
		testToolDefinition(tool),
	)
}

func cleanupToolCall(id string) *Message {
	return AssistantMessage("", []ToolCall{{
		ID: id, Type: "function", Function: FunctionCall{Name: "read", Arguments: `{}`},
	}})
}

func TestCleanupRunsAtEveryModelSeamButSettlesOnceAndReappliesProjection(t *testing.T) {
	manager := &scriptedCleanupManager{}
	model := &lifecycleModel{responses: []*Message{
		cleanupToolCall("first-provider-call"),
		cleanupToolCall("second-provider-call"),
		AssistantMessage("done", nil),
	}}
	owner, err := New(context.Background(), Definition{
		Model: model, Tools: cleanupLifecycleTools(t), Cleanup: manager,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("cleanup-every-seam"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), Input{Text: "inspect", IdempotencyKey: "cleanup-every-seam"})
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
		t.Fatalf("result=%#v err=%v", result, waitErr)
	}
	if manager.calls() != 2 {
		t.Fatalf("cleanup Plan calls=%d, want initial observation plus post-tool selection", manager.calls())
	}
	calls := model.calls()
	if len(calls) != 3 {
		t.Fatalf("model calls=%d", len(calls))
	}
	for index := 1; index < len(calls); index++ {
		if !containsCleanupPlaceholder(calls[index]) {
			t.Fatalf("model call %d lost the staged cleanup projection: %#v", index, calls[index])
		}
	}
	state, present, err := session.Cleanup(context.Background())
	if err != nil || !present || state.Revision != 1 || len(state.Replacements) != 1 {
		t.Fatalf("cleanup state=%#v present=%v err=%v", state, present, err)
	}
	completed := 0
	for event := range run.Events() {
		if _, ok := event.Payload.(CleanupCompleted); ok {
			completed++
			if event.Durability != DurableEvent {
				t.Fatalf("settled Cleanup completion durability=%q", event.Durability)
			}
		}
	}
	if completed != 1 {
		t.Fatalf("CleanupCompleted events=%d, want one durable settlement", completed)
	}
}

func TestCleanupExactReplayAcrossSessionInstancesKeepsOneProjection(t *testing.T) {
	store, err := sessionfile.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	modelIdentity := CapabilityIdentity{Kind: "model.cleanup-exact-replay", Version: 1}
	definition := func(model BaseChatModel) Definition {
		return Definition{
			Key: "cleanup-exact-replay", Model: model, ModelIdentity: modelIdentity,
			Tools: cleanupLifecycleTools(t), Cleanup: &scriptedCleanupManager{},
		}
	}
	firstModel := &lifecycleModel{responses: []*Message{
		cleanupToolCall("cleanup-exact-replay-call"), AssistantMessage("settled once", nil),
	}}
	first, err := New(context.Background(), definition(firstModel), WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	key := NamedSession("cleanup-exact-replay")
	session, err := first.Session(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	input := Input{Text: "inspect once", IdempotencyKey: "cleanup-exact-command"}
	run, err := session.Run(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
		t.Fatalf("first Run=%#v err=%v", result, waitErr)
	}
	want, present, err := session.Cleanup(context.Background())
	if err != nil || !present {
		t.Fatalf("first Cleanup=%#v present=%t err=%v", want, present, err)
	}
	if err := first.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	replayModel := &countingLifecycleModel{}
	second, err := New(context.Background(), definition(replayModel), WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close(context.Background()) })
	session, err = second.Session(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := session.Run(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := replayed.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
		t.Fatalf("replayed Run=%#v err=%v", result, waitErr)
	}
	got, present, err := session.Cleanup(context.Background())
	if err != nil || !present || got.ID != want.ID || got.Revision != want.Revision || !reflect.DeepEqual(got.Replacements, want.Replacements) {
		t.Fatalf("replayed Cleanup=%#v present=%t err=%v, want %#v", got, present, err, want)
	}
	if calls := replayModel.calls.Load(); calls != 0 {
		t.Fatalf("exact replay called model %d time(s)", calls)
	}
}

func TestCleanupRejectsInvalidDurableProjection(t *testing.T) {
	owner, err := New(context.Background(), Definition{Model: &lifecycleModel{responses: []*Message{AssistantMessage("unused", nil)}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("invalid-cleanup-projection"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	setClearTestCapability(t, session, cleanupCapability, CleanupState{
		ID: "invalid-cleanup", Revision: 1, SourceHash: "invalid-but-nonempty", SourceStart: 0, SourceEnd: 1,
		Replacements: []CleanupReplacement{{MessageIndex: 1, ToolCallID: "outside-range", Placeholder: "[invalid]"}},
		Renderer:     "test.invalid", CreatedAt: now, UpdatedAt: now,
	})
	if state, present, err := session.Cleanup(context.Background()); err == nil || present || !strings.Contains(err.Error(), "durable Cleanup replacements are invalid") {
		t.Fatalf("invalid Cleanup=%#v present=%t err=%v", state, present, err)
	}
}

func TestContextNormalizerRunsImmediatelyBeforeContextMaintenance(t *testing.T) {
	order := &contextMaintenanceOrder{}
	owner, err := New(context.Background(), Definition{
		Model: orderingModel{order: order}, Cleanup: orderingCleanupManager{order: order},
		Middlewares: []Middleware{&reportingNormalizerMiddleware{order: order}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	run, err := owner.Run(context.Background(), Input{Text: "inspect", IdempotencyKey: "context-normalizer-order"})
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
		t.Fatalf("result=%#v err=%v", result, waitErr)
	}
	if got := order.snapshot(); !reflect.DeepEqual(got, []string{"normalizer", "maintenance", "provider"}) {
		t.Fatalf("model seam order=%v", got)
	}
	normalized := 0
	for event := range run.Events() {
		if payload, ok := event.Payload.(ContextNormalized); ok {
			normalized++
			if event.Durability != EphemeralEvent || payload.RepairCount != 1 || payload.MessagesBefore != payload.MessagesAfter {
				t.Fatalf("ContextNormalized event=%#v durability=%q", payload, event.Durability)
			}
		}
	}
	if normalized != 1 {
		t.Fatalf("ContextNormalized events=%d, want 1", normalized)
	}
}

func containsCleanupPlaceholder(messages []*Message) bool {
	for _, message := range messages {
		if message != nil && message.Role == ToolRole && strings.HasPrefix(message.Content, "[cleaned ") {
			return true
		}
	}
	return false
}

func TestCleanupFailureDoesNotRetryAfterToolAndFallsBackOnlyAtCompactionPressure(t *testing.T) {
	for _, test := range []struct {
		name        string
		fallback    bool
		compaction  bool
		wantPlan    int
		wantCompact int
	}{
		{name: "low pressure keeps unchanged request", wantPlan: 2},
		{name: "checkpoint pressure falls back", fallback: true, compaction: true, wantPlan: 2, wantCompact: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager := &scriptedCleanupManager{failWithHistory: true, fallbackToCompaction: test.fallback}
			model := &lifecycleModel{responses: []*Message{
				AssistantMessage("seed answer", nil),
				cleanupToolCall("after-failure"), AssistantMessage("done", nil),
			}}
			definition := Definition{Model: model, Tools: cleanupLifecycleTools(t), Cleanup: manager}
			var compaction *maintenanceAwareCompactionManager
			if test.compaction {
				compaction = &maintenanceAwareCompactionManager{}
				definition.Compaction = compaction
			}
			owner, err := New(context.Background(), definition)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = owner.Close(context.Background()) })
			session, err := owner.Session(context.Background(), NamedSession("cleanup-fallback-"+test.name))
			if err != nil {
				t.Fatal(err)
			}
			for index, text := range []string{"seed", "trigger"} {
				run, runErr := session.Run(context.Background(), Input{Text: text, IdempotencyKey: fmt.Sprintf("cleanup-fallback-%d", index)})
				if runErr != nil {
					t.Fatal(runErr)
				}
				if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
					t.Fatalf("run %d result=%#v err=%v", index, result, waitErr)
				}
			}
			if manager.calls() != test.wantPlan {
				t.Fatalf("cleanup Plan calls=%d, want %d", manager.calls(), test.wantPlan)
			}
			if compaction != nil {
				_, compactCalls := compaction.calls()
				if compactCalls != test.wantCompact {
					t.Fatalf("Compaction calls=%d, want %d", compactCalls, test.wantCompact)
				}
			}
		})
	}
}

func TestCleanupSettlementFailureLeavesNoStateOrCompletedEvent(t *testing.T) {
	manager := &scriptedCleanupManager{}
	model := &scriptedModel{responses: []scriptedModelResponse{
		{message: cleanupToolCall("failing-final")},
		{err: errors.New("provider unavailable")},
	}}
	owner, err := New(context.Background(), Definition{
		Model: model, Tools: cleanupLifecycleTools(t), Cleanup: manager,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("cleanup-failed-settlement"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), Input{Text: "inspect", IdempotencyKey: "cleanup-failed-settlement"})
	if err != nil {
		t.Fatal(err)
	}
	if _, waitErr := run.Wait(context.Background()); waitErr == nil {
		t.Fatal("run unexpectedly completed after the owning final model failed")
	}
	if state, present, cleanupErr := session.Cleanup(context.Background()); cleanupErr != nil || present {
		t.Fatalf("failed run committed Cleanup state=%#v present=%v err=%v", state, present, cleanupErr)
	}
	for event := range run.Events() {
		if completed, ok := event.Payload.(CleanupCompleted); ok {
			t.Fatalf("failed settlement published CleanupCompleted: %#v", completed)
		}
	}
}

func TestCleanupRejectsMiddlewareOnlyTargetBeforeProviderCall(t *testing.T) {
	manager := &scriptedCleanupManager{}
	model := &countingLifecycleModel{}
	owner, err := New(context.Background(), Definition{
		Model: model, Cleanup: manager, Middlewares: []Middleware{&injectedToolContextMiddleware{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	run, err := owner.Run(context.Background(), Input{Text: "inspect", IdempotencyKey: "cleanup-injected-target"})
	if err != nil {
		t.Fatal(err)
	}
	if _, waitErr := run.Wait(context.Background()); waitErr == nil ||
		(!strings.Contains(waitErr.Error(), "cannot be persisted") && !strings.Contains(waitErr.Error(), "must originate in the Agent loop")) {
		t.Fatalf("run error=%v, want unpersistable Cleanup target", waitErr)
	}
	if model.calls.Load() != 0 {
		t.Fatalf("provider calls=%d, want zero", model.calls.Load())
	}
}

func TestCleanupClearCreatesVisibleNewGenerationAndSurvivesReopen(t *testing.T) {
	store, err := sessionfile.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := &scriptedCleanupManager{}
	identity := CapabilityIdentity{Kind: "model.cleanup-clear-test", Version: 1}
	model := &lifecycleModel{responses: []*Message{
		cleanupToolCall("generation-one"), AssistantMessage("one", nil),
		cleanupToolCall("generation-two"), AssistantMessage("two", nil),
		cleanupToolCall("generation-three"), AssistantMessage("three", nil),
	}}
	definition := Definition{
		Key: "cleanup-clear-test", Model: model, ModelIdentity: identity,
		Tools: cleanupLifecycleTools(t), Cleanup: manager,
	}
	owner, err := New(context.Background(), definition, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	session, err := owner.Session(context.Background(), NamedSession("cleanup-clear"))
	if err != nil {
		t.Fatal(err)
	}
	for generation := 1; generation <= 3; generation++ {
		if generation > 1 {
			if clearErr := session.Clear(context.Background()); clearErr != nil {
				t.Fatal(clearErr)
			}
			if hidden, present, cleanupErr := session.Cleanup(context.Background()); cleanupErr != nil || present {
				t.Fatalf("generation %d Clear left Cleanup visible: %#v present=%v err=%v", generation, hidden, present, cleanupErr)
			}
		}
		run, runErr := session.Run(context.Background(), Input{
			Text: fmt.Sprintf("generation %d", generation), IdempotencyKey: fmt.Sprintf("cleanup-generation-%d", generation),
		})
		if runErr != nil {
			t.Fatal(runErr)
		}
		if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
			t.Fatalf("generation %d result=%#v err=%v", generation, result, waitErr)
		}
		state, present, cleanupErr := session.Cleanup(context.Background())
		if cleanupErr != nil || !present || state.Revision != uint64(generation) {
			t.Fatalf("generation %d Cleanup=%#v present=%v err=%v", generation, state, present, cleanupErr)
		}
	}
	if err := owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	reopened, err := New(context.Background(), Definition{
		Key: "cleanup-clear-test", Model: &lifecycleModel{}, ModelIdentity: identity,
		Tools: cleanupLifecycleTools(t), Cleanup: &scriptedCleanupManager{},
	}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close(context.Background()) })
	reopenedSession, err := reopened.Session(context.Background(), NamedSession("cleanup-clear"))
	if err != nil {
		t.Fatal(err)
	}
	state, present, err := reopenedSession.Cleanup(context.Background())
	if err != nil || !present || state.Revision != 3 {
		t.Fatalf("reopened Cleanup=%#v present=%v err=%v", state, present, err)
	}
}

func TestToolResultCleanupIsInvalidatedByCompactionAndClearBoundaries(t *testing.T) {
	store, err := sessionfile.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	modelIdentity := CapabilityIdentity{Kind: "model.cleanup-compaction-boundary", Version: 1}
	manager := cleanupBoundaryCompactionManager{}
	model := &lifecycleModel{responses: []*Message{
		cleanupToolCall("cleanup-before-compaction"), AssistantMessage("seed complete", nil),
	}}
	owner, err := New(context.Background(), Definition{
		Key: "cleanup-compaction-boundary", Model: model, ModelIdentity: modelIdentity,
		Tools: cleanupLifecycleTools(t), Cleanup: &scriptedCleanupManager{}, Compaction: manager,
	}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	session, err := owner.Session(context.Background(), NamedSession("cleanup-compaction-boundary"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), Input{Text: "seed", IdempotencyKey: "cleanup-boundary-seed"})
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
		t.Fatalf("seed result=%#v err=%v", result, waitErr)
	}
	cleanupState, cleanupPresent, err := session.Cleanup(context.Background())
	if err != nil || !cleanupPresent {
		t.Fatalf("cleanup before Compaction=%#v present=%v err=%v", cleanupState, cleanupPresent, err)
	}
	compact, err := session.Compact(context.Background(), CompactionRequest{
		Force: true, IdempotencyKey: "cleanup-boundary-compact",
	})
	if err != nil || !compact.Changed || compact.State.CleanupRevisionAtCompaction != cleanupState.Revision {
		t.Fatalf("Compaction result=%#v err=%v", compact, err)
	}
	if hidden, present, cleanupErr := session.Cleanup(context.Background()); cleanupErr != nil || present {
		t.Fatalf("pre-Compaction Cleanup remained visible: %#v present=%v err=%v", hidden, present, cleanupErr)
	}
	removed, err := session.RemoveCompaction(context.Background(), CompactionRemoveRequest{
		ID: compact.State.ID, ExpectedRevision: compact.State.Revision, IdempotencyKey: "cleanup-boundary-remove",
	})
	if err != nil || !removed {
		t.Fatalf("remove Compaction removed=%v err=%v", removed, err)
	}
	if revived, present, cleanupErr := session.Cleanup(context.Background()); cleanupErr != nil || present {
		t.Fatalf("removed Compaction revived Cleanup=%#v present=%v err=%v", revived, present, cleanupErr)
	}
	if snapshot, snapshotErr := session.Snapshot(context.Background()); snapshotErr != nil || snapshot.Cleanup != nil {
		t.Fatalf("removed Compaction public snapshot Cleanup=%#v err=%v", snapshot.Cleanup, snapshotErr)
	}
	if err := owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	reopenModel := &lifecycleModel{responses: []*Message{AssistantMessage("rich history restored", nil)}}
	reopened, err := New(context.Background(), Definition{
		Key: "cleanup-compaction-boundary", Model: reopenModel, ModelIdentity: modelIdentity,
		Tools: cleanupLifecycleTools(t), Compaction: manager,
	}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close(context.Background()) })
	reopenedSession, err := reopened.Session(context.Background(), NamedSession("cleanup-compaction-boundary"))
	if err != nil {
		t.Fatal(err)
	}
	if cleanup, present, cleanupErr := reopenedSession.Cleanup(context.Background()); cleanupErr != nil || present {
		t.Fatalf("cold reopen revived Cleanup=%#v present=%v err=%v", cleanup, present, cleanupErr)
	}
	if snapshot, snapshotErr := reopenedSession.Snapshot(context.Background()); snapshotErr != nil || snapshot.Cleanup != nil {
		t.Fatalf("cold reopen public snapshot Cleanup=%#v err=%v", snapshot.Cleanup, snapshotErr)
	}
	reopenedRun, err := reopenedSession.Run(context.Background(), Input{Text: "continue", IdempotencyKey: "cleanup-boundary-reopen"})
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := reopenedRun.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
		t.Fatalf("reopened result=%#v err=%v", result, waitErr)
	}
	calls := reopenModel.calls()
	if len(calls) != 1 || !messagesContainContent(calls[0], "recoverable payload") || containsCleanupPlaceholder(calls[0]) {
		t.Fatalf("removed Compaction did not restore rich raw history: %#v", calls)
	}
}

func TestFrozenCleanupDisambiguatesReusedToolCallIDs(t *testing.T) {
	base := []*Message{
		UserMessage("request"),
		{Role: ToolRole, Content: "first", ToolCallID: "reused", ToolName: "read"},
		{Role: ToolRole, Content: "second", ToolCallID: "reused", ToolName: "read"},
	}
	plan := []CleanupReplacement{{MessageIndex: 2, ToolCallID: "reused", Placeholder: "[cleaned second]"}}
	frozen, err := freezeCleanupTargets(base, base, base, len(base), plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(frozen.raw) != 1 || frozen.raw[0].MessageIndex != 2 {
		t.Fatalf("frozen targets=%#v", frozen.raw)
	}
	later := append(cloneMessages(base), &Message{Role: ToolRole, Content: "third", ToolCallID: "reused", ToolName: "read"})
	projected, err := applyStagedCleanupProjection(later, frozen.projection)
	if err != nil {
		t.Fatal(err)
	}
	if projected[1].Content != "first" || projected[2].Content != "[cleaned second]" || projected[3].Content != "third" {
		t.Fatalf("reused call projection=%#v", projected)
	}
}

func TestCleanupIdentityChangesRestoreKeyWithoutPollutingStablePrefixOrRunIdentity(t *testing.T) {
	model := &lifecycleModel{}
	definition := Definition{
		Key: "cleanup-identity", Model: model,
		ModelIdentity: CapabilityIdentity{Kind: "model.cleanup-identity-test", Version: 1},
		Cleanup:       identityCleanupManager{identity: CapabilityIdentity{Kind: "cleanup.identity-test", Version: 1, ConfigHash: "one"}},
	}
	prepare := func(runID, commandID string, cleanupHash string) preparedDefinition {
		t.Helper()
		definition.Cleanup = identityCleanupManager{identity: CapabilityIdentity{
			Kind: "cleanup.identity-test", Version: 1, ConfigHash: cleanupHash,
		}}
		prepared, err := prepareDefinition(context.Background(), definition, PrepareRequest{
			Session: SessionView{Key: NamedSession("cleanup-identity-session"), Revision: 7},
			Run:     RunView{ID: runID, CommandID: commandID, Cycle: 1}, Reason: TurnReasonStart,
		})
		if err != nil {
			t.Fatal(err)
		}
		return prepared
	}
	first := prepare("run-one", "command-one", "one")
	samePolicyOtherRun := prepare("run-two", "command-two", "one")
	changedPolicy := prepare("run-three", "command-three", "two")
	if first.restoreKey != samePolicyOtherRun.restoreKey || first.prefixFingerprint != samePolicyOtherRun.prefixFingerprint {
		t.Fatalf("run/session identity polluted stable definition identity: first=%#v second=%#v", first, samePolicyOtherRun)
	}
	if first.restoreKey == changedPolicy.restoreKey {
		t.Fatal("Cleanup policy identity did not change exact restore semantics")
	}
	if first.prefixFingerprint != changedPolicy.prefixFingerprint {
		t.Fatal("storage-free Cleanup policy polluted the stable provider prefix fingerprint")
	}
}

func TestCleanupAndCompactionShareAuthenticatedLeadingUserAndCheckpointPrefix(t *testing.T) {
	capture := &stablePrefixCapture{}
	model := &lifecycleModel{responses: []*Message{
		AssistantMessage("first answer", nil), AssistantMessage("second answer", nil), AssistantMessage("third answer", nil),
	}}
	owner, err := New(context.Background(), Definition{
		Instructions: "stable instruction", Model: model, Context: userLeadingPrefixSource{},
		Cleanup:    prefixCaptureCleanupManager{capture: capture},
		Compaction: prefixCaptureCompactionManager{capture: capture},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("authenticated-prefix"))
	if err != nil {
		t.Fatal(err)
	}
	for index, text := range []string{strings.Repeat("old body ", 100), "second body"} {
		run, runErr := session.Run(context.Background(), Input{Text: text, IdempotencyKey: fmt.Sprintf("prefix-turn-%d", index)})
		if runErr != nil {
			t.Fatal(runErr)
		}
		if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
			t.Fatalf("turn=%#v err=%v", result, waitErr)
		}
	}
	if result, compactErr := session.Compact(context.Background(), CompactionRequest{Force: true, IdempotencyKey: "prefix-compact"}); compactErr != nil || !result.Changed {
		t.Fatalf("Compaction=%#v err=%v", result, compactErr)
	}
	third, err := session.Run(context.Background(), Input{Text: "third body", IdempotencyKey: "prefix-third"})
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := third.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
		t.Fatalf("third=%#v err=%v", result, waitErr)
	}
	cleanupPrefix, compactionPrefix := capture.values()
	// Instruction + user-role leading Context + active checkpoint are one
	// authenticated prefix. Neither manager relies on Message.Extra or text.
	if cleanupPrefix != 3 || compactionPrefix != 3 {
		t.Fatalf("stable prefix cleanup=%d compaction=%d", cleanupPrefix, compactionPrefix)
	}
	calls := model.calls()
	if len(calls) != 3 || calls[2][0].Role != System || calls[2][1].Role != User || calls[2][2].Role != System {
		t.Fatalf("provider prefix layout=%#v", calls)
	}
}
