package api

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestProjectFilesAPIKeepsBackgroundProjectScoped(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	foreground := application.Workspace()
	background := filepath.Join(t.TempDir(), "background-project")
	if err := os.MkdirAll(filepath.Join(background, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(background, "src", "main.ts"), []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	record, err := application.AgentChat().AddProject(background)
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/projects/" + url.PathEscape(record.ID) + "/files"

	listResponse := performJSONRequest(t, server, http.MethodPost, base+"/resolve", map[string]any{
		"targets":                         []map[string]string{{"id": "root", "path": ""}},
		"follow_single_child_directories": true,
	})
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", listResponse.Code, listResponse.Body.String())
	}
	var listed struct {
		ProjectID string `json:"project_id"`
		Results   []struct {
			OK          bool `json:"ok"`
			Directories []struct {
				Path    string `json:"path"`
				Entries []struct {
					Path string `json:"path"`
					Type string `json:"type"`
				} `json:"entries"`
			} `json:"directories"`
		} `json:"results"`
	}
	decodeResponse(t, listResponse.Body.Bytes(), &listed)
	if listed.ProjectID != record.ID || len(listed.Results) != 1 || !listed.Results[0].OK || len(listed.Results[0].Directories) != 2 {
		t.Fatalf("unexpected directory response: %#v", listed)
	}
	root := listed.Results[0].Directories[0]
	if len(root.Entries) != 1 || root.Entries[0].Path != "src" || root.Entries[0].Type != "dir" {
		t.Fatalf("unexpected root response: %#v", root)
	}

	filePath := url.QueryEscape("src/main.ts")
	readResponse := performJSONRequest(t, server, http.MethodGet, base+"/file?path="+filePath, nil)
	if readResponse.Code != http.StatusOK {
		t.Fatalf("read status = %d body=%s", readResponse.Code, readResponse.Body.String())
	}
	var document struct {
		Content  string `json:"content"`
		Revision string `json:"revision"`
	}
	decodeResponse(t, readResponse.Body.Bytes(), &document)
	if document.Content != "before\n" || document.Revision == "" {
		t.Fatalf("unexpected file response: %#v", document)
	}

	saveResponse := performJSONRequest(t, server, http.MethodPut, base+"/file", map[string]string{
		"path":          "src/main.ts",
		"content":       "after\n",
		"base_revision": document.Revision,
	})
	if saveResponse.Code != http.StatusOK {
		t.Fatalf("save status = %d body=%s", saveResponse.Code, saveResponse.Body.String())
	}
	content, err := os.ReadFile(filepath.Join(background, "src", "main.ts"))
	if err != nil || string(content) != "after\n" {
		t.Fatalf("background content = %q err=%v", content, err)
	}
	if application.Workspace() != foreground {
		t.Fatalf("file API switched foreground workspace: got=%q want=%q", application.Workspace(), foreground)
	}

	staleResponse := performJSONRequest(t, server, http.MethodPut, base+"/file", map[string]string{
		"path":          "src/main.ts",
		"content":       "stale\n",
		"base_revision": document.Revision,
	})
	if staleResponse.Code != http.StatusConflict {
		t.Fatalf("stale save status = %d body=%s", staleResponse.Code, staleResponse.Body.String())
	}
	var staleError struct {
		Code string `json:"code"`
	}
	decodeResponse(t, staleResponse.Body.Bytes(), &staleError)
	if staleError.Code != "revision_conflict" {
		t.Fatalf("stale save code = %q body=%s", staleError.Code, staleResponse.Body.String())
	}
}

func TestProjectFileOperationsAPIReportsPartialSuccess(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	workspace := filepath.Join(t.TempDir(), "general-project")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "existing.txt"), []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	record, err := application.AgentChat().AddProject(workspace)
	if err != nil {
		t.Fatal(err)
	}
	response := performJSONRequest(t, server, http.MethodPost, "/api/projects/"+url.PathEscape(record.ID)+"/files/operations", map[string]any{
		"operations": []map[string]string{
			{"id": "created", "kind": "create", "path": "created.txt", "type": "file", "content": "created"},
			{"id": "duplicate", "kind": "create", "path": "existing.txt", "type": "file"},
		},
	})
	if response.Code != http.StatusOK {
		t.Fatalf("operations status = %d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Results []struct {
			ID   string `json:"id"`
			OK   bool   `json:"ok"`
			Code string `json:"code"`
		} `json:"results"`
	}
	decodeResponse(t, response.Body.Bytes(), &body)
	if len(body.Results) != 2 || !body.Results[0].OK || body.Results[1].OK || body.Results[1].Code != "target_exists" {
		t.Fatalf("unexpected operation results: %#v", body.Results)
	}
	content, err := os.ReadFile(filepath.Join(workspace, "created.txt"))
	if err != nil || string(content) != "created" {
		t.Fatalf("successful operation was not retained: content=%q err=%v", content, err)
	}
}
