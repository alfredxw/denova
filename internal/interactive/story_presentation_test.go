package interactive

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestPresentationPatchPreservesInvalidAndOmittedSlots(t *testing.T) {
	bg := PresentationMaterial{ItemID: "station", AssetID: "day", Path: "assets/day.png", Name: "Day"}
	hero := PresentationMaterial{ItemID: "hero", AssetID: "calm", Path: "assets/calm.png", Name: "Calm"}
	base := &TurnPresentation{Background: &bg, Characters: []PresentationMaterial{hero}}
	resolve := func(item, asset string) (PresentationMaterial, error) {
		if asset == "missing" {
			return PresentationMaterial{}, errors.New("image unavailable")
		}
		return PresentationMaterial{ItemID: item, AssetID: asset, Path: "assets/" + asset + ".png", Name: asset}, nil
	}
	for _, raw := range []string{"", `null`, `"wrong"`, `{}`, `{"characters":[]}`, `{"characters":null}`, `{"background":{"item_id":"station","asset_id":"missing"},"characters":[{"item_id":"hero"}]}`} {
		t.Run(raw, func(t *testing.T) {
			got, _ := ApplyPresentationPatch(base, json.RawMessage(raw), nil, resolve)
			if !reflect.DeepEqual(got, base) {
				t.Fatalf("previous stage changed: %#v", got)
			}
		})
	}
	got, receipt := ApplyPresentationPatch(base, json.RawMessage(`{"background":{"item_id":"station","asset_id":"missing"},"characters":[{"item_id":"hero","asset_id":"happy"},false,{"item_id":"friend","asset_id":"calm"}]}`), nil, resolve)
	if receipt.Applied != 2 || receipt.Ignored != 2 || got.Background.AssetID != "day" || len(got.Characters) != 2 || got.Characters[0].AssetID != "happy" || got.Characters[1].ItemID != "friend" {
		t.Fatalf("partial success: stage=%#v receipt=%#v", got, receipt)
	}
	cleared, receipt := ApplyPresentationPatch(got, json.RawMessage(`{"background":null,"characters":[{"item_id":"hero","asset_id":null}]}`), nil, resolve)
	if cleared.Background != nil || len(cleared.Characters) != 1 || cleared.Characters[0].ItemID != "friend" || receipt.Applied != 2 {
		t.Fatalf("explicit removal: %#v %#v", cleared, receipt)
	}
	if base.Background.AssetID != "day" || len(base.Characters) != 1 || base.Characters[0].AssetID != "calm" {
		t.Fatal("patch mutated parent snapshot")
	}
	disabled, receipt := ApplyPresentationPatch(base, json.RawMessage(`{"background":null,"characters":[{"item_id":"hero","asset_id":null}]}`), &StoryPresentationSettings{}, resolve)
	if !reflect.DeepEqual(disabled, base) || receipt.Ignored != 2 {
		t.Fatalf("disabled layers changed: %#v", disabled)
	}
}

func TestPresentationSnapshotsAndPreferencesSurviveJournalAndBranching(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	story, err := store.CreateStory(CreateStoryRequest{Title: "Presentation", PlanningMode: StoryPlanningModeDisabled})
	if err != nil {
		t.Fatal(err)
	}
	if !story.PresentationSettings.Background || !story.PresentationSettings.Characters {
		t.Fatal("missing settings must enable both layers")
	}
	firstStage := &TurnPresentation{Background: &PresentationMaterial{ItemID: "station", AssetID: "day", Path: "assets/day.png", Name: "Day"}}
	first, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "Enter", Narrative: "Daylight.", TurnResult: &TurnResult{Choices: testTurnChoices(), Presentation: firstStage}})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "Wait", Narrative: "Night.", TurnResult: &TurnResult{Choices: testTurnChoices(), Presentation: &TurnPresentation{}}})
	if err != nil {
		t.Fatal(err)
	}
	settings := &StoryPresentationSettings{Background: false, Characters: true}
	if _, err := store.UpdateStory(story.ID, UpdateStoryRequest{PresentationSettings: settings}); err != nil {
		t.Fatal(err)
	}
	branch, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: first.ID, Title: "Day branch"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(workspace)
	defer reopened.Close()
	main, err := reopened.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(main.Meta.PresentationSettings, settings) || main.Snapshot.CurrentTurn.TurnResult.Presentation.Background != nil {
		t.Fatal("journal lost preferences or explicit clear")
	}
	branched, err := reopened.StoryContext(story.ID, branch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(branched.Snapshot.CurrentTurn.TurnResult.Presentation, firstStage) {
		t.Fatal("branch inherited another branch's presentation")
	}
	parent, err := reopened.StoryContextAtTurnParent(story.ID, "main", second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(parent.Snapshot.CurrentTurn.TurnResult.Presentation, firstStage) {
		t.Fatal("regeneration did not restore parent presentation")
	}
}
