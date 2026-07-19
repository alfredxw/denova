package api

import (
	"net/http"
	"testing"

	"denova/internal/automation"
)

func TestAutomationUpdateRejectsStaleRevisionAPI(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	created, err := application.CreateAutomation(automation.Task{
		Scope:    automation.ScopeWorkspace,
		Name:     "Review",
		Template: automation.TemplateReview,
		Prompt:   "original",
	})
	if err != nil {
		t.Fatalf("CreateAutomation failed: %v", err)
	}
	agent, err := application.UpdateAutomation(created.ID, automation.Task{Prompt: "agent update"})
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
	tasks, err := application.Automations()
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
	created, err := application.CreateAutomation(automation.Task{
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

	tasks, err := application.Automations()
	if err != nil {
		t.Fatalf("list tasks failed: %v", err)
	}
	for _, task := range tasks {
		if task.ID == created.ID && task.Prompt != "original" {
			t.Fatalf("unrevisioned API update overwrote task: %q", task.Prompt)
		}
	}
}
