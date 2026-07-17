package workspacechange

import (
	"errors"
	"sort"
)

// recoverOperations deterministically rolls every unfinished group operation
// forward. A path still at its recorded before state is safe to write, a path
// already at after is recognized as completed, and any third state becomes a
// terminal conflict without being overwritten.
func (s *Service) recoverOperations() error {
	if len(s.operations) == 0 {
		return nil
	}
	ids := make([]string, 0, len(s.operations))
	for id := range s.operations {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		prepared, ok := s.operations[id]
		if !ok {
			continue
		}
		operation := prepared.Operation
		conflictPath := ""
		for _, planned := range operation.Changes {
			change := planned.ChangeSet
			result := mutationResult{
				Stage:            mutationStageVisible,
				ParentRel:        visibleParentRel(change.Path),
				WorkspaceMutated: true,
			}
			current, currentExists, err := s.readVisibleState(change.Path)
			if err != nil {
				return err
			}
			currentRevision := stateRevision(current, currentExists)
			switch {
			case currentExists == change.AfterExists && currentRevision == change.Revision:
				// The namespace mutation may have completed before its progress
				// record. Persist its parent entry before recording progress.
				if err := s.syncVisibleParent(change.Path); err != nil {
					s.markPendingParentSync(change.Path, result.ParentRel)
					return durabilityPendingError(change.Path, "", id, result, err)
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
						return durabilityPendingError(change.Path, "", id, result, err)
					}
					visible, visibleExists, readErr := s.readVisibleState(change.Path)
					if readErr != nil {
						return errors.Join(err, readErr)
					}
					visibleRevision := stateRevision(visible, visibleExists)
					if visibleExists != change.AfterExists || visibleRevision != change.Revision {
						if visibleExists == change.BeforeExists && visibleRevision == change.BaseRevision {
							return err
						}
						conflictPath = change.Path
						break
					}
					result = mutationResult{
						Stage:            mutationStageVisible,
						ParentRel:        visibleParentRel(change.Path),
						WorkspaceMutated: false,
					}
					if syncErr := s.syncVisibleParent(change.Path); syncErr != nil {
						s.markPendingParentSync(change.Path, result.ParentRel)
						return durabilityPendingError(change.Path, "", id, result, errors.Join(err, syncErr))
					}
					result.Stage = mutationStageDurable
				}
				if result.Stage != mutationStageDurable {
					return durabilityPendingError(change.Path, "", id, result, nil)
				}
			default:
				conflictPath = change.Path
			}
			if conflictPath != "" {
				break
			}
			prepared = s.operations[id]
			if !prepared.AppliedPaths[change.Path] {
				if err := s.appendAndApply(ledgerEvent{
					Type:          eventOperationPathApplied,
					OperationID:   id,
					OperationPath: change.Path,
				}); err != nil {
					return durabilityPendingError(change.Path, "", id, result, err)
				}
			}
		}
		if conflictPath != "" {
			if err := s.appendAndApply(ledgerEvent{
				Type:          eventOperationConflicted,
				OperationID:   id,
				ConflictPaths: []string{conflictPath},
			}); err != nil {
				return err
			}
			continue
		}
		for _, planned := range operation.Changes {
			current, currentExists, err := s.readVisibleState(planned.ChangeSet.Path)
			if err != nil {
				return err
			}
			if currentExists != planned.ChangeSet.AfterExists || stateRevision(current, currentExists) != planned.ChangeSet.Revision {
				conflictPath = planned.ChangeSet.Path
				break
			}
			if err := s.syncVisibleParent(planned.ChangeSet.Path); err != nil {
				result := mutationResult{
					Stage:            mutationStageVisible,
					ParentRel:        visibleParentRel(planned.ChangeSet.Path),
					WorkspaceMutated: true,
				}
				s.markPendingParentSync(planned.ChangeSet.Path, result.ParentRel)
				return durabilityPendingError(planned.ChangeSet.Path, "", id, result, err)
			}
		}
		if conflictPath != "" {
			if err := s.appendAndApply(ledgerEvent{
				Type:          eventOperationConflicted,
				OperationID:   id,
				ConflictPaths: []string{conflictPath},
			}); err != nil {
				return err
			}
			continue
		}
		if err := s.appendAndApply(ledgerEvent{Type: eventOperationCommitted, OperationID: id}); err != nil {
			result := mutationResult{Stage: mutationStageDurable, WorkspaceMutated: len(operation.Changes) > 0}
			path := ""
			if len(operation.Changes) > 0 {
				path = operation.Changes[0].ChangeSet.Path
				result.ParentRel = visibleParentRel(path)
			}
			return durabilityPendingError(path, "", id, result, err)
		}
	}
	return nil
}

func (s *Service) recoverPrepared() error {
	if len(s.prepared) == 0 {
		return nil
	}
	ids := make([]string, 0, len(s.prepared))
	for id := range s.prepared {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		prepared := s.prepared[id]
		change := prepared.ChangeSet
		current, currentExists, err := s.readVisibleState(change.Path)
		if err != nil {
			return err
		}
		currentRevision := stateRevision(current, currentExists)
		switch {
		case currentExists == change.AfterExists && currentRevision == change.Revision:
			result := mutationResult{
				Stage:            mutationStageVisible,
				ParentRel:        visibleParentRel(change.Path),
				WorkspaceMutated: true,
			}
			if err := s.syncVisibleParent(change.Path); err != nil {
				s.markPendingParentSync(change.Path, result.ParentRel)
				return durabilityPendingError(change.Path, id, "", result, err)
			}
			if err := s.appendAndApply(ledgerEvent{Type: eventChangeRecoveredApplied, ChangeSetID: id}); err != nil {
				result.Stage = mutationStageDurable
				return durabilityPendingError(change.Path, id, "", result, err)
			}
		case currentExists == change.BeforeExists && currentRevision == change.BaseRevision:
			if err := s.appendAndApply(ledgerEvent{Type: eventChangeAborted, ChangeSetID: id}); err != nil {
				return err
			}
		default:
			// Expose the unresolved intent for review without choosing either blob or
			// overwriting the independently modified workspace file.
			if err := s.appendAndApply(ledgerEvent{Type: eventChangeConflicted, ChangeSetID: id}); err != nil {
				return err
			}
		}
	}
	return nil
}
