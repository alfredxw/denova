package resourceexchange

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"denova/internal/book/lore"
	imagepreset "denova/internal/image/preset"
	"denova/internal/interactive"
	"denova/internal/interactive/teller"
	"denova/internal/project"
	"denova/internal/style"
)

func TestPresetExportIncludesTypedDependenciesAndRewritesLocalIdentities(t *testing.T) {
	ctx := context.Background()
	s := testService(t)
	styleRef, err := style.NewLibrary(s.root).Create(style.WriteRequest{Name: "Portable style", Filename: "source-style", Content: "Use concrete details."})
	if err != nil {
		t.Fatal(err)
	}
	narrative, err := s.catalog.CreateTeller(teller.Definition{Name: "Portable narrative", StyleRefs: []string{styleRef.Path}, Slots: []teller.PromptSlot{{ID: "system", Name: "System", Target: "system", Enabled: true, Content: "Tell the story."}}})
	if err != nil {
		t.Fatal(err)
	}
	// Exercise every preset format directly; builtin definitions are hidden
	// from the picker but must remain exportable as resource dependencies.
	refs := []LocalRef{
		{Kind: "preset.narrative", Scope: "global", ID: narrative.ID},
		{Kind: "preset.image", Scope: "global", ID: imagepreset.DefaultID},
		{Kind: "preset.game_planning", Scope: "global", ID: interactive.DefaultGamePlanningTemplateID},
		{Kind: "preset.events", Scope: "global", ID: interactive.DefaultEventPackageID},
		{Kind: "preset.rules", Scope: "global", ID: interactive.DefaultRuleSystemID},
		{Kind: "preset.actor_state", Scope: "global", ID: interactive.DefaultActorStateModuleID},
	}
	raw, err := s.Export(ctx, ExportRequest{Package: PackageInfo{ID: "all-presets", Name: "All presets"}, Resources: refs})
	if err != nil {
		t.Fatal(err)
	}
	next := testService(t)
	preview, err := next.Preview(ctx, Source{Kind: "file", Filename: "all-presets.zip"}, raw)
	if err != nil {
		t.Fatal(err)
	}
	candidate := preview.Candidates[0]
	selected := []string{}
	for _, item := range candidate.Resources {
		if item.Kind != "style.reference" {
			selected = append(selected, item.ID)
		}
	}
	plan, err := next.Plan(ctx, PlanRequest{PreviewID: preview.ID, CandidateID: candidate.ID, Resources: selected})
	if err != nil {
		t.Fatal(err)
	}
	installed, err := next.Apply(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	var narrativeID, styleID string
	for _, binding := range installed.Bindings {
		switch binding.Local.Kind {
		case "preset.narrative":
			narrativeID = binding.Local.ID
		case "style.reference":
			styleID = binding.Local.ID
		}
	}
	got, err := next.catalog.Teller(narrativeID)
	if err != nil {
		t.Fatal(err)
	}
	if styleID == "" || !slices.Equal(got.StyleRefs, []string{style.StoragePath(styleID)}) || got.ID == narrative.ID {
		t.Fatalf("dependency identity was not remapped: %+v, style=%q", got, styleID)
	}
	if got.Slots[0].Content != "Tell the story." {
		t.Fatalf("prompt content changed: %+v", got.Slots)
	}
}

func TestCharacterCardUsesFrozenPortableResources(t *testing.T) {
	ctx := context.Background()
	s := testService(t)
	raw := []byte(`{"spec":"chara_card_v2","data":{"name":"Explorer","description":"A careful explorer.","first_mes":"You arrive.","alternate_greetings":["Welcome back."],"character_book":{"entries":[{"keys":["Harbor"],"comment":"Harbor","content":"A quiet harbor.","enabled":true}]}}}`)
	preview, err := s.Preview(ctx, Source{Kind: "file", Filename: "explorer.json"}, raw)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Character == nil || preview.Character.OpeningPresetCount != 2 {
		t.Fatalf("missing conversion preview: %+v", preview.Character)
	}
	workspace := filepath.Join(s.root, "projects", "card")
	if err := os.MkdirAll(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	book, err := s.registry.Add(workspace, project.TypeBook, "Card")
	if err != nil {
		t.Fatal(err)
	}
	candidate := preview.Candidates[0]
	selected := []string{}
	for _, item := range candidate.Resources {
		selected = append(selected, item.ID)
	}
	plan, err := s.Plan(ctx, PlanRequest{PreviewID: preview.ID, CandidateID: candidate.ID, ProjectID: book.ID, Resources: selected})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(ctx, plan.ID); err != nil {
		t.Fatal(err)
	}
	items, err := lore.NewStore(workspace).ListAll()
	if err != nil || len(items) != preview.Character.ItemCount {
		t.Fatalf("converted lore: %+v %v", items, err)
	}
	openingBytes, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(openingPath)))
	if err != nil {
		t.Fatal(err)
	}
	var got openings
	if err := json.Unmarshal(openingBytes, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Presets) != 2 || !strings.Contains(string(openingBytes), "You arrive.") {
		t.Fatalf("converted openings: %s", openingBytes)
	}
}
