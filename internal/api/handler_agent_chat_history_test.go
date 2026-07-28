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
	created, err := application.CreateProjectSession(workspace, "Needle conversation")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.CreateProjectSession(workspace, "Other conversation"); err != nil {
		t.Fatal(err)
	}

	response := performJSONRequest(t, server, http.MethodGet, "/api/agent-chat/history?query="+url.QueryEscape("needle")+"&limit=1", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("history status = %d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Items []struct {
			Workspace string `json:"workspace"`
			Session   struct {
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
	if body.Items[0].Workspace == "" || body.Items[0].Session.Title != "Needle conversation" || body.Offset != 0 || body.HasMore {
		t.Fatalf("unexpected history item metadata: %#v", body)
	}

	paginated := performJSONRequest(t, server, http.MethodGet, "/api/agent-chat/history?query=conversation&limit=1", nil)
	if paginated.Code != http.StatusOK {
		t.Fatalf("paginated history status = %d body=%s", paginated.Code, paginated.Body.String())
	}
	decodeResponse(t, paginated.Body.Bytes(), &body)
	if body.Total != 2 || len(body.Items) != 1 || !body.HasMore {
		t.Fatalf("unexpected paginated history response: %#v", body)
	}

	invalid := performJSONRequest(t, server, http.MethodGet, "/api/agent-chat/history?offset=-1", nil)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid offset status = %d body=%s", invalid.Code, invalid.Body.String())
	}
}
