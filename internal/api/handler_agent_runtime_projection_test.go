package api

import (
	"net/http"
	"testing"
)

func TestActiveEndpointsExposeDurableRuntimeProjection(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	writingResponse := performJSONRequest(t, server, http.MethodGet, "/api/chat/active?session_id="+activeWritingSessionID(t, application), nil)
	if writingResponse.Code != http.StatusOK {
		t.Fatalf("writing active status=%d body=%s", writingResponse.Code, writingResponse.Body.String())
	}
	assertIdleRuntimeProjection(t, writingResponse.Body.Bytes())

	createResponse := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories", map[string]string{
		"title":           "投影测试",
		"origin":          "测试游戏运行时投影。",
		"story_teller_id": "classic",
	})
	if createResponse.Code != http.StatusOK {
		t.Fatalf("create story status=%d body=%s", createResponse.Code, createResponse.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	decodeResponse(t, createResponse.Body.Bytes(), &created)
	gameResponse := performJSONRequest(t, server, http.MethodGet, "/api/interactive/chat/active?story_id="+created.ID+"&branch=main", nil)
	if gameResponse.Code != http.StatusOK {
		t.Fatalf("game active status=%d body=%s", gameResponse.Code, gameResponse.Body.String())
	}
	assertIdleRuntimeProjection(t, gameResponse.Body.Bytes())
}

func assertIdleRuntimeProjection(t *testing.T, body []byte) {
	t.Helper()
	var response struct {
		Active            bool   `json:"active"`
		Cursor            uint64 `json:"cursor"`
		Phase             string `json:"phase"`
		RecoveryPaused    bool   `json:"recovery_paused"`
		ActiveOperationID string `json:"active_operation_id"`
		ActiveCycle       int    `json:"active_cycle"`
		ActiveOutput      struct {
			Content  string `json:"content"`
			Thinking string `json:"thinking"`
		} `json:"active_output"`
		Queue              []any `json:"queue"`
		OpenTools          []any `json:"open_tools"`
		RuntimeRecoverable bool  `json:"runtime_recoverable"`
		StreamAttached     bool  `json:"stream_attached"`
		RecoveryActions    []any `json:"recovery_actions"`
	}
	decodeResponse(t, body, &response)
	if response.Active || response.Cursor != 0 || response.Phase != "idle" || response.ActiveOperationID != "" || response.ActiveCycle != 0 {
		t.Fatalf("runtime projection identity mismatch: %#v body=%s", response, body)
	}
	if response.Queue == nil || response.OpenTools == nil || response.RecoveryActions == nil {
		t.Fatalf("runtime projection collections must be explicit empty arrays: %s", body)
	}
	if response.RuntimeRecoverable || response.StreamAttached || len(response.RecoveryActions) != 0 {
		t.Fatalf("idle runtime must not advertise recovery or an attached stream: %#v body=%s", response, body)
	}
}
