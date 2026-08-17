package interactive

import (
	"errors"
	"testing"
)

func TestSwitchTurnVersionRejectsHistoricalTurnWithoutMutatingParents(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "不可变版本切换", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "推门", Narrative: "门后是旧仓库。"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RewindToTurnParent(story.ID, RewindTurnRequest{BranchID: "main", TurnID: first.ID}); err != nil {
		t.Fatal(err)
	}
	firstAlt, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "推门", Narrative: "门后是白色长廊。"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "进入", Narrative: "长廊尽头传来钟声。"})
	if err != nil {
		t.Fatal(err)
	}
	third, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "寻找钟声", Narrative: "你在墙后发现暗门。"})
	if err != nil {
		t.Fatal(err)
	}

	if err := store.SwitchTurnVersion(story.ID, SwitchTurnVersionRequest{
		BranchID: "main", TurnID: firstAlt.ID, VersionTurnID: first.ID,
	}); !errors.Is(err, ErrHistoricalTurnRequiresBranch) {
		t.Fatalf("historical version switch error = %v", err)
	}

	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Turns) != 3 {
		t.Fatalf("active turn count = %d, want 3: %#v", len(snapshot.Turns), snapshot.Turns)
	}
	if snapshot.Turns[0].ID != firstAlt.ID || snapshot.Turns[1].ID != second.ID || snapshot.Turns[2].ID != third.ID {
		t.Fatalf("rejected historical switch changed the active path: %#v", snapshot.Turns)
	}
	_, lines, err := store.readStoryJournalLocked(story.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := eventParentForTest(t, lines, second.ID); got != firstAlt.ID {
		t.Fatalf("historical second-turn parent was mutated to %q, want %q", got, firstAlt.ID)
	}
	if got := eventParentForTest(t, lines, third.ID); got != second.ID {
		t.Fatalf("historical third-turn parent was mutated to %q, want %q", got, second.ID)
	}
	for _, record := range lines {
		if record.Envelope.Type == StoryEventTypeTurnVersionSelected {
			t.Fatalf("rejected historical switch appended a selection event: %#v", record.Raw)
		}
	}
}

func TestPendingPlayerInputsRequireReachableActiveAncestry(t *testing.T) {
	t.Run("rewind", func(t *testing.T) {
		store := NewStore(t.TempDir())
		story, err := store.CreateStory(CreateStoryRequest{Title: "回退输入投影", StoryTellerID: "classic"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "第一步", Narrative: "你来到路口。"})
		if err != nil {
			t.Fatal(err)
		}
		second, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "第二步", Narrative: "你走上右侧小路。"})
		if err != nil {
			t.Fatal(err)
		}
		accepted := commitPendingPlayerInputForProjectionTest(t, store, story.ID, "main", "rewind-input", "查看小路尽头")
		if err := store.RewindToTurnParent(story.ID, RewindTurnRequest{BranchID: "main", TurnID: second.ID}); err != nil {
			t.Fatal(err)
		}
		assertPendingInputIsAuditOnly(t, store, story.ID, second.ID, accepted)
	})

	t.Run("version switch", func(t *testing.T) {
		store := NewStore(t.TempDir())
		story, err := store.CreateStory(CreateStoryRequest{Title: "版本输入投影", StoryTellerID: "classic"})
		if err != nil {
			t.Fatal(err)
		}
		first, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "开门", Narrative: "旧门通往花园。"})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.RewindToTurnParent(story.ID, RewindTurnRequest{BranchID: "main", TurnID: first.ID}); err != nil {
			t.Fatal(err)
		}
		firstAlt, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "开门", Narrative: "旧门通往地窖。"})
		if err != nil {
			t.Fatal(err)
		}
		second, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "进入", Narrative: "地窖里很冷。"})
		if err != nil {
			t.Fatal(err)
		}
		accepted := commitPendingPlayerInputForProjectionTest(t, store, story.ID, "main", "switch-input", "点亮火把")
		if err := store.SwitchTurnVersion(story.ID, SwitchTurnVersionRequest{
			BranchID: "main", TurnID: firstAlt.ID, VersionTurnID: first.ID,
		}); !errors.Is(err, ErrHistoricalTurnRequiresBranch) {
			t.Fatalf("historical version switch error = %v", err)
		}
		snapshot, err := store.Snapshot(story.ID, "main")
		if err != nil {
			t.Fatal(err)
		}
		if len(snapshot.PendingPlayerInputs) != 1 || snapshot.PendingPlayerInputs[0].ID != accepted.Event.ID {
			t.Fatalf("rejected switch changed the active pending input: %#v", snapshot.PendingPlayerInputs)
		}
		if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.ID != second.ID {
			t.Fatalf("rejected switch changed the current turn: %#v", snapshot.CurrentTurn)
		}
	})
}

func commitPendingPlayerInputForProjectionTest(t *testing.T, store *Store, storyID, branchID, identitySeed, text string) PlayerInputReceipt {
	t.Helper()
	intent, err := NewPlayerInputIntent(DomainCommitIdentity{
		CommandID: identitySeed + "-command", OperationID: identitySeed + "-operation", Cycle: 1,
	}, branchID, text)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := store.CommitPlayerInput(storyID, intent)
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func assertPendingInputIsAuditOnly(t *testing.T, store *Store, storyID, abandonedParent string, receipt PlayerInputReceipt) {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.PendingPlayerInputs) != 0 {
		t.Fatalf("abandoned future input leaked into active projection: %#v", snapshot.PendingPlayerInputs)
	}
	if receipt.Event.ParentID != abandonedParent {
		t.Fatalf("test setup accepted input parent = %q, want abandoned parent %q", receipt.Event.ParentID, abandonedParent)
	}
	found, ok, err := store.FindPlayerInputCommit(storyID, "main", receipt.Identity, receipt.Hash)
	if err != nil || !ok || found.Revision != receipt.Revision {
		t.Fatalf("abandoned input must remain auditable: receipt=%#v ok=%t err=%v", found, ok, err)
	}
}

func eventParentForTest(t *testing.T, lines []StoryEventRecord, eventID string) string {
	t.Helper()
	for _, record := range lines {
		if record.Envelope.ID == eventID {
			return parentIDFromRaw(record.Raw)
		}
	}
	t.Fatalf("event %q not found", eventID)
	return ""
}

func branchHeadForTest(t *testing.T, store *Store, storyID, branchID string) string {
	t.Helper()
	meta, _, err := store.readStoryJournalLocked(storyID)
	if err != nil {
		t.Fatal(err)
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		t.Fatalf("branch %q not found", branchID)
	}
	return branch.Head
}
