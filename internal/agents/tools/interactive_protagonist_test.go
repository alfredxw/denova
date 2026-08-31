package tools

import (
	"context"
	"strings"
	"testing"

	"denova/internal/interactive"
)

func TestInteractiveProtagonistToolSelectsAnyEnabledCharacterID(t *testing.T) {
	calledWith := ""
	definitions, err := newInteractiveProtagonistTools(InteractiveContext{
		SelectStoryProtagonist: func(_ context.Context, loreItemID string) (interactive.StoryProtagonist, error) {
			calledWith = loreItemID
			return interactive.StoryProtagonist{Mode: interactive.StoryProtagonistModeLore, Name: "顾岚", SourceLoreItemID: loreItemID}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 1 {
		t.Fatalf("protagonist tool count = %d, want 1", len(definitions))
	}
	info, err := definitions[0].Tool.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "select_story_protagonist" || strings.Contains(info.Desc, "tagged characters only") {
		t.Fatalf("unexpected protagonist tool contract: %#v", info)
	}
	result, err := definitions[0].Tool.Run(context.Background(), `{"lore_item_id":"companion"}`)
	if err != nil {
		t.Fatal(err)
	}
	if calledWith != "companion" || !strings.Contains(result.ModelContent, `"source_lore_item_id": "companion"`) {
		t.Fatalf("unexpected selection call/result: id=%q result=%s", calledWith, result.ModelContent)
	}
}
