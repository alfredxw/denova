package agent

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type partialRetryModel struct {
	calls atomic.Int32
}

func TestSessionAdmitsOnlyTheRecoveredResponse(t *testing.T) {
	model := &partialRetryModel{}
	owner, err := New(context.Background(), Definition{Name: "retry", Model: model, Execution: ExecutionPolicy{
		ModelMaxAttempts: 2, Retry: &RetryConfig{Decide: retryEveryTestError},
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("retry"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), Text("go"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.Wait(context.Background())
	if err != nil || result.Status != ResultCompleted {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	snapshot, err := session.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.RecentRuns) != 1 || snapshot.RecentRuns[0].Output != "complete" {
		t.Fatalf("settled output=%#v", snapshot.RecentRuns)
	}
	state, err := decodeEngineTranscript(session.engineState)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Messages) != 2 || state.Messages[1].Content != "complete" || len(state.Messages[1].ToolCalls) != 0 {
		t.Fatalf("canonical transcript=%#v", state.Messages)
	}
}

func TestModelRetryAndOutputRepairShareBudget(t *testing.T) {
	model := &scriptedModel{responses: []scriptedModelResponse{{err: errors.New("transient")}, {message: AssistantMessage("reject me", nil)}}}
	native, err := newModelToolLoop(context.Background(), loopConfig{
		Model: model, ModelMaxAttempts: 2, Retry: &RetryConfig{Decide: retryEveryTestError},
		Middlewares: []Middleware{&retryNormalizationMiddleware{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	iterator := newLoopRunner(loopRunnerConfig{Agent: native}).Query(context.Background(), "go")
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
	if terminal == nil || !strings.Contains(terminal.Error(), "after 2 attempts") || len(model.capturedInputs()) != 2 {
		t.Fatalf("terminal=%v provider calls=%d", terminal, len(model.capturedInputs()))
	}
}

func TestRetryBackoffCanBeCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	model := &partialRetryModel{}
	native, err := newModelToolLoop(ctx, loopConfig{Model: model, ModelMaxAttempts: 2, Retry: &RetryConfig{Decide: func(context.Context, RetryContext) RetryDecision {
		return RetryDecision{Action: RetryAgain, Delay: time.Hour, Reason: "test_wait"}
	}}})
	if err != nil {
		t.Fatal(err)
	}
	iterator := newLoopRunner(loopRunnerConfig{Agent: native, EnableStreaming: true}).Query(ctx, "go")
	for {
		event, ok := iterator.Next()
		if !ok {
			t.Fatal("ended before backoff")
		}
		if event.Output != nil && event.Output.MessageOutput != nil {
			_, _ = event.Output.MessageOutput.GetMessage()
		}
		if event.Output != nil && event.Output.ModelRetry != nil {
			cancel()
			break
		}
	}
	finished := make(chan error, 1)
	safeGo(func() {
		for {
			event, ok := iterator.Next()
			if !ok {
				break
			}
			if event.Err != nil {
				finished <- event.Err
				return
			}
		}
		finished <- nil
	}, func(err error) { finished <- err })
	select {
	case err := <-finished:
		if err == nil || model.calls.Load() != 1 {
			t.Fatalf("err=%v calls=%d", err, model.calls.Load())
		}
	case <-time.After(time.Second):
		t.Fatal("retry did not stop during backoff")
	}
}

func TestSideCallRetriesPartialStreamWithinItsOwnBudget(t *testing.T) {
	model := &partialRetryModel{}
	call := &ModelCall{Model: model, Messages: []*Message{UserMessage("summary")}, Streaming: true}
	response, err := call.Snapshot().Complete(context.Background(), ExecutionPolicy{ModelMaxAttempts: 2, Retry: &RetryConfig{Decide: retryEveryTestError}})
	if err != nil || response.Content != "complete" || model.calls.Load() != 2 {
		t.Fatalf("response=%#v err=%v calls=%d", response, err, model.calls.Load())
	}
}

func TestSideCallCancellationDoesNotWaitForProvider(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "generate", true: "stream"}[streaming], func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			defer close(release)
			var model BaseChatModel = &blockingGenerateModel{started: started, release: release}
			if streaming {
				model = &blockingModelStart{started: started, release: release}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			call := &ModelCall{Model: model, Messages: []*Message{UserMessage("summary")}, Streaming: streaming}
			done := make(chan error, 1)
			safeGo(func() { _, err := call.Snapshot().Complete(ctx, ExecutionPolicy{}); done <- err }, func(err error) { done <- err })
			<-started
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("side call cancellation=%v", err)
				}
			case <-time.After(100 * time.Millisecond):
				t.Fatal("side call ignored cancellation")
			}
		})
	}
}

func retryEveryTestError(context.Context, RetryContext) RetryDecision {
	return RetryDecision{Action: RetryAgain, Reason: "test_transient"}
}

func (*partialRetryModel) Generate(context.Context, []*Message, ...ModelOption) (*Message, error) {
	return nil, errors.New("unexpected Generate")
}

func (model *partialRetryModel) Stream(context.Context, []*Message, ...ModelOption) (*StreamReader[*Message], error) {
	if model.calls.Add(1) > 1 {
		return StreamReaderFromArray([]*Message{AssistantMessage("complete", nil)}), nil
	}
	reader, writer := Pipe[*Message](-1)
	writer.Send(AssistantMessage("partial", []ToolCall{{ID: "unaccepted", Type: "function", Function: FunctionCall{Name: "echo", Arguments: `{}`}}}), nil)
	writer.Send(nil, errors.New("connection lost"))
	writer.Close()
	return reader, nil
}

func TestPartialResponseRetriesWithoutExecutingUnacceptedTools(t *testing.T) {
	model := &partialRetryModel{}
	var toolCalls atomic.Int32
	native, err := newModelToolLoop(context.Background(), loopConfig{
		Name: "partial-retry", Model: model,
		Tools: []ToolDefinition{testToolDefinition(&functionTool{name: "echo", run: func(context.Context, string) (string, error) {
			toolCalls.Add(1)
			return "unexpected", nil
		}})},
		ModelMaxAttempts: 2, Retry: &RetryConfig{Decide: retryEveryTestError},
	})
	if err != nil {
		t.Fatal(err)
	}
	iterator := newLoopRunner(loopRunnerConfig{Agent: native, EnableStreaming: true}).Query(context.Background(), "go")
	var completed string
	var ordinals []int
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			t.Fatalf("retry terminated the run: %v", event.Err)
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		output := event.Output.MessageOutput
		ordinals = append(ordinals, output.ModelResponseOrdinal)
		message, streamErr := output.GetMessage()
		if streamErr == nil && message != nil {
			completed = message.Content
		}
	}
	if completed != "complete" || model.calls.Load() != 2 || toolCalls.Load() != 0 {
		t.Fatalf("completed=%q provider_calls=%d tool_calls=%d", completed, model.calls.Load(), toolCalls.Load())
	}
	if len(ordinals) != 2 || ordinals[1] <= ordinals[0] {
		t.Fatalf("response ordinals=%v, want distinct increasing responses", ordinals)
	}
}
