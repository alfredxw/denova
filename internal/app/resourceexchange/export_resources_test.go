package resourceexchange

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"denova/internal/agents/skills"
	"denova/internal/app/resourcecatalog"
	imagepreset "denova/internal/image/preset"
	"denova/internal/project"
)

func TestExportResourcesHidesUnmodifiedBuiltinPresets(t *testing.T) {
	ctx := context.Background()
	s := testService(t)
	assertPresets := func(want ...ExportResource) {
		t.Helper()
		resources, err := s.ExportResources(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		got := slices.DeleteFunc(resources, func(item ExportResource) bool {
			return !strings.HasPrefix(item.Local.Kind, "preset.")
		})
		if !slices.Equal(got, want) {
			t.Fatalf("exportable presets = %+v, want %+v", got, want)
		}
	}
	// A fresh catalog contains all six builtin preset kinds, none user-authored.
	assertPresets()
	builtin, err := s.catalog.ImagePreset(imagepreset.DefaultID)
	if err != nil {
		t.Fatal(err)
	}
	builtin.Name = "Edited builtin image"
	if _, err := s.catalog.UpdateImagePreset(builtin.ID, builtin, builtin.Revision); err != nil {
		t.Fatal(err)
	}
	edited := ExportResource{Local: LocalRef{Kind: "preset.image", Scope: "global", ID: builtin.ID}, Name: builtin.Name, Description: builtin.Description}
	assertPresets(edited)
	copy := builtin
	copy.ID, copy.Name = "", "Custom image"
	custom, err := s.catalog.CreateImagePreset(copy)
	if err != nil {
		t.Fatal(err)
	}
	created := ExportResource{Local: LocalRef{Kind: "preset.image", Scope: "global", ID: custom.ID}, Name: custom.Name, Description: custom.Description}
	assertPresets(edited, created)
	// Deleting an overridden builtin restores its shipped definition.
	if err := s.catalog.DeleteImagePreset(builtin.ID); err != nil {
		t.Fatal(err)
	}
	assertPresets(created)
}

type exportSkillDirectories []skills.Directory

func (dirs exportSkillDirectories) SkillDirectories(resourcecatalog.SkillTarget) ([]skills.Directory, error) {
	return dirs, nil
}

func TestExportResourcesHidesBuiltinSkillsAndKeepsUserCopies(t *testing.T) {
	ctx := context.Background()
	s := testService(t)
	// Keep the shared library and preferences isolated from the host account.
	home := t.TempDir()
	t.Setenv("HOME", home)
	workspace := filepath.Join(s.root, "projects", "export")
	if err := os.MkdirAll(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	book, err := s.registry.Add(workspace, project.TypeBook, "Export")
	if err != nil {
		t.Fatal(err)
	}
	dirs := skills.NewDirectories(t.TempDir(), s.root, workspace)
	s.catalog = resourcecatalog.NewService(s.root, exportSkillDirectories(dirs))
	for _, dir := range dirs {
		name := string(dir.Scope) + "-skill"
		if err := writeFiles(filepath.Join(dir.Path, name), map[string][]byte{
			skills.SkillFileName: []byte(skills.DefaultContent(name, "Export fixture")),
		}); err != nil {
			t.Fatal(err)
		}
	}
	assertSkills := func(want ...LocalRef) {
		t.Helper()
		resources, err := s.ExportResources(ctx, book.ID)
		if err != nil {
			t.Fatal(err)
		}
		var got []LocalRef
		for _, item := range resources {
			if item.Local.Kind == "skill" {
				got = append(got, item.Local)
			}
		}
		if len(got) != len(want) {
			t.Fatalf("exportable Skills = %+v, want %+v", got, want)
		}
		for _, ref := range want {
			if !slices.Contains(got, ref) {
				t.Fatalf("exportable Skills = %+v, missing %+v", got, ref)
			}
		}
	}
	want := []LocalRef{
		{Kind: "skill", Scope: "user", ID: "user-skill"},
		{Kind: "skill", Scope: "workspace", ProjectID: book.ID, ID: "workspace-skill"},
		{Kind: "skill", Scope: "shared", ID: "shared-skill"},
	}
	assertSkills(want...)
	// Availability preferences do not turn builtin content into user content.
	if err := skills.SetLibraryEnabled(ctx, dirs, skills.ScopeBuiltin, "builtin-skill", false); err != nil {
		t.Fatal(err)
	}
	if err := skills.SetLibraryEnabled(ctx, dirs, skills.ScopeUser, "user-skill", false); err != nil {
		t.Fatal(err)
	}
	assertSkills(want...)
	if _, err := skills.SaveDocumentAs(ctx, dirs, skills.ScopeBuiltin, "builtin-skill", skills.ScopeUser, "builtin-skill", skills.DefaultContent("builtin-skill", "Edited copy")); err != nil {
		t.Fatal(err)
	}
	assertSkills(append(want, LocalRef{Kind: "skill", Scope: "user", ID: "builtin-skill"})...)
	if err := skills.DeleteDocument(ctx, dirs, skills.ScopeUser, "builtin-skill"); err != nil {
		t.Fatal(err)
	}
	assertSkills(want...)
}
