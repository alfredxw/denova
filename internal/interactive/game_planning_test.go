package interactive

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGamePlanningTemplateLibraryProvidesDistinctBuiltins(t *testing.T) {
	library := NewGamePlanningTemplateLibrary(t.TempDir())
	items, err := library.List()
	if err != nil {
		t.Fatal(err)
	}
	wantIDs := []string{"default", "directed-longform", "character-relationships", "mystery-dread", "episodic-emergent"}
	if len(items) != len(wantIDs) {
		t.Fatalf("builtin planning templates = %d, want %d", len(items), len(wantIDs))
	}
	for index, item := range items {
		if item.ID != wantIDs[index] {
			t.Fatalf("builtin[%d] ID = %q, want %q", index, item.ID, wantIDs[index])
		}
		if item.Custom {
			t.Fatalf("builtin %q unexpectedly marked custom", item.ID)
		}
		if len(item.Sections) == 0 {
			t.Fatalf("builtin %q has no sections", item.ID)
		}
		if err := validateGamePlanningTemplate(item); err != nil {
			t.Fatalf("builtin %q invalid: %v", item.ID, err)
		}
		rendered := strings.ToLower(RenderGamePlanningTemplateMarkdown(item))
		for _, forbidden := range []string{"recommended choices", "exact state values", "completed-event summary"} {
			if strings.Contains(rendered, forbidden) {
				t.Fatalf("builtin %q crosses the planning boundary with %q", item.ID, forbidden)
			}
		}
	}
}

func TestGamePlanningGuideDefinesFutureBlueprintBoundary(t *testing.T) {
	guide := GamePlanningGuideMarkdown(DefaultGamePlanningTemplate(), nil, StoryContextMaxBytes)
	for _, required := range []string{
		"mutable adventure blueprint",
		"next few candidate scenes",
		"only as constraints, never as content to summarize",
		"Remove completed or invalid material",
		"Do not duplicate exact state values or action-choice labels",
	} {
		if !strings.Contains(guide, required) {
			t.Fatalf("planning guide missing %q:\n%s", required, guide)
		}
	}
}

func TestReleasedGamePresetMigrationPreservesEffectiveStoryChoices(t *testing.T) {
	novaDir := t.TempDir()
	workspace := t.TempDir()
	legacyRefs := DefaultStoryDirectorModuleRefs()
	legacyRefs.EventPackagesDisabled = true
	legacy, err := NewStoryDirectorLibrary(novaDir).Create(StoryDirector{
		ID: "legacy-mystery", Name: "Legacy mystery", ModuleRefs: legacyRefs,
		Strategy: StoryDirectorStrategy{
			RuleStateConsumptionMode: RuleStateConsumptionModeSuggestionsOnly,
			RuleVisibilityMode:       RuleVisibilityModePublicRoll,
			PromptMarkdown:           "## Legacy private outline\n\nDo not infer a new structure from this text.",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	store := NewStoreWithNovaDir(workspace, novaDir)
	story, err := store.CreateStory(CreateStoryRequest{Title: "Released story", ModuleRefs: &legacyRefs})
	if err != nil {
		t.Fatal(err)
	}
	storyPath := store.storyPath(story.ID)
	data, err := os.ReadFile(storyPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	var header map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
		t.Fatal(err)
	}
	delete(header, "planning_template_id")
	delete(header, "module_refs")
	delete(header, "check_settings")
	header["story_director_id"] = legacy.ID
	headerJSON, err := json.Marshal(header)
	if err != nil {
		t.Fatal(err)
	}
	lines[0] = string(headerJSON)
	if err := os.WriteFile(storyPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	migratedStore := NewStoreWithNovaDir(workspace, novaDir)
	if _, err := migratedStore.Snapshot(story.ID, "main"); err != nil {
		t.Fatal(err)
	}
	meta, _, err := migratedStore.readStoryLocked(story.ID)
	if err != nil {
		t.Fatal(err)
	}
	if meta.StoryDirectorID != "" || meta.PlanningTemplateID != DefaultGamePlanningTemplateID {
		t.Fatalf("migrated planning identity = legacy:%q template:%q", meta.StoryDirectorID, meta.PlanningTemplateID)
	}
	if meta.ModuleRefs == nil || !meta.ModuleRefs.EventPackagesDisabled {
		t.Fatalf("migrated module refs = %#v", meta.ModuleRefs)
	}
	if meta.CheckSettings.RuleStateConsumptionMode != RuleStateConsumptionModeSuggestionsOnly || meta.CheckSettings.RuleVisibilityMode != RuleVisibilityModePublicRoll {
		t.Fatalf("migrated check settings = %#v", meta.CheckSettings)
	}
	backupDir := filepath.Join(workspace, "interactive", "story", "migrations", "v0.3.3-game-planning")
	for _, name := range []string{"story-" + story.ID + ".jsonl.bak", "game-preset-" + legacy.ID + ".json.bak"} {
		if _, err := os.Stat(filepath.Join(backupDir, name)); err != nil {
			t.Fatalf("missing migration backup %s: %v", name, err)
		}
	}
	presetBackup, err := os.ReadFile(filepath.Join(backupDir, "game-preset-"+legacy.ID+".json.bak"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(presetBackup), "Legacy private outline") {
		t.Fatalf("legacy planning Markdown missing from preset backup:\n%s", presetBackup)
	}
	if _, err := migratedStore.Snapshot(story.ID, "main"); err != nil {
		t.Fatalf("idempotent reload: %v", err)
	}
}

func TestReleasedGamePresetSummaryFallsBackToDefaultPlanning(t *testing.T) {
	summary := normalizeStorySummary(StorySummary{LegacyStoryDirectorID: "legacy-mystery"})
	if summary.PlanningTemplateID != DefaultGamePlanningTemplateID {
		t.Fatalf("planning template = %q, want %q", summary.PlanningTemplateID, DefaultGamePlanningTemplateID)
	}
	if summary.LegacyStoryDirectorID != "" {
		t.Fatalf("legacy preset identity leaked after normalization: %q", summary.LegacyStoryDirectorID)
	}
}

func TestGamePlanningTemplateRendersOrderedMarkdown(t *testing.T) {
	item := GamePlanningTemplate{ID: "custom", Name: "Custom", Sections: []GamePlanningSection{
		{ID: "first", Title: "First horizon", Description: "Plan the first concern."},
		{ID: "second", Title: "Second horizon", Description: "Plan the second concern."},
	}}
	markdown := RenderGamePlanningTemplateMarkdown(item)
	first := strings.Index(markdown, "## First horizon")
	second := strings.Index(markdown, "## Second horizon")
	if first < 0 || second <= first {
		t.Fatalf("rendered order is wrong:\n%s", markdown)
	}
	if strings.Contains(markdown, "first\"") || strings.Contains(markdown, "second\"") {
		t.Fatalf("section IDs leaked into model-visible Markdown:\n%s", markdown)
	}
}

func TestGamePlanningTemplateLibraryCustomCRUD(t *testing.T) {
	library := NewGamePlanningTemplateLibrary(t.TempDir())
	created, err := library.Create(GamePlanningTemplate{
		ID: "custom-plan", Name: "Custom plan", Description: "A copied outline.",
		Sections: []GamePlanningSection{{ID: "opening", Title: "Opening pressure", Description: "Track the first pressure."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !created.Custom || created.Revision == "" {
		t.Fatalf("created metadata = %#v", created)
	}
	created.Sections = append(created.Sections, GamePlanningSection{ID: "payoff", Title: "Payoff", Description: "Track the payoff."})
	updated, err := library.Update(created.ID, created, created.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Sections) != 2 || updated.Revision == created.Revision {
		t.Fatalf("updated template = %#v", updated)
	}
	if _, err := library.Update(updated.ID, updated, created.Revision); err == nil || !strings.Contains(err.Error(), "reload") {
		t.Fatalf("stale update error = %v", err)
	}
	if err := library.Delete(updated.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := library.Get(updated.ID); err == nil {
		t.Fatal("deleted planning template still exists")
	}
}

func TestGamePlanningTemplateRejectsDuplicateTitlesAndBuiltinMutation(t *testing.T) {
	duplicate := GamePlanningTemplate{ID: "duplicate", Name: "Duplicate", Sections: []GamePlanningSection{
		{Title: "Threads"}, {Title: " threads "},
	}}
	if err := validateGamePlanningTemplate(normalizeGamePlanningTemplate(duplicate)); err == nil {
		t.Fatal("duplicate section titles were accepted")
	}
	library := NewGamePlanningTemplateLibrary(t.TempDir())
	builtin := DefaultGamePlanningTemplate()
	if _, err := library.Update(builtin.ID, builtin, ""); err == nil {
		t.Fatal("builtin planning template update was accepted")
	}
	if err := library.Delete(builtin.ID); err == nil {
		t.Fatal("builtin planning template delete was accepted")
	}
}
