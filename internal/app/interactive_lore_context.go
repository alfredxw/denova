package app

import (
	"fmt"
	"strings"

	"denova/internal/book"
	"denova/internal/interactive"
)

const (
	interactiveResolvedLoreContextMaxBytes   = interactive.DirectorContextMaxBytes
	interactiveDirectorLoreRosterMaxBytes    = 64 * 1024
	interactiveTemporaryLoreRecallMaxEntries = 16
)

// interactiveDirectorStableContext keeps complete resident Lore outside the
// per-run instruction budget so the 64 KiB discovery catalog cannot be
// displaced by resident bodies.
type interactiveDirectorStableContext struct {
	Title     string
	Content   string
	MaxBytes  int
	Revision  string
	BodyBytes int
}

func buildInteractiveDirectorStableContext(workspace string) (interactiveDirectorStableContext, error) {
	resident, err := assembleResidentLore(book.NewLoreStore(workspace))
	if err != nil {
		return interactiveDirectorStableContext{}, fmt.Errorf("装配后台导演的完整常驻资料失败: %w", err)
	}
	if err := validateResidentLoreSnapshot(resident, "后台导演", interactiveResidentLoreMessageMaxBytes); err != nil {
		return interactiveDirectorStableContext{}, err
	}
	return interactiveDirectorStableContext{
		Title: fmt.Sprintf(
			"完整常驻资料（source: enabled resident lore bodies; complete=true; body_bytes=%d; max_body_bytes=%d; lore_revision=%s）",
			resident.BodyBytes, book.ResidentLoreSafetyMaxBytes, resident.Revision,
		),
		Content:   resident.Content,
		MaxBytes:  interactiveResidentLoreMessageMaxBytes,
		Revision:  resident.Revision,
		BodyBytes: resident.BodyBytes,
	}, nil
}

func buildInteractiveStoryLoreContext(workspace string, plan interactive.DirectorPlan, userAction string) (string, error) {
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

func buildInteractiveDirectorLoreContext(workspace string, plan interactive.DirectorPlan, turn interactive.TurnEvent) (string, error) {
	store := book.NewLoreStore(workspace)
	startRevision, err := store.Revision()
	if err != nil {
		return "", fmt.Errorf("读取资料库装配前 revision 失败: %w", err)
	}
	items, err := store.List()
	if err != nil {
		return "", fmt.Errorf("读取 Director 资料库失败: %w", err)
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
	workset := strings.TrimSpace(plan.Docs.LoreContext)
	if workset != "" {
		workset = "## 分支资料工作集（source: lore-context.md）\n\n" + workset
	}
	roster, err := store.LoreNameRosterMarkdown(interactiveDirectorLoreRosterMaxBytes, true)
	if err != nil {
		return "", fmt.Errorf("生成资料名称目录失败: %w", err)
	}
	if roster != "" {
		roster = "## 非驻留资料名称目录（source: lore/items.json, revision-bound, max 64 KiB）\n\n" + roster
	}
	currentRevision, err := store.Revision()
	if err != nil {
		return "", fmt.Errorf("读取资料库装配后 revision 失败: %w", err)
	}
	if strings.TrimSpace(startRevision) != strings.TrimSpace(currentRevision) {
		return "", fmt.Errorf("资料库在导演发现上下文装配期间发生变化: before=%s after=%s", strings.TrimSpace(startRevision), strings.TrimSpace(currentRevision))
	}
	temporary := formatTemporaryLoreRecalls(items, turn.ModelContextMessages)
	reviewStatus := "## 资料库审阅状态（source: lore revision）\n\n"
	if strings.TrimSpace(plan.Metadata.LoreRevision) == "" {
		reviewStatus += "这是当前分支首次资料审阅。名称目录已作为有界发现索引提供；选择候选后再按需读取简介或正文。"
	} else if plan.Metadata.LoreRevision != currentRevision {
		reviewStatus += fmt.Sprintf("资料库已变化（上次：%s，当前：%s）。名称目录已刷新，请重新判断新增或修改后的候选资料。", plan.Metadata.LoreRevision, currentRevision)
	} else {
		reviewStatus += "资料库自上次 Director 完成审阅后没有变化；名称目录仍会每轮提供，遇到 replan、场景切换或角色功能空缺时据此扩展候选。"
	}
	return joinLoreContextSections(reviewStatus, roster, workset, activeContext, temporary), nil
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
	for _, item := range items {
		byID[item.ID] = item
	}
	toolNamesByCallID := map[string]string{}
	for _, message := range messages {
		if strings.TrimSpace(message.Role) != "assistant" {
			continue
		}
		for _, call := range message.ToolCalls {
			name := strings.TrimSpace(call.Function.Name)
			if !isLoreBodyReadTool(name) {
				continue
			}
			if id := strings.TrimSpace(call.ID); id != "" {
				toolNamesByCallID[id] = name
			}
		}
	}
	names := []string{}
	seen := map[string]bool{}
	for _, message := range messages {
		if strings.TrimSpace(message.Role) != "tool" {
			continue
		}
		toolName := strings.TrimSpace(message.ToolName)
		if toolName == "" {
			toolName = strings.TrimSpace(message.Name)
		}
		if toolName == "" {
			toolName = toolNamesByCallID[strings.TrimSpace(message.ToolCallID)]
		}
		if !isLoreBodyReadTool(toolName) {
			continue
		}
		for _, id := range successfulLoreResultIDs(message.Content) {
			item, ok := byID[id]
			if !ok || seen[item.Name] || len(names) >= interactiveTemporaryLoreRecallMaxEntries {
				continue
			}
			seen[item.Name] = true
			names = append(names, "- [["+item.Name+"]]：本回合由 Game Agent 临时读取；请判断是否应加入当前、候场或保持临时召回。")
		}
	}
	if len(names) == 0 {
		return ""
	}
	return "## 本回合临时召回资料（source: committed tool calls）\n\n" + strings.Join(names, "\n")
}

func isLoreBodyReadTool(name string) bool {
	switch strings.TrimSpace(name) {
	case "read_lore_items", "list_lore_items":
		return true
	default:
		return false
	}
}

// successfulLoreResultIDs reads only the stable IDs emitted by a successful
// full-body lore tool result. Index responses and tool errors do not use this
// document header, so they cannot create false read receipts.
func successfulLoreResultIDs(content string) []string {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "# 资料库条目") {
		return nil
	}
	result := []string{}
	seen := map[string]bool{}
	inFence := false
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if !strings.HasPrefix(line, "ID：") {
			continue
		}
		id := strings.TrimSpace(strings.TrimPrefix(line, "ID："))
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, id)
	}
	return result
}
