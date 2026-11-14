package interactive

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// TurnAbandonedEvent settles an accepted input without creating a fictional
// story turn. The original input, interrupted draft, and tool audit remain
// in the canonical journal, but cannot leak into subsequent model context.
type TurnAbandonedEvent struct {
	V                int    `json:"v"`
	Type             string `json:"type"`
	ID               string `json:"id"`
	ParentID         string `json:"parent_id,omitempty"`
	BranchID         string `json:"branch_id"`
	Ts               string `json:"ts"`
	PlayerInputID    string `json:"player_input_id"`
	AgentCommandID   string `json:"agent_command_id,omitempty"`
	AgentOperationID string `json:"agent_operation_id,omitempty"`
	AgentCycle       int    `json:"agent_cycle,omitempty"`
	Reason           string `json:"reason"`
}

func validateTurnAbandonedEvent(event TurnAbandonedEvent) error {
	if event.V != schemaVersion || event.Type != StoryEventTypeTurnAbandoned ||
		strings.TrimSpace(event.BranchID) == "" || strings.TrimSpace(event.PlayerInputID) == "" ||
		strings.TrimSpace(event.Ts) == "" || strings.TrimSpace(event.Reason) == "" ||
		event.ID != deterministicTurnAbandonmentID(event.PlayerInputID) {
		return fmt.Errorf("turn abandonment event is invalid")
	}
	return nil
}

func deterministicTurnAbandonmentID(playerInputID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(playerInputID)))
	return "turn-abandoned-" + hex.EncodeToString(sum[:16])
}

// AbandonPendingTurn is only for a user-requested abort of an interrupted
// Game operation. It leaves the branch head, character state, plan and
// previously committed turns untouched; repeating the call is harmless.
func (s *Store) AbandonPendingTurn(storyID, branchID, playerInputID string) error {
	if s == nil {
		return fmt.Errorf("interactive store is nil")
	}
	storyID, branchID, playerInputID = strings.TrimSpace(storyID), strings.TrimSpace(branchID), strings.TrimSpace(playerInputID)
	if storyID == "" || branchID == "" || playerInputID == "" {
		return fmt.Errorf("story, branch, and player input are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return err
	}
	defer release()
	meta, _, err := s.readStoryRecentLocked(storyID, branchID)
	if err != nil {
		return err
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return fmt.Errorf("interactive branch does not exist: %s", branchID)
	}
	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return err
	}
	if !projection.hasPendingPlayerInput(playerInputID) {
		return nil
	}
	_, snapshot, err := s.boundedStorySnapshotLocked(storyID, branchID)
	if err != nil {
		return err
	}
	var accepted *PlayerInputAcceptedEvent
	for i := range snapshot.PendingPlayerInputs {
		if snapshot.PendingPlayerInputs[i].ID == playerInputID {
			accepted = &snapshot.PendingPlayerInputs[i]
			break
		}
	}
	if accepted == nil {
		return fmt.Errorf("player input is not pending on the active branch: %s", playerInputID)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	event := TurnAbandonedEvent{
		V: schemaVersion, Type: StoryEventTypeTurnAbandoned,
		ID: deterministicTurnAbandonmentID(playerInputID), ParentID: branch.Head,
		BranchID: branchID, Ts: now, PlayerInputID: playerInputID,
		AgentCommandID: accepted.AgentCommandID,
		AgentOperationID: accepted.AgentOperationID,
		AgentCycle: accepted.AgentCycle, Reason: "user_requested",
	}
	if err := validateTurnAbandonedEvent(event); err != nil {
		return err
	}
	meta.UpdatedAt = now
	if err := s.appendStoryTransactionLocked(storyID, meta, event); err != nil {
		return err
	}
	s.syncStoryIndexProjectionLocked(storyID)
	return nil
}
