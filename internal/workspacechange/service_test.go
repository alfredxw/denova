package workspacechange

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyEditsUsesOneBaseAndCommitsAtomically(t *testing.T) {
	service, path := newTestServiceWithFile(t, "alpha beta gamma")
	change, err := service.ApplyEdits(context.Background(), ApplyEditsRequest{
		Path:         path,
		BaseRevision: Revision([]byte("alpha beta gamma")),
		Edits: []TextEdit{
			{ID: "first", OldString: "alpha", NewString: "ALPHA"},
			{ID: "last", OldString: "gamma", NewString: "GAMMA"},
		},
		Metadata: ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "run-1"},
	})
	if err != nil {
		t.Fatalf("ApplyEdits failed: %v", err)
	}
	if got := readTestFile(t, service.workspace, path); got != "ALPHA beta GAMMA" {
		t.Fatalf("unexpected file content %q", got)
	}
	if change.BaseRevision != Revision([]byte("alpha beta gamma")) || change.Revision != Revision([]byte("ALPHA beta GAMMA")) {
		t.Fatalf("unexpected revisions: %#v", change)
	}
	if len(change.Edits) != 2 || change.Edits[0].Hunks[0].BeforeStart != 0 || change.Edits[1].Hunks[0].AfterStart != 11 {
		t.Fatalf("unexpected edit projection: %#v", change.Edits)
	}

	beforeFailure := readTestFile(t, service.workspace, path)
	_, err = service.ApplyEdits(context.Background(), ApplyEditsRequest{
		Path:         path,
		BaseRevision: change.Revision,
		Edits: []TextEdit{
			{OldString: "ALPHA beta", NewString: "x"},
			{OldString: "beta GAMMA", NewString: "y"},
		},
	})
	assertChangeErrorCode(t, err, ErrorCodeInvalidEdit)
	if got := readTestFile(t, service.workspace, path); got != beforeFailure {
		t.Fatalf("failed batch mutated file: before=%q after=%q", beforeFailure, got)
	}

	_, err = service.ApplyEdits(context.Background(), ApplyEditsRequest{
		Path:         path,
		BaseRevision: change.Revision,
		Edits:        []TextEdit{{OldString: "ALPHA", NewString: "A"}, {OldString: "missing", NewString: "M"}},
	})
	assertChangeErrorCode(t, err, ErrorCodeInvalidEdit)
	if got := readTestFile(t, service.workspace, path); got != beforeFailure {
		t.Fatalf("partially invalid batch mutated file: before=%q after=%q", beforeFailure, got)
	}
}

func TestApplyEditsReplaceAll(t *testing.T) {
	service, path := newTestServiceWithFile(t, "red blue red")
	change, err := service.ApplyEdits(context.Background(), ApplyEditsRequest{
		Path:         path,
		BaseRevision: Revision([]byte("red blue red")),
		Edits:        []TextEdit{{ID: "colors", OldString: "red", NewString: "green", ReplaceAll: true}},
	})
	if err != nil {
		t.Fatalf("ApplyEdits failed: %v", err)
	}
	if got := readTestFile(t, service.workspace, path); got != "green blue green" {
		t.Fatalf("unexpected content %q", got)
	}
	if got := len(change.Edits[0].Hunks); got != 2 {
		t.Fatalf("replace_all hunks = %d", got)
	}
}

func TestApplyEditsRejectsStaleRevision(t *testing.T) {
	service, path := newTestServiceWithFile(t, "first")
	_, revision, err := service.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(service.workspace, filepath.FromSlash(path)), []byte("external"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = service.ApplyEdits(context.Background(), ApplyEditsRequest{
		Path:         path,
		BaseRevision: revision,
		Edits:        []TextEdit{{OldString: "external", NewString: "agent"}},
	})
	assertChangeErrorCode(t, err, ErrorCodeRevisionConflict)
	if got := readTestFile(t, service.workspace, path); got != "external" {
		t.Fatalf("stale edit overwrote external content: %q", got)
	}
}

func TestPersistenceReloadHydratesContentWithoutInliningManuscript(t *testing.T) {
	workspace := t.TempDir()
	path := "chapters/ch01.md"
	writeTestFile(t, workspace, path, "secret-before-text")
	service, err := NewService(workspace)
	if err != nil {
		t.Fatal(err)
	}
	change, err := service.ApplyEdits(context.Background(), ApplyEditsRequest{
		Path:         path,
		BaseRevision: Revision([]byte("secret-before-text")),
		Edits:        []TextEdit{{OldString: "secret-before-text", NewString: "secret-after-text"}},
		Metadata:     ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "persisted-group", RunID: "run-persisted"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := os.ReadFile(service.store.ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(ledger), "secret-before-text") || strings.Contains(string(ledger), "secret-after-text") {
		t.Fatalf("ledger inlined manuscript content: %s", ledger)
	}
	reloaded, err := NewService(workspace)
	if err != nil {
		t.Fatal(err)
	}
	group, err := reloaded.GetGroup(context.Background(), change.GroupID)
	if err != nil {
		t.Fatal(err)
	}
	if len(group.ChangeSets) != 1 || group.ChangeSets[0].BeforeContent != "secret-before-text" || group.ChangeSets[0].AfterContent != "secret-after-text" {
		t.Fatalf("reloaded group did not hydrate blobs: %#v", group)
	}
	if group.ChangeSets[0].Edits[0].OldString != "secret-before-text" || group.ChangeSets[0].Edits[0].NewString != "secret-after-text" {
		t.Fatalf("reloaded edit text was not reconstructed: %#v", group.ChangeSets[0].Edits)
	}
}

func TestReviewPartiallyRejectsOneEdit(t *testing.T) {
	service, path := newTestServiceWithFile(t, "one two three")
	change, err := service.ApplyEdits(context.Background(), ApplyEditsRequest{
		Path:         path,
		BaseRevision: Revision([]byte("one two three")),
		Edits: []TextEdit{
			{ID: "one", OldString: "one", NewString: "ONE"},
			{ID: "three", OldString: "three", NewString: "THREE"},
		},
		Metadata: ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "review-group"},
	})
	if err != nil {
		t.Fatal(err)
	}
	group, err := service.Review(context.Background(), ReviewRequest{
		GroupID:     change.GroupID,
		ChangeSetID: change.ID,
		Decision:    ReviewDecisionReject,
		EditIDs:     []string{"one"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, service.workspace, path); got != "one two THREE" {
		t.Fatalf("partial reject content = %q", got)
	}
	if group.ReviewStatus != ReviewStatusMixed || len(group.ChangeSets) != 2 {
		t.Fatalf("unexpected group after partial reject: %#v", group)
	}
	var original *ChangeSet
	for index := range group.ChangeSets {
		if group.ChangeSets[index].ID == change.ID {
			original = &group.ChangeSets[index]
		}
	}
	if original == nil || original.Edits[0].ReviewStatus != ReviewStatusRejected || original.Edits[1].ReviewStatus != ReviewStatusPending {
		t.Fatalf("unexpected review statuses: %#v", original)
	}
}

func TestReviewAcceptValidatesSelectionBeforeRecording(t *testing.T) {
	service, path := newTestServiceWithFile(t, "one two")
	change, err := service.ApplyEdits(context.Background(), ApplyEditsRequest{
		Path:         path,
		BaseRevision: Revision([]byte("one two")),
		Edits: []TextEdit{
			{ID: "one", OldString: "one", NewString: "ONE"},
			{ID: "two", OldString: "two", NewString: "TWO"},
		},
		Metadata: ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "accept-group"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Review(context.Background(), ReviewRequest{
		GroupID: change.GroupID, Decision: ReviewDecisionAccept, EditIDs: []string{"one", "missing"},
	})
	assertChangeErrorCode(t, err, ErrorCodeNotFound)
	group, err := service.GetGroup(context.Background(), change.GroupID)
	if err != nil {
		t.Fatal(err)
	}
	if group.ChangeSets[0].Edits[0].ReviewStatus != ReviewStatusPending || group.ChangeSets[0].Edits[1].ReviewStatus != ReviewStatusPending {
		t.Fatalf("invalid accept recorded a partial decision: %#v", group.ChangeSets[0].Edits)
	}
	group, err = service.Review(context.Background(), ReviewRequest{
		GroupID: change.GroupID, Decision: ReviewDecisionAccept, EditIDs: []string{"one"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if group.ReviewStatus != ReviewStatusMixed || readTestFile(t, service.workspace, path) != "ONE TWO" {
		t.Fatalf("accept should only update review state: %#v", group)
	}
}

func TestReviewAllTransitionsOnlyRemainingPendingEdits(t *testing.T) {
	service, path := newTestServiceWithFile(t, "one two")
	change, err := service.ApplyEdits(context.Background(), ApplyEditsRequest{
		Path:         path,
		BaseRevision: Revision([]byte("one two")),
		Edits: []TextEdit{
			{ID: "one", OldString: "one", NewString: "ONE"},
			{ID: "two", OldString: "two", NewString: "TWO"},
		},
		Metadata: ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "remaining-review-group"},
	})
	if err != nil {
		t.Fatal(err)
	}
	group, err := service.Review(context.Background(), ReviewRequest{
		GroupID: change.GroupID, Decision: ReviewDecisionReject, EditIDs: []string{"one"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if group.PendingEditCount != 1 {
		t.Fatalf("pending edits after partial reject = %d", group.PendingEditCount)
	}
	pendingGroups, err := service.ListGroups(context.Background(), ChangeFilter{Status: ReviewStatusPending})
	if err != nil || len(pendingGroups) != 1 {
		t.Fatalf("partially reviewed mixed group should remain pending: groups=%#v err=%v", pendingGroups, err)
	}
	group, err = service.Review(context.Background(), ReviewRequest{
		GroupID: change.GroupID, Decision: ReviewDecisionAccept,
	})
	if err != nil {
		t.Fatal(err)
	}
	if group.PendingEditCount != 0 || group.ReviewStatus != ReviewStatusMixed {
		t.Fatalf("accepted+rejected group should be fully reviewed mixed: %#v", group)
	}
	if got := readTestFile(t, service.workspace, path); got != "one TWO" {
		t.Fatalf("accept all rewrote the terminal rejection: %q", got)
	}
	var original *ChangeSet
	for index := range group.ChangeSets {
		if group.ChangeSets[index].ID == change.ID {
			original = &group.ChangeSets[index]
		}
	}
	if original == nil || original.Edits[0].ReviewStatus != ReviewStatusRejected || original.Edits[1].ReviewStatus != ReviewStatusAccepted {
		t.Fatalf("unexpected terminal decisions: %#v", original)
	}
	_, err = service.Review(context.Background(), ReviewRequest{
		GroupID: change.GroupID, Decision: ReviewDecisionAccept, EditIDs: []string{"one"},
	})
	assertChangeErrorCode(t, err, ErrorCodeConflict)
	_, err = service.Review(context.Background(), ReviewRequest{
		GroupID: change.GroupID, Decision: ReviewDecisionReject, EditIDs: []string{"two"},
	})
	assertChangeErrorCode(t, err, ErrorCodeConflict)
	summaries, err := service.ListGroups(context.Background(), ChangeFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 || summaries[0].PendingEditCount != 0 {
		t.Fatalf("summary did not expose completed review: %#v", summaries)
	}
	pendingGroups, err = service.ListGroups(context.Background(), ChangeFilter{Status: ReviewStatusPending})
	if err != nil || len(pendingGroups) != 0 {
		t.Fatalf("fully reviewed mixed group leaked into pending filter: groups=%#v err=%v", pendingGroups, err)
	}
}

func TestUndoRedoPersistsInverseAndReplay(t *testing.T) {
	service, path := newTestServiceWithFile(t, "draft")
	change, err := service.ReplaceFile(context.Background(), ReplaceFileRequest{
		Path: path, Content: "published", BaseRevision: Revision([]byte("draft")), Metadata: ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "history-group"},
	})
	if err != nil {
		t.Fatal(err)
	}
	undone, err := service.Undo(context.Background(), HistoryRequest{GroupID: change.GroupID})
	if err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, service.workspace, path); got != "draft" || undone.ApplyState != ApplyStateReverted {
		t.Fatalf("undo failed: content=%q group=%#v", got, undone)
	}
	redone, err := service.Redo(context.Background(), HistoryRequest{GroupID: change.GroupID})
	if err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, service.workspace, path); got != "published" || redone.ApplyState != ApplyStateApplied {
		t.Fatalf("redo failed: content=%q group=%#v", got, redone)
	}
	if len(redone.ChangeSets) != 3 {
		t.Fatalf("history should retain original, inverse, and replay: %#v", redone.ChangeSets)
	}
	reloaded, err := NewService(service.workspace)
	if err != nil {
		t.Fatal(err)
	}
	group, err := reloaded.GetGroup(context.Background(), change.GroupID)
	if err != nil || len(group.ChangeSets) != 3 {
		t.Fatalf("history did not survive reload: group=%#v err=%v", group, err)
	}
}

func TestRedoIsInvalidatedByLocalSave(t *testing.T) {
	service, path := newTestServiceWithFile(t, "draft")
	first, err := service.ReplaceFile(context.Background(), ReplaceFileRequest{
		Path: path, Content: "first", BaseRevision: Revision([]byte("draft")), Metadata: ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "first-group"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Undo(context.Background(), HistoryRequest{GroupID: first.GroupID}); err != nil {
		t.Fatal(err)
	}
	_, baseRevision, err := service.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveFile(context.Background(), path, "second", baseRevision); err != nil {
		t.Fatal(err)
	}
	group, err := service.GetGroup(context.Background(), first.GroupID)
	if err != nil {
		t.Fatal(err)
	}
	if group.CanRedo {
		t.Fatalf("local save left stale redo enabled: %#v", group)
	}
	_, err = service.Redo(context.Background(), HistoryRequest{GroupID: first.GroupID})
	assertChangeErrorCode(t, err, ErrorCodeNoRedo)
	if got := readTestFile(t, service.workspace, path); got != "second" {
		t.Fatalf("invalid redo changed file: %q", got)
	}
}

func TestHistoryCapabilitiesValidateLiveWorkspaceHead(t *testing.T) {
	service, path := newTestServiceWithFile(t, "draft")
	change, err := service.ReplaceFile(context.Background(), ReplaceFileRequest{
		Path: path, Content: "agent draft", BaseRevision: Revision([]byte("draft")), Metadata: ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "live-head-undo-group"},
	})
	if err != nil {
		t.Fatal(err)
	}
	group, err := service.GetGroup(context.Background(), change.GroupID)
	if err != nil || !group.CanUndo {
		t.Fatalf("fresh group should be undoable: group=%#v err=%v", group, err)
	}
	if _, err := service.SaveFile(context.Background(), path, "local draft", change.Revision); err != nil {
		t.Fatal(err)
	}
	group, err = service.GetGroup(context.Background(), change.GroupID)
	if err != nil {
		t.Fatal(err)
	}
	if group.CanUndo || group.CanRedo {
		t.Fatalf("stale group exposed history action after local save: %#v", group)
	}
	summaries, err := service.ListGroups(context.Background(), ChangeFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 || summaries[0].CanUndo || summaries[0].CanRedo {
		t.Fatalf("list did not validate live workspace head: %#v", summaries)
	}
	_, err = service.Undo(context.Background(), HistoryRequest{GroupID: change.GroupID})
	assertChangeErrorCode(t, err, ErrorCodeRevisionConflict)
}

func TestRedoCapabilityValidatesLiveWorkspaceHead(t *testing.T) {
	service, path := newTestServiceWithFile(t, "draft")
	change, err := service.ReplaceFile(context.Background(), ReplaceFileRequest{
		Path: path, Content: "agent draft", BaseRevision: Revision([]byte("draft")), Metadata: ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "live-head-redo-group"},
	})
	if err != nil {
		t.Fatal(err)
	}
	group, err := service.Undo(context.Background(), HistoryRequest{GroupID: change.GroupID})
	if err != nil || !group.CanRedo {
		t.Fatalf("undone group should be redoable: group=%#v err=%v", group, err)
	}
	writeTestFile(t, service.workspace, path, "external draft")
	group, err = service.GetGroup(context.Background(), change.GroupID)
	if err != nil {
		t.Fatal(err)
	}
	if group.CanRedo {
		t.Fatalf("stale redo remained enabled after external write: %#v", group)
	}
	_, err = service.Redo(context.Background(), HistoryRequest{GroupID: change.GroupID})
	assertChangeErrorCode(t, err, ErrorCodeRevisionConflict)
}

func TestUndoAndRedoCreatedFile(t *testing.T) {
	workspace := t.TempDir()
	service, err := NewService(workspace)
	if err != nil {
		t.Fatal(err)
	}
	path := "chapters/new.md"
	change, err := service.ReplaceFile(context.Background(), ReplaceFileRequest{
		Path: path, Content: "new chapter", BaseRevision: "missing", Metadata: ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "create-group"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Undo(context.Background(), HistoryRequest{GroupID: change.GroupID}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(workspace, filepath.FromSlash(path))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("undo should remove created file, stat err=%v", err)
	}
	if _, err := service.Redo(context.Background(), HistoryRequest{GroupID: change.GroupID}); err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, workspace, path); got != "new chapter" {
		t.Fatalf("redo did not recreate file: %q", got)
	}
}

func TestForWorkspaceReturnsSharedService(t *testing.T) {
	workspace := t.TempDir()
	first, err := ForWorkspace(workspace)
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(cwd, workspace)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ForWorkspace(relative)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("ForWorkspace returned distinct services for the same absolute workspace")
	}
}

func TestCommentsAreAppendOnlyAndReloaded(t *testing.T) {
	service, path := newTestServiceWithFile(t, "draft")
	change, err := service.ReplaceFile(context.Background(), ReplaceFileRequest{
		Path: path, Content: "next", BaseRevision: Revision([]byte("draft")), Metadata: ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "comment-group"},
	})
	if err != nil {
		t.Fatal(err)
	}
	comment, err := service.AddComment(context.Background(), AddCommentRequest{
		GroupID: change.GroupID, ChangeSetID: change.ID, EditID: change.Edits[0].ID, Body: "Please clarify this sentence", Author: "reviewer",
		Anchor: CommentAnchor{Side: CommentAnchorSideAfter, Encoding: CommentAnchorEncodingUTF8Byte, Kind: "text", Revision: change.Revision, Start: 0, End: 4, Quote: "next"},
	})
	if err != nil {
		t.Fatal(err)
	}
	comment, err = service.UpdateComment(context.Background(), UpdateCommentRequest{ID: comment.ID, Body: "Please simplify this sentence"})
	if err != nil {
		t.Fatal(err)
	}
	comment, err = service.ResolveComment(context.Background(), ResolveCommentRequest{ID: comment.ID, Resolved: true})
	if err != nil {
		t.Fatal(err)
	}
	comment, err = service.DeleteComment(context.Background(), DeleteCommentRequest{ID: comment.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !comment.Resolved || !comment.Deleted {
		t.Fatalf("unexpected final comment: %#v", comment)
	}
	reloaded, err := NewService(service.workspace)
	if err != nil {
		t.Fatal(err)
	}
	group, err := reloaded.GetGroup(context.Background(), change.GroupID)
	if err != nil || len(group.Comments) != 1 || !group.Comments[0].Deleted || group.Comments[0].Body != "Please simplify this sentence" {
		t.Fatalf("comment did not survive replay: group=%#v err=%v", group, err)
	}
}

func TestCommentAnchorValidatesUTF8ByteProtocolAndPersists(t *testing.T) {
	before := "甲🙂乙"
	after := "甲🙂新乙"
	service, path := newTestServiceWithFile(t, before)
	change, err := service.ReplaceFile(context.Background(), ReplaceFileRequest{
		Path: path, Content: after, BaseRevision: Revision([]byte(before)),
		Metadata: ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "utf8-anchor-group"},
	})
	if err != nil {
		t.Fatal(err)
	}
	start := len("甲")
	end := start + len("🙂新")
	valid := CommentAnchor{
		Side:     CommentAnchorSideAfter,
		Encoding: CommentAnchorEncodingUTF8Byte,
		Kind:     "text",
		Revision: change.Revision,
		Start:    start,
		End:      end,
		Quote:    "🙂新",
		Prefix:   "甲",
		Suffix:   "乙",
	}
	comment, err := service.AddComment(context.Background(), AddCommentRequest{
		GroupID: change.GroupID, ChangeSetID: change.ID, Body: "保留 emoji 边界", Anchor: valid,
	})
	if err != nil {
		t.Fatal(err)
	}
	if comment.Anchor.Side != CommentAnchorSideAfter || comment.Anchor.Encoding != CommentAnchorEncodingUTF8Byte {
		t.Fatalf("anchor protocol was not stored: %#v", comment.Anchor)
	}

	invalid := map[string]CommentAnchor{}
	wrongSide := valid
	wrongSide.Side = "current"
	invalid["side"] = wrongSide
	wrongEncoding := valid
	wrongEncoding.Encoding = "utf16-code-units"
	invalid["encoding"] = wrongEncoding
	staleRevision := valid
	staleRevision.Revision = change.BaseRevision
	invalid["revision"] = staleRevision
	midRune := valid
	midRune.Start++
	invalid["byte-boundary"] = midRune
	wrongQuote := valid
	wrongQuote.Quote = "🙂旧"
	invalid["quote"] = wrongQuote
	wrongPrefix := valid
	wrongPrefix.Prefix = "错"
	invalid["prefix"] = wrongPrefix
	wrongSuffix := valid
	wrongSuffix.Suffix = "错"
	invalid["suffix"] = wrongSuffix
	for name, anchor := range invalid {
		_, addErr := service.AddComment(context.Background(), AddCommentRequest{
			GroupID: change.GroupID, ChangeSetID: change.ID, Body: "invalid " + name, Anchor: anchor,
		})
		if addErr == nil {
			t.Fatalf("%s anchor was accepted: %#v", name, anchor)
		}
	}

	updateAnchor := valid
	updateAnchor.Revision = change.BaseRevision
	if _, err := service.UpdateComment(context.Background(), UpdateCommentRequest{ID: comment.ID, Body: comment.Body, Anchor: &updateAnchor}); err == nil {
		t.Fatal("stale anchor update was accepted")
	}

	reloaded, err := NewService(service.workspace)
	if err != nil {
		t.Fatal(err)
	}
	group, err := reloaded.GetGroup(context.Background(), change.GroupID)
	if err != nil {
		t.Fatal(err)
	}
	if len(group.Comments) != 1 || group.Comments[0].Anchor != valid {
		t.Fatalf("UTF-8 anchor did not survive ledger replay: %#v", group.Comments)
	}
}

func TestCommentAnchorAllowsExactUTF8PointOnEmptyLineAndFile(t *testing.T) {
	for _, test := range []struct {
		name   string
		before string
		after  string
		offset int
		prefix string
		suffix string
	}{
		{name: "empty-line", before: "top\nbottom", after: "top\n\nbottom", offset: len("top\n"), prefix: "top\n", suffix: "\nbottom"},
		{name: "empty-file", before: "seed", after: "", offset: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, path := newTestServiceWithFile(t, test.before)
			change, err := service.ReplaceFile(context.Background(), ReplaceFileRequest{
				Path: path, Content: test.after, BaseRevision: Revision([]byte(test.before)),
				Metadata: ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "point-anchor-" + test.name},
			})
			if err != nil {
				t.Fatal(err)
			}
			anchor := CommentAnchor{
				Side: CommentAnchorSideAfter, Encoding: CommentAnchorEncodingUTF8Byte,
				Revision: change.Revision, Start: test.offset, End: test.offset, Quote: "",
				Prefix: test.prefix, Suffix: test.suffix,
			}
			comment, err := service.AddComment(context.Background(), AddCommentRequest{
				GroupID: change.GroupID, ChangeSetID: change.ID, Body: "point comment", Anchor: anchor,
			})
			if err != nil {
				t.Fatal(err)
			}
			if comment.Anchor != anchor {
				t.Fatalf("point anchor changed: %#v", comment.Anchor)
			}

			badQuote := anchor
			badQuote.Quote = "not empty"
			if _, err := service.AddComment(context.Background(), AddCommentRequest{
				GroupID: change.GroupID, ChangeSetID: change.ID, Body: "invalid point", Anchor: badQuote,
			}); err == nil {
				t.Fatal("point anchor with a non-empty quote was accepted")
			}
		})
	}
}

func TestPreparedRecoveryRecognizesAlreadyWrittenAfterState(t *testing.T) {
	service, path := newTestServiceWithFile(t, "before")
	metadata := normalizeMetadata(ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "recover-group"})
	edits := []AppliedEdit{{ID: "recover-edit", OldString: "before", NewString: "after", ReviewStatus: ReviewStatusPending, Hunks: []Hunk{{ID: "recover-hunk", BeforeStart: 0, BeforeEnd: 6, AfterStart: 0, AfterEnd: 5}}}}
	change := newChangeSet(path, []byte("before"), []byte("after"), true, true, edits, metadata)
	beforeBlob, err := service.store.writeBlob([]byte("before"))
	if err != nil {
		t.Fatal(err)
	}
	afterBlob, err := service.store.writeBlob([]byte("after"))
	if err != nil {
		t.Fatal(err)
	}
	change.BeforeBlob, change.AfterBlob = beforeBlob, afterBlob
	if err := service.appendAndApply(ledgerEvent{Type: eventChangePrepared, Metadata: &metadata, ChangeSet: &change}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.atomicWriteVisibleFile(path, []byte("after")); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewService(service.workspace)
	if err != nil {
		t.Fatal(err)
	}
	group, err := reloaded.GetGroup(context.Background(), metadata.ChangeGroupID)
	if err != nil || len(group.ChangeSets) != 1 || group.ChangeSets[0].ApplyState != ApplyStateApplied {
		t.Fatalf("prepared recovery failed: group=%#v err=%v", group, err)
	}
}

func TestPreparedRecoveryProjectsAmbiguousConflictWithoutOverwriting(t *testing.T) {
	service, path := newTestServiceWithFile(t, "before")
	metadata := normalizeMetadata(ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "conflicted-recovery-group"})
	edits := []AppliedEdit{{
		ID: "conflicted-edit", OldString: "before", NewString: "intended", ReviewStatus: ReviewStatusPending,
		Hunks: []Hunk{{ID: "conflicted-hunk", BeforeStart: 0, BeforeEnd: 6, AfterStart: 0, AfterEnd: 8}},
	}}
	change := newChangeSet(path, []byte("before"), []byte("intended"), true, true, edits, metadata)
	beforeBlob, err := service.store.writeBlob([]byte("before"))
	if err != nil {
		t.Fatal(err)
	}
	afterBlob, err := service.store.writeBlob([]byte("intended"))
	if err != nil {
		t.Fatal(err)
	}
	change.BeforeBlob, change.AfterBlob = beforeBlob, afterBlob
	if err := service.appendAndApply(ledgerEvent{Type: eventChangePrepared, Metadata: &metadata, ChangeSet: &change}); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, service.workspace, path, "independent user text")

	reloaded, err := NewService(service.workspace)
	if err != nil {
		t.Fatal(err)
	}
	group, err := reloaded.GetGroup(context.Background(), metadata.ChangeGroupID)
	if err != nil {
		t.Fatal(err)
	}
	if len(group.ChangeSets) != 1 || group.ChangeSets[0].ApplyState != ApplyStateConflicted {
		t.Fatalf("ambiguous prepared change was not projected as conflicted: %#v", group)
	}
	if group.ChangeSets[0].BeforeContent != "before" || group.ChangeSets[0].AfterContent != "intended" {
		t.Fatalf("conflicted intent was not hydrated: %#v", group.ChangeSets[0])
	}
	if group.CanUndo || group.CanRedo || group.PendingEditCount != 0 || len(reloaded.prepared) != 0 {
		t.Fatalf("conflicted projection exposed invalid history state: group=%#v prepared=%d", group, len(reloaded.prepared))
	}
	_, err = reloaded.Review(context.Background(), ReviewRequest{
		GroupID: metadata.ChangeGroupID, ChangeSetID: change.ID, Decision: ReviewDecisionAccept,
	})
	assertChangeErrorCode(t, err, ErrorCodeConflict)
	group, err = reloaded.GetGroup(context.Background(), metadata.ChangeGroupID)
	if err != nil || group.ChangeSets[0].ReviewStatus != ReviewStatusPending {
		t.Fatalf("conflicted review metadata changed: group=%#v err=%v", group, err)
	}
	if got := readTestFile(t, service.workspace, path); got != "independent user text" {
		t.Fatalf("recovery overwrote independent content: %q", got)
	}

	secondReload, err := NewService(service.workspace)
	if err != nil {
		t.Fatal(err)
	}
	group, err = secondReload.GetGroup(context.Background(), metadata.ChangeGroupID)
	if err != nil || len(group.ChangeSets) != 1 || len(secondReload.prepared) != 0 {
		t.Fatalf("conflicted recovery was not terminal: group=%#v prepared=%d err=%v", group, len(secondReload.prepared), err)
	}
}

func TestConcurrentEditsFromOneRevisionHaveOneCASWinner(t *testing.T) {
	service, path := newTestServiceWithFile(t, "left right")
	baseRevision := Revision([]byte("left right"))
	type result struct {
		edit TextEdit
		err  error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	for _, edit := range []TextEdit{{OldString: "left", NewString: "LEFT"}, {OldString: "right", NewString: "RIGHT"}} {
		edit := edit
		go func() {
			<-start
			_, err := service.ApplyEdits(context.Background(), ApplyEditsRequest{
				Path: path, BaseRevision: baseRevision, Edits: []TextEdit{edit},
			})
			results <- result{edit: edit, err: err}
		}()
	}
	close(start)
	winners := 0
	conflicts := 0
	winnerContent := ""
	for range 2 {
		result := <-results
		if result.err == nil {
			winners++
			if result.edit.OldString == "left" {
				winnerContent = "LEFT right"
			} else {
				winnerContent = "left RIGHT"
			}
			continue
		}
		var typed *Error
		if !errors.As(result.err, &typed) || typed.Code != ErrorCodeRevisionConflict {
			t.Fatalf("concurrent edit failed with unexpected error: %v", result.err)
		}
		conflicts++
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("CAS results: winners=%d conflicts=%d", winners, conflicts)
	}
	if got := readTestFile(t, service.workspace, path); got != winnerContent {
		t.Fatalf("winning CAS edit was lost: got=%q want=%q", got, winnerContent)
	}
}

func TestMutationsRequireBaseRevisionWithoutChangingWorkspace(t *testing.T) {
	service, path := newTestServiceWithFile(t, "draft")
	tests := []struct {
		name   string
		mutate func() error
	}{
		{
			name: "apply edits",
			mutate: func() error {
				_, err := service.ApplyEdits(context.Background(), ApplyEditsRequest{
					Path: path, Edits: []TextEdit{{OldString: "draft", NewString: "agent"}},
				})
				return err
			},
		},
		{
			name: "replace file",
			mutate: func() error {
				_, err := service.ReplaceFile(context.Background(), ReplaceFileRequest{Path: path, Content: "agent"})
				return err
			},
		},
		{
			name: "save file",
			mutate: func() error {
				_, err := service.SaveFile(context.Background(), path, "local", "")
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.mutate()
			var typed *Error
			if !errors.As(err, &typed) || typed.Code != ErrorCodeInvalidEdit {
				t.Fatalf("error=%v, want %s", err, ErrorCodeInvalidEdit)
			}
			mutated, ok := typed.Details["workspace_mutated"].(bool)
			if !ok || mutated {
				t.Fatalf("workspace_mutated detail = %#v", typed.Details["workspace_mutated"])
			}
			if got := readTestFile(t, service.workspace, path); got != "draft" {
				t.Fatalf("missing base revision mutated workspace: %q", got)
			}
		})
	}
	groups, err := service.ListGroups(context.Background(), ChangeFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("missing base revision recorded history: %#v", groups)
	}
}

func TestSaveFileCreatesOnlyFromExplicitMissingRevision(t *testing.T) {
	workspace := t.TempDir()
	service, err := NewService(workspace)
	if err != nil {
		t.Fatal(err)
	}
	path := "chapters/new-local.md"
	result, err := service.SaveFile(context.Background(), path, "new local draft", "missing")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || result.Revision != Revision([]byte("new local draft")) {
		t.Fatalf("unexpected create result: %#v", result)
	}
	if got := readTestFile(t, workspace, path); got != "new local draft" {
		t.Fatalf("created content = %q", got)
	}
}

func TestSaveFileUsesCASWithoutCreatingReviewHistory(t *testing.T) {
	service, path := newTestServiceWithFile(t, "draft")
	_, baseRevision, err := service.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.SaveFile(context.Background(), path, "local draft", baseRevision)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || result.Revision != Revision([]byte("local draft")) || readTestFile(t, service.workspace, path) != "local draft" {
		t.Fatalf("unexpected changed local save result %#v", result)
	}
	noOp, err := service.SaveFile(context.Background(), path, "local draft", result.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if noOp.Changed || noOp.Revision != result.Revision {
		t.Fatalf("no-op save result = %#v", noOp)
	}
	groups, err := service.ListGroups(context.Background(), ChangeFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("local save created review history: %#v", groups)
	}
	entries, err := os.ReadDir(service.store.blobDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("local save created content blobs: %#v", entries)
	}
}

func TestSaveFileAndApplyEditsCASDoNotLoseUpdates(t *testing.T) {
	service, path := newTestServiceWithFile(t, "left right")
	_, baseRevision, err := service.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		kind string
		err  error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	go func() {
		<-start
		_, err := service.SaveFile(context.Background(), path, "manual right", baseRevision)
		results <- result{kind: "save", err: err}
	}()
	go func() {
		<-start
		_, err := service.ApplyEdits(context.Background(), ApplyEditsRequest{
			Path: path, BaseRevision: baseRevision, Edits: []TextEdit{{OldString: "left", NewString: "agent"}},
			Metadata: ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "concurrent-cas-group"},
		})
		results <- result{kind: "agent", err: err}
	}()
	close(start)
	var winner string
	conflicts := 0
	for range 2 {
		result := <-results
		if result.err == nil {
			if winner != "" {
				t.Fatalf("both CAS writers succeeded: first=%s second=%s", winner, result.kind)
			}
			winner = result.kind
			continue
		}
		var typed *Error
		if !errors.As(result.err, &typed) || typed.Code != ErrorCodeRevisionConflict {
			t.Fatalf("%s writer failed with unexpected error: %v", result.kind, result.err)
		}
		conflicts++
	}
	if winner == "" || conflicts != 1 {
		t.Fatalf("winner=%q revision_conflicts=%d", winner, conflicts)
	}
	want := "manual right"
	if winner == "agent" {
		want = "agent right"
	}
	if got := readTestFile(t, service.workspace, path); got != want {
		t.Fatalf("winning CAS write was lost: winner=%s content=%q", winner, got)
	}
}

func TestReloadTruncatesOnlyTornLedgerTail(t *testing.T) {
	service, path := newTestServiceWithFile(t, "before")
	change, err := service.ApplyEdits(context.Background(), ApplyEditsRequest{
		Path: path, BaseRevision: Revision([]byte("before")),
		Edits:    []TextEdit{{OldString: "before", NewString: "after"}},
		Metadata: ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "torn-tail-group"},
	})
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(service.store.ledgerPath, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"type":"comment_upserted"`); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewService(service.workspace)
	if err != nil {
		t.Fatal(err)
	}
	group, err := reloaded.GetGroup(context.Background(), change.GroupID)
	if err != nil || len(group.ChangeSets) != 1 {
		t.Fatalf("valid events before torn tail were not recovered: group=%#v err=%v", group, err)
	}
	ledger, err := os.ReadFile(service.store.ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(ledger) == 0 || ledger[len(ledger)-1] != '\n' || strings.Contains(string(ledger), `{"type":"comment_upserted"`) {
		t.Fatalf("torn tail was not truncated: %q", ledger)
	}
	if _, err := reloaded.AddComment(context.Background(), AddCommentRequest{GroupID: change.GroupID, Body: "after repair"}); err != nil {
		t.Fatalf("ledger was not appendable after repair: %v", err)
	}
}

func TestCommentRejectsOversizedAnchorBeforeLedgerAppend(t *testing.T) {
	service, path := newTestServiceWithFile(t, "before")
	change, err := service.ApplyEdits(context.Background(), ApplyEditsRequest{
		Path: path, BaseRevision: Revision([]byte("before")),
		Edits:    []TextEdit{{OldString: "before", NewString: "after"}},
		Metadata: ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "comment-bound-group"},
	})
	if err != nil {
		t.Fatal(err)
	}
	beforeLedger, err := os.ReadFile(service.store.ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.AddComment(context.Background(), AddCommentRequest{
		GroupID: change.GroupID,
		Body:    "bounded",
		Anchor:  CommentAnchor{Quote: strings.Repeat("x", maxCommentAnchorBytes+1)},
	})
	assertChangeErrorCode(t, err, ErrorCodeConflict)
	afterLedger, err := os.ReadFile(service.store.ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterLedger) != string(beforeLedger) {
		t.Fatal("rejected oversized comment mutated the ledger")
	}
}

func newTestServiceWithFile(t *testing.T, content string) (*Service, string) {
	t.Helper()
	workspace := t.TempDir()
	path := "chapters/ch01.md"
	writeTestFile(t, workspace, path, content)
	service, err := NewService(workspace)
	if err != nil {
		t.Fatal(err)
	}
	return service, path
}

func writeTestFile(t *testing.T, workspace, path, content string) {
	t.Helper()
	abs := filepath.Join(workspace, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, workspace, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func assertChangeErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	var typed *Error
	if !errors.As(err, &typed) || typed.Code != code {
		encoded, _ := json.Marshal(err)
		t.Fatalf("error code = %q, want %q; err=%v json=%s", func() string {
			if typed == nil {
				return ""
			}
			return typed.Code
		}(), code, err, encoded)
	}
}
