package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"denova/internal/agentruntime"
)

// automationManualRunID turns the caller-owned HTTP command identity into the
// durable run identity used by the automation store. The task is part of the
// scope so two automation definitions never alias even if a client reuses an
// idempotency key by mistake.
func automationManualRunID(taskID, commandID string) (string, error) {
	taskID = strings.TrimSpace(taskID)
	commandID = strings.TrimSpace(commandID)
	if taskID == "" || commandID == "" {
		return "", ErrAgentCommandIDRequired
	}
	if err := agentruntime.ValidateCommandID(commandID, agentruntime.DefaultInputLimits()); err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("manual\x00%s\x00%s", taskID, commandID)))
	return "run-command-" + hex.EncodeToString(sum[:16]), nil
}

// automationRunAgentCommandID is stable across scheduler, HTTP, process and
// receipt-reconciliation retries because the run ID is durable before the
// Agent Runtime accepts the StartTurn command.
func automationRunAgentCommandID(runID string) string {
	return "automation-run:" + strings.TrimSpace(runID)
}
