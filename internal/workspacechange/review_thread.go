package workspacechange

import (
	"context"
	"sort"
	"strings"
	"time"
)

// GetReviewThread derives a cross-run review projection without changing the
// history identity of any constituent group.
func (s *Service) GetReviewThread(ctx context.Context, id string) (ReviewThread, error) {
	if s == nil {
		return ReviewThread{}, newError(ErrorCodeConflict, "change service is nil", nil)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := s.contextError(ctx); err != nil {
		return ReviewThread{}, err
	}

	id = strings.TrimSpace(id)
	groups := s.reviewThreadGroupsLocked(id)
	if len(groups) == 0 {
		return ReviewThread{}, newError(ErrorCodeNotFound, "review thread not found", map[string]any{"review_thread_id": id})
	}

	thread := ReviewThread{
		ID:            id,
		CreatedAt:     groups[0].CreatedAt,
		UpdatedAt:     groups[0].CreatedAt,
		LatestGroupID: groups[len(groups)-1].ID,
		Groups:        make([]ChangeGroupSummary, 0, len(groups)),
		Comments:      make([]Comment, 0),
		Files:         make([]ReviewThreadFile, 0),
	}
	allChanges := make([]ChangeSet, 0)
	byPath := map[string][]*ChangeSet{}
	for _, group := range groups {
		if group.CreatedAt.Before(thread.CreatedAt) {
			thread.CreatedAt = group.CreatedAt
		}
		if group.CreatedAt.After(thread.UpdatedAt) {
			thread.UpdatedAt = group.CreatedAt
		}
		thread.PendingEditCount += group.PendingEditCount
		thread.Groups = append(thread.Groups, s.groupSummaryLocked(group, uniqueGroupPaths(group.ChangeSets)))
		for index := range group.ChangeSets {
			change := s.changeSets[group.ChangeSets[index].ID]
			if change == nil {
				continue
			}
			copy := cloneChangeSet(*change)
			allChanges = append(allChanges, copy)
			byPath[copy.Path] = append(byPath[copy.Path], change)
			if copy.CreatedAt.After(thread.UpdatedAt) {
				thread.UpdatedAt = copy.CreatedAt
			}
		}
		for _, comment := range group.Comments {
			thread.Comments = append(thread.Comments, comment)
			if !comment.Deleted && !comment.Resolved {
				thread.UnresolvedCommentCount++
			}
			if comment.UpdatedAt.After(thread.UpdatedAt) {
				thread.UpdatedAt = comment.UpdatedAt
			}
		}
	}

	sort.SliceStable(allChanges, func(i, j int) bool {
		if allChanges[i].Sequence == allChanges[j].Sequence {
			return allChanges[i].ID < allChanges[j].ID
		}
		return allChanges[i].Sequence < allChanges[j].Sequence
	})
	thread.ReviewStatus = aggregateChangeReviewStatus(allChanges)
	thread.ApplyState = aggregateChangeApplyState(allChanges)

	sort.SliceStable(thread.Comments, func(i, j int) bool {
		if thread.Comments[i].CreatedAt.Equal(thread.Comments[j].CreatedAt) {
			return thread.Comments[i].ID < thread.Comments[j].ID
		}
		return thread.Comments[i].CreatedAt.Before(thread.Comments[j].CreatedAt)
	})

	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		file, err := s.reviewThreadFileLocked(path, byPath[path])
		if err != nil {
			return ReviewThread{}, err
		}
		thread.Files = append(thread.Files, file)
	}
	return thread, nil
}

func (s *Service) reviewThreadGroupsLocked(id string) []*ChangeGroup {
	if id == "" {
		return nil
	}
	groups := make([]*ChangeGroup, 0)
	for _, group := range s.groups {
		if reviewThreadID(group) == id {
			groups = append(groups, group)
		}
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].CreatedAt.Equal(groups[j].CreatedAt) {
			return groups[i].ID < groups[j].ID
		}
		return groups[i].CreatedAt.Before(groups[j].CreatedAt)
	})
	return groups
}

func (s *Service) reviewThreadFileLocked(path string, changes []*ChangeSet) (ReviewThreadFile, error) {
	sort.SliceStable(changes, func(i, j int) bool {
		if changes[i].Sequence == changes[j].Sequence {
			return changes[i].ID < changes[j].ID
		}
		return changes[i].Sequence < changes[j].Sequence
	})

	file := ReviewThreadFile{
		Path:           path,
		Continuity:     ReviewThreadContinuityContinuous,
		GroupIDs:       make([]string, 0),
		ChangeSetIDs:   make([]string, 0, len(changes)),
		PendingEditIDs: make([]string, 0),
	}
	groupSeen := map[string]bool{}
	projected := make([]ChangeSet, 0, len(changes))
	segmentStart := 0
	for index, change := range changes {
		copy := cloneChangeSet(*change)
		projected = append(projected, copy)
		file.ChangeSetIDs = append(file.ChangeSetIDs, copy.ID)
		if !groupSeen[copy.GroupID] {
			groupSeen[copy.GroupID] = true
			file.GroupIDs = append(file.GroupIDs, copy.GroupID)
		}
		if copy.Origin != OriginUndo && copy.Origin != OriginRedo && copy.Origin != OriginReview && copy.ApplyState == ApplyStateApplied {
			for _, edit := range copy.Edits {
				if firstNonEmpty(edit.ReviewStatus, ReviewStatusPending) == ReviewStatusPending {
					file.PendingEditIDs = append(file.PendingEditIDs, edit.ID)
				}
			}
		}
		if index > 0 {
			previous := changes[index-1]
			if previous.AfterExists != change.BeforeExists || previous.Revision != change.BaseRevision {
				file.Continuity = ReviewThreadContinuityConflicted
				segmentStart = index
			}
		}
	}

	first := cloneChangeSet(*changes[segmentStart])
	last := cloneChangeSet(*changes[len(changes)-1])
	if err := s.hydrateChange(&first, true); err != nil {
		return ReviewThreadFile{}, err
	}
	if last.ID == first.ID {
		last = first
	} else if err := s.hydrateChange(&last, true); err != nil {
		return ReviewThreadFile{}, err
	}
	file.BeforeExists = first.BeforeExists
	file.AfterExists = last.AfterExists
	file.BaseRevision = first.BaseRevision
	file.Revision = last.Revision
	file.BeforeContent = first.BeforeContent
	file.AfterContent = last.AfterContent
	file.BaseGroupID = first.GroupID
	file.BaseChangeSetID = first.ID
	file.LatestGroupID = last.GroupID
	file.LatestChangeSetID = last.ID
	file.OmittedIterationCount = segmentStart
	file.ReviewStatus = aggregateChangeReviewStatus(projected)
	file.ApplyState = aggregateChangeApplyState(projected)
	return file, nil
}

// GetReviewComments resolves trusted, unresolved ledger comments for one Agent
// feedback turn. Duplicate IDs are returned once in caller order.
func (s *Service) GetReviewComments(ctx context.Context, threadID, sessionID string, commentIDs []string) ([]ReviewFeedbackComment, error) {
	if s == nil {
		return nil, newError(ErrorCodeConflict, "change service is nil", nil)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := s.contextError(ctx); err != nil {
		return nil, err
	}

	return s.reviewCommentsLocked(threadID, sessionID, commentIDs)
}

// ConsumeReviewComments atomically marks one validated feedback batch deleted
// after its durable user message has been committed.
func (s *Service) ConsumeReviewComments(ctx context.Context, threadID, sessionID string, commentIDs []string) ([]Comment, error) {
	if s == nil {
		return nil, newError(ErrorCodeConflict, "change service is nil", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.contextError(ctx); err != nil {
		return nil, err
	}
	if err := s.reconcilePendingDurabilityLocked(); err != nil {
		return nil, err
	}
	resolved, err := s.reviewCommentsLocked(threadID, sessionID, commentIDs)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	consumed := make([]Comment, 0, len(resolved))
	for _, item := range resolved {
		comment := item.Comment
		comment.Deleted = true
		comment.UpdatedAt = now
		consumed = append(consumed, comment)
	}
	if len(consumed) == 0 {
		return nil, nil
	}
	if err := s.appendAndApply(ledgerEvent{Type: eventCommentsUpserted, Comments: consumed}); err != nil {
		return nil, err
	}
	return consumed, nil
}

func (s *Service) reviewCommentsLocked(threadID, sessionID string, commentIDs []string) ([]ReviewFeedbackComment, error) {
	threadID = strings.TrimSpace(threadID)
	sessionID = strings.TrimSpace(sessionID)
	groups := s.reviewThreadGroupsLocked(threadID)
	if len(groups) == 0 {
		return nil, newError(ErrorCodeNotFound, "review thread not found", map[string]any{"review_thread_id": threadID})
	}
	if sessionID == "" {
		return nil, newError(ErrorCodeConflict, "review feedback requires a session identity", map[string]any{"review_thread_id": threadID})
	}
	for _, group := range groups {
		if group.SessionID != sessionID {
			return nil, newError(ErrorCodeConflict, "review thread does not belong to the active session", map[string]any{
				"review_thread_id": threadID,
				"session_id":       sessionID,
			})
		}
	}
	result := make([]ReviewFeedbackComment, 0, len(commentIDs))
	seen := make(map[string]bool, len(commentIDs))
	for _, requestedID := range commentIDs {
		id := strings.TrimSpace(requestedID)
		if seen[id] {
			continue
		}
		seen[id] = true
		comment := s.comments[id]
		if comment == nil || comment.Deleted {
			return nil, newError(ErrorCodeNotFound, "review comment not found", map[string]any{"comment_id": id})
		}
		group := s.groups[comment.GroupID]
		if group == nil || reviewThreadID(group) != threadID {
			return nil, newError(ErrorCodeConflict, "review comment does not belong to the requested thread", map[string]any{
				"comment_id":       id,
				"review_thread_id": threadID,
			})
		}
		if comment.Resolved {
			return nil, newError(ErrorCodeConflict, "review comment is already resolved", map[string]any{"comment_id": id})
		}
		resolved := ReviewFeedbackComment{Comment: *comment}
		if comment.ChangeSetID != "" {
			change := s.changeSets[comment.ChangeSetID]
			if change == nil || change.GroupID != comment.GroupID {
				return nil, newError(ErrorCodeConflict, "review comment target is unavailable", map[string]any{"comment_id": id})
			}
			resolved.Path = change.Path
		}
		result = append(result, resolved)
	}
	return result, nil
}
