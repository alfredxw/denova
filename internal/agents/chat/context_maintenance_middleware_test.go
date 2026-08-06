package chat

import (
	"context"
	"denova/internal/agents/run"
	"denova/internal/agents/toolresult"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	agentcontext "denova/internal/agents/context"
	agentcompaction "denova/internal/agents/context/compaction"
)

type maintenancePlanningConversation struct {
	policy           agentcontext.ContextPressurePolicy
	stageCalls       int
	discardCalls     int
	compactionRuns   int
	plan             toolresult.CleanupPlan
	compactionIn     agentcompaction.Input
	sawSnapshot      bool
	compacted        []*agent.Message
	compactionErr    error
	stageErr         error
	compactionResult agentcompaction.Result
}

func (conversation *maintenancePlanningConversation) ContextPressurePolicy([]*agent.Message) agentcontext.ContextPressurePolicy {
	return conversation.policy
}

func (conversation *maintenancePlanningConversation) StageToolResultCleanup(_ context.Context, _ []*agent.Message, plan toolresult.CleanupPlan) error {
	conversation.stageCalls++
	conversation.plan = plan
	return conversation.stageErr
}

func (conversation *maintenancePlanningConversation) DiscardStagedToolResultCleanup() {
	conversation.discardCalls++
}

type recordingNativeCleanupExecutor struct {
	calls    int
	snapshot *agent.ModelRequestSnapshot
	plan     toolresult.CleanupPlan
	err      error
}

func (*recordingNativeCleanupExecutor) ExecutionMode() agentcontext.ToolResultCleanupExecutionMode {
	return agentcontext.ToolResultCleanupNativeCacheEdit
}

func (executor *recordingNativeCleanupExecutor) Execute(_ context.Context, snapshot *agent.ModelRequestSnapshot, plan toolresult.CleanupPlan) error {
	executor.calls++
	executor.snapshot = snapshot
	executor.plan = plan
	return executor.err
}

func TestNativeAndLocalCleanupExecutorsProduceIdenticalModelProjection(t *testing.T) {
	messages := pressureHistory(10, 9000, agent.ToolResultDeferred, agent.ToolResultContextDiscardable)
	messages = append(messages, agent.UserMessage(strings.Repeat("cold-suffix ", 12_000)))
	policy := pressureTestPolicy(240_000)
	policy.ProviderCacheState = agentcontext.ProviderCacheCold
	call := &agent.ModelCall{Model: &compactionForkCaptureModel{}, Messages: messages, Options: []agent.ModelOption{agent.WithTools(nil)}}

	localConversation := &maintenancePlanningConversation{policy: policy}
	local, localResult, err := prepareContextMaintenance(
		context.Background(), &contextCompactionController{conversation: localConversation}, call, &agent.ModelContext{},
	)
	if err != nil || localResult.Action != agentcontext.ContextMaintenanceCleanup {
		t.Fatalf("local cleanup result=%#v err=%v", localResult, err)
	}

	nativeExecutor := &recordingNativeCleanupExecutor{}
	nativeConversation := &maintenancePlanningConversation{policy: policy}
	nativeCtx := contextWithToolResultCleanupExecutor(context.Background(), nativeExecutor)
	native, nativeResult, err := prepareContextMaintenance(
		nativeCtx, &contextCompactionController{conversation: nativeConversation}, call, &agent.ModelContext{},
	)
	if err != nil || nativeResult.Action != agentcontext.ContextMaintenanceCleanup {
		t.Fatalf("native cleanup result=%#v err=%v", nativeResult, err)
	}
	if nativeExecutor.calls != 1 || nativeExecutor.snapshot == nil || nativeResult.Cleanup.CleanupExecutionMode != agentcontext.ToolResultCleanupNativeCacheEdit {
		t.Fatalf("native executor was not selected from capability: calls=%d result=%#v", nativeExecutor.calls, nativeResult)
	}
	if !reflect.DeepEqual(local.Messages, native.Messages) || !reflect.DeepEqual(localResult.Cleanup.Cleanup, nativeResult.Cleanup.Cleanup) {
		t.Fatalf("cleanup modes diverged\nlocal=%#v\nnative=%#v", localResult.Cleanup.Cleanup, nativeResult.Cleanup.Cleanup)
	}
}

func TestNativeCleanupPreparationFailureDiscardsStagedRecord(t *testing.T) {
	messages := pressureHistory(10, 9000, agent.ToolResultDeferred, agent.ToolResultContextDiscardable)
	policy := pressureTestPolicy(240_000)
	policy.ProviderCacheState = agentcontext.ProviderCacheCold
	conversation := &maintenancePlanningConversation{policy: policy}
	executor := &recordingNativeCleanupExecutor{err: fmt.Errorf("native edit unavailable")}
	ctx := contextWithToolResultCleanupExecutor(context.Background(), executor)
	call := &agent.ModelCall{Model: &compactionForkCaptureModel{}, Messages: messages, Options: []agent.ModelOption{agent.WithTools(nil)}}

	next, result, err := prepareContextMaintenance(ctx, &contextCompactionController{conversation: conversation}, call, &agent.ModelContext{})
	if err == nil || !result.Attempted || result.Action != agentcontext.ContextMaintenanceNone || next != call {
		t.Fatalf("native failure result=%#v next_changed=%t err=%v", result, next != call, err)
	}
	if conversation.stageCalls != 1 || conversation.discardCalls != 1 {
		t.Fatalf("staged cleanup was not discarded: stage=%d discard=%d", conversation.stageCalls, conversation.discardCalls)
	}
}

func (conversation *maintenancePlanningConversation) CompactContextIfNeeded(ctx context.Context, input agentcompaction.Input) ([]*agent.Message, agentcompaction.Result, error) {
	conversation.compactionRuns++
	conversation.compactionIn = input
	conversation.sawSnapshot = input.PrimaryRequestSnapshot != nil
	if conversation.compactionErr != nil {
		return input.Messages, agentcompaction.Result{}, conversation.compactionErr
	}
	if conversation.compacted != nil {
		result := conversation.compactionResult
		result.Triggered = true
		if result.Phase == "" {
			result.Phase = input.Phase
		}
		return conversation.compacted, result, nil
	}
	return input.Messages, agentcompaction.Result{}, nil
}

func TestCleanupStageFailureFallsBackOnlyAtCompactionPressure(t *testing.T) {
	messages := pressureHistory(10, 9000, agent.ToolResultDeferred, agent.ToolResultContextDiscardable)
	messages = append(messages, agent.UserMessage(strings.Repeat("cold-suffix ", 12_000)))
	findPolicy := func(hard bool) agentcontext.ContextPressurePolicy {
		for window := 120_000; window <= 400_000; window += 2_000 {
			policy := pressureTestPolicy(window)
			policy.ProviderCacheState = agentcontext.ProviderCacheCold
			decision := agentcontext.PlanContextPressure(messages, nil, policy)
			atHard := decision.Pressure >= policy.CompactionThreshold || decision.FullPressure >= policy.CompactionThreshold
			if decision.Action == agentcontext.ContextMaintenanceCleanup && atHard == hard {
				return policy
			}
		}
		t.Fatalf("no cleanup fixture found hard=%t", hard)
		return agentcontext.ContextPressurePolicy{}
	}
	for _, test := range []struct {
		name            string
		hard            bool
		wantCompactions int
		wantErr         bool
	}{
		{name: "below_85", hard: false, wantErr: true},
		{name: "at_or_above_85", hard: true, wantCompactions: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			conversation := &maintenancePlanningConversation{
				policy: findPolicy(test.hard), stageErr: fmt.Errorf("stage failed before mutation"),
				compacted: []*agent.Message{agentcontext.NewCompactionSummaryMessage(1, "checkpoint"), agent.UserMessage("current")},
			}
			call := &agent.ModelCall{Model: &compactionForkCaptureModel{}, Messages: messages, Options: []agent.ModelOption{agent.WithTools(nil)}}
			_, result, err := prepareContextMaintenance(context.Background(), &contextCompactionController{conversation: conversation}, call, &agent.ModelContext{})
			if (err != nil) != test.wantErr || conversation.compactionRuns != test.wantCompactions {
				t.Fatalf("result=%#v err=%v cleanup=%d compaction=%d", result, err, conversation.stageCalls, conversation.compactionRuns)
			}
			if test.hard && result.Action != agentcontext.ContextMaintenanceCompaction {
				t.Fatalf("hard fallback action=%q", result.Action)
			}
			if !test.hard && result.Action != agentcontext.ContextMaintenanceNone {
				t.Fatalf("pre-mutation low-pressure failure consumed latch: %#v", result)
			}
		})
	}
}

func TestDegradedCompactionDefersPrimaryProviderCall(t *testing.T) {
	messages := []*agent.Message{agent.SystemMessage("stable"), agent.UserMessage(strings.Repeat("history ", 2_000))}
	conversation := &maintenancePlanningConversation{
		policy:           pressureTestPolicy(2_000),
		compacted:        []*agent.Message{agentcontext.NewCompactionSummaryMessage(1, "checkpoint")},
		compactionResult: agentcompaction.Result{Degraded: true},
	}
	model := &maintenanceSequenceModel{}
	built, err := agent.NewAgent(context.Background(), agent.AgentConfig{
		Name: "degraded-maintenance", Model: model,
		Middlewares: []agent.Middleware{&contextMaintenanceMiddleware{BaseMiddleware: &agent.BaseMiddleware{}, agentKind: "ide"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(context.Background(), contextCompactionContextKey{}, &contextCompactionController{conversation: conversation})
	iterator := built.Run(ctx, &agent.AgentInput{Messages: messages})
	var sawDeferred bool
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event.Err != nil && event.Err != io.EOF {
			sawDeferred = strings.Contains(event.Err.Error(), errAutomaticContextCompactionDeferred.Error())
		}
	}
	if !sawDeferred || model.calls != 0 || conversation.compactionRuns != 1 {
		t.Fatalf("deferred=%t model_calls=%d compaction_calls=%d", sawDeferred, model.calls, conversation.compactionRuns)
	}
}

type maintenanceSequenceModel struct {
	calls  int
	inputs [][]*agent.Message
}

func (model *maintenanceSequenceModel) Generate(_ context.Context, input []*agent.Message, _ ...agent.ModelOption) (*agent.Message, error) {
	model.calls++
	copy := make([]*agent.Message, len(input))
	for index, message := range input {
		if message != nil {
			copy[index] = message.Clone()
		}
	}
	model.inputs = append(model.inputs, copy)
	if model.calls == 1 {
		return agent.AssistantMessage("", []agent.ToolCall{{
			ID: "maintenance-noop", Type: "function",
			Function: agent.FunctionCall{Name: "maintenance_noop", Arguments: `{}`},
		}}), nil
	}
	return agent.AssistantMessage("done", nil), nil
}

func (model *maintenanceSequenceModel) Stream(ctx context.Context, input []*agent.Message, opts ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	message, err := model.Generate(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	return agent.StreamReaderFromArray([]*agent.Message{message}), nil
}

type maintenanceNoopTool struct{}

func (maintenanceNoopTool) Info(context.Context) (*agent.ToolInfo, error) {
	return &agent.ToolInfo{Name: "maintenance_noop", Desc: "continue the test loop"}, nil
}

func (maintenanceNoopTool) Run(context.Context, string, ...agent.ToolOption) (agent.ToolResult, error) {
	return agent.TextToolResult("ok"), nil
}

func maintenanceToolDefinition() agent.ToolDefinition {
	return agent.ToolDefinition{Tool: maintenanceNoopTool{}, Descriptor: agent.ToolDescriptor{
		Source: agent.ToolSourceRead, Execution: agent.ToolExecutionParallelRead,
		MutationScope: agent.ToolMutationNone, PostCheck: agent.ToolPostCheckNone,
		Recovery: agent.ToolRecoveryReadOnly, ResultProjection: agent.ToolResultBoundedModelContext,
		ContextRetention: agent.ToolContextReceipt,
		Steering:         agent.SteeringFinishCurrent, MaxResultBytes: 1024,
	}}
}

func TestContextMaintenanceRunsAtEveryModelSeamButMutatesOnlyOncePerRun(t *testing.T) {
	messages := pressureHistory(10, 9000, agent.ToolResultDeferred, agent.ToolResultContextDiscardable)
	messages = append(messages, agent.UserMessage(fmt.Sprintf("cold suffix %s", strings.Repeat("cold ", 8_000))))
	policy := pressureTestPolicy(160_000)
	policy.ProviderCacheState = agentcontext.ProviderCacheCold
	conversation := &maintenancePlanningConversation{policy: policy}
	model := &maintenanceSequenceModel{}
	middleware := &contextMaintenanceMiddleware{BaseMiddleware: &agent.BaseMiddleware{}, agentKind: "ide"}
	built, err := agent.NewAgent(context.Background(), agent.AgentConfig{
		Name: "maintenance-loop", Model: model,
		Tools:       []agent.ToolDefinition{maintenanceToolDefinition()},
		Middlewares: []agent.Middleware{middleware},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(context.Background(), contextCompactionContextKey{}, &contextCompactionController{conversation: conversation})
	iterator := built.Run(ctx, &agent.AgentInput{Messages: messages})
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event.Err != nil && event.Err != io.EOF {
			t.Fatal(event.Err)
		}
	}
	if model.calls != 2 {
		t.Fatalf("model calls = %d, want first and post-tool calls", model.calls)
	}
	if conversation.stageCalls != 1 || conversation.compactionRuns != 0 {
		t.Fatalf("maintenance calls = cleanup:%d compaction:%d", conversation.stageCalls, conversation.compactionRuns)
	}
	if len(conversation.plan.Replacements) == 0 {
		t.Fatal("cleanup planner did not select replacements")
	}
	for _, replacement := range conversation.plan.Replacements {
		if got := model.inputs[0][replacement.MessageIndex].Content; got != replacement.Placeholder {
			t.Fatalf("first request replacement %d = %q", replacement.MessageIndex, got)
		}
		if got := model.inputs[1][replacement.MessageIndex].Content; got != replacement.Placeholder {
			t.Fatalf("post-tool request lost replacement %d = %q", replacement.MessageIndex, got)
		}
	}
}

func TestFailedCompactionForkIsNotRetriedAfterAToolCallInTheSameRun(t *testing.T) {
	conversation := &maintenancePlanningConversation{
		policy:        pressureTestPolicy(2_000),
		compactionErr: fmt.Errorf("fork failed"),
	}
	model := &maintenanceSequenceModel{}
	built, err := agent.NewAgent(context.Background(), agent.AgentConfig{
		Name: "failed-maintenance-loop", Model: model,
		Tools:       []agent.ToolDefinition{maintenanceToolDefinition()},
		Middlewares: []agent.Middleware{&contextMaintenanceMiddleware{BaseMiddleware: &agent.BaseMiddleware{}, agentKind: "ide"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	messages := []*agent.Message{agent.SystemMessage("stable"), agent.UserMessage(strings.Repeat("history ", 2_000))}
	ctx := context.WithValue(context.Background(), contextCompactionContextKey{}, &contextCompactionController{conversation: conversation})
	iterator := built.Run(ctx, &agent.AgentInput{Messages: messages})
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event.Err != nil && event.Err != io.EOF {
			t.Fatal(event.Err)
		}
	}
	if model.calls != 2 || conversation.compactionRuns != 1 {
		t.Fatalf("calls after failed maintenance = model:%d compaction:%d", model.calls, conversation.compactionRuns)
	}
}

func TestFailedCleanupIsNotRetriedAfterAToolCallInTheSameRun(t *testing.T) {
	for _, test := range []struct {
		name           string
		stageErr       error
		nativeExecutor *recordingNativeCleanupExecutor
	}{
		{name: "durable stage", stageErr: fmt.Errorf("stage failed")},
		{name: "native cache edit", nativeExecutor: &recordingNativeCleanupExecutor{err: fmt.Errorf("native edit failed")}},
	} {
		t.Run(test.name, func(t *testing.T) {
			policy := pressureTestPolicy(160_000)
			policy.ProviderCacheState = agentcontext.ProviderCacheCold
			conversation := &maintenancePlanningConversation{policy: policy, stageErr: test.stageErr}
			model := &maintenanceSequenceModel{}
			built, err := agent.NewAgent(context.Background(), agent.AgentConfig{
				Name: "failed-cleanup-loop", Model: model,
				Tools:       []agent.ToolDefinition{maintenanceToolDefinition()},
				Middlewares: []agent.Middleware{&contextMaintenanceMiddleware{BaseMiddleware: &agent.BaseMiddleware{}, agentKind: "ide"}},
			})
			if err != nil {
				t.Fatal(err)
			}
			messages := pressureHistory(10, 9000, agent.ToolResultDeferred, agent.ToolResultContextDiscardable)
			messages = append(messages, agent.UserMessage(fmt.Sprintf("cold suffix %s", strings.Repeat("cold ", 8_000))))
			ctx := context.WithValue(context.Background(), contextCompactionContextKey{}, &contextCompactionController{conversation: conversation})
			if test.nativeExecutor != nil {
				ctx = contextWithToolResultCleanupExecutor(ctx, test.nativeExecutor)
			}
			iterator := built.Run(ctx, &agent.AgentInput{Messages: messages})
			for {
				event, ok := iterator.Next()
				if !ok {
					break
				}
				if event.Err != nil && event.Err != io.EOF {
					t.Fatal(event.Err)
				}
			}
			if model.calls != 2 || conversation.stageCalls != 1 {
				t.Fatalf("calls after failed cleanup = model:%d stage:%d", model.calls, conversation.stageCalls)
			}
			if test.nativeExecutor != nil && test.nativeExecutor.calls != 1 {
				t.Fatalf("native cleanup attempts = %d, want 1", test.nativeExecutor.calls)
			}
		})
	}
}

func TestContextMaintenancePlannerRoutesHardPressureThroughSnapshotFork(t *testing.T) {
	messages := []*agent.Message{
		agent.SystemMessage("stable"),
		agent.UserMessage(strings.Repeat("non-cleanable history ", 800)),
		agent.UserMessage("current"),
	}
	policy := pressureTestPolicy(2_000)
	conversation := &maintenancePlanningConversation{
		policy: policy,
		compacted: []*agent.Message{
			agentcontext.NewCompactionSummaryMessage(1, "checkpoint"),
			agent.UserMessage("current"),
		},
	}
	model := &compactionForkCaptureModel{response: agent.AssistantMessage("unused", nil)}
	call := &agent.ModelCall{Model: model, Messages: messages, Options: []agent.ModelOption{agent.WithTools(nil)}}
	var events []agentrun.Event
	next, result, err := prepareContextMaintenance(
		context.Background(),
		&contextCompactionController{conversation: conversation, emit: func(event agentrun.Event) { events = append(events, event) }},
		call,
		&agent.ModelContext{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Triggered || result.Action != agentcontext.ContextMaintenanceCompaction || conversation.compactionRuns != 1 {
		t.Fatalf("maintenance result = %#v compaction_calls=%d", result, conversation.compactionRuns)
	}
	if conversation.compactionIn.Force || !conversation.compactionIn.Planned || conversation.compactionIn.Phase != agentcompaction.PhaseModelStep || !conversation.sawSnapshot {
		t.Fatalf("compaction input did not preserve the model-step snapshot: %#v snapshot=%t", conversation.compactionIn, conversation.sawSnapshot)
	}
	if len(next.Messages) != 2 || !agentcontext.IsCompactionSummaryMessage(next.Messages[0]) || next.Messages[1].Content != "current" {
		t.Fatalf("rewritten primary request = %#v", next.Messages)
	}
	if model.requests != 0 {
		t.Fatalf("conversation fake owns compaction; primary model calls = %d", model.requests)
	}
	cleanupEvents := 0
	for _, event := range events {
		if event.Type != "context_cleanup" {
			continue
		}
		cleanupEvents++
		data, ok := event.Data.(map[string]any)
		if !ok || data["status"] != "skipped" || data["action"] != agentcontext.ContextMaintenanceCompaction {
			t.Fatalf("hard-pressure cleanup attribution = %#v", event.Data)
		}
	}
	if cleanupEvents != 1 {
		t.Fatalf("hard-pressure cleanup events = %d, want 1: %#v", cleanupEvents, events)
	}
}

func TestContextMaintenanceHardOverflowCleansToolResultsBeforeCompaction(t *testing.T) {
	messages := pressureHistory(5, 9000, agent.ToolResultDeferred, agent.ToolResultContextNormal)
	messages = append(messages[:len(messages)-1], agent.UserMessage(strings.Repeat("non-cleanable history ", 30_000)), messages[len(messages)-1])
	policy := pressureTestPolicy(40_000)
	policy.ProviderCacheState = agentcontext.ProviderCacheWarm
	conversation := &maintenancePlanningConversation{
		policy: policy,
		compacted: []*agent.Message{
			agentcontext.NewCompactionSummaryMessage(1, "checkpoint"),
			agent.UserMessage("current turn"),
		},
	}
	call := &agent.ModelCall{
		Model: &compactionForkCaptureModel{}, Messages: messages,
		Options: []agent.ModelOption{agent.WithTools(nil)},
	}

	next, result, err := prepareContextMaintenance(
		context.Background(), &contextCompactionController{conversation: conversation}, call, &agent.ModelContext{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Action != agentcontext.ContextMaintenanceCompaction || conversation.compactionRuns != 1 {
		t.Fatalf("maintenance result=%#v compaction_runs=%d", result, conversation.compactionRuns)
	}
	if conversation.stageCalls != 0 {
		t.Fatalf("cleanup superseded by compaction must not stage a second structural record: %d", conversation.stageCalls)
	}
	if len(result.Cleanup.Cleanup.Replacements) == 0 {
		t.Fatalf("hard overflow did not plan tool-result cleanup: %#v", result.Cleanup)
	}
	for _, replacement := range result.Cleanup.Cleanup.Replacements {
		if got := conversation.compactionIn.Messages[replacement.MessageIndex].Content; got != replacement.Placeholder {
			t.Fatalf("compaction input kept rich tool result at %d: %q", replacement.MessageIndex, got)
		}
	}
	if len(next.Messages) != 2 || !agentcontext.IsCompactionSummaryMessage(next.Messages[0]) {
		t.Fatalf("compacted request = %#v", next.Messages)
	}
}

func TestContextMaintenanceAdvancesCompactionBeforeForkCapacityIsExhausted(t *testing.T) {
	messages := []*agent.Message{agent.SystemMessage("stable"), agent.UserMessage(strings.Repeat("x", 120_000))}
	policy := pressureTestPolicy(100_000)
	conversation := &maintenancePlanningConversation{
		policy:    policy,
		compacted: []*agent.Message{agentcontext.NewCompactionSummaryMessage(1, "checkpoint")},
	}
	model := &compactionForkCaptureModel{response: agent.AssistantMessage("unused", nil)}
	call := &agent.ModelCall{
		Model: model, Messages: messages,
		Options: []agent.ModelOption{agent.WithTools(nil), agent.WithMaxTokens(70_000)},
	}
	_, result, err := prepareContextMaintenance(
		context.Background(), &contextCompactionController{conversation: conversation}, call, &agent.ModelContext{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Triggered || result.Action != agentcontext.ContextMaintenanceCompaction || result.Cleanup.Reason != "compaction_capacity_reserve" {
		t.Fatalf("capacity maintenance = %#v", result)
	}
	if conversation.compactionIn.TriggerReason != "compaction_capacity_reserve" {
		t.Fatalf("trigger reason = %q", conversation.compactionIn.TriggerReason)
	}
}
