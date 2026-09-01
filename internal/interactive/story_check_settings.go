package interactive

import (
	"fmt"
	"strings"
)

const (
	MinStoryCheckDifficultyShift = -2
	MaxStoryCheckDifficultyShift = 2
	MinStoryCheckRollModifier    = -20
	MaxStoryCheckRollModifier    = 20
)

// StoryCheckSettings tunes fixed-rule checks and their story-specific handling
// without mutating the reusable Rule System.
type StoryCheckSettings struct {
	DifficultyShift          int    `json:"difficulty_shift,omitempty"`
	RollModifier             int    `json:"roll_modifier,omitempty"`
	RuleStateConsumptionMode string `json:"rule_state_consumption_mode,omitempty"`
	RuleVisibilityMode       string `json:"rule_visibility_mode,omitempty"`
}

func normalizeStoryCheckSettings(settings StoryCheckSettings) StoryCheckSettings {
	if settings.DifficultyShift < MinStoryCheckDifficultyShift {
		settings.DifficultyShift = MinStoryCheckDifficultyShift
	}
	if settings.DifficultyShift > MaxStoryCheckDifficultyShift {
		settings.DifficultyShift = MaxStoryCheckDifficultyShift
	}
	if settings.RollModifier < MinStoryCheckRollModifier {
		settings.RollModifier = MinStoryCheckRollModifier
	}
	if settings.RollModifier > MaxStoryCheckRollModifier {
		settings.RollModifier = MaxStoryCheckRollModifier
	}
	settings.RuleStateConsumptionMode = normalizeRuleStateConsumptionMode(settings.RuleStateConsumptionMode)
	settings.RuleVisibilityMode = normalizeRuleVisibilityMode(settings.RuleVisibilityMode)
	return settings
}

func validateStoryCheckSettings(settings StoryCheckSettings) error {
	if settings.DifficultyShift < MinStoryCheckDifficultyShift || settings.DifficultyShift > MaxStoryCheckDifficultyShift {
		return fmt.Errorf("story check difficulty shift must be between %d and %d", MinStoryCheckDifficultyShift, MaxStoryCheckDifficultyShift)
	}
	if settings.RollModifier < MinStoryCheckRollModifier || settings.RollModifier > MaxStoryCheckRollModifier {
		return fmt.Errorf("story check roll modifier must be between %d and %d", MinStoryCheckRollModifier, MaxStoryCheckRollModifier)
	}
	if normalized := normalizeRuleStateConsumptionMode(settings.RuleStateConsumptionMode); strings.TrimSpace(settings.RuleStateConsumptionMode) != "" && normalized != strings.TrimSpace(settings.RuleStateConsumptionMode) {
		return fmt.Errorf("unsupported rule state consumption mode: %q", settings.RuleStateConsumptionMode)
	}
	if normalized := normalizeRuleVisibilityMode(settings.RuleVisibilityMode); strings.TrimSpace(settings.RuleVisibilityMode) != "" && normalized != strings.TrimSpace(settings.RuleVisibilityMode) {
		return fmt.Errorf("unsupported rule visibility mode: %q", settings.RuleVisibilityMode)
	}
	return nil
}

func storyCheckDifficulty(value string, shift int) string {
	difficulties := [...]string{"very_easy", "easy", "normal", "hard", "very_hard"}
	normalized := normalizeTurnCheckDifficulty(value)
	index := 2
	for candidateIndex, candidate := range difficulties {
		if candidate == normalized {
			index = candidateIndex
			break
		}
	}
	index += normalizeStoryCheckSettings(StoryCheckSettings{DifficultyShift: shift}).DifficultyShift
	if index < 0 {
		index = 0
	}
	if index >= len(difficulties) {
		index = len(difficulties) - 1
	}
	return difficulties[index]
}
