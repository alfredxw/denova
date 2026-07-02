package book

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoreStoreNormalizesProgressiveLoadingDefaults(t *testing.T) {
	workspace := t.TempDir()
	store := NewLoreStore(workspace)
	if err := os.MkdirAll(filepath.Dir(store.itemsPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	data := `{
  "version": 1,
  "items": [
    {"id":"hero","type":"character","name":"林川","importance":"major","tags":["主角"],"content":"主角设定"},
    {"id":"base","type":"location","name":"黄泉酒馆","importance":"important","content":"据点设定"}
  ]
}`
	if err := os.WriteFile(store.itemsPath(), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	items, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]LoreItem{}
	for _, item := range items {
		byID[item.ID] = item
	}
	if byID["hero"].LoadMode != LoreLoadModeResident {
		t.Fatalf("major legacy item should default to resident: %#v", byID["hero"])
	}
	if byID["base"].LoadMode != LoreLoadModeAuto {
		t.Fatalf("important legacy item should default to auto: %#v", byID["base"])
	}
	if !byID["hero"].Enabled || !byID["base"].Enabled {
		t.Fatalf("legacy items should default to enabled: %#v", byID)
	}
	if byID["hero"].Keywords == nil || len(byID["hero"].Keywords) != 0 {
		t.Fatalf("missing keywords should normalize to empty array: %#v", byID["hero"].Keywords)
	}
}

func TestLoreStoreDisabledItemsStayEditableButLeaveModelContext(t *testing.T) {
	store := NewLoreStore(t.TempDir())
	disabled := false
	if _, err := store.Create(LoreItemInput{ID: "visible", Type: "character", Name: "可见角色", Importance: "major", LoadMode: LoreLoadModeResident, Content: "可见正文"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(LoreItemInput{ID: "hidden", Enabled: &disabled, Type: "rule", Name: "禁用规则", Importance: "important", LoadMode: LoreLoadModeAuto, Content: "禁用正文"}); err != nil {
		t.Fatal(err)
	}

	items, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "visible" {
		t.Fatalf("List should only return enabled items: %#v", items)
	}
	all, err := store.ListAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("ListAll should retain disabled items for editing: %#v", all)
	}
	if _, err := store.Read("hidden"); err == nil {
		t.Fatalf("disabled item should not be readable through model-facing Read")
	}
	context, err := store.ProgressiveContextMarkdown()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(context, "禁用规则") || strings.Contains(context, "禁用正文") {
		t.Fatalf("disabled item leaked into progressive context: %s", context)
	}
	results, err := store.Search("禁用", "", 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("disabled item should not be searchable: %#v", results)
	}
}

func TestLoreStoreProgressiveContextSplitsResidentAndIndex(t *testing.T) {
	store := NewLoreStore(t.TempDir())
	if _, err := store.Create(LoreItemInput{ID: "hero", Type: "character", Name: "林川", Importance: "major", LoadMode: LoreLoadModeResident, Content: "主角完整正文"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(LoreItemInput{ID: "base", Type: "location", Name: "黄泉酒馆", Importance: "important", LoadMode: LoreLoadModeAuto, Keywords: []string{"据点"}, BriefDescription: "黄泉酒馆索引简介", Content: "据点完整正文"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(LoreItemInput{ID: "secret", Type: "rule", Name: "隐藏规则", Importance: "minor", LoadMode: LoreLoadModeManual, BriefDescription: "隐藏规则索引简介", Content: "隐藏完整正文"}); err != nil {
		t.Fatal(err)
	}

	context, err := store.ProgressiveContextMarkdown()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(context, "## 常驻资料库") || !strings.Contains(context, "主角完整正文") {
		t.Fatalf("resident context missing full content: %s", context)
	}
	if !strings.Contains(context, "## 资料库索引") || !strings.Contains(context, "base") || !strings.Contains(context, "secret") {
		t.Fatalf("index context missing non-resident items: %s", context)
	}
	if strings.Contains(context, "据点完整正文") || strings.Contains(context, "隐藏完整正文") {
		t.Fatalf("non-resident full content should not be in progressive context: %s", context)
	}
}

func TestLoreStoreCompactIndexOmitsHeavyFieldsAndDisabledItems(t *testing.T) {
	store := NewLoreStore(t.TempDir())
	disabled := false
	if _, err := store.Create(LoreItemInput{ID: "base", Type: "location", Name: "黄泉酒馆", Importance: "important", LoadMode: LoreLoadModeAuto, Tags: []string{"据点"}, Keywords: []string{"黄泉"}, BriefDescription: "地点 黄泉酒馆。索引简介。上下文出现相关内容时，一定要参考本项详情。", Content: "据点完整正文"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(LoreItemInput{ID: "hidden", Enabled: &disabled, Type: "rule", Name: "禁用规则", Importance: "important", LoadMode: LoreLoadModeAuto, BriefDescription: "禁用规则简介", Content: "禁用正文"}); err != nil {
		t.Fatal(err)
	}

	index, err := store.LoreIndexMarkdown(LoreIndexOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# 资料库索引", "id: base", "名称: 黄泉酒馆", "简介: 地点 黄泉酒馆。索引简介。"} {
		if !strings.Contains(index, want) {
			t.Fatalf("compact index missing %q:\n%s", want, index)
		}
	}
	for _, unexpected := range []string{"据点完整正文", "禁用规则", "类型:", "标签:", "重要度:", "加载策略:"} {
		if strings.Contains(index, unexpected) {
			t.Fatalf("compact index should not contain %q:\n%s", unexpected, index)
		}
	}
}

func TestLoreStoreCompactIndexQueryMatchesMetadataAndContentWithoutLeakingContent(t *testing.T) {
	store := NewLoreStore(t.TempDir())
	if _, err := store.Create(LoreItemInput{ID: "archive", Type: "location", Name: "旧档案室", Importance: "important", LoadMode: LoreLoadModeAuto, Keywords: []string{"档案柜"}, BriefDescription: "地点 旧档案室。只有尘封档案。上下文出现相关内容时，一定要参考本项详情。", Content: "暗门后藏着完整原文线索。"}); err != nil {
		t.Fatal(err)
	}

	keywordIndex, err := store.LoreIndexMarkdown(LoreIndexOptions{Query: "档案柜"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(keywordIndex, "id: archive") || !strings.Contains(keywordIndex, "匹配: 关键词") {
		t.Fatalf("keyword query should return match source:\n%s", keywordIndex)
	}

	contentIndex, err := store.LoreIndexMarkdown(LoreIndexOptions{Query: "完整原文"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(contentIndex, "id: archive") || !strings.Contains(contentIndex, "匹配: 正文") {
		t.Fatalf("content query should return content match source:\n%s", contentIndex)
	}
	if strings.Contains(contentIndex, "暗门后藏着完整原文线索") {
		t.Fatalf("query index should not leak full content:\n%s", contentIndex)
	}
}

func TestLoreStoreCompactIndexBudgetFallsBackToNameRoster(t *testing.T) {
	store := NewLoreStore(t.TempDir())
	longBrief := strings.Repeat("很长简介", 90)
	for i := 0; i < 8; i++ {
		id := fmt.Sprintf("item_%02d", i)
		if _, err := store.Create(LoreItemInput{ID: id, Type: "other", Name: "资料" + id, Importance: "important", LoadMode: LoreLoadModeAuto, BriefDescription: longBrief, Content: "正文"}); err != nil {
			t.Fatal(err)
		}
	}

	index, err := store.LoreIndexMarkdown(LoreIndexOptions{MaxBytes: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if len([]byte(index)) > 1000 {
		t.Fatalf("budgeted index bytes = %d, want <= 1000\n%s", len([]byte(index)), index)
	}
	if !strings.Contains(index, "已降级为仅 ID 和名称") {
		t.Fatalf("budgeted index should explain name-only fallback:\n%s", index)
	}
	if strings.Contains(index, "简介:") {
		t.Fatalf("name-only fallback should omit briefs:\n%s", index)
	}
	for i := 0; i < 8; i++ {
		id := fmt.Sprintf("item_%02d", i)
		if !strings.Contains(index, "id: "+id) || !strings.Contains(index, "名称: 资料"+id) {
			t.Fatalf("name-only fallback should retain full roster entry %s:\n%s", id, index)
		}
	}
}

func TestLoreStoreStoryMemoryContextIncludesBoundedFullLore(t *testing.T) {
	store := NewLoreStore(t.TempDir())
	if _, err := store.Create(LoreItemInput{ID: "hero", Type: "character", Name: "林川", Importance: "major", LoadMode: LoreLoadModeResident, Content: "主角完整正文"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(LoreItemInput{ID: "base", Type: "location", Name: "黄泉酒馆", Importance: "important", LoadMode: LoreLoadModeAuto, BriefDescription: "黄泉酒馆索引简介", Content: "据点完整正文"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(LoreItemInput{ID: "secret", Type: "rule", Name: "隐藏规则", Importance: "minor", LoadMode: LoreLoadModeManual, BriefDescription: "隐藏规则索引简介", Content: strings.Repeat("隐藏完整正文", 800)}); err != nil {
		t.Fatal(err)
	}

	context, err := store.StoryMemoryContextMarkdown(20 * 1024)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(context, "主角完整正文") || !strings.Contains(context, "据点完整正文") || !strings.Contains(context, "隐藏完整正文") {
		t.Fatalf("story memory context should include full lore while within budget: %s", context)
	}

	bounded, err := store.StoryMemoryContextMarkdown(1200)
	if err != nil {
		t.Fatal(err)
	}
	if len([]byte(bounded)) > 1200 {
		t.Fatalf("bounded context bytes = %d, want <= 1200", len([]byte(bounded)))
	}
	if !strings.Contains(bounded, "secret") || !strings.Contains(bounded, "隐藏规则") {
		t.Fatalf("bounded context should retain an index for omitted lore: %s", bounded)
	}
}

func TestLoreStoreReadAndSearch(t *testing.T) {
	store := NewLoreStore(t.TempDir())
	if _, err := store.Create(LoreItemInput{ID: "base", Type: "location", Name: "黄泉酒馆", Importance: "important", LoadMode: LoreLoadModeAuto, Tags: []string{"据点"}, Keywords: []string{"黄泉"}, Content: "据点正文"}); err != nil {
		t.Fatal(err)
	}
	item, err := store.Read("base")
	if err != nil {
		t.Fatal(err)
	}
	if item.Content != "据点正文" {
		t.Fatalf("read item content mismatch: %#v", item)
	}
	if _, err := store.Read("missing"); err == nil || !strings.Contains(err.Error(), "资料不存在") {
		t.Fatalf("missing item should return chinese error, got %v", err)
	}
	results, err := store.Search("黄泉", "", 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ID != "base" {
		t.Fatalf("search by keyword failed: %#v", results)
	}
	results, err = store.Search("", "location", 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ID != "base" {
		t.Fatalf("search by type failed: %#v", results)
	}
}

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
	if !strings.HasPrefix(item.ID, "林川_") {
		t.Fatalf("generated ID should use lore item name, got %s", item.ID)
	}
	if item.BriefDescription == "" || !strings.Contains(item.BriefDescription, "角色 林川。") || !strings.Contains(item.BriefDescription, "一定要参考本项详情") {
		t.Fatalf("brief description should be generated: %#v", item)
	}
	if _, err := store.Create(LoreItemInput{ID: item.ID, Type: "character", Name: "重复林川"}); err == nil || !strings.Contains(err.Error(), "资料 ID 已存在") {
		t.Fatalf("expected duplicate ID error, got %v", err)
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

func TestLoreStoreReadsLegacyNovaLoreWhenDenovaWasGeneratedEmpty(t *testing.T) {
	workspace := t.TempDir()
	currentLore := filepath.Join(workspace, ".denova", "lore", "items.json")
	legacyLore := filepath.Join(workspace, ".nova", "lore", "items.json")
	if err := os.MkdirAll(filepath.Dir(currentLore), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(currentLore, []byte(`{"version":1,"items":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(legacyLore), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyLore, []byte(`{
  "version": 1,
  "items": [
    {"id":"hero","enabled":true,"type":"character","name":"林川","importance":"major","content":"旧资料库正文"}
  ]
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	store := NewLoreStore(workspace)
	items, err := store.ListAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "hero" {
		t.Fatalf("should read existing legacy lore items: %#v", items)
	}

	if _, err := store.Create(LoreItemInput{ID: "base", Type: "location", Name: "黄泉酒馆", Importance: "important", Content: "据点。"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(legacyLore)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"id": "hero"`) || !strings.Contains(string(data), `"id": "base"`) {
		t.Fatalf("writes should stay with legacy lore file, got:\n%s", data)
	}
}

func TestLoreStoreUpdateRejectsStaleRevision(t *testing.T) {
	store := NewLoreStore(t.TempDir())
	item, err := store.Create(LoreItemInput{Type: "character", Name: "林川", Importance: "major", Content: "旧内容"})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := store.Update(item.ID, LoreItemInput{Type: "character", Name: "林川", Importance: "major", Content: "Agent 内容", BaseRevision: item.UpdatedAt})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(item.ID, LoreItemInput{Type: "character", Name: "林川", Importance: "major", Content: "前端旧内容", BaseRevision: item.UpdatedAt}); !errors.Is(err, ErrLoreRevisionConflict) {
		t.Fatalf("expected lore revision conflict, got %v", err)
	}
	got, err := store.Read(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Content != agent.Content {
		t.Fatalf("stale save should not overwrite Agent content: %#v", got)
	}
}

func TestLoreStoreImageSurvivesTextUpdateAndCanBeCleared(t *testing.T) {
	store := NewLoreStore(t.TempDir())
	item, err := store.Create(LoreItemInput{Type: "character", Name: "林川", Importance: "major", Content: "旧内容"})
	if err != nil {
		t.Fatal(err)
	}
	withImage, err := store.SetImage(item.ID, &LoreItemImage{
		Schema:    "lore_item_image.v1",
		ImagePath: "assets/lore/images/hero/run/image.png",
		MetaPath:  "assets/lore/images/hero/run/meta.json",
		ProfileID: "default",
		Provider:  "openai",
		Model:     "gpt-image-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if withImage.Image == nil || withImage.Image.ImagePath == "" {
		t.Fatalf("image should be attached: %#v", withImage)
	}
	updated, err := store.Update(item.ID, LoreItemInput{Type: "character", Name: "林川", Importance: "major", Content: "新内容"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Image == nil || updated.Image.ImagePath != withImage.Image.ImagePath {
		t.Fatalf("text update should preserve current image: %#v", updated)
	}
	cleared, err := store.SetImage(item.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Image != nil {
		t.Fatalf("image should be cleared: %#v", cleared.Image)
	}
}

func TestLoreStoreApplyOperationsDoesNotCreateSeparateVersions(t *testing.T) {
	workspace := t.TempDir()
	store := NewLoreStore(workspace)
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
	if len(result.Updated) != 1 || len(result.Created) != 1 {
		t.Fatalf("unexpected apply result: %#v", result)
	}
	if !strings.HasPrefix(result.Created[0].ID, "黄泉酒馆_") {
		t.Fatalf("agent-created item should use name-based ID, got %s", result.Created[0].ID)
	}

	items, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("apply operations should update the lore store: %#v", items)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".nova", "lore", "versions")); !os.IsNotExist(err) {
		t.Fatalf("lore store should not create a separate versions directory, err=%v", err)
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
