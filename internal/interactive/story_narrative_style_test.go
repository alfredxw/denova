package interactive

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStoryNarrativeRulesSurviveJournalRebuildAndResetOnStyleChange(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	story, err := store.CreateStory(CreateStoryRequest{Title: "Local rules", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	enabled, content := false, ""
	overrides := &StoryNarrativeStyleOverrides{StyleID: "classic", Slots: map[string]NarrativeSlotOverride{
		"voice": {Enabled: &enabled, Content: &content},
	}}
	saved, err := store.UpdateStory(story.ID, UpdateStoryRequest{NarrativeStyleOverrides: overrides})
	if err != nil || !reflect.DeepEqual(saved.NarrativeStyleOverrides, overrides) {
		t.Fatalf("save rules: result=%#v err=%v", saved.NarrativeStyleOverrides, err)
	}
	// Remove all derived indexes and reload solely from the canonical journal.
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && filepath.Ext(path) == ".json" {
			return os.Remove(path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(root)
	ctx, err := reopened.StoryContext(story.ID, "main")
	if err != nil || !reflect.DeepEqual(ctx.Meta.NarrativeStyleOverrides, overrides) {
		t.Fatalf("journal rebuild lost rules: result=%#v err=%v", ctx.Meta.NarrativeStyleOverrides, err)
	}
	reset, err := reopened.UpdateStory(story.ID, UpdateStoryRequest{NarrativeStyleOverrides: &StoryNarrativeStyleOverrides{StyleID: "classic", Slots: map[string]NarrativeSlotOverride{}}})
	if err != nil || reset.NarrativeStyleOverrides != nil {
		t.Fatalf("restore defaults: result=%#v err=%v", reset.NarrativeStyleOverrides, err)
	}
	if _, err = reopened.UpdateStory(story.ID, UpdateStoryRequest{NarrativeStyleOverrides: overrides}); err != nil {
		t.Fatal(err)
	}
	switched, err := reopened.UpdateStory(story.ID, UpdateStoryRequest{StoryTellerID: "grimdark"})
	if err != nil || switched.NarrativeStyleOverrides != nil {
		t.Fatalf("style change kept unrelated rules: result=%#v err=%v", switched.NarrativeStyleOverrides, err)
	}
	if _, err = reopened.UpdateStory(story.ID, UpdateStoryRequest{NarrativeStyleOverrides: overrides}); !errors.Is(err, ErrNarrativeStyleOverrides) {
		t.Fatalf("late save for previous style must fail: %v", err)
	}
}
