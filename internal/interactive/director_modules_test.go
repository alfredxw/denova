package interactive

import "testing"

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
