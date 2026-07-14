package interactive

import "testing"

func TestNewStateDeltaRejectsSetOperationsWithoutValues(t *testing.T) {
	tests := []struct {
		name  string
		delta StateDeltaEvent
	}{
		{
			name: "story state",
			delta: StateDeltaEvent{
				V: schemaVersion, Type: StoryEventTypeStateDelta, ID: "delta-story", BranchID: "main", Ts: "2026-07-14T00:00:00Z",
				Ops: []StateOp{{Op: "set", Path: "scene.location"}},
			},
		},
		{
			name: "Actor state without ordinary ops",
			delta: StateDeltaEvent{
				V: schemaVersion, Type: StoryEventTypeStateDelta, ID: "delta-actor", BranchID: "main", Ts: "2026-07-14T00:00:00Z",
				ActorOps: []ActorStateOp{{Op: "set", ActorID: "protagonist", FieldID: "生命"}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := storyEventRecordForWrite(test.delta); err == nil {
				t.Fatal("new events must reject set operations whose JSON value is missing/null")
			}
		})
	}
}

func TestLegacySetWithoutValueRemainsReadableAndHasNoReplayEffect(t *testing.T) {
	record, err := decodeStoryEventRecord([]byte(`{"v":1,"type":"state_delta","id":"legacy-null","branch_id":"main","ts":"2026-07-14T00:00:00Z","schema_version":2,"ops":[{"op":"set","path":"scene.location"}],"actor_ops":[{"op":"set","actor_id":"protagonist","field_id":"生命"}]}`))
	if err != nil {
		t.Fatalf("legacy events must remain readable: %v", err)
	}
	var delta StateDeltaEvent
	if err := mapToStruct(record.Raw, &delta); err != nil {
		t.Fatal(err)
	}
	state := map[string]any{
		"scene": map[string]any{"location": "旧城"},
		actorStateRoot: map[string]any{
			"protagonist": map[string]any{"state": map[string]any{"生命": float64(73)}},
		},
	}
	for _, op := range delta.Ops {
		applyStateOp(state, op)
	}
	for _, op := range delta.ActorOps {
		applyActorStateOp(state, op)
	}
	if got := getPathExact(state, "scene.location"); got != "旧城" {
		t.Fatalf("legacy set(null) must not erase story state, got %#v", got)
	}
	if got := actorStateFieldValue(state, "protagonist", "生命"); got != float64(73) {
		t.Fatalf("legacy Actor set(null) must not erase confirmed state, got %#v", got)
	}
}
