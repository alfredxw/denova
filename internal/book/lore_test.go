package book

import "testing"

func TestLoreStoreCreateUpdateDelete(t *testing.T) {
	store := NewLoreStore(t.TempDir())
	item, err := store.Create(LoreItemInput{
		Type:       "character",
		Name:       "林川",
		Importance: "major",
		Tags:       []string{"主角", "主角"},
		Content:    "## 林川\n\n谨慎。",
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.ID == "" || len(item.Tags) != 1 {
		t.Fatalf("unexpected item: %#v", item)
	}

	updated, err := store.Update(item.ID, LoreItemInput{
		Type:       "location",
		Name:       "黄泉酒馆",
		Importance: "important",
		Content:    "会回应火光。",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Type != "location" || updated.Name != "黄泉酒馆" {
		t.Fatalf("unexpected updated item: %#v", updated)
	}

	if err := store.Delete(item.ID); err != nil {
		t.Fatal(err)
	}
	items, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("items should be empty after delete: %#v", items)
	}
}

func TestLoreStoreApplyOperationsCreatesVersionAndRestores(t *testing.T) {
	store := NewLoreStore(t.TempDir())
	item, err := store.Create(LoreItemInput{
		ID:         "hero",
		Type:       "character",
		Name:       "林川",
		Importance: "major",
		Content:    "旧设定",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := store.ApplyOperations("Agent 整理资料库", []LoreOperation{
		{
			Op: "update",
			ID: item.ID,
			Item: LoreItemInput{
				ID:         item.ID,
				Type:       "character",
				Name:       "林川",
				Importance: "major",
				Tags:       []string{"主角"},
				Content:    "新设定",
			},
		},
		{
			Op: "create",
			Item: LoreItemInput{
				ID:         "base",
				Type:       "location",
				Name:       "黄泉酒馆",
				Importance: "important",
				Content:    "据点。",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Version == nil || len(result.Updated) != 1 || len(result.Created) != 1 {
		t.Fatalf("unexpected apply result: %#v", result)
	}

	versions, err := store.Versions()
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) == 0 {
		t.Fatal("expected at least one lore version")
	}

	restored, err := store.RestoreVersion(result.Version.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored) != 1 || restored[0].Content != "旧设定" {
		t.Fatalf("restore should recover pre-agent snapshot: %#v", restored)
	}
}

func TestUniqueLoreIDFromBaseAppendsSuffixOnCollision(t *testing.T) {
	items := []LoreItem{
		{ID: "world-1780235672765251000"},
		{ID: "world-1780235672765251000-2"},
	}

	got := uniqueLoreIDFromBase(items, "world-1780235672765251000")
	if got != "world-1780235672765251000-3" {
		t.Fatalf("唯一资料 ID 不符合预期: %s", got)
	}
}
