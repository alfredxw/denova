package interactive

import (
	"strings"
	"testing"
)

func TestEventPackageLibraryMaterializesGenreBuiltins(t *testing.T) {
	library := NewEventPackageLibrary(t.TempDir())
	items, err := library.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	wantIDs := []string{
		DefaultEventPackageID,
		GenreXuanhuanEventPackageID,
		GenreXiuxianEventPackageID,
		GenreApocalypseEventPackageID,
		GenreWesternEventPackageID,
		GenreUrbanEventPackageID,
		GenreTRPGEventPackageID,
	}
	byID := map[string]EventPackageModule{}
	for _, item := range items {
		byID[item.ID] = item
	}
	for _, id := range wantIDs {
		item, ok := byID[id]
		if !ok {
			t.Fatalf("missing built-in event package %s in %#v", id, items)
		}
		if item.Custom || !IsBuiltinEventPackageID(id) {
			t.Fatalf("event package %s should be read-only built-in: %#v", id, item)
		}
		if len(item.Events) == 0 {
			t.Fatalf("event package %s should include non-empty event cards: %#v", id, item.Events)
		}
	}

	xiuxian, err := library.Get(GenreXiuxianEventPackageID)
	if err != nil {
		t.Fatalf("Get xiuxian preset failed: %v", err)
	}
	if xiuxian.ID != GenreXiuxianEventPackageID || len(xiuxian.Events) != 8 {
		t.Fatalf("xiuxian event package mismatch: %#v", xiuxian)
	}
	if xiuxian.Name != "修仙核心事件包" {
		t.Fatalf("genre package name should default to Chinese only: %#v", xiuxian)
	}
	firstCard := xiuxian.Events[0]
	if firstCard.TypeName != "瓶颈突破" || !strings.Contains(firstCard.DescriptionMarkdown, "## 触发场景") || strings.Contains(firstCard.DescriptionMarkdown, "Trigger Scene") {
		t.Fatalf("genre cards should default to Chinese names and structured markdown: %#v", firstCard)
	}
	xiuxian.Name = "我的修仙事件包"
	overridden, err := library.Update(GenreXiuxianEventPackageID, xiuxian, xiuxian.UpdatedAt)
	if err != nil {
		t.Fatalf("Update built-in genre event package should create override: %v", err)
	}
	if overridden.Custom || !overridden.BuiltinOverridden || overridden.ID != GenreXiuxianEventPackageID || overridden.Name != "我的修仙事件包" {
		t.Fatalf("unexpected built-in event package override: %#v", overridden)
	}
	if err := library.Delete(GenreXiuxianEventPackageID); err != nil {
		t.Fatalf("Delete built-in event package override should restore builtin: %v", err)
	}
	restored, err := library.Get(GenreXiuxianEventPackageID)
	if err != nil {
		t.Fatalf("Get restored xiuxian preset failed: %v", err)
	}
	if restored.Custom || restored.BuiltinOverridden || restored.Name == "我的修仙事件包" {
		t.Fatalf("unexpected restored built-in event package: %#v", restored)
	}
}

func TestDirectorModuleBuiltinOverridesRestore(t *testing.T) {
	novaDir := t.TempDir()
	ruleLibrary := NewRuleSystemLibrary(novaDir)
	rule, err := ruleLibrary.Get(DefaultRuleSystemID)
	if err != nil {
		t.Fatal(err)
	}
	rule.Name = "我的 TRPG 检定"
	overriddenRule, err := ruleLibrary.Update(DefaultRuleSystemID, rule, rule.UpdatedAt)
	if err != nil {
		t.Fatalf("Update built-in rule system should create override: %v", err)
	}
	if overriddenRule.Custom || !overriddenRule.BuiltinOverridden || overriddenRule.Name != "我的 TRPG 检定" {
		t.Fatalf("unexpected rule override: %#v", overriddenRule)
	}
	if err := ruleLibrary.Delete(DefaultRuleSystemID); err != nil {
		t.Fatalf("Delete rule override should restore builtin: %v", err)
	}
	restoredRule, err := ruleLibrary.Get(DefaultRuleSystemID)
	if err != nil {
		t.Fatal(err)
	}
	if restoredRule.Custom || restoredRule.BuiltinOverridden || restoredRule.Name == "我的 TRPG 检定" {
		t.Fatalf("unexpected restored rule system: %#v", restoredRule)
	}

	actorLibrary := NewActorStateLibrary(novaDir)
	actorState, err := actorLibrary.Get(DefaultActorStateModuleID)
	if err != nil {
		t.Fatal(err)
	}
	actorState.Name = "我的状态系统"
	overriddenActorState, err := actorLibrary.Update(DefaultActorStateModuleID, actorState, actorState.UpdatedAt)
	if err != nil {
		t.Fatalf("Update built-in actor state should create override: %v", err)
	}
	if overriddenActorState.Custom || !overriddenActorState.BuiltinOverridden || overriddenActorState.Name != "我的状态系统" {
		t.Fatalf("unexpected actor state override: %#v", overriddenActorState)
	}
	if err := actorLibrary.Delete(DefaultActorStateModuleID); err != nil {
		t.Fatalf("Delete actor state override should restore builtin: %v", err)
	}
	restoredActorState, err := actorLibrary.Get(DefaultActorStateModuleID)
	if err != nil {
		t.Fatal(err)
	}
	if restoredActorState.Custom || restoredActorState.BuiltinOverridden || restoredActorState.Name == "我的状态系统" {
		t.Fatalf("unexpected restored actor state: %#v", restoredActorState)
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

	memoryLibrary := NewStoryMemoryStructureLibrary(novaDir)
	memory, err := memoryLibrary.Get(DefaultStoryMemoryStructureModuleID)
	if err != nil {
		t.Fatal(err)
	}
	memory.Name = "我的记忆结构"
	overriddenMemory, err := memoryLibrary.Update(DefaultStoryMemoryStructureModuleID, memory, memory.UpdatedAt)
	if err != nil {
		t.Fatalf("Update built-in memory structure should create override: %v", err)
	}
	if overriddenMemory.Custom || !overriddenMemory.BuiltinOverridden || overriddenMemory.Name != "我的记忆结构" {
		t.Fatalf("unexpected memory structure override: %#v", overriddenMemory)
	}
	if err := memoryLibrary.Delete(DefaultStoryMemoryStructureModuleID); err != nil {
		t.Fatalf("Delete memory structure override should restore builtin: %v", err)
	}
	restoredMemory, err := memoryLibrary.Get(DefaultStoryMemoryStructureModuleID)
	if err != nil {
		t.Fatal(err)
	}
	if restoredMemory.Custom || restoredMemory.BuiltinOverridden || restoredMemory.Name == "我的记忆结构" {
		t.Fatalf("unexpected restored memory structure: %#v", restoredMemory)
	}
}

func TestDirectorEventCatalogPrioritizesConfiguredEventCardsBeforeDefaults(t *testing.T) {
	module := builtinGenreEventPackageModule(
		"test-pack",
		"测试事件包",
		"用于验证事件目录顺序。",
		nil,
		urbanEventCards(),
	)
	director := normalizeStoryDirector(StoryDirector{
		ID:            "catalog-order",
		Name:          "目录顺序",
		ModuleRefs:    StoryDirectorModuleRefs{EventPackagesDisabled: false},
		Strategy:      StoryDirectorStrategy{Enabled: true},
		EventPackages: []TellerEventPackage{tellerEventPackageFromModule(module)},
	})

	catalog := DirectorEventCatalogFromStoryDirector(director)
	packCards := module.Events
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
	eventLibrary := NewEventPackageLibrary(novaDir)
	ruleLibrary := NewRuleSystemLibrary(novaDir)
	actorStateLibrary := NewActorStateLibrary(novaDir)
	memoryLibrary := NewStoryMemoryStructureLibrary(novaDir)
	openingLibrary := NewOpeningSelectorLibrary(novaDir)
	directorLibrary := NewStoryDirectorLibrary(novaDir)

	eventModule, err := eventLibrary.Create(EventPackageModule{
		ID:   "storm-events",
		Name: "风暴事件包",
		Events: []TellerEventCard{{
			ID:                  "storm",
			TypeName:            "风暴",
			Enabled:             true,
			DescriptionMarkdown: "v1",
		}},
	})
	if err != nil {
		t.Fatalf("create event package failed: %v", err)
	}
	ruleModule, err := ruleLibrary.Create(RuleSystemModule{
		ID:   "survival-rules",
		Name: "生存 TRPG 检定",
		TRPGSystem: StoryDirectorTRPGSystem{RuleTemplates: []RuleCheck{{
			ID:         "heat-check",
			Label:      "耐热检定",
			Kind:       "dice",
			Mode:       "d20_dc",
			Dice:       "1d20",
			Difficulty: 12,
		}}},
	})
	if err != nil {
		t.Fatalf("create rule system failed: %v", err)
	}
	actorModule, err := actorStateLibrary.Create(ActorStateModule{
		ID:   "survival-actors",
		Name: "生存 Actor 状态",
		ActorState: StoryDirectorActorStateSystem{
			Templates: []ActorStateTemplate{{
				ID:   "protagonist",
				Name: "主角",
				Fields: []ActorStateField{{
					ID:         "heat",
					Path:       "resources.heat",
					Name:       "热量",
					Type:       "number",
					Default:    float64(1),
					Visibility: "visible",
				}},
			}},
			InitialActors: []ActorStateInitialActor{{
				ID:         DefaultActorID,
				Name:       "主角",
				TemplateID: "protagonist",
				Role:       "protagonist",
			}},
		},
	})
	if err != nil {
		t.Fatalf("create actor state failed: %v", err)
	}
	memoryModule, err := memoryLibrary.Create(StoryMemoryStructureModule{
		ID:   "survival-memory",
		Name: "生存记忆结构",
		Structures: []StoryMemoryStructure{{
			ID:      "camp",
			Name:    "营地状态",
			Mode:    "singleton",
			Enabled: boolPtr(true),
			Fields:  []StoryMemoryField{{ID: "status", Name: "状态", Order: 10}},
		}},
	})
	if err != nil {
		t.Fatalf("create memory structure failed: %v", err)
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
			EventPackageIDs:   []string{eventModule.ID},
			RuleSystemID:      ruleModule.ID,
			ActorStateID:      actorModule.ID,
			MemoryStructureID: memoryModule.ID,
			OpeningSelectorID: openingModule.ID,
			ImagePresetID:     "game-cg",
		},
		Strategy: StoryDirectorStrategy{Enabled: true},
	})
	if err != nil {
		t.Fatalf("create story director failed: %v", err)
	}
	if len(director.EventPackages) != 1 || len(director.EventPackages[0].Events) != 1 || director.EventPackages[0].Events[0].DescriptionMarkdown != "v1" {
		t.Fatalf("director should resolve event package on create: %#v", director.EventPackages)
	}
	if len(director.TRPGSystem.RuleTemplates) != 1 || director.TRPGSystem.RuleTemplates[0].ID != "heat-check" {
		t.Fatalf("director should resolve TRPG module on create: %#v", director.TRPGSystem.RuleTemplates)
	}
	if len(director.ActorState.Templates) != 1 || director.ActorState.Templates[0].ID != "protagonist" || len(director.ActorState.InitialActors) != 1 {
		t.Fatalf("director should resolve actor state module on create: %#v", director.ActorState)
	}
	if len(director.ResolvedSnapshot.StoryMemoryStructures) != 1 || director.ResolvedSnapshot.StoryMemoryStructures[0].ID != "camp" {
		t.Fatalf("director should resolve memory structure module on create: %#v", director.ResolvedSnapshot.StoryMemoryStructures)
	}
	if !containsStateOp(director.OpeningSelector.InitialStateOps, "flags.wasteland", true) {
		t.Fatalf("director should resolve opening module on create: %#v", director.OpeningSelector.InitialStateOps)
	}

	eventModule.Events[0].DescriptionMarkdown = "v2"
	if _, err := eventLibrary.Update(eventModule.ID, eventModule, eventModule.UpdatedAt); err != nil {
		t.Fatalf("update event package failed: %v", err)
	}
	live, err := directorLibrary.Get("modular")
	if err != nil {
		t.Fatalf("get live director failed: %v", err)
	}
	if live.EventPackages[0].Events[0].DescriptionMarkdown != "v2" {
		t.Fatalf("director should resolve latest module content, got %#v", live.EventPackages[0].Events[0])
	}

	if err := eventLibrary.Delete(eventModule.ID); err != nil {
		t.Fatalf("delete event package failed: %v", err)
	}
	if err := actorStateLibrary.Delete(actorModule.ID); err != nil {
		t.Fatalf("delete actor state failed: %v", err)
	}
	if err := memoryLibrary.Delete(memoryModule.ID); err != nil {
		t.Fatalf("delete memory structure failed: %v", err)
	}
	fallback, err := directorLibrary.Get("modular")
	if err != nil {
		t.Fatalf("get fallback director failed: %v", err)
	}
	if fallback.EventPackages[0].Events[0].DescriptionMarkdown != "v2" {
		t.Fatalf("director should use last resolved snapshot after module deletion, got %#v", fallback.EventPackages[0].Events[0])
	}
	if len(fallback.ActorState.Templates) != 1 || fallback.ActorState.Templates[0].ID != "protagonist" {
		t.Fatalf("director should use actor state snapshot after module deletion, got %#v", fallback.ActorState)
	}
	if len(fallback.ResolvedSnapshot.StoryMemoryStructures) != 1 || fallback.ResolvedSnapshot.StoryMemoryStructures[0].ID != "camp" {
		t.Fatalf("director should use memory structure snapshot after module deletion, got %#v", fallback.ResolvedSnapshot.StoryMemoryStructures)
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
			EventPackageIDs:         []string{"missing-events"},
			EventPackagesDisabled:   true,
			RuleSystemID:            "missing-rules",
			RuleSystemDisabled:      true,
			ActorStateID:            "missing-actors",
			ActorStateDisabled:      true,
			MemoryStructureID:       "missing-memory",
			MemoryStructureDisabled: true,
			OpeningSelectorID:       "missing-opening",
			OpeningSelectorDisabled: true,
			ImagePresetID:           "missing-image",
			ImagePresetDisabled:     true,
		},
		Strategy: StoryDirectorStrategy{Enabled: true},
		ResolvedSnapshot: StoryDirectorResolvedSnapshot{
			EventPackages: []TellerEventPackage{{
				ID:      "snapshot-pack",
				Name:    "旧快照包",
				Enabled: true,
				Events: []TellerEventCard{{
					ID:       "snapshot-event",
					TypeName: "旧快照事件",
					Enabled:  true,
				}},
			}},
			TRPGSystem: StoryDirectorTRPGSystem{RuleTemplates: []RuleCheck{{
				ID:         "snapshot-rule",
				Label:      "旧快照规则",
				Kind:       "dice",
				Mode:       "d20_dc",
				Dice:       "1d20",
				Difficulty: 10,
			}}},
			ActorState: StoryDirectorActorStateSystem{
				Templates: []ActorStateTemplate{{ID: "snapshot-template", Name: "旧状态模板"}},
			},
			StoryMemoryStructures: []StoryMemoryStructure{{
				ID:      "snapshot-memory",
				Name:    "旧记忆结构",
				Mode:    "append",
				Enabled: boolPtr(true),
				Fields:  []StoryMemoryField{{ID: "value", Name: "内容", Order: 10}},
			}},
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
	if !director.ModuleRefs.EventPackagesDisabled || len(director.ModuleRefs.EventPackageIDs) != 1 || director.ModuleRefs.EventPackageIDs[0] != "missing-events" {
		t.Fatalf("disabled event ref should be preserved: %#v", director.ModuleRefs)
	}
	if len(director.ResolvedSnapshot.Warnings) != 0 || director.ResolvedSnapshot.Status != "ready" {
		t.Fatalf("disabled missing modules should not warn: %#v", director.ResolvedSnapshot)
	}
	if len(director.EventPackages) != 0 {
		t.Fatalf("disabled event packages should stay empty, got %#v", director.EventPackages)
	}
	if len(director.TRPGSystem.RuleTemplates) != 0 {
		t.Fatalf("disabled TRPG checks should not use defaults or snapshot, got %#v", director.TRPGSystem)
	}
	if len(director.ActorState.Templates) != 0 || len(director.ActorState.InitialActors) != 0 {
		t.Fatalf("disabled actor state should not use defaults or snapshot, got %#v", director.ActorState)
	}
	if len(director.ResolvedSnapshot.StoryMemoryStructures) != 0 || StoryDirectorMemoryStructureEnabled(director) {
		t.Fatalf("disabled memory structure should stay detached, got refs=%#v snapshot=%#v", director.ModuleRefs, director.ResolvedSnapshot.StoryMemoryStructures)
	}
	if director.OpeningSelector.Enabled || len(director.OpeningSelector.InitialStateOps) != 0 || len(director.OpeningSelector.TraitPools) != 0 {
		t.Fatalf("disabled opening selector should stay off, got %#v", director.OpeningSelector)
	}
	if len(StoryDirectorInitialStateOps(director)) != 0 {
		t.Fatalf("disabled rule/opening modules should not generate initial state ops")
	}
	if events := DirectorEventCatalogFromStoryDirector(director); len(events) != 0 {
		t.Fatalf("disabled event packages should not expose default event catalog: %#v", events)
	}
}
