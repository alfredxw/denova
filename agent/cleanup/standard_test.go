package cleanup_test

import (
	"context"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/cleanup"
)

func TestStandardSeparatesPrimaryOutputCapacityFromCheckpointOutputReserve(t *testing.T) {
	manager := cleanup.Standard(cleanup.StandardConfig{
		ContextWindowTokens:     10_000,
		ReservedTokens:          1000,
		CleanupThreshold:        .70,
		CleanupTarget:           .60,
		CompactionEnabled:       true,
		CompactionThreshold:     .85,
		CompactionPromptTokens:  500,
		CheckpointOutputReserve: 500,
	})
	if err := manager.(agent.DefinitionInitializer).InitializeDefinition(context.Background()); err != nil {
		t.Fatal(err)
	}
	maxOutput := 8000
	messages := []*agent.Message{agent.UserMessage("short current request")}
	plan, err := manager.Plan(context.Background(), agent.CleanupPlanRequest{
		Messages:     messages,
		ModelRequest: messages,
		ModelInspection: agent.ModelRequestInspection{
			Messages: messages,
			Options:  agent.Options{MaxTokens: &maxOutput},
		},
		CompactionAvailable: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Action != agent.CleanupNone || plan.FallbackToCompaction {
		t.Fatalf("short request with available primary output capacity should not compact: %#v", plan)
	}
	want := cleanup.EstimateInspectedTokens(messages, agent.ModelRequestInspection{
		Messages: messages, Options: agent.Options{MaxTokens: &maxOutput},
	}) + 6500
	if plan.Metrics.EstimatedTokensBefore != want {
		t.Fatalf("capacity-aware Cleanup estimate = %d, want %d", plan.Metrics.EstimatedTokensBefore, want)
	}
}
