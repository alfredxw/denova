package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"nova/config"
	"nova/internal/book"
	"nova/internal/interactive"
)

func TestInteractiveConversationBuildsHistoryAndPersistsAssistantToStory(t *testing.T) {
	workspace := t.TempDir()
	loreStore := book.NewLoreStore(workspace)
	if _, err := loreStore.Create(book.LoreItemInput{ID: "hero", Type: "character", Name: "林川", Importance: "major", LoadMode: book.LoreLoadModeResident, Content: "林川：谨慎的幸存者"}); err != nil {
		t.Fatal(err)
	}
	if _, err := loreStore.Create(book.LoreItemInput{ID: "world", Type: "world", Name: "黄昏末日", Importance: "major", LoadMode: book.LoreLoadModeResident, Content: "世界已进入黄昏末日。"}); err != nil {
		t.Fatal(err)
	}
	if _, err := loreStore.Create(book.LoreItemInput{ID: "base", Type: "location", Name: "黄泉酒馆", Importance: "important", LoadMode: book.LoreLoadModeAuto, BriefDescription: "黄泉酒馆据点索引", Content: "黄泉酒馆完整设定：柜台后的影子不能离开酒馆。"}); err != nil {
		t.Fatal(err)
	}
	novaDir := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:            "末日开端",
		Origin:           "主角醒来发现世界已末日",
		StoryTellerID:    "classic",
		ReplyTargetChars: 800,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
		User:      "我推开酒馆的门",
		Narrative: "门后传来低沉的风声。",
	}); err != nil {
		t.Fatal(err)
	}

	conversation := newInteractiveConversation(store, novaDir, workspace, story.ID, "", "我点燃火把", story.ReplyTargetChars, nil)
	history, err := conversation.PrepareMessages("我点燃火把", "我点燃火把")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 3 {
		t.Fatalf("history length = %d, want 3", len(history))
	}
	if history[0].Role != schema.User || history[0].Content != "我推开酒馆的门" {
		t.Fatalf("history[0] mismatch: %#v", history[0])
	}
	if strings.Contains(history[0].Content, "故事记忆") || strings.Contains(history[0].Content, "最高篇幅约束") {
		t.Fatalf("history[0] should remain plain recent history, got: %#v", history[0])
	}
	if history[1].Role != schema.Assistant || history[1].Content != "门后传来低沉的风声。" {
		t.Fatalf("history[1] mismatch: %#v", history[1])
	}
	if history[2].Role != schema.User || !strings.Contains(history[2].Content, "我点燃火把") || strings.Contains(history[2].Content, "</STATE_DELTA>") {
		t.Fatalf("history[2] mismatch: %#v", history[2])
	}
	for _, want := range []string{
		"导演本轮上下文规则",
		"导演随机事件率",
		"[本轮动态上下文]",
		"末日开端",
		"主角醒来发现世界已末日",
		"800 个中文字",
		"最高篇幅约束",
		"list_lore_items",
		"list_interactive_memories",
		"当前分支故事记忆",
		`"on_stage"`,
	} {
		if !strings.Contains(history[2].Content, want) {
			t.Fatalf("history[2] should include %q: %#v", want, history[2])
		}
	}
	for _, forbidden := range []string{"经典叙事者", "林川：谨慎的幸存者", "世界已进入黄昏末日。"} {
		if strings.Contains(history[2].Content, forbidden) {
			t.Fatalf("history[2] should not include %q: %#v", forbidden, history[2])
		}
	}
	if sources := conversation.ContextSourceSummary(); !strings.Contains(sources, "导演注入规则") || !strings.Contains(sources, "本轮上下文") {
		t.Fatalf("context sources should include teller slots: %s", sources)
	}

	if err := conversation.AppendAssistantWithThinking(`<NARRATIVE>
火光照亮了墙上的新线索。
</NARRATIVE>
<STATE_DELTA>
{"ops":[{"op":"set","path":"on_stage","value":["林川"]},{"op":"merge","path":"characters.林川","value":{"location":"黄泉酒馆"}}]}
</STATE_DELTA>`, "先判断现场风险。"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(story.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Turns) != 2 {
		t.Fatalf("turn count = %d, want 2", len(snapshot.Turns))
	}
	last := snapshot.Turns[1]
	if last.User != "我点燃火把" || last.Narrative != "火光照亮了墙上的新线索。" {
		t.Fatalf("last turn mismatch: %#v", last)
	}
	if last.Thinking != "先判断现场风险。" {
		t.Fatalf("last thinking = %q, want persisted thinking", last.Thinking)
	}
	if last.StateDelta == nil || len(last.StateDelta.Ops) != 2 {
		t.Fatalf("last turn should persist state_delta: %#v", last.StateDelta)
	}
	stateInstruction, err := conversation.BuildStateInstruction(last)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stateInstruction, "导演记忆沉淀规则") || !strings.Contains(stateInstruction, "帮助后续回合稳定承接") {
		t.Fatalf("state instruction should include state_memory rules: %s", stateInstruction)
	}
	if !strings.Contains(stateInstruction, "黄泉酒馆完整设定") {
		t.Fatalf("state instruction should include bounded full lore for memory calibration: %s", stateInstruction)
	}
	for _, want := range []string{
		"故事记忆结构与字段协议",
		"## important_character",
		"key_field_id: name",
		"name（姓名） required",
		"plot_summary",
		"最近回合上下文历史",
		"第 2 回合用户行动：我点燃火把",
	} {
		if !strings.Contains(stateInstruction, want) {
			t.Fatalf("state instruction should include story memory schema %q: %s", want, stateInstruction)
		}
	}
	if strings.Contains(stateInstruction, "经典叙事者") || strings.Contains(stateInstruction, "导演本轮上下文规则") {
		t.Fatalf("state instruction should not include story-only teller rules: %s", stateInstruction)
	}
	onStage := snapshot.State["on_stage"].([]any)
	if len(onStage) != 1 || onStage[0] != "林川" {
		t.Fatalf("unexpected on_stage: %#v", onStage)
	}
	characters := snapshot.State["characters"].(map[string]any)
	linchuan := characters["林川"].(map[string]any)
	if linchuan["location"] != "黄泉酒馆" {
		t.Fatalf("unexpected character state: %#v", linchuan)
	}

	if err := conversation.AppendAssistant(`<NARRATIVE>
柜台后的影子露出一道能通往地窖的缝。
</NARRATIVE>
<STATE_DELTA>
{"ops":[{"op":"merge","path":"scene","value":{"danger_level":"升高","interactive_objects":["柜台","地窖门"]}},{"op":"push","path":"action_space","value":{"target":"地窖门","risk":"可能惊动柜台后的影子"}},{"op":"push","path":"threads","value":{"title":"柜台后的影子","status":"未解决"}},{"op":"push","path":"world_flags","value":"黄泉酒馆会回应火光"}]}
</STATE_DELTA>`); err != nil {
		t.Fatal(err)
	}
	snapshot, err = store.Snapshot(story.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	scene := snapshot.State["scene"].(map[string]any)
	if scene["danger_level"] != "升高" {
		t.Fatalf("unexpected scene state: %#v", scene)
	}
	actionSpace := snapshot.State["action_space"].([]any)
	if len(actionSpace) != 1 {
		t.Fatalf("unexpected action_space: %#v", actionSpace)
	}
	threads := snapshot.State["threads"].([]any)
	if len(threads) != 1 {
		t.Fatalf("unexpected threads: %#v", threads)
	}
}

func TestInteractiveConversationIgnoresLegacyTellerReplyTargetChars(t *testing.T) {
	workspace := t.TempDir()
	novaDir := t.TempDir()
	tellerDir := filepath.Join(novaDir, "story-tellers")
	if err := os.MkdirAll(tellerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacyTeller := `{
  "version": 3,
  "id": "legacy",
  "name": "旧字段导演",
  "description": "包含旧字数字段",
  "random_event_rate": 0.15,
  "reply_target_chars": 50,
  "tags": ["测试"],
  "context_policy": {
    "creator": "always",
    "lore": "relevant",
    "runtime_state": "always",
    "recent_turns": 8
  },
  "slots": [
    {
      "id": "identity",
      "name": "系统提示",
      "target": "system",
      "enabled": true,
      "content": "旧字段导演系统规则"
    },
    {
      "id": "turn_context",
      "name": "本轮上下文",
      "target": "turn_context",
      "enabled": true,
      "content": "旧字段导演本轮规则"
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(tellerDir, "legacy.json"), []byte(legacyTeller), 0o644); err != nil {
		t.Fatal(err)
	}

	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:            "旧字段测试",
		StoryTellerID:    "legacy",
		ReplyTargetChars: 700,
	})
	if err != nil {
		t.Fatal(err)
	}

	conversation := newInteractiveConversation(store, novaDir, workspace, story.ID, "", "我观察四周", story.ReplyTargetChars, nil)
	history, err := conversation.PrepareMessages("我观察四周", "我观察四周")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) < 1 || !strings.Contains(history[len(history)-1].Content, "700 个中文字") {
		t.Fatalf("story reply target chars should be used: %#v", history)
	}
	if !strings.Contains(history[len(history)-1].Content, "最高篇幅约束") {
		t.Fatalf("story reply target chars should be marked as highest priority: %#v", history[len(history)-1])
	}
	if strings.Contains(history[len(history)-1].Content, "50 个中文字") {
		t.Fatalf("legacy teller reply target chars should be ignored: %#v", history[len(history)-1])
	}
}

func TestInteractiveConversationUsesAgentContextRecentTurns(t *testing.T) {
	workspace := t.TempDir()
	novaDir := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:            "窗口测试",
		Origin:           "主角进入旧城",
		StoryTellerID:    "classic",
		ReplyTargetChars: 700,
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 4; i++ {
		if _, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
			User:      "第" + string(rune('0'+i)) + "次行动",
			Narrative: "第" + string(rune('0'+i)) + "段剧情",
		}); err != nil {
			t.Fatal(err)
		}
	}
	recentTurns := 2
	cfg := &config.Config{AgentContexts: config.AgentContextSettings{
		InteractiveStory: config.AgentContextOverride{RecentTurns: &recentTurns},
	}}
	conversation := newInteractiveConversation(store, novaDir, workspace, story.ID, "", "我继续探索", story.ReplyTargetChars, cfg)
	history, err := conversation.PrepareMessages("我继续探索", "我继续探索")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 5 {
		t.Fatalf("history length = %d, want 5", len(history))
	}
	if history[0].Content != "第3次行动" || history[2].Content != "第4次行动" {
		t.Fatalf("recent history should use agent context window: %#v", history)
	}
	if strings.Contains(history[4].Content, "第3次行动") || !strings.Contains(history[4].Content, "较早 2 个回合") {
		t.Fatalf("older turns should be summarized in runtime context: %s", history[4].Content)
	}
}

func TestInteractiveTurnMemoryCompressesOlderTurns(t *testing.T) {
	turns := []interactive.TurnEvent{
		{User: "第1次行动", Narrative: "第1段剧情"},
		{User: "第2次行动", Narrative: "第2段剧情"},
		{User: "第3次行动", Narrative: "第3段剧情"},
		{User: "第4次行动", Narrative: "第4段剧情"},
		{User: "第5次行动", Narrative: "第5段剧情"},
	}
	memory := buildInteractiveTurnMemory(turns, 2)
	if len(memory.RecentTurns) != 2 {
		t.Fatalf("recent turns = %d, want 2", len(memory.RecentTurns))
	}
	if memory.RecentTurns[0].User != "第4次行动" || memory.RecentTurns[1].User != "第5次行动" {
		t.Fatalf("unexpected recent turns: %#v", memory.RecentTurns)
	}
	if !strings.Contains(memory.PreviousSummary, "较早 3 个回合") ||
		!strings.Contains(memory.PreviousSummary, "第 1 回合") ||
		strings.Contains(memory.PreviousSummary, "第4次行动") {
		t.Fatalf("unexpected previous summary: %s", memory.PreviousSummary)
	}
}

func TestParseInteractiveAssistantOutput(t *testing.T) {
	narrative, ops, hotState, err := parseInteractiveAssistantOutput(`<NARRATIVE>
门后传来低沉的风声。
</NARRATIVE>
<HOT_STATE>
{"choices":["我贴近门缝听里面的动静。"]}
</HOT_STATE>
<STATE_DELTA>
{"ops":[{"op":"set","path":"on_stage","value":["林川"]}]}
</STATE_DELTA>`)
	if err != nil {
		t.Fatal(err)
	}
	if narrative != "门后传来低沉的风声。" || len(ops) != 1 || ops[0].Path != "on_stage" {
		t.Fatalf("unexpected parsed output narrative=%q ops=%#v", narrative, ops)
	}
	if hotState == nil || len(hotState.Choices) != 1 || hotState.Choices[0] != "我贴近门缝听里面的动静。" {
		t.Fatalf("unexpected hot state: %#v", hotState)
	}

	narrative, _, hotState, err = parseInteractiveAssistantOutput("门后传来风声。\n< hot_state >{\"choices\":[\"我推门进去。\"]}</hot_state>")
	if err != nil || narrative != "门后传来风声。" {
		t.Fatalf("expected spaced lowercase hot state to be hidden, narrative=%q hot=%#v err=%v", narrative, hotState, err)
	}
	if hotState == nil || len(hotState.Choices) != 1 || hotState.Choices[0] != "我推门进去。" {
		t.Fatalf("unexpected lowercase hot state: %#v", hotState)
	}

	narrative, ops, hotState, err = parseInteractiveAssistantOutput("<NARRATIVE>只有正文。</NARRATIVE>")
	if err != nil || narrative != "只有正文。" || len(ops) != 0 {
		t.Fatalf("expected missing state delta to preserve narrative, narrative=%q ops=%#v err=%v", narrative, ops, err)
	}
	if hotState != nil {
		t.Fatalf("unexpected hot state: %#v", hotState)
	}

	narrative, ops, _, err = parseInteractiveAssistantOutput("旧格式正文\n<STATE_DELTA>{\"ops\":[]}</STATE_DELTA>")
	if err != nil || narrative != "旧格式正文" || len(ops) != 0 {
		t.Fatalf("expected empty ops to fall back to async state, narrative=%q ops=%#v err=%v", narrative, ops, err)
	}

	narrative, ops, _, err = parseInteractiveAssistantOutput("旧格式正文\n<STATE_DELTA>{bad json}</STATE_DELTA>")
	if err != nil || narrative != "旧格式正文" || len(ops) != 0 {
		t.Fatalf("expected invalid state to fall back to async state, narrative=%q ops=%#v err=%v", narrative, ops, err)
	}

	_, _, _, err = parseInteractiveAssistantOutput("<STATE_DELTA>{\"ops\":[]}</STATE_DELTA>")
	if err == nil {
		t.Fatalf("expected empty narrative error")
	}
}
