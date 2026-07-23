package agent

import (
	"context"

	"github.com/alfredxw/denova/adk"
)

// NewRunnerWithOptions connects the product Agent to Denova's durable runtime.
// Durable recovery is owned by agent/runtime and the session journal; the
// native ADK runner deliberately keeps no second checkpoint store.
func NewRunnerWithOptions(ctx context.Context, builtAgent adk.Runnable, _ RunOptions) *adk.Runner {
	return adk.NewRunner(adk.RunnerConfig{
		Agent:           builtAgent,
		EnableStreaming: true,
	})
}
