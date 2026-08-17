package api

import (
	"net/http"
	"testing"
)

func TestWritingQueueCommandsRequireAnExactQueuedCommand(t *testing.T) {
	t.Parallel()

	application := newTestApplication(t)
	server := NewServer(application, "0")

	missingTarget := performJSONRequest(t, server, http.MethodPost, "/api/chat/commands", map[string]any{
		"session_id": activeWritingSessionID(t, application), "type": "cancel_queued", "command_id": "cancel-1", "target_operation_id": "operation-1",
	})
	if missingTarget.Code != http.StatusBadRequest {
		t.Fatalf("missing target status=%d body=%s", missingTarget.Code, missingTarget.Body.String())
	}

	noActive := performJSONRequest(t, server, http.MethodPost, "/api/chat/commands", map[string]any{
		"session_id": activeWritingSessionID(t, application), "type": "steer_queued", "command_id": "steer-1", "target_operation_id": "operation-1",
		"target_command_id": "queued-1",
	})
	if noActive.Code != http.StatusConflict {
		t.Fatalf("valid queue control status=%d body=%s", noActive.Code, noActive.Body.String())
	}
}
