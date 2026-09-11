package interactive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"denova/internal/agents/conversationjournal"
	"denova/internal/agents/sessionjournal"

	agent "github.com/alfredxw/denova/agent"
)

const maxTurnDraftBytes = 16 << 20

// TurnSubmissionProgress is the product-owned acceptance state, including a
// partially accepted plan document. It contains no validator or runtime object.
type TurnSubmissionProgress struct {
	Result               TurnResult `json:"result"`
	PlanUpdate           *string    `json:"plan_update,omitempty"`
	StateChangesAccepted bool       `json:"state_changes_accepted"`
	ChoicesAccepted      bool       `json:"choices_accepted"`
	PlanUpdateAccepted   bool       `json:"plan_update_accepted"`
	PlanUpdateStarted    bool       `json:"plan_update_started"`
}

func (prepared *PreparedTurnSubmission) Progress() *TurnSubmissionProgress {
	if prepared == nil {
		return nil
	}
	return &TurnSubmissionProgress{Result: prepared.TurnResult(), PlanUpdate: cloneStringPointer(prepared.result.PlanUpdate), StateChangesAccepted: prepared.stateUpdatesAccepted,
		ChoicesAccepted: prepared.choicesAccepted, PlanUpdateAccepted: prepared.planUpdateAccepted, PlanUpdateStarted: prepared.planUpdateStarted}
}

func (progress *TurnSubmissionProgress) Prepared() *PreparedTurnSubmission {
	if progress == nil {
		return nil
	}
	result := progress.Result
	result.PlanUpdate = cloneStringPointer(progress.PlanUpdate)
	return clonePreparedTurnSubmission(&PreparedTurnSubmission{result: result, stateUpdatesAccepted: progress.StateChangesAccepted,
		choicesAccepted: progress.ChoicesAccepted, planUpdateAccepted: progress.PlanUpdateAccepted, planUpdateStarted: progress.PlanUpdateStarted})
}

// TurnDraft is the unfinished product turn. A final Turn with the same exact
// identity ends it in that Turn's transaction; draft text never becomes Actor
// State or official story history by itself.
type TurnDraft struct {
	Identity        DomainCommitIdentity    `json:"identity"`
	Narrative       string                  `json:"narrative,omitempty"`
	NarrativeSource string                  `json:"narrative_source,omitempty"`
	Submission      *TurnSubmissionProgress `json:"submission,omitempty"`
	RuleResolution  *RuleResolution         `json:"rule_resolution,omitempty"`
}

type TurnDraftEvent struct {
	V        int       `json:"v"`
	Type     string    `json:"type"`
	ID       string    `json:"id"`
	ParentID string    `json:"parent_id,omitempty"`
	BranchID string    `json:"branch_id"`
	Ts       string    `json:"ts"`
	Draft    TurnDraft `json:"draft"`
}

type storyDraftLocator struct {
	ID     string                     `json:"id"`
	Cursor conversationjournal.Cursor `json:"cursor"`
}

func turnDraftKey(branchID string, identity DomainCommitIdentity) string {
	return branchID + ":" + deterministicPlayerInputID(identity)
}

func validateTurnDraft(draft TurnDraft) error {
	if draft.Identity.CommandID == "" || draft.Identity.OperationID == "" || draft.Identity.Cycle <= 0 {
		return errors.New("turn draft requires an exact Agent cycle identity")
	}
	if draft.Narrative != "" && draft.NarrativeSource == "" {
		return errors.New("turn draft narrative requires a source")
	}
	encoded, err := json.Marshal(draft)
	if err != nil {
		return err
	}
	if len(encoded) > maxTurnDraftBytes {
		return fmt.Errorf("turn draft exceeds the %d-byte product limit", maxTurnDraftBytes)
	}
	return nil
}

// SaveTurnDraft confirms product progress and the Agent acceptance facts in
// one journal transaction, before returning acceptance to a model or tool.
func (s *Store) SaveTurnDraft(storyID, branchID string, draft TurnDraft, checkpoint agent.CanonicalCheckpoint) error {
	if err := validateTurnDraft(draft); err != nil {
		return err
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
	branch, found := meta.Branches[branchID]
	if !found {
		return errors.New("turn draft branch is unavailable")
	}
	handle := s.storyJournals[storyID]
	projection := handle.projection.branch(branchID)
	if !projection.hasPendingPlayerInput(deterministicPlayerInputID(draft.Identity)) {
		return errors.New("turn draft has no pending canonical player input")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	event := TurnDraftEvent{V: schemaVersion, Type: StoryEventTypeTurnDraft, ID: newID("draft"), ParentID: branch.Head, BranchID: branchID, Ts: now, Draft: draft}
	meta.UpdatedAt = now
	agentRecords, err := sessionjournal.CheckpointRecords(&handle.projection.AgentSessions, checkpoint, event.ID)
	if err != nil {
		return err
	}
	return s.appendStoryTransactionLocked(storyID, meta, append([]any{event}, agentRecords...)...)
}

// LoadTurnDraft uses a rebuildable locator, retaining every unfinished draft
// regardless of the recent-turn window. It never executes a submission tool.
func (s *Store) LoadTurnDraft(storyID, branchID string, identity DomainCommitIdentity) (TurnDraft, bool, error) {
	if strings.TrimSpace(identity.CommandID) == "" {
		return TurnDraft{}, false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	handle, err := s.openStoryJournalLocked(storyID)
	if err != nil {
		return TurnDraft{}, false, err
	}
	if _, err := handle.journal.ReadRange(context.Background(), conversationjournal.Range{After: handle.journal.Head().Cursor}); err != nil {
		return TurnDraft{}, false, err
	}
	locator, found := handle.projection.TurnDrafts[turnDraftKey(branchID, identity)]
	if !found {
		return TurnDraft{}, false, nil
	}
	records, err := handle.journal.ReadRange(context.Background(), conversationjournal.Range{After: locator.Cursor - 1, Through: locator.Cursor})
	if err != nil {
		return TurnDraft{}, false, err
	}
	for _, record := range records {
		var event TurnDraftEvent
		if err := json.Unmarshal(record.Payload, &event); err != nil {
			return TurnDraft{}, false, err
		}
		if event.Type == StoryEventTypeTurnDraft && event.ID == locator.ID {
			if event.Draft.Identity != identity || event.BranchID != branchID {
				return TurnDraft{}, false, errors.New("turn draft identity mismatch")
			}
			return event.Draft, true, validateTurnDraft(event.Draft)
		}
	}
	return TurnDraft{}, false, errors.New("turn draft locator has no canonical record")
}
