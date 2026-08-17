package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestStaleWritingTaskStreamRequiresTypedRehydration(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	response := performJSONRequest(t, server, http.MethodGet, "/api/chat/stream?task_id=task-from-old-process&session_id="+activeWritingSessionID(t, application), nil)
	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Code    string         `json:"code"`
		Details map[string]any `json:"details"`
	}
	decodeResponse(t, response.Body.Bytes(), &body)
	if body.Code != "agent_runtime.rehydrate_required" || body.Details["task_id"] != "task-from-old-process" {
		t.Fatalf("stale stream response = %#v", body)
	}
	if _, leaked := body.Details["replacement_task_id"]; leaked {
		t.Fatalf("stale stream leaked replacement identity: %#v", body.Details)
	}
}

func TestStaleInteractiveTaskStreamRequiresTypedRehydration(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	response := performJSONRequest(t, server, http.MethodGet, "/api/interactive/chat/stream?story_id=story-from-old-process&branch=main&task_id=task-from-old-process", nil)
	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Code    string         `json:"code"`
		Details map[string]any `json:"details"`
	}
	decodeResponse(t, response.Body.Bytes(), &body)
	if body.Code != "agent_runtime.rehydrate_required" || body.Details["task_id"] != "task-from-old-process" {
		t.Fatalf("stale game stream response = %#v", body)
	}
	if _, leaked := body.Details["replacement_task_id"]; leaked {
		t.Fatalf("stale game stream leaked replacement identity: %#v", body.Details)
	}
}

func TestRecoveryEndpointDoesNotEchoOrTrustCallerPayload(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	request := map[string]any{
		"session_id": activeWritingSessionID(t, application),
		"action": map[string]any{
			"kind": "start_turn", "command_id": "not-durable", "operation_id": "not-durable",
			"descriptor": map[string]any{"secret": "DO_NOT_ECHO"},
			"input":      map[string]any{"message": "DO_NOT_ECHO"},
		},
		"payload": "DO_NOT_ECHO",
	}
	response := performJSONRequest(t, server, http.MethodPost, "/api/chat/recovery", request)
	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "DO_NOT_ECHO") || strings.Contains(response.Body.String(), "descriptor") || strings.Contains(response.Body.String(), "payload") {
		t.Fatalf("recovery response leaked caller payload: %s", response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "agent_runtime.recovery_changed" {
		t.Fatalf("recovery error = %#v", body)
	}
}

func TestRecoveryEndpointBoundsIdentityBeforeEchoingIt(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	oversized := strings.Repeat("x", 4<<10+1)
	response := performJSONRequest(t, server, http.MethodPost, "/api/chat/recovery", map[string]any{
		"action": map[string]any{
			"kind": "start_turn", "command_id": oversized, "operation_id": "operation-1",
		},
	})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), oversized) {
		t.Fatalf("oversized recovery identity was echoed: %s", response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "agent_runtime.invalid_recovery" {
		t.Fatalf("recovery error = %#v", body)
	}
}
