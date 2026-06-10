package automation

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreSeparatesUserAndWorkspaceTasks(t *testing.T) {
	root := t.TempDir()
	userDir := filepath.Join(root, "user")
	workspace := filepath.Join(root, "book")
	if err := os.MkdirAll(filepath.Join(workspace, ".nova"), 0o755); err != nil {
		t.Fatal(err)
	}
	store := NewStore(userDir, workspace)

	userTask, err := store.Create(Task{Scope: ScopeUser, Name: "User task", Template: TemplateCustomPrompt})
	if err != nil {
		t.Fatalf("create user task: %v", err)
	}
	workspaceTask, err := store.Create(Task{Scope: ScopeWorkspace, Name: "Workspace task", Template: TemplateReview})
	if err != nil {
		t.Fatalf("create workspace task: %v", err)
	}
	if _, err := os.Stat(filepath.Join(userDir, "automations", "tasks.json")); err != nil {
		t.Fatalf("user tasks not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".nova", "automations", "tasks.json")); err != nil {
		t.Fatalf("workspace tasks not written: %v", err)
	}

	tasks, err := store.List()
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("task count = %d, want 2", len(tasks))
	}

	userOnly, err := NewStore(userDir, "").List()
	if err != nil {
		t.Fatalf("list user-only: %v", err)
	}
	if len(userOnly) != 1 || userOnly[0].ID != userTask.ID {
		t.Fatalf("user-only tasks = %#v, want %s", userOnly, userTask.ID)
	}
	if _, err := NewStore(userDir, "").Get(workspaceTask.ID); err == nil {
		t.Fatalf("workspace task should not be visible without workspace")
	}
}

func TestNormalizeScheduleBuildsCronShape(t *testing.T) {
	tests := []struct {
		name     string
		schedule Schedule
		wantCron string
	}{
		{"daily", Schedule{Kind: ScheduleDaily, Hour: 9, Minute: 30}, "30 9 * * *"},
		{"weekly", Schedule{Kind: ScheduleWeekly, Weekday: 2, Hour: 8, Minute: 5}, "5 8 * * 2"},
		{"monthly", Schedule{Kind: ScheduleMonthly, DayOfMonth: 12, Hour: 7, Minute: 0}, "0 7 12 * *"},
		{"every-hours", Schedule{Kind: ScheduleEveryHours, EveryHours: 6, Minute: 15}, "15 */6 * * *"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeSchedule(tt.schedule)
			if err != nil {
				t.Fatalf("NormalizeSchedule failed: %v", err)
			}
			if got.Cron != tt.wantCron {
				t.Fatalf("cron = %q, want %q", got.Cron, tt.wantCron)
			}
		})
	}
}

func TestDueHandlesStructuredSchedules(t *testing.T) {
	now := time.Date(2026, 6, 10, 10, 0, 0, 0, time.UTC)
	last := now.Add(-25 * time.Hour)
	task := Task{
		Enabled:  true,
		Schedule: Schedule{Kind: ScheduleDaily, Hour: 9, Minute: 0},
		LastRun:  &RunRecord{StartedAt: last},
	}
	if !Due(now, task) {
		t.Fatalf("daily task should be due")
	}
	task.Enabled = false
	if Due(now, task) {
		t.Fatalf("disabled task should not be due")
	}
	task.Enabled = true
	task.Schedule = Schedule{Kind: ScheduleManual}
	if Due(now, task) {
		t.Fatalf("manual task should not be due")
	}
}

func TestNormalizeTaskAcceptsContinueWritingTemplate(t *testing.T) {
	task, err := NormalizeTask(Task{Scope: ScopeWorkspace, Name: "Continue", Template: TemplateContinueWriting})
	if err != nil {
		t.Fatalf("NormalizeTask failed: %v", err)
	}
	if task.Template != TemplateContinueWriting {
		t.Fatalf("template = %q, want %q", task.Template, TemplateContinueWriting)
	}
}
