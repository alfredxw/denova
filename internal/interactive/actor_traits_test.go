package interactive

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildNewActorStateOpsEmitsOneSetPerResolvedField(t *testing.T) {
	template := ActorStateTemplate{
		ID: "protagonist",
		Fields: []ActorStateField{
			{Name: "生命", Type: "number", Default: 100},
			{Name: "备注", Type: "string"},
		},
	}
	_, actorOps, normalized, err := buildNewActorStateOps(template, "protagonist", "主角", "protagonist", "", map[string]any{
		"生命": 80,
		"备注": nil,
	}, "初始化", "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(actorOps) != 1 {
		t.Fatalf("defaults and explicit values must resolve before emitting one op per field: %#v", actorOps)
	}
	if op := actorOps[0]; op.FieldID != "生命" || op.Value != float64(80) {
		t.Fatalf("explicit value must replace the default in the single emitted op: %#v", op)
	}
	if normalized["生命"] != float64(80) {
		t.Fatalf("normalized state must keep the explicit override: %#v", normalized)
	}
	if _, exists := normalized["备注"]; exists {
		t.Fatalf("null input must remain omitted instead of becoming a set(null): %#v", normalized)
	}
}

func TestRollActorTraitsUsesFixedSelectionsWithoutDuplicates(t *testing.T) {
	system := actorTraitTestSystem()
	result, err := RollActorTraits(system, ActorTraitRollRequest{
		ActorID:    "hero",
		TemplateID: "protagonist",
		Seed:       42,
		Selections: []ActorTraitSelection{{PoolID: "nature", TraitIDs: []string{"patient"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Seed != 42 || len(result.Traits) != 2 || result.Traits[0].TraitID != "patient" {
		t.Fatalf("unexpected fixed roll: %#v", result)
	}
	if result.Traits[0].TraitID == result.Traits[1].TraitID {
		t.Fatalf("weighted draw must not duplicate traits: %#v", result.Traits)
	}
	replayed, err := RollActorTraits(system, ActorTraitRollRequest{ActorID: "hero", TemplateID: "protagonist", Seed: 42, Selections: []ActorTraitSelection{{PoolID: "nature", TraitIDs: []string{"patient"}}}})
	if err != nil || replayed.Traits[1].TraitID != result.Traits[1].TraitID {
		t.Fatalf("same seed and selection should replay identically: first=%#v replay=%#v err=%v", result, replayed, err)
	}
	if _, err := RollActorTraits(system, ActorTraitRollRequest{ActorID: "hero", TemplateID: "protagonist", Selections: []ActorTraitSelection{{PoolID: "nature", TraitIDs: []string{"missing"}}}}); err == nil {
		t.Fatal("unknown fixed trait should be rejected")
	}
	if _, err := RollActorTraits(system, ActorTraitRollRequest{ActorID: "hero", TemplateID: "protagonist", Selections: []ActorTraitSelection{{PoolID: "forbidden", TraitIDs: []string{"x"}}}}); err == nil {
		t.Fatal("pool not bound to template should be rejected")
	}
}

func TestActorCreationUsesOneFlowAndPreservesTraitSnapshots(t *testing.T) {
	system := actorTraitTestSystem()
	initialOps, initialActorOps, err := BuildActorStateInitialChanges(system, []InitialActorTraitRoll{{
		ActorID: "protagonist", Seed: 9, Selections: []ActorTraitSelection{{PoolID: "nature", TraitIDs: []string{"patient", "bold"}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	state := applyActorTraitTestOps(nil, initialOps, initialActorOps)
	initialTraits := actorTraitInstancesFromState(state, "protagonist")
	if len(initialTraits) != 2 || initialTraits[0].SourceKind != "initial_trait_roll" || initialTraits[0].SourceTurnID != "story_create" {
		t.Fatalf("initial Actor should persist trait snapshots: %#v", initialTraits)
	}

	for _, actor := range []ActorStatePatch{
		{ActorID: "mentor", ActorName: "导师", TemplateID: "important_character", Role: "supporting"},
		{ActorID: "wolf", ActorName: "狼王", TemplateID: "opponent", Role: "monster"},
	} {
		created, err := ValidateActorStatePatchesAgainstState(system, state, []ActorStatePatch{actor}, "turn-create")
		if err != nil {
			t.Fatalf("create %s failed: %v", actor.ActorID, err)
		}
		if len(created.CreatedActors) != 1 || len(created.AssignedTraits[actor.ActorID]) != 1 {
			t.Fatalf("runtime Actor should use shared creation flow: %#v", created)
		}
		state = applyActorTraitTestOps(state, created.Ops)
	}

	update, err := ValidateActorStatePatchesAgainstState(system, state, []ActorStatePatch{{
		ActorID: "wolf", State: map[string]any{"status": "wounded"},
	}}, "turn-update")
	if err != nil {
		t.Fatal(err)
	}
	if containsStateOpPath(update.Ops, "actors.wolf.traits") {
		t.Fatalf("existing Actor updates must not redraw traits: %#v", update.Ops)
	}
	if _, err := ValidateActorStatePatchesAgainstState(system, state, []ActorStatePatch{{ActorID: "wolf", TemplateID: "important_character", State: map[string]any{"status": "calm"}}}, "turn-update"); err == nil {
		t.Fatal("ordinary patch must not replace an existing Actor template")
	}

	// The persisted definition snapshot remains stable after library edits.
	system.TraitPools[0].Traits[0].Name = "配置中已改名"
	if actorTraitInstancesFromState(state, "protagonist")[0].Name != initialTraits[0].Name {
		t.Fatalf("library edits must not rewrite existing story traits: %#v", state)
	}
}

func TestLegacyActorWithoutTemplateCanBeBoundExplicitly(t *testing.T) {
	system := actorTraitTestSystem()
	state := map[string]any{
		"actors": map[string]any{
			"legacy": map[string]any{
				"name":  "旧角色",
				"state": map[string]any{"status": "unknown"},
			},
		},
	}
	result, err := ValidateActorStatePatchesAgainstState(system, state, []ActorStatePatch{{
		ActorID:    "legacy",
		TemplateID: "important_character",
		State:      map[string]any{"status": "ready"},
	}}, "turn-migrate")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.CreatedActors) != 0 {
		t.Fatalf("binding a legacy Actor must not recreate it: %#v", result)
	}
	next := applyActorTraitTestOps(state, result.Ops, result.ActorOps)
	if got := getPath(next, "actors.legacy.template_id"); got != "important_character" {
		t.Fatalf("legacy Actor template binding was not persisted: %#v", next)
	}
	if got := actorStateFieldValue(next, "legacy", "状态"); got != "ready" {
		t.Fatalf("legacy Actor state update was not applied: %#v", next)
	}
}

func TestActorTraitLifecycleDrawRerollSetRemove(t *testing.T) {
	system := actorTraitTestSystem()
	created, err := ValidateActorStatePatchesAgainstState(system, nil, []ActorStatePatch{{ActorID: "hero", TemplateID: "protagonist"}}, "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	state := applyActorTraitTestOps(nil, created.Ops)
	current := actorTraitInstancesFromState(state, "hero")
	if len(current) != 2 {
		t.Fatalf("expected two traits on creation: %#v", current)
	}

	removed, err := ValidateActorStatePatchesAgainstState(system, state, []ActorStatePatch{{ActorID: "hero", TraitChanges: []ActorTraitChange{{Op: "remove", PoolID: "nature", TraitIDs: []string{current[0].TraitID}}}}}, "turn-2")
	if err != nil {
		t.Fatal(err)
	}
	state = applyActorTraitTestOps(state, removed.Ops)
	if len(actorTraitInstancesFromState(state, "hero")) != 1 {
		t.Fatalf("remove should drop one trait: %#v", state)
	}

	drawn, err := ValidateActorStatePatchesAgainstState(system, state, []ActorStatePatch{{ActorID: "hero", TraitChanges: []ActorTraitChange{{Op: "draw", PoolID: "nature", Seed: 17}}}}, "turn-3")
	if err != nil {
		t.Fatal(err)
	}
	state = applyActorTraitTestOps(state, drawn.Ops)
	if len(actorTraitInstancesFromState(state, "hero")) != 2 {
		t.Fatalf("draw should fill the template count: %#v", state)
	}

	setResult, err := ValidateActorStatePatchesAgainstState(system, state, []ActorStatePatch{{ActorID: "hero", TraitChanges: []ActorTraitChange{{Op: "set", PoolID: "nature", TraitIDs: []string{"patient", "secret"}, Seed: 18}}}}, "turn-4")
	if err != nil {
		t.Fatal(err)
	}
	state = applyActorTraitTestOps(state, setResult.Ops)
	setTraits := actorTraitInstancesFromState(state, "hero")
	if len(setTraits) != 2 || setTraits[0].TraitID != "patient" || setTraits[1].TraitID != "secret" {
		t.Fatalf("set should install exactly the requested snapshots: %#v", setTraits)
	}

	reroll, err := ValidateActorStatePatchesAgainstState(system, state, []ActorStatePatch{{ActorID: "hero", TraitChanges: []ActorTraitChange{{Op: "reroll", PoolID: "nature", Seed: 19}}}}, "turn-5")
	if err != nil {
		t.Fatal(err)
	}
	state = applyActorTraitTestOps(state, reroll.Ops)
	for _, trait := range actorTraitInstancesFromState(state, "hero") {
		if trait.SourceKind != "actor_trait_change" || trait.SourceTurnID != "turn-5" {
			t.Fatalf("reroll should record lifecycle provenance: %#v", trait)
		}
	}
}

func TestActorStateRuntimeContextFiltersVisibilityAndLibrary(t *testing.T) {
	system := actorTraitTestSystem()
	system.Templates[0].Fields[0].Description = "主角当前能够被剧情直接观察到的状态。"
	system.Templates[0].Fields[0].UpdateInstruction = "仅在正文已经明确改变该状态时更新。"
	ops, actorOps, err := BuildActorStateInitialChanges(system, []InitialActorTraitRoll{{
		ActorID: "protagonist", Seed: 1, Selections: []ActorTraitSelection{{PoolID: "nature", TraitIDs: []string{"patient", "secret"}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	state := applyActorTraitTestOps(nil, ops, actorOps)
	context := ActorStateRuntimeContext(system, state, 64*1024, 3)
	if !strings.Contains(context, "patient") || !strings.Contains(context, "耐心") {
		t.Fatalf("visible assigned trait should enter runtime context: %s", context)
	}
	if strings.Contains(context, "secret") || strings.Contains(context, "未被抽取的配置词条") {
		t.Fatalf("hidden traits and the reusable library must stay out of runtime context: %s", context)
	}
	for _, expected := range []string{
		"# Actor 状态手册",
		"来源：`effective_actor_state_schema` + `Snapshot.State.actors`",
		"Actor ID：`protagonist`",
		"字段 ID：`状态`",
		"当前值：`ready`",
		"字段说明：主角当前能够被剧情直接观察到的状态。",
		"更新指引：仅在正文已经明确改变该状态时更新。",
		"## 提交参数模板",
		`"state_changes"`,
		`"actor_id": "protagonist"`,
		`"field_id": "状态"`,
		"## 新 Actor 可用模板",
		"Template ID：`important_character`",
	} {
		if !strings.Contains(context, expected) {
			t.Fatalf("runtime context should render a readable schema-derived Markdown guide; missing %q in:\n%s", expected, context)
		}
	}
	if len(context) > 64*1024 || json.Valid([]byte(context)) {
		t.Fatalf("runtime context must be bounded Markdown rather than raw JSON: bytes=%d context=%s", len(context), context)
	}
	if strings.Count(context, "{{next_action_") != 3 {
		t.Fatalf("submission example must match the story's configured choice count: %s", context)
	}
	if strings.Count(context, "主角当前能够被剧情直接观察到的状态。") != 1 || strings.Count(context, "仅在正文已经明确改变该状态时更新。") != 1 {
		t.Fatalf("field semantics should appear once in template definitions instead of being duplicated for every current Actor: %s", context)
	}
	bounded := ActorStateRuntimeContext(system, state, 512)
	if bounded == "" || len(bounded) > 512 || json.Valid([]byte(bounded)) || !strings.Contains(bounded, "内容已按上下文上限截断") {
		t.Fatalf("small runtime context must remain bounded Markdown with an explicit truncation marker: bytes=%d context=%s", len(bounded), bounded)
	}
}

func TestBuiltinActorStateRuntimeContextsKeepCentralizedSchemaWithinLimit(t *testing.T) {
	for _, module := range builtinActorStateModules() {
		context := ActorStateRuntimeContext(module.ActorState, nil, DirectorContextMaxBytes)
		if strings.Contains(context, actorStateRuntimeTruncatedNotice) {
			t.Fatalf("built-in actor state %s should fit its complete centralized schema in the runtime context: bytes=%d", module.ID, len(context))
		}
		for _, expected := range []string{
			"Actor ID：`protagonist`",
			"Actor ID：`story`",
			"Actor ID：`world`",
			"Template ID：`important_character`",
			"Template ID：`opponent`",
			"Template ID：`world_entities`",
			"字段 ID：`技能与能力`",
			"字段 ID：`当前任务`",
			"字段 ID：`地点记录`",
		} {
			if !strings.Contains(context, expected) {
				t.Fatalf("built-in actor state %s runtime context missing %q: bytes=%d", module.ID, expected, len(context))
			}
		}
	}
}

func TestActorTraitSnapshotsReplayIdenticallyAcrossBranches(t *testing.T) {
	system := actorTraitTestSystem()
	root := t.TempDir()
	store := NewStore(root)
	story, err := store.CreateStory(CreateStoryRequest{
		Title: "词条重放", Origin: "开始", ActorState: &system,
		InitialTraitRolls: []InitialActorTraitRoll{{
			ActorID: "protagonist",
			Seed:    31,
			Selections: []ActorTraitSelection{{
				PoolID: "nature", TraitIDs: []string{"patient", "bold"},
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "继续", Narrative: "故事继续。"})
	if err != nil {
		t.Fatal(err)
	}
	branch, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: turn.ID, Title: "词条支线"})
	if err != nil {
		t.Fatal(err)
	}
	mainSnapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	branchSnapshot, err := store.Snapshot(story.ID, branch.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := actorTraitInstancesFromState(mainSnapshot.State, "protagonist")
	got := actorTraitInstancesFromState(branchSnapshot.State, "protagonist")
	if len(want) != 2 || len(got) != 2 || want[0] != got[0] || want[1] != got[1] {
		t.Fatalf("branch replay should preserve exact trait snapshots: main=%#v branch=%#v", want, got)
	}

	// Reloading from disk replays StateOps rather than consulting the edited library.
	system.TraitPools[0].Traits[0].Name = "后来改名"
	reloaded, err := NewStore(root).Snapshot(story.ID, branch.ID)
	if err != nil {
		t.Fatal(err)
	}
	replayed := actorTraitInstancesFromState(reloaded.State, "protagonist")
	if len(replayed) != 2 || replayed[0] != want[0] || replayed[1] != want[1] {
		t.Fatalf("disk replay must remain independent of current library definitions: want=%#v got=%#v", want, replayed)
	}
}

func actorTraitTestSystem() StoryDirectorActorStateSystem {
	return StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{
			{ID: "protagonist", Name: "主角", Fields: []ActorStateField{{ID: "status", Path: "status", Name: "状态", Type: "string", Default: "ready", Visibility: "visible"}}, TraitRules: []ActorTraitRule{{PoolID: "nature", DrawCount: 2}}},
			{ID: "important_character", Name: "重要角色", Fields: []ActorStateField{{ID: "status", Path: "status", Name: "状态", Type: "string", Visibility: "visible"}}, TraitRules: []ActorTraitRule{{PoolID: "nature", DrawCount: 1}}},
			{ID: "opponent", Name: "敌人/怪物", Fields: []ActorStateField{{ID: "status", Path: "status", Name: "状态", Type: "string", Visibility: "visible"}}, TraitRules: []ActorTraitRule{{PoolID: "nature", DrawCount: 1}}},
		},
		TraitPools: []ActorTraitPool{{
			ID: "nature", Name: "性格", Description: "未被抽取的配置词条",
			Traits: []ActorTraitDefinition{
				{ID: "patient", Name: "耐心", Summary: "善于等待。", Weight: 10, Visibility: "visible"},
				{ID: "bold", Name: "果断", Summary: "敢于冒险。", Weight: 1, Visibility: "spoiler"},
				{ID: "secret", Name: "隐藏真相", Summary: "不应进入正文上下文。", Weight: 1, Visibility: "hidden"},
			},
		}},
		InitialActors: []ActorStateInitialActor{{ID: "protagonist", Name: "主角", TemplateID: "protagonist", Role: "protagonist"}},
	}
}

func applyActorTraitTestOps(state map[string]any, ops []StateOp, actorOpGroups ...[]ActorStateOp) map[string]any {
	if state == nil {
		state = map[string]any{}
	}
	for _, op := range ops {
		applyStateOp(state, op)
	}
	for _, actorOps := range actorOpGroups {
		for _, op := range actorOps {
			applyActorStateOp(state, op)
		}
	}
	return state
}
