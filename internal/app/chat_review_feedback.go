package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"denova/internal/agent"
	"denova/internal/book"
	"denova/internal/documentreview"
	"denova/internal/workspacechange"
)

const maxReviewFeedbackCommentIDs = 256

// resolveReviewFeedback replaces client-supplied IDs with trusted comments
// from the canonical service for the captured workspace. Comment bodies never
// cross the HTTP boundary into ChatRequest.
func (s *ChatAppService) resolveReviewFeedback(runtime ideChatRuntime, req *agent.ChatRequest) error {
	if req == nil {
		return nil
	}
	threadID := strings.TrimSpace(req.ReviewFeedback.ReviewThreadID)
	requestedIDs := normalizeReviewFeedbackCommentIDs(req.ReviewFeedback.CommentIDs)
	if threadID == "" && len(requestedIDs) == 0 {
		req.ReviewFeedback = agent.ReviewFeedbackRef{}
		req.ResolvedReviewFeedback = agent.ReviewFeedbackContext{}
		return nil
	}
	source, validSource := agent.NormalizeReviewFeedbackSource(req.ReviewFeedback.Source)
	if !validSource {
		return invalidReviewFeedbackError("review feedback source is invalid", map[string]any{"source": req.ReviewFeedback.Source})
	}
	if threadID == "" || len(requestedIDs) == 0 {
		return invalidReviewFeedbackError("review_thread_id and comment_ids must be provided together", nil)
	}
	if len(requestedIDs) > maxReviewFeedbackCommentIDs {
		return invalidReviewFeedbackError("too many review comments were referenced", map[string]any{
			"maximum": maxReviewFeedbackCommentIDs,
			"actual":  len(requestedIDs),
		})
	}
	if source == agent.ReviewFeedbackSourceWorkspaceChange && (runtime.sess == nil || strings.TrimSpace(runtime.sess.ID) == "") {
		return invalidReviewFeedbackError("the active session identity is unavailable", nil)
	}

	feedback := agent.ReviewFeedbackContext{
		Source:         source,
		ReviewThreadID: threadID,
		Comments:       make([]agent.ReviewFeedbackComment, 0, len(requestedIDs)),
	}
	if source == agent.ReviewFeedbackSourceDocument {
		if err := s.resolveDocumentReviewFeedback(runtime, threadID, requestedIDs, &feedback); err != nil {
			return err
		}
	} else {
		if err := s.resolveWorkspaceChangeReviewFeedback(runtime, threadID, requestedIDs, &feedback); err != nil {
			return err
		}
	}
	if feedback.EncodedSize() > agent.MaxReviewFeedbackContextBytes {
		return invalidReviewFeedbackError("review feedback context exceeds the allowed size", map[string]any{
			"maximum_bytes": agent.MaxReviewFeedbackContextBytes,
		})
	}
	req.ReviewFeedback = agent.ReviewFeedbackRef{Source: source, ReviewThreadID: threadID, CommentIDs: requestedIDs}
	req.ResolvedReviewFeedback = feedback
	return nil
}

func (s *ChatAppService) resolveWorkspaceChangeReviewFeedback(runtime ideChatRuntime, threadID string, requestedIDs []string, feedback *agent.ReviewFeedbackContext) error {
	var resolved []workspacechange.ReviewFeedbackComment
	err := s.app.WithWorkspaceChangeService(runtime.workspace, func(service *workspacechange.Service) error {
		var resolveErr error
		resolved, resolveErr = service.GetReviewComments(context.Background(), threadID, runtime.sess.ID, requestedIDs)
		return resolveErr
	})
	if err != nil {
		return err
	}
	for _, item := range resolved {
		comment := item.Comment
		feedback.Comments = append(feedback.Comments, agent.ReviewFeedbackComment{
			ID: comment.ID, GroupID: comment.GroupID, ChangeSetID: comment.ChangeSetID, EditID: comment.EditID,
			HunkID: comment.HunkID, Path: item.Path, Body: comment.Body,
			Anchor: agent.ReviewFeedbackAnchor{
				Side: comment.Anchor.Side, Encoding: comment.Anchor.Encoding, Kind: comment.Anchor.Kind,
				Revision: comment.Anchor.Revision, Start: comment.Anchor.Start, End: comment.Anchor.End,
				Quote: comment.Anchor.Quote, Prefix: comment.Anchor.Prefix, Suffix: comment.Anchor.Suffix,
			},
		})
	}
	return nil
}

func (s *ChatAppService) resolveDocumentReviewFeedback(runtime ideChatRuntime, threadID string, requestedIDs []string, feedback *agent.ReviewFeedbackContext) error {
	_, err := s.app.WithDocumentReviewService(runtime.workspace, func(service *documentreview.Service, files *book.Service) error {
		comments, resolveErr := service.GetReviewComments(context.Background(), threadID, requestedIDs)
		if resolveErr != nil {
			return resolveErr
		}
		for _, comment := range comments {
			content, revision, readErr := files.ReadFileWithRevision(comment.Path)
			if readErr != nil {
				return readErr
			}
			anchor, outdated := documentreview.ProjectAnchor(content, revision, comment.Anchor)
			if outdated {
				return invalidReviewFeedbackError("a document comment no longer identifies unique source text", map[string]any{
					"comment_id": comment.ID, "path": comment.Path,
				})
			}
			feedback.Comments = append(feedback.Comments, agent.ReviewFeedbackComment{
				ID: comment.ID, Path: comment.Path, Body: comment.Body,
				Anchor: agent.ReviewFeedbackAnchor{
					Encoding: anchor.Encoding, Kind: anchor.Kind, Revision: anchor.Revision,
					Start: anchor.Start, End: anchor.End, Quote: anchor.Quote, Prefix: anchor.Prefix,
					Suffix: anchor.Suffix, DisplayQuote: anchor.DisplayQuote,
				},
			})
		}
		return nil
	})
	if err == nil {
		return nil
	}
	var reviewErr *documentreview.Error
	if errors.As(err, &reviewErr) {
		code := workspacechange.ErrorCodeConflict
		switch reviewErr.Code {
		case documentreview.ErrorCodeNotFound:
			code = workspacechange.ErrorCodeNotFound
		case documentreview.ErrorCodeInvalid:
			code = workspacechange.ErrorCodeInvalidEdit
		}
		return &workspacechange.Error{Code: code, Message: reviewErr.Message, Details: reviewErr.Details}
	}
	return err
}

func (s *ChatAppService) consumeResolvedReviewFeedback(runtime ideChatRuntime, req agent.ChatRequest) error {
	if req.ResolvedReviewFeedback.Empty() {
		return nil
	}
	if req.ResolvedReviewFeedback.Source == agent.ReviewFeedbackSourceDocument {
		_, err := s.app.WithDocumentReviewService(runtime.workspace, func(service *documentreview.Service, _ *book.Service) error {
			_, consumeErr := service.ConsumeReviewComments(context.Background(), req.ResolvedReviewFeedback.ReviewThreadID, req.ReviewFeedback.CommentIDs)
			return consumeErr
		})
		return err
	}
	if runtime.sess == nil || strings.TrimSpace(runtime.sess.ID) == "" {
		return invalidReviewFeedbackError("the active session identity is unavailable", nil)
	}
	return s.app.WithWorkspaceChangeService(runtime.workspace, func(service *workspacechange.Service) error {
		_, err := service.ConsumeReviewComments(context.Background(), req.ResolvedReviewFeedback.ReviewThreadID, runtime.sess.ID, req.ReviewFeedback.CommentIDs)
		return err
	})
}

func normalizeReviewFeedbackCommentIDs(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func invalidReviewFeedbackError(message string, details map[string]any) error {
	return &workspacechange.Error{
		Code:    workspacechange.ErrorCodeInvalidEdit,
		Message: fmt.Sprintf("invalid review feedback: %s", message),
		Details: details,
	}
}
