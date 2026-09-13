package platform

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestAddingSettingsPreservesUnconfiguredPinnedReleases(t *testing.T) {
	for _, kind := range []Kind{Game, Plugin} {
		t.Run(string(kind), func(t *testing.T) {
			m, projectID := testManager(t)
			fixture := "static"
			if kind == Plugin {
				fixture = "tool"
			}
			candidate := testCandidate(t, m, projectID, fixture, "test.new-settings", kind)
			manifest := candidate.Manifest
			manifest.Settings = nil
			manifest.Version = "0.9.0"
			files := map[string][]byte{}
			for name, data := range candidate.files {
				files[name] = data
			}
			files[kind.manifestFile()], _ = json.Marshal(manifest)
			previous, err := m.freeze(kind, files)
			if err != nil {
				t.Fatal(err)
			}
			old := testInstall(t, m, previous)
			options := OpenOptions{ParentOrigin: "http://127.0.0.1:15173"}
			var instanceID string
			var plugin ActivatePlugin
			if kind == Game {
				instance, err := m.CreateInstance(CreateInstance{Title: "Existing story", GameID: old.Manifest.ID, ReleaseID: old.Ref.ReleaseID})
				if err != nil {
					t.Fatal(err)
				}
				instanceID = instance.ID
			} else {
				plugin = ActivatePlugin{PluginID: old.Manifest.ID, ReleaseID: old.Ref.ReleaseID, Scope: Scope{Kind: "project", ProjectID: projectID}, OpenOptions: options}
				runtime, err := m.ActivatePlugin(context.Background(), plugin)
				if err != nil {
					t.Fatal(err)
				}
				instanceID = runtime.ID
			}
			next := testInstall(t, m, candidate)
			doc, err := m.PackageConfiguration(kind, next.Manifest.ID, "en-US")
			if err != nil {
				t.Fatal(err)
			}
			toml := "[display]\nvariant = 'full'\n"
			if kind == Plugin {
				toml = "enabled = true"
			}
			saved, err := m.SavePackageConfiguration(kind, next.Manifest.ID, "en-US", ConfigurationInput{ReleaseID: next.Ref.ReleaseID, ExpectedRevision: doc.Revision, Format: "toml", TOML: toml})
			if err != nil {
				t.Fatalf("adding settings blocked by a release without a declaration: %v", err)
			}
			values, err := m.settingsValues(next, "installed", nil)
			if err != nil || !reflect.DeepEqual(values, saved.Values) {
				t.Fatalf("new release lost settings: %v %v", values, err)
			}
			var resumed RuntimeSnapshot
			if kind == Game {
				resumed, err = m.OpenInstance(context.Background(), instanceID, options)
			} else {
				if err := m.Stop(context.Background(), instanceID); err != nil {
					t.Fatal(err)
				}
				resumed, err = m.ActivatePlugin(context.Background(), plugin)
			}
			if err != nil || resumed.Context.Source != old.Ref || len(resumed.Context.Settings) != 0 {
				t.Fatalf("old release must reopen unchanged with empty settings: %#v %v", resumed.Context, err)
			}
			if _, err := m.settingsValues(old, "installed", map[string]any{"unexpected": true}); err == nil {
				t.Fatal("accepted explicitly supplied settings without a declaration")
			}
		})
	}
}
