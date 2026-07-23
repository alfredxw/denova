package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestInteractiveImageRequiresCallerCommandIDBeforeStoryLookup(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	response := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories/missing/images/generate", map[string]any{
		"turn_id": "turn-1",
		"source":  "manual",
		"force":   true,
	})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("missing command_id status=%d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, "command_id") || !strings.Contains(body, "无法安全重试") || !strings.Contains(body, "safe request retries") {
		t.Fatalf("missing command_id response is not bilingual: %s", body)
	}
}

func TestInteractiveImageRejectsOversizedCommandIDAsBilingualBadRequest(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	response := performJSONRequest(t, server, http.MethodPost, "/api/interactive/stories/missing/images/generate", map[string]any{
		"command_id": strings.Repeat("x", 4097),
		"turn_id":    "turn-1",
		"source":     "manual",
		"force":      true,
	})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("oversized command_id status=%d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, "command_id") || !strings.Contains(body, "请求标识无效") || !strings.Contains(body, "invalid request identifier") {
		t.Fatalf("oversized command_id response is not bilingual: %s", body)
	}
}
