package agent

import (
	"encoding/json"
	"testing"
)

func TestUnknownToolEffectResultIsPublicProviderNeutralRecoveryProtocol(t *testing.T) {
	var payload struct {
		Schema         string `json:"schema"`
		Status         string `json:"status"`
		AutomaticRetry bool   `json:"automatic_retry"`
		Message        string `json:"message"`
	}
	if err := json.Unmarshal([]byte(UnknownToolEffectResult), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Schema != "agent.tool_result.recovery.v1" || payload.Status != "effect_unknown" ||
		payload.AutomaticRetry || payload.Message == "" {
		t.Fatalf("unknown tool effect protocol = %#v", payload)
	}
}
