package skills

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBackendWorkspaceOverridesUserAndBuiltin(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	user := filepath.Join(root, "user")
	workspace := filepath.Join(root, "workspace")
	writeSkillFile(t, builtin, "outline", "outline", "builtin desc")
	writeSkillFile(t, user, "outline", "outline", "user desc")
	writeSkillFile(t, workspace, "outline", "outline", "workspace desc")
	writeSkillFile(t, user, "rewrite", "rewrite", "rewrite desc")

	backend := NewBackend([]Directory{
		{Scope: ScopeBuiltin, Path: builtin},
		{Scope: ScopeUser, Path: user, Writable: true},
		{Scope: ScopeWorkspace, Path: workspace, Writable: true},
	})

	list, err := backend.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("List() len = %d, want 2", len(list))
	}
	outline, err := backend.Get(ctx, "outline")
	if err != nil {
		t.Fatalf("Get(outline) error = %v", err)
	}
	if outline.Description != "workspace desc" {
		t.Fatalf("outline description = %q, want workspace desc", outline.Description)
	}

	snapshot, err := SnapshotFor(ctx, []Directory{
		{Scope: ScopeBuiltin, Path: builtin},
		{Scope: ScopeUser, Path: user, Writable: true},
		{Scope: ScopeWorkspace, Path: workspace, Writable: true},
	})
	if err != nil {
		t.Fatalf("SnapshotFor() error = %v", err)
	}
	activeByScope := map[Scope]bool{}
	for _, item := range snapshot.Skills {
		if item.Name == "outline" {
			activeByScope[item.Scope] = item.Active
		}
	}
	if !activeByScope[ScopeWorkspace] || activeByScope[ScopeUser] || activeByScope[ScopeBuiltin] {
		t.Fatalf("active scopes for outline = %#v", activeByScope)
	}
}

func TestCreateAndSaveDocument(t *testing.T) {
	ctx := context.Background()
	user := filepath.Join(t.TempDir(), "skills")
	dirs := []Directory{{Scope: ScopeUser, Path: user, Writable: true}}

	doc, err := CreateDocument(ctx, dirs, ScopeUser, "beats", "Draft beat sheets.")
	if err != nil {
		t.Fatalf("CreateDocument() error = %v", err)
	}
	if doc.Name != "beats" || !doc.Editable {
		t.Fatalf("created doc = %#v", doc)
	}

	content := `---
name: beats
description: Build chapter beat sheets.
---

# Beats

Use numbered beats.
`
	saved, err := SaveDocument(ctx, dirs, ScopeUser, "beats", content)
	if err != nil {
		t.Fatalf("SaveDocument() error = %v", err)
	}
	if saved.Description != "Build chapter beat sheets." || saved.Content != content {
		t.Fatalf("saved doc = %#v", saved)
	}
}

func TestSaveDocumentRejectsReadonlyAndMismatchedName(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	readonly := []Directory{{Scope: ScopeBuiltin, Path: filepath.Join(root, "builtin")}}
	if _, err := SaveDocument(ctx, readonly, ScopeBuiltin, "locked", DefaultContent("locked", "")); err == nil {
		t.Fatalf("SaveDocument() expected readonly error")
	}

	user := []Directory{{Scope: ScopeUser, Path: filepath.Join(root, "user"), Writable: true}}
	mismatched := DefaultContent("other", "")
	if _, err := SaveDocument(ctx, user, ScopeUser, "locked", mismatched); err == nil {
		t.Fatalf("SaveDocument() expected mismatched name error")
	}
}

func writeSkillFile(t *testing.T, root, dirName, skillName, description string) {
	t.Helper()
	dir := filepath.Join(root, dirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	content := DefaultContent(skillName, description)
	if err := os.WriteFile(filepath.Join(dir, SkillFileName), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
