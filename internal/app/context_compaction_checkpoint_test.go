package app

import (
	"encoding/json"
	"testing"

	"denova/config"
	agents "denova/internal/agents"
)

func TestManualCompactionRecordsPreserveDurableDiagnostics(t *testing.T) {
	result := agents.ContextCompactionResult{
		Triggered: true, Phase: "manual", TriggerReason: "manual", Epoch: 3, Summary: "bounded checkpoint",
		EstimatedTokensBefore: 900, ObservedPromptTokens: 990, ObservedEstimateTokens: 880,
		TokensBefore: 1010, TokensAfter: 240, ContextWindowTokens: 1200,
		Strategy: "summary", Threshold: 0.85, RecoveryBand: 0.72, TargetRatio: 0.18,
		RetainedTurns: 2, CandidateFingerprint: "sha256:candidate", CandidateGeneration: 7,
	}

	writing := sessionCompactionRecord("cc-writing", config.AgentKindIDE, 0, 4, result)
	game := interactiveCompactionEvent("cc-game", "turn-4", 2, result)
	tests := []struct {
		name      string
		agentKind string
		record    any
		restored  agents.ContextCompactionResult
	}{
		{name: "writing", agentKind: config.AgentKindIDE, record: writing, restored: contextCompactionResultFromSession(writing)},
		{name: "game", agentKind: config.AgentKindInteractiveStory, record: game, restored: contextCompactionResultFromInteractive(game)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data, err := json.Marshal(test.record)
			if err != nil {
				t.Fatal(err)
			}
			var persisted map[string]any
			if err := json.Unmarshal(data, &persisted); err != nil {
				t.Fatal(err)
			}
			for field, want := range map[string]any{
				"estimated_tokens_before":  float64(result.EstimatedTokensBefore),
				"observed_prompt_tokens":   float64(result.ObservedPromptTokens),
				"observed_estimate_tokens": float64(result.ObservedEstimateTokens),
				"recovery_band":            result.RecoveryBand,
				"reason":                   result.TriggerReason,
				"candidate_fingerprint":    result.CandidateFingerprint,
				"candidate_generation":     float64(result.CandidateGeneration),
			} {
				if got := persisted[field]; got != want {
					t.Errorf("%s = %#v, want %#v; record=%s", field, got, want, data)
				}
			}
			if got, want := agents.NewContextCompactionCheckpoint(test.agentKind, test.restored), agents.NewContextCompactionCheckpoint(test.agentKind, result); got != want {
				t.Fatalf("durable checkpoint round trip changed fields:\ngot  %#v\nwant %#v", got, want)
			}
		})
	}
}
