package interactive

import (
	"bytes"
	interactivestate "denova/internal/interactive/state"
	"os"
	"reflect"
	"testing"
)

func TestOpeningSelectsStateTemplatesAndReplaysCreatedActors(t *testing.T) {
	for _, templateID := range []string{"explorer", "scholar"} {
		for _, configured := range []bool{false, true} {
			name := templateID + "/templates-only"
			if configured {
				name = templateID + "/configured-protagonist"
			}
			t.Run(name, func(t *testing.T) {
				workspace := t.TempDir()
				store := NewStore(workspace)
				base := StoryDirectorActorStateSystem{
					Templates: []ActorStateTemplate{
						{ID: "explorer", Name: "Explorer", Fields: []ActorStateField{{Name: "supplies", Type: "number"}}},
						{ID: "scholar", Name: "Scholar", Fields: []ActorStateField{{Name: "insight", Type: "number"}}},
						actorStateTemplateByID(GeneratedStoryActorStateCore(), ActorStateStoryContextTemplateID),
					},
				}
				actorID := "林川"
				if configured {
					actorID = DefaultActorID
					base.InitialActors = []ActorStateInitialActor{{ID: actorID, Name: "林川", TemplateID: templateID, Role: "protagonist"}}
				}
				policy := StoryStateSchemaPolicy{Mode: StoryStateSchemaModeAdaptTemplate}
				story, err := store.CreateStory(CreateStoryRequest{
					Title: "Template selection", ActorState: &base, StateSchemaPolicy: &policy, ChoiceCount: 2,
					StateSchemaInitialization: &StateSchemaInitializationStatus{
						Mode: policy.Mode, Status: StateSchemaInitializationWaitingOpening, BaseRevision: 1,
					},
				})
				if err != nil {
					t.Fatal(err)
				}
				if story.Events != 0 {
					t.Fatalf("opening materialized Actors before submission: %#v", story)
				}
				fieldID := actorStateTemplateByID(base, templateID).Fields[0].Name
				draft := NewOpeningActorStateSchemaBatchDraft(base, StoryDirectorTRPGSystem{})
				review := draft.SubmitStructureOnly(ActorStateSchemaBatch{Finalize: true, Items: []ActorStateSchemaBatchItem{{
					ItemID: "opening-sheet",
					Requirements: []ActorStateSchemaRequirementReview{{
						Source:      ActorStateSchemaRequirementSource{Kind: "opening", ID: "opening-draft"},
						Requirement: "Track the resource established by the opening", ValuePolicy: ActorStateSchemaValuePolicySchemaOnly,
						ExpectedType: "number", Decision: "covered", TemplateID: templateID, FieldID: fieldID,
					}},
				}}}, ActorStateSchemaBatchAudit{OpeningSourceIDs: []string{"opening-draft"}})
				if !review.Finalized {
					t.Fatalf("opening rejected a valid template binding: %#v", review)
				}
				proposal, ok := draft.FinalProposal()
				if !ok {
					t.Fatal("finalized draft has no proposal")
				}
				state, err := BuildActorStateInitialSnapshot(base, nil)
				if err != nil {
					t.Fatal(err)
				}
				updates := []interactivestate.Update{{
					Op: interactivestate.Create, Path: "/" + actorID,
					Value: map[string]any{"template_id": templateID, "name": "林川", "role": "protagonist", "state": map[string]any{fieldID: 4}},
				}, {
					Op: interactivestate.Create, Path: "/story",
					Value: map[string]any{"template_id": ActorStateStoryContextTemplateID, "name": "故事状态", "state": map[string]any{
						storyContextCurrentLocationField: "旧车站", storyContextCurrentEventField: "林川准备出发",
					}},
				}}
				if configured {
					updates[0] = interactivestate.Update{Op: interactivestate.Replace, Path: "/" + actorID + "/" + fieldID, Value: 4}
				}
				choices := []string{"出发", "等待"}
				prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{
					ActorState: base, CurrentState: state, ChoiceCount: 2, RequireCompleteInitialState: true,
				}, nil, TurnSubmissionInput{StateUpdates: &updates, Choices: &choices})
				if !receipt.Ready {
					t.Fatalf("opening could not create Actors from selected templates: %#v", receipt)
				}
				result := prepared.TurnResult()
				turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
					BranchID: "main", Narrative: "林川在旧车站准备出发。", StateSchemaProposal: &proposal, TurnResult: &result,
				})
				if err != nil {
					t.Fatal(err)
				}
				branch, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: turn.ID, Title: "Alternate route"})
				if err != nil {
					t.Fatal(err)
				}
				journalBefore, err := os.ReadFile(store.storyPath(story.ID))
				if err != nil {
					t.Fatal(err)
				}
				store = NewStore(workspace)
				for _, branchID := range []string{"main", branch.ID} {
					loaded, err := store.StoryContext(story.ID, branchID)
					if err != nil {
						t.Fatal(err)
					}
					if got := getPath(loaded.Snapshot.State, actorStateActorPath(actorID, "template_id")); got != templateID {
						t.Fatalf("replay lost the selected template: got=%v want=%s", got, templateID)
					}
					if got := getPath(loaded.Snapshot.State, actorStateActorPath(actorID, "name")); got != "林川" {
						t.Fatalf("stable protagonist ID replaced the display name: %v", got)
					}
					if got := actorStateFieldValue(loaded.Snapshot.State, actorID, fieldID); got != float64(4) {
						t.Fatalf("replay lost the selected template's state: %v", got)
					}
					if !reflect.DeepEqual(loaded.Meta.ActorStateSchema.System.InitialActors, normalizeActorStateSystem(base).InitialActors) {
						t.Fatal("runtime Actor creation changed reusable initial Actor definitions")
					}
					next := []interactivestate.Update{
						{Op: interactivestate.Delta, Path: "/" + actorID + "/" + fieldID, Value: -1},
						{Op: interactivestate.Replace, Path: "/story/" + storyContextCurrentEventField, Value: "林川离开旧车站"},
					}
					_, nextReceipt := PrepareTurnSubmission(TurnSubmissionContext{
						ActorState: loaded.Meta.ActorStateSchema.System, CurrentState: loaded.Snapshot.State, ChoiceCount: 2,
					}, nil, TurnSubmissionInput{StateUpdates: &next, Choices: &choices})
					if !nextReceipt.Ready {
						t.Fatalf("next turn could not use the replayed binding: %#v", nextReceipt)
					}
				}
				journalAfter, err := os.ReadFile(store.storyPath(story.ID))
				if err != nil || !bytes.Equal(journalBefore, journalAfter) {
					t.Fatalf("reloading rewrote the canonical journal: %v", err)
				}
			})
		}
	}
}

func TestOpeningCreatedActorsRequireCompleteState(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "traveler", Fields: []ActorStateField{{Name: "destination", Type: "string"}},
	}}}
	updates := []interactivestate.Update{{
		Op: interactivestate.Create, Path: "/旅人", Value: map[string]any{"template_id": "traveler", "name": "旅人"},
	}}
	choices := []string{"出发", "等待"}
	_, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: initialStoryState(), ChoiceCount: 2, RequireCompleteInitialState: true,
	}, nil, TurnSubmissionInput{StateUpdates: &updates, Choices: &choices})
	if receipt.Ready || len(receipt.Diagnostics) != 1 || receipt.Diagnostics[0].Code != TurnSubmissionDiagnosticInitialStateIncomplete {
		t.Fatalf("opening accepted a newly created Actor with missing state: %#v", receipt)
	}
}
