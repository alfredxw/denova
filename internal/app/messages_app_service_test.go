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

func TestMergeMessagesDedupesAndSortsByPublishedTime(t *testing.T) {
	older := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	newer := time.Date(2026, 7, 18, 12, 30, 0, 0, time.UTC).Format(time.RFC3339)

	changelog := []messages.Message{
		{ID: "changelog:v0.2.0:", Type: messages.MessageTypeChangelog, Title: "v0.2.0", PublishedAt: older},
	}
	dynamic := []messages.Message{
		{ID: "automation-run:run-1", Type: messages.MessageTypeAutomation, Title: "run", PublishedAt: newer},
		// Duplicate id should be dropped (changelog wins since it's first).
		{ID: "changelog:v0.2.0:", Type: messages.MessageTypeChangelog, Title: "dup", PublishedAt: newer},
		// Empty id should be skipped.
		{ID: "  ", Type: messages.MessageTypeAutomation, Title: "empty"},
	}

	merged := mergeMessages(changelog, dynamic)
	if len(merged) != 2 {
		t.Fatalf("merged length = %d, want 2 (deduped + empty id skipped): %#v", len(merged), merged)
	}
	// Sorted newest first.
	if merged[0].ID != "automation-run:run-1" {
		t.Fatalf("first merged = %s, want automation-run:run-1", merged[0].ID)
	}
	if merged[1].ID != "changelog:v0.2.0:" {
		t.Fatalf("second merged = %s, want changelog:v0.2.0:", merged[1].ID)
	}
	// ReadAt is always nil after merge (applied separately).
	for _, item := range merged {
		if item.ReadAt != nil {
			t.Fatalf("item %s should have nil ReadAt after merge", item.ID)
		}
	}
}

func TestApplyMessageReadStateCountsUnread(t *testing.T) {
	readTime := time.Date(2026, 7, 18, 12, 30, 0, 0, time.UTC)
	items := []messages.Message{
		{ID: "read-1"},
		{ID: "unread-1"},
		{ID: "read-2"},
	}
	state := map[string]time.Time{
		"read-1": readTime,
		"read-2": readTime,
	}

	unread := applyMessageReadState(items, state)
	if unread != 1 {
		t.Fatalf("unread count = %d, want 1", unread)
	}
	if items[0].ReadAt == nil || items[2].ReadAt == nil {
		t.Fatalf("read items should have ReadAt set: %#v", items)
	}
	if items[1].ReadAt != nil {
		t.Fatalf("unread item should have nil ReadAt: %#v", items[1])
	}
}
