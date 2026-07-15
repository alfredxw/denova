package interactive

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestStoryDirectorLibraryCRUDAndRevisionConflict(t *testing.T) {
	novaDir := t.TempDir()
	library := NewStoryDirectorLibrary(novaDir)

	directors, err := library.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(directors) == 0 || directors[0].ID != DefaultStoryDirectorID || directors[0].Custom {
		t.Fatalf("default story director should be materialized first: %#v", directors)
	}
	if directors[0].ModuleRefs.NarrativeStyleDisabled || directors[0].ModuleRefs.EventPackagesDisabled || directors[0].ModuleRefs.RuleSystemDisabled || directors[0].ModuleRefs.ActorStateDisabled || directors[0].ModuleRefs.ImagePresetDisabled {
		t.Fatalf("default story director modules should start enabled: %#v", directors[0].ModuleRefs)
	}
	if directors[0].Strategy.DirectorAgentMode != DirectorAgentModeTriggered || directors[0].Strategy.StateSchemaAdaptationMode != StateSchemaAdaptationModeAfterOpening || directors[0].Strategy.BranchPlanningTurns != defaultBranchPlanningTurns {
		t.Fatalf("default story director should use triggered background director schedule: %#v", directors[0].Strategy)
	}
	if normalized := NormalizeStoryDirectorStrategy(StoryDirectorStrategy{StateSchemaAdaptationMode: StateSchemaAdaptationModeOff}); normalized.StateSchemaAdaptationMode != StateSchemaAdaptationModeOff {
		t.Fatalf("story director should preserve disabled state schema adaptation: %#v", normalized)
	}

	actorStateModule, err := NewActorStateLibrary(novaDir).Create(ActorStateModule{
		ID:   "custom-state",
		Name: "自定义状态系统",
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
	})
	if err != nil {
		t.Fatalf("Create actor state failed: %v", err)
	}
	ruleModule, err := NewRuleSystemLibrary(novaDir).Create(RuleSystemModule{
		ID:   "custom-rules",
		Name: "自定义规则",
		TRPGSystem: StoryDirectorTRPGSystem{RuleTemplates: []RuleCheck{{
			ID:                  "luck",
			Label:               "幸运",
			Dice:                "1d20",
			Modifier:            10,
			FailurePolicy:       "success_at_cost",
			DifficultyGuidance:  "幸运耗尽时提高难度。",
			StateEffectGuidance: "成功可增加机会，失败可消耗资源。",
		}}},
	})
	if err != nil {
		t.Fatalf("Create rule system failed: %v", err)
	}

	created, err := library.Create(StoryDirector{
		ID:          "custom-director",
		Name:        "自定义导演",
		Description: "用于测试",
		ModuleRefs: StoryDirectorModuleRefs{
			ActorStateID: actorStateModule.ID,
			RuleSystemID: ruleModule.ID,
		},
		Strategy: StoryDirectorStrategy{
			Enabled:             true,
			EventFrequency:      EventFrequencyFrequent,
			DirectorAgentMode:   "unknown",
			BranchPlanningTurns: 99,
		},
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if !created.Custom || created.Strategy.EventFrequency != EventFrequencyFrequent || created.Strategy.DirectorAgentMode != DirectorAgentModeTriggered || created.Strategy.BranchPlanningTurns != 12 {
		t.Fatalf("custom director should be marked and strategy should be normalized: %#v", created)
	}
	if created.ModuleRefs.EventPackagesDisabled || created.ModuleRefs.RuleSystemDisabled {
		t.Fatalf("director modules should remain enabled: %#v", created.ModuleRefs)
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
		Name:       "Agent 更新",
		ModuleRefs: created.ModuleRefs,
		Strategy:   StoryDirectorStrategy{Enabled: true},
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
