package interactive

import (
	"testing"
)

func TestSwitchTurnVersionClonesSuffixWithoutMutatingHistoricalParents(t *testing.T) {
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
	}); err != nil {
		t.Fatal(err)
	}

	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Turns) != 3 {
		t.Fatalf("active turn count = %d, want 3: %#v", len(snapshot.Turns), snapshot.Turns)
	}
	if snapshot.Turns[0].ID != first.ID {
		t.Fatalf("selected first version = %q, want %q", snapshot.Turns[0].ID, first.ID)
	}
	projectedSecond, projectedThird := snapshot.Turns[1], snapshot.Turns[2]
	if projectedSecond.ID == second.ID || projectedThird.ID == third.ID {
		t.Fatalf("suffix reused mutable historical IDs: second=%#v third=%#v", projectedSecond, projectedThird)
	}
	if projectedSecond.Narrative != second.Narrative || projectedThird.Narrative != third.Narrative {
		t.Fatalf("projected suffix changed content: second=%#v third=%#v", projectedSecond, projectedThird)
	}
	if parentIDString(projectedSecond.ParentID) != first.ID || parentIDString(projectedThird.ParentID) != projectedSecond.ID {
		t.Fatalf("projected suffix is not a continuous immutable path: %#v", snapshot.Turns)
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
	selection := latestTurnVersionSelectionForTest(t, lines)
	if selection.ReplacedTurnID != firstAlt.ID || selection.SelectedTurnID != first.ID || selection.PreviousHeadID != third.ID {
		t.Fatalf("version selection audit event = %#v", selection)
	}
	if len(selection.ProjectedEvents) != 2 || selection.ProjectedEvents[0].SourceID != second.ID || selection.ProjectedEvents[1].SourceID != third.ID {
		t.Fatalf("version selection projection audit = %#v", selection.ProjectedEvents)
	}
}

func TestSwitchTurnVersionInvalidatesDescendantCompaction(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "版本切换压缩失效", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "观察", Narrative: "旧版本的大厅。"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RewindToTurnParent(story.ID, RewindTurnRequest{BranchID: "main", TurnID: first.ID}); err != nil {
		t.Fatal(err)
	}
	firstAlt, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "观察", Narrative: "新版本的大厅。"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "前进", Narrative: "你走到旋转楼梯旁。"})
	if err != nil {
		t.Fatal(err)
	}
	expected := second.ID
	compaction, err := store.AppendContextCompaction(story.ID, "main", ContextCompactionEvent{
		Summary: "大厅通往旋转楼梯。", SourceTurnCount: 2, RetainedTurns: 1, ExpectedParentID: &expected,
	})
	if err != nil {
		t.Fatal(err)
	}
	third, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "上楼", Narrative: "二楼传来乐声。"})
	if err != nil {
		t.Fatal(err)
	}

	if err := store.SwitchTurnVersion(story.ID, SwitchTurnVersionRequest{
		BranchID: "main", TurnID: firstAlt.ID, VersionTurnID: first.ID,
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ContextCompaction != nil {
		t.Fatalf("descendant compaction remained model-visible: %#v", snapshot.ContextCompaction)
	}
	if snapshot.ContextCompactionRemoval == nil || snapshot.ContextCompactionRemoval.CompactionID != compaction.ID || snapshot.ContextCompactionRemoval.Reason != "turn_version_switched" {
		t.Fatalf("missing explicit compaction invalidation: %#v", snapshot.ContextCompactionRemoval)
	}
	if len(snapshot.Turns) != 3 || snapshot.Turns[0].ID != first.ID || snapshot.Turns[1].ID == second.ID || snapshot.Turns[2].ID == third.ID {
		t.Fatalf("unexpected projected path after compaction invalidation: %#v", snapshot.Turns)
	}
	if historical, ok, err := store.ContextCompactionByID(story.ID, compaction.ID); err != nil || !ok || historical.ID != compaction.ID {
		t.Fatalf("historical compaction must remain auditable: event=%#v ok=%t err=%v", historical, ok, err)
	}
}

func TestSwitchTurnVersionPreservesRemovalOfPrefixCompaction(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "前缀压缩撤销投影", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "进入", Narrative: "你进入钟楼。"})
	if err != nil {
		t.Fatal(err)
	}
	expected := first.ID
	compaction, err := store.AppendContextCompaction(story.ID, "main", ContextCompactionEvent{
		Summary: "你已经进入钟楼。", SourceTurnCount: 1, RetainedTurns: 1, ExpectedParentID: &expected,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "上楼", Narrative: "旧版本的二楼空无一人。"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RewindToTurnParent(story.ID, RewindTurnRequest{BranchID: "main", TurnID: second.ID}); err != nil {
		t.Fatal(err)
	}
	secondAlt, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "上楼", Narrative: "新版本的二楼有人弹琴。"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "靠近", Narrative: "琴声突然停了。"}); err != nil {
		t.Fatal(err)
	}
	removeExpected := branchHeadForTest(t, store, story.ID, "main")
	removal, err := store.AppendContextCompactionRemoval(story.ID, "main", ContextCompactionRemovalEvent{
		CompactionID: compaction.ID, SourceTurnCount: 1, Reason: "user_removed", ExpectedParentID: &removeExpected,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := store.SwitchTurnVersion(story.ID, SwitchTurnVersionRequest{
		BranchID: "main", TurnID: secondAlt.ID, VersionTurnID: second.ID,
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ContextCompaction != nil {
		t.Fatalf("projecting the suffix resurrected a removed prefix compaction: %#v", snapshot.ContextCompaction)
	}
	if snapshot.ContextCompactionRemoval == nil || snapshot.ContextCompactionRemoval.ID == removal.ID || snapshot.ContextCompactionRemoval.CompactionID != compaction.ID {
		t.Fatalf("prefix compaction removal was not immutably projected: %#v", snapshot.ContextCompactionRemoval)
	}
	_, lines, err := store.readStoryJournalLocked(story.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := eventParentForTest(t, lines, removal.ID); got != removeExpected {
		t.Fatalf("historical removal parent was mutated to %q, want %q", got, removeExpected)
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
		}); err != nil {
			t.Fatal(err)
		}
		assertPendingInputIsAuditOnly(t, store, story.ID, second.ID, accepted)
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

func latestTurnVersionSelectionForTest(t *testing.T, lines []StoryEventRecord) TurnVersionSelectionEvent {
	t.Helper()
	for index := len(lines) - 1; index >= 0; index-- {
		if lines[index].Envelope.Type != StoryEventTypeTurnVersionSelected {
			continue
		}
		var event TurnVersionSelectionEvent
		if err := mapToStruct(lines[index].Raw, &event); err != nil {
			t.Fatal(err)
		}
		return event
	}
	t.Fatal("turn version selection audit event not found")
	return TurnVersionSelectionEvent{}
}
