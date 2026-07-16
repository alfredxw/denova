package workspacechange

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewThreadAggregatesRunsFiltersCommentsAndKeepsUndoBoundary(t *testing.T) {
	service, path := newTestServiceWithFile(t, "zero")
	ctx := context.Background()
	first, err := service.ReplaceFile(ctx, ReplaceFileRequest{
		Path:         path,
		Content:      "one",
		BaseRevision: Revision([]byte("zero")),
		Metadata: ChangeMetadata{
			Origin: OriginAgent, ChangeGroupID: "group-one", ReviewThreadID: "thread-one", RunID: "run-one", SessionID: "session-one",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.ReplaceFile(ctx, ReplaceFileRequest{
		Path:         path,
		Content:      "two",
		BaseRevision: first.Revision,
		Metadata: ChangeMetadata{
			Origin: OriginAgent, ChangeGroupID: "group-two", ReviewThreadID: "thread-one", RunID: "run-two", SessionID: "session-one",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	unresolved, err := service.AddComment(ctx, AddCommentRequest{
		GroupID: "group-one", ChangeSetID: first.ID, Body: "Tighten the opening.",
	})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := service.AddComment(ctx, AddCommentRequest{GroupID: "group-two", Body: "Already handled."})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ResolveComment(ctx, ResolveCommentRequest{ID: resolved.ID, Resolved: true}); err != nil {
		t.Fatal(err)
	}

	thread, err := service.GetReviewThread(ctx, "thread-one")
	if err != nil {
		t.Fatal(err)
	}
	if thread.ID != "thread-one" || len(thread.Groups) != 2 || thread.LatestGroupID != "group-two" {
		t.Fatalf("unexpected thread identity: %#v", thread)
	}
	if thread.PendingEditCount != 2 || thread.UnresolvedCommentCount != 1 || len(thread.Comments) != 2 {
		t.Fatalf("unexpected thread counts: %#v", thread)
	}
	if len(thread.Files) != 1 {
		t.Fatalf("thread files = %#v", thread.Files)
	}
	file := thread.Files[0]
	if file.Path != path || file.BeforeContent != "zero" || file.AfterContent != "two" || file.Continuity != ReviewThreadContinuityContinuous {
		t.Fatalf("unexpected cumulative file: %#v", file)
	}
	if file.BaseRevision != Revision([]byte("zero")) || file.Revision != Revision([]byte("two")) || len(file.ChangeSetIDs) != 2 {
		t.Fatalf("unexpected cumulative revisions: %#v", file)
	}
	if file.BaseGroupID != first.GroupID || file.BaseChangeSetID != first.ID || file.LatestGroupID != second.GroupID || file.LatestChangeSetID != second.ID {
		t.Fatalf("unexpected cumulative comment targets: %#v", file)
	}
	if _, err := service.AddComment(ctx, AddCommentRequest{
		GroupID: file.BaseGroupID, ChangeSetID: file.BaseChangeSetID, Body: "Comment on the cumulative before side.",
		Anchor: CommentAnchor{
			Kind: "text-range", Side: CommentAnchorSideBefore, Encoding: CommentAnchorEncodingUTF8Byte,
			Revision: file.BaseRevision, Start: 0, End: 4, Quote: "zero",
		},
	}); err != nil {
		t.Fatalf("before-side cumulative comment: %v", err)
	}

	for name, filter := range map[string]ChangeFilter{
		"run":     {RunID: "run-two"},
		"session": {SessionID: "session-one"},
		"thread":  {ReviewThreadID: "thread-one"},
	} {
		groups, listErr := service.ListGroups(ctx, filter)
		if listErr != nil {
			t.Fatalf("%s filter: %v", name, listErr)
		}
		want := 2
		if name == "run" {
			want = 1
		}
		if len(groups) != want {
			t.Fatalf("%s groups = %#v", name, groups)
		}
		for _, group := range groups {
			if group.ReviewThreadID != "thread-one" {
				t.Fatalf("%s summary lost thread: %#v", name, group)
			}
		}
	}
	groupOne, err := service.GetGroup(ctx, "group-one")
	if err != nil || groupOne.UnresolvedCommentCount != 2 {
		t.Fatalf("group comment count = %d, err=%v", groupOne.UnresolvedCommentCount, err)
	}

	feedback, err := service.GetReviewComments(ctx, "thread-one", "session-one", []string{unresolved.ID, unresolved.ID})
	if err != nil || len(feedback) != 1 || feedback[0].Path != path || feedback[0].Comment.Body != unresolved.Body {
		t.Fatalf("feedback = %#v, err=%v", feedback, err)
	}
	if _, err := service.GetReviewComments(ctx, "thread-one", "session-one", []string{resolved.ID}); err == nil {
		t.Fatal("resolved comment was accepted as feedback")
	}
	otherPath := "chapters/other.md"
	writeTestFile(t, service.workspace, otherPath, "other")
	otherChange, err := service.ReplaceFile(ctx, ReplaceFileRequest{
		Path: otherPath, Content: "changed", BaseRevision: Revision([]byte("other")),
		Metadata: ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "group-other", ReviewThreadID: "thread-other"},
	})
	if err != nil {
		t.Fatal(err)
	}
	otherComment, err := service.AddComment(ctx, AddCommentRequest{
		GroupID: otherChange.GroupID, ChangeSetID: otherChange.ID, Body: "Other thread feedback.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetReviewComments(ctx, "thread-one", "session-one", []string{otherComment.ID}); err == nil {
		t.Fatal("comment from another review thread was accepted as feedback")
	}
	if _, err := service.GetReviewComments(ctx, "thread-one", "session-two", []string{unresolved.ID}); err == nil {
		t.Fatal("comment from another session was accepted as feedback")
	}

	if _, err := service.Undo(ctx, HistoryRequest{GroupID: second.GroupID}); err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, service.workspace, path); got != "one" {
		t.Fatalf("undo of second run crossed the first run boundary: %q", got)
	}
	thread, err = service.GetReviewThread(ctx, "thread-one")
	if err != nil {
		t.Fatal(err)
	}
	if got := thread.Files[0].AfterContent; got != "one" {
		t.Fatalf("thread head after second-run undo = %q", got)
	}
}

func TestReviewThreadMarksRevisionGapAndShowsLatestContiguousSegment(t *testing.T) {
	service, path := newTestServiceWithFile(t, "zero")
	ctx := context.Background()
	first, err := service.ReplaceFile(ctx, ReplaceFileRequest{
		Path: path, Content: "one", BaseRevision: Revision([]byte("zero")),
		Metadata: ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "gap-one", ReviewThreadID: "gap-thread"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(service.workspace, filepath.FromSlash(path)), []byte("external"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := service.ReplaceFile(ctx, ReplaceFileRequest{
		Path: path, Content: "two", BaseRevision: Revision([]byte("external")),
		Metadata: ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "gap-two", ReviewThreadID: "gap-thread"},
	})
	if err != nil {
		t.Fatal(err)
	}
	thread, err := service.GetReviewThread(ctx, "gap-thread")
	if err != nil {
		t.Fatal(err)
	}
	if thread.Groups == nil || thread.Comments == nil || thread.Files == nil {
		t.Fatalf("review thread collections must encode as arrays: %#v", thread)
	}
	file := thread.Files[0]
	if file.GroupIDs == nil || file.ChangeSetIDs == nil || file.PendingEditIDs == nil {
		t.Fatalf("review file collections must encode as arrays: %#v", file)
	}
	if file.Continuity != ReviewThreadContinuityConflicted || file.OmittedIterationCount != 1 {
		t.Fatalf("revision gap was silently folded: %#v", file)
	}
	if file.BeforeContent != "external" || file.AfterContent != "two" || file.LatestChangeSetID != second.ID {
		t.Fatalf("latest contiguous segment = %#v; first=%s", file, first.ID)
	}
}

func TestReviewThreadDefaultsToGroupID(t *testing.T) {
	service, path := newTestServiceWithFile(t, "before")
	change, err := service.ReplaceFile(context.Background(), ReplaceFileRequest{
		Path: path, Content: "after", BaseRevision: Revision([]byte("before")),
		Metadata: ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "legacy-group"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if change.ReviewThreadID != "legacy-group" {
		t.Fatalf("change thread = %q", change.ReviewThreadID)
	}
	ledger, err := os.ReadFile(service.store.ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	legacyLedger := strings.ReplaceAll(string(ledger), `,"review_thread_id":"legacy-group"`, "")
	if legacyLedger == string(ledger) || strings.Contains(legacyLedger, "review_thread_id") {
		t.Fatalf("test fixture did not remove review_thread_id: %s", ledger)
	}
	if err := os.WriteFile(service.store.ledgerPath, []byte(legacyLedger), 0o600); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewService(service.workspace)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := reloaded.GetReviewThread(context.Background(), "legacy-group")
	if err != nil || len(thread.Groups) != 1 || thread.Groups[0].ReviewThreadID != "legacy-group" {
		t.Fatalf("legacy thread = %#v, err=%v", thread, err)
	}
}
