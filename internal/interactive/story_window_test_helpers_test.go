package interactive

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func appendModelInvisibleStoryEvents(t *testing.T, store *Store, storyID, branchID string, count int) {
	t.Helper()
	store.mu.Lock()
	defer store.mu.Unlock()
	release, err := store.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	meta, _, err := store.readStoryRecentLocked(storyID, branchID)
	if err != nil {
		t.Fatal(err)
	}
	branch := meta.Branches[branchID]
	now := time.Now().UTC().Format(time.RFC3339Nano)
	events := make([]any, count)
	for index := range count {
		events[index] = HotChoicesEvent{
			V: schemaVersion, Type: StoryEventTypeHotChoices, ID: fmt.Sprintf("window-side-%04d", index),
			ParentID: branch.Head, BranchID: branchID, Ts: now, Choices: []string{"continue"},
		}
	}
	meta.UpdatedAt = now
	if err := store.appendStoryTransactionLocked(storyID, meta, events...); err != nil {
		t.Fatal(err)
	}
}

// appendStoryTurns batches a canonical test history into one durable
// transaction. It preserves the same parent chain and event schema as
// AppendTurn while avoiding one fsync per fixture row.
func appendStoryTurns(t *testing.T, store *Store, storyID, branchID string, requests []AppendTurnRequest) {
	t.Helper()
	store.mu.Lock()
	defer store.mu.Unlock()
	release, err := store.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	meta, lines, err := store.readStoryRecentLocked(storyID, branchID)
	if err != nil {
		t.Fatal(err)
	}
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		t.Fatalf("branch does not exist: %s", branchID)
	}
	if branchIsTerminal(lines, branch.Head) {
		t.Fatalf("branch is terminal: %s", branchID)
	}
	events := make([]any, 0, len(requests))
	for _, request := range requests {
		if request.BranchID != "" && request.BranchID != branchID {
			t.Fatalf("fixture request branch=%q want=%q", request.BranchID, branchID)
		}
		parentID := any(nil)
		if branch.Head != "" {
			parentID = branch.Head
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		event := TurnEvent{
			V: schemaVersion, Type: StoryEventTypeTurn, ID: newID("ev"),
			ParentID: parentID, BranchID: branchID, Ts: now,
			User: request.User, Narrative: request.Narrative,
			Thinking:             strings.TrimSpace(request.Thinking),
			DisplayEvents:        sanitizeDisplayEvents(request.DisplayEvents),
			ModelContextMessages: sanitizeModelContextMessages(request.ModelContextMessages),
			Flags:                map[string]bool{"pinned": false, "locked": false},
		}
		branch.Head = event.ID
		events = append(events, event)
		meta.UpdatedAt = now
	}
	meta.Branches[branchID] = branch
	if err := store.appendStoryTransactionLocked(storyID, meta, events...); err != nil {
		t.Fatal(err)
	}
	if err := store.syncStorySummaryLocked(storyID); err != nil {
		t.Fatal(err)
	}
}
