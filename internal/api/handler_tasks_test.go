package api

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"denova/config"
	"denova/internal/app"
	"denova/internal/automation"
	"denova/internal/book"
)

func TestTaskCenterListsActionableAutomationWorkAcrossProjectsAPI(t *testing.T) {
	dataDir := t.TempDir()
	firstProject := filepath.Join(dataDir, "project-one")
	secondProject := filepath.Join(dataDir, "project-two")
	for _, project := range []string{firstProject, secondProject} {
		if err := book.NewState(project).InitWorkspace(); err != nil {
			t.Fatalf("InitWorkspace(%q) failed: %v", project, err)
		}
	}
	application, err := app.New(context.Background(), &config.Config{
		OpenAIModel:         "test-model",
		NovaDir:             dataDir,
		Workspace:           firstProject,
		ResumeLastWorkspace: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)

	failedDefinition, err := application.CreateAutomation(automation.Task{
		Scope:    automation.ScopeWorkspace,
		Name:     "Project one review",
		Template: automation.TemplateReview,
		Prompt:   "Review the current draft",
	})
	if err != nil {
		t.Fatalf("CreateAutomation(first project) failed: %v", err)
	}
	failedAt := time.Date(2026, 8, 3, 9, 30, 0, 0, time.UTC)
	if _, err := automation.NewStore(dataDir, firstProject).AppendRun(failedDefinition.ID, automation.RunRecord{
		ID:         "run-failed",
		TaskID:     failedDefinition.ID,
		Scope:      automation.ScopeWorkspace,
		Workspace:  firstProject,
		Trigger:    automation.TriggerManual,
		Status:     automation.RunStatusFailed,
		StartedAt:  failedAt,
		FinishedAt: failedAt.Add(2 * time.Minute),
		Error:      "model unavailable",
	}); err != nil {
		t.Fatalf("AppendRun(first project) failed: %v", err)
	}

	if _, err := application.SwitchWorkspace(context.Background(), secondProject); err != nil {
		t.Fatalf("SwitchWorkspace(second project) failed: %v", err)
	}
	waitingDefinition, err := application.CreateAutomation(automation.Task{
		Scope:    automation.ScopeWorkspace,
		Name:     "Project two continuation",
		Template: automation.TemplateContinueWriting,
		Prompt:   "Continue the latest chapter",
	})
	if err != nil {
		t.Fatalf("CreateAutomation(second project) failed: %v", err)
	}
	waitingAt := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	if _, err := automation.NewStore(dataDir, secondProject).CreateInboxItem(automation.TriggerInboxItem{
		ID:           "inbox-waiting",
		TaskID:       waitingDefinition.ID,
		TriggerID:    "manual-review",
		Purpose:      automation.InboxPurposeTrigger,
		Scope:        automation.ScopeWorkspace,
		Workspace:    secondProject,
		ActionPolicy: automation.ActionPolicyConfirm,
		NotifyPolicy: automation.NotifyPolicyInbox,
		Title:        "Continue chapter",
		Summary:      "A continuation is ready for approval",
		CreatedAt:    waitingAt,
		UpdatedAt:    waitingAt,
	}); err != nil {
		t.Fatalf("CreateInboxItem(second project) failed: %v", err)
	}

	server := NewServer(application, "0")
	response := performJSONRequest(t, server, http.MethodGet, "/api/tasks", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("GET /api/tasks status = %d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Tasks []struct {
			ID      string `json:"id"`
			Type    string `json:"type"`
			Status  string `json:"status"`
			Title   string `json:"title"`
			Project struct {
				Name string `json:"name"`
				Path string `json:"path"`
			} `json:"project"`
			StartedAt time.Time `json:"started_at"`
			UpdatedAt time.Time `json:"updated_at"`
			Recovery  struct {
				Kind      string `json:"kind"`
				Workspace string `json:"workspace"`
				TaskID    string `json:"task_id,omitempty"`
				RunID     string `json:"run_id,omitempty"`
				InboxID   string `json:"inbox_id,omitempty"`
			} `json:"recovery"`
		} `json:"tasks"`
		ActionRequiredCount int `json:"action_required_count"`
	}
	decodeResponse(t, response.Body.Bytes(), &payload)
	if payload.ActionRequiredCount != 2 || len(payload.Tasks) != 2 {
		t.Fatalf("task center summary = count %d tasks %#v", payload.ActionRequiredCount, payload.Tasks)
	}

	byID := make(map[string]struct {
		Type, Status, Title, ProjectName, ProjectPath, RecoveryKind, RecoveryWorkspace, TaskID, RunID, InboxID string
		StartedAt, UpdatedAt                                                                                   time.Time
	}, len(payload.Tasks))
	for _, task := range payload.Tasks {
		byID[task.ID] = struct {
			Type, Status, Title, ProjectName, ProjectPath, RecoveryKind, RecoveryWorkspace, TaskID, RunID, InboxID string
			StartedAt, UpdatedAt                                                                                   time.Time
		}{task.Type, task.Status, task.Title, task.Project.Name, task.Project.Path, task.Recovery.Kind, task.Recovery.Workspace, task.Recovery.TaskID, task.Recovery.RunID, task.Recovery.InboxID, task.StartedAt, task.UpdatedAt}
	}
	failed := byID["automation:run-failed"]
	if failed.Type != "automation" || failed.Status != "failed" || failed.Title != "Project one review" || failed.ProjectName != "project-one" || failed.ProjectPath != firstProject || failed.RecoveryKind != "automation_run" || failed.RecoveryWorkspace != firstProject || failed.TaskID != failedDefinition.ID || failed.RunID != "run-failed" || !failed.StartedAt.Equal(failedAt) || !failed.UpdatedAt.Equal(failedAt.Add(2*time.Minute)) {
		t.Fatalf("failed task = %#v", failed)
	}
	waiting := byID["automation-inbox:inbox-waiting"]
	if waiting.Type != "automation" || waiting.Status != "waiting_user" || waiting.Title != "Continue chapter" || waiting.ProjectName != "project-two" || waiting.ProjectPath != secondProject || waiting.RecoveryKind != "automation_inbox" || waiting.RecoveryWorkspace != secondProject || waiting.TaskID != waitingDefinition.ID || waiting.InboxID != "inbox-waiting" || !waiting.StartedAt.Equal(waitingAt) || !waiting.UpdatedAt.Equal(waitingAt) {
		t.Fatalf("waiting task = %#v", waiting)
	}
}
