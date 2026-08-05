package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	runtimeapp "denova/internal/app"
)

func configManagerAPIPath(application *runtimeapp.App, suffix string) string {
	return "/api/projects/" + url.PathEscape(application.ProjectID()) + "/config-manager" + suffix
}

func TestConfigManagerRequiresCallerCommandIDBeforeStartingTask(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	response := performJSONRequest(t, server, http.MethodPost, configManagerAPIPath(application, "/stream"), map[string]any{
		"instruction": "update configuration",
	})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("missing command_id status=%d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, "command_id") || !strings.Contains(body, "无法安全重试") || !strings.Contains(body, "safe request retries") {
		t.Fatalf("missing command_id response is not bilingual: %s", body)
	}
}

func TestConfigManagerRejectsOversizedCommandIDAsBilingualBadRequest(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	response := performJSONRequest(t, server, http.MethodPost, configManagerAPIPath(application, "/stream"), map[string]any{
		"command_id":  strings.Repeat("x", 4097),
		"instruction": "update configuration",
	})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("oversized command_id status=%d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, "command_id") || !strings.Contains(body, "请求标识无效") || !strings.Contains(body, "invalid request identifier") {
		t.Fatalf("oversized command_id response is not bilingual: %s", body)
	}
}

func TestConfigManagerActiveExposesExplicitIdleRuntimeForExactScope(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	for _, origin := range []string{"settings", "story-settings", "agent-settings"} {
		response := performJSONRequest(t, server, http.MethodGet, configManagerAPIPath(application, "/active")+"?origin="+origin+"&resource_id=resource-1", nil)
		if response.Code != http.StatusOK {
			t.Fatalf("origin=%s status=%d body=%s", origin, response.Code, response.Body.String())
		}
		assertIdleRuntimeProjection(t, response.Body.Bytes())
	}
}

func TestConfigManagerTaskStreamRequiresExactScopedTaskID(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")

	missing := performJSONRequest(t, server, http.MethodGet, configManagerAPIPath(application, "/stream")+"?origin=settings", nil)
	if missing.Code != http.StatusBadRequest || !strings.Contains(missing.Body.String(), "task_id") ||
		!strings.Contains(missing.Body.String(), "精确恢复") || !strings.Contains(missing.Body.String(), "exact Agent stream recovery") {
		t.Fatalf("missing task identity status=%d body=%s", missing.Code, missing.Body.String())
	}

	stale := performJSONRequest(t, server, http.MethodGet, configManagerAPIPath(application, "/stream")+"?origin=settings&resource_id=resource-1&task_id=task-from-old-scope", nil)
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale task status=%d body=%s", stale.Code, stale.Body.String())
	}
	var body struct {
		Code    string         `json:"code"`
		Details map[string]any `json:"details"`
	}
	decodeResponse(t, stale.Body.Bytes(), &body)
	if body.Code != "agent_runtime.rehydrate_required" || body.Details["task_id"] != "task-from-old-scope" {
		t.Fatalf("stale task response = %#v", body)
	}
}

func TestConfigManagerRecoveryDoesNotTrustOrEchoCallerPayload(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	response := performJSONRequest(t, server, http.MethodPost, configManagerAPIPath(application, "/recovery")+"?origin=settings&resource_id=resource-1", map[string]any{
		"action": map[string]any{
			"kind": "follow_up", "command_id": "not-durable", "operation_id": "not-durable",
			"input": map[string]any{"message": "CONFIG_MANAGER_SECRET"},
		},
		"payload": "CONFIG_MANAGER_SECRET",
	})
	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "CONFIG_MANAGER_SECRET") || strings.Contains(response.Body.String(), "payload") || strings.Contains(response.Body.String(), "input") {
		t.Fatalf("Config Manager recovery leaked caller payload: %s", response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "agent_runtime.recovery_changed" {
		t.Fatalf("recovery error = %#v", body)
	}
}
