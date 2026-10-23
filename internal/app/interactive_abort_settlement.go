package app

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"denova/internal/interactive"
)

// settleAbortedExternalGameInput only settles the canonical input owned by
// the exact model command being stopped. Runtime operation IDs are deliberately
// different from canonical Game cycle IDs, so they must not be compared.
func settleAbortedExternalGameInput(store *interactive.Store, storyID, branchID, commandID string) error {
	commandID = strings.TrimSpace(commandID)
	if commandID == "" {
		return nil
	}
	snapshot, err := store.Snapshot(storyID, branchID)
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(commandID))
	cyclePrefix := "external-game-" + hex.EncodeToString(sum[:16])
	for _, input := range snapshot.PendingPlayerInputs {
		if input.AgentCommandID != commandID || input.AgentCycle != 1 {
			continue
		}
		if input.AgentOperationID != cyclePrefix && !strings.HasPrefix(input.AgentOperationID, cyclePrefix+".replace.") {
			continue
		}
		if err := store.AbandonPendingTurn(storyID, branchID, input.ID); err != nil {
			return err
		}
	}
	return nil
}
