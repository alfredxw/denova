package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/book"
)

type readLoreItemsInput struct {
	IDs   []string `json:"ids,omitempty" jsonschema:"description=资料库条目 ID 列表；优先使用 names 按唯一名称读取"`
	Names []string `json:"names,omitempty" jsonschema:"description=资料库条目唯一名称列表；Director 和创作 Agent 优先使用名称读取"`
}

type listLoreItemsInput struct {
	Keywords  []string `json:"keywords,omitempty" jsonschema_description:"可选检索词数组，每项独立匹配 ID、名称、别名、标签、简介和正文；不要把多个关键词拼成一个字符串。"`
	Match     string   `json:"match,omitempty" jsonschema:"enum=any,enum=all" jsonschema_description:"多关键词关系：any 表示命中任意关键词（OR，默认），all 表示命中全部关键词（AND）。"`
	Types     []string `json:"types,omitempty" jsonschema_description:"可选资料类型数组：character/world/location/faction/rule/item/other。"`
	LoadModes []string `json:"load_modes,omitempty" jsonschema_description:"可选加载策略数组：resident/auto/manual；状态结构审查优先使用 resident。"`
	Detail    string   `json:"detail,omitempty" jsonschema:"enum=index,enum=full" jsonschema_description:"返回粒度：index（默认）返回目录/简介；full 在提供筛选条件时直接返回完整正文，避免再调用 read_lore_items。"`
	Limit     int      `json:"limit,omitempty" jsonschema_description:"筛选结果的本页数量，默认 10，可按需提高；空筛选目录由索引字节预算自动分页。"`
	Offset    int      `json:"offset,omitempty" jsonschema_description:"分页起点，默认 0；根据返回的下一页 offset 继续读取。"`
}

type writeLoreItemsInput struct {
	Message   string               `json:"message,omitempty" jsonschema:"description=可选的本次资料库变更说明，用中文简要概括"`
	Items     []writeLoreItemInput `json:"items,omitempty" jsonschema:"description=要创建或局部更新的资料条目列表；创建至少填写 name，更新填写已有 id 和实际变化字段，省略字段会保留原值"`
	DeleteIDs []string             `json:"delete_ids,omitempty" jsonschema:"description=要删除的资料条目 ID 列表；只有作者明确要求删除时才使用"`
}

type writeLoreItemInput struct {
	ID               string   `json:"id,omitempty" jsonschema:"description=资料 ID；更新已有条目时必须填写准确 ID，新建时可留空自动生成"`
	Enabled          *bool    `json:"enabled,omitempty" jsonschema:"description=是否启用该资料条目；禁用条目会保留在资料库中，但不会进入资料库索引、读取工具或模型上下文；不确定时留空"`
	Type             string   `json:"type,omitempty" jsonschema:"description=资料类型：character/world/location/faction/rule/item/other；创建时默认 other，更新时省略会保留原值"`
	Name             string   `json:"name,omitempty" jsonschema:"description=资料名称；创建时必填，更新时省略会保留原值"`
	Importance       string   `json:"importance,omitempty" jsonschema:"description=重要度：major/important/minor；创建时默认 important，更新时省略会保留原值"`
	Tags             []string `json:"tags,omitempty" jsonschema:"description=标签列表；更新时省略会保留原值，传空数组会清空"`
	BriefDescription string   `json:"brief_description,omitempty" jsonschema:"description=资料索引简介；以“类型 名称。”开头，用 3-5 句概括身份、别名、关键事实、适用场景和触发词；创建时省略会按正文自动生成，更新时省略会保留原值"`
	Keywords         []string `json:"keywords,omitempty" jsonschema:"description=别名、关键词或触发词列表；更新时省略会保留原值，传空数组会清空"`
	LoadMode         string   `json:"load_mode,omitempty" jsonschema:"description=加载策略：resident/auto/manual；创建时自动推导，更新时省略会保留原值"`
	Content          string   `json:"content,omitempty" jsonschema:"description=中文 Markdown 正文，记录长期稳定设定、核心关系、能力体系和需要追踪的设定事实；更新时省略会保留原值；每章后的当前位置、伤势、心理、目标等当前状态写入 setting/character-states.md，不写入资料库"`
}

type loreToolsOptions struct {
	ReadPolicy *loreReadPolicy
}

// loreReadPolicy observes only lore bodies successfully returned to the model.
// Output sizing belongs to the shared tool-result projection boundary; Lore
// must not reject an otherwise valid batch using a second, stricter budget.
type loreReadPolicy struct {
	OnRead func([]string)
}

func defaultLoreReadPolicy() *loreReadPolicy {
	return &loreReadPolicy{}
}

func (p *loreReadPolicy) validateBatch(input readLoreItemsInput) error {
	if len(input.IDs) > 0 && len(input.Names) > 0 {
		return fmt.Errorf("ids 和 names 只能选择一种读取方式")
	}
	count := len(input.IDs) + len(input.Names)
	if count == 0 {
		return fmt.Errorf("至少提供一个资料 ID 或唯一名称")
	}
	return nil
}

func (p *loreReadPolicy) observe(items []book.LoreItem) {
	if p == nil || p.OnRead == nil {
		return
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if id := strings.TrimSpace(item.ID); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) > 0 {
		p.OnRead(ids)
	}
}

func newLoreTools(workspace string, allowWrite bool, options ...loreToolsOptions) ([]agent.ToolDefinition, error) {
	workspace = strings.TrimSpace(workspace)
	var readPolicy *loreReadPolicy
	if len(options) > 0 {
		readPolicy = options[0].ReadPolicy
	}
	if readPolicy == nil {
		readPolicy = defaultLoreReadPolicy()
	}
	readTool, err := agent.InferTool("read_lore_items", "按资料库条目 ID 或唯一名称批量读取完整正文。名称已在上下文目录中出现时可直接读取，无需先调用 list_lore_items。", func(ctx context.Context, input readLoreItemsInput) (string, error) {
		_ = ctx
		if workspace == "" {
			return "", fmt.Errorf("当前 workspace 不可用，无法读取资料库")
		}
		if err := readPolicy.validateBatch(input); err != nil {
			return "", err
		}
		store := book.NewLoreStore(workspace)
		var result book.LoreReadResult
		var err error
		missingField := "ids"
		if len(input.Names) > 0 {
			result, err = store.ReadManyNames(input.Names)
			missingField = "names"
		} else {
			result, err = store.ReadMany(input.IDs)
		}
		if err != nil {
			return "", err
		}
		if len(result.Items) == 0 {
			missing, _ := json.Marshal(result.Missing)
			return "", fmt.Errorf("未匹配到任何资料条目；不存在的 %s: %s", missingField, missing)
		}
		output := formatLoreReadResult(result, missingField)
		readPolicy.observe(result.Items)
		return output, nil
	})
	if err != nil {
		return nil, err
	}
	listTool, err := agent.InferTool("list_lore_items", "浏览或检索启用的资料库。空筛选返回最多 256 KiB 的名称目录；筛选时 detail=index 返回简介，detail=full 可在同一次调用中返回完整正文。已知唯一名称时可直接使用 read_lore_items。", func(ctx context.Context, input listLoreItemsInput) (string, error) {
		_ = ctx
		if workspace == "" {
			return "", fmt.Errorf("当前 workspace 不可用，无法列出资料库")
		}
		if err := validateListLoreItemsInput(input); err != nil {
			return "", err
		}
		store := book.NewLoreStore(workspace)
		if !hasLoreListFilters(input) {
			catalog, err := store.LoreNameCatalogMarkdown(book.LoreNameCatalogOptions{
				Offset:   input.Offset,
				MaxBytes: book.LoreIndexDefaultMaxBytes,
			})
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(catalog), nil
		}
		options := book.LoreIndexOptions{
			Keywords:  input.Keywords,
			Match:     input.Match,
			Types:     input.Types,
			LoadModes: input.LoadModes,
			Limit:     input.Limit,
			Offset:    input.Offset,
			Paginate:  true,
		}
		if strings.EqualFold(strings.TrimSpace(input.Detail), "full") {
			items, err := store.QueryLoreItems(options)
			if err != nil {
				return "", err
			}
			output := formatLoreItems(items)
			readPolicy.observe(items)
			return output, nil
		}
		index, err := store.LoreIndexMarkdown(options)
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
	readDescriptor := boundedReadDescriptor(ToolSourceLore, config.AgentToolLoreRead)
	definedReadTool, err := defineTool(readTool, readDescriptor)
	if err != nil {
		return nil, err
	}
	definedListTool, err := defineTool(listTool, readDescriptor)
	if err != nil {
		return nil, err
	}
	tools := []agent.ToolDefinition{definedListTool, definedReadTool}
	if !allowWrite {
		return tools, nil
	}
	writeTool, err := agent.InferTool("write_lore_items", "批量创建、局部更新或删除资料库条目。用于同步角色身份、人设、长期关系、能力体系、世界规则、地点、势力和物品等稳定设定；创建至少填写 name，更新填写已有 id 和实际变化字段，省略字段会保留原值；brief_description 创建时可由后端生成。章节新增或实质性改写后的当前位置、伤势、心理、目标、持有物等当前角色状态应写入 setting/character-states.md，不要默认写入资料库；不要写入章节规划或未来剧情。", func(ctx context.Context, input writeLoreItemsInput) (agent.ToolResult, error) {
		_ = ctx
		if workspace == "" {
			return agent.ToolResult{}, fmt.Errorf("当前 workspace 不可用，无法写入资料库")
		}
		store := book.NewLoreStore(workspace)
		ops, err := buildWriteLoreOperations(store, input)
		if err != nil {
			return agent.ToolResult{}, err
		}
		result, err := store.ApplyOperations(input.Message, ops)
		if err != nil {
			return agent.ToolResult{}, err
		}
		details, err := json.Marshal(map[string]any{
			"schema": "lore.write.v1", "item_ids": writeLoreChangedItemIDs(result),
			"deleted_ids": result.DeletedIDs,
		})
		if err != nil {
			return agent.ToolResult{}, err
		}
		toolResult := agent.TextToolResult(formatWriteLoreItemsResult(result))
		toolResult.Details = details
		return toolResult, nil
	})
	if err != nil {
		return nil, err
	}
	definedWriteTool, err := defineTool(writeTool, workspaceWriteDescriptor(ToolSourceLore, config.AgentToolLoreWrite, agent.ToolRecoveryReconcilable))
	if err != nil {
		return nil, err
	}
	return append(tools, definedWriteTool), nil
}

func validateListLoreItemsInput(input listLoreItemsInput) error {
	match := strings.TrimSpace(input.Match)
	if match != "" && match != book.LoreIndexMatchAny && match != book.LoreIndexMatchAll {
		return fmt.Errorf("match 只能是 any 或 all")
	}
	validTypes := map[string]bool{"character": true, "world": true, "location": true, "faction": true, "rule": true, "item": true, "other": true}
	for _, itemType := range input.Types {
		if !validTypes[strings.TrimSpace(itemType)] {
			return fmt.Errorf("无效资料类型: %s", strings.TrimSpace(itemType))
		}
	}
	validLoadModes := map[string]bool{book.LoreLoadModeResident: true, book.LoreLoadModeAuto: true, book.LoreLoadModeManual: true}
	for _, loadMode := range input.LoadModes {
		if !validLoadModes[strings.TrimSpace(loadMode)] {
			return fmt.Errorf("无效资料加载策略: %s", strings.TrimSpace(loadMode))
		}
	}
	if input.Limit < 0 {
		return fmt.Errorf("limit 不能小于 0；省略时默认 %d", book.LoreIndexDefaultLimit)
	}
	if input.Offset < 0 {
		return fmt.Errorf("offset 不能小于 0")
	}
	detail := strings.ToLower(strings.TrimSpace(input.Detail))
	if detail != "" && detail != "index" && detail != "full" {
		return fmt.Errorf("detail 只能是 index 或 full")
	}
	if detail == "full" && !hasLoreListFilters(input) {
		return fmt.Errorf("detail=full 必须提供 keywords、types 或 load_modes 筛选，禁止无界读取整个资料库正文")
	}
	return nil
}

func hasLoreListFilters(input listLoreItemsInput) bool {
	return len(input.Keywords) > 0 || len(input.Types) > 0 || len(input.LoadModes) > 0
}

func formatLoreItems(items []book.LoreItem) string {
	if len(items) == 0 {
		return "未读取到资料库条目。"
	}
	var sb strings.Builder
	fmt.Fprintln(&sb, "# 资料库条目")
	fmt.Fprintln(&sb)
	for _, item := range items {
		fmt.Fprintln(&sb, book.LoreReferenceMarkdown(item))
		fmt.Fprintln(&sb)
	}
	return strings.TrimSpace(sb.String())
}

func formatLoreReadResult(result book.LoreReadResult, missingField string) string {
	output := formatLoreItems(result.Items)
	if len(result.Missing) == 0 {
		return output
	}
	missing, _ := json.Marshal(result.Missing)
	return output + "\n\n## 未找到的资料\n\n" + missingField + ": " + string(missing)
}

func buildWriteLoreOperations(store *book.LoreStore, input writeLoreItemsInput) ([]book.LoreOperation, error) {
	itemsByID := map[string]book.LoreItem{}
	// Explicit write IDs may refer to disabled entries. Those entries stay out
	// of model read tools, but an author-approved review snapshot can still
	// safely drive an update without accidentally treating the ID as a create.
	existing, err := store.ListAll()
	if err != nil {
		return nil, err
	}
	for _, item := range existing {
		itemsByID[item.ID] = item
	}
	ops := make([]book.LoreOperation, 0, len(input.Items)+len(input.DeleteIDs))
	for _, item := range input.Items {
		item.ID = strings.TrimSpace(item.ID)
		item.Name = strings.TrimSpace(item.Name)
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
		if item.ID != "" {
			if _, ok := itemsByID[item.ID]; ok {
				op = "update"
			}
		}
		if op == "create" && item.Name == "" {
			return nil, fmt.Errorf("创建资料时 name 不能为空")
		}
		if op == "update" && !hasWriteLoreItemChanges(item) {
			return nil, fmt.Errorf("更新资料 %s 时至少提供一个实际变化字段", item.ID)
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

func hasWriteLoreItemChanges(item writeLoreItemInput) bool {
	return item.Enabled != nil || strings.TrimSpace(item.Type) != "" || strings.TrimSpace(item.Name) != "" ||
		strings.TrimSpace(item.Importance) != "" || item.Tags != nil || strings.TrimSpace(item.BriefDescription) != "" ||
		item.Keywords != nil || strings.TrimSpace(item.LoadMode) != "" || strings.TrimSpace(item.Content) != ""
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
