package interactive

import "testing"

func TestResolveRuleStateBindingReadsNestedPanelNumber(t *testing.T) {
	system := normalizeActorStateSystem(StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{{
			ID:   DefaultActorID,
			Name: "主角",
			Fields: []ActorStateField{{
				Name: "面板",
				Type: "object",
				Default: map[string]any{
					"力量": map[string]any{"基础值": float64(12), "当前值": float64(14), "修正说明": "巨力腰带 +2"},
				},
			}},
		}},
		InitialActors: []ActorStateInitialActor{{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID, Role: "protagonist"}},
	})
	director := normalizeStoryDirector(StoryDirector{
		ID:         "nested-panel-binding",
		Name:       "Nested Panel Binding",
		ActorState: system,
		TRPGSystem: StoryDirectorTRPGSystem{RuleTemplates: []RuleCheck{{
			ID: "attribute-check",
			StateBindings: []RuleStateBinding{{
				ID:              "strength-check",
				ActorTemplateID: DefaultActorID,
				Modifiers: []RuleStateBindingModifier{{
					Source:    "actor",
					FieldID:   "面板",
					ValuePath: []string{"力量", "当前值"},
					Effect:    "advantage",
				}},
			}},
		}}},
	})
	state := initialStoryState()
	initialOps, initialActorOps, err := BuildActorStateInitialChanges(system, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range initialOps {
		applyStateOp(state, op)
	}
	for _, op := range initialActorOps {
		applyActorStateOp(state, op)
	}

	audit, err := resolveRuleStateBinding(state, director, TurnCheckRequest{Rule: TurnCheckRule{
		TemplateID: "attribute-check",
		BindingID:  "strength-check",
		ActorID:    DefaultActorID,
	}})
	if err != nil {
		t.Fatalf("nested panel binding should resolve: %v", err)
	}
	if audit == nil || audit.BindingBonusTotal != 14 || len(audit.StateInputs) != 1 {
		t.Fatalf("nested panel value should feed the binding: %#v", audit)
	}
	if got := audit.StateInputs[0].ValuePath; len(got) != 2 || got[0] != "力量" || got[1] != "当前值" {
		t.Fatalf("audit should retain the nested value path: %#v", audit.StateInputs[0])
	}
}
