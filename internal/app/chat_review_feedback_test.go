package app

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	agentharness "denova/internal/agents/harness"
	"errors"
	"os"
	"path/filepath"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	agents "denova/internal/agents"
	agentreview "denova/internal/agents/review"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	"denova/internal/book"
	"denova/internal/book/lore"
	workspacelayout "denova/internal/workspace"
	workspacechange "denova/internal/workspace/change"
	"denova/internal/workspace/documentreview"
)

func bookReviewRuntime(workspace string, sess *session.Session) ideChatRuntime {
	return ideChatRuntime{
		agentKind: agentrun.AgentKindIDE, projectType: ProjectTypeBook,
		workspace: workspace, projectState: workspacelayout.Dir(workspace), state: book.NewState(workspace),
		bookService: book.NewService(workspace), sess: sess,
	}
}

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
		Target: documentreview.Target{Kind: documentreview.TargetKindWorkspaceFile, ID: path},
		Body:   "Make this image more specific.",
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
	runtime := bookReviewRuntime(workspace, nil)
	req := agentchat.ChatRequest{ReviewFeedback: agentreview.Refs{{
		Source: agentreview.SourceDocument, ReviewThreadID: thread.ID, CommentIDs: []string{comment.ID},
	}}}
	if err := chat.resolveReviewFeedback(context.Background(), runtime, &req); err != nil {
		t.Fatal(err)
	}
	if len(req.ResolvedReviewFeedback) != 1 || req.ResolvedReviewFeedback[0].Source != agentreview.SourceDocument || len(req.ResolvedReviewFeedback[0].Comments) != 1 {
		t.Fatalf("resolved document feedback = %#v", req.ResolvedReviewFeedback)
	}
	resolved := req.ResolvedReviewFeedback[0].Comments[0]
	if resolved.Target == nil || resolved.Target.Kind != documentreview.TargetKindWorkspaceFile || resolved.Target.ID != path || resolved.Body != comment.Body || resolved.Anchor.Revision != workspacechange.Revision([]byte(after)) || resolved.Anchor.Start != len("Intro\nAlpha ") {
		t.Fatalf("document anchor was not projected from the canonical file: %#v", resolved)
	}
	if err := chat.consumeResolvedReviewFeedback(context.Background(), runtime, req); err != nil {
		t.Fatal(err)
	}
	pending, err := reviews.CurrentThread(context.Background())
	if err != nil || pending.ID != "" || len(pending.Comments) != 0 {
		t.Fatalf("document feedback remained pending after commit: %#v err=%v", pending, err)
	}
}

func TestLoreReviewFeedbackResolvesStructuredTargetAndCurrentAnchor(t *testing.T) {
	workspace := t.TempDir()
	store := lore.NewStore(workspace)
	disabled := false
	item, err := store.Create(lore.ItemInput{
		ID: "hero", Enabled: &disabled, Type: "character", Name: "林川", Content: "谨慎的旅人。\n他害怕失去同伴。",
	})
	if err != nil {
		t.Fatal(err)
	}
	quote := "害怕失去同伴"
	start := len("谨慎的旅人。\n他")
	reviews, err := documentreview.ForWorkspace(workspace)
	if err != nil {
		t.Fatal(err)
	}
	thread, comment, err := reviews.AddComment(context.Background(), documentreview.AddCommentRequest{
		Target: documentreview.Target{Kind: documentreview.TargetKindLoreItem, ID: item.ID, Field: documentreview.TargetFieldLoreContent},
		Body:   "说明这种恐惧来自哪次事件。",
		Anchor: documentreview.Anchor{
			Kind: documentreview.AnchorKindTextRange, Encoding: documentreview.AnchorEncodingUTF8,
			Revision: item.UpdatedAt, Start: start, End: start + len(quote), Quote: quote,
			Prefix: "。\n他", Suffix: "。", DisplayQuote: quote,
		},
	}, documentreview.Snapshot{Content: item.Content, Revision: item.UpdatedAt})
	if err != nil {
		t.Fatal(err)
	}
	updatedContent := "角色底色：\n" + item.Content
	updated, err := store.Update(item.ID, lore.ItemInput{
		Type: item.Type, Name: item.Name, Content: updatedContent, BaseRevision: item.UpdatedAt,
	})
	if err != nil {
		t.Fatal(err)
	}

	application := &App{workspace: workspace, bookService: book.NewService(workspace)}
	chat := &ChatAppService{app: application}
	runtime := bookReviewRuntime(workspace, nil)
	req := agentchat.ChatRequest{ReviewFeedback: agentreview.Refs{{
		Source: agentreview.SourceDocument, ReviewThreadID: thread.ID, CommentIDs: []string{comment.ID},
	}}}
	if err := chat.resolveReviewFeedback(context.Background(), runtime, &req); err != nil {
		t.Fatal(err)
	}
	resolved := req.ResolvedReviewFeedback[0].Comments[0]
	if resolved.Target == nil || resolved.Target.Kind != documentreview.TargetKindLoreItem || resolved.Target.ID != item.ID || resolved.Target.Field != documentreview.TargetFieldLoreContent || resolved.Target.Name != item.Name {
		t.Fatalf("resolved lore target = %#v", resolved.Target)
	}
	if resolved.Target.Snapshot == nil || resolved.Target.Snapshot.Revision != updated.UpdatedAt || resolved.Target.Snapshot.Content != updatedContent {
		t.Fatalf("disabled lore target is missing its canonical review snapshot: %#v", resolved.Target)
	}
	if resolved.Anchor.Revision != updated.UpdatedAt || resolved.Anchor.Start != len("角色底色：\n")+start || resolved.Body != comment.Body {
		t.Fatalf("resolved lore feedback = %#v", resolved)
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

	stateRoot := workspacelayout.Dir(workspace)
	changes, err := workspacechange.ForWorkspaceAt(workspace, stateRoot)
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

	documents, err := documentreview.ForWorkspaceAt(workspace, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	documentStart := len("Alpha ")
	documentThread, documentComment, err := documents.AddComment(context.Background(), documentreview.AddCommentRequest{
		Target: documentreview.Target{Kind: documentreview.TargetKindWorkspaceFile, ID: documentPath}, Body: "Make the document image more specific.",
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
	runtime := bookReviewRuntime(workspace, &session.Session{ID: "session-1"})
	req := agentchat.ChatRequest{ReviewFeedback: agentreview.Refs{
		{Source: agentreview.SourceWorkspaceChange, ReviewThreadID: "diff-thread", CommentIDs: []string{diffComment.ID}},
		{Source: agentreview.SourceDocument, ReviewThreadID: documentThread.ID, CommentIDs: []string{documentComment.ID}},
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
			order:         []string{agentreview.SourceWorkspaceChange, agentreview.SourceDocument},
			failingLedger: "reviews",
		},
		{
			name:          "restore document feedback after diff ledger failure",
			order:         []string{agentreview.SourceDocument, agentreview.SourceWorkspaceChange},
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

	stateRoot := workspacelayout.Dir(workspace)
	changes, err := workspacechange.ForWorkspaceAt(workspace, stateRoot)
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

	documents, err := documentreview.ForWorkspaceAt(workspace, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	documentThread, documentComment, err := documents.AddComment(context.Background(), documentreview.AddCommentRequest{
		Target: documentreview.Target{Kind: documentreview.TargetKindWorkspaceFile, ID: documentPath}, Body: "Keep this document comment pending on failure.",
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
	runtime := bookReviewRuntime(workspace, &session.Session{ID: "session-rollback"})
	refs := make(agentreview.Refs, 0, len(order))
	for _, source := range order {
		switch source {
		case agentreview.SourceDocument:
			refs = append(refs, agentreview.Ref{Source: source, ReviewThreadID: documentThread.ID, CommentIDs: []string{documentComment.ID}})
		case agentreview.SourceWorkspaceChange:
			refs = append(refs, agentreview.Ref{Source: source, ReviewThreadID: "diff-rollback", CommentIDs: []string{diffComment.ID}})
		default:
			t.Fatalf("unsupported test feedback source: %s", source)
		}
	}
	req := agentchat.ChatRequest{ReviewFeedback: refs}
	if err := chat.resolveReviewFeedback(context.Background(), runtime, &req); err != nil {
		t.Fatal(err)
	}

	ledgerPath := filepath.Join(workspacelayout.Path(workspace, failingLedger), "ledger.jsonl")
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
	req := agentchat.ChatRequest{
		CommandID: "review-feedback-commit", Message: "Please handle this review comment.",
		ReviewFeedback: agentreview.Refs{{
			ReviewThreadID: "thread-1",
			CommentIDs:     []string{comment.ID},
		}},
	}
	acceptedRequest := agentchat.CaptureChatRequestCallerInput(req)
	if err := chat.resolveReviewFeedback(context.Background(), runtime, &req); err != nil {
		t.Fatal(err)
	}
	identity := agentrun.CycleIdentity{CommandID: "review-feedback-commit", OperationID: "review-feedback-operation", Cycle: 1}
	plan, err := application.PlanHarnessInputMaterialization(context.Background(), agentharness.InputMaterializationRequest{
		Binding:  agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindIDE, Workspace: workspace, SessionID: sess.ID},
		Identity: identity, AgentKind: agentrun.AgentKindIDE,
		Message: req.Message, Request: acceptedRequest,
	})
	if err != nil {
		t.Fatal(err)
	}
	normalIntent, err := session.NewDomainCommitIntent(session.DomainCommitIdentity{
		CommandID: string(identity.CommandID), OperationID: string(identity.OperationID), Cycle: identity.Cycle,
	}, agents.UserMessage(req.Message), session.MessageMetadata{
		AgentKind: agentrun.AgentKindIDE, UserReferences: agentchat.UserMessageReferencesForRequest(req),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Required || plan.Hash != normalIntent.Hash {
		t.Fatalf("provider-free review input hash = %#v, normal hash = %q", plan, normalIntent.Hash)
	}

	ctx := context.Background()
	builtAgent, err := agent.NewAgent(ctx, agent.AgentConfig{
		Name:          "review-feedback-commit-test",
		Description:   "test",
		Instruction:   "test",
		Model:         &reviewFeedbackCommitChatModel{},
		MaxIterations: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := agent.NewRunner(agent.RunnerConfig{Agent: builtAgent, EnableStreaming: true})
	var callbackSawDurableReference bool
	var emittedEventTypes []string
	options := chat.bindReviewFeedbackInputCommit(agentrun.Options{
		AgentKind: agentrun.AgentKindIDE,
		SessionID: sess.ID,
		Workspace: workspace,
	}, runtime, req)
	consumeFeedback := options.OnUserMessageCommitted
	options.OnUserMessageCommitted = func(ctx context.Context) error {
		history := sess.History()
		if len(history) != 1 || len(history[0].UserReferences) != 1 {
			return errors.New("review reference was not durable before comment consumption")
		}
		callbackSawDurableReference = history[0].UserReferences[0].ID == comment.ID
		return consumeFeedback(ctx)
	}
	agentharness.NewEphemeralService().RunWithOptions(
		ctx,
		runner,
		agentconversation.NewSessionConversation(sess),
		nil,
		req,
		options,
		func(event agentrun.Event) {
			emittedEventTypes = append(emittedEventTypes, event.Type)
		},
	)
	if !callbackSawDurableReference {
		t.Fatal("comment consumption ran before the durable user-message reference was visible")
	}
	workspaceChangeIndex, doneIndex := -1, -1
	for index, eventType := range emittedEventTypes {
		switch eventType {
		case "workspace_change":
			if workspaceChangeIndex < 0 {
				workspaceChangeIndex = index
			}
		case "done":
			if doneIndex < 0 {
				doneIndex = index
			}
		}
	}
	if workspaceChangeIndex < 0 || doneIndex < 0 || workspaceChangeIndex >= doneIndex {
		t.Fatalf("review consumption event must precede terminal done: %v", emittedEventTypes)
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

func (*reviewFeedbackCommitChatModel) Generate(context.Context, []*agents.Message, ...agent.ModelOption) (*agents.Message, error) {
	return agents.AssistantMessage("Acknowledged.", nil), nil
}

func (*reviewFeedbackCommitChatModel) Stream(context.Context, []*agents.Message, ...agent.ModelOption) (*agents.StreamReader[*agents.Message], error) {
	return agents.StreamReaderFromArray([]*agents.Message{agents.AssistantMessage("Acknowledged.", nil)}), nil
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
	req := agentchat.ChatRequest{ReviewFeedback: agentreview.Refs{{
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

	crossSession := agentchat.ChatRequest{ReviewFeedback: agentreview.Refs{{
		ReviewThreadID: "thread-1", CommentIDs: []string{comment.ID},
	}}}
	var crossSessionErr *workspacechange.Error
	if err := chat.resolveReviewFeedback(context.Background(), ideChatRuntime{workspace: workspace, sess: &session.Session{ID: "session-2"}}, &crossSession); !errors.As(err, &crossSessionErr) || crossSessionErr.Code != workspacechange.ErrorCodeConflict {
		t.Fatalf("cross-session feedback error=%v", err)
	}
}

func TestResolveReviewFeedbackRejectsForgedThreadWithinCapturedRuntime(t *testing.T) {
	workspace := t.TempDir()
	application := &App{workspace: workspace}
	chat := &ChatAppService{app: application}

	req := agentchat.ChatRequest{ReviewFeedback: agentreview.Refs{{ReviewThreadID: "missing", CommentIDs: []string{"forged"}}}}
	var changeErr *workspacechange.Error
	if err := chat.resolveReviewFeedback(context.Background(), ideChatRuntime{workspace: workspace, sess: &session.Session{ID: "session-1"}}, &req); !errors.As(err, &changeErr) || changeErr.Code != workspacechange.ErrorCodeNotFound {
		t.Fatalf("forged feedback error=%v", err)
	}

	application.workspace = t.TempDir()
	err := chat.resolveReviewFeedback(context.Background(), ideChatRuntime{workspace: workspace, sess: &session.Session{ID: "session-1"}}, &req)
	if !errors.As(err, &changeErr) || changeErr.Code != workspacechange.ErrorCodeNotFound || errors.Is(err, ErrWorkspaceChanged) {
		t.Fatalf("captured runtime lookup error=%v", err)
	}
}
