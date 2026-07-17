package workspacechange

import (
	"context"
	"errors"
	"log"
)

// SaveFile persists a local editor snapshot under the same workspace mutation
// lock as Agent changes. It deliberately creates no review group or content
// blobs: local editor undo owns user keystrokes, while durable group history
// owns Agent-authored changes.
func (s *Service) SaveFile(ctx context.Context, path, content, baseRevision string) (SaveResult, error) {
	if s == nil {
		return SaveResult{}, newError(ErrorCodeConflict, "change service is nil", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.contextError(ctx); err != nil {
		return SaveResult{}, err
	}
	if err := s.reconcilePendingDurabilityLocked(); err != nil {
		return SaveResult{}, err
	}
	rel, err := s.visibleRelPath(path)
	if err != nil {
		return SaveResult{}, err
	}
	expectedRevision, err := requireBaseRevision(rel, baseRevision)
	if err != nil {
		return SaveResult{}, err
	}
	after := []byte(content)
	revision := Revision(after)
	if pending, ok := s.pendingSaves[rel]; ok && pending.Durable &&
		pending.BaseRevision == expectedRevision && pending.Revision == revision {
		current, currentExists, readErr := s.readVisibleState(rel)
		if readErr != nil {
			return SaveResult{}, readErr
		}
		delete(s.pendingSaves, rel)
		if currentExists && Revision(current) == pending.Revision {
			return SaveResult{Revision: revision, Changed: true}, nil
		}
	}
	before, beforeExists, err := s.readVisibleState(rel)
	if err != nil {
		return SaveResult{}, err
	}
	actualRevision := "missing"
	if beforeExists {
		actualRevision = Revision(before)
	}
	if err := requireRevision(rel, expectedRevision, actualRevision); err != nil {
		return SaveResult{}, err
	}
	if beforeExists && actualRevision == revision {
		return SaveResult{Revision: revision, Changed: false}, nil
	}
	mutation, writeErr := s.atomicWriteVisibleFile(rel, after)
	if writeErr != nil {
		if mutation.Stage == mutationStageVisible {
			s.markPendingParentSync(rel, mutation.ParentRel)
			s.pendingSaves[rel] = pendingSaveIntent{
				Path:         rel,
				ParentRel:    mutation.ParentRel,
				BaseRevision: expectedRevision,
				Revision:     revision,
			}
			return SaveResult{}, durabilityPendingError(rel, "", "", mutation, writeErr)
		}
		return SaveResult{}, writeErr
	}
	if mutation.Stage != mutationStageDurable {
		return SaveResult{}, durabilityPendingError(rel, "", "", mutation, nil)
	}
	delete(s.pendingSaves, rel)
	result := SaveResult{Revision: revision, Changed: true}
	// Manual input starts a new local editing branch. Keep the redo projection
	// accurate without recording the editor snapshot as a reviewable change.
	if err := s.invalidateRedoExcept(OriginUser); err != nil {
		log.Printf("[workspace-change] editor save committed but redo invalidation persistence failed path=%q err=%v", rel, err)
	}
	return result, nil
}

func (s *Service) readVisibleState(rel string) ([]byte, bool, error) {
	content, err := s.readVisibleFile(rel)
	if err == nil {
		return content, true, nil
	}
	var typed *Error
	if errors.As(err, &typed) && typed.Code == ErrorCodeNotFound {
		return nil, false, nil
	}
	return nil, false, err
}
