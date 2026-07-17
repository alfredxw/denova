package workspacechange

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"
)

type plannedSpan struct {
	editIndex  int
	start      int
	end        int
	afterStart int
	afterEnd   int
}

// ApplyEdits validates every edit against one immutable base snapshot and
// commits the resulting file exactly once.
func (s *Service) ApplyEdits(ctx context.Context, req ApplyEditsRequest) (ChangeSet, error) {
	if s == nil {
		return ChangeSet{}, newError(ErrorCodeConflict, "change service is nil", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.contextError(ctx); err != nil {
		return ChangeSet{}, err
	}
	if err := s.reconcilePendingDurabilityLocked(); err != nil {
		return ChangeSet{}, err
	}
	rel, err := s.visibleRelPath(req.Path)
	if err != nil {
		return ChangeSet{}, err
	}
	expectedRevision, err := requireBaseRevision(rel, req.BaseRevision)
	if err != nil {
		return ChangeSet{}, err
	}
	before, err := s.readVisibleFile(rel)
	if err != nil {
		return ChangeSet{}, err
	}
	baseRevision := Revision(before)
	if err := requireRevision(rel, expectedRevision, baseRevision); err != nil {
		return ChangeSet{}, err
	}
	after, edits, err := planTextEdits(rel, string(before), req.Edits, req.Metadata.AutoAccept)
	if err != nil {
		return ChangeSet{}, err
	}
	metadata := normalizeMetadata(req.Metadata)
	change := newChangeSet(rel, before, []byte(after), true, true, edits, metadata)
	if err := s.commitChangeLocked(ctx, &change, before, []byte(after), metadata); err != nil {
		return ChangeSet{}, err
	}
	return cloneChangeSet(change), nil
}

// ReplaceFile records a full-file replacement through the same journal used by
// batch edits. It also supports creating a previously missing visible file.
func (s *Service) ReplaceFile(ctx context.Context, req ReplaceFileRequest) (ChangeSet, error) {
	if s == nil {
		return ChangeSet{}, newError(ErrorCodeConflict, "change service is nil", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.contextError(ctx); err != nil {
		return ChangeSet{}, err
	}
	if err := s.reconcilePendingDurabilityLocked(); err != nil {
		return ChangeSet{}, err
	}
	rel, err := s.visibleRelPath(req.Path)
	if err != nil {
		return ChangeSet{}, err
	}
	expectedRevision, err := requireBaseRevision(rel, req.BaseRevision)
	if err != nil {
		return ChangeSet{}, err
	}
	before, readErr := s.readVisibleFile(rel)
	beforeExists := readErr == nil
	if readErr != nil {
		var typed *Error
		if !errors.As(readErr, &typed) || typed.Code != ErrorCodeNotFound {
			return ChangeSet{}, readErr
		}
		before = nil
	}
	actualRevision := "missing"
	if beforeExists {
		actualRevision = Revision(before)
	}
	if err := requireRevision(rel, expectedRevision, actualRevision); err != nil {
		return ChangeSet{}, err
	}
	after := []byte(req.Content)
	if beforeExists && string(before) == req.Content {
		return ChangeSet{}, newError(ErrorCodeInvalidEdit, "replacement does not change the file", map[string]any{"path": rel})
	}
	metadata := normalizeMetadata(req.Metadata)
	reviewStatus := ReviewStatusPending
	if metadata.AutoAccept {
		reviewStatus = ReviewStatusAccepted
	}
	editID := newID("edit")
	edits := []AppliedEdit{{
		ID:           editID,
		OldString:    string(before),
		NewString:    req.Content,
		ReviewStatus: reviewStatus,
		Hunks: []Hunk{{
			ID:          newID("hunk"),
			BeforeStart: 0,
			BeforeEnd:   len(before),
			AfterStart:  0,
			AfterEnd:    len(after),
		}},
	}}
	change := newChangeSet(rel, before, after, beforeExists, true, edits, metadata)
	if err := s.commitChangeLocked(ctx, &change, before, after, metadata); err != nil {
		return ChangeSet{}, err
	}
	return cloneChangeSet(change), nil
}

func requireBaseRevision(path, expected string) (string, error) {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return "", newError(ErrorCodeInvalidEdit, "base_revision is required", map[string]any{
			"path":              path,
			"field":             "base_revision",
			"workspace_mutated": false,
		})
	}
	return expected, nil
}

func requireRevision(path, expected, actual string) error {
	if expected == actual {
		return nil
	}
	return newError(ErrorCodeRevisionConflict, "workspace file revision changed", map[string]any{
		"path":              path,
		"expected_revision": expected,
		"actual_revision":   actual,
		"workspace_mutated": false,
	})
}

func normalizeMetadata(metadata ChangeMetadata) ChangeMetadata {
	metadata.Origin = firstNonEmpty(metadata.Origin, OriginUser)
	metadata.ChangeGroupID = firstNonEmpty(metadata.ChangeGroupID, metadata.RunID, metadata.ToolCallID)
	if metadata.ChangeGroupID == "" {
		metadata.ChangeGroupID = newID("group")
	}
	// A run is its own review thread unless it explicitly continues feedback
	// from an earlier run. This fallback also keeps old callers and ledgers
	// compatible without weakening the per-group history boundary.
	metadata.ReviewThreadID = firstNonEmpty(metadata.ReviewThreadID, metadata.ChangeGroupID)
	return metadata
}

func planTextEdits(path, base string, requested []TextEdit, autoAccept bool) (string, []AppliedEdit, error) {
	if len(requested) == 0 {
		return "", nil, newError(ErrorCodeInvalidEdit, "at least one edit is required", map[string]any{"path": path})
	}
	reviewStatus := ReviewStatusPending
	if autoAccept {
		reviewStatus = ReviewStatusAccepted
	}
	applied := make([]AppliedEdit, len(requested))
	spans := make([]plannedSpan, 0, len(requested))
	seenIDs := map[string]bool{}
	for index, edit := range requested {
		editID := strings.TrimSpace(edit.ID)
		if editID == "" {
			editID = newID("edit")
		}
		if seenIDs[editID] {
			return "", nil, invalidEdit(path, index, editID, "duplicate edit id", nil)
		}
		seenIDs[editID] = true
		if edit.OldString == "" {
			return "", nil, invalidEdit(path, index, editID, "old_string must not be empty", nil)
		}
		if edit.OldString == edit.NewString {
			return "", nil, invalidEdit(path, index, editID, "new_string must differ from old_string", nil)
		}
		matches := literalMatches(base, edit.OldString)
		if len(matches) == 0 {
			return "", nil, invalidEdit(path, index, editID, "old_string was not found", map[string]any{"match_count": 0})
		}
		if len(matches) > 1 && !edit.ReplaceAll {
			return "", nil, invalidEdit(path, index, editID, "old_string is not unique", map[string]any{"match_count": len(matches)})
		}
		if !edit.ReplaceAll {
			matches = matches[:1]
		}
		applied[index] = AppliedEdit{
			ID:           editID,
			OldString:    edit.OldString,
			NewString:    edit.NewString,
			ReplaceAll:   edit.ReplaceAll,
			ReviewStatus: reviewStatus,
		}
		for _, start := range matches {
			spans = append(spans, plannedSpan{editIndex: index, start: start, end: start + len(edit.OldString)})
		}
	}
	sort.SliceStable(spans, func(i, j int) bool {
		if spans[i].start == spans[j].start {
			return spans[i].end < spans[j].end
		}
		return spans[i].start < spans[j].start
	})
	for index := 1; index < len(spans); index++ {
		if spans[index].start < spans[index-1].end {
			return "", nil, newError(ErrorCodeInvalidEdit, "edit ranges overlap", map[string]any{
				"path":              path,
				"edit_id":           applied[spans[index].editIndex].ID,
				"other_edit_id":     applied[spans[index-1].editIndex].ID,
				"workspace_mutated": false,
			})
		}
	}
	delta := 0
	for index := range spans {
		span := &spans[index]
		newText := applied[span.editIndex].NewString
		span.afterStart = span.start + delta
		span.afterEnd = span.afterStart + len(newText)
		delta += len(newText) - (span.end - span.start)
		applied[span.editIndex].Hunks = append(applied[span.editIndex].Hunks, Hunk{
			ID:          newID("hunk"),
			BeforeStart: span.start,
			BeforeEnd:   span.end,
			AfterStart:  span.afterStart,
			AfterEnd:    span.afterEnd,
		})
	}
	result := base
	for index := len(spans) - 1; index >= 0; index-- {
		span := spans[index]
		result = result[:span.start] + applied[span.editIndex].NewString + result[span.end:]
	}
	return result, applied, nil
}

func literalMatches(content, needle string) []int {
	var matches []int
	for offset := 0; offset <= len(content)-len(needle); {
		index := strings.Index(content[offset:], needle)
		if index < 0 {
			break
		}
		start := offset + index
		matches = append(matches, start)
		offset = start + len(needle)
	}
	return matches
}

func invalidEdit(path string, index int, editID, message string, extra map[string]any) *Error {
	details := map[string]any{
		"path":              path,
		"edit_index":        index,
		"edit_id":           editID,
		"workspace_mutated": false,
	}
	for key, value := range extra {
		details[key] = value
	}
	return newError(ErrorCodeInvalidEdit, message, details)
}

func newChangeSet(path string, before, after []byte, beforeExists, afterExists bool, edits []AppliedEdit, metadata ChangeMetadata) ChangeSet {
	reviewStatus := aggregateEditReviewStatus(edits)
	baseRevision := "missing"
	if beforeExists {
		baseRevision = Revision(before)
	}
	revision := "missing"
	if afterExists {
		revision = Revision(after)
	}
	return ChangeSet{
		ID:             newID("change"),
		GroupID:        metadata.ChangeGroupID,
		Path:           path,
		BaseRevision:   baseRevision,
		Revision:       revision,
		BeforeExists:   beforeExists,
		AfterExists:    afterExists,
		Edits:          edits,
		ReviewStatus:   reviewStatus,
		ApplyState:     ApplyStatePrepared,
		CreatedAt:      time.Now().UTC(),
		Origin:         metadata.Origin,
		ReviewThreadID: metadata.ReviewThreadID,
		RunID:          metadata.RunID,
		SessionID:      metadata.SessionID,
		ToolCallID:     metadata.ToolCallID,
	}
}

func (s *Service) commitChangeLocked(ctx context.Context, change *ChangeSet, before, after []byte, metadata ChangeMetadata) error {
	if err := s.contextError(ctx); err != nil {
		return err
	}
	if err := s.reconcilePendingDurabilityLocked(); err != nil {
		return err
	}
	if err := s.verifyChangeBase(*change); err != nil {
		return err
	}
	s.assignChangeSequence(change)
	beforeBlob, err := s.store.writeBlob(before)
	if err != nil {
		return err
	}
	afterBlob, err := s.store.writeBlob(after)
	if err != nil {
		return err
	}
	change.BeforeBlob = beforeBlob
	change.AfterBlob = afterBlob
	prepared := ledgerEvent{Type: eventChangePrepared, Metadata: &metadata, ChangeSet: change}
	if err := s.appendAndApply(prepared); err != nil {
		return err
	}
	// Blob and ledger fsyncs can be relatively expensive. Revalidate immediately
	// before the filesystem mutation so a writer that changed the file while the
	// prepared record was being persisted cannot be silently overwritten.
	if err := s.verifyChangeBase(*change); err != nil {
		if ledgerErr := s.appendAndApply(ledgerEvent{Type: eventChangeAborted, ChangeSetID: change.ID}); ledgerErr != nil {
			return errors.Join(err, ledgerErr)
		}
		return err
	}
	result, writeErr := s.writeChangeTarget(*change, after)
	if result.Stage == mutationStageVisible || result.Stage == mutationStageDurable {
		delete(s.pendingSaves, change.Path)
	}
	if writeErr != nil {
		if result.Stage == mutationStageVisible {
			s.markPendingParentSync(change.Path, result.ParentRel)
			return durabilityPendingError(change.Path, change.ID, "", result, writeErr)
		}
		currentRevision, currentExists := s.currentRevision(change.Path)
		switch {
		case currentExists == change.BeforeExists && currentRevision == change.BaseRevision:
			if ledgerErr := s.appendAndApply(ledgerEvent{Type: eventChangeAborted, ChangeSetID: change.ID}); ledgerErr != nil {
				return errors.Join(writeErr, ledgerErr)
			}
			return writeErr
		default:
			if ledgerErr := s.appendAndApply(ledgerEvent{Type: eventChangeConflicted, ChangeSetID: change.ID}); ledgerErr != nil {
				return errors.Join(writeErr, ledgerErr)
			}
			return newError(ErrorCodeConflict, "file state is ambiguous after a failed atomic write", map[string]any{"path": change.Path, "change_set_id": change.ID, "workspace_mutated": true})
		}
	}
	if result.Stage != mutationStageDurable {
		return durabilityPendingError(change.Path, change.ID, "", result, nil)
	}
	if err := s.appendAndApply(ledgerEvent{Type: eventChangeApplied, ChangeSetID: change.ID}); err != nil {
		return durabilityPendingError(change.Path, change.ID, "", result, err)
	}
	change.ApplyState = ApplyStateApplied
	if err := s.invalidateRedoExcept(metadata.Origin); err != nil {
		// Redo capability also validates the live head, so a ledger failure here
		// cannot make a stale replay overwrite this committed file.
		log.Printf("[workspace-change] committed change but failed to persist redo invalidation path=%q change_set=%q err=%v", change.Path, change.ID, err)
	}
	return nil
}

func (s *Service) verifyChangeBase(change ChangeSet) error {
	current, currentExists, err := s.readVisibleState(change.Path)
	if err != nil {
		return err
	}
	actualRevision := stateRevision(current, currentExists)
	if currentExists == change.BeforeExists && actualRevision == change.BaseRevision {
		return nil
	}
	return newError(ErrorCodeRevisionConflict, "workspace file changed before the prepared change could commit", map[string]any{
		"path":              change.Path,
		"expected_revision": change.BaseRevision,
		"actual_revision":   actualRevision,
		"expected_exists":   change.BeforeExists,
		"actual_exists":     currentExists,
	})
}

func (s *Service) writeChangeTarget(change ChangeSet, after []byte) (mutationResult, error) {
	if change.AfterExists {
		return s.atomicWriteVisibleFile(change.Path, after)
	}
	return s.atomicRemoveVisibleFile(change.Path)
}

func (s *Service) currentRevision(rel string) (string, bool) {
	data, err := s.readVisibleFile(rel)
	if err != nil {
		var typed *Error
		if errors.As(err, &typed) && typed.Code == ErrorCodeNotFound {
			return "missing", false
		}
		return "", false
	}
	return Revision(data), true
}

func (s *Service) hydrateChange(change *ChangeSet, includeContent bool) error {
	if change == nil {
		return nil
	}
	before, err := s.store.readBlob(change.BeforeBlob)
	if err != nil {
		return fmt.Errorf("read before blob for %s: %w", change.ID, err)
	}
	after, err := s.store.readBlob(change.AfterBlob)
	if err != nil {
		return fmt.Errorf("read after blob for %s: %w", change.ID, err)
	}
	for editIndex := range change.Edits {
		edit := &change.Edits[editIndex]
		if len(edit.Hunks) == 0 {
			continue
		}
		hunk := edit.Hunks[0]
		if validSlice(before, hunk.BeforeStart, hunk.BeforeEnd) {
			edit.OldString = string(before[hunk.BeforeStart:hunk.BeforeEnd])
		}
		if validSlice(after, hunk.AfterStart, hunk.AfterEnd) {
			edit.NewString = string(after[hunk.AfterStart:hunk.AfterEnd])
		}
	}
	if includeContent {
		change.BeforeContent = string(before)
		change.AfterContent = string(after)
	}
	return nil
}

func validSlice(content []byte, start, end int) bool {
	return start >= 0 && end >= start && end <= len(content)
}
