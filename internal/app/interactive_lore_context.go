package app

import (
	"encoding/json"
	"fmt"
	"strings"

	"denova/config"
	"denova/internal/book"
	"denova/internal/interactive"
)

const (
	interactiveResolvedLoreContextMaxBytes   = 48 * 1024
	interactiveDirectorLoreCatalogMaxBytes   = 16 * 1024
	interactiveTemporaryLoreRecallMaxEntries = 16
)

func buildInteractiveStoryLoreContext(workspace string, plan interactive.DirectorPlan, userAction string, configs ...*config.Config) (string, error) {
	items, err := book.NewLoreStore(workspace).List()
	if err != nil {
		return "", fmt.Errorf("读取互动故事资料库失败: %w", err)
	}
	byName := loreItemsByName(items)

	refs := interactive.ParseDirectorLoreContextReferences(plan.Docs.LoreContext)
	selected := make([]book.LoreItem, 0, len(refs.Active))
	seen := map[string]bool{}
	for _, name := range refs.Active {
		item, ok := byName[strings.ToLower(strings.TrimSpace(name))]
		if !ok || item.LoadMode == book.LoreLoadModeResident {
			continue
		}
		selected = append(selected, item)
		seen[item.ID] = true
	}
	for _, item := range items {
		if seen[item.ID] || item.LoadMode == book.LoreLoadModeResident || !loreItemMentionedByName(item, userAction) {
			continue
		}
		selected = append(selected, item)
		seen[item.ID] = true
	}
	selectedContext, err := formatBoundedCompleteLoreSection("当前分支资料工作集（source: lore-context.md active references, complete）", selected, interactiveResolvedLoreContextMaxBytes)
	if err != nil {
		return "", err
	}
	return selectedContext, nil
}

func buildInteractiveDirectorLoreContext(workspace string, plan interactive.DirectorPlan, turn interactive.TurnEvent, configs ...*config.Config) (string, error) {
	store := book.NewLoreStore(workspace)
	items, err := store.List()
	if err != nil {
		return "", fmt.Errorf("读取 Director 资料库失败: %w", err)
	}
	currentRevision, err := store.Revision()
	if err != nil {
		return "", fmt.Errorf("读取资料库 revision 失败: %w", err)
	}
	residentItems := loreItemsOfLoadMode(items, book.LoreLoadModeResident)
	residentBodyBytes := 0
	for _, item := range residentItems {
		residentBodyBytes += len([]byte(strings.TrimSpace(item.Content)))
	}
	residentLimit := residentLoreLimitBytes(configs...)
	if residentBodyBytes > residentLimit {
		return "", fmt.Errorf("常驻资料正文合计超过 %d KB；请缩短正文、改为按需资料或提高常驻资料上限", residentLimit/1024)
	}
	residentContext, err := formatBoundedCompleteLoreSection("常驻资料（source: enabled resident lore, complete）", residentItems, residentLimit+len(residentItems)*1024+1024)
	if err != nil {
		return "", err
	}
	byName := loreItemsByName(items)
	refs := interactive.ParseDirectorLoreContextReferences(plan.Docs.LoreContext)
	active := make([]book.LoreItem, 0, len(refs.Active))
	for _, name := range refs.Active {
		if item, ok := byName[strings.ToLower(strings.TrimSpace(name))]; ok && item.LoadMode != book.LoreLoadModeResident {
			active = append(active, item)
		}
	}
	activeContext, err := formatBoundedCompleteLoreSection("当前资料正文（source: lore-context.md active references, complete）", active, interactive.DirectorLoreActiveContextMaxBytes)
	if err != nil {
		return "", err
	}
	catalog, err := store.LoreIndexMarkdown(book.LoreIndexOptions{Limit: 50, Paginate: true, MaxBytes: interactiveDirectorLoreCatalogMaxBytes, ExcludeResident: true})
	if err != nil {
		return "", fmt.Errorf("生成资料库目录失败: %w", err)
	}
	workset := strings.TrimSpace(plan.Docs.LoreContext)
	if workset != "" {
		workset = "## 分支资料工作集（source: lore-context.md）\n\n" + workset
	}
	if catalog != "" {
		catalog = "## 全库名称与简介目录首批（source: lore/items.json, paged）\n\n" + catalog
	}
	temporary := formatTemporaryLoreRecalls(items, turn.ModelContextMessages)
	reviewStatus := "## 资料库审阅状态（source: lore revision）\n\n"
	if strings.TrimSpace(plan.Metadata.LoreRevision) == "" {
		reviewStatus += "这是当前分支首次资料审阅。请从 offset=0 开始分页审阅全部名称与简介，再设计当前和候场引用。"
	} else if plan.Metadata.LoreRevision != currentRevision {
		reviewStatus += fmt.Sprintf("资料库已变化（上次：%s，当前：%s）。请重新分页检查新增或修改后的候选资料。", plan.Metadata.LoreRevision, currentRevision)
	} else {
		reviewStatus += "资料库自上次 Director 完成审阅后没有变化；仅在 replan、场景切换或角色功能空缺时重新扩展候选。"
	}
	return joinLoreContextSections(residentContext, reviewStatus, workset, activeContext, catalog, temporary), nil
}

func residentLoreLimitBytes(configs ...*config.Config) int {
	limitKB := config.DefaultResidentLoreLimitKB
	if len(configs) > 0 && configs[0] != nil && configs[0].ResidentLoreLimitKB > 0 {
		limitKB = configs[0].ResidentLoreLimitKB
	}
	if limitKB > config.MaxResidentLoreLimitKB {
		limitKB = config.MaxResidentLoreLimitKB
	}
	return limitKB * 1024
}

func loreItemsOfLoadMode(items []book.LoreItem, loadMode string) []book.LoreItem {
	result := []book.LoreItem{}
	for _, item := range items {
		if item.LoadMode == loadMode {
			result = append(result, item)
		}
	}
	return result
}

func formatBoundedCompleteLoreSection(title string, items []book.LoreItem, maxBytes int) (string, error) {
	if len(items) == 0 {
		return "", nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "## %s\n\n", title)
	for _, item := range items {
		block := formatInteractiveLoreItem(item)
		if sb.Len()+len([]byte(block))+2 > maxBytes {
			return "", fmt.Errorf("%s合计超过 %d bytes；系统不会静默截断，请缩短资料正文、减少当前引用或调整资料类型", title, maxBytes)
		}
		sb.WriteString(block)
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String()), nil
}

func formatInteractiveLoreItem(item book.LoreItem) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "### [[%s]]（%s）\n", strings.TrimSpace(item.Name), strings.TrimSpace(item.Type))
	if brief := strings.TrimSpace(item.BriefDescription); brief != "" {
		fmt.Fprintf(&sb, "简介：%s\n", brief)
	}
	if content := strings.TrimSpace(item.Content); content != "" {
		sb.WriteString("\n")
		sb.WriteString(content)
	}
	return strings.TrimSpace(sb.String())
}

func loreItemsByName(items []book.LoreItem) map[string]book.LoreItem {
	result := make(map[string]book.LoreItem, len(items))
	for _, item := range items {
		result[strings.ToLower(strings.TrimSpace(item.Name))] = item
	}
	return result
}

func loreItemsOfType(items []book.LoreItem, itemType string) []book.LoreItem {
	result := []book.LoreItem{}
	for _, item := range items {
		if item.Type == itemType {
			result = append(result, item)
		}
	}
	return result
}

func loreItemMentionedByName(item book.LoreItem, text string) bool {
	name := strings.TrimSpace(item.Name)
	return name != "" && strings.Contains(strings.ToLower(text), strings.ToLower(name))
}

func joinLoreContextSections(sections ...string) string {
	nonEmpty := make([]string, 0, len(sections))
	for _, section := range sections {
		if section = strings.TrimSpace(section); section != "" {
			nonEmpty = append(nonEmpty, section)
		}
	}
	return strings.Join(nonEmpty, "\n\n")
}

func formatTemporaryLoreRecalls(items []book.LoreItem, messages []interactive.ModelContextMessage) string {
	byID := make(map[string]book.LoreItem, len(items))
	byName := make(map[string]book.LoreItem, len(items))
	for _, item := range items {
		byID[item.ID] = item
		byName[item.Name] = item
	}
	names := []string{}
	seen := map[string]bool{}
	for _, message := range messages {
		for _, call := range message.ToolCalls {
			if strings.TrimSpace(call.Function.Name) != "read_lore_items" {
				continue
			}
			var args struct {
				IDs   []string `json:"ids"`
				Names []string `json:"names"`
			}
			if json.Unmarshal([]byte(call.Function.Arguments), &args) != nil {
				continue
			}
			for _, id := range args.IDs {
				item, ok := byID[strings.TrimSpace(id)]
				if !ok || seen[item.Name] || len(names) >= interactiveTemporaryLoreRecallMaxEntries {
					continue
				}
				seen[item.Name] = true
				names = append(names, "- [["+item.Name+"]]：本回合由 Game Agent 临时读取；请判断是否应加入当前、候场或保持临时召回。")
			}
			for _, name := range args.Names {
				item, ok := byName[strings.TrimSpace(name)]
				if !ok || seen[item.Name] || len(names) >= interactiveTemporaryLoreRecallMaxEntries {
					continue
				}
				seen[item.Name] = true
				names = append(names, "- [["+item.Name+"]]：本回合由 Game Agent 临时读取；请判断是否应加入当前、候场或保持临时召回。")
			}
		}
	}
	if len(names) == 0 {
		return ""
	}
	return "## 本回合临时召回资料（source: committed tool calls）\n\n" + strings.Join(names, "\n")
}
