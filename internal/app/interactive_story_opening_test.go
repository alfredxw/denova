package app

import (
	"strings"
	"testing"

	booklore "denova/internal/book/lore"
	"denova/internal/interactive"
)

func TestInteractiveStoryOpeningCatalogKeepsAllCharactersEligible(t *testing.T) {
	workspace := t.TempDir()
	loreStore := booklore.NewStore(workspace)
	for _, item := range []booklore.ItemInput{
		{ID: "companion", Type: "character", Name: "顾岚", Tags: []string{"同伴"}, BriefDescription: "机关师"},
		{ID: "rival", Type: "character", Name: "陆沉", Tags: []string{"对手"}, BriefDescription: "剑客"},
	} {
		if _, err := loreStore.Create(item); err != nil {
			t.Fatal(err)
		}
	}
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "雾港"})
	if err != nil {
		t.Fatal(err)
	}
	service := &InteractiveAppService{app: &App{workspace: workspace, interactive: store}}
	instruction, err := service.InteractiveStoryOpeningInstruction(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"select_story_protagonist", `lore_item_id="companion"`, `lore_item_id="rival"`, "Every listed character is eligible regardless of tags"} {
		if !strings.Contains(instruction, expected) {
			t.Fatalf("opening candidate catalog missing %q: %s", expected, instruction)
		}
	}
}

func TestInteractiveStoryOpeningResolvesTaggedDefaultBeforeAgentRun(t *testing.T) {
	workspace := t.TempDir()
	if _, err := booklore.NewStore(workspace).Create(booklore.ItemInput{
		ID: "hero", Type: "character", Name: "林川", Tags: []string{"主角"}, Content: "失忆的领航员。",
	}); err != nil {
		t.Fatal(err)
	}
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "雾港"})
	if err != nil {
		t.Fatal(err)
	}
	service := &InteractiveAppService{app: &App{workspace: workspace, interactive: store}}
	instruction, err := service.InteractiveStoryOpeningInstruction(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(instruction, "select_story_protagonist") {
		t.Fatalf("a tagged default should resolve before the Agent run: %s", instruction)
	}
	storyContext, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if storyContext.Meta.Protagonist.Mode != interactive.StoryProtagonistModeLore || storyContext.Meta.Protagonist.SourceLoreItemID != "hero" {
		t.Fatalf("tagged default was not frozen: %#v", storyContext.Meta.Protagonist)
	}
}
