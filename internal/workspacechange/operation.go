package workspacechange

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

const (
	operationKindReview = "review"
	operationKindUndo   = "undo"
	operationKindRedo   = "redo"

	operationStageAfterPrepare     = "after_prepare"
	operationStageAfterPathVisible = "after_path_visible"
	operationStageBeforeCommit     = "before_commit"
)

// groupChangePlan is the in-memory form used while preparing an operation.
// Content is moved to immutable blobs before the durable intent is appended.
type groupChangePlan struct {
	ChangeSet ChangeSet
	Metadata  ChangeMetadata
	Before    []byte
	After     []byte
}

type operationChange struct {
	ChangeSet ChangeSet      `json:"change_set"`
	Metadata  ChangeMetadata `json:"metadata"`
}

type operationReviewUpdate struct {
	ChangeSetID string            `json:"change_set_id"`
	Statuses    map[string]string `json:"statuses"`
}

type operationChangeState struct {
	ChangeSetID string `json:"change_set_id"`
	ApplyState  string `json:"apply_state"`
}

// operationProjection is applied only after every visible path has reached its
// target. Keeping it in the prepared record prevents a crash from exposing a
// partially updated review/history projection.
type operationProjection struct {
	ReviewUpdates []operationReviewUpdate `json:"review_updates,omitempty"`
	ChangeStates  []operationChangeState  `json:"change_states,omitempty"`
	HistoryState  string                  `json:"history_state,omitempty"`
}

// durableOperation is the write-ahead record for one logical multi-path
// Review/Undo/Redo operation. ChangeSet blob references contain the complete
// before/after intent without inlining workspace content in the ledger.
type durableOperation struct {
	ID         string              `json:"id"`
	Kind       string              `json:"kind"`
	GroupID    string              `json:"group_id"`
	Changes    []operationChange   `json:"changes,omitempty"`
	Projection operationProjection `json:"projection"`
}

type preparedOperation struct {
	Operation    durableOperation
	AppliedPaths map[string]bool
}

// commitGroupOperationLocked persists the entire operation before the first
// visible write. Once prepared, cancellation no longer interrupts the
// roll-forward sequence: callers may stop waiting, but the durable operation
// remains recoverable and this process continues toward one terminal event.
func (s *Service) commitGroupOperationLocked(
	ctx context.Context,
	kind, groupID string,
	plans []groupChangePlan,
	projection operationProjection,
) error {
	if err := s.contextError(ctx); err != nil {
		return err
	}
	operation := durableOperation{
		ID:         newID("operation"),
		Kind:       kind,
		GroupID:    groupID,
		Changes:    make([]operationChange, 0, len(plans)),
		Projection: cloneOperationProjection(projection),
	}
	seenPaths := make(map[string]bool, len(plans))
	for index := range plans {
		plan := &plans[index]
		if seenPaths[plan.ChangeSet.Path] {
			return newError(ErrorCodeConflict, "group operation contains more than one change for a path", map[string]any{
				"group_id": groupID,
				"path":     plan.ChangeSet.Path,
			})
		}
		seenPaths[plan.ChangeSet.Path] = true
		if stateRevision(plan.Before, plan.ChangeSet.BeforeExists) != plan.ChangeSet.BaseRevision ||
			stateRevision(plan.After, plan.ChangeSet.AfterExists) != plan.ChangeSet.Revision {
			return fmt.Errorf("group operation change %q does not match its content revisions", plan.ChangeSet.ID)
		}
		if err := s.verifyChangeBase(plan.ChangeSet); err != nil {
			return err
		}
		s.assignChangeSequence(&plan.ChangeSet)
		beforeBlob, err := s.store.writeBlob(plan.Before)
		if err != nil {
			return err
		}
		afterBlob, err := s.store.writeBlob(plan.After)
		if err != nil {
			return err
		}
		plan.ChangeSet.BeforeBlob = beforeBlob
		plan.ChangeSet.AfterBlob = afterBlob
		operation.Changes = append(operation.Changes, operationChange{
			ChangeSet: cloneChangeSet(plan.ChangeSet),
			Metadata:  plan.Metadata,
		})
	}
	sort.SliceStable(operation.Changes, func(i, j int) bool {
		return operation.Changes[i].ChangeSet.Path < operation.Changes[j].ChangeSet.Path
	})
	if err := s.validateOperationProjection(operation); err != nil {
		return err
	}
	if err := s.contextError(ctx); err != nil {
		return err
	}
	if err := s.appendAndApply(ledgerEvent{Type: eventOperationPrepared, Operation: &operation}); err != nil {
		return err
	}
	if err := s.runGroupOperationHook(operationStageAfterPrepare, ""); err != nil {
		return err
	}

	// Revalidate the complete set only after the write-ahead record is durable.
	// An external writer racing this phase produces an explicit conflict rather
	// than allowing the operation to partially overwrite workspace state.
	for _, planned := range operation.Changes {
		if err := s.verifyChangeBase(planned.ChangeSet); err != nil {
			return s.conflictOperationLocked(operation.ID, []string{planned.ChangeSet.Path}, err)
		}
	}
	for _, planned := range operation.Changes {
		if err := s.rollForwardOperationPathLocked(operation.ID, planned); err != nil {
			return err
		}
	}
	if err := s.runGroupOperationHook(operationStageBeforeCommit, ""); err != nil {
		return err
	}
	for _, planned := range operation.Changes {
		current, currentExists, err := s.readVisibleState(planned.ChangeSet.Path)
		if err != nil {
			return err
		}
		if currentExists != planned.ChangeSet.AfterExists || stateRevision(current, currentExists) != planned.ChangeSet.Revision {
			return s.conflictOperationLocked(operation.ID, []string{planned.ChangeSet.Path}, newError(
				ErrorCodeRevisionConflict,
				"workspace file diverged before a group operation could finalize",
				map[string]any{"path": planned.ChangeSet.Path},
			))
		}
		if err := s.syncVisibleParent(planned.ChangeSet.Path); err != nil {
			result := mutationResult{
				Stage:            mutationStageVisible,
				ParentRel:        visibleParentRel(planned.ChangeSet.Path),
				WorkspaceMutated: true,
			}
			s.markPendingParentSync(planned.ChangeSet.Path, result.ParentRel)
			return durabilityPendingError(planned.ChangeSet.Path, "", operation.ID, result, err)
		}
	}
	if err := s.appendAndApply(ledgerEvent{Type: eventOperationCommitted, OperationID: operation.ID}); err != nil {
		result := mutationResult{Stage: mutationStageDurable, WorkspaceMutated: len(operation.Changes) > 0}
		if len(operation.Changes) > 0 {
			result.ParentRel = visibleParentRel(operation.Changes[0].ChangeSet.Path)
			return durabilityPendingError(operation.Changes[0].ChangeSet.Path, "", operation.ID, result, err)
		}
		return durabilityPendingError("", "", operation.ID, result, err)
	}
	return nil
}

func (s *Service) rollForwardOperationPathLocked(operationID string, planned operationChange) error {
	change := planned.ChangeSet
	current, currentExists, err := s.readVisibleState(change.Path)
	if err != nil {
		return err
	}
	currentRevision := stateRevision(current, currentExists)
	result := mutationResult{
		Stage:            mutationStageVisible,
		ParentRel:        visibleParentRel(change.Path),
		WorkspaceMutated: true,
	}
	switch {
	case currentExists == change.AfterExists && currentRevision == change.Revision:
		// A previous attempt may have completed the namespace mutation before its
		// progress record. Synchronize the parent before claiming durable progress.
		if err := s.syncVisibleParent(change.Path); err != nil {
			s.markPendingParentSync(change.Path, result.ParentRel)
			return durabilityPendingError(change.Path, "", operationID, result, err)
		}
		result.Stage = mutationStageDurable
	case currentExists == change.BeforeExists && currentRevision == change.BaseRevision:
		after, err := s.store.readBlob(change.AfterBlob)
		if err != nil {
			return err
		}
		result, err = s.writeChangeTarget(change, after)
		if result.Stage == mutationStageVisible || result.Stage == mutationStageDurable {
			delete(s.pendingSaves, change.Path)
		}
		if err != nil {
			if result.Stage == mutationStageVisible {
				s.markPendingParentSync(change.Path, result.ParentRel)
				return durabilityPendingError(change.Path, "", operationID, result, err)
			}
			afterRevision, afterExists := s.currentRevision(change.Path)
			if afterExists != change.AfterExists || afterRevision != change.Revision {
				beforeRevision, beforeExists := s.currentRevision(change.Path)
				if beforeExists == change.BeforeExists && beforeRevision == change.BaseRevision {
					return err
				}
				return s.conflictOperationLocked(operationID, []string{change.Path}, err)
			}
		}
		if result.Stage != mutationStageDurable {
			return durabilityPendingError(change.Path, "", operationID, result, nil)
		}
	default:
		return s.conflictOperationLocked(operationID, []string{change.Path}, newError(
			ErrorCodeRevisionConflict,
			"workspace file diverged while a group operation was being applied",
			map[string]any{
				"path":              change.Path,
				"expected_revision": change.BaseRevision,
				"target_revision":   change.Revision,
				"actual_revision":   currentRevision,
			},
		))
	}
	if err := s.runGroupOperationHook(operationStageAfterPathVisible, change.Path); err != nil {
		return err
	}
	if err := s.appendAndApply(ledgerEvent{
		Type:          eventOperationPathApplied,
		OperationID:   operationID,
		OperationPath: change.Path,
	}); err != nil {
		// The prepared record and visible content still provide an unambiguous
		// recovery path, so retain the operation for a retry/restart.
		return durabilityPendingError(change.Path, "", operationID, result, err)
	}
	return nil
}

func (s *Service) conflictOperationLocked(operationID string, paths []string, cause error) error {
	workspaceMutated := false
	if prepared, ok := s.operations[operationID]; ok {
		workspaceMutated = len(prepared.AppliedPaths) > 0
	}
	ledgerErr := s.appendAndApply(ledgerEvent{
		Type:          eventOperationConflicted,
		OperationID:   operationID,
		ConflictPaths: append([]string(nil), paths...),
	})
	conflict := newError(ErrorCodeConflict, "group operation stopped because workspace content diverged", map[string]any{
		"operation_id":      operationID,
		"conflict_paths":    append([]string(nil), paths...),
		"workspace_mutated": workspaceMutated,
	})
	if ledgerErr != nil {
		return errors.Join(conflict, cause, ledgerErr)
	}
	return errors.Join(conflict, cause)
}

func (s *Service) runGroupOperationHook(stage, path string) error {
	if s.groupOperationHook == nil {
		return nil
	}
	return s.groupOperationHook(stage, path)
}

func (s *Service) validateOperationProjection(operation durableOperation) error {
	if operation.ID == "" || operation.GroupID == "" {
		return fmt.Errorf("workspace group operation is missing identity")
	}
	if operation.Kind != operationKindReview && operation.Kind != operationKindUndo && operation.Kind != operationKindRedo {
		return fmt.Errorf("workspace group operation has invalid kind %q", operation.Kind)
	}
	if state := operation.Projection.HistoryState; state != "" && state != historyStateUndone && state != historyStateRedone {
		return fmt.Errorf("workspace group operation has invalid history state %q", state)
	}
	switch operation.Kind {
	case operationKindReview:
		if operation.Projection.HistoryState != "" {
			return fmt.Errorf("review operation cannot change history state")
		}
	case operationKindUndo:
		if operation.Projection.HistoryState != historyStateUndone {
			return fmt.Errorf("undo operation is missing undone history projection")
		}
	case operationKindRedo:
		if operation.Projection.HistoryState != historyStateRedone {
			return fmt.Errorf("redo operation is missing redone history projection")
		}
	}
	known := make(map[string]*ChangeSet, len(s.changeSets)+len(operation.Changes))
	for id, change := range s.changeSets {
		known[id] = change
	}
	paths := make(map[string]bool, len(operation.Changes))
	for _, planned := range operation.Changes {
		change := planned.ChangeSet
		if change.ID == "" || change.GroupID != operation.GroupID || change.Path == "" {
			return fmt.Errorf("workspace group operation contains invalid change data")
		}
		if known[change.ID] != nil {
			return fmt.Errorf("workspace group operation contains duplicate change %q", change.ID)
		}
		if paths[change.Path] {
			return fmt.Errorf("workspace group operation contains duplicate path %q", change.Path)
		}
		paths[change.Path] = true
		copy := change
		known[change.ID] = &copy
	}
	for _, update := range operation.Projection.ReviewUpdates {
		target := known[update.ChangeSetID]
		if target == nil {
			return fmt.Errorf("workspace group operation review target %q does not exist", update.ChangeSetID)
		}
		if target.GroupID != operation.GroupID {
			return fmt.Errorf("workspace group operation review target %q belongs to another group", update.ChangeSetID)
		}
		knownEdits := make(map[string]bool, len(target.Edits))
		for _, edit := range target.Edits {
			knownEdits[edit.ID] = true
		}
		for editID, status := range update.Statuses {
			if !knownEdits[editID] {
				return fmt.Errorf("workspace group operation review target %q has no edit %q", update.ChangeSetID, editID)
			}
			if status != ReviewStatusAccepted && status != ReviewStatusRejected {
				return fmt.Errorf("workspace group operation has invalid review status %q", status)
			}
		}
	}
	for _, update := range operation.Projection.ChangeStates {
		target := known[update.ChangeSetID]
		if target == nil {
			return fmt.Errorf("workspace group operation state target %q does not exist", update.ChangeSetID)
		}
		if target.GroupID != operation.GroupID {
			return fmt.Errorf("workspace group operation state target %q belongs to another group", update.ChangeSetID)
		}
		if update.ApplyState != ApplyStateApplied && update.ApplyState != ApplyStateReverted {
			return fmt.Errorf("workspace group operation has invalid apply state %q", update.ApplyState)
		}
	}
	return nil
}

func cloneOperationProjection(input operationProjection) operationProjection {
	output := operationProjection{
		ReviewUpdates: append([]operationReviewUpdate(nil), input.ReviewUpdates...),
		ChangeStates:  append([]operationChangeState(nil), input.ChangeStates...),
		HistoryState:  input.HistoryState,
	}
	for index := range output.ReviewUpdates {
		statuses := make(map[string]string, len(input.ReviewUpdates[index].Statuses))
		for id, status := range input.ReviewUpdates[index].Statuses {
			statuses[id] = status
		}
		output.ReviewUpdates[index].Statuses = statuses
	}
	return output
}

func cloneDurableOperation(input durableOperation) durableOperation {
	output := durableOperation{
		ID:         input.ID,
		Kind:       input.Kind,
		GroupID:    input.GroupID,
		Changes:    append([]operationChange(nil), input.Changes...),
		Projection: cloneOperationProjection(input.Projection),
	}
	for index := range output.Changes {
		output.Changes[index].ChangeSet = cloneChangeSet(input.Changes[index].ChangeSet)
	}
	return output
}
