package interactive

import (
	"os"
	"path/filepath"
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

func TestActorStateLibraryMaterializesGenreBuiltins(t *testing.T) {
	novaDir := t.TempDir()
	library := NewActorStateLibrary(novaDir)
	items, err := library.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	wantIDs := []string{
		DefaultActorStateModuleID,
		ActorStateXiuxianID,
		ActorStateWesternFantasyID,
		ActorStateApocalypseID,
		ActorStateInfiniteFlowID,
	}
	byID := map[string]ActorStateModule{}
	for _, item := range items {
		byID[item.ID] = item
	}
	for index, id := range wantIDs {
		item, ok := byID[id]
		if !ok {
			t.Fatalf("missing built-in actor state %s in %#v", id, items)
		}
		if item.Custom || !IsBuiltinActorStateID(id) {
			t.Fatalf("actor state %s should be built-in: %#v", id, item)
		}
		if items[index].ID != id {
			t.Fatalf("built-in actor state order mismatch at %d: got %s want %s; items=%#v", index, items[index].ID, id, items)
		}
		requireActorStateTemplates(t, item, "protagonist", ActorStateStoryContextTemplateID, ActorStateImportantCharacterTemplateID, ActorStateOpponentTemplateID)
		if len(item.ActorState.InitialActors) != 2 ||
			item.ActorState.InitialActors[0].ID != DefaultActorID ||
			item.ActorState.InitialActors[0].TemplateID != "protagonist" ||
			item.ActorState.InitialActors[1].ID != DefaultStoryContextActorID ||
			item.ActorState.InitialActors[1].TemplateID != ActorStateStoryContextTemplateID {
			t.Fatalf("actor state %s should ship starter protagonist and story context state objects: %#v", id, item.ActorState.InitialActors)
		}
		requireNoActorStateFieldBounds(t, item)
	}

	defaultActorState, err := library.Get(DefaultActorStateModuleID)
	if err != nil {
		t.Fatalf("Get default actor state failed: %v", err)
	}
	if !actorStateTemplateHasField(defaultActorState, "protagonist", "current.body_status") ||
		!actorStateTemplateHasField(defaultActorState, ActorStateStoryContextTemplateID, "scene.current_event") ||
		!actorStateTemplateHasField(defaultActorState, ActorStateImportantCharacterTemplateID, "relationship.attitude_to_protagonist") ||
		!actorStateTemplateHasField(defaultActorState, ActorStateOpponentTemplateID, "threat.status") {
		t.Fatalf("default actor state should expose generic protagonist, story-context, important-character, and opponent fields: %#v", defaultActorState.ActorState.Templates)
	}

	xiuxian, err := library.Get(ActorStateXiuxianID)
	if err != nil {
		t.Fatalf("Get xiuxian preset failed: %v", err)
	}
	if xiuxian.Name != "修仙状态系统" || !actorStateTemplateHasField(xiuxian, "protagonist", "cultivation.realm") {
		t.Fatalf("xiuxian actor state should expose cultivation protagonist fields: %#v", xiuxian)
	}
	xiuxian.Name = "我的修仙状态系统"
	overridden, err := library.Update(ActorStateXiuxianID, xiuxian, xiuxian.UpdatedAt)
	if err != nil {
		t.Fatalf("Update built-in xiuxian actor state should create override: %v", err)
	}
	if overridden.Custom || !overridden.BuiltinOverridden || overridden.Name != "我的修仙状态系统" {
		t.Fatalf("unexpected xiuxian actor state override: %#v", overridden)
	}
	if err := library.Delete(ActorStateXiuxianID); err != nil {
		t.Fatalf("Delete built-in xiuxian actor state should restore builtin: %v", err)
	}
	restored, err := library.Get(ActorStateXiuxianID)
	if err != nil {
		t.Fatalf("Get restored xiuxian preset failed: %v", err)
	}
	if restored.Custom || restored.BuiltinOverridden || restored.Name == "我的修仙状态系统" || !actorStateTemplateHasField(restored, ActorStateOpponentTemplateID, "cultivation.realm_pressure") {
		t.Fatalf("unexpected restored xiuxian actor state: %#v", restored)
	}

	resolved := ResolveStoryDirectorModules(novaDir, StoryDirector{
		ID:   "genre-director",
		Name: "题材导演",
		ModuleRefs: StoryDirectorModuleRefs{
			NarrativeStyleDisabled:  true,
			EventPackagesDisabled:   true,
			RuleSystemDisabled:      true,
			ActorStateID:            ActorStateInfiniteFlowID,
			MemoryStructureDisabled: true,
			OpeningSelectorDisabled: true,
			ImagePresetDisabled:     true,
		},
	})
	if !actorStateTemplateHasField(ActorStateModule{ActorState: resolved.ActorState}, ActorStateOpponentTemplateID, "rules.triggers") {
		t.Fatalf("director should resolve infinite-flow actor state templates: %#v", resolved.ActorState)
	}
}

func TestActorStateModuleMigratesOpeningSelectorIntoTraitLibrary(t *testing.T) {
	novaDir := t.TempDir()
	actorStateLibrary := NewActorStateLibrary(novaDir)
	module, err := actorStateLibrary.Create(ActorStateModule{
		ID:          "state-with-opening",
		Name:        "带开局词条的状态系统",
		Description: "验证开局词条归属状态系统。",
		ActorState:  defaultActorStateSystem(),
		OpeningSelector: StoryDirectorOpeningSelector{
			Enabled: true,
			InitialStateOps: []StateOp{{
				Op:    "set",
				Path:  "rules.opening_origin",
				Value: "state-system",
			}},
			TraitPools: []OpeningTraitPool{{
				ID:        "talent",
				Name:      "天赋",
				DrawCount: 1,
				Traits: []OpeningTrait{{
					ID:      "clear-mind",
					Name:    "澄心",
					Summary: "开局精神状态更稳定。",
					Ops: []StateOp{{
						Op:    "set",
						Path:  "actors.protagonist.state.current.mental_status",
						Value: "澄明稳定",
					}},
				}},
			}},
		},
	})
	if err != nil {
		t.Fatalf("create actor state with opening failed: %v", err)
	}
	if len(module.OpeningSelector.TraitPools) != 0 || len(module.OpeningSelector.InitialStateOps) != 0 {
		t.Fatalf("opening selector should be cleared after migration: %#v", module.OpeningSelector)
	}
	if len(module.ActorState.TraitPools) != 1 || module.ActorState.TraitPools[0].ID != "talent" {
		t.Fatalf("legacy traits should migrate into actor state trait pools: %#v", module.ActorState.TraitPools)
	}
	protagonist := actorStateTemplateByID(module.ActorState, DefaultActorID)
	if len(protagonist.TraitRules) != 1 || protagonist.TraitRules[0].PoolID != "talent" || protagonist.TraitRules[0].DrawCount != 1 {
		t.Fatalf("legacy draw count should become protagonist template rule: %#v", protagonist.TraitRules)
	}
	if len(module.MigrationWarnings) < 2 {
		t.Fatalf("discarded legacy StateOps should be reported: %#v", module.MigrationWarnings)
	}

	director := ResolveStoryDirectorModules(novaDir, StoryDirector{
		ID:   "opening-from-state",
		Name: "开局归属状态系统",
		ModuleRefs: StoryDirectorModuleRefs{
			NarrativeStyleDisabled:  true,
			EventPackagesDisabled:   true,
			RuleSystemDisabled:      true,
			ActorStateID:            module.ID,
			MemoryStructureDisabled: true,
			ImagePresetDisabled:     true,
		},
	})
	if len(director.ActorState.TraitPools) != 1 || director.ActorState.TraitPools[0].ID != "talent" {
		t.Fatalf("director should resolve the trait library from actor state module: %#v", director.ActorState)
	}
	if director.ModuleRefs.OpeningSelectorID != "" {
		t.Fatalf("new director refs should not need opening_selector_id: %#v", director.ModuleRefs)
	}
	roll, err := RollActorTraits(director.ActorState, ActorTraitRollRequest{
		ActorID:    DefaultActorID,
		TemplateID: DefaultActorID,
		Seed:       7,
	})
	if err != nil {
		t.Fatalf("roll actor traits failed: %v", err)
	}
	if len(roll.Traits) != 1 || roll.Traits[0].TraitID != "clear-mind" {
		t.Fatalf("actor roll should use state-system traits: %#v", roll.Traits)
	}
	if containsStateOp(StoryDirectorInitialStateOps(director), "rules.opening_origin", "state-system") || containsStateOp(StoryDirectorInitialStateOps(director), "actors.protagonist.state.current.mental_status", "澄明稳定") {
		t.Fatalf("legacy arbitrary StateOps must not execute: %#v", StoryDirectorInitialStateOps(director))
	}
}

func TestDirectorModuleBuiltinOverridesRestore(t *testing.T) {
	novaDir := t.TempDir()
	ruleLibrary := NewRuleSystemLibrary(novaDir)
	rule, err := ruleLibrary.Get(DefaultRuleSystemID)
	if err != nil {
		t.Fatal(err)
	}
	ruleSystems, err := ruleLibrary.List()
	if err != nil {
		t.Fatalf("List built-in rule systems failed: %v", err)
	}
	if len(ruleSystems) < 7 {
		t.Fatalf("expected multiple built-in DM style rule systems, got %#v", ruleSystems)
	}
	for _, item := range ruleSystems {
		if IsBuiltinRuleSystemID(item.ID) && (item.Custom || item.BuiltinOverridden || len(item.TRPGSystem.RuleTemplates) != 1) {
			t.Fatalf("built-in rule system should be a single non-overridden config: %#v", item)
		}
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
	styleRule, err := ruleLibrary.Get(RuleSystemOSRPlayerSkillID)
	if err != nil {
		t.Fatal(err)
	}
	styleRule.Name = "我的 OSR 检定"
	overriddenStyleRule, err := ruleLibrary.Update(RuleSystemOSRPlayerSkillID, styleRule, styleRule.UpdatedAt)
	if err != nil {
		t.Fatalf("Update built-in style rule system should create override: %v", err)
	}
	if overriddenStyleRule.Custom || !overriddenStyleRule.BuiltinOverridden || overriddenStyleRule.Name != "我的 OSR 检定" {
		t.Fatalf("unexpected style rule override: %#v", overriddenStyleRule)
	}
	if err := ruleLibrary.Delete(RuleSystemOSRPlayerSkillID); err != nil {
		t.Fatalf("Delete style rule override should restore builtin: %v", err)
	}
	restoredStyleRule, err := ruleLibrary.Get(RuleSystemOSRPlayerSkillID)
	if err != nil {
		t.Fatal(err)
	}
	if restoredStyleRule.Custom || restoredStyleRule.BuiltinOverridden || restoredStyleRule.Name == "我的 OSR 检定" || len(restoredStyleRule.TRPGSystem.RuleTemplates) != 1 {
		t.Fatalf("unexpected restored style rule system: %#v", restoredStyleRule)
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

func TestStoryMemoryStructureBuiltinRefreshesWhenNotOverridden(t *testing.T) {
	novaDir := t.TempDir()
	library := NewStoryMemoryStructureLibrary(novaDir)
	if err := os.MkdirAll(library.dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(library.dir(), DefaultStoryMemoryStructureModuleID+".json")
	stale := DefaultStoryMemoryStructureModule()
	for i := range stale.Structures {
		if stale.Structures[i].ID == "current_state" {
			stale.Structures[i].Description = "旧版当前状态说明"
			stale.Structures[i].Enabled = boolPtr(true)
			break
		}
	}
	if err := writeStoryMemoryStructureFile(path, stale); err != nil {
		t.Fatal(err)
	}
	refreshed, err := library.Get(DefaultStoryMemoryStructureModuleID)
	if err != nil {
		t.Fatal(err)
	}
	currentState := storyMemoryStructureByID(refreshed.Structures, "current_state")
	if strings.Contains(currentState.Description, "旧版当前状态说明") || storyMemoryStructureEnabled(currentState) {
		t.Fatalf("non-overridden built-in memory structure should refresh to current defaults: %#v", currentState)
	}

	overridden := DefaultStoryMemoryStructureModule()
	overridden.BuiltinOverridden = true
	overridden.Name = "用户覆盖的默认记忆结构"
	for i := range overridden.Structures {
		if overridden.Structures[i].ID == "current_state" {
			overridden.Structures[i].Description = "用户覆盖当前状态说明"
			overridden.Structures[i].Enabled = boolPtr(true)
			break
		}
	}
	if err := writeStoryMemoryStructureFile(path, overridden); err != nil {
		t.Fatal(err)
	}
	kept, err := library.Get(DefaultStoryMemoryStructureModuleID)
	if err != nil {
		t.Fatal(err)
	}
	currentState = storyMemoryStructureByID(kept.Structures, "current_state")
	if kept.Name != "用户覆盖的默认记忆结构" || !strings.Contains(currentState.Description, "用户覆盖当前状态说明") || !storyMemoryStructureEnabled(currentState) {
		t.Fatalf("overridden built-in memory structure should be preserved: module=%#v current_state=%#v", kept, currentState)
	}
}

func requireActorStateTemplates(t *testing.T, item ActorStateModule, ids ...string) {
	t.Helper()
	templates := map[string]bool{}
	for _, template := range item.ActorState.Templates {
		templates[template.ID] = true
	}
	for _, id := range ids {
		if !templates[id] {
			t.Fatalf("actor state %s missing template %s: %#v", item.ID, id, item.ActorState.Templates)
		}
	}
}

func requireNoActorStateFieldBounds(t *testing.T, item ActorStateModule) {
	t.Helper()
	for _, template := range item.ActorState.Templates {
		for _, field := range template.Fields {
			if field.Min != nil || field.Max != nil {
				t.Fatalf("genre actor state %s field %s should not define min/max: %#v", item.ID, field.Path, field)
			}
		}
	}
}

func actorStateTemplateHasField(item ActorStateModule, templateID, fieldPath string) bool {
	for _, template := range item.ActorState.Templates {
		if template.ID != templateID {
			continue
		}
		for _, field := range template.Fields {
			if field.Path == fieldPath {
				return true
			}
		}
	}
	return false
}

func TestParseLegacyRuleSystemKeepsSingleCheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.json")
	if err := os.WriteFile(path, []byte(`{
  "id": "legacy-rules",
  "name": "旧 TRPG 检定",
  "trpg_system": {
    "rule_templates": [
      {
        "id": "first-rule",
        "label": "第一条旧规则",
        "dice": "1d20",
        "failure_policy": "fail_forward"
      },
      {
        "id": "second-rule",
        "label": "第二条旧规则",
        "dice": "1d100",
        "failure_policy": "hard_failure"
      }
    ]
  }
}`), 0o644); err != nil {
		t.Fatalf("write legacy rule system failed: %v", err)
	}

	item, err := parseRuleSystemFile(path)
	if err != nil {
		t.Fatalf("parse legacy rule system failed: %v", err)
	}
	if len(item.TRPGSystem.RuleTemplates) != 1 || item.TRPGSystem.RuleTemplates[0].ID != "first-rule" {
		t.Fatalf("legacy rule system should keep one check config: %#v", item.TRPGSystem.RuleTemplates)
	}
}

func TestDirectorEventCatalogUsesOnlyExplicitConfiguredEventCards(t *testing.T) {
	module := builtinGenreEventPackageModule(
		"test-pack",
		"测试事件包",
		"用于验证事件目录顺序。",
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
	if len(catalog) != len(packCards) {
		t.Fatalf("catalog should contain exactly the selected package cards, got %d: %#v", len(catalog), catalog)
	}
	for i, card := range packCards {
		wantRef := module.ID + "/" + card.ID
		if catalog[i].ID != wantRef {
			t.Fatalf("configured event card refs should be namespaced, index %d got %s want %s in %#v", i, catalog[i].ID, wantRef, catalog)
		}
	}
	if directorEventQueued(catalog, "face_slap") {
		t.Fatalf("unselected default templates must not leak into catalog: %#v", catalog)
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
			ID:                  "heat-check",
			Label:               "耐热检定",
			Dice:                "1d20",
			Modifier:            5,
			FailurePolicy:       "success_at_cost",
			DifficultyGuidance:  "高温、缺水或负重时提高难度。",
			StateEffectGuidance: "失败可扣减体力并增加中暑风险。",
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
	if _, err := openingLibrary.Create(OpeningSelectorModule{
		ID:   "wasteland-openings",
		Name: "旧废土开局",
		OpeningSelector: StoryDirectorOpeningSelector{Enabled: true, InitialStateOps: []StateOp{{
			Op: "set", Path: "flags.wasteland", Value: true,
		}}},
	}); err != nil {
		t.Fatalf("create legacy opening selector file failed: %v", err)
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
			OpeningSelectorID: "wasteland-openings",
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
	if len(director.OpeningSelector.InitialStateOps) != 0 || len(director.OpeningSelector.TraitPools) != 0 {
		t.Fatalf("standalone opening selectors must not be loaded: %#v", director.OpeningSelector)
	}
	if director.ModuleRefs.OpeningSelectorID != "" {
		t.Fatalf("legacy standalone opening selector refs should be discarded: %#v", director.ModuleRefs)
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
				ID:                  "snapshot-rule",
				Label:               "旧快照规则",
				Dice:                "1d20",
				FailurePolicy:       "fail_forward",
				DifficultyGuidance:  "快照难度说明。",
				StateEffectGuidance: "快照状态说明。",
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
