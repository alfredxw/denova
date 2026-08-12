package agent

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestNativeLoopIdleTimeoutInterruptsSilentModelStream(t *testing.T) {
	model := &blockingStreamModel{started: make(chan struct{})}
	loop, err := newModelToolLoop(context.Background(), loopConfig{
		Name: "idle-model", Model: model, IdleTimeout: 30 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	iterator := loop.Run(context.Background(), &loopInput{
		Messages: []*Message{UserMessage("wait")}, EnableStreaming: true,
	})
	streamEvent, ok := iterator.Next()
	if !ok || streamEvent.Output == nil || streamEvent.Output.MessageOutput == nil {
		t.Fatalf("stream event = %#v", streamEvent)
	}
	if _, err := streamEvent.Output.MessageOutput.GetMessage(); !errors.Is(err, ErrIdleTimeout) {
		t.Fatalf("stream error = %v, want IdleTimeout", err)
	}
	terminal, ok := iterator.Next()
	if !ok || !errors.Is(terminal.Err, ErrIdleTimeout) {
		t.Fatalf("terminal event = %#v", terminal)
	}
}

func TestNativeLoopZeroIdleTimeoutRemainsUnlimited(t *testing.T) {
	model := &blockingStreamModel{started: make(chan struct{})}
	loop, err := newModelToolLoop(context.Background(), loopConfig{Name: "unlimited-model", Model: model})
	if err != nil {
		t.Fatal(err)
	}
	runOption, cancel := newLoopCancellation()
	iterator := loop.Run(context.Background(), &loopInput{
		Messages: []*Message{UserMessage("wait")}, EnableStreaming: true,
	}, runOption)
	streamEvent, ok := iterator.Next()
	if !ok || streamEvent.Output == nil || streamEvent.Output.MessageOutput == nil {
		t.Fatalf("stream event = %#v", streamEvent)
	}
	select {
	case <-model.started:
	case <-time.After(time.Second):
		t.Fatal("model stream did not start")
	}
	time.Sleep(40 * time.Millisecond)
	if _, contributed := cancel(withCancelMode(cancelImmediately)); !contributed {
		t.Fatal("cancel did not contribute after an unlimited idle interval")
	}
	if _, err := streamEvent.Output.MessageOutput.GetMessage(); !errors.Is(err, errStreamCanceled) {
		t.Fatalf("stream error = %v, want caller cancellation", err)
	}
}

type pacedStreamModel struct{}

func (pacedStreamModel) Generate(context.Context, []*Message, ...ModelOption) (*Message, error) {
	return nil, errors.New("unexpected Generate")
}

func (pacedStreamModel) Stream(context.Context, []*Message, ...ModelOption) (*StreamReader[*Message], error) {
	remaining := 4
	return &StreamReader[*Message]{recvFn: func() (*Message, error) {
		if remaining == 0 {
			return nil, io.EOF
		}
		time.Sleep(12 * time.Millisecond)
		remaining--
		return AssistantMessage("x", nil), nil
	}}, nil
}

func TestNativeLoopIdleTimeoutResetsOnEveryModelChunk(t *testing.T) {
	loop, err := newModelToolLoop(context.Background(), loopConfig{
		Name: "paced-model", Model: pacedStreamModel{}, IdleTimeout: 30 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	iterator := loop.Run(context.Background(), &loopInput{
		Messages: []*Message{UserMessage("continue")}, EnableStreaming: true,
	})
	var content strings.Builder
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
		message, err := event.Output.MessageOutput.GetMessage()
		if err != nil {
			t.Fatal(err)
		}
		if message != nil {
			content.WriteString(message.Content)
		}
	}
	if content.String() != "xxxx" {
		t.Fatalf("content = %q", content.String())
	}
}

func TestNativeLoopIdleTimeoutInterruptsSilentTool(t *testing.T) {
	tool := &functionTool{name: "wait", run: func(ctx context.Context, _ string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}}
	model := &scriptedModel{responses: []scriptedModelResponse{{message: AssistantMessage("", []ToolCall{{
		ID: "wait-call", Type: "function", Function: FunctionCall{Name: "wait", Arguments: `{}`},
	}})}}}
	loop, err := newModelToolLoop(context.Background(), loopConfig{
		Name: "idle-tool", Model: model, Tools: []ToolDefinition{testToolDefinition(tool)},
		IdleTimeout: 30 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	iterator := newLoopRunner(loopRunnerConfig{Agent: loop}).Query(context.Background(), "wait")
	var terminal error
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			terminal = event.Err
		}
	}
	if !errors.Is(terminal, ErrIdleTimeout) {
		t.Fatalf("terminal error = %v, want IdleTimeout", terminal)
	}
}
