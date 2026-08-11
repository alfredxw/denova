package api

import (
	"net/http"
	"testing"
)

func TestSystemNotificationsDefaultOffAndPersistAsUserPreference(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	var initial settingsSnapshot
	initialResp := performJSONRequest(t, server, http.MethodGet, "/api/settings", nil)
	if initialResp.Code != http.StatusOK {
		t.Fatalf("settings status = %d body=%s", initialResp.Code, initialResp.Body.String())
	}
	decodeResponse(t, initialResp.Body.Bytes(), &initial)
	if initial.Effective.SystemNotificationsEnabled == nil || *initial.Effective.SystemNotificationsEnabled {
		t.Fatalf("system notifications should default off: %#v", initial.Effective.SystemNotificationsEnabled)
	}

	updateResp := performJSONRequest(t, server, http.MethodPut, "/api/settings/user", map[string]any{
		"settings": map[string]any{"system_notifications_enabled": true},
	})
	if updateResp.Code != http.StatusOK {
		t.Fatalf("user settings update status = %d body=%s", updateResp.Code, updateResp.Body.String())
	}
	var updated settingsSnapshot
	decodeResponse(t, updateResp.Body.Bytes(), &updated)
	if updated.Effective.SystemNotificationsEnabled == nil || !*updated.Effective.SystemNotificationsEnabled {
		t.Fatalf("system notifications should be enabled after user update: %#v", updated.Effective.SystemNotificationsEnabled)
	}

	var persisted settingsSnapshot
	persistedResp := performJSONRequest(t, server, http.MethodGet, "/api/settings", nil)
	decodeResponse(t, persistedResp.Body.Bytes(), &persisted)
	if persisted.Effective.SystemNotificationsEnabled == nil || !*persisted.Effective.SystemNotificationsEnabled {
		t.Fatalf("system notifications should persist after reload: %#v", persisted.Effective.SystemNotificationsEnabled)
	}
}

func TestSystemNotificationsAreNotOverriddenByWorkspaceSettings(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	enableResp := performJSONRequest(t, server, http.MethodPut, "/api/settings/user", map[string]any{
		"settings": map[string]any{"system_notifications_enabled": true},
	})
	if enableResp.Code != http.StatusOK {
		t.Fatalf("enable user setting status = %d body=%s", enableResp.Code, enableResp.Body.String())
	}

	workspaceResp := performJSONRequest(t, server, http.MethodPut, "/api/settings/workspace", map[string]any{
		"settings": map[string]any{"system_notifications_enabled": false},
	})
	if workspaceResp.Code != http.StatusOK {
		t.Fatalf("workspace settings update status = %d body=%s", workspaceResp.Code, workspaceResp.Body.String())
	}
	var snapshot settingsSnapshot
	decodeResponse(t, workspaceResp.Body.Bytes(), &snapshot)
	if snapshot.Effective.SystemNotificationsEnabled == nil || !*snapshot.Effective.SystemNotificationsEnabled {
		t.Fatalf("workspace write must not override the user preference: %#v", snapshot.Effective.SystemNotificationsEnabled)
	}
}

type settingsSnapshot struct {
	Effective settingsEffective `json:"effective"`
}

type settingsEffective struct {
	SystemNotificationsEnabled *bool `json:"system_notifications_enabled"`
}
