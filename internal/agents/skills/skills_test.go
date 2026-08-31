package skills

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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

func TestReadAndSaveSkillFile(t *testing.T) {
	ctx := context.Background()
	user := filepath.Join(t.TempDir(), "skills")
	dirs := []Directory{{Scope: ScopeUser, Path: user, Writable: true}}
	writeSkillFile(t, user, "outline", "outline", "user desc")
	refPath := filepath.Join(user, "outline", "references", "style.md")
	if err := os.MkdirAll(filepath.Dir(refPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(refPath, []byte("# Style\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	doc, err := ReadDocument(ctx, dirs, ScopeUser, "outline")
	if err != nil {
		t.Fatalf("ReadDocument() error = %v", err)
	}
	var foundRef bool
	for _, file := range doc.Files {
		if file.Path == "references/style.md" && file.Editable && !file.Entry {
			foundRef = true
		}
	}
	if !foundRef {
		t.Fatalf("reference file missing from document files: %#v", doc.Files)
	}

	fileDoc, err := ReadSkillFile(ctx, dirs, ScopeUser, "outline", "references/style.md")
	if err != nil {
		t.Fatalf("ReadSkillFile() error = %v", err)
	}
	if fileDoc.Content != "# Style\n" || fileDoc.File.Path != "references/style.md" {
		t.Fatalf("file document = %#v", fileDoc)
	}

	saved, err := SaveSkillFile(ctx, dirs, ScopeUser, "outline", "references/style.md", "# Updated\n")
	if err != nil {
		t.Fatalf("SaveSkillFile() error = %v", err)
	}
	if saved.Content != "# Updated\n" {
		t.Fatalf("saved content = %q", saved.Content)
	}
	data, err := os.ReadFile(refPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# Updated\n" {
		t.Fatalf("file content = %q", string(data))
	}
}

func TestCreateUpdateAndDeleteSkillReferenceWithRevision(t *testing.T) {
	ctx := context.Background()
	user := filepath.Join(t.TempDir(), "skills")
	dirs := []Directory{{Scope: ScopeUser, Path: user, Writable: true}}
	writeSkillFile(t, user, "outline", "outline", "user desc")

	created, err := CreateSkillFile(ctx, dirs, ScopeUser, "outline", "references/style.md", "# Style\n")
	if err != nil {
		t.Fatal(err)
	}
	if created.Revision == "" || created.File.Path != "references/style.md" {
		t.Fatalf("created reference = %#v", created)
	}
	if _, err := CreateSkillFile(ctx, dirs, ScopeUser, "outline", "references/style.md", "overwrite"); err == nil {
		t.Fatal("reference create overwrote an existing file")
	}
	updated, err := SaveSkillFileIfRevision(ctx, dirs, ScopeUser, "outline", "references/style.md", "# Updated\n", created.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision == created.Revision {
		t.Fatal("reference update did not advance revision")
	}
	if _, err := DeleteSkillFileIfRevision(ctx, dirs, ScopeUser, "outline", "references/style.md", created.Revision); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale delete error = %v", err)
	}
	deleted, err := DeleteSkillFileIfRevision(ctx, dirs, ScopeUser, "outline", "references/style.md", updated.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Path != "references/style.md" || deleted.Revision != updated.Revision {
		t.Fatalf("delete receipt = %#v", deleted)
	}
	if _, err := ReadSkillFile(ctx, dirs, ScopeUser, "outline", "references/style.md"); !os.IsNotExist(err) {
		t.Fatalf("deleted reference read error = %v", err)
	}
}

func TestSaveDocumentIfRevisionRejectsExternalUpdate(t *testing.T) {
	ctx := context.Background()
	user := filepath.Join(t.TempDir(), "skills")
	dirs := []Directory{{Scope: ScopeUser, Path: user, Writable: true}}
	writeSkillFile(t, user, "outline", "outline", "original")

	doc, err := ReadDocument(ctx, dirs, ScopeUser, "outline")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Revision == "" {
		t.Fatal("ReadDocument() revision is empty")
	}
	path := filepath.Join(user, "outline", SkillFileName)
	external := DefaultContent("outline", "external")
	if err := os.WriteFile(path, []byte(external), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = SaveDocumentIfRevision(ctx, dirs, ScopeUser, "outline", DefaultContent("outline", "stale editor"), doc.Revision)
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("SaveDocumentIfRevision() error = %v, want ErrRevisionConflict", err)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != external {
		t.Fatalf("stale save overwrote external content: %q", data)
	}
}

func TestSaveSkillFileIfRevisionRejectsExternalUpdate(t *testing.T) {
	ctx := context.Background()
	user := filepath.Join(t.TempDir(), "skills")
	dirs := []Directory{{Scope: ScopeUser, Path: user, Writable: true}}
	writeSkillFile(t, user, "outline", "outline", "original")
	refPath := filepath.Join(user, "outline", "references", "style.md")
	if err := os.MkdirAll(filepath.Dir(refPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(refPath, []byte("# Original\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	doc, err := ReadSkillFile(ctx, dirs, ScopeUser, "outline", "references/style.md")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Revision == "" {
		t.Fatal("ReadSkillFile() revision is empty")
	}
	if err := os.WriteFile(refPath, []byte("# External\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = SaveSkillFileIfRevision(ctx, dirs, ScopeUser, "outline", "references/style.md", "# Stale\n", doc.Revision)
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("SaveSkillFileIfRevision() error = %v, want ErrRevisionConflict", err)
	}
	data, readErr := os.ReadFile(refPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "# External\n" {
		t.Fatalf("stale save overwrote external content: %q", data)
	}
}

func TestReadAndSaveSkillFileRejectSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires optional Windows symlink privileges")
	}
	ctx := context.Background()
	root := t.TempDir()
	userDir := filepath.Join(root, "skills")
	dirs := []Directory{{Scope: ScopeUser, Path: userDir, Writable: true}}
	skillDir := filepath.Join(userDir, "outline")
	if err := os.MkdirAll(filepath.Join(skillDir, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, SkillFileName), []byte("---\nname: outline\ndescription: test\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	outsideDir := filepath.Join(root, "outside")
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outsideFile := filepath.Join(outsideDir, "secret.md")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(skillDir, "references", "file-link.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(skillDir, "directory-link")); err != nil {
		t.Fatal(err)
	}

	for _, filePath := range []string{"references/file-link.md", "directory-link/secret.md"} {
		if _, err := ReadSkillFile(ctx, dirs, ScopeUser, "outline", filePath); err == nil {
			t.Fatalf("ReadSkillFile(%q) should reject a symlink that escapes the scope root", filePath)
		}
		if _, err := SaveSkillFile(ctx, dirs, ScopeUser, "outline", filePath, "changed"); err == nil {
			t.Fatalf("SaveSkillFile(%q) should reject a symlink that escapes the scope root", filePath)
		}
	}
	data, err := os.ReadFile(outsideFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "outside" {
		t.Fatalf("outside file was modified through a symlink: %q", data)
	}

	if err := os.Remove(filepath.Join(skillDir, SkillFileName)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(skillDir, SkillFileName)); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveDocument(ctx, dirs, ScopeUser, "outline", "---\nname: outline\ndescription: changed\n---\n"); err == nil {
		t.Fatal("SaveDocument should reject a symlinked SKILL.md entry")
	}
	data, err = os.ReadFile(outsideFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "outside" {
		t.Fatalf("outside entry file was modified through SKILL.md: %q", data)
	}
}

func TestSkillFileRejectsTraversalAndEntrySave(t *testing.T) {
	ctx := context.Background()
	user := filepath.Join(t.TempDir(), "skills")
	dirs := []Directory{{Scope: ScopeUser, Path: user, Writable: true}}
	writeSkillFile(t, user, "outline", "outline", "user desc")

	if _, err := ReadSkillFile(ctx, dirs, ScopeUser, "outline", "../outside.md"); err == nil {
		t.Fatalf("ReadSkillFile() expected traversal error")
	}
	if _, err := SaveSkillFile(ctx, dirs, ScopeUser, "outline", SkillFileName, "bad"); err == nil {
		t.Fatalf("SaveSkillFile(%s) expected error", SkillFileName)
	}
}

func TestCreateAndSaveDocument(t *testing.T) {
	ctx := context.Background()
	user := filepath.Join(t.TempDir(), "skills")
	dirs := []Directory{{Scope: ScopeUser, Path: user, Writable: true}}

	doc, err := CreateDocumentWithMetadata(ctx, dirs, ScopeUser, "beats", CreateMetadata{
		Description:  "Draft beat sheets.",
		Agents:       []string{"ide", "config_manager"},
		Category:     CategoryWriting,
		Capabilities: []string{CapabilityWritingWorkflow},
	})
	if err != nil {
		t.Fatalf("CreateDocument() error = %v", err)
	}
	if doc.Name != "beats" || !doc.Editable || doc.Agent != "ide,config_manager" || doc.Category != CategoryWriting || len(doc.Capabilities) != 1 || doc.Capabilities[0] != CapabilityWritingWorkflow {
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

func TestCreateDocumentWithContentNeverOverwritesExistingSkill(t *testing.T) {
	ctx := context.Background()
	user := filepath.Join(t.TempDir(), "skills")
	dirs := []Directory{{Scope: ScopeUser, Path: user, Writable: true}}
	content := DefaultContent("beats", "first")
	if _, err := CreateDocumentWithContent(ctx, dirs, ScopeUser, "beats", content); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateDocumentWithContent(ctx, dirs, ScopeUser, "beats", DefaultContent("beats", "second")); err == nil {
		t.Fatal("custom document create overwrote an existing Skill")
	}
	doc, err := ReadDocument(ctx, dirs, ScopeUser, "beats")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Description != "first" {
		t.Fatalf("existing Skill changed: %#v", doc)
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
	refPath := filepath.Join(user, "outline", "references", "style.md")
	if err := os.MkdirAll(filepath.Dir(refPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(refPath, []byte("# Style\n"), 0o644); err != nil {
		t.Fatal(err)
	}
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
	if data, err := os.ReadFile(filepath.Join(workspace, "beats", "references", "style.md")); err != nil || string(data) != "# Style\n" {
		t.Fatalf("moved reference file = %q, err=%v", string(data), err)
	}
}

func TestSaveDocumentAsCopiesReadonlySkillDirectoryForOverride(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	user := filepath.Join(root, "user")
	writeSkillFile(t, builtin, "outline", "outline", "builtin outline")
	refPath := filepath.Join(builtin, "outline", "references", "style.md")
	if err := os.MkdirAll(filepath.Dir(refPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(refPath, []byte("# Builtin Style\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirs := []Directory{
		{Scope: ScopeBuiltin, Path: builtin},
		{Scope: ScopeUser, Path: user, Writable: true},
	}

	doc, err := SaveDocumentAs(ctx, dirs, ScopeBuiltin, "outline", ScopeUser, "outline", DefaultContent("outline", "user override"))
	if err != nil {
		t.Fatalf("SaveDocumentAs() error = %v", err)
	}
	if doc.Scope != ScopeUser || doc.Description != "user override" {
		t.Fatalf("override doc = %#v", doc)
	}
	if _, err := os.Stat(filepath.Join(builtin, "outline", SkillFileName)); err != nil {
		t.Fatalf("builtin source should remain: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(user, "outline", "references", "style.md")); err != nil || string(data) != "# Builtin Style\n" {
		t.Fatalf("copied reference file = %q, err=%v", string(data), err)
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
	writeSkillFileForAgents(t, root, "writing-only", "writing-only", "writing desc", "ide")
	writeSkillFileForAgents(t, root, "story-helper", "story-helper", "story desc", "config_manager,interactive_story")
	writeSkillFileForAgents(t, root, "general", "general", "general desc", "")

	backend := NewAgentBackend([]Directory{{Scope: ScopeUser, Path: root, Writable: true}}, "interactive_story", nil)
	list, err := backend.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	got := skillNames(list)
	if len(got) != 2 || !got["story-helper"] || !got["general"] {
		t.Fatalf("interactive_story skills = %#v", got)
	}
	if _, err := backend.Get(ctx, "writing-only"); err == nil {
		t.Fatalf("Get(writing-only) should be filtered out for interactive_story")
	}

	overrideBackend := NewAgentBackend([]Directory{{Scope: ScopeUser, Path: root, Writable: true}}, "interactive_story", map[string]bool{
		"writing-only": true,
		"story-helper": false,
	})
	overrideList, err := overrideBackend.List(ctx)
	if err != nil {
		t.Fatalf("override List() error = %v", err)
	}
	overrideGot := skillNames(overrideList)
	if len(overrideGot) != 2 || !overrideGot["writing-only"] || !overrideGot["general"] || overrideGot["story-helper"] {
		t.Fatalf("override interactive_story skills = %#v", overrideGot)
	}
}

func TestAgentBackendExplicitPolicyAdmitsOnlyPinnedSkills(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeSkillFileForAgents(t, root, "writing", "writing", "Writing", "ide")
	writeSkillFileForAgents(t, root, "general", "general", "General", "")
	writeSkillFileForAgents(t, root, "pinned", "pinned", "Pinned", "interactive_story")

	backend := NewAgentBackendWithPolicy(
		[]Directory{{Scope: ScopeUser, Path: root, Writable: true}},
		"ide",
		map[string]bool{"pinned": true, "general": false},
		true,
	)
	list, err := backend.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := skillNames(list)
	if len(got) != 1 || !got["pinned"] {
		t.Fatalf("explicit-only Skills = %#v", got)
	}
}

func TestAgentBackendExposesConfigurationSkillToProjectAgents(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	workspace := filepath.Join(root, "workspace")
	writeSkillFileForAgents(t, builtin, "configuration", "configuration", "all config resources", "general,ide")
	writeSkillFileForAgents(t, builtin, "ide-only", "ide-only", "ide only", "ide")
	writeSkillFileForAgents(t, workspace, "configuration", "configuration", "workspace config routing", "general,ide")

	for _, agentKind := range []string{"general", "ide"} {
		backend := NewAgentBackend([]Directory{
			{Scope: ScopeBuiltin, Path: builtin},
			{Scope: ScopeWorkspace, Path: workspace, Writable: true},
		}, agentKind, nil)
		list, err := backend.List(ctx)
		if err != nil {
			t.Fatalf("%s List() error = %v", agentKind, err)
		}
		got := skillNames(list)
		if !got["configuration"] {
			t.Fatalf("%s skills = %#v, want configuration", agentKind, got)
		}
		skill, err := backend.Get(ctx, "configuration")
		if err != nil {
			t.Fatalf("%s Get(configuration) error = %v", agentKind, err)
		}
		if skill.Description != "workspace config routing" {
			t.Fatalf("%s active configuration description = %q, want workspace override", agentKind, skill.Description)
		}
	}

	overrideBackend := NewAgentBackend([]Directory{{Scope: ScopeBuiltin, Path: builtin}}, "general", map[string]bool{
		"configuration": false,
	})
	overrideList, err := overrideBackend.List(ctx)
	if err != nil {
		t.Fatalf("override List() error = %v", err)
	}
	overrideGot := skillNames(overrideList)
	if len(overrideGot) != 0 {
		t.Fatalf("override general skills = %#v", overrideGot)
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
	if rec.skill.Name != "beats" || rec.skill.Description != "Line one:\n- keep as text\nkey: value" || rec.skill.Category != CategoryGeneral {
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

func skillNames(list []FrontMatter) map[string]bool {
	out := map[string]bool{}
	for _, item := range list {
		out[item.Name] = true
	}
	return out
}
