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

func buildTavernCardLoreOperations(card normalizedTavernCard, source, userCharacterName string, names *lore.NameAllocator) ([]lore.Operation, tavernImportStats) {
	if names == nil {
		names = lore.NewNameAllocator(nil)
	}
	stats := tavernImportStats{ClassificationMode: lore.ClassificationModeHeuristic, ClassificationCounts: map[string]int{}}
	cardLoreName := names.Claim(card.Name)
	cardContent := renderTavernCardLoreContent(card)
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
				DetailFields:     tavernCardDetailFieldCount(card),
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
				DetailFields:     1,
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
				DetailFields:     1,
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

// tavernCardDetailField is one detail section of the character lore body. Keeping
// the field list in one place keeps rendering and detail counting in sync.
type tavernCardDetailField struct {
	title string
	value string
}

func tavernCardDetailFields(card normalizedTavernCard) []tavernCardDetailField {
	return []tavernCardDetailField{
		{title: "角色描述", value: sanitizeTavernNaturalLanguage(card.Description)},
		{title: "性格", value: sanitizeTavernNaturalLanguage(card.Personality)},
		{title: "场景", value: sanitizeTavernNaturalLanguage(card.Scenario)},
		{title: "对话示例", value: sanitizeTavernNaturalLanguage(card.MesExample)},
		{title: "作者备注", value: sanitizeTavernRuntimeProneField(card.CreatorNotes)},
		{title: "创建者备注", value: sanitizeTavernRuntimeProneField(card.CreatorComment)},
		{title: "系统提示", value: sanitizeTavernRuntimeProneField(card.SystemPrompt)},
		{title: "历史后置提示", value: sanitizeTavernRuntimeProneField(card.PostHistoryInstructions)},
	}
}

// tavernCardDetailFieldCount reports how many of the card's detail fields carry
// content after sanitization. It is the detail measure used by duplicate
// resolution: more populated fields means a richer setting.
func tavernCardDetailFieldCount(card normalizedTavernCard) int {
	count := 0
	for _, field := range tavernCardDetailFields(card) {
		if normalizeCardText(field.value) != "" {
			count++
		}
	}
	return count
}

func renderTavernCardLoreContent(card normalizedTavernCard) string {
	var sb strings.Builder
	for _, field := range tavernCardDetailFields(card) {
		writeMarkdownSection(&sb, field.title, field.value)
	}
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
