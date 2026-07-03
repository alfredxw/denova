package interactive

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestResolveTurnRulesDeterministicFailureAndTerminalCandidate(t *testing.T) {
	state := map[string]any{
		"resources": map[string]any{"hp": float64(12), "stamina": float64(5)},
	}
	brief := TurnBrief{
		UserAction: "我强行冲进秘境入口",
		Intent:     "冒险",
		TurnGoal:   "让主角承担强闯禁制的后果",
		RuleChecks: []RuleCheck{{
			ID:                "force_gate",
			Label:             "强闯秘境入口",
			AttributePath:     "resources.stamina",
			Dice:              "1d20",
			Difficulty:        100,
			ResourceCostPath:  "resources.stamina",
			ResourceCost:      2,
			FailureStateOps:   []StateOp{{Op: "inc", Path: "resources.hp", Value: float64(-6)}},
			TerminalOnFailure: true,
			TerminalType:      "bad_end",
			TerminalReason:    "主角在禁制反噬中失去继续推进主线的能力。",
			Seed:              42,
		}},
	}

	first, err := ResolveTurnRules("st_1", "main", state, brief)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ResolveTurnRules("st_1", "main", state, brief)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.RuleResults) != 1 || first.RuleResults[0].Outcome != "critical_failure" {
		t.Fatalf("unexpected rule result: %#v", first.RuleResults)
	}
	if len(first.RuleResults[0].Rolls) == 0 || first.RuleResults[0].Rolls[0] != second.RuleResults[0].Rolls[0] {
		t.Fatalf("same seed should reproduce rolls: first=%#v second=%#v", first.RuleResults[0].Rolls, second.RuleResults[0].Rolls)
	}
	if first.TerminalCandidate == nil || first.TerminalCandidate.Type != "bad_end" {
		t.Fatalf("expected terminal candidate: %#v", first.TerminalCandidate)
	}
	if len(first.StateOpsPreview) != 2 {
		t.Fatalf("expected failure op plus resource cost: %#v", first.StateOpsPreview)
	}
}

func TestResolveTurnRulesUsesSafeExpressionAndStatePath(t *testing.T) {
	state := map[string]any{
		"resources": map[string]any{"stamina": float64(6)},
		"traits":    map[string]any{"focus": float64(2)},
	}
	brief := TurnBrief{
		UserAction: "我调整呼吸后突进",
		Intent:     "战斗",
		TurnGoal:   "用数值公式结算突进是否成功",
		RuleChecks: []RuleCheck{{
			ID:         "dash",
			Label:      "突进检定",
			Expression: "resources.stamina + max(traits.focus, 1) * 2",
			Modifier:   1,
			Difficulty: 10,
			Seed:       7,
		}},
	}
	resolution, err := ResolveTurnRules("st_expr", "main", state, brief)
	if err != nil {
		t.Fatal(err)
	}
	result := resolution.RuleResults[0]
	if result.ExpressionValue != 10 || result.Total != 11 || result.Outcome != "success" {
		t.Fatalf("expression should contribute to total: %#v", result)
	}
	if _, err := ResolveTurnRules("st_expr", "main", state, TurnBrief{
		UserAction: "尝试非法公式",
		Intent:     "观察",
		TurnGoal:   "返回结构化错误",
		RuleChecks: []RuleCheck{{ID: "bad", Expression: "resources.stamina / (2 - 2)"}},
	}); err != nil {
		t.Fatalf("runtime expression errors should be rule results, not validation failure: %v", err)
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

	brief := TurnBrief{UserAction: "观察擂台", Intent: "观察", TurnGoal: "建立比拼压力"}
	resolution, err := ResolveTurnRules(story.ID, "main", snapshot.State, brief)
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:       "main",
		User:           "观察擂台",
		Narrative:      "擂台上的钟声压住了人群。",
		TurnBrief:      &brief,
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
	if snapshot.CurrentTurn.TurnBrief == nil || snapshot.CurrentTurn.TurnBrief.TurnGoal != "建立比拼压力" {
		t.Fatalf("turn brief not persisted: %#v", snapshot.CurrentTurn.TurnBrief)
	}
	if snapshot.CurrentTurn.RuleResolution == nil || snapshot.CurrentTurn.RuleResolution.ID != resolution.ID {
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
