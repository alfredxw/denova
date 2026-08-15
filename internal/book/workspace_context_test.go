package book

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"denova/internal/book/lore"
)

func TestWorkspaceContextIndexesFilesWithoutInjectingTheirBodies(t *testing.T) {
	dir := t.TempDir()
	state := NewState(dir)
	if err := state.InitWorkspace(); err != nil {
		t.Fatalf("initialize workspace: %v", err)
	}

	files := map[string]string{
		state.IdeasPath(): "IDEAS_BODY_SHOULD_BE_LAZY",
		filepath.Join(state.SettingDir(), "outline.md"):                "OUTLINE_BODY_SHOULD_BE_LAZY",
		filepath.Join(state.SettingDir(), "progress.md"):               "PROGRESS_BODY_SHOULD_BE_LAZY",
		filepath.Join(state.SettingDir(), CharacterStatesFileName):     "CHARACTER_BODY_SHOULD_BE_LAZY",
		filepath.Join(state.ChapterGroupDir(), "group01-wasteland.md"): "GROUP_BODY_SHOULD_BE_LAZY",
		filepath.Join(dir, "chapters", "ch00001-opening.md"):           "CHAPTER_BODY_SHOULD_BE_LAZY",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	store := lore.NewStore(dir)
	if _, err := store.Create(lore.ItemInput{
		ID: "resident", Type: "character", Name: "Resident Hero",
		LoadMode: lore.LoadModeResident, Content: "RESIDENT_LORE_BODY_STAYS_INLINE",
	}); err != nil {
		t.Fatalf("create resident Lore: %v", err)
	}
	if _, err := store.Create(lore.ItemInput{
		ID: "manual", Type: "location", Name: "Hidden Archive",
		LoadMode: lore.LoadModeManual, Content: "ON_DEMAND_LORE_BODY_STAYS_LAZY",
	}); err != nil {
		t.Fatalf("create on-demand Lore: %v", err)
	}

	context := state.WorkspaceContext()
	for _, required := range []string{
		"## Workspace Source Index",
		`"ideas.md"`,
		`"setting/outline.md"`,
		"## Resident Lore",
		"RESIDENT_LORE_BODY_STAYS_INLINE",
		"Hidden Archive",
	} {
		if !strings.Contains(context.Stable, required) {
			t.Fatalf("stable workspace context missing %q:\n%s", required, context.Stable)
		}
	}
	for _, required := range []string{
		"## Current Writing Source Index",
		`"setting/progress.md"`,
		`"setting/character-states.md"`,
		`"setting/chapter-groups/group01-wasteland.md"`,
		`"chapters/ch00001-opening.md"`,
	} {
		if !strings.Contains(context.Dynamic, required) {
			t.Fatalf("dynamic workspace context missing %q:\n%s", required, context.Dynamic)
		}
	}
	for _, forbidden := range []string{
		"IDEAS_BODY_SHOULD_BE_LAZY",
		"OUTLINE_BODY_SHOULD_BE_LAZY",
		"PROGRESS_BODY_SHOULD_BE_LAZY",
		"CHARACTER_BODY_SHOULD_BE_LAZY",
		"GROUP_BODY_SHOULD_BE_LAZY",
		"CHAPTER_BODY_SHOULD_BE_LAZY",
		"ON_DEMAND_LORE_BODY_STAYS_LAZY",
	} {
		if strings.Contains(context.Markdown(), forbidden) {
			t.Fatalf("workspace context injected lazy body %q:\n%s", forbidden, context.Markdown())
		}
	}
}

func TestWorkspaceContextOmitsIdeasTemplateAndIndexesEditedIdeasByPath(t *testing.T) {
	dir := t.TempDir()
	state := NewState(dir)
	if err := state.InitWorkspace(); err != nil {
		t.Fatalf("initialize workspace: %v", err)
	}
	if context := state.WorkspaceContext(); context.Markdown() != "" {
		t.Fatalf("an untouched workspace should use the new-work hint, got:\n%s", context.Markdown())
	}

	if err := os.WriteFile(state.IdeasPath(), []byte("A revenge story on a flooded planet."), 0o644); err != nil {
		t.Fatal(err)
	}
	context := state.WorkspaceContext()
	if !strings.Contains(context.Stable, `"ideas.md"`) {
		t.Fatalf("edited ideas path missing from stable index:\n%s", context.Stable)
	}
	if strings.Contains(context.Stable, "A revenge story on a flooded planet.") {
		t.Fatalf("ideas body must be loaded on demand:\n%s", context.Stable)
	}
}

func TestWorkspaceContextPathIndexesStayStableAcrossBodyChanges(t *testing.T) {
	dir := t.TempDir()
	state := NewState(dir)
	if err := state.InitWorkspace(); err != nil {
		t.Fatalf("initialize workspace: %v", err)
	}
	outline := filepath.Join(state.SettingDir(), "outline.md")
	progress := filepath.Join(state.SettingDir(), "progress.md")
	if err := os.WriteFile(outline, []byte("outline revision one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(progress, []byte("progress revision one"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := state.WorkspaceContext()

	if err := os.WriteFile(outline, []byte("a materially different outline revision"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(progress, []byte("a materially different progress revision"), 0o644); err != nil {
		t.Fatal(err)
	}
	after := state.WorkspaceContext()
	if before != after {
		t.Fatalf("body-only edits should not rotate path indexes:\nbefore=%#v\nafter=%#v", before, after)
	}
}

func TestWorkspaceContextBoundsRecentChapterAndGroupPaths(t *testing.T) {
	dir := t.TempDir()
	state := NewState(dir)
	if err := state.InitWorkspace(); err != nil {
		t.Fatalf("initialize workspace: %v", err)
	}
	for index := 1; index <= workspaceContextChapterGroupLimit+1; index++ {
		name := fmt.Sprintf("group%02d.md", index)
		if err := os.WriteFile(filepath.Join(state.ChapterGroupDir(), name), []byte("plan"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for index := 1; index <= workspaceContextChapterLimit+1; index++ {
		name := fmt.Sprintf("ch%05d.md", index)
		if err := os.WriteFile(filepath.Join(dir, "chapters", name), []byte("prose"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	dynamic := state.WorkspaceContext().Dynamic
	for _, omitted := range []string{"group01.md", "ch00001.md"} {
		if strings.Contains(dynamic, omitted) {
			t.Fatalf("old path %q should fall outside the bounded index:\n%s", omitted, dynamic)
		}
	}
	for _, included := range []string{"group02.md", "group03.md", "ch00002.md", "ch00013.md"} {
		if !strings.Contains(dynamic, included) {
			t.Fatalf("recent path %q missing from bounded index:\n%s", included, dynamic)
		}
	}
}
