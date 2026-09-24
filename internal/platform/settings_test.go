package platform

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"denova/internal/revisionfile"
)

func TestInstalledSettingsPreserveSourceAndPreviewIsolation(t *testing.T) {
	m, projectID := testManager(t)
	candidate := testCandidate(t, m, projectID, "tool", "test.settings", Plugin)
	manifest := candidate.Manifest
	manifest.Settings = &ConfigurationDeclaration{Schema: "schema.json", Defaults: "defaults.toml"}
	manifest.Distribution.Files = append(manifest.Distribution.Files, "schema.json", "defaults.toml")
	files := make(map[string][]byte)
	for name, data := range candidate.files {
		files[name] = data
	}
	files["schema.json"] = []byte(`{"type":"object","properties":{"tone":{"type":"string","x-titleKey":"tone"}},"additionalProperties":false}`)
	files["defaults.toml"] = []byte(`tone = "quiet"`)
	manifest.Locales = map[string]string{"zh-CN": "zh.json", "en-US": "en.json"}
	manifest.Distribution.Files = append(manifest.Distribution.Files, "zh.json", "en.json")
	files["zh.json"] = []byte(`{"tone":"语气"}`)
	files["en.json"] = []byte(`{"tone":"Tone"}`)
	files[Plugin.manifestFile()], _ = json.Marshal(manifest)
	configured, err := m.freeze(Plugin, files)
	if err != nil {
		t.Fatal(err)
	}
	release := testInstall(t, m, configured)
	packageManifest := filepath.Join(m.releasePath(release.Ref), Plugin.manifestFile())
	original, err := os.ReadFile(packageManifest)
	if err != nil {
		t.Fatal(err)
	}
	settings := ConfigurationInput{ExpectedRevision: revisionfile.MissingRevision, ReleaseID: release.Ref.ReleaseID, Overrides: map[string]any{"tone": "bright"}}
	if err := m.SetAvailability(context.Background(), Plugin, manifest.ID, PackageAvailability{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SavePackageConfiguration(Plugin, manifest.ID, "en-US", settings); err != nil {
		t.Fatal(err)
	}
	loaded, installed, err := m.release(release.Ref)
	if err != nil || installed.Enabled {
		t.Fatalf("settings changed availability: %#v %v", installed, err)
	}
	if loaded.Digest != release.Digest || !reflect.DeepEqual(loaded.Manifest, release.Manifest) {
		t.Fatal("settings changed release identity or manifest")
	}
	effective, err := m.settingsValues(loaded, "installed", nil)
	if err != nil || !reflect.DeepEqual(effective, map[string]any{"tone": "bright"}) {
		t.Fatalf("installed overrides: %v %v", effective, err)
	}
	override, err := m.settingsValues(loaded, "installed", map[string]any{"tone": "instance"})
	if err != nil || override["tone"] != "instance" {
		t.Fatalf("runtime override: %v %v", override, err)
	}
	preview, err := m.PreparePreview(configured.ID, release.Grants)
	if err != nil {
		t.Fatal(err)
	}
	previewValues, err := m.settingsValues(preview, "preview", nil)
	if err != nil || previewValues["tone"] != "quiet" {
		t.Fatalf("preview consumed installed settings: %v %v", previewValues, err)
	}
	settings.Overrides = map[string]any{"tone": 42}
	if _, err := m.SavePackageConfiguration(Plugin, manifest.ID, "en-US", settings); err == nil {
		t.Fatal("invalid schema value accepted")
	}
	retained, _ := m.settingsOverrides(release.Ref)
	if !reflect.DeepEqual(retained, map[string]any{"tone": "bright"}) {
		t.Fatal("failed settings write changed persisted values")
	}
	doc, _ := m.PackageConfiguration(ReleaseRef{Package: PackageRef{Kind: Plugin, ID: manifest.ID}}, "en-US")
	settings.ExpectedRevision = doc.Revision
	settings.Overrides = map[string]any{}
	if _, err := m.SavePackageConfiguration(Plugin, manifest.ID, "en-US", settings); err != nil {
		t.Fatal(err)
	}
	reset, _, _ := m.release(release.Ref)
	values, err := m.settingsValues(reset, "installed", nil)
	if err != nil || values["tone"] != "quiet" {
		t.Fatalf("reset did not restore release defaults: %v %v", values, err)
	}
	after, err := os.ReadFile(packageManifest)
	if err != nil || string(after) != string(original) {
		t.Fatal("settings changed package bytes")
	}
	settings.ReleaseID = "stale"
	if _, err := m.SavePackageConfiguration(Plugin, manifest.ID, "en-US", settings); err == nil {
		t.Fatal("stale release update accepted")
	}
}

func TestInstalledPermissionChangeRevokesRuntimeAndPreservesSave(t *testing.T) {
	m, projectID := testManager(t)
	candidate := testCandidate(t, m, projectID, "static", "test.settings-grants", Game)
	release, err := m.Install(candidate.ID, []string{"gameData", "tools.invoke"})
	if err != nil {
		t.Fatal(err)
	}
	instance, err := m.CreateInstance(CreateInstance{GameID: release.Manifest.ID, ReleaseID: release.Ref.ReleaseID, Title: "Garden"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.OpenInstance(context.Background(), instance.ID, OpenOptions{ParentOrigin: "http://127.0.0.1:15173"}); err != nil {
		t.Fatal(err)
	}
	old := m.runtimes[instance.ID]
	settings := PackagePermissions{ReleaseID: release.Ref.ReleaseID, Grants: []string{"gameData"}}
	if err := m.SetPackagePermissions(context.Background(), Game, release.Manifest.ID, settings); err != nil {
		t.Fatal(err)
	}
	if len(m.RuntimeSnapshots()) != 0 || old.ctx.Err() == nil {
		t.Fatal("revoked runtime is still active")
	}
	saved, err := m.Instance(instance.ID)
	if err != nil || !reflect.DeepEqual(saved, instance) {
		t.Fatalf("permission update changed save: %#v %v", saved, err)
	}
	settings.Grants = nil
	if err := m.SetPackagePermissions(context.Background(), Game, release.Manifest.ID, settings); err == nil {
		t.Fatal("required permission removed")
	}
}
