package interactive

import (
	"encoding/json"
	"testing"
)

func TestSnapshotJSONContractIncludesRuntimeProjectionAndOmitsDirectorPlan(t *testing.T) {
	snapshot := Snapshot{
		StoryID: "story", BranchID: "main", ContextRevision: 7,
		Turns:                      []TurnEvent{},
		PendingPlayerInputs:        []PlayerInputAcceptedEvent{{ID: "input"}},
		PendingModelContextBatches: []ModelContextBatchEvent{{ID: "batch"}},
		CurrentTurn:                &TurnEvent{ID: "turn"},
		TokenUsageEvents:           []TokenUsageEvent{{ID: "usage"}},
		ContextCompaction:          &ContextCompactionEvent{ID: "compaction"},
		ContextCompactionRemoval:   &ContextCompactionRemovalEvent{ID: "removal"},
		ToolResultCleanup:          &ToolResultCleanupEvent{ID: "cleanup"},
		DirectorPlan:               &DirectorPlan{},
		DirectorPlanStatus:         &DirectorPlanStatus{StoryID: "story"},
		State:                      map[string]any{},
		ActorStateSchema:           &ActorStateSchemaSnapshot{},
		StateSchemaInitialization: &StateSchemaInitializationStatus{
			Mode: "generate", Status: "ready",
		},
		Graph:     StoryGraph{Nodes: []PlotNode{}, Branches: []BranchSummary{}},
		TurnCount: 1, TurnStart: 0, HistoryBeforeCursor: "cursor", HasEarlierTurns: true,
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"story_id", "branch_id", "context_revision", "turns", "pending_player_inputs",
		"pending_model_context_batches", "current_turn", "token_usage_events", "context_compaction",
		"context_compaction_removal", "tool_result_cleanup", "director_plan_status", "state",
		"actor_state_schema", "state_schema_initialization", "graph", "turn_count", "turn_start",
		"history_before_cursor", "has_earlier_turns",
	} {
		if _, ok := fields[name]; !ok {
			t.Fatalf("Snapshot JSON field %q is missing: %s", name, encoded)
		}
	}
	if _, ok := fields["director_plan"]; ok {
		t.Fatalf("Snapshot leaked the dedicated Director API payload: %s", encoded)
	}
}
