package api

import (
	"net/http"
	"net/url"
	"testing"
)

func TestAgentChatHistorySearchAndPagination(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	createBook := performJSONRequest(t, server, http.MethodPost, "/api/books/create", map[string]string{"title": "History Book"})
	if createBook.Code != http.StatusOK {
		t.Fatalf("create book status = %d body=%s", createBook.Code, createBook.Body.String())
	}
	var createdBook struct {
		Workspace string `json:"workspace"`
	}
	decodeResponse(t, createBook.Body.Bytes(), &createdBook)
	workspace := createdBook.Workspace
	projects := performJSONRequest(t, server, http.MethodGet, "/api/agent-chat/projects", nil)
	if projects.Code != http.StatusOK {
		t.Fatalf("projects status = %d body=%s", projects.Code, projects.Body.String())
	}
	var projectList struct {
		Projects []struct {
			ID   string `json:"id"`
			Path string `json:"path"`
		} `json:"projects"`
	}
	decodeResponse(t, projects.Body.Bytes(), &projectList)
	projectID := ""
	for _, project := range projectList.Projects {
		if project.Path == workspace {
			projectID = project.ID
			break
		}
	}
	if projectID == "" {
		t.Fatalf("created workspace %q was not registered as a project: %#v", workspace, projectList)
	}
	created, err := application.AgentChat().CreateSession(workspace, "Needle conversation")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.AgentChat().CreateSession(workspace, "Other conversation"); err != nil {
		t.Fatal(err)
	}

	response := performJSONRequest(t, server, http.MethodGet, "/api/agent-chat/history?query="+url.QueryEscape("needle")+"&limit=1", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("history status = %d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Items []struct {
			ProjectID   string `json:"project_id"`
			ProjectName string `json:"project_name"`
			Session     struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"session"`
		} `json:"items"`
		Total   int  `json:"total"`
		Offset  int  `json:"offset"`
		HasMore bool `json:"has_more"`
	}
	decodeResponse(t, response.Body.Bytes(), &body)
	if body.Total != 1 || len(body.Items) != 1 || body.Items[0].Session.ID != created.ID {
		t.Fatalf("unexpected history response: %#v", body)
	}
	if body.Items[0].ProjectID != projectID || body.Items[0].ProjectName != "History Book" ||
		body.Items[0].Session.Title != "Needle conversation" || body.Offset != 0 || body.HasMore {
		t.Fatalf("unexpected history item metadata: %#v", body)
	}

	otherBook := performJSONRequest(t, server, http.MethodPost, "/api/books/create", map[string]string{"title": "Other History Book"})
	if otherBook.Code != http.StatusOK {
		t.Fatalf("create other book status = %d body=%s", otherBook.Code, otherBook.Body.String())
	}
	var createdOtherBook struct {
		Workspace string `json:"workspace"`
	}
	decodeResponse(t, otherBook.Body.Bytes(), &createdOtherBook)
	if _, err := application.AgentChat().CreateSession(createdOtherBook.Workspace, "Newest conversation"); err != nil {
		t.Fatal(err)
	}

	paginated := performJSONRequest(t, server, http.MethodGet, "/api/agent-chat/history?query=conversation&limit=1", nil)
	if paginated.Code != http.StatusOK {
		t.Fatalf("paginated history status = %d body=%s", paginated.Code, paginated.Body.String())
	}
	decodeResponse(t, paginated.Body.Bytes(), &body)
	if body.Total != 3 || len(body.Items) != 1 || !body.HasMore {
		t.Fatalf("unexpected paginated history response: %#v", body)
	}

	filtered := performJSONRequest(t, server, http.MethodGet,
		"/api/agent-chat/history?query=conversation&project_id="+url.QueryEscape(projectID)+"&limit=1", nil)
	if filtered.Code != http.StatusOK {
		t.Fatalf("filtered history status = %d body=%s", filtered.Code, filtered.Body.String())
	}
	decodeResponse(t, filtered.Body.Bytes(), &body)
	if body.Total != 2 || len(body.Items) != 1 || body.Items[0].ProjectID != projectID || !body.HasMore {
		t.Fatalf("history was not filtered to the requested project: %#v", body)
	}

	invalid := performJSONRequest(t, server, http.MethodGet, "/api/agent-chat/history?offset=-1", nil)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid offset status = %d body=%s", invalid.Code, invalid.Body.String())
	}
}
