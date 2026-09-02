package interactive

import (
	"errors"
	"os"
	"testing"

	interactivestate "denova/internal/interactive/state"
)

func TestPlayerInputAcceptedIsIdempotentPendingAndConsumedByTurn(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "输入 outbox", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	identity := DomainCommitIdentity{CommandID: "input-command", OperationID: "input-operation", Cycle: 1}
	intent, err := NewPlayerInputIntent(identity, "main", "推门")
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.CommitPlayerInput(story.ID, intent)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CommitPlayerInput(story.ID, intent)
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision != second.Revision || first.Hash != second.Hash {
		t.Fatalf("exact player input retry changed receipt: first=%#v second=%#v", first, second)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.PendingPlayerInputs) != 1 || len(snapshot.Turns) != 0 {
		t.Fatalf("accepted input must be pending without narrative: %#v", snapshot)
	}
	conflict, err := NewPlayerInputIntent(identity, "main", "转身离开")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPlayerInput(story.ID, conflict); !errors.Is(err, ErrPlayerInputIdentityConflict) {
		t.Fatalf("same identity different input error = %v", err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: intent.Text, Narrative: "门后是一条长廊。",
		AgentCommandID: identity.CommandID, AgentOperationID: identity.OperationID, AgentCycle: identity.Cycle,
	})
	if err != nil {
		t.Fatal(err)
	}
	if turn.PlayerInputID != first.Revision || turn.PlayerInputHash != first.Hash {
		t.Fatalf("turn did not consume exact player input: %#v", turn)
	}
	snapshot, err = store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.PendingPlayerInputs) != 0 || len(snapshot.Turns) != 1 {
		t.Fatalf("completed turn did not consume pending input: %#v", snapshot)
	}
}

func TestContextOnlyPlayerInputRemainsModelVisibleAndIsNotPlayerAuthored(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "model-only input", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	identity := DomainCommitIdentity{CommandID: "goal-next", OperationID: "goal-run", Cycle: 2}
	visible, err := NewPlayerInputIntent(identity, "main", "完成剩余目标")
	if err != nil {
		t.Fatal(err)
	}
	contextOnly, err := visible.WithContextOnly()
	if err != nil {
		t.Fatal(err)
	}
	if contextOnly.Hash == visible.Hash {
		t.Fatal("context-only projection must participate in the player input hash")
	}
	receipt, err := store.CommitPlayerInput(story.ID, contextOnly)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Event.ContextOnly {
		t.Fatalf("accepted input lost context-only projection: %#v", receipt.Event)
	}
	if _, err := store.CommitPlayerInput(story.ID, visible); !errors.Is(err, ErrPlayerInputIdentityConflict) {
		t.Fatalf("same identity changed from context-only to visible: %v", err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: contextOnly.Text, Narrative: "目标相关剧情继续推进。",
		AgentCommandID: identity.CommandID, AgentOperationID: identity.OperationID, AgentCycle: identity.Cycle,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !turn.UserContextOnly {
		t.Fatalf("turn lost model-only user projection: %#v", turn)
	}
	history, err := store.ReadModelHistory(story.ID, StoryModelHistoryQuery{BranchID: "main", StartTurn: 0, EndTurn: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Turns) != 1 || history.Turns[0].User != contextOnly.Text {
		t.Fatalf("context-only input disappeared from model history: %#v", history.Turns)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Graph.Nodes) != 1 || snapshot.Graph.Nodes[0].Title != turn.Narrative {
		t.Fatalf("context-only input leaked into the player-facing graph: %#v", snapshot.Graph.Nodes)
	}
	search, err := store.SearchStoryHistory(story.ID, "main", StoryHistorySearchRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(search.Hits) != 1 || search.Hits[0].UserAction != "" {
		t.Fatalf("context-only input leaked as a player action: %#v", search.Hits)
	}
}

func TestJournalProjectionTracksOnlyUnconsumedPlayerInputIDs(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	story, err := store.CreateStory(CreateStoryRequest{Title: "pending input projection", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	firstIdentity := DomainCommitIdentity{CommandID: "first-command", OperationID: "first-operation", Cycle: 1}
	firstIntent, err := NewPlayerInputIntent(firstIdentity, "main", "打开门")
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.CommitPlayerInput(story.ID, firstIntent)
	if err != nil {
		t.Fatal(err)
	}
	if got := projectedPendingPlayerInputIDsForTest(t, store, story.ID); len(got) != 1 || got[0] != first.Event.ID {
		t.Fatalf("pending projection after input = %#v", got)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: firstIntent.Text, Narrative: "门开了。",
		AgentCommandID: firstIdentity.CommandID, AgentOperationID: firstIdentity.OperationID, AgentCycle: firstIdentity.Cycle,
	}); err != nil {
		t.Fatal(err)
	}
	if got := projectedPendingPlayerInputIDsForTest(t, store, story.ID); len(got) != 0 {
		t.Fatalf("consumed input remained in projection: %#v", got)
	}

	secondIdentity := DomainCommitIdentity{CommandID: "second-command", OperationID: "second-operation", Cycle: 1}
	secondIntent, err := NewPlayerInputIntent(secondIdentity, "main", "进入走廊")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CommitPlayerInput(story.ID, secondIntent)
	if err != nil {
		t.Fatal(err)
	}
	cold := NewStore(root)
	if got := projectedPendingPlayerInputIDsForTest(t, cold, story.ID); len(got) != 1 || got[0] != second.Event.ID {
		t.Fatalf("cold pending projection = %#v, want only %q", got, second.Event.ID)
	}
	filtered := pendingPlayerInputsFromProjection([]PlayerInputAcceptedEvent{first.Event, second.Event}, projectedPendingPlayerInputIDsForTest(t, cold, story.ID))
	if len(filtered) != 1 || filtered[0].ID != second.Event.ID {
		t.Fatalf("bounded page lifecycle filter = %#v", filtered)
	}
}

func projectedPendingPlayerInputIDsForTest(t *testing.T, store *Store, storyID string) []string {
	t.Helper()
	store.mu.Lock()
	defer store.mu.Unlock()
	projection, err := store.storyBranchProjectionLocked(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	return append([]string(nil), projection.PendingPlayerInputIDs...)
}

func TestAppendTurnWithStateIsIdempotentByDurableAgentIdentity(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "幂等回合", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	request := AppendTurnWithStateRequest{
		BranchID: "main", User: "推门", Narrative: "门后是一条长廊。",
		AgentCommandID: "command-1", AgentOperationID: "operation-1", AgentCycle: 1,
		Ops: []interactivestate.Op{{Op: "set", Path: "location", Value: "长廊"}},
	}
	commitPlayerInputForTurnTest(t, store, story.ID, request)
	first, firstDelta, err := store.AppendTurnWithState(story.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	second, secondDelta, err := store.AppendTurnWithState(story.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID || firstDelta == nil || secondDelta == nil || secondDelta.ID != firstDelta.ID {
		t.Fatalf("idempotent turns/deltas differ: first=%#v second=%#v first_delta=%#v second_delta=%#v", first, second, firstDelta, secondDelta)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Turns) != 1 || snapshot.Turns[0].AgentCommandID != request.AgentCommandID {
		t.Fatalf("snapshot contains duplicate or uncorrelated turns: %#v", snapshot.Turns)
	}
	index, err := store.Index()
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Stories) != 1 || index.Stories[0].Events != 2 {
		t.Fatalf("index event projection drifted after retry: %#v", index.Stories)
	}
}

func TestRecentRuntimeCommitLookupReadsOnlyLocatedTransaction(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	story, err := store.CreateStory(CreateStoryRequest{Title: "有界恢复", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	request := AppendTurnWithStateRequest{
		BranchID: "main", User: "推门", Narrative: "门后是一条长廊。",
		AgentCommandID: "bounded-command", AgentOperationID: "bounded-operation", AgentCycle: 1,
	}
	commitPlayerInputForTurnTest(t, store, story.ID, request)
	turn, _, err := store.AppendTurnWithState(story.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := NewStore(root)
	identity := DomainCommitIdentity{CommandID: request.AgentCommandID, OperationID: request.AgentOperationID, Cycle: request.AgentCycle}
	inputHash, err := NewPlayerInputIntent(identity, "main", request.User)
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := reopened.FindRecentPlayerInputCommit(story.ID, "main", identity, inputHash.Hash); err != nil || !found {
		t.Fatalf("recent input lookup found=%t err=%v", found, err)
	}
	if stats := reopened.LastStoryJournalReplayStats(story.ID); stats.RecordsRead != 1 || stats.TransactionsRead != 1 {
		t.Fatalf("recent input lookup replayed more than its located transaction: %#v", stats)
	}
	if receipt, found, err := reopened.FindRecentDomainTurnCommit(story.ID, "main", identity, turn.AgentCommitHash); err != nil || !found || receipt.Revision != turn.ID {
		t.Fatalf("recent turn lookup receipt=%#v found=%t err=%v", receipt, found, err)
	}
	if stats := reopened.LastStoryJournalReplayStats(story.ID); stats.RecordsRead != 1 || stats.TransactionsRead != 1 {
		t.Fatalf("recent turn lookup replayed more than its located transaction: %#v", stats)
	}

	missing := DomainCommitIdentity{CommandID: "new-command", OperationID: "new-operation", Cycle: 1}
	if _, found, err := reopened.FindRecentDomainTurnCommit(story.ID, "main", missing, "new-hash"); err != nil || found {
		t.Fatalf("missing recent turn found=%t err=%v", found, err)
	}
	if stats := reopened.LastStoryJournalReplayStats(story.ID); stats.BytesRead != 0 || stats.RecordsRead != 0 {
		t.Fatalf("missing command replayed canonical history: %#v", stats)
	}
}

func TestAppendTurnWithStateRejectsAgentIdentityPayloadConflict(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "冲突回合", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	request := AppendTurnWithStateRequest{
		BranchID: "main", User: "观察", Narrative: "原始结果。",
		AgentCommandID: "command-1", AgentOperationID: "operation-1", AgentCycle: 1,
	}
	commitPlayerInputForTurnTest(t, store, story.ID, request)
	if _, _, err := store.AppendTurnWithState(story.ID, request); err != nil {
		t.Fatal(err)
	}
	request.Narrative = "同一命令下的另一份结果。"
	if _, _, err := store.AppendTurnWithState(story.ID, request); !errors.Is(err, ErrAgentTurnIdentityConflict) {
		t.Fatalf("conflicting retry error = %v, want ErrAgentTurnIdentityConflict", err)
	}
}

func TestAppendTurnWithStateDoesNotReportProjectionFailureAsCanonicalFailure(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "索引故障", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	request := AppendTurnWithStateRequest{
		BranchID: "main", User: "继续", Narrative: "正文已经安全落盘。",
		AgentCommandID: "command-1", AgentOperationID: "operation-1", AgentCycle: 1,
	}
	commitPlayerInputForTurnTest(t, store, story.ID, request)
	if err := os.Remove(store.indexPath()); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(store.indexPath(), 0o755); err != nil {
		t.Fatal(err)
	}

	turn, _, err := store.AppendTurnWithState(story.ID, request)
	if err != nil {
		t.Fatalf("canonical append returned projection error: %v", err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Turns) != 1 || snapshot.Turns[0].ID != turn.ID {
		t.Fatalf("canonical turn missing after projection failure: %#v", snapshot.Turns)
	}
}

func commitPlayerInputForTurnTest(t *testing.T, store *Store, storyID string, request AppendTurnWithStateRequest) {
	t.Helper()
	intent, err := NewPlayerInputIntent(DomainCommitIdentity{
		CommandID: request.AgentCommandID, OperationID: request.AgentOperationID, Cycle: request.AgentCycle,
	}, request.BranchID, request.User)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPlayerInput(storyID, intent); err != nil {
		t.Fatal(err)
	}
}
