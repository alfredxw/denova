package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestChatRequiresCallerCommandIDBeforeStartingTask(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	response := performJSONRequest(t, server, http.MethodPost, "/api/chat", map[string]any{
		"message": "write",
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
