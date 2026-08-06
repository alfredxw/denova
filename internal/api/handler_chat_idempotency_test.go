package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestWritingAgentEndpointsRejectAStaleSessionBinding(t *testing.T) {
	application := newTestApplication(t)
	staleSessionID := activeWritingSessionID(t, application)
	if _, err := application.CreateSession("new active Session"); err != nil {
		t.Fatal(err)
	}
	server := NewServer(application, "0")

	start := performJSONRequest(t, server, http.MethodPost, "/api/chat", map[string]any{
		"session_id": staleSessionID,
		"command_id": "stale-session-http-start",
		"message":    "continue the stale Session",
	})
	if start.Code != http.StatusConflict {
		t.Fatalf("stale start status=%d body=%s", start.Code, start.Body.String())
	}
	assertAgentRuntimeErrorCode(t, start.Body.Bytes(), "agent_runtime.context_changed")

	active := performJSONRequest(t, server, http.MethodGet, "/api/chat/active?session_id="+staleSessionID, nil)
	if active.Code != http.StatusConflict {
		t.Fatalf("stale active status=%d body=%s", active.Code, active.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(active.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["code"] != "agent_runtime.context_changed" {
		t.Fatalf("stale active error=%#v", payload)
	}

	command := performJSONRequest(t, server, http.MethodPost, "/api/chat/commands", map[string]any{
		"session_id": staleSessionID, "type": "abort", "command_id": "stale-session-abort",
		"target_operation_id": "operation-from-session-a", "reason": "user_requested",
	})
	if command.Code != http.StatusConflict {
		t.Fatalf("stale command status=%d body=%s", command.Code, command.Body.String())
	}
	assertAgentRuntimeErrorCode(t, command.Body.Bytes(), "agent_runtime.context_changed")

	recovery := performJSONRequest(t, server, http.MethodPost, "/api/chat/recovery", map[string]any{
		"session_id": staleSessionID,
		"action": map[string]any{
			"kind": "follow_up", "command_id": "stale-session-recovery", "operation_id": "operation-from-session-a",
		},
	})
	if recovery.Code != http.StatusConflict {
		t.Fatalf("stale recovery status=%d body=%s", recovery.Code, recovery.Body.String())
	}
	assertAgentRuntimeErrorCode(t, recovery.Body.Bytes(), "agent_runtime.context_changed")

	stream := performJSONRequest(t, server, http.MethodGet, "/api/chat/stream?task_id=task-from-session-a&session_id="+staleSessionID, nil)
	if stream.Code != http.StatusConflict {
		t.Fatalf("stale stream status=%d body=%s", stream.Code, stream.Body.String())
	}
	assertAgentRuntimeErrorCode(t, stream.Body.Bytes(), "agent_runtime.context_changed")
}

func TestChatRequiresCallerCommandIDBeforeStartingTask(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	response := performJSONRequest(t, server, http.MethodPost, "/api/chat", map[string]any{
		"session_id": activeWritingSessionID(t, application), "message": "write",
	})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("missing command_id status=%d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, "command_id") || !strings.Contains(body, "无法安全重试") || !strings.Contains(body, "safe request retries") {
		t.Fatalf("missing command_id response is not bilingual: %s", body)
	}
}

func TestContextCompactionEndpointsRequireRetryIdentity(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	tests := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{name: "writing compact", method: http.MethodPost, path: "/api/chat/context-compaction", body: map[string]any{}},
		{name: "writing remove", method: http.MethodDelete, path: "/api/chat/context-compaction/active", body: nil},
		{name: "game compact", method: http.MethodPost, path: "/api/interactive/stories/story-1/context-compaction", body: map[string]any{"branch_id": "main"}},
		{name: "game remove", method: http.MethodDelete, path: "/api/interactive/stories/story-1/context-compaction/active?branch=main", body: nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := performJSONRequest(t, server, test.method, test.path, test.body)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			assertAgentRuntimeErrorCode(t, response.Body.Bytes(), "agent_runtime.invalid_command")
			body := response.Body.String()
			if !strings.Contains(body, "无法安全重试") || !strings.Contains(body, "safe retries") {
				t.Fatalf("retry identity error is not bilingual: %s", body)
			}
		})
	}
}
