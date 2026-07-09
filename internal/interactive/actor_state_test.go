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
		Title:           "Actor 状态",
		StoryTellerID:   "classic",
		InitialStateOps: actorStateInitialOps(system),
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
			"resources.hp":    float64(7),
			"conditions.main": "wounded",
		},
		Reason: "本回合主角受伤。",
	}}, turn.ID)
	if err != nil {
		t.Fatalf("valid actor patch should pass: %v", err)
	}
	if len(result.Ops) == 0 || result.Ops[0].SourceTurnID != turn.ID {
		t.Fatalf("actor patch should produce traced state ops: %#v", result.Ops)
	}
	if _, err := store.AppendStateDelta(story.ID, AppendStateDeltaRequest{ParentID: turn.ID, BranchID: "main", Ops: result.Ops}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if got := numberFromAny(getPath(snapshot.State, "actors.protagonist.state.resources.hp")); got != 7 {
		t.Fatalf("actor hp should replay from actor path, got %v state=%#v", got, snapshot.State)
	}
	if got := numberFromAny(getPath(snapshot.State, "resources.hp")); got != 7 {
		t.Fatalf("legacy hp path should read actor state, got %v state=%#v", got, snapshot.State)
	}
	if got := getPath(snapshot.State, "actors.protagonist.state.conditions.main"); got != "wounded" {
		t.Fatalf("enum state should replay, got %#v", got)
	}
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
		Title:           "自定义状态对象",
		StoryTellerID:   "classic",
		InitialStateOps: actorStateInitialOps(system),
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
	if got := getPath(initialSnapshot.State, "actors.world.state.crisis.countdown"); got != "100天后世界毁灭" {
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
			"crisis.countdown": "99天后世界毁灭",
		},
		Reason: "钟楼事件确认世界倒计时推进。",
	}, {
		ActorID:    "heroine_lan",
		ActorName:  "兰若攻略线",
		TemplateID: "heroine_route",
		Role:       "specific_character_route",
		State: map[string]any{
			"route.current_stage": "从戒备转为愿意合作",
			"route.flags":         []any{"知道主角救过她", "仍隐瞒家族契约"},
		},
		Reason: "本回合兰若明确改变对主角的合作态度。",
	}}, turn.ID)
	if err != nil {
		t.Fatalf("custom non-character state patches should pass: %v", err)
	}
	if _, err := store.AppendStateDelta(story.ID, AppendStateDeltaRequest{ParentID: turn.ID, BranchID: "main", Ops: result.Ops}); err != nil {
		t.Fatal(err)
	}

	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if got := getPath(snapshot.State, "actors.world.state.crisis.countdown"); got != "99天后世界毁灭" {
		t.Fatalf("world countdown should replay through actor-state path, got %#v", got)
	}
	if got := getPath(snapshot.State, "actors.heroine_lan.template_id"); got != "heroine_route" {
		t.Fatalf("heroine route should use custom template, got %#v", got)
	}
	if got := getPath(snapshot.State, "actors.heroine_lan.state.route.current_stage"); got != "从戒备转为愿意合作" {
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
