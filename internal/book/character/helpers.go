package character

import (
	"denova/internal/book/lore"
	"fmt"
	"strings"
)

func tavernBookEntryTitle(entry tavernBookEntry, index int) string {
	return firstNonEmpty(entry.Comment, strings.Join(entry.Keys, "、"), fmt.Sprintf("条目 %d", index+1))
}

func tavernCardTags(values ...string) []string {
	tags := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		tags = append(tags, value)
	}
	return tags
}

func normalizeCardText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.TrimSpace(text)
}

func characterBookEntryCount(book *tavernCharacterBook) int {
	if book == nil {
		return 0
	}
	return len(book.Entries)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func mergeCharacterCardImportOptions(opts ...ImportOptions) ImportOptions {
	var merged ImportOptions
	for _, opt := range opts {
		if name := strings.TrimSpace(opt.UserCharacterName); name != "" {
			merged.UserCharacterName = name
		}
		if mode := strings.TrimSpace(opt.ClassificationMode); mode != "" {
			merged.ClassificationMode = mode
		}
		if opt.ClassifyLore != nil {
			merged.ClassifyLore = opt.ClassifyLore
		}
	}
	merged.ClassificationMode = lore.NormalizeClassificationMode(merged.ClassificationMode)
	return merged
}

func bytesToKB(value int) int {
	if value <= 0 {
		return 0
	}
	return (value + 1023) / 1024
}

func tavernUserCharacterName(card normalizedTavernCard, name string) string {
	if !card.HasUserPlaceholder {
		return ""
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "玩家角色"
	}
	return name
}

func tavernBookEntryEnabled(entry tavernBookEntry) *bool {
	if entry.Enabled == nil {
		return loreEnabledPtr(true)
	}
	return loreEnabledPtr(*entry.Enabled)
}

func loreEnabledPtr(enabled bool) *bool {
	return &enabled
}

func loreItemsRelPath(workspace string) string {
	return lore.ItemsRelativePath
}
