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
	"denova/internal/book"
	"denova/internal/documentreview"
	"denova/internal/session"
	"denova/internal/workspacechange"
	"denova/internal/workspacepath"
)

func TestDocumentReviewFeedbackResolvesCurrentAnchorAndConsumesAfterCommit(t *testing.T) {
	workspace := t.TempDir()
	path := "chapters/ch01.md"
	before := "Alpha target Omega\n"
	if err := os.MkdirAll(filepath.Join(workspace, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, filepath.FromSlash(path)), []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	reviews, err := documentreview.ForWorkspace(workspace)
	if err != nil {
		t.Fatal(err)
	}
	start := len("Alpha ")
	thread, comment, err := reviews.AddComment(context.Background(), documentreview.AddCommentRequest{
		Path: path,
		Body: "Make this image more specific.",
		Anchor: documentreview.Anchor{
			Kind: documentreview.AnchorKindTextRange, Encoding: documentreview.AnchorEncodingUTF8,
			Revision: workspacechange.Revision([]byte(before)), Start: start, End: start + len("target"), Quote: "target",
			Prefix: "Alpha ", Suffix: " Omega\n", DisplayQuote: "target", EditorFrom: 7, EditorTo: 13,
		},
	}, documentreview.Snapshot{Content: before, Revision: workspacechange.Revision([]byte(before))})
	if err != nil {
		t.Fatal(err)
	}
	after := "Intro\n" + before
	if err := os.WriteFile(filepath.Join(workspace, filepath.FromSlash(path)), []byte(after), 0o644); err != nil {
		t.Fatal(err)
	}

	application := &App{workspace: workspace, bookService: book.NewService(workspace)}
	chat := &ChatAppService{app: application}
	req := agent.ChatRequest{ReviewFeedback: agent.ReviewFeedbackRefs{{
		Source: agent.ReviewFeedbackSourceDocument, ReviewThreadID: thread.ID, CommentIDs: []string{comment.ID},
	}}}
	if err := chat.resolveReviewFeedback(context.Background(), ideChatRuntime{workspace: workspace}, &req); err != nil {
		t.Fatal(err)
	}
	if len(req.ResolvedReviewFeedback) != 1 || req.ResolvedReviewFeedback[0].Source != agent.ReviewFeedbackSourceDocument || len(req.ResolvedReviewFeedback[0].Comments) != 1 {
		t.Fatalf("resolved document feedback = %#v", req.ResolvedReviewFeedback)
	}
	resolved := req.ResolvedReviewFeedback[0].Comments[0]
	if resolved.Path != path || resolved.Body != comment.Body || resolved.Anchor.Revision != workspacechange.Revision([]byte(after)) || resolved.Anchor.Start != len("Intro\nAlpha ") {
		t.Fatalf("document anchor was not projected from the canonical file: %#v", resolved)
	}
	if err := chat.consumeResolvedReviewFeedback(context.Background(), ideChatRuntime{workspace: workspace}, req); err != nil {
		t.Fatal(err)
	}
	pending, err := reviews.CurrentThread(context.Background())
	if err != nil || pending.ID != "" || len(pending.Comments) != 0 {
		t.Fatalf("document feedback remained pending after commit: %#v err=%v", pending, err)
	}
}

func TestReviewFeedbackResolvesAndConsumesDocumentAndDiffSelectionsTogether(t *testing.T) {
	workspace := t.TempDir()
	changePath := "chapters/change.md"
	documentPath := "chapters/document.md"
	if err := os.MkdirAll(filepath.Join(workspace, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, filepath.FromSlash(changePath)), []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	documentContent := "Alpha target Omega\n"
	if err := os.WriteFile(filepath.Join(workspace, filepath.FromSlash(documentPath)), []byte(documentContent), 0o644); err != nil {
		t.Fatal(err)
	}

	changes, err := workspacechange.ForWorkspace(workspace)
	if err != nil {
		t.Fatal(err)
	}
	change, err := changes.ReplaceFile(context.Background(), workspacechange.ReplaceFileRequest{
		Path: changePath, Content: "after", BaseRevision: workspacechange.Revision([]byte("before")),
		Metadata: workspacechange.ChangeMetadata{
			Origin: workspacechange.OriginAgent, ChangeGroupID: "group-1", ReviewThreadID: "diff-thread", SessionID: "session-1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	diffComment, err := changes.AddComment(context.Background(), workspacechange.AddCommentRequest{
		GroupID: "group-1", ChangeSetID: change.ID, Body: "Clarify the diff transition.",
		Anchor: workspacechange.CommentAnchor{
			Side: workspacechange.CommentAnchorSideAfter, Encoding: workspacechange.CommentAnchorEncodingUTF8Byte,
			Revision: change.Revision, Start: 0, End: len("after"), Quote: "after",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	documents, err := documentreview.ForWorkspace(workspace)
	if err != nil {
		t.Fatal(err)
	}
	documentStart := len("Alpha ")
	documentThread, documentComment, err := documents.AddComment(context.Background(), documentreview.AddCommentRequest{
		Path: documentPath, Body: "Make the document image more specific.",
		Anchor: documentreview.Anchor{
			Kind: documentreview.AnchorKindTextRange, Encoding: documentreview.AnchorEncodingUTF8,
			Revision: workspacechange.Revision([]byte(documentContent)), Start: documentStart, End: documentStart + len("target"),
			Quote: "target", Prefix: "Alpha ", Suffix: " Omega\n", DisplayQuote: "target",
		},
	}, documentreview.Snapshot{Content: documentContent, Revision: workspacechange.Revision([]byte(documentContent))})
	if err != nil {
		t.Fatal(err)
	}

	application := &App{workspace: workspace, bookService: book.NewService(workspace)}
	chat := &ChatAppService{app: application}
	runtime := ideChatRuntime{workspace: workspace, sess: &session.Session{ID: "session-1"}}
	req := agent.ChatRequest{ReviewFeedback: agent.ReviewFeedbackRefs{
		{Source: agent.ReviewFeedbackSourceWorkspaceChange, ReviewThreadID: "diff-thread", CommentIDs: []string{diffComment.ID}},
		{Source: agent.ReviewFeedbackSourceDocument, ReviewThreadID: documentThread.ID, CommentIDs: []string{documentComment.ID}},
	}}
	if err := chat.resolveReviewFeedback(context.Background(), runtime, &req); err != nil {
		t.Fatal(err)
	}
	if len(req.ResolvedReviewFeedback) != 2 || req.ResolvedReviewFeedback.CommentCount() != 2 {
		t.Fatalf("resolved feedback = %#v", req.ResolvedReviewFeedback)
	}
	if got := req.ResolvedReviewFeedback.PrimaryReviewThreadID(); got != "diff-thread" {
		t.Fatalf("primary review thread = %q", got)
	}
	if err := chat.consumeResolvedReviewFeedback(context.Background(), runtime, req); err != nil {
		t.Fatal(err)
	}
	group, err := changes.GetGroup(context.Background(), "group-1")
	if err != nil || len(group.Comments) != 1 || !group.Comments[0].Deleted {
		t.Fatalf("diff feedback was not consumed: group=%#v err=%v", group, err)
	}
	pendingDocuments, err := documents.CurrentThread(context.Background())
	if err != nil || pendingDocuments.ID != "" || len(pendingDocuments.Comments) != 0 {
		t.Fatalf("document feedback was not consumed: thread=%#v err=%v", pendingDocuments, err)
	}
}

func TestMixedReviewFeedbackRestoresEarlierConsumptionWhenLaterLedgerWriteFails(t *testing.T) {
	tests := []struct {
		name          string
		order         []string
		failingLedger string
	}{
		{
			name:          "restore diff feedback after document ledger failure",
			order:         []string{agent.ReviewFeedbackSourceWorkspaceChange, agent.ReviewFeedbackSourceDocument},
			failingLedger: "reviews",
		},
		{
			name:          "restore document feedback after diff ledger failure",
			order:         []string{agent.ReviewFeedbackSourceDocument, agent.ReviewFeedbackSourceWorkspaceChange},
			failingLedger: "changes",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertMixedReviewFeedbackRollback(t, test.order, test.failingLedger)
		})
	}
}

func assertMixedReviewFeedbackRollback(t *testing.T, order []string, failingLedger string) {
	t.Helper()
	workspace := t.TempDir()
	changePath := "chapters/change.md"
	documentPath := "chapters/document.md"
	if err := os.MkdirAll(filepath.Join(workspace, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, filepath.FromSlash(changePath)), []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	documentContent := "Alpha target Omega\n"
	if err := os.WriteFile(filepath.Join(workspace, filepath.FromSlash(documentPath)), []byte(documentContent), 0o644); err != nil {
		t.Fatal(err)
	}

	changes, err := workspacechange.ForWorkspace(workspace)
	if err != nil {
		t.Fatal(err)
	}
	change, err := changes.ReplaceFile(context.Background(), workspacechange.ReplaceFileRequest{
		Path: changePath, Content: "after", BaseRevision: workspacechange.Revision([]byte("before")),
		Metadata: workspacechange.ChangeMetadata{
			Origin: workspacechange.OriginAgent, ChangeGroupID: "group-rollback", ReviewThreadID: "diff-rollback", SessionID: "session-rollback",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	diffComment, err := changes.AddComment(context.Background(), workspacechange.AddCommentRequest{
		GroupID: change.GroupID, ChangeSetID: change.ID, Body: "Keep this diff comment pending on failure.",
	})
	if err != nil {
		t.Fatal(err)
	}

	documents, err := documentreview.ForWorkspace(workspace)
	if err != nil {
		t.Fatal(err)
	}
	documentThread, documentComment, err := documents.AddComment(context.Background(), documentreview.AddCommentRequest{
		Path: documentPath, Body: "Keep this document comment pending on failure.",
		Anchor: documentreview.Anchor{
			Kind: documentreview.AnchorKindTextRange, Encoding: documentreview.AnchorEncodingUTF8,
			Revision: workspacechange.Revision([]byte(documentContent)), Start: len("Alpha "), End: len("Alpha target"),
			Quote: "target", Prefix: "Alpha ", Suffix: " Omega\n", DisplayQuote: "target",
		},
	}, documentreview.Snapshot{Content: documentContent, Revision: workspacechange.Revision([]byte(documentContent))})
	if err != nil {
		t.Fatal(err)
	}

	application := &App{workspace: workspace, bookService: book.NewService(workspace)}
	chat := &ChatAppService{app: application}
	runtime := ideChatRuntime{workspace: workspace, sess: &session.Session{ID: "session-rollback"}}
	refs := make(agent.ReviewFeedbackRefs, 0, len(order))
	for _, source := range order {
		switch source {
		case agent.ReviewFeedbackSourceDocument:
			refs = append(refs, agent.ReviewFeedbackRef{Source: source, ReviewThreadID: documentThread.ID, CommentIDs: []string{documentComment.ID}})
		case agent.ReviewFeedbackSourceWorkspaceChange:
			refs = append(refs, agent.ReviewFeedbackRef{Source: source, ReviewThreadID: "diff-rollback", CommentIDs: []string{diffComment.ID}})
		default:
			t.Fatalf("unsupported test feedback source: %s", source)
		}
	}
	req := agent.ChatRequest{ReviewFeedback: refs}
	if err := chat.resolveReviewFeedback(context.Background(), runtime, &req); err != nil {
		t.Fatal(err)
	}

	ledgerPath := filepath.Join(workspacepath.Path(workspace, failingLedger), "ledger.jsonl")
	backupPath := ledgerPath + ".test-backup"
	if err := os.Rename(ledgerPath, backupPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(ledgerPath, 0o700); err != nil {
		t.Fatal(err)
	}
	consumeErr := chat.consumeResolvedReviewFeedback(context.Background(), runtime, req)
	if err := os.Remove(ledgerPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backupPath, ledgerPath); err != nil {
		t.Fatal(err)
	}
	if consumeErr == nil {
		t.Fatalf("mixed feedback consumption unexpectedly succeeded with an unwritable %s ledger", failingLedger)
	}

	group, err := changes.GetGroup(context.Background(), change.GroupID)
	if err != nil {
		t.Fatal(err)
	}
	if len(group.Comments) != 1 || group.Comments[0].Deleted {
		t.Fatalf("earlier diff consumption was not restored: %#v", group.Comments)
	}
	pendingDocuments, err := documents.CurrentThread(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if pendingDocuments.ID != documentThread.ID || len(pendingDocuments.Comments) != 1 || pendingDocuments.Comments[0].Deleted {
		t.Fatalf("document feedback changed after failed batch: %#v", pendingDocuments)
	}
}

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
		ReviewFeedback: agent.ReviewFeedbackRefs{{
			ReviewThreadID: "thread-1",
			CommentIDs:     []string{comment.ID},
		}},
	}
	if err := chat.resolveReviewFeedback(context.Background(), runtime, &req); err != nil {
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
			ReviewThreadID: req.ResolvedReviewFeedback.PrimaryReviewThreadID(),
			Workspace:      workspace,
			OnUserMessageCommitted: func(ctx context.Context) error {
				history := sess.History()
				if len(history) != 1 || len(history[0].UserReferences) != 1 {
					return errors.New("review reference was not durable before comment consumption")
				}
				callbackSawDurableReference = history[0].UserReferences[0].ID == comment.ID
				return chat.consumeResolvedReviewFeedback(ctx, runtime, req)
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
	if len(thread.Comments) != 1 || !thread.Comments[0].Deleted || thread.CommentCount != 0 {
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
	req := agent.ChatRequest{ReviewFeedback: agent.ReviewFeedbackRefs{{
		ReviewThreadID: " thread-1 ", CommentIDs: []string{" " + comment.ID, comment.ID},
	}}}
	if err := chat.resolveReviewFeedback(context.Background(), ideChatRuntime{workspace: workspace, sess: &session.Session{ID: "session-1"}}, &req); err != nil {
		t.Fatal(err)
	}
	if len(req.ReviewFeedback) != 1 || len(req.ReviewFeedback[0].CommentIDs) != 1 || req.ReviewFeedback[0].CommentIDs[0] != comment.ID {
		t.Fatalf("request IDs were not normalized: %#v", req.ReviewFeedback)
	}
	if len(req.ResolvedReviewFeedback) != 1 || len(req.ResolvedReviewFeedback[0].Comments) != 1 {
		t.Fatalf("resolved feedback = %#v", req.ResolvedReviewFeedback)
	}
	resolved := req.ResolvedReviewFeedback[0].Comments[0]
	if resolved.Body != comment.Body || resolved.Path != path || resolved.Anchor.Revision != change.Revision || resolved.Anchor.Side != workspacechange.CommentAnchorSideAfter || resolved.Anchor.Encoding != workspacechange.CommentAnchorEncodingUTF8Byte {
		t.Fatalf("resolved comment did not come from the ledger: %#v", resolved)
	}
	if err := chat.consumeResolvedReviewFeedback(context.Background(), ideChatRuntime{workspace: workspace, sess: &session.Session{ID: "session-1"}}, req); err != nil {
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

	crossSession := agent.ChatRequest{ReviewFeedback: agent.ReviewFeedbackRefs{{
		ReviewThreadID: "thread-1", CommentIDs: []string{comment.ID},
	}}}
	var crossSessionErr *workspacechange.Error
	if err := chat.resolveReviewFeedback(context.Background(), ideChatRuntime{workspace: workspace, sess: &session.Session{ID: "session-2"}}, &crossSession); !errors.As(err, &crossSessionErr) || crossSessionErr.Code != workspacechange.ErrorCodeConflict {
		t.Fatalf("cross-session feedback error=%v", err)
	}
}

func TestResolveReviewFeedbackRejectsForgedThreadAndStaleWorkspace(t *testing.T) {
	workspace := t.TempDir()
	application := &App{workspace: workspace}
	chat := &ChatAppService{app: application}

	req := agent.ChatRequest{ReviewFeedback: agent.ReviewFeedbackRefs{{ReviewThreadID: "missing", CommentIDs: []string{"forged"}}}}
	var changeErr *workspacechange.Error
	if err := chat.resolveReviewFeedback(context.Background(), ideChatRuntime{workspace: workspace, sess: &session.Session{ID: "session-1"}}, &req); !errors.As(err, &changeErr) || changeErr.Code != workspacechange.ErrorCodeNotFound {
		t.Fatalf("forged feedback error=%v", err)
	}

	application.workspace = t.TempDir()
	err := chat.resolveReviewFeedback(context.Background(), ideChatRuntime{workspace: workspace, sess: &session.Session{ID: "session-1"}}, &req)
	if !errors.Is(err, ErrWorkspaceChanged) {
		t.Fatalf("stale workspace error=%v", err)
	}
}
