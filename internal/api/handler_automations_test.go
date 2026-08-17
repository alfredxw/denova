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
	created, err := application.Automation().Create(automation.TaskDefinition{
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

func TestAutomationCreateAcceptsOnlyDefinitionFields(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	resp := performJSONRequest(t, server, http.MethodPost, "/api/automations", map[string]any{
		"id":         "caller-owned-id",
		"catalog_id": "caller-owned-catalog",
		"revision":   "caller-owned-revision",
		"scope":      automation.ScopeWorkspace,
		"enabled":    true,
		"name":       "Definition only",
		"template":   automation.TemplateReview,
		"prompt":     "review the project",
		"trigger_state": map[string]any{
			"schedule": map[string]any{"last_evidence_fingerprint": "caller-state"},
		},
		"last_run":    map[string]any{"id": "caller-run", "status": automation.RunStatusSuccess},
		"recent_runs": []map[string]any{{"id": "caller-run", "status": automation.RunStatusSuccess}},
		"created_at":  "2000-01-01T00:00:00Z",
		"updated_at":  "2000-01-01T00:00:00Z",
		"archived_at": "2000-01-01T00:00:00Z",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", resp.Code, resp.Body.String())
	}
	var created automation.Task
	decodeResponse(t, resp.Body.Bytes(), &created)
	if created.ID == "" || created.ID == "caller-owned-id" || created.CatalogID == "caller-owned-catalog" || created.Revision == "caller-owned-revision" {
		t.Fatalf("caller controlled runtime identity: %#v", created)
	}
	if len(created.TriggerState) != 0 || created.LastRun != nil || len(created.RecentRuns) != 0 || created.ArchivedAt != nil {
		t.Fatalf("caller controlled runtime state: %#v", created)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() || created.CreatedAt.Year() == 2000 || created.UpdatedAt.Year() == 2000 {
		t.Fatalf("caller controlled timestamps: created=%s updated=%s", created.CreatedAt, created.UpdatedAt)
	}
}

func TestAutomationRunUsesDurableJSONAdmissionAndRemovesPrivateChatRoutes(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	created, err := application.Automation().Create(automation.TaskDefinition{
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
	created, err := application.Automation().Create(automation.TaskDefinition{
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
	created, err := application.Automation().Create(automation.TaskDefinition{
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
