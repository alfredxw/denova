package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type receiptProbeMiddleware struct {
	BaseMiddleware
	mu     sync.Mutex
	events []string
}

func (middleware *receiptProbeMiddleware) WrapToolCall(
	_ context.Context,
	endpoint ToolCallEndpoint,
	toolCtx *ToolContext,
) (ToolCallEndpoint, error) {
	return func(ctx context.Context, arguments string, options ...ToolOption) (ToolResult, error) {
		middleware.mu.Lock()
		middleware.events = append(middleware.events, "start:"+toolCtx.ExecutionID)
		middleware.mu.Unlock()
		result, err := endpoint(ctx, arguments, options...)
		status := "success"
		if err != nil {
			status = "error"
		}
		middleware.mu.Lock()
		middleware.events = append(middleware.events, "finish:"+toolCtx.ExecutionID+":"+status)
		middleware.mu.Unlock()
		return result, err
	}, nil
}

func (middleware *receiptProbeMiddleware) snapshot() []string {
	middleware.mu.Lock()
	defer middleware.mu.Unlock()
	return append([]string(nil), middleware.events...)
}

func TestConcreteToolPanicCrossesLifecycleWrapperAsPairedError(t *testing.T) {
	model := &scriptedModel{responses: []scriptedModelResponse{
		{message: AssistantMessage("", []ToolCall{{ID: "panic-call", Type: "function", Function: FunctionCall{Name: "panic_tool", Arguments: `{}`}}})},
		{message: AssistantMessage("recovered", nil)},
	}}
	tool := &functionTool{name: "panic_tool", run: func(context.Context, string) (string, error) {
		panic("effect panic")
	}}
	receipts := &receiptProbeMiddleware{}
	native, err := newModelToolLoop(context.Background(), loopConfig{
		Name: "panic-receipt", Model: model, Tools: []ToolDefinition{testToolDefinition(tool)},
		Middlewares: []Middleware{receipts},
	})
	if err != nil {
		t.Fatal(err)
	}
	iterator := newLoopRunner(loopRunnerConfig{Agent: native}).Query(context.Background(), "go")
	var result *Message
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			t.Fatal(event.Err)
		}
		if event.Output != nil && event.Output.MessageOutput != nil && event.Output.MessageOutput.Role == ToolRole {
			result = event.Output.MessageOutput.Message
		}
	}
	lifecycle := receipts.snapshot()
	if len(lifecycle) != 2 || !strings.HasPrefix(lifecycle[0], "start:tool-") ||
		lifecycle[1] != "finish:"+strings.TrimPrefix(lifecycle[0], "start:")+":error" {
		t.Fatalf("lifecycle receipts = %v", lifecycle)
	}
	if result == nil || result.ToolResult == nil || result.ToolResult.Status != ToolResultError || !strings.Contains(result.Content, "panic recovered: effect panic") {
		t.Fatalf("paired panic result = %#v", result)
	}
}

type cancelBeforeToolExecutionMiddleware struct {
	BaseMiddleware
	cancel context.CancelFunc
}

func (middleware *cancelBeforeToolExecutionMiddleware) WrapToolCall(
	_ context.Context,
	endpoint ToolCallEndpoint,
	_ *ToolContext,
) (ToolCallEndpoint, error) {
	middleware.cancel()
	return endpoint, nil
}

func TestNativeLoopChecksContextImmediatelyBeforeToolExecution(t *testing.T) {
	var calls atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	model := &scriptedModel{responses: []scriptedModelResponse{{message: AssistantMessage("", []ToolCall{{
		ID: "target", Type: "function", Function: FunctionCall{Name: "target", Arguments: `{}`},
	}})}}}
	definition := testToolDefinition(&functionTool{name: "target", run: func(context.Context, string) (string, error) {
		calls.Add(1)
		return "unexpected", nil
	}})
	native, err := newModelToolLoop(context.Background(), loopConfig{
		Name: "cancel-before-tool", Model: model, Tools: []ToolDefinition{definition},
		Middlewares: []Middleware{&cancelBeforeToolExecutionMiddleware{cancel: cancel}},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := newLoopRunner(loopRunnerConfig{Agent: native}).Query(ctx, "go")
	var canceled bool
	for {
		event, ok := events.Next()
		if !ok {
			break
		}
		if errors.Is(event.Err, context.Canceled) {
			canceled = true
		}
	}
	if !canceled || calls.Load() != 0 {
		t.Fatalf("canceled=%t tool calls=%d", canceled, calls.Load())
	}
}

type blockBeforeConcreteExecutionMiddleware struct{ BaseMiddleware }

func (*blockBeforeConcreteExecutionMiddleware) WrapToolCall(
	_ context.Context,
	_ ToolCallEndpoint,
	_ *ToolContext,
) (ToolCallEndpoint, error) {
	return func(context.Context, string, ...ToolOption) (ToolResult, error) {
		return SyntheticToolResult(ToolResultBlocked, ToolSyntheticPolicyBlocked, "approval denied"), nil
	}, nil
}

func TestToolStartedEventIsEmittedOnlyAfterMiddlewarePreflight(t *testing.T) {
	var calls atomic.Int32
	model := &scriptedModel{responses: []scriptedModelResponse{
		{message: AssistantMessage("", []ToolCall{{ID: "blocked", Type: "function", Function: FunctionCall{Name: "target", Arguments: `{}`}}})},
		{message: AssistantMessage("done", nil)},
	}}
	definition := testToolDefinition(&functionTool{name: "target", run: func(context.Context, string) (string, error) {
		calls.Add(1)
		return "unexpected", nil
	}})
	native, err := newModelToolLoop(context.Background(), loopConfig{
		Name: "preflight-before-start", Model: model, Tools: []ToolDefinition{definition},
		Middlewares: []Middleware{&blockBeforeConcreteExecutionMiddleware{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := newLoopRunner(loopRunnerConfig{Agent: native}).Query(context.Background(), "go")
	var started, finished int
	for {
		event, ok := events.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			t.Fatal(event.Err)
		}
		if event.Output == nil || event.Output.ToolExecution == nil {
			continue
		}
		switch event.Output.ToolExecution.Phase {
		case toolExecutionStarted:
			started++
		case toolExecutionFinished:
			finished++
		}
	}
	if calls.Load() != 0 || started != 0 || finished != 1 {
		t.Fatalf("calls=%d started=%d finished=%d", calls.Load(), started, finished)
	}
}

type rewriteArgumentsMiddleware struct{ BaseMiddleware }

func (*rewriteArgumentsMiddleware) WrapToolCall(_ context.Context, endpoint ToolCallEndpoint, _ *ToolContext) (ToolCallEndpoint, error) {
	return func(ctx context.Context, _ string, options ...ToolOption) (ToolResult, error) {
		return endpoint(ctx, `{"value":"wrong"}`, options...)
	}, nil
}

func TestMiddlewareArgumentRewriteCannotBypassSchema(t *testing.T) {
	var calls atomic.Int32
	tool, err := InferTool("typed", "", func(context.Context, struct {
		Value int `json:"value"`
	}) (string, error) {
		calls.Add(1)
		return "ok", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &scriptedModel{responses: []scriptedModelResponse{
		{message: AssistantMessage("", []ToolCall{{ID: "typed", Type: "function", Function: FunctionCall{Name: "typed", Arguments: `{"value":1}`}}})},
		{message: AssistantMessage("done", nil)},
	}}
	native, err := newModelToolLoop(context.Background(), loopConfig{
		Name: "schema-rewrite", Model: model, Tools: []ToolDefinition{testToolDefinition(tool)},
		Middlewares: []Middleware{&rewriteArgumentsMiddleware{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := newLoopRunner(loopRunnerConfig{Agent: native}).Query(context.Background(), "go")
	var result *Message
	for {
		event, ok := events.Next()
		if !ok {
			break
		}
		if event.Output != nil && event.Output.MessageOutput != nil && event.Output.MessageOutput.Role == ToolRole {
			result = event.Output.MessageOutput.Message
		}
	}
	if calls.Load() != 0 || result == nil || result.ToolResult == nil || result.ToolResult.Status != ToolResultError {
		t.Fatalf("calls=%d result=%#v", calls.Load(), result)
	}
}

func TestInvalidArgumentsRemainModelVisibleAndCanBeRetried(t *testing.T) {
	var calls atomic.Int32
	tool, err := InferTool("typed_retry", "", func(_ context.Context, input struct {
		Value int `json:"value"`
	}) (string, error) {
		calls.Add(1)
		return fmt.Sprintf("value=%d", input.Value), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &scriptedModel{responses: []scriptedModelResponse{
		{message: AssistantMessage("", []ToolCall{{ID: "bad", Type: "function", Function: FunctionCall{Name: "typed_retry", Arguments: `{}`}}})},
		{message: AssistantMessage("", []ToolCall{{ID: "fixed", Type: "function", Function: FunctionCall{Name: "typed_retry", Arguments: `{"value":"7"}`}}})},
		{message: AssistantMessage("done", nil)},
	}}
	native, err := newModelToolLoop(context.Background(), loopConfig{
		Name: "argument-retry", Model: model, Tools: []ToolDefinition{testToolDefinition(tool)},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := newLoopRunner(loopRunnerConfig{Agent: native}).Query(context.Background(), "go")
	var invalid, successful *Message
	for {
		event, ok := events.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			t.Fatal(event.Err)
		}
		if event.Output == nil || event.Output.MessageOutput == nil || event.Output.MessageOutput.Message == nil {
			continue
		}
		message := event.Output.MessageOutput.Message
		if message.ToolResult == nil {
			continue
		}
		if message.ToolResult.SyntheticReason == ToolSyntheticInvalidArguments {
			invalid = message
		} else if message.ToolResult.Status == ToolResultSuccess {
			successful = message
		}
	}
	if calls.Load() != 1 || invalid == nil || successful == nil ||
		!strings.Contains(invalid.Content, `"code":"invalid_arguments"`) ||
		!strings.Contains(invalid.Content, `"code":"missing_required"`) {
		t.Fatalf("calls=%d invalid=%#v successful=%#v", calls.Load(), invalid, successful)
	}
}

func TestMalformedArgumentsRemainModelVisibleAndCanBeRetried(t *testing.T) {
	var calls atomic.Int32
	tool, err := InferTool("typed_malformed_retry", "", func(_ context.Context, input struct {
		Value int `json:"value"`
	}) (string, error) {
		calls.Add(1)
		return fmt.Sprintf("value=%d", input.Value), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &scriptedModel{responses: []scriptedModelResponse{
		{message: AssistantMessage("", []ToolCall{{
			ID: "bad", Type: "function",
			Function: FunctionCall{Name: "typed_malformed_retry", Arguments: `[`},
		}})},
		{message: AssistantMessage("", []ToolCall{{
			ID: "fixed", Type: "function",
			Function: FunctionCall{Name: "typed_malformed_retry", Arguments: `{"value":7}`},
		}})},
		{message: AssistantMessage("done", nil)},
	}}
	native, err := newModelToolLoop(context.Background(), loopConfig{
		Name: "malformed-argument-retry", Model: model, Tools: []ToolDefinition{testToolDefinition(tool)},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := newLoopRunner(loopRunnerConfig{Agent: native}).Query(context.Background(), "go")
	for {
		event, ok := events.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			t.Fatal(event.Err)
		}
	}
	inputs := model.capturedInputs()
	if len(inputs) != 3 || len(inputs[1]) != 3 {
		t.Fatalf("model inputs = %#v", inputs)
	}
	failedCall, failedResult := inputs[1][1], inputs[1][2]
	if len(failedCall.ToolCalls) != 1 || failedCall.ToolCalls[0].Function.Arguments != `{}` {
		t.Fatalf("failed call was not projected safely: %#v", failedCall)
	}
	if failedResult.ToolResult == nil || failedResult.ToolResult.SyntheticReason != ToolSyntheticInvalidArguments ||
		!strings.Contains(failedResult.Content, `"received_arguments":"["`) {
		t.Fatalf("invalid argument feedback was not preserved: %#v", failedResult)
	}
	if calls.Load() != 1 {
		t.Fatalf("tool calls = %d, want 1", calls.Load())
	}
}

func TestRepairedArgumentsUseTheExecutedCanonicalValueInModelContext(t *testing.T) {
	var executed string
	tool := &functionTool{name: "repairable", run: func(_ context.Context, arguments string) (string, error) {
		executed = arguments
		return "ok", nil
	}}
	model := &scriptedModel{responses: []scriptedModelResponse{
		{message: AssistantMessage("", []ToolCall{{
			ID: "repair", Type: "function",
			Function: FunctionCall{Name: "repairable", Arguments: `{value: '7'}`},
		}})},
		{message: AssistantMessage("done", nil)},
	}}
	native, err := newModelToolLoop(context.Background(), loopConfig{
		Name: "repairable-arguments", Model: model, Tools: []ToolDefinition{testToolDefinition(tool)},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := newLoopRunner(loopRunnerConfig{Agent: native}).Query(context.Background(), "go")
	for {
		event, ok := events.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			t.Fatal(event.Err)
		}
	}
	inputs := model.capturedInputs()
	if len(inputs) != 2 || len(inputs[1]) != 3 || len(inputs[1][1].ToolCalls) != 1 {
		t.Fatalf("model inputs = %#v", inputs)
	}
	retained := inputs[1][1].ToolCalls[0].Function.Arguments
	if retained != executed || retained != `{"value":"7"}` {
		t.Fatalf("retained arguments = %q, executed = %q", retained, executed)
	}
}

func TestCanonicalToolBatchBoundariesBracketExecution(t *testing.T) {
	var calls atomic.Int32
	var executed string
	tool := &functionTool{name: "boundary_tool", run: func(_ context.Context, arguments string) (string, error) {
		calls.Add(1)
		executed = arguments
		return "complete", nil
	}}
	model := &scriptedModel{responses: []scriptedModelResponse{
		{message: AssistantMessage("", []ToolCall{{
			ID: "boundary-call", Type: "function",
			Function: FunctionCall{Name: "boundary_tool", Arguments: `{value: 'ready'}`},
		}})},
		{message: AssistantMessage("done", nil)},
	}}
	native, err := newModelToolLoop(context.Background(), loopConfig{
		Name: "canonical-boundary", Model: model, Tools: []ToolDefinition{testToolDefinition(tool)},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := contextWithToolStartReceipt(context.Background())
	events := newLoopRunner(loopRunnerConfig{Agent: native}).Query(ctx, "go")
	var phases []toolBatchPhase
	for {
		event, ok := events.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			t.Fatal(event.Err)
		}
		if event.Output == nil {
			continue
		}
		if boundary := event.Output.ToolBatch; boundary != nil {
			phase, messages := boundary.snapshot()
			phases = append(phases, phase)
			switch phase {
			case toolBatchPrepared:
				if calls.Load() != 0 || len(messages) != 1 || messages[0].ToolCalls[0].Function.Arguments != `{"value":"ready"}` {
					t.Fatalf("prepared boundary calls=%d messages=%#v", calls.Load(), messages)
				}
			case toolBatchCompleted:
				if calls.Load() != 1 || len(messages) != 2 || messages[1].Role != ToolRole || messages[1].ToolCallID != "boundary-call" {
					t.Fatalf("completed boundary calls=%d messages=%#v", calls.Load(), messages)
				}
			default:
				t.Fatalf("unexpected boundary phase %q", phase)
			}
			boundary.acknowledge(nil)
		}
		if execution := event.Output.ToolExecution; execution != nil && execution.Phase == toolExecutionStarted {
			if calls.Load() != 0 {
				t.Fatalf("tool ran before its start receipt: calls=%d", calls.Load())
			}
			execution.acknowledgeStart(nil)
		}
	}
	if len(phases) != 2 || phases[0] != toolBatchPrepared || phases[1] != toolBatchCompleted || executed != `{"value":"ready"}` {
		t.Fatalf("canonical boundary phases=%v executed=%q", phases, executed)
	}
}

type progressOnlyTool struct{}

func (*progressOnlyTool) Info(context.Context) (*ToolInfo, error) {
	return GoStruct2ToolInfo[struct{}]("progress", "")
}

func (*progressOnlyTool) Run(ctx context.Context, _ string, _ ...ToolOption) (ToolResult, error) {
	EmitToolProgress(ctx, "first ")
	EmitToolProgress(ctx, "second")
	return ToolResult{Status: ToolResultSuccess}, nil
}

func TestProgressCollectorBuildsMissingFinalContent(t *testing.T) {
	model := &scriptedModel{responses: []scriptedModelResponse{
		{message: AssistantMessage("", []ToolCall{{ID: "p", Type: "function", Function: FunctionCall{Name: "progress", Arguments: `{}`}}})},
		{message: AssistantMessage("done", nil)},
	}}
	native, err := newModelToolLoop(context.Background(), loopConfig{Name: "progress", Model: model, Tools: []ToolDefinition{testToolDefinition(&progressOnlyTool{})}})
	if err != nil {
		t.Fatal(err)
	}
	events := newLoopRunner(loopRunnerConfig{Agent: native}).Query(context.Background(), "go")
	var progress strings.Builder
	var result *Message
	for {
		event, ok := events.Next()
		if !ok {
			break
		}
		if event.Output != nil && event.Output.ToolExecution != nil && event.Output.ToolExecution.Phase == toolExecutionProgress {
			progress.WriteString(event.Output.ToolExecution.Delta)
		}
		if event.Output != nil && event.Output.MessageOutput != nil && event.Output.MessageOutput.Role == ToolRole {
			result = event.Output.MessageOutput.Message
		}
	}
	if progress.String() != "first second" || result == nil || result.Content != "first second" {
		t.Fatalf("progress=%q result=%#v", progress.String(), result)
	}
}

type overflowingProgressTool struct{}

func (*overflowingProgressTool) Info(context.Context) (*ToolInfo, error) {
	return GoStruct2ToolInfo[struct{}]("overflowing_progress", "")
}

func (*overflowingProgressTool) Run(ctx context.Context, _ string, _ ...ToolOption) (ToolResult, error) {
	for range 100 {
		EmitToolProgress(ctx, "0123456789")
	}
	return ToolResult{Status: ToolResultSuccess}, nil
}

func TestProgressCollectorEmitsOneBoundedTruncationAndStops(t *testing.T) {
	model := &scriptedModel{responses: []scriptedModelResponse{
		{message: AssistantMessage("", []ToolCall{{ID: "p", Type: "function", Function: FunctionCall{Name: "overflowing_progress", Arguments: `{}`}}})},
		{message: AssistantMessage("done", nil)},
	}}
	definition := testToolDefinition(&overflowingProgressTool{})
	definition.Descriptor.MaxResultBytes = 64
	native, err := newModelToolLoop(context.Background(), loopConfig{Name: "progress", Model: model, Tools: []ToolDefinition{definition}})
	if err != nil {
		t.Fatal(err)
	}
	events := newLoopRunner(loopRunnerConfig{Agent: native}).Query(context.Background(), "go")
	var progress strings.Builder
	progressEvents := 0
	var result *Message
	for {
		event, ok := events.Next()
		if !ok {
			break
		}
		if event.Output != nil && event.Output.ToolExecution != nil && event.Output.ToolExecution.Phase == toolExecutionProgress {
			progress.WriteString(event.Output.ToolExecution.Delta)
			progressEvents++
		}
		if event.Output != nil && event.Output.MessageOutput != nil && event.Output.MessageOutput.Role == ToolRole {
			result = event.Output.MessageOutput.Message
		}
	}
	if progress.Len() > definition.Descriptor.MaxResultBytes+len(toolProgressTruncatedMarker) {
		t.Fatalf("progress has %d bytes: %q", progress.Len(), progress.String())
	}
	if strings.Count(progress.String(), toolProgressTruncatedMarker) != 1 || !strings.HasSuffix(progress.String(), toolProgressTruncatedMarker) {
		t.Fatalf("progress truncation = %q", progress.String())
	}
	if progressEvents >= 100 {
		t.Fatalf("progress continued after truncation: events=%d", progressEvents)
	}
	if result == nil || len(result.Content) > definition.Descriptor.MaxResultBytes || !strings.HasSuffix(result.Content, toolProgressTruncatedMarker) {
		t.Fatalf("bounded final progress result = %#v", result)
	}
}
