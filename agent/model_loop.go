package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
)

func (agent *Agent) modelForCall(ctx context.Context, modelContext *ModelContext) (BaseChatModel, error) {
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

func (agent *Agent) callModelWithRetry(
	ctx context.Context,
	model BaseChatModel,
	registry *Registry,
	messages []*Message,
	modelOptions []ModelOption,
	streaming bool,
	events *AsyncGenerator[*AgentEvent],
	cancel *cancelControl,
) (*Message, error) {
	currentMessages := messages
	currentOptions := modelOptions
	var streamOutput *modelStreamOutput
	if streaming {
		streamOutput = &modelStreamOutput{agent: agent, events: events, registry: registry}
		defer streamOutput.close()
	}
	maxRetries := 0
	if agent.retry != nil {
		maxRetries = agent.retry.MaxRetries
	}
	for attempt := 0; ; attempt++ {
		message, err, deliveredOnStream := agent.callModel(ctx, model, registry, currentMessages, currentOptions, streaming, events, cancel, streamOutput)
		if contextErr := agent.contextError(ctx, cancel); contextErr != nil {
			if streamOutput != nil && !deliveredOnStream {
				streamOutput.sendError(publicStreamError(contextErr, cancel))
			}
			return nil, contextErr
		}
		if err != nil && deliveredOnStream {
			return nil, err
		}
		decision := agent.retryDecision(ctx, attempt, currentMessages, currentOptions, message, err)
		if decision == nil || !decision.Retry {
			if err != nil && streamOutput != nil {
				streamOutput.sendError(err)
			}
			return message, err
		}
		if attempt >= maxRetries {
			if err != nil {
				if streamOutput != nil {
					streamOutput.sendError(err)
				}
				return nil, err
			}
			return nil, fmt.Errorf("model output rejected after %d retries", maxRetries)
		}
		if deliveredOnStream && streamOutput != nil {
			// A completed response rejected by policy remains one visible stream;
			// the retry gets a fresh stream event. Pre-first-chunk failures keep the
			// current stream open so callers never observe a failed attempt.
			streamOutput.close()
		}
		if decision.Messages != nil {
			currentMessages = cloneMessages(decision.Messages)
		}
		if decision.Options != nil {
			currentOptions = append([]ModelOption(nil), decision.Options...)
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
				return nil, err
			}
		}
	}
}

type modelStreamOutput struct {
	agent    *Agent
	events   *AsyncGenerator[*AgentEvent]
	writer   *StreamWriter[*Message]
	registry *Registry
}

func (output *modelStreamOutput) expose() {
	if output == nil || output.writer != nil {
		return
	}
	stream, writer := Pipe[*Message](-1)
	output.writer = writer
	event := output.agent.messageEvent(nil, stream, Assistant, "")
	event.Output.MessageOutput.ToolInfos = output.registry.Schemas()
	event.Output.MessageOutput.ToolDefinitions = output.registry.Snapshots()
	output.events.Send(event)
}

func (output *modelStreamOutput) send(message *Message, err error) {
	if output == nil || output.writer == nil {
		return
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
	var cancelErr *CancelError
	if errors.As(err, &cancelErr) || (cancel != nil && cancel.isImmediateRequested()) {
		return ErrStreamCanceled
	}
	return err
}

func (agent *Agent) retryDecision(
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

func (agent *Agent) callModel(
	ctx context.Context,
	model BaseChatModel,
	registry *Registry,
	messages []*Message,
	options []ModelOption,
	streaming bool,
	events *AsyncGenerator[*AgentEvent],
	cancel *cancelControl,
	streamOutput *modelStreamOutput,
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
	streamOutput.expose()
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
