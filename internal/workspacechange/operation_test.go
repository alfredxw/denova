package workspacechange

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

var errSimulatedOperationCrash = errors.New("simulated group operation crash")

func TestOperationLedgerSanitizesNestedChangeContentAndRehydratesFromBlobs(t *testing.T) {
	workspace := t.TempDir()
	path := "chapter.md"
	before := "private-before-body-4e9fd9"
	after := "private-after-body-8c32aa"
	writeTestFile(t, workspace, path, before)
	service, err := NewService(workspace)
	if err != nil {
		t.Fatal(err)
	}
	change, err := service.ReplaceFile(context.Background(), ReplaceFileRequest{
		Path: path, Content: after, BaseRevision: Revision([]byte(before)),
		Metadata: ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "sanitized-operation"},
	})
	if err != nil {
		t.Fatal(err)
	}
	service.groupOperationHook = func(stage, _ string) error {
		if stage == operationStageAfterPrepare {
			return errSimulatedOperationCrash
		}
		return nil
	}
	if _, err := service.Undo(context.Background(), HistoryRequest{GroupID: change.GroupID}); !errors.Is(err, errSimulatedOperationCrash) {
		t.Fatalf("undo error=%v, want injected failure", err)
	}
	ledger, err := os.ReadFile(service.store.ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(ledger), before) || strings.Contains(string(ledger), after) {
		t.Fatalf("operation ledger inlined workspace content: %s", ledger)
	}

	reloaded, err := NewService(workspace)
	if err != nil {
		t.Fatalf("sanitized operation did not replay from blobs: %v", err)
	}
	if got := readTestFile(t, workspace, path); got != before {
		t.Fatalf("replayed undo content=%q, want %q", got, before)
	}
	group, err := reloaded.GetGroup(context.Background(), change.GroupID)
	if err != nil {
		t.Fatal(err)
	}
	foundHydrated := false
	for _, projected := range group.ChangeSets {
		if projected.BeforeContent == after && projected.AfterContent == before {
			foundHydrated = true
			break
		}
	}
	if !foundHydrated {
		t.Fatalf("operation content was not rehydrated from blobs: %#v", group.ChangeSets)
	}
}

func TestOperationReplayIsIdempotentAndRejectsContradictoryTerminal(t *testing.T) {
	service, groupID := newMultiPathChangeGroup(t, "idempotent-operation-replay")
	service.groupOperationHook = func(stage, _ string) error {
		if stage == operationStageBeforeCommit {
			return errSimulatedOperationCrash
		}
		return nil
	}
	if _, err := service.Undo(context.Background(), HistoryRequest{GroupID: groupID}); !errors.Is(err, errSimulatedOperationCrash) {
		t.Fatalf("undo error=%v, want injected failure", err)
	}
	var operationID, operationPath string
	for id, prepared := range service.operations {
		operationID = id
		operationPath = prepared.Operation.Changes[0].ChangeSet.Path
	}
	if operationID == "" {
		t.Fatal("prepared operation was not retained")
	}
	if err := service.store.append(ledgerEvent{
		Type: eventOperationPathApplied, OperationID: operationID, OperationPath: operationPath,
	}); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewService(service.workspace)
	if err != nil {
		t.Fatalf("duplicate path progress broke replay: %v", err)
	}
	if err := reloaded.store.append(ledgerEvent{Type: eventOperationCommitted, OperationID: operationID}); err != nil {
		t.Fatal(err)
	}
	reloaded, err = NewService(service.workspace)
	if err != nil {
		t.Fatalf("duplicate committed terminal broke replay: %v", err)
	}
	if err := reloaded.store.append(ledgerEvent{Type: eventOperationConflicted, OperationID: operationID}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewService(service.workspace); err == nil || !strings.Contains(err.Error(), "conflicts with terminal state") {
		t.Fatalf("contradictory operation terminal did not fail closed: %v", err)
	}
}

func TestConflictedOperationReplayAcceptsDuplicateSameTerminal(t *testing.T) {
	service, groupID := newMultiPathChangeGroup(t, "idempotent-conflict-replay")
	service.groupOperationHook = func(stage, _ string) error {
		if stage == operationStageAfterPrepare {
			return errSimulatedOperationCrash
		}
		return nil
	}
	if _, err := service.Undo(context.Background(), HistoryRequest{GroupID: groupID}); !errors.Is(err, errSimulatedOperationCrash) {
		t.Fatalf("undo error=%v, want injected failure", err)
	}
	operationID := ""
	for id := range service.operations {
		operationID = id
	}
	for range 2 {
		if err := service.store.append(ledgerEvent{
			Type: eventOperationConflicted, OperationID: operationID, ConflictPaths: []string{"chapters/a.md"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := NewService(service.workspace); err != nil {
		t.Fatalf("duplicate conflicted terminal broke replay: %v", err)
	}
}

func TestChangeTerminalReplayIsIdempotentAndRejectsContradiction(t *testing.T) {
	service, path := newTestServiceWithFile(t, "before")
	change, err := service.ReplaceFile(context.Background(), ReplaceFileRequest{
		Path: path, Content: "after", BaseRevision: Revision([]byte("before")),
		Metadata: ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "change-terminal-replay"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.store.append(ledgerEvent{Type: eventChangeApplied, ChangeSetID: change.ID}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewService(service.workspace)
	if err != nil {
		t.Fatalf("duplicate applied terminal broke replay: %v", err)
	}
	if err := reloaded.store.append(ledgerEvent{Type: eventChangeConflicted, ChangeSetID: change.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewService(service.workspace); err == nil {
		t.Fatal("contradictory change terminal did not fail closed")
	}
}

func TestGroupOperationIgnoresCancellationAfterPrepare(t *testing.T) {
	service, groupID := newMultiPathChangeGroup(t, "cancel-after-prepare")
	ctx, cancel := context.WithCancel(context.Background())
	pathCount := 0
	service.groupOperationHook = func(stage, _ string) error {
		if stage == operationStageAfterPathVisible {
			pathCount++
			if pathCount == 1 {
				cancel()
			}
		}
		return nil
	}

	group, err := service.Undo(ctx, HistoryRequest{GroupID: groupID})
	if err != nil {
		t.Fatalf("durable undo was interrupted after prepare: %v", err)
	}
	if ctx.Err() == nil || pathCount != 2 {
		t.Fatalf("cancellation hook did not run across both paths: ctx=%v paths=%d", ctx.Err(), pathCount)
	}
	assertMultiPathContent(t, service.workspace, "a draft", "b draft")
	if group.CanUndo || !group.CanRedo || len(service.operations) != 0 {
		t.Fatalf("completed operation has incorrect projection: %#v operations=%d", group, len(service.operations))
	}
}

func TestGroupOperationRecoversFailureAfterFirstPath(t *testing.T) {
	service, groupID := newMultiPathChangeGroup(t, "recover-first-path")
	pathCount := 0
	service.groupOperationHook = func(stage, _ string) error {
		if stage != operationStageAfterPathVisible {
			return nil
		}
		pathCount++
		if pathCount == 1 {
			return errSimulatedOperationCrash
		}
		return nil
	}

	_, err := service.Undo(context.Background(), HistoryRequest{GroupID: groupID})
	if !errors.Is(err, errSimulatedOperationCrash) {
		t.Fatalf("undo error = %v, want simulated crash", err)
	}
	assertMultiPathContent(t, service.workspace, "a draft", "b agent")
	if len(service.operations) != 1 {
		t.Fatalf("prepared operation was not retained after partial visibility: %d", len(service.operations))
	}
	for _, prepared := range service.operations {
		if len(prepared.AppliedPaths) != 0 {
			t.Fatalf("simulated crash should precede durable path progress: %#v", prepared.AppliedPaths)
		}
	}

	reloaded, err := NewService(service.workspace)
	if err != nil {
		t.Fatalf("reload did not roll operation forward: %v", err)
	}
	assertMultiPathContent(t, service.workspace, "a draft", "b draft")
	group, err := reloaded.GetGroup(context.Background(), groupID)
	if err != nil {
		t.Fatal(err)
	}
	if group.CanUndo || !group.CanRedo || len(reloaded.operations) != 0 {
		t.Fatalf("recovered undo projection is incomplete: %#v operations=%d", group, len(reloaded.operations))
	}

	redone, err := reloaded.Redo(context.Background(), HistoryRequest{GroupID: groupID})
	if err != nil {
		t.Fatalf("multi-path redo failed after recovery: %v", err)
	}
	assertMultiPathContent(t, service.workspace, "a agent", "b agent")
	if !redone.CanUndo || redone.CanRedo {
		t.Fatalf("redo projection is incorrect: %#v", redone)
	}
}

func TestReviewOperationRecoversAfterFilesBeforeProjection(t *testing.T) {
	service, groupID := newMultiPathChangeGroup(t, "review-before-projection")
	service.groupOperationHook = func(stage, _ string) error {
		if stage == operationStageBeforeCommit {
			return errSimulatedOperationCrash
		}
		return nil
	}

	_, err := service.Review(context.Background(), ReviewRequest{
		GroupID:  groupID,
		Decision: ReviewDecisionReject,
	})
	if !errors.Is(err, errSimulatedOperationCrash) {
		t.Fatalf("review error = %v, want simulated crash", err)
	}
	assertMultiPathContent(t, service.workspace, "a draft", "b draft")
	if len(service.operations) != 1 {
		t.Fatalf("review intent was not retained before final projection: %d", len(service.operations))
	}
	for _, prepared := range service.operations {
		if len(prepared.Operation.Changes) != 2 || len(prepared.Operation.Projection.ReviewUpdates) != 2 {
			t.Fatalf("prepared review did not contain the complete intent: %#v", prepared.Operation)
		}
	}

	reloaded, err := NewService(service.workspace)
	if err != nil {
		t.Fatalf("reload did not finalize visible review: %v", err)
	}
	group, err := reloaded.GetGroup(context.Background(), groupID)
	if err != nil {
		t.Fatal(err)
	}
	if group.ReviewStatus != ReviewStatusRejected || group.ApplyState != ApplyStateReverted || group.PendingEditCount != 0 {
		t.Fatalf("review projection was not atomically finalized: %#v", group)
	}
	if len(reloaded.operations) != 0 {
		t.Fatalf("finalized review retained prepared operations: %d", len(reloaded.operations))
	}
}

func TestReviewAcceptProjectionRecoversWithoutVisiblePaths(t *testing.T) {
	service, groupID := newMultiPathChangeGroup(t, "accept-projection-recovery")
	service.groupOperationHook = func(stage, _ string) error {
		if stage == operationStageAfterPrepare {
			return errSimulatedOperationCrash
		}
		return nil
	}
	if _, err := service.Review(context.Background(), ReviewRequest{
		GroupID: groupID, Decision: ReviewDecisionAccept,
	}); !errors.Is(err, errSimulatedOperationCrash) {
		t.Fatalf("review accept error = %v, want simulated crash", err)
	}
	assertMultiPathContent(t, service.workspace, "a agent", "b agent")

	reloaded, err := NewService(service.workspace)
	if err != nil {
		t.Fatal(err)
	}
	group, err := reloaded.GetGroup(context.Background(), groupID)
	if err != nil {
		t.Fatal(err)
	}
	if group.ReviewStatus != ReviewStatusAccepted || group.PendingEditCount != 0 || len(reloaded.operations) != 0 {
		t.Fatalf("accept-only projection was not recovered: %#v operations=%d", group, len(reloaded.operations))
	}
}

func TestGroupOperationRecoveryDoesNotOverwriteDivergentPath(t *testing.T) {
	service, groupID := newMultiPathChangeGroup(t, "divergent-recovery")
	pathCount := 0
	service.groupOperationHook = func(stage, _ string) error {
		if stage == operationStageAfterPathVisible {
			pathCount++
			if pathCount == 1 {
				return errSimulatedOperationCrash
			}
		}
		return nil
	}
	if _, err := service.Undo(context.Background(), HistoryRequest{GroupID: groupID}); !errors.Is(err, errSimulatedOperationCrash) {
		t.Fatalf("undo error = %v, want simulated crash", err)
	}
	writeTestFile(t, service.workspace, "chapters/b.md", "external b")

	reloaded, err := NewService(service.workspace)
	if err != nil {
		t.Fatalf("divergent recovery should project a conflict, not fail startup: %v", err)
	}
	assertMultiPathContent(t, service.workspace, "a draft", "external b")
	group, err := reloaded.GetGroup(context.Background(), groupID)
	if err != nil {
		t.Fatal(err)
	}
	if group.ApplyState != ApplyStateConflicted || group.CanUndo || group.CanRedo || len(reloaded.operations) != 0 {
		t.Fatalf("divergent operation was not terminally conflicted: %#v operations=%d", group, len(reloaded.operations))
	}
}

func newMultiPathChangeGroup(t *testing.T, groupID string) (*Service, string) {
	t.Helper()
	workspace := t.TempDir()
	writeTestFile(t, workspace, "chapters/a.md", "a draft")
	writeTestFile(t, workspace, "chapters/b.md", "b draft")
	service, err := NewService(workspace)
	if err != nil {
		t.Fatal(err)
	}
	metadata := ChangeMetadata{Origin: OriginAgent, ChangeGroupID: groupID}
	for _, input := range []struct {
		path   string
		before string
		after  string
	}{
		{path: "chapters/a.md", before: "a draft", after: "a agent"},
		{path: "chapters/b.md", before: "b draft", after: "b agent"},
	} {
		if _, err := service.ReplaceFile(context.Background(), ReplaceFileRequest{
			Path: input.path, Content: input.after, BaseRevision: Revision([]byte(input.before)), Metadata: metadata,
		}); err != nil {
			t.Fatal(err)
		}
	}
	return service, groupID
}

func assertMultiPathContent(t *testing.T, workspace, wantA, wantB string) {
	t.Helper()
	if got := readTestFile(t, workspace, "chapters/a.md"); got != wantA {
		t.Fatalf("chapters/a.md = %q, want %q", got, wantA)
	}
	if got := readTestFile(t, workspace, "chapters/b.md"); got != wantB {
		t.Fatalf("chapters/b.md = %q, want %q", got, wantB)
	}
}
