package interactive

import (
	"testing"

	"denova/internal/interactive/director"
)

func TestStorePersistsStoryDirectorRunPolicy(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{
		Title: "定时导演",
		DirectorRunPolicy: &director.RunPolicy{
			Mode:          director.RunModeInterval,
			IntervalTurns: 4,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if story.DirectorRunPolicy == nil || story.DirectorRunPolicy.Mode != director.RunModeInterval || story.DirectorRunPolicy.IntervalTurns != 4 {
		t.Fatalf("create result should expose the persisted policy: %#v", story.DirectorRunPolicy)
	}
	storyContext, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if storyContext.Meta.DirectorRunPolicy == nil || *storyContext.Meta.DirectorRunPolicy != *story.DirectorRunPolicy {
		t.Fatalf("story metadata should preserve the policy: %#v", storyContext.Meta.DirectorRunPolicy)
	}

	manual := director.RunPolicy{Mode: director.RunModeManual}
	updated, err := store.UpdateStory(story.ID, UpdateStoryRequest{DirectorRunPolicy: &manual})
	if err != nil {
		t.Fatal(err)
	}
	if updated.DirectorRunPolicy == nil || updated.DirectorRunPolicy.Mode != director.RunModeManual || updated.DirectorRunPolicy.IntervalTurns != 0 {
		t.Fatalf("story update should replace the policy: %#v", updated.DirectorRunPolicy)
	}
}
