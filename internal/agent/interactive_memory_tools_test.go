package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"

	"denova/internal/interactive"
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

func TestApplyStoryMemoryPatchesToolWritesValidatedCurrentBranchRecords(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "导演记忆工具", Origin: "主角进入旧城"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
		BranchID:  "main",
		User:      "我提醒林川小心钟楼",
		Narrative: "林川点头，确认钟楼里有人盯着我们。",
	})
	if err != nil {
		t.Fatal(err)
	}
	applied := 0
	tools, err := newInteractiveStoryMemoryPatchTools(InteractiveStoryToolContext{
		Store:    store,
		StoryID:  story.ID,
		BranchID: "main",
		TurnID:   turn.ID,
		OnStoryMemoryApplied: func(count int) {
			applied += count
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 {
		t.Fatalf("tool count = %d, want 1", len(tools))
	}
	info, err := tools[0].Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "apply_story_memory_patches" {
		t.Fatalf("tool name = %s", info.Name)
	}
	output, err := tools[0].(tool.InvokableTool).InvokableRun(context.Background(), `{"patches":[{"op":"upsert","structure_id":"important_character","values":{"name":"林川","current_status":"确认钟楼有人盯梢","relationship_to_protagonist":"接受主角提醒并共同戒备","unexpected_field":"should be dropped"}}]}`)
	if err != nil {
		t.Fatal(err)
	}
	var parsed storyMemoryPatchToolOutput
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("invalid tool output JSON: %v\n%s", err, output)
	}
	if parsed.AppliedRecords != 1 || parsed.BranchID != "main" || parsed.TurnID != turn.ID || applied != 1 {
		t.Fatalf("unexpected tool output/callback: output=%#v applied=%d", parsed, applied)
	}
	memory, err := store.StoryMemory(story.ID, "main", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(memory.Records) != 1 {
		t.Fatalf("record count = %d, want 1: %#v", len(memory.Records), memory.Records)
	}
	record := memory.Records[0]
	if record.StructureID != "important_character" || record.BranchID != "main" || record.AnchorTurnID != turn.ID || record.Key != "林川" {
		t.Fatalf("unexpected current-branch record: %#v", record)
	}
	if record.Values["current_status"] != "确认钟楼有人盯梢" || record.Values["unexpected_field"] != "" {
		t.Fatalf("record values should be schema-filtered: %#v", record.Values)
	}
}

func TestPrepareInteractiveTurnToolUsesRuleResolutionCallback(t *testing.T) {
	expected := interactive.RuleResolution{
		ID: "rr_test",
		Request: interactive.TurnCheckRequest{
			Action:     "强闯秘境",
			Intent:     "冒险",
			Challenge:  "秘境入口禁制",
			Cost:       "失败会受伤",
			State:      "禁制正在收束",
			Difficulty: "normal",
		},
		Result: interactive.RuleResult{ID: "check_1", Dice: "1d20", Rolls: []int{7}, RollMode: "normal", KeptRoll: 7, Total: 7, Target: 15, Outcome: "failure", Result: "强闯失败"},
	}
	tools, err := newInteractiveTurnTools(InteractiveStoryToolContext{
		StoryID:  "st_tool",
		BranchID: "main",
		PrepareTurn: func(ctx context.Context, request interactive.TurnCheckRequest) (interactive.RuleResolution, error) {
			if request.Action != "强闯秘境" || request.Intent != "冒险" || request.Challenge != "秘境入口禁制" {
				t.Fatalf("unexpected request: %#v", request)
			}
			return expected, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 {
		t.Fatalf("tool count = %d, want 1", len(tools))
	}
	info, err := tools[0].Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "prepare_interactive_turn" {
		t.Fatalf("tool name = %s", info.Name)
	}
	output, err := tools[0].(tool.InvokableTool).InvokableRun(context.Background(), `{"action":"强闯秘境","intent":"冒险","challenge":"秘境入口禁制","cost":"失败会受伤","state":"禁制正在收束","difficulty":"normal","outcomes":{"critical_success":{"result":"大成功"},"success":{"result":"成功"},"failure":{"result":"强闯失败"},"critical_failure":{"result":"大失败"}}}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, `"resolution_id": "rr_test"`) || !strings.Contains(output, `"outcome": "failure"`) || strings.Contains(output, "accepted_brief") || strings.Contains(output, `"seed"`) || strings.Contains(output, `"request"`) {
		t.Fatalf("unexpected output: %s", output)
	}
}

func TestSubmitInteractiveTurnResultToolStagesStructuredOutcome(t *testing.T) {
	want := interactive.TurnResult{
		Contract:       interactive.TurnContract{PlayerIntent: "调查丹炉", SceneGoal: "确认药味来源"},
		FactCandidates: []interactive.StoryFactCandidate{{Kind: "clue", Subject: "丹炉", Fact: "药味中混有陌生灵气", Visibility: "player_known", Importance: "high"}},
		Choices:        []string{"检查炉灰", "询问苏灿灿"},
	}
	called := false
	tools, err := newInteractiveTurnTools(InteractiveStoryToolContext{
		SubmitTurnResult: func(_ context.Context, input interactive.TurnResult) (interactive.TurnResult, error) {
			called = true
			if input.Contract.SceneGoal != want.Contract.SceneGoal {
				t.Fatalf("unexpected turn result: %#v", input)
			}
			return input, nil
		},
	})
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
	submit, ok := byName["submit_interactive_turn_result"].(tool.InvokableTool)
	if !ok {
		t.Fatalf("submit_interactive_turn_result missing: %#v", byName)
	}
	payload, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	result, err := submit.InvokableRun(context.Background(), string(payload))
	if err != nil {
		t.Fatal(err)
	}
	if !called || !strings.Contains(result, "确认药味来源") {
		t.Fatalf("submit callback/result mismatch: called=%v result=%s", called, result)
	}
}

func TestPrepareInteractiveTurnToolSchemaDocumentsEnums(t *testing.T) {
	tools, err := newInteractiveTurnTools(InteractiveStoryToolContext{
		PrepareTurn: func(ctx context.Context, request interactive.TurnCheckRequest) (interactive.RuleResolution, error) {
			return interactive.RuleResolution{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 {
		t.Fatalf("tool count = %d, want 1", len(tools))
	}
	info, err := tools[0].Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.ParamsOneOf == nil {
		t.Fatal("prepare_interactive_turn should expose parameter schema")
	}
	params, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	schemaText := string(data)
	for _, want := range []string{
		`"difficulty"`,
		`"very_easy"`,
		`"easy"`,
		`"normal"`,
		`"hard"`,
		`"very_hard"`,
		`"template"`,
		`"dice_check"`,
		`"1d20"`,
		`"modifier"`,
		`"binding_id"`,
		`"actor_id"`,
		`"target_actor_id"`,
		`"roll_mode"`,
		`"advantage"`,
		`"disadvantage"`,
		`用户行为`,
		`四档后果`,
	} {
		if !strings.Contains(schemaText, want) {
			t.Fatalf("prepare_interactive_turn schema missing %q:\n%s", want, schemaText)
		}
	}
	if !strings.Contains(info.Desc, "difficulty") || !strings.Contains(info.Desc, "very_easy/easy/normal/hard/very_hard") || !strings.Contains(info.Desc, "固定 d20") || !strings.Contains(info.Desc, "正数更难") || !strings.Contains(info.Desc, "difficulty_guidance") || !strings.Contains(info.Desc, "state_effect_guidance") || !strings.Contains(info.Desc, "must_check_examples") || !strings.Contains(info.Desc, "skip_check_examples") || !strings.Contains(info.Desc, "state_bindings") {
		t.Fatalf("prepare_interactive_turn description should spell out enum protocol:\n%s", info.Desc)
	}
}

func TestInteractiveMemoryReadToolReturnsAllRequestedContent(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "长记忆工具测试", Origin: "主角整理档案"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{User: "整理档案", Narrative: "档案柜逐层打开。"})
	if err != nil {
		t.Fatal(err)
	}

	ids := make([]string, 0, 7)
	for i := 0; i < 7; i++ {
		tail := fmt.Sprintf("完整结尾-%02d", i)
		memory, err := store.AppendInteractiveMemory(story.ID, "main", turn.ID, interactive.InteractiveMemoryCreateRequest{
			Title:      fmt.Sprintf("长记忆 %02d", i),
			Summary:    fmt.Sprintf("摘要 %02d", i),
			Content:    strings.Repeat("长正文", 1800) + tail,
			Importance: 5,
		})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, memory.ID)
	}

	tools, err := newInteractiveMemoryTools(InteractiveStoryToolContext{Store: store, StoryID: story.ID, BranchID: "main"})
	if err != nil {
		t.Fatal(err)
	}
	var readTool tool.InvokableTool
	for _, item := range tools {
		info, err := item.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if info.Name == "read_interactive_memories" {
			readTool = item.(tool.InvokableTool)
			break
		}
	}
	if readTool == nil {
		t.Fatalf("expected read_interactive_memories tool")
	}

	idsJSON, err := json.Marshal(ids)
	if err != nil {
		t.Fatal(err)
	}
	readOutput, err := readTool.InvokableRun(context.Background(), `{"ids":`+string(idsJSON)+`}`)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Truncated bool                                 `json:"truncated"`
		Memories  []interactive.InteractiveMemoryEntry `json:"memories"`
	}
	if err := json.Unmarshal([]byte(readOutput), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Truncated {
		t.Fatalf("read output should not be truncated:\n%s", readOutput)
	}
	if len(parsed.Memories) != len(ids) {
		t.Fatalf("memory count = %d, want %d", len(parsed.Memories), len(ids))
	}
	for i, memory := range parsed.Memories {
		if !strings.Contains(memory.Content, fmt.Sprintf("完整结尾-%02d", i)) {
			t.Fatalf("memory %d content was truncated", i)
		}
	}
}
