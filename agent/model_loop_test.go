package agent

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
)

type failingPublicStreamModel struct {
	calls atomic.Int32
	err   error
}

type retryBeforeFirstChunkModel struct {
	calls atomic.Int32
	err   error
}

func (*retryBeforeFirstChunkModel) Generate(context.Context, []*Message, ...ModelOption) (*Message, error) {
	return nil, errors.New("unexpected Generate")
}

func (model *retryBeforeFirstChunkModel) Stream(context.Context, []*Message, ...ModelOption) (*StreamReader[*Message], error) {
	if model.calls.Add(1) == 1 {
		reader, writer := Pipe[*Message](-1)
		writer.Send(nil, model.err)
		writer.Close()
		return reader, nil
	}
	return StreamReaderFromArray([]*Message{AssistantMessage("recovered", nil)}), nil
}

func TestNativeLoopRetriesStreamErrorBeforeFirstChunk(t *testing.T) {
	streamErr := errors.New("stream failed before first chunk")
	model := &retryBeforeFirstChunkModel{err: streamErr}
	agent, err := NewAgent(context.Background(), AgentConfig{
		Name: "stream-retry-before-first-chunk", Model: model,
		Retry: &RetryConfig{MaxRetries: 1, IsRetryable: func(context.Context, error) bool { return true }},
	})
	if err != nil {
		t.Fatal(err)
	}

	iterator := NewRunner(RunnerConfig{Agent: agent, EnableStreaming: true}).Query(context.Background(), "go")
	streamEvent, ok := iterator.Next()
	if !ok || streamEvent == nil || streamEvent.Err != nil || streamEvent.Output == nil ||
		streamEvent.Output.MessageOutput == nil || !streamEvent.Output.MessageOutput.IsStreaming {
		t.Fatalf("assistant stream event = %#v", streamEvent)
	}
	message, err := streamEvent.Output.MessageOutput.GetMessage()
	if err != nil {
		t.Fatalf("assistant stream error = %v", err)
	}
	if message == nil || message.Content != "recovered" {
		t.Fatalf("assistant message = %#v", message)
	}
	if _, ok := iterator.Next(); ok {
		t.Fatal("unexpected event after recovered stream")
	}
	if calls := model.calls.Load(); calls != 2 {
		t.Fatalf("model stream calls = %d, want 2", calls)
	}
}

type emptyStreamModel struct {
	calls atomic.Int32
}

func (*emptyStreamModel) Generate(context.Context, []*Message, ...ModelOption) (*Message, error) {
	return nil, errors.New("unexpected Generate")
}

func (model *emptyStreamModel) Stream(context.Context, []*Message, ...ModelOption) (*StreamReader[*Message], error) {
	model.calls.Add(1)
	return StreamReaderFromArray([]*Message{}), nil
}

func TestNativeLoopRejectsStreamThatEndsBeforeFirstChunk(t *testing.T) {
	model := &emptyStreamModel{}
	agent, err := NewAgent(context.Background(), AgentConfig{Name: "empty-stream", Model: model})
	if err != nil {
		t.Fatal(err)
	}

	iterator := NewRunner(RunnerConfig{Agent: agent, EnableStreaming: true}).Query(context.Background(), "go")
	streamEvent, ok := iterator.Next()
	if !ok || streamEvent == nil || streamEvent.Err != nil || streamEvent.Output == nil ||
		streamEvent.Output.MessageOutput == nil || !streamEvent.Output.MessageOutput.IsStreaming {
		t.Fatalf("assistant stream event = %#v", streamEvent)
	}
	if _, err := streamEvent.Output.MessageOutput.GetMessage(); err == nil || !strings.Contains(err.Error(), "ended before first message chunk") {
		t.Fatalf("assistant stream error = %v", err)
	}
	errorEvent, ok := iterator.Next()
	if !ok || errorEvent == nil || errorEvent.Err == nil || !strings.Contains(errorEvent.Err.Error(), "ended before first message chunk") {
		t.Fatalf("terminal empty-stream event = %#v", errorEvent)
	}
	if _, ok := iterator.Next(); ok {
		t.Fatal("unexpected event after empty stream failure")
	}
	if calls := model.calls.Load(); calls != 1 {
		t.Fatalf("model stream calls = %d, want 1", calls)
	}
}

func (*failingPublicStreamModel) Generate(context.Context, []*Message, ...ModelOption) (*Message, error) {
	return nil, errors.New("unexpected Generate")
}

func (model *failingPublicStreamModel) Stream(context.Context, []*Message, ...ModelOption) (*StreamReader[*Message], error) {
	model.calls.Add(1)
	reader, writer := Pipe[*Message](-1)
	writer.Send(&Message{Role: Assistant, Content: "partial"}, nil)
	writer.Send(nil, model.err)
	writer.Close()
	return reader, nil
}

func TestNativeLoopDoesNotRetryErrorPublishedOnAssistantStream(t *testing.T) {
	streamErr := errors.New("stream failed after publication")
	model := &failingPublicStreamModel{err: streamErr}
	agent, err := NewAgent(context.Background(), AgentConfig{
		Name: "stream-retry", Model: model,
		Retry: &RetryConfig{MaxRetries: 2, IsRetryable: func(context.Context, error) bool { return true }},
	})
	if err != nil {
		t.Fatal(err)
	}
	iterator := NewRunner(RunnerConfig{Agent: agent, EnableStreaming: true}).Query(context.Background(), "go")
	streamEvent, ok := iterator.Next()
	if !ok || streamEvent == nil || streamEvent.Err != nil || streamEvent.Output == nil ||
		streamEvent.Output.MessageOutput == nil || !streamEvent.Output.MessageOutput.IsStreaming {
		t.Fatalf("assistant stream event = %#v", streamEvent)
	}
	if _, err := streamEvent.Output.MessageOutput.GetMessage(); !errors.Is(err, streamErr) {
		t.Fatalf("assistant stream error = %v", err)
	}
	errorEvent, ok := iterator.Next()
	if !ok || errorEvent == nil || !errors.Is(errorEvent.Err, streamErr) {
		t.Fatalf("terminal stream error event = %#v", errorEvent)
	}
	if _, ok := iterator.Next(); ok {
		t.Fatal("unexpected event after published stream error")
	}
	if calls := model.calls.Load(); calls != 1 {
		t.Fatalf("model stream calls = %d, want 1", calls)
	}
}
