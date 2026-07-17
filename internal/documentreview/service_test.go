package documentreview

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"denova/internal/workspacechange"
)

func TestDocumentReviewCommentLifecyclePersistsAndRotatesThread(t *testing.T) {
	workspace := t.TempDir()
	service, err := NewService(workspace)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	snapshot := Snapshot{Content: "第一段。\n\n第二段。\n", Revision: workspacechange.Revision([]byte("第一段。\n\n第二段。\n"))}
	anchor := testAnchor(snapshot, "第二段。")

	thread, comment, err := service.AddComment(context.Background(), AddCommentRequest{
		Path: "chapters/ch01.md", Body: "这里需要补足动机", Anchor: anchor,
	}, snapshot)
	if err != nil {
		t.Fatalf("add comment: %v", err)
	}
	if thread.ID == "" || comment.ThreadID != thread.ID || len(thread.Comments) != 1 {
		t.Fatalf("unexpected thread/comment: %#v %#v", thread, comment)
	}

	thread, comment, err = service.UpdateComment(context.Background(), UpdateCommentRequest{ID: comment.ID, Body: "这里需要补足人物动机"})
	if err != nil {
		t.Fatalf("update comment: %v", err)
	}
	if comment.Body != "这里需要补足人物动机" || thread.Comments[0].Body != comment.Body {
		t.Fatalf("updated comment was not projected: %#v", thread)
	}

	reloaded, err := NewService(workspace)
	if err != nil {
		t.Fatalf("reload service: %v", err)
	}
	persisted, err := reloaded.CurrentThread(context.Background())
	if err != nil {
		t.Fatalf("current thread: %v", err)
	}
	if persisted.ID != thread.ID || len(persisted.Comments) != 1 || persisted.Comments[0].Body != comment.Body {
		t.Fatalf("persisted thread = %#v", persisted)
	}

	consumed, err := reloaded.ConsumeReviewComments(context.Background(), thread.ID, []string{comment.ID, comment.ID})
	if err != nil {
		t.Fatalf("consume comments: %v", err)
	}
	if len(consumed) != 1 || !consumed[0].Deleted {
		t.Fatalf("consumed comments = %#v", consumed)
	}
	empty, err := reloaded.CurrentThread(context.Background())
	if err != nil || empty.ID != "" || len(empty.Comments) != 0 {
		t.Fatalf("current thread after consume = %#v err=%v", empty, err)
	}

	nextThread, _, err := reloaded.AddComment(context.Background(), AddCommentRequest{
		Path: "chapters/ch01.md", Body: "新的审阅批次", Anchor: anchor,
	}, snapshot)
	if err != nil {
		t.Fatalf("add next comment: %v", err)
	}
	if nextThread.ID == thread.ID {
		t.Fatalf("consumed review thread was reused: %q", thread.ID)
	}
}

func TestDocumentReviewRejectsStaleAndForgedAnchors(t *testing.T) {
	workspace := t.TempDir()
	service, err := NewService(workspace)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	snapshot := Snapshot{Content: "正文内容", Revision: workspacechange.Revision([]byte("正文内容"))}
	anchor := testAnchor(snapshot, "正文")
	anchor.Revision = "sha256:stale"

	_, _, err = service.AddComment(context.Background(), AddCommentRequest{Path: "chapter.md", Body: "意见", Anchor: anchor}, snapshot)
	var reviewErr *Error
	if !errors.As(err, &reviewErr) || reviewErr.Code != ErrorCodeConflict {
		t.Fatalf("stale anchor error = %v", err)
	}

	anchor = testAnchor(snapshot, "正文")
	anchor.Quote = "伪造原文"
	_, _, err = service.AddComment(context.Background(), AddCommentRequest{Path: "chapter.md", Body: "意见", Anchor: anchor}, snapshot)
	if !errors.As(err, &reviewErr) || reviewErr.Code != ErrorCodeConflict {
		t.Fatalf("forged quote error = %v", err)
	}
}

func TestDocumentReviewStorageRejectsSymlinkedDirectory(t *testing.T) {
	workspace := t.TempDir()
	external := t.TempDir()
	if err := os.Mkdir(filepath.Join(workspace, ".denova"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(workspace, ".denova", "reviews")); err != nil {
		t.Fatal(err)
	}
	if _, err := NewService(workspace); err == nil {
		t.Fatal("expected symlinked review storage to be rejected")
	}
}

func TestDocumentReviewRepairsTornLedgerTail(t *testing.T) {
	workspace := t.TempDir()
	service, err := NewService(workspace)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	snapshot := Snapshot{Content: "需要审阅的正文", Revision: workspacechange.Revision([]byte("需要审阅的正文"))}
	_, comment, err := service.AddComment(context.Background(), AddCommentRequest{
		Path: "chapters/ch01.md", Body: "补充细节", Anchor: testAnchor(snapshot, "需要审阅"),
	}, snapshot)
	if err != nil {
		t.Fatalf("add comment: %v", err)
	}
	service.store.close()

	ledger := filepath.Join(workspace, ".denova", "reviews", "ledger.jsonl")
	complete, err := os.Stat(ledger)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(ledger, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"type":"comments_upserted"`); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewService(workspace)
	if err != nil {
		t.Fatalf("reload with torn tail: %v", err)
	}
	thread, err := reloaded.CurrentThread(context.Background())
	if err != nil || len(thread.Comments) != 1 || thread.Comments[0].ID != comment.ID {
		t.Fatalf("repaired thread = %#v err=%v", thread, err)
	}
	repaired, err := os.Stat(ledger)
	if err != nil {
		t.Fatal(err)
	}
	if repaired.Size() != complete.Size() {
		t.Fatalf("torn tail was not truncated: complete=%d repaired=%d", complete.Size(), repaired.Size())
	}
}

func TestProjectAnchorOnlyRelocatesUniqueContext(t *testing.T) {
	before := "开头\n目标段落\n结尾"
	snapshot := Snapshot{Content: before, Revision: workspacechange.Revision([]byte(before))}
	anchor := testAnchor(snapshot, "目标段落")
	after := "新增\n开头\n目标段落\n结尾"
	projected, outdated := ProjectAnchor(after, workspacechange.Revision([]byte(after)), anchor)
	if outdated || after[projected.Start:projected.End] != "目标段落" {
		t.Fatalf("projected anchor = %#v outdated=%v", projected, outdated)
	}

	ambiguous := "目标段落\n目标段落"
	anchor.Prefix = ""
	anchor.Suffix = ""
	if _, outdated := ProjectAnchor(ambiguous, workspacechange.Revision([]byte(ambiguous)), anchor); !outdated {
		t.Fatal("ambiguous quote should remain outdated")
	}
}

func testAnchor(snapshot Snapshot, quote string) Anchor {
	start := len(snapshot.Content[:indexOf(snapshot.Content, quote)])
	end := start + len(quote)
	prefixStart := start - 4
	if prefixStart < 0 {
		prefixStart = 0
	}
	suffixEnd := end + 4
	if suffixEnd > len(snapshot.Content) {
		suffixEnd = len(snapshot.Content)
	}
	return Anchor{
		Kind: AnchorKindTextRange, Encoding: AnchorEncodingUTF8, Revision: snapshot.Revision,
		Start: start, End: end, Quote: quote, Prefix: snapshot.Content[prefixStart:start], Suffix: snapshot.Content[end:suffixEnd],
		DisplayQuote: quote, EditorFrom: 1, EditorTo: 2,
	}
}

func indexOf(content, quote string) int {
	for index := 0; index+len(quote) <= len(content); index++ {
		if content[index:index+len(quote)] == quote {
			return index
		}
	}
	return -1
}
