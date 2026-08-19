package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"

	"denova/internal/interactive"
)

func TestNewInteractiveHistoryToolsRegistersMemorySearch(t *testing.T) {
	store := interactive.NewStore(t.TempDir())
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "记忆工具", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "得到蚀骨剑",
		Narrative: "林舟获得蚀骨剑,岚神色微变。",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendNarrativeMemory(story.ID, "main", interactive.NarrativeMemoryEvent{
		SourceTurnID: turn.ID,
		Records: []interactive.NarrativeMemoryRecord{
			{ID: "mem_sword", Kind: interactive.MemoryKindObjectState, Subject: "蚀骨剑", Object: "林舟", Text: "蚀骨剑在林舟手中。", Evidence: "获得蚀骨剑", ValidFrom: turn.ID},
			{ID: "mem_promise", Kind: interactive.MemoryKindPromise, Subject: "蚀骨剑", Text: "剑的来历未揭示。", Evidence: "神色微变", ValidFrom: turn.ID, Status: interactive.MemoryStatusOpen},
		},
	}); err != nil {
		t.Fatal(err)
	}

	tools, err := newInteractiveHistoryTools(InteractiveStoryToolContext{Store: store, StoryID: story.ID, BranchID: "main"})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]tool.BaseTool{}
	for _, item := range tools {
		info, err := item.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		byName[info.Name] = item
	}
	if _, ok := byName["search_story_memory"]; !ok {
		t.Fatal("search_story_memory should be registered")
	}

	memoryTool, ok := byName["search_story_memory"].(tool.InvokableTool)
	if !ok {
		t.Fatalf("search_story_memory should be invokable: %T", byName["search_story_memory"])
	}
	output, err := memoryTool.InvokableRun(context.Background(), `{"keywords":["蚀骨剑"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output, "\"explain\"") {
		t.Fatalf("tool output must not contain explain payload: %s", output)
	}
	var result interactive.MemorySearchResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatal(err)
	}
	if result.Explain != nil {
		t.Fatalf("decoded explain should be nil: %#v", result.Explain)
	}
	if len(result.Hits) == 0 {
		t.Fatalf("expected hits: %s", output)
	}
	// 伏笔加成应把 promise 排在 object_state 之前。
	if result.Hits[0].RecordID != "mem_promise" {
		t.Fatalf("promise should rank first: %#v", result.Hits)
	}
	// 溯源字段必须保留,供模型核对证据。
	if result.Hits[0].ValidFrom != turn.ID {
		t.Fatalf("hit should carry valid_from: %#v", result.Hits[0])
	}
}
