package interactive

import "fmt"

const (
	MinStoryCheckDifficultyShift = -2
	MaxStoryCheckDifficultyShift = 2
	MinStoryCheckRollModifier    = -20
	MaxStoryCheckRollModifier    = 20
)

// StoryCheckSettings tunes fixed-rule checks for one story without mutating
// the reusable Rule System selected by its Game Preset.
type StoryCheckSettings struct {
	DifficultyShift int `json:"difficulty_shift,omitempty"`
	RollModifier    int `json:"roll_modifier,omitempty"`
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
	return settings
}

func validateStoryCheckSettings(settings StoryCheckSettings) error {
	if settings.DifficultyShift < MinStoryCheckDifficultyShift || settings.DifficultyShift > MaxStoryCheckDifficultyShift {
		return fmt.Errorf("story check difficulty shift must be between %d and %d", MinStoryCheckDifficultyShift, MaxStoryCheckDifficultyShift)
	}
	if settings.RollModifier < MinStoryCheckRollModifier || settings.RollModifier > MaxStoryCheckRollModifier {
		return fmt.Errorf("story check roll modifier must be between %d and %d", MinStoryCheckRollModifier, MaxStoryCheckRollModifier)
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
