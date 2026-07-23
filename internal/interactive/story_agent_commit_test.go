package interactive

import (
	"errors"
	"fmt"
	"os"
	"testing"
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

func TestAppendTurnWithStateIsIdempotentByDurableAgentIdentity(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "幂等回合", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	request := AppendTurnWithStateRequest{
		BranchID: "main", User: "推门", Narrative: "门后是一条长廊。",
		AgentCommandID: "command-1", AgentOperationID: "operation-1", AgentCycle: 1,
		Ops: []StateOp{{Op: "set", Path: "location", Value: "长廊"}},
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

func TestAppendTurnWithStateReconcilesAmbiguousCanonicalAppend(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "模糊提交", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	request := AppendTurnWithStateRequest{
		BranchID: "main", User: "继续", Narrative: "这一回合已经完成原子替换。",
		AgentCommandID: "command-ambiguous", AgentOperationID: "operation-ambiguous", AgentCycle: 2,
	}
	commitPlayerInputForTurnTest(t, store, story.ID, request)
	store.appendStoryRecord = func(path string, record []byte) error {
		if err := appendStoryRecord(path, record); err != nil {
			return err
		}
		return &storyAppendRecordError{directoryErr: fmt.Errorf("simulated directory sync result loss")}
	}
	turn, _, err := store.AppendTurnWithState(story.ID, request)
	if err != nil {
		t.Fatalf("ambiguous but committed rewrite returned failure: %v", err)
	}
	if turn.AgentCommandID != request.AgentCommandID {
		t.Fatalf("reconciled turn = %#v", turn)
	}

	store.appendStoryRecord = nil
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Turns) != 1 || snapshot.Turns[0].ID != turn.ID {
		t.Fatalf("canonical story contains duplicate or missing turn: %#v", snapshot.Turns)
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
