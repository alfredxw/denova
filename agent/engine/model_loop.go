package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	agentexecution "github.com/alfredxw/denova/agent/engine/execution"
	agentmiddleware "github.com/alfredxw/denova/agent/engine/middleware"
	agentasync "github.com/alfredxw/denova/agent/internal/async"
	agentretry "github.com/alfredxw/denova/agent/internal/retry"
	agentevent "github.com/alfredxw/denova/agent/lifecycle/event"
	agentmodel "github.com/alfredxw/denova/agent/model"
	agentstream "github.com/alfredxw/denova/agent/model/stream"
	agentschema "github.com/alfredxw/denova/agent/schema"
	agenttool "github.com/alfredxw/denova/agent/tool"
)

func (agent *modelToolLoop) modelForCall(ctx context.Context, modelContext *modelStepContext) (agentmodel.BaseChatModel, error) {
	model := agent.model
	if toolCalling, ok := model.(agentmodel.ToolCallingChatModel); ok {
		bound, err := toolCalling.WithTools(modelContext.Tools)
		if err != nil {
			return nil, fmt.Errorf("bind model tools: %w", err)
		}
		model = bound
	}
	for index := len(agent.middlewares) - 1; index >= 0; index-- {
		wrapped, err := agent.middlewares[index].WrapModel(ctx, model, &modelContext.ModelContext)
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
	initial *modelCall,
	initialContext *modelStepContext,
	registry *agenttool.Registry,
	events *asyncGenerator[*loopEvent],
	cancel *cancelControl,
	deferFinal bool,
) (*agentschema.Message, int, []*agentschema.Message, context.Context, error) {
	if initial == nil || initial.Model == nil || initialContext == nil {
		return nil, 0, nil, ctx, errors.New("model retry boundary requires an initial model call and context")
	}
	modelCtx, stopModel := context.WithCancel(ctx)
	cancel.bindModel(stopModel)
	defer func() { cancel.bindModel(nil); stopModel() }()
	currentCall := &modelCall{
		Model: initial.Model, Messages: agentschema.CloneMessages(initial.Messages), Options: append([]agentmodel.ModelOption(nil), initial.Options...), Streaming: initial.Streaming,
		modelIdentity: initial.modelIdentity, inputEstimator: initial.inputEstimator,
		stablePrefixMessages: initial.stablePrefixMessages, providerMessages: agentschema.CloneMessages(initial.providerMessages),
	}
	acceptedMessages := agentschema.CloneMessages(initial.Messages)
	stableOptions := initial.Snapshot().ResolvedOptions()
	var retryFeedback []*agentschema.Message
	var streamOutput *modelStreamOutput
	if initial.Streaming && !deferFinal {
		streamOutput = &modelStreamOutput{agent: agent, events: events, registry: registry}
		defer streamOutput.close()
	}
	responseOrdinal := 0
	publish := func(message *agentschema.Message, action agentretry.ModelOutputAction) {
		if currentCall.Streaming && !deferFinal || message == nil {
			return
		}
		event := agent.messageEvent(message.Clone(), nil, agentschema.Assistant, "")
		output := event.Output.MessageOutput
		output.ToolInfos, output.ToolDefinitions = registry.Schemas(), registry.Snapshots()
		if scope, ok := agentexecution.InvocationScopeFromContext(ctx); ok {
			output.ToolExecutionNamespace = scope.ToolNamespace
		}
		output.ModelResponseOrdinal, output.previewOnly = responseOrdinal, action == agentretry.ModelOutputRepair
		output.discarded = deferFinal && len(message.ToolCalls) == 0
		events.Send(event)
	}
	message, err := agentretry.ExecuteModelAttempts(modelCtx, agent.modelMaxAttempts, agent.retry,
		func(attempt int) (agentretry.ModelAttemptResult, error) {
			if attempt > 1 {
				preparedCtx, preparedCall, preparedBase, prepareErr := agent.prepareRetryModelCall(
					ctx, currentCall, initialContext, acceptedMessages, retryFeedback, stableOptions, attempt-1, cancel,
				)
				if prepareErr != nil {
					return agentretry.ModelAttemptResult{}, prepareErr
				}
				ctx, currentCall, acceptedMessages = preparedCtx, preparedCall, preparedBase
			}
			responseOrdinal = agentexecution.NextModelResponseOrdinal(ctx)
			if toolStartReceiptRequired(ctx) {
				boundary := &modelAttemptBoundary{Ordinal: responseOrdinal, Receipt: make(chan error, 1)}
				events.Send(&loopEvent{AgentName: agent.name, Output: &loopOutput{ModelAttempt: boundary}})
				select {
				case err := <-boundary.Receipt:
					if err != nil {
						return agentretry.ModelAttemptResult{}, err
					}
				case <-modelCtx.Done():
					return agentretry.ModelAttemptResult{}, modelCtx.Err()
				}
			}
			providerMessages := currentCall.providerMessages
			if providerMessages == nil {
				var projectionErr error
				providerMessages, projectionErr = projectToolArtifactPaths(ctx, agent.artifacts, currentCall.Messages)
				if projectionErr != nil {
					return agentretry.ModelAttemptResult{}, projectionErr
				}
			}
			size, estimateErr := currentCall.inputEstimator.Estimate(providerMessages, agentmodel.GetCommonOptions(nil, currentCall.Options...).Tools)
			if estimateErr != nil {
				return agentretry.ModelAttemptResult{}, estimateErr
			}
			inputEstimate := agentschema.ModelInputEstimate{
				Version: agentmodel.InputEstimateVersion,
				Tokens:  size.Tokens,
				Model:   currentCall.modelIdentity,
			}
			callCtx, stopCall := context.WithCancel(ctx)
			stopPropagation := context.AfterFunc(modelCtx, stopCall)
			output, callErr, delivered := agent.callModel(callCtx, currentCall.Model, registry, providerMessages,
				currentCall.Options, currentCall.Streaming, events, cancel, streamOutput, responseOrdinal, inputEstimate)
			stopPropagation()
			stopCall()
			if contextErr := agent.contextError(ctx, cancel); contextErr != nil {
				return agentretry.ModelAttemptResult{}, contextErr
			}
			result := agentretry.ModelAttemptResult{Message: output, Failure: callErr, OutputState: agentretry.ModelOutputComplete, Review: agentretry.ModelOutputAccept}
			if callErr != nil {
				result.OutputState = agentretry.ModelOutputNone
				if delivered {
					result.OutputState = agentretry.ModelOutputPartial
				}
				return result, nil
			}
			for _, middleware := range agent.middlewares {
				review, reviewErr := middleware.ReviewModelOutput(ctx, agentmodel.ModelOutput{
					Attempt: attempt, Message: output.Clone(), Request: currentCall.Snapshot(),
				})
				if reviewErr != nil {
					return result, fmt.Errorf("review model output: %w", reviewErr)
				}
				switch review.Action {
				case agentretry.ModelOutputAccept:
					continue
				case agentretry.ModelOutputRepair:
					feedback, feedbackErr := agentmodel.ModelFeedbackMessages(review.Feedback)
					if feedbackErr != nil {
						return result, feedbackErr
					}
					retryFeedback = feedback
					result.Review, result.Reason = review.Action, review.Reason
				default:
					return result, fmt.Errorf("unsupported model output review action %q", review.Action)
				}
				break
			}
			return result, nil
		},
		func(attempt int, result agentretry.ModelAttemptResult, decision agentretry.RetryDecision) error {
			publish(result.Message, agentretry.ModelOutputRepair)
			if streamOutput != nil {
				streamOutput.sendError(&modelResponseRejected{reason: decision.Reason})
				streamOutput.close()
			}
			events.Send(&loopEvent{AgentName: agent.name, Output: &loopOutput{ModelRetry: &agentevent.ModelRetry{
				Attempt: attempt, MaxAttempts: agent.modelMaxAttempts, ResponseOrdinal: responseOrdinal,
				OutputState: result.OutputState, Delay: decision.Delay, Reason: decision.Reason,
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
	publish(message, agentretry.ModelOutputAccept)
	return message, responseOrdinal, acceptedMessages, ctx, nil
}

func (agent *modelToolLoop) prepareRetryModelCall(
	ctx context.Context,
	current *modelCall,
	initial *modelStepContext,
	accepted, feedback []*agentschema.Message,
	stable *agentmodel.Options,
	attempt int,
	cancel *cancelControl,
) (context.Context, *modelCall, []*agentschema.Message, error) {
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
	initial.stablePrefixSeed = agentschema.CloneMessages(step.modelContext.stablePrefixSeed)
	return step.ctx, step.call, base, nil
}

func (agent *modelToolLoop) prepareRetryCall(
	ctx context.Context, current *modelCall, initial *modelStepContext,
	accepted, feedback []*agentschema.Message, stable *agentmodel.Options, attempt int,
) (*preparedModelCall, error) {
	entryCtx := ctx
	modelContext := &modelStepContext{
		ModelContext:     agentmiddleware.ModelContext{Tools: agentschema.CloneToolInfos(initial.Tools), Iteration: initial.Iteration, Attempt: attempt},
		stablePrefixSeed: agentschema.CloneMessages(initial.stablePrefixSeed), instruction: initial.instruction,
	}
	options := append([]agentmodel.ModelOption(nil), current.Options...)
	options = append(options, agentmodel.WithTools(modelContext.Tools))
	if stable != nil && stable.SessionKey != "" {
		options = append(options, agentmodel.WithSessionKey(stable.SessionKey))
	}
	model, err := agent.modelForCall(ctx, modelContext)
	if err != nil {
		return nil, err
	}
	call := &modelCall{Model: model, Messages: markedRetryMessages(accepted, feedback), Options: options, Streaming: current.Streaming}
	modelContext.maintenanceMessages = append(agentschema.CloneMessages(accepted), agentschema.CloneMessages(feedback)...)
	ctx, call, err = agent.beforeModelCall(ctx, call, modelContext)
	if err != nil {
		return nil, err
	}
	// Retry feedback is request-local; schemas and cache routing remain stable.
	call.Options = append(call.Options, agentmodel.WithTools(modelContext.Tools))
	if stable != nil && stable.SessionKey != "" {
		call.Options = append(call.Options, agentmodel.WithSessionKey(stable.SessionKey))
	}
	cleaned, feedbackIndexes, err := stripRetryFeedbackMarkers(call.Messages, len(feedback))
	if err != nil {
		return nil, err
	}
	call.Messages = cleaned
	call.stablePrefixMessages = authenticatedStablePrefixMessages(call.Messages, modelContext.stablePrefixSeed)
	modelContext.prepareCompaction = func(messages []*agentschema.Message, prefix int) (*preparedModelCall, error) {
		if modelContext.instruction != "" {
			messages = append([]*agentschema.Message{agentschema.SystemMessage(modelContext.instruction)}, messages...)
			prefix++
		}
		nextContext := *modelContext
		nextContext.stablePrefixSeed = agentschema.CloneMessages(messages[:min(prefix, len(messages))])
		next, err := agent.prepareRetryCall(agentmiddleware.ContextWithMaintenanceCommitted(entryCtx), call, &nextContext, messages, feedback, stable, attempt)
		if err != nil {
			return nil, err
		}
		return agent.freezeCompactionCall(next)
	}
	return &preparedModelCall{ctx: ctx, call: call, modelContext: modelContext, feedbackIndexes: feedbackIndexes}, nil
}

func authenticatedStablePrefixMessages(messages, seed []*agentschema.Message) int {
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

func markedRetryMessages(accepted, feedback []*agentschema.Message) []*agentschema.Message {
	result := agentschema.CloneMessages(accepted)
	for index, message := range agentschema.CloneMessages(feedback) {
		if message == nil {
			message = &agentschema.Message{}
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

func stripRetryFeedbackMarkers(messages []*agentschema.Message, want int) ([]*agentschema.Message, []int, error) {
	result := agentschema.CloneMessages(messages)
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

func removeRetryFeedbackIndexes(messages []*agentschema.Message, indexes []int) ([]*agentschema.Message, error) {
	if len(indexes) == 0 {
		return agentschema.CloneMessages(messages), nil
	}
	remove := make(map[int]struct{}, len(indexes))
	for _, index := range indexes {
		if index < 0 || index >= len(messages) {
			return nil, errors.New("context maintenance changed the retry feedback message layout")
		}
		remove[index] = struct{}{}
	}
	result := make([]*agentschema.Message, 0, len(messages)-len(remove))
	for index, message := range messages {
		if _, ephemeral := remove[index]; !ephemeral {
			result = append(result, message.Clone())
		}
	}
	return result, nil
}

func canonicalMessagesEqual(left, right *agentschema.Message) (bool, error) {
	leftHash, err := agentschema.HashCanonical(agentschema.CanonicalMessage(left))
	if err != nil {
		return false, err
	}
	rightHash, err := agentschema.HashCanonical(agentschema.CanonicalMessage(right))
	if err != nil {
		return false, err
	}
	return leftHash == rightHash, nil
}

type modelStreamOutput struct {
	agent                  *modelToolLoop
	events                 *asyncGenerator[*loopEvent]
	writer                 *agentstream.StreamWriter[*agentschema.Message]
	registry               *agenttool.Registry
	toolExecutionNamespace string
	modelResponseOrdinal   int
	activity               func()
}

func (output *modelStreamOutput) expose(ctx context.Context, responseOrdinal int) {
	if output == nil || output.writer != nil {
		return
	}
	output.modelResponseOrdinal = responseOrdinal
	output.activity = agentasync.IdleActivityFromContext(ctx)
	if scope, ok := agentexecution.InvocationScopeFromContext(ctx); ok {
		output.toolExecutionNamespace = scope.ToolNamespace
	}
	stream, writer := agentstream.Pipe[*agentschema.Message](-1)
	output.writer = writer
	event := output.agent.messageEvent(nil, stream, agentschema.Assistant, "")
	event.Output.MessageOutput.ToolInfos = output.registry.Schemas()
	event.Output.MessageOutput.ToolDefinitions = output.registry.Snapshots()
	event.Output.MessageOutput.ToolExecutionNamespace = output.toolExecutionNamespace
	event.Output.MessageOutput.ModelResponseOrdinal = output.modelResponseOrdinal
	output.events.Send(event)
}

func (output *modelStreamOutput) send(message *agentschema.Message, err error) {
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
	model agentmodel.BaseChatModel,
	registry *agenttool.Registry,
	messages []*agentschema.Message,
	options []agentmodel.ModelOption,
	streaming bool,
	events *asyncGenerator[*loopEvent],
	cancel *cancelControl,
	streamOutput *modelStreamOutput,
	responseOrdinal int,
	inputEstimate agentschema.ModelInputEstimate,
) (*agentschema.Message, error, bool) {
	if !streaming {
		message, err := agentasync.AwaitContextCall(ctx, func() (*agentschema.Message, error) {
			return model.Generate(ctx, agentschema.CloneMessages(messages), options...)
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
		bindModelInputEstimate(message, inputEstimate)
		if message.Role == "" {
			message.Role = agentschema.Assistant
		}
		return message, nil, false
	}

	modelStream, err := agentasync.AwaitContextCall(ctx, func() (*agentstream.StreamReader[*agentschema.Message], error) {
		return model.Stream(ctx, agentschema.CloneMessages(messages), options...)
	}, nil, func(stream *agentstream.StreamReader[*agentschema.Message]) {
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
		agentasync.SafeGo(modelStream.Close, func(error) {})
		return nil, err, false
	}
	defer func() {
		if ctx.Err() == nil {
			modelStream.Close()
			return
		}
		agentasync.SafeGo(modelStream.Close, func(error) {})
	}()
	streamOutput.expose(ctx, responseOrdinal)
	chunks := make([]*agentschema.Message, 0, 16)
	for {
		chunk, recvErr := agentasync.AwaitContextCall(ctx, modelStream.Recv, modelStream.Close, nil)
		if errors.Is(recvErr, io.EOF) {
			if len(chunks) == 0 {
				return nil, errors.New("model stream ended before first message chunk"), false
			}
			message, err := agentschema.ConcatMessages(chunks)
			if err != nil {
				return nil, err, true
			}
			if message.Role == "" {
				message.Role = agentschema.Assistant
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
		if streamOutput == nil {
			if activity := agentasync.IdleActivityFromContext(ctx); activity != nil {
				activity()
			}
		}
		bindModelInputEstimate(chunk, inputEstimate)
		chunks = append(chunks, chunk.Clone())
		streamOutput.send(chunk.Clone(), nil)
	}
}

// Attach the estimate before publishing either a buffered response or a usage
// chunk, so the live loop and the canonical event consumer receive one pair.
// Never trust a provider-supplied estimate or reuse one from a rejected attempt.
func bindModelInputEstimate(message *agentschema.Message, estimate agentschema.ModelInputEstimate) {
	if message == nil || message.ResponseMeta == nil {
		return
	}
	meta := message.ResponseMeta
	meta.InputEstimate = nil
	if meta.Usage != nil && meta.Usage.PromptTokens > 0 && estimate.Model.Validate("Model") == nil {
		meta.InputEstimate = &estimate
	}
}
