package interactiveapp

import (
	"fmt"
	"strings"

	"denova/internal/book/lore"
	"denova/internal/interactive"
)

const ResolvedLoreContextMaxBytes = interactive.StoryContextMaxBytes

func buildInteractiveStoryLoreContext(workspace string, plan *interactive.BranchPlan, userAction string) (string, error) {
	items, err := lore.NewStore(workspace).List()
	if err != nil {
		return "", fmt.Errorf("read interactive-story lore: %w", err)
	}
	byName := loreItemsByName(items)

	planMarkdown := ""
	if plan != nil {
		planMarkdown = plan.Markdown
	}
	refs := interactive.ParseLoreReferences(planMarkdown)
	selected := make([]lore.Item, 0, len(refs))
	seen := map[string]bool{}
	for _, name := range refs {
		item, ok := byName[strings.ToLower(strings.TrimSpace(name))]
		if !ok || item.LoadMode == lore.LoadModeResident {
			continue
		}
		selected = append(selected, item)
		seen[item.ID] = true
	}
	for _, item := range items {
		if seen[item.ID] || item.LoadMode == lore.LoadModeResident || !loreItemMentionedByName(item, userAction) {
			continue
		}
		selected = append(selected, item)
		seen[item.ID] = true
	}
	selectedContext, err := formatBoundedCompleteLoreSection("Current Branch Lore Working Set (source: branch-plan references and current user action, complete)", selected, ResolvedLoreContextMaxBytes)
	if err != nil {
		return "", err
	}
	return selectedContext, nil
}

func formatBoundedCompleteLoreSection(title string, items []lore.Item, maxBytes int) (string, error) {
	if len(items) == 0 {
		return "", nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "## %s\n\n", title)
	for _, item := range items {
		block := formatInteractiveLoreItem(item)
		if sb.Len()+len([]byte(block))+2 > maxBytes {
			return "", fmt.Errorf("%s exceeds %d bytes in total; the system will not truncate it silently, so shorten lore content, reduce active references, or adjust lore types", title, maxBytes)
		}
		sb.WriteString(block)
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String()), nil
}

func formatInteractiveLoreItem(item lore.Item) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "### [[%s]] (%s)\n", strings.TrimSpace(item.Name), strings.TrimSpace(item.Type))
	if brief := strings.TrimSpace(item.BriefDescription); brief != "" {
		fmt.Fprintf(&sb, "Brief: %s\n", brief)
	}
	if content := strings.TrimSpace(item.Content); content != "" {
		sb.WriteString("\n")
		sb.WriteString(content)
	}
	return strings.TrimSpace(sb.String())
}

func loreItemsByName(items []lore.Item) map[string]lore.Item {
	result := make(map[string]lore.Item, len(items))
	for _, item := range items {
		result[strings.ToLower(strings.TrimSpace(item.Name))] = item
	}
	return result
}

func loreItemMentionedByName(item lore.Item, text string) bool {
	name := strings.TrimSpace(item.Name)
	return name != "" && strings.Contains(strings.ToLower(text), strings.ToLower(name))
}
