package app

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"denova/internal/automation"
	"denova/internal/messages"
)

const maxAutomationNotificationMessages = 200

// MessagesAppService owns cross-source message merging. It composes changelog
// messages (from messages.Service) with dynamic automation notices, then
// applies the shared read state. Keeping the merge in the app layer means
// messages.Service stays a focused changelog + read-state persistence service.
func (a *App) Messages(locale string) (messages.ListResult, error) {
	changelog, dynamic, err := a.messageSources(locale)
	if err != nil {
		return messages.ListResult{}, err
	}
	merged := mergeMessages(changelog, dynamic)
	state, err := messages.NewService(a.novaDir()).ReadState()
	if err != nil {
		return messages.ListResult{}, err
	}
	unread := applyMessageReadState(merged, state)
	return messages.ListResult{Items: merged, UnreadCount: unread}, nil
}

func (a *App) MarkMessageRead(id, locale string) (messages.Message, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return messages.Message{}, fmt.Errorf("message id is required")
	}
	changelog, dynamic, err := a.messageSources(locale)
	if err != nil {
		return messages.Message{}, err
	}
	merged := mergeMessages(changelog, dynamic)
	var found *messages.Message
	for i := range merged {
		if merged[i].ID == id {
			found = &merged[i]
			break
		}
	}
	if found == nil {
		return messages.Message{}, fmt.Errorf("message %s not found", id)
	}
	svc := messages.NewService(a.novaDir())
	state, err := svc.ReadState()
	if err != nil {
		return messages.Message{}, err
	}
	if t, ok := state[id]; ok {
		found.ReadAt = &t
		return *found, nil
	}
	readAt, err := svc.MarkRead(id)
	if err != nil {
		return messages.Message{}, err
	}
	found.ReadAt = &readAt
	return *found, nil
}

func (a *App) MarkAllMessagesRead(locale string) (messages.ListResult, error) {
	changelog, dynamic, err := a.messageSources(locale)
	if err != nil {
		return messages.ListResult{}, err
	}
	merged := mergeMessages(changelog, dynamic)
	ids := make([]string, 0, len(merged))
	for _, item := range merged {
		if strings.TrimSpace(item.ID) != "" {
			ids = append(ids, item.ID)
		}
	}
	if err := messages.NewService(a.novaDir()).MarkAllRead(ids); err != nil {
		return messages.ListResult{}, err
	}
	state, err := messages.NewService(a.novaDir()).ReadState()
	if err != nil {
		return messages.ListResult{}, err
	}
	applyMessageReadState(merged, state)
	return messages.ListResult{Items: merged, UnreadCount: 0}, nil
}

// messageSources fetches both changelog and dynamic messages.
func (a *App) messageSources(locale string) ([]messages.Message, []messages.Message, error) {
	svc := messages.NewService(a.novaDir())
	changelog, err := svc.ChangelogForLocale(locale)
	if err != nil {
		return nil, nil, err
	}
	dynamic, err := a.automationNotificationMessages(locale)
	if err != nil {
		return nil, nil, err
	}
	return changelog, dynamic, nil
}

// mergeMessages combines changelog and dynamic messages, deduplicates by id,
// and sorts by published time descending (newest first).
func mergeMessages(changelog, dynamic []messages.Message) []messages.Message {
	merged := make([]messages.Message, 0, len(changelog)+len(dynamic))
	seen := make(map[string]struct{}, len(changelog)+len(dynamic))
	for _, item := range append(changelog, dynamic...) {
		item.ID = strings.TrimSpace(item.ID)
		if item.ID == "" {
			continue
		}
		if _, exists := seen[item.ID]; exists {
			continue
		}
		seen[item.ID] = struct{}{}
		item.ReadAt = nil
		merged = append(merged, item)
	}
	sort.SliceStable(merged, func(i, j int) bool {
		return messagePublishedTime(merged[i].PublishedAt).After(messagePublishedTime(merged[j].PublishedAt))
	})
	return merged
}

// messagePublishedTime parses a published-at timestamp for sorting.
func messagePublishedTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339, time.RFC3339Nano, time.DateOnly} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

// applyMessageReadState sets ReadAt on each message from the read state and
// returns the number of unread messages.
func applyMessageReadState(items []messages.Message, state map[string]time.Time) int {
	unread := 0
	for i := range items {
		if readAt, ok := state[items[i].ID]; ok {
			t := readAt
			items[i].ReadAt = &t
			continue
		}
		unread++
	}
	return unread
}

func (a *App) automationNotificationMessages(locale string) ([]messages.Message, error) {
	tasks, err := a.Automations()
	if err != nil {
		return nil, fmt.Errorf("list automations for messages: %w", err)
	}
	inbox, err := a.AutomationInbox()
	if err != nil {
		return nil, fmt.Errorf("list automation inbox for messages: %w", err)
	}
	return automationMessagesForLocale(tasks, inbox, locale), nil
}

func automationMessagesForLocale(tasks []automation.Task, inbox []automation.TriggerInboxItem, locale string) []messages.Message {
	items := make([]messages.Message, 0, len(tasks)+len(inbox))
	for _, task := range tasks {
		for _, run := range task.RecentRuns {
			if run.Status == automation.RunStatusRunning || strings.TrimSpace(run.ID) == "" {
				continue
			}
			items = append(items, automationRunMessage(task, run, locale))
		}
	}
	for _, item := range inbox {
		if item.Status != automation.InboxStatusPending || strings.TrimSpace(item.ID) == "" {
			continue
		}
		task := automationTaskForInbox(tasks, item)
		items = append(items, automationInboxMessage(task, item, locale))
	}
	sort.SliceStable(items, func(i, j int) bool {
		return automationMessageTime(items[i]).After(automationMessageTime(items[j]))
	})
	if len(items) > maxAutomationNotificationMessages {
		items = items[:maxAutomationNotificationMessages]
	}
	return items
}

func automationRunMessage(task automation.Task, run automation.RunRecord, locale string) messages.Message {
	publishedAt := run.FinishedAt
	if publishedAt.IsZero() {
		publishedAt = run.StartedAt
	}
	title := strings.TrimSpace(task.Name)
	if title == "" {
		title = strings.TrimSpace(run.TaskID)
	}
	summary := automationRunStatusLabel(run.Status, locale)
	detail := strings.TrimSpace(run.Summary)
	if run.Status == automation.RunStatusFailed && strings.TrimSpace(run.Error) != "" {
		detail = strings.TrimSpace(run.Error)
	}
	body := automationRunMessageBody(title, summary, detail, locale)
	return messages.Message{
		ID:          "automation-run:" + run.ID,
		Type:        messages.MessageTypeAutomation,
		Title:       title,
		Summary:     summary,
		Body:        body,
		PublishedAt: formatAutomationMessageTime(publishedAt),
		TaskID:      automationCatalogID(task),
		RunID:       run.ID,
		Workspace:   firstNonEmpty(run.Workspace, task.Target.Workspace),
		Status:      run.Status,
	}
}

func automationInboxMessage(task automation.Task, item automation.TriggerInboxItem, locale string) messages.Message {
	title := strings.TrimSpace(item.Title)
	if title == "" {
		if isChineseLocale(locale) {
			title = "自动化任务需要确认"
		} else {
			title = "Automation needs confirmation"
		}
	}
	taskName := strings.TrimSpace(task.Name)
	if taskName == "" {
		taskName = strings.TrimSpace(item.TaskID)
	}
	detail := boundedAutomationMessageText(item.Summary, 4000)
	body := "任务：" + taskName + "\n\n需要你的确认后才能继续。"
	if !isChineseLocale(locale) {
		body = "Task: " + taskName + "\n\nYour confirmation is required before it can continue."
	}
	if detail != "" {
		body += "\n\n" + detail
	}
	return messages.Message{
		ID:             "automation-inbox:" + item.ID,
		Type:           messages.MessageTypeAutomationAction,
		Title:          title,
		Summary:        firstNonEmpty(boundedAutomationMessageText(item.Summary, 240), automationActionLabel(locale)),
		Body:           body,
		PublishedAt:    formatAutomationMessageTime(item.CreatedAt),
		TaskID:         automationCatalogID(task),
		RunID:          item.RunID,
		InboxID:        item.ID,
		Workspace:      firstNonEmpty(item.Workspace, task.Target.Workspace),
		Status:         item.Status,
		ActionRequired: true,
	}
}

func automationTaskForInbox(tasks []automation.Task, item automation.TriggerInboxItem) automation.Task {
	workspace := canonicalAutomationWorkspace(item.Workspace)
	for _, task := range tasks {
		if task.ID != item.TaskID {
			continue
		}
		if canonicalAutomationWorkspace(task.Target.Workspace) == workspace {
			return task
		}
	}
	return automation.Task{ID: item.TaskID, CatalogID: item.TaskID, Name: item.TaskID}
}

func automationCatalogID(task automation.Task) string {
	if id := strings.TrimSpace(task.CatalogID); id != "" {
		return id
	}
	return strings.TrimSpace(task.ID)
}

func automationRunStatusLabel(status, locale string) string {
	zh := isChineseLocale(locale)
	switch status {
	case automation.RunStatusSuccess:
		if zh {
			return "自动化任务已完成"
		}
		return "Automation completed"
	case automation.RunStatusFailed:
		if zh {
			return "自动化任务执行失败"
		}
		return "Automation failed"
	case automation.RunStatusAborted:
		if zh {
			return "自动化任务已中止"
		}
		return "Automation aborted"
	default:
		if zh {
			return "自动化任务状态已更新"
		}
		return "Automation status updated"
	}
}

func automationActionLabel(locale string) string {
	if isChineseLocale(locale) {
		return "需要你的确认"
	}
	return "Your confirmation is required"
}

func automationRunMessageBody(taskName, status, detail, locale string) string {
	body := fmt.Sprintf("任务\"%s\"%s。", taskName, strings.TrimPrefix(status, "自动化任务"))
	if !isChineseLocale(locale) {
		body = fmt.Sprintf("Task \"%s\": %s.", taskName, strings.TrimSuffix(status, "."))
	}
	if detail != "" {
		body += "\n\n" + boundedAutomationMessageText(detail, 4000)
	}
	return body
}

func boundedAutomationMessageText(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return strings.TrimSpace(string(runes[:limit])) + "…"
}

func automationMessageTime(item messages.Message) time.Time {
	parsed, _ := time.Parse(time.RFC3339, item.PublishedAt)
	return parsed
}

func formatAutomationMessageTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func isChineseLocale(locale string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "zh")
}

func (a *App) novaDir() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.cfg == nil {
		return ""
	}
	return a.cfg.DataDir()
}
