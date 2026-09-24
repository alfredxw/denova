package platform

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestSetupUsesActivationDependenciesAndModelRequirements(t *testing.T) {
	m, projectID := testManager(t)
	plugin := testCandidate(t, m, projectID, "tool", "test.setup-provider", Plugin)
	plugin.Manifest.ModelSlots = []ModelSlot{{ID: "art", TitleKey: "writer", Kind: "image", Required: true}}
	plugin.files[Plugin.manifestFile()], _ = json.Marshal(plugin.Manifest)
	plugin, err := m.freeze(Plugin, plugin.files)
	if err != nil {
		t.Fatal(err)
	}
	testInstall(t, m, plugin)
	game := testCandidate(t, m, projectID, "agent", "test.setup-consumer", Game)
	game.Manifest.Requires = []Dependency{{PluginID: plugin.Manifest.ID, VersionRange: "^1.0.0", Contributions: []string{"probes"}}}
	game.files[Game.manifestFile()], _ = json.Marshal(game.Manifest)
	game, err = m.freeze(Game, game.files)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := m.CandidateSetup(game.ID, "en-US")
	want := []ModelRequirement{{Key: "local:writer", Kind: "text", Required: true}, {Key: plugin.Manifest.ID + "/art", Kind: "image", Required: true}}
	if err != nil || !reflect.DeepEqual(preview.Models, want) {
		t.Fatalf("preview requirements: %+v %v", preview.Models, err)
	}
	release := testInstall(t, m, game)
	installed, err := m.SetupConfiguration(game.Manifest.ID, release.Ref.ReleaseID, "en-US")
	if err != nil || !reflect.DeepEqual(installed.Models, want) {
		t.Fatalf("installed requirements: %+v %v", installed.Models, err)
	}
	input := CreateInstance{GameID: game.Manifest.ID, ReleaseID: release.Ref.ReleaseID, Title: "Setup", ProjectID: projectID, Models: map[string]string{"local:writer": "text-profile"}}
	if _, err := m.CreateInstance(input); err == nil {
		t.Fatal("activation omitted a required dependency model")
	}
	input.Models[plugin.Manifest.ID+"/art"] = "image-profile"
	if _, err := m.CreateInstance(input); err != nil {
		t.Fatal(err)
	}
	if err := m.SetAvailability(t.Context(), Plugin, plugin.Manifest.ID, PackageAvailability{}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CandidateSetup(game.ID, "en-US"); err == nil {
		t.Fatal("setup ignored unavailable activation dependency")
	}
}
