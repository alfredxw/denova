package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	agentexecution "denova/internal/agents/execution"
	agentruntime "denova/internal/agents/runtime"
	"denova/internal/interactive"
)

// settleBoundAbortedGameInput settles only the Game input a stop request
// halted. The runtime persists that binding with the request receipt, so
// replaying an old request after a newer round started must not touch the
// newer round's pending input. A binding read error is returned after the
// stop request was already accepted; the caller surfaces it and a retry
// replays the receipt and reconciles the settlement.
func settleBoundAbortedGameInput(ctx context.Context, control *agentruntime.ExternalController, store *interactive.Store, storyID, branchID, commandID string) error {
	abortedInputCommandID, err := control.AbortedInputCommandID(ctx, commandID)
	if err != nil {
		return err
	}
	if abortedInputCommandID == "" {
		return nil
	}
	return settleAbortedExternalGameInput(store, storyID, branchID, abortedInputCommandID)
}

// settleRecoveredAbortedGameInput is the recovery counterpart of
// settleBoundAbortedGameInput: it resolves the aborted input from the recovery
// receipt binding instead of the current RecoveryInput snapshot.
func settleRecoveredAbortedGameInput(ctx context.Context, control *agentruntime.ExternalController, store *interactive.Store, storyID, branchID string, action agentexecution.RuntimeRecoveryAction) error {
	abortedInputCommandID, err := control.AbortedRecoveryInputCommandID(ctx, action)
	if err != nil {
		return err
	}
	if abortedInputCommandID == "" {
		return nil
	}
	return settleAbortedExternalGameInput(store, storyID, branchID, abortedInputCommandID)
}

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
