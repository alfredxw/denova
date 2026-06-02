package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"nova/internal/book"
)

type readLoreItemsInput struct {
	IDs []string `json:"ids" jsonschema:"description=资料库条目 ID 列表"`
}

type searchLoreItemsInput struct {
	Query string `json:"query" jsonschema:"description=要匹配的名称、标签、简介或资料类型，可为空"`
	Type  string `json:"type" jsonschema:"description=可选资料类型：character/world/location/faction/rule/item/other"`
	Limit int    `json:"limit" jsonschema:"description=最多返回条目数，默认 8，最大 20"`
}

func newLoreTools(workspace string) ([]tool.BaseTool, error) {
	workspace = strings.TrimSpace(workspace)
	readTool, err := utils.InferTool("read_lore_items", "按资料库条目 ID 列表批量读取完整资料正文。用于根据资料库索引判断本轮涉及多个自动加载条目后，一次读取相关完整设定。", func(ctx context.Context, input readLoreItemsInput) (string, error) {
		_ = ctx
		if workspace == "" {
			return "", fmt.Errorf("当前 workspace 不可用，无法读取资料库")
		}
		items, err := book.NewLoreStore(workspace).ReadMany(input.IDs)
		if err != nil {
			return "", err
		}
		if len(items) == 0 {
			return "未读取到资料库条目。", nil
		}
		var sb strings.Builder
		fmt.Fprintln(&sb, "# 资料库条目")
		fmt.Fprintln(&sb)
		for _, item := range items {
			fmt.Fprintln(&sb, formatLoreReference(item))
			fmt.Fprintln(&sb)
		}
		return strings.TrimSpace(sb.String()), nil
	})
	if err != nil {
		return nil, err
	}
	searchTool, err := utils.InferTool("search_lore_items", "搜索资料库索引，按名称、标签、简介或类型查找可能相关的资料条目；返回轻量索引，若需要正文再调用 read_lore_items。", func(ctx context.Context, input searchLoreItemsInput) (string, error) {
		_ = ctx
		if workspace == "" {
			return "", fmt.Errorf("当前 workspace 不可用，无法搜索资料库")
		}
		items, err := book.NewLoreStore(workspace).Search(input.Query, input.Type, input.Limit)
		if err != nil {
			return "", err
		}
		if len(items) == 0 {
			return "未找到匹配的资料库条目。", nil
		}
		var sb strings.Builder
		sb.WriteString("# 资料库搜索结果\n\n")
		for _, item := range items {
			fmt.Fprintf(&sb, "- id: %s\n  名称: %s\n  类型: %s\n  重要度: %s\n  加载策略: %s\n", item.ID, item.Name, item.Type, item.Importance, item.LoadMode)
			if len(item.Tags) > 0 {
				fmt.Fprintf(&sb, "  标签: %s\n", strings.Join(item.Tags, "、"))
			}
			if item.BriefDescription != "" {
				fmt.Fprintf(&sb, "  简介: %s\n", item.BriefDescription)
			}
			sb.WriteString("\n")
		}
		return strings.TrimSpace(sb.String()), nil
	})
	if err != nil {
		return nil, err
	}
	return []tool.BaseTool{readTool, searchTool}, nil
}
