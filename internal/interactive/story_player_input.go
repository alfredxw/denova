package interactive

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrPlayerInputIdentityConflict = errors.New("player input identity conflict")

// PlayerInputIntent is the append-only canonical form of one accepted game
// cycle before any model, tool, or narrative effect starts.
type PlayerInputIntent struct {
	Identity DomainCommitIdentity `json:"identity"`
	BranchID string               `json:"branch_id"`
	Text     string               `json:"text"`
	Hash     string               `json:"hash"`
}

type PlayerInputAcceptedEvent struct {
	V        int    `json:"v"`
	Type     string `json:"type"`
	ID       string `json:"id"`
	ParentID string `json:"parent_id,omitempty"`
	BranchID string `json:"branch_id"`
	Ts       string `json:"ts"`
	Text     string `json:"text"`
	// AcceptedTurnCount is the completed-turn boundary visible when the input
	// was accepted. Side events do not advance branch.Head, so ParentID alone
	// cannot place an interrupted input around later turns when its parent is a
	// structural event. Keeping this logical boundary makes the model projection
	// stable across settlement and cold reload.
	AcceptedTurnCount *int   `json:"accepted_turn_count,omitempty"`
	AgentCommandID    string `json:"agent_command_id"`
	AgentOperationID  string `json:"agent_operation_id"`
	AgentCycle        int    `json:"agent_cycle"`
	AgentCommitHash   string `json:"agent_commit_hash"`
}

type PlayerInputReceipt struct {
	Identity DomainCommitIdentity     `json:"identity"`
	Hash     string                   `json:"hash"`
	Revision string                   `json:"revision"`
	Event    PlayerInputAcceptedEvent `json:"event"`
}

func NewPlayerInputIntent(identity DomainCommitIdentity, branchID, text string) (PlayerInputIntent, error) {
	identity.CommandID = strings.TrimSpace(identity.CommandID)
	identity.OperationID = strings.TrimSpace(identity.OperationID)
	branchID = strings.TrimSpace(branchID)
	if identity.CommandID == "" || identity.OperationID == "" || identity.Cycle <= 0 || branchID == "" {
		return PlayerInputIntent{}, fmt.Errorf("%w: command_id, operation_id, positive cycle, and branch_id are required", ErrPlayerInputIdentityConflict)
	}
	if strings.TrimSpace(text) == "" {
		return PlayerInputIntent{}, fmt.Errorf("%w: player input is empty", ErrPlayerInputIdentityConflict)
	}
	payload, err := json.Marshal(struct {
		BranchID string `json:"branch_id"`
		Text     string `json:"text"`
	}{BranchID: branchID, Text: text})
	if err != nil {
		return PlayerInputIntent{}, err
	}
	sum := sha256.Sum256(payload)
	return PlayerInputIntent{
		Identity: identity, BranchID: branchID, Text: text,
		Hash: "sha256:" + hex.EncodeToString(sum[:]),
	}, nil
}

func (s *Store) CommitPlayerInput(storyID string, intent PlayerInputIntent) (PlayerInputReceipt, error) {
	canonical, err := NewPlayerInputIntent(intent.Identity, intent.BranchID, intent.Text)
	if err != nil {
		return PlayerInputReceipt{}, err
	}
	if canonical.Hash != strings.TrimSpace(intent.Hash) {
		return PlayerInputReceipt{}, fmt.Errorf("%w: staged player input hash changed", ErrPlayerInputIdentityConflict)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return PlayerInputReceipt{}, err
	}
	defer releaseStory()
	meta, lines, err := s.readStoryRecentLocked(storyID, canonical.BranchID)
	if err != nil {
		return PlayerInputReceipt{}, err
	}
	branch, ok := meta.Branches[canonical.BranchID]
	if !ok {
		return PlayerInputReceipt{}, fmt.Errorf("分支不存在: %s", canonical.BranchID)
	}
	if receipt, found, err := findPlayerInputCommitInLines(lines, canonical.Identity, canonical.BranchID, canonical.Hash); err != nil || found {
		return receipt, err
	}
	projection, err := s.storyBranchProjectionLocked(storyID, canonical.BranchID)
	if err != nil {
		return PlayerInputReceipt{}, err
	}
	acceptedTurnCount := projection.Depth
	now := time.Now().UTC().Format(time.RFC3339Nano)
	event := PlayerInputAcceptedEvent{
		V: schemaVersion, Type: StoryEventTypePlayerInput,
		ID: deterministicPlayerInputID(canonical.Identity), ParentID: branch.Head,
		BranchID: canonical.BranchID, Ts: now, Text: canonical.Text, AcceptedTurnCount: &acceptedTurnCount,
		AgentCommandID: canonical.Identity.CommandID, AgentOperationID: canonical.Identity.OperationID,
		AgentCycle: canonical.Identity.Cycle, AgentCommitHash: canonical.Hash,
	}
	meta.UpdatedAt = now
	if appendErr := s.appendStoryTransactionLocked(storyID, meta, event); appendErr != nil {
		return PlayerInputReceipt{}, appendErr
	}
	s.syncStoryIndexProjectionLocked(storyID)
	return playerInputReceipt(canonical.Identity, event), nil
}

// FindPlayerInputCommit is a pure exact identity+hash query used by runtime
// receipt recovery. It never performs story migration or canonical writes.
func (s *Store) FindPlayerInputCommit(
	storyID, branchID string,
	identity DomainCommitIdentity,
	hash string,
) (PlayerInputReceipt, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, lines, err := s.readStoryJournalLocked(storyID)
	if err != nil {
		return PlayerInputReceipt{}, false, err
	}
	return findPlayerInputCommitInLines(lines, identity, strings.TrimSpace(branchID), strings.TrimSpace(hash))
}

// FindRecentPlayerInputCommit is the bounded counterpart used by active
// runtime recovery. It proves new command IDs absent from the projection
// window without scanning historical prose, thinking, or display records.
func (s *Store) FindRecentPlayerInputCommit(
	storyID, branchID string,
	identity DomainCommitIdentity,
	hash string,
) (PlayerInputReceipt, bool, error) {
	if s == nil {
		return PlayerInputReceipt{}, false, fmt.Errorf("interactive store is nil")
	}
	identity.CommandID = strings.TrimSpace(identity.CommandID)
	identity.OperationID = strings.TrimSpace(identity.OperationID)
	branchID = strings.TrimSpace(branchID)
	hash = strings.TrimSpace(hash)
	s.mu.Lock()
	defer s.mu.Unlock()
	record, locator, found, err := s.recentStoryCommitLocked(storyID, StoryEventTypePlayerInput, identity)
	if err != nil || !found {
		if locator.CommandID == identity.CommandID &&
			(locator.OperationID != identity.OperationID || locator.Cycle != identity.Cycle) {
			return PlayerInputReceipt{}, false, fmt.Errorf("%w: command_id=%q operation_id=%q cycle=%d", ErrPlayerInputIdentityConflict, identity.CommandID, identity.OperationID, identity.Cycle)
		}
		return PlayerInputReceipt{}, false, err
	}
	return findPlayerInputCommitInLines([]StoryEventRecord{record}, identity, branchID, hash)
}

func findPlayerInputCommitInLines(
	lines []StoryEventRecord,
	identity DomainCommitIdentity,
	branchID, hash string,
) (PlayerInputReceipt, bool, error) {
	identity.CommandID = strings.TrimSpace(identity.CommandID)
	identity.OperationID = strings.TrimSpace(identity.OperationID)
	for _, record := range lines {
		if record.Envelope.Type != StoryEventTypePlayerInput {
			continue
		}
		var event PlayerInputAcceptedEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return PlayerInputReceipt{}, false, err
		}
		if strings.TrimSpace(event.AgentCommandID) != identity.CommandID {
			continue
		}
		if strings.TrimSpace(event.AgentOperationID) != identity.OperationID || event.AgentCycle != identity.Cycle ||
			event.BranchID != branchID || strings.TrimSpace(event.AgentCommitHash) != hash {
			return PlayerInputReceipt{}, false, fmt.Errorf("%w: command_id=%q operation_id=%q cycle=%d", ErrPlayerInputIdentityConflict, identity.CommandID, identity.OperationID, identity.Cycle)
		}
		canonical, err := NewPlayerInputIntent(identity, branchID, event.Text)
		if err != nil || canonical.Hash != hash || event.ID != deterministicPlayerInputID(identity) {
			return PlayerInputReceipt{}, false, fmt.Errorf("%w: canonical player input payload changed", ErrPlayerInputIdentityConflict)
		}
		return playerInputReceipt(identity, event), true, nil
	}
	return PlayerInputReceipt{}, false, nil
}

func playerInputReceipt(identity DomainCommitIdentity, event PlayerInputAcceptedEvent) PlayerInputReceipt {
	return PlayerInputReceipt{Identity: identity, Hash: event.AgentCommitHash, Revision: event.ID, Event: event}
}

func deterministicPlayerInputID(identity DomainCommitIdentity) string {
	sum := sha256.Sum256([]byte(identity.CommandID + "\x00" + identity.OperationID + "\x00" + fmt.Sprint(identity.Cycle)))
	return "player-input-" + hex.EncodeToString(sum[:16])
}

func playerInputForTurnRequest(
	lines []StoryEventRecord,
	branchID string,
	req AppendTurnWithStateRequest,
) (PlayerInputAcceptedEvent, bool, error) {
	identity := DomainCommitIdentity{
		CommandID: strings.TrimSpace(req.AgentCommandID), OperationID: strings.TrimSpace(req.AgentOperationID), Cycle: req.AgentCycle,
	}
	if identity.CommandID == "" {
		return PlayerInputAcceptedEvent{}, false, nil
	}
	for _, record := range lines {
		if record.Envelope.Type != StoryEventTypePlayerInput {
			continue
		}
		var event PlayerInputAcceptedEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return PlayerInputAcceptedEvent{}, false, err
		}
		if event.AgentCommandID != identity.CommandID {
			continue
		}
		if event.AgentOperationID != identity.OperationID || event.AgentCycle != identity.Cycle || event.BranchID != branchID || event.Text != req.User {
			return PlayerInputAcceptedEvent{}, false, fmt.Errorf("%w: completed turn does not match accepted player input", ErrPlayerInputIdentityConflict)
		}
		canonical, err := NewPlayerInputIntent(identity, branchID, req.User)
		if err != nil || canonical.Hash != event.AgentCommitHash {
			return PlayerInputAcceptedEvent{}, false, fmt.Errorf("%w: completed turn player input hash changed", ErrPlayerInputIdentityConflict)
		}
		return event, true, nil
	}
	return PlayerInputAcceptedEvent{}, false, nil
}
