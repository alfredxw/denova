package workspacechange

import (
	"context"
	"reflect"
	"testing"
)

func TestReviewResultReportsOnlyPathsChangedByThisDecision(t *testing.T) {
	workspace := t.TempDir()
	writeTestFile(t, workspace, "chapters/one.md", "one draft")
	writeTestFile(t, workspace, "settings/world.md", "world draft")
	service, err := NewService(workspace)
	if err != nil {
		t.Fatal(err)
	}
	metadata := ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "selective-review-receipt"}
	chapter, err := service.ReplaceFile(context.Background(), ReplaceFileRequest{
		Path: "chapters/one.md", Content: "one agent", BaseRevision: Revision([]byte("one draft")), Metadata: metadata,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReplaceFile(context.Background(), ReplaceFileRequest{
		Path: "settings/world.md", Content: "world agent", BaseRevision: Revision([]byte("world draft")), Metadata: metadata,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := service.ReviewWithResult(context.Background(), ReviewRequest{
		GroupID: metadata.ChangeGroupID, ChangeSetID: chapter.ID, Decision: ReviewDecisionReject,
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"chapters/one.md"}; !reflect.DeepEqual(result.AffectedPaths, want) {
		t.Fatalf("affected paths = %#v, want %#v", result.AffectedPaths, want)
	}
	if len(uniqueGroupPaths(result.Group.ChangeSets)) != 2 {
		t.Fatalf("receipt was inferred from incomplete group history: %#v", result.Group.ChangeSets)
	}
	if got := readTestFile(t, workspace, "chapters/one.md"); got != "one draft" {
		t.Fatalf("chapter content = %q", got)
	}
	if got := readTestFile(t, workspace, "settings/world.md"); got != "world agent" {
		t.Fatalf("unselected setting content = %q", got)
	}
}

func TestRejectAllPreservesAlreadyAcceptedEdits(t *testing.T) {
	service, path := newTestServiceWithFile(t, "one two")
	change, err := service.ApplyEdits(context.Background(), ApplyEditsRequest{
		Path:         path,
		BaseRevision: Revision([]byte("one two")),
		Edits: []TextEdit{
			{ID: "one", OldString: "one", NewString: "ONE"},
			{ID: "two", OldString: "two", NewString: "TWO"},
		},
		Metadata: ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "terminal-review-group"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Review(context.Background(), ReviewRequest{
		GroupID: change.GroupID, Decision: ReviewDecisionAccept, EditIDs: []string{"one"},
	}); err != nil {
		t.Fatal(err)
	}
	group, err := service.Review(context.Background(), ReviewRequest{
		GroupID: change.GroupID, Decision: ReviewDecisionReject,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, service.workspace, path); got != "ONE two" {
		t.Fatalf("reject all rewrote an accepted edit: %q", got)
	}
	if group.PendingEditCount != 0 || group.ReviewStatus != ReviewStatusMixed || !group.CanUndo {
		t.Fatalf("unexpected fully reviewed mixed projection: %#v", group)
	}
}

func TestReviewAllSkipsTerminalRevertedChangeSets(t *testing.T) {
	for _, decision := range []string{ReviewDecisionAccept, ReviewDecisionReject} {
		decision := decision
		t.Run(decision, func(t *testing.T) {
			workspace := t.TempDir()
			writeTestFile(t, workspace, "chapters/one.md", "one draft")
			writeTestFile(t, workspace, "chapters/two.md", "two draft")
			service, err := NewService(workspace)
			if err != nil {
				t.Fatal(err)
			}
			metadata := ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "multi-set-terminal-group"}
			first, err := service.ReplaceFile(context.Background(), ReplaceFileRequest{
				Path: "chapters/one.md", Content: "one agent", BaseRevision: Revision([]byte("one draft")), Metadata: metadata,
			})
			if err != nil {
				t.Fatal(err)
			}
			second, err := service.ReplaceFile(context.Background(), ReplaceFileRequest{
				Path: "chapters/two.md", Content: "two agent", BaseRevision: Revision([]byte("two draft")), Metadata: metadata,
			})
			if err != nil {
				t.Fatal(err)
			}
			group, err := service.Review(context.Background(), ReviewRequest{
				GroupID: metadata.ChangeGroupID, ChangeSetID: first.ID, Decision: ReviewDecisionReject,
			})
			if err != nil {
				t.Fatal(err)
			}
			if group.PendingEditCount != 1 {
				t.Fatalf("remaining applied set was not pending: %#v", group)
			}
			group, err = service.Review(context.Background(), ReviewRequest{GroupID: metadata.ChangeGroupID, Decision: decision})
			if err != nil {
				t.Fatal(err)
			}
			if group.PendingEditCount != 0 || readTestFile(t, workspace, "chapters/one.md") != "one draft" {
				t.Fatalf("terminal reverted set was not skipped: %#v", group)
			}
			wantSecond := "two agent"
			wantStatus := ReviewStatusAccepted
			if decision == ReviewDecisionReject {
				wantSecond = "two draft"
				wantStatus = ReviewStatusRejected
			}
			if got := readTestFile(t, workspace, "chapters/two.md"); got != wantSecond {
				t.Fatalf("remaining set content = %q, want %q", got, wantSecond)
			}
			var firstStatus, secondStatus string
			for _, change := range group.ChangeSets {
				switch change.ID {
				case first.ID:
					firstStatus = change.ReviewStatus
				case second.ID:
					secondStatus = change.ReviewStatus
				}
			}
			if firstStatus != ReviewStatusRejected || secondStatus != wantStatus {
				t.Fatalf("terminal statuses changed: first=%q second=%q", firstStatus, secondStatus)
			}
		})
	}
}

func TestReviewRejectRelocatesUniqueTextAfterExternalChange(t *testing.T) {
	service, path := newTestServiceWithFile(t, "one two")
	change, err := service.ApplyEdits(context.Background(), ApplyEditsRequest{
		Path: path, BaseRevision: Revision([]byte("one two")), Edits: []TextEdit{{ID: "one", OldString: "one", NewString: "ONE"}},
		Metadata: ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "relocate-group"},
	})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, service.workspace, path, "preface ONE two")
	group, err := service.Review(context.Background(), ReviewRequest{
		GroupID: change.GroupID, ChangeSetID: change.ID, Decision: ReviewDecisionReject, EditIDs: []string{"one"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, service.workspace, path); got != "preface one two" {
		t.Fatalf("relocated rejection content = %q", got)
	}
	if group.ReviewStatus != ReviewStatusRejected || group.ApplyState != ApplyStateReverted {
		t.Fatalf("unexpected relocated review projection: %#v", group)
	}
}

func TestPartialReviewRejectUndoRedoUsesReviewedState(t *testing.T) {
	service, path := newTestServiceWithFile(t, "one two three")
	change, err := service.ApplyEdits(context.Background(), ApplyEditsRequest{
		Path:         path,
		BaseRevision: Revision([]byte("one two three")),
		Edits: []TextEdit{
			{ID: "one", OldString: "one", NewString: "ONE"},
			{ID: "three", OldString: "three", NewString: "THREE"},
		},
		Metadata: ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "review-history-group"},
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
	if !group.CanUndo || group.CanRedo {
		t.Fatalf("partial review should remain undoable: %#v", group)
	}
	group, err = service.Undo(context.Background(), HistoryRequest{GroupID: change.GroupID})
	if err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, service.workspace, path); got != "one two three" {
		t.Fatalf("undo did not restore pre-Agent state: %q", got)
	}
	if group.CanUndo || !group.CanRedo {
		t.Fatalf("undone reviewed group has incorrect capabilities: %#v", group)
	}
	group, err = service.Redo(context.Background(), HistoryRequest{GroupID: change.GroupID})
	if err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, service.workspace, path); got != "one two THREE" {
		t.Fatalf("redo did not restore reviewed state: %q", got)
	}
	if !group.CanUndo || group.CanRedo {
		t.Fatalf("redone reviewed group has incorrect capabilities: %#v", group)
	}
}

func TestReviewRejectsChangesOutsideAppliedState(t *testing.T) {
	service, path := newTestServiceWithFile(t, "draft")
	change, err := service.ReplaceFile(context.Background(), ReplaceFileRequest{
		Path: path, Content: "agent draft", BaseRevision: Revision([]byte("draft")), Metadata: ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "undone-review-group"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Undo(context.Background(), HistoryRequest{GroupID: change.GroupID}); err != nil {
		t.Fatal(err)
	}
	group, err := service.GetGroup(context.Background(), change.GroupID)
	if err != nil || group.PendingEditCount != 0 {
		t.Fatalf("undone edits must not remain reviewable: group=%#v err=%v", group, err)
	}
	for _, decision := range []string{ReviewDecisionAccept, ReviewDecisionReject} {
		_, err := service.Review(context.Background(), ReviewRequest{
			GroupID: change.GroupID, ChangeSetID: change.ID, Decision: decision,
		})
		assertChangeErrorCode(t, err, ErrorCodeConflict)
	}
	if got := readTestFile(t, service.workspace, path); got != "draft" {
		t.Fatalf("review mutated an undone change: %q", got)
	}
}

func TestFullyRejectedGroupHasNoHistoryAction(t *testing.T) {
	service, path := newTestServiceWithFile(t, "draft")
	change, err := service.ApplyEdits(context.Background(), ApplyEditsRequest{
		Path: path, BaseRevision: Revision([]byte("draft")), Edits: []TextEdit{{ID: "draft", OldString: "draft", NewString: "published"}},
		Metadata: ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "fully-rejected-group"},
	})
	if err != nil {
		t.Fatal(err)
	}
	group, err := service.Review(context.Background(), ReviewRequest{
		GroupID: change.GroupID, Decision: ReviewDecisionReject,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, service.workspace, path); got != "draft" {
		t.Fatalf("full reject did not restore base: %q", got)
	}
	if group.CanUndo || group.CanRedo {
		t.Fatalf("net-zero rejected group should have no history action: %#v", group)
	}
	_, err = service.Undo(context.Background(), HistoryRequest{GroupID: change.GroupID})
	assertChangeErrorCode(t, err, ErrorCodeConflict)
	_, err = service.Redo(context.Background(), HistoryRequest{GroupID: change.GroupID})
	assertChangeErrorCode(t, err, ErrorCodeNoRedo)
	summaries, err := service.ListGroups(context.Background(), ChangeFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 || summaries[0].CanUndo || summaries[0].CanRedo {
		t.Fatalf("summary exposed an invalid history action: %#v", summaries)
	}
}

func TestReviewRejectsSamePathTimelineWithOneMergedWrite(t *testing.T) {
	service, path := newTestServiceWithFile(t, "one two")
	metadata := ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "same-path-review-group"}
	for _, request := range []ApplyEditsRequest{
		{Path: path, Edits: []TextEdit{{ID: "one", OldString: "one", NewString: "ONE"}}, Metadata: metadata},
		{Path: path, Edits: []TextEdit{{ID: "two", OldString: "two", NewString: "TWO"}}, Metadata: metadata},
		{Path: path, Edits: []TextEdit{{ID: "dependent", OldString: "ONE", NewString: "Uno"}}, Metadata: metadata},
	} {
		_, revision, err := service.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		request.BaseRevision = revision
		if _, err := service.ApplyEdits(context.Background(), request); err != nil {
			t.Fatal(err)
		}
	}
	group, err := service.Review(context.Background(), ReviewRequest{
		GroupID: metadata.ChangeGroupID, Decision: ReviewDecisionReject,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, service.workspace, path); got != "one two" {
		t.Fatalf("same-path reject did not restore the base timeline: %q", got)
	}
	reviewChanges := 0
	agentChanges := 0
	for _, change := range group.ChangeSets {
		switch change.Origin {
		case OriginReview:
			reviewChanges++
		case OriginAgent:
			agentChanges++
			if change.ReviewStatus != ReviewStatusRejected || change.ApplyState != ApplyStateReverted {
				t.Fatalf("original change was not fully rejected: %#v", change)
			}
		}
	}
	if agentChanges != 3 || reviewChanges != 1 {
		t.Fatalf("same path should produce one merged inverse: agents=%d reviews=%d changes=%#v", agentChanges, reviewChanges, group.ChangeSets)
	}
	if group.CanUndo || group.CanRedo || group.ReviewStatus != ReviewStatusRejected {
		t.Fatalf("fully restored timeline has incorrect projection: %#v", group)
	}
}

func TestCommitRevalidatesBaseBeforeWriting(t *testing.T) {
	service, path := newTestServiceWithFile(t, "before")
	metadata := normalizeMetadata(ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "commit-cas-group"})
	edits := []AppliedEdit{{
		ID: "cas-edit", OldString: "before", NewString: "after", ReviewStatus: ReviewStatusPending,
		Hunks: []Hunk{{ID: "cas-hunk", BeforeStart: 0, BeforeEnd: 6, AfterStart: 0, AfterEnd: 5}},
	}}
	change := newChangeSet(path, []byte("before"), []byte("after"), true, true, edits, metadata)
	writeTestFile(t, service.workspace, path, "external")
	service.mu.Lock()
	err := service.commitChangeLocked(context.Background(), &change, []byte("before"), []byte("after"), metadata)
	service.mu.Unlock()
	assertChangeErrorCode(t, err, ErrorCodeRevisionConflict)
	if got := readTestFile(t, service.workspace, path); got != "external" {
		t.Fatalf("stale prepared change overwrote external content: %q", got)
	}
	groups, listErr := service.ListGroups(context.Background(), ChangeFilter{})
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(groups) != 0 {
		t.Fatalf("failed CAS created a review group: %#v", groups)
	}
}

func TestUndoRefusesToEraseInterveningWriteInsideOneGroup(t *testing.T) {
	service, path := newTestServiceWithFile(t, "zero")
	metadata := ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "non-contiguous-group"}
	first, err := service.ApplyEdits(context.Background(), ApplyEditsRequest{
		Path: path, BaseRevision: Revision([]byte("zero")),
		Edits: []TextEdit{{OldString: "zero", NewString: "one"}}, Metadata: metadata,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveFile(context.Background(), path, "two", first.Revision); err != nil {
		t.Fatal(err)
	}
	second, err := service.ApplyEdits(context.Background(), ApplyEditsRequest{
		Path: path, BaseRevision: Revision([]byte("two")),
		Edits: []TextEdit{{OldString: "two", NewString: "three"}}, Metadata: metadata,
	})
	if err != nil {
		t.Fatal(err)
	}
	group, err := service.GetGroup(context.Background(), second.GroupID)
	if err != nil {
		t.Fatal(err)
	}
	if group.CanUndo {
		t.Fatalf("non-contiguous group must not advertise destructive undo: %#v", group)
	}
	_, err = service.Undo(context.Background(), HistoryRequest{GroupID: second.GroupID})
	assertChangeErrorCode(t, err, ErrorCodeConflict)
	if got := readTestFile(t, service.workspace, path); got != "three" {
		t.Fatalf("refused undo changed workspace content: %q", got)
	}
}
