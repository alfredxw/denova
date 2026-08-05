package api

import (
	"net/http"
	"net/url"
	"testing"
)

func TestConversationConfigRequiresProjectRouteForProjectModes(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	sessions, err := application.Sessions()
	if err != nil || len(sessions) == 0 {
		t.Fatalf("list Writing sessions: count=%d err=%v", len(sessions), err)
	}
	query := "?mode=writing&session_id=" + url.QueryEscape(sessions[0].ID)

	scoped := performJSONRequest(
		t, server, http.MethodGet,
		"/api/projects/"+url.PathEscape(application.ProjectID())+"/conversation-config"+query,
		nil,
	)
	if scoped.Code != http.StatusOK {
		t.Fatalf("Project conversation config status=%d body=%s", scoped.Code, scoped.Body.String())
	}

	unscoped := performJSONRequest(t, server, http.MethodGet, "/api/conversation-config"+query, nil)
	if unscoped.Code != http.StatusBadRequest {
		t.Fatalf("unscoped Project conversation config status=%d body=%s", unscoped.Code, unscoped.Body.String())
	}
}
