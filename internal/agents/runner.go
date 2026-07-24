package agents

import (
	"context"

	agent "github.com/alfredxw/denova/agent"
)

// NewRunnerWithOptions connects the product Agent to Denova's durable runtime.
// Durable recovery is owned by agent/runtime and the session journal; the
// native Agent runner deliberately keeps no second checkpoint store.
func NewRunnerWithOptions(ctx context.Context, builtAgent agent.Runnable, _ RunOptions) *agent.Runner {
	return agent.NewRunner(agent.RunnerConfig{
		Agent:           builtAgent,
		EnableStreaming: true,
	})
}
