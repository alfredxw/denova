package app

import (
	"fmt"
	"strings"
	"time"

	"denova/internal/agent"
	"denova/internal/agent/session"
	"denova/internal/interactive"
)

func (c *interactiveConversation) AppendDisplayEvent(event session.DisplayEvent) error {
	if c == nil {
		return nil
	}
	role := strings.TrimSpace(event.Role)
	if role == "" {
		return fmt.Errorf("展示事件 role 不能为空")
	}
	if role == "token_usage" {
		return c.appendTokenUsageEvent(event)
	}
	if role != "thinking" && role != "tool_call" && role != "tool_result" && !(role == "assistant" && event.SubAgent) {
		return nil
	}
	name := strings.TrimSpace(event.Name)
	content := strings.TrimSpace(event.Content)
	if role == "tool_call" {
		if name == "" {
			name = content
		}
		if name == "" {
			name = "unknown_tool"
		}
		content = name
	}
	status := strings.TrimSpace(event.Status)
	if role == "tool_call" && status == "" {
		status = "running"
	}
	createdAt := ""
	if !event.CreatedAt.IsZero() {
		createdAt = event.CreatedAt.UTC().Format(time.RFC3339Nano)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	next := interactive.DisplayEvent{
		ID:                strings.TrimSpace(event.ID),
		Role:              role,
		Content:           content,
		Name:              name,
		Args:              event.Args,
		Status:            status,
		Result:            event.Result,
		CreatedAt:         createdAt,
		AgentKind:         event.AgentKind,
		RunID:             event.RunID,
		AgentName:         event.AgentName,
		RootAgentName:     event.RootAgentName,
		RunPath:           append([]string(nil), event.RunPath...),
		SubAgent:          event.SubAgent,
		SubAgentSessionID: event.SubAgentSessionID,
		SubAgentType:      event.SubAgentType,
		SSEHiddenFields:   append([]string(nil), event.SSEHiddenFields...),
		SSEHiddenReason:   event.SSEHiddenReason,
		SSEDisplayNotice:  event.SSEDisplayNotice,
		SSEGeneratedChars: event.SSEGeneratedChars,
	}
	c.displayEvents = appendOrReplaceDisplayEvent(c.displayEvents, next)
	turnID := ""
	branchID := c.branchID
	if c.lastTurn != nil {
		turnID = c.lastTurn.ID
		branchID = c.lastTurn.BranchID
		c.lastTurn.DisplayEvents = appendOrReplaceDisplayEvent(c.lastTurn.DisplayEvents, next)
	}
	storyID := c.storyID
	store := c.store
	if turnID == "" || store == nil {
		return nil
	}
	c.mu.Unlock()
	err := store.AppendTurnDisplayEvent(storyID, branchID, turnID, next)
	c.mu.Lock()
	return err
}

func (c *interactiveConversation) AppendDisplayToolArgs(id, name, delta string) error {
	if c == nil || delta == "" {
		return nil
	}
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	c.mu.Lock()
	defer c.mu.Unlock()
	if index := findInteractiveDisplayToolEventIndex(c.displayEvents, id, name); index >= 0 {
		c.displayEvents[index].Args += delta
		return c.persistLastTurnDisplayEventLocked(c.displayEvents[index])
	}
	return nil
}

func (c *interactiveConversation) AppendDisplayEventContent(id, role, delta string) error {
	if c == nil || delta == "" {
		return nil
	}
	id = strings.TrimSpace(id)
	role = strings.TrimSpace(role)
	if id == "" || role == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if index := findInteractiveDisplayEventIndex(c.displayEvents, id, role); index >= 0 {
		c.displayEvents[index].Content += delta
		return c.persistLastTurnDisplayEventLocked(c.displayEvents[index])
	}
	return nil
}

func (c *interactiveConversation) appendTokenUsageEvent(event session.DisplayEvent) error {
	createdAt := ""
	if !event.CreatedAt.IsZero() {
		createdAt = event.CreatedAt.UTC().Format(time.RFC3339Nano)
	}
	c.mu.Lock()
	store := c.store
	storyID := c.storyID
	branchID := c.branchID
	c.mu.Unlock()
	if store == nil {
		return nil
	}
	return store.AppendTokenUsageEvent(storyID, interactive.TokenUsageEvent{
		ID:                   strings.TrimSpace(event.ID),
		BranchID:             branchID,
		CreatedAt:            createdAt,
		RunID:                strings.TrimSpace(event.RunID),
		AgentKind:            strings.TrimSpace(event.AgentKind),
		PromptTokens:         event.PromptTokens,
		CachedPromptTokens:   event.CachedPromptTokens,
		UncachedPromptTokens: event.UncachedPromptTokens,
		CacheHitRate:         event.CacheHitRate,
		CompletionTokens:     event.CompletionTokens,
		ReasoningTokens:      event.ReasoningTokens,
		TotalTokens:          event.TotalTokens,
		ModelCalls:           event.ModelCalls,
		GeneratedBytes:       event.GeneratedBytes,
		UsageCalls:           interactiveTokenUsageCalls(event.UsageCalls),
	})
}

func interactiveTokenUsageCalls(calls []session.TokenUsageCall) []interactive.TokenUsageCall {
	if len(calls) == 0 {
		return nil
	}
	result := make([]interactive.TokenUsageCall, 0, len(calls))
	for _, call := range calls {
		result = append(result, interactive.TokenUsageCall{
			Index:                call.Index,
			CreatedAt:            call.CreatedAt,
			FinishReason:         call.FinishReason,
			RequestedTools:       append([]string(nil), call.RequestedTools...),
			AfterTools:           append([]string(nil), call.AfterTools...),
			PromptTokens:         call.PromptTokens,
			CachedPromptTokens:   call.CachedPromptTokens,
			UncachedPromptTokens: call.UncachedPromptTokens,
			CacheHitRate:         call.CacheHitRate,
			CompletionTokens:     call.CompletionTokens,
			ReasoningTokens:      call.ReasoningTokens,
			TotalTokens:          call.TotalTokens,
		})
	}
	return result
}

func (c *interactiveConversation) UpdateDisplayToolStatus(id, name, status string) error {
	if c == nil {
		return nil
	}
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	status = strings.TrimSpace(status)
	if status == "" {
		status = "success"
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if index := findInteractiveDisplayToolEventIndex(c.displayEvents, id, name); index >= 0 {
		c.displayEvents[index].Status = status
		return c.persistLastTurnDisplayEventLocked(c.displayEvents[index])
	}
	return nil
}

func (c *interactiveConversation) UpdateDisplayToolResult(id, name, status, result string) error {
	if c == nil {
		return nil
	}
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	status = strings.TrimSpace(status)
	if status == "" {
		status = "success"
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if index := findInteractiveDisplayToolEventIndex(c.displayEvents, id, name); index >= 0 {
		c.displayEvents[index].Status = status
		c.displayEvents[index].Result = result
		return c.persistLastTurnDisplayEventLocked(c.displayEvents[index])
	}
	return nil
}

func findInteractiveDisplayToolEventIndex(events []interactive.DisplayEvent, id, name string) int {
	if id != "" {
		for i := len(events) - 1; i >= 0; i-- {
			if events[i].Role == "tool_call" && events[i].ID == id {
				return i
			}
		}
		return -1
	}
	if name != "" {
		match := -1
		for i := len(events) - 1; i >= 0; i-- {
			if events[i].Role == "tool_call" && events[i].Name == name {
				if match >= 0 {
					return -1
				}
				match = i
			}
		}
		return match
	}
	if id == "" && name == "" {
		for i := len(events) - 1; i >= 0; i-- {
			if events[i].Role == "tool_call" {
				return i
			}
		}
	}
	return -1
}

func findInteractiveDisplayEventIndex(events []interactive.DisplayEvent, id, role string) int {
	if id == "" || role == "" {
		return -1
	}
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].ID == id && events[i].Role == role {
			return i
		}
	}
	return -1
}

func (c *interactiveConversation) persistLastTurnDisplayEventLocked(event interactive.DisplayEvent) error {
	turnID := ""
	branchID := c.branchID
	if c.lastTurn != nil {
		turnID = c.lastTurn.ID
		branchID = c.lastTurn.BranchID
		c.lastTurn.DisplayEvents = appendOrReplaceDisplayEvent(c.lastTurn.DisplayEvents, event)
	}
	storyID := c.storyID
	store := c.store
	if turnID == "" || store == nil {
		return nil
	}
	c.mu.Unlock()
	err := store.AppendTurnDisplayEvent(storyID, branchID, turnID, event)
	c.mu.Lock()
	return err
}

func appendOrReplaceDisplayEvent(events []interactive.DisplayEvent, next interactive.DisplayEvent) []interactive.DisplayEvent {
	if strings.TrimSpace(next.ID) == "" {
		return append(events, next)
	}
	key := strings.TrimSpace(next.Role) + ":" + strings.TrimSpace(next.ID)
	for i := range events {
		if strings.TrimSpace(events[i].Role)+":"+strings.TrimSpace(events[i].ID) == key {
			events[i] = next
			return events
		}
	}
	return append(events, next)
}

func (c *interactiveConversation) displayEventsSnapshot() []interactive.DisplayEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.displayEvents) == 0 {
		return nil
	}
	result := make([]interactive.DisplayEvent, len(c.displayEvents))
	copy(result, c.displayEvents)
	return result
}

// interactiveNarrativeAnchorEventID 是正文锚点展示事件的固定 ID，一个回合最多一个锚点。
const interactiveNarrativeAnchorEventID = "narrative-anchor"

// withInteractiveNarrativeAnchor 在持久化的展示时间线中插入正文锚点，标记正文
// 实际流出的位置：正文在 submit_interactive_turn 之前输出完整，因此锚点
// 插在首个提交工具调用事件前。找不到提交工具事件时
// （异常或旧数据）不插入锚点，前端按“正文在最后”的旧布局兜底；已含锚点的
// 事件列表原样返回。
func withInteractiveNarrativeAnchor(events []interactive.DisplayEvent) []interactive.DisplayEvent {
	if len(events) == 0 {
		return events
	}
	for _, event := range events {
		if event.Role == interactive.DisplayEventRoleNarrative {
			return events
		}
	}
	anchor := interactive.DisplayEvent{ID: interactiveNarrativeAnchorEventID, Role: interactive.DisplayEventRoleNarrative}
	for index, event := range events {
		if event.Role == "tool_call" && agent.IsInteractiveTurnSubmissionTool(event.Name) {
			result := make([]interactive.DisplayEvent, 0, len(events)+1)
			result = append(result, events[:index]...)
			result = append(result, anchor)
			return append(result, events[index:]...)
		}
	}
	return events
}
