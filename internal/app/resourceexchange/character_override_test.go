package resourceexchange

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"denova/internal/book/lore"
	"denova/internal/project"
	"denova/internal/revisionfile"
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

func TestCharacterCardApplyRejectsConcurrentEdit(t *testing.T) {
	ctx := context.Background()
	s := testService(t)
	dir := filepath.Join(s.root, "projects", "concurrent-edit")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	record, err := s.registry.Add(dir, project.TypeBook, "ConcurrentEdit")
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
	// The Lore collection changes between planning and applying (a save after
	// the preview): Apply must surface a revision conflict instead of silently
	// overwriting the fresh edit.
	if _, err := store.Update("hero", lore.ItemInput{ID: "hero", Name: "Hero", Type: "character", Content: "Fresh edit saved after preview"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(ctx, plan.ID); err == nil {
		t.Fatal("expected a revision conflict for a plan whose target changed")
	} else {
		var conflict *revisionfile.ConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("expected *revisionfile.ConflictError, got %T: %v", err, err)
		}
	}
	// The concurrent edit is untouched.
	items, err := store.ListAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Content != "Fresh edit saved after preview" {
		t.Fatalf("concurrent edit was overwritten: %+v", items)
	}
}

func TestCharacterCardChoiceOutsideReviewedDuplicatesRejected(t *testing.T) {
	s := testService(t)
	dir := filepath.Join(s.root, "projects", "choice-outside")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	record, err := s.registry.Add(dir, project.TypeBook, "ChoiceOutside")
	if err != nil {
		t.Fatal(err)
	}
	incoming := portableCollection[json.RawMessage]{
		Version: 1,
		Items: []json.RawMessage{jsonBytes(t, map[string]any{
			"id":      "hero",
			"name":    "Hero",
			"type":    "character",
			"content": "A longer description of the hero.",
		})},
	}
	// A choice for a name that was never part of the reviewed duplicates must
	// fail validation instead of silently replacing an unreviewed entry.
	previewPlan, err := planOverrideFixture(t, s, record.ID, incoming, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(previewPlan.ImportDuplicates) != 0 {
		t.Fatalf("expected no collision at preview time, got %+v", previewPlan.ImportDuplicates)
	}
	if _, err := planOverrideFixtureWithConfirm(t, s, record.ID, incoming, nil, map[string]string{"Ghost": "override"}); err == nil {
		t.Fatal("expected a choice outside the reviewed duplicates to fail validation")
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

// planOverrideFixtureWithConfirm plans an override with the review binding a
// confirmed character card import carries: the duplicates the user reviewed at
// preview time plus the per-name decisions, exactly as the handler sends them.
func planOverrideFixtureWithConfirm(t *testing.T, s *Service, projectID string, incoming portableCollection[json.RawMessage], duplicates []ImportDuplicate, choices map[string]string) (Plan, error) {
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
	return s.Plan(ctx, PlanRequest{
		PreviewID:       preview.ID,
		CandidateID:     preview.Candidates[0].ID,
		Resources:       []string{"lore"},
		ProjectID:       projectID,
		OverrideByDetail: true,
		OverrideChoices:  choices,
		ConfirmReview:    &ConfirmReview{Duplicates: duplicates, Choices: choices},
	})
}

func TestCharacterCardConfirmRejectsEditedLocalEntry(t *testing.T) {
	ctx := context.Background()
	s := testService(t)
	dir := filepath.Join(s.root, "projects", "edited")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	record, err := s.registry.Add(dir, project.TypeBook, "Edited")
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
	// The preview captures the local identity and version the user reviews.
	previewPlan, err := planOverrideFixture(t, s, record.ID, incoming, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(previewPlan.ImportDuplicates) != 1 {
		t.Fatalf("expected one preview duplicate, got %+v", previewPlan.ImportDuplicates)
	}
	dup := previewPlan.ImportDuplicates[0]
	if dup.ExistingID == "" || dup.ExistingVersion == "" {
		t.Fatalf("preview duplicate must bind local identity and version, got %+v", dup)
	}
	// The user saves new content for the same entry after the preview.
	if _, err := store.Update("hero", lore.ItemInput{ID: "hero", Name: "Hero", Type: "character", Content: "Fresh edit saved after preview"}); err != nil {
		t.Fatal(err)
	}
	// Confirming the stale preview must not replace the edited entry.
	plan, err := planOverrideFixtureWithConfirm(t, s, record.ID, incoming, []ImportDuplicate{dup}, map[string]string{"Hero": "override"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.ImportChanged) != 1 || plan.ImportChanged[0] != "Hero" {
		t.Fatalf("expected edited entry reported changed, got changed=%+v overrides=%+v", plan.ImportChanged, plan.ImportOverrides)
	}
	if len(plan.ImportOverrides) != 0 {
		t.Fatalf("stale confirmation must not override, got %+v", plan.ImportOverrides)
	}
	if _, err := s.Apply(ctx, plan.ID); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Content != "Fresh edit saved after preview" {
		t.Fatalf("edited local entry was overwritten: %+v", items)
	}
}

func TestCharacterCardConfirmEmptyChoicesKeepsLocal(t *testing.T) {
	ctx := context.Background()
	s := testService(t)
	dir := filepath.Join(s.root, "projects", "empty-choice")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	record, err := s.registry.Add(dir, project.TypeBook, "EmptyChoice")
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
	previewPlan, err := planOverrideFixture(t, s, record.ID, incoming, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(previewPlan.ImportDuplicates) != 1 {
		t.Fatalf("expected one preview duplicate, got %+v", previewPlan.ImportDuplicates)
	}
	// Confirming without any per-entry choice keeps every local entry: an empty
	// choice map must never fall back to detail-aware automatic overrides.
	plan, err := planOverrideFixtureWithConfirm(t, s, record.ID, incoming, previewPlan.ImportDuplicates, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.ImportOverrides) != 0 {
		t.Fatalf("empty choices must not override anything, got %+v", plan.ImportOverrides)
	}
	if len(plan.ImportSkipped) != 1 || plan.ImportSkipped[0] != "Hero" {
		t.Fatalf("expected Hero kept, got %+v", plan.ImportSkipped)
	}
	if _, err := s.Apply(ctx, plan.ID); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Content != "Short" {
		t.Fatalf("local entry was replaced without explicit choice: %+v", items)
	}
}

func TestCharacterCardConfirmUnreviewedSameNameEntryKept(t *testing.T) {
	ctx := context.Background()
	s := testService(t)
	dir := filepath.Join(s.root, "projects", "unreviewed")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	record, err := s.registry.Add(dir, project.TypeBook, "Unreviewed")
	if err != nil {
		t.Fatal(err)
	}
	store := lore.NewStore(dir)
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
	// Preview sees no same-name collision, so the review has no binding for it.
	previewPlan, err := planOverrideFixture(t, s, record.ID, incoming, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(previewPlan.ImportDuplicates) != 0 {
		t.Fatalf("expected no collision at preview time, got %+v", previewPlan.ImportDuplicates)
	}
	// A same-name entry appears after the preview; it was never reviewed.
	if _, err := store.Create(lore.ItemInput{ID: "hero", Name: "Hero", Type: "character", Content: "Short"}); err != nil {
		t.Fatal(err)
	}
	plan, err := planOverrideFixtureWithConfirm(t, s, record.ID, incoming, nil, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.ImportUnreviewed) != 1 || plan.ImportUnreviewed[0] != "Hero" {
		t.Fatalf("expected unreviewed entry reported, got unreviewed=%+v overrides=%+v", plan.ImportUnreviewed, plan.ImportOverrides)
	}
	if len(plan.ImportOverrides) != 0 {
		t.Fatalf("unreviewed entry must not be overridden, got %+v", plan.ImportOverrides)
	}
	if _, err := s.Apply(ctx, plan.ID); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Content != "Short" {
		t.Fatalf("unreviewed entry was replaced: %+v", items)
	}
}

func TestCharacterCardConfirmMatchingVersionAppliesOverride(t *testing.T) {
	ctx := context.Background()
	s := testService(t)
	dir := filepath.Join(s.root, "projects", "confirm-match")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	record, err := s.registry.Add(dir, project.TypeBook, "ConfirmMatch")
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
	previewPlan, err := planOverrideFixture(t, s, record.ID, incoming, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(previewPlan.ImportDuplicates) != 1 {
		t.Fatalf("expected one preview duplicate, got %+v", previewPlan.ImportDuplicates)
	}
	// The user confirms the untouched preview with an explicit override: the
	// reviewed version still matches and the replacement applies.
	plan, err := planOverrideFixtureWithConfirm(t, s, record.ID, incoming, previewPlan.ImportDuplicates, map[string]string{"Hero": "override"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.ImportChanged) != 0 || len(plan.ImportUnreviewed) != 0 {
		t.Fatalf("unexpected review conflicts: changed=%+v unreviewed=%+v", plan.ImportChanged, plan.ImportUnreviewed)
	}
	if len(plan.ImportOverrides) != 1 || plan.ImportOverrides[0] != "Hero" {
		t.Fatalf("expected Hero overridden, got %+v", plan.ImportOverrides)
	}
	if _, err := s.Apply(ctx, plan.ID); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Content != "A much longer and more detailed description of the hero." {
		t.Fatalf("matching override was not applied: %+v", items)
	}
}
