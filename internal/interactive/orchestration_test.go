package interactive

import (
	"os"
	"strings"
	"testing"
	"time"
)

func sampleTurnCheckRequest() TurnCheckRequest {
	return TurnCheckRequest{
		Action:     "撬开仓库后门的锁",
		Intent:     "潜入仓库寻找线索",
		Challenge:  "锁很旧但周围有人巡逻",
		Cost:       "尝试会消耗体力并增加暴露风险",
		State:      "主角体力尚可，手上有简易开锁工具。",
		Rule:       TurnCheckRule{Template: "dice_check", Dice: "1d20", RollMode: "normal"},
		Difficulty: "normal",
		Outcomes: TurnCheckOutcomes{
			CriticalSuccess: TurnCheckOutcome{Result: "非常成功，轻而易举地撬开了锁，没有任何人发现。"},
			Success:         TurnCheckOutcome{Result: "撬开了锁，体力-1。"},
			Failure:         TurnCheckOutcome{Result: "没撬开，体力-1，只能想别的办法。"},
			CriticalFailure: TurnCheckOutcome{Result: "使尽浑身解数锁也打不开，体力-2，还被发现了。"},
		},
	}
}

func seedForTurnCheckOutcome(t *testing.T, dice, mode, difficulty string, modifier, bonus float64, want string) int64 {
	t.Helper()
	baseTarget, ok := turnCheckDifficultyTarget(dice, difficulty)
	if !ok {
		t.Fatalf("invalid difficulty %q for %s", difficulty, dice)
	}
	target := turnCheckTarget(dice, baseTarget, modifier, bonus)
	for seed := int64(1); seed < 10000; seed++ {
		_, keptRoll, err := rollTurnCheck(seed, dice, mode)
		if err != nil {
			t.Fatal(err)
		}
		if got := resolveTurnCheckOutcome(dice, keptRoll, turnCheckTotal(dice, keptRoll, bonus), target); got == want {
			return seed
		}
	}
	t.Fatalf("failed to find seed for outcome %s", want)
	return 0
}

func maxInt(values ...int) int {
	out := values[0]
	for _, value := range values[1:] {
		if value > out {
			out = value
		}
	}
	return out
}

func minInt(values ...int) int {
	out := values[0]
	for _, value := range values[1:] {
		if value < out {
			out = value
		}
	}
	return out
}

func TestResolveTurnRulesSingleD20CheckSelectsOutcomeAndStateChanges(t *testing.T) {
	req := sampleTurnCheckRequest()
	req.Difficulty = "normal"
	req.Adjudication = TurnCheckAdjudication{
		Reason:           "巡逻靠近，开锁失败会改变局势。",
		Stakes:           "失败会消耗体力并提高警戒。",
		DifficultyReason: "旧锁简单但有时间压力。",
		RollModeReason:   "有工具也有雨水干扰，正常投骰。",
		StatePaths:       []string{"resources.stamina"},
	}
	req.Rule.TemplateID = "stealth-lock"
	req.Rule.Label = "潜行与开锁"
	req.Rule.FailurePolicy = "blocked"
	req.Bonuses = []TurnCheckBonus{{Kind: "equipment", SourcePath: "inventory.lockpick", Reason: "有开锁工具", Value: 2}, {Kind: "environment", Reason: "雨中手冷", Value: -1}}
	req.Outcomes.Failure.StateChanges = []TurnStateChange{{Path: "resources.stamina", Change: -1, Reason: "紧张尝试消耗体力"}}
	seed := seedForTurnCheckOutcome(t, "1d20", "normal", "normal", 0, 1, "failure")

	resolution, err := resolveTurnRulesWithSeed("st_1", "main", initialStoryState(), req, seed)
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Result.Outcome != "failure" || resolution.Result.Result != req.Outcomes.Failure.Result {
		t.Fatalf("unexpected result: %#v", resolution.Result)
	}
	if resolution.Result.BonusTotal != 1 || resolution.Result.Total != resolution.Result.KeptRoll+1 {
		t.Fatalf("bonus should contribute to total: %#v", resolution.Result)
	}
	if resolution.Result.BaseTarget != 10 || len(resolution.Result.BonusDetails) != 2 || resolution.Result.BonusDetails[0].Kind != "equipment" {
		t.Fatalf("expected auditable target and bonus details: %#v", resolution.Result)
	}
	if resolution.Request.Adjudication.StatePaths[0] != "actors.protagonist.state.resources.stamina" || resolution.Request.Rule.TemplateID != "stealth-lock" {
		t.Fatalf("expected normalized adjudication and rule audit: %#v", resolution.Request)
	}
	if len(resolution.Result.StateChanges) != 1 || resolution.Result.StateChanges[0].Change != -1 {
		t.Fatalf("state changes should come from selected outcome: %#v", resolution.Result.StateChanges)
	}
	output := resolution.ToolOutput()
	if output.ResolutionID != resolution.ID || output.Result != req.Outcomes.Failure.Result || output.Target != 10 || output.BaseTarget != 10 || len(output.BonusDetails) != 2 {
		t.Fatalf("unexpected tool output: %#v", output)
	}
}

func TestResolveTurnRulesRollModesAndDifficultyTargets(t *testing.T) {
	for difficulty, target := range turnCheckD20DifficultyTargets {
		req := sampleTurnCheckRequest()
		req.Difficulty = difficulty
		resolution, err := resolveTurnRulesWithSeed("st_diff", "main", initialStoryState(), req, 7)
		if err != nil {
			t.Fatal(err)
		}
		if resolution.Result.Target != target {
			t.Fatalf("difficulty %s target = %v, want %v", difficulty, resolution.Result.Target, target)
		}
	}
	req := sampleTurnCheckRequest()
	req.Rule = TurnCheckRule{}
	resolution, err := resolveTurnRulesWithSeed("st_default_rule", "main", initialStoryState(), req, 7)
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Result.Dice != "1d20" || resolution.Result.RollMode != "normal" {
		t.Fatalf("empty rule should default to 1d20 normal: %#v", resolution.Result)
	}
	for _, mode := range []string{"normal", "advantage", "disadvantage"} {
		req := sampleTurnCheckRequest()
		req.Rule.RollMode = mode
		resolution, err := resolveTurnRulesWithSeed("st_mode", "main", initialStoryState(), req, 11)
		if err != nil {
			t.Fatal(err)
		}
		if mode == "normal" && len(resolution.Result.Rolls) != 1 {
			t.Fatalf("normal should roll once: %#v", resolution.Result.Rolls)
		}
		if mode != "normal" && len(resolution.Result.Rolls) != 2 {
			t.Fatalf("%s should roll twice: %#v", mode, resolution.Result.Rolls)
		}
		if mode == "advantage" && int(resolution.Result.KeptRoll) != maxInt(resolution.Result.Rolls...) {
			t.Fatalf("advantage should keep high roll: %#v", resolution.Result)
		}
		if mode == "disadvantage" && int(resolution.Result.KeptRoll) != minInt(resolution.Result.Rolls...) {
			t.Fatalf("disadvantage should keep low roll: %#v", resolution.Result)
		}
	}
}

func TestResolveTurnRulesLegacyD100InputsUseFixedD20(t *testing.T) {
	req := sampleTurnCheckRequest()
	req.Rule.Dice = "1d100"
	req.Difficulty = "normal"

	resolution, err := resolveTurnRulesWithSeed("st_legacy_d100", "main", initialStoryState(), req, 7)
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Result.Dice != "1d20" || resolution.Result.Mode != "d20_dc" || resolution.Result.Target != 10 {
		t.Fatalf("legacy d100 should resolve as fixed d20: %#v", resolution.Result)
	}

	req.Rule.Modifier = 10
	resolution, err = resolveTurnRulesWithSeed("st_legacy_d100_modifier", "main", initialStoryState(), req, 7)
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Result.Target != 20 {
		t.Fatalf("positive modifier should raise d20 target: %#v", resolution.Result)
	}

	for _, mode := range []string{"normal", "advantage", "disadvantage"} {
		req := sampleTurnCheckRequest()
		req.Rule.Dice = "1d100"
		req.Rule.RollMode = mode
		resolution, err := resolveTurnRulesWithSeed("st_legacy_d100_mode", "main", initialStoryState(), req, 11)
		if err != nil {
			t.Fatal(err)
		}
		if mode == "normal" && len(resolution.Result.Rolls) != 1 {
			t.Fatalf("normal should roll once: %#v", resolution.Result.Rolls)
		}
		if mode != "normal" && len(resolution.Result.Rolls) != 2 {
			t.Fatalf("%s should roll twice: %#v", mode, resolution.Result.Rolls)
		}
		if mode == "advantage" && int(resolution.Result.KeptRoll) != maxInt(resolution.Result.Rolls...) {
			t.Fatalf("fixed d20 advantage should keep high roll: %#v", resolution.Result)
		}
		if mode == "disadvantage" && int(resolution.Result.KeptRoll) != minInt(resolution.Result.Rolls...) {
			t.Fatalf("fixed d20 disadvantage should keep low roll: %#v", resolution.Result)
		}
	}
}

func TestResolveTurnRulesAppliesStateBindingModifiersAndOutcomeChanges(t *testing.T) {
	director, state := stateBindingTestDirectorAndState()
	req := sampleTurnCheckRequest()
	req.Rule.TemplateID = "combat"
	req.Rule.BindingID = "melee_attack"
	req.Rule.ActorID = "protagonist"
	req.Rule.TargetActorID = "wolf_1"
	req.Difficulty = "normal"
	req.Outcomes.Success.StateChanges = []TurnStateChange{{ActorID: "wolf_1", FieldID: "护体", Change: -1, Reason: "临场追击继续削弱护体。"}}
	seed := seedForTurnCheckOutcome(t, "1d20", "normal", "normal", 2, 3, "success")

	resolution, err := resolveTurnRulesWithSeedAndDirector("st_binding", "main", state, director, req, seed)
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Result.Outcome != "success" {
		t.Fatalf("expected success, got %#v", resolution.Result)
	}
	if resolution.Result.BonusTotal != 3 || resolution.Result.Target != 12 || resolution.Result.Modifier != 2 {
		t.Fatalf("binding modifiers should affect total and target: %#v", resolution.Result)
	}
	if resolution.StateBinding == nil || resolution.StateBinding.BindingBonusTotal != 3 || resolution.StateBinding.BindingResistanceTotal != 2 {
		t.Fatalf("missing binding audit: %#v", resolution.StateBinding)
	}
	if len(resolution.StateBinding.StateInputs) != 2 || resolution.StateBinding.StateInputs[0].ActorID != "protagonist" || resolution.StateBinding.StateInputs[0].FieldID != "力量" || resolution.StateBinding.StateInputs[1].ActorID != "wolf_1" || resolution.StateBinding.StateInputs[1].FieldID != "防御" {
		t.Fatalf("unexpected state inputs: %#v", resolution.StateBinding.StateInputs)
	}
	if len(resolution.Result.StateChanges) != 2 {
		t.Fatalf("expected binding state change plus manual state change: %#v", resolution.Result.StateChanges)
	}
	if resolution.Result.StateChanges[0].ActorID != "wolf_1" || resolution.Result.StateChanges[0].FieldID != "护体" || resolution.Result.StateChanges[0].Change != -4 {
		t.Fatalf("binding outcome state change should be computed from attack-defense: %#v", resolution.Result.StateChanges)
	}
	if len(resolution.StateBinding.Warnings) != 1 || !strings.Contains(resolution.StateBinding.Warnings[0].Reason, "同一状态字段") {
		t.Fatalf("duplicate state changes should produce warning: %#v", resolution.StateBinding.Warnings)
	}

	ops := applyRuleStateConsumption(state, director.ActorState, "turn_1", &resolution, RuleStateConsumptionModeHybridAuto)
	if len(ops) != 2 {
		t.Fatalf("expected both state changes to be consumed in order: %#v", ops)
	}
	if got := numberFromAny(actorStateFieldValue(state, "wolf_1", "护体")); got != 0 {
		t.Fatalf("guard should be clamped after configured and manual changes, got %v", got)
	}
}

func TestResolveTurnRulesStateBindingValidationErrors(t *testing.T) {
	director, state := stateBindingTestDirectorAndState()
	t.Run("missing target", func(t *testing.T) {
		req := stateBindingTestRequest()
		req.Rule.TargetActorID = ""
		_, err := resolveTurnRulesWithSeedAndDirector("st_binding", "main", state, director, req, 7)
		if err == nil || !strings.Contains(err.Error(), "target_actor_id") {
			t.Fatalf("expected target_actor_id error, got %v", err)
		}
	})
	t.Run("template mismatch", func(t *testing.T) {
		req := stateBindingTestRequest()
		req.Rule.ActorID = "wolf_1"
		_, err := resolveTurnRulesWithSeedAndDirector("st_binding", "main", state, director, req, 7)
		if err == nil || !strings.Contains(err.Error(), "模板不匹配") {
			t.Fatalf("expected template mismatch, got %v", err)
		}
	})
	t.Run("non number modifier", func(t *testing.T) {
		next := director
		next.TRPGSystem.RuleTemplates[0].StateBindings[0].Modifiers[0].FieldID = "conditions.realm"
		req := stateBindingTestRequest()
		_, err := resolveTurnRulesWithSeedAndDirector("st_binding", "main", state, next, req, 7)
		if err == nil || !strings.Contains(err.Error(), "不是 number") {
			t.Fatalf("expected non-number field error, got %v", err)
		}
	})
}

func TestResolveTurnRulesNormalizesTurnCheckAliases(t *testing.T) {
	for _, tc := range []struct {
		name           string
		difficulty     string
		wantDifficulty string
		template       string
		wantTemplate   string
	}{
		{name: "medium", difficulty: "medium", wantDifficulty: "normal"},
		{name: "moderate", difficulty: "moderate", wantDifficulty: "normal"},
		{name: "space separated", difficulty: "very easy", wantDifficulty: "very_easy"},
		{name: "hyphenated", difficulty: "very-hard", wantDifficulty: "very_hard"},
		{name: "legacy template", difficulty: "normal", wantDifficulty: "normal", template: "d20_check", wantTemplate: "dice_check"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := sampleTurnCheckRequest()
			req.Difficulty = tc.difficulty
			if tc.template != "" {
				req.Rule.Template = tc.template
			}
			resolution, err := resolveTurnRulesWithSeed("st_alias", "main", initialStoryState(), req, 7)
			if err != nil {
				t.Fatal(err)
			}
			if resolution.Request.Difficulty != tc.wantDifficulty {
				t.Fatalf("difficulty = %q, want %q", resolution.Request.Difficulty, tc.wantDifficulty)
			}
			if tc.wantTemplate != "" && resolution.Request.Rule.Template != tc.wantTemplate {
				t.Fatalf("template = %q, want %q", resolution.Request.Rule.Template, tc.wantTemplate)
			}
		})
	}
}

func stateBindingTestRequest() TurnCheckRequest {
	req := sampleTurnCheckRequest()
	req.Rule.TemplateID = "combat"
	req.Rule.BindingID = "melee_attack"
	req.Rule.ActorID = "protagonist"
	req.Rule.TargetActorID = "wolf_1"
	req.Difficulty = "normal"
	return req
}

func stateBindingTestDirectorAndState() (StoryDirector, map[string]any) {
	guardMin, guardMax := 0.0, 10.0
	staminaMin, staminaMax := 0.0, 5.0
	system := normalizeActorStateSystem(StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{
			{
				ID:   "protagonist",
				Name: "主角",
				Fields: []ActorStateField{
					{ID: "strength", Path: "attributes.strength", Name: "力量", Type: "number", Default: 0.0},
					{ID: "attack", Path: "attributes.attack", Name: "攻击", Type: "number", Default: 0.0},
					{ID: "stamina", Path: "resources.stamina", Name: "体力", Type: "number", Default: 5.0, Min: &staminaMin, Max: &staminaMax},
					{ID: "realm", Path: "conditions.realm", Name: "境界", Type: "string", Default: "炼气"},
				},
			},
			{
				ID:   "monster",
				Name: "妖兽",
				Fields: []ActorStateField{
					{ID: "defense", Path: "attributes.defense", Name: "防御", Type: "number", Default: 0.0},
					{ID: "guard", Path: "resources.guard", Name: "护体", Type: "number", Default: 5.0, Min: &guardMin, Max: &guardMax},
					{ID: "guard_style", Path: "conditions.guard_style", Name: "护体方式", Type: "string", Default: "护体灵光"},
				},
			},
		},
		InitialActors: []ActorStateInitialActor{
			{ID: "protagonist", Name: "主角", TemplateID: "protagonist", Role: "protagonist"},
			{ID: "wolf_1", Name: "妖狼", TemplateID: "monster", Role: "enemy"},
		},
	})
	director := normalizeStoryDirector(StoryDirector{
		ID:         "binding-director",
		Name:       "Binding Director",
		ActorState: system,
		TRPGSystem: StoryDirectorTRPGSystem{RuleTemplates: []RuleCheck{{
			ID:            "combat",
			Label:         "战斗",
			Dice:          "1d20",
			FailurePolicy: "fail_forward",
			StateBindings: []RuleStateBinding{{
				ID:               "melee_attack",
				Label:            "近战攻击",
				ActorTemplateID:  "protagonist",
				TargetTemplateID: "monster",
				Modifiers: []RuleStateBindingModifier{
					{Source: "actor", FieldID: "attributes.strength", Effect: "advantage", Scale: 1},
					{Source: "target", FieldID: "attributes.defense", Effect: "resistance", Scale: 1},
				},
				NarrativeStateRefs: []RuleNarrativeStateRef{
					{Source: "actor", FieldID: "conditions.realm", Usage: "outcome_design", Guidance: "境界只影响四档结果写法。"},
				},
				OutcomeStateChanges: []RuleOutcomeStateChangeBinding{{
					Outcome: "success",
					StateChanges: []RuleComputedStateChange{{
						Source:  "target",
						FieldID: "resources.guard",
						ChangeFormula: RuleStateChangeFormula{
							Terms: []RuleStateFormulaTerm{
								{Source: "actor", FieldID: "attributes.attack", Scale: -1},
								{Source: "target", FieldID: "attributes.defense", Scale: 1},
							},
							Min:      floatPtr(-8),
							Max:      floatPtr(-1),
							Rounding: "nearest",
						},
						Reason: "攻击命中后削弱护体。",
					}},
				}},
			}},
		}}},
	})
	state := initialStoryState()
	for _, op := range actorStateInitialOps(system) {
		applyStateOp(state, op)
	}
	setPath(state, "actors.protagonist.state.attributes.strength", 3.0)
	setPath(state, "actors.protagonist.state.attributes.attack", 6.0)
	setPath(state, "actors.wolf_1.state.attributes.defense", 2.0)
	setPath(state, "actors.wolf_1.state.resources.guard", 5.0)
	return director, state
}

func TestValidateTurnCheckRequestListsAllowedEnums(t *testing.T) {
	req := sampleTurnCheckRequest()
	req.Difficulty = "mediumish"
	_, err := resolveTurnRulesWithSeed("st_invalid", "main", initialStoryState(), req, 7)
	if err == nil {
		t.Fatal("expected invalid difficulty error")
	}
	if !strings.Contains(err.Error(), "合法值") || !strings.Contains(err.Error(), "very_easy/easy/normal/hard/very_hard") {
		t.Fatalf("difficulty error should list allowed values, got: %v", err)
	}

	req = sampleTurnCheckRequest()
	req.Rule.Template = "safe_expression"
	_, err = resolveTurnRulesWithSeed("st_invalid_template", "main", initialStoryState(), req, 7)
	if err == nil {
		t.Fatal("expected invalid template error")
	}
	if !strings.Contains(err.Error(), "合法值") || !strings.Contains(err.Error(), "dice_check") {
		t.Fatalf("template error should list allowed values, got: %v", err)
	}
}

func TestNormalizeRuleCheckKeepsTriggerExamples(t *testing.T) {
	checks := normalizeRuleChecks([]RuleCheck{
		{
			ID:                "example-rule",
			Label:             "示例规则",
			Dice:              "1d20",
			FailurePolicy:     "fail_forward",
			MustCheckExamples: []string{"  强行撬锁  ", "强行撬锁", "", "攻击守卫"},
			SkipCheckExamples: []string{"观察空房间", "  观察空房间  ", "", "闲聊"},
		},
		{
			ID:            "extra-rule",
			Label:         "多余规则",
			Dice:          "1d100",
			FailurePolicy: "hard_failure",
		},
	})
	if len(checks) != 1 {
		t.Fatalf("check count = %d", len(checks))
	}
	if checks[0].ID != "example-rule" {
		t.Fatalf("normalize should keep only the first TRPG check config, got: %#v", checks)
	}
	if got := checks[0].MustCheckExamples; len(got) != 2 || got[0] != "强行撬锁" || got[1] != "攻击守卫" {
		t.Fatalf("must examples not normalized: %#v", got)
	}
	if got := checks[0].SkipCheckExamples; len(got) != 2 || got[0] != "观察空房间" || got[1] != "闲聊" {
		t.Fatalf("skip examples not normalized: %#v", got)
	}

	checks = normalizeRuleChecks([]RuleCheck{
		{},
		{
			ID:            "legacy-valid-rule",
			Label:         "旧有效规则",
			Dice:          "1d20",
			FailurePolicy: "fail_forward",
		},
	})
	if len(checks) != 1 || checks[0].ID != "legacy-valid-rule" {
		t.Fatalf("normalize should keep the first valid TRPG check config: %#v", checks)
	}
}

func TestResolveTurnCheckOutcomeCriticalThresholds(t *testing.T) {
	tests := []struct {
		name     string
		keptRoll int
		total    float64
		target   float64
		want     string
	}{
		{name: "natural 20", keptRoll: 20, total: 20, target: 25, want: "critical_success"},
		{name: "natural 1", keptRoll: 1, total: 16, target: 5, want: "critical_failure"},
		{name: "margin critical success", keptRoll: 15, total: 25, target: 15, want: "critical_success"},
		{name: "margin critical failure", keptRoll: 5, total: 5, target: 15, want: "critical_failure"},
		{name: "success", keptRoll: 15, total: 15, target: 15, want: "success"},
		{name: "failure", keptRoll: 10, total: 10, target: 15, want: "failure"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveTurnCheckOutcome("1d20", tt.keptRoll, tt.total, tt.target); got != tt.want {
				t.Fatalf("resolveTurnCheckOutcome() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestResolveTurnRulesCriticalOutcomes(t *testing.T) {
	req := sampleTurnCheckRequest()
	criticalSuccessSeed := seedForTurnCheckOutcome(t, "1d20", "normal", "normal", 0, 0, "critical_success")
	criticalFailureSeed := seedForTurnCheckOutcome(t, "1d20", "normal", "normal", 0, 0, "critical_failure")

	success, err := resolveTurnRulesWithSeed("st_crit", "main", initialStoryState(), req, criticalSuccessSeed)
	if err != nil {
		t.Fatal(err)
	}
	if success.Result.Outcome != "critical_success" || success.Result.Result != req.Outcomes.CriticalSuccess.Result {
		t.Fatalf("unexpected critical success: %#v", success.Result)
	}
	failure, err := resolveTurnRulesWithSeed("st_crit", "main", initialStoryState(), req, criticalFailureSeed)
	if err != nil {
		t.Fatal(err)
	}
	if failure.Result.Outcome != "critical_failure" || failure.Result.Result != req.Outcomes.CriticalFailure.Result {
		t.Fatalf("unexpected critical failure: %#v", failure.Result)
	}
}

func TestOpeningRollProducesTraitsStateOps(t *testing.T) {
	teller := Teller{
		Version:         tellerVersion,
		ID:              "growth",
		Name:            "成长流",
		Description:     "demo",
		RandomEventRate: 0.2,
		ContextPolicy: TellerContextPolicy{
			Creator:      "always",
			Lore:         "relevant",
			RuntimeState: "always",
		},
		Slots: []TellerPromptSlot{{ID: "identity", Name: "系统提示", Target: "system", Enabled: true, Content: "demo"}},
		Orchestration: &TellerOrchestrationConfig{
			Enabled: true,
			Opening: TellerOpeningConfig{
				Enabled:         true,
				InitialStateOps: []StateOp{{Op: "set", Path: "resources.hp", Value: float64(10)}},
				TraitPools: []OpeningTraitPool{{
					ID:        "talent",
					Name:      "天赋",
					DrawCount: 1,
					Traits: []OpeningTrait{{
						ID:      "hidden-bloodline",
						Name:    "隐脉",
						Summary: "灵力上限更高",
						Weight:  1,
						Ops:     []StateOp{{Op: "set", Path: "traits.hidden_bloodline", Value: true}},
					}},
				}},
			},
		},
	}
	result, err := RollOpening(teller, OpeningRollRequest{Seed: 42})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Traits) != 1 || result.Traits[0].ID != "hidden-bloodline" {
		t.Fatalf("unexpected opening traits: %#v", result.Traits)
	}
	if len(result.StateOps) < 3 {
		t.Fatalf("opening should include initial, trait and audit ops: %#v", result.StateOps)
	}
}

func TestOpeningRollWithoutTraitPoolsReturnsEmptyArrays(t *testing.T) {
	teller := builtinTellers["classic"]
	result, err := RollOpening(teller, OpeningRollRequest{Seed: 42})
	if err != nil {
		t.Fatal(err)
	}
	if result.Traits == nil || result.StateOps == nil {
		t.Fatalf("opening roll should return JSON-safe empty arrays: %#v", result)
	}
	if len(result.Traits) != 0 {
		t.Fatalf("expected no traits for default classic teller, got %#v", result.Traits)
	}
}

func TestCreateStoryAppliesOpeningInitialStateOps(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{
		Title:           "开局词条",
		StoryTellerID:   "classic",
		InitialStateOps: []StateOp{{Op: "set", Path: "resources.hp", Value: float64(18)}, {Op: "push", Path: "rules.opening_traits", Value: "隐脉"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if got := numberFromAny(getPath(snapshot.State, "resources.hp")); got != 18 {
		t.Fatalf("initial state ops should be applied, got %v state=%#v", got, snapshot.State)
	}
	if story.Events != 1 {
		t.Fatalf("initial state delta should count as an event: %#v", story)
	}
}

func TestStorySnapshotSeedsDirectorPlanAndPersistsRuleAudit(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "导演规划", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.DirectorPlan == nil || snapshot.DirectorPlan.Metadata.LastRun == nil {
		t.Fatalf("unexpected director plan: %#v", snapshot.DirectorPlan)
	}

	request := sampleTurnCheckRequest()
	request.Action = "观察擂台"
	request.Intent = "观察"
	request.Challenge = "看清擂台上的暗手"
	request.Cost = "可能错过其他人行动"
	request.State = "擂台上的钟声压住了人群。"
	resolution, err := ResolveTurnRules(story.ID, "main", snapshot.State, request)
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:       "main",
		User:           "观察擂台",
		Narrative:      "擂台上的钟声压住了人群。",
		RuleResolution: &resolution,
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.ID != turn.ID {
		t.Fatalf("unexpected current turn: %#v", snapshot.CurrentTurn)
	}
	if snapshot.CurrentTurn.RuleResolution == nil || snapshot.CurrentTurn.RuleResolution.ID != resolution.ID || snapshot.CurrentTurn.RuleResolution.Request.Challenge != "看清擂台上的暗手" {
		t.Fatalf("rule resolution not persisted: %#v", snapshot.CurrentTurn.RuleResolution)
	}
}

func TestLegacyStoryMetaDoesNotFabricateDirectorPlan(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	meta := map[string]any{
		"v":                  schemaVersion,
		"type":               StoryEventTypeMeta,
		"story_id":           "st_legacy_director",
		"title":              "旧故事",
		"story_teller_id":    "classic",
		"reply_target_chars": DefaultStoryReplyTargetChars,
		"opening":            StoryOpeningConfig{Mode: StoryOpeningModeAI},
		"image_settings":     normalizeStoryImageSettings(StoryImageSettings{}),
		"current_branch":     "main",
		"branches":           map[string]any{"main": map[string]any{"created_at": now}},
		"created_at":         now,
		"updated_at":         now,
	}
	if err := writeJSONL(store.storyPath("st_legacy_director"), []any{meta}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(store.storyDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.indexPath(), []byte(`{"current_story_id":"st_legacy_director","stories":[{"id":"st_legacy_director","title":"旧故事","story_teller_id":"classic","created_at":"`+now+`","updated_at":"`+now+`","branches":1}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	snapshot, err := store.Snapshot("st_legacy_director", "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.DirectorPlan != nil {
		t.Fatalf("legacy story without director docs should not fabricate director plan: %#v", snapshot.DirectorPlan)
	}
	data, err := os.ReadFile(store.storyPath("st_legacy_director"))
	if err != nil {
		t.Fatal(err)
	}
	legacyDirectorField := strings.Join([]string{"director", "state"}, "_")
	if strings.Contains(string(data), legacyDirectorField) {
		t.Fatalf("lazy initialization should not rewrite legacy story file: %s", string(data))
	}
}
