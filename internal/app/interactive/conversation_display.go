package interactiveapp

import (
	agentinteractive "denova/internal/agents/interactive"
	"fmt"
	"strings"
	"time"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/agents/session"
	"denova/internal/interactive"
)

func (c *Conversation) AppendDisplayEvent(event session.DisplayEvent) error {
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
		ToolPresentation:  cloneDisplayToolPresentation(event.ToolPresentation),
		CreatedAt:         createdAt,
		AgentKind:         event.AgentKind,
		RunID:             event.RunID,
		AgentName:         event.AgentName,
		RootAgentName:     event.RootAgentName,
		RunPath:           append([]string(nil), event.RunPath...),
		SubAgent:          event.SubAgent,
		SubAgentSessionID: event.SubAgentSessionID,
		SubAgentType:      event.SubAgentType,
	}
	replacesRunningEvent := status == "running" && findInteractiveDisplayEventIndex(c.displayEvents, next.ID, next.Role) >= 0
	c.displayEvents = appendOrReplaceDisplayEvent(c.displayEvents, next)
	// Streaming progress can replace the same display card hundreds of times.
	// Keep those transient snapshots live in memory and persist the stable
	// terminal replacement instead of rewriting the whole story journal for
	// every progress tick.
	if replacesRunningEvent {
		return nil
	}
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

func cloneDisplayToolPresentation(presentation *agent.ToolPresentation) *agent.ToolPresentation {
	if presentation == nil {
		return nil
	}
	cloned := *presentation
	return &cloned
}

func (c *Conversation) AppendDisplayToolArgs(id, name, delta string) error {
	if c == nil || delta == "" {
		return nil
	}
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	c.mu.Lock()
	defer c.mu.Unlock()
	if index := findInteractiveDisplayToolEventIndex(c.displayEvents, id, name); index >= 0 {
		c.displayEvents[index].Args += delta
	}
	return nil
}

func (c *Conversation) AppendDisplayEventContent(id, role, delta string) error {
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
	}
	return nil
}

// FlushDisplayEventContent persists the final streamed display tail at a part
// boundary. The live event remains available from DisplayEventsSnapshot while
// streaming, without forcing one full story-journal append per token.
func (c *Conversation) FlushDisplayEventContent(id, role string) error {
	if c == nil {
		return nil
	}
	id = strings.TrimSpace(id)
	role = strings.TrimSpace(role)
	if id == "" || role == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	index := findInteractiveDisplayEventIndex(c.displayEvents, id, role)
	if index < 0 {
		return nil
	}
	if c.lastTurn != nil {
		persistedIndex := findInteractiveDisplayEventIndex(c.lastTurn.DisplayEvents, id, role)
		if persistedIndex >= 0 && c.lastTurn.DisplayEvents[persistedIndex].Content == c.displayEvents[index].Content {
			return nil
		}
	}
	return c.persistLastTurnDisplayEventLocked(c.displayEvents[index])
}

func (c *Conversation) appendTokenUsageEvent(event session.DisplayEvent) error {
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

func (c *Conversation) UpdateDisplayToolStatus(id, name, status string) error {
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

func (c *Conversation) UpdateDisplayToolResult(id, name, status, result string, presentation *agent.ToolPresentation) error {
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
		if normalized := cloneDisplayToolPresentation(presentation); normalized != nil {
			c.displayEvents[index].ToolPresentation = normalized
		}
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

func (c *Conversation) persistLastTurnDisplayEventLocked(event interactive.DisplayEvent) error {
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

func (c *Conversation) DisplayEventsSnapshot() []interactive.DisplayEvent {
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
		if event.Role == "tool_call" && agentinteractive.IsInteractiveTurnSubmissionTool(event.Name) {
			result := make([]interactive.DisplayEvent, 0, len(events)+1)
			result = append(result, events[:index]...)
			result = append(result, anchor)
			return append(result, events[index:]...)
		}
	}
	return events
}
