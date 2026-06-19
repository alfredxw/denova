package interactive

import (
	"strings"
	"testing"
)

func TestInteractiveMemoryStoreFiltersUpdatesAndHidesByBranch(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "记忆测试"})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "我拾起钥匙",
		Narrative: "钥匙刻着旧宅的徽记。",
	})
	if err != nil {
		t.Fatal(err)
	}
	generated, err := store.AppendInteractiveMemory(story.ID, "main", turn.ID, InteractiveMemoryCreateRequest{
		Title:      "旧宅钥匙",
		Summary:    "主角获得刻着旧宅徽记的钥匙。",
		Content:    "这把钥匙后续可以用于进入旧宅或证明主角接触过旧宅相关线索。",
		People:     []string{"主角"},
		Places:     []string{"旧宅"},
		Tags:       []string{"线索", "物品"},
		Importance: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.InteractiveMemory(story.ID, "main", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Entries) != 1 || state.Entries[0].ID != generated.ID || state.SyncStatus != "ready" {
		t.Fatalf("memory state mismatch: %#v", state)
	}
	if _, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: turn.ID, Title: "支线"}); err != nil {
		t.Fatal(err)
	}
	branchState, err := store.InteractiveMemory(story.ID, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if branchState.BranchID == "main" || len(branchState.Entries) != 1 || branchState.Entries[0].ID != generated.ID {
		t.Fatalf("branch memory should inherit pre-fork records: %#v", branchState)
	}
	updatedTitle := "铜钥匙"
	updatedImportance := 5
	updated, err := store.UpdateInteractiveMemory(story.ID, generated.ID, InteractiveMemoryUpdateRequest{
		Title:      &updatedTitle,
		Importance: &updatedImportance,
		Tags:       []string{"钥匙"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != updatedTitle {
		t.Fatalf("updated memory mismatch: %#v", updated)
	}
	mainState, err := store.InteractiveMemory(story.ID, "main", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(mainState.Entries) != 1 || mainState.Entries[0].Title != "旧宅钥匙" {
		t.Fatalf("main branch should keep original inherited memory: %#v", mainState.Entries)
	}
	if _, err := store.SetInteractiveMemoryHidden(story.ID, updated.ID, true); err != nil {
		t.Fatal(err)
	}
	state, err = store.InteractiveMemory(story.ID, branchState.BranchID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Entries) != 0 {
		t.Fatalf("hidden memory should be excluded: %#v", state.Entries)
	}
	state, err = store.InteractiveMemory(story.ID, branchState.BranchID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Entries) != 1 || !state.Entries[0].Hidden {
		t.Fatalf("hidden memory should be restorable: %#v", state.Entries)
	}
}

func TestCreateInteractiveMemoryDefaultsToCurrentBranch(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "手动记忆"})
	if err != nil {
		t.Fatal(err)
	}
	entry, err := store.CreateInteractiveMemory(story.ID, InteractiveMemoryCreateRequest{
		Title:   "手动线索",
		Summary: "用户手动补充的线索。",
	})
	if err != nil {
		t.Fatal(err)
	}
	if entry.BranchID != "main" || !entry.Manual {
		t.Fatalf("manual memory mismatch: %#v", entry)
	}
}

func TestStoryMemoryStructuresRecordsAndBranchCopyOnWrite(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "故事记忆"})
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.StoryMemory(story.ID, "main", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Structures) < 5 || state.Settings.AutoIntervalTurns != defaultStoryMemoryInterval || !state.Settings.Enabled {
		t.Fatalf("default story memory state mismatch: %#v", state)
	}
	currentState := storyMemoryStructureByID(state.Structures, "current_state")
	if currentState.Description != "记录当前主角所在地点及时间相关参数。此表有且仅有一行。" {
		t.Fatalf("current_state preset description mismatch: %#v", currentState)
	}
	for _, want := range []string{"location", "time", "previous_time", "elapsed_time"} {
		if !storyMemoryStructureHasField(currentState, want) {
			t.Fatalf("current_state preset missing field %q: %#v", want, currentState.Fields)
		}
	}
	protagonist := storyMemoryStructureByID(state.Structures, "protagonist")
	for _, want := range []string{"identity", "personality", "skills", "items"} {
		if !storyMemoryStructureHasField(protagonist, want) {
			t.Fatalf("protagonist preset missing field %q: %#v", want, protagonist.Fields)
		}
	}
	structure, err := store.SaveStoryMemoryStructure(story.ID, StoryMemoryStructureRequest{
		ID:         "relationship_clock",
		Name:       "关系时钟",
		Mode:       "keyed",
		KeyFieldID: "name",
		Fields: []StoryMemoryField{
			{ID: "name", Name: "姓名", Required: true, Order: 10},
			{ID: "status", Name: "状态", Order: 20},
		},
		Order: 90,
	})
	if err != nil {
		t.Fatal(err)
	}
	if structure.ID != "relationship_clock" {
		t.Fatalf("structure mismatch: %#v", structure)
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "我叫住林川", Narrative: "林川停下脚步。"})
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.SaveStoryMemoryRecord(story.ID, StoryMemoryRecordRequest{
		BranchID:    "main",
		StructureID: structure.ID,
		Key:         "林川",
		Values:      map[string]string{"name": "林川", "status": "开始信任主角"},
	})
	if err != nil {
		t.Fatal(err)
	}
	branch, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: turn.ID, Title: "另一种回应"})
	if err != nil {
		t.Fatal(err)
	}
	branchState, err := store.StoryMemory(story.ID, branch.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(branchState.Records) != 1 || branchState.Records[0].ID != record.ID {
		t.Fatalf("branch should inherit parent record: %#v", branchState.Records)
	}
	updated, err := store.SaveStoryMemoryRecord(story.ID, StoryMemoryRecordRequest{
		ID:          record.ID,
		BranchID:    branch.ID,
		StructureID: structure.ID,
		Key:         "林川",
		Values:      map[string]string{"name": "林川", "status": "怀疑主角"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID == record.ID || updated.InheritedFrom != record.ID {
		t.Fatalf("expected copy-on-write record, got %#v", updated)
	}
	mainState, err := store.StoryMemory(story.ID, "main", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(mainState.Records) != 1 || mainState.Records[0].Values["status"] != "开始信任主角" {
		t.Fatalf("main branch should keep original record: %#v", mainState.Records)
	}
}

func TestNormalizeMemoryBookRefreshesBuiltInStoryMemoryPresets(t *testing.T) {
	book := normalizeMemoryBook(interactiveMemoryBook{
		V:       2,
		StoryID: "story-1",
		Settings: StoryMemorySettings{
			Enabled:           true,
			AutoIntervalTurns: defaultStoryMemoryInterval,
		},
		Structures: []StoryMemoryStructure{
			{
				ID:      "current_state",
				Name:    "当前状态",
				Mode:    "singleton",
				BuiltIn: true,
				Fields: []StoryMemoryField{
					{ID: "time", Name: "时间", Order: 10},
					{ID: "location", Name: "地点", Order: 20},
					{ID: "event", Name: "当前事件", Order: 30},
				},
			},
			{
				ID:      "plot_summary",
				Name:    "剧情纪要",
				Mode:    "append",
				BuiltIn: true,
				Fields: []StoryMemoryField{
					{ID: "time", Name: "时间", Order: 10},
					{ID: "place", Name: "地点", Order: 20},
					{ID: "event", Name: "事件", Order: 30},
				},
			},
			{
				ID:     "custom",
				Name:   "自定义",
				Mode:   "append",
				Fields: []StoryMemoryField{{ID: "value", Name: "内容", Order: 10}},
			},
		},
		Records: []StoryMemoryRecord{
			{
				ID:          "mem-1",
				StructureID: "plot_summary",
				BranchID:    "main",
				Values: map[string]string{
					"time":  "旧时间",
					"place": "旧地点",
					"event": "旧事件",
				},
				CreatedAt: "2026-06-19T00:00:00Z",
				UpdatedAt: "2026-06-19T00:00:00Z",
			},
		},
	})

	currentState := storyMemoryStructureByID(book.Structures, "current_state")
	if !storyMemoryStructureHasField(currentState, "previous_time") || storyMemoryStructureHasField(currentState, "event") {
		t.Fatalf("current_state built-in preset was not refreshed: %#v", currentState.Fields)
	}
	plotSummary := storyMemoryStructureByID(book.Structures, "plot_summary")
	if plotSummary.Name != "纪要" || !storyMemoryStructureHasField(plotSummary, "time_span") || !storyMemoryStructureHasField(plotSummary, "sequence") {
		t.Fatalf("plot_summary built-in preset was not refreshed: %#v", plotSummary)
	}
	custom := storyMemoryStructureByID(book.Structures, "custom")
	if custom.Name != "自定义" || !storyMemoryStructureHasField(custom, "value") {
		t.Fatalf("custom structure should be preserved: %#v", custom)
	}
	if got := book.Records[0].Values["time_span"]; got != "旧时间" {
		t.Fatalf("legacy plot_summary time should migrate to time_span, got %q", got)
	}
}

func TestStoryMemorySchemaContextIncludesStructuresWithoutRecords(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "结构上下文"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveStoryMemoryStructure(story.ID, StoryMemoryStructureRequest{
		ID:                    "relationship_clock",
		Name:                  "关系时钟",
		Description:           "追踪关键人物关系变化",
		GenerationInstruction: "每次整理只更新已经被剧情证实的关系变化",
		Mode:                  "keyed",
		KeyFieldID:            "name",
		Fields: []StoryMemoryField{
			{ID: "name", Name: "姓名", Required: true, Description: "角色姓名或称呼", Order: 10},
			{ID: "status", Name: "状态", Description: "当前关系阶段", GenerationInstruction: "不少于 300 字，必须包含触发事件和当前态度", Order: 20},
		},
		Order: 90,
	}); err != nil {
		t.Fatal(err)
	}
	context, err := store.StoryMemorySchemaContext(story.ID, 12*1024)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"structure_id",
		"## current_state",
		"## important_character",
		"## relationship_clock",
		"mode: keyed",
		"key_field_id: name",
		"generation_instruction: 每次整理只更新已经被剧情证实的关系变化",
		"name（姓名） required: 角色姓名或称呼",
		"status（状态）: 当前关系阶段",
		"generation_instruction: 不少于 300 字，必须包含触发事件和当前态度",
	} {
		if !strings.Contains(context, want) {
			t.Fatalf("schema context missing %q:\n%s", want, context)
		}
	}
}

func storyMemoryStructureHasField(structure StoryMemoryStructure, fieldID string) bool {
	for _, field := range structure.Fields {
		if field.ID == fieldID {
			return true
		}
	}
	return false
}

func TestApplyStoryMemoryPatchesNormalizesKeyedAgentPatches(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "Agent 故事记忆"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{
		BranchID:  "main",
		User:      "我叫住林川",
		Narrative: "林川压低声音提醒我别靠近钟楼。",
	})
	if err != nil {
		t.Fatal(err)
	}
	records, err := store.ApplyStoryMemoryPatches(story.ID, "main", turn.ID, []StoryMemoryPatch{
		{
			Op:          "upsert",
			StructureID: "important_character",
			Values: map[string]string{
				"name":         "林川",
				"relationship": "提醒主角远离钟楼",
			},
		},
		{
			Op:          "upsert",
			StructureID: "quest_event",
			Values: map[string]string{
				"progress": "有人提醒钟楼危险，但任务名未知。",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Key != "林川" {
		t.Fatalf("expected one normalized keyed record, got %#v", records)
	}
	state, err := store.StoryMemory(story.ID, "main", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Records) != 1 || state.Records[0].StructureID != "important_character" {
		t.Fatalf("invalid keyless patch should be skipped without failing the batch: %#v", state.Records)
	}
	updated, err := store.ApplyStoryMemoryPatches(story.ID, "main", turn.ID, []StoryMemoryPatch{
		{
			Op:          "upsert",
			StructureID: "important_character",
			RecordID:    records[0].ID,
			Values: map[string]string{
				"relationship": "继续提醒主角远离钟楼",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated) != 1 || updated[0].Key != "林川" || updated[0].Values["relationship"] != "继续提醒主角远离钟楼" {
		t.Fatalf("record_id update should preserve keyed record key: %#v", updated)
	}
}
