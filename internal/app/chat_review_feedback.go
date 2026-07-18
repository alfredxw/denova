package app

import (
	"context"
	"errors"
	"fmt"
	"log"
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
	refs, err := normalizeReviewFeedbackRefs(req.ReviewFeedback)
	if err != nil {
		return err
	}
	if len(refs) == 0 {
		req.ReviewFeedback = nil
		req.ResolvedReviewFeedback = nil
		return nil
	}
	totalComments := 0
	for _, ref := range refs {
		totalComments += len(ref.CommentIDs)
		if ref.Source == agent.ReviewFeedbackSourceWorkspaceChange && (runtime.sess == nil || strings.TrimSpace(runtime.sess.ID) == "") {
			return invalidReviewFeedbackError("the active session identity is unavailable", nil)
		}
	}
	if totalComments > maxReviewFeedbackCommentIDs {
		return invalidReviewFeedbackError("too many review comments were referenced", map[string]any{
			"maximum": maxReviewFeedbackCommentIDs,
			"actual":  totalComments,
		})
	}

	resolved := make(agent.ReviewFeedbackContexts, 0, len(refs))
	for _, ref := range refs {
		feedback := agent.ReviewFeedbackContext{
			Source:         ref.Source,
			ReviewThreadID: ref.ReviewThreadID,
			Comments:       make([]agent.ReviewFeedbackComment, 0, len(ref.CommentIDs)),
		}
		if ref.Source == agent.ReviewFeedbackSourceDocument {
			if err := s.resolveDocumentReviewFeedback(runtime, ref.ReviewThreadID, ref.CommentIDs, &feedback); err != nil {
				return err
			}
		} else if err := s.resolveWorkspaceChangeReviewFeedback(runtime, ref.ReviewThreadID, ref.CommentIDs, &feedback); err != nil {
			return err
		}
		resolved = append(resolved, feedback)
	}
	if resolved.EncodedSize() > agent.MaxReviewFeedbackContextBytes {
		return invalidReviewFeedbackError("review feedback context exceeds the allowed size", map[string]any{
			"maximum_bytes": agent.MaxReviewFeedbackContextBytes,
		})
	}
	req.ReviewFeedback = refs
	req.ResolvedReviewFeedback = resolved
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
	consumptions := make([]reviewFeedbackConsumption, 0, len(req.ResolvedReviewFeedback))
	scope := reviewFeedbackServiceScope{}
	for _, feedback := range req.ResolvedReviewFeedback {
		commentIDs := reviewFeedbackCommentIDs(req.ReviewFeedback, feedback)
		if len(commentIDs) == 0 {
			return invalidReviewFeedbackError("resolved review feedback lost its comment references", map[string]any{"review_thread_id": feedback.ReviewThreadID})
		}
		switch feedback.Source {
		case agent.ReviewFeedbackSourceDocument:
			scope.documents = true
		case agent.ReviewFeedbackSourceWorkspaceChange:
			scope.workspaceChanges = true
		default:
			return invalidReviewFeedbackError("resolved review feedback source is invalid", map[string]any{"source": feedback.Source})
		}
		consumptions = append(consumptions, reviewFeedbackConsumption{
			source: feedback.Source, threadID: feedback.ReviewThreadID, commentIDs: commentIDs,
		})
	}
	sessionID := ""
	if runtime.sess != nil {
		sessionID = strings.TrimSpace(runtime.sess.ID)
	}
	if scope.workspaceChanges && sessionID == "" {
		return invalidReviewFeedbackError("the active session identity is unavailable", nil)
	}

	ctx := context.Background()
	return s.app.withReviewFeedbackServices(runtime.workspace, scope, func(changes *workspacechange.Service, documents *documentreview.Service) error {
		// Validate every ledger before the first append. Domain services validate
		// again while consuming to protect against concurrent mutations.
		for _, consumption := range consumptions {
			var err error
			switch consumption.source {
			case agent.ReviewFeedbackSourceDocument:
				_, err = documents.GetReviewComments(ctx, consumption.threadID, consumption.commentIDs)
			case agent.ReviewFeedbackSourceWorkspaceChange:
				_, err = changes.GetReviewComments(ctx, consumption.threadID, sessionID, consumption.commentIDs)
			}
			if err != nil {
				return err
			}
		}

		applied := make([]reviewFeedbackConsumption, 0, len(consumptions))
		for _, consumption := range consumptions {
			var err error
			switch consumption.source {
			case agent.ReviewFeedbackSourceDocument:
				consumption.documentComments, err = documents.ConsumeReviewComments(ctx, consumption.threadID, consumption.commentIDs)
			case agent.ReviewFeedbackSourceWorkspaceChange:
				consumption.workspaceComments, err = changes.ConsumeReviewComments(ctx, consumption.threadID, sessionID, consumption.commentIDs)
			}
			if err == nil {
				applied = append(applied, consumption)
				continue
			}
			rollbackErr := rollbackReviewFeedbackConsumptions(ctx, changes, documents, sessionID, applied)
			if rollbackErr != nil {
				log.Printf("[review-feedback] mixed batch compensation failed workspace=%q applied_batches=%d error=%v rollback_error=%v", runtime.workspace, len(applied), err, rollbackErr)
				return errors.Join(err, fmt.Errorf("restore partially consumed review feedback: %w", rollbackErr))
			}
			log.Printf("[review-feedback] mixed batch consumption rolled back workspace=%q applied_batches=%d error=%v", runtime.workspace, len(applied), err)
			return err
		}
		return nil
	})
}

type reviewFeedbackConsumption struct {
	source            string
	threadID          string
	commentIDs        []string
	workspaceComments []workspacechange.Comment
	documentComments  []documentreview.Comment
}

func rollbackReviewFeedbackConsumptions(
	ctx context.Context,
	changes *workspacechange.Service,
	documents *documentreview.Service,
	sessionID string,
	consumptions []reviewFeedbackConsumption,
) error {
	errs := make([]error, 0)
	for index := len(consumptions) - 1; index >= 0; index-- {
		consumption := consumptions[index]
		var err error
		switch consumption.source {
		case agent.ReviewFeedbackSourceDocument:
			_, err = documents.RestoreConsumedReviewComments(ctx, consumption.threadID, consumption.documentComments)
		case agent.ReviewFeedbackSourceWorkspaceChange:
			_, err = changes.RestoreConsumedReviewComments(ctx, consumption.threadID, sessionID, consumption.workspaceComments)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("restore %s review thread %s: %w", consumption.source, consumption.threadID, err))
		}
	}
	return errors.Join(errs...)
}

func normalizeReviewFeedbackRefs(values agent.ReviewFeedbackRefs) (agent.ReviewFeedbackRefs, error) {
	result := make(agent.ReviewFeedbackRefs, 0, len(values))
	indexByKey := make(map[string]int, len(values))
	for _, value := range values {
		threadID := strings.TrimSpace(value.ReviewThreadID)
		commentIDs := normalizeReviewFeedbackCommentIDs(value.CommentIDs)
		if threadID == "" && len(commentIDs) == 0 {
			continue
		}
		source, validSource := agent.NormalizeReviewFeedbackSource(value.Source)
		if !validSource {
			return nil, invalidReviewFeedbackError("review feedback source is invalid", map[string]any{"source": value.Source})
		}
		if threadID == "" || len(commentIDs) == 0 {
			return nil, invalidReviewFeedbackError("review_thread_id and comment_ids must be provided together", nil)
		}
		key := source + "\x00" + threadID
		if index, exists := indexByKey[key]; exists {
			result[index].CommentIDs = normalizeReviewFeedbackCommentIDs(append(result[index].CommentIDs, commentIDs...))
			continue
		}
		indexByKey[key] = len(result)
		result = append(result, agent.ReviewFeedbackRef{Source: source, ReviewThreadID: threadID, CommentIDs: commentIDs})
	}
	return result, nil
}

func reviewFeedbackCommentIDs(refs agent.ReviewFeedbackRefs, feedback agent.ReviewFeedbackContext) []string {
	wantedSource, _ := agent.NormalizeReviewFeedbackSource(feedback.Source)
	for _, ref := range refs {
		source, _ := agent.NormalizeReviewFeedbackSource(ref.Source)
		if source == wantedSource && ref.ReviewThreadID == feedback.ReviewThreadID {
			return ref.CommentIDs
		}
	}
	return nil
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
