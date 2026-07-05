package interactive

import "testing"

func TestActorStatePatchValidationAndReplay(t *testing.T) {
	maxHP := 12.0
	system := normalizeActorStateSystem(StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{{
			ID:   "protagonist",
			Name: "主角",
			Fields: []ActorStateField{{
				ID:         "hp",
				Path:       "resources.hp",
				Name:       "生命",
				Type:       "number",
				Default:    float64(10),
				Min:        floatPtr(0),
				Max:        &maxHP,
				Visibility: "visible",
			}, {
				ID:         "condition",
				Path:       "conditions.main",
				Name:       "状态",
				Type:       "enum",
				Options:    []string{"normal", "wounded"},
				Default:    "normal",
				Visibility: "spoiler",
			}},
		}},
		InitialActors: []ActorStateInitialActor{{
			ID:         DefaultActorID,
			Name:       "主角",
			TemplateID: "protagonist",
			Role:       "protagonist",
		}},
	})
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{
		Title:           "Actor 状态",
		StoryTellerID:   "classic",
		InitialStateOps: actorStateInitialOps(system),
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "冒险", Narrative: "主角受伤但仍能行动。"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := ValidateActorStatePatches(system, []ActorStatePatch{{
		ActorID:    DefaultActorID,
		ActorName:  "主角",
		TemplateID: "protagonist",
		State: map[string]any{
			"resources.hp":    float64(7),
			"conditions.main": "wounded",
		},
		Reason: "本回合主角受伤。",
	}}, turn.ID)
	if err != nil {
		t.Fatalf("valid actor patch should pass: %v", err)
	}
	if len(result.Ops) == 0 || result.Ops[0].SourceTurnID != turn.ID {
		t.Fatalf("actor patch should produce traced state ops: %#v", result.Ops)
	}
	if _, err := store.AppendStateDelta(story.ID, AppendStateDeltaRequest{ParentID: turn.ID, BranchID: "main", Ops: result.Ops}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if got := numberFromAny(getPath(snapshot.State, "actors.protagonist.state.resources.hp")); got != 7 {
		t.Fatalf("actor hp should replay from actor path, got %v state=%#v", got, snapshot.State)
	}
	if got := numberFromAny(getPath(snapshot.State, "resources.hp")); got != 7 {
		t.Fatalf("legacy hp path should read actor state, got %v state=%#v", got, snapshot.State)
	}
	if got := getPath(snapshot.State, "actors.protagonist.state.conditions.main"); got != "wounded" {
		t.Fatalf("enum state should replay, got %#v", got)
	}
}

func TestActorStatePatchRejectsInvalidFieldAndType(t *testing.T) {
	system := normalizeActorStateSystem(StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{{
			ID:   "antagonist",
			Name: "反派",
			Fields: []ActorStateField{{
				ID:   "threat",
				Path: "attributes.threat",
				Name: "威胁",
				Type: "number",
			}},
		}},
	})
	if _, err := ValidateActorStatePatches(system, []ActorStatePatch{{
		ActorID:    "villain",
		TemplateID: "missing",
		State:      map[string]any{"attributes.threat": float64(3)},
	}}, "turn-1"); err == nil {
		t.Fatal("missing template should be rejected")
	}
	if _, err := ValidateActorStatePatches(system, []ActorStatePatch{{
		ActorID:    "villain",
		TemplateID: "antagonist",
		State:      map[string]any{"attributes.unknown": float64(3)},
	}}, "turn-1"); err == nil {
		t.Fatal("unknown field should be rejected")
	}
	if _, err := ValidateActorStatePatches(system, []ActorStatePatch{{
		ActorID:    "villain",
		TemplateID: "antagonist",
		State:      map[string]any{"attributes.threat": "high"},
	}}, "turn-1"); err == nil {
		t.Fatal("type mismatch should be rejected")
	}
}

func floatPtr(value float64) *float64 {
	return &value
}
