package app

import (
	"context"
	"errors"
	"fmt"

	"denova/internal/agent"
	"denova/internal/book"
	"denova/internal/documentreview"
	"denova/internal/workspacechange"
)

// reviewFeedbackResolver abstracts one review-feedback source (workspace-change
// diffs, author document comments, ...) behind a single seam. The orchestration
// code in ChatAppService only looks up a resolver by source and delegates; it no
// longer switches on source strings in three places (resolve/consume/rollback).
//
// Resolvers operate on already-opened ledger services passed by the caller, so
// they never open their own store handles. Adding a new source means implementing
// this interface and registering it in newReviewFeedbackResolvers, instead of
// editing N switch statements.
type reviewFeedbackResolver interface {
	Resolve(ctx context.Context, runtime ideChatRuntime, threadID string, commentIDs []string, feedback *agent.ReviewFeedbackContext) error
	Validate(ctx context.Context, sessionID, threadID string, commentIDs []string) error
	Consume(ctx context.Context, sessionID, threadID string, commentIDs []string) (reviewFeedbackConsumption, error)
	Restore(ctx context.Context, sessionID, threadID string, consumption reviewFeedbackConsumption) error
}

// reviewFeedbackResolvers is the source → resolver registry. It is built once
// per call and reused within a batch; resolvers are stateless.
type reviewFeedbackResolvers map[string]reviewFeedbackResolver

func newReviewFeedbackResolvers(changes *workspacechange.Service, documents *documentreview.Service, files *book.Service) reviewFeedbackResolvers {
	return reviewFeedbackResolvers{
		agent.ReviewFeedbackSourceWorkspaceChange: workspaceChangeReviewFeedbackResolver{changes: changes},
		agent.ReviewFeedbackSourceDocument:        documentReviewFeedbackResolver{documents: documents, files: files},
	}
}

// reviewFeedbackConsumption captures the outcome of consuming one feedback
// batch so a later failure in the same cross-ledger write can be compensated.
type reviewFeedbackConsumption struct {
	source            string
	threadID          string
	commentIDs        []string
	workspaceComments []workspacechange.Comment
	documentComments  []documentreview.Comment
}

// workspaceChangeReviewFeedbackResolver backs review feedback sourced from
// agent-proposed workspace changes.
type workspaceChangeReviewFeedbackResolver struct {
	changes *workspacechange.Service
}

func (r workspaceChangeReviewFeedbackResolver) Resolve(ctx context.Context, runtime ideChatRuntime, threadID string, commentIDs []string, feedback *agent.ReviewFeedbackContext) error {
	resolved, err := r.changes.GetReviewComments(ctx, threadID, runtime.sess.ID, commentIDs)
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

func (r workspaceChangeReviewFeedbackResolver) Validate(ctx context.Context, sessionID, threadID string, commentIDs []string) error {
	_, err := r.changes.GetReviewComments(ctx, threadID, sessionID, commentIDs)
	return err
}

func (r workspaceChangeReviewFeedbackResolver) Consume(ctx context.Context, sessionID, threadID string, commentIDs []string) (reviewFeedbackConsumption, error) {
	comments, err := r.changes.ConsumeReviewComments(ctx, threadID, sessionID, commentIDs)
	return reviewFeedbackConsumption{
		source: agent.ReviewFeedbackSourceWorkspaceChange, threadID: threadID, commentIDs: commentIDs,
		workspaceComments: comments,
	}, err
}

func (r workspaceChangeReviewFeedbackResolver) Restore(ctx context.Context, sessionID, threadID string, consumption reviewFeedbackConsumption) error {
	_, err := r.changes.RestoreConsumedReviewComments(ctx, threadID, sessionID, consumption.workspaceComments)
	return err
}

// documentReviewFeedbackResolver backs review feedback sourced from author
// document comments.
type documentReviewFeedbackResolver struct {
	documents *documentreview.Service
	files     *book.Service
}

func (r documentReviewFeedbackResolver) Resolve(ctx context.Context, runtime ideChatRuntime, threadID string, commentIDs []string, feedback *agent.ReviewFeedbackContext) error {
	comments, err := r.documents.GetReviewComments(ctx, threadID, commentIDs)
	if err != nil {
		return translateDocumentReviewError(err)
	}
	for _, comment := range comments {
		content, revision, readErr := r.files.ReadFileWithRevision(comment.Path)
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
}

func (r documentReviewFeedbackResolver) Validate(ctx context.Context, sessionID, threadID string, commentIDs []string) error {
	_, err := r.documents.GetReviewComments(ctx, threadID, commentIDs)
	return err
}

func (r documentReviewFeedbackResolver) Consume(ctx context.Context, sessionID, threadID string, commentIDs []string) (reviewFeedbackConsumption, error) {
	comments, err := r.documents.ConsumeReviewComments(ctx, threadID, commentIDs)
	return reviewFeedbackConsumption{
		source: agent.ReviewFeedbackSourceDocument, threadID: threadID, commentIDs: commentIDs,
		documentComments: comments,
	}, err
}

func (r documentReviewFeedbackResolver) Restore(ctx context.Context, sessionID, threadID string, consumption reviewFeedbackConsumption) error {
	_, err := r.documents.RestoreConsumedReviewComments(ctx, threadID, consumption.documentComments)
	return err
}

// translateDocumentReviewError maps a document-review error onto the shared
// coded-error surface used by the chat preparation path so callers keep a
// single error contract.
func translateDocumentReviewError(err error) error {
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

// rollbackReviewFeedbackConsumptions compensates a partially-consumed batch by
// restoring every applied consumption in reverse order, delegating to each
// source's resolver.
func rollbackReviewFeedbackConsumptions(
	ctx context.Context,
	resolvers reviewFeedbackResolvers,
	sessionID string,
	consumptions []reviewFeedbackConsumption,
) error {
	errs := make([]error, 0)
	for index := len(consumptions) - 1; index >= 0; index-- {
		consumption := consumptions[index]
		resolver := resolvers[consumption.source]
		if resolver == nil {
			errs = append(errs, fmt.Errorf("restore %s review thread %s: unknown review feedback source", consumption.source, consumption.threadID))
			continue
		}
		if err := resolver.Restore(ctx, sessionID, consumption.threadID, consumption); err != nil {
			errs = append(errs, fmt.Errorf("restore %s review thread %s: %w", consumption.source, consumption.threadID, err))
		}
	}
	return errors.Join(errs...)
}

// reviewFeedbackCommentIDs extracts the original client comment IDs for a
// resolved feedback context from the request refs.
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
