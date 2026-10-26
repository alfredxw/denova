package agentruntime

import (
	"context"
	"crypto/rand"
	"errors"
	"time"

	"denova/config"
	"denova/internal/agents/runtime/external"
	"denova/internal/statistics"
	agentmodel "github.com/alfredxw/denova/agent/model"
)

// statisticsAdapter observes each host invocation, not the operation aggregate.
// Continuations, maintenance and Goal evaluations are therefore included once.
type statisticsAdapter struct {
	external.Adapter
	root, model, runtime string
}

// Preserve the adapter's optional image-aware estimator at the same boundary;
// observing usage must not change context preparation or media budgets.
func (adapter *statisticsAdapter) InputEstimator(input external.Input) agentmodel.InputEstimator {
	if provider, ok := adapter.Adapter.(interface {
		InputEstimator(external.Input) agentmodel.InputEstimator
	}); ok {
		return provider.InputEstimator(input)
	}
	return agentmodel.InputEstimator{}
}

func observeAdapter(adapter external.Adapter, cfg config.Config, selection config.RuntimeSelection) external.Adapter {
	model := ""
	if settings, err := config.ResolveRuntimeModel(&cfg, selection); err == nil {
		model = settings.Model
	} else if selection.Codex != nil {
		model = selection.Codex.Model
	} else if selection.Claude != nil {
		model = selection.Claude.Model
	}
	return &statisticsAdapter{Adapter: adapter, root: cfg.ProjectStoreDir, model: model, runtime: string(selection.Kind)}
}

func (adapter *statisticsAdapter) Run(ctx context.Context, input external.Input, host external.Host) (result external.Result, err error) {
	started := time.Now().UTC()
	finished := false
	defer func() {
		// This sentinel is a cache miss before work is admitted, not a request.
		if errors.Is(err, external.ErrSessionUnavailable) && result.Usage == nil {
			return
		}
		call := statistics.Call{ID: rand.Text(), At: started, Kind: statistics.RuntimeTurn, Model: adapter.model,
			Runtime: adapter.runtime, DurationMS: time.Since(started).Milliseconds(), Failed: err != nil || !finished}
		if usage := result.Usage; usage != nil {
			call.Usage = &statistics.Usage{Prompt: int64(usage.PromptTokens), Cached: int64(usage.PromptTokenDetails.CachedTokens), Output: int64(usage.CompletionTokens), Total: int64(usage.TotalTokens)}
		}
		statistics.RecordCall(adapter.root, call)
	}()
	result, err = adapter.Adapter.Run(ctx, input, host)
	finished = true
	return result, err
}
