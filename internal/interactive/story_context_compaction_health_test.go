package interactive

import "testing"

func TestStoryContextCompactionHealthIsSidebandAndDurable(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	story, err := store.CreateStory(CreateStoryRequest{Title: "health", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	firstTurn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "one", Narrative: "first"})
	if err != nil {
		t.Fatal(err)
	}
	beforeHead := firstTurn.ID
	revision, _, _, err := store.ContextCompactionHealthState(story.ID, "main", "interactive_story")
	if err != nil {
		t.Fatal(err)
	}
	firstIntent := ContextCompactionHealthEvent{
		ID: "game-health-1", AgentKind: "interactive_story", StructureFingerprint: "stable",
		Outcome: ContextCompactionHealthOutcomeFailure, FailureCode: "summary_failed", ExpectedContextRevision: revision,
	}
	first, err := store.AppendContextCompactionHealth(story.ID, "main", firstIntent)
	if err != nil {
		t.Fatal(err)
	}
	after, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if after.Meta.Branches["main"].Head != beforeHead || first.ParentID != beforeHead || first.ConsecutiveFailures != 1 {
		t.Fatalf("health changed head or count: head=%q event=%#v", after.Meta.Branches["main"].Head, first)
	}
	afterRevision, latest, ok, err := store.ContextCompactionHealthState(story.ID, "main", "interactive_story")
	if err != nil || !ok || afterRevision != revision || latest.ID != first.ID {
		t.Fatalf("health state revision=%d latest=%#v ok=%t err=%v", afterRevision, latest, ok, err)
	}

	secondTurn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "two", Narrative: "second"})
	if err != nil {
		t.Fatal(err)
	}
	secondRevision, carried, ok, err := store.ContextCompactionHealthState(story.ID, "main", "interactive_story")
	if err != nil || !ok || secondRevision <= revision || carried.ID != first.ID {
		t.Fatalf("ordinary turn did not carry health: revision=%d health=%#v ok=%t err=%v", secondRevision, carried, ok, err)
	}
	secondIntent := ContextCompactionHealthEvent{
		ID: "game-health-2", AgentKind: "interactive_story", StructureFingerprint: "stable",
		Outcome: ContextCompactionHealthOutcomeFailure, FailureCode: "summary_failed", ExpectedContextRevision: secondRevision,
	}
	second, err := store.AppendContextCompactionHealth(story.ID, "main", secondIntent)
	if err != nil || second.ConsecutiveFailures != 2 {
		t.Fatalf("second health=%#v err=%v", second, err)
	}
	replayed, err := store.AppendContextCompactionHealth(story.ID, "main", secondIntent)
	if err != nil || replayed.ConsecutiveFailures != 2 {
		t.Fatalf("exact retry=%#v err=%v", replayed, err)
	}
	if err := store.RewindToTurnParent(story.ID, RewindTurnRequest{BranchID: "main", TurnID: secondTurn.ID}); err != nil {
		t.Fatal(err)
	}
	rewoundRevision, stale, ok, err := store.ContextCompactionHealthState(story.ID, "main", "interactive_story")
	if err != nil || ok {
		t.Fatalf("rewind retained stale compaction health: revision=%d health=%#v ok=%t err=%v", rewoundRevision, stale, ok, err)
	}
	reset, err := store.AppendContextCompactionHealth(story.ID, "main", ContextCompactionHealthEvent{
		ID: "game-health-reset", AgentKind: "interactive_story", StructureFingerprint: "manual",
		Outcome: ContextCompactionHealthOutcomeManualRetry, ExpectedContextRevision: rewoundRevision,
	})
	if err != nil || reset.ConsecutiveFailures != 0 {
		t.Fatalf("manual reset=%#v err=%v", reset, err)
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reloaded := NewStore(dir)
	defer reloaded.Close()
	reloadedRevision, reloadedHealth, ok, err := reloaded.ContextCompactionHealthState(story.ID, "main", "interactive_story")
	if err != nil || !ok || reloadedRevision != rewoundRevision || reloadedHealth.ID != reset.ID || reloadedHealth.ConsecutiveFailures != 0 {
		t.Fatalf("reloaded revision=%d health=%#v ok=%t err=%v", reloadedRevision, reloadedHealth, ok, err)
	}
}
