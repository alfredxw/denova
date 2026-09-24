package platform

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"testing"
)

func TestSourceUpdateKeepsSavedGameAndDependencySnapshots(t *testing.T) {
	m, projectID := testManager(t)
	plugin := testCandidate(t, m, projectID, "tool", "test.source-tool", Plugin)
	oldPlugin := testInstall(t, m, plugin)
	game := testCandidate(t, m, projectID, "static", "test.source-game", Game)
	oldGame := testInstall(t, m, game)
	instance, err := m.CreateInstance(CreateInstance{GameID: oldGame.Manifest.ID, ReleaseID: oldGame.Ref.ReleaseID, Title: "Existing save"})
	if err != nil {
		t.Fatal(err)
	}
	for _, original := range []Candidate{plugin, game} {
		files := maps.Clone(original.files)
		files["update.txt"] = []byte("New source with the same declared version")
		candidate, err := m.freeze(original.Kind, files)
		if err != nil {
			t.Fatal(err)
		}
		next := testInstall(t, m, candidate)
		if next.Ref.ReleaseID == original.Digest || next.Manifest.Version != original.Manifest.Version {
			t.Fatal("source update did not create a distinct snapshot of the same version")
		}
		_, installed, err := m.release(next.Ref)
		if err != nil || installed.CurrentRelease != next.Ref.ReleaseID || len(installed.Releases) != 2 {
			t.Fatalf("source update did not retain the previous snapshot: %+v %v", installed, err)
		}
	}
	request := Manifest{Requires: []Dependency{{PluginID: plugin.Manifest.ID, VersionRange: "*"}}}
	pins, err := m.resolveDependencies(request, nil)
	if err != nil || len(pins) != 1 || pins[0].ReleaseID == oldPlugin.Ref.ReleaseID {
		t.Fatalf("new dependency selection did not use the current source: %+v %v", pins, err)
	}
	pins, err = m.resolveDependencies(request, []DependencyPin{{PluginID: plugin.Manifest.ID, ReleaseID: oldPlugin.Ref.ReleaseID}})
	if err != nil || len(pins) != 1 || pins[0].ReleaseID != oldPlugin.Ref.ReleaseID {
		t.Fatalf("saved dependency changed after update: %+v %v", pins, err)
	}
	runtime, err := m.OpenInstance(context.Background(), instance.ID, OpenOptions{ParentOrigin: "http://127.0.0.1"})
	if err != nil || runtime.Context.Source.ReleaseID != oldGame.Ref.ReleaseID {
		t.Fatalf("existing save did not open its original code: %+v %v", runtime, err)
	}
	if _, err := os.Stat(filepath.Join(m.releasePath(oldGame.Ref), "update.txt")); !os.IsNotExist(err) {
		t.Fatalf("update overwrote old package bytes: %v", err)
	}
}
