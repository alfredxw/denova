package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"

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
		BriefDescription: "角色 林川。谨慎的幸存者。上下文出现林川、角色相关内容时，一定要参考本项详情。",
		Content:          "完整正文不应出现在索引里。档案柜线索只存在于正文。",
	}); err != nil {
		t.Fatal(err)
	}

	tools, err := newLoreTools(workspace, true)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]tool.BaseTool{}
	for _, item := range tools {
		info, err := item.Info(context.Background())
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

	listTool, ok := byName["list_lore_items"].(tool.InvokableTool)
	if !ok {
		t.Fatalf("list_lore_items should be invokable: %T", byName["list_lore_items"])
	}
	listInfo, err := byName["list_lore_items"].Info(context.Background())
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
	output, err := listTool.InvokableRun(context.Background(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# 资料名称目录", "source: lore/items.json", "total: 1", "shown: 1", "next_offset: null", "[character/major] 林川"} {
		if !strings.Contains(output, want) {
			t.Fatalf("list_lore_items output missing %q:\n%s", want, output)
		}
	}
	for _, unexpected := range []string{"简介: 角色 林川。", "标签: 主角、火光", "完整正文不应出现在索引里", "档案柜线索只存在于正文"} {
		if strings.Contains(output, unexpected) {
			t.Fatalf("list_lore_items should not include %q:\n%s", unexpected, output)
		}
	}

	queryOutput, err := listTool.InvokableRun(context.Background(), `{"keywords":["无关词","档案柜"],"match":"any","types":["character"],"limit":5}`)
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
	fullOutput, err := listTool.InvokableRun(context.Background(), `{"keywords":["档案柜"],"detail":"full","limit":5}`)
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
		if _, err := listTool.InvokableRun(context.Background(), args); err == nil {
			t.Fatalf("list_lore_items should reject invalid args: %s", args)
		}
	}
	readTool, ok := byName["read_lore_items"].(tool.InvokableTool)
	if !ok {
		t.Fatalf("read_lore_items should be invokable: %T", byName["read_lore_items"])
	}
	readOutput, err := readTool.InvokableRun(context.Background(), `{"names":["林川"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readOutput, "完整正文不应出现在索引里") {
		t.Fatalf("read_lore_items should resolve unique names:\n%s", readOutput)
	}

	// Mixed ids and names in a single call should work and deduplicate.
	mixedOutput, err := readTool.InvokableRun(context.Background(), `{"ids":["hero"],"names":["林川"]}`)
	if err != nil {
		t.Fatalf("read_lore_items should accept mixed ids and names: %v", err)
	}
	if !strings.Contains(mixedOutput, "完整正文不应出现在索引里") {
		t.Fatalf("mixed ids+names should return content:\n%s", mixedOutput)
	}
	if strings.Count(mixedOutput, "ID：hero") != 1 {
		t.Fatalf("mixed ids+names should deduplicate by ID:\n%s", mixedOutput)
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
	var listTool tool.InvokableTool
	for _, candidate := range tools {
		info, err := candidate.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if info.Name == "list_lore_items" {
			listTool, _ = candidate.(tool.InvokableTool)
		}
	}
	if listTool == nil {
		t.Fatal("list_lore_items tool missing")
	}
	output, err := listTool.InvokableRun(context.Background(), `{"load_modes":["resident"],"limit":50}`)
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
	var readTool tool.InvokableTool
	for _, candidate := range tools {
		info, err := candidate.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if info.Name == "read_lore_items" {
			readTool, _ = candidate.(tool.InvokableTool)
		}
	}
	if readTool == nil {
		t.Fatal("read_lore_items tool missing")
	}
	if _, err := readTool.InvokableRun(context.Background(), `{"ids":["rule-a","rule-b","rule-c"]}`); err == nil {
		t.Fatal("state-schema lore reads must reject oversized batches")
	}
	output, err := readTool.InvokableRun(context.Background(), `{"ids":["rule-a","rule-b"]}`)
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
		info, _ := candidate.Info(context.Background())
		if info.Name != "read_lore_items" {
			continue
		}
		if _, err := candidate.(tool.InvokableTool).InvokableRun(context.Background(), `{"ids":["rule-a"]}`); err == nil {
			t.Fatal("read result exceeding the context budget must be rejected")
		}
	}
	if len(reviewed) != 0 {
		t.Fatalf("rejected lore content must not be recorded as model-reviewed: %v", reviewed)
	}
}
