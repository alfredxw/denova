package reviewapp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	agentchat "denova/internal/agents/chat"
	agentreview "denova/internal/agents/review"
	agentrun "denova/internal/agents/run"
	"denova/internal/book"
	workspacelayout "denova/internal/workspace"
	workspacechange "denova/internal/workspace/change"
	"denova/internal/workspace/documentreview"
)

func TestReviewInputEffectConvergesAfterOneLedgerCommittedBeforeCrash(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	changePath := "chapters/change.md"
	documentPath := "chapters/document.md"
	if err := os.WriteFile(filepath.Join(workspace, changePath), []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	documentContent := "Alpha target Omega\n"
	if err := os.WriteFile(filepath.Join(workspace, documentPath), []byte(documentContent), 0o644); err != nil {
		t.Fatal(err)
	}
	stateRoot := workspacelayout.Dir(workspace)
	changes, err := workspacechange.ForWorkspaceAt(workspace, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	change, err := changes.ReplaceFile(ctx, workspacechange.ReplaceFileRequest{
		Path: changePath, Content: "after", BaseRevision: workspacechange.Revision([]byte("before")),
		Metadata: workspacechange.ChangeMetadata{
			Origin: workspacechange.OriginAgent, ChangeGroupID: "group-effect", ReviewThreadID: "thread-effect", SessionID: "session-effect",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	diffComment, err := changes.AddComment(ctx, workspacechange.AddCommentRequest{
		GroupID: change.GroupID, ChangeSetID: change.ID, Body: "diff review",
	})
	if err != nil {
		t.Fatal(err)
	}
	documents, err := documentreview.ForWorkspaceAt(workspace, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	documentThread, documentComment, err := documents.AddComment(ctx, documentreview.AddCommentRequest{
		Target: documentreview.Target{Kind: documentreview.TargetKindWorkspaceFile, ID: documentPath},
		Body:   "document review",
		Anchor: documentreview.Anchor{
			Kind: documentreview.AnchorKindTextRange, Encoding: documentreview.AnchorEncodingUTF8,
			Revision: workspacechange.Revision([]byte(documentContent)), Start: len("Alpha "), End: len("Alpha target"),
			Quote: "target", Prefix: "Alpha ", Suffix: " Omega\n", DisplayQuote: "target",
		},
	}, documentreview.Snapshot{Content: documentContent, Revision: workspacechange.Revision([]byte(documentContent))})
	if err != nil {
		t.Fatal(err)
	}
	request := agentchat.ChatRequest{
		ReviewFeedback: agentreview.Refs{
			{Source: agentreview.SourceWorkspaceChange, ReviewThreadID: "thread-effect", CommentIDs: []string{diffComment.ID}},
			{Source: agentreview.SourceDocument, ReviewThreadID: documentThread.ID, CommentIDs: []string{documentComment.ID}},
		},
		ResolvedReviewFeedback: agentreview.Contexts{
			{Source: agentreview.SourceWorkspaceChange, ReviewThreadID: "thread-effect", Comments: []agentreview.Comment{{ID: diffComment.ID, Body: diffComment.Body}}},
			{Source: agentreview.SourceDocument, ReviewThreadID: documentThread.ID, Comments: []agentreview.Comment{{ID: documentComment.ID, Body: documentComment.Body}}},
		},
	}
	runtime := Runtime{
		Workspace: workspace, StateRoot: stateRoot, SessionID: "session-effect",
		DocumentsEnabled: true, BookService: book.NewService(workspace),
	}
	effect := agentrun.InputCommitEffectRequest{CommandID: "command", OperationID: "operation", Cycle: 1, Hash: "canonical-hash"}
	effectID, err := effect.ID()
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a process crash after the first independent ledger committed.
	if _, err := changes.ConsumeReviewCommentsForAgentInput(ctx, "thread-effect", "session-effect", []string{diffComment.ID}, effectID); err != nil {
		t.Fatal(err)
	}
	if found, err := ReconcileConsumed(ctx, runtime, request, effectID); err != nil || found {
		t.Fatalf("partial effect incorrectly acknowledged found=%t err=%v", found, err)
	}
	if err := consume(ctx, runtime, request, effectID); err != nil {
		t.Fatalf("partial effect did not converge: %v", err)
	}
	if found, err := ReconcileConsumed(ctx, runtime, request, effectID); err != nil || !found {
		t.Fatalf("converged effect receipt found=%t err=%v", found, err)
	}
}
