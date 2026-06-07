package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"

	"nova/internal/book"
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
		Content:          "完整正文不应出现在索引里。",
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
	output, err := listTool.InvokableRun(context.Background(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# 资料库索引", "id: hero", "名称: 林川", "类型: character", "标签: 主角、火光", "简介: 角色 林川。"} {
		if !strings.Contains(output, want) {
			t.Fatalf("list_lore_items output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "完整正文不应出现在索引里") {
		t.Fatalf("list_lore_items should not include full content:\n%s", output)
	}
}
