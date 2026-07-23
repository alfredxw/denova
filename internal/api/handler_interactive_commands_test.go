package api

import (
	"net/http"
	"testing"
)

func TestInteractiveAgentCommandEndpointUsesTypedRuntimeErrors(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	invalid := performJSONRequest(t, server, http.MethodPost, "/api/interactive/chat/commands", map[string]any{
		"type": "follow_up",
	})
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid command status=%d body=%s", invalid.Code, invalid.Body.String())
	}
	assertAgentRuntimeErrorCode(t, invalid.Body.Bytes(), "agent_runtime.invalid_command")

	create := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories", map[string]string{
		"title": "命令测试", "origin": "验证游戏 typed command。", "story_teller_id": "classic",
	})
	if create.Code != http.StatusOK {
		t.Fatalf("create story status=%d body=%s", create.Code, create.Body.String())
	}
	var story struct {
		ID string `json:"id"`
	}
	decodeResponse(t, create.Body.Bytes(), &story)

	noActive := performJSONRequest(t, server, http.MethodPost, "/api/interactive/chat/commands", map[string]any{
		"type": "abort", "command_id": "abort-1", "target_operation_id": "operation-1",
		"story_id": story.ID, "branch_id": "main", "reason": "test",
	})
	if noActive.Code != http.StatusConflict {
		t.Fatalf("no-active command status=%d body=%s", noActive.Code, noActive.Body.String())
	}
	assertAgentRuntimeErrorCode(t, noActive.Body.Bytes(), "agent_runtime.invalid_phase")
}

func assertAgentRuntimeErrorCode(t *testing.T, body []byte, want string) {
	t.Helper()
	var response struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	decodeResponse(t, body, &response)
	if response.Code != want || response.Error == "" {
		t.Fatalf("runtime error = %#v, want code %q body=%s", response, want, body)
	}
}
