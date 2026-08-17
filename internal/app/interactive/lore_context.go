package interactiveapp

import (
	"fmt"
	"strings"

	"denova/internal/book/lore"
	"denova/internal/interactive"
)

const (
	ResolvedLoreContextMaxBytes              = interactive.DirectorContextMaxBytes
	interactiveDirectorLoreRosterMaxBytes    = 64 * 1024
	interactiveTemporaryLoreRecallMaxEntries = 16
)

// DirectorStableContext keeps complete resident Lore outside the
// per-run instruction budget so the 64 KiB discovery catalog cannot be
// displaced by resident bodies.
type DirectorStableContext struct {
	Title     string
	Content   string
	MaxBytes  int
	Revision  string
	BodyBytes int
}

func buildInteractiveDirectorStableContext(workspace string) (DirectorStableContext, error) {
	resident, err := assembleResidentLore(lore.NewStore(workspace))
	if err != nil {
		return DirectorStableContext{}, fmt.Errorf("assemble complete resident lore for the background Director: %w", err)
	}
	if err := validateResidentLoreSnapshot(resident, "background Director", interactiveResidentLoreMessageMaxBytes); err != nil {
		return DirectorStableContext{}, err
	}
	return DirectorStableContext{
		Title: fmt.Sprintf(
			"Complete Resident Lore (source: enabled resident lore bodies; complete=true; body_bytes=%d; max_body_bytes=%d; lore_revision=%s)",
			resident.BodyBytes, lore.ResidentLoreSafetyMaxBytes, resident.Revision,
		),
		Content:   resident.Content,
		MaxBytes:  interactiveResidentLoreMessageMaxBytes,
		Revision:  resident.Revision,
		BodyBytes: resident.BodyBytes,
	}, nil
}

func buildInteractiveStoryLoreContext(workspace string, plan interactive.DirectorPlan, userAction string) (string, error) {
	items, err := lore.NewStore(workspace).List()
	if err != nil {
		return "", fmt.Errorf("read interactive-story lore: %w", err)
	}
	byName := loreItemsByName(items)

	refs := interactive.ParseDirectorLoreContextReferences(plan.Docs.LoreContext)
	selected := make([]lore.Item, 0, len(refs.Active))
	seen := map[string]bool{}
	for _, name := range refs.Active {
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
	selectedContext, err := formatBoundedCompleteLoreSection("Current Branch Lore Working Set (source: lore-context.md active references, complete)", selected, ResolvedLoreContextMaxBytes)
	if err != nil {
		return "", err
	}
	return selectedContext, nil
}

func buildInteractiveDirectorLoreContext(workspace string, plan interactive.DirectorPlan, turn interactive.TurnEvent) (string, error) {
	store := lore.NewStore(workspace)
	startRevision, err := store.Revision()
	if err != nil {
		return "", fmt.Errorf("read lore revision before assembly: %w", err)
	}
	items, err := store.List()
	if err != nil {
		return "", fmt.Errorf("read Director lore: %w", err)
	}
	byName := loreItemsByName(items)
	refs := interactive.ParseDirectorLoreContextReferences(plan.Docs.LoreContext)
	active := make([]lore.Item, 0, len(refs.Active))
	for _, name := range refs.Active {
		if item, ok := byName[strings.ToLower(strings.TrimSpace(name))]; ok && item.LoadMode != lore.LoadModeResident {
			active = append(active, item)
		}
	}
	activeContext, err := formatBoundedCompleteLoreSection("Current Lore Content (source: lore-context.md active references, complete)", active, interactive.DirectorLoreActiveContextMaxBytes)
	if err != nil {
		return "", err
	}
	workset := strings.TrimSpace(plan.Docs.LoreContext)
	if workset != "" {
		workset = "## Branch Lore Working Set (source: lore-context.md)\n\n" + workset
	}
	roster, err := store.NameRosterMarkdown(interactiveDirectorLoreRosterMaxBytes, true)
	if err != nil {
		return "", fmt.Errorf("generate lore name roster: %w", err)
	}
	if roster != "" {
		roster = fmt.Sprintf("## Non-resident Lore Name Roster (source: %s, revision-bound, max 64 KiB)\n\n%s", lore.ItemsRelativePath, roster)
	}
	currentRevision, err := store.Revision()
	if err != nil {
		return "", fmt.Errorf("read lore revision after assembly: %w", err)
	}
	if strings.TrimSpace(startRevision) != strings.TrimSpace(currentRevision) {
		return "", fmt.Errorf("lore changed while assembling Director discovery context: before=%s after=%s", strings.TrimSpace(startRevision), strings.TrimSpace(currentRevision))
	}
	temporary := formatTemporaryLoreRecalls(items, turn.ModelContextMessages)
	reviewStatus := "## Lore Review Status (source: lore revision)\n\n"
	if strings.TrimSpace(plan.Metadata.LoreRevision) == "" {
		reviewStatus += "This is the first lore review for the current branch. The name roster is provided as a bounded discovery index; read summaries or full content only after selecting candidates."
	} else if plan.Metadata.LoreRevision != currentRevision {
		reviewStatus += fmt.Sprintf("Lore changed (previous: %s, current: %s). The name roster was refreshed; reassess newly added or modified candidates.", plan.Metadata.LoreRevision, currentRevision)
	} else {
		reviewStatus += "Lore has not changed since the Director's previous review. The name roster is still provided every turn; use it to expand candidates during a replan, scene transition, or role gap."
	}
	return joinLoreContextSections(reviewStatus, roster, workset, activeContext, temporary), nil
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

func loreItemsOfType(items []lore.Item, itemType string) []lore.Item {
	result := []lore.Item{}
	for _, item := range items {
		if item.Type == itemType {
			result = append(result, item)
		}
	}
	return result
}

func loreItemMentionedByName(item lore.Item, text string) bool {
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

func formatTemporaryLoreRecalls(items []lore.Item, messages []interactive.ModelContextMessage) string {
	byID := make(map[string]lore.Item, len(items))
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
			names = append(names, "- [["+item.Name+"]]: temporarily read by the Game Agent this turn; decide whether it belongs in 当前, 候场, or remains a temporary recall.")
		}
	}
	if len(names) == 0 {
		return ""
	}
	return "## Temporary Lore Recalls for This Turn (source: committed tool calls)\n\n" + strings.Join(names, "\n")
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
	if !strings.HasPrefix(content, "# Lore Items") {
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
		if !strings.HasPrefix(line, "ID:") {
			continue
		}
		id := strings.TrimSpace(strings.TrimPrefix(line, "ID:"))
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, id)
	}
	return result
}
