package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"nova/internal/interactive"
)

func TestInteractiveConversationBuildsHistoryAndPersistsAssistantToStory(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "setting"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "setting", "characters.md"), []byte("林川：谨慎的幸存者"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "setting", "world-building.md"), []byte("世界已进入黄昏末日。"), 0o644); err != nil {
		t.Fatal(err)
	}
	novaDir := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:         "末日开端",
		Origin:        "主角醒来发现世界已末日",
		StoryTellerID: "classic",
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

	conversation := newInteractiveConversation(store, novaDir, workspace, story.ID, "", "我点燃火把", 1200)
	history, err := conversation.PrepareMessages("我点燃火把", "我点燃火把")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 4 {
		t.Fatalf("history length = %d, want 4", len(history))
	}
	if history[0].Role != schema.User ||
		!strings.Contains(history[0].Content, "末日开端") ||
		!strings.Contains(history[0].Content, "主角醒来发现世界已末日") ||
		!strings.Contains(history[0].Content, "经典叙事者") ||
		!strings.Contains(history[0].Content, "1200 个中文字") ||
		!strings.Contains(history[0].Content, "林川：谨慎的幸存者") ||
		!strings.Contains(history[0].Content, "世界已进入黄昏末日。") ||
		!strings.Contains(history[0].Content, `"on_stage"`) {
		t.Fatalf("history[0] mismatch: %#v", history[0])
	}
	if history[1].Role != schema.User || history[1].Content != "我推开酒馆的门" {
		t.Fatalf("history[1] mismatch: %#v", history[1])
	}
	if history[2].Role != schema.Assistant || history[2].Content != "门后传来低沉的风声。" {
		t.Fatalf("history[2] mismatch: %#v", history[2])
	}
	if history[3].Role != schema.User || !strings.Contains(history[3].Content, "我点燃火把") || !strings.Contains(history[3].Content, "<NARRATIVE>") {
		t.Fatalf("history[3] mismatch: %#v", history[3])
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

func TestParseInteractiveAssistantOutput(t *testing.T) {
	narrative, ops, err := parseInteractiveAssistantOutput(`<NARRATIVE>
门后传来低沉的风声。
</NARRATIVE>
<STATE_DELTA>
{"ops":[{"op":"set","path":"on_stage","value":["林川"]}]}
</STATE_DELTA>`)
	if err != nil {
		t.Fatal(err)
	}
	if narrative != "门后传来低沉的风声。" || len(ops) != 1 || ops[0].Path != "on_stage" {
		t.Fatalf("unexpected parsed output narrative=%q ops=%#v", narrative, ops)
	}

	narrative, ops, err = parseInteractiveAssistantOutput("<NARRATIVE>只有正文。</NARRATIVE>")
	if err != nil {
		t.Fatal(err)
	}
	if narrative != "只有正文。" || len(ops) != 0 {
		t.Fatalf("unexpected no-state output narrative=%q ops=%#v", narrative, ops)
	}

	narrative, ops, err = parseInteractiveAssistantOutput("旧格式正文\n<STATE_DELTA>{\"ops\":[]}</STATE_DELTA>")
	if err != nil {
		t.Fatal(err)
	}
	if narrative != "旧格式正文" || len(ops) != 0 {
		t.Fatalf("unexpected legacy output narrative=%q ops=%#v", narrative, ops)
	}

	narrative, ops, err = parseInteractiveAssistantOutput("旧格式正文\n<STATE_DELTA>{bad json}</STATE_DELTA>")
	if err == nil || narrative != "旧格式正文" || len(ops) != 0 {
		t.Fatalf("expected invalid state to preserve narrative, narrative=%q ops=%#v err=%v", narrative, ops, err)
	}

	_, _, err = parseInteractiveAssistantOutput("<STATE_DELTA>{\"ops\":[]}</STATE_DELTA>")
	if err == nil {
		t.Fatalf("expected empty narrative error")
	}
}
