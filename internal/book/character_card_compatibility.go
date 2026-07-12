package book

import (
	"sort"
	"strings"
)

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
	report := CharacterCardCompatibilityReport{
		Capabilities: []string{"character_lore"},
		Warnings:     append([]string(nil), card.Warnings...),
	}
	if card.CharacterBook != nil {
		for _, entry := range card.CharacterBook.Entries {
			if entry.Selective || entry.Position != nil || entry.InsertionOrder != 0 || len(entry.SecondaryKeys) > 0 ||
				entry.SelectiveLogic != nil || entry.Probability != nil || entry.UseProbability || strings.TrimSpace(entry.Group) != "" ||
				entry.Depth != nil || entry.Role != nil || entry.PreventRecursion || entry.DelayUntilRecursion || entry.Sticky != nil ||
				entry.Cooldown != nil || entry.Vectorized != nil {
				report.IgnoredLoadingRules = true
			}
			sanitized := sanitizeTavernBookEntry(entry)
			if sanitized.Removed || sanitized.MixedCleaned {
				report.SanitizedRuntime = addCompatibilityFields(report.SanitizedRuntime, "worldbook_runtime")
			}
			if sanitized.Removed {
				continue
			}
			if entry.Constant {
				report.Capabilities = addCompatibilityFields(report.Capabilities, "resident_lore")
			} else {
				report.Capabilities = addCompatibilityFields(report.Capabilities, "on_demand_lore")
			}
			if entry.Enabled != nil && !*entry.Enabled {
				report.Capabilities = addCompatibilityFields(report.Capabilities, "disabled_lore")
			}
		}
	}
	if tavernCardOpeningPresetCount(card) > 0 {
		report.Capabilities = addCompatibilityFields(report.Capabilities, "narrative_openings")
	}
	if card.IsPNG {
		report.Capabilities = addCompatibilityFields(report.Capabilities, "cover")
	}
	if card.HasUserPlaceholder {
		report.Capabilities = addCompatibilityFields(report.Capabilities, "player_character")
	}
	for key := range card.Extensions {
		lower := strings.ToLower(key)
		summary := "unknown"
		switch {
		case strings.Contains(lower, "regex"):
			summary = "regex"
		case strings.Contains(lower, "mvu") || strings.Contains(lower, "zod"):
			summary = "mvu"
		case strings.Contains(lower, "helper"):
			summary = "helper"
		case strings.Contains(lower, "xiaobaix"):
			summary = "workshop"
		}
		report.DiscardedExtensions = addCompatibilityFields(report.DiscardedExtensions, summary)
	}
	if len(report.DiscardedExtensions) > 0 {
		sort.Strings(report.DiscardedExtensions)
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
