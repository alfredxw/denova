package api

import (
	"net/http"
	"net/url"
	"testing"

	"denova/config"
	"denova/internal/agents/conversationconfig"
)

func TestSettingsAPIPartiallyMutatesOneLayerThroughOneRoute(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	first := performJSONRequest(t, server, http.MethodPatch, "/api/settings", map[string]any{
		"layer": "user",
		"changes": map[string]any{
			"theme":    "light",
			"language": "en-US",
		},
	})
	if first.Code != http.StatusOK {
		t.Fatalf("first patch status = %d body=%s", first.Code, first.Body.String())
	}

	second := performJSONRequest(t, server, http.MethodPatch, "/api/settings", map[string]any{
		"layer":   "user",
		"changes": map[string]any{"theme": "dark"},
	})
	if second.Code != http.StatusOK {
		t.Fatalf("second patch status = %d body=%s", second.Code, second.Body.String())
	}
	var layered config.LayeredSettings
	decodeResponse(t, second.Body.Bytes(), &layered)
	if layered.User.Theme != "dark" || layered.User.Language != "en-US" {
		t.Fatalf("omitted user fields must remain unchanged: %#v", layered.User)
	}

	unknown := performJSONRequest(t, server, http.MethodPatch, "/api/settings", map[string]any{
		"layer": "user", "changes": map[string]any{"retired_setting": nil},
	})
	if unknown.Code != http.StatusBadRequest {
		t.Fatalf("unknown null field status = %d body=%s", unknown.Code, unknown.Body.String())
	}
	workspaceLeak := performJSONRequest(t, server, http.MethodPatch, "/api/settings", map[string]any{
		"layer": "workspace", "changes": map[string]any{"theme": "light"},
	})
	if workspaceLeak.Code != http.StatusBadRequest {
		t.Fatalf("user-only workspace field status = %d body=%s", workspaceLeak.Code, workspaceLeak.Body.String())
	}

	for _, retiredPath := range []string{
		"/api/settings/user",
		"/api/settings/workspace",
		"/api/settings/agent-approval-mode",
	} {
		response := performJSONRequest(t, server, http.MethodPut, retiredPath, map[string]any{})
		if response.Code != http.StatusNotFound {
			t.Fatalf("retired route %s status = %d body=%s", retiredPath, response.Code, response.Body.String())
		}
	}
}

func TestConversationConfigAPIKeepsOlderSessionsIndependentAndSeedsNewSessions(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	firstSessionID := application.Session().ID

	first := readWritingConversationConfig(t, server, firstSessionID)
	configuredResponse := performJSONRequest(t, server, http.MethodPatch, "/api/conversation-config", map[string]any{
		"binding":       map[string]any{"mode": "writing", "session_id": firstSessionID},
		"base_revision": first.Revision,
		"changes": map[string]any{
			"thinking_level": "high",
			"approval_mode":  "full_access",
		},
	})
	if configuredResponse.Code != http.StatusOK {
		t.Fatalf("configure first session status = %d body=%s", configuredResponse.Code, configuredResponse.Body.String())
	}
	var configured conversationconfig.Snapshot
	decodeResponse(t, configuredResponse.Body.Bytes(), &configured)

	createdResponse := performJSONRequest(t, server, http.MethodPost, "/api/sessions", map[string]any{"title": "Inherited"})
	if createdResponse.Code != http.StatusOK {
		t.Fatalf("create session status = %d body=%s", createdResponse.Code, createdResponse.Body.String())
	}
	var created testSessionDTO
	decodeResponse(t, createdResponse.Body.Bytes(), &created)
	inherited := readWritingConversationConfig(t, server, created.ID)
	if inherited.Revision != 1 || inherited.Config != configured.Config {
		t.Fatalf("new session config = %#v, want inherited %#v", inherited, configured.Config)
	}

	updatedResponse := performJSONRequest(t, server, http.MethodPatch, "/api/conversation-config", map[string]any{
		"binding":       map[string]any{"mode": "writing", "session_id": created.ID},
		"base_revision": inherited.Revision,
		"changes": map[string]any{
			"thinking_level": "low",
			"approval_mode":  "ask",
		},
	})
	if updatedResponse.Code != http.StatusOK {
		t.Fatalf("configure second session status = %d body=%s", updatedResponse.Code, updatedResponse.Body.String())
	}

	firstAfter := readWritingConversationConfig(t, server, firstSessionID)
	if firstAfter != configured {
		t.Fatalf("older session changed with newer session: before=%#v after=%#v", configured, firstAfter)
	}
}

func readWritingConversationConfig(t *testing.T, server *Server, sessionID string) conversationconfig.Snapshot {
	t.Helper()
	path := "/api/conversation-config?mode=writing&session_id=" + url.QueryEscape(sessionID)
	response := performJSONRequest(t, server, http.MethodGet, path, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("conversation config status = %d body=%s", response.Code, response.Body.String())
	}
	var snapshot conversationconfig.Snapshot
	decodeResponse(t, response.Body.Bytes(), &snapshot)
	return snapshot
}
