package platform

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"denova/internal/project"
)

func testManager(t *testing.T) (*Manager, string) {
	t.Helper()
	root := t.TempDir()
	registry := project.NewRegistry(root)
	directory := filepath.Join(root, "projects", "Author")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	record, err := registry.Add(directory, project.TypeGeneral, "Author")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.EnsureStore(record); err != nil {
		t.Fatal(err)
	}
	m := New(root, registry)
	t.Cleanup(func() {
		if err := m.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return m, record.ID
}

func testCandidate(t *testing.T, m *Manager, projectID, fixture, id string, kind Kind) Candidate {
	t.Helper()
	development := testSource(t, m, projectID, id, fixture, id, kind)
	candidate, err := m.CheckDevelopment(development.ID)
	if err != nil {
		t.Fatal(err)
	}
	return candidate
}

func testInstall(t *testing.T, m *Manager, candidate Candidate) Release {
	t.Helper()
	release, err := m.Install(candidate.ID, candidate.Manifest.Permissions.Required)
	if err != nil {
		t.Fatal(err)
	}
	return release
}

func testRequest(t *testing.T, connection Connection, method, route, origin string, body any) (int, []byte) {
	t.Helper()
	var input io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		input = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, connection.BaseURL+route, input)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+connection.Token)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, data
}

func TestFrozenPackageAndArchiveSafety(t *testing.T) {
	m, projectID := testManager(t)
	for template, kind := range map[string]Kind{"tool": Plugin, "agent": Game, "static": Game, "backend": Game} {
		t.Run(template, func(t *testing.T) {
			candidate := testCandidate(t, m, projectID, template, "test."+template, kind)
			release := testInstall(t, m, candidate)
			var archive bytes.Buffer
			if err := m.ExportCandidate(candidate.ID, &archive); err != nil {
				t.Fatal(err)
			}
			imported, err := m.PreviewZIP(kind, archive.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			if imported.Digest != release.Digest {
				t.Fatal("export changed frozen bytes")
			}
		})
	}
	for _, name := range []string{"../outside", "CON.txt", "foo\\bar", "/absolute", "foo:bar"} {
		var archive bytes.Buffer
		writer := zip.NewWriter(&archive)
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = entry.Write([]byte("bad"))
		_ = writer.Close()
		if _, err := m.PreviewZIP(Game, archive.Bytes()); err == nil {
			t.Fatalf("accepted unsafe path %q", name)
		}
	}
}

func TestGameInstanceIsolationStopAndRelocation(t *testing.T) {
	m, projectID := testManager(t)
	release := testInstall(t, m, testCandidate(t, m, projectID, "static", "test.storage", Game))
	var runtimes []RuntimeSnapshot
	for _, title := range []string{"First", "Second"} {
		instance, err := m.CreateInstance(CreateInstance{GameID: release.Manifest.ID, ReleaseID: release.Ref.ReleaseID, ProjectID: projectID, Title: title})
		if err != nil {
			t.Fatal(err)
		}
		runtime, err := m.OpenInstance(context.Background(), instance.ID, OpenOptions{ParentOrigin: "http://127.0.0.1:15173", Locale: "zh-CN", Theme: "light"})
		if err != nil {
			t.Fatal(err)
		}
		runtimes = append(runtimes, runtime)
	}
	first, second := runtimes[0], runtimes[1]
	if first.Connection.Token == second.Connection.Token || first.Connection.BaseURL == second.Connection.BaseURL {
		t.Fatal("instances share credentials or origin")
	}
	status, data := testRequest(t, first.Connection, "PUT", "/game-data/file?path=state.json", "", map[string]any{"path": "state.json", "content": `{"value":1}`, "expectedRevision": nil})
	if status != 200 {
		t.Fatalf("save: %d %s", status, data)
	}
	status, _ = testRequest(t, second.Connection, "GET", "/game-data/file?path=state.json", "", nil)
	if status != 404 {
		t.Fatalf("other instance read save: %d", status)
	}
	status, _ = testRequest(t, Connection{BaseURL: second.Connection.BaseURL, Token: first.Connection.Token}, "GET", "/context", "", nil)
	if status != 403 {
		t.Fatalf("other credential accepted: %d", status)
	}
	status, _ = testRequest(t, first.Connection, "GET", "/context", "http://malicious.example", nil)
	if status != 403 {
		t.Fatalf("foreign origin accepted: %d", status)
	}
	status, _ = testRequest(t, first.Connection, "PUT", "/game-data/file?path=state.json", "", map[string]any{"path": "state.json", "content": "overwrite", "expectedRevision": nil})
	if status != 409 {
		t.Fatalf("stale save overwrote content: %d", status)
	}
	if err := m.Stop(context.Background(), first.ID); err != nil {
		t.Fatal(err)
	}
	opened, err := m.OpenInstance(context.Background(), first.ID, OpenOptions{ParentOrigin: "http://127.0.0.1:15173"})
	if err != nil {
		t.Fatal(err)
	}
	status, data = testRequest(t, opened.Connection, "GET", "/game-data/file?path=state.json", "", nil)
	if status != 200 || !strings.Contains(string(data), "value") {
		t.Fatalf("save did not survive restart: %d %s", status, data)
	}
}

func TestNodeGamePOSTAndPersistentSave(t *testing.T) {
	m, projectID := testManager(t)
	release := testInstall(t, m, testCandidate(t, m, projectID, "backend", "test.backend", Game))
	instance, err := m.CreateInstance(CreateInstance{GameID: release.Manifest.ID, ReleaseID: release.Ref.ReleaseID, Title: "Backend"})
	if err != nil {
		t.Fatal(err)
	}
	options := OpenOptions{ParentOrigin: "http://127.0.0.1:15173"}
	opened, err := m.OpenInstance(context.Background(), instance.ID, options)
	if err != nil {
		t.Fatal(err)
	}
	connection := Connection{BaseURL: opened.ViewURL}
	status, data := testRequest(t, connection, "POST", "increment", strings.TrimSuffix(opened.Connection.BaseURL, "/api/platform/v1"), map[string]any{})
	if status != 200 {
		t.Fatalf("backend POST: %d %s", status, data)
	}
	if err := m.Stop(context.Background(), instance.ID); err != nil {
		t.Fatal(err)
	}
	opened, err = m.OpenInstance(context.Background(), instance.ID, options)
	if err != nil {
		t.Fatal(err)
	}
	status, data = testRequest(t, Connection{BaseURL: opened.ViewURL}, "GET", "state", "", nil)
	if status != 200 || !strings.Contains(string(data), `"value":1`) {
		t.Fatalf("backend save lost: %d %s", status, data)
	}
}
