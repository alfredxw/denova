package book

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestLoreStoreSerializesConcurrentCreatesAcrossInstances(t *testing.T) {
	workspace := t.TempDir()
	const count = 16
	start := make(chan struct{})
	errs := make(chan error, count)
	var wg sync.WaitGroup
	for index := 0; index < count; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			defer func() {
				if recovered := recover(); recovered != nil {
					errs <- fmt.Errorf("concurrent Create panic: %v", recovered)
				}
			}()
			<-start
			_, err := NewLoreStore(workspace).Create(LoreItemInput{
				ID: fmt.Sprintf("item-%02d", index), Type: "world", Name: fmt.Sprintf("资料-%02d", index), Content: "并发写入",
			})
			errs <- err
		}(index)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Create failed: %v", err)
		}
	}

	items, err := NewLoreStore(workspace).ListAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != count {
		t.Fatalf("concurrent creates lost updates: got=%d want=%d items=%#v", len(items), count, items)
	}
	data, err := os.ReadFile(NewLoreStore(workspace).itemsPath())
	if err != nil {
		t.Fatal(err)
	}
	var persisted LoreCollection
	if err := json.Unmarshal(data, &persisted); err != nil || len(persisted.Items) != count {
		t.Fatalf("persisted lore must remain valid and complete: items=%d err=%v", len(persisted.Items), err)
	}
}

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

func TestResidentContextIsStableAndSortedByLoreID(t *testing.T) {
	store := NewLoreStore(t.TempDir())
	for _, input := range []LoreItemInput{
		{ID: "z-last", Type: "world", Name: "先创建但后排序", LoadMode: LoreLoadModeResident, Content: "Z正文"},
		{ID: "a-first", Type: "location", Name: "后创建但先排序", LoadMode: LoreLoadModeResident, Content: "A正文"},
		{ID: "auto", Type: "world", Name: "按需资料", LoadMode: LoreLoadModeAuto, Content: "不应注入"},
	} {
		if _, err := store.Create(input); err != nil {
			t.Fatal(err)
		}
	}
	first, err := store.ResidentContextMarkdown()
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.ResidentContextMarkdown()
	if err != nil {
		t.Fatal(err)
	}
	if first != second || strings.Index(first, "A正文") > strings.Index(first, "Z正文") || strings.Contains(first, "不应注入") {
		t.Fatalf("常驻上下文必须按 ID 稳定且排除按需资料:\n%s", first)
	}
}

func TestResidentContextDoesNotLogRepeatedSizeWarnings(t *testing.T) {
	store := NewLoreStore(t.TempDir())
	if _, err := store.Create(LoreItemInput{
		ID: "large-resident", Type: "rule", Name: "大型常驻资料",
		LoadMode: LoreLoadModeResident, Content: strings.Repeat("常驻规则", 5000),
	}); err != nil {
		t.Fatal(err)
	}

	var logs bytes.Buffer
	previousWriter := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(previousWriter) })
	for range 3 {
		if _, err := store.ResidentContextMarkdown(); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Contains(logs.String(), "[lore-context]") {
		t.Fatalf("稳定的常驻资料不应在上下文热路径重复打印大小 warning:\n%s", logs.String())
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
	for i := 0; i < 12; i++ {
		if _, err := store.Create(LoreItemInput{ID: fmt.Sprintf("candidate-%02d", i), Type: "character", Name: fmt.Sprintf("候选角色%02d", i), Importance: "minor", LoadMode: LoreLoadModeAuto, Content: "按需正文"}); err != nil {
			t.Fatal(err)
		}
	}

	context, err := store.ProgressiveContextMarkdown()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(context, "## 常驻资料库") || !strings.Contains(context, "主角完整正文") {
		t.Fatalf("resident context missing full content: %s", context)
	}
	if !strings.Contains(context, "## 按需资料名称目录") || !strings.Contains(context, "黄泉酒馆") || !strings.Contains(context, "隐藏规则") || !strings.Contains(context, "候选角色11") {
		t.Fatalf("name catalog context missing non-resident items: %s", context)
	}
	if strings.Contains(context, "id: base") || strings.Contains(context, "黄泉酒馆索引简介") {
		t.Fatalf("name catalog should remain a compact discovery surface: %s", context)
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

func TestLoreStoreCompactIndexMatchesMultipleKeywordsWithAnyAndAll(t *testing.T) {
	store := NewLoreStore(t.TempDir())
	if _, err := store.Create(LoreItemInput{ID: "archive", Type: "location", Name: "旧档案室", Importance: "important", LoadMode: LoreLoadModeAuto, Keywords: []string{"档案柜"}, BriefDescription: "地点 旧档案室。只有尘封档案。上下文出现相关内容时，一定要参考本项详情。", Content: "暗门后藏着完整原文线索。"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(LoreItemInput{ID: "hero", Type: "character", Name: "林川", Importance: "major", LoadMode: LoreLoadModeAuto, Tags: []string{"主角"}, BriefDescription: "角色 林川。故事开场时抵达旧城。", Content: "林川不了解档案柜。"}); err != nil {
		t.Fatal(err)
	}

	anyIndex, err := store.LoreIndexMarkdown(LoreIndexOptions{Keywords: []string{"主角", "完整原文"}, Match: LoreIndexMatchAny})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"archive", "hero"} {
		if !strings.Contains(anyIndex, "id: "+id) {
			t.Fatalf("any keyword match should return %s:\n%s", id, anyIndex)
		}
	}

	allIndex, err := store.LoreIndexMarkdown(LoreIndexOptions{Keywords: []string{"档案柜", "完整原文"}, Match: LoreIndexMatchAll})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(allIndex, "id: archive") || strings.Contains(allIndex, "id: hero") {
		t.Fatalf("all keyword match should require every keyword:\n%s", allIndex)
	}
	if !strings.Contains(allIndex, "匹配词: 档案柜、完整原文") || !strings.Contains(allIndex, "匹配来源: 关键词、正文") {
		t.Fatalf("keyword result should explain matched terms and fields:\n%s", allIndex)
	}
	if strings.Contains(allIndex, "暗门后藏着完整原文线索") {
		t.Fatalf("keyword index should not leak full content:\n%s", allIndex)
	}
}

func TestLoreStoreCompactIndexFuzzyMatchesShortMetadataAndRanksExactFirst(t *testing.T) {
	store := NewLoreStore(t.TempDir())
	for _, input := range []LoreItemInput{
		{ID: "exact", Type: "location", Name: "旧档案时", Importance: "minor", LoadMode: LoreLoadModeAuto, BriefDescription: "名称完全命中。", Content: "正文"},
		{ID: "fuzzy", Type: "location", Name: "旧档案室据点", Importance: "major", LoadMode: LoreLoadModeAuto, BriefDescription: "名称片段有一个错字也可召回。", Content: "正文"},
	} {
		if _, err := store.Create(input); err != nil {
			t.Fatal(err)
		}
	}

	index, err := store.LoreIndexMarkdown(LoreIndexOptions{Keywords: []string{"旧档案时"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Index(index, "id: exact") > strings.Index(index, "id: fuzzy") {
		t.Fatalf("exact name match should rank before fuzzy match:\n%s", index)
	}
	if !strings.Contains(index, "匹配来源: 模糊名称") {
		t.Fatalf("fuzzy match should expose its source:\n%s", index)
	}
}

func TestLoreStoreCompactIndexPaginatesWithDefaultAndExplicitLimits(t *testing.T) {
	store := NewLoreStore(t.TempDir())
	for i := 0; i < 12; i++ {
		id := fmt.Sprintf("item_%02d", i)
		if _, err := store.Create(LoreItemInput{ID: id, Type: "other", Name: "资料" + id, LoadMode: LoreLoadModeAuto, Content: "正文"}); err != nil {
			t.Fatal(err)
		}
	}

	first, err := store.LoreIndexMarkdown(LoreIndexOptions{Paginate: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(first, "- id:") != LoreIndexDefaultLimit || !strings.Contains(first, "下一页使用 offset=10") {
		t.Fatalf("default page should return %d entries and the next offset:\n%s", LoreIndexDefaultLimit, first)
	}
	second, err := store.LoreIndexMarkdown(LoreIndexOptions{Paginate: true, Offset: 10, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(second, "- id:") != 2 || strings.Contains(second, "下一页使用") {
		t.Fatalf("explicit final page should return the remaining two entries:\n%s", second)
	}
}

func TestLoreStoreNameRosterIsBoundedAndOmitsResidentBodies(t *testing.T) {
	store := NewLoreStore(t.TempDir())
	for _, input := range []LoreItemInput{
		{ID: "resident", Type: "rule", Name: "常驻规则", Importance: "major", LoadMode: LoreLoadModeResident, Content: "不应重复进入名称目录"},
		{ID: "hero", Type: "character", Name: "林川", Importance: "major", LoadMode: LoreLoadModeAuto, BriefDescription: "不应注入简介", Content: "不应注入正文"},
		{ID: "base", Type: "location", Name: "黄泉酒馆", Importance: "important", LoadMode: LoreLoadModeAuto, Content: "不应注入正文"},
	} {
		if _, err := store.Create(input); err != nil {
			t.Fatal(err)
		}
	}

	roster, err := store.LoreNameRosterMarkdown(1024, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"资料名称目录", "共 2 条", "[character/major] 林川", "[location/important] 黄泉酒馆"} {
		if !strings.Contains(roster, want) {
			t.Fatalf("name roster missing %q:\n%s", want, roster)
		}
	}
	for _, forbidden := range []string{"常驻规则", "不应注入简介", "不应注入正文"} {
		if strings.Contains(roster, forbidden) {
			t.Fatalf("name roster should omit %q:\n%s", forbidden, roster)
		}
	}

	bounded, err := store.LoreNameRosterMarkdown(128, false)
	if err != nil {
		t.Fatal(err)
	}
	if len([]byte(bounded)) > 128 || !strings.Contains(bounded, "omitted: 3") || strings.Contains(bounded, "常驻规则") {
		t.Fatalf("bounded roster should report omitted names within its byte limit:\n%s", bounded)
	}
}

func TestLoreStoreNameRosterUsesThe64KiBDiscoveryBudget(t *testing.T) {
	store := NewLoreStore(t.TempDir())
	items := make([]LoreItem, 400)
	for index := range items {
		items[index] = LoreItem{
			ID:         fmt.Sprintf("entry-%03d", index),
			Enabled:    true,
			Type:       "character",
			Name:       fmt.Sprintf("候选资料%03d-%s", index, strings.Repeat("长名称", 6)),
			Importance: "minor",
			LoadMode:   LoreLoadModeAuto,
			Content:    "不应出现的正文",
		}
	}
	data, err := json.Marshal(LoreCollection{Version: loreItemsVersion, Items: items})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(store.itemsPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.itemsPath(), data, 0o644); err != nil {
		t.Fatal(err)
	}

	roster, err := store.LoreNameRosterMarkdown(LoreIndexDefaultMaxBytes, true)
	if err != nil {
		t.Fatal(err)
	}
	if len([]byte(roster)) <= 8*1024 || len([]byte(roster)) > 64*1024 {
		t.Fatalf("name roster should use more than the old 8 KiB budget without crossing 64 KiB, got %d bytes", len([]byte(roster)))
	}
	if !strings.Contains(roster, "候选资料399") || !strings.Contains(roster, "omitted: 0") || strings.Contains(roster, "不应出现的正文") {
		t.Fatalf("64 KiB roster should expose late names, report completeness, and omit bodies:\n%s", roster)
	}
}

func TestLoreStoreCompactIndexNoMatchDoesNotClaimLibraryIsEmpty(t *testing.T) {
	store := NewLoreStore(t.TempDir())
	if _, err := store.Create(LoreItemInput{ID: "hero", Type: "character", Name: "林川", LoadMode: LoreLoadModeAuto, Content: "主角设定"}); err != nil {
		t.Fatal(err)
	}

	index, err := store.LoreIndexMarkdown(LoreIndexOptions{Keywords: []string{"不存在的资料"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"资料库共有 1 条启用资料", "本次检索匹配 0 条", "未命中不代表资料库为空"} {
		if !strings.Contains(index, want) {
			t.Fatalf("no-match result missing %q:\n%s", want, index)
		}
	}
	if strings.Contains(index, "资料库暂无") {
		t.Fatalf("no-match result must not claim the library is empty:\n%s", index)
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
	if item.ID != "林川" {
		t.Fatalf("generated ID should be based on the lore item name without random suffix, got %s", item.ID)
	}
	if item.BriefDescription == "" || !strings.Contains(item.BriefDescription, "角色 林川。") || !strings.Contains(item.BriefDescription, "一定要参考本项详情") {
		t.Fatalf("brief description should be generated: %#v", item)
	}
	if _, err := store.Create(LoreItemInput{ID: item.ID, Type: "character", Name: "重复林川"}); err == nil || !strings.Contains(err.Error(), "资料 ID 已存在") {
		t.Fatalf("expected duplicate ID error, got %v", err)
	}
	if _, err := store.Create(LoreItemInput{Type: "character", Name: "林川"}); err == nil || !strings.Contains(err.Error(), "资料名称已存在") {
		t.Fatalf("expected duplicate name error, got %v", err)
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
	if _, err := store.Create(LoreItemInput{Type: "location", Name: "林川", Importance: "important"}); err != nil {
		t.Fatalf("old name should be available after rename: %v", err)
	}
	if _, err := store.Update(updated.ID, LoreItemInput{Type: "location", Name: "林川", Importance: "important"}); err == nil || !strings.Contains(err.Error(), "资料名称已存在") {
		t.Fatalf("expected duplicate name error on update, got %v", err)
	}

	if err := store.Delete(item.ID); err != nil {
		t.Fatal(err)
	}
	items, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "林川" {
		t.Fatalf("only the replacement item should remain after delete: %#v", items)
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
	if result.Created[0].ID != "黄泉酒馆" {
		t.Fatalf("agent-created item should use name-based ID without random suffix, got %s", result.Created[0].ID)
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

func TestLoreStoreApplyOperationsRejectsDuplicateNames(t *testing.T) {
	store := NewLoreStore(t.TempDir())
	if _, err := store.Create(LoreItemInput{ID: "hero", Type: "character", Name: "林川", Importance: "major"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyOperations("创建重名资料", []LoreOperation{
		{Op: "create", Item: LoreItemInput{Type: "character", Name: "林川", Importance: "major"}},
	}); err == nil || !strings.Contains(err.Error(), "资料名称已存在") {
		t.Fatalf("expected duplicate name error on create operation, got %v", err)
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

func TestNewUniqueLoreIDUsesNameWithoutRandomSuffix(t *testing.T) {
	if got := newUniqueLoreID(nil, "黄泉酒馆", "location"); got != "黄泉酒馆" {
		t.Fatalf("generated lore ID should use the normalized name directly, got %s", got)
	}
	items := []LoreItem{{ID: "huang_quan"}}
	if got := newUniqueLoreID(items, "Huang Quan", "location"); got != "huang_quan-2" {
		t.Fatalf("ID collision should use numeric suffix, got %s", got)
	}
}
