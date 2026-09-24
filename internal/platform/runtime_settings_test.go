package platform

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRuntimeSettingsUsesNativeCASAndOwnerAuthority(t *testing.T) {
	m, projectID := testManager(t)
	candidate := testCandidate(t, m, projectID, "static", "test.runtime-settings", Game)
	release := testInstall(t, m, candidate)
	caller := &activation{release: release, context: RuntimeContext{Environment: "installed", Locale: "en-US"}}
	runtime := &Runtime{manager: m, owner: caller}
	response := httptest.NewRecorder()
	runtime.serveSettings(response, httptest.NewRequest("GET", "/settings", nil), caller)
	var document ConfigurationDocument
	if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || document.ReleaseID != release.Ref.ReleaseID || document.Values["display"].(map[string]any)["variant"] != "compact" {
		t.Fatalf("settings read failed: %s", response.Body.String())
	}
	input := ConfigurationInput{ExpectedRevision: document.Revision, ReleaseID: document.ReleaseID, Overrides: map[string]any{"display": map[string]any{"variant": "full"}}}
	raw, _ := json.Marshal(input)
	response = httptest.NewRecorder()
	runtime.serveSettings(response, httptest.NewRequest("PUT", "/settings", strings.NewReader(string(raw))), caller)
	if response.Code != 403 {
		t.Fatal("settings write permission was ignored")
	}
	caller.grants = []string{"settings.write"}
	response = httptest.NewRecorder()
	runtime.serveSettings(response, httptest.NewRequest("PUT", "/settings", strings.NewReader(string(raw))), caller)
	if response.Code != 200 {
		t.Fatalf("settings save failed: %s", response.Body.String())
	}
	saved, err := m.PackageConfiguration(ReleaseRef{Package: PackageRef{Kind: Game, ID: release.Manifest.ID}}, "en-US")
	if err != nil || saved.Values["display"].(map[string]any)["variant"] != "full" || saved.Revision == document.Revision {
		t.Fatalf("native settings not persisted: %+v %v", saved, err)
	}
	response = httptest.NewRecorder()
	runtime.serveSettings(response, httptest.NewRequest("PUT", "/settings", strings.NewReader(string(raw))), caller)
	if response.Code != 409 {
		t.Fatal("stale settings revision was accepted")
	}
	candidate.Manifest.Version = "2.0.0"
	candidate.files[Game.manifestFile()], _ = json.Marshal(candidate.Manifest)
	updated, err := m.freeze(Game, candidate.files)
	if err != nil {
		t.Fatal(err)
	}
	next := testInstall(t, m, updated)
	response = httptest.NewRecorder()
	runtime.serveSettings(response, httptest.NewRequest("GET", "/settings", nil), caller)
	if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil || document.ReleaseID != release.Ref.ReleaseID {
		t.Fatalf("an older runtime read newer settings: %s %v", response.Body.String(), err)
	}
	input.ReleaseID = next.Ref.ReleaseID
	raw, _ = json.Marshal(input)
	response = httptest.NewRecorder()
	runtime.serveSettings(response, httptest.NewRequest("PUT", "/settings", strings.NewReader(string(raw))), caller)
	if response.Code != 403 {
		t.Fatal("view modified another release's settings")
	}
	dependency := *caller
	response = httptest.NewRecorder()
	runtime.serveSettings(response, httptest.NewRequest("GET", "/settings", nil), &dependency)
	if response.Code != 403 {
		t.Fatal("dependency read owning view settings")
	}
	caller.context.Environment = "preview"
	response = httptest.NewRecorder()
	runtime.serveSettings(response, httptest.NewRequest("PUT", "/settings", strings.NewReader(string(raw))), caller)
	if response.Code != 409 {
		t.Fatal("preview modified installed preferences")
	}
	caller.context.Environment = "installed"
	runtime.hostOnly = true
	response = httptest.NewRecorder()
	runtime.serveSettings(response, httptest.NewRequest("GET", "/settings", nil), caller)
	if response.Code != 403 {
		t.Fatal("host-only Agent accessed view settings")
	}
}
