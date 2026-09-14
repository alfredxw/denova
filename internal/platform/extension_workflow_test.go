package platform

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSourceAndPreviewRemainUninstalled(t *testing.T) {
	m, projectID := testManager(t)
	if catalog, err := m.Catalog(); err != nil || len(catalog) != 0 {
		t.Fatalf("new manager installed extensions: %#v %v", catalog, err)
	}
	for template, kind := range map[string]Kind{"tool": Plugin, "agent": Game} {
		candidate := testCandidate(t, m, projectID, template, "test."+template, kind)
		if _, err := m.PreparePreview(candidate.ID, candidate.Manifest.Permissions.Required); err != nil {
			t.Fatal(err)
		}
	}
	// Importing source packages, including isolated preview preparation,
	// must never make them available to writing or game consumers on restart.
	reopened := New(m.root, m.registry)
	for _, manager := range []*Manager{m, reopened} {
		if catalog, err := manager.Catalog(); err != nil || len(catalog) != 0 {
			t.Fatalf("source entered installed catalog: %#v %v", catalog, err)
		}
		if preferences, err := manager.GamePreferences(); err != nil || preferences.DefaultGameID != BuiltinGameID {
			t.Fatalf("source changed default game: %#v %v", preferences, err)
		}
	}
	if sources, err := reopened.Developments(); err != nil || len(sources) != 2 {
		t.Fatalf("source workspaces were lost: %#v %v", sources, err)
	}
}

func TestGameDefaultAndRenamePreserveSaveBindings(t *testing.T) {
	m, projectID := testManager(t)
	candidate := testCandidate(t, m, projectID, "static", "test.default", Game)
	release := testInstall(t, m, candidate)
	if err := m.SetDefaultGame(release.Manifest.ID); err != nil {
		t.Fatal(err)
	}
	preferences, err := m.GamePreferences()
	if err != nil || preferences.DefaultGameID != release.Manifest.ID {
		t.Fatalf("default: %#v %v", preferences, err)
	}
	instance, err := m.CreateInstance(CreateInstance{GameID: release.Manifest.ID, ReleaseID: release.Ref.ReleaseID, Title: "Original", ProjectID: projectID})
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := m.RenameInstance(instance.ID, "Renamed")
	if err != nil {
		t.Fatal(err)
	}
	instance.Title = "Renamed"
	if !reflect.DeepEqual(renamed, instance) {
		t.Fatalf("rename changed save identity: %#v", renamed)
	}
	if err := m.SetAvailability(context.Background(), Game, release.Manifest.ID, PackageAvailability{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	preferences, err = m.GamePreferences()
	if err != nil || preferences.DefaultGameID != BuiltinGameID {
		t.Fatalf("fallback: %#v %v", preferences, err)
	}
	if err := m.SetDefaultGame(release.Manifest.ID); err == nil {
		t.Fatal("disabled game accepted as default")
	}
	if saved, err := m.Instance(instance.ID); err != nil || !reflect.DeepEqual(saved, instance) {
		t.Fatalf("preference changed save: %#v %v", saved, err)
	}
}

func TestExtensionImportDetectsKindAndOpenedSource(t *testing.T) {
	m, projectID := testManager(t)
	development := testSource(t, m, projectID, ".", "static", "test.detect", Game)
	_, directory, err := m.Development(development.ID)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := m.PreviewExtensionDirectory(directory)
	if err != nil || candidate.Kind != Game {
		t.Fatalf("directory detection: %#v %v", candidate, err)
	}
	var archive bytes.Buffer
	if err := m.ExportCandidate(candidate.ID, &archive); err != nil {
		t.Fatal(err)
	}
	imported, err := m.PreviewExtensionZIP(archive.Bytes())
	if err != nil || imported.Digest != candidate.Digest || imported.Kind != Game {
		t.Fatalf("archive detection: %#v %v", imported, err)
	}
	items, err := m.Developments()
	if err != nil || len(items) != 1 || items[0].RelativePath != "." {
		t.Fatalf("source discovery: %#v %v", items, err)
	}
	manifest, err := os.ReadFile(filepath.Join(directory, "denova.game.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "denova.plugin.json"), manifest, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.PreviewExtensionDirectory(directory); err == nil {
		t.Fatal("ambiguous root manifests accepted")
	}
}

func TestDisableKeepsCurrentGameAndBlocksNewStarts(t *testing.T) {
	m, projectID := testManager(t)
	release := testInstall(t, m, testCandidate(t, m, projectID, "static", "test.availability", Game))
	request := CreateInstance{GameID: release.Manifest.ID, ReleaseID: release.Ref.ReleaseID, Title: "Journey", ProjectID: projectID}
	instance, err := m.CreateInstance(request)
	if err != nil {
		t.Fatal(err)
	}
	options := OpenOptions{ParentOrigin: "http://127.0.0.1:15173"}
	opened, err := m.OpenInstance(context.Background(), instance.ID, options)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.SetAvailability(context.Background(), Game, release.Manifest.ID, PackageAvailability{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	resumed, err := m.OpenInstance(context.Background(), instance.ID, options)
	if err != nil || resumed.Connection != opened.Connection {
		t.Fatalf("active journey was interrupted: %#v %v", resumed, err)
	}
	if _, err := m.CreateInstance(request); err == nil {
		t.Fatal("disabled game accepted a new journey")
	}
	if err := m.Stop(context.Background(), instance.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.OpenInstance(context.Background(), instance.ID, options); err == nil {
		t.Fatal("disabled game restarted")
	}
	if saved, err := m.Instance(instance.ID); err != nil || saved.ReleaseID != release.Ref.ReleaseID {
		t.Fatalf("disabled game lost its save binding: %#v %v", saved, err)
	}
	if err := m.SetAvailability(context.Background(), Game, release.Manifest.ID, PackageAvailability{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.OpenInstance(context.Background(), instance.ID, options); err != nil {
		t.Fatal(err)
	}
}

func TestDevelopmentUsesProjectRoot(t *testing.T) {
	m, projectID := testManager(t)
	development, err := m.CreateDevelopment(CreateDevelopment{Kind: Game, ProjectID: projectID, RelativePath: ".", ID: "test.root", Name: LocalizedText{Chinese: "测试", English: "Test"}})
	if err != nil {
		t.Fatal(err)
	}
	if development.ProjectID != projectID || development.RelativePath != "." {
		t.Fatalf("unexpected source binding: %#v", development)
	}
	if _, err := m.CheckDevelopment(development.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreateDevelopment(CreateDevelopment{Kind: Game, ProjectID: projectID, RelativePath: ".", ID: "test.overwrite", Name: LocalizedText{Chinese: "覆盖", English: "Overwrite"}}); err == nil {
		t.Fatal("template overwrote existing source")
	}
}
