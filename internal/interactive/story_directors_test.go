package interactive

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestStoryDirectorLegacyRandomRateMigratesAndIsNotPersisted(t *testing.T) {
	rate := 0.10
	director := normalizeStoryDirector(StoryDirector{ID: "legacy-rate", Name: "旧概率", Strategy: StoryDirectorStrategy{Enabled: true, LegacyRandomEventRate: &rate}})
	if director.Strategy.EventFrequency != EventFrequencySparse || director.Strategy.LegacyRandomEventRate != nil {
		t.Fatalf("legacy random rate should migrate to sparse frequency: %#v", director.Strategy)
	}
	data, err := json.Marshal(director)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "random_event_rate") {
		t.Fatalf("normalized director should not persist legacy probability field: %s", data)
	}
}

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
	if directors[0].Strategy.DirectorAgentMode != DirectorAgentModeTriggered || directors[0].Strategy.StateSchemaAdaptationMode != StateSchemaAdaptationModeAuto || directors[0].Strategy.BranchPlanningTurns != defaultBranchPlanningTurns {
		t.Fatalf("default story director should use triggered background director schedule: %#v", directors[0].Strategy)
	}
	if normalized := NormalizeStoryDirectorStrategy(StoryDirectorStrategy{StateSchemaAdaptationMode: StateSchemaAdaptationModeOff}); normalized.StateSchemaAdaptationMode != StateSchemaAdaptationModeOff {
		t.Fatalf("story director should preserve disabled state schema adaptation: %#v", normalized)
	}

	created, err := library.Create(StoryDirector{
		ID:          "custom-director",
		Name:        "自定义导演",
		Description: "用于测试",
		Strategy: StoryDirectorStrategy{
			Enabled:             true,
			EventFrequency:      EventFrequencyFrequent,
			DirectorAgentMode:   "unknown",
			BranchPlanningTurns: 99,
		},
		ActorState: StoryDirectorActorStateSystem{
			Templates: []ActorStateTemplate{{
				ID:   "protagonist",
				Name: "主角",
				TraitRules: []ActorTraitRule{{
					PoolID: "origins", DrawCount: 1,
				}},
				Fields: []ActorStateField{
					{ID: "mana", Path: "resources.mana", Name: "法力", Type: "number", Default: float64(3), Max: floatPtr(9), Visibility: "hidden"},
					{ID: "invalid", Path: ".bad", Name: "无效", Type: "number"},
				},
			}},
			TraitPools:    []ActorTraitPool{{ID: "origins", Name: "出身", Traits: []ActorTraitDefinition{{ID: "wanderer", Name: "旅人", Weight: 1, Visibility: "visible"}}}},
			InitialActors: []ActorStateInitialActor{{ID: DefaultActorID, Name: "主角", TemplateID: "protagonist", Role: "protagonist"}},
		},
		TRPGSystem: StoryDirectorTRPGSystem{RuleTemplates: []RuleCheck{{
			ID:                  "luck",
			Label:               "幸运",
			Dice:                "1d100",
			Modifier:            10,
			FailurePolicy:       "success_at_cost",
			DifficultyGuidance:  "幸运耗尽时提高难度。",
			StateEffectGuidance: "成功可增加机会，失败可消耗资源。",
		}}},
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if !created.Custom || created.Strategy.EventFrequency != EventFrequencyFrequent || created.Strategy.DirectorAgentMode != DirectorAgentModeTriggered || created.Strategy.BranchPlanningTurns != 12 {
		t.Fatalf("custom director should be marked and strategy should be normalized: %#v", created)
	}
	if created.ModuleRefs.EventPackagesDisabled || created.ModuleRefs.RuleSystemDisabled || created.ModuleRefs.OpeningSelectorDisabled {
		t.Fatalf("legacy-style directors without disabled refs should keep modules enabled: %#v", created.ModuleRefs)
	}
	if len(created.ActorState.Templates) != 1 || len(created.ActorState.Templates[0].Fields) != 2 || created.ActorState.Templates[0].Fields[0].Name != "法力" || created.ActorState.Templates[0].Fields[0].Visibility != "hidden" {
		t.Fatalf("state field names should be preserved as IDs without path syntax filtering: %#v", created.ActorState)
	}
	if len(created.TRPGSystem.RuleTemplates) != 1 || created.TRPGSystem.RuleTemplates[0].Dice != "1d20" || created.TRPGSystem.RuleTemplates[0].Modifier != 10 {
		t.Fatalf("rule templates should normalize to the simplified schema: %#v", created.TRPGSystem.RuleTemplates)
	}
	if created.TRPGSystem.RuleTemplates[0].DifficultyGuidance != "幸运耗尽时提高难度。" || created.TRPGSystem.RuleTemplates[0].StateEffectGuidance != "成功可增加机会，失败可消耗资源。" {
		t.Fatalf("rule templates should preserve natural-language guidance: %#v", created.TRPGSystem.RuleTemplates[0])
	}
	ruleData, err := json.Marshal(created.TRPGSystem.RuleTemplates[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(ruleData), "impact") || strings.Contains(string(ruleData), "category") || strings.Contains(string(ruleData), "default_difficulty") || strings.Contains(string(ruleData), "default_roll_mode") {
		t.Fatalf("rule template JSON should not keep removed fields: %s", string(ruleData))
	}
	ops, actorOps, err := BuildActorStateInitialChanges(created.ActorState, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !containsActorStateOp(actorOps, "protagonist", "法力", float64(3)) || !containsStateOpPath(ops, "actors.protagonist.traits") {
		t.Fatalf("initial state changes should use the name-based field ID and trait snapshots: ops=%#v actor_ops=%#v", ops, actorOps)
	}

	updated, err := library.Update(created.ID, StoryDirector{
		Name:          "Agent 更新",
		Strategy:      StoryDirectorStrategy{Enabled: true},
		TRPGSystem:    created.TRPGSystem,
		ActorState:    created.ActorState,
		EventPackages: created.EventPackages,
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
			Enabled:            true,
			RuleVisibilityMode: "bad-value",
			PromptMarkdown:     longPrompt,
		},
		TRPGSystem: StoryDirectorTRPGSystem{RuleTemplates: []RuleCheck{{
			ID:                  "guidance-rule",
			Label:               "指引规则",
			Dice:                "1d20",
			FailurePolicy:       "fail_forward",
			DifficultyGuidance:  "按状态数值判断难度。",
			StateEffectGuidance: "按检定结果调整状态数值。",
			MustCheckExamples:   []string{"在守卫逼近时撬锁。"},
			SkipCheckExamples:   []string{"观察空房间。"},
		}}},
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
	if director.Strategy.RuleVisibilityMode != RuleVisibilityModeAuditOnly {
		t.Fatalf("invalid rule visibility should fall back to audit_only: %#v", director.Strategy)
	}
	if DefaultStoryDirector().Strategy.PromptMarkdown != "" {
		t.Fatalf("default story director should not set a custom prompt")
	}
	if DefaultStoryDirector().Strategy.RuleVisibilityMode != RuleVisibilityModeAuditOnly {
		t.Fatalf("default story director should keep rule audit sidebar only: %#v", DefaultStoryDirector().Strategy)
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
		if !strings.Contains(summary, `"director_agent_mode"`) || !strings.Contains(summary, `"branch_planning_turns"`) || !strings.Contains(summary, `"rule_visibility_mode"`) || !strings.Contains(summary, `"state_schema_adaptation_mode"`) {
			t.Fatalf("%s summary should retain background director schedule:\n%s", name, summary)
		}
		if !strings.Contains(summary, `"difficulty_guidance"`) || !strings.Contains(summary, `"state_effect_guidance"`) || !strings.Contains(summary, `"must_check_examples"`) || !strings.Contains(summary, `"skip_check_examples"`) || strings.Contains(summary, `"impact"`) {
			t.Fatalf("%s summary should expose natural-language rule guidance without legacy impact:\n%s", name, summary)
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
		ID:                    "legacy-style",
		Name:                  "旧风格",
		LegacyRandomEventRate: floatPtr(0.42),
		Orchestration:         legacyOrchestrationForTest(),
		Slots:                 []TellerPromptSlot{testTellerSlot()},
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
	if migrated.Name != "旧风格 故事导演" || migrated.Strategy.EventFrequency != EventFrequencyFrequent {
		t.Fatalf("unexpected migrated metadata: %#v", migrated)
	}
	if len(migrated.ModuleRefs.EventPackageIDs) == 0 || migrated.ModuleRefs.RuleSystemID == "" || migrated.ModuleRefs.ActorStateID == "" {
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
	if len(migrated.TRPGSystem.RuleTemplates) != 1 || migrated.TRPGSystem.RuleTemplates[0].Dice != "1d20" || migrated.TRPGSystem.RuleTemplates[0].Modifier != 0 {
		t.Fatalf("rule templates should be copied: %#v", migrated.TRPGSystem.RuleTemplates)
	}
	if len(migrated.ActorState.TraitPools) != 1 || migrated.ActorState.TraitPools[0].ID != "talent" {
		t.Fatalf("legacy traits should migrate into the actor state library: %#v", migrated.ActorState)
	}
	if migrated.ModuleRefs.OpeningSelectorID != "" {
		t.Fatalf("opening selector should migrate into actor state module instead of a standalone ref: %#v", migrated.ModuleRefs)
	}
	actorStateModule, err := NewActorStateLibrary(novaDir).Get(migrated.ModuleRefs.ActorStateID)
	if err != nil {
		t.Fatalf("migrated actor state module should be readable: %v", err)
	}
	if len(actorStateModule.ActorState.TraitPools) != 1 || actorStateModule.ActorState.TraitPools[0].ID != "talent" || len(actorStateModule.MigrationWarnings) == 0 {
		t.Fatalf("migrated actor state should own trait pools and warn about discarded ops: %#v", actorStateModule)
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
			}},
		}},
		RuleTemplates: []RuleCheck{{
			ID:                  "stealth",
			Label:               "潜行",
			Dice:                "1d20",
			FailurePolicy:       "blocked",
			DifficultyGuidance:  "守卫警觉时提高难度。",
			StateEffectGuidance: "失败可消耗体力并增加警戒。",
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

func containsStateOpPath(ops []StateOp, path string) bool {
	for _, op := range ops {
		if op.Path == path {
			return true
		}
	}
	return false
}

func containsActorStateOp(ops []ActorStateOp, actorID, fieldID string, value any) bool {
	for _, op := range ops {
		if op.ActorID == actorID && op.FieldID == fieldID && op.Value == value {
			return true
		}
	}
	return false
}
