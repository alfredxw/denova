package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"

	"denova/config"
	"denova/internal/agent"
	agentcontext "denova/internal/agent/context"
	"denova/internal/book"
	"denova/internal/interactive"
	"denova/internal/prompts"
	"denova/internal/session"
	"denova/internal/workspacepath"
)

type interactiveConversation struct {
	store                *interactive.Store
	novaDir              string
	workspace            string
	cfg                  *config.Config
	storyID              string
	branchID             string
	user                 string
	replyTargetChars     int
	mu                   sync.Mutex
	lastTurn             *interactive.TurnEvent
	lastStateReady       bool
	lastSources          string
	displayEvents        []interactive.DisplayEvent
	modelContextMessages []interactive.ModelContextMessage
	ruleResolution       *interactive.RuleResolution
}

func newInteractiveConversation(store *interactive.Store, novaDir, workspace, storyID, branchID, user string, replyTargetChars int, cfg *config.Config) *interactiveConversation {
	return &interactiveConversation{store: store, novaDir: novaDir, workspace: workspace, cfg: cfg, storyID: storyID, branchID: branchID, user: user, replyTargetChars: replyTargetChars}
}

func (c *interactiveConversation) PrepareMessages(originalMessage, agentMessage string) ([]*schema.Message, error) {
	_ = originalMessage
	if c == nil || c.store == nil {
		return nil, fmt.Errorf("互动故事不存在")
	}
	storyCtx, err := c.store.StoryContext(c.storyID, c.branchID)
	if err != nil {
		return nil, err
	}
	teller := c.teller(storyCtx.Meta.StoryTellerID)
	storyDirector := c.storyDirector(storyCtx.Meta.StoryDirectorID)
	tellerTurnContextPrompt := teller.PromptForTargets("turn_context")
	turnMemory := buildInteractiveModelVisibleTurnMemory(storyCtx.Snapshot.Turns, storyCtx.Snapshot.ContextCompaction)
	storyMemory, err := c.store.StoryMemoryContextSummary(c.storyID, storyCtx.Snapshot.BranchID, 12*1024)
	if err != nil {
		log.Printf("[interactive-agent] load story memory failed story_id=%s branch_id=%s err=%v", c.storyID, storyCtx.Snapshot.BranchID, err)
		storyMemory = ""
	}
	directorPlanVisible := ""
	if storyCtx.Snapshot.DirectorPlan != nil {
		directorPlanVisible = interactive.DirectorPlanVisibleContext(*storyCtx.Snapshot.DirectorPlan, 12*1024)
	}
	ruleSummary := interactive.StoryDirectorRuleSummary(storyDirector, 8*1024)
	strategyPrompt := interactive.StoryDirectorStrategyPromptMarkdown(storyDirector)
	runtimeContext := prompts.InteractiveStoryRuntimeContext(prompts.InteractiveStoryPromptInput{
		Title:                       storyCtx.Meta.Title,
		Origin:                      storyCtx.Meta.Origin,
		StoryTellerID:               storyCtx.Meta.StoryTellerID,
		StoryDirectorID:             storyCtx.Meta.StoryDirectorID,
		BranchID:                    storyCtx.Snapshot.BranchID,
		ReplyTargetChars:            c.replyTargetChars,
		LongTermMemory:              storyMemory,
		DirectorPlanVisible:         directorPlanVisible,
		StoryDirectorRules:          ruleSummary,
		StoryDirectorStrategyPrompt: strategyPrompt,
		PreviousTurnsSummary:        turnMemory.PreviousSummary,
	})
	history := make([]*schema.Message, 0, len(turnMemory.Turns)*2+3)
	if storyCtx.Snapshot.ContextCompaction != nil && strings.TrimSpace(storyCtx.Snapshot.ContextCompaction.Summary) != "" {
		history = append(history, agent.NewContextCompactionSummaryMessage(storyCtx.Snapshot.ContextCompaction.Epoch, storyCtx.Snapshot.ContextCompaction.Summary))
	}
	for _, turn := range turnMemory.Turns {
		history = append(history, schema.UserMessage(turn.User))
		history = append(history, schemaMessagesFromInteractiveContext(turn.ModelContextMessages)...)
		history = append(history, schema.AssistantMessage(turn.Narrative, nil))
	}
	history = agent.ApplyToolResultContextPolicyForConversation(history, c.ToolResultContextPolicy())
	history = append(history, schema.UserMessage(prompts.InteractiveStoryTurnInstruction(agentMessage, tellerTurnContextPrompt, storyDirector.Strategy.RandomEventRate, runtimeContext)))
	sourceSummary := interactiveStorySourceSummary(storyCtx.Meta.Title, storyCtx.Meta.Origin, teller, storyMemory, directorPlanVisible, ruleSummary, strategyPrompt, turnMemory, agentMessage)
	c.mu.Lock()
	c.lastSources = sourceSummary
	c.mu.Unlock()
	log.Printf(
		"[interactive-agent] context composition story_id=%s branch_id=%s story_title=%s origin=%s teller_id=%s story_director_id=%s teller_slots=%s teller_turn_context=%s random_event_rate=%.2f story_memory=%s director_plan=%s turns=%d model_turns=%d compressed_turns=%s history=%s turn_instruction=%s sources=%s",
		c.storyID,
		storyCtx.Snapshot.BranchID,
		interactivePartSummary(storyCtx.Meta.Title),
		interactivePartSummary(storyCtx.Meta.Origin),
		storyCtx.Meta.StoryTellerID,
		storyCtx.Meta.StoryDirectorID,
		interactiveTellerSlotSummary(teller, "turn_context"),
		interactivePartSummary(tellerTurnContextPrompt),
		storyDirector.Strategy.RandomEventRate,
		interactivePartSummary(storyMemory),
		interactivePartSummary(directorPlanVisible),
		len(storyCtx.Snapshot.Turns),
		len(turnMemory.Turns),
		interactivePartSummary(turnMemory.PreviousSummary),
		interactiveMessageListSummary(history),
		interactivePartSummary(history[len(history)-1].Content),
		sourceSummary,
	)
	return history, nil
}

func (c *interactiveConversation) ContextSourceSummary() string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastSources
}

func (c *interactiveConversation) PrepareInteractiveTurn(ctx context.Context, request interactive.TurnCheckRequest) (interactive.RuleResolution, error) {
	if c == nil || c.store == nil {
		return interactive.RuleResolution{}, fmt.Errorf("互动故事不存在")
	}
	storyCtx, err := c.store.StoryContext(c.storyID, c.branchID)
	if err != nil {
		return interactive.RuleResolution{}, err
	}
	select {
	case <-ctx.Done():
		return interactive.RuleResolution{}, ctx.Err()
	default:
	}
	resolution, err := interactive.ResolveTurnRules(c.storyID, storyCtx.Snapshot.BranchID, storyCtx.Snapshot.State, request)
	if err != nil {
		return interactive.RuleResolution{}, err
	}
	c.mu.Lock()
	c.ruleResolution = &resolution
	c.mu.Unlock()
	return resolution, nil
}

func (c *interactiveConversation) CompactContextIfNeeded(ctx context.Context, input agent.ContextCompactionInput) ([]*schema.Message, agent.ContextCompactionResult, error) {
	if c == nil || c.store == nil {
		return input.Messages, agent.ContextCompactionResult{}, fmt.Errorf("互动故事不存在")
	}
	storyCtx, err := c.store.StoryContext(c.storyID, c.branchID)
	if err != nil {
		return input.Messages, agent.ContextCompactionResult{}, err
	}
	if !input.Force && storyCtx.Snapshot.ContextCompactionRemoval != nil && storyCtx.Snapshot.ContextCompactionRemoval.SourceTurnCount >= len(storyCtx.Snapshot.Turns) {
		return input.Messages, agent.ContextCompactionResult{SkippedReason: "removed_same_source"}, nil
	}
	source, existingMemory := interactiveCompactionSource(storyCtx.Snapshot.Turns, storyCtx.Snapshot.ContextCompaction)
	source = agent.ApplyToolResultContextPolicyForConversation(source, c.ToolResultContextPolicy())
	epoch := 1
	if storyCtx.Snapshot.ContextCompaction != nil {
		epoch = storyCtx.Snapshot.ContextCompaction.Epoch + 1
	}
	input.SourceMessages = source
	if strings.TrimSpace(input.ExistingMemory) == "" {
		input.ExistingMemory = existingMemory
	}
	if strings.TrimSpace(input.ReferenceContext) == "" {
		input.ReferenceContext = interactiveCompactionReferenceContext(c.store, c.storyID, storyCtx.Snapshot.BranchID)
	}
	input.KeepLatestUser = true
	newMessages, result, err := agent.BuildContextCompaction(ctx, c.cfg, config.AgentKindInteractiveStory, input, epoch)
	if err != nil || !result.Triggered {
		return newMessages, result, err
	}
	event := interactive.ContextCompactionEvent{
		AgentKind:           config.AgentKindInteractiveStory,
		Epoch:               result.Epoch,
		Summary:             result.Summary,
		SourceTurnCount:     len(storyCtx.Snapshot.Turns),
		RetainedTurns:       result.RetainedTurns,
		TokensBefore:        result.TokensBefore,
		TokensAfter:         result.TokensAfter,
		TargetRatio:         result.TargetRatio,
		ContextWindowTokens: result.ContextWindowTokens,
		Strategy:            result.Strategy,
		Threshold:           result.Threshold,
		Reason:              "context_usage_threshold",
		Phase:               result.Phase,
	}
	event, err = c.store.AppendContextCompaction(c.storyID, storyCtx.Snapshot.BranchID, event)
	if err != nil {
		return input.Messages, result, err
	}
	if event.Epoch != result.Epoch {
		result.Epoch = event.Epoch
		newMessages = agent.BuildCompactedModelMessages(input.Messages, result.Summary, event.Epoch, result.RetainedTurns)
		result.TokensAfter = agent.EstimateContextTokens(newMessages, input.Tools)
		result.MessageCountAfter = len(newMessages)
	}
	return newMessages, result, nil
}

func interactiveTurnMessages(turns []interactive.TurnEvent) []*schema.Message {
	messages := make([]*schema.Message, 0, len(turns)*2)
	for _, turn := range turns {
		if strings.TrimSpace(turn.User) != "" {
			messages = append(messages, schema.UserMessage(turn.User))
		}
		messages = append(messages, schemaMessagesFromInteractiveContext(turn.ModelContextMessages)...)
		if strings.TrimSpace(turn.Narrative) != "" {
			messages = append(messages, schema.AssistantMessage(turn.Narrative, nil))
		}
	}
	return messages
}

func interactiveContextMessageFromSchema(msg *schema.Message) (interactive.ModelContextMessage, bool) {
	if msg == nil {
		return interactive.ModelContextMessage{}, false
	}
	switch msg.Role {
	case schema.Assistant:
		calls := interactiveToolCallsFromSchema(msg.ToolCalls)
		if len(calls) == 0 {
			return interactive.ModelContextMessage{}, false
		}
		return interactive.ModelContextMessage{Role: string(schema.Assistant), ToolCalls: calls}, true
	case schema.Tool:
		if strings.TrimSpace(msg.ToolCallID) == "" && strings.TrimSpace(msg.ToolName) == "" {
			return interactive.ModelContextMessage{}, false
		}
		return interactive.ModelContextMessage{
			Role:       string(schema.Tool),
			Content:    msg.Content,
			Name:       msg.Name,
			ToolCallID: msg.ToolCallID,
			ToolName:   msg.ToolName,
		}, true
	default:
		return interactive.ModelContextMessage{}, false
	}
}

func interactiveToolCallsFromSchema(calls []schema.ToolCall) []interactive.ModelContextToolCall {
	if len(calls) == 0 {
		return nil
	}
	result := make([]interactive.ModelContextToolCall, 0, len(calls))
	for _, call := range calls {
		if strings.TrimSpace(call.Function.Name) == "" {
			continue
		}
		result = append(result, interactive.ModelContextToolCall{
			Index: call.Index,
			ID:    call.ID,
			Type:  call.Type,
			Function: interactive.ModelContextFunctionCall{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
			Extra: call.Extra,
		})
	}
	return result
}

func schemaMessagesFromInteractiveContext(messages []interactive.ModelContextMessage) []*schema.Message {
	if len(messages) == 0 {
		return nil
	}
	result := make([]*schema.Message, 0, len(messages))
	for _, msg := range messages {
		switch strings.TrimSpace(msg.Role) {
		case string(schema.Assistant):
			calls := schemaToolCallsFromInteractive(msg.ToolCalls)
			if len(calls) > 0 {
				result = append(result, schema.AssistantMessage("", calls))
			}
		case string(schema.Tool):
			if strings.TrimSpace(msg.ToolCallID) != "" || strings.TrimSpace(msg.ToolName) != "" {
				result = append(result, schema.ToolMessage(msg.Content, msg.ToolCallID, schema.WithToolName(msg.ToolName)))
			}
		}
	}
	return result
}

func schemaToolCallsFromInteractive(calls []interactive.ModelContextToolCall) []schema.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	result := make([]schema.ToolCall, 0, len(calls))
	for _, call := range calls {
		if strings.TrimSpace(call.Function.Name) == "" {
			continue
		}
		result = append(result, schema.ToolCall{
			Index: call.Index,
			ID:    call.ID,
			Type:  call.Type,
			Function: schema.FunctionCall{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
			Extra: call.Extra,
		})
	}
	return result
}

func interactiveCompactionSource(turns []interactive.TurnEvent, compaction *interactive.ContextCompactionEvent) ([]*schema.Message, string) {
	sourceStart := 0
	existingMemory := ""
	if compaction != nil && strings.TrimSpace(compaction.Summary) != "" {
		existingMemory = compaction.Summary
		sourceStart = compaction.SourceTurnCount
		if sourceStart < 0 {
			sourceStart = 0
		}
		if sourceStart > len(turns) {
			sourceStart = len(turns)
		}
	}
	return interactiveTurnMessages(turns[sourceStart:]), existingMemory
}

func interactiveCompactionReferenceContext(store *interactive.Store, storyID, branchID string) string {
	if store == nil {
		return ""
	}
	storyMemory, err := store.StoryMemoryCompactionContext(storyID, branchID)
	if err != nil {
		log.Printf("[interactive-agent] load story memory for compaction failed story_id=%s branch_id=%s err=%v", storyID, branchID, err)
		return ""
	}
	storyMemory = strings.TrimSpace(storyMemory)
	if storyMemory == "" {
		return ""
	}
	return "Story Memory reference for context compaction. Treat plot_summary / 剧情纪要 records as highest-priority continuity evidence.\n\n" + storyMemory
}

func (c *interactiveConversation) AppendAssistant(content string) error {
	return c.AppendAssistantWithThinking(content, "")
}

func (c *interactiveConversation) AppendContextMessage(msg *schema.Message) error {
	if c == nil || msg == nil {
		return nil
	}
	converted, ok := interactiveContextMessageFromSchema(msg)
	if !ok {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.modelContextMessages = append(c.modelContextMessages, converted)
	return nil
}

func (c *interactiveConversation) ToolResultContextPolicy() agent.ToolResultContextPolicy {
	return agent.ResolveToolResultContextPolicyForConversation(c.cfg, config.AgentKindInteractiveStory)
}

func (c *interactiveConversation) AppendAssistantWithThinking(content, thinking string) error {
	if c == nil || c.store == nil {
		return fmt.Errorf("互动故事不存在")
	}
	log.Printf("[interactive-agent] parse assistant output content story_id=%s branch_id=%s content=%q", c.storyID, c.branchID, content)
	narrative, parseErr := parseInteractiveAssistantOutput(content)
	if parseErr != nil {
		log.Printf("[interactive-agent] parse assistant output failed story_id=%s branch_id=%s err=%v content=%q", c.storyID, c.branchID, parseErr, content)
		return parseErr
	}
	log.Printf("[interactive-agent] parse assistant output result story_id=%s branch_id=%s narrative=%q", c.storyID, c.branchID, narrative)
	turn, _, err := c.store.AppendTurnWithState(c.storyID, interactive.AppendTurnWithStateRequest{
		BranchID:             c.branchID,
		User:                 c.user,
		Narrative:            narrative,
		Thinking:             thinking,
		DisplayEvents:        c.displayEventsSnapshot(),
		ModelContextMessages: c.modelContextMessagesSnapshot(),
		RuleResolution:       c.ruleResolutionSnapshot(),
		TerminalOutcome:      c.terminalOutcomeSnapshot(narrative),
	})
	if err == nil {
		c.mu.Lock()
		c.lastTurn = &turn
		c.lastStateReady = false
		c.mu.Unlock()
	}
	return err
}

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
		RunID:             event.RunID,
		AgentName:         event.AgentName,
		RootAgentName:     event.RootAgentName,
		RunPath:           append([]string(nil), event.RunPath...),
		SubAgent:          event.SubAgent,
		SubAgentSessionID: event.SubAgentSessionID,
		SubAgentType:      event.SubAgentType,
	}
	c.displayEvents = append(c.displayEvents, next)
	turnID := ""
	branchID := c.branchID
	if c.lastTurn != nil {
		turnID = c.lastTurn.ID
		branchID = c.lastTurn.BranchID
		c.lastTurn.DisplayEvents = append(c.lastTurn.DisplayEvents, next)
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

func (c *interactiveConversation) modelContextMessagesSnapshot() []interactive.ModelContextMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.modelContextMessages) == 0 {
		return nil
	}
	result := make([]interactive.ModelContextMessage, len(c.modelContextMessages))
	copy(result, c.modelContextMessages)
	return result
}

func (c *interactiveConversation) ruleResolutionSnapshot() *interactive.RuleResolution {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ruleResolution == nil {
		return nil
	}
	resolution := *c.ruleResolution
	return &resolution
}

func (c *interactiveConversation) terminalOutcomeSnapshot(narrative string) *interactive.TerminalOutcome {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ruleResolution == nil || c.ruleResolution.TerminalCandidate == nil {
		return nil
	}
	candidate := c.ruleResolution.TerminalCandidate
	return &interactive.TerminalOutcome{
		Terminal:              true,
		Type:                  candidate.Type,
		Reason:                candidate.Reason,
		FinalNarrativeSummary: strings.TrimSpace(narrative),
		RuleResolutionID:      c.ruleResolution.ID,
	}
}

func (c *interactiveConversation) LastTurnForState() (interactive.TurnEvent, bool, bool) {
	if c == nil {
		return interactive.TurnEvent{}, false, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastTurn == nil {
		return interactive.TurnEvent{}, false, false
	}
	return *c.lastTurn, c.lastStateReady, true
}

func (c *interactiveConversation) BuildStateInstruction(turn interactive.TurnEvent) (string, error) {
	if c == nil || c.store == nil {
		return "", fmt.Errorf("互动故事不存在")
	}
	storyCtx, err := c.store.StoryContext(c.storyID, c.branchID)
	if err != nil {
		return "", err
	}
	storyMemory, err := c.store.StoryMemoryContextSummary(c.storyID, storyCtx.Snapshot.BranchID, 12*1024)
	if err != nil {
		log.Printf("[interactive-state-agent] load story memory failed story_id=%s branch_id=%s err=%v", c.storyID, storyCtx.Snapshot.BranchID, err)
		storyMemory = ""
	}
	storyMemorySchema, err := c.store.StoryMemorySchemaContext(c.storyID, interactiveStateMemorySchemaBytes)
	if err != nil {
		log.Printf("[interactive-state-agent] load story memory schema failed story_id=%s branch_id=%s err=%v", c.storyID, storyCtx.Snapshot.BranchID, err)
		storyMemorySchema = ""
	}
	teller := c.teller(storyCtx.Meta.StoryTellerID)
	loreContext := c.stateLoreContext()
	turnMemory := buildInteractiveModelVisibleTurnMemory(storyCtx.Snapshot.Turns, storyCtx.Snapshot.ContextCompaction)
	turnHistory := formatInteractiveTurnMemoryHistory(turnMemory, storyCtx.Snapshot.ContextCompaction, "（暂无历史回合，请基于本回合行动、正文、资料库和既有故事记忆填表。）")
	instruction := prompts.InteractiveStateInstruction(prompts.InteractiveStatePromptInput{
		Title:             storyCtx.Meta.Title,
		Origin:            storyCtx.Meta.Origin,
		StoryTellerID:     storyCtx.Meta.StoryTellerID,
		StoryTellerMemory: teller.PromptForTargets("state_memory"),
		BranchID:          storyCtx.Snapshot.BranchID,
		LoreItems:         loreContext,
		StoryMemorySchema: storyMemorySchema,
		StoryMemory:       storyMemory,
		TurnHistory:       turnHistory,
		UserAction:        turn.User,
		Narrative:         turn.Narrative,
	})
	log.Printf(
		"[interactive-state-agent] context composition story_id=%s branch_id=%s turn_id=%s teller_id=%s teller_slots=%s sources=%s instruction=%s",
		c.storyID,
		storyCtx.Snapshot.BranchID,
		turn.ID,
		storyCtx.Meta.StoryTellerID,
		interactiveTellerSlotSummary(teller, "state_memory"),
		interactiveStateSourceSummary(c.workspace, storyCtx.Meta.Title, storyCtx.Meta.Origin, teller, loreContext, storyMemorySchema, storyMemory, turnHistory, turn.User, turn.Narrative),
		interactivePartSummary(instruction),
	)
	return instruction, nil
}

func (c *interactiveConversation) BuildDirectorInstruction(turn interactive.TurnEvent) (string, error) {
	if c == nil || c.store == nil {
		return "", fmt.Errorf("互动故事不存在")
	}
	storyCtx, err := c.store.StoryContext(c.storyID, c.branchID)
	if err != nil {
		return "", err
	}
	storyMemory, err := c.store.StoryMemoryContextSummary(c.storyID, storyCtx.Snapshot.BranchID, 8*1024)
	if err != nil {
		log.Printf("[interactive-director-agent] load story memory failed story_id=%s branch_id=%s err=%v", c.storyID, storyCtx.Snapshot.BranchID, err)
		storyMemory = ""
	}
	storyDirector := c.storyDirector(storyCtx.Meta.StoryDirectorID)
	strategyPrompt := interactive.StoryDirectorStrategyPromptMarkdown(storyDirector)
	loreContext := c.directorLoreContext(turn)
	turnMemory := buildInteractiveModelVisibleTurnMemory(storyCtx.Snapshot.Turns, storyCtx.Snapshot.ContextCompaction)
	turnHistory := formatInteractiveTurnMemoryHistory(turnMemory, storyCtx.Snapshot.ContextCompaction, "（暂无历史回合，请基于本回合审计更新导演计划。）")
	directorPlan := interactive.DirectorPlan{}
	if storyCtx.Snapshot.DirectorPlan != nil {
		directorPlan = *storyCtx.Snapshot.DirectorPlan
	} else if plan, err := c.store.DirectorPlan(c.storyID, storyCtx.Snapshot.BranchID); err == nil {
		directorPlan = plan
	}
	allowedPaths := c.store.DirectorPlanAllowedPaths(c.storyID, storyCtx.Snapshot.BranchID)
	instruction := prompts.InteractiveDirectorInstruction(prompts.InteractiveDirectorPromptInput{
		Title:                       storyCtx.Meta.Title,
		Origin:                      storyCtx.Meta.Origin,
		StoryTellerID:               storyCtx.Meta.StoryTellerID,
		StoryDirectorID:             storyCtx.Meta.StoryDirectorID,
		BranchID:                    storyCtx.Snapshot.BranchID,
		DirectorPlanPaths:           strings.Join(allowedPaths, "\n"),
		DirectorPlanDocs:            boundedJSON(directorPlan.Docs, 32*1024),
		PlanningTemplates:           boundedJSON(storyDirector.Strategy.PlanningTemplates, 32*1024),
		BranchPlanningTurns:         storyDirector.Strategy.BranchPlanningTurns,
		LoreContext:                 loreContext,
		TurnAuditJSON:               boundedJSON(interactiveDirectorTurnAudit(turn), 10*1024),
		TurnHistory:                 boundedText(turnHistory, 12*1024),
		StoryMemorySummary:          boundedText(storyMemory, 8*1024),
		StoryDirectorPlan:           interactive.StoryDirectorPlanningSummary(storyDirector, 16*1024),
		StoryDirectorStrategyPrompt: strategyPrompt,
		DirectorEventCatalog:        boundedJSON(interactiveDirectorEventCatalog(storyDirector), 12*1024),
	})
	log.Printf(
		"[interactive-director-agent] context composition story_id=%s branch_id=%s turn_id=%s teller_id=%s story_director_id=%s director_plan=%s allowed_paths=%d lore=%s turn_audit=%s story_memory=%s history=%s instruction=%s",
		c.storyID,
		storyCtx.Snapshot.BranchID,
		turn.ID,
		storyCtx.Meta.StoryTellerID,
		storyCtx.Meta.StoryDirectorID,
		interactivePartSummary(boundedJSON(directorPlan.Docs, 32*1024)),
		len(allowedPaths),
		interactivePartSummary(loreContext),
		interactivePartSummary(boundedJSON(interactiveDirectorTurnAudit(turn), 10*1024)),
		interactivePartSummary(storyMemory),
		interactivePartSummary(turnHistory),
		interactivePartSummary(instruction),
	)
	return instruction, nil
}

func interactiveDirectorTurnAudit(turn interactive.TurnEvent) map[string]any {
	return map[string]any{
		"turn_id":          turn.ID,
		"branch_id":        turn.BranchID,
		"user_action":      turn.User,
		"narrative":        turn.Narrative,
		"rule_resolution":  turn.RuleResolution,
		"terminal_outcome": turn.TerminalOutcome,
	}
}

func interactiveDirectorEventCatalog(director interactive.StoryDirector) []interactive.DirectorEvent {
	events := interactive.DirectorEventCatalogFromStoryDirector(director)
	if len(events) > 32 {
		return events[:32]
	}
	return events
}

func boundedJSON(value any, limit int) string {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return ""
	}
	return boundedText(string(data), limit)
}

func boundedText(value string, limit int) string {
	trimmed, truncated := trimStringToUTF8Bytes(value, limit)
	if truncated {
		return trimmed + "\n...（已按上下文上限截断）"
	}
	return trimmed
}

func (c *interactiveConversation) teller(tellerID string) interactive.Teller {
	return loadInteractiveTeller(c.novaDir, tellerID)
}

func (c *interactiveConversation) storyDirector(directorID string) interactive.StoryDirector {
	return loadStoryDirector(c.novaDir, directorID)
}

func loadInteractiveTeller(novaDir, tellerID string) interactive.Teller {
	if novaDir == "" {
		return interactive.Teller{}
	}
	teller, err := interactive.NewTellerLibrary(novaDir).Get(tellerID)
	if err == nil {
		return teller
	}
	log.Printf("[interactive-agent] load teller failed id=%s err=%v", tellerID, err)
	fallback, fallbackErr := interactive.NewTellerLibrary(novaDir).Get("classic")
	if fallbackErr != nil {
		log.Printf("[interactive-agent] load fallback teller failed err=%v", fallbackErr)
		return interactive.Teller{}
	}
	return fallback
}

func loadStoryDirector(novaDir, directorID string) interactive.StoryDirector {
	if novaDir == "" {
		return interactive.DefaultStoryDirector()
	}
	director, err := interactive.NewStoryDirectorLibrary(novaDir).Get(directorID)
	if err == nil {
		return director
	}
	log.Printf("[interactive-agent] load story director failed id=%s err=%v", directorID, err)
	fallback, fallbackErr := interactive.NewStoryDirectorLibrary(novaDir).Get(interactive.DefaultStoryDirectorID)
	if fallbackErr != nil {
		log.Printf("[interactive-agent] load fallback story director failed err=%v", fallbackErr)
		return interactive.DefaultStoryDirector()
	}
	return fallback
}

func interactiveStoryTellerSystemInput(teller interactive.Teller, styleRules ...[]agent.StyleRule) prompts.InteractiveStorySystemInstructionInput {
	var rules []agent.StyleRule
	if len(styleRules) > 0 {
		rules = styleRules[0]
	}
	return prompts.InteractiveStorySystemInstructionInput{
		StoryTellerID:           teller.ID,
		StoryTellerName:         teller.Name,
		StoryTellerDescription:  teller.Description,
		StoryTellerSystemPrompt: teller.PromptForTargets("system"),
		StyleRules:              rules,
	}
}

func (c *interactiveConversation) stateLoreContext() string {
	if c.workspace == "" {
		return ""
	}
	context, err := book.NewLoreStore(c.workspace).StoryMemoryContextMarkdown(interactiveStateLoreContextBytes)
	if err != nil {
		log.Printf("[interactive-state-agent] load lore context failed workspace=%s err=%v", c.workspace, err)
		return ""
	}
	return context
}

func (c *interactiveConversation) directorLoreContext(turn interactive.TurnEvent) string {
	if c.workspace == "" {
		return ""
	}
	store := book.NewLoreStore(c.workspace)
	var sb strings.Builder
	index, err := store.LoreIndexMarkdown(book.LoreIndexOptions{
		Limit:    50,
		MaxBytes: interactiveDirectorLoreIndexBytes,
	})
	if err != nil {
		log.Printf("[interactive-director-agent] load lore index failed workspace=%s err=%v", c.workspace, err)
	} else {
		appendDirectorLoreContextSection(&sb, "## 资料库索引（source: lore/items.json, limit: 6144 bytes）", index)
	}
	items, err := store.List()
	if err != nil {
		log.Printf("[interactive-director-agent] load lore items failed workspace=%s err=%v", c.workspace, err)
		return boundedText(sb.String(), interactiveDirectorLoreContextBytes)
	}
	selected := selectDirectorLoreItems(items, turn)
	if len(selected) > 0 {
		var full strings.Builder
		full.WriteString("以下条目优先供导演规划重要角色、势力、规则、地点和当前回合相关设定；不要把未列出的资料库内容当作不存在。\n\n")
		for _, item := range selected {
			full.WriteString(formatDirectorLoreItem(item))
			full.WriteString("\n\n")
		}
		appendDirectorLoreContextSection(&sb, "## 重点资料正文（source: lore/items.json, limit: 8192 bytes）", boundedText(full.String(), interactiveDirectorLoreItemsBytes))
	}
	return boundedText(sb.String(), interactiveDirectorLoreContextBytes)
}

func appendDirectorLoreContextSection(sb *strings.Builder, title, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	if sb.Len() > 0 {
		sb.WriteString("\n\n")
	}
	sb.WriteString(title)
	sb.WriteString("\n\n")
	sb.WriteString(content)
}

func selectDirectorLoreItems(items []book.LoreItem, turn interactive.TurnEvent) []book.LoreItem {
	const maxItems = 12
	selected := make([]book.LoreItem, 0, maxItems)
	seen := make(map[string]bool, maxItems)
	add := func(item book.LoreItem) {
		if len(selected) >= maxItems || strings.TrimSpace(item.ID) == "" || seen[item.ID] {
			return
		}
		seen[item.ID] = true
		selected = append(selected, item)
	}
	for _, item := range items {
		if isDirectorPriorityLoreItem(item) {
			add(item)
		}
	}
	for _, item := range items {
		if loreItemRelevantToDirectorTurn(item, turn) {
			add(item)
		}
	}
	return selected
}

func isDirectorPriorityLoreItem(item book.LoreItem) bool {
	switch item.Type {
	case "character", "faction", "rule", "location":
	default:
		return false
	}
	return item.Importance == "major" || item.Importance == "important" || item.LoadMode == book.LoreLoadModeResident
}

func loreItemRelevantToDirectorTurn(item book.LoreItem, turn interactive.TurnEvent) bool {
	haystack := strings.ToLower(turn.User + "\n" + turn.Narrative)
	if strings.TrimSpace(haystack) == "" {
		return false
	}
	probes := append([]string{item.ID, item.Name}, item.Tags...)
	probes = append(probes, item.Keywords...)
	for _, probe := range probes {
		probe = strings.ToLower(strings.TrimSpace(probe))
		if len([]rune(probe)) < 2 {
			continue
		}
		if strings.Contains(haystack, probe) {
			return true
		}
	}
	return false
}

func formatDirectorLoreItem(item book.LoreItem) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "### %s（%s / %s）\n", strings.TrimSpace(item.Name), directorLoreTypeLabel(item.Type), directorLoreImportanceLabel(item.Importance))
	if strings.TrimSpace(item.ID) != "" {
		fmt.Fprintf(&sb, "ID：%s\n", strings.TrimSpace(item.ID))
	}
	if len(item.Tags) > 0 {
		fmt.Fprintf(&sb, "标签：%s\n", strings.Join(item.Tags, "、"))
	}
	if strings.TrimSpace(item.BriefDescription) != "" {
		fmt.Fprintf(&sb, "简介：%s\n", strings.TrimSpace(item.BriefDescription))
	}
	if content := strings.TrimSpace(item.Content); content != "" {
		sb.WriteString("\n正文摘录：\n")
		sb.WriteString(boundedText(content, interactiveDirectorLoreItemBytes))
	}
	return strings.TrimSpace(sb.String())
}

func directorLoreTypeLabel(value string) string {
	switch value {
	case "character":
		return "角色"
	case "world":
		return "世界观"
	case "location":
		return "地点"
	case "faction":
		return "势力"
	case "rule":
		return "规则"
	case "item":
		return "物品"
	default:
		return "其他"
	}
}

func directorLoreImportanceLabel(value string) string {
	switch value {
	case "major":
		return "核心"
	case "important":
		return "重要"
	case "minor":
		return "次要"
	default:
		return "未标注"
	}
}

func (c *interactiveConversation) MarkInterrupted(userMessage, assistantContent, reason string) error {
	log.Printf("[interactive-agent] interruption ignored story_id=%s branch_id=%s reason=%s", c.storyID, c.branchID, reason)
	return nil
}

func (c *interactiveConversation) PendingInterruption() *session.Interruption {
	return nil
}

func (c *interactiveConversation) ResolveInterruption(id string) error {
	return nil
}

type interactiveContextSource struct {
	Source  string
	Title   string
	Content string
	Note    string
}

type interactiveTurnMemory struct {
	PreviousSummary string
	Turns           []interactive.TurnEvent
	PreviousCount   int
	OmittedCount    int
}

const (
	interactiveStateMemorySchemaBytes   = 32 * 1024
	interactiveStateLoreContextBytes    = 32 * 1024
	interactiveDirectorLoreContextBytes = 16 * 1024
	interactiveDirectorLoreIndexBytes   = 6 * 1024
	interactiveDirectorLoreItemsBytes   = 8 * 1024
	interactiveDirectorLoreItemBytes    = 1200
)

func buildInteractiveTurnMemory(turns []interactive.TurnEvent) interactiveTurnMemory {
	return interactiveTurnMemory{Turns: append([]interactive.TurnEvent(nil), turns...)}
}

func buildInteractiveModelVisibleTurnMemory(turns []interactive.TurnEvent, compaction *interactive.ContextCompactionEvent) interactiveTurnMemory {
	return buildInteractiveTurnMemoryWithCompaction(turns, compaction, retainedTurnsForInteractiveCompaction(compaction))
}

func retainedTurnsForInteractiveCompaction(compaction *interactive.ContextCompactionEvent) int {
	if compaction == nil || strings.TrimSpace(compaction.Summary) == "" {
		return 0
	}
	if compaction.RetainedTurns > 0 {
		return compaction.RetainedTurns
	}
	return config.DefaultContextCompactionRetainedTurns
}

func buildInteractiveTurnMemoryWithCompaction(turns []interactive.TurnEvent, compaction *interactive.ContextCompactionEvent, retainedTurns int) interactiveTurnMemory {
	if compaction == nil || strings.TrimSpace(compaction.Summary) == "" {
		return buildInteractiveTurnMemory(turns)
	}
	if retainedTurns <= 0 {
		retainedTurns = config.DefaultContextCompactionRetainedTurns
	}
	if retainedTurns > config.MaxContextCompactionRetainedTurns {
		retainedTurns = config.MaxContextCompactionRetainedTurns
	}
	sourceCount := compaction.SourceTurnCount
	if sourceCount < 0 {
		sourceCount = 0
	}
	if sourceCount > len(turns) {
		sourceCount = len(turns)
	}
	sourceTail := append([]interactive.TurnEvent(nil), turns[:sourceCount]...)
	if len(sourceTail) > retainedTurns {
		sourceTail = sourceTail[len(sourceTail)-retainedTurns:]
	}
	appended := append([]interactive.TurnEvent(nil), turns[sourceCount:]...)
	retained := make([]interactive.TurnEvent, 0, len(sourceTail)+len(appended))
	retained = append(retained, sourceTail...)
	retained = append(retained, appended...)
	return interactiveTurnMemory{
		PreviousSummary: "",
		Turns:           retained,
		PreviousCount:   sourceCount,
		OmittedCount:    sourceCount,
	}
}

func formatInteractiveTurnHistory(turns []interactive.TurnEvent, emptyMessage string) string {
	if len(turns) == 0 {
		return emptyMessage
	}
	var sb strings.Builder
	for i, turn := range turns {
		idx := i + 1
		fmt.Fprintf(&sb, "第 %d 回合用户行动：%s\n", idx, strings.TrimSpace(turn.User))
		fmt.Fprintf(&sb, "第 %d 回合剧情：%s\n\n", idx, strings.TrimSpace(turn.Narrative))
	}
	return strings.TrimSpace(sb.String())
}

func formatInteractiveTurnMemoryHistory(turnMemory interactiveTurnMemory, compaction *interactive.ContextCompactionEvent, emptyMessage string) string {
	var sb strings.Builder
	if compaction != nil && strings.TrimSpace(compaction.Summary) != "" {
		sb.WriteString("[上下文压缩摘要]\n")
		sb.WriteString(agent.NewContextCompactionSummaryMessage(compaction.Epoch, compaction.Summary).Content)
		sb.WriteString("\n\n")
	}
	if len(turnMemory.Turns) > 0 {
		sb.WriteString(formatInteractiveTurnHistory(turnMemory.Turns, emptyMessage))
	}
	result := strings.TrimSpace(sb.String())
	if result == "" {
		return emptyMessage
	}
	return result
}

func interactiveStorySourceSummary(title, origin string, teller interactive.Teller, storyMemory, directorPlanVisible, ruleSummary, strategyPrompt string, turnMemory interactiveTurnMemory, userAction string) string {
	parts := []interactiveContextSource{
		{Source: "互动故事", Title: "故事标题", Content: title},
		{Source: "互动故事", Title: "开端", Content: origin},
	}
	parts = append(parts, interactiveTellerSlotSources(teller, "turn_context")...)
	if strings.TrimSpace(storyMemory) != "" {
		parts = append(parts, interactiveContextSource{Source: "故事记忆", Title: "当前分支可见故事记忆", Content: storyMemory})
	}
	if strings.TrimSpace(directorPlanVisible) != "" {
		parts = append(parts, interactiveContextSource{Source: "DirectorPlan", Title: "后台导演规划可读区", Content: directorPlanVisible, Note: "limit=12288"})
	}
	if strings.TrimSpace(ruleSummary) != "" {
		parts = append(parts, interactiveContextSource{Source: "StoryDirector", Title: "故事导演规则清单", Content: ruleSummary, Note: "limit=8192"})
	}
	if strings.TrimSpace(strategyPrompt) != "" {
		parts = append(parts, interactiveContextSource{Source: "StoryDirector.strategy.prompt_markdown", Title: "故事导演 Markdown 策略提示", Content: strategyPrompt, Note: "limit=4000"})
	}
	if strings.TrimSpace(turnMemory.PreviousSummary) != "" {
		parts = append(parts, interactiveContextSource{Source: "历史回合", Title: fmt.Sprintf("较早 %d 回合压缩摘要", turnMemory.PreviousCount), Content: turnMemory.PreviousSummary, Note: "compressed"})
	}
	for i, turn := range turnMemory.Turns {
		parts = append(parts,
			interactiveContextSource{Source: "历史回合", Title: fmt.Sprintf("第 %d 回合用户行动", i+1), Content: turn.User},
			interactiveContextSource{Source: "历史回合", Title: fmt.Sprintf("第 %d 回合剧情", i+1), Content: turn.Narrative},
		)
	}
	parts = append(parts, interactiveContextSource{Source: "本轮行动", Title: "当前用户行动", Content: userAction})
	return interactiveContextSourceListSummary(parts)
}

func interactiveStateSourceSummary(workspace, title, origin string, teller interactive.Teller, loreItems, storyMemorySchema, storyMemory, turnHistory, userAction, narrative string) string {
	parts := []interactiveContextSource{
		{Source: "互动故事", Title: "故事标题", Content: title},
		{Source: "互动故事", Title: "开端", Content: origin},
	}
	parts = append(parts, interactiveTellerSlotSources(teller, "state_memory")...)
	if strings.TrimSpace(loreItems) != "" {
		parts = append(parts, interactiveContextSource{Source: "资料库", Title: workspacepath.Rel(workspace, "lore", "items.json"), Content: loreItems})
	}
	parts = append(parts,
		interactiveContextSource{Source: "故事记忆结构", Title: "story memory schema", Content: storyMemorySchema},
		interactiveContextSource{Source: "故事记忆", Title: "当前分支可见故事记忆", Content: storyMemory},
		interactiveContextSource{Source: "历史回合", Title: "完整回合上下文", Content: turnHistory},
		interactiveContextSource{Source: "本轮行动", Title: "用户行动", Content: userAction},
		interactiveContextSource{Source: "本轮剧情", Title: "Agent 正文", Content: narrative},
	)
	return interactiveContextSourceListSummary(parts)
}

func interactiveTellerSlotSources(teller interactive.Teller, targets ...string) []interactiveContextSource {
	allowed := make(map[string]bool, len(targets))
	for _, target := range targets {
		allowed[target] = true
	}
	parts := []interactiveContextSource{}
	for _, slot := range teller.Slots {
		if !slot.Enabled || !allowed[slot.Target] || strings.TrimSpace(slot.Content) == "" {
			continue
		}
		parts = append(parts, interactiveContextSource{
			Source:  "导演注入规则",
			Title:   fmt.Sprintf("%s（%s）", slot.Name, slot.Target),
			Content: slot.Content,
			Note:    "teller=" + teller.ID,
		})
	}
	return parts
}

func interactiveTellerSlotSummary(teller interactive.Teller, targets ...string) string {
	sources := interactiveTellerSlotSources(teller, targets...)
	if len(sources) == 0 {
		return "count=0"
	}
	names := make([]string, 0, len(sources))
	for _, source := range sources {
		names = append(names, source.Title)
	}
	return fmt.Sprintf("count=%d names=%q", len(names), names)
}

func interactiveContextSourceListSummary(parts []interactiveContextSource) string {
	sources := make([]agentcontext.Source, 0, len(parts))
	for _, part := range parts {
		sources = append(sources, agentcontext.Source{
			Source:    part.Source,
			Title:     part.Title,
			Content:   part.Content,
			Placement: agentcontext.PlacementAuditOnly,
			Included:  true,
			Note:      part.Note,
		})
	}
	return agentcontext.SourceSummary(sources, agentcontext.DefaultPreviewChars)
}

func interactiveMessageListSummary(messages []*schema.Message) string {
	if len(messages) == 0 {
		return "count=0"
	}
	parts := make([]string, 0, len(messages))
	for i, msg := range messages {
		parts = append(parts, interactiveMessageSummary(i, len(messages), msg))
	}
	return fmt.Sprintf("count=%d parts=[%s]", len(messages), strings.Join(parts, "; "))
}

func interactiveMessageSummary(index, total int, msg *schema.Message) string {
	if msg == nil {
		return fmt.Sprintf("%d:<nil>", index)
	}
	source := "互动上下文"
	if index > 0 && index < total-1 {
		source = "历史回合"
	}
	if index == total-1 {
		source = "本轮行动指令"
	}
	return fmt.Sprintf("%d:source=%s role=%s(%s)", index, source, msg.Role, interactivePartSummary(msg.Content))
}

func interactivePartSummary(s string) string {
	s = strings.TrimSpace(s)
	return strings.Join([]string{
		"present=" + interactiveBoolString(s != ""),
		"bytes=" + fmt.Sprint(len(s)),
		"chars=" + fmt.Sprint(utf8.RuneCountInString(s)),
		"lines=" + fmt.Sprint(interactiveLineCount(s)),
		"sha=" + interactiveShortSHA256(s),
		"preview=" + strconv.Quote(interactiveSafePreview(s, 80)),
	}, ",")
}

func interactiveSafePreview(content string, limit int) string {
	content = strings.ReplaceAll(content, "\n", "\\n")
	content = strings.ReplaceAll(content, "\r", "\\r")
	if len(content) <= limit {
		return content
	}
	for limit > 0 && !utf8.RuneStart(content[limit]) {
		limit--
	}
	return content[:limit] + "..."
}

func interactiveBoolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func interactiveLineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func interactiveShortSHA256(s string) string {
	if s == "" {
		return "-"
	}
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:12]
}

func interactiveMinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
