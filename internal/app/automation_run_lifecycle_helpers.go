package app

import (
	"errors"
	"fmt"
	"strings"

	agents "denova/internal/agents"
)

func cloneAutomationToolMutations(mutations []agents.ToolMutation) []agents.ToolMutation {
	result := append([]agents.ToolMutation(nil), mutations...)
	for index := range result {
		result[index].LoreItemIDs = append([]string(nil), result[index].LoreItemIDs...)
		result[index].DeletedLoreItemIDs = append([]string(nil), result[index].DeletedLoreItemIDs...)
	}
	return result
}

func automationRunOutcomeError(outcome agents.RunOutcome) error {
	if outcome.Error != nil {
		return outcome.Error
	}
	reason := strings.TrimSpace(outcome.Reason)
	if reason == "" {
		reason = fmt.Sprintf("automation Agent did not complete: status=%s", outcome.Status)
	}
	return errors.New(reason)
}
