package agent

import (
	"context"
	"errors"
)

// RunnerConfig configures the thin Agent entry point.
type RunnerConfig struct {
	Agent           Runnable
	EnableStreaming bool
}

// Runner delegates execution without owning durable checkpoints.
type Runner struct {
	agent           Runnable
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

// NewRunner constructs a thin runner. Each Run uses its own context.
func NewRunner(config RunnerConfig) *Runner {
	return &Runner{agent: config.Agent, enableStreaming: config.EnableStreaming}
}

// Run delegates a caller-owned transcript snapshot to the configured Agent.
func (runner *Runner) Run(ctx context.Context, messages []*Message, opts ...AgentRunOption) *AsyncIterator[*AgentEvent] {
	if runner == nil || runner.agent == nil {
		iterator, generator := NewAsyncIteratorPair[*AgentEvent]()
		generator.Send(&AgentEvent{Err: errors.New("runner: agent is required")})
		generator.Close()
		return iterator
	}
	return runner.agent.Run(ctx, &AgentInput{
		Messages:        cloneMessages(messages),
		EnableStreaming: runner.enableStreaming,
	}, opts...)
}

// Query starts a run with one user message.
func (runner *Runner) Query(ctx context.Context, query string, opts ...AgentRunOption) *AsyncIterator[*AgentEvent] {
	return runner.Run(ctx, []*Message{UserMessage(query)}, opts...)
}

// PrepareModelRequest runs the exact first-call assembly pipeline without
// invoking the provider. It is intended for host-owned side requests such as
// manual context compaction that must reuse the real model, system prompt,
// tools, wrappers, and cache-sensitive options.
func (runner *Runner) PrepareModelRequest(ctx context.Context, messages []*Message) (*ModelRequestSnapshot, error) {
	if runner == nil || runner.agent == nil {
		return nil, errors.New("runner: agent is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	iterator := runner.Run(context.WithValue(ctx, modelRequestCaptureKey{}, true), messages)
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
