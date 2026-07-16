package workspacechange

import (
	"context"
	"strings"
	"testing"
)

func TestReviewRejectDoesNotRelocateToDifferentIdenticalOccurrence(t *testing.T) {
	service, path := newTestServiceWithFile(t, "first one | second ONE")
	change, err := service.ApplyEdits(context.Background(), ApplyEditsRequest{
		Path:         path,
		BaseRevision: Revision([]byte("first one | second ONE")),
		Edits:        []TextEdit{{ID: "first", OldString: "one", NewString: "ONE"}},
		Metadata:     ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "reject-identity-group"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// The agent-modified occurrence is gone. A literal search sees one remaining
	// "ONE", but it belongs to a different part of the document and must not be
	// reverted.
	const external = "second ONE"
	writeTestFile(t, service.workspace, path, external)
	_, err = service.Review(context.Background(), ReviewRequest{
		GroupID: change.GroupID, ChangeSetID: change.ID,
		Decision: ReviewDecisionReject, EditIDs: []string{"first"},
	})
	assertChangeErrorCode(t, err, ErrorCodeRevisionConflict)
	if got := readTestFile(t, service.workspace, path); got != external {
		t.Fatalf("failed rejection modified a different occurrence: got %q want %q", got, external)
	}
	group, getErr := service.GetGroup(context.Background(), change.GroupID)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if group.PendingEditCount != 1 || group.ChangeSets[0].ReviewStatus != ReviewStatusPending {
		t.Fatalf("failed rejection changed review projection: %#v", group)
	}
}

func TestChangeProjectionStaysSanitizedAndRetrievalHydratesOnDemand(t *testing.T) {
	service, path := newTestServiceWithFile(t, "private before body")
	change, err := service.ApplyEdits(context.Background(), ApplyEditsRequest{
		Path:         path,
		BaseRevision: Revision([]byte("private before body")),
		Edits:        []TextEdit{{ID: "body", OldString: "before", NewString: "after"}},
		Metadata:     ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "bounded-projection-group"},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSanitizedProjection(t, service, change.ID)
	assertHydratedGroup(t, service, change.GroupID)

	reloaded, err := NewService(service.workspace)
	if err != nil {
		t.Fatal(err)
	}
	assertSanitizedProjection(t, reloaded, change.ID)
	assertHydratedGroup(t, reloaded, change.GroupID)
}

func TestReplayRejectsUnknownLedgerEventType(t *testing.T) {
	service, _ := newTestServiceWithFile(t, "draft")
	const unknown = "future_event_without_projection_support"
	if err := service.store.append(ledgerEvent{Type: unknown}); err != nil {
		t.Fatal(err)
	}
	_, err := NewService(service.workspace)
	if err == nil || !strings.Contains(err.Error(), unknown) {
		t.Fatalf("unknown ledger event did not fail replay loudly: %v", err)
	}
}

func assertSanitizedProjection(t *testing.T, service *Service, changeID string) {
	t.Helper()
	projected := service.changeSets[changeID]
	if projected == nil {
		t.Fatalf("projected change %q not found", changeID)
	}
	if projected.BeforeContent != "" || projected.AfterContent != "" ||
		len(projected.Edits) != 1 || projected.Edits[0].OldString != "" || projected.Edits[0].NewString != "" {
		t.Fatalf("in-memory projection retained file bodies: %#v", projected)
	}
	group := service.groups[projected.GroupID]
	if group == nil || len(group.ChangeSets) != 1 ||
		group.ChangeSets[0].BeforeContent != "" || group.ChangeSets[0].AfterContent != "" ||
		group.ChangeSets[0].Edits[0].OldString != "" || group.ChangeSets[0].Edits[0].NewString != "" {
		t.Fatalf("group projection retained file bodies: %#v", group)
	}
}

func assertHydratedGroup(t *testing.T, service *Service, groupID string) {
	t.Helper()
	group, err := service.GetGroup(context.Background(), groupID)
	if err != nil {
		t.Fatal(err)
	}
	if len(group.ChangeSets) != 1 ||
		group.ChangeSets[0].BeforeContent != "private before body" ||
		group.ChangeSets[0].AfterContent != "private after body" ||
		group.ChangeSets[0].Edits[0].OldString != "before" ||
		group.ChangeSets[0].Edits[0].NewString != "after" {
		t.Fatalf("public group retrieval was not hydrated: %#v", group.ChangeSets)
	}
}
