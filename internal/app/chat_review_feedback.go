package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"denova/internal/agent"
	"denova/internal/documentreview"
	"denova/internal/workspacechange"
)

const maxReviewFeedbackCommentIDs = 256

// resolveReviewFeedback replaces client-supplied IDs with trusted comments
// from the canonical service for the captured workspace. Comment bodies never
// cross the HTTP boundary into ChatRequest.
func (s *ChatAppService) resolveReviewFeedback(ctx context.Context, runtime ideChatRuntime, req *agent.ChatRequest) error {
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
	scope := reviewFeedbackServiceScope{}
	for _, ref := range refs {
		totalComments += len(ref.CommentIDs)
		switch ref.Source {
		case agent.ReviewFeedbackSourceWorkspaceChange:
			scope.workspaceChanges = true
			if runtime.sess == nil || strings.TrimSpace(runtime.sess.ID) == "" {
				return invalidReviewFeedbackError("the active session identity is unavailable", nil)
			}
		case agent.ReviewFeedbackSourceDocument:
			scope.documents = true
		}
	}
	if totalComments > maxReviewFeedbackCommentIDs {
		return invalidReviewFeedbackError("too many review comments were referenced", map[string]any{
			"maximum": maxReviewFeedbackCommentIDs,
			"actual":  totalComments,
		})
	}

	resolved := make(agent.ReviewFeedbackContexts, 0, len(refs))
	err = s.app.withReviewFeedbackServices(runtime.workspace, scope, func(changes *workspacechange.Service, documents *documentreview.Service) error {
		resolvers := newReviewFeedbackResolvers(changes, documents, s.app.bookService)
		for _, ref := range refs {
			resolver := resolvers[ref.Source]
			if resolver == nil {
				return invalidReviewFeedbackError("review feedback source is invalid", map[string]any{"source": ref.Source})
			}
			feedback := agent.ReviewFeedbackContext{
				Source:         ref.Source,
				ReviewThreadID: ref.ReviewThreadID,
				Comments:       make([]agent.ReviewFeedbackComment, 0, len(ref.CommentIDs)),
			}
			if err := resolver.Resolve(ctx, runtime, ref.ReviewThreadID, ref.CommentIDs, &feedback); err != nil {
				return err
			}
			resolved = append(resolved, feedback)
		}
		return nil
	})
	if err != nil {
		return err
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

func (s *ChatAppService) consumeResolvedReviewFeedback(ctx context.Context, runtime ideChatRuntime, req agent.ChatRequest) error {
	if req.ResolvedReviewFeedback.Empty() {
		return nil
	}
	consumptions := make([]reviewFeedbackConsumption, 0, len(req.ResolvedReviewFeedback))
	scope := reviewFeedbackServiceScope{}
	sessionID := ""
	for _, feedback := range req.ResolvedReviewFeedback {
		commentIDs := reviewFeedbackCommentIDs(req.ReviewFeedback, feedback)
		if len(commentIDs) == 0 {
			return invalidReviewFeedbackError("resolved review feedback lost its comment references", map[string]any{"review_thread_id": feedback.ReviewThreadID})
		}
		source, _ := agent.NormalizeReviewFeedbackSource(feedback.Source)
		switch source {
		case agent.ReviewFeedbackSourceDocument:
			scope.documents = true
		case agent.ReviewFeedbackSourceWorkspaceChange:
			scope.workspaceChanges = true
		}
		consumptions = append(consumptions, reviewFeedbackConsumption{
			source: source, threadID: feedback.ReviewThreadID, commentIDs: commentIDs,
		})
	}
	if scope.workspaceChanges {
		if runtime.sess == nil || strings.TrimSpace(runtime.sess.ID) == "" {
			return invalidReviewFeedbackError("the active session identity is unavailable", nil)
		}
		sessionID = strings.TrimSpace(runtime.sess.ID)
	}

	// Use a cancel-detached context: ledger writes here are durable side effects
	// of a user message that has already crossed the conversation boundary, so
	// they must not be aborted by a client disconnect, while still carrying
	// trace values.
	ctx = context.WithoutCancel(ctx)
	return s.app.withReviewFeedbackServices(runtime.workspace, scope, func(changes *workspacechange.Service, documents *documentreview.Service) error {
		resolvers := newReviewFeedbackResolvers(changes, documents, s.app.bookService)

		// Validate every ledger before the first append. Domain services validate
		// again while consuming to protect against concurrent mutations.
		for _, consumption := range consumptions {
			if err := resolvers[consumption.source].Validate(ctx, sessionID, consumption.threadID, consumption.commentIDs); err != nil {
				return err
			}
		}

		applied := make([]reviewFeedbackConsumption, 0, len(consumptions))
		for _, consumption := range consumptions {
			consumed, err := resolvers[consumption.source].Consume(ctx, sessionID, consumption.threadID, consumption.commentIDs)
			if err == nil {
				applied = append(applied, consumed)
				continue
			}
			rollbackErr := rollbackReviewFeedbackConsumptions(ctx, resolvers, sessionID, applied)
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
