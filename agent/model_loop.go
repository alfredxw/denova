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
	modelCtx, stopModel := context.WithCancel(ctx)
	cancel.bindModel(stopModel)
	defer func() { cancel.bindModel(nil); stopModel() }()
	currentCall := &ModelCall{
		Model: initial.Model, Messages: cloneMessages(initial.Messages),
		Options: append([]ModelOption(nil), initial.Options...), Streaming: initial.Streaming,
		stablePrefixMessages: initial.stablePrefixMessages, providerMessages: cloneMessages(initial.providerMessages),
	}
	acceptedMessages := cloneMessages(initial.Messages)
	stableOptions := initial.Snapshot().ResolvedOptions()
	var retryFeedback []*Message
	var streamOutput *modelStreamOutput
	if initial.Streaming {
		streamOutput = &modelStreamOutput{agent: agent, events: events, registry: registry}
		defer streamOutput.close()
	}
	responseOrdinal := 0
	publish := func(message *Message, action ModelOutputAction) {
		if currentCall.Streaming || message == nil {
			return
		}
		event := agent.messageEvent(message.Clone(), nil, Assistant, "")
		output := event.Output.MessageOutput
		output.ToolInfos, output.ToolDefinitions = registry.Schemas(), registry.Snapshots()
		if scope, ok := InvocationScopeFromContext(ctx); ok {
			output.ToolExecutionNamespace = scope.ToolNamespace
		}
		output.ModelResponseOrdinal, output.previewOnly = responseOrdinal, action == ModelOutputRepair
		events.Send(event)
	}
	message, err := executeModelAttempts(modelCtx, agent.modelMaxAttempts, agent.retry,
		func(attempt int) (modelAttemptResult, error) {
			if attempt > 1 {
				preparedCtx, preparedCall, preparedBase, prepareErr := agent.prepareRetryModelCall(
					ctx, currentCall, initialContext, acceptedMessages, retryFeedback, stableOptions, attempt-1, cancel,
				)
				if prepareErr != nil {
					return modelAttemptResult{}, prepareErr
				}
				ctx, currentCall, acceptedMessages = preparedCtx, preparedCall, preparedBase
			}
			responseOrdinal = nextModelResponseOrdinal(ctx)
			if toolStartReceiptRequired(ctx) {
				boundary := &modelAttemptBoundary{Ordinal: responseOrdinal, Receipt: make(chan error, 1)}
				events.Send(&loopEvent{AgentName: agent.name, Output: &loopOutput{ModelAttempt: boundary}})
				select {
				case err := <-boundary.Receipt:
					if err != nil {
						return modelAttemptResult{}, err
					}
				case <-modelCtx.Done():
					return modelAttemptResult{}, modelCtx.Err()
				}
			}
			providerMessages := currentCall.providerMessages
			if providerMessages == nil {
				var projectionErr error
				providerMessages, projectionErr = projectToolArtifactPaths(ctx, agent.artifacts, currentCall.Messages)
				if projectionErr != nil {
					return modelAttemptResult{}, projectionErr
				}
			}
			callCtx, stopCall := context.WithCancel(ctx)
			stopPropagation := context.AfterFunc(modelCtx, stopCall)
			output, callErr, delivered := agent.callModel(callCtx, currentCall.Model, registry, providerMessages,
				currentCall.Options, currentCall.Streaming, events, cancel, streamOutput, responseOrdinal)
			stopPropagation()
			stopCall()
			if contextErr := agent.contextError(ctx, cancel); contextErr != nil {
				return modelAttemptResult{}, contextErr
			}
			result := modelAttemptResult{message: output, failure: callErr, outputState: ModelOutputComplete, review: ModelOutputAccept}
			if callErr != nil {
				result.outputState = ModelOutputNone
				if delivered {
					result.outputState = ModelOutputPartial
				}
				return result, nil
			}
			for _, middleware := range agent.middlewares {
				review, reviewErr := middleware.ReviewModelOutput(ctx, ModelOutput{
					Attempt: attempt, Message: output.Clone(), Request: currentCall.Snapshot(),
				})
				if reviewErr != nil {
					return result, fmt.Errorf("review model output: %w", reviewErr)
				}
				switch review.Action {
				case ModelOutputAccept:
					continue
				case ModelOutputRepair:
					feedback, feedbackErr := modelRepairMessages(review)
					if feedbackErr != nil {
						return result, feedbackErr
					}
					retryFeedback = feedback
					result.review, result.reason = review.Action, review.Reason
				default:
					return result, fmt.Errorf("unsupported model output review action %q", review.Action)
				}
				break
			}
			return result, nil
		},
		func(attempt int, result modelAttemptResult, decision RetryDecision) error {
			publish(result.message, ModelOutputRepair)
			if streamOutput != nil {
				streamOutput.sendError(&modelResponseRejected{reason: decision.Reason})
				streamOutput.close()
			}
			events.Send(&loopEvent{AgentName: agent.name, Output: &loopOutput{ModelRetry: &ModelRetry{
				Attempt: attempt, MaxAttempts: agent.modelMaxAttempts, ResponseOrdinal: responseOrdinal,
				OutputState: result.outputState, Delay: decision.Delay, Reason: decision.Reason,
			}}})
			return nil
		},
	)
	if err != nil {
		if streamOutput != nil {
			streamOutput.sendError(publicStreamError(err, cancel))
		}
		return nil, 0, acceptedMessages, ctx, err
	}
	publish(message, ModelOutputAccept)
	return message, responseOrdinal, acceptedMessages, ctx, nil
}
func (agent *modelToolLoop) prepareRetryModelCall(
	ctx context.Context,
	current *ModelCall,
	initial *ModelContext,
	accepted, feedback []*Message,
	stable *Options,
	attempt int,
	cancel *cancelControl,
) (context.Context, *ModelCall, []*Message, error) {
	step, err := agent.prepareRetryCall(ctx, current, initial, accepted, feedback, stable, attempt)
	if err != nil {
		return ctx, nil, nil, err
	}
	if agent.modelCallGate != nil {
		replacement, gateErr := agent.applyModelCallGate(step.ctx, step.call, step.modelContext, cancel)
		if gateErr != nil {
			return ctx, nil, nil, fmt.Errorf("agent model call gate: %w", gateErr)
		}
		if replacement != nil {
			step = replacement
		}
	}
	base, err := removeRetryFeedbackIndexes(step.call.Messages, step.feedbackIndexes)
	if err != nil {
		return ctx, nil, nil, err
	}
	initial.stablePrefixSeed = cloneMessages(step.modelContext.stablePrefixSeed)
	return step.ctx, step.call, base, nil
}

func (agent *modelToolLoop) prepareRetryCall(
	ctx context.Context, current *ModelCall, initial *ModelContext,
	accepted, feedback []*Message, stable *Options, attempt int,
) (*preparedModelCall, error) {
	entryCtx := ctx
	modelContext := &ModelContext{
		Tools: cloneToolInfos(initial.Tools), Iteration: initial.Iteration, Attempt: attempt,
		stablePrefixSeed: cloneMessages(initial.stablePrefixSeed), instruction: initial.instruction,
	}
	options := append([]ModelOption(nil), current.Options...)
	options = append(options, WithTools(modelContext.Tools))
	if stable != nil && stable.SessionKey != "" {
		options = append(options, WithSessionKey(stable.SessionKey))
	}
	model, err := agent.modelForCall(ctx, modelContext)
	if err != nil {
		return nil, err
	}
	call := &ModelCall{Model: model, Messages: markedRetryMessages(accepted, feedback), Options: options, Streaming: current.Streaming}
	modelContext.maintenanceMessages = append(cloneMessages(accepted), cloneMessages(feedback)...)
	ctx, call, err = agent.beforeModelCall(ctx, call, modelContext)
	if err != nil {
		return nil, err
	}
	// Retry feedback is request-local; schemas and cache routing remain stable.
	call.Options = append(call.Options, WithTools(modelContext.Tools))
	if stable != nil && stable.SessionKey != "" {
		call.Options = append(call.Options, WithSessionKey(stable.SessionKey))
	}
	cleaned, feedbackIndexes, err := stripRetryFeedbackMarkers(call.Messages, len(feedback))
	if err != nil {
		return nil, err
	}
	call.Messages = cleaned
	call.stablePrefixMessages = authenticatedStablePrefixMessages(call.Messages, modelContext.stablePrefixSeed)
	modelContext.prepareCompaction = func(messages []*Message, prefix int) (*preparedModelCall, error) {
		if modelContext.instruction != "" {
			messages = append([]*Message{SystemMessage(modelContext.instruction)}, messages...)
			prefix++
		}
		nextContext := *modelContext
		nextContext.stablePrefixSeed = cloneMessages(messages[:min(prefix, len(messages))])
		next, err := agent.prepareRetryCall(contextWithMaintenanceCommitted(entryCtx), call, &nextContext, messages, feedback, stable, attempt)
		if err != nil {
			return nil, err
		}
		return agent.freezeCompactionCall(next)
	}
	return &preparedModelCall{ctx: ctx, call: call, modelContext: modelContext, feedbackIndexes: feedbackIndexes}, nil
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
			return nil, recvErr, true
		}
		if chunk == nil {
			err := errors.New("model stream returned nil message chunk")
			if len(chunks) == 0 {
				return nil, err, false
			}
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
