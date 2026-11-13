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
	ctx := context.Background()
	manifest := Manifest{Format: "denova.resource-pack", SchemaVersion: 1, Package: PackageInfo{ID: "card", Name: "Card"}, Resources: []Resource{{ID: "lore", Kind: "lore.collection", Path: "lore.json"}}}
	preview, err := s.previewFiles(ctx, Source{Kind: "file", Filename: "card.zip"}, map[string][]byte{
		"denova-pack.json": jsonBytes(t, manifest),
		"lore.json":        jsonBytes(t, incoming),
	})
	if err != nil {
		t.Fatal(err)
	}
	return s.Plan(ctx, PlanRequest{PreviewID: preview.ID, CandidateID: preview.Candidates[0].ID, Resources: []string{"lore"}, ProjectID: projectID, OverrideByDetail: overrideByDetail})
}
