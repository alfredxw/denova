package api

import (
	"net/http"
	"testing"
)

func TestLoreAPIRejectsStaleWorkspaceIdentity(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	staleWorkspace := t.TempDir()

	response := performWorkspaceChangeRequest(t, server, http.MethodPost, "/api/lore/items", staleWorkspace, map[string]any{
		"enabled": true, "type": "world", "name": "不应写入", "importance": "important", "load_mode": "auto", "content": "stale",
	})
	assertWorkspaceChangedResponse(t, response, staleWorkspace, application.Workspace())
	items, err := application.Lore().Items()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("stale workspace request mutated active lore: %#v", items)
	}

	preview := performWorkspaceChangeRequest(t, server, http.MethodPost, "/api/lore/classification/preview", staleWorkspace, map[string]any{"mode": "heuristic"})
	assertWorkspaceChangedResponse(t, preview, staleWorkspace, application.Workspace())
}

func TestLoreImageAPIRejectsStaleWorkspaceIdentityBeforeStartingWork(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	staleWorkspace := t.TempDir()

	tests := []struct {
		name string
		path string
		body map[string]any
	}{
		{name: "single item", path: "/api/lore/items/hero/image/generate", body: map[string]any{}},
		{name: "batch", path: "/api/lore/images/generate/stream", body: map[string]any{"item_ids": []string{"hero"}}},
		{name: "abort", path: "/api/lore/images/generate/abort", body: map[string]any{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := performWorkspaceChangeRequest(t, server, http.MethodPost, test.path, staleWorkspace, test.body)
			assertWorkspaceChangedResponse(t, response, staleWorkspace, application.Workspace())
		})
	}
}

func TestLoreAPIRequiresWorkspaceIdentity(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	tests := []struct {
		name   string
		method string
		path   string
		body   map[string]any
	}{
		{name: "items", method: http.MethodGet, path: "/api/lore/items"},
		{name: "classification preview", method: http.MethodPost, path: "/api/lore/classification/preview", body: map[string]any{"mode": "heuristic"}},
		{name: "single image", method: http.MethodPost, path: "/api/lore/items/hero/image/generate", body: map[string]any{}},
		{name: "batch images", method: http.MethodPost, path: "/api/lore/images/generate/stream", body: map[string]any{"item_ids": []string{"hero"}}},
		{name: "abort images", method: http.MethodPost, path: "/api/lore/images/generate/abort", body: map[string]any{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := performJSONRequest(t, server, test.method, test.path, test.body)
			assertWorkspaceChangedResponse(t, response, "", application.Workspace())
		})
	}
}
