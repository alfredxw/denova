package interactiveapp

import (
	"testing"

	"denova/internal/interactive"
)

func TestConversationKeepsCycleConfigWhileReadingLatestStorySnapshot(t *testing.T) {
	store := interactive.NewStore(t.TempDir())
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:            "Original title",
		StoryTellerID:    "classic",
		PlanningMode:     interactive.StoryPlanningModeDisabled,
		ChoiceCount:      5,
		ReplyTargetChars: 800,
		CheckSettings:    interactive.StoryCheckSettings{RollModifier: 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	cycleContext, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	conversation := NewConversation(store, t.TempDir(), store.Root(), story.ID, "main", "Continue", 800, nil).
		WithCycleStoryConfig(cycleContext.Meta)

	choiceCount := 3
	planningMode := interactive.StoryPlanningModeEnabled
	checkSettings := interactive.StoryCheckSettings{RollModifier: 10}
	selectedProtagonist := interactive.StoryProtagonist{
		Mode: interactive.StoryProtagonistModeCustom, Name: "Mira", Profile: "A patient cartographer.",
	}
	if _, err := store.UpdateStory(story.ID, interactive.UpdateStoryRequest{
		Title:         "Updated title",
		StoryTellerID: "grimdark",
		PlanningMode:  &planningMode,
		ChoiceCount:   &choiceCount,
		CheckSettings: &checkSettings,
		Protagonist:   &selectedProtagonist,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
		BranchID: "main", User: "Open the door", Narrative: "The door opens.",
	}); err != nil {
		t.Fatal(err)
	}

	liveContext, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	got, err := conversation.storyContextForCycle()
	if err != nil {
		t.Fatal(err)
	}
	if got.Meta.Title != cycleContext.Meta.Title ||
		got.Meta.StoryTellerID != cycleContext.Meta.StoryTellerID ||
		got.Meta.PlanningMode != cycleContext.Meta.PlanningMode ||
		got.Meta.ChoiceCount != cycleContext.Meta.ChoiceCount ||
		got.Meta.CheckSettings.RollModifier != cycleContext.Meta.CheckSettings.RollModifier {
		t.Fatalf("cycle config changed after story update: %#v", got.Meta)
	}
	if len(got.Snapshot.Turns) != 1 || got.Snapshot.Turns[0].Narrative != "The door opens." {
		t.Fatalf("cycle did not read the latest story snapshot: %#v", got.Snapshot)
	}
	if got.Meta.Branches["main"].Head != liveContext.Meta.Branches["main"].Head {
		t.Fatalf("cycle kept a stale branch head: got %q want %q", got.Meta.Branches["main"].Head, liveContext.Meta.Branches["main"].Head)
	}
	if got.Meta.Protagonist != selectedProtagonist {
		t.Fatalf("cycle kept a stale structural state: got %#v want %#v", got.Meta.Protagonist, selectedProtagonist)
	}
}
