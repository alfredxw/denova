package interactive

import (
	"fmt"
	"strings"
	"testing"
)

func TestInteractiveMemoryStoreFiltersUpdatesAndArchivesByBranch(t *testing.T) {
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
	if _, err := store.SetInteractiveMemoryArchived(story.ID, updated.ID, true); err != nil {
		t.Fatal(err)
	}
	state, err = store.InteractiveMemory(story.ID, branchState.BranchID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Entries) != 0 {
		t.Fatalf("archived memory should be excluded: %#v", state.Entries)
	}
	state, err = store.InteractiveMemory(story.ID, branchState.BranchID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Entries) != 1 || !state.Entries[0].Archived {
		t.Fatalf("archived memory should be restorable: %#v", state.Entries)
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
	if enabled := enabledStoryMemoryStructureCount(state.Structures); enabled != 7 || state.Settings.AutoIntervalTurns != defaultStoryMemoryInterval || !state.Settings.Enabled {
		t.Fatalf("default story memory state mismatch: %#v", state)
	}
	currentState := storyMemoryStructureByID(state.Structures, "current_state")
	if !strings.Contains(currentState.Description, "旧版派生摘要") {
		t.Fatalf("current_state preset description mismatch: %#v", currentState)
	}
	if !currentState.ReadOnly || !currentState.Derived || storyMemoryStructureEnabled(currentState) {
		t.Fatalf("current_state should be disabled read-only derived narrative state: %#v", currentState)
	}
	for _, want := range []string{"story_start_date", "location", "time", "current_day", "event"} {
		if !storyMemoryStructureHasField(currentState, want) {
			t.Fatalf("current_state preset missing field %q: %#v", want, currentState.Fields)
		}
	}
	protagonist := storyMemoryStructureByID(state.Structures, "protagonist")
	for _, want := range []string{"identity", "current_condition", "skills", "items", "relationships"} {
		if !storyMemoryStructureHasField(protagonist, want) {
			t.Fatalf("protagonist preset missing field %q: %#v", want, protagonist.Fields)
		}
	}
	for _, disabledID := range []string{"current_state", "rule_state_summary", "relationship_state", "romance_profile", "romance_diary", "mature_relationship_profile"} {
		if structure := storyMemoryStructureByID(state.Structures, disabledID); storyMemoryStructureEnabled(structure) {
			t.Fatalf("optional built-in structure should be disabled by default: %#v", structure)
		}
	}
	defaultNarrativeStructures := map[string][]string{
		"foreshadowing_resolved": {"title", "status", "payoff_condition", "payoff_result"},
		"long_term_arc_progress": {"arc_name", "arc_type", "current_phase", "terminal_risk"},
	}
	for structureID, fields := range defaultNarrativeStructures {
		structure := storyMemoryStructureByID(state.Structures, structureID)
		if !storyMemoryStructureEnabled(structure) {
			t.Fatalf("default narrative structure should be enabled: %#v", structure)
		}
		for _, fieldID := range fields {
			if !storyMemoryStructureHasField(structure, fieldID) {
				t.Fatalf("default narrative structure %s missing field %q: %#v", structureID, fieldID, structure.Fields)
			}
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
	if _, err := store.SetStoryMemoryRecordArchived(story.ID, updated.ID, branch.ID, true); err != nil {
		t.Fatal(err)
	}
	branchState, err = store.StoryMemory(story.ID, branch.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(branchState.Records) != 0 {
		t.Fatalf("archived story memory should be excluded by default: %#v", branchState.Records)
	}
	branchState, err = store.StoryMemory(story.ID, branch.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(branchState.Records) != 1 || !branchState.Records[0].Archived {
		t.Fatalf("archived story memory should be available when requested: %#v", branchState.Records)
	}
	context, err := store.StoryMemoryContextSummary(story.ID, branch.ID, 12*1024)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(context, "怀疑主角") {
		t.Fatalf("archived story memory should not enter model context:\n%s", context)
	}
}

func TestDerivedStoryMemoryStateTablesRejectManualEdits(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "派生记忆边界"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveStoryMemoryStructure(story.ID, StoryMemoryStructureRequest{
		ID:   "current_state",
		Name: "改写当前状态",
		Mode: "singleton",
		Fields: []StoryMemoryField{{
			ID:       "location",
			Name:     "地点",
			Required: true,
			Order:    10,
		}},
	}); err == nil {
		t.Fatal("manual structure edit for current_state should be rejected")
	}
	if _, err := store.SaveStoryMemoryRecord(story.ID, StoryMemoryRecordRequest{
		BranchID:    "main",
		StructureID: "rule_state_summary",
		Values: map[string]string{
			"resources": "hp=999",
		},
	}); err == nil {
		t.Fatal("manual record edit for rule_state_summary should be rejected")
	}
	records, err := store.ApplyStoryMemoryPatches(story.ID, "main", "", []StoryMemoryPatch{{
		Op:          "upsert",
		StructureID: "rule_state_summary",
		Values: map[string]string{
			"resources":           "生命：7/10｜本回合受伤",
			"attributes":          "无变化",
			"conditions":          "轻伤",
			"relationship_scores": "",
			"flags":               "",
			"last_rule_checks":    "",
		},
	}})
	if err != nil {
		t.Fatalf("disabled derived summary patch should be ignored without failing: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("disabled derived narrative summary patch should be ignored: %#v", records)
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
				Enabled: boolPtr(false),
				Fields: []StoryMemoryField{
					{ID: "time", Name: "时间", Enabled: boolPtr(false), Order: 10},
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
	if !storyMemoryStructureHasField(currentState, "story_start_date") || !storyMemoryStructureHasField(currentState, "current_day") || !storyMemoryStructureHasField(currentState, "event") {
		t.Fatalf("current_state built-in preset was not refreshed: %#v", currentState.Fields)
	}
	if storyMemoryStructureEnabled(currentState) {
		t.Fatalf("built-in structure enabled flag should be preserved: %#v", currentState)
	}
	if field := storyMemoryFieldByID(currentState.Fields, "time"); storyMemoryFieldEnabled(field) {
		t.Fatalf("built-in field enabled flag should be preserved: %#v", field)
	}
	plotSummary := storyMemoryStructureByID(book.Structures, "plot_summary")
	if plotSummary.Name != "剧情纪要" || !storyMemoryStructureHasField(plotSummary, "time_span") || !storyMemoryStructureHasField(plotSummary, "code_index") {
		t.Fatalf("plot_summary built-in preset was not refreshed: %#v", plotSummary)
	}
	openThreads := storyMemoryStructureByID(book.Structures, "open_threads")
	if openThreads.ID == "" || !storyMemoryStructureEnabled(openThreads) {
		t.Fatalf("new built-in open_threads structure should be added and enabled: %#v", openThreads)
	}
	longArcProgress := storyMemoryStructureByID(book.Structures, "long_term_arc_progress")
	if longArcProgress.ID == "" || !storyMemoryStructureHasField(longArcProgress, "terminal_risk") {
		t.Fatalf("new built-in long_term_arc_progress structure should be added: %#v", longArcProgress)
	}
	romanceProfile := storyMemoryStructureByID(book.Structures, "romance_profile")
	if romanceProfile.ID == "" || storyMemoryStructureEnabled(romanceProfile) {
		t.Fatalf("optional built-in romance_profile should be added disabled: %#v", romanceProfile)
	}
	custom := storyMemoryStructureByID(book.Structures, "custom")
	if custom.Name != "自定义" || !storyMemoryStructureHasField(custom, "value") {
		t.Fatalf("custom structure should be preserved: %#v", custom)
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
	context, err := store.StoryMemorySchemaContext(story.ID, 32*1024)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"structure_id",
		"## important_character",
		"## open_threads",
		"## foreshadowing_resolved",
		"## long_term_arc_progress",
		"## relationship_clock",
		"values 必须包含目标结构列出的所有字段",
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
	for _, disabled := range []string{"## current_state", "## rule_state_summary", "## relationship_state"} {
		if strings.Contains(context, disabled) {
			t.Fatalf("disabled state-like structure should stay out of schema context %q:\n%s", disabled, context)
		}
	}
}

func TestStoryMemoryUsesDirectorMemoryStructureModule(t *testing.T) {
	root := t.TempDir()
	novaDir := t.TempDir()
	memoryLibrary := NewStoryMemoryStructureLibrary(novaDir)
	module, err := memoryLibrary.Create(StoryMemoryStructureModule{
		ID:   "quest-memory",
		Name: "任务记忆结构",
		Structures: []StoryMemoryStructure{{
			ID:      "quest_tracker",
			Name:    "任务追踪",
			Mode:    "keyed",
			Enabled: boolPtr(true),
			Fields: []StoryMemoryField{
				{ID: "name", Name: "任务名", Required: true, Order: 10},
				{ID: "status", Name: "状态", Order: 20},
			},
			KeyFieldID: "name",
			Order:      10,
		}},
	})
	if err != nil {
		t.Fatalf("create memory module failed: %v", err)
	}
	directorLibrary := NewStoryDirectorLibrary(novaDir)
	director, err := directorLibrary.Create(StoryDirector{
		ID:   "quest-director",
		Name: "任务导演",
		ModuleRefs: StoryDirectorModuleRefs{
			MemoryStructureID: module.ID,
		},
		Strategy: StoryDirectorStrategy{Enabled: true},
	})
	if err != nil {
		t.Fatalf("create director failed: %v", err)
	}

	store := NewStoreWithNovaDir(root, novaDir)
	story, err := store.CreateStory(CreateStoryRequest{Title: "模块记忆", StoryDirectorID: director.ID})
	if err != nil {
		t.Fatalf("CreateStory failed: %v", err)
	}
	state, err := store.StoryMemory(story.ID, "main", false)
	if err != nil {
		t.Fatalf("StoryMemory failed: %v", err)
	}
	if state.MemoryStructureID != module.ID || state.MemoryStructureName != module.Name || state.MemoryStructureDisabled {
		t.Fatalf("story memory should expose director memory source: %#v", state)
	}
	if got := storyMemoryStructureByID(state.Structures, "quest_tracker"); got.ID == "" {
		t.Fatalf("story memory should use director module structures: %#v", state.Structures)
	}
	schema, err := store.StoryMemorySchemaContext(story.ID, 12*1024)
	if err != nil {
		t.Fatalf("StoryMemorySchemaContext failed: %v", err)
	}
	if !strings.Contains(schema, "任务追踪") || strings.Contains(schema, "current_state") {
		t.Fatalf("schema context should use memory module only:\n%s", schema)
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "接下任务", Narrative: "任务已确认。"})
	if err != nil {
		t.Fatalf("AppendTurn failed: %v", err)
	}
	records, err := store.ApplyStoryMemoryPatches(story.ID, "main", turn.ID, []StoryMemoryPatch{{
		StructureID: "quest_tracker",
		Key:         "寻找钥匙",
		Values:      map[string]string{"name": "寻找钥匙", "status": "进行中"},
	}})
	if err != nil {
		t.Fatalf("ApplyStoryMemoryPatches failed: %v", err)
	}
	if len(records) != 1 || records[0].StructureID != "quest_tracker" {
		t.Fatalf("patch should write module-backed structure record: %#v", records)
	}
}

func TestDisabledDirectorMemoryStructureSkipsRuntimeMemory(t *testing.T) {
	root := t.TempDir()
	novaDir := t.TempDir()
	directorLibrary := NewStoryDirectorLibrary(novaDir)
	director, err := directorLibrary.Create(StoryDirector{
		ID:   "memory-off",
		Name: "关闭记忆导演",
		ModuleRefs: StoryDirectorModuleRefs{
			MemoryStructureID:       DefaultStoryMemoryStructureModuleID,
			MemoryStructureDisabled: true,
		},
		Strategy: StoryDirectorStrategy{Enabled: true},
	})
	if err != nil {
		t.Fatalf("create director failed: %v", err)
	}
	store := NewStoreWithNovaDir(root, novaDir)
	story, err := store.CreateStory(CreateStoryRequest{Title: "关闭记忆", StoryDirectorID: director.ID})
	if err != nil {
		t.Fatalf("CreateStory failed: %v", err)
	}
	state, err := store.StoryMemory(story.ID, "main", false)
	if err != nil {
		t.Fatalf("StoryMemory failed: %v", err)
	}
	if !state.MemoryStructureDisabled || state.MemoryStructureID != DefaultStoryMemoryStructureModuleID {
		t.Fatalf("story memory should expose disabled module source: %#v", state)
	}
	schema, err := store.StoryMemorySchemaContext(story.ID, 12*1024)
	if err != nil {
		t.Fatalf("StoryMemorySchemaContext failed: %v", err)
	}
	if schema != "" {
		t.Fatalf("disabled memory structure should not enter schema context:\n%s", schema)
	}
	should, _, err := store.ShouldGenerateStoryMemory(story.ID, "main")
	if err != nil {
		t.Fatalf("ShouldGenerateStoryMemory failed: %v", err)
	}
	if should {
		t.Fatalf("disabled memory structure should not trigger automatic generation")
	}
	records, err := store.ApplyStoryMemoryPatches(story.ID, "main", "", []StoryMemoryPatch{{
		StructureID: "current_state",
		Values:      map[string]string{"event": "不应写入"},
	}})
	if err != nil {
		t.Fatalf("ApplyStoryMemoryPatches failed: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("disabled memory structure should ignore patches: %#v", records)
	}
}

func TestMigrateStoryMemoryStructuresToDirectorModulesPreservesRecords(t *testing.T) {
	root := t.TempDir()
	novaDir := t.TempDir()
	legacyStore := NewStore(root)
	story, err := legacyStore.CreateStory(CreateStoryRequest{Title: "旧结构故事", StoryDirectorID: DefaultStoryDirectorID})
	if err != nil {
		t.Fatalf("CreateStory failed: %v", err)
	}
	structure, err := legacyStore.SaveStoryMemoryStructure(story.ID, StoryMemoryStructureRequest{
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
		t.Fatalf("SaveStoryMemoryStructure failed: %v", err)
	}
	record, err := legacyStore.SaveStoryMemoryRecord(story.ID, StoryMemoryRecordRequest{
		BranchID:    "main",
		StructureID: structure.ID,
		Key:         "林川",
		Values:      map[string]string{"name": "林川", "status": "信任主角"},
	})
	if err != nil {
		t.Fatalf("SaveStoryMemoryRecord failed: %v", err)
	}

	store := NewStoreWithNovaDir(root, novaDir)
	if err := store.MigrateStoryMemoryStructuresToDirectorModules(); err != nil {
		t.Fatalf("MigrateStoryMemoryStructuresToDirectorModules failed: %v", err)
	}
	index, err := store.Index()
	if err != nil {
		t.Fatalf("Index failed: %v", err)
	}
	migratedStory := StorySummary{}
	for _, candidate := range index.Stories {
		if candidate.ID == story.ID {
			migratedStory = candidate
			break
		}
	}
	if migratedStory.ID == "" {
		t.Fatalf("migrated story missing from index: %#v", index.Stories)
	}
	if migratedStory.StoryDirectorID == "" || migratedStory.StoryDirectorID == DefaultStoryDirectorID {
		t.Fatalf("story using built-in director should be moved to story-specific director: %#v", migratedStory)
	}
	wantMemoryID := normalizeDirectorModuleID(fmt.Sprintf("story-%s-memory", story.ID))
	director, err := NewStoryDirectorLibrary(novaDir).Get(migratedStory.StoryDirectorID)
	if err != nil {
		t.Fatalf("get migrated director failed: %v", err)
	}
	if director.ModuleRefs.MemoryStructureID != wantMemoryID || director.ModuleRefs.MemoryStructureDisabled {
		t.Fatalf("migrated director should reference story memory module: %#v", director.ModuleRefs)
	}
	module, err := NewStoryMemoryStructureLibrary(novaDir).Get(wantMemoryID)
	if err != nil {
		t.Fatalf("get migrated memory module failed: %v", err)
	}
	if got := storyMemoryStructureByID(module.Structures, structure.ID); got.ID == "" {
		t.Fatalf("migrated memory module should contain custom structure: %#v", module.Structures)
	}
	state, err := store.StoryMemory(story.ID, "main", false)
	if err != nil {
		t.Fatalf("StoryMemory failed: %v", err)
	}
	if state.MemoryStructureID != wantMemoryID {
		t.Fatalf("story memory should use migrated module, got %#v", state)
	}
	if len(state.Records) != 1 || state.Records[0].ID != record.ID || state.Records[0].Values["status"] != "信任主角" {
		t.Fatalf("migration should preserve existing records without rewriting: %#v", state.Records)
	}
}

func TestDisabledStoryMemoryStructuresAndFieldsStayOutOfAgentContext(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "关闭结构"})
	if err != nil {
		t.Fatal(err)
	}
	disabled := false
	enabled := true
	if _, err := store.SaveStoryMemoryStructure(story.ID, StoryMemoryStructureRequest{
		ID:         "private_notes",
		Name:       "关闭表",
		Mode:       "keyed",
		KeyFieldID: "name",
		Enabled:    &disabled,
		Fields: []StoryMemoryField{
			{ID: "name", Name: "名称", Enabled: &enabled, Required: true, Order: 10},
			{ID: "secret", Name: "秘密", Enabled: &enabled, Order: 20},
		},
		Order: 90,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveStoryMemoryStructure(story.ID, StoryMemoryStructureRequest{
		ID:      "field_filter",
		Name:    "字段过滤",
		Mode:    "append",
		Enabled: &enabled,
		Fields: []StoryMemoryField{
			{ID: "visible", Name: "可见", Enabled: &enabled, Order: 10},
			{ID: "hidden", Name: "隐藏", Enabled: &disabled, Order: 20},
		},
		Order: 91,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveStoryMemoryRecord(story.ID, StoryMemoryRecordRequest{
		BranchID:    "main",
		StructureID: "private_notes",
		Key:         "密钥",
		Values:      map[string]string{"name": "密钥", "secret": "不可注入"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveStoryMemoryRecord(story.ID, StoryMemoryRecordRequest{
		BranchID:    "main",
		StructureID: "field_filter",
		Values:      map[string]string{"visible": "可以注入", "hidden": "不可注入字段"},
	}); err != nil {
		t.Fatal(err)
	}
	schemaContext, err := store.StoryMemorySchemaContext(story.ID, 12*1024)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(schemaContext, "private_notes") || strings.Contains(schemaContext, "hidden（隐藏）") {
		t.Fatalf("disabled structure or field should not enter schema context:\n%s", schemaContext)
	}
	memoryContext, err := store.StoryMemoryContextSummary(story.ID, "main", 12*1024)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(memoryContext, "不可注入") || strings.Contains(memoryContext, "不可注入字段") || !strings.Contains(memoryContext, "可以注入") {
		t.Fatalf("disabled structure or field should be filtered from memory context:\n%s", memoryContext)
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "继续", Narrative: "继续剧情。"})
	if err != nil {
		t.Fatal(err)
	}
	records, err := store.ApplyStoryMemoryPatches(story.ID, "main", turn.ID, []StoryMemoryPatch{{
		Op:          "upsert",
		StructureID: "private_notes",
		Key:         "自动密钥",
		Values:      map[string]string{"name": "自动密钥", "secret": "自动写入"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("disabled structure patch should be ignored: %#v", records)
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

func storyMemoryFieldByID(fields []StoryMemoryField, fieldID string) StoryMemoryField {
	for _, field := range fields {
		if field.ID == fieldID {
			return field
		}
	}
	return StoryMemoryField{}
}

func enabledStoryMemoryStructureCount(structures []StoryMemoryStructure) int {
	count := 0
	for _, structure := range structures {
		if storyMemoryStructureEnabled(structure) {
			count++
		}
	}
	return count
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
				"name":                        "林川",
				"relationship_to_protagonist": "提醒主角远离钟楼",
			},
		},
		{
			Op:          "upsert",
			StructureID: "open_threads",
			Values: map[string]string{
				"progress": "有人提醒钟楼危险，但事项标题未知。",
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
				"relationship_to_protagonist": "继续提醒主角远离钟楼",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated) != 1 || updated[0].Key != "林川" || updated[0].Values["relationship_to_protagonist"] != "继续提醒主角远离钟楼" {
		t.Fatalf("record_id update should preserve keyed record key: %#v", updated)
	}
}

func TestStoryMemoryCompactionContextKeepsAllVisibleTableRecords(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "完整表格记忆"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{
		BranchID:  "main",
		User:      "继续整理线索",
		Narrative: "所有线索被摊开放在桌上。",
	})
	if err != nil {
		t.Fatal(err)
	}
	patches := make([]StoryMemoryPatch, 0, maxMemoryListItems+6)
	for i := 1; i <= maxMemoryListItems+6; i++ {
		name := fmt.Sprintf("角色%02d", i)
		patches = append(patches, StoryMemoryPatch{
			Op:          "upsert",
			StructureID: "important_character",
			Key:         name,
			Values: map[string]string{
				"name":                        name,
				"relationship_to_protagonist": strings.Repeat("重要关系", 80),
			},
		})
	}
	if _, err := store.ApplyStoryMemoryPatches(story.ID, "main", turn.ID, patches); err != nil {
		t.Fatal(err)
	}

	bounded, err := store.StoryMemoryContextSummary(story.ID, "main", 256)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(bounded, "后续故事记忆已截断") {
		t.Fatalf("bounded story memory context should still truncate large tables")
	}

	full, err := store.StoryMemoryCompactionContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(full, "后续故事记忆已截断") {
		t.Fatalf("compaction story memory context should not truncate:\n%s", full)
	}
	if !strings.Contains(full, "角色01") || !strings.Contains(full, "角色30") {
		t.Fatalf("full compaction context should include early and late records:\n%s", full)
	}
}
