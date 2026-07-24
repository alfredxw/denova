package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNativeLoopToolInterruptEmitsActionAndStops(t *testing.T) {
	model := &scriptedModel{responses: []scriptedModelResponse{
		{message: AssistantMessage("", []ToolCall{{ID: "approval", Type: "function", Function: FunctionCall{Name: "approval", Arguments: `{}`}}})},
		{message: AssistantMessage("must not run", nil)},
	}}
	interrupt := &InterruptError{Reason: "approval required", ResumeToken: []byte(`{"request":"approval"}`)}
	tool := &functionTool{name: "approval", run: func(context.Context, string) (string, error) {
		return "", fmt.Errorf("request approval: %w", interrupt)
	}}
	agent, err := NewAgent(context.Background(), AgentConfig{Name: "interrupt", Model: model, Tools: []BaseTool{tool}})
	if err != nil {
		t.Fatal(err)
	}
	iterator := NewRunner(RunnerConfig{Agent: agent}).Query(context.Background(), "go")
	assistantEvent, ok := iterator.Next()
	if !ok || assistantEvent == nil || assistantEvent.Err != nil || assistantEvent.Output == nil {
		t.Fatalf("assistant event = %#v", assistantEvent)
	}
	toolEvent, ok := iterator.Next()
	if !ok || toolEvent == nil || toolEvent.Err != nil || toolEvent.Output == nil ||
		toolEvent.Output.MessageOutput == nil || toolEvent.Output.MessageOutput.ToolName != "approval" {
		t.Fatalf("approval result event = %#v", toolEvent)
	}
	interruptEvent, ok := iterator.Next()
	if !ok || interruptEvent == nil || interruptEvent.Err == nil || interruptEvent.Action == nil ||
		interruptEvent.Action.Interrupted == nil || interruptEvent.Action.Interrupted.Reason != interrupt.Reason {
		t.Fatalf("interrupt event = %#v", interruptEvent)
	}
	var gotInterrupt *InterruptError
	if !errors.As(interruptEvent.Err, &gotInterrupt) || gotInterrupt != interrupt {
		t.Fatalf("interrupt error = %T: %v", interruptEvent.Err, interruptEvent.Err)
	}
	if _, ok := iterator.Next(); ok {
		t.Fatal("unexpected event after tool interruption")
	}
	if calls := len(model.capturedInputs()); calls != 1 {
		t.Fatalf("model calls = %d, want 1", calls)
	}
}

func TestNativeLoopToolInterruptPreservesCompletedSiblingResults(t *testing.T) {
	model := &scriptedModel{responses: []scriptedModelResponse{{message: AssistantMessage("", []ToolCall{
		{ID: "write", Type: "function", Function: FunctionCall{Name: "write", Arguments: `{}`}},
		{ID: "approval", Type: "function", Function: FunctionCall{Name: "approval", Arguments: `{}`}},
	})}}}
	writeStarted := make(chan struct{})
	releaseWrite := make(chan struct{})
	write := &functionTool{name: "write", run: func(context.Context, string) (string, error) {
		close(writeStarted)
		<-releaseWrite
		return `{"written":true}`, nil
	}}
	interrupt := &InterruptError{Reason: "approval required"}
	approval := &functionTool{name: "approval", run: func(context.Context, string) (string, error) {
		<-writeStarted
		return "", interrupt
	}}
	agent, err := NewAgent(context.Background(), AgentConfig{
		Name: "interrupt-batch", Model: model, Tools: []BaseTool{write, approval},
	})
	if err != nil {
		t.Fatal(err)
	}
	iterator := NewRunner(RunnerConfig{Agent: agent}).Query(context.Background(), "go")
	assistantEvent, ok := iterator.Next()
	if !ok || assistantEvent == nil || assistantEvent.Err != nil || assistantEvent.Output == nil {
		t.Fatalf("assistant event = %#v", assistantEvent)
	}

	next := make(chan nextAgentEventResult, 1)
	safeGo(func() {
		event, available := iterator.Next()
		next <- nextAgentEventResult{event: event, ok: available}
	}, func(err error) {
		next <- nextAgentEventResult{err: err}
	})
	select {
	case event := <-next:
		t.Fatalf("interrupt surfaced before sibling completion: %#v", event)
	case <-time.After(20 * time.Millisecond):
	}
	close(releaseWrite)

	writeEvent := <-next
	if writeEvent.err != nil || !writeEvent.ok || writeEvent.event == nil || writeEvent.event.Err != nil ||
		writeEvent.event.Output == nil || writeEvent.event.Output.MessageOutput == nil ||
		writeEvent.event.Output.MessageOutput.ToolName != "write" {
		t.Fatalf("write result event = %#v", writeEvent)
	}
	approvalEvent, ok := iterator.Next()
	if !ok || approvalEvent == nil || approvalEvent.Err != nil || approvalEvent.Output == nil ||
		approvalEvent.Output.MessageOutput == nil || approvalEvent.Output.MessageOutput.ToolName != "approval" {
		t.Fatalf("approval result event = %#v", approvalEvent)
	}
	interruptEvent, ok := iterator.Next()
	if !ok || interruptEvent == nil || interruptEvent.Err == nil || interruptEvent.Action == nil ||
		interruptEvent.Action.Interrupted != interrupt {
		t.Fatalf("interrupt event = %#v", interruptEvent)
	}
	if _, ok := iterator.Next(); ok {
		t.Fatal("unexpected event after interruption")
	}
}

type cancelBeforeToolExecutionMiddleware struct {
	BaseMiddleware
	cancel context.CancelFunc
}

func (middleware *cancelBeforeToolExecutionMiddleware) WrapInvokableToolCall(
	_ context.Context,
	endpoint InvokableToolCallEndpoint,
	_ *ToolContext,
) (InvokableToolCallEndpoint, error) {
	middleware.cancel()
	return endpoint, nil
}

func (middleware *cancelBeforeToolExecutionMiddleware) WrapStreamableToolCall(
	_ context.Context,
	endpoint StreamableToolCallEndpoint,
	_ *ToolContext,
) (StreamableToolCallEndpoint, error) {
	middleware.cancel()
	return endpoint, nil
}

type countingStreamableTool struct {
	calls *atomic.Int32
}

func (*countingStreamableTool) Info(context.Context) (*ToolInfo, error) {
	return &ToolInfo{Name: "target"}, nil
}

func (tool *countingStreamableTool) StreamableRun(context.Context, string, ...ToolOption) (*StreamReader[string], error) {
	tool.calls.Add(1)
	return StreamReaderFromArray([]string{"unexpected"}), nil
}

func TestNativeLoopChecksContextImmediatelyBeforeToolExecution(t *testing.T) {
	tests := []struct {
		name string
		tool func(*atomic.Int32) BaseTool
	}{
		{name: "invokable", tool: func(calls *atomic.Int32) BaseTool {
			return &functionTool{name: "target", run: func(context.Context, string) (string, error) {
				calls.Add(1)
				return "unexpected", nil
			}}
		}},
		{name: "streamable", tool: func(calls *atomic.Int32) BaseTool {
			return &countingStreamableTool{calls: calls}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			model := &scriptedModel{responses: []scriptedModelResponse{{message: AssistantMessage("", []ToolCall{{
				ID: "target", Type: "function", Function: FunctionCall{Name: "target", Arguments: `{}`},
			}})}}}
			agent, err := NewAgent(context.Background(), AgentConfig{
				Name: "cancel-before-tool", Model: model, Tools: []BaseTool{test.tool(&calls)},
				Middlewares: []Middleware{&cancelBeforeToolExecutionMiddleware{cancel: cancel}},
			})
			if err != nil {
				t.Fatal(err)
			}
			iterator := NewRunner(RunnerConfig{Agent: agent}).Query(ctx, "go")
			assistantEvent, ok := iterator.Next()
			if !ok || assistantEvent == nil || assistantEvent.Err != nil || assistantEvent.Output == nil {
				t.Fatalf("assistant event = %#v", assistantEvent)
			}
			cancelEvent, ok := iterator.Next()
			if !ok || cancelEvent == nil || !errors.Is(cancelEvent.Err, context.Canceled) {
				t.Fatalf("cancellation event = %#v", cancelEvent)
			}
			if _, ok := iterator.Next(); ok {
				t.Fatal("unexpected event after context cancellation")
			}
			if got := calls.Load(); got != 0 {
				t.Fatalf("tool calls = %d, want 0", got)
			}
		})
	}
}

type closeAwareBlockingStreamTool struct {
	started   chan struct{}
	release   chan struct{}
	closed    chan struct{}
	startOnce sync.Once
	closeOnce sync.Once
}

func (*closeAwareBlockingStreamTool) Info(context.Context) (*ToolInfo, error) {
	return &ToolInfo{Name: "blocking-stream"}, nil
}

func (tool *closeAwareBlockingStreamTool) StreamableRun(context.Context, string, ...ToolOption) (*StreamReader[string], error) {
	return &StreamReader[string]{
		recvFn: func() (string, error) {
			tool.startOnce.Do(func() { close(tool.started) })
			<-tool.release
			return "", io.EOF
		},
		closeFn: func() {
			tool.closeOnce.Do(func() {
				close(tool.release)
				close(tool.closed)
			})
		},
	}, nil
}

func TestNativeLoopCancellationClosesBlockingToolStream(t *testing.T) {
	model := &scriptedModel{responses: []scriptedModelResponse{{message: AssistantMessage("", []ToolCall{{
		ID: "blocking-stream", Type: "function", Function: FunctionCall{Name: "blocking-stream", Arguments: `{}`},
	}})}}}
	tool := &closeAwareBlockingStreamTool{
		started: make(chan struct{}), release: make(chan struct{}), closed: make(chan struct{}),
	}
	agent, err := NewAgent(context.Background(), AgentConfig{Name: "stream-cancel", Model: model, Tools: []BaseTool{tool}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	iterator := NewRunner(RunnerConfig{Agent: agent}).Query(ctx, "go")
	assistantEvent, ok := iterator.Next()
	if !ok || assistantEvent == nil || assistantEvent.Err != nil || assistantEvent.Output == nil {
		t.Fatalf("assistant event = %#v", assistantEvent)
	}
	<-tool.started
	cancel()
	cancelEvent, ok := nextAgentEventWithin(t, iterator, 100*time.Millisecond)
	if !ok || cancelEvent == nil || !errors.Is(cancelEvent.Err, context.Canceled) {
		t.Fatalf("cancellation event = %#v", cancelEvent)
	}
	select {
	case <-tool.closed:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("blocking tool stream was not closed after cancellation")
	}
	if _, ok := iterator.Next(); ok {
		t.Fatal("unexpected event after stream cancellation")
	}
}
