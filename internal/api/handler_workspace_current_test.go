package api

import (
	"net/http"
	"testing"
)

func TestWorkspaceCurrentIncludesStableProjectIdentity(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	response := performJSONRequest(t, server, http.MethodGet, "/api/workspace/current", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("workspace current status = %d body=%s", response.Code, response.Body.String())
	}
	var current struct {
		Workspace string `json:"workspace"`
		ProjectID string `json:"project_id"`
	}
	decodeResponse(t, response.Body.Bytes(), &current)
	if current.Workspace == "" || current.ProjectID == "" {
		t.Fatalf("workspace current must expose workspace and project identity: %#v", current)
	}

	projects := application.AgentChat().Projects()
	for _, project := range projects {
		if project.Current {
			if current.ProjectID != project.ID {
				t.Fatalf("workspace project ID = %q, current catalog project = %q", current.ProjectID, project.ID)
			}
			return
		}
	}
	t.Fatal("current workspace project was not present in the project catalog")
}
