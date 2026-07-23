package app

import (
	"errors"
	"fmt"
	"strings"

	"denova/internal/agent"
)

func cloneAutomationToolMutations(mutations []agent.ToolMutation) []agent.ToolMutation {
	result := append([]agent.ToolMutation(nil), mutations...)
	for index := range result {
		result[index].LoreItemIDs = append([]string(nil), result[index].LoreItemIDs...)
		result[index].DeletedLoreItemIDs = append([]string(nil), result[index].DeletedLoreItemIDs...)
	}
	return result
}

func automationRunOutcomeError(outcome agent.RunOutcome) error {
	if outcome.Error != nil {
		return outcome.Error
	}
	reason := strings.TrimSpace(outcome.Reason)
	if reason == "" {
		reason = fmt.Sprintf("automation Agent did not complete: status=%s", outcome.Status)
	}
	return errors.New(reason)
}
