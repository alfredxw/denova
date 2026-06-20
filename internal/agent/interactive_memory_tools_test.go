package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"

	"nova/internal/interactive"
)

func TestInteractiveMemoryToolsListReadAndRecordRecall(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "记忆工具测试", Origin: "主角进入旧站台"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{User: "我进入站台", Narrative: "站台灯光闪烁。"})
	if err != nil {
		t.Fatal(err)
	}
	branch, err := store.CreateBranch(story.ID, interactive.CreateBranchRequest{ParentEventID: turn.ID, Title: "支线"})
	if err != nil {
		t.Fatal(err)
	}
	mainMemory, err := store.AppendInteractiveMemory(story.ID, "main", turn.ID, interactive.InteractiveMemoryCreateRequest{
		Title:      "站台钥匙",
		Summary:    "林川在旧站台拿到铜钥匙。",
		Content:    "林川在旧站台售票窗口下方拿到一枚铜钥匙，钥匙上刻着北门编号。",
		People:     []string{"林川"},
		Places:     []string{"旧站台"},
		Tags:       []string{"钥匙", "线索"},
		Importance: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	archivedMemory, err := store.CreateInteractiveMemory(story.ID, interactive.InteractiveMemoryCreateRequest{
		BranchID:   "main",
		Title:      "归档记忆",
		Summary:    "这条记忆不应给模型读取。",
		Content:    "归档正文",
		Importance: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetInteractiveMemoryArchived(story.ID, archivedMemory.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateInteractiveMemory(story.ID, interactive.InteractiveMemoryCreateRequest{
		BranchID:   branch.ID,
		Title:      "支线记忆",
		Summary:    "其他分支的事实不应泄露。",
		Content:    "支线正文",
		Importance: 4,
	}); err != nil {
		t.Fatal(err)
	}

	tools, err := newInteractiveMemoryTools(InteractiveStoryToolContext{Store: store, StoryID: story.ID, BranchID: "main"})
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
	for _, name := range []string{"list_interactive_memories", "read_interactive_memories"} {
		if _, ok := byName[name]; !ok {
			t.Fatalf("expected tool %s to be registered", name)
		}
	}

	listTool := byName["list_interactive_memories"].(tool.InvokableTool)
	listOutput, err := listTool.InvokableRun(context.Background(), `{"query":"铜钥匙","limit":10}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listOutput, mainMemory.ID) || !strings.Contains(listOutput, "站台钥匙") {
		t.Fatalf("list output should contain visible current-branch memory:\n%s", listOutput)
	}
	for _, forbidden := range []string{archivedMemory.ID, "归档正文", "支线记忆", "支线正文", "北门编号"} {
		if strings.Contains(listOutput, forbidden) {
			t.Fatalf("list output leaked %q:\n%s", forbidden, listOutput)
		}
	}

	readTool := byName["read_interactive_memories"].(tool.InvokableTool)
	readOutput, err := readTool.InvokableRun(context.Background(), `{"ids":["`+mainMemory.ID+`","`+archivedMemory.ID+`"],"query":"确认钥匙来源"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readOutput, "北门编号") || strings.Contains(readOutput, "归档正文") {
		t.Fatalf("read output should include only visible requested memory:\n%s", readOutput)
	}
	var parsed struct {
		Memories []interactive.InteractiveMemoryEntry `json:"memories"`
	}
	if err := json.Unmarshal([]byte(readOutput), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Memories) != 1 || parsed.Memories[0].ID != mainMemory.ID {
		t.Fatalf("unexpected parsed memories: %#v", parsed.Memories)
	}
	state, err := store.InteractiveMemory(story.ID, "main", false)
	if err != nil {
		t.Fatal(err)
	}
	if state.RecentRecall == nil || len(state.RecentRecall.MemoryIDs) != 1 || state.RecentRecall.MemoryIDs[0] != mainMemory.ID {
		t.Fatalf("recent recall not recorded: %#v", state.RecentRecall)
	}
}
