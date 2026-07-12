package book

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoreRenameRewritesDirectorNameReferencesAndDeleteBlocks(t *testing.T) {
	workspace := t.TempDir()
	store := NewLoreStore(workspace)
	item, err := store.Create(LoreItemInput{ID: "shen", Type: "character", Name: "沈凝", Content: "角色正文"})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(workspace, "interactive", "stories", "story", "director", "main", directorLoreContextFilename)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("## 当前角色\n\n- [[沈凝]]：当前角色\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	updated, err := store.Update(item.ID, LoreItemInput{Type: item.Type, Name: "沈凝真人", Content: item.Content})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "沈凝真人" || !strings.Contains(string(data), "[[沈凝真人]]") || strings.Contains(string(data), "[[沈凝]]") {
		t.Fatalf("rename should rewrite exact Director reference: item=%#v file=%s", updated, data)
	}
	if err := store.Delete(item.ID); err == nil || !strings.Contains(err.Error(), "正被 1 个互动分支引用") {
		t.Fatalf("referenced lore deletion should be blocked, got %v", err)
	}
}
