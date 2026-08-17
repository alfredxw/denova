package book

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceContextRefreshesChapterPathsWhenNestedDirectoryChanges(t *testing.T) {
	root := t.TempDir()
	volume := filepath.Join(root, "chapters", "volume-one")
	if err := os.MkdirAll(volume, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(volume, "ch01.md"), []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	state := NewState(root)
	if context := state.WorkspaceContext().Dynamic; !strings.Contains(context, "ch01.md") {
		t.Fatalf("initial context = %q", context)
	}
	if err := os.WriteFile(filepath.Join(volume, "ch02.md"), []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	if context := state.WorkspaceContext().Dynamic; !strings.Contains(context, "ch02.md") {
		t.Fatalf("refreshed context = %q", context)
	}
}

func TestWorkspaceContextChapterIndexIgnoresContentOnlyChanges(t *testing.T) {
	root := t.TempDir()
	chapterDir := filepath.Join(root, "chapters")
	if err := os.MkdirAll(chapterDir, 0o755); err != nil {
		t.Fatal(err)
	}
	chapter := filepath.Join(chapterDir, "ch01.md")
	if err := os.WriteFile(chapter, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	state := NewState(root)
	first := state.WorkspaceContext().Dynamic
	if err := os.WriteFile(chapter, []byte("updated content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if second := state.WorkspaceContext().Dynamic; second != first {
		t.Fatalf("content-only update changed path context: first=%q second=%q", first, second)
	}
}
