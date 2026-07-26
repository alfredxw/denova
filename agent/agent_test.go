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
)

type scriptedModelResponse struct {
	message *Message
	chunks  []*Message
	err     error
}

type scriptedModel struct {
	mu         sync.Mutex
	responses  []scriptedModelResponse
	inputs     [][]*Message
	toolCounts []int
}

func (model *scriptedModel) next(input []*Message, opts []ModelOption) (scriptedModelResponse, error) {
	model.mu.Lock()
	defer model.mu.Unlock()
	model.inputs = append(model.inputs, cloneMessages(input))
	model.toolCounts = append(model.toolCounts, len(GetCommonOptions(nil, opts...).Tools))
	if len(model.responses) == 0 {
		return scriptedModelResponse{}, errors.New("scripted model exhausted")
	}
	response := model.responses[0]
	model.responses = model.responses[1:]
	return response, nil
}

func (model *scriptedModel) Generate(_ context.Context, input []*Message, opts ...ModelOption) (*Message, error) {
	response, err := model.next(input, opts)
	if err != nil {
		return nil, err
	}
	if response.err != nil {
		return nil, response.err
	}
	if response.message != nil {
		return response.message.Clone(), nil
	}
	return ConcatMessages(cloneMessages(response.chunks))
}

func (model *scriptedModel) Stream(_ context.Context, input []*Message, opts ...ModelOption) (*StreamReader[*Message], error) {
	response, err := model.next(input, opts)
	if err != nil {
		return nil, err
	}
	if response.err != nil {
		return nil, response.err
	}
	if response.message != nil {
		return StreamReaderFromArray([]*Message{response.message.Clone()}), nil
	}
	return StreamReaderFromArray(cloneMessages(response.chunks)), nil
}

func (model *scriptedModel) capturedInputs() [][]*Message {
	model.mu.Lock()
	defer model.mu.Unlock()
	result := make([][]*Message, len(model.inputs))
	for index := range model.inputs {
		result[index] = cloneMessages(model.inputs[index])
	}
	return result
}

type functionTool struct {
	name string
	run  func(context.Context, string) (string, error)
}

func (tool *functionTool) Info(context.Context) (*ToolInfo, error) {
	return &ToolInfo{Name: tool.name, Desc: tool.name}, nil
}

func (tool *functionTool) Run(ctx context.Context, arguments string, _ ...ToolOption) (ToolResult, error) {
	content, err := tool.run(ctx, arguments)
	if err != nil {
		return ToolResult{}, err
	}
	return TextToolResult(content), nil
}

func testToolDefinition(tool Tool) ToolDefinition {
	return ToolDefinition{Tool: tool, Descriptor: ToolDescriptor{
		Source: ToolSourceRead, Execution: ToolExecutionParallelRead,
		MutationScope: ToolMutationNone, PostCheck: ToolPostCheckNone,
		Recovery: ToolRecoveryReadOnly, ResultProjection: ToolResultBoundedModelContext,
		Steering: SteeringFinishCurrent, MaxResultBytes: 1 << 20,
	}}
}

type panicBeforeAgentMiddleware struct {
	BaseMiddleware
}

func (*panicBeforeAgentMiddleware) BeforeAgent(context.Context, *RunContext) (context.Context, *RunContext, error) {
	panic("before agent panic")
}

func TestNativeLoopReportsRecoveredRunPanicBeforeClosingIterator(t *testing.T) {
	model := &scriptedModel{responses: []scriptedModelResponse{{message: AssistantMessage("unused", nil)}}}
	agent, err := NewAgent(context.Background(), AgentConfig{
		Name:        "panic",
		Model:       model,
		Middlewares: []Middleware{&panicBeforeAgentMiddleware{}},
	})
	if err != nil {
		t.Fatal(err)
	}

	iterator := agent.Run(context.Background(), &AgentInput{Messages: []*Message{UserMessage("go")}})
	event, ok := iterator.Next()
	if !ok {
		t.Fatal("missing recovered panic event")
	}
	var panicErr *PanicError
	if !errors.As(event.Err, &panicErr) || panicErr.Value != "before agent panic" {
		t.Fatalf("panic event = %#v", event)
	}
	if _, ok := iterator.Next(); ok {
		t.Fatal("unexpected event after recovered panic")
	}
}

func TestNativeLoopSingleStreamingTurn(t *testing.T) {
	model := &scriptedModel{responses: []scriptedModelResponse{{chunks: []*Message{
		{Role: Assistant, Content: "hel", ReasoningContent: "rea"},
		{Content: "lo", ReasoningContent: "son"},
		{ResponseMeta: &ResponseMeta{FinishReason: "stop", Usage: &TokenUsage{TotalTokens: 9}}},
	}}}}
	agent, err := NewAgent(context.Background(), AgentConfig{Name: "writer", Instruction: "be useful", Model: model})
	if err != nil {
		t.Fatal(err)
	}
	iterator := NewRunner(RunnerConfig{Agent: agent, EnableStreaming: true}).Query(context.Background(), "hi")
	event, ok := iterator.Next()
	if !ok || event.Err != nil || event.Output == nil || event.Output.MessageOutput == nil || !event.Output.MessageOutput.IsStreaming {
		t.Fatalf("assistant event = %#v", event)
	}
	message, err := event.Output.MessageOutput.GetMessage()
	if err != nil {
		t.Fatal(err)
	}
	if message.Content != "hello" || message.ReasoningContent != "reason" ||
		message.ResponseMeta == nil || message.ResponseMeta.Usage.TotalTokens != 9 {
		t.Fatalf("assistant = %#v", message)
	}
	if _, ok := iterator.Next(); ok {
		t.Fatal("unexpected event after final answer")
	}
	inputs := model.capturedInputs()
	if len(inputs) != 1 || len(inputs[0]) != 2 || inputs[0][0].Role != System || inputs[0][1].Role != User {
		t.Fatalf("model input = %#v", inputs)
	}
}

func TestNativeLoopConcurrentToolsKeepResultAndTranscriptSourceOrder(t *testing.T) {
	model := &scriptedModel{responses: []scriptedModelResponse{
		{message: AssistantMessage("", []ToolCall{
			{ID: "slow-id", Type: "function", Function: FunctionCall{Name: "slow", Arguments: `{}`}},
			{ID: "fast-id", Type: "function", Function: FunctionCall{Name: "fast", Arguments: `{}`}},
		})},
		{message: AssistantMessage("done", nil)},
	}}
	started := make(chan string, 2)
	release := make(chan struct{})
	makeTool := func(name string) ToolDefinition {
		return testToolDefinition(&functionTool{name: name, run: func(ctx context.Context, _ string) (string, error) {
			if ToolCallID(ctx) != name+"-id" || ToolName(ctx) != name {
				return "", fmt.Errorf("missing tool context for %s: id=%q name=%q", name, ToolCallID(ctx), ToolName(ctx))
			}
			started <- name
			<-release
			return "result-" + name, nil
		}})
	}
	agent, err := NewAgent(context.Background(), AgentConfig{Name: "parallel", Model: model, Tools: []ToolDefinition{makeTool("slow"), makeTool("fast")}})
	if err != nil {
		t.Fatal(err)
	}
	iterator := NewRunner(RunnerConfig{Agent: agent}).Query(context.Background(), "go")
	first, ok := iterator.Next()
	if !ok || first.Output.MessageOutput.Message.Role != Assistant {
		t.Fatalf("first event = %#v", first)
	}
	for count := 0; count < 2; count++ {
		select {
		case <-started:
		case <-time.After(200 * time.Millisecond):
			t.Fatal("tool calls did not start concurrently")
		}
	}
	close(release)

	var toolEvents []*Message
	var final *Message
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			t.Fatal(event.Err)
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		variant := event.Output.MessageOutput
		if variant.Role == ToolRole {
			toolEvents = append(toolEvents, variant.Message)
		} else if variant.Role == Assistant {
			final = variant.Message
		}
	}
	if len(toolEvents) != 2 || toolEvents[0].ToolName != "slow" || toolEvents[1].ToolName != "fast" {
		t.Fatalf("tool event order = %#v", toolEvents)
	}
	if final == nil || final.Content != "done" {
		t.Fatalf("final = %#v", final)
	}
	inputs := model.capturedInputs()
	if len(inputs) != 2 {
		t.Fatalf("model calls = %d", len(inputs))
	}
	second := inputs[1]
	if len(second) != 4 || second[2].ToolName != "slow" || second[3].ToolName != "fast" {
		t.Fatalf("source transcript order changed: %#v", second)
	}
	if model.toolCounts[0] != 2 {
		t.Fatalf("model saw %d tools", model.toolCounts[0])
	}
}

func TestNativeLoopMergesStreamingToolCalls(t *testing.T) {
	zero, one := 0, 1
	model := &scriptedModel{responses: []scriptedModelResponse{
		{chunks: []*Message{
			{Role: Assistant, ToolCalls: []ToolCall{
				{Index: &one, ID: "b", Type: "function", Function: FunctionCall{Name: "beta", Arguments: `{"value":`}},
				{Index: &zero, ID: "a", Type: "function", Function: FunctionCall{Name: "alpha", Arguments: `{"value":`}},
			}},
			{ToolCalls: []ToolCall{
				{Index: &zero, Function: FunctionCall{Arguments: `1}`}},
				{Index: &one, Function: FunctionCall{Arguments: `2}`}},
			}},
			{ResponseMeta: &ResponseMeta{FinishReason: "tool_calls"}},
		}},
		{chunks: []*Message{{Role: Assistant, Content: "done"}, {ResponseMeta: &ResponseMeta{FinishReason: "stop"}}}},
	}}
	var mu sync.Mutex
	arguments := map[string]string{}
	makeTool := func(name string) ToolDefinition {
		return testToolDefinition(&functionTool{name: name, run: func(_ context.Context, raw string) (string, error) {
			mu.Lock()
			arguments[name] = raw
			mu.Unlock()
			return name, nil
		}})
	}
	agent, err := NewAgent(context.Background(), AgentConfig{Name: "stream-tools", Model: model, Tools: []ToolDefinition{makeTool("alpha"), makeTool("beta")}})
	if err != nil {
		t.Fatal(err)
	}
	iterator := NewRunner(RunnerConfig{Agent: agent, EnableStreaming: true}).Query(context.Background(), "go")
	var roles []RoleType
	var toolNames []string
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			t.Fatal(event.Err)
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		variant := event.Output.MessageOutput
		roles = append(roles, variant.Role)
		message, err := variant.GetMessage()
		if err != nil {
			t.Fatal(err)
		}
		if variant.Role == ToolRole {
			toolNames = append(toolNames, message.ToolName)
		}
	}
	if fmt.Sprint(roles) != fmt.Sprint([]RoleType{Assistant, ToolRole, ToolRole, Assistant}) {
		t.Fatalf("event roles = %#v", roles)
	}
	if fmt.Sprint(toolNames) != fmt.Sprint([]string{"alpha", "beta"}) {
		t.Fatalf("tool names = %#v", toolNames)
	}
	mu.Lock()
	defer mu.Unlock()
	if arguments["alpha"] != `{"value":1}` || arguments["beta"] != `{"value":2}` {
		t.Fatalf("merged arguments = %#v", arguments)
	}
}

func TestNativeLoopToolFailuresBecomeToolMessages(t *testing.T) {
	tests := []struct {
		name        string
		call        ToolCall
		tools       func(*atomic.Int32) []ToolDefinition
		finish      string
		wantContent string
		wantReason  ToolSyntheticReason
		wantCalls   int32
	}{
		{
			name: "unknown", call: ToolCall{ID: "1", Type: "function", Function: FunctionCall{Name: "missing", Arguments: `{}`}},
			tools: func(*atomic.Int32) []ToolDefinition { return nil }, wantContent: "unknown tool", wantReason: ToolSyntheticUnknownTool, wantCalls: 0,
		},
		{
			name: "invalid arguments", call: ToolCall{ID: "1", Type: "function", Function: FunctionCall{Name: "typed", Arguments: `{"value":"bad"}`}},
			tools: func(count *atomic.Int32) []ToolDefinition {
				current, err := InferTool("typed", "", func(context.Context, struct {
					Value int `json:"value"`
				}) (string, error) {
					count.Add(1)
					return "ok", nil
				})
				if err != nil {
					panic(err)
				}
				return []ToolDefinition{testToolDefinition(current)}
			},
			wantContent: "invalid arguments", wantReason: ToolSyntheticInvalidArguments, wantCalls: 0,
		},
		{
			name: "panic", call: ToolCall{ID: "1", Type: "function", Function: FunctionCall{Name: "panic", Arguments: `{}`}},
			tools: func(count *atomic.Int32) []ToolDefinition {
				return []ToolDefinition{testToolDefinition(&functionTool{name: "panic", run: func(context.Context, string) (string, error) {
					count.Add(1)
					panic("boom")
				}})}
			},
			wantContent: "panic recovered: boom", wantCalls: 1,
		},
		{
			name: "length", call: ToolCall{ID: "1", Type: "function", Function: FunctionCall{Name: "never", Arguments: `{`}}, finish: "length",
			tools: func(count *atomic.Int32) []ToolDefinition {
				return []ToolDefinition{testToolDefinition(&functionTool{name: "never", run: func(context.Context, string) (string, error) {
					count.Add(1)
					return "unexpected", nil
				}})}
			},
			wantContent: "was not executed", wantReason: ToolSyntheticModelIncomplete, wantCalls: 0,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			first := AssistantMessage("", []ToolCall{test.call})
			first.ResponseMeta = &ResponseMeta{FinishReason: test.finish}
			model := &scriptedModel{responses: []scriptedModelResponse{
				{message: first},
				{message: AssistantMessage("recovered", nil)},
			}}
			agent, err := NewAgent(context.Background(), AgentConfig{Name: test.name, Model: model, Tools: test.tools(&calls)})
			if err != nil {
				t.Fatal(err)
			}
			iterator := NewRunner(RunnerConfig{Agent: agent}).Query(context.Background(), "go")
			var toolResult *Message
			for {
				event, ok := iterator.Next()
				if !ok {
					break
				}
				if event.Err != nil {
					t.Fatal(event.Err)
				}
				if event.Output == nil || event.Output.MessageOutput == nil {
					continue
				}
				if event.Output.MessageOutput.Role == ToolRole {
					toolResult = event.Output.MessageOutput.Message
				}
			}
			if toolResult == nil || !strings.Contains(toolResult.Content, test.wantContent) {
				t.Fatalf("tool result = %#v, want content %q", toolResult, test.wantContent)
			}
			if toolResult.ToolResult == nil || toolResult.ToolResult.SyntheticReason != test.wantReason {
				t.Fatalf("tool result summary = %#v, want reason %q", toolResult.ToolResult, test.wantReason)
			}
			if calls.Load() != test.wantCalls {
				t.Fatalf("tool calls = %d, want %d", calls.Load(), test.wantCalls)
			}
		})
	}
}

type blockingStreamModel struct {
	started chan struct{}
	once    sync.Once
}

func (model *blockingStreamModel) Generate(context.Context, []*Message, ...ModelOption) (*Message, error) {
	return nil, errors.New("unexpected Generate")
}

func (model *blockingStreamModel) Stream(ctx context.Context, _ []*Message, _ ...ModelOption) (*StreamReader[*Message], error) {
	model.once.Do(func() { close(model.started) })
	return &StreamReader[*Message]{recvFn: func() (*Message, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}, nil
}

func TestNativeLoopImmediateCancelClosesPublicStream(t *testing.T) {
	model := &blockingStreamModel{started: make(chan struct{})}
	agent, err := NewAgent(context.Background(), AgentConfig{Name: "cancel", Model: model})
	if err != nil {
		t.Fatal(err)
	}
	runOption, cancel := WithCancel()
	iterator := NewRunner(RunnerConfig{Agent: agent, EnableStreaming: true}).Query(context.Background(), "go", runOption)
	event, ok := iterator.Next()
	if !ok || event.Output == nil || event.Output.MessageOutput == nil {
		t.Fatalf("stream event = %#v", event)
	}
	<-model.started
	handle, contributed := cancel(WithAgentCancelMode(CancelImmediate))
	if !contributed {
		t.Fatal("cancel did not contribute")
	}
	if _, err := event.Output.MessageOutput.GetMessage(); !errors.Is(err, ErrStreamCanceled) {
		t.Fatalf("public stream error = %v", err)
	}
	var cancelEvent *AgentEvent
	for {
		event, available := iterator.Next()
		if !available {
			break
		}
		if event.Err != nil {
			cancelEvent = event
			break
		}
	}
	if cancelEvent == nil {
		t.Fatal("missing cancel event")
	}
	var cancelErr *CancelError
	if !errors.As(cancelEvent.Err, &cancelErr) || cancelErr.Info.Mode != CancelImmediate {
		t.Fatalf("cancel event = %#v", cancelEvent)
	}
	if _, ok := iterator.Next(); ok {
		t.Fatal("unexpected event after cancel")
	}
	if err := handle.Wait(); err != nil {
		t.Fatalf("cancel wait = %v", err)
	}
}

type blockingModelStart struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (model *blockingModelStart) Generate(context.Context, []*Message, ...ModelOption) (*Message, error) {
	return nil, errors.New("unexpected Generate")
}

func (model *blockingModelStart) Stream(context.Context, []*Message, ...ModelOption) (*StreamReader[*Message], error) {
	model.once.Do(func() { close(model.started) })
	<-model.release
	return StreamReaderFromArray([]*Message{AssistantMessage("late", nil)}), nil
}

func TestNativeLoopImmediateCancelDoesNotWaitForBlockingModelCall(t *testing.T) {
	model := &blockingModelStart{started: make(chan struct{}), release: make(chan struct{})}
	defer close(model.release)
	agent, err := NewAgent(context.Background(), AgentConfig{Name: "blocking-model", Model: model})
	if err != nil {
		t.Fatal(err)
	}
	runOption, cancel := WithCancel()
	iterator := NewRunner(RunnerConfig{Agent: agent, EnableStreaming: true}).Query(context.Background(), "go", runOption)
	<-model.started
	handle, contributed := cancel(WithAgentCancelMode(CancelImmediate))
	if !contributed {
		t.Fatal("cancel did not contribute")
	}
	var cancelErr *CancelError
	deadline := time.After(100 * time.Millisecond)
	for cancelErr == nil {
		result := make(chan nextAgentEventResult, 1)
		safeGo(func() {
			event, ok := iterator.Next()
			result <- nextAgentEventResult{event: event, ok: ok}
		}, func(err error) {
			result <- nextAgentEventResult{err: err}
		})
		select {
		case next := <-result:
			if next.err != nil || !next.ok {
				t.Fatalf("cancel stream ended: %#v", next)
			}
			if next.event.Err != nil && !errors.As(next.event.Err, &cancelErr) {
				t.Fatalf("cancel event = %#v", next.event)
			}
		case <-deadline:
			t.Fatal("timed out waiting for immediate cancel")
		}
	}
	if cancelErr.Info.Mode != CancelImmediate {
		t.Fatalf("cancel mode = %#v", cancelErr.Info)
	}
	if err := handle.Wait(); err != nil {
		t.Fatalf("cancel wait = %v", err)
	}
}

func TestNativeLoopImmediateCancelEscalatesPendingSafePoint(t *testing.T) {
	model := &blockingModelStart{started: make(chan struct{}), release: make(chan struct{})}
	defer close(model.release)
	agent, err := NewAgent(context.Background(), AgentConfig{Name: "escalated-model", Model: model})
	if err != nil {
		t.Fatal(err)
	}
	runOption, cancel := WithCancel()
	iterator := NewRunner(RunnerConfig{Agent: agent, EnableStreaming: true}).Query(context.Background(), "go", runOption)
	<-model.started
	preemptHandle, contributed := cancel(WithAgentCancelMode(CancelAfterChatModel | CancelAfterToolCalls))
	if !contributed {
		t.Fatal("safe-point cancel did not contribute")
	}
	abortHandle, contributed := cancel(WithAgentCancelMode(CancelImmediate))
	if !contributed {
		t.Fatal("immediate escalation did not contribute")
	}
	event, ok := nextAgentEventWithin(t, iterator, 100*time.Millisecond)
	var cancelErr *CancelError
	if !ok || !errors.As(event.Err, &cancelErr) || cancelErr.Info.Mode != CancelImmediate {
		t.Fatalf("escalated cancel event = %#v", event)
	}
	if err := preemptHandle.Wait(); err != nil {
		t.Fatalf("safe-point cancel wait = %v", err)
	}
	if err := abortHandle.Wait(); err != nil {
		t.Fatalf("immediate cancel wait = %v", err)
	}
}

func TestNativeLoopImmediateCancelDoesNotWaitForBlockingToolCall(t *testing.T) {
	model := &scriptedModel{responses: []scriptedModelResponse{
		{message: AssistantMessage("", []ToolCall{{ID: "blocked", Type: "function", Function: FunctionCall{Name: "blocked", Arguments: `{}`}}})},
		{message: AssistantMessage("late", nil)},
	}}
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	tool := &functionTool{name: "blocked", run: func(context.Context, string) (string, error) {
		close(started)
		<-release
		return "late", nil
	}}
	agent, err := NewAgent(context.Background(), AgentConfig{Name: "blocking-tool", Model: model, Tools: []ToolDefinition{testToolDefinition(tool)}})
	if err != nil {
		t.Fatal(err)
	}
	runOption, cancel := WithCancel()
	iterator := NewRunner(RunnerConfig{Agent: agent}).Query(context.Background(), "go", runOption)
	first, ok := iterator.Next()
	if !ok || first.Err != nil || first.Output == nil || first.Output.MessageOutput == nil {
		t.Fatalf("assistant event = %#v", first)
	}
	<-started
	handle, contributed := cancel(WithAgentCancelMode(CancelImmediate))
	if !contributed {
		t.Fatal("cancel did not contribute")
	}
	var cancelErr *CancelError
	deadline := time.Now().Add(100 * time.Millisecond)
	for cancelErr == nil {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatal("timed out waiting for immediate cancel")
		}
		event, ok := nextAgentEventWithin(t, iterator, remaining)
		if !ok {
			t.Fatal("missing cancel event")
		}
		if event.Err != nil && !errors.As(event.Err, &cancelErr) {
			t.Fatalf("cancel event = %#v", event)
		}
	}
	if cancelErr.Info.Mode != CancelImmediate {
		t.Fatalf("cancel mode = %#v", cancelErr.Info)
	}
	if err := handle.Wait(); err != nil {
		t.Fatalf("cancel wait = %v", err)
	}
}

type nextAgentEventResult struct {
	event *AgentEvent
	ok    bool
	err   error
}

func nextAgentEventWithin(t *testing.T, iterator *AsyncIterator[*AgentEvent], limit time.Duration) (*AgentEvent, bool) {
	t.Helper()
	result := make(chan nextAgentEventResult, 1)
	safeGo(func() {
		event, ok := iterator.Next()
		result <- nextAgentEventResult{event: event, ok: ok}
	}, func(err error) {
		result <- nextAgentEventResult{err: err}
	})
	select {
	case received := <-result:
		if received.err != nil {
			t.Fatalf("iterator panic: %v", received.err)
		}
		return received.event, received.ok
	case <-time.After(limit):
		t.Fatalf("timed out waiting for agent event after %s", limit)
		return nil, false
	}
}

type blockingGenerateModel struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (model *blockingGenerateModel) Generate(context.Context, []*Message, ...ModelOption) (*Message, error) {
	model.once.Do(func() { close(model.started) })
	<-model.release
	return AssistantMessage("", []ToolCall{{ID: "1", Type: "function", Function: FunctionCall{Name: "never", Arguments: `{}`}}}), nil
}

func (model *blockingGenerateModel) Stream(context.Context, []*Message, ...ModelOption) (*StreamReader[*Message], error) {
	return nil, errors.New("unexpected Stream")
}

func TestNativeLoopCancelAfterModelSkipsTools(t *testing.T) {
	model := &blockingGenerateModel{started: make(chan struct{}), release: make(chan struct{})}
	var calls atomic.Int32
	current := &functionTool{name: "never", run: func(context.Context, string) (string, error) {
		calls.Add(1)
		return "unexpected", nil
	}}
	agent, err := NewAgent(context.Background(), AgentConfig{Name: "safe-cancel", Model: model, Tools: []ToolDefinition{testToolDefinition(current)}})
	if err != nil {
		t.Fatal(err)
	}
	runOption, cancel := WithCancel()
	iterator := NewRunner(RunnerConfig{Agent: agent}).Query(context.Background(), "go", runOption)
	<-model.started
	handle, contributed := cancel(WithAgentCancelMode(CancelAfterChatModel))
	if !contributed {
		t.Fatal("cancel did not contribute")
	}
	close(model.release)
	assistantEvent, ok := iterator.Next()
	if !ok || assistantEvent.Err != nil || assistantEvent.Output.MessageOutput.Role != Assistant {
		t.Fatalf("assistant event = %#v", assistantEvent)
	}
	var cancelErr *CancelError
	for {
		cancelEvent, next := iterator.Next()
		if !next {
			t.Fatal("missing cancel event")
		}
		if cancelEvent.Err == nil {
			continue
		}
		if !errors.As(cancelEvent.Err, &cancelErr) || cancelErr.Info.Mode != CancelAfterChatModel {
			t.Fatalf("cancel event = %#v", cancelEvent)
		}
		break
	}
	if calls.Load() != 0 {
		t.Fatalf("tool ran %d times", calls.Load())
	}
	if err := handle.Wait(); err != nil {
		t.Fatalf("cancel wait = %v", err)
	}
}

func TestNativeLoopRetryAndNestedEventSink(t *testing.T) {
	model := &scriptedModel{responses: []scriptedModelResponse{
		{err: errors.New("transient")},
		{message: AssistantMessage("", []ToolCall{{ID: "child-call", Type: "function", Function: FunctionCall{Name: "child", Arguments: `{}`}}})},
		{message: AssistantMessage("done", nil)},
	}}
	child := &functionTool{name: "child", run: func(ctx context.Context, _ string) (string, error) {
		if !EmitEvent(ctx, &AgentEvent{AgentName: "nested", Output: &AgentOutput{CustomizedOutput: "progress"}}) {
			return "", errors.New("event sink missing")
		}
		return "child-result", nil
	}}
	agent, err := NewAgent(context.Background(), AgentConfig{
		Name: "retry", Model: model, Tools: []ToolDefinition{testToolDefinition(child)},
		Retry: &RetryConfig{MaxRetries: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	iterator := NewRunner(RunnerConfig{Agent: agent}).Query(context.Background(), "go")
	var names []string
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			t.Fatal(event.Err)
		}
		if event.Output != nil && event.Output.ToolExecution != nil {
			continue
		}
		names = append(names, event.AgentName)
	}
	if fmt.Sprint(names) != fmt.Sprint([]string{"retry", "nested", "retry", "retry"}) {
		t.Fatalf("event order/names = %#v", names)
	}
	if len(model.capturedInputs()) != 3 {
		t.Fatalf("model calls = %d", len(model.capturedInputs()))
	}
}

func TestExternalContextCancelTerminatesLoop(t *testing.T) {
	model := &blockingStreamModel{started: make(chan struct{})}
	agent, err := NewAgent(context.Background(), AgentConfig{Name: "context-cancel", Model: model})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	iterator := NewRunner(RunnerConfig{Agent: agent, EnableStreaming: true}).Query(ctx, "go")
	event, ok := iterator.Next()
	if !ok {
		t.Fatal("missing streaming event")
	}
	cancel()
	if _, err := event.Output.MessageOutput.GetMessage(); !errors.Is(err, context.Canceled) {
		t.Fatalf("stream error = %v", err)
	}
	errorEvent, ok := iterator.Next()
	if !ok || !errors.Is(errorEvent.Err, context.Canceled) {
		t.Fatalf("context event = %#v", errorEvent)
	}
	if _, ok := iterator.Next(); ok {
		t.Fatal("unexpected event after context cancellation")
	}
}
