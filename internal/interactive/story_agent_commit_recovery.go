package interactive

import (
	"fmt"
	"os"
	"strings"
)

// FindDomainTurnCommit queries the canonical story log without replaying a
// turn. Found is returned only when branch, full durable cycle identity, and
// semantic hash all match the same turn event.
func (s *Store) FindDomainTurnCommit(
	storyID string,
	branchID string,
	identity DomainCommitIdentity,
	hash string,
) (DomainCommitReceipt, bool, error) {
	if s == nil {
		return DomainCommitReceipt{}, false, fmt.Errorf("interactive store is nil")
	}
	storyID = strings.TrimSpace(storyID)
	branchID = strings.TrimSpace(branchID)
	identity.CommandID = strings.TrimSpace(identity.CommandID)
	identity.OperationID = strings.TrimSpace(identity.OperationID)
	hash = strings.TrimSpace(hash)
	if storyID == "" || branchID == "" || identity.CommandID == "" || identity.OperationID == "" || identity.Cycle <= 0 || hash == "" {
		return DomainCommitReceipt{}, false, fmt.Errorf("%w: story, branch, identity, and hash are required", ErrAgentTurnIdentityConflict)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	_, lines, err := s.readStoryJournalLocked(storyID)
	if os.IsNotExist(err) {
		return DomainCommitReceipt{}, false, nil
	}
	if err != nil {
		return DomainCommitReceipt{}, false, err
	}
	for _, record := range lines {
		if record.Envelope.Type != StoryEventTypeTurn {
			continue
		}
		var turn TurnEvent
		if err := mapToStruct(record.Raw, &turn); err != nil {
			return DomainCommitReceipt{}, false, fmt.Errorf("decode committed agent turn: %w", err)
		}
		if strings.TrimSpace(turn.AgentCommandID) != identity.CommandID {
			continue
		}
		if turn.BranchID != branchID || strings.TrimSpace(turn.AgentOperationID) != identity.OperationID ||
			turn.AgentCycle != identity.Cycle || strings.TrimSpace(turn.AgentCommitHash) != hash {
			return DomainCommitReceipt{}, false, fmt.Errorf(
				"%w: command_id=%q canonical turn does not match requested branch, operation, cycle, and hash",
				ErrAgentTurnIdentityConflict,
				identity.CommandID,
			)
		}
		return DomainCommitReceipt{
			Identity: identity,
			Hash:     hash,
			Revision: turn.ID,
			Turn:     turn,
			Delta:    stateDeltaEventForCommittedTurn(turn),
		}, true, nil
	}
	return DomainCommitReceipt{}, false, nil
}

// FindRecentDomainTurnCommit is the bounded runtime-recovery lookup. Only the
// recent commit window can belong to an active operation; callers that
// explicitly audit arbitrary historical commands use FindDomainTurnCommit.
func (s *Store) FindRecentDomainTurnCommit(
	storyID string,
	branchID string,
	identity DomainCommitIdentity,
	hash string,
) (DomainCommitReceipt, bool, error) {
	if s == nil {
		return DomainCommitReceipt{}, false, fmt.Errorf("interactive store is nil")
	}
	storyID = strings.TrimSpace(storyID)
	branchID = strings.TrimSpace(branchID)
	identity.CommandID = strings.TrimSpace(identity.CommandID)
	identity.OperationID = strings.TrimSpace(identity.OperationID)
	hash = strings.TrimSpace(hash)
	if storyID == "" || branchID == "" || identity.CommandID == "" || identity.OperationID == "" || identity.Cycle <= 0 || hash == "" {
		return DomainCommitReceipt{}, false, fmt.Errorf("%w: story, branch, identity, and hash are required", ErrAgentTurnIdentityConflict)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, locator, found, err := s.recentStoryCommitLocked(storyID, StoryEventTypeTurn, identity)
	if os.IsNotExist(err) {
		return DomainCommitReceipt{}, false, nil
	}
	if err != nil || !found {
		if locator.CommandID == identity.CommandID &&
			(locator.OperationID != identity.OperationID || locator.Cycle != identity.Cycle) {
			return DomainCommitReceipt{}, false, fmt.Errorf("%w: command_id=%q recent turn has a different operation or cycle", ErrAgentTurnIdentityConflict, identity.CommandID)
		}
		return DomainCommitReceipt{}, false, err
	}
	var turn TurnEvent
	if err := mapToStruct(record.Raw, &turn); err != nil {
		return DomainCommitReceipt{}, false, fmt.Errorf("decode committed agent turn: %w", err)
	}
	if turn.BranchID != branchID || strings.TrimSpace(turn.AgentOperationID) != identity.OperationID ||
		turn.AgentCycle != identity.Cycle || strings.TrimSpace(turn.AgentCommitHash) != hash {
		return DomainCommitReceipt{}, false, fmt.Errorf(
			"%w: command_id=%q canonical turn does not match requested branch, operation, cycle, and hash",
			ErrAgentTurnIdentityConflict, identity.CommandID,
		)
	}
	return DomainCommitReceipt{
		Identity: identity, Hash: hash, Revision: turn.ID, Turn: turn,
		Delta: stateDeltaEventForCommittedTurn(turn),
	}, true, nil
}
