package resourceexchange

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"denova/internal/book/lore"
	"denova/internal/project"
)

func TestLoreDetailScoring(t *testing.T) {
	rich := lore.Item{Name: "Rich", Content: "x", DetailFields: 3}
	poor := lore.Item{Name: "Poor", Content: "x", DetailFields: 1}
	if !moreDetailed(rich, poor) {
		t.Fatal("more populated fields should win")
	}
	if moreDetailed(poor, rich) {
		t.Fatal("fewer fields must not win")
	}
	short := lore.Item{Name: "Short", Content: "abc", DetailFields: 1}
	long := lore.Item{Name: "Long", Content: "abcdefghij", DetailFields: 1}
	if !moreDetailed(long, short) {
		t.Fatal("longer body should win on equal fields")
	}
	equal := lore.Item{Name: "Equal", Content: "abc", DetailFields: 1}
	if moreDetailed(short, equal) || moreDetailed(equal, short) {
		t.Fatal("equal items must not be more detailed")
	}
	if detailFields(lore.Item{Name: "Manual", Content: "hand written"}) != 1 {
		t.Fatal("hand-written content should fall back to one field")
	}
	if detailFields(lore.Item{Name: "Empty", Content: ""}) != 0 {
		t.Fatal("empty content should count zero fields")
	}
}

func TestCharacterCardOverrideByDetail(t *testing.T) {
	ctx := context.Background()
	s := testService(t)
	dir := filepath.Join(s.root, "projects", "override")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	record, err := s.registry.Add(dir, project.TypeBook, "Override")
	if err != nil {
		t.Fatal(err)
	}
	store := lore.NewStore(dir)
	if _, err := store.Create(lore.ItemInput{ID: "hero", Name: "Hero", Type: "character", Content: "Short"}); err != nil {
		t.Fatal(err)
	}

	incoming := portableCollection[json.RawMessage]{
		Version: 1,
		Items: []json.RawMessage{jsonBytes(t, map[string]any{
			"id":         "hero",
			"name":       "Hero",
			"type":       "character",
			"content":    "A much longer and more detailed description of the hero.",
			"detail_fields": 5,
		})},
	}
	plan, err := planOverrideFixture(t, s, record.ID, incoming, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.ImportOverrides) != 1 || plan.ImportOverrides[0] != "Hero" {
		t.Fatalf("expected one override, got %+v", plan.ImportOverrides)
	}
	if len(plan.ImportSkipped) != 0 {
		t.Fatalf("unexpected skips: %+v", plan.ImportSkipped)
	}
	if _, err := s.Apply(ctx, plan.ID); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Content != "A much longer and more detailed description of the hero." {
		t.Fatalf("override not applied: %+v", items)
	}
}

func TestCharacterCardKeepLessDetailed(t *testing.T) {
	ctx := context.Background()
	s := testService(t)
	dir := filepath.Join(s.root, "projects", "keep")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	record, err := s.registry.Add(dir, project.TypeBook, "Keep")
	if err != nil {
		t.Fatal(err)
	}
	store := lore.NewStore(dir)
	if _, err := store.Create(lore.ItemInput{ID: "hero", Name: "Hero", Type: "character", Content: "An existing richly detailed hero description.", DetailFields: 5}); err != nil {
		t.Fatal(err)
	}

	incoming := portableCollection[json.RawMessage]{
		Version: 1,
		Items: []json.RawMessage{jsonBytes(t, map[string]any{
			"id":         "hero",
			"name":       "Hero",
			"type":       "character",
			"content":    "Short",
			"detail_fields": 1,
		})},
	}
	plan, err := planOverrideFixture(t, s, record.ID, incoming, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.ImportSkipped) != 1 || plan.ImportSkipped[0] != "Hero" {
		t.Fatalf("expected one skip, got %+v", plan.ImportSkipped)
	}
	if len(plan.ImportOverrides) != 0 {
		t.Fatalf("unexpected overrides: %+v", plan.ImportOverrides)
	}
	if _, err := s.Apply(ctx, plan.ID); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Content != "An existing richly detailed hero description." {
		t.Fatalf("existing entry was not kept: %+v", items)
	}
}

func TestCharacterCardWithoutOverrideKeepsNameConflict(t *testing.T) {
	s := testService(t)
	dir := filepath.Join(s.root, "projects", "conflict")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	record, err := s.registry.Add(dir, project.TypeBook, "Conflict")
	if err != nil {
		t.Fatal(err)
	}
	store := lore.NewStore(dir)
	if _, err := store.Create(lore.ItemInput{ID: "hero", Name: "Hero", Type: "character", Content: "Short"}); err != nil {
		t.Fatal(err)
	}

	incoming := portableCollection[json.RawMessage]{
		Version: 1,
		Items: []json.RawMessage{jsonBytes(t, map[string]any{
			"id":         "hero",
			"name":       "Hero",
			"type":       "character",
			"content":    "A much longer and more detailed description of the hero.",
			"detail_fields": 5,
		})},
	}
	if _, err := planOverrideFixture(t, s, record.ID, incoming, false); err == nil {
		t.Fatal("expected duplicate name conflict without OverrideByDetail")
	}
}

// planOverrideFixture builds a single lore-collection resource pack and plans it
// against the given Project, toggling detail-aware override resolution.
func planOverrideFixture(t *testing.T, s *Service, projectID string, incoming portableCollection[json.RawMessage], overrideByDetail bool) (Plan, error) {
	t.Helper()
	return planOverrideFixtureWithChoices(t, s, projectID, incoming, overrideByDetail, nil)
}

// planOverrideFixtureWithChoices additionally supplies explicit per-name
// duplicate choices, as a confirmed character card import would.
func planOverrideFixtureWithChoices(t *testing.T, s *Service, projectID string, incoming portableCollection[json.RawMessage], overrideByDetail bool, choices map[string]string) (Plan, error) {
	t.Helper()
	ctx := context.Background()
	manifest := Manifest{Format: "denova.resource-pack", SchemaVersion: 1, Package: PackageInfo{ID: "card", Name: "Card"}, Resources: []Resource{{ID: "lore", Kind: "lore.collection", Path: "lore.json"}}}
	preview, err := s.previewFiles(ctx, Source{Kind: "file", Filename: "card.zip"}, map[string][]byte{
		"denova-pack.json": jsonBytes(t, manifest),
		"lore.json":        jsonBytes(t, incoming),
	})
	if err != nil {
		t.Fatal(err)
	}
	return s.Plan(ctx, PlanRequest{PreviewID: preview.ID, CandidateID: preview.Candidates[0].ID, Resources: []string{"lore"}, ProjectID: projectID, OverrideByDetail: overrideByDetail, OverrideChoices: choices})
}

func TestCharacterCardHandWrittenDuplicateReportsOverride(t *testing.T) {
	s := testService(t)
	dir := filepath.Join(s.root, "projects", "handwritten")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	record, err := s.registry.Add(dir, project.TypeBook, "Handwritten")
	if err != nil {
		t.Fatal(err)
	}
	store := lore.NewStore(dir)
	// A hand-written entry has no detail fields; detailFields falls back to 1.
	if _, err := store.Create(lore.ItemInput{ID: "hero", Name: "Hero", Type: "character", Content: "Short"}); err != nil {
		t.Fatal(err)
	}

	incoming := portableCollection[json.RawMessage]{
		Version: 1,
		Items: []json.RawMessage{jsonBytes(t, map[string]any{
			"id":            "hero",
			"name":          "Hero",
			"type":          "character",
			"content":       "A much longer and more detailed description of the hero.",
			"detail_fields": 5,
		})},
	}
	plan, err := planOverrideFixture(t, s, record.ID, incoming, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.ImportDuplicates) != 1 {
		t.Fatalf("expected the same-name collision to be listed, got %+v", plan.ImportDuplicates)
	}
	dup := plan.ImportDuplicates[0]
	if dup.Name != "Hero" || dup.Suggested != "override" {
		t.Fatalf("expected Hero suggested override, got %+v", dup)
	}
	if dup.ExistingPreview == "" || dup.IncomingPreview == "" {
		t.Fatalf("expected preview digests on both sides, got %+v", dup)
	}
	if len(plan.ImportOverrides) != 1 || plan.ImportOverrides[0] != "Hero" {
		t.Fatalf("expected one override, got %+v", plan.ImportOverrides)
	}
}

func TestCharacterCardDuplicateKeepChoiceRetainsLocal(t *testing.T) {
	ctx := context.Background()
	s := testService(t)
	dir := filepath.Join(s.root, "projects", "keep-choice")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	record, err := s.registry.Add(dir, project.TypeBook, "KeepChoice")
	if err != nil {
		t.Fatal(err)
	}
	store := lore.NewStore(dir)
	if _, err := store.Create(lore.ItemInput{ID: "hero", Name: "Hero", Type: "character", Content: "An existing richly detailed hero description.", DetailFields: 5}); err != nil {
		t.Fatal(err)
	}

	incoming := portableCollection[json.RawMessage]{
		Version: 1,
		Items: []json.RawMessage{jsonBytes(t, map[string]any{
			"id":            "hero",
			"name":          "Hero",
			"type":          "character",
			"content":       "A much longer and more detailed description of the hero.",
			"detail_fields": 6,
		})},
	}
	// The automatic suggestion is override, but the user keeps the local entry.
	plan, err := planOverrideFixtureWithChoices(t, s, record.ID, incoming, true, map[string]string{"Hero": "keep"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.ImportDuplicates) != 1 || plan.ImportDuplicates[0].Suggested != "override" {
		t.Fatalf("expected the collision listed with override suggestion, got %+v", plan.ImportDuplicates)
	}
	if len(plan.ImportSkipped) != 1 || plan.ImportSkipped[0] != "Hero" {
		t.Fatalf("expected one skip, got %+v", plan.ImportSkipped)
	}
	if _, err := s.Apply(ctx, plan.ID); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Content != "An existing richly detailed hero description." {
		t.Fatalf("existing entry was not retained: %+v", items)
	}
}

func TestCharacterCardMixedOverrideChoices(t *testing.T) {
	ctx := context.Background()
	s := testService(t)
	dir := filepath.Join(s.root, "projects", "mixed")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	record, err := s.registry.Add(dir, project.TypeBook, "Mixed")
	if err != nil {
		t.Fatal(err)
	}
	store := lore.NewStore(dir)
	for _, name := range []string{"Alpha", "Beta"} {
		if _, err := store.Create(lore.ItemInput{ID: name, Name: name, Type: "character", Content: "Local " + name}); err != nil {
			t.Fatal(err)
		}
	}

	incoming := portableCollection[json.RawMessage]{
		Version: 1,
		Items: []json.RawMessage{
			jsonBytes(t, map[string]any{"id": "alpha", "name": "Alpha", "type": "character", "content": "A much longer and more detailed description of Alpha.", "detail_fields": 5}),
			jsonBytes(t, map[string]any{"id": "beta", "name": "Beta", "type": "character", "content": "A much longer and more detailed description of Beta.", "detail_fields": 5}),
		},
	}
	// Only Alpha is explicitly overridden; Beta is omitted and must stay: a
	// confirmed import never replaces entries the user did not choose.
	plan, err := planOverrideFixtureWithChoices(t, s, record.ID, incoming, true, map[string]string{"Alpha": "override"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.ImportOverrides) != 1 || plan.ImportOverrides[0] != "Alpha" {
		t.Fatalf("expected Alpha overridden, got %+v", plan.ImportOverrides)
	}
	if len(plan.ImportSkipped) != 1 || plan.ImportSkipped[0] != "Beta" {
		t.Fatalf("expected Beta kept, got %+v", plan.ImportSkipped)
	}
	if _, err := s.Apply(ctx, plan.ID); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected both entries to remain, got %+v", items)
	}
	for _, item := range items {
		if item.Name == "Alpha" && item.Content != "A much longer and more detailed description of Alpha." {
			t.Fatalf("Alpha was not replaced: %+v", item)
		}
		if item.Name == "Beta" && item.Content != "Local Beta" {
			t.Fatalf("Beta was replaced: %+v", item)
		}
	}
}

func TestCharacterCardInvalidOverrideChoice(t *testing.T) {
	s := testService(t)
	dir := filepath.Join(s.root, "projects", "invalid-choice")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	record, err := s.registry.Add(dir, project.TypeBook, "InvalidChoice")
	if err != nil {
		t.Fatal(err)
	}
	store := lore.NewStore(dir)
	if _, err := store.Create(lore.ItemInput{ID: "hero", Name: "Hero", Type: "character", Content: "Short"}); err != nil {
		t.Fatal(err)
	}
	incoming := portableCollection[json.RawMessage]{
		Version: 1,
		Items: []json.RawMessage{jsonBytes(t, map[string]any{
			"id":      "hero",
			"name":    "Hero",
			"type":    "character",
			"content": "A much longer and more detailed description of the hero.",
		})},
	}
	if _, err := planOverrideFixtureWithChoices(t, s, record.ID, incoming, true, map[string]string{"Hero": "replace"}); err == nil {
		t.Fatal("expected invalid override choice to fail validation")
	}
}

func TestCharacterCardPendingPlanLeavesLocalUntouched(t *testing.T) {
	s := testService(t)
	dir := filepath.Join(s.root, "projects", "pending")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	record, err := s.registry.Add(dir, project.TypeBook, "Pending")
	if err != nil {
		t.Fatal(err)
	}
	store := lore.NewStore(dir)
	if _, err := store.Create(lore.ItemInput{ID: "hero", Name: "Hero", Type: "character", Content: "Short"}); err != nil {
		t.Fatal(err)
	}
	incoming := portableCollection[json.RawMessage]{
		Version: 1,
		Items: []json.RawMessage{jsonBytes(t, map[string]any{
			"id":            "hero",
			"name":          "Hero",
			"type":          "character",
			"content":       "A much longer and more detailed description of the hero.",
			"detail_fields": 5,
		})},
	}
	plan, err := planOverrideFixture(t, s, record.ID, incoming, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.ImportDuplicates) == 0 {
		t.Fatal("expected a held duplicate conflict")
	}
	// The handler returns a pending review without applying; local content must
	// stay exactly as written until the user confirms.
	items, err := store.ListAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Content != "Short" {
		t.Fatalf("local entry changed before confirmation: %+v", items)
	}
}

func TestDiscardPlanRemovesPersistedPlan(t *testing.T) {
	ctx := context.Background()
	s := testService(t)
	dir := filepath.Join(s.root, "projects", "discard")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	record, err := s.registry.Add(dir, project.TypeBook, "Discard")
	if err != nil {
		t.Fatal(err)
	}
	store := lore.NewStore(dir)
	if _, err := store.Create(lore.ItemInput{ID: "hero", Name: "Hero", Type: "character", Content: "Short"}); err != nil {
		t.Fatal(err)
	}
	incoming := portableCollection[json.RawMessage]{
		Version: 1,
		Items: []json.RawMessage{jsonBytes(t, map[string]any{
			"id":            "hero",
			"name":          "Hero",
			"type":          "character",
			"content":       "A much longer and more detailed description of the hero.",
			"detail_fields": 5,
		})},
	}
	plan, err := planOverrideFixture(t, s, record.ID, incoming, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadPlan(ctx, plan.ID); err != nil {
		t.Fatalf("plan should be persisted before discard: %v", err)
	}
	if err := s.DiscardPlan(plan.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadPlan(ctx, plan.ID); err == nil {
		t.Fatal("expected plan to be gone after DiscardPlan")
	}
	// Discarding again is idempotent.
	if err := s.DiscardPlan(plan.ID); err != nil {
		t.Fatalf("discarding a missing plan should succeed, got %v", err)
	}
}

func TestCharacterCardLongerLocalBodyStillSuggestsOverrideByFields(t *testing.T) {
	s := testService(t)
	dir := filepath.Join(s.root, "projects", "field-rule")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	record, err := s.registry.Add(dir, project.TypeBook, "FieldRule")
	if err != nil {
		t.Fatal(err)
	}
	store := lore.NewStore(dir)
	// Longer hand-written body with no detail fields falls back to one field.
	if _, err := store.Create(lore.ItemInput{ID: "hero", Name: "Hero", Type: "character", Content: "A very long hand-written description... it keeps going and going."}); err != nil {
		t.Fatal(err)
	}
	incoming := portableCollection[json.RawMessage]{
		Version: 1,
		Items: []json.RawMessage{jsonBytes(t, map[string]any{
			"id":            "hero",
			"name":          "Hero",
			"type":          "character",
			"content":       "Short",
			"detail_fields": 3,
		})},
	}
	plan, err := planOverrideFixture(t, s, record.ID, incoming, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.ImportDuplicates) != 1 || plan.ImportDuplicates[0].Suggested != "override" {
		t.Fatalf("expected field count to win over body length, got %+v", plan.ImportDuplicates)
	}
	if len(plan.ImportOverrides) != 1 || plan.ImportOverrides[0] != "Hero" {
		t.Fatalf("expected automatic override, got %+v", plan.ImportOverrides)
	}
}
