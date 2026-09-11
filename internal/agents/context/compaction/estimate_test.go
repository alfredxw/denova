package compaction

import (
	"denova/config"
	"testing"
)

func TestContextProjectionReserveUsesSingleToolResultBoundary(t *testing.T) {
	cfg := &config.Config{
		OpenAIContextWindowTokens: 1_000_000,
		AgentToolResultLimitKB:    192,
	}
	_, toolTokens := EstimateProjectionReserves(cfg, config.AgentKindIDE, 0)
	if want := 192 * 1024 / 3; toolTokens != want {
		t.Fatalf("tool result reserve = %d, want configured single-result boundary %d", toolTokens, want)
	}

	_, disabledTokens := EstimateProjectionReserves(cfg, config.AgentKindAutomation, 0)
	if disabledTokens != 0 {
		t.Fatalf("agent without cross-turn tool retention should reserve 0 tokens, got %d", disabledTokens)
	}
}
