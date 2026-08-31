package app

import (
	"testing"

	booklore "denova/internal/book/lore"
	"denova/internal/interactive"
)

func TestStoryProtagonistSnapshotFromLoreItem(t *testing.T) {
	snapshot, err := storyProtagonistSnapshotFromLoreItem(booklore.Item{
		ID: "hero", Enabled: true, Type: "character", Name: "林川",
		BriefDescription: "失忆的领航员", Content: "熟悉旧港的每一条水道。",
		UpdatedAt: "2026-08-31T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Mode != interactive.StoryProtagonistModeLore || snapshot.Name != "林川" || snapshot.Profile != "熟悉旧港的每一条水道。" || snapshot.SourceLoreItemID != "hero" || snapshot.SourceLoreUpdatedAt == "" {
		t.Fatalf("unexpected protagonist snapshot: %#v", snapshot)
	}
}

func TestStoryProtagonistSnapshotRejectsUnavailableLore(t *testing.T) {
	for _, item := range []booklore.Item{
		{ID: "disabled", Enabled: false, Type: "character", Name: "林川"},
		{ID: "place", Enabled: true, Type: "location", Name: "雾港"},
	} {
		if _, err := storyProtagonistSnapshotFromLoreItem(item); err == nil {
			t.Fatalf("expected unavailable Lore item to be rejected: %#v", item)
		}
	}
}
