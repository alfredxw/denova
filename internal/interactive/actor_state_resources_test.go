package interactive

import (
	"reflect"
	"testing"

	interactivestate "denova/internal/interactive/state"
)

func TestStoryResourceCapacityChangesAreAtomicAndReplayable(t *testing.T) {
	for _, capacityFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "current-first", true: "capacity-first"}[capacityFirst], func(t *testing.T) {
			dir := t.TempDir()
			system := StoryDirectorActorStateSystem{
				Templates: []ActorStateTemplate{{ID: "protagonist", Fields: []ActorStateField{
					{Name: "生命", Type: "number", Min: presetFloatPointer(0), MaxField: "生命上限"},
					{Name: "生命上限", Type: "number", Min: presetFloatPointer(0)},
				}}},
				InitialActors: []ActorStateInitialActor{
					{ID: "protagonist", TemplateID: "protagonist", State: map[string]any{"生命": 8, "生命上限": 10}},
					{ID: "同伴", TemplateID: "protagonist", State: map[string]any{"生命": 30, "生命上限": 40}},
				},
			}
			store := NewStore(dir)
			story, err := store.CreateStory(CreateStoryRequest{Title: "Resource capacity", ActorState: &system})
			if err != nil {
				t.Fatal(err)
			}
			updates := []interactivestate.Update{
				{Op: interactivestate.Replace, Path: "/protagonist/生命", Value: 18},
				{Op: interactivestate.Replace, Path: "/protagonist/生命上限", Value: 20},
			}
			if capacityFirst {
				updates[0], updates[1] = updates[1], updates[0]
			}
			turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
				BranchID: "main", Narrative: "The protagonist's capacity increases.",
				TurnResult: &TurnResult{Choices: testTurnChoices(), StateUpdates: updates},
			})
			if err != nil {
				t.Fatal(err)
			}
			before, err := store.Snapshot(story.ID, "main")
			if err != nil {
				t.Fatal(err)
			}
			if actorStateFieldValue(before.State, "protagonist", "生命") != float64(18) || actorStateFieldValue(before.State, "protagonist", "生命上限") != float64(20) || actorStateFieldValue(before.State, "同伴", "生命上限") != float64(40) {
				t.Fatalf("resource capacities must remain per Actor: %#v", before.State)
			}
			changes := []TurnStateChange{{ActorID: "protagonist", FieldID: "生命", Change: 10}, {ActorID: "protagonist", FieldID: "生命上限", Change: 5}}
			if capacityFirst {
				changes[0], changes[1] = changes[1], changes[0]
			}
			turn, _, err = store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
				BranchID: "main", Narrative: "Recovery saturates at the new capacity.",
				RuleResolution: &RuleResolution{ID: "resource-recovery", Result: RuleResult{Outcome: "success", StateChanges: changes}},
			})
			if err != nil {
				t.Fatal(err)
			}
			before, err = store.Snapshot(story.ID, "main")
			if err != nil {
				t.Fatal(err)
			}
			if actorStateFieldValue(before.State, "protagonist", "生命") != float64(25) || actorStateFieldValue(before.State, "protagonist", "生命上限") != float64(25) {
				t.Fatalf("automatic recovery must use final capacity: %#v", before.State)
			}
			turn, _, err = store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
				BranchID: "main", Narrative: "Capacity loss and rule damage share one atomic turn.",
				TurnResult:     &TurnResult{Choices: testTurnChoices(), StateUpdates: []interactivestate.Update{{Op: interactivestate.Replace, Path: "/protagonist/生命上限", Value: 10}}},
				RuleResolution: &RuleResolution{ID: "resource-damage", Result: RuleResult{Outcome: "success", StateChanges: []TurnStateChange{{ActorID: "protagonist", FieldID: "生命", Change: -20}}}},
			})
			if err != nil {
				t.Fatal(err)
			}
			before, err = store.Snapshot(story.ID, "main")
			if err != nil {
				t.Fatal(err)
			}
			if actorStateFieldValue(before.State, "protagonist", "生命") != float64(5) || actorStateFieldValue(before.State, "protagonist", "生命上限") != float64(10) {
				t.Fatalf("rule consumption and submitted changes must validate together: %#v", before.State)
			}
			request := sampleTurnCheckRequest()
			recovery := TurnCheckOutcome{Result: "Recovery reaches the increased capacity.", StateChanges: []TurnStateChange{{ActorID: "protagonist", FieldID: "生命", Change: 100}}}
			request.Outcomes = TurnCheckOutcomes{CriticalSuccess: recovery, Success: recovery, Failure: recovery, CriticalFailure: recovery}
			turn, _, err = store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
				BranchID: "main", Narrative: "Recovery also uses submitted capacity growth.",
				TurnResult:     &TurnResult{Choices: testTurnChoices(), StateUpdates: []interactivestate.Update{{Op: interactivestate.Replace, Path: "/protagonist/生命上限", Value: 40}}},
				RuleResolution: &RuleResolution{ID: "resource-reroll", Request: request, Result: RuleResult{Outcome: "success", StateChanges: recovery.StateChanges}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.RerollRuleResolution(story.ID, "resource-reroll", RuleResolutionRerollRequest{BranchID: "main", TurnID: turn.ID}); err != nil {
				t.Fatal(err)
			}
			before, err = store.Snapshot(story.ID, "main")
			if err != nil {
				t.Fatal(err)
			}
			if actorStateFieldValue(before.State, "protagonist", "生命") != float64(40) || actorStateFieldValue(before.State, "protagonist", "生命上限") != float64(40) {
				t.Fatalf("reroll must retain submitted capacity when recomputing recovery: %#v", before.State)
			}
			_, _, err = store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
				BranchID: "main", Narrative: "Invalid capacity reduction.",
				TurnResult: &TurnResult{Choices: testTurnChoices(), StateUpdates: []interactivestate.Update{{Op: interactivestate.Replace, Path: "/protagonist/生命上限", Value: 4}}},
			})
			if err == nil {
				t.Fatal("capacity below current must reject the complete turn")
			}
			branch, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: turn.ID, Title: "Resource branch"})
			if err != nil {
				t.Fatal(err)
			}
			reopened := NewStore(dir)
			for _, branchID := range []string{"main", branch.ID} {
				after, err := reopened.Snapshot(story.ID, branchID)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(before.State, after.State) || len(after.Turns) != 4 || !reflect.DeepEqual(before.ActorStateSchema, after.ActorStateSchema) {
					t.Fatalf("rejected turn or reopen changed committed resource state: %#v", after)
				}
			}
		})
	}
}

func TestResourceReferencesAndPartialInitializationAreRejected(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{ID: "protagonist", Fields: []ActorStateField{
		{Name: "生命", Type: "number", MaxField: "生命上限"},
		{Name: "生命上限", Type: "number"},
	}}}}
	_, err := CompileTurnStateUpdates(system, nil, []interactivestate.Update{{
		Op: interactivestate.Create, Path: "/旅人", Value: map[string]any{"template_id": "protagonist", "name": "旅人", "state": map[string]any{"生命": 8}},
	}}, TurnStateUpdateCompileOptions{})
	if err == nil {
		t.Fatal("a resource must not be initialized without its capacity")
	}
	system.Templates[0].Fields[1].Type = "string"
	if _, err := PreparePortableStoryState(system, StoryDirectorTRPGSystem{}); err == nil {
		t.Fatal("portable state must reject a non-numeric capacity")
	}
}
