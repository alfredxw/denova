package interactive

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestDecodeInteractiveTurnSubmissionDoesNotApplyAnOperationCountLimit(t *testing.T) {
	changes := make([]map[string]any, 201)
	for index := range changes {
		changes[index] = map[string]any{
			"op":       TurnStateUpdateReplace,
			"actor_id": fmt.Sprintf("actor-%d", index),
			"field_id": "status",
			"value":    index,
		}
	}
	arguments, err := json.Marshal(map[string]any{"state_changes": changes})
	if err != nil {
		t.Fatal(err)
	}

	input := DecodeInteractiveTurnSubmissionInput(string(arguments))
	if input.StateUpdates == nil || len(*input.StateUpdates) != 201 || len(input.Diagnostics) != 0 {
		t.Fatalf("state_changes should not have an operation-count limit, got %#v", input)
	}
}
