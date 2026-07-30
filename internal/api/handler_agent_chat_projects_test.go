package api

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestAgentChatProjectManagementKeepsStableIdentityAndState(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	root := t.TempDir()
	generalPath := filepath.Join(root, "general")
	bookPath := filepath.Join(root, "book")
	relinkedPath := filepath.Join(root, "general-moved")
	for _, path := range []string{generalPath, bookPath, relinkedPath} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(bookPath, "book.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	createdGeneral := performJSONRequest(t, server, http.MethodPost, "/api/agent-chat/projects", map[string]any{
		"path": generalPath,
	})
	if createdGeneral.Code != http.StatusCreated {
		t.Fatalf("create general project status = %d body=%s", createdGeneral.Code, createdGeneral.Body.String())
	}
	var general projectResponse
	decodeResponse(t, createdGeneral.Body.Bytes(), &general)
	if general.ID == "" || general.Type != "general" || general.Name != "general" || general.Status != "available" {
		t.Fatalf("unexpected general project: %#v", general)
	}

	createdBook := performJSONRequest(t, server, http.MethodPost, "/api/agent-chat/projects", map[string]any{
		"path": bookPath,
	})
	if createdBook.Code != http.StatusCreated {
		t.Fatalf("create book project status = %d body=%s", createdBook.Code, createdBook.Body.String())
	}
	var book projectResponse
	decodeResponse(t, createdBook.Body.Bytes(), &book)
	if book.Type != "book" || book.Name != "book" {
		t.Fatalf("book project should be inferred from its folder: %#v", book)
	}

	createdSession := performJSONRequest(t, server, http.MethodPost, "/api/agent-chat/sessions", map[string]any{
		"project_id": general.ID,
		"title":      "Stable conversation",
	})
	if createdSession.Code != http.StatusOK {
		t.Fatalf("create project session status = %d body=%s", createdSession.Code, createdSession.Body.String())
	}

	renamed := performJSONRequest(t, server, http.MethodPatch, "/api/agent-chat/projects/"+general.ID, map[string]any{
		"name": "Renamed General",
	})
	if renamed.Code != http.StatusOK {
		t.Fatalf("rename project status = %d body=%s", renamed.Code, renamed.Body.String())
	}
	decodeResponse(t, renamed.Body.Bytes(), &general)
	if general.ID == "" || general.Name != "Renamed General" {
		t.Fatalf("unexpected renamed project: %#v", general)
	}
	stableID := general.ID

	relinked := performJSONRequest(t, server, http.MethodPatch, "/api/agent-chat/projects/"+general.ID, map[string]any{
		"path": relinkedPath,
	})
	if relinked.Code != http.StatusOK {
		t.Fatalf("relink project status = %d body=%s", relinked.Code, relinked.Body.String())
	}
	decodeResponse(t, relinked.Body.Bytes(), &general)
	if general.ID != stableID || general.Path != canonicalTestPath(t, relinkedPath) {
		t.Fatalf("project identity changed after relink: %#v", general)
	}

	reordered := performJSONRequest(t, server, http.MethodPost, "/api/agent-chat/projects/reorder", map[string]any{
		"project_ids": []string{book.ID, general.ID},
	})
	if reordered.Code != http.StatusOK {
		t.Fatalf("reorder projects status = %d body=%s", reordered.Code, reordered.Body.String())
	}
	listed := performJSONRequest(t, server, http.MethodGet, "/api/agent-chat/projects", nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("list projects status = %d body=%s", listed.Code, listed.Body.String())
	}
	var list struct {
		Projects []struct {
			projectResponse
			Total int `json:"total"`
		} `json:"projects"`
	}
	decodeResponse(t, listed.Body.Bytes(), &list)
	if len(list.Projects) < 2 || list.Projects[0].ID != book.ID || list.Projects[1].ID != general.ID {
		t.Fatalf("unexpected project order: %#v", list.Projects)
	}
	if list.Projects[1].Total != 1 || list.Projects[1].Path != canonicalTestPath(t, relinkedPath) {
		t.Fatalf("project session state did not follow stable identity: %#v", list.Projects[1])
	}

	archived := performJSONRequest(t, server, http.MethodDelete, "/api/agent-chat/projects/"+general.ID, nil)
	if archived.Code != http.StatusOK {
		t.Fatalf("archive project status = %d body=%s", archived.Code, archived.Body.String())
	}
	decodeResponse(t, archived.Body.Bytes(), &general)
	if general.ID != stableID || general.Status != "archived" {
		t.Fatalf("unexpected archived project: %#v", general)
	}
	listed = performJSONRequest(t, server, http.MethodGet, "/api/agent-chat/projects", nil)
	decodeResponse(t, listed.Body.Bytes(), &list)
	if len(list.Projects) == 0 || list.Projects[0].ID != book.ID {
		t.Fatalf("archived project should be hidden from the active list: %#v", list.Projects)
	}
	for _, project := range list.Projects {
		if project.ID == general.ID {
			t.Fatalf("archived project should be hidden from the active list: %#v", list.Projects)
		}
	}
}

type projectResponse struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Name   string `json:"name"`
	Path   string `json:"path"`
	Status string `json:"status"`
}

func canonicalTestPath(t *testing.T, path string) string {
	t.Helper()
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(canonical)
}
