package agentruntime

import (
	"context"
	"errors"
	"testing"
	"time"

	"denova/internal/agents/runtime/external"
	"denova/internal/statistics"
	agentmodel "github.com/alfredxw/denova/agent/model"
	agentschema "github.com/alfredxw/denova/agent/schema"
)

type statisticsTestAdapter struct{ calls int }

func (*statisticsTestAdapter) Version() string { return "test" }

func (*statisticsTestAdapter) InputEstimator(external.Input) agentmodel.InputEstimator {
	return agentmodel.InputEstimator{ImageTokens: func(int, int) int { return 19 }}
}
func (adapter *statisticsTestAdapter) Run(context.Context, external.Input, external.Host) (external.Result, error) {
	adapter.calls++
	if adapter.calls == 1 {
		return external.Result{}, external.ErrSessionUnavailable
	}
	if adapter.calls == 2 {
		return external.Result{Usage: &agentschema.TokenUsage{TotalTokens: 10}, CumulativeUsage: &agentschema.TokenUsage{TotalTokens: 1000}}, nil
	}
	return external.Result{Usage: &agentschema.TokenUsage{TotalTokens: 6}, CumulativeUsage: &agentschema.TokenUsage{TotalTokens: 1006}}, errors.New("provider failed after reported usage")
}

func TestExternalStatisticsUseAttemptDeltasAndExcludeCacheRejection(t *testing.T) {
	root := t.TempDir()
	adapter := &statisticsAdapter{Adapter: &statisticsTestAdapter{}, root: root, runtime: "codex", model: "test"}
	if got := adapter.InputEstimator(external.Input{}).ImageTokens(1, 1); got != 19 {
		t.Fatalf("image estimator = %d", got)
	}
	for range 3 {
		_, _ = adapter.Run(context.Background(), external.Input{}, nil)
	}
	var calls []statistics.Call
	if err := statistics.ReadCalls(root, time.Now().Add(-time.Hour), time.Now().Add(time.Hour), func(call statistics.Call) { calls = append(calls, call) }); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0].Kind != statistics.RuntimeTurn || calls[0].Usage.Total != 10 || calls[1].Usage.Total != 6 || !calls[1].Failed {
		t.Fatalf("external observations = %+v", calls)
	}
	var totals statistics.Totals
	for _, call := range calls {
		totals.Add(call)
	}
	if totals.Requests != 0 || totals.ExternalTurns != 2 || totals.Tokens != 16 || totals.DurationMS != 0 {
		t.Fatalf("opaque turns must not become requests: %+v", totals)
	}
}
