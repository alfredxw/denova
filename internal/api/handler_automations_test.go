package api

import (
	"net/http"
	"strings"
	"testing"

	"denova/internal/automation"
)

func TestAutomationAgentStreamsRequireCallerCommandID(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	created, err := application.Automation().Create(automation.Task{
		Scope: automation.ScopeWorkspace, Name: "Review", Template: automation.TemplateReview, Prompt: "review",
	})
	if err != nil {
		t.Fatal(err)
	}

	run := performJSONRequest(t, server, http.MethodPost, "/api/automations/"+created.ID+"/run/stream", map[string]any{})
	if run.Code != http.StatusBadRequest || !strings.Contains(run.Body.String(), "command_id") || !strings.Contains(run.Body.String(), "无法安全重试") {
		t.Fatalf("run status=%d body=%s", run.Code, run.Body.String())
	}
	followUp := performJSONRequest(t, server, http.MethodPost, "/api/automations/runs/missing/chat/stream", map[string]any{"message": "continue"})
	if followUp.Code != http.StatusBadRequest || !strings.Contains(followUp.Body.String(), "command_id") || !strings.Contains(followUp.Body.String(), "follow-up") {
		t.Fatalf("follow-up status=%d body=%s", followUp.Code, followUp.Body.String())
	}
	abort := performJSONRequest(t, server, http.MethodPost, "/api/automations/runs/missing/abort", map[string]any{})
	if abort.Code != http.StatusBadRequest || !strings.Contains(abort.Body.String(), "target_operation_id") {
		t.Fatalf("abort status=%d body=%s", abort.Code, abort.Body.String())
	}
	legacy := performJSONRequest(t, server, http.MethodPost, "/api/automations/"+created.ID+"/run", map[string]any{})
	if legacy.Code != http.StatusNotFound && legacy.Code != http.StatusMethodNotAllowed {
		t.Fatalf("legacy non-idempotent run route remains reachable: status=%d body=%s", legacy.Code, legacy.Body.String())
	}
}

func TestAutomationUpdateRejectsStaleRevisionAPI(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	created, err := application.Automation().Create(automation.Task{
		Scope:    automation.ScopeWorkspace,
		Name:     "Review",
		Template: automation.TemplateReview,
		Prompt:   "original",
	})
	if err != nil {
		t.Fatalf("CreateAutomation failed: %v", err)
	}
	agent, err := application.Automation().Update(created.ID, automation.Task{Prompt: "agent update"})
	if err != nil {
		t.Fatalf("agent UpdateAutomation failed: %v", err)
	}

	resp := performJSONRequest(t, server, http.MethodPatch, "/api/automations/"+created.ID, map[string]any{
		"base_revision": created.Revision,
		"prompt":        "stale editor",
	})
	if resp.Code != http.StatusConflict {
		t.Fatalf("stale update status = %d body=%s", resp.Code, resp.Body.String())
	}
	var payload map[string]any
	decodeResponse(t, resp.Body.Bytes(), &payload)
	if payload["code"] != "revision_conflict" {
		t.Fatalf("conflict code = %#v body=%s", payload["code"], resp.Body.String())
	}
	tasks, err := application.Automation().List()
	if err != nil {
		t.Fatalf("list latest tasks failed: %v", err)
	}
	var latest automation.Task
	for _, task := range tasks {
		if task.ID == created.ID {
			latest = task
			break
		}
	}
	if latest.Prompt != agent.Prompt {
		t.Fatalf("stale API update overwrote agent content: %q", latest.Prompt)
	}
}

func TestAutomationUpdateRequiresBaseRevisionAPI(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	created, err := application.Automation().Create(automation.Task{
		Scope:    automation.ScopeWorkspace,
		Name:     "Review",
		Template: automation.TemplateReview,
		Prompt:   "original",
	})
	if err != nil {
		t.Fatalf("CreateAutomation failed: %v", err)
	}

	resp := performJSONRequest(t, server, http.MethodPatch, "/api/automations/"+created.ID, map[string]any{
		"prompt": "unrevisioned editor",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("unrevisioned update status = %d body=%s", resp.Code, resp.Body.String())
	}
	var payload map[string]any
	decodeResponse(t, resp.Body.Bytes(), &payload)
	if payload["code"] != "base_revision_required" {
		t.Fatalf("missing revision code = %#v body=%s", payload["code"], resp.Body.String())
	}

	tasks, err := application.Automation().List()
	if err != nil {
		t.Fatalf("list tasks failed: %v", err)
	}
	for _, task := range tasks {
		if task.ID == created.ID && task.Prompt != "original" {
			t.Fatalf("unrevisioned API update overwrote task: %q", task.Prompt)
		}
	}
}
