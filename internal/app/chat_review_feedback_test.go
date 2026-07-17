package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"denova/internal/agent"
	"denova/internal/session"
	"denova/internal/workspacechange"
)

func TestCommittedReviewFeedbackPersistsWithUserMessageAndDisappearsAfterReload(t *testing.T) {
	workspace := t.TempDir()
	path := "chapters/ch01.md"
	if err := os.MkdirAll(filepath.Join(workspace, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, filepath.FromSlash(path)), []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	changeService, err := workspacechange.ForWorkspace(workspace)
	if err != nil {
		t.Fatal(err)
	}
	change, err := changeService.ReplaceFile(context.Background(), workspacechange.ReplaceFileRequest{
		Path: path, Content: "after", BaseRevision: workspacechange.Revision([]byte("before")),
		Metadata: workspacechange.ChangeMetadata{
			Origin: workspacechange.OriginAgent, ChangeGroupID: "group-1", ReviewThreadID: "thread-1", SessionID: "session-1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	comment, err := changeService.AddComment(context.Background(), workspacechange.AddCommentRequest{
		GroupID: "group-1", ChangeSetID: change.ID, Body: "Clarify the transition.",
		Anchor: workspacechange.CommentAnchor{Side: workspacechange.CommentAnchorSideAfter, Encoding: workspacechange.CommentAnchorEncodingUTF8Byte, Revision: change.Revision, Start: 2, End: 5, Quote: "ter"},
	})
	if err != nil {
		t.Fatal(err)
	}

	sessionDir := filepath.Join(t.TempDir(), "sessions")
	sessionStore, err := session.NewStore(sessionDir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := sessionStore.GetOrCreate("session-1")
	if err != nil {
		t.Fatal(err)
	}
	application := &App{workspace: workspace}
	chat := &ChatAppService{app: application}
	runtime := ideChatRuntime{workspace: workspace, sess: sess}
	req := agent.ChatRequest{
		Message: "Please handle this review comment.",
		ReviewFeedback: agent.ReviewFeedbackRef{
			ReviewThreadID: "thread-1",
			CommentIDs:     []string{comment.ID},
		},
	}
	if err := chat.resolveReviewFeedback(runtime, &req); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	builtAgent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          "review-feedback-commit-test",
		Description:   "test",
		Instruction:   "test",
		Model:         &reviewFeedbackCommitChatModel{},
		MaxIterations: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: builtAgent, EnableStreaming: true})
	var callbackSawDurableReference bool
	agent.NewChatService().RunWithOptions(
		ctx,
		runner,
		agent.NewSessionConversation(sess),
		nil,
		req,
		agent.RunOptions{
			AgentKind:      agent.AgentKindIDE,
			SessionID:      sess.ID,
			ReviewThreadID: req.ResolvedReviewFeedback.ReviewThreadID,
			Workspace:      workspace,
			OnUserMessageCommitted: func(context.Context) error {
				history := sess.History()
				if len(history) != 1 || len(history[0].UserReferences) != 1 {
					return errors.New("review reference was not durable before comment consumption")
				}
				callbackSawDurableReference = history[0].UserReferences[0].ID == comment.ID
				return chat.consumeResolvedReviewFeedback(runtime, req)
			},
		},
		func(agent.Event) {},
	)
	if !callbackSawDurableReference {
		t.Fatal("comment consumption ran before the durable user-message reference was visible")
	}

	reloadedStore, err := session.NewStore(sessionDir)
	if err != nil {
		t.Fatal(err)
	}
	reloadedSession, err := reloadedStore.Get("session-1")
	if err != nil {
		t.Fatal(err)
	}
	history := reloadedSession.History()
	if len(history) != 2 || len(history[0].UserReferences) != 1 || history[1].Role != "assistant" {
		t.Fatalf("reloaded user message lost review references: %#v", history)
	}
	reference := history[0].UserReferences[0]
	if reference.Kind != "review_comment" || reference.ID != comment.ID || reference.Label != path || reference.Detail != comment.Body {
		t.Fatalf("reloaded review reference = %#v", reference)
	}

	reloadedChanges, err := workspacechange.NewService(workspace)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := reloadedChanges.GetReviewThread(context.Background(), "thread-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(thread.Comments) != 1 || !thread.Comments[0].Deleted || thread.UnresolvedCommentCount != 0 {
		t.Fatalf("submitted comment reappeared after ledger reload: %#v", thread.Comments)
	}
}

type reviewFeedbackCommitChatModel struct{}

func (*reviewFeedbackCommitChatModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return schema.AssistantMessage("Acknowledged.", nil), nil
}

func (*reviewFeedbackCommitChatModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray([]*schema.Message{schema.AssistantMessage("Acknowledged.", nil)}), nil
}

func TestResolveReviewFeedbackUsesCanonicalWorkspaceLedger(t *testing.T) {
	workspace := t.TempDir()
	path := "chapters/ch01.md"
	if err := os.MkdirAll(filepath.Join(workspace, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, filepath.FromSlash(path)), []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	service, err := workspacechange.ForWorkspace(workspace)
	if err != nil {
		t.Fatal(err)
	}
	change, err := service.ReplaceFile(context.Background(), workspacechange.ReplaceFileRequest{
		Path: path, Content: "after", BaseRevision: workspacechange.Revision([]byte("before")),
		Metadata: workspacechange.ChangeMetadata{
			Origin: workspacechange.OriginAgent, ChangeGroupID: "group-1", ReviewThreadID: "thread-1", SessionID: "session-1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	comment, err := service.AddComment(context.Background(), workspacechange.AddCommentRequest{
		GroupID: "group-1", ChangeSetID: change.ID, Body: "Clarify the transition.",
		Anchor: workspacechange.CommentAnchor{Side: workspacechange.CommentAnchorSideAfter, Encoding: workspacechange.CommentAnchorEncodingUTF8Byte, Revision: change.Revision, Start: 2, End: 5, Quote: "ter"},
	})
	if err != nil {
		t.Fatal(err)
	}

	application := &App{workspace: workspace}
	chat := &ChatAppService{app: application}
	req := agent.ChatRequest{ReviewFeedback: agent.ReviewFeedbackRef{
		ReviewThreadID: " thread-1 ", CommentIDs: []string{" " + comment.ID, comment.ID},
	}}
	if err := chat.resolveReviewFeedback(ideChatRuntime{workspace: workspace, sess: &session.Session{ID: "session-1"}}, &req); err != nil {
		t.Fatal(err)
	}
	if len(req.ReviewFeedback.CommentIDs) != 1 || req.ReviewFeedback.CommentIDs[0] != comment.ID {
		t.Fatalf("request IDs were not normalized: %#v", req.ReviewFeedback)
	}
	if len(req.ResolvedReviewFeedback.Comments) != 1 {
		t.Fatalf("resolved feedback = %#v", req.ResolvedReviewFeedback)
	}
	resolved := req.ResolvedReviewFeedback.Comments[0]
	if resolved.Body != comment.Body || resolved.Path != path || resolved.Anchor.Revision != change.Revision || resolved.Anchor.Side != workspacechange.CommentAnchorSideAfter || resolved.Anchor.Encoding != workspacechange.CommentAnchorEncodingUTF8Byte {
		t.Fatalf("resolved comment did not come from the ledger: %#v", resolved)
	}
	if err := chat.consumeResolvedReviewFeedback(ideChatRuntime{workspace: workspace, sess: &session.Session{ID: "session-1"}}, req); err != nil {
		t.Fatal(err)
	}
	group, err := service.GetGroup(context.Background(), "group-1")
	if err != nil || len(group.Comments) != 1 || !group.Comments[0].Deleted {
		t.Fatalf("submitted review comments were not consumed: group=%#v err=%v", group, err)
	}
	reloaded, err := workspacechange.NewService(workspace)
	if err != nil {
		t.Fatal(err)
	}
	reloadedGroup, err := reloaded.GetGroup(context.Background(), "group-1")
	if err != nil || len(reloadedGroup.Comments) != 1 || !reloadedGroup.Comments[0].Deleted {
		t.Fatalf("consumed review comments did not survive replay: group=%#v err=%v", reloadedGroup, err)
	}

	crossSession := agent.ChatRequest{ReviewFeedback: agent.ReviewFeedbackRef{
		ReviewThreadID: "thread-1", CommentIDs: []string{comment.ID},
	}}
	var crossSessionErr *workspacechange.Error
	if err := chat.resolveReviewFeedback(ideChatRuntime{workspace: workspace, sess: &session.Session{ID: "session-2"}}, &crossSession); !errors.As(err, &crossSessionErr) || crossSessionErr.Code != workspacechange.ErrorCodeConflict {
		t.Fatalf("cross-session feedback error=%v", err)
	}
}

func TestResolveReviewFeedbackRejectsForgedThreadAndStaleWorkspace(t *testing.T) {
	workspace := t.TempDir()
	application := &App{workspace: workspace}
	chat := &ChatAppService{app: application}

	req := agent.ChatRequest{ReviewFeedback: agent.ReviewFeedbackRef{ReviewThreadID: "missing", CommentIDs: []string{"forged"}}}
	var changeErr *workspacechange.Error
	if err := chat.resolveReviewFeedback(ideChatRuntime{workspace: workspace, sess: &session.Session{ID: "session-1"}}, &req); !errors.As(err, &changeErr) || changeErr.Code != workspacechange.ErrorCodeNotFound {
		t.Fatalf("forged feedback error=%v", err)
	}

	application.workspace = t.TempDir()
	err := chat.resolveReviewFeedback(ideChatRuntime{workspace: workspace, sess: &session.Session{ID: "session-1"}}, &req)
	if !errors.Is(err, ErrWorkspaceChanged) {
		t.Fatalf("stale workspace error=%v", err)
	}
}
