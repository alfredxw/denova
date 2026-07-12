package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"denova/internal/book"
)

type readLoreItemsInput struct {
	IDs   []string `json:"ids,omitempty" jsonschema:"description=资料库条目 ID 列表；优先使用 names 按唯一名称读取"`
	Names []string `json:"names,omitempty" jsonschema:"description=资料库条目唯一名称列表；Director 和创作 Agent 优先使用名称读取"`
}

type listLoreItemsInput struct {
	Query  string `json:"query,omitempty" jsonschema:"description=可选关键词；会搜索 ID、名称、类型、标签、关键词、简介和正文，但只返回索引，不返回正文"`
	Type   string `json:"type,omitempty" jsonschema:"description=可选资料类型过滤：character/world/location/faction/rule/item/other"`
	Limit  int    `json:"limit,omitempty" jsonschema:"description=可选返回上限；仅 query 或 type 过滤时生效，默认 20，最大 50"`
	Offset int    `json:"offset,omitempty" jsonschema:"description=分页起点，默认 0；根据返回的下一页 offset 继续读取，直到审阅全部名称与简介"`
}

type writeLoreItemsInput struct {
	Message   string               `json:"message" jsonschema:"description=本次资料库变更说明，用中文简要概括"`
	Items     []writeLoreItemInput `json:"items" jsonschema:"description=要创建或更新的完整资料条目列表；已有 ID 的条目会更新，没有 ID 或 ID 不存在的条目会创建"`
	DeleteIDs []string             `json:"delete_ids" jsonschema:"description=要删除的资料条目 ID 列表；只有作者明确要求删除时才使用"`
}

type writeLoreItemInput struct {
	ID               string   `json:"id" jsonschema:"description=资料 ID；更新已有条目时必须填写准确 ID，新建时可留空自动生成"`
	Enabled          *bool    `json:"enabled,omitempty" jsonschema:"description=是否启用该资料条目；禁用条目会保留在资料库中，但不会进入资料库索引、读取工具或模型上下文；不确定时留空"`
	Type             string   `json:"type" jsonschema:"description=资料类型：character/world/location/faction/rule/item/other"`
	Name             string   `json:"name" jsonschema:"description=资料名称"`
	Importance       string   `json:"importance" jsonschema:"description=重要度：major/important/minor"`
	Tags             []string `json:"tags" jsonschema:"description=标签列表"`
	BriefDescription string   `json:"brief_description" jsonschema:"description=资料索引简介；必须写成“类型 名称。”开头，后接 3-5 句身份/别名/关键事实/适用场景/触发词说明，并以“上下文出现相关内容时，一定要参考本项详情。”收束，便于 Agent 自动判断何时读取完整正文；若遗漏后端会按正文自动生成"`
	Keywords         []string `json:"keywords" jsonschema:"description=别名、关键词或触发词列表"`
	LoadMode         string   `json:"load_mode" jsonschema:"description=加载策略：resident/auto/manual"`
	Content          string   `json:"content" jsonschema:"description=中文 Markdown 正文，记录长期稳定设定、核心关系、能力体系和需要追踪的设定事实；每章后的当前位置、伤势、心理、目标等当前状态写入 setting/character-states.md，不写入资料库"`
}

func newLoreTools(workspace string, allowWrite bool) ([]tool.BaseTool, error) {
	workspace = strings.TrimSpace(workspace)
	readTool, err := utils.InferTool("read_lore_items", "按资料库条目 ID 列表批量读取完整资料正文。用于根据资料库索引判断本轮涉及多个自动加载条目后，一次读取相关完整设定。", func(ctx context.Context, input readLoreItemsInput) (string, error) {
		_ = ctx
		if workspace == "" {
			return "", fmt.Errorf("当前 workspace 不可用，无法读取资料库")
		}
		store := book.NewLoreStore(workspace)
		var items []book.LoreItem
		var err error
		if len(input.Names) > 0 {
			items, err = store.ReadManyNames(input.Names)
		} else {
			items, err = store.ReadMany(input.IDs)
		}
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
	listTool, err := utils.InferTool("list_lore_items", "分页列出资料库名称与简介目录。默认每页 20 条，最大 50 条；用返回的下一页 offset 继续审阅。可传 query/type 缩小范围，搜索 ID、名称、类型、标签、关键词、简介和正文，但只返回索引和匹配来源；选中条目后用 read_lore_items 的 names 读取正文。", func(ctx context.Context, input listLoreItemsInput) (string, error) {
		_ = ctx
		if workspace == "" {
			return "", fmt.Errorf("当前 workspace 不可用，无法列出资料库")
		}
		index, err := book.NewLoreStore(workspace).LoreIndexMarkdown(book.LoreIndexOptions{
			Query:    input.Query,
			Type:     input.Type,
			Limit:    input.Limit,
			Offset:   input.Offset,
			Paginate: true,
		})
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(index) == "" {
			return "资料库暂无条目。", nil
		}
		return strings.TrimSpace(index), nil
	})
	if err != nil {
		return nil, err
	}
	tools := []tool.BaseTool{listTool, readTool}
	if !allowWrite {
		return tools, nil
	}
	writeTool, err := utils.InferTool("write_lore_items", "批量创建、更新或删除资料库条目。用于同步角色身份、人设、长期关系、能力体系、世界规则、地点、势力和物品等稳定设定；章节定稿后的当前位置、伤势、心理、目标、持有物等当前角色状态应写入 setting/character-states.md，不要默认写入资料库；每个创建或更新的条目都要填写 brief_description，格式为“类型 名称。”开头，后接 3-5 句身份/别名/关键事实/适用场景/触发词说明，并以“上下文出现相关内容时，一定要参考本项详情。”收束，便于简介自动匹配加载；不要写入章节规划或未来剧情。", func(ctx context.Context, input writeLoreItemsInput) (string, error) {
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
			Enabled:          item.Enabled,
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
