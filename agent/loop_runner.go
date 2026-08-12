package agent

import (
	"context"
	"errors"
)

// loopRunnerConfig configures the thin Agent entry point.
type loopRunnerConfig struct {
	Agent           loopRunnable
	EnableStreaming bool
}

// loopRunner delegates execution without owning durable checkpoints.
type loopRunner struct {
	agent           loopRunnable
	enableStreaming bool
}

type modelRequestCaptureKey struct{}

type preparedModelRequest struct {
	snapshot *ModelRequestSnapshot
}

func modelRequestCaptureRequested(ctx context.Context) bool {
	requested, _ := ctx.Value(modelRequestCaptureKey{}).(bool)
	return requested
}

// newLoopRunner constructs a thin runner. Each Run uses its own context.
func newLoopRunner(config loopRunnerConfig) *loopRunner {
	return &loopRunner{agent: config.Agent, enableStreaming: config.EnableStreaming}
}

// Run delegates a caller-owned transcript snapshot to the configured Agent.
func (runner *loopRunner) Run(ctx context.Context, messages []*Message, opts ...loopRunOption) *asyncIterator[*loopEvent] {
	if runner == nil || runner.agent == nil {
		iterator, generator := newAsyncIteratorPair[*loopEvent]()
		generator.Send(&loopEvent{Err: errors.New("runner: agent is required")})
		generator.Close()
		return iterator
	}
	return runner.agent.Run(ctx, &loopInput{
		Messages:        cloneMessages(messages),
		EnableStreaming: runner.enableStreaming,
	}, opts...)
}

// Query starts a run with one user message.
func (runner *loopRunner) Query(ctx context.Context, query string, opts ...loopRunOption) *asyncIterator[*loopEvent] {
	return runner.Run(ctx, []*Message{UserMessage(query)}, opts...)
}

// PrepareModelRequest runs the exact first-call assembly pipeline without
// invoking the provider. It is intended for host-owned side requests such as
// manual context compaction that must reuse the real model, system prompt,
// tools, wrappers, and cache-sensitive options.
func (runner *loopRunner) PrepareModelRequest(ctx context.Context, messages []*Message) (*ModelRequestSnapshot, error) {
	return runner.prepareModelRequest(ctx, messages, 0)
}

func (runner *loopRunner) prepareModelRequest(ctx context.Context, messages []*Message, stablePrefixMessages int) (*ModelRequestSnapshot, error) {
	if runner == nil || runner.agent == nil {
		return nil, errors.New("runner: agent is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if stablePrefixMessages < 0 || stablePrefixMessages > len(messages) {
		return nil, errors.New("runner: stable prefix boundary is outside messages")
	}
	iterator := runner.agent.Run(context.WithValue(ctx, modelRequestCaptureKey{}, true), &loopInput{
		Messages: cloneMessages(messages), EnableStreaming: runner.enableStreaming,
		stablePrefixMessages: stablePrefixMessages,
	})
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event == nil {
			continue
		}
		if event.Err != nil {
			return nil, event.Err
		}
		if event.Action == nil {
			continue
		}
		prepared, ok := event.Action.CustomizedAction.(preparedModelRequest)
		if ok && prepared.snapshot != nil {
			return prepared.snapshot, nil
		}
	}
	return nil, errors.New("runner: model request preparation completed without a snapshot")
}
