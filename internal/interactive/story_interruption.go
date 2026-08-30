package interactive

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// TurnInterruptedEvent keeps the partial narrative for one accepted player
// input so a later turn can continue it without replaying completed work.
type TurnInterruptedEvent struct {
	V                int    `json:"v"`
	Type             string `json:"type"`
	ID               string `json:"id"`
	ParentID         string `json:"parent_id,omitempty"`
	BranchID         string `json:"branch_id"`
	Ts               string `json:"ts"`
	PlayerInputID    string `json:"player_input_id"`
	UserMessage      string `json:"user_message,omitempty"`
	AssistantContent string `json:"assistant_content,omitempty"`
	Reason           string `json:"reason"`
}

func (s *Store) MarkTurnInterrupted(
	storyID,
	branchID,
	playerInputID,
	userMessage,
	assistantContent,
	reason string,
) (TurnInterruptedEvent, error) {
	if s == nil {
		return TurnInterruptedEvent{}, fmt.Errorf("interactive store is nil")
	}
	storyID = strings.TrimSpace(storyID)
	branchID = strings.TrimSpace(branchID)
	playerInputID = strings.TrimSpace(playerInputID)
	reason = strings.TrimSpace(reason)
	if storyID == "" || branchID == "" || playerInputID == "" || reason == "" {
		return TurnInterruptedEvent{}, fmt.Errorf("story, branch, player input, and interruption reason are required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return TurnInterruptedEvent{}, err
	}
	defer releaseStory()
	meta, _, err := s.readStoryRecentLocked(storyID, branchID)
	if err != nil {
		return TurnInterruptedEvent{}, err
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return TurnInterruptedEvent{}, fmt.Errorf("interactive branch does not exist: %s", branchID)
	}
	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return TurnInterruptedEvent{}, err
	}
	if !projection.hasPendingPlayerInput(playerInputID) {
		return TurnInterruptedEvent{}, fmt.Errorf("player input is no longer pending: %s", playerInputID)
	}
	event := TurnInterruptedEvent{
		V: schemaVersion, Type: StoryEventTypeTurnInterrupted,
		ID: deterministicTurnInterruptionID(playerInputID), ParentID: branch.Head,
		BranchID: branchID, Ts: time.Now().UTC().Format(time.RFC3339Nano),
		PlayerInputID: playerInputID, UserMessage: strings.TrimSpace(userMessage),
		AssistantContent: assistantContent, Reason: reason,
	}
	if existing := projection.PendingInterruption; existing != nil && existing.ID == event.ID {
		if existing.PlayerInputID == event.PlayerInputID && existing.UserMessage == event.UserMessage &&
			existing.AssistantContent == event.AssistantContent && existing.Reason == event.Reason {
			return *existing, nil
		}
		return TurnInterruptedEvent{}, fmt.Errorf("turn interruption identity conflict: %s", event.ID)
	}
	if err := validateTurnInterruptedEvent(event); err != nil {
		return TurnInterruptedEvent{}, err
	}
	meta.UpdatedAt = event.Ts
	if err := s.appendStoryTransactionLocked(storyID, meta, event); err != nil {
		return TurnInterruptedEvent{}, err
	}
	s.syncStoryIndexProjectionLocked(storyID)
	return event, nil
}

// PendingTurnInterruption returns the latest interruption only while its
// player input still belongs to the active branch ancestry.
func (s *Store) PendingTurnInterruption(storyID, branchID string) (*TurnInterruptedEvent, error) {
	if s == nil {
		return nil, fmt.Errorf("interactive store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	storyID = strings.TrimSpace(storyID)
	branchID = strings.TrimSpace(branchID)
	_, snapshot, err := s.boundedStorySnapshotLocked(storyID, branchID)
	if err != nil {
		return nil, err
	}
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return nil, err
	}
	defer release()
	handle, err := s.refreshStoryJournalLocked(storyID, true)
	if err != nil {
		return nil, err
	}
	branch := handle.projection.Branches[snapshot.BranchID]
	if branch == nil || branch.PendingInterruption == nil {
		return nil, nil
	}
	pendingInput := false
	for _, input := range snapshot.PendingPlayerInputs {
		if input.ID == branch.PendingInterruption.PlayerInputID {
			pendingInput = true
			break
		}
	}
	if !pendingInput {
		return nil, nil
	}
	result := *branch.PendingInterruption
	return &result, nil
}

func validateTurnInterruptedEvent(event TurnInterruptedEvent) error {
	if event.V != schemaVersion || event.Type != StoryEventTypeTurnInterrupted ||
		strings.TrimSpace(event.ID) == "" || strings.TrimSpace(event.BranchID) == "" ||
		strings.TrimSpace(event.PlayerInputID) == "" || strings.TrimSpace(event.Reason) == "" ||
		strings.TrimSpace(event.Ts) == "" {
		return fmt.Errorf("turn interruption event is invalid")
	}
	if event.ID != deterministicTurnInterruptionID(event.PlayerInputID) {
		return fmt.Errorf("turn interruption identity does not match player input")
	}
	return nil
}

func deterministicTurnInterruptionID(playerInputID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(playerInputID)))
	return "turn-interruption-" + hex.EncodeToString(sum[:16])
}
