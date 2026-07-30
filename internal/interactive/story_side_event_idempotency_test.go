package interactive

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestToolResultCleanupCanonicalCommitSurvivesAndRepairsIndexFailure(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "cleanup projection repair", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "inspect", Narrative: "done"})
	if err != nil {
		t.Fatal(err)
	}
	intent := cleanupIdempotencyFixture("cleanup-index-repair", turn.ID)
	breakStoryIndexWithDirectory(t, store)

	committed, err := store.AppendToolResultCleanup(story.ID, "main", intent)
	if err != nil {
		t.Fatalf("durable cleanup reported derived-index failure: %v", err)
	}
	if committed.ID != intent.ID || countCanonicalStoryEventID(t, store, story.ID, intent.ID) != 1 {
		t.Fatalf("canonical cleanup was not committed exactly once: %#v", committed)
	}
	if err := os.Remove(store.indexPath()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendToolResultCleanup(story.ID, "main", intent); err != nil {
		t.Fatalf("exact cleanup retry did not repair derived index: %v", err)
	}
	assertStoryIndexProjection(t, store, story.ID, 2)
}

func TestContextCompactionHealthCanonicalCommitSurvivesAndRepairsIndexFailure(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "health projection repair", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "inspect", Narrative: "done"}); err != nil {
		t.Fatal(err)
	}
	revision, _, _, err := store.ContextCompactionHealthState(story.ID, "main", "interactive_story")
	if err != nil {
		t.Fatal(err)
	}
	intent := healthIdempotencyFixture("health-index-repair", revision, "repair")
	breakStoryIndexWithDirectory(t, store)

	committed, err := store.AppendContextCompactionHealth(story.ID, "main", intent)
	if err != nil {
		t.Fatalf("durable health row reported derived-index failure: %v", err)
	}
	if committed.ID != intent.ID || countCanonicalStoryEventID(t, store, story.ID, intent.ID) != 1 {
		t.Fatalf("canonical health row was not committed exactly once: %#v", committed)
	}
	if err := os.Remove(store.indexPath()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendContextCompactionHealth(story.ID, "main", intent); err != nil {
		t.Fatalf("exact health retry did not repair derived index: %v", err)
	}
	assertStoryIndexProjection(t, store, story.ID, 2)
}

func TestToolResultCleanupRetryBeyondRecentWindowRemainsIdempotent(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "old cleanup retry", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "inspect", Narrative: "done"})
	if err != nil {
		t.Fatal(err)
	}
	intent := cleanupIdempotencyFixture("cleanup-outside-recent", turn.ID)
	first, err := store.AppendToolResultCleanup(story.ID, "main", intent)
	if err != nil {
		t.Fatal(err)
	}
	appendModelInvisibleStoryEvents(t, store, story.ID, "main", storyRecentCacheRecordLimit+1)

	retried, err := store.AppendToolResultCleanup(story.ID, "main", intent)
	if err != nil {
		t.Fatalf("old exact cleanup retry failed: %v", err)
	}
	if retried.ID != first.ID || retried.Ts != first.Ts || countCanonicalStoryEventID(t, store, story.ID, intent.ID) != 1 {
		t.Fatalf("old cleanup retry was not reconciled exactly: first=%#v retry=%#v", first, retried)
	}
}

func TestContextCompactionHealthRetryBeyondRecentWindowRemainsIdempotent(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "old health retry", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "inspect", Narrative: "done"}); err != nil {
		t.Fatal(err)
	}
	revision, _, _, err := store.ContextCompactionHealthState(story.ID, "main", "interactive_story")
	if err != nil {
		t.Fatal(err)
	}
	oldIntent := healthIdempotencyFixture("health-outside-recent", revision, "old")
	first, err := store.AppendContextCompactionHealth(story.ID, "main", oldIntent)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendContextCompactionHealth(story.ID, "main", healthIdempotencyFixture("health-newer", revision, "new")); err != nil {
		t.Fatal(err)
	}
	appendModelInvisibleStoryEvents(t, store, story.ID, "main", storyRecentCacheRecordLimit+1)

	retried, err := store.AppendContextCompactionHealth(story.ID, "main", oldIntent)
	if err != nil {
		t.Fatalf("old exact health retry failed: %v", err)
	}
	if retried.ID != first.ID || retried.Ts != first.Ts || countCanonicalStoryEventID(t, store, story.ID, oldIntent.ID) != 1 {
		t.Fatalf("old health retry was not reconciled exactly: first=%#v retry=%#v", first, retried)
	}
}

func cleanupIdempotencyFixture(id, expectedParent string) ToolResultCleanupEvent {
	return ToolResultCleanupEvent{
		ID: id, AgentKind: "interactive_story", SourceStart: 0, SourceEnd: 2,
		Replacements:    []ToolResultReplacement{{MessageIndex: 1, ToolCallID: "call-read", Placeholder: "[read result archived]"}},
		ReclaimedTokens: 4096, TriggeredAtUsage: 120_000, WarmSuffixTokens: 1024, RendererVersion: "receipt/v1",
		ExpectedParentID: &expectedParent,
	}
}

func healthIdempotencyFixture(id string, revision uint64, fingerprint string) ContextCompactionHealthEvent {
	return ContextCompactionHealthEvent{
		ID: id, AgentKind: "interactive_story", StructureFingerprint: fingerprint,
		Outcome: ContextCompactionHealthOutcomeFailure, FailureCode: "summary_failed", ExpectedContextRevision: revision,
	}
}

func breakStoryIndexWithDirectory(t *testing.T, store *Store) {
	t.Helper()
	if err := os.Remove(store.indexPath()); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(store.indexPath(), 0o755); err != nil {
		t.Fatal(err)
	}
}

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

func countCanonicalStoryEventID(t *testing.T, store *Store, storyID, eventID string) int {
	t.Helper()
	store.mu.Lock()
	defer store.mu.Unlock()
	_, events, err := store.readStoryJournalLocked(storyID)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range events {
		if event.Envelope.ID == eventID {
			count++
		}
	}
	return count
}

func assertStoryIndexProjection(t *testing.T, store *Store, storyID string, expectedEvents int) {
	t.Helper()
	index, err := store.Index()
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Stories) != 1 || index.Stories[0].ID != storyID || index.Stories[0].Events != expectedEvents {
		t.Fatalf("story index projection = %#v, want story=%q events=%d", index.Stories, storyID, expectedEvents)
	}
}
