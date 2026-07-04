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

func seedForTurnCheckOutcome(t *testing.T, mode, difficulty string, bonus float64, want string) int64 {
	t.Helper()
	target := turnCheckDifficultyTargets[difficulty]
	for seed := int64(1); seed < 10000; seed++ {
		_, keptRoll, err := rollTurnCheck(seed, mode)
		if err != nil {
			t.Fatal(err)
		}
		if got := resolveTurnCheckOutcome(keptRoll, float64(keptRoll)+bonus, target); got == want {
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
	req.Bonuses = []TurnCheckBonus{{Reason: "有开锁工具", Value: 2}, {Reason: "雨中手冷", Value: -1}}
	req.Outcomes.Failure.StateChanges = []TurnStateChange{{Path: "resources.stamina", Change: -1}}
	seed := seedForTurnCheckOutcome(t, "normal", "normal", 1, "failure")

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
	if len(resolution.Result.StateChanges) != 1 || resolution.Result.StateChanges[0].Change != -1 {
		t.Fatalf("state changes should come from selected outcome: %#v", resolution.Result.StateChanges)
	}
	output := resolution.ToolOutput()
	if output.ResolutionID != resolution.ID || output.Result != req.Outcomes.Failure.Result || output.Target != 15 {
		t.Fatalf("unexpected tool output: %#v", output)
	}
}

func TestResolveTurnRulesRollModesAndDifficultyTargets(t *testing.T) {
	for difficulty, target := range turnCheckDifficultyTargets {
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
			if got := resolveTurnCheckOutcome(tt.keptRoll, tt.total, tt.target); got != tt.want {
				t.Fatalf("resolveTurnCheckOutcome() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestResolveTurnRulesCriticalOutcomes(t *testing.T) {
	req := sampleTurnCheckRequest()
	criticalSuccessSeed := seedForTurnCheckOutcome(t, "normal", "normal", 0, "critical_success")
	criticalFailureSeed := seedForTurnCheckOutcome(t, "normal", "normal", 0, "critical_failure")

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
		Tags:            []string{},
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
