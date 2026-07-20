package interactive

import "testing"

func TestActorStatePatchValidationAndReplay(t *testing.T) {
	maxHP := 12.0
	system := normalizeActorStateSystem(StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{{
			ID:   "protagonist",
			Name: "主角",
			Fields: []ActorStateField{{
				ID:         "hp",
				Path:       "resources.hp",
				Name:       "生命",
				Type:       "number",
				Default:    float64(10),
				Min:        floatPtr(0),
				Max:        &maxHP,
				Visibility: "visible",
			}, {
				ID:         "condition",
				Path:       "conditions.main",
				Name:       "状态",
				Type:       "enum",
				Options:    []string{"normal", "wounded"},
				Default:    "normal",
				Visibility: "spoiler",
			}},
		}},
		InitialActors: []ActorStateInitialActor{{
			ID:         DefaultActorID,
			Name:       "主角",
			TemplateID: "protagonist",
			Role:       "protagonist",
		}},
	})
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{
		Title:         "Actor 状态",
		StoryTellerID: "classic",
		ActorState:    &system,
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "冒险", Narrative: "主角受伤但仍能行动。"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := ValidateActorStatePatches(system, []ActorStatePatch{{
		ActorID:    DefaultActorID,
		ActorName:  "主角",
		TemplateID: "protagonist",
		State: map[string]any{
			"生命": float64(7),
			"状态": "wounded",
		},
		Reason: "本回合主角受伤。",
	}}, turn.ID)
	if err != nil {
		t.Fatalf("valid actor patch should pass: %v", err)
	}
	if len(result.Ops) == 0 || result.Ops[0].SourceTurnID != turn.ID {
		t.Fatalf("actor patch should produce traced state ops: %#v", result.Ops)
	}
	if _, err := store.AppendStateDelta(story.ID, AppendStateDeltaRequest{ParentID: turn.ID, BranchID: "main", Ops: result.Ops, ActorOps: result.ActorOps}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if got := numberFromAny(actorStateFieldValue(snapshot.State, "protagonist", "生命")); got != 7 {
		t.Fatalf("actor hp should replay from the frozen field ID, got %v state=%#v", got, snapshot.State)
	}
	if got := actorStateFieldValue(snapshot.State, "protagonist", "状态"); got != "wounded" {
		t.Fatalf("enum state should replay, got %#v", got)
	}
}

func TestDefaultActorStateIncludesStoryContextStateObject(t *testing.T) {
	system := defaultActorStateSystem()
	if template := actorStateTemplateByID(system, ActorStateStoryContextTemplateID); template.ID == "" {
		t.Fatalf("default actor state should include story context template: %#v", system.Templates)
	}
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{
		Title:         "故事上下文状态",
		StoryTellerID: "classic",
		ActorState:    &system,
	})
	if err != nil {
		t.Fatal(err)
	}
	initialSnapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if got := getPath(initialSnapshot.State, "actors.story.template_id"); got != ActorStateStoryContextTemplateID {
		t.Fatalf("story context object should be initialized, got %#v state=%#v", got, initialSnapshot.State)
	}

	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "查看四周", Narrative: "主角停在黄泉酒馆门口。"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := ValidateActorStatePatches(system, []ActorStatePatch{{
		ActorID:    DefaultStoryContextActorID,
		ActorName:  "故事上下文",
		TemplateID: ActorStateStoryContextTemplateID,
		Role:       "story_context",
		State: map[string]any{
			"当前详细地点": "黄泉酒馆门口",
			"当前时间":   "七月初九子时",
			"当前事件":   "主角抵达黄泉酒馆，门内有人等待主角表态",
		},
		Reason: "本回合确认当前地点、时间和事件压力。",
	}}, turn.ID)
	if err != nil {
		t.Fatalf("default story context patch should pass: %v", err)
	}
	if _, err := store.AppendStateDelta(story.ID, AppendStateDeltaRequest{ParentID: turn.ID, BranchID: "main", Ops: result.Ops, ActorOps: result.ActorOps}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if got := actorStateFieldValue(snapshot.State, "story", "当前详细地点"); got != "黄泉酒馆门口" {
		t.Fatalf("story context location should replay, got %#v", got)
	}
	if got := actorStateFieldValue(snapshot.State, "story", "当前事件"); got != "主角抵达黄泉酒馆，门内有人等待主角表态" {
		t.Fatalf("story context event should replay, got %#v", got)
	}
}

func TestDefaultActorStateCentralizedRecordsUseNestedUpdates(t *testing.T) {
	system := defaultActorStateSystem()
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{
		Title:         "集中状态记录",
		StoryTellerID: "classic",
		ActorState:    &system,
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if got := getPath(snapshot.State, "actors.world.template_id"); got != ActorStateWorldEntitiesTemplateID {
		t.Fatalf("world entity actor should be initialized, got %#v state=%#v", got, snapshot.State)
	}

	updates := []StateUpdate{
		{Op: TurnStateUpdateReplace, Path: formatStateUpdatePath([]string{DefaultActorID, "技能与能力", "洞察"}), Value: map[string]any{"名称": "洞察", "类型": "探索", "当前状态": "可用"}},
		{Op: TurnStateUpdateReplace, Path: formatStateUpdatePath([]string{DefaultActorID, "重要物品", "旧钥匙"}), Value: map[string]any{"名称": "旧钥匙", "类型": "线索", "数量": float64(1)}},
		{Op: TurnStateUpdateReplace, Path: formatStateUpdatePath([]string{DefaultActorID, "关系", "引路人"}), Value: map[string]any{"关系类型": "同伴", "好感度": float64(60), "当前态度": "愿意合作"}},
		{Op: TurnStateUpdateReplace, Path: formatStateUpdatePath([]string{DefaultStoryContextActorID, "当前任务", "进入遗迹"}), Value: map[string]any{"任务名称": "进入遗迹", "当前状态": "进行中"}},
		{Op: TurnStateUpdateReplace, Path: formatStateUpdatePath([]string{DefaultWorldEntitiesActorID, "地点记录", "沉没遗迹"}), Value: map[string]any{"地点名称": "沉没遗迹", "探索状态": "已发现"}},
		{Op: TurnStateUpdateReplace, Path: formatStateUpdatePath([]string{DefaultWorldEntitiesActorID, "势力记录", "灰塔协会"}), Value: map[string]any{"势力名称": "灰塔协会", "对主角立场": "观望"}},
	}
	compiled, err := CompileTurnStateUpdates(system, snapshot.State, updates, TurnStateUpdateCompileOptions{SourceTurnID: "turn-centralized"})
	if err != nil {
		t.Fatalf("centralized object records should use the existing nested update protocol: %v", err)
	}
	if len(compiled.Updates) != len(updates) || len(compiled.ActorOps) != len(updates) {
		t.Fatalf("unexpected centralized update compilation: %#v", compiled)
	}
	working := cloneActorStateRoot(snapshot.State)
	for _, op := range compiled.ActorOps {
		applyActorStateOp(working, op)
	}
	assertActorObjectRecord := func(actorID, fieldID, recordID, key string, want any) {
		t.Helper()
		field, ok := actorStateFieldValue(working, actorID, fieldID).(map[string]any)
		if !ok {
			t.Fatalf("field %s/%s should be an object, got %#v", actorID, fieldID, actorStateFieldValue(working, actorID, fieldID))
		}
		record, ok := field[recordID].(map[string]any)
		if !ok || record[key] != want {
			t.Fatalf("record %s/%s/%s mismatch: %#v", actorID, fieldID, recordID, field)
		}
	}
	assertActorObjectRecord(DefaultActorID, "技能与能力", "洞察", "名称", "洞察")
	assertActorObjectRecord(DefaultActorID, "重要物品", "旧钥匙", "名称", "旧钥匙")
	assertActorObjectRecord(DefaultActorID, "关系", "引路人", "关系类型", "同伴")
	assertActorObjectRecord(DefaultStoryContextActorID, "当前任务", "进入遗迹", "任务名称", "进入遗迹")
	assertActorObjectRecord(DefaultWorldEntitiesActorID, "地点记录", "沉没遗迹", "地点名称", "沉没遗迹")
	assertActorObjectRecord(DefaultWorldEntitiesActorID, "势力记录", "灰塔协会", "势力名称", "灰塔协会")
}

func TestActorStateSupportsCustomNonCharacterStateObjects(t *testing.T) {
	system := normalizeActorStateSystem(StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{{
			ID:          "world_state",
			Name:        "世界状态表",
			Description: "用于承接世界级危机、倒计时和全局规则状态。",
			Fields: []ActorStateField{{
				ID:         "countdown",
				Path:       "crisis.countdown",
				Name:       "毁灭倒计时",
				Type:       "string",
				Default:    "100天后世界毁灭",
				Visibility: "visible",
			}, {
				ID:         "pressure",
				Path:       "crisis.pressure",
				Name:       "危机压力",
				Type:       "string",
				Visibility: "spoiler",
			}},
		}, {
			ID:          "heroine_route",
			Name:        "特定女主攻略状态表",
			Description: "用于承接单女主线的当前阶段、旗标和误解。",
			Fields: []ActorStateField{{
				ID:         "stage",
				Path:       "route.current_stage",
				Name:       "当前攻略阶段",
				Type:       "string",
				Visibility: "visible",
			}, {
				ID:         "flags",
				Path:       "route.flags",
				Name:       "关键旗标",
				Type:       "list",
				Visibility: "spoiler",
			}},
		}},
		InitialActors: []ActorStateInitialActor{{
			ID:          "world",
			Name:        "世界状态",
			TemplateID:  "world_state",
			Role:        "world",
			Description: "故事全局倒计时状态对象。",
			State: map[string]any{
				"crisis.pressure": "天象异常开始影响边境城镇。",
			},
		}},
	})

	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{
		Title:         "自定义状态对象",
		StoryTellerID: "classic",
		ActorState:    &system,
	})
	if err != nil {
		t.Fatal(err)
	}
	initialSnapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if got := getPath(initialSnapshot.State, "actors.world.template_id"); got != "world_state" {
		t.Fatalf("world state object should use custom template, got %#v state=%#v", got, initialSnapshot.State)
	}
	if got := actorStateFieldValue(initialSnapshot.State, "world", "毁灭倒计时"); got != "100天后世界毁灭" {
		t.Fatalf("world countdown should come from template default, got %#v", got)
	}

	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "进入旧钟楼", Narrative: "钟声提前响起，兰若愿意暂时合作。"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := ValidateActorStatePatches(system, []ActorStatePatch{{
		ActorID:    "world",
		ActorName:  "世界状态",
		TemplateID: "world_state",
		Role:       "world",
		State: map[string]any{
			"毁灭倒计时": "99天后世界毁灭",
		},
		Reason: "钟楼事件确认世界倒计时推进。",
	}, {
		ActorID:    "heroine_lan",
		ActorName:  "兰若攻略线",
		TemplateID: "heroine_route",
		Role:       "specific_character_route",
		State: map[string]any{
			"当前攻略阶段": "从戒备转为愿意合作",
			"关键旗标":   []any{"知道主角救过她", "仍隐瞒家族契约"},
		},
		Reason: "本回合兰若明确改变对主角的合作态度。",
	}}, turn.ID)
	if err != nil {
		t.Fatalf("custom non-character state patches should pass: %v", err)
	}
	if _, err := store.AppendStateDelta(story.ID, AppendStateDeltaRequest{ParentID: turn.ID, BranchID: "main", Ops: result.Ops, ActorOps: result.ActorOps}); err != nil {
		t.Fatal(err)
	}

	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if got := actorStateFieldValue(snapshot.State, "world", "毁灭倒计时"); got != "99天后世界毁灭" {
		t.Fatalf("world countdown should replay through actor-state path, got %#v", got)
	}
	if got := getPath(snapshot.State, "actors.heroine_lan.template_id"); got != "heroine_route" {
		t.Fatalf("heroine route should use custom template, got %#v", got)
	}
	if got := actorStateFieldValue(snapshot.State, "heroine_lan", "当前攻略阶段"); got != "从戒备转为愿意合作" {
		t.Fatalf("heroine route stage should replay, got %#v", got)
	}
}

func TestActorStatePatchRejectsInvalidFieldAndType(t *testing.T) {
	system := normalizeActorStateSystem(StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{{
			ID:   "antagonist",
			Name: "反派",
			Fields: []ActorStateField{{
				ID:   "threat",
				Path: "attributes.threat",
				Name: "威胁",
				Type: "number",
			}},
		}},
	})
	if _, err := ValidateActorStatePatches(system, []ActorStatePatch{{
		ActorID:    "villain",
		TemplateID: "missing",
		State:      map[string]any{"attributes.threat": float64(3)},
	}}, "turn-1"); err == nil {
		t.Fatal("missing template should be rejected")
	}
	if _, err := ValidateActorStatePatches(system, []ActorStatePatch{{
		ActorID:    "villain",
		TemplateID: "antagonist",
		State:      map[string]any{"attributes.unknown": float64(3)},
	}}, "turn-1"); err == nil {
		t.Fatal("unknown field should be rejected")
	}
	if _, err := ValidateActorStatePatches(system, []ActorStatePatch{{
		ActorID:    "villain",
		TemplateID: "antagonist",
		State:      map[string]any{"attributes.threat": "high"},
	}}, "turn-1"); err == nil {
		t.Fatal("type mismatch should be rejected")
	}
}

func floatPtr(value float64) *float64 {
	return &value
}
