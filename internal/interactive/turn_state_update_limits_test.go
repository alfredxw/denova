package interactive

import (
	"fmt"
	"testing"
)

func TestCompileTurnStateUpdatesDoesNotApplyAnOperationCountLimit(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{ID: "npc"}}}
	updates := make([]StateUpdate, 201)
	for index := range updates {
		actorID := fmt.Sprintf("actor-%d", index)
		updates[index] = StateUpdate{
			Op:    TurnStateUpdateCreate,
			Path:  "/" + actorID,
			Value: map[string]any{"template_id": "npc", "name": actorID},
		}
	}

	compiled, err := CompileTurnStateUpdates(system, map[string]any{}, updates, TurnStateUpdateCompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.Updates) != 201 {
		t.Fatalf("compiled %d state changes, want 201", len(compiled.Updates))
	}
	replayed := map[string]any{}
	for _, op := range compiled.Ops {
		applyStateOp(replayed, op)
	}
	if got := getPath(replayed, actorStateRoot+".actor-200.id"); got != "actor-200" {
		t.Fatalf("compiled ops must preserve the final state change, got last actor id %#v from %d ops", got, len(compiled.Ops))
	}
}

func TestValidateTurnResultDoesNotApplyAnOperationCountLimit(t *testing.T) {
	updates := make([]StateUpdate, 201)
	for index := range updates {
		updates[index] = StateUpdate{
			Op:    TurnStateUpdateReplace,
			Path:  fmt.Sprintf("/actor-%d/status", index),
			Value: index,
		}
	}
	result := TurnResult{
		StateUpdates: updates,
		Choices:      []string{"前进", "观察", "交谈", "等待", "后退"},
	}

	if err := ValidateTurnResult(result); err != nil {
		t.Fatalf("TurnResult should not impose a state change count limit: %v", err)
	}
}
