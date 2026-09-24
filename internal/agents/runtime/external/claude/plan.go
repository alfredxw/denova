package claude

import (
	"encoding/json"
	"slices"

	"denova/internal/agents/runtime/external"
	agent "github.com/alfredxw/denova/agent"
)

// observePlan translates successful native task observations only. Validation,
// dependencies and mutations remain owned by the CLI, including partial errors.
func (s *streamOutput) observePlan(host external.Host, call contentBlock, raw json.RawMessage) error {
	var input struct {
		TaskID  string `json:"taskId"`
		Subject string `json:"subject"`
		Status  string `json:"status"`
	}
	if err := json.Unmarshal(call.Input, &input); err != nil {
		return err
	}
	type task struct {
		ID      string           `json:"id"`
		Subject string           `json:"subject"`
		Status  agent.TodoStatus `json:"status"`
	}
	var result struct {
		Task          *task    `json:"task"`
		Tasks         []task   `json:"tasks"`
		Success       bool     `json:"success"`
		UpdatedFields []string `json:"updatedFields"`
	}
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return err
	}
	items := append([]agent.TodoItem(nil), s.plan...)
	switch call.Name {
	case "TaskCreate":
		if result.Task == nil || result.Task.ID == "" {
			return nil
		}
		item := agent.TodoItem{ID: result.Task.ID, Text: result.Task.Subject, Status: agent.TodoPending}
		index := slices.IndexFunc(items, func(existing agent.TodoItem) bool { return existing.ID == item.ID })
		if index >= 0 {
			items[index] = item
		} else {
			items = append(items, item)
		}
	case "TaskUpdate":
		if !result.Success {
			return nil
		}
		index := slices.IndexFunc(items, func(item agent.TodoItem) bool { return item.ID == input.TaskID })
		if index < 0 {
			return nil
		}
		if slices.Contains(result.UpdatedFields, "deleted") {
			items = slices.Delete(items, index, index+1)
		} else {
			if slices.Contains(result.UpdatedFields, "subject") {
				items[index].Text = input.Subject
			}
			if slices.Contains(result.UpdatedFields, "status") {
				items[index].Status = agent.TodoStatus(input.Status)
			}
		}
	case "TaskList":
		if result.Tasks == nil {
			return nil
		}
		items = make([]agent.TodoItem, 0, len(result.Tasks))
		for _, task := range result.Tasks {
			items = append(items, agent.TodoItem{ID: task.ID, Text: task.Subject, Status: task.Status})
		}
	case "TaskGet":
		if result.Task == nil {
			return nil
		}
		item := agent.TodoItem{ID: result.Task.ID, Text: result.Task.Subject, Status: result.Task.Status}
		index := slices.IndexFunc(items, func(existing agent.TodoItem) bool { return existing.ID == item.ID })
		if index >= 0 {
			items[index] = item
		} else {
			items = append(items, item)
		}
	}
	if s.planObserved && slices.Equal(items, s.plan) {
		return nil
	}
	event := external.PlanEvent(items)
	if _, err := external.PlanItems(event); err != nil {
		return err
	}
	if err := host.Emit(event); err != nil {
		return err
	}
	s.plan = items
	s.planObserved = true
	return nil
}
