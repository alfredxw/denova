package interactive

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestStoryDirectorLibraryCRUDAndRevisionConflict(t *testing.T) {
	library := NewStoryDirectorLibrary(t.TempDir())

	directors, err := library.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(directors) == 0 || directors[0].ID != DefaultStoryDirectorID || directors[0].Custom {
		t.Fatalf("default story director should be materialized first: %#v", directors)
	}
	if directors[0].ModuleRefs.NarrativeStyleDisabled || directors[0].ModuleRefs.EventPackagesDisabled || directors[0].ModuleRefs.RuleSystemDisabled || directors[0].ModuleRefs.ActorStateDisabled || directors[0].ModuleRefs.OpeningSelectorDisabled || directors[0].ModuleRefs.ImagePresetDisabled {
		t.Fatalf("default story director modules should start enabled: %#v", directors[0].ModuleRefs)
	}
	if directors[0].Strategy.DirectorAgentMode != DirectorAgentModeTriggered || directors[0].Strategy.BranchPlanningTurns != defaultBranchPlanningTurns {
		t.Fatalf("default story director should use triggered background director schedule: %#v", directors[0].Strategy)
	}

	created, err := library.Create(StoryDirector{
		ID:          "custom-director",
		Name:        "自定义导演",
		Description: "用于测试",
		Strategy: StoryDirectorStrategy{
			Enabled:             true,
			RandomEventRate:     2,
			DirectorAgentMode:   "unknown",
			BranchPlanningTurns: 99,
		},
		ActorState: StoryDirectorActorStateSystem{
			Templates: []ActorStateTemplate{{
				ID:   "protagonist",
				Name: "主角",
				Fields: []ActorStateField{
					{ID: "mana", Path: "resources.mana", Name: "法力", Type: "number", Default: float64(3), Max: floatPtr(9), Visibility: "hidden"},
					{ID: "invalid", Path: ".bad", Name: "无效", Type: "number"},
				},
			}},
			InitialActors: []ActorStateInitialActor{{ID: DefaultActorID, Name: "主角", TemplateID: "protagonist", Role: "protagonist"}},
		},
		TRPGSystem: StoryDirectorTRPGSystem{RuleTemplates: []RuleCheck{{
			ID:               "luck",
			Label:            "幸运",
			Kind:             "dice",
			Mode:             "d100_under",
			AttributePath:    "resources.mana",
			Dice:             "1d100",
			Difficulty:       55,
			ResourceCostPath: "resources.mana",
			ResourceCost:     1,
		}}},
		OpeningSelector: StoryDirectorOpeningSelector{
			Enabled: true,
			InitialStateOps: []StateOp{{
				Op:    "set",
				Path:  "flags.opening",
				Value: true,
			}},
		},
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if !created.Custom || created.Strategy.RandomEventRate != 1 || created.Strategy.DirectorAgentMode != DirectorAgentModeTriggered || created.Strategy.BranchPlanningTurns != 12 {
		t.Fatalf("custom director should be marked and strategy should be normalized: %#v", created)
	}
	if created.ModuleRefs.EventPackagesDisabled || created.ModuleRefs.RuleSystemDisabled || created.ModuleRefs.OpeningSelectorDisabled {
		t.Fatalf("legacy-style directors without disabled refs should keep modules enabled: %#v", created.ModuleRefs)
	}
	if len(created.ActorState.Templates) != 1 || len(created.ActorState.Templates[0].Fields) != 1 || created.ActorState.Templates[0].Fields[0].Visibility != "hidden" {
		t.Fatalf("state fields should be validated and preserve visibility: %#v", created.ActorState)
	}
	if len(created.TRPGSystem.RuleTemplates) != 1 || created.TRPGSystem.RuleTemplates[0].AttributePath != "actors.protagonist.state.resources.mana" || created.TRPGSystem.RuleTemplates[0].ResourceCostPath != "actors.protagonist.state.resources.mana" {
		t.Fatalf("legacy rule paths should migrate to protagonist actor state: %#v", created.TRPGSystem.RuleTemplates)
	}
	ops := StoryDirectorInitialStateOps(created)
	if !containsStateOp(ops, "actors.protagonist.state.resources.mana", float64(3)) || !containsStateOp(ops, "actors.protagonist.state.resources.mana_max", float64(9)) || !containsStateOp(ops, "flags.opening", true) {
		t.Fatalf("initial state ops should include attribute defaults and opening ops: %#v", ops)
	}

	updated, err := library.Update(created.ID, StoryDirector{
		Name:          "Agent 更新",
		Strategy:      StoryDirectorStrategy{Enabled: true},
		TRPGSystem:    created.TRPGSystem,
		ActorState:    created.ActorState,
		EventPackages: created.EventPackages,
		OpeningSelector: StoryDirectorOpeningSelector{
			Enabled: true,
		},
	}, created.UpdatedAt)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if _, err := library.Update(created.ID, StoryDirector{Name: "旧前端保存"}, created.UpdatedAt); !errors.Is(err, ErrStoryDirectorRevisionConflict) {
		t.Fatalf("expected story director revision conflict, got %v", err)
	}
	got, err := library.Get(created.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Name != updated.Name {
		t.Fatalf("stale update should not overwrite story director: %#v", got)
	}
}

func TestStoryDirectorBuiltinOverrideAndRestore(t *testing.T) {
	library := NewStoryDirectorLibrary(t.TempDir())
	builtin, err := library.Get(DefaultStoryDirectorID)
	if err != nil {
		t.Fatal(err)
	}
	builtin.Name = "我的默认导演"
	overridden, err := library.Update(DefaultStoryDirectorID, builtin, builtin.UpdatedAt)
	if err != nil {
		t.Fatalf("Update built-in story director should create override: %v", err)
	}
	if overridden.Custom || !overridden.BuiltinOverridden || overridden.ID != DefaultStoryDirectorID || overridden.Name != "我的默认导演" {
		t.Fatalf("unexpected built-in director override: %#v", overridden)
	}

	listed, err := library.List()
	if err != nil {
		t.Fatal(err)
	}
	foundOverride := false
	for _, director := range listed {
		if director.ID == DefaultStoryDirectorID {
			foundOverride = true
			if director.Custom || !director.BuiltinOverridden || director.Name != "我的默认导演" {
				t.Fatalf("list should expose built-in director override: %#v", director)
			}
		}
	}
	if !foundOverride {
		t.Fatalf("default story director missing from list: %#v", listed)
	}

	if err := library.Delete(DefaultStoryDirectorID); err != nil {
		t.Fatalf("Delete built-in director override should restore builtin: %v", err)
	}
	restored, err := library.Get(DefaultStoryDirectorID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Custom || restored.BuiltinOverridden || restored.Name != DefaultStoryDirector().Name {
		t.Fatalf("unexpected restored built-in director: %#v", restored)
	}
}

func TestStoryDirectorStrategyPromptMarkdownNormalizeAndSummaries(t *testing.T) {
	longPrompt := "  " + strings.Repeat("策略", 3000)
	director := normalizeStoryDirector(StoryDirector{
		ID:   "prompt-director",
		Name: "提示导演",
		Strategy: StoryDirectorStrategy{
			Enabled:        true,
			PromptMarkdown: longPrompt,
		},
	})
	if director.Strategy.PromptMarkdown == "" {
		t.Fatalf("prompt markdown should be preserved")
	}
	if strings.HasPrefix(director.Strategy.PromptMarkdown, " ") {
		t.Fatalf("prompt markdown should be trimmed: %q", director.Strategy.PromptMarkdown[:8])
	}
	if len(director.Strategy.PromptMarkdown) > MaxStoryDirectorStrategyPromptBytes {
		t.Fatalf("prompt markdown should be bounded, bytes=%d", len(director.Strategy.PromptMarkdown))
	}
	if !utf8.ValidString(director.Strategy.PromptMarkdown) {
		t.Fatalf("prompt markdown should remain valid UTF-8")
	}
	if got := StoryDirectorStrategyPromptMarkdown(director); got != director.Strategy.PromptMarkdown {
		t.Fatalf("strategy prompt helper mismatch: %q vs %q", got, director.Strategy.PromptMarkdown)
	}
	if DefaultStoryDirector().Strategy.PromptMarkdown != "" {
		t.Fatalf("default story director should not set a custom prompt")
	}
	oversized := normalizeStoryDirector(StoryDirector{
		ID:   "oversized-prompt-director",
		Name: "超长提示导演",
		Strategy: StoryDirectorStrategy{
			Enabled:        true,
			PromptMarkdown: strings.Repeat("a", MaxStoryDirectorStrategyPromptBytes+128),
		},
	})
	if len([]byte(oversized.Strategy.PromptMarkdown)) != MaxStoryDirectorStrategyPromptBytes {
		t.Fatalf("oversized prompt should be trimmed to %d bytes, got %d", MaxStoryDirectorStrategyPromptBytes, len([]byte(oversized.Strategy.PromptMarkdown)))
	}
	ruleSummary := StoryDirectorRuleSummary(director, 8*1024)
	planningSummary := StoryDirectorPlanningSummary(director, 128*1024)
	for name, summary := range map[string]string{"rule": ruleSummary, "planning": planningSummary} {
		if strings.Contains(summary, "prompt_markdown") || strings.Contains(summary, "策略策略策略") {
			t.Fatalf("%s summary should keep markdown prompt out of structured summary:\n%s", name, summary)
		}
		if !strings.Contains(summary, `"strategy"`) || !strings.Contains(summary, `"mainline_strength"`) {
			t.Fatalf("%s summary should retain structured strategy fields:\n%s", name, summary)
		}
		if !strings.Contains(summary, `"director_agent_mode"`) || !strings.Contains(summary, `"branch_planning_turns"`) {
			t.Fatalf("%s summary should retain background director schedule:\n%s", name, summary)
		}
	}
}

func TestStoryDirectorLibraryMigratesLegacyCustomTellerOrchestration(t *testing.T) {
	novaDir := t.TempDir()
	tellers := NewTellerLibrary(novaDir)
	directors := NewStoryDirectorLibrary(novaDir)
	if _, err := directors.Create(StoryDirector{
		ID:       "preexisting",
		Name:     "手动导演",
		Strategy: StoryDirectorStrategy{Enabled: true},
	}); err != nil {
		t.Fatalf("create preexisting director failed: %v", err)
	}
	if _, err := tellers.Create(Teller{
		ID:              "legacy-style",
		Name:            "旧风格",
		RandomEventRate: 0.42,
		Orchestration:   legacyOrchestrationForTest(),
		Slots:           []TellerPromptSlot{testTellerSlot()},
	}); err != nil {
		t.Fatalf("create legacy teller failed: %v", err)
	}
	if _, err := tellers.Create(Teller{
		ID:            "preexisting",
		Name:          "不应覆盖",
		Orchestration: legacyOrchestrationForTest(),
		Slots:         []TellerPromptSlot{testTellerSlot()},
	}); err != nil {
		t.Fatalf("create preexisting teller failed: %v", err)
	}

	if _, err := directors.List(); err != nil {
		t.Fatalf("List should migrate legacy tellers: %v", err)
	}
	migrated, err := directors.Get("legacy-style")
	if err != nil {
		t.Fatalf("legacy teller orchestration should migrate: %v", err)
	}
	if migrated.Name != "旧风格 故事导演" || migrated.Strategy.RandomEventRate != 0.42 {
		t.Fatalf("unexpected migrated metadata: %#v", migrated)
	}
	if len(migrated.ModuleRefs.EventPackageIDs) == 0 || migrated.ModuleRefs.RuleSystemID == "" || migrated.ModuleRefs.OpeningSelectorID == "" {
		t.Fatalf("legacy embedded orchestration should be split into module refs: %#v", migrated.ModuleRefs)
	}
	eventModule, err := NewEventPackageLibrary(novaDir).Get(migrated.ModuleRefs.EventPackageIDs[0])
	if err != nil {
		t.Fatalf("migrated event module should be readable: %v", err)
	}
	if eventModule.ID != "legacy-pack" || len(eventModule.Events) != 1 {
		t.Fatalf("migrated event package should preserve event cards: %#v", eventModule)
	}
	if migrated.Strategy.MainlineStrength != "hard_guidance" || migrated.Strategy.FailurePolicy != "fail_forward" || migrated.Strategy.PacingCurve != "spiky" {
		t.Fatalf("strategy should be copied: %#v", migrated.Strategy)
	}
	if len(migrated.EventPackages) != 1 || len(migrated.EventPackages[0].Events) != 1 {
		t.Fatalf("event packages should be copied: %#v", migrated.EventPackages)
	}
	if len(migrated.TRPGSystem.RuleTemplates) != 1 || migrated.TRPGSystem.RuleTemplates[0].Mode != "d20_dc" {
		t.Fatalf("rule templates should be copied: %#v", migrated.TRPGSystem.RuleTemplates)
	}
	if len(migrated.OpeningSelector.TraitPools) != 1 || !containsStateOp(migrated.OpeningSelector.InitialStateOps, "actors.protagonist.state.resources.hp", float64(12)) {
		t.Fatalf("opening selector should be copied: %#v", migrated.OpeningSelector)
	}

	preexisting, err := directors.Get("preexisting")
	if err != nil {
		t.Fatalf("Get preexisting failed: %v", err)
	}
	if preexisting.Name != "手动导演" {
		t.Fatalf("migration should not overwrite existing story director: %#v", preexisting)
	}
}

func legacyOrchestrationForTest() *TellerOrchestrationConfig {
	return &TellerOrchestrationConfig{
		Enabled:          true,
		MainlineStrength: "hard_guidance",
		FailurePolicy:    "fail_forward",
		PacingCurve:      "spiky",
		EventPackages: []TellerEventPackage{{
			ID:      "legacy-pack",
			Name:    "旧事件包",
			Enabled: true,
			Events: []TellerEventCard{{
				ID:                  "reversal",
				TypeName:            "反转",
				DescriptionMarkdown: "误会后反转。",
				Enabled:             true,
				Weight:              2,
			}},
		}},
		RuleTemplates: []RuleCheck{{
			ID:         "stealth",
			Label:      "潜行",
			Kind:       "dice",
			Mode:       "d20_dc",
			Dice:       "1d20",
			Difficulty: 15,
		}},
		Opening: TellerOpeningConfig{
			Enabled: true,
			TraitPools: []OpeningTraitPool{{
				ID:        "talent",
				Name:      "天赋",
				DrawCount: 1,
				Traits:    []OpeningTrait{{ID: "sharp-eye", Name: "慧眼"}},
			}},
			InitialStateOps: []StateOp{{
				Op:    "set",
				Path:  "resources.hp",
				Value: float64(12),
			}},
		},
	}
}

func testTellerSlot() TellerPromptSlot {
	return TellerPromptSlot{
		ID:      "identity",
		Name:    "系统提示",
		Target:  "system",
		Enabled: true,
		Content: "规则",
	}
}

func containsStateOp(ops []StateOp, path string, value any) bool {
	for _, op := range ops {
		if op.Path == path && op.Value == value {
			return true
		}
	}
	return false
}
