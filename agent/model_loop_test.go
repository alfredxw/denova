package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
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
	tool  bool
}

type fullSeamRetryModel struct {
	mu        sync.Mutex
	responses []*Message
	inputs    [][]*Message
	options   []*Options
}

func (model *fullSeamRetryModel) Generate(_ context.Context, input []*Message, options ...ModelOption) (*Message, error) {
	return model.next(input, options...)
}

func (model *fullSeamRetryModel) Stream(_ context.Context, input []*Message, options ...ModelOption) (*StreamReader[*Message], error) {
	message, err := model.next(input, options...)
	if err != nil {
		return nil, err
	}
	return StreamReaderFromArray([]*Message{message}), nil
}

func (model *fullSeamRetryModel) next(input []*Message, options ...ModelOption) (*Message, error) {
	model.mu.Lock()
	defer model.mu.Unlock()
	model.inputs = append(model.inputs, cloneMessages(input))
	model.options = append(model.options, GetCommonOptions(nil, options...))
	if len(model.responses) == 0 {
		return nil, errors.New("full-seam retry model exhausted")
	}
	message := model.responses[0].Clone()
	model.responses = model.responses[1:]
	return message, nil
}

type retryNormalizationMiddleware struct {
	BaseMiddleware
	mu       sync.Mutex
	attempts []int
}

func (middleware *retryNormalizationMiddleware) BeforeModelCall(
	ctx context.Context,
	call *ModelCall,
	modelContext *ModelContext,
) (context.Context, *ModelCall, error) {
	middleware.mu.Lock()
	middleware.attempts = append(middleware.attempts, modelContext.Attempt)
	middleware.mu.Unlock()
	next := *call
	next.Messages = cloneMessages(call.Messages)
	// Presentation middleware often uses a JSON-normalized intermediate form.
	// Retry-only provenance must survive that provider-neutral round trip.
	roundTrip, err := json.Marshal(next.Messages)
	if err != nil {
		return ctx, call, err
	}
	if err := json.Unmarshal(roundTrip, &next.Messages); err != nil {
		return ctx, call, err
	}
	for _, message := range next.Messages {
		if message != nil {
			message.Content = strings.ReplaceAll(message.Content, "BROKEN_RETRY_FEEDBACK", "NORMALIZED_RETRY_FEEDBACK")
		}
	}
	return ctx, &next, nil
}

func (*retryNormalizationMiddleware) ReviewModelOutput(_ context.Context, output ModelOutput) (ModelOutputReview, error) {
	if output.Message.Content != "reject me" {
		return ModelOutputReview{Action: ModelOutputAccept}, nil
	}
	return ModelOutputReview{Action: ModelOutputRepair, Reason: "invalid_output", Feedback: []ContextFragment{{
		Source: "test_review", Purpose: "repair_invalid_output", Content: "BROKEN_RETRY_FEEDBACK", HardLimit: 128,
	}}}, nil
}

func (middleware *retryNormalizationMiddleware) calls() []int {
	middleware.mu.Lock()
	defer middleware.mu.Unlock()
	return append([]int(nil), middleware.attempts...)
}

func TestRetryReentersCompleteModelSeamAndKeepsFeedbackEphemeral(t *testing.T) {
	model := &fullSeamRetryModel{responses: []*Message{
		AssistantMessage("reject me", nil),
		AssistantMessage("", []ToolCall{{
			ID: "retry-tool", Type: "function", Function: FunctionCall{Name: "echo", Arguments: `{}`},
		}}),
		AssistantMessage("done", nil),
	}}
	middleware := &retryNormalizationMiddleware{}
	gateCalls := 0
	restarted := false
	echo := testToolDefinition(&functionTool{name: "echo", run: func(context.Context, string) (string, error) {
		return "echoed", nil
	}})
	native, err := newModelToolLoop(context.Background(), loopConfig{
		Name: "retry-complete-seam", Model: model, Tools: []ToolDefinition{echo},
		Middlewares:      []Middleware{middleware},
		ModelMaxAttempts: 2,
		modelCallGate: func(_ context.Context, call *ModelCall, modelContext *ModelContext) (*modelCallRestart, error) {
			gateCalls++
			if modelContext.Attempt == 1 && !restarted {
				if !messagesContainContent(call.Messages, "NORMALIZED_RETRY_FEEDBACK") {
					return nil, errors.New("retry maintenance ran before normalization")
				}
				restarted = true
				return &modelCallRestart{Messages: []*Message{UserMessage("COMPACTED_ACCEPTED_BASE")}}, nil
			}
			return nil, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := ContextWithSessionKey(context.Background(), "stable-cache-key")
	iterator := newLoopRunner(loopRunnerConfig{Agent: native}).Query(ctx, "source")
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event != nil && event.Err != nil {
			t.Fatal(event.Err)
		}
	}
	model.mu.Lock()
	inputs := make([][]*Message, len(model.inputs))
	for index := range model.inputs {
		inputs[index] = cloneMessages(model.inputs[index])
	}
	options := append([]*Options(nil), model.options...)
	model.mu.Unlock()
	if len(inputs) != 3 {
		t.Fatalf("provider calls=%d, want rejected attempt, retry, and post-tool call", len(inputs))
	}
	if !messagesContainContent(inputs[1], "COMPACTED_ACCEPTED_BASE") ||
		!messagesContainContent(inputs[1], "NORMALIZED_RETRY_FEEDBACK") {
		t.Fatalf("retry provider input=%#v", inputs[1])
	}
	if messagesContainContent(inputs[2], "BROKEN_RETRY_FEEDBACK") ||
		messagesContainContent(inputs[2], "NORMALIZED_RETRY_FEEDBACK") {
		t.Fatalf("retry-only feedback leaked into accepted transcript: %#v", inputs[2])
	}
	if len(options) != 3 || options[0].SessionKey != "stable-cache-key" ||
		options[1].SessionKey != "stable-cache-key" || options[2].SessionKey != "stable-cache-key" ||
		len(options[1].Tools) != 1 || options[1].Tools[0].Name != "echo" {
		t.Fatalf("retry cache/schema options=%#v", options)
	}
	if got := fmt.Sprint(middleware.calls()); got != fmt.Sprint([]int{0, 1, 1, 0}) {
		t.Fatalf("BeforeModelCall attempts=%s", got)
	}
	if gateCalls != 4 || !restarted {
		t.Fatalf("maintenance gate calls=%d restarted=%v", gateCalls, restarted)
	}
}

func messagesContainContent(messages []*Message, content string) bool {
	for _, message := range messages {
		if message != nil && strings.Contains(message.Content, content) {
			return true
		}
	}
	return false
}

func (*retryBeforeFirstChunkModel) Generate(context.Context, []*Message, ...ModelOption) (*Message, error) {
	return nil, errors.New("unexpected Generate")
}

func (model *retryBeforeFirstChunkModel) Stream(context.Context, []*Message, ...ModelOption) (*StreamReader[*Message], error) {
	call := model.calls.Add(1)
	if call == 1 {
		reader, writer := Pipe[*Message](-1)
		writer.Send(nil, model.err)
		writer.Close()
		return reader, nil
	}
	if model.tool && call == 2 {
		return StreamReaderFromArray([]*Message{AssistantMessage("", []ToolCall{{
			ID: "provider-reused", Type: "function",
			Function: FunctionCall{Name: "echo", Arguments: `{}`},
		}})}), nil
	}
	return StreamReaderFromArray([]*Message{AssistantMessage("recovered", nil)}), nil
}

func TestNativeLoopRetriesStreamErrorBeforeFirstChunk(t *testing.T) {
	streamErr := errors.New("stream failed before first chunk")
	model := &retryBeforeFirstChunkModel{err: streamErr}
	agent, err := newModelToolLoop(context.Background(), loopConfig{
		Name: "stream-retry-before-first-chunk", Model: model,
		ModelMaxAttempts: 2, Retry: &RetryConfig{Decide: retryEveryTestError},
	})
	if err != nil {
		t.Fatal(err)
	}

	iterator := newLoopRunner(loopRunnerConfig{Agent: agent, EnableStreaming: true}).Query(context.Background(), "go")
	streamEvent, ok := iterator.Next()
	if !ok || streamEvent == nil || streamEvent.Err != nil || streamEvent.Output == nil ||
		streamEvent.Output.MessageOutput == nil || !streamEvent.Output.MessageOutput.IsStreaming {
		t.Fatalf("assistant stream event = %#v", streamEvent)
	}
	if _, err := streamEvent.Output.MessageOutput.GetMessage(); err == nil {
		t.Fatal("failed attempt was accepted")
	}
	retryEvent, ok := iterator.Next()
	if !ok || retryEvent.Output == nil || retryEvent.Output.ModelRetry == nil {
		t.Fatalf("retry event=%#v", retryEvent)
	}
	streamEvent, ok = iterator.Next()
	if !ok || streamEvent.Output == nil || streamEvent.Output.MessageOutput == nil {
		t.Fatalf("retry response=%#v", streamEvent)
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

func TestNativeLoopRetryBeforeFirstChunkKeepsToolExecutionIdentityAligned(t *testing.T) {
	model := &retryBeforeFirstChunkModel{err: errors.New("stream failed before first chunk"), tool: true}
	echo := testToolDefinition(&functionTool{name: "echo", run: func(context.Context, string) (string, error) {
		return "ok", nil
	}})
	native, err := newModelToolLoop(context.Background(), loopConfig{
		Name: "stream-retry-tool-identity", Model: model, Tools: []ToolDefinition{echo},
		ModelMaxAttempts: 2, Retry: &RetryConfig{Decide: retryEveryTestError},
	})
	if err != nil {
		t.Fatal(err)
	}

	iterator := newLoopRunner(loopRunnerConfig{Agent: native, EnableStreaming: true}).Query(context.Background(), "go")
	var displayedID string
	var lifecycleIDs []string
	var transcriptID string
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event == nil {
			continue
		}
		if event.Err != nil {
			t.Fatal(event.Err)
		}
		if event.Output != nil && event.Output.ToolExecution != nil {
			lifecycleIDs = append(lifecycleIDs, event.Output.ToolExecution.ExecutionID)
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		variant := event.Output.MessageOutput
		if variant.Role == Assistant {
			message, messageErr := variant.GetMessage()
			if messageErr != nil {
				var rejected *modelResponseRejected
				if errors.As(messageErr, &rejected) {
					continue
				}
				t.Fatal(messageErr)
			}
			if len(message.ToolCalls) > 0 {
				displayedID = variant.ToolExecutionID(0)
			}
		}
		if variant.Role == ToolRole {
			transcriptID = variant.ExecutionID
		}
	}
	if displayedID == "" || transcriptID == "" {
		t.Fatalf("displayed execution id=%q transcript execution id=%q", displayedID, transcriptID)
	}
	if len(lifecycleIDs) < 2 {
		t.Fatalf("tool lifecycle ids = %v", lifecycleIDs)
	}
	for _, executionID := range append(lifecycleIDs, transcriptID) {
		if executionID != displayedID {
			t.Fatalf("tool execution identity diverged: display=%q lifecycle/transcript=%q all=%v", displayedID, executionID, lifecycleIDs)
		}
	}
	if calls := model.calls.Load(); calls != 3 {
		t.Fatalf("model stream calls = %d, want 3", calls)
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
	agent, err := newModelToolLoop(context.Background(), loopConfig{Name: "empty-stream", Model: model})
	if err != nil {
		t.Fatal(err)
	}

	iterator := newLoopRunner(loopRunnerConfig{Agent: agent, EnableStreaming: true}).Query(context.Background(), "go")
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

func TestNativeLoopBoundsRetriesAfterPartialPublication(t *testing.T) {
	streamErr := errors.New("stream failed after publication")
	model := &failingPublicStreamModel{err: streamErr}
	agent, err := newModelToolLoop(context.Background(), loopConfig{
		Name: "stream-retry", Model: model,
		ModelMaxAttempts: 3, Retry: &RetryConfig{Decide: retryEveryTestError},
	})
	if err != nil {
		t.Fatal(err)
	}
	iterator := newLoopRunner(loopRunnerConfig{Agent: agent, EnableStreaming: true}).Query(context.Background(), "go")
	streams, retries := 0, 0
	var terminal error
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			terminal = event.Err
			continue
		}
		if event.Output.ModelRetry != nil {
			retries++
			continue
		}
		if event.Output.MessageOutput != nil {
			streams++
			if _, err := event.Output.MessageOutput.GetMessage(); err == nil {
				t.Fatal("partial response accepted")
			}
		}
	}
	if !errors.Is(terminal, streamErr) || streams != 3 || retries != 2 || model.calls.Load() != 3 {
		t.Fatalf("terminal=%v streams=%d retries=%d calls=%d", terminal, streams, retries, model.calls.Load())
	}
}
