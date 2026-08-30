package interactive

import (
	interactivestate "denova/internal/interactive/state"
	"fmt"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// TurnResult is the complete hidden result produced by the Game Agent. The
// backend compiles StateUpdates into replayable StateDelta operations.
type TurnResult struct {
	StateUpdates []interactivestate.Update `json:"state_updates"`
	Choices      []string                  `json:"choices"`
	// PlanUpdate is committed as a private branch event, never inside the Turn.
	PlanUpdate *string `json:"-"`
}

func NormalizeTurnResult(result TurnResult) TurnResult {
	result.StateUpdates = interactivestate.NormalizeUpdates(result.StateUpdates)
	result.Choices = normalizeChoiceListLimit(result.Choices, MaxStoryChoiceCount+1)
	if result.PlanUpdate != nil {
		value := normalizeBranchPlanMarkdown(*result.PlanUpdate)
		result.PlanUpdate = &value
	}
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
	if result.PlanUpdate != nil {
		if err := validateBranchPlanMarkdown(*result.PlanUpdate); err != nil {
			return fmt.Errorf("TurnResult plan_update is invalid: %w", err)
		}
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
