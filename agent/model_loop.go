package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

func (agent *modelToolLoop) modelForCall(ctx context.Context, modelContext *ModelContext) (BaseChatModel, error) {
	model := agent.model
	if toolCalling, ok := model.(ToolCallingChatModel); ok {
		bound, err := toolCalling.WithTools(modelContext.Tools)
		if err != nil {
			return nil, fmt.Errorf("bind model tools: %w", err)
		}
		model = bound
	}
	for index := len(agent.middlewares) - 1; index >= 0; index-- {
		wrapped, err := agent.middlewares[index].WrapModel(ctx, model, modelContext)
		if err != nil {
			return nil, fmt.Errorf("wrap model middleware: %w", err)
		}
		if wrapped == nil {
			return nil, errors.New("wrap model middleware returned nil model")
		}
		model = wrapped
	}
	return model, nil
}

func (agent *modelToolLoop) callModelWithRetry(
	ctx context.Context,
	initial *ModelCall,
	initialContext *ModelContext,
	registry *Registry,
	events *asyncGenerator[*loopEvent],
	cancel *cancelControl,
) (*Message, int, []*Message, context.Context, error) {
	if initial == nil || initial.Model == nil || initialContext == nil {
		return nil, 0, nil, ctx, errors.New("model retry boundary requires an initial model call and context")
	}
	currentCall := &ModelCall{
		Model: initial.Model, Messages: cloneMessages(initial.Messages),
		Options: append([]ModelOption(nil), initial.Options...), Streaming: initial.Streaming,
		stablePrefixMessages: initial.stablePrefixMessages,
	}
	acceptedMessages := cloneMessages(initial.Messages)
	stableOptions := initial.Snapshot().ResolvedOptions()
	var retryFeedback []*Message
	var streamOutput *modelStreamOutput
	if initial.Streaming {
		streamOutput = &modelStreamOutput{agent: agent, events: events, registry: registry}
		defer streamOutput.close()
	}
	maxRetries := 0
	if agent.retry != nil {
		maxRetries = agent.retry.MaxRetries
	}
	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			preparedCtx, preparedCall, preparedBase, prepareErr := agent.prepareRetryModelCall(
				ctx, currentCall, initialContext, acceptedMessages, retryFeedback, stableOptions, attempt,
			)
			if prepareErr != nil {
				if streamOutput != nil {
					streamOutput.sendError(prepareErr)
				}
				return nil, 0, acceptedMessages, ctx, prepareErr
			}
			ctx, currentCall, acceptedMessages = preparedCtx, preparedCall, preparedBase
		}
		responseOrdinal := 0
		if streamOutput != nil {
			responseOrdinal = streamOutput.openResponseOrdinal()
		}
		if responseOrdinal == 0 {
			responseOrdinal = nextModelResponseOrdinal(ctx)
		}
		message, err, deliveredOnStream := agent.callModel(
			ctx, currentCall.Model, registry, currentCall.Messages, currentCall.Options,
			currentCall.Streaming, events, cancel, streamOutput, responseOrdinal,
		)
		if contextErr := agent.contextError(ctx, cancel); contextErr != nil {
			if streamOutput != nil && !deliveredOnStream {
				streamOutput.sendError(publicStreamError(contextErr, cancel))
			}
			return nil, 0, acceptedMessages, ctx, contextErr
		}
		if err != nil && deliveredOnStream {
			return nil, 0, acceptedMessages, ctx, err
		}
		decision := agent.retryDecision(ctx, attempt, currentCall.Messages, currentCall.Options, message, err)
		if decision == nil || !decision.Retry {
			if err != nil && streamOutput != nil {
				streamOutput.sendError(err)
			}
			return message, responseOrdinal, acceptedMessages, ctx, err
		}
		if attempt >= maxRetries {
			if err != nil {
				if streamOutput != nil {
					streamOutput.sendError(err)
				}
				return nil, 0, acceptedMessages, ctx, err
			}
			return nil, 0, acceptedMessages, ctx, fmt.Errorf("model output rejected after %d retries", maxRetries)
		}
		if deliveredOnStream && streamOutput != nil {
			// A completed response rejected by policy remains one visible stream;
			// the retry gets a fresh stream event. Pre-first-chunk failures keep the
			// current stream open so callers never observe a failed attempt.
			streamOutput.close()
		}
		if decision.Messages != nil {
			feedback, feedbackErr := retryFeedbackSuffix(acceptedMessages, decision.Messages)
			if feedbackErr != nil {
				return nil, 0, acceptedMessages, ctx, feedbackErr
			}
			retryFeedback = feedback
			currentCall.Messages = cloneMessages(decision.Messages)
		}
		if decision.Options != nil {
			currentCall.Options = append([]ModelOption(nil), decision.Options...)
		}
		backoff := decision.Backoff
		if backoff == 0 && agent.retry != nil && agent.retry.BackoffFunc != nil {
			backoff = agent.retry.BackoffFunc(ctx, attempt+1)
		}
		if backoff > 0 {
			if err := waitContext(ctx, backoff); err != nil {
				if streamOutput != nil {
					streamOutput.sendError(publicStreamError(err, cancel))
				}
				return nil, 0, acceptedMessages, ctx, err
			}
		}
	}
}

func (agent *modelToolLoop) prepareRetryModelCall(
	ctx context.Context,
	current *ModelCall,
	initial *ModelContext,
	accepted, feedback []*Message,
	stable *Options,
	attempt int,
) (context.Context, *ModelCall, []*Message, error) {
	modelContext := &ModelContext{
		Tools: cloneToolInfos(initial.Tools), Retry: initial.Retry,
		Iteration: initial.Iteration, Attempt: attempt,
		stablePrefixSeed: cloneMessages(initial.stablePrefixSeed),
	}
	messages := append(cloneMessages(accepted), cloneMessages(feedback)...)
	options := append([]ModelOption(nil), current.Options...)
	options = append(options, WithTools(modelContext.Tools))
	if stable != nil && stable.SessionKey != "" {
		options = append(options, WithSessionKey(stable.SessionKey))
	}
	for restartCount := 0; ; restartCount++ {
		if restartCount > 1 {
			return ctx, nil, nil, errors.New("model retry maintenance restarted more than once")
		}
		model, err := agent.modelForCall(ctx, modelContext)
		if err != nil {
			return ctx, nil, nil, err
		}
		call := &ModelCall{
			Model: model, Messages: markedRetryMessages(accepted, feedback), Options: append([]ModelOption(nil), options...),
			Streaming: current.Streaming,
		}
		modelContext.maintenanceMessages = cloneMessages(messages)
		for _, middleware := range agent.middlewares {
			ctx, call, err = middleware.BeforeModelCall(ctx, call, modelContext)
			if err != nil {
				return ctx, nil, nil, fmt.Errorf("before model call middleware: %w", err)
			}
			if ctx == nil {
				return nil, nil, nil, errors.New("before model call middleware returned nil Go context")
			}
			if call == nil || call.Model == nil {
				return ctx, nil, nil, errors.New("before model call middleware returned nil model call")
			}
		}
		// Tool schemas and cache routing are stable provider-input identity. A
		// retry policy may alter output/tool choice, but cannot silently fork the
		// conversation cache or drop schemas used by maintenance estimates.
		call.Options = append(call.Options, WithTools(modelContext.Tools))
		if stable != nil && stable.SessionKey != "" {
			call.Options = append(call.Options, WithSessionKey(stable.SessionKey))
		}
		cleaned, feedbackIndexes, markerErr := stripRetryFeedbackMarkers(call.Messages, len(feedback))
		if markerErr != nil {
			return ctx, nil, nil, markerErr
		}
		call.Messages = cleaned
		call.stablePrefixMessages = authenticatedStablePrefixMessages(call.Messages, modelContext.stablePrefixSeed)
		if agent.modelCallGate != nil {
			restart, gateErr := agent.modelCallGate(ctx, call, modelContext)
			if gateErr != nil {
				return ctx, nil, nil, fmt.Errorf("agent model call gate: %w", gateErr)
			}
			if restart != nil {
				if len(restart.Messages) == 0 {
					return ctx, nil, nil, errors.New("agent model call gate returned an empty restart context")
				}
				accepted = cloneMessages(restart.Messages)
				stablePrefixMessages := min(max(0, restart.stablePrefixMessages), len(accepted))
				modelContext.stablePrefixSeed = cloneMessages(accepted[:stablePrefixMessages])
				messages = append(cloneMessages(accepted), cloneMessages(feedback)...)
				ctx = contextWithMaintenanceCommitted(ctx)
				continue
			}
		}
		call.stablePrefixMessages = authenticatedStablePrefixMessages(call.Messages, modelContext.stablePrefixSeed)
		projectedBase, splitErr := removeRetryFeedbackIndexes(call.Messages, feedbackIndexes)
		if splitErr != nil {
			return ctx, nil, nil, splitErr
		}
		return ctx, call, projectedBase, nil
	}
}

func authenticatedStablePrefixMessages(messages, seed []*Message) int {
	if len(seed) == 0 || len(messages) < len(seed) {
		return 0
	}
	for index := range seed {
		equal, err := canonicalMessagesEqual(messages[index], seed[index])
		if err != nil || !equal {
			// A middleware that changes lifecycle-owned prefix bytes invalidates the
			// whole cache boundary. Falling back to zero is safe and observable in
			// maintenance telemetry; content can never extend the trusted prefix.
			return 0
		}
	}
	return len(seed)
}

func retryFeedbackSuffix(base, decision []*Message) ([]*Message, error) {
	if len(decision) < len(base) {
		return nil, errors.New("RetryDecision Messages must preserve the complete accepted model request prefix")
	}
	for index := range base {
		equal, err := canonicalMessagesEqual(base[index], decision[index])
		if err != nil {
			return nil, err
		}
		if !equal {
			return nil, errors.New("RetryDecision Messages must preserve the complete accepted model request prefix")
		}
	}
	return cloneMessages(decision[len(base):]), nil
}

const (
	retryFeedbackMarker       = "__agent_internal_retry_feedback_v1"
	retryFeedbackMarkerPrefix = "feedback:"
)

func markedRetryMessages(accepted, feedback []*Message) []*Message {
	result := cloneMessages(accepted)
	for index, message := range cloneMessages(feedback) {
		if message == nil {
			message = &Message{}
		}
		if message.Extra == nil {
			message.Extra = make(map[string]any)
		}
		// Use a string so a middleware JSON round-trip cannot coerce the marker
		// from int to float64 and make an otherwise valid retry unrecoverable.
		message.Extra[retryFeedbackMarker] = retryFeedbackMarkerPrefix + strconv.Itoa(index+1)
		result = append(result, message)
	}
	return result
}

func stripRetryFeedbackMarkers(messages []*Message, want int) ([]*Message, []int, error) {
	result := cloneMessages(messages)
	indexes := make([]int, 0, want)
	seen := make(map[int]struct{}, want)
	for index, message := range result {
		if message == nil || message.Extra == nil {
			continue
		}
		value, marked := message.Extra[retryFeedbackMarker]
		if !marked {
			continue
		}
		encoded, ok := value.(string)
		if !ok || !strings.HasPrefix(encoded, retryFeedbackMarkerPrefix) {
			return nil, nil, errors.New("retry middleware corrupted an ephemeral feedback marker")
		}
		ordinal, parseErr := strconv.Atoi(strings.TrimPrefix(encoded, retryFeedbackMarkerPrefix))
		if parseErr != nil || ordinal <= 0 || ordinal > want {
			return nil, nil, errors.New("retry middleware corrupted an ephemeral feedback marker")
		}
		if _, duplicate := seen[ordinal]; duplicate {
			return nil, nil, errors.New("retry middleware duplicated an ephemeral feedback message")
		}
		seen[ordinal] = struct{}{}
		delete(message.Extra, retryFeedbackMarker)
		if len(message.Extra) == 0 {
			message.Extra = nil
		}
		indexes = append(indexes, index)
	}
	if len(indexes) != want {
		return nil, nil, errors.New("retry middleware removed an ephemeral feedback message")
	}
	return result, indexes, nil
}

func removeRetryFeedbackIndexes(messages []*Message, indexes []int) ([]*Message, error) {
	if len(indexes) == 0 {
		return cloneMessages(messages), nil
	}
	remove := make(map[int]struct{}, len(indexes))
	for _, index := range indexes {
		if index < 0 || index >= len(messages) {
			return nil, errors.New("context maintenance changed the retry feedback message layout")
		}
		remove[index] = struct{}{}
	}
	result := make([]*Message, 0, len(messages)-len(remove))
	for index, message := range messages {
		if _, ephemeral := remove[index]; !ephemeral {
			result = append(result, message.Clone())
		}
	}
	return result, nil
}

func canonicalMessagesEqual(left, right *Message) (bool, error) {
	leftHash, err := hashCanonical(left)
	if err != nil {
		return false, err
	}
	rightHash, err := hashCanonical(right)
	if err != nil {
		return false, err
	}
	return leftHash == rightHash, nil
}

type modelStreamOutput struct {
	agent                  *modelToolLoop
	events                 *asyncGenerator[*loopEvent]
	writer                 *StreamWriter[*Message]
	registry               *Registry
	toolExecutionNamespace string
	modelResponseOrdinal   int
	activity               func()
}

// openResponseOrdinal returns the identity of the one stream already exposed
// to consumers. A provider failure before the first chunk is retried inside
// that same still-open stream, so the successful retry must keep this ordinal
// for its tool cards, lifecycle events, transcript result, and host
// effects. Once a delivered response is closed, the next attempt receives a
// fresh ordinal.
func (output *modelStreamOutput) openResponseOrdinal() int {
	if output == nil || output.writer == nil {
		return 0
	}
	return output.modelResponseOrdinal
}

func (output *modelStreamOutput) expose(ctx context.Context, responseOrdinal int) {
	if output == nil || output.writer != nil {
		return
	}
	output.modelResponseOrdinal = responseOrdinal
	output.activity = idleActivityFromContext(ctx)
	if scope, ok := InvocationScopeFromContext(ctx); ok {
		output.toolExecutionNamespace = scope.ToolNamespace
	}
	stream, writer := Pipe[*Message](-1)
	output.writer = writer
	event := output.agent.messageEvent(nil, stream, Assistant, "")
	event.Output.MessageOutput.ToolInfos = output.registry.Schemas()
	event.Output.MessageOutput.ToolDefinitions = output.registry.Snapshots()
	event.Output.MessageOutput.ToolExecutionNamespace = output.toolExecutionNamespace
	event.Output.MessageOutput.ModelResponseOrdinal = output.modelResponseOrdinal
	output.events.Send(event)
}

func (output *modelStreamOutput) send(message *Message, err error) {
	if output == nil || output.writer == nil {
		return
	}
	if output.activity != nil {
		output.activity()
	}
	output.writer.Send(message, err)
}

func (output *modelStreamOutput) sendError(err error) {
	output.send(nil, err)
}

func (output *modelStreamOutput) close() {
	if output == nil || output.writer == nil {
		return
	}
	output.writer.Close()
	output.writer = nil
}

func publicStreamError(err error, cancel *cancelControl) error {
	var cancelErr *cancelError
	if errors.As(err, &cancelErr) || (cancel != nil && cancel.isImmediateRequested()) {
		return errStreamCanceled
	}
	return err
}

func (agent *modelToolLoop) retryDecision(
	ctx context.Context,
	attempt int,
	messages []*Message,
	options []ModelOption,
	output *Message,
	err error,
) *RetryDecision {
	if agent.retry == nil {
		return nil
	}
	retryContext := &RetryContext{
		Attempt:       attempt,
		Messages:      cloneMessages(messages),
		OutputMessage: output.Clone(),
		Err:           err,
		Options:       append([]ModelOption(nil), options...),
	}
	if agent.retry.ShouldRetry != nil {
		return agent.retry.ShouldRetry(ctx, retryContext)
	}
	if err == nil {
		return nil
	}
	if agent.retry.IsRetryable != nil && !agent.retry.IsRetryable(ctx, err) {
		return nil
	}
	return &RetryDecision{Retry: true}
}

func (agent *modelToolLoop) callModel(
	ctx context.Context,
	model BaseChatModel,
	registry *Registry,
	messages []*Message,
	options []ModelOption,
	streaming bool,
	events *asyncGenerator[*loopEvent],
	cancel *cancelControl,
	streamOutput *modelStreamOutput,
	responseOrdinal int,
) (*Message, error, bool) {
	if !streaming {
		message, err := awaitContextCall(ctx, func() (*Message, error) {
			return model.Generate(ctx, cloneMessages(messages), options...)
		}, nil, nil)
		if err != nil {
			return nil, err, false
		}
		if message == nil {
			return nil, errors.New("model Generate returned nil message"), false
		}
		if err := ctx.Err(); err != nil {
			return nil, err, false
		}
		message = message.Clone()
		if message.Role == "" {
			message.Role = Assistant
		}
		event := agent.messageEvent(message.Clone(), nil, Assistant, "")
		event.Output.MessageOutput.ToolInfos = registry.Schemas()
		event.Output.MessageOutput.ToolDefinitions = registry.Snapshots()
		if scope, ok := InvocationScopeFromContext(ctx); ok {
			event.Output.MessageOutput.ToolExecutionNamespace = scope.ToolNamespace
		}
		event.Output.MessageOutput.ModelResponseOrdinal = responseOrdinal
		events.Send(event)
		return message, nil, false
	}

	modelStream, err := awaitContextCall(ctx, func() (*StreamReader[*Message], error) {
		return model.Stream(ctx, cloneMessages(messages), options...)
	}, nil, func(stream *StreamReader[*Message]) {
		if stream != nil {
			stream.Close()
		}
	})
	if err != nil {
		return nil, err, false
	}
	if modelStream == nil {
		return nil, errors.New("model Stream returned nil reader"), false
	}
	if err := ctx.Err(); err != nil {
		safeGo(modelStream.Close, func(error) {})
		return nil, err, false
	}
	defer func() {
		if ctx.Err() == nil {
			modelStream.Close()
			return
		}
		safeGo(modelStream.Close, func(error) {})
	}()
	streamOutput.expose(ctx, responseOrdinal)
	chunks := make([]*Message, 0, 16)
	for {
		chunk, recvErr := awaitContextCall(ctx, modelStream.Recv, modelStream.Close, nil)
		if errors.Is(recvErr, io.EOF) {
			if len(chunks) == 0 {
				return nil, errors.New("model stream ended before first message chunk"), false
			}
			message, err := ConcatMessages(chunks)
			if err != nil {
				streamOutput.sendError(err)
				return nil, err, true
			}
			if message.Role == "" {
				message.Role = Assistant
			}
			return message, nil, true
		}
		if recvErr != nil {
			if len(chunks) == 0 {
				return nil, recvErr, false
			}
			streamOutput.sendError(publicStreamError(recvErr, cancel))
			return nil, recvErr, true
		}
		if chunk == nil {
			err := errors.New("model stream returned nil message chunk")
			if len(chunks) == 0 {
				return nil, err, false
			}
			streamOutput.sendError(err)
			return nil, err, true
		}
		chunk = chunk.Clone()
		chunks = append(chunks, chunk.Clone())
		streamOutput.send(chunk.Clone(), nil)
	}
}

func waitContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
