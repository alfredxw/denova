package book

import "strings"

func tavernCardContainsUserPlaceholder(card normalizedTavernCard) bool {
	if strings.Contains(tavernCardSearchText(
		card.Name,
		card.Description,
		card.Personality,
		card.Scenario,
		card.FirstMes,
		card.MesExample,
		card.CreatorNotes,
		card.CreatorComment,
		card.SystemPrompt,
		card.PostHistoryInstructions,
		card.Creator,
		card.CharacterVersion,
		strings.Join(card.Tags, "\n"),
		strings.Join(card.AlternateGreetings, "\n"),
	), "{{user}}") {
		return true
	}
	if card.CharacterBook == nil {
		return false
	}
	for _, entry := range card.CharacterBook.Entries {
		if strings.Contains(tavernCardSearchText(
			entry.Comment,
			entry.Content,
			strings.Join(entry.Keys, "\n"),
			strings.Join(entry.SecondaryKeys, "\n"),
		), "{{user}}") {
			return true
		}
	}
	return false
}

func tavernCardSearchText(values ...string) string {
	return strings.ToLower(strings.Join(values, "\n"))
}

func tavernCardCompatibility(card normalizedTavernCard) CharacterCardCompatibilityReport {
	var report CharacterCardCompatibilityReport
	report.ImportedFields = addCompatibilityFields(report.ImportedFields, "name")
	if strings.TrimSpace(card.Description) != "" {
		report.ImportedFields = addCompatibilityFields(report.ImportedFields, "description")
	}
	if strings.TrimSpace(card.Personality) != "" {
		report.ImportedFields = addCompatibilityFields(report.ImportedFields, "personality")
	}
	if strings.TrimSpace(card.Scenario) != "" {
		report.ImportedFields = addCompatibilityFields(report.ImportedFields, "scenario")
	}
	if strings.TrimSpace(card.MesExample) != "" {
		report.ImportedFields = addCompatibilityFields(report.ImportedFields, "mes_example")
	}
	if strings.TrimSpace(card.CreatorNotes) != "" || strings.TrimSpace(card.CreatorComment) != "" {
		report.ImportedFields = addCompatibilityFields(report.ImportedFields, "creator_notes")
	}
	if strings.TrimSpace(card.SystemPrompt) != "" {
		report.ImportedFields = addCompatibilityFields(report.ImportedFields, "system_prompt")
	}
	if strings.TrimSpace(card.PostHistoryInstructions) != "" {
		report.ImportedFields = addCompatibilityFields(report.ImportedFields, "post_history_instructions")
	}
	if len(card.Tags) > 0 {
		report.ImportedFields = addCompatibilityFields(report.ImportedFields, "tags")
	}
	if characterBookEntryCount(card.CharacterBook) > 0 {
		report.ImportedFields = addCompatibilityFields(report.ImportedFields, "character_book")
	}
	if card.IsPNG {
		report.ImportedFields = addCompatibilityFields(report.ImportedFields, "png_cover")
	}
	if card.HasUserPlaceholder {
		report.ImportedFields = addCompatibilityFields(report.ImportedFields, "user_placeholder")
	}
	if strings.TrimSpace(card.FirstMes) != "" {
		report.DowngradedFields = addCompatibilityFields(report.DowngradedFields, "first_mes")
	}
	if len(card.AlternateGreetings) > 0 {
		report.DowngradedFields = addCompatibilityFields(report.DowngradedFields, "alternate_greetings")
	}
	if strings.TrimSpace(card.Creator) != "" {
		report.DowngradedFields = addCompatibilityFields(report.DowngradedFields, "creator")
	}
	if strings.TrimSpace(card.CharacterVersion) != "" {
		report.DowngradedFields = addCompatibilityFields(report.DowngradedFields, "character_version")
	}
	if card.Avatar != "" {
		report.UnsupportedFields = addCompatibilityFields(report.UnsupportedFields, "avatar")
	}
	if card.Talkativeness != nil {
		report.UnsupportedFields = addCompatibilityFields(report.UnsupportedFields, "talkativeness")
	}
	if card.Fav != nil {
		report.UnsupportedFields = addCompatibilityFields(report.UnsupportedFields, "fav")
	}
	if card.CreateDate != nil {
		report.UnsupportedFields = addCompatibilityFields(report.UnsupportedFields, "create_date")
	}
	if len(card.Extensions) > 0 {
		report.UnsupportedFields = addCompatibilityFields(report.UnsupportedFields, "extensions")
	}
	return report
}

func addCompatibilityFields(fields []string, values ...string) []string {
	seen := make(map[string]bool, len(fields)+len(values))
	for _, field := range fields {
		seen[field] = true
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		fields = append(fields, value)
	}
	return fields
}
