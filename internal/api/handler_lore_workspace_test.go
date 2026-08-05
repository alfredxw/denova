package api

import (
	"net/http"
	"net/url"
	"testing"
)

func TestProjectLoreAPIUsesStableProjectIdentity(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	projectID := application.ProjectID()
	path := "/api/projects/" + url.PathEscape(projectID) + "/book/lore/items"

	response := performJSONRequest(t, server, http.MethodPost, path, map[string]any{
		"enabled": true, "type": "world", "name": "按项目写入", "importance": "important", "load_mode": "auto", "content": "stable",
	})
	if response.Code != http.StatusOK {
		t.Fatalf("project Lore create status=%d body=%s", response.Code, response.Body.String())
	}
	var envelope struct {
		ProjectID string `json:"project_id"`
	}
	decodeResponse(t, response.Body.Bytes(), &envelope)
	if envelope.ProjectID != projectID {
		t.Fatalf("project Lore response lost stable identity: %#v", envelope)
	}
	items, err := application.ProjectBook().LoreItems(projectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "按项目写入" {
		t.Fatalf("Project ID did not select the canonical Lore store: %#v", items)
	}

	preview := performJSONRequest(t, server, http.MethodPost, "/api/projects/"+url.PathEscape(projectID)+"/book/lore/classification/preview", map[string]any{"mode": "heuristic"})
	if preview.Code != http.StatusOK {
		t.Fatalf("stable Project lore preview status=%d body=%s", preview.Code, preview.Body.String())
	}
}

func TestLoreAuxiliaryAPIsRequireStableProjectScope(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	tests := []struct {
		name   string
		method string
		path   string
		body   map[string]any
	}{
		{name: "legacy classification route", method: http.MethodPost, path: "/api/lore/classification/preview", body: map[string]any{"mode": "heuristic"}},
		{name: "legacy image route", method: http.MethodPost, path: "/api/lore/items/hero/image/generate", body: map[string]any{}},
		{name: "unknown Project", method: http.MethodPost, path: "/api/projects/project-missing/book/lore/classification/preview", body: map[string]any{"mode": "heuristic"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := performJSONRequest(t, server, test.method, test.path, test.body)
			if response.Code != http.StatusNotFound {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}
