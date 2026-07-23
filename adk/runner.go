package adk

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
