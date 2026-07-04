package interactive

import (
	"strings"
	"testing"
)

func TestEventSystemLibraryMaterializesGenreBuiltins(t *testing.T) {
	library := NewEventSystemLibrary(t.TempDir())
	items, err := library.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	wantIDs := []string{
		DefaultEventSystemID,
		GenreXuanhuanEventSystemID,
		GenreXiuxianEventSystemID,
		GenreApocalypseEventSystemID,
		GenreWesternEventSystemID,
		GenreUrbanEventSystemID,
		GenreTRPGEventSystemID,
	}
	byID := map[string]EventSystemModule{}
	for _, item := range items {
		byID[item.ID] = item
	}
	for _, id := range wantIDs {
		item, ok := byID[id]
		if !ok {
			t.Fatalf("missing built-in event system %s in %#v", id, items)
		}
		if item.Custom || !IsBuiltinEventSystemID(id) {
			t.Fatalf("event system %s should be read-only built-in: %#v", id, item)
		}
		if len(item.EventSystem.EventPackages) != 1 || len(item.EventSystem.EventPackages[0].Events) == 0 {
			t.Fatalf("event system %s should include one non-empty event package: %#v", id, item.EventSystem.EventPackages)
		}
	}

	xiuxian, err := library.Get(GenreXiuxianEventSystemID)
	if err != nil {
		t.Fatalf("Get xiuxian preset failed: %v", err)
	}
	if xiuxian.EventSystem.EventPackages[0].ID != "xiuxian-core" || len(xiuxian.EventSystem.EventPackages[0].Events) != 8 {
		t.Fatalf("xiuxian event pack mismatch: %#v", xiuxian.EventSystem.EventPackages[0])
	}
	firstCard := xiuxian.EventSystem.EventPackages[0].Events[0]
	if !strings.Contains(firstCard.TypeName, "Bottleneck") || !strings.Contains(firstCard.DescriptionMarkdown, "Trigger Scene") {
		t.Fatalf("genre cards should include bilingual names and structured markdown: %#v", firstCard)
	}
	xiuxian.Name = "我的修仙事件系统"
	overridden, err := library.Update(GenreXiuxianEventSystemID, xiuxian, xiuxian.UpdatedAt)
	if err != nil {
		t.Fatalf("Update built-in genre event system should create override: %v", err)
	}
	if overridden.Custom || !overridden.BuiltinOverridden || overridden.ID != GenreXiuxianEventSystemID || overridden.Name != "我的修仙事件系统" {
		t.Fatalf("unexpected built-in event system override: %#v", overridden)
	}
	if err := library.Delete(GenreXiuxianEventSystemID); err != nil {
		t.Fatalf("Delete built-in event system override should restore builtin: %v", err)
	}
	restored, err := library.Get(GenreXiuxianEventSystemID)
	if err != nil {
		t.Fatalf("Get restored xiuxian preset failed: %v", err)
	}
	if restored.Custom || restored.BuiltinOverridden || restored.Name == "我的修仙事件系统" {
		t.Fatalf("unexpected restored built-in event system: %#v", restored)
	}
}

func TestDirectorModuleBuiltinOverridesRestore(t *testing.T) {
	novaDir := t.TempDir()
	ruleLibrary := NewRuleSystemLibrary(novaDir)
	rule, err := ruleLibrary.Get(DefaultRuleSystemID)
	if err != nil {
		t.Fatal(err)
	}
	rule.Name = "我的数值规则"
	overriddenRule, err := ruleLibrary.Update(DefaultRuleSystemID, rule, rule.UpdatedAt)
	if err != nil {
		t.Fatalf("Update built-in rule system should create override: %v", err)
	}
	if overriddenRule.Custom || !overriddenRule.BuiltinOverridden || overriddenRule.Name != "我的数值规则" {
		t.Fatalf("unexpected rule override: %#v", overriddenRule)
	}
	if err := ruleLibrary.Delete(DefaultRuleSystemID); err != nil {
		t.Fatalf("Delete rule override should restore builtin: %v", err)
	}
	restoredRule, err := ruleLibrary.Get(DefaultRuleSystemID)
	if err != nil {
		t.Fatal(err)
	}
	if restoredRule.Custom || restoredRule.BuiltinOverridden || restoredRule.Name == "我的数值规则" {
		t.Fatalf("unexpected restored rule system: %#v", restoredRule)
	}

	openingLibrary := NewOpeningSelectorLibrary(novaDir)
	opening, err := openingLibrary.Get(DefaultOpeningSelectorID)
	if err != nil {
		t.Fatal(err)
	}
	opening.Name = "我的开局选择"
	overriddenOpening, err := openingLibrary.Update(DefaultOpeningSelectorID, opening, opening.UpdatedAt)
	if err != nil {
		t.Fatalf("Update built-in opening selector should create override: %v", err)
	}
	if overriddenOpening.Custom || !overriddenOpening.BuiltinOverridden || overriddenOpening.Name != "我的开局选择" {
		t.Fatalf("unexpected opening override: %#v", overriddenOpening)
	}
	if err := openingLibrary.Delete(DefaultOpeningSelectorID); err != nil {
		t.Fatalf("Delete opening override should restore builtin: %v", err)
	}
	restoredOpening, err := openingLibrary.Get(DefaultOpeningSelectorID)
	if err != nil {
		t.Fatal(err)
	}
	if restoredOpening.Custom || restoredOpening.BuiltinOverridden || restoredOpening.Name == "我的开局选择" {
		t.Fatalf("unexpected restored opening selector: %#v", restoredOpening)
	}
}

func TestDirectorEventCatalogPrioritizesConfiguredEventCardsBeforeDefaults(t *testing.T) {
	module := builtinGenreEventSystem(
		"test-genre-events",
		"测试事件系统",
		"用于验证事件目录顺序。",
		nil,
		"test-pack",
		"测试事件包",
		urbanEventCards(),
	)
	director := normalizeStoryDirector(StoryDirector{
		ID:          "catalog-order",
		Name:        "目录顺序",
		ModuleRefs:  StoryDirectorModuleRefs{EventSystemDisabled: false},
		Strategy:    StoryDirectorStrategy{Enabled: true},
		EventSystem: module.EventSystem,
	})

	catalog := DirectorEventCatalogFromStoryDirector(director)
	packCards := module.EventSystem.EventPackages[0].Events
	if len(catalog) != maxTurnBriefListItems {
		t.Fatalf("catalog should still be filled to the bounded default size, got %d: %#v", len(catalog), catalog)
	}
	for i, card := range packCards {
		if catalog[i].ID != card.ID {
			t.Fatalf("configured event cards should be first, index %d got %s want %s in %#v", i, catalog[i].ID, card.ID, catalog)
		}
	}
	if !directorEventQueued(catalog, "face_slap") {
		t.Fatalf("default templates should fill remaining catalog slots: %#v", catalog)
	}
}

func TestStoryDirectorResolvesLiveModulesAndFallsBackToSnapshot(t *testing.T) {
	novaDir := t.TempDir()
	eventLibrary := NewEventSystemLibrary(novaDir)
	ruleLibrary := NewRuleSystemLibrary(novaDir)
	openingLibrary := NewOpeningSelectorLibrary(novaDir)
	directorLibrary := NewStoryDirectorLibrary(novaDir)

	eventModule, err := eventLibrary.Create(EventSystemModule{
		ID:   "storm-events",
		Name: "风暴事件",
		EventSystem: StoryDirectorEventSystem{CustomEvents: []DirectorEvent{{
			ID:      "storm",
			Name:    "风暴",
			Enabled: true,
			Summary: "v1",
		}}},
	})
	if err != nil {
		t.Fatalf("create event system failed: %v", err)
	}
	ruleModule, err := ruleLibrary.Create(RuleSystemModule{
		ID:   "survival-rules",
		Name: "生存规则",
		StatSystem: StoryDirectorStatSystem{Attributes: []StoryDirectorAttribute{{
			ID:         "heat",
			Path:       "resources.heat",
			Name:       "热量",
			Default:    1,
			Max:        5,
			Visibility: "visible",
		}}},
	})
	if err != nil {
		t.Fatalf("create rule system failed: %v", err)
	}
	openingModule, err := openingLibrary.Create(OpeningSelectorModule{
		ID:   "wasteland-openings",
		Name: "废土开局",
		OpeningSelector: StoryDirectorOpeningSelector{
			Enabled: true,
			InitialStateOps: []StateOp{{
				Op:    "set",
				Path:  "flags.wasteland",
				Value: true,
			}},
		},
	})
	if err != nil {
		t.Fatalf("create opening selector failed: %v", err)
	}

	director, err := directorLibrary.Create(StoryDirector{
		ID:   "modular",
		Name: "模块化导演",
		ModuleRefs: StoryDirectorModuleRefs{
			NarrativeStyleID:  "classic",
			EventSystemID:     eventModule.ID,
			RuleSystemID:      ruleModule.ID,
			OpeningSelectorID: openingModule.ID,
			ImagePresetID:     "game-cg",
		},
		Strategy: StoryDirectorStrategy{Enabled: true},
	})
	if err != nil {
		t.Fatalf("create story director failed: %v", err)
	}
	if len(director.EventSystem.CustomEvents) != 1 || director.EventSystem.CustomEvents[0].Summary != "v1" {
		t.Fatalf("director should resolve event module on create: %#v", director.EventSystem.CustomEvents)
	}
	if len(director.StatSystem.Attributes) != 1 || director.StatSystem.Attributes[0].Path != "resources.heat" {
		t.Fatalf("director should resolve rule module on create: %#v", director.StatSystem.Attributes)
	}
	if !containsStateOp(director.OpeningSelector.InitialStateOps, "flags.wasteland", true) {
		t.Fatalf("director should resolve opening module on create: %#v", director.OpeningSelector.InitialStateOps)
	}

	eventModule.EventSystem.CustomEvents[0].Summary = "v2"
	if _, err := eventLibrary.Update(eventModule.ID, eventModule, eventModule.UpdatedAt); err != nil {
		t.Fatalf("update event system failed: %v", err)
	}
	live, err := directorLibrary.Get("modular")
	if err != nil {
		t.Fatalf("get live director failed: %v", err)
	}
	if live.EventSystem.CustomEvents[0].Summary != "v2" {
		t.Fatalf("director should resolve latest module content, got %#v", live.EventSystem.CustomEvents[0])
	}

	if err := eventLibrary.Delete(eventModule.ID); err != nil {
		t.Fatalf("delete event system failed: %v", err)
	}
	fallback, err := directorLibrary.Get("modular")
	if err != nil {
		t.Fatalf("get fallback director failed: %v", err)
	}
	if fallback.EventSystem.CustomEvents[0].Summary != "v2" {
		t.Fatalf("director should use last resolved snapshot after module deletion, got %#v", fallback.EventSystem.CustomEvents[0])
	}
	if fallback.ResolvedSnapshot.Status != "warning" || len(fallback.ResolvedSnapshot.Warnings) == 0 {
		t.Fatalf("missing module should produce warning snapshot: %#v", fallback.ResolvedSnapshot)
	}
}

func TestStoryDirectorDisabledModulesStayDetached(t *testing.T) {
	novaDir := t.TempDir()
	library := NewStoryDirectorLibrary(novaDir)

	director, err := library.Create(StoryDirector{
		ID:   "detached",
		Name: "可关闭模块导演",
		ModuleRefs: StoryDirectorModuleRefs{
			NarrativeStyleID:        "missing-style",
			NarrativeStyleDisabled:  true,
			EventSystemID:           "missing-events",
			EventSystemDisabled:     true,
			RuleSystemID:            "missing-rules",
			RuleSystemDisabled:      true,
			OpeningSelectorID:       "missing-opening",
			OpeningSelectorDisabled: true,
			ImagePresetID:           "missing-image",
			ImagePresetDisabled:     true,
		},
		Strategy: StoryDirectorStrategy{Enabled: true},
		ResolvedSnapshot: StoryDirectorResolvedSnapshot{
			EventSystem: StoryDirectorEventSystem{CustomEvents: []DirectorEvent{{
				ID:      "snapshot-event",
				Name:    "旧快照事件",
				Enabled: true,
			}}},
			StatSystem: StoryDirectorStatSystem{Attributes: []StoryDirectorAttribute{{
				ID:         "snapshot-stat",
				Path:       "resources.snapshot",
				Name:       "旧快照属性",
				Visibility: "visible",
			}}},
			TRPGSystem: StoryDirectorTRPGSystem{RuleTemplates: []RuleCheck{{
				ID:         "snapshot-rule",
				Label:      "旧快照规则",
				Kind:       "dice",
				Mode:       "d20_dc",
				Dice:       "1d20",
				Difficulty: 10,
			}}},
			OpeningSelector: StoryDirectorOpeningSelector{
				Enabled: true,
				InitialStateOps: []StateOp{{
					Op:    "set",
					Path:  "flags.snapshot",
					Value: true,
				}},
			},
		},
	})
	if err != nil {
		t.Fatalf("create detached director failed: %v", err)
	}
	if !director.ModuleRefs.EventSystemDisabled || director.ModuleRefs.EventSystemID != "missing-events" {
		t.Fatalf("disabled event ref should be preserved: %#v", director.ModuleRefs)
	}
	if len(director.ResolvedSnapshot.Warnings) != 0 || director.ResolvedSnapshot.Status != "ready" {
		t.Fatalf("disabled missing modules should not warn: %#v", director.ResolvedSnapshot)
	}
	if len(director.EventSystem.CustomEvents) != 0 || len(director.EventSystem.EventPackages) != 0 {
		t.Fatalf("disabled event system should stay empty, got %#v", director.EventSystem)
	}
	if len(director.StatSystem.Attributes) != 0 || len(director.TRPGSystem.RuleTemplates) != 0 {
		t.Fatalf("disabled rule system should not use defaults or snapshot, got stats=%#v trpg=%#v", director.StatSystem, director.TRPGSystem)
	}
	if director.OpeningSelector.Enabled || len(director.OpeningSelector.InitialStateOps) != 0 || len(director.OpeningSelector.TraitPools) != 0 {
		t.Fatalf("disabled opening selector should stay off, got %#v", director.OpeningSelector)
	}
	if len(StoryDirectorInitialStateOps(director)) != 0 {
		t.Fatalf("disabled rule/opening modules should not generate initial state ops")
	}
	if events := DirectorEventCatalogFromStoryDirector(director); len(events) != 0 {
		t.Fatalf("disabled event system should not expose default event catalog: %#v", events)
	}
}
