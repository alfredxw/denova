package platform

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"denova/internal/project"
)

func TestPreviewCatalogIsolationAndManagedDataRelocation(t *testing.T) {
	m, projectID := testManager(t)
	candidate := testCandidate(t, m, projectID, "static", "test.preview", Game)
	release, err := m.PreparePreview(candidate.ID, []string{"gameData"})
	if err != nil {
		t.Fatal(err)
	}
	installed, err := m.List(Game)
	if err != nil || len(installed) != 0 {
		t.Fatalf("preview entered catalog: %v %v", installed, err)
	}
	instance, err := m.CreateInstance(CreateInstance{GameID: candidate.Manifest.ID, ReleaseID: release.Ref.ReleaseID, Title: "Test save", ProjectID: projectID})
	if err != nil {
		t.Fatal(err)
	}
	if !instance.Preview {
		t.Fatal("preview identity was not derived from its release")
	}
	if err := writeBytes(filepath.Join(m.instancePath(instance.GameID, instance.ID), "data", "proof.txt"), []byte("portable")); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "moved")
	if err := os.Rename(m.root, root); err != nil {
		t.Fatal(err)
	}
	m.root, m.registry = root, project.NewRegistry(root)
	bindings, err := m.Developments()
	if err != nil || len(bindings) != 1 || bindings[0].ProjectID != projectID || bindings[0].RelativePath != "test.preview" {
		t.Fatalf("source binding lost: %v %v", bindings, err)
	}
	opened, err := m.OpenInstance(context.Background(), instance.ID, OpenOptions{ParentOrigin: "http://127.0.0.1:15173"})
	if err != nil {
		t.Fatal(err)
	}
	status, data := testRequest(t, opened.Connection, "GET", "/game-data/file?path=proof.txt", "", nil)
	if status != 200 || !strings.Contains(string(data), "portable") {
		t.Fatalf("preview lost after relocation: %d %s", status, data)
	}
}

func TestSharedPluginSelectionPinsAndDisable(t *testing.T) {
	m, projectID := testManager(t)
	tool := testInstall(t, m, testCandidate(t, m, projectID, "tool", "test.provider", Plugin))
	// A game can have the same local ID as a Plugin; kind keeps them distinct.
	gameSource := testSource(t, m, projectID, "games/test.consumer", "agent", "test.consumer", Game)
	gameCandidate, err := m.CheckDevelopment(gameSource.ID)
	if err != nil {
		t.Fatal(err)
	}
	gameCandidate.Manifest.Requires = []Dependency{{PluginID: "test.provider", VersionRange: "^1.0.0", Contributions: []string{"probes"}}}
	gameCandidate.Manifest.Game.Uses.Toolsets = []string{"test.provider/probes"}
	gameCandidate.Manifest.Permissions.Required = append(gameCandidate.Manifest.Permissions.Required, "tools.invoke")
	gameCandidate.files[Game.manifestFile()], _ = json.Marshal(gameCandidate.Manifest)
	gameCandidate, err = m.freeze(Game, gameCandidate.files)
	if err != nil {
		t.Fatal(err)
	}
	game := testInstall(t, m, gameCandidate)
	plugin, err := m.ActivatePlugin(context.Background(), ActivatePlugin{PluginID: tool.Manifest.ID, ReleaseID: tool.Ref.ReleaseID, Scope: Scope{Kind: "project", ProjectID: projectID}, OpenOptions: OpenOptions{ParentOrigin: "http://127.0.0.1:15173"}})
	if err != nil {
		t.Fatal(err)
	}
	instance, err := m.CreateInstance(CreateInstance{GameID: game.Manifest.ID, ReleaseID: game.Ref.ReleaseID, Title: "NPC town", ProjectID: projectID, Models: map[string]string{"local:writer": "test"}})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := m.OpenInstance(context.Background(), instance.ID, OpenOptions{ParentOrigin: "http://127.0.0.1:15173"})
	if err != nil {
		t.Fatal(err)
	}
	for _, connection := range []Connection{plugin.Connection, opened.Connection} {
		status, data := testRequest(t, connection, "POST", "/tools/test.provider/probe/invoke", "", map[string]any{"input": map[string]string{"text": "A🌷中"}})
		if status != 200 || !strings.Contains(string(data), `"value":3`) {
			t.Fatalf("shared tool failed: %d %s", status, data)
		}
	}
	bindings, err := m.Developments()
	if err != nil {
		t.Fatal(err)
	}
	for _, binding := range bindings {
		if binding.Kind != Plugin || binding.RelativePath != "test.provider" {
			continue
		}
		_, directory, err := m.Development(binding.ID)
		if err != nil {
			t.Fatal(err)
		}
		manifest := tool.Manifest
		manifest.Version = "1.1.0"
		if err := writeJSON(filepath.Join(directory, Plugin.manifestFile()), manifest); err != nil {
			t.Fatal(err)
		}
		candidate, err := m.CheckDevelopment(binding.ID)
		if err != nil {
			t.Fatal(err)
		}
		testInstall(t, m, candidate)
	}
	if err := m.Stop(context.Background(), instance.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.OpenInstance(context.Background(), instance.ID, OpenOptions{ParentOrigin: "http://127.0.0.1:15173"}); err != nil {
		t.Fatal(err)
	}
	if got := m.runtimes[instance.ID].providers["test.provider"].release.Ref.ReleaseID; got != tool.Ref.ReleaseID {
		t.Fatal("reopen silently changed dependency")
	}
	files := map[string][]byte{}
	for name, data := range gameCandidate.files {
		files[name] = data
	}
	nextManifest := game.Manifest
	nextManifest.Version = "1.1.0"
	nextManifest.Game.Setup = &ConfigurationDeclaration{Schema: "setup-schema.json", Defaults: "defaults.toml"}
	files["setup-schema.json"] = []byte(`{"type":"object","required":["tone"],"properties":{"tone":{"type":"string","x-titleKey":"tone"}},"additionalProperties":false}`)
	files["defaults.toml"] = []byte(`tone = "quiet"`)
	for language, path := range nextManifest.Locales {
		var labels map[string]any
		if err := json.Unmarshal(files[path], &labels); err != nil {
			t.Fatal(err)
		}
		labels["tone"] = "Tone"
		if language == "zh-CN" {
			labels["tone"] = "语气"
		}
		files[path], _ = json.Marshal(labels)
	}
	nextManifest.Distribution.Files = append(nextManifest.Distribution.Files, "setup-schema.json", "defaults.toml")
	files[Game.manifestFile()], _ = json.Marshal(nextManifest)
	nextCandidate, err := m.freeze(Game, files)
	if err != nil {
		t.Fatal(err)
	}
	next := testInstall(t, m, nextCandidate)
	upgraded, err := m.UpgradeInstance(context.Background(), instance.ID, next.Ref.ReleaseID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if upgraded.Setup["tone"] != "quiet" {
		t.Fatalf("upgrade did not retain validated configuration defaults: %v", upgraded.Setup)
	}
	for _, pin := range upgraded.Dependencies {
		if pin.PluginID == tool.Manifest.ID && pin.ReleaseID != tool.Ref.ReleaseID {
			t.Fatal("game upgrade silently changed its saved NPC's dependencies")
		}
	}
	if _, err := m.OpenInstance(context.Background(), instance.ID, OpenOptions{ParentOrigin: "http://127.0.0.1:15173"}); err != nil {
		t.Fatal(err)
	}
	if err := m.SetAvailability(context.Background(), Plugin, tool.Manifest.ID, PackageAvailability{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if len(m.RuntimeSnapshots()) == 0 {
		t.Fatal("disabling interrupted active runtimes")
	}
	continued, err := m.OpenInstance(context.Background(), instance.ID, OpenOptions{ParentOrigin: "http://127.0.0.1:15173"})
	if err != nil {
		t.Fatal(err)
	}
	status, data := testRequest(t, continued.Connection, "POST", "/tools/test.provider/probe/invoke", "", map[string]any{"input": map[string]string{"text": "A🌷中"}})
	if status != 200 || !strings.Contains(string(data), `"value":3`) {
		t.Fatalf("disabled dependency interrupted ongoing tool use: %d %s", status, data)
	}
	catalog, err := m.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range catalog {
		if entry.Kind == Game && entry.ID == game.Manifest.ID && entry.UnavailableReason != "platform.errors.DEPENDENCY_UNAVAILABLE" {
			t.Fatalf("catalog did not report the unavailable dependency: %#v", entry)
		}
	}
	if err := m.Stop(context.Background(), instance.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.OpenInstance(context.Background(), instance.ID, OpenOptions{ParentOrigin: "http://127.0.0.1:15173"}); err == nil {
		t.Fatal("opened without required plugin")
	}
}

func TestReauthorizationRevokesOldRuntimeCredentials(t *testing.T) {
	m, projectID := testManager(t)
	candidate := testCandidate(t, m, projectID, "static", "test.grants", Game)
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
	release, err = m.Install(candidate.ID, []string{"gameData"})
	if err != nil || len(release.Grants) != 1 {
		t.Fatalf("reauthorize: %v %v", release.Grants, err)
	}
	if len(m.RuntimeSnapshots()) != 0 || old.ctx.Err() == nil {
		t.Fatal("reauthorization retained an activation with revoked grants")
	}
	opened, err := m.OpenInstance(context.Background(), instance.ID, OpenOptions{ParentOrigin: "http://127.0.0.1:15173"})
	if err != nil {
		t.Fatal(err)
	}
	status, _ := testRequest(t, opened.Connection, "POST", "/tools/local/unavailable/invoke", "", map[string]any{"input": map[string]any{}})
	if status != 403 {
		t.Fatalf("revoked tool grant accepted: %d", status)
	}
}

func TestCrashedBackendStopsDependencyProcesses(t *testing.T) {
	m, projectID := testManager(t)
	tool := testInstall(t, m, testCandidate(t, m, projectID, "tool", "test.tool", Plugin))
	candidate := testCandidate(t, m, projectID, "backend", "test.crash", Game)
	manifest := candidate.Manifest
	manifest.Requires = []Dependency{{PluginID: tool.Manifest.ID, VersionRange: "^1.0.0", Contributions: []string{"probes"}}}
	candidate.files[Game.manifestFile()], _ = json.Marshal(manifest)
	candidate, err := m.freeze(Game, candidate.files)
	if err != nil {
		t.Fatal(err)
	}
	release := testInstall(t, m, candidate)
	instance, err := m.CreateInstance(CreateInstance{GameID: release.Manifest.ID, ReleaseID: release.Ref.ReleaseID, Title: "Crash cleanup"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.OpenInstance(context.Background(), instance.ID, OpenOptions{ParentOrigin: "http://127.0.0.1:15173"}); err != nil {
		t.Fatal(err)
	}
	runtime := m.runtimes[instance.ID]
	if err := runtime.owner.process.command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runtime.providers[tool.Manifest.ID].process.done:
	case <-time.After(2 * time.Second):
		t.Fatal("backend crash left its dependency process alive")
	}
}

func TestFailedUpgradePreservesBindingAndSave(t *testing.T) {
	m, projectID := testManager(t)
	candidate := testCandidate(t, m, projectID, "backend", "test.upgrade", Game)
	release := testInstall(t, m, candidate)
	instance, err := m.CreateInstance(CreateInstance{GameID: release.Manifest.ID, ReleaseID: release.Ref.ReleaseID, Title: "Original"})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(m.instancePath(instance.GameID, instance.ID), "data", "proof.json")
	if err := writeBytes(path, []byte(`{"preserve":true}`)); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for key, value := range candidate.files {
		files[key] = value
	}
	manifest := candidate.Manifest
	manifest.Version = "1.1.0"
	files[Game.manifestFile()], _ = json.Marshal(manifest)
	files["server.mjs"] = []byte("process.exit(23)")
	bad, err := m.freeze(Game, files)
	if err != nil {
		t.Fatal(err)
	}
	target := testInstall(t, m, bad)
	if _, err := m.UpgradeInstance(context.Background(), instance.ID, target.Ref.ReleaseID, nil); err == nil {
		t.Fatal("accepted a backend that cannot start")
	}
	preserved, err := m.Instance(instance.ID)
	if err != nil || preserved.ReleaseID != instance.ReleaseID {
		t.Fatalf("lost original binding: %v %v", preserved, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != `{"preserve":true}` {
		t.Fatalf("save changed: %s %v", data, err)
	}
	backups, err := filepath.Glob(filepath.Join(m.packagePath(PackageRef{Kind: Game, ID: instance.GameID}), "backups", "*.zip"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("missing backup: %v %v", backups, err)
	}
}
