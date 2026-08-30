package lore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLegacyDirectorSidecarDoesNotControlLoreMutation(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	item, err := store.Create(ItemInput{ID: "shen", Type: "character", Name: "沈凝", Content: "角色正文"})
	if err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(workspace, "interactive", "stories", "story", "director", "main", "lore-context.md")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	legacyContent := []byte("[[沈凝]]")
	if err := os.WriteFile(legacyPath, legacyContent, 0o644); err != nil {
		t.Fatal(err)
	}

	updated, err := store.Update(item.ID, ItemInput{Type: item.Type, Name: "沈凝真人", Content: item.Content})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "沈凝真人" {
		t.Fatalf("updated name = %q", updated.Name)
	}
	unchanged, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(unchanged) != string(legacyContent) {
		t.Fatalf("legacy sidecar was unexpectedly rewritten: %q", unchanged)
	}
	if err := store.Delete(item.ID); err != nil {
		t.Fatalf("legacy sidecar should not block deletion: %v", err)
	}
}

func TestLoreReferenceNameValidationCoversDirectAndBatchWrites(t *testing.T) {
	store := NewStore(t.TempDir())
	if _, err := store.Create(ItemInput{ID: "bad-direct", Type: "character", Name: "[[沈凝]]"}); err == nil {
		t.Fatal("direct create accepted reserved reference markers")
	}
	if _, err := store.ApplyOperations("batch", []Operation{{
		Op:   "create",
		Item: ItemInput{ID: "bad-batch", Type: "character", Name: "沈[[凝"},
	}}); err == nil {
		t.Fatal("batch create accepted reserved reference markers")
	}
}
