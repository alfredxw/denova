package api

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"denova/internal/automation"
)

func TestAutomationCatalogIsScopedToOwningProject(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	created, err := application.Automation().Create(automation.Task{
		Scope: automation.ScopeWorkspace, Name: "Project review", Template: automation.TemplateReview,
	})
	if err != nil {
		t.Fatal(err)
	}

	path := "/api/automations?project_id=" + url.QueryEscape(created.Target.ProjectID) + "&workspace=" + url.QueryEscape(created.Target.Workspace)
	resp := performJSONRequest(t, server, http.MethodGet, path, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("scoped catalog status=%d body=%s", resp.Code, resp.Body.String())
	}
	var result automation.ListResult
	decodeResponse(t, resp.Body.Bytes(), &result)
	if len(result.Tasks) != 1 || result.Tasks[0].ID != created.ID {
		t.Fatalf("scoped catalog=%#v", result.Tasks)
	}
	unscoped := performJSONRequest(t, server, http.MethodGet, "/api/automations", nil)
	if unscoped.Code != http.StatusBadRequest {
		t.Fatalf("unscoped catalog status=%d body=%s", unscoped.Code, unscoped.Body.String())
	}
	unscopedInbox := performJSONRequest(t, server, http.MethodGet, "/api/automations/inbox", nil)
	if unscopedInbox.Code != http.StatusBadRequest {
		t.Fatalf("unscoped inbox status=%d body=%s", unscopedInbox.Code, unscopedInbox.Body.String())
	}

	foreign := performJSONRequest(t, server, http.MethodGet, "/api/automations?project_id=foreign-project&workspace="+url.QueryEscape(created.Target.Workspace), nil)
	decodeResponse(t, foreign.Body.Bytes(), &result)
	if len(result.Tasks) != 0 {
		t.Fatalf("foreign Project catalog leaked tasks: %#v", result.Tasks)
	}
}

func TestAutomationRunUsesDurableJSONAdmissionAndRemovesPrivateChatRoutes(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	created, err := application.Automation().Create(automation.Task{
		Scope: automation.ScopeWorkspace, Name: "Review", Template: automation.TemplateReview, Prompt: "review",
	})
	if err != nil {
		t.Fatal(err)
	}

	run := performJSONRequest(t, server, http.MethodPost, "/api/automations/"+created.ID+"/run", map[string]any{})
	if run.Code != http.StatusBadRequest || !strings.Contains(run.Body.String(), "command_id") || !strings.Contains(run.Body.String(), "无法安全重试") {
		t.Fatalf("run status=%d body=%s", run.Code, run.Body.String())
	}
	for _, path := range []string{
		"/api/automations/" + created.ID + "/run/stream",
		"/api/automations/runs/missing/chat/stream",
		"/api/automations/runs/missing/abort",
		"/api/automations/runs/missing/messages",
	} {
		legacy := performJSONRequest(t, server, http.MethodPost, path, map[string]any{})
		if legacy.Code != http.StatusNotFound && legacy.Code != http.StatusMethodNotAllowed {
			t.Fatalf("legacy Automation chat route remains reachable: path=%s status=%d body=%s", path, legacy.Code, legacy.Body.String())
		}
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
