package interactiveapp

import (
	"context"
	"errors"
	"testing"

	"denova/config"
	agentchat "denova/internal/agents/chat"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/runtime/external"
)

// An engine that keeps returning prose without invoking any host tool makes no
// progress. The turn must end with a recoverable reason instead of prompting the
// engine forever while the user watches a "thinking" state.
func TestExternalGameEndsWhenRoundsMakeNoProgress(t *testing.T) {
	store, story, cfg := externalGameFixture(t)
	cfg.ActiveAgentRuntime = &config.RuntimeSelection{Kind: config.RuntimeClaude, Claude: &config.ClaudeRuntimeSettings{Model: "test"}}
	attempts := 0
	turn := externalGameForTest(t, store, story, cfg, agentchat.ChatRequest{CommandID: "stalled", Message: "Open the gate"},
		gameAdapterFunc(func(context.Context, external.Input, external.Host) (external.Result, error) {
			attempts++
			return external.Result{Text: "The gate stays shut."}, nil
		}))
	outcome := turn.Wait(t.Context())
	if outcome.Status != agentrun.OutcomeFailed || !errors.Is(outcome.Error, external.ErrTurnNotConverged) {
		t.Fatalf("stalled turn did not converge: %+v attempts=%d", outcome, attempts)
	}
	if want := maxStalledRounds + 1; attempts != want {
		t.Fatalf("stalled turn ran %d attempts, want %d", attempts, want)
	}
	if key := external.FailureKey(outcome.Error); key != "agentRuntime.turnIncomplete" {
		t.Fatalf("error key = %q", key)
	}
}
