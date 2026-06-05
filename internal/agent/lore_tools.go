package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"nova/internal/book"
)

type readLoreItemsInput struct {
	IDs []string `json:"ids" jsonschema:"description=资料库条目 ID 列表"`
}

type writeLoreItemsInput struct {
	Message   string               `json:"message" jsonschema:"description=本次资料库变更说明，用中文简要概括"`
	Items     []writeLoreItemInput `json:"items" jsonschema:"description=要创建或更新的完整资料条目列表；已有 ID 的条目会更新，没有 ID 或 ID 不存在的条目会创建"`
	DeleteIDs []string             `json:"delete_ids" jsonschema:"description=要删除的资料条目 ID 列表；只有作者明确要求删除时才使用"`
}

type writeLoreItemInput struct {
	ID               string   `json:"id" jsonschema:"description=资料 ID；更新已有条目时必须填写准确 ID，新建时可留空自动生成"`
	Type             string   `json:"type" jsonschema:"description=资料类型：character/world/location/faction/rule/item/other"`
	Name             string   `json:"name" jsonschema:"description=资料名称"`
	Importance       string   `json:"importance" jsonschema:"description=重要度：major/important/minor"`
	Tags             []string `json:"tags" jsonschema:"description=标签列表"`
	BriefDescription string   `json:"brief_description" jsonschema:"description=一两句中文简介，用于 Agent 判断何时加载完整正文；创建或更新资料时必须填写，若遗漏后端会按正文自动生成"`
	Keywords         []string `json:"keywords" jsonschema:"description=别名、关键词或触发词列表"`
	LoadMode         string   `json:"load_mode" jsonschema:"description=加载策略：resident/auto/manual"`
	Content          string   `json:"content" jsonschema:"description=中文 Markdown 正文，记录长期稳定设定、核心关系、能力体系和需要追踪的设定事实；每章后的当前位置、伤势、心理、目标等当前状态写入 setting/character-states.md，不写入资料库"`
}

type searchLoreItemsInput struct {
	Query string `json:"query" jsonschema:"description=要匹配的名称、标签、简介或资料类型，可为空"`
	Type  string `json:"type" jsonschema:"description=可选资料类型：character/world/location/faction/rule/item/other"`
	Limit int    `json:"limit" jsonschema:"description=最多返回条目数，默认 8，最大 20"`
}

func newLoreTools(workspace string, allowWrite bool) ([]tool.BaseTool, error) {
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
	tools := []tool.BaseTool{readTool, searchTool}
	if !allowWrite {
		return tools, nil
	}
	writeTool, err := utils.InferTool("write_lore_items", "批量创建、更新或删除资料库条目。用于同步角色身份、人设、长期关系、能力体系、世界规则、地点、势力和物品等稳定设定；章节定稿后的当前位置、伤势、心理、目标、持有物等当前角色状态应写入 setting/character-states.md，不要默认写入资料库；每个创建或更新的条目都要填写 brief_description，便于简介自动匹配加载；不要写入章节规划或未来剧情。", func(ctx context.Context, input writeLoreItemsInput) (string, error) {
		_ = ctx
		if workspace == "" {
			return "", fmt.Errorf("当前 workspace 不可用，无法写入资料库")
		}
		store := book.NewLoreStore(workspace)
		ops, err := buildWriteLoreOperations(store, input)
		if err != nil {
			return "", err
		}
		result, err := store.ApplyOperations(input.Message, ops)
		if err != nil {
			return "", err
		}
		return formatWriteLoreItemsResult(result), nil
	})
	if err != nil {
		return nil, err
	}
	return append(tools, writeTool), nil
}

func buildWriteLoreOperations(store *book.LoreStore, input writeLoreItemsInput) ([]book.LoreOperation, error) {
	itemsByID := map[string]book.LoreItem{}
	existing, err := store.List()
	if err != nil {
		return nil, err
	}
	for _, item := range existing {
		itemsByID[item.ID] = item
	}
	ops := make([]book.LoreOperation, 0, len(input.Items)+len(input.DeleteIDs))
	for _, item := range input.Items {
		loreInput := book.LoreItemInput{
			ID:               item.ID,
			Type:             item.Type,
			Name:             item.Name,
			Importance:       item.Importance,
			Tags:             item.Tags,
			BriefDescription: item.BriefDescription,
			Keywords:         item.Keywords,
			LoadMode:         item.LoadMode,
			Content:          item.Content,
		}
		op := "create"
		if strings.TrimSpace(item.ID) != "" {
			if _, ok := itemsByID[strings.TrimSpace(item.ID)]; ok {
				op = "update"
			}
		}
		ops = append(ops, book.LoreOperation{Op: op, ID: item.ID, Item: loreInput})
	}
	for _, id := range input.DeleteIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		ops = append(ops, book.LoreOperation{Op: "delete", ID: id})
	}
	if len(ops) == 0 {
		return nil, fmt.Errorf("没有可写入的资料库条目")
	}
	return ops, nil
}

func formatWriteLoreItemsResult(result book.LoreApplyResult) string {
	changed := []string{}
	if len(result.Created) > 0 {
		changed = append(changed, fmt.Sprintf("新增 %d", len(result.Created)))
	}
	if len(result.Updated) > 0 {
		changed = append(changed, fmt.Sprintf("更新 %d", len(result.Updated)))
	}
	if len(result.DeletedIDs) > 0 {
		changed = append(changed, fmt.Sprintf("删除 %d", len(result.DeletedIDs)))
	}
	message := strings.TrimSpace(result.Message)
	if message == "" {
		message = "资料库已更新"
	}
	if len(changed) > 0 {
		message += "（" + strings.Join(changed, "，") + "）"
	}
	itemIDs := writeLoreChangedItemIDs(result)
	itemIDsJSON, _ := json.Marshal(itemIDs)
	deletedIDsJSON, _ := json.Marshal(result.DeletedIDs)
	lines := []string{message}
	lines = append(lines, "item_ids: "+string(itemIDsJSON))
	lines = append(lines, "deleted_ids: "+string(deletedIDsJSON))
	if result.Version != nil && result.Version.ID != "" {
		lines = append(lines, "version_id: "+result.Version.ID)
	}
	return strings.Join(lines, "\n")
}

func writeLoreChangedItemIDs(result book.LoreApplyResult) []string {
	ids := make([]string, 0, len(result.Created)+len(result.Updated)+len(result.DeletedIDs))
	seen := map[string]bool{}
	for _, item := range result.Created {
		if item.ID != "" && !seen[item.ID] {
			seen[item.ID] = true
			ids = append(ids, item.ID)
		}
	}
	for _, item := range result.Updated {
		if item.ID != "" && !seen[item.ID] {
			seen[item.ID] = true
			ids = append(ids, item.ID)
		}
	}
	for _, id := range result.DeletedIDs {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}
