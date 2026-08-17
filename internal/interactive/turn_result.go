package interactive

import (
	interactivestate "denova/internal/interactive/state"
	"fmt"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const maxDirectorUpdateReasonBytes = 1024

// DirectorUpdateHint is a lightweight post-narrative signal from the Game
// Agent. It only reports that committed facts materially affect future
// planning; the Director remains responsible for deciding patch versus replan
// and which Markdown documents actually need edits.
type DirectorUpdateHint struct {
	Needed bool   `json:"needed" jsonschema_description:"Set true only when this turn materially changes the current objective, phase, key relationship, major clue, or planning premise. Routine continuation must use false."`
	Reason string `json:"reason,omitempty" jsonschema_description:"When needed=true, briefly identify which committed facts affect future planning. Do not propose a specific director.md rewrite."`
}

// TurnResult is the complete hidden result produced by the Game Agent. The
// backend compiles StateUpdates into replayable StateDelta operations.
type TurnResult struct {
	StateUpdates   []interactivestate.Update `json:"state_updates"`
	Choices        []string                  `json:"choices"`
	DirectorUpdate *DirectorUpdateHint       `json:"director_update,omitempty"`
}

func NormalizeTurnResult(result TurnResult) TurnResult {
	result.StateUpdates = interactivestate.NormalizeUpdates(result.StateUpdates)
	result.Choices = normalizeChoiceListLimit(result.Choices, MaxStoryChoiceCount+1)
	result.DirectorUpdate = normalizeDirectorUpdateHint(result.DirectorUpdate)
	return result
}

func ValidateTurnResult(result TurnResult, configuredChoiceCount ...int) error {
	choiceCount := DefaultStoryChoiceCount
	if len(configuredChoiceCount) > 0 {
		choiceCount = normalizeStoryChoiceCount(configuredChoiceCount[0])
	}
	return validateTurnResult(result, choiceCount, false)
}

func validateTerminalTurnResult(result TurnResult, configuredChoiceCount int) error {
	return validateTurnResult(result, configuredChoiceCount, true)
}

func validateTurnResult(result TurnResult, configuredChoiceCount int, terminal bool) error {
	choiceCount := normalizeStoryChoiceCount(configuredChoiceCount)
	if err := validateStoryChoiceCount(choiceCount); err != nil {
		return err
	}
	for index, update := range result.StateUpdates {
		if err := interactivestate.ValidateUpdate(update); err != nil {
			return fmt.Errorf("TurnResult state_updates[%d] is invalid: %w", index, err)
		}
	}
	if err := validateDirectorUpdateHint(result.DirectorUpdate); err != nil {
		return fmt.Errorf("TurnResult director_update is invalid: %w", err)
	}
	if terminal {
		if len(result.Choices) != 0 {
			return fmt.Errorf("TurnResult choices must be empty for an explicitly terminal turn")
		}
		return nil
	}
	if len(result.Choices) != choiceCount {
		return fmt.Errorf("TurnResult choices must contain exactly %d distinct action suggestions", choiceCount)
	}
	return nil
}

func normalizeDirectorUpdateHint(hint *DirectorUpdateHint) *DirectorUpdateHint {
	if hint == nil {
		return nil
	}
	normalized := &DirectorUpdateHint{
		Needed: hint.Needed,
		Reason: strings.TrimSpace(trimBytes(hint.Reason, maxDirectorUpdateReasonBytes)),
	}
	if !normalized.Needed {
		return nil
	}
	return normalized
}

func validateDirectorUpdateHint(hint *DirectorUpdateHint) error {
	if hint == nil {
		return nil
	}
	if !hint.Needed {
		return fmt.Errorf("omit director_update when needed=false")
	}
	if strings.TrimSpace(hint.Reason) == "" {
		return fmt.Errorf("reason must be non-empty when needed=true")
	}
	if len([]byte(hint.Reason)) > maxDirectorUpdateReasonBytes {
		return fmt.Errorf("reason exceeds %d bytes", maxDirectorUpdateReasonBytes)
	}
	return nil
}

func normalizeTurnResultPointer(result *TurnResult, configuredChoiceCount int, terminal bool) *TurnResult {
	if result == nil {
		return nil
	}
	normalized := NormalizeTurnResult(*result)
	var err error
	if terminal {
		err = validateTerminalTurnResult(normalized, configuredChoiceCount)
	} else {
		err = ValidateTurnResult(normalized, configuredChoiceCount)
	}
	if err != nil {
		return nil
	}
	return &normalized
}

func normalizedChoiceKey(value string) string {
	return cases.Fold().String(norm.NFKC.String(strings.TrimSpace(value)))
}

// normalizeEnum remains shared by Director decision normalization.
func normalizeEnum(value string, allowed ...string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, candidate := range allowed {
		if value == candidate {
			return value
		}
	}
	if len(allowed) > 0 {
		return allowed[0]
	}
	return ""
}
