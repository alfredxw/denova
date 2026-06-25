package skills

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	einoskill "github.com/cloudwego/eino/adk/middlewares/skill"
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

func TestReadAndSaveDocumentReportActiveScope(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	user := filepath.Join(root, "user")
	workspace := filepath.Join(root, "workspace")
	writeSkillFile(t, user, "outline", "outline", "user desc")
	writeSkillFile(t, workspace, "outline", "outline", "workspace desc")
	dirs := []Directory{
		{Scope: ScopeUser, Path: user, Writable: true},
		{Scope: ScopeWorkspace, Path: workspace, Writable: true},
	}

	userDoc, err := ReadDocument(ctx, dirs, ScopeUser, "outline")
	if err != nil {
		t.Fatalf("ReadDocument(user) error = %v", err)
	}
	workspaceDoc, err := ReadDocument(ctx, dirs, ScopeWorkspace, "outline")
	if err != nil {
		t.Fatalf("ReadDocument(workspace) error = %v", err)
	}
	if userDoc.Active || !workspaceDoc.Active {
		t.Fatalf("active status mismatch: user=%v workspace=%v", userDoc.Active, workspaceDoc.Active)
	}

	savedUser, err := SaveDocument(ctx, dirs, ScopeUser, "outline", DefaultContent("outline", "updated user desc"))
	if err != nil {
		t.Fatalf("SaveDocument(user) error = %v", err)
	}
	if savedUser.Active {
		t.Fatalf("saved overridden user document should remain inactive: %#v", savedUser)
	}
}

func TestCreateAndSaveDocument(t *testing.T) {
	ctx := context.Background()
	user := filepath.Join(t.TempDir(), "skills")
	dirs := []Directory{{Scope: ScopeUser, Path: user, Writable: true}}

	doc, err := CreateDocument(ctx, dirs, ScopeUser, "beats", "Draft beat sheets.", "ide", "config_manager")
	if err != nil {
		t.Fatalf("CreateDocument() error = %v", err)
	}
	if doc.Name != "beats" || !doc.Editable || doc.Agent != "ide,config_manager" {
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

func TestSaveDocumentCreatesWorkspaceOverrideForBuiltinSkill(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	workspace := filepath.Join(root, "workspace", ".nova", "skills")
	writeSkillFile(t, builtin, "outline", "outline", "builtin outline")
	dirs := []Directory{
		{Scope: ScopeBuiltin, Path: builtin},
		{Scope: ScopeWorkspace, Path: workspace, Writable: true},
	}

	content := DefaultContent("outline", "workspace outline")
	doc, err := SaveDocument(ctx, dirs, ScopeWorkspace, "outline", content)
	if err != nil {
		t.Fatalf("SaveDocument(workspace override) error = %v", err)
	}
	if doc.Scope != ScopeWorkspace || !doc.Active || !doc.Editable {
		t.Fatalf("workspace override doc = %#v", doc)
	}
	if _, err := os.Stat(filepath.Join(workspace, "outline", SkillFileName)); err != nil {
		t.Fatalf("workspace override file missing: %v", err)
	}
	backend := NewBackend(dirs)
	active, err := backend.Get(ctx, "outline")
	if err != nil {
		t.Fatalf("Get(outline) error = %v", err)
	}
	if active.Description != "workspace outline" {
		t.Fatalf("active outline description = %q, want workspace outline", active.Description)
	}
}

func TestSaveDocumentAsRenamesAndMovesEditableSkill(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	user := filepath.Join(root, "user")
	workspace := filepath.Join(root, "workspace")
	writeSkillFile(t, user, "outline", "outline", "user outline")
	dirs := []Directory{
		{Scope: ScopeUser, Path: user, Writable: true},
		{Scope: ScopeWorkspace, Path: workspace, Writable: true},
	}

	doc, err := SaveDocumentAs(ctx, dirs, ScopeUser, "outline", ScopeWorkspace, "beats", DefaultContent("beats", "workspace beats"))
	if err != nil {
		t.Fatalf("SaveDocumentAs() error = %v", err)
	}
	if doc.Scope != ScopeWorkspace || doc.Name != "beats" || !doc.Active || !doc.Editable {
		t.Fatalf("moved doc = %#v", doc)
	}
	if _, err := os.Stat(filepath.Join(user, "outline", SkillFileName)); !os.IsNotExist(err) {
		t.Fatalf("old skill should be removed, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "beats", SkillFileName)); err != nil {
		t.Fatalf("moved skill missing: %v", err)
	}
}

func TestSaveDocumentAsRejectsExistingTarget(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	user := filepath.Join(root, "user")
	workspace := filepath.Join(root, "workspace")
	writeSkillFile(t, user, "outline", "outline", "user outline")
	writeSkillFile(t, workspace, "beats", "beats", "workspace beats")
	dirs := []Directory{
		{Scope: ScopeUser, Path: user, Writable: true},
		{Scope: ScopeWorkspace, Path: workspace, Writable: true},
	}

	if _, err := SaveDocumentAs(ctx, dirs, ScopeUser, "outline", ScopeWorkspace, "beats", DefaultContent("beats", "new beats")); err == nil {
		t.Fatalf("SaveDocumentAs() expected conflict error")
	}
	if _, err := os.Stat(filepath.Join(user, "outline", SkillFileName)); err != nil {
		t.Fatalf("source skill should remain after conflict: %v", err)
	}
}

func TestAgentBackendFiltersByAgentFrontmatterAndOverrides(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeSkillFileForAgents(t, root, "outline", "outline", "outline desc", "ide")
	writeSkillFileForAgents(t, root, "lore-init", "lore-init", "lore desc", "config_manager,interactive_story")
	writeSkillFileForAgents(t, root, "general", "general", "general desc", "")

	backend := NewAgentBackend([]Directory{{Scope: ScopeUser, Path: root, Writable: true}}, "interactive_story", nil)
	list, err := backend.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	got := skillNames(list)
	if len(got) != 2 || !got["lore-init"] || !got["general"] {
		t.Fatalf("interactive_story skills = %#v", got)
	}
	if _, err := backend.Get(ctx, "outline"); err == nil {
		t.Fatalf("Get(outline) should be filtered out for interactive_story")
	}

	overrideBackend := NewAgentBackend([]Directory{{Scope: ScopeUser, Path: root, Writable: true}}, "interactive_story", map[string]bool{
		"outline":   true,
		"lore-init": false,
	})
	overrideList, err := overrideBackend.List(ctx)
	if err != nil {
		t.Fatalf("override List() error = %v", err)
	}
	overrideGot := skillNames(overrideList)
	if len(overrideGot) != 2 || !overrideGot["outline"] || !overrideGot["general"] || overrideGot["lore-init"] {
		t.Fatalf("override interactive_story skills = %#v", overrideGot)
	}
}

func TestAgentBackendExposesConfigManagerBuiltinSkills(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	workspace := filepath.Join(root, "workspace")
	writeSkillFileForAgents(t, builtin, "automation-config", "automation-config", "automation config", "config_manager")
	writeSkillFileForAgents(t, builtin, "story-memory-config", "story-memory-config", "story memory config", "config_manager")
	writeSkillFileForAgents(t, builtin, "ide-only", "ide-only", "ide only", "ide")
	writeSkillFileForAgents(t, workspace, "automation-config", "automation-config", "workspace automation config", "config_manager")

	backend := NewAgentBackend([]Directory{
		{Scope: ScopeBuiltin, Path: builtin},
		{Scope: ScopeWorkspace, Path: workspace, Writable: true},
	}, "config_manager", nil)
	list, err := backend.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	got := skillNames(list)
	if len(got) != 2 || !got["automation-config"] || !got["story-memory-config"] || got["ide-only"] {
		t.Fatalf("config_manager skills = %#v", got)
	}
	skill, err := backend.Get(ctx, "automation-config")
	if err != nil {
		t.Fatalf("Get(automation-config) error = %v", err)
	}
	if skill.Description != "workspace automation config" {
		t.Fatalf("active automation-config description = %q, want workspace override", skill.Description)
	}

	overrideBackend := NewAgentBackend([]Directory{{Scope: ScopeBuiltin, Path: builtin}}, "config_manager", map[string]bool{
		"automation-config": false,
	})
	overrideList, err := overrideBackend.List(ctx)
	if err != nil {
		t.Fatalf("override List() error = %v", err)
	}
	overrideGot := skillNames(overrideList)
	if overrideGot["automation-config"] || !overrideGot["story-memory-config"] {
		t.Fatalf("override config_manager skills = %#v", overrideGot)
	}
}

func TestDefaultContentEscapesFrontmatterDescription(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "beats", SkillFileName)
	content := DefaultContent("beats", "Line one:\n- keep as text\nkey: value")

	rec, err := parseRecord(ctx, Directory{Scope: ScopeUser, Path: filepath.Dir(filepath.Dir(path)), Writable: true}, path, content)
	if err != nil {
		t.Fatalf("parseRecord(DefaultContent()) error = %v\ncontent:\n%s", err, content)
	}
	if rec.skill.Name != "beats" || rec.skill.Description != "Line one:\n- keep as text\nkey: value" {
		t.Fatalf("parsed frontmatter = %#v", rec.skill.FrontMatter)
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
	writeSkillFileForAgents(t, root, dirName, skillName, description, "")
}

func writeSkillFileForAgents(t *testing.T, root, dirName, skillName, description, agents string) {
	t.Helper()
	dir := filepath.Join(root, dirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	var content string
	if agents == "" {
		content = DefaultContent(skillName, description)
	} else {
		content = DefaultContent(skillName, description, agents)
	}
	if err := os.WriteFile(filepath.Join(dir, SkillFileName), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func skillNames(list []einoskill.FrontMatter) map[string]bool {
	out := map[string]bool{}
	for _, item := range list {
		out[item.Name] = true
	}
	return out
}
