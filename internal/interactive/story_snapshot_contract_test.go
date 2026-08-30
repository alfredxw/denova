package interactive

import (
	"encoding/json"
	"testing"
)

func TestSnapshotJSONContractIncludesRuntimeProjectionAndBranchPlan(t *testing.T) {
	snapshot := Snapshot{
		StoryID: "story", BranchID: "main", ContextRevision: 7,
		Turns:                      []TurnEvent{},
		PendingPlayerInputs:        []PlayerInputAcceptedEvent{{ID: "input"}},
		PendingModelContextBatches: []ModelContextBatchEvent{{ID: "batch"}},
		CurrentTurn:                &TurnEvent{ID: "turn"},
		TokenUsageEvents:           []TokenUsageEvent{{ID: "usage"}},
		ContextCompaction:          &ContextCompactionProjection{ID: "compaction"},
		BranchPlan:                 &BranchPlan{Markdown: "Keep the branch coherent."},
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
		"branch_plan", "state",
		"actor_state_schema", "state_schema_initialization", "graph", "turn_count", "turn_start",
		"history_before_cursor", "has_earlier_turns",
	} {
		if _, ok := fields[name]; !ok {
			t.Fatalf("Snapshot JSON field %q is missing: %s", name, encoded)
		}
	}
	if _, ok := fields["director_plan_status"]; ok {
		t.Fatalf("Snapshot leaked the removed Director runtime payload: %s", encoded)
	}
}
