package platform

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestConfigurationTOMLAndValidation(t *testing.T) {
	values, err := parseConfiguration([]byte("[display]\nsize = 2\ntags = ['a', 'b']\n"))
	if err != nil {
		t.Fatal(err)
	}
	form := &ConfigurationForm{
		Schema:   map[string]any{"type": "object", "properties": map[string]any{"display": map[string]any{"type": "object", "properties": map[string]any{"size": map[string]any{"type": "integer", "maximum": 3}}}}},
		Defaults: map[string]any{"display": map[string]any{"size": 1, "enabled": true, "tags": []any{"old"}}},
	}
	got, err := validateConfiguration(form, values)
	want := map[string]any{"display": map[string]any{"size": float64(2), "enabled": true, "tags": []any{"a", "b"}}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("merged settings: %#v %v", got, err)
	}
	for _, raw := range []string{"size = [", "size = 9007199254740992", "size = inf", "date = 2026-09-12"} {
		if _, err := parseConfiguration([]byte(raw)); err == nil {
			t.Fatalf("accepted invalid settings: %s", raw)
		}
	}
	if _, err := configurationInput(ConfigurationInput{Format: "values", Overrides: map[string]any{"nil": nil}}); err == nil {
		t.Fatal("accepted null")
	}
	_, err = validateConfiguration(form, map[string]any{"display": map[string]any{"size": 9}})
	_, problem := ErrorResponse(err)
	if problem.Code != "INVALID_CONFIGURATION" || !reflect.DeepEqual(problem.Fields, []ConfigurationIssue{{Path: []string{"display", "size"}, Keyword: "maximum"}}) {
		t.Fatalf("field diagnostics: %#v", problem)
	}
}

func TestSharedSettingsSurviveUpgradeWithoutChangingRunningGameOrSetup(t *testing.T) {
	m, projectID := testManager(t)
	candidate := testCandidate(t, m, projectID, "static", "test.shared-settings", Game)
	release := testInstall(t, m, candidate)
	instance, err := m.CreateInstance(CreateInstance{Title: "Test", GameID: release.Manifest.ID, ReleaseID: release.Ref.ReleaseID, Setup: map[string]any{"initialValue": 7}})
	if err != nil {
		t.Fatal(err)
	}
	options := OpenOptions{ParentOrigin: "http://127.0.0.1:15173"}
	first, err := m.OpenInstance(context.Background(), instance.ID, options)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := m.PackageConfiguration(Game, release.Manifest.ID, "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	displaySchema := doc.Form.Schema["properties"].(map[string]any)["display"].(map[string]any)
	if displaySchema["title"] != "显示设置" {
		t.Fatalf("unlocalized form: %v", displaySchema)
	}
	input := ConfigurationInput{ReleaseID: release.Ref.ReleaseID, ExpectedRevision: doc.Revision, Format: "toml", TOML: "[display]\nvariant = 'full'\n"}
	doc, err = m.SavePackageConfiguration(Game, release.Manifest.ID, "en-US", input)
	if err != nil {
		t.Fatal(err)
	}
	current := m.runtimes[instance.ID]
	if current.ctx.Err() != nil || current.owner.context.Settings["display"].(map[string]any)["variant"] != "compact" {
		t.Fatal("save changed running settings")
	}
	if _, err := m.SavePackageConfiguration(Game, release.Manifest.ID, "en-US", input); err == nil {
		t.Fatal("accepted stale settings revision")
	}
	if err := m.Stop(context.Background(), instance.ID); err != nil {
		t.Fatal(err)
	}
	reopened, err := m.OpenInstance(context.Background(), instance.ID, options)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Context.Settings["display"].(map[string]any)["variant"] != "full" || reopened.Context.Setup["initialValue"] != float64(7) || reopened.Connection.Token == first.Connection.Token {
		t.Fatalf("next start: %#v", reopened.Context)
	}
	if err := m.Stop(context.Background(), instance.ID); err != nil {
		t.Fatal(err)
	}

	// A new release inherits the same override file without rebinding old saves.
	nextManifest := candidate.Manifest
	nextManifest.Version = "1.1.0"
	files := map[string][]byte{}
	for name, data := range candidate.files {
		files[name] = data
	}
	files[Game.manifestFile()], _ = json.Marshal(nextManifest)
	nextCandidate, err := m.freeze(Game, files)
	if err != nil {
		t.Fatal(err)
	}
	next := testInstall(t, m, nextCandidate)
	nextDoc, err := m.PackageConfiguration(Game, release.Manifest.ID, "en-US")
	if err != nil || !reflect.DeepEqual(nextDoc.Values, doc.Values) {
		t.Fatalf("upgrade lost settings: %#v %v", nextDoc, err)
	}
	saved, err := m.Instance(instance.ID)
	if err != nil || saved.ReleaseID != release.Ref.ReleaseID || !reflect.DeepEqual(saved.Setup, instance.Setup) {
		t.Fatalf("upgrade changed save: %#v %v", saved, err)
	}
	root := m.packagePath(release.Ref.Package)
	for _, relative := range []string{"installed.json", "settings.toml", "releases/" + release.Ref.ReleaseID, "releases/" + next.Ref.ReleaseID, "instances/" + instance.ID + "/instance.json"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(m.root, "games", "installed.json")); !os.IsNotExist(err) {
		t.Fatal("duplicate global installation registry")
	}
	// Reopening from a moved data root reconstructs the same installation and save.
	moved := filepath.Join(t.TempDir(), "moved")
	if err := os.CopyFS(moved, os.DirFS(m.root)); err != nil {
		t.Fatal(err)
	}
	relocated := New(moved, m.registry)
	restored, err := relocated.PackageConfiguration(Game, release.Manifest.ID, "en-US")
	if err != nil || !reflect.DeepEqual(restored.Values, doc.Values) {
		t.Fatalf("relocated settings: %#v %v", restored, err)
	}
	if _, err := relocated.Instance(instance.ID); err != nil {
		t.Fatal(err)
	}
}

func TestSettingsRejectPinnedIncompatibilityAndRepairInvalidFile(t *testing.T) {
	m, projectID := testManager(t)
	candidate := testCandidate(t, m, projectID, "static", "test.settings-conflict", Game)
	old := testInstall(t, m, candidate)
	if _, err := m.CreateInstance(CreateInstance{Title: "Test", GameID: old.Manifest.ID, ReleaseID: old.Ref.ReleaseID}); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for name, data := range candidate.files {
		files[name] = data
	}
	manifest := candidate.Manifest
	manifest.Version = "2.0.0"
	var schema map[string]any
	if err := json.Unmarshal(files[manifest.Settings.Schema], &schema); err != nil {
		t.Fatal(err)
	}
	schema["properties"].(map[string]any)["extra"] = map[string]any{"type": "boolean", "x-titleKey": "settings.showLabel"}
	files[manifest.Settings.Schema], _ = json.Marshal(schema)
	files[Game.manifestFile()], _ = json.Marshal(manifest)
	nextCandidate, err := m.freeze(Game, files)
	if err != nil {
		t.Fatal(err)
	}
	next := testInstall(t, m, nextCandidate)
	doc, _ := m.PackageConfiguration(Game, manifest.ID, "en-US")
	input := ConfigurationInput{ReleaseID: next.Ref.ReleaseID, ExpectedRevision: doc.Revision, Format: "toml", TOML: "extra = true"}
	_, err = m.SavePackageConfiguration(Game, manifest.ID, "en-US", input)
	_, problem := ErrorResponse(err)
	if problem.Code != "CONFIGURATION_CONFLICT" || problem.Version != "1.0.0" {
		t.Fatalf("pinned conflict: %#v", problem)
	}
	path := filepath.Join(m.packagePath(next.Ref.Package), "settings.toml")
	broken := "[display\nvariant = 'full'"
	if err := writeBytes(path, []byte(broken)); err != nil {
		t.Fatal(err)
	}
	repair, err := m.PackageConfiguration(Game, manifest.ID, "en-US")
	if err != nil || repair.Problem == nil || repair.TOML != broken {
		t.Fatalf("repair form: %#v %v", repair, err)
	}
	input.ExpectedRevision, input.TOML = repair.Revision, "[display]\nvariant = 'full'"
	if _, err := m.SavePackageConfiguration(Game, manifest.ID, "en-US", input); err != nil {
		t.Fatal(err)
	}
	backups, err := filepath.Glob(filepath.Join(filepath.Dir(path), "backups", "settings-*.toml"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("missing recovery backup: %v %v", backups, err)
	}
	prior, _ := os.ReadFile(backups[0])
	if string(prior) != broken {
		t.Fatal("backup did not retain original bytes")
	}
	// Invalid overrides also block installing an incompatible target version.
	manifest.Version = "3.0.0"
	files[manifest.Settings.Schema] = []byte(strings.ReplaceAll(string(files[manifest.Settings.Schema]), `"const":"full"`, `"const":"invalid"`))
	files[Game.manifestFile()], _ = json.Marshal(manifest)
	invalidCandidate, err := m.freeze(Game, files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Install(invalidCandidate.ID, old.Grants); err == nil {
		t.Fatal("installed incompatible settings schema")
	}
}

func TestPluginProviderConsumesSettingsOnlyOnNextStart(t *testing.T) {
	m, projectID := testManager(t)
	release := testInstall(t, m, testCandidate(t, m, projectID, "tool", "test.provider-settings", Plugin))
	input := ActivatePlugin{PluginID: release.Manifest.ID, ReleaseID: release.Ref.ReleaseID, Scope: Scope{Kind: "project", ProjectID: projectID}, OpenOptions: OpenOptions{ParentOrigin: "http://127.0.0.1:15173"}}
	runtime, err := m.ActivatePlugin(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	count := func(runtime RuntimeSnapshot, expected float64) {
		t.Helper()
		status, raw := testRequest(t, runtime.Connection, "POST", "/tools/"+release.Manifest.ID+"/probe/invoke", "", map[string]any{"input": map[string]any{"text": "A 中"}})
		var output struct {
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal(raw, &output); err != nil || status != 200 || output.Data["value"] != expected {
			t.Fatalf("provider settings: status=%d body=%s err=%v", status, raw, err)
		}
	}
	count(runtime, 3)
	doc, err := m.PackageConfiguration(Plugin, release.Manifest.ID, "en-US")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.SavePackageConfiguration(Plugin, release.Manifest.ID, "en-US", ConfigurationInput{ReleaseID: release.Ref.ReleaseID, ExpectedRevision: doc.Revision, Format: "toml", TOML: "enabled = true"}); err != nil {
		t.Fatal(err)
	}
	reused, err := m.ActivatePlugin(context.Background(), input)
	if err != nil || reused.Connection.Token != runtime.Connection.Token {
		t.Fatalf("save interrupted active plugin: %v", err)
	}
	count(reused, 3)
	if err := m.Stop(context.Background(), runtime.ID); err != nil {
		t.Fatal(err)
	}
	next, err := m.ActivatePlugin(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	count(next, 2)
}
