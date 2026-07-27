package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/book"
)

func TestNewLoreToolsUsesListLoreItemsInsteadOfSearch(t *testing.T) {
	workspace := t.TempDir()
	store := book.NewLoreStore(workspace)
	if _, err := store.Create(book.LoreItemInput{
		ID:               "hero",
		Type:             "character",
		Name:             "林川",
		Importance:       "major",
		Tags:             []string{"主角", "火光"},
		BriefDescription: "角色 林川。谨慎的幸存者。",
		Content:          "完整正文不应出现在索引里。档案柜线索只存在于正文。",
	}); err != nil {
		t.Fatal(err)
	}

	tools, err := newLoreTools(workspace, true)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]agent.ToolDefinition{}
	for _, item := range tools {
		info, err := item.Tool.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		byName[info.Name] = item
	}
	if _, ok := byName["search_lore_items"]; ok {
		t.Fatal("search_lore_items should not be registered")
	}
	for _, name := range []string{"list_lore_items", "read_lore_items", "write_lore_items"} {
		if _, ok := byName[name]; !ok {
			t.Fatalf("expected tool %s to be registered", name)
		}
	}

	listTool, ok := byName["list_lore_items"]
	if !ok {
		t.Fatal("list_lore_items should be defined")
	}
	listInfo, err := listTool.Tool.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	schemaJSON, err := json.Marshal(listInfo)
	if err != nil {
		t.Fatal(err)
	}
	schemaText := string(schemaJSON)
	for _, want := range []string{"keywords", "match", "types", "detail", "limit", "offset"} {
		if !strings.Contains(schemaText, want) {
			t.Fatalf("list_lore_items schema missing %q: %s", want, schemaText)
		}
	}
	for _, removed := range []string{`\"query\"`, `\"type\"`} {
		if strings.Contains(schemaText, removed) {
			t.Fatalf("list_lore_items schema should remove legacy field %s: %s", removed, schemaText)
		}
	}
	output, err := runToolForTest(context.Background(), listTool, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# 资料名称目录", "source: setting/lore/items.json", "total: 1", "shown: 1", "next_offset: null", "[character/major] 林川"} {
		if !strings.Contains(output, want) {
			t.Fatalf("list_lore_items output missing %q:\n%s", want, output)
		}
	}
	for _, unexpected := range []string{"简介: 角色 林川。", "标签: 主角、火光", "完整正文不应出现在索引里", "档案柜线索只存在于正文"} {
		if strings.Contains(output, unexpected) {
			t.Fatalf("list_lore_items should not include %q:\n%s", unexpected, output)
		}
	}

	queryOutput, err := runToolForTest(context.Background(), listTool, `{"keywords":["无关词","档案柜"],"match":"any","types":["character"],"limit":5}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"id: hero", "名称: 林川", "匹配词: 档案柜", "匹配来源: 正文"} {
		if !strings.Contains(queryOutput, want) {
			t.Fatalf("keyword list_lore_items output missing %q:\n%s", want, queryOutput)
		}
	}
	if strings.Contains(queryOutput, "档案柜线索只存在于正文") {
		t.Fatalf("keyword list_lore_items should not include full content:\n%s", queryOutput)
	}
	fullOutput, err := runToolForTest(context.Background(), listTool, `{"keywords":["档案柜"],"detail":"full","limit":5}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fullOutput, "档案柜线索只存在于正文") {
		t.Fatalf("detail=full should return complete bodies in one call:\n%s", fullOutput)
	}
	for _, args := range []string{
		`{"match":"some"}`,
		`{"types":["unknown"]}`,
		`{"detail":"unknown"}`,
		`{"detail":"full"}`,
		`{"limit":51}`,
		`{"offset":-1}`,
		`{"keywords":["1","2","3","4","5","6","7","8","9"]}`,
	} {
		if _, err := runToolForTest(context.Background(), listTool, args); err == nil {
			t.Fatalf("list_lore_items should reject invalid args: %s", args)
		}
	}
	readTool, ok := byName["read_lore_items"]
	if !ok {
		t.Fatal("read_lore_items should be defined")
	}
	readOutput, err := runToolForTest(context.Background(), readTool, `{"names":["林川"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readOutput, "完整正文不应出现在索引里") {
		t.Fatalf("read_lore_items should resolve unique names:\n%s", readOutput)
	}
}

func TestWriteLoreItemsToolSupportsSparseCreateUpdateAndDelete(t *testing.T) {
	workspace := t.TempDir()
	definitions, err := newLoreTools(workspace, true)
	if err != nil {
		t.Fatal(err)
	}
	var writeTool *agent.ToolDefinition
	for _, definition := range definitions {
		info, err := definition.Tool.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if info.Name == "write_lore_items" {
			selected := definition
			writeTool = &selected
			break
		}
	}
	if writeTool == nil {
		t.Fatal("write_lore_items tool missing")
	}

	info, err := writeTool.Tool.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	schema, err := info.ToJSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	if len(schema.Required) != 0 {
		t.Fatalf("write_lore_items root fields should be individually optional: %v", schema.Required)
	}
	itemsSchema, ok := schema.Properties.Get("items")
	if !ok || itemsSchema == nil || itemsSchema.Items == nil {
		t.Fatalf("write_lore_items items schema missing: %#v", itemsSchema)
	}
	if len(itemsSchema.Items.Required) != 0 {
		t.Fatalf("sparse lore item fields should be optional in schema: %v", itemsSchema.Items.Required)
	}

	if _, err := runToolForTest(context.Background(), writeTool, `{"items":[{"name":"黄泉酒馆","content":"最初的据点。","tags":["据点"]}]}`); err != nil {
		t.Fatalf("sparse create failed: %v", err)
	}
	store := book.NewLoreStore(workspace)
	created, err := store.Get("黄泉酒馆")
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "黄泉酒馆" || created.Type != "other" || created.Importance != "important" || created.BriefDescription == "" {
		t.Fatalf("sparse create defaults mismatch: %#v", created)
	}

	if _, err := runToolForTest(context.Background(), writeTool, `{"items":[{"id":"黄泉酒馆","content":"更新后的据点。"}]}`); err != nil {
		t.Fatalf("sparse update failed: %v", err)
	}
	updated, err := store.Get("黄泉酒馆")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Content != "更新后的据点。" || updated.Name != created.Name || updated.Type != created.Type || len(updated.Tags) != 1 || updated.Tags[0] != "据点" {
		t.Fatalf("sparse update did not preserve omitted fields: %#v", updated)
	}
	if _, err := runToolForTest(context.Background(), writeTool, `{"items":[{"id":"黄泉酒馆","tags":[]}]}`); err != nil {
		t.Fatalf("explicit empty tags update failed: %v", err)
	}
	cleared, err := store.Get("黄泉酒馆")
	if err != nil {
		t.Fatal(err)
	}
	if len(cleared.Tags) != 0 {
		t.Fatalf("explicit empty tags should clear the list: %#v", cleared.Tags)
	}

	if _, err := runToolForTest(context.Background(), writeTool, `{"items":[{"id":"黄泉酒馆"}]}`); err == nil || !strings.Contains(err.Error(), "至少提供一个实际变化字段") {
		t.Fatalf("id-only update should fail clearly, got %v", err)
	}
	if _, err := runToolForTest(context.Background(), writeTool, `{"delete_ids":["黄泉酒馆"]}`); err != nil {
		t.Fatalf("delete-only call failed: %v", err)
	}
	if _, err := store.Get("黄泉酒馆"); err == nil {
		t.Fatal("delete-only call did not remove the lore item")
	}
	if _, err := runToolForTest(context.Background(), writeTool, `{}`); err == nil || !strings.Contains(err.Error(), "没有可写入的资料库条目") {
		t.Fatalf("empty mutation should fail clearly, got %v", err)
	}
	if _, err := runToolForTest(context.Background(), writeTool, `{"items":[{"content":"缺少名称"}]}`); err == nil || !strings.Contains(err.Error(), "name 不能为空") {
		t.Fatalf("create without name should fail clearly, got %v", err)
	}
}

func TestListLoreItemsFiltersByResidentLoadMode(t *testing.T) {
	workspace := t.TempDir()
	store := book.NewLoreStore(workspace)
	for _, input := range []book.LoreItemInput{
		{ID: "resident-rule", Type: "rule", Name: "常驻数值规则", Importance: "major", LoadMode: book.LoreLoadModeResident, BriefDescription: "定义数值状态。", Content: "生命为 0-100。"},
		{ID: "auto-place", Type: "location", Name: "按需地点", Importance: "major", LoadMode: book.LoreLoadModeAuto, BriefDescription: "进入地点时读取。", Content: "地点正文。"},
	} {
		if _, err := store.Create(input); err != nil {
			t.Fatal(err)
		}
	}
	tools, err := newLoreTools(workspace, false)
	if err != nil {
		t.Fatal(err)
	}
	var listTool *agent.ToolDefinition
	for _, candidate := range tools {
		info, err := candidate.Tool.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if info.Name == "list_lore_items" {
			selected := candidate
			listTool = &selected
		}
	}
	if listTool == nil {
		t.Fatal("list_lore_items tool missing")
	}
	output, err := runToolForTest(context.Background(), listTool, `{"load_modes":["resident"],"limit":50}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "常驻数值规则") || strings.Contains(output, "按需地点") {
		t.Fatalf("resident load-mode filter mismatch:\n%s", output)
	}
}

func TestLoreReadPolicyTracksVisibleItemsAndEnforcesHardBounds(t *testing.T) {
	workspace := t.TempDir()
	store := book.NewLoreStore(workspace)
	for _, input := range []book.LoreItemInput{
		{ID: "rule-a", Type: "rule", Name: "规则甲", LoadMode: book.LoreLoadModeResident, Content: "甲规则正文。"},
		{ID: "rule-b", Type: "rule", Name: "规则乙", LoadMode: book.LoreLoadModeResident, Content: "乙规则正文。"},
		{ID: "rule-c", Type: "rule", Name: "规则丙", LoadMode: book.LoreLoadModeResident, Content: "丙规则正文。"},
	} {
		if _, err := store.Create(input); err != nil {
			t.Fatal(err)
		}
	}
	var reviewed []string
	tools, err := newLoreTools(workspace, false, loreToolsOptions{ReadPolicy: &loreReadPolicy{
		MaxItemsPerCall: 2,
		MaxResultBytes:  4 * 1024,
		MaxTotalBytes:   8 * 1024,
		OnRead: func(ids []string) {
			reviewed = append(reviewed, ids...)
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var readTool *agent.ToolDefinition
	for _, candidate := range tools {
		info, err := candidate.Tool.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if info.Name == "read_lore_items" {
			selected := candidate
			readTool = &selected
		}
	}
	if readTool == nil {
		t.Fatal("read_lore_items tool missing")
	}
	if _, err := runToolForTest(context.Background(), readTool, `{"ids":["rule-a","rule-b","rule-c"]}`); err == nil {
		t.Fatal("state-schema lore reads must reject oversized batches")
	}
	output, err := runToolForTest(context.Background(), readTool, `{"ids":["rule-a","rule-b"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "甲规则正文") || !strings.Contains(output, "乙规则正文") || strings.Join(reviewed, ",") != "rule-a,rule-b" {
		t.Fatalf("visible lore reads should be tracked by returned IDs: reviewed=%v output=%s", reviewed, output)
	}

	reviewed = nil
	boundedTools, err := newLoreTools(workspace, false, loreToolsOptions{ReadPolicy: &loreReadPolicy{
		MaxItemsPerCall: 1,
		MaxResultBytes:  1,
		MaxTotalBytes:   1,
		OnRead: func(ids []string) {
			reviewed = append(reviewed, ids...)
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range boundedTools {
		info, _ := candidate.Tool.Info(context.Background())
		if info.Name != "read_lore_items" {
			continue
		}
		if _, err := runToolForTest(context.Background(), candidate, `{"ids":["rule-a"]}`); err == nil {
			t.Fatal("read result exceeding the context budget must be rejected")
		}
	}
	if len(reviewed) != 0 {
		t.Fatalf("rejected lore content must not be recorded as model-reviewed: %v", reviewed)
	}
}

func TestBuildWriteLoreOperationsUpdatesDisabledKnownID(t *testing.T) {
	workspace := t.TempDir()
	store := book.NewLoreStore(workspace)
	disabled := false
	item, err := store.Create(book.LoreItemInput{
		ID: "archived-hero", Enabled: &disabled, Type: "character", Name: "封存角色", Content: "旧正文",
	})
	if err != nil {
		t.Fatal(err)
	}

	ops, err := buildWriteLoreOperations(store, writeLoreItemsInput{
		Message: "落实作者对停用设定的评论",
		Items:   []writeLoreItemInput{{ID: item.ID, Content: "新正文"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].Op != "update" {
		t.Fatalf("disabled item operation = %#v, want one update", ops)
	}
	if _, err := store.ApplyOperations("落实评论", ops); err != nil {
		t.Fatal(err)
	}
	updated, err := store.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Enabled || updated.Content != "新正文" {
		t.Fatalf("disabled item was not safely updated: %#v", updated)
	}
	if visible, err := store.List(); err != nil || len(visible) != 0 {
		t.Fatalf("disabled item leaked into normal model-visible Lore: %#v err=%v", visible, err)
	}
}
