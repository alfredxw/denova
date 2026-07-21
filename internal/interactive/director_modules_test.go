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
		requireActorStateTemplates(t, item, "protagonist", ActorStateStoryContextTemplateID, ActorStateImportantCharacterTemplateID, ActorStateOpponentTemplateID, ActorStateWorldEntitiesTemplateID)
		if len(item.ActorState.Templates) != 5 {
			t.Fatalf("actor state %s should use exactly five centralized templates: %#v", id, item.ActorState.Templates)
		}
		if len(item.ActorState.InitialActors) != 3 ||
			item.ActorState.InitialActors[0].ID != DefaultActorID ||
			item.ActorState.InitialActors[0].TemplateID != "protagonist" ||
			item.ActorState.InitialActors[1].ID != DefaultStoryContextActorID ||
			item.ActorState.InitialActors[1].TemplateID != ActorStateStoryContextTemplateID ||
			item.ActorState.InitialActors[2].ID != DefaultWorldEntitiesActorID ||
			item.ActorState.InitialActors[2].TemplateID != ActorStateWorldEntitiesTemplateID {
			t.Fatalf("actor state %s should ship only protagonist, story, and world starter actors: %#v", id, item.ActorState.InitialActors)
		}
		requireWritableActorStatePresetFields(t, item)
	}

	defaultActorState, err := library.Get(DefaultActorStateModuleID)
	if err != nil {
		t.Fatalf("Get default actor state failed: %v", err)
	}
	if actorStateTemplateHasField(defaultActorState, "protagonist", "current.status") ||
		actorStateTemplateHasField(defaultActorState, "protagonist", "mechanics.panel") ||
		actorStateTemplateHasField(defaultActorState, "protagonist", "current.dynamic_state") ||
		!actorStateTemplateHasField(defaultActorState, "protagonist", "abilities.records") ||
		!actorStateTemplateHasField(defaultActorState, "protagonist", "assets.important_items") ||
		!actorStateTemplateHasField(defaultActorState, "protagonist", "relations.records") ||
		!actorStateTemplateHasField(defaultActorState, ActorStateStoryContextTemplateID, "scene.current_event") ||
		!actorStateTemplateHasField(defaultActorState, ActorStateStoryContextTemplateID, "scene.continuation_hook") ||
		!actorStateTemplateHasField(defaultActorState, ActorStateStoryContextTemplateID, "world.situation") ||
		!actorStateTemplateHasField(defaultActorState, ActorStateStoryContextTemplateID, "tasks.current") ||
		actorStateTemplateHasField(defaultActorState, ActorStateStoryContextTemplateID, "scene.elements") ||
		actorStateTemplateHasField(defaultActorState, ActorStateStoryContextTemplateID, "rules.active") ||
		actorStateTemplateHasField(defaultActorState, ActorStateStoryContextTemplateID, "world.setting") ||
		!actorStateTemplateHasField(defaultActorState, ActorStateImportantCharacterTemplateID, "identity.appearance_style") ||
		!actorStateTemplateHasField(defaultActorState, ActorStateImportantCharacterTemplateID, "protagonist_relation.favorability") ||
		!actorStateTemplateHasField(defaultActorState, ActorStateImportantCharacterTemplateID, "knowledge.about_protagonist") ||
		!actorStateTemplateHasField(defaultActorState, ActorStateImportantCharacterTemplateID, "abilities.records") ||
		!actorStateTemplateHasField(defaultActorState, ActorStateOpponentTemplateID, "threat.assessment") ||
		!actorStateTemplateHasField(defaultActorState, ActorStateOpponentTemplateID, "assets.important_items") ||
		!actorStateTemplateHasField(defaultActorState, ActorStateWorldEntitiesTemplateID, "world.locations") ||
		!actorStateTemplateHasField(defaultActorState, ActorStateWorldEntitiesTemplateID, "world.factions") {
		t.Fatalf("default actor state should expose centralized story, actor-owned, and world-entity fields: %#v", defaultActorState.ActorState.Templates)
	}
	wantDefaultFieldCounts := map[string]int{
		DefaultActorID:                         21,
		ActorStateStoryContextTemplateID:       7,
		ActorStateImportantCharacterTemplateID: 24,
		ActorStateOpponentTemplateID:           21,
		ActorStateWorldEntitiesTemplateID:      2,
	}
	protagonist := actorStateTemplateByID(defaultActorState.ActorState, DefaultActorID)
	for _, fieldPath := range []string{
		"panel.level", "panel.strength", "panel.dexterity", "panel.constitution", "panel.intelligence",
		"panel.wisdom", "panel.charisma", "panel.attack_ac", "panel.defense_dc",
	} {
		field, ok := actorStateFieldByPath(protagonist, fieldPath)
		if !ok || field.Type != "number" || field.Group != "面板" {
			t.Fatalf("default TRPG panel should use grouped ordinary number fields; %s = %#v", fieldPath, field)
		}
	}
	for _, fieldPath := range []string{"state.health", "state.mana", "state.effects", "state.cooldowns"} {
		field, ok := actorStateFieldByPath(protagonist, fieldPath)
		if !ok || field.Group != "状态" || field.Type == "object" {
			t.Fatalf("default dynamic state should use grouped ordinary fields; %s = %#v", fieldPath, field)
		}
	}
	for _, template := range defaultActorState.ActorState.Templates {
		if want := wantDefaultFieldCounts[template.ID]; len(template.Fields) != want {
			t.Fatalf("default actor state template %s field count = %d, want %d", template.ID, len(template.Fields), want)
		}
	}
	genreFields := []struct {
		id         string
		templateID string
		fieldPath  string
	}{
		{id: ActorStateXiuxianID, templateID: DefaultActorID, fieldPath: "cultivation.foundation"},
		{id: ActorStateWesternFantasyID, templateID: DefaultActorID, fieldPath: "fantasy.progression"},
		{id: ActorStateApocalypseID, templateID: ActorStateStoryContextTemplateID, fieldPath: "apocalypse.base"},
		{id: ActorStateInfiniteFlowID, templateID: ActorStateStoryContextTemplateID, fieldPath: "infinite_space.current_instance"},
	}
	for _, expected := range genreFields {
		if !actorStateTemplateHasField(byID[expected.id], expected.templateID, expected.fieldPath) {
			t.Fatalf("genre actor state %s missing field %s/%s: %#v", expected.id, expected.templateID, expected.fieldPath, byID[expected.id].ActorState.Templates)
		}
	}
	genreMechanicCases := []struct {
		id         string
		wantPaths  []string
		avoidPaths []string
	}{
		{
			id:         ActorStateXiuxianID,
			wantPaths:  []string{"panel.realm", "state.realm_progress", "state.effects", "state.cooldowns"},
			avoidPaths: []string{"panel.strength", "panel.dexterity", "state.health", "state.mana"},
		},
		{
			id:         ActorStateWesternFantasyID,
			wantPaths:  []string{"panel.profession", "panel.level", "panel.attack_ac", "panel.defense_dc", "state.health", "state.spell_resource", "state.effects", "state.cooldowns"},
			avoidPaths: []string{"panel.strength", "panel.dexterity", "panel.constitution"},
		},
		{
			id:         ActorStateApocalypseID,
			wantPaths:  []string{"state.survival_condition", "state.effects", "state.cooldowns"},
			avoidPaths: []string{"panel.level", "panel.strength", "state.hunger", "state.thirst", "state.fatigue"},
		},
		{
			id:         ActorStateInfiniteFlowID,
			wantPaths:  []string{"panel.space_rating", "state.current_resources", "state.rule_effects", "state.cooldowns"},
			avoidPaths: []string{"panel.strength", "panel.dexterity", "panel.constitution", "state.health", "state.mana"},
		},
	}
	for _, testCase := range genreMechanicCases {
		for _, templateID := range []string{DefaultActorID, ActorStateImportantCharacterTemplateID, ActorStateOpponentTemplateID} {
			for _, path := range testCase.wantPaths {
				if !actorStateTemplateHasField(byID[testCase.id], templateID, path) {
					t.Fatalf("genre actor state %s template %s missing setting-specific field %s", testCase.id, templateID, path)
				}
			}
			for _, path := range testCase.avoidPaths {
				if actorStateTemplateHasField(byID[testCase.id], templateID, path) {
					t.Fatalf("genre actor state %s template %s should not blindly include %s", testCase.id, templateID, path)
				}
			}
		}
	}
	if actorStateTemplateHasField(byID[ActorStateXiuxianID], DefaultActorID, "cultivation.progress") ||
		actorStateTemplateHasField(byID[ActorStateApocalypseID], DefaultActorID, "survival.exposure") ||
		actorStateTemplateHasField(byID[ActorStateInfiniteFlowID], DefaultActorID, "infinite_space.conditions") {
		t.Fatalf("genre presets should not duplicate panel or dynamic-state facts in text fields")
	}

	xiuxian, err := library.Get(ActorStateXiuxianID)
	if err != nil {
		t.Fatalf("Get xiuxian preset failed: %v", err)
	}
	if xiuxian.Name != "修仙状态系统" || !actorStateTemplateHasField(xiuxian, "protagonist", "cultivation.foundation") {
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
	if restored.Custom || restored.BuiltinOverridden || restored.Name == "我的修仙状态系统" || !actorStateTemplateHasField(restored, ActorStateOpponentTemplateID, "cultivation.threat_profile") {
		t.Fatalf("unexpected restored xiuxian actor state: %#v", restored)
	}

	resolved := ResolveStoryDirectorModules(novaDir, StoryDirector{
		ID:   "genre-director",
		Name: "题材导演",
		ModuleRefs: StoryDirectorModuleRefs{
			NarrativeStyleDisabled: true,
			EventPackagesDisabled:  true,
			RuleSystemDisabled:     true,
			ActorStateID:           ActorStateInfiniteFlowID,
			ImagePresetDisabled:    true,
		},
	})
	if !actorStateTemplateHasField(ActorStateModule{ActorState: resolved.ActorState}, ActorStateOpponentTemplateID, "infinite_space.rule_profile") {
		t.Fatalf("director should resolve infinite-flow actor state templates: %#v", resolved.ActorState)
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

func requireWritableActorStatePresetFields(t *testing.T, item ActorStateModule) {
	t.Helper()
	forbiddenPaths := map[string]bool{
		"identity.outfit":       true,
		"current.mental_status": true,
		"threat.level":          true,
		"world.major_region":    true,
		"world.region":          true,
		"survival.hunger":       true,
		"survival.thirst":       true,
		"knowledge.misunderstood_about_protagonist": true,
	}
	for _, template := range item.ActorState.Templates {
		if len(template.Fields) == 0 || len(template.Fields) > 32 {
			t.Fatalf("genre actor state %s template %s has invalid field count: %d", item.ID, template.ID, len(template.Fields))
		}
		if len(template.DisplayGroups) != 0 {
			t.Fatalf("genre actor state %s template %s must not freeze UI group order in schema: %#v", item.ID, template.ID, template.DisplayGroups)
		}
		for _, field := range template.Fields {
			if field.Order != 0 {
				t.Fatalf("genre actor state %s field %s must use array order only as the UI fallback: %#v", item.ID, field.Path, field)
			}
			if strings.HasPrefix(field.Path, "attributes.") || forbiddenPaths[field.Path] {
				t.Fatalf("genre actor state %s retains a fragmented or generic field %s: %#v", item.ID, field.Path, field)
			}
			if field.Group == "" {
				t.Fatalf("genre actor state %s field %s should be visible in the grouped structure: %#v", item.ID, field.Path, field)
			}
			if (field.Path == "mechanics.panel" || field.Path == "current.dynamic_state") && field.Type == "object" {
				t.Fatalf("genre actor state %s should not hide panel or status fields inside JSON: %#v", item.ID, field)
			}
			if strings.Contains(field.UpdateInstruction, "只记录正文、规则检定") {
				t.Fatalf("genre actor state %s field %s retains boilerplate update guidance: %#v", item.ID, field.Path, field)
			}
			if field.Type == "number" {
				if !strings.Contains(field.Description, "–") ||
					field.Min == nil || field.Max == nil || *field.Min >= *field.Max {
					t.Fatalf("genre actor state %s numeric field must provide a meaningful scale: %#v", item.ID, field)
				}
			}
		}
		if template.ID == DefaultActorID || template.ID == ActorStateImportantCharacterTemplateID || template.ID == ActorStateOpponentTemplateID {
			if _, ok := actorStateFieldByPath(template, "current.status"); ok {
				t.Fatalf("genre actor state %s template %s still has the replaced text status field", item.ID, template.ID)
			}
		}
	}
	if err := validateActorStateSystem(item.ActorState); err != nil {
		t.Fatalf("genre actor state %s should be a valid frozen-schema source: %v", item.ID, err)
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
					ID:      "heat",
					Path:    "resources.heat",
					Name:    "热量",
					Type:    "number",
					Default: float64(1),
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
	director, err := directorLibrary.Create(StoryDirector{
		ID:   "modular",
		Name: "模块化导演",
		ModuleRefs: StoryDirectorModuleRefs{
			NarrativeStyleID: "classic",
			EventPackageIDs:  []string{eventModule.ID},
			RuleSystemID:     ruleModule.ID,
			ActorStateID:     actorModule.ID,
			ImagePresetID:    "game-cg",
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
			NarrativeStyleID:       "missing-style",
			NarrativeStyleDisabled: true,
			EventPackageIDs:        []string{"missing-events"},
			EventPackagesDisabled:  true,
			RuleSystemID:           "missing-rules",
			RuleSystemDisabled:     true,
			ActorStateID:           "missing-actors",
			ActorStateDisabled:     true,
			ImagePresetID:          "missing-image",
			ImagePresetDisabled:    true,
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
	if events := DirectorEventCatalogFromStoryDirector(director); len(events) != 0 {
		t.Fatalf("disabled event packages should not expose default event catalog: %#v", events)
	}
}
