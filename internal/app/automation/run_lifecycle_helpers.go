package automationapp

import (
	agentrun "denova/internal/agents/run"
	agenttool "denova/internal/agents/tool"
	"errors"
	"fmt"
	"strings"
)

func cloneAutomationToolMutations(mutations []agenttool.Mutation) []agenttool.Mutation {
	result := append([]agenttool.Mutation(nil), mutations...)
	for index := range result {
		result[index].LoreItemIDs = append([]string(nil), result[index].LoreItemIDs...)
		result[index].DeletedLoreItemIDs = append([]string(nil), result[index].DeletedLoreItemIDs...)
	}
	return result
}

func automationRunOutcomeError(outcome agentrun.Outcome) error {
	if outcome.Error != nil {
		return outcome.Error
	}
	reason := strings.TrimSpace(outcome.Reason)
	if reason == "" {
		reason = fmt.Sprintf("automation Agent did not complete: status=%s", outcome.Status)
	}
	return errors.New(reason)
}
