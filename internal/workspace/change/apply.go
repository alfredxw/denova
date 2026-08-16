package change

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxWorkspaceMutationFileBytes     = 16 * 1024 * 1024
	maxWorkspaceMutationFragmentBytes = 4 * 1024 * 1024
	maxWorkspaceMutationEdits         = 256
	maxWorkspaceMutationReplacements  = 10_000
	maxWorkspaceMutationScanBytes     = 64 * 1024 * 1024
)

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
	before, beforeMode, err := s.readVisibleFileWithMode(rel)
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
	change.BeforeMode = uint32(beforeMode)
	change.AfterMode = uint32(beforeMode)
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
	if len(req.Content) > maxWorkspaceMutationFileBytes {
		return ChangeSet{}, newError(ErrorCodeInvalidEdit, "replacement exceeds the workspace mutation file limit", map[string]any{
			"max_bytes": maxWorkspaceMutationFileBytes,
		})
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
	before, beforeMode, readErr := s.readVisibleFileWithMode(rel)
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
		return ChangeSet{}, newError(ErrorCodeNoChange, "file already matches the requested content", map[string]any{
			"path": rel, "workspace_mutated": false,
		})
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
	change.BeforeMode = uint32(beforeMode)
	if beforeExists {
		change.AfterMode = uint32(beforeMode)
	}
	if err := s.commitChangeLocked(ctx, &change, before, after, metadata); err != nil {
		return ChangeSet{}, err
	}
	return cloneChangeSet(change), nil
}

// DeleteFile records a complete-file deletion through the same durable journal
// as write and edit. Missing, non-text, non-regular, and unsafe paths fail
// before a prepared change can become visible.
func (s *Service) DeleteFile(ctx context.Context, req DeleteFileRequest) (ChangeSet, error) {
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
	before, beforeMode, err := s.readVisibleFileWithMode(rel)
	if err != nil {
		return ChangeSet{}, err
	}
	if !utf8.Valid(before) {
		return ChangeSet{}, newError(ErrorCodeInvalidEdit, "workspace edit deletion only supports UTF-8 text files", map[string]any{
			"path": rel, "workspace_mutated": false,
		})
	}
	baseRevision := Revision(before)
	if err := requireRevision(rel, expectedRevision, baseRevision); err != nil {
		return ChangeSet{}, err
	}
	metadata := normalizeMetadata(req.Metadata)
	reviewStatus := ReviewStatusPending
	if metadata.AutoAccept {
		reviewStatus = ReviewStatusAccepted
	}
	edit := AppliedEdit{
		ID:           newID("edit"),
		OldString:    string(before),
		NewString:    "",
		ReviewStatus: reviewStatus,
		Hunks: []Hunk{{
			ID:          newID("hunk"),
			BeforeStart: 0,
			BeforeEnd:   len(before),
			AfterStart:  0,
			AfterEnd:    0,
		}},
	}
	change := newChangeSet(rel, before, nil, true, false, []AppliedEdit{edit}, metadata)
	change.BeforeMode = uint32(beforeMode)
	if err := s.commitChangeLocked(ctx, &change, before, nil, metadata); err != nil {
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
	var beforeMode, afterMode uint32
	if beforeExists {
		beforeMode = uint32(defaultVisibleFileMode)
	}
	if afterExists {
		afterMode = uint32(defaultVisibleFileMode)
	}
	var afterFileStats *FileStats
	if afterExists {
		stats := measureFileStats(after)
		afterFileStats = &stats
	}
	return ChangeSet{
		ID:             newID("change"),
		GroupID:        metadata.ChangeGroupID,
		Path:           path,
		BaseRevision:   baseRevision,
		Revision:       revision,
		BeforeExists:   beforeExists,
		AfterExists:    afterExists,
		BeforeMode:     beforeMode,
		AfterMode:      afterMode,
		afterFileStats: afterFileStats,
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
		slog.ErrorContext(ctx, fmt.Sprintf("[workspace-change] committed change but failed to persist redo invalidation path=%q change_set=%q err=%v", change.Path, change.ID, err))
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
		if change.BeforeExists != change.AfterExists {
			mode := os.FileMode(change.AfterMode)
			return s.atomicWriteVisibleFile(change.Path, after, &mode)
		}
		return s.atomicWriteVisibleFile(change.Path, after, nil)
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
