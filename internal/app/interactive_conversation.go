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

	"nova/config"
	"nova/internal/agent"
	"nova/internal/book"
	"nova/internal/interactive"
	"nova/internal/prompts"
	"nova/internal/session"
)

type interactiveConversation struct {
	store            *interactive.Store
	novaDir          string
	workspace        string
	cfg              *config.Config
	storyID          string
	branchID         string
	user             string
	replyTargetChars int
	mu               sync.Mutex
	lastTurn         *interactive.TurnEvent
	lastStateReady   bool
	lastSources      string
	displayEvents    []interactive.DisplayEvent
}

const interactiveCompactionStoryMemoryLimit = 64 * 1024

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
	tellerTurnContextPrompt := teller.PromptForTargets("turn_context")
	turnMemory := buildInteractiveModelVisibleTurnMemory(storyCtx.Snapshot.Turns, storyCtx.Snapshot.ContextCompaction)
	storyMemory, err := c.store.StoryMemoryContextSummary(c.storyID, storyCtx.Snapshot.BranchID, 12*1024)
	if err != nil {
		log.Printf("[interactive-agent] load story memory failed story_id=%s branch_id=%s err=%v", c.storyID, storyCtx.Snapshot.BranchID, err)
		storyMemory = ""
	}
	runtimeContext := prompts.InteractiveStoryRuntimeContext(prompts.InteractiveStoryPromptInput{
		Title:                storyCtx.Meta.Title,
		Origin:               storyCtx.Meta.Origin,
		StoryTellerID:        storyCtx.Meta.StoryTellerID,
		BranchID:             storyCtx.Snapshot.BranchID,
		ReplyTargetChars:     c.replyTargetChars,
		LongTermMemory:       storyMemory,
		PreviousTurnsSummary: turnMemory.PreviousSummary,
	})
	history := make([]*schema.Message, 0, len(turnMemory.Turns)*2+3)
	if storyCtx.Snapshot.ContextCompaction != nil && strings.TrimSpace(storyCtx.Snapshot.ContextCompaction.Summary) != "" {
		history = append(history, agent.NewContextCompactionSummaryMessage(storyCtx.Snapshot.ContextCompaction.Epoch, storyCtx.Snapshot.ContextCompaction.Summary))
	}
	for _, turn := range turnMemory.Turns {
		history = append(history, schema.UserMessage(turn.User))
		history = append(history, schema.AssistantMessage(turn.Narrative, nil))
	}
	history = append(history, schema.UserMessage(prompts.InteractiveStoryTurnInstruction(agentMessage, tellerTurnContextPrompt, teller.RandomEventRate, runtimeContext)))
	sourceSummary := interactiveStorySourceSummary(storyCtx.Meta.Title, storyCtx.Meta.Origin, teller, storyMemory, turnMemory, agentMessage)
	c.mu.Lock()
	c.lastSources = sourceSummary
	c.mu.Unlock()
	log.Printf(
		"[interactive-agent] context composition story_id=%s branch_id=%s story_title=%s origin=%s teller_id=%s teller_slots=%s teller_turn_context=%s random_event_rate=%.2f story_memory=%s turns=%d model_turns=%d compressed_turns=%s history=%s turn_instruction=%s sources=%s",
		c.storyID,
		storyCtx.Snapshot.BranchID,
		interactivePartSummary(storyCtx.Meta.Title),
		interactivePartSummary(storyCtx.Meta.Origin),
		storyCtx.Meta.StoryTellerID,
		interactiveTellerSlotSummary(teller, "turn_context"),
		interactivePartSummary(tellerTurnContextPrompt),
		teller.RandomEventRate,
		interactivePartSummary(storyMemory),
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
	source := interactiveTurnMessages(storyCtx.Snapshot.Turns)
	epoch := 1
	if storyCtx.Snapshot.ContextCompaction != nil {
		epoch = storyCtx.Snapshot.ContextCompaction.Epoch + 1
	}
	input.SourceMessages = source
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
		if strings.TrimSpace(turn.Narrative) != "" {
			messages = append(messages, schema.AssistantMessage(turn.Narrative, nil))
		}
	}
	return messages
}

func interactiveCompactionReferenceContext(store *interactive.Store, storyID, branchID string) string {
	if store == nil {
		return ""
	}
	storyMemory, err := store.StoryMemoryContextSummary(storyID, branchID, interactiveCompactionStoryMemoryLimit)
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

func (c *interactiveConversation) AppendAssistantWithThinking(content, thinking string) error {
	if c == nil || c.store == nil {
		return fmt.Errorf("互动故事不存在")
	}
	log.Printf("[interactive-agent] parse assistant output content story_id=%s branch_id=%s content=%q", c.storyID, c.branchID, content)
	narrative, ops, _, parseErr := parseInteractiveAssistantOutput(content)
	if parseErr != nil {
		log.Printf("[interactive-agent] parse assistant output failed story_id=%s branch_id=%s err=%v content=%q", c.storyID, c.branchID, parseErr, content)
		return parseErr
	}
	log.Printf("[interactive-agent] parse assistant output result story_id=%s branch_id=%s narrative=%q ops=%s", c.storyID, c.branchID, narrative, interactiveStateOpsLogJSON(ops))
	turn, _, err := c.store.AppendTurnWithState(c.storyID, interactive.AppendTurnWithStateRequest{
		BranchID:      c.branchID,
		User:          c.user,
		Narrative:     narrative,
		Thinking:      thinking,
		DisplayEvents: c.displayEventsSnapshot(),
		Ops:           ops,
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
	if role != "thinking" && role != "tool_call" && role != "tool_result" {
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
		ID:        strings.TrimSpace(event.ID),
		Role:      role,
		Content:   content,
		Name:      name,
		Args:      event.Args,
		Status:    status,
		Result:    event.Result,
		CreatedAt: createdAt,
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
	for i := len(c.displayEvents) - 1; i >= 0; i-- {
		event := c.displayEvents[i]
		if event.Role != "tool_call" {
			continue
		}
		if id != "" && event.ID != id {
			continue
		}
		if id == "" && name != "" && event.Name != name {
			continue
		}
		c.displayEvents[i].Args += delta
		return c.persistLastTurnDisplayEventLocked(c.displayEvents[i])
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
	for i := len(c.displayEvents) - 1; i >= 0; i-- {
		event := c.displayEvents[i]
		if event.Role != "tool_call" {
			continue
		}
		if id != "" && event.ID != id {
			continue
		}
		if id == "" && name != "" && event.Name != name {
			continue
		}
		c.displayEvents[i].Status = status
		return c.persistLastTurnDisplayEventLocked(c.displayEvents[i])
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
	for i := len(c.displayEvents) - 1; i >= 0; i-- {
		event := c.displayEvents[i]
		if event.Role != "tool_call" {
			continue
		}
		if id != "" && event.ID != id {
			continue
		}
		if id == "" && name != "" && event.Name != name {
			continue
		}
		c.displayEvents[i].Status = status
		c.displayEvents[i].Result = result
		return c.persistLastTurnDisplayEventLocked(c.displayEvents[i])
	}
	return nil
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
		interactiveStateSourceSummary(storyCtx.Meta.Title, storyCtx.Meta.Origin, teller, loreContext, storyMemorySchema, storyMemory, turnHistory, turn.User, turn.Narrative),
		interactivePartSummary(instruction),
	)
	return instruction, nil
}

func (c *interactiveConversation) teller(tellerID string) interactive.Teller {
	return loadInteractiveTeller(c.novaDir, tellerID)
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

func interactiveStoryTellerSystemInput(teller interactive.Teller) prompts.InteractiveStorySystemInstructionInput {
	return prompts.InteractiveStorySystemInstructionInput{
		StoryTellerID:           teller.ID,
		StoryTellerName:         teller.Name,
		StoryTellerDescription:  teller.Description,
		StoryTellerSystemPrompt: teller.PromptForTargets("system"),
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

func interactiveStateOpsLogJSON(ops []interactive.StateOp) string {
	data, err := json.Marshal(ops)
	if err != nil {
		return fmt.Sprintf("<marshal error: %v>", err)
	}
	return string(data)
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
	interactiveStateMemorySchemaBytes = 8 * 1024
	interactiveStateLoreContextBytes  = 32 * 1024
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

func interactiveStorySourceSummary(title, origin string, teller interactive.Teller, storyMemory string, turnMemory interactiveTurnMemory, userAction string) string {
	parts := []interactiveContextSource{
		{Source: "互动故事", Title: "故事标题", Content: title},
		{Source: "互动故事", Title: "开端", Content: origin},
	}
	parts = append(parts, interactiveTellerSlotSources(teller, "turn_context")...)
	if strings.TrimSpace(storyMemory) != "" {
		parts = append(parts, interactiveContextSource{Source: "故事记忆", Title: "当前分支可见故事记忆", Content: storyMemory})
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

func interactiveStateSourceSummary(title, origin string, teller interactive.Teller, loreItems, storyMemorySchema, storyMemory, turnHistory, userAction, narrative string) string {
	parts := []interactiveContextSource{
		{Source: "互动故事", Title: "故事标题", Content: title},
		{Source: "互动故事", Title: "开端", Content: origin},
	}
	parts = append(parts, interactiveTellerSlotSources(teller, "state_memory")...)
	if strings.TrimSpace(loreItems) != "" {
		parts = append(parts, interactiveContextSource{Source: "资料库", Title: ".nova/lore/items.json", Content: loreItems})
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
	if len(parts) == 0 {
		return "count=0"
	}
	items := make([]string, 0, len(parts))
	for i, part := range parts {
		content := strings.TrimSpace(part.Content)
		if content == "" {
			continue
		}
		fields := []string{
			fmt.Sprintf("%d:source=%q", i, part.Source),
			fmt.Sprintf("title=%q", part.Title),
			fmt.Sprintf("bytes=%d", len(content)),
			fmt.Sprintf("chars=%d", utf8.RuneCountInString(content)),
			"preview=" + strconv.Quote(interactiveSafePreview(content, 100)),
		}
		if part.Note != "" {
			fields = append(fields, "note="+strconv.Quote(part.Note))
		}
		items = append(items, strings.Join(fields, ","))
	}
	return fmt.Sprintf("count=%d parts=[%s]", len(items), strings.Join(items, "; "))
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
