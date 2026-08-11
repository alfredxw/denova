// Package compactionapp contains the application boundary for submitting
// Agent-owned context compaction commands.
package compactionapp

import (
	"strings"

	agentrun "denova/internal/agents/run"
)

// ResolveCommandID normalizes a caller-provided idempotency key and validates
// the final value before a structural Agent command is admitted.
func ResolveCommandID(requested, fallback string) (string, error) {
	commandID := strings.TrimSpace(requested)
	if commandID == "" {
		commandID = strings.TrimSpace(fallback)
	}
	if err := agentrun.ValidateCommandID(commandID); err != nil {
		return "", err
	}
	return commandID, nil
}
