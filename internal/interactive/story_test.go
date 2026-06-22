package interactive

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateStoryInitializesIndexAndStoryFile(t *testing.T) {
	store := NewStore(t.TempDir())

	story, err := store.CreateStory(CreateStoryRequest{
		Title:         "末日开端",
		Origin:        "主角醒来发现世界已末日",
		StoryTellerID: "grimdark",
	})
	if err != nil {
		t.Fatalf("CreateStory failed: %v", err)
	}

	index, err := store.Index()
	if err != nil {
		t.Fatalf("Index failed: %v", err)
	}
	if index.CurrentStoryID != story.ID {
		t.Fatalf("current story = %q, want %q", index.CurrentStoryID, story.ID)
	}
	if len(index.Stories) != 1 || index.Stories[0].Title != "末日开端" {
		t.Fatalf("unexpected index stories: %+v", index.Stories)
	}
	if story.ReplyTargetChars != DefaultStoryReplyTargetChars || index.Stories[0].ReplyTargetChars != DefaultStoryReplyTargetChars {
		t.Fatalf("reply target chars = story:%d index:%d, want %d", story.ReplyTargetChars, index.Stories[0].ReplyTargetChars, DefaultStoryReplyTargetChars)
	}

	storyFile := filepath.Join(store.Root(), "interactive", "story", "story-"+story.ID+".jsonl")
	data, err := os.ReadFile(storyFile)
	if err != nil {
		t.Fatalf("story file not created: %v", err)
	}
	assertContains(t, string(data), `"type":"meta"`)
	assertContains(t, string(data), `"current_branch":"main"`)
	assertContains(t, string(data), `"story_teller_id":"grimdark"`)
	assertContains(t, string(data), fmt.Sprintf(`"reply_target_chars":%d`, DefaultStoryReplyTargetChars))
}

func TestCreateStoryPersistsCustomReplyTargetChars(t *testing.T) {
	store := NewStore(t.TempDir())

	story, err := store.CreateStory(CreateStoryRequest{
		Title:            "短回合",
		StoryTellerID:    "classic",
		ReplyTargetChars: 800,
	})
	if err != nil {
		t.Fatalf("CreateStory failed: %v", err)
	}
	if story.ReplyTargetChars != 800 {
		t.Fatalf("reply target chars = %d, want 800", story.ReplyTargetChars)
	}
	ctx, err := store.StoryContext(story.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Meta.ReplyTargetChars != 800 {
		t.Fatalf("meta reply target chars = %d, want 800", ctx.Meta.ReplyTargetChars)
	}
}

func TestCreateStoryPersistsOpeningConfig(t *testing.T) {
	store := NewStore(t.TempDir())

	story, err := store.CreateStory(CreateStoryRequest{
		Title:         "雨夜",
		StoryTellerID: "classic",
		Opening: StoryOpeningConfig{
			Mode:       StoryOpeningModePreset,
			PresetID:   "arrival",
			PresetText: "雨夜里，旧城的招牌微微发亮。",
			CustomText: "should be dropped",
		},
	})
	if err != nil {
		t.Fatalf("CreateStory failed: %v", err)
	}
	if story.Opening.Mode != StoryOpeningModePreset || story.Opening.PresetID != "arrival" || story.Opening.PresetText == "" {
		t.Fatalf("unexpected story opening: %#v", story.Opening)
	}
	if story.Opening.CustomText != "" {
		t.Fatalf("preset opening should drop custom text: %#v", story.Opening)
	}

	index, err := store.Index()
	if err != nil {
		t.Fatal(err)
	}
	if index.Stories[0].Opening.Mode != StoryOpeningModePreset || index.Stories[0].Opening.PresetText == "" {
		t.Fatalf("unexpected index opening: %#v", index.Stories[0].Opening)
	}
	ctx, err := store.StoryContext(story.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Meta.Opening.Mode != StoryOpeningModePreset || ctx.Meta.Opening.PresetText == "" {
		t.Fatalf("unexpected meta opening: %#v", ctx.Meta.Opening)
	}
}

func TestUpdateStoryRejectsEmptyCustomOpening(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "开局", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.UpdateStory(story.ID, UpdateStoryRequest{
		Opening: &StoryOpeningConfig{Mode: StoryOpeningModeCustom, CustomText: "   "},
	})
	if err == nil {
		t.Fatal("expected empty custom opening to be rejected")
	}
	if !strings.Contains(err.Error(), "自定义开场白不能为空") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOldStoryWithoutOpeningDefaultsToAI(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	now := "2026-06-18T00:00:00Z"
	meta := map[string]any{
		"v":               schemaVersion,
		"type":            StoryEventTypeMeta,
		"story_id":        "st_legacy",
		"title":           "旧故事",
		"story_teller_id": "classic",
		"current_branch":  "main",
		"branches":        map[string]any{"main": map[string]any{"created_at": now}},
		"created_at":      now,
		"updated_at":      now,
	}
	if err := writeJSONL(store.storyPath("st_legacy"), []any{meta}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(store.indexPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.indexPath(), []byte(`{"current_story_id":"st_legacy","stories":[{"id":"st_legacy","title":"旧故事","story_teller_id":"classic","created_at":"`+now+`","updated_at":"`+now+`","branches":1}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	index, err := store.Index()
	if err != nil {
		t.Fatal(err)
	}
	if index.Stories[0].Opening.Mode != StoryOpeningModeAI {
		t.Fatalf("legacy index opening = %#v, want ai", index.Stories[0].Opening)
	}
	ctx, err := store.StoryContext("st_legacy", "")
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Meta.Opening.Mode != StoryOpeningModeAI {
		t.Fatalf("legacy meta opening = %#v, want ai", ctx.Meta.Opening)
	}
}

func TestSnapshotAppliesTurnAndStateDelta(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{
		Title:         "酒馆",
		Origin:        "推门进入酒馆",
		StoryTellerID: "classic",
	})
	if err != nil {
		t.Fatalf("CreateStory failed: %v", err)
	}

	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{
		BranchID:  "main",
		User:      "我推开酒馆的门",
		Narrative: "门轴发出沉闷的吱呀声。",
	})
	if err != nil {
		t.Fatalf("AppendTurn failed: %v", err)
	}
	_, err = store.AppendStateDelta(story.ID, AppendStateDeltaRequest{
		ParentID: turn.ID,
		BranchID: "main",
		Ops: []StateOp{
			{Op: "set", Path: "on_stage", Value: []any{"林川", "酒保老李"}},
			{Op: "merge", Path: "characters.林川", Value: map[string]any{"hp": 80, "location": "黄泉酒馆"}},
			{Op: "push", Path: "events", Value: map[string]any{"flag": "遇到神秘老人"}},
			{Op: "merge", Path: "scene", Value: map[string]any{"danger_level": "低", "interactive_objects": []any{"柜台"}}},
			{Op: "push", Path: "action_space", Value: map[string]any{"target": "柜台", "approach": "询问酒保"}},
			{Op: "push", Path: "threads", Value: map[string]any{"title": "神秘老人留下的暗号"}},
			{Op: "push", Path: "world_flags", Value: "黄泉酒馆午夜后只进不出"},
		},
	})
	if err != nil {
		t.Fatalf("AppendStateDelta failed: %v", err)
	}

	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}
	if len(snapshot.Turns) != 1 || snapshot.Turns[0].Narrative != "门轴发出沉闷的吱呀声。" {
		t.Fatalf("unexpected turns: %+v", snapshot.Turns)
	}
	onStage, ok := snapshot.State["on_stage"].([]any)
	if !ok || len(onStage) != 2 || onStage[0] != "林川" {
		t.Fatalf("unexpected on_stage: %#v", snapshot.State["on_stage"])
	}
	characters := snapshot.State["characters"].(map[string]any)
	linchuan := characters["林川"].(map[string]any)
	if linchuan["location"] != "黄泉酒馆" {
		t.Fatalf("unexpected character state: %#v", linchuan)
	}
	events := snapshot.State["events"].([]any)
	if len(events) != 1 {
		t.Fatalf("unexpected events: %#v", events)
	}
	scene := snapshot.State["scene"].(map[string]any)
	if scene["danger_level"] != "低" {
		t.Fatalf("unexpected scene: %#v", scene)
	}
	actionSpace := snapshot.State["action_space"].([]any)
	if len(actionSpace) != 1 {
		t.Fatalf("unexpected action_space: %#v", actionSpace)
	}
	threads := snapshot.State["threads"].([]any)
	if len(threads) != 1 {
		t.Fatalf("unexpected threads: %#v", threads)
	}
	worldFlags := snapshot.State["world_flags"].([]any)
	if len(worldFlags) != 1 {
		t.Fatalf("unexpected world_flags: %#v", worldFlags)
	}
}

func TestAppendTokenUsageEventPersistsSeparatelyFromStory(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{
		Title:         "缓存统计",
		StoryTellerID: "classic",
	})
	if err != nil {
		t.Fatalf("CreateStory failed: %v", err)
	}

	_, err = store.AppendTurn(story.ID, AppendTurnRequest{
		BranchID:  "main",
		User:      "继续",
		Narrative: "故事继续。",
	})
	if err != nil {
		t.Fatalf("AppendTurn failed: %v", err)
	}
	err = store.AppendTokenUsageEvent(story.ID, TokenUsageEvent{
		ID:                   "usage-1",
		BranchID:             "main",
		CreatedAt:            "2026-06-22T10:00:00Z",
		RunID:                "run-1",
		AgentKind:            "interactive_story",
		PromptTokens:         1200,
		CachedPromptTokens:   300,
		UncachedPromptTokens: 900,
		CacheHitRate:         0.25,
		CompletionTokens:     240,
		ReasoningTokens:      16,
		TotalTokens:          1440,
		ModelCalls:           1,
		GeneratedBytes:       88,
		UsageCalls: []TokenUsageCall{{
			Index:                1,
			CreatedAt:            "2026-06-22T10:00:00Z",
			FinishReason:         "tool_calls",
			RequestedTools:       []string{"search_workspace"},
			AfterTools:           []string{"read_workspace_file"},
			PromptTokens:         1200,
			CachedPromptTokens:   300,
			UncachedPromptTokens: 900,
			CacheHitRate:         0.25,
			CompletionTokens:     240,
			ReasoningTokens:      16,
			TotalTokens:          1440,
		}},
	})
	if err != nil {
		t.Fatalf("AppendTokenUsageEvent failed: %v", err)
	}

	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}
	if len(snapshot.Turns) != 1 || len(snapshot.Turns[0].DisplayEvents) != 0 {
		t.Fatalf("usage should not be mixed into turn display events: %#v", snapshot.Turns)
	}
	if len(snapshot.TokenUsageEvents) != 1 {
		t.Fatalf("usage event missing from snapshot: %#v", snapshot.TokenUsageEvents)
	}
	usage := snapshot.TokenUsageEvents[0]
	if usage.Type != TokenUsageEventType || usage.RunID != "run-1" || usage.PromptTokens != 1200 || usage.CachedPromptTokens != 300 {
		t.Fatalf("usage event fields were not preserved: %#v", usage)
	}
	if usage.UncachedPromptTokens != 900 {
		t.Fatalf("usage uncached prompt tokens were not preserved: %#v", usage)
	}
	if usage.CacheHitRate != 0.25 || usage.TotalTokens != 1440 || usage.ModelCalls != 1 || usage.GeneratedBytes != 88 {
		t.Fatalf("usage metrics were not preserved: %#v", usage)
	}
	if len(usage.UsageCalls) != 1 || usage.UsageCalls[0].RequestedTools[0] != "search_workspace" || usage.UsageCalls[0].AfterTools[0] != "read_workspace_file" {
		t.Fatalf("usage call details were not preserved: %#v", usage.UsageCalls)
	}
	if usage.UsageCalls[0].UncachedPromptTokens != 900 {
		t.Fatalf("usage call uncached prompt tokens were not preserved: %#v", usage.UsageCalls)
	}
	storyData, err := os.ReadFile(store.storyPath(story.ID))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(storyData), TokenUsageEventType) {
		t.Fatalf("story file should not contain token usage:\n%s", string(storyData))
	}
	usageData, err := os.ReadFile(store.usagePath(story.ID))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(usageData), `"run_id":"run-1"`) {
		t.Fatalf("usage file missing run id:\n%s", string(usageData))
	}
	index, err := store.Index()
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Stories) != 1 || index.Stories[0].Events != 1 {
		t.Fatalf("usage should not increment story event count: %#v", index.Stories)
	}
}

func TestSnapshotOrdersTokenUsageEventsByCreatedAt(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{
		Title:         "后置用量",
		StoryTellerID: "classic",
	})
	if err != nil {
		t.Fatalf("CreateStory failed: %v", err)
	}

	err = store.AppendTokenUsageEvent(story.ID, TokenUsageEvent{
		ID:         "usage-new",
		BranchID:   "main",
		CreatedAt:  "2026-06-22T10:02:00Z",
		RunID:      "run-new",
		ModelCalls: 1,
		UsageCalls: []TokenUsageCall{{
			Index:        1,
			PromptTokens: 20,
			TotalTokens:  20,
		}},
	})
	if err != nil {
		t.Fatalf("AppendTokenUsageEvent new failed: %v", err)
	}
	err = store.AppendTokenUsageEvent(story.ID, TokenUsageEvent{
		ID:         "usage-old",
		BranchID:   "main",
		CreatedAt:  "2026-06-22T10:01:00Z",
		RunID:      "run-old",
		ModelCalls: 1,
		UsageCalls: []TokenUsageCall{{
			Index:        1,
			PromptTokens: 10,
			TotalTokens:  10,
		}},
	})
	if err != nil {
		t.Fatalf("AppendTokenUsageEvent old failed: %v", err)
	}

	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}
	if len(snapshot.TokenUsageEvents) != 2 {
		t.Fatalf("usage events missing from snapshot: %#v", snapshot.TokenUsageEvents)
	}
	if snapshot.TokenUsageEvents[0].RunID != "run-old" || snapshot.TokenUsageEvents[1].RunID != "run-new" {
		t.Fatalf("usage events should follow created_at order: %#v", snapshot.TokenUsageEvents)
	}
}

func TestAppendTurnWithStatePersistsTurnAndDeltaAtomically(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{
		Title:         "酒馆",
		Origin:        "推门进入酒馆",
		StoryTellerID: "classic",
	})
	if err != nil {
		t.Fatalf("CreateStory failed: %v", err)
	}

	turn, delta, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我点燃火把",
		Narrative: "火光照亮了墙上的新线索。",
		Thinking:  "先判断现场风险。",
		Ops: []StateOp{
			{Op: "set", Path: "on_stage", Value: []any{"林川"}},
			{Op: "merge", Path: "characters.林川", Value: map[string]any{"location": "黄泉酒馆"}},
		},
		HotState: &HotState{Choices: []string{
			"我靠近地窖门，观察门缝和周围痕迹。",
			"",
			"我靠近地窖门，观察门缝和周围痕迹。",
			"我回头询问柜台后的影子是谁。",
		}},
	})
	if err != nil {
		t.Fatalf("AppendTurnWithState failed: %v", err)
	}
	if delta == nil {
		t.Fatal("expected state_delta event")
	}
	if delta.ID != turn.ID {
		t.Fatalf("delta id = %q, want turn id %q", delta.ID, turn.ID)
	}
	if turn.StateDelta == nil || len(turn.StateDelta.Ops) != 2 {
		t.Fatalf("expected turn to carry embedded state delta: %#v", turn.StateDelta)
	}
	if turn.HotState == nil || len(turn.HotState.Choices) != 2 {
		t.Fatalf("expected normalized hot state choices: %#v", turn.HotState)
	}

	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}
	if len(snapshot.Turns) != 1 || snapshot.Turns[0].Narrative != "火光照亮了墙上的新线索。" || snapshot.Turns[0].Thinking != "先判断现场风险。" {
		t.Fatalf("unexpected turns: %+v", snapshot.Turns)
	}
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.ID != turn.ID {
		t.Fatalf("current turn = %#v, want %s", snapshot.CurrentTurn, turn.ID)
	}
	onStage, ok := snapshot.State["on_stage"].([]any)
	if !ok || len(onStage) != 1 || onStage[0] != "林川" {
		t.Fatalf("unexpected on_stage: %#v", snapshot.State["on_stage"])
	}
	characters := snapshot.State["characters"].(map[string]any)
	linchuan := characters["林川"].(map[string]any)
	if linchuan["location"] != "黄泉酒馆" {
		t.Fatalf("unexpected character state: %#v", linchuan)
	}

	data, err := os.ReadFile(filepath.Join(store.Root(), "interactive", "story", "story-"+story.ID+".jsonl"))
	if err != nil {
		t.Fatalf("read story file failed: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("jsonl line count = %d, want 2\n%s", len(lines), string(data))
	}
	var turnLine map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &turnLine); err != nil {
		t.Fatalf("parse turn line failed: %v", err)
	}
	if turnLine["type"] != "turn" {
		t.Fatalf("unexpected event type: %#v", turnLine["type"])
	}
	if turnLine["thinking"] != "先判断现场风险。" {
		t.Fatalf("thinking in file = %q, want persisted thinking", turnLine["thinking"])
	}
	if _, ok := turnLine["alts"]; ok {
		t.Fatalf("fresh generated turn should not persist alts: %#v", turnLine["alts"])
	}
	if _, ok := turnLine["alt_idx"]; ok {
		t.Fatalf("fresh generated turn should not persist alt_idx: %#v", turnLine["alt_idx"])
	}
	stateDelta, ok := turnLine["state_delta"].(map[string]any)
	if !ok {
		t.Fatalf("turn should embed state_delta: %#v", turnLine)
	}
	ops, ok := stateDelta["ops"].([]any)
	if !ok || len(ops) != 2 {
		t.Fatalf("embedded state_delta ops = %#v, want 2 ops", stateDelta["ops"])
	}
	if _, ok := turnLine["state_delta"].(map[string]any)["narrative"]; ok {
		t.Fatalf("state_delta should not contain narrative: %#v", turnLine["state_delta"])
	}
	hotState, ok := turnLine["hot_state"].(map[string]any)
	if !ok {
		t.Fatalf("turn should embed hot_state: %#v", turnLine)
	}
	choices, ok := hotState["choices"].([]any)
	if !ok || len(choices) != 2 {
		t.Fatalf("hot_state choices = %#v, want 2 choices", hotState["choices"])
	}
	if _, ok := turnLine["state"]; ok {
		t.Fatalf("turn should not persist copied full state: %#v", turnLine["state"])
	}
}

func TestAppendTurnWithStatePersistsDisplayEventTimelineDetails(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{
		Title:         "工具时间线",
		Origin:        "主角检查档案室",
		StoryTellerID: "classic",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "检查档案柜",
		Narrative: "档案柜里露出一张潮湿的地图。",
		DisplayEvents: []DisplayEvent{
			{Role: "thinking", Content: "先确认可用线索。"},
			{ID: "call-1", Role: "tool_call", Name: "list_lore_items", Args: `{"query":"档案柜"}`, Status: "success", Result: "找到 2 条资料"},
			{Role: "thinking", Content: "基于资料继续判断下一步。"},
			{ID: "call-2", Role: "tool_call", Name: "apply_story_memory_patches", Args: `{"patches":[{"table":"plot_summary"}]}`, Status: "success", Result: "已写入 1 条记忆"},
		},
	})
	if err != nil {
		t.Fatalf("AppendTurnWithState failed: %v", err)
	}

	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	events := snapshot.Turns[0].DisplayEvents
	if len(events) != 4 {
		t.Fatalf("display event count = %d, want 4: %#v", len(events), events)
	}
	wantRoles := []string{"thinking", "tool_call", "thinking", "tool_call"}
	for i, want := range wantRoles {
		if events[i].Role != want {
			t.Fatalf("event[%d].role = %q, want %q; events=%#v", i, events[i].Role, want, events)
		}
	}
	if events[1].Args != `{"query":"档案柜"}` || events[1].Result != "找到 2 条资料" {
		t.Fatalf("first tool details not persisted: %#v", events[1])
	}
	if events[3].Args == "" || events[3].Result != "已写入 1 条记忆" {
		t.Fatalf("second tool details not persisted: %#v", events[3])
	}
}

func TestAppendTurnWithStateCanFinalizePendingState(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{
		Title:         "异步状态",
		Origin:        "主角站在门前",
		StoryTellerID: "classic",
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, delta, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我推门",
		Narrative: "门轴发出轻响。",
	})
	if err != nil {
		t.Fatalf("AppendTurnWithState failed: %v", err)
	}
	if delta != nil {
		t.Fatalf("pending turn should not return state delta: %#v", delta)
	}
	if turn.StateStatus != "pending" || turn.StateDelta != nil {
		t.Fatalf("unexpected pending turn: %#v", turn)
	}

	_, err = store.AppendStateDelta(story.ID, AppendStateDeltaRequest{
		ParentID: turn.ID,
		BranchID: "main",
		Ops: []StateOp{
			{Op: "set", Path: "on_stage", Value: []any{"主角"}},
		},
	})
	if err != nil {
		t.Fatalf("AppendStateDelta failed: %v", err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}
	last := snapshot.Turns[len(snapshot.Turns)-1]
	if last.StateStatus != "ready" || last.StateDelta == nil || len(last.StateDelta.Ops) != 1 {
		t.Fatalf("turn should be finalized with state delta: %#v", last)
	}
	onStage := snapshot.State["on_stage"].([]any)
	if len(onStage) != 1 || onStage[0] != "主角" {
		t.Fatalf("unexpected state: %#v", snapshot.State)
	}
}

func TestStoryGraphLinksTurnsDirectlyWhenStateDeltaIsEmbedded(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{
		Title:         "可见父节点",
		Origin:        "岔路口",
		StoryTellerID: "classic",
	})
	if err != nil {
		t.Fatalf("CreateStory failed: %v", err)
	}

	first, delta, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "检查石门",
		Narrative: "石门上的符文被逐一点亮。",
		Ops:       []StateOp{{Op: "set", Path: "scene.mood", Value: "紧张"}},
	})
	if err != nil {
		t.Fatalf("AppendTurnWithState failed: %v", err)
	}
	if delta == nil {
		t.Fatal("expected state delta")
	}
	second, err := store.AppendTurn(story.ID, AppendTurnRequest{
		BranchID:  "main",
		User:      "推开石门",
		Narrative: "门后传来潮湿的风。",
	})
	if err != nil {
		t.Fatalf("AppendTurn failed: %v", err)
	}
	if second.ParentID != first.ID {
		t.Fatalf("persisted turn parent = %q, want previous turn %q", second.ParentID, first.ID)
	}

	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}
	nodesByID := make(map[string]PlotNode, len(snapshot.Graph.Nodes))
	for _, node := range snapshot.Graph.Nodes {
		nodesByID[node.ID] = node
	}
	if nodesByID[second.ID].ParentID != first.ID {
		t.Fatalf("graph parent = %q, want nearest visible turn %q; nodes=%#v", nodesByID[second.ID].ParentID, first.ID, snapshot.Graph.Nodes)
	}
}

func TestSnapshotReadsLargePersistedTurn(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{
		Title:         "长篇回合",
		StoryTellerID: "classic",
	})
	if err != nil {
		t.Fatalf("CreateStory failed: %v", err)
	}
	longNarrative := strings.Repeat("很长的正文。", 20000)
	_, err = store.AppendTurn(story.ID, AppendTurnRequest{
		BranchID:  "main",
		User:      "继续",
		Narrative: longNarrative,
	})
	if err != nil {
		t.Fatalf("AppendTurn failed: %v", err)
	}

	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}
	if len(snapshot.Turns) != 1 || snapshot.Turns[0].Narrative != longNarrative {
		t.Fatalf("large turn was not restored")
	}
}

func TestCreateAndSwitchBranch(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "分支故事", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "向左走", Narrative: "你走向左侧长廊。"})
	if err != nil {
		t.Fatal(err)
	}
	branch, err := store.CreateBranch(story.ID, CreateBranchRequest{
		ParentEventID: turn.ID,
		Title:         "改向右走",
	})
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	if branch.ID == "" || branch.From != "main" || branch.Head != turn.ID {
		t.Fatalf("unexpected branch: %#v", branch)
	}
	if err := store.SwitchBranch(story.ID, branch.ID); err != nil {
		t.Fatalf("SwitchBranch failed: %v", err)
	}
	snapshot, err := store.Snapshot(story.ID, branch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.BranchID != branch.ID || len(snapshot.Turns) != 1 {
		t.Fatalf("branch snapshot should inherit parent turn: %#v", snapshot)
	}
}

func TestDeleteEmptyBranch(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "清理分支", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "向左走", Narrative: "你走向左侧长廊。"})
	if err != nil {
		t.Fatal(err)
	}
	branch, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: turn.ID, Title: "空分支"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteBranch(story.ID, branch.ID); err != nil {
		t.Fatalf("DeleteBranch failed: %v", err)
	}
	branches, err := store.Branches(story.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(branches) != 1 || branches[0].ID != "main" {
		t.Fatalf("unexpected branches after delete: %#v", branches)
	}
}

func TestDeleteBranchWithOwnTurnFails(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "保护分支", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "向左走", Narrative: "你走向左侧长廊。"})
	if err != nil {
		t.Fatal(err)
	}
	branch, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: turn.ID, Title: "已有内容"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: branch.ID, User: "改走右边", Narrative: "你看见另一扇门。"}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteBranch(story.ID, branch.ID); err == nil {
		t.Fatal("expected non-empty branch delete to fail")
	}
}

func TestBranchSnapshotFollowsParentChain(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "父链故事", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "进入密林", Narrative: "树影吞没了来路。"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "继续深入", Narrative: "前方出现断桥。"})
	if err != nil {
		t.Fatal(err)
	}
	branch, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: first.ID, Title: "折返路线"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.AppendTurn(story.ID, AppendTurnRequest{BranchID: branch.ID, User: "折返回营地", Narrative: "你在旧营地发现脚印。"})
	if err != nil {
		t.Fatal(err)
	}

	snapshot, err := store.Snapshot(story.ID, branch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Turns) != 2 {
		t.Fatalf("branch snapshot should contain only parent chain turns, got %#v", snapshot.Turns)
	}
	if snapshot.Turns[0].ID != first.ID || snapshot.Turns[1].BranchID != branch.ID {
		t.Fatalf("unexpected branch path: %#v", snapshot.Turns)
	}
	for _, turn := range snapshot.Turns {
		if turn.ID == second.ID {
			t.Fatalf("snapshot included future sibling turn: %#v", snapshot.Turns)
		}
	}
	if len(snapshot.Graph.Nodes) != 3 {
		t.Fatalf("graph should expose all plot nodes, got %#v", snapshot.Graph.Nodes)
	}
}

func TestSwitchTurnVersionKeepsLaterCanonicalPath(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "回合版本", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "推门", Narrative: "旧门发出沉闷声响。"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RewindToTurnParent(story.ID, RewindTurnRequest{BranchID: "main", TurnID: first.ID}); err != nil {
		t.Fatal(err)
	}
	firstAlt, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "推门", Narrative: "门后亮起冷白灯光。"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "继续", Narrative: "你停在门槛前观察。"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RewindToTurnParent(story.ID, RewindTurnRequest{BranchID: "main", TurnID: second.ID}); err != nil {
		t.Fatal(err)
	}
	secondAlt, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "继续", Narrative: "你跨过门槛，听见远处脚步声。"})
	if err != nil {
		t.Fatal(err)
	}

	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Turns) != 2 || snapshot.Turns[0].ID != firstAlt.ID || snapshot.Turns[1].ID != secondAlt.ID {
		t.Fatalf("unexpected path before switch: %#v", snapshot.Turns)
	}
	if len(snapshot.Turns[0].Versions) != 2 || len(snapshot.Turns[1].Versions) != 2 {
		t.Fatalf("expected independent version groups before switch: %#v", snapshot.Turns)
	}

	if err := store.SwitchTurnVersion(story.ID, SwitchTurnVersionRequest{
		BranchID:      "main",
		TurnID:        firstAlt.ID,
		VersionTurnID: first.ID,
	}); err != nil {
		t.Fatalf("SwitchTurnVersion failed: %v", err)
	}
	snapshot, err = store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Turns) != 2 || snapshot.Turns[0].ID != first.ID || snapshot.Turns[1].ID != secondAlt.ID {
		t.Fatalf("switching an earlier version should keep the chosen later canon path: %#v", snapshot.Turns)
	}
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.ID != secondAlt.ID {
		t.Fatalf("current turn should stay on later canon: %#v", snapshot.CurrentTurn)
	}

	third, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "再继续", Narrative: "你沿着脚步声走向走廊深处。"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Turns) != 3 || snapshot.Turns[0].ID != first.ID || snapshot.Turns[1].ID != secondAlt.ID || snapshot.Turns[2].ID != third.ID {
		t.Fatalf("new turn should continue from the selected canon path: %#v", snapshot.Turns)
	}
	if parentIDString(third.ParentID) != secondAlt.ID {
		t.Fatalf("new turn parent = %q, want %q", parentIDString(third.ParentID), secondAlt.ID)
	}
}

func TestBackgroundTurnUpdatesDoNotRewindBranchHead(t *testing.T) {
	t.Run("state delta", func(t *testing.T) {
		store := NewStore(t.TempDir())
		story, err := store.CreateStory(CreateStoryRequest{Title: "状态晚到", StoryTellerID: "classic"})
		if err != nil {
			t.Fatal(err)
		}
		first, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "第一步", Narrative: "第一段。"})
		if err != nil {
			t.Fatal(err)
		}
		second, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "第二步", Narrative: "第二段。"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.AppendStateDelta(story.ID, AppendStateDeltaRequest{
			ParentID: first.ID,
			BranchID: "main",
			Ops:      []StateOp{{Op: "set", Path: "scene.phase", Value: "late-state"}},
		}); err != nil {
			t.Fatal(err)
		}

		snapshot, err := store.Snapshot(story.ID, "main")
		if err != nil {
			t.Fatal(err)
		}
		if len(snapshot.Turns) != 2 || snapshot.Turns[0].ID != first.ID || snapshot.Turns[1].ID != second.ID {
			t.Fatalf("late state update should not rewind branch head: %#v", snapshot.Turns)
		}
		if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.ID != second.ID {
			t.Fatalf("current turn should remain latest after late state update: %#v", snapshot.CurrentTurn)
		}
	})

	t.Run("memory ready", func(t *testing.T) {
		store := NewStore(t.TempDir())
		story, err := store.CreateStory(CreateStoryRequest{Title: "记忆晚到", StoryTellerID: "classic"})
		if err != nil {
			t.Fatal(err)
		}
		first, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "第一步", Narrative: "第一段。"})
		if err != nil {
			t.Fatal(err)
		}
		second, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "第二步", Narrative: "第二段。"})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.MarkInteractiveMemoryReady(story.ID, "main", first.ID); err != nil {
			t.Fatal(err)
		}

		snapshot, err := store.Snapshot(story.ID, "main")
		if err != nil {
			t.Fatal(err)
		}
		if len(snapshot.Turns) != 2 || snapshot.Turns[0].ID != first.ID || snapshot.Turns[1].ID != second.ID {
			t.Fatalf("late memory update should not rewind branch head: %#v", snapshot.Turns)
		}
		if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.ID != second.ID {
			t.Fatalf("current turn should remain latest after late memory update: %#v", snapshot.CurrentTurn)
		}
	})
}

func TestUpdateAndDeleteStory(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	story, err := store.CreateStory(CreateStoryRequest{Title: "旧标题", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	target := 900
	updated, err := store.UpdateStory(story.ID, UpdateStoryRequest{Title: "新标题", StoryTellerID: "grimdark", ReplyTargetChars: &target})
	if err != nil {
		t.Fatalf("UpdateStory failed: %v", err)
	}
	if updated.Title != "新标题" || updated.StoryTellerID != "grimdark" || updated.ReplyTargetChars != 900 {
		t.Fatalf("unexpected updated story: %#v", updated)
	}
	invalidTarget := 0
	if _, err := store.UpdateStory(story.ID, UpdateStoryRequest{ReplyTargetChars: &invalidTarget}); err == nil {
		t.Fatal("expected invalid reply target chars to fail")
	}
	if err := store.DeleteStory(story.ID); err != nil {
		t.Fatalf("DeleteStory failed: %v", err)
	}
	index, err := store.Index()
	if err != nil {
		t.Fatal(err)
	}
	if index.CurrentStoryID != "" || len(index.Stories) != 0 {
		t.Fatalf("story should be removed from index: %#v", index)
	}
	if _, err := os.Stat(filepath.Join(root, "interactive", "story", "story-"+story.ID+".jsonl")); !os.IsNotExist(err) {
		t.Fatalf("story file should be removed, err=%v", err)
	}
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !contains(got, want) {
		t.Fatalf("expected %q to contain %q", got, want)
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
