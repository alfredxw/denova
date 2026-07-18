package app

import (
	"strings"
	"testing"
	"time"

	"denova/internal/automation"
	"denova/internal/messages"
)

func TestAutomationMessagesIncludeCrossWorkspaceCompletionsAndPendingActions(t *testing.T) {
	finishedAt := time.Date(2026, 7, 18, 12, 30, 0, 0, time.UTC)
	tasks := []automation.Task{{
		ID:        "task-1",
		CatalogID: "workspace-a:task-1",
		Name:      "整理人物设定",
		Target: automation.ExecutionTarget{
			Kind:      automation.TargetKindWorkspace,
			Workspace: "/books/a",
		},
		RecentRuns: []automation.RunRecord{{
			ID:         "run-1",
			TaskID:     "task-1",
			Workspace:  "/books/a",
			Status:     automation.RunStatusSuccess,
			Summary:    "已合并重复设定。",
			StartedAt:  finishedAt.Add(-time.Minute),
			FinishedAt: finishedAt,
		}, {
			ID:        "run-running",
			TaskID:    "task-1",
			Workspace: "/books/a",
			Status:    automation.RunStatusRunning,
			StartedAt: finishedAt.Add(time.Minute),
		}},
	}}
	inbox := []automation.TriggerInboxItem{{
		ID:        "inbox-1",
		TaskID:    "task-1",
		Workspace: "/books/a",
		Status:    automation.InboxStatusPending,
		Title:     "发现新章节",
		Summary:   "是否开始审查？",
		CreatedAt: finishedAt.Add(time.Minute),
	}}

	items := automationMessagesForLocale(tasks, inbox, "zh-CN")
	if len(items) != 2 {
		t.Fatalf("automation messages = %#v, want completion and action", items)
	}
	if items[0].ID != "automation-inbox:inbox-1" || items[0].Type != messages.MessageTypeAutomationAction || !items[0].ActionRequired {
		t.Fatalf("pending action message = %#v", items[0])
	}
	if items[0].TaskID != "workspace-a:task-1" || items[0].InboxID != "inbox-1" || items[0].Workspace != "/books/a" {
		t.Fatalf("pending action navigation metadata = %#v", items[0])
	}
	if items[1].ID != "automation-run:run-1" || items[1].RunID != "run-1" || items[1].Status != automation.RunStatusSuccess {
		t.Fatalf("completion message = %#v", items[1])
	}
	if !strings.Contains(items[1].Body, "已完成") || !strings.Contains(items[1].Body, "已合并重复设定") {
		t.Fatalf("localized completion body = %q", items[1].Body)
	}
}
