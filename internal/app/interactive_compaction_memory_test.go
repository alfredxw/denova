package app

import (
	"context"
	"errors"
	"sync"

	"denova/config"
	"denova/internal/agent"
	"testing"

	"denova/internal/interactive"
)

func TestInteractiveCompactionCoveredTurns(t *testing.T) {
	turns := []interactive.TurnEvent{
		{ID: "t1"}, {ID: "t2"}, {ID: "t3"}, {ID: "t4"},
	}
	tests := []struct {
		name       string
		compaction *interactive.ContextCompactionEvent
		want       []string
	}{
		{
			// 首次压缩:没有前序检查点,整段历史都归本次覆盖。
			name:       "first compaction covers everything",
			compaction: nil,
			want:       []string{"t1", "t2", "t3", "t4"},
		},
		{
			// 增量压缩:只覆盖上次截止位置之后的回合,不重复补抽。
			name:       "incremental compaction resumes from the checkpoint",
			compaction: &interactive.ContextCompactionEvent{Summary: "前情", SourceTurnCount: 2},
			want:       []string{"t3", "t4"},
		},
		{
			// 空摘要的压缩事件不算有效检查点,与首次压缩同义。
			name:       "empty summary is not a checkpoint",
			compaction: &interactive.ContextCompactionEvent{SourceTurnCount: 2},
			want:       []string{"t1", "t2", "t3", "t4"},
		},
		{
			// 计数越界不得 panic:分支回滚后 SourceTurnCount 可能超过现有回合数。
			name:       "out-of-range count is clamped",
			compaction: &interactive.ContextCompactionEvent{Summary: "前情", SourceTurnCount: 99},
			want:       nil,
		},
		{
			name:       "negative count is clamped",
			compaction: &interactive.ContextCompactionEvent{Summary: "前情", SourceTurnCount: -5},
			want:       []string{"t1", "t2", "t3", "t4"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := interactiveCompactionCoveredTurns(turns, tc.compaction)
			if len(got) != len(tc.want) {
				t.Fatalf("covered %d turns, want %d: %#v", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i].ID != tc.want[i] {
					t.Fatalf("covered[%d] = %q, want %q", i, got[i].ID, tc.want[i])
				}
			}
		})
	}
}

// TestInteractiveCompactionCoveredTurnsMatchesSourceRange 钉住两者同口径:
// 补抽的回合必须正好是被摘要吃掉的那些,否则会漏抽或重复抽。
func TestInteractiveCompactionCoveredTurnsMatchesSourceRange(t *testing.T) {
	turns := []interactive.TurnEvent{
		{ID: "t1", User: "a", Narrative: "A"},
		{ID: "t2", User: "b", Narrative: "B"},
		{ID: "t3", User: "c", Narrative: "C"},
	}
	compaction := &interactive.ContextCompactionEvent{Summary: "前情", SourceTurnCount: 1}

	messages, _ := interactiveCompactionSource(turns, compaction)
	covered := interactiveCompactionCoveredTurns(turns, compaction)

	// interactiveCompactionSource 每个回合产出 user + assistant 两条消息。
	if len(messages) != len(covered)*2 {
		t.Fatalf("source produced %d messages for %d covered turns", len(messages), len(covered))
	}
}

// compactionMemoryFixture 建一个三回合故事与一个可注入抽取器的会话。
func compactionMemoryFixture(t *testing.T, mode string) (*interactive.Store, interactive.StorySummary, *interactiveConversation, []interactive.TurnEvent) {
	t.Helper()
	store := interactive.NewStore(t.TempDir())
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "补抽", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turns := make([]interactive.TurnEvent, 0, 3)
	for _, narrative := range []string{"林舟抵达银月港。", "岚交出蚀骨剑。", "黑帆船在午夜靠岸。"} {
		turn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
			BranchID:  "main",
			User:      "继续",
			Narrative: narrative,
		})
		if err != nil {
			t.Fatal(err)
		}
		turns = append(turns, turn)
	}
	conversation := newInteractiveConversation(store, t.TempDir(), t.TempDir(), story.ID, "main", "继续", story.ReplyTargetChars,
		&config.Config{NarrativeMemoryPublishMode: mode})
	return store, story, conversation, turns
}

// TestStartInteractiveCompactionMemoryTaskBackfills 走完整链路:派发 → 去重 →
// 抽取 → 落库,只把模型调用换成固定产出。
func TestStartInteractiveCompactionMemoryTaskBackfills(t *testing.T) {
	store, story, conversation, turns := compactionMemoryFixture(t, config.NarrativeMemoryPublishModeOnCompact)

	// 第二个回合先行抽取过,补抽必须跳过它而不是重复抽。
	if _, err := store.AppendNarrativeMemory(story.ID, "main", interactive.NarrativeMemoryEvent{
		SourceTurnID: turns[1].ID,
		Records: []interactive.NarrativeMemoryRecord{
			{ID: "pre_1", Kind: interactive.MemoryKindBeat, Subject: "岚", Text: "岚交出了剑。", Evidence: "交出蚀骨剑", ValidFrom: turns[1].ID},
		},
	}); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var extracted []string
	conversation.memoryGenerator = func(_ context.Context, _ *config.Config, input agent.MemoryExtractionInput) (agent.MemoryExtractionResult, error) {
		mu.Lock()
		extracted = append(extracted, input.Turn.ID)
		mu.Unlock()
		return agent.MemoryExtractionResult{
			Records: []interactive.NarrativeMemoryRecord{{
				ID:        "gen_" + input.Turn.ID,
				Kind:      interactive.MemoryKindBeat,
				Subject:   "林舟",
				Text:      "本回合的节拍。",
				Evidence:  input.Turn.Narrative,
				ValidFrom: input.Turn.ID,
			}},
		}, nil
	}

	done := startInteractiveCompactionMemoryTask(conversation.cfg, conversation, "main", turns)
	if done == nil {
		t.Fatal("on_compaction mode should dispatch a backfill task")
	}
	<-done

	mu.Lock()
	defer mu.Unlock()
	if len(extracted) != 2 {
		t.Fatalf("expected 2 extractions (the pre-covered turn skipped), got %v", extracted)
	}
	for _, turnID := range extracted {
		if turnID == turns[1].ID {
			t.Fatalf("already-covered turn must not be re-extracted: %v", extracted)
		}
	}
	covered, err := store.NarrativeMemoryCoveredTurns(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	for _, turn := range turns {
		if !covered[turn.ID] {
			t.Fatalf("turn %s should be covered after backfill", turn.ID)
		}
	}
}

// TestStartInteractiveCompactionMemoryTaskModeGate 确认只有 on_compaction 会补抽。
func TestStartInteractiveCompactionMemoryTaskModeGate(t *testing.T) {
	for _, mode := range []string{
		config.NarrativeMemoryPublishModeManual,
		config.NarrativeMemoryPublishModeEveryTurn,
	} {
		t.Run(mode, func(t *testing.T) {
			_, _, conversation, turns := compactionMemoryFixture(t, mode)
			conversation.memoryGenerator = func(_ context.Context, _ *config.Config, _ agent.MemoryExtractionInput) (agent.MemoryExtractionResult, error) {
				t.Fatal("extraction must not run outside on_compaction mode")
				return agent.MemoryExtractionResult{}, nil
			}
			if done := startInteractiveCompactionMemoryTask(conversation.cfg, conversation, "main", turns); done != nil {
				t.Fatalf("mode %q should not dispatch a backfill task", mode)
			}
		})
	}
	// 没有覆盖回合时不该空跑一个任务。
	_, _, conversation, _ := compactionMemoryFixture(t, config.NarrativeMemoryPublishModeOnCompact)
	if done := startInteractiveCompactionMemoryTask(conversation.cfg, conversation, "main", nil); done != nil {
		t.Fatal("an empty covered range should not dispatch a task")
	}
}

// TestStartInteractiveCompactionMemoryTaskSurvivesFailures 确认单个回合抽取失败
// 不会中断整批补抽,且失败回合被记为已覆盖(不再无限重试)。
func TestStartInteractiveCompactionMemoryTaskSurvivesFailures(t *testing.T) {
	store, story, conversation, turns := compactionMemoryFixture(t, config.NarrativeMemoryPublishModeOnCompact)

	conversation.memoryGenerator = func(_ context.Context, _ *config.Config, input agent.MemoryExtractionInput) (agent.MemoryExtractionResult, error) {
		if input.Turn.ID == turns[0].ID {
			return agent.MemoryExtractionResult{}, errors.New("boom")
		}
		return agent.MemoryExtractionResult{
			Records: []interactive.NarrativeMemoryRecord{{
				ID:        "gen_" + input.Turn.ID,
				Kind:      interactive.MemoryKindBeat,
				Subject:   "林舟",
				Text:      "本回合的节拍。",
				Evidence:  input.Turn.Narrative,
				ValidFrom: input.Turn.ID,
			}},
		}, nil
	}

	done := startInteractiveCompactionMemoryTask(conversation.cfg, conversation, "main", turns)
	if done == nil {
		t.Fatal("expected a backfill task")
	}
	<-done

	covered, err := store.NarrativeMemoryCoveredTurns(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	// 失败的回合落了一条 trace-only 事件,同样算已覆盖。
	for _, turn := range turns {
		if !covered[turn.ID] {
			t.Fatalf("turn %s should be covered even when its extraction failed", turn.ID)
		}
	}
	// 后续回合没有因为前一个失败而被跳过。
	view, err := store.BrowseStoryMemory(story.ID, "main", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if view.Stats.Records != 2 {
		t.Fatalf("expected the two successful turns to produce records, got %d", view.Stats.Records)
	}
}
