package workspacechange

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxCommentBodyBytes   = 64 * 1024
	maxCommentAuthorBytes = 1024
	maxCommentAnchorBytes = 128 * 1024
)

func (s *Service) AddComment(ctx context.Context, req AddCommentRequest) (Comment, error) {
	if s == nil {
		return Comment{}, newError(ErrorCodeConflict, "change service is nil", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.contextError(ctx); err != nil {
		return Comment{}, err
	}
	if err := s.reconcilePendingDurabilityLocked(); err != nil {
		return Comment{}, err
	}
	req.GroupID = strings.TrimSpace(req.GroupID)
	if s.groups[req.GroupID] == nil {
		return Comment{}, newError(ErrorCodeNotFound, "change group not found", map[string]any{"group_id": req.GroupID})
	}
	if err := s.validateCommentTarget(req.GroupID, req.ChangeSetID, req.EditID, req.HunkID); err != nil {
		return Comment{}, err
	}
	req.ChangeSetID = strings.TrimSpace(req.ChangeSetID)
	req.EditID = strings.TrimSpace(req.EditID)
	req.HunkID = strings.TrimSpace(req.HunkID)
	req.Anchor = normalizeCommentAnchor(req.Anchor)
	body, err := validateCommentBody(req.Body)
	if err != nil {
		return Comment{}, err
	}
	if err := validateCommentMetadata(req.Author, req.Anchor); err != nil {
		return Comment{}, err
	}
	if err := s.validateCommentAnchor(req.GroupID, req.ChangeSetID, req.Anchor); err != nil {
		return Comment{}, err
	}
	now := time.Now().UTC()
	comment := Comment{
		ID:          newID("comment"),
		GroupID:     req.GroupID,
		ChangeSetID: req.ChangeSetID,
		EditID:      req.EditID,
		HunkID:      req.HunkID,
		Body:        body,
		Author:      strings.TrimSpace(req.Author),
		CreatedAt:   now,
		UpdatedAt:   now,
		Anchor:      req.Anchor,
	}
	if err := s.appendAndApply(ledgerEvent{Type: eventCommentUpserted, Comment: &comment}); err != nil {
		return Comment{}, err
	}
	return comment, nil
}

func (s *Service) UpdateComment(ctx context.Context, req UpdateCommentRequest) (Comment, error) {
	if s == nil {
		return Comment{}, newError(ErrorCodeConflict, "change service is nil", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.contextError(ctx); err != nil {
		return Comment{}, err
	}
	if err := s.reconcilePendingDurabilityLocked(); err != nil {
		return Comment{}, err
	}
	current := s.comments[strings.TrimSpace(req.ID)]
	if current == nil || current.Deleted {
		return Comment{}, newError(ErrorCodeNotFound, "comment not found", map[string]any{"comment_id": req.ID})
	}
	body, err := validateCommentBody(req.Body)
	if err != nil {
		return Comment{}, err
	}
	next := *current
	next.Body = body
	if author := strings.TrimSpace(req.Author); author != "" {
		next.Author = author
	}
	if req.Anchor != nil {
		anchor := normalizeCommentAnchor(*req.Anchor)
		if err := validateCommentMetadata(next.Author, anchor); err != nil {
			return Comment{}, err
		}
		if err := s.validateCommentAnchor(next.GroupID, next.ChangeSetID, anchor); err != nil {
			return Comment{}, err
		}
		next.Anchor = anchor
	} else if err := validateCommentMetadata(next.Author, next.Anchor); err != nil {
		return Comment{}, err
	}
	next.UpdatedAt = time.Now().UTC()
	if err := s.appendAndApply(ledgerEvent{Type: eventCommentUpserted, Comment: &next}); err != nil {
		return Comment{}, err
	}
	return next, nil
}

func validateCommentMetadata(author string, anchor CommentAnchor) error {
	if len(author) > maxCommentAuthorBytes {
		return newError(ErrorCodeConflict, "comment author is too large", map[string]any{"max_bytes": maxCommentAuthorBytes})
	}
	anchorBytes := len(anchor.Side) + len(anchor.Encoding) + len(anchor.Kind) + len(anchor.Revision) + len(anchor.Quote) + len(anchor.Prefix) + len(anchor.Suffix)
	if anchorBytes > maxCommentAnchorBytes {
		return newError(ErrorCodeConflict, "comment anchor is too large", map[string]any{"max_bytes": maxCommentAnchorBytes})
	}
	if anchor.Start < 0 || anchor.End < 0 || anchor.End < anchor.Start {
		return newError(ErrorCodeConflict, "comment anchor range is invalid", map[string]any{"start": anchor.Start, "end": anchor.End})
	}
	return nil
}

func normalizeCommentAnchor(anchor CommentAnchor) CommentAnchor {
	anchor.Side = strings.TrimSpace(anchor.Side)
	anchor.Encoding = strings.TrimSpace(anchor.Encoding)
	anchor.Kind = strings.TrimSpace(anchor.Kind)
	anchor.Revision = strings.TrimSpace(anchor.Revision)
	return anchor
}

func emptyCommentAnchor(anchor CommentAnchor) bool {
	return anchor.Side == "" && anchor.Encoding == "" && anchor.Kind == "" && anchor.Revision == "" &&
		anchor.Start == 0 && anchor.End == 0 && anchor.Quote == "" && anchor.Prefix == "" && anchor.Suffix == ""
}

func (s *Service) validateCommentAnchor(groupID, changeSetID string, anchor CommentAnchor) error {
	if emptyCommentAnchor(anchor) {
		return nil
	}
	if changeSetID == "" {
		return newError(ErrorCodeConflict, "an inline comment anchor requires a change_set_id", map[string]any{"group_id": groupID})
	}
	if anchor.Side != CommentAnchorSideBefore && anchor.Side != CommentAnchorSideAfter {
		return newError(ErrorCodeConflict, "comment anchor side is invalid", map[string]any{
			"side": anchor.Side, "allowed": []string{CommentAnchorSideBefore, CommentAnchorSideAfter},
		})
	}
	if anchor.Encoding != CommentAnchorEncodingUTF8Byte {
		return newError(ErrorCodeConflict, "comment anchor encoding is invalid", map[string]any{
			"encoding": anchor.Encoding, "allowed": CommentAnchorEncodingUTF8Byte,
		})
	}
	change := s.changeSets[changeSetID]
	if change == nil || change.GroupID != groupID {
		return newError(ErrorCodeNotFound, "comment change set not found", map[string]any{"group_id": groupID, "change_set_id": changeSetID})
	}
	hydrated := cloneChangeSet(*change)
	if err := s.hydrateChange(&hydrated, true); err != nil {
		return err
	}

	revision := hydrated.BaseRevision
	content := hydrated.BeforeContent
	exists := hydrated.BeforeExists
	if anchor.Side == CommentAnchorSideAfter {
		revision = hydrated.Revision
		content = hydrated.AfterContent
		exists = hydrated.AfterExists
	}
	if !exists || anchor.Revision != revision {
		return newError(ErrorCodeConflict, "comment anchor revision does not match the selected change side", map[string]any{
			"change_set_id": changeSetID,
			"side":          anchor.Side,
			"expected":      revision,
			"actual":        anchor.Revision,
		})
	}
	if !utf8.ValidString(content) || anchor.Start < 0 || anchor.End < anchor.Start || anchor.End > len(content) ||
		!utf8ByteBoundary(content, anchor.Start) || !utf8ByteBoundary(content, anchor.End) {
		return newError(ErrorCodeConflict, "comment anchor is not a valid UTF-8 byte range", map[string]any{
			"start": anchor.Start, "end": anchor.End, "content_bytes": len(content),
		})
	}
	if content[anchor.Start:anchor.End] != anchor.Quote {
		return newError(ErrorCodeConflict, "comment anchor quote does not match the selected change side", map[string]any{
			"change_set_id": changeSetID, "side": anchor.Side, "start": anchor.Start, "end": anchor.End,
		})
	}
	if anchor.Prefix != "" && (anchor.Start < len(anchor.Prefix) || content[anchor.Start-len(anchor.Prefix):anchor.Start] != anchor.Prefix) {
		return newError(ErrorCodeConflict, "comment anchor prefix does not match the selected change side", map[string]any{"change_set_id": changeSetID})
	}
	if anchor.Suffix != "" && (anchor.End+len(anchor.Suffix) > len(content) || content[anchor.End:anchor.End+len(anchor.Suffix)] != anchor.Suffix) {
		return newError(ErrorCodeConflict, "comment anchor suffix does not match the selected change side", map[string]any{"change_set_id": changeSetID})
	}
	return nil
}

func utf8ByteBoundary(content string, offset int) bool {
	return offset == 0 || offset == len(content) || (offset > 0 && offset < len(content) && utf8.RuneStart(content[offset]))
}

func (s *Service) ResolveComment(ctx context.Context, req ResolveCommentRequest) (Comment, error) {
	if s == nil {
		return Comment{}, newError(ErrorCodeConflict, "change service is nil", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.contextError(ctx); err != nil {
		return Comment{}, err
	}
	if err := s.reconcilePendingDurabilityLocked(); err != nil {
		return Comment{}, err
	}
	current := s.comments[strings.TrimSpace(req.ID)]
	if current == nil || current.Deleted {
		return Comment{}, newError(ErrorCodeNotFound, "comment not found", map[string]any{"comment_id": req.ID})
	}
	next := *current
	next.Resolved = req.Resolved
	next.UpdatedAt = time.Now().UTC()
	if err := s.appendAndApply(ledgerEvent{Type: eventCommentUpserted, Comment: &next}); err != nil {
		return Comment{}, err
	}
	return next, nil
}

func (s *Service) DeleteComment(ctx context.Context, req DeleteCommentRequest) (Comment, error) {
	if s == nil {
		return Comment{}, newError(ErrorCodeConflict, "change service is nil", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.contextError(ctx); err != nil {
		return Comment{}, err
	}
	if err := s.reconcilePendingDurabilityLocked(); err != nil {
		return Comment{}, err
	}
	current := s.comments[strings.TrimSpace(req.ID)]
	if current == nil {
		return Comment{}, newError(ErrorCodeNotFound, "comment not found", map[string]any{"comment_id": req.ID})
	}
	if current.Deleted {
		return *current, nil
	}
	next := *current
	next.Deleted = true
	next.UpdatedAt = time.Now().UTC()
	if err := s.appendAndApply(ledgerEvent{Type: eventCommentUpserted, Comment: &next}); err != nil {
		return Comment{}, err
	}
	return next, nil
}

func validateCommentBody(body string) (string, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", newError(ErrorCodeConflict, "comment body is empty", nil)
	}
	if len(body) > maxCommentBodyBytes {
		return "", newError(ErrorCodeConflict, "comment body is too large", map[string]any{"max_bytes": maxCommentBodyBytes})
	}
	return body, nil
}

func (s *Service) validateCommentTarget(groupID, changeSetID, editID, hunkID string) error {
	changeSetID = strings.TrimSpace(changeSetID)
	editID = strings.TrimSpace(editID)
	hunkID = strings.TrimSpace(hunkID)
	if changeSetID == "" {
		if editID != "" || hunkID != "" {
			return newError(ErrorCodeConflict, "edit or hunk comments require a change_set_id", map[string]any{"group_id": groupID})
		}
		return nil
	}
	change := s.changeSets[changeSetID]
	if change == nil || change.GroupID != groupID {
		return newError(ErrorCodeNotFound, "comment change set not found", map[string]any{"group_id": groupID, "change_set_id": changeSetID})
	}
	if editID == "" && hunkID == "" {
		return nil
	}
	for _, edit := range change.Edits {
		if editID != "" && edit.ID != editID {
			continue
		}
		if hunkID == "" {
			return nil
		}
		for _, hunk := range edit.Hunks {
			if hunk.ID == hunkID {
				return nil
			}
		}
	}
	return newError(ErrorCodeNotFound, "comment edit or hunk not found", map[string]any{"change_set_id": changeSetID, "edit_id": editID, "hunk_id": hunkID})
}
