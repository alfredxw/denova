package character

import (
	"crypto/sha256"
	"denova/internal/book/lore"
	"encoding/json"
	"fmt"
	"strings"
)

type tavernImportStats struct {
	EnabledEntryCount    int
	DisabledEntryCount   int
	ResidentEntryCount   int
	ResidentEntryBytes   int
	ResidentLoreBytes    int
	AutoEntryCount       int
	RemovedRuntimeCount  int
	SanitizedMixedCount  int
	Warnings             []string
	ClassificationMode   string
	ClassificationCounts map[string]int
	UncertainTypeCount   int
	UncertainOpIndexes   []int
}

func buildTavernCardLoreOperations(card normalizedTavernCard, source, coverPath, userCharacterName string, names *lore.NameAllocator) ([]lore.Operation, tavernImportStats) {
	if names == nil {
		names = lore.NewNameAllocator(nil)
	}
	stats := tavernImportStats{ClassificationMode: lore.ClassificationModeHeuristic, ClassificationCounts: map[string]int{}}
	cardLoreName := names.Claim(card.Name)
	cardContent := renderTavernCardLoreContent(card, coverPath)
	cardKeywords := tavernCardTags(card.Tags...)
	ops := []lore.Operation{
		{
			Op: "create",
			Item: lore.ItemInput{
				Enabled:          loreEnabledPtr(true),
				Type:             "character",
				TypeSource:       lore.TypeSourceHeuristic,
				Name:             cardLoreName,
				Importance:       "major",
				Tags:             tavernCardTags(append([]string{"酒馆角色卡", card.Name}, card.Tags...)...),
				BriefDescription: tavernLoreSearchBrief("character", cardLoreName, cardKeywords),
				Keywords:         cardKeywords,
				LoadMode:         lore.LoadModeResident,
				Content:          cardContent,
				Provenance:       tavernLoreProvenance("tavern_character_card", source, "character", card),
			},
		},
	}
	stats.ResidentLoreBytes += len([]byte(cardContent))
	if card.HasUserPlaceholder {
		name := names.Claim(tavernUserCharacterName(card, userCharacterName))
		content := renderTavernUserPlaceholderLoreContent(card, name)
		ops = append(ops, lore.Operation{
			Op: "create",
			Item: lore.ItemInput{
				Enabled:          loreEnabledPtr(true),
				Type:             "character",
				TypeSource:       lore.TypeSourceHeuristic,
				Name:             name,
				Importance:       "major",
				Tags:             tavernCardTags("酒馆角色卡", "{{user}}", "玩家角色"),
				BriefDescription: tavernLoreSearchBrief("character", name, []string{"{{user}}", card.Name}),
				Keywords:         tavernCardTags("{{user}}", card.Name),
				LoadMode:         lore.LoadModeResident,
				Content:          content,
				Provenance:       tavernLoreProvenance("tavern_character_card", source, "user", card),
			},
		})
		stats.ResidentLoreBytes += len([]byte(content))
	}
	if card.CharacterBook == nil {
		return ops, stats
	}
	for i, entry := range card.CharacterBook.Entries {
		if entry.Enabled != nil && !*entry.Enabled {
			stats.DisabledEntryCount++
		} else {
			stats.EnabledEntryCount++
		}
		sanitized := sanitizeTavernBookEntry(entry)
		if sanitized.Removed {
			stats.RemovedRuntimeCount++
			continue
		}
		if sanitized.MixedCleaned {
			stats.SanitizedMixedCount++
		}
		title := names.Claim(tavernBookEntryTitle(entry, i))
		content := sanitized.Content
		loadMode := lore.LoadModeAuto
		if entry.Constant {
			loadMode = lore.LoadModeResident
			stats.ResidentEntryCount++
			if entry.Enabled == nil || *entry.Enabled {
				stats.ResidentLoreBytes += len([]byte(content))
				stats.ResidentEntryBytes += len([]byte(content))
			}
		} else {
			stats.AutoEntryCount++
		}
		keywords := tavernCardTags(append(append([]string{}, entry.Keys...), entry.SecondaryKeys...)...)
		suggestion := lore.ClassifyItemHeuristic(lore.ClassificationInput{
			Name: title, Tags: []string{"酒馆世界书", card.Name}, Keywords: keywords, Content: content,
		})
		itemType := suggestion.Type
		tags := tavernCardTags("酒馆世界书", card.Name)
		ops = append(ops, lore.Operation{
			Op: "create",
			Item: lore.ItemInput{
				Enabled:          tavernBookEntryEnabled(entry),
				Type:             itemType,
				TypeSource:       lore.TypeSourceHeuristic,
				Name:             title,
				Importance:       "important",
				Tags:             tags,
				Keywords:         keywords,
				BriefDescription: tavernLoreSearchBrief(itemType, title, keywords),
				LoadMode:         loadMode,
				Content:          content,
				Provenance:       tavernLoreProvenance("tavern_worldbook_entry", source, tavernEntryRecordID(entry, i), entry),
			},
		})
		stats.ClassificationCounts[itemType]++
		if suggestion.Confidence != lore.ClassificationConfidenceHigh {
			stats.UncertainTypeCount++
			stats.UncertainOpIndexes = append(stats.UncertainOpIndexes, len(ops)-1)
		}
	}
	return ops, stats
}

func tavernLoreSearchBrief(itemType, name string, keywords []string) string {
	subject := fmt.Sprintf("%s「%s」", lore.TypeLabel(itemType), strings.TrimSpace(name))
	keywords = tavernCardTags(keywords...)
	if len(keywords) == 0 {
		return subject + "；无额外搜索关键词，可按名称读取正文。"
	}
	return truncateCardRunes(subject+"；搜索关键词："+strings.Join(keywords, "、")+"。", 240)
}

func renderTavernCardLoreContent(card normalizedTavernCard, coverPath string) string {
	var sb strings.Builder
	if coverPath != "" {
		sb.WriteString("![")
		sb.WriteString(card.Name)
		sb.WriteString("](")
		sb.WriteString(coverPath)
		sb.WriteString(")\n\n")
	}

	writeMarkdownSection(&sb, "角色描述", sanitizeTavernNaturalLanguage(card.Description))
	writeMarkdownSection(&sb, "性格", sanitizeTavernNaturalLanguage(card.Personality))
	writeMarkdownSection(&sb, "场景", sanitizeTavernNaturalLanguage(card.Scenario))
	writeMarkdownSection(&sb, "对话示例", sanitizeTavernNaturalLanguage(card.MesExample))
	writeMarkdownSection(&sb, "作者备注", sanitizeTavernRuntimeProneField(card.CreatorNotes))
	writeMarkdownSection(&sb, "创建者备注", sanitizeTavernRuntimeProneField(card.CreatorComment))
	writeMarkdownSection(&sb, "系统提示", sanitizeTavernRuntimeProneField(card.SystemPrompt))
	writeMarkdownSection(&sb, "历史后置提示", sanitizeTavernRuntimeProneField(card.PostHistoryInstructions))
	return strings.TrimSpace(sb.String())
}

func renderTavernUserPlaceholderLoreContent(card normalizedTavernCard, name string) string {
	var sb strings.Builder
	sb.WriteString(name)
	sb.WriteString(" 是与 ")
	sb.WriteString(card.Name)
	sb.WriteString(" 互动的玩家角色。请补充姓名、身份、关系与需要保持稳定的个人事实。\n")
	return strings.TrimSpace(sb.String())
}

func tavernEntryRecordID(entry tavernBookEntry, index int) string {
	if entry.ID != 0 {
		return fmt.Sprintf("%d", entry.ID)
	}
	return fmt.Sprintf("entry-%d", index+1)
}

func tavernLoreProvenance(kind, source, recordID string, record any) *lore.Provenance {
	data, _ := json.Marshal(record)
	sum := sha256.Sum256(data)
	return &lore.Provenance{Kind: kind, SourceName: source, SourceRecordID: recordID, SourceHash: fmt.Sprintf("%x", sum[:])}
}

func writeMarkdownSection(sb *strings.Builder, title, content string) {
	content = normalizeCardText(content)
	if content == "" {
		return
	}
	sb.WriteString("### ")
	sb.WriteString(title)
	sb.WriteString("\n\n")
	sb.WriteString(content)
	sb.WriteString("\n\n")
}
