package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"denova/internal/book"
)

type readLoreItemsInput struct {
	IDs   []string `json:"ids,omitempty" jsonschema:"description=资料库条目 ID 列表；可与 names 混合使用，结果按 ID 去重"`
	Names []string `json:"names,omitempty" jsonschema:"description=资料库条目唯一名称列表；可与 ids 混合使用，结果按 ID 去重"`
}

type listLoreItemsInput struct {
	Keywords  []string `json:"keywords,omitempty" jsonschema_description:"可选检索词数组，每项独立匹配 ID、名称、别名、标签、简介和正文；最多 8 项，不要把多个关键词拼成一个字符串。"`
	Match     string   `json:"match,omitempty" jsonschema:"enum=any,enum=all" jsonschema_description:"多关键词关系：any 表示命中任意关键词（OR，默认），all 表示命中全部关键词（AND）。"`
	Types     []string `json:"types,omitempty" jsonschema_description:"可选资料类型数组：character/world/location/faction/rule/item/other。"`
	LoadModes []string `json:"load_modes,omitempty" jsonschema_description:"可选加载策略数组：resident/auto/manual；状态结构审查优先使用 resident。"`
	Detail    string   `json:"detail,omitempty" jsonschema:"enum=index,enum=full" jsonschema_description:"返回粒度：index（默认）返回目录/简介；full 在提供筛选条件时直接返回完整正文，避免再调用 read_lore_items。"`
	Limit     int      `json:"limit,omitempty" jsonschema_description:"筛选结果的本页数量，默认 10，最大 50；空筛选目录由 64 KiB 上限自动分页。"`
	Offset    int      `json:"offset,omitempty" jsonschema_description:"分页起点，默认 0；根据返回的下一页 offset 继续读取。"`
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

type loreToolsOptions struct {
	ReadPolicy  *loreReadPolicy
	WritePolicy *loreWritePolicy
}

// loreWritePolicy bounds write operations to prevent the model from generating
// enormous JSON payloads that block the tool pipeline for tens of seconds.
type loreWritePolicy struct {
	MaxItemsPerCall int
	MaxContentBytes int
}

const (
	defaultLoreWriteMaxItems        = 8
	defaultLoreWriteMaxContentBytes = 128 * 1024
)

func defaultLoreWritePolicy() *loreWritePolicy {
	return &loreWritePolicy{
		MaxItemsPerCall: defaultLoreWriteMaxItems,
		MaxContentBytes: defaultLoreWriteMaxContentBytes,
	}
}

// loreReadPolicy bounds task-specific model context and observes only lore
// bodies that were successfully returned to the model.
type loreReadPolicy struct {
	MaxItemsPerCall int
	MaxResultBytes  int
	MaxTotalBytes   int
	OnRead          func([]string)

	mu        sync.Mutex
	usedBytes int
}

const (
	defaultLoreReadMaxItems       = 16
	defaultLoreReadMaxResultBytes = 128 * 1024
	defaultLoreReadMaxTotalBytes  = 256 * 1024
)

func defaultLoreReadPolicy() *loreReadPolicy {
	return &loreReadPolicy{
		MaxItemsPerCall: defaultLoreReadMaxItems,
		MaxResultBytes:  defaultLoreReadMaxResultBytes,
		MaxTotalBytes:   defaultLoreReadMaxTotalBytes,
	}
}

func (p *loreReadPolicy) validateBatch(input readLoreItemsInput) error {
	count := len(input.IDs) + len(input.Names)
	if count == 0 {
		return fmt.Errorf("至少提供一个资料 ID 或唯一名称")
	}
	return p.validateItemCount(count)
}

func (p *loreReadPolicy) validateItemCount(count int) error {
	if p == nil || p.MaxItemsPerCall <= 0 {
		return nil
	}
	if count > p.MaxItemsPerCall {
		return fmt.Errorf("单次最多读取 %d 个资料条目，当前请求 %d 个", p.MaxItemsPerCall, count)
	}
	return nil
}

func (p *loreReadPolicy) accept(output string, items []book.LoreItem) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	resultBytes := len(output)
	if p.MaxResultBytes > 0 && resultBytes > p.MaxResultBytes {
		p.mu.Unlock()
		return fmt.Errorf("资料正文结果超过单次上下文上限: %d > %d bytes；请减少条目或拆分过长资料", resultBytes, p.MaxResultBytes)
	}
	if p.MaxTotalBytes > 0 && p.usedBytes+resultBytes > p.MaxTotalBytes {
		usedBytes := p.usedBytes
		p.mu.Unlock()
		return fmt.Errorf("资料正文累计超过本任务上下文上限: %d + %d > %d bytes", usedBytes, resultBytes, p.MaxTotalBytes)
	}
	p.usedBytes += resultBytes
	p.mu.Unlock()

	if p.OnRead == nil {
		return nil
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
	return nil
}

func (p *loreWritePolicy) validate(input writeLoreItemsInput) error {
	if p == nil {
		return nil
	}
	total := len(input.Items) + len(input.DeleteIDs)
	if p.MaxItemsPerCall > 0 && total > p.MaxItemsPerCall {
		return fmt.Errorf("单次最多写入 %d 个资料条目（含删除），当前 %d 个；请分批提交", p.MaxItemsPerCall, total)
	}
	if p.MaxContentBytes > 0 {
		totalBytes := 0
		for _, item := range input.Items {
			totalBytes += len(item.Content)
		}
		if totalBytes > p.MaxContentBytes {
			return fmt.Errorf("资料正文总大小超过单次写入上限: %d > %d bytes；请减少条目或拆分过长内容", totalBytes, p.MaxContentBytes)
		}
	}
	return nil
}

func newLoreTools(workspace string, allowWrite bool, options ...loreToolsOptions) ([]tool.BaseTool, error) {
	workspace = strings.TrimSpace(workspace)
	var readPolicy *loreReadPolicy
	var writePolicy *loreWritePolicy
	if len(options) > 0 {
		readPolicy = options[0].ReadPolicy
		writePolicy = options[0].WritePolicy
	}
	if readPolicy == nil {
		readPolicy = defaultLoreReadPolicy()
	}
	if writePolicy == nil {
		writePolicy = defaultLoreWritePolicy()
	}
	readTool, err := utils.InferTool("read_lore_items", "按资料库条目 ID 或唯一名称批量读取完整正文，可在一次调用中混合传入 ids 和 names。名称已在上下文目录中出现时可直接读取，无需先调用 list_lore_items。", func(ctx context.Context, input readLoreItemsInput) (string, error) {
		_ = ctx
		if workspace == "" {
			return "", fmt.Errorf("当前 workspace 不可用，无法读取资料库")
		}
		if err := readPolicy.validateBatch(input); err != nil {
			return "", err
		}
		store := book.NewLoreStore(workspace)
		var items []book.LoreItem
		seen := make(map[string]bool)
		if len(input.IDs) > 0 {
			byID, err := store.ReadMany(input.IDs)
			if err != nil {
				return "", err
			}
			for _, item := range byID {
				if !seen[item.ID] {
					seen[item.ID] = true
					items = append(items, item)
				}
			}
		}
		if len(input.Names) > 0 {
			byName, err := store.ReadManyNames(input.Names)
			if err != nil {
				return "", err
			}
			for _, item := range byName {
				if !seen[item.ID] {
					seen[item.ID] = true
					items = append(items, item)
				}
			}
		}
		output := formatLoreItems(items)
		if err := readPolicy.accept(output, items); err != nil {
			return "", err
		}
		return output, nil
	})
	if err != nil {
		return nil, err
	}
	listTool, err := utils.InferTool("list_lore_items", "浏览或检索启用的资料库。空筛选返回最多 64 KiB 的名称目录；筛选时 detail=index 返回简介，detail=full 可在同一次调用中返回完整正文。已知唯一名称时可直接使用 read_lore_items。", func(ctx context.Context, input listLoreItemsInput) (string, error) {
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
			if err := readPolicy.validateItemCount(len(items)); err != nil {
				return "", err
			}
			output := formatLoreItems(items)
			if err := readPolicy.accept(output, items); err != nil {
				return "", err
			}
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
	tools := []tool.BaseTool{listTool, readTool}
	if !allowWrite {
		return tools, nil
	}
	writeTool, err := utils.InferTool("write_lore_items", "批量创建、更新或删除资料库条目。用于同步角色身份、人设、长期关系、能力体系、世界规则、地点、势力和物品等稳定设定；章节定稿后的当前位置、伤势、心理、目标、持有物等当前角色状态应写入 setting/character-states.md，不要默认写入资料库；每个创建或更新的条目都要填写 brief_description，格式为“类型 名称。”开头，后接 3-5 句身份/别名/关键事实/适用场景/触发词说明，并以“上下文出现相关内容时，一定要参考本项详情。”收束，便于简介自动匹配加载；不要写入章节规划或未来剧情。", func(ctx context.Context, input writeLoreItemsInput) (string, error) {
		_ = ctx
		if workspace == "" {
			return "", fmt.Errorf("当前 workspace 不可用，无法写入资料库")
		}
		if writePolicy != nil {
			if err := writePolicy.validate(input); err != nil {
				return "", err
			}
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

func validateListLoreItemsInput(input listLoreItemsInput) error {
	if len(input.Keywords) > 8 {
		return fmt.Errorf("keywords 最多 8 项")
	}
	for _, keyword := range input.Keywords {
		if utf8.RuneCountInString(strings.TrimSpace(keyword)) > 64 {
			return fmt.Errorf("单个 keyword 最多 64 个字符")
		}
	}
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
	if input.Limit < 0 || input.Limit > book.LoreIndexMaxLimit {
		return fmt.Errorf("limit 必须在 1 到 %d 之间；省略时默认 %d", book.LoreIndexMaxLimit, book.LoreIndexDefaultLimit)
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
		fmt.Fprintln(&sb, formatLoreReference(item))
		fmt.Fprintln(&sb)
	}
	return strings.TrimSpace(sb.String())
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
