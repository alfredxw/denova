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
	directorTask         string
	mu                   sync.Mutex
	lastTurn             *interactive.TurnEvent
	lastStateReady       bool
	lastSources          string
	assistantMetadata    session.MessageMetadata
	displayEvents        []interactive.DisplayEvent
	modelContextMessages []interactive.ModelContextMessage
	ruleResolution       *interactive.RuleResolution
	turnResult           *interactive.TurnResult
	baseParentID         *string
	directorTasks        *workspaceDirectorTaskGroup
	directorGenerator    interactiveDirectorGenerator
}

type interactiveDirectorGenerator func(context.Context, *config.Config, *book.State, agent.InteractiveStoryToolContext, string) (string, error)

func newInteractiveConversation(store *interactive.Store, novaDir, workspace, storyID, branchID, user string, replyTargetChars int, cfg *config.Config) *interactiveConversation {
	return &interactiveConversation{store: store, novaDir: novaDir, workspace: workspace, cfg: cfg, storyID: storyID, branchID: branchID, user: user, replyTargetChars: replyTargetChars, directorGenerator: generateInteractiveDirector}
}

func (c *interactiveConversation) bindDirectorRuntime(tasks *workspaceDirectorTaskGroup, generators ...interactiveDirectorGenerator) *interactiveConversation {
	if c != nil {
		c.directorTasks = tasks
		if len(generators) > 0 && generators[0] != nil {
			c.directorGenerator = generators[0]
		}
	}
	return c
}

func (c *interactiveConversation) withDirectorTask(task string) *interactiveConversation {
	if c != nil {
		c.directorTask = strings.TrimSpace(task)
	}
	return c
}

func (c *interactiveConversation) withBaseParentID(parentID string) *interactiveConversation {
	if c != nil {
		parentID = strings.TrimSpace(parentID)
		c.baseParentID = &parentID
	}
	return c
}

func (c *interactiveConversation) directorTaskHint() string {
	if c == nil {
		return ""
	}
	switch strings.TrimSpace(c.directorTask) {
	case "memory_update":
		return "memory_update：只根据已提交的 TurnResult、StateDelta 和最终正文维护 Story Memory；不得写 Actor State 或 director.md。"
	case "director_plan_update":
		return "director_plan_update：观察已提交事实并判断 keep、patch 或 replan；只维护当前分支 director.md 与 lore-context.md，不得写 Story Memory 或 Actor State。"
	default:
		return "director_plan_update：观察已提交事实并判断 keep、patch 或 replan；只维护当前分支 director.md 与 lore-context.md，不得写 Story Memory 或 Actor State。"
	}
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
	storyDirector := storyDirectorForSnapshot(c.storyDirector(storyCtx.Meta.StoryDirectorID), storyCtx.Meta.ActorStateSchema)
	tellerTurnContextPrompt := teller.PromptForTargets("turn_context")
	turnMemory := buildInteractiveModelVisibleTurnMemory(storyCtx.Snapshot.Turns, storyCtx.Snapshot.ContextCompaction)
	storyMemory, err := c.store.StoryMemoryContextSummary(c.storyID, storyCtx.Snapshot.BranchID, interactiveStoryRuntimeContextBytes)
	if err != nil {
		log.Printf("[interactive-agent] load story memory failed story_id=%s branch_id=%s err=%v", c.storyID, storyCtx.Snapshot.BranchID, err)
		storyMemory = ""
	}
	directorPlanVisible := ""
	directorPlan := interactive.DirectorPlan{}
	if storyCtx.Snapshot.DirectorPlan != nil {
		directorPlan = *storyCtx.Snapshot.DirectorPlan
		directorPlanVisible = interactive.DirectorPlanVisibleContext(directorPlan, interactiveStoryRuntimeContextBytes)
	}
	loreRuntime, err := buildInteractiveStoryLoreContext(c.workspace, directorPlan, agentMessage, c.cfg)
	if err != nil {
		return nil, err
	}
	residentLore, err := book.NewLoreStore(c.workspace).ResidentContextMarkdown()
	if err != nil {
		return nil, fmt.Errorf("读取常驻资料失败: %w", err)
	}
	residentContentBytes, err := book.NewLoreStore(c.workspace).ResidentContentBytes()
	if err != nil {
		return nil, fmt.Errorf("读取常驻资料预算失败: %w", err)
	}
	if residentContentBytes > residentLoreLimitBytes(c.cfg) {
		return nil, fmt.Errorf("常驻资料合计超过 %d KB；请缩短常驻正文、改为按需资料或提高常驻资料上限", residentLoreLimitBytes(c.cfg)/1024)
	}
	ruleSummary := interactive.StoryDirectorRuleSummary(storyDirector, interactiveStoryRuntimeContextBytes)
	actorStateRuntime := interactive.ActorStateRuntimeContext(storyDirector.ActorState, storyCtx.Snapshot.State, interactiveStoryRuntimeContextBytes)
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
		ActorState:                  actorStateRuntime,
		StoryDirectorStrategyPrompt: strategyPrompt,
		PreviousTurnsSummary:        turnMemory.PreviousSummary,
		LoreContext:                 loreRuntime,
	})
	history := make([]*schema.Message, 0, len(turnMemory.Turns)*2+4)
	if residentLore != "" {
		history = append(history, schema.UserMessage(agentcontext.StandaloneMessage("常驻资料库", residentLore, "source: enabled resident lore; stable leading context")))
	}
	if storyCtx.Snapshot.ContextCompaction != nil && strings.TrimSpace(storyCtx.Snapshot.ContextCompaction.Summary) != "" {
		history = append(history, agent.NewContextCompactionSummaryMessage(storyCtx.Snapshot.ContextCompaction.Epoch, storyCtx.Snapshot.ContextCompaction.Summary))
	}
	for _, turn := range turnMemory.Turns {
		history = append(history, schema.UserMessage(turn.User))
		history = append(history, schemaMessagesFromInteractiveContext(turn.ModelContextMessages)...)
		history = append(history, schema.AssistantMessage(turn.Narrative, nil))
	}
	history = agent.ApplyToolResultContextPolicyForConversation(history, c.ToolResultContextPolicy())
	history = append(history, schema.UserMessage(prompts.InteractiveStoryTurnInstruction(agentMessage, tellerTurnContextPrompt, runtimeContext)))
	sourceSummary := interactiveStorySourceSummary(storyCtx.Meta.Title, storyCtx.Meta.Origin, teller, storyMemory, directorPlanVisible, joinLoreContextSections(residentLore, loreRuntime), ruleSummary, strategyPrompt, turnMemory, agentMessage)
	c.mu.Lock()
	c.lastSources = sourceSummary
	c.mu.Unlock()
	log.Printf(
		"[interactive-agent] context composition story_id=%s branch_id=%s story_title=%s origin=%s teller_id=%s story_director_id=%s teller_slots=%s teller_turn_context=%s story_memory=%s director_plan=%s turns=%d model_turns=%d compressed_turns=%s history=%s turn_instruction=%s sources=%s",
		c.storyID,
		storyCtx.Snapshot.BranchID,
		interactivePartSummary(storyCtx.Meta.Title),
		interactivePartSummary(storyCtx.Meta.Origin),
		storyCtx.Meta.StoryTellerID,
		storyCtx.Meta.StoryDirectorID,
		interactiveTellerSlotSummary(teller, "turn_context"),
		interactivePartSummary(tellerTurnContextPrompt),
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
	storyDirector := storyDirectorForSnapshot(c.storyDirector(storyCtx.Meta.StoryDirectorID), storyCtx.Meta.ActorStateSchema)
	resolution, err := interactive.ResolveTurnRulesWithDirector(c.storyID, storyCtx.Snapshot.BranchID, storyCtx.Snapshot.State, storyDirector, request)
	if err != nil {
		return interactive.RuleResolution{}, err
	}
	c.mu.Lock()
	c.ruleResolution = &resolution
	c.mu.Unlock()
	return resolution, nil
}

// SubmitTurnResult stages the Game Agent's structured outcome. Nothing is
// persisted until the final narrative is accepted and committed atomically.
func (c *interactiveConversation) SubmitTurnResult(ctx context.Context, result interactive.TurnResult) (interactive.TurnResult, error) {
	if c == nil || c.store == nil {
		return interactive.TurnResult{}, fmt.Errorf("互动故事不存在")
	}
	select {
	case <-ctx.Done():
		return interactive.TurnResult{}, ctx.Err()
	default:
	}
	result = interactive.NormalizeTurnResult(result)
	if strings.TrimSpace(result.Contract.PlayerIntent) == "" {
		result.Contract.PlayerIntent = strings.TrimSpace(c.user)
	}
	if err := interactive.ValidateTurnResult(result); err != nil {
		return interactive.TurnResult{}, err
	}
	if len(result.ActorStatePatches) > 0 {
		storyCtx, err := c.store.StoryContext(c.storyID, c.branchID)
		if err != nil {
			return interactive.TurnResult{}, err
		}
		actorState := interactive.StoryDirectorActorStateSystem{}
		if storyCtx.Meta.ActorStateSchema != nil {
			actorState = storyCtx.Meta.ActorStateSchema.System
		} else {
			actorState = c.storyDirector(storyCtx.Meta.StoryDirectorID).ActorState
		}
		if _, err := interactive.ValidateActorStatePatchesAgainstState(actorState, storyCtx.Snapshot.State, result.ActorStatePatches, ""); err != nil {
			return interactive.TurnResult{}, fmt.Errorf("Actor 状态更新校验失败，可修正参数后重试 submit_interactive_turn_result: %w", err)
		}
	}
	c.mu.Lock()
	c.turnResult = &result
	c.mu.Unlock()
	log.Printf("[interactive-agent] staged turn result story_id=%s branch_id=%s state_patches=%d facts=%d choices=%d scene_status=%s deviation=%s", c.storyID, c.branchID, len(result.ActorStatePatches), len(result.FactCandidates), len(result.Choices), result.SceneResult.Status, result.PlanSignals.DeviationLevel)
	return result, nil
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
	completionReserve, toolReserve := agent.EstimateContextProjectionReserves(c.cfg, config.AgentKindInteractiveStory, c.replyTargetChars)
	if input.ReservedCompletionTokens <= 0 {
		input.ReservedCompletionTokens = completionReserve
	}
	if input.ReservedToolResultTokens <= 0 {
		input.ReservedToolResultTokens = toolReserve
	}
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
	return c.AppendAssistantWithMetadata(content, thinking, session.MessageMetadata{})
}

func (c *interactiveConversation) AppendAssistantWithMetadata(content, thinking string, metadata session.MessageMetadata) error {
	if c == nil || c.store == nil {
		return fmt.Errorf("互动故事不存在")
	}
	if strings.TrimSpace(metadata.RunID) != "" {
		c.mu.Lock()
		c.assistantMetadata = metadata
		c.mu.Unlock()
	}
	log.Printf("[interactive-agent] parse assistant output content story_id=%s branch_id=%s content=%q", c.storyID, c.branchID, content)
	narrative, parseErr := parseInteractiveAssistantOutput(content)
	if parseErr != nil {
		log.Printf("[interactive-agent] parse assistant output failed story_id=%s branch_id=%s err=%v content=%q", c.storyID, c.branchID, parseErr, content)
		return parseErr
	}
	log.Printf("[interactive-agent] parse assistant output result story_id=%s branch_id=%s narrative=%q", c.storyID, c.branchID, narrative)
	assistantMetadata := c.assistantMetadataSnapshot()
	turnResult := c.turnResultSnapshot()
	if turnResult == nil {
		return fmt.Errorf("互动回合缺少 submit_interactive_turn_result，已拒绝写入不完整状态")
	}
	turn, _, err := c.store.AppendTurnWithState(c.storyID, interactive.AppendTurnWithStateRequest{
		BranchID:             c.branchID,
		ExpectedParentID:     c.baseParentIDSnapshot(),
		User:                 c.user,
		Narrative:            narrative,
		Thinking:             thinking,
		RunID:                assistantMetadata.RunID,
		AgentKind:            assistantMetadata.AgentKind,
		DisplayEvents:        c.displayEventsSnapshot(),
		ModelContextMessages: c.modelContextMessagesSnapshot(),
		RuleResolution:       c.ruleResolutionSnapshot(),
		TurnResult:           turnResult,
		TerminalOutcome:      c.terminalOutcomeSnapshot(narrative),
	})
	if err == nil {
		c.mu.Lock()
		c.lastTurn = &turn
		c.lastStateReady = turn.StateStatus == "ready"
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

func (c *interactiveConversation) assistantMetadataSnapshot() session.MessageMetadata {
	c.mu.Lock()
	defer c.mu.Unlock()
	metadata := c.assistantMetadata
	metadata.RunPath = append([]string(nil), metadata.RunPath...)
	return metadata
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

func (c *interactiveConversation) turnResultSnapshot() *interactive.TurnResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.turnResult == nil {
		return nil
	}
	result := interactive.NormalizeTurnResult(*c.turnResult)
	return &result
}

func (c *interactiveConversation) baseParentIDSnapshot() *string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.baseParentID == nil {
		return nil
	}
	value := *c.baseParentID
	return &value
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

func (c *interactiveConversation) BuildDirectorInstruction(turn interactive.TurnEvent) (string, error) {
	if c == nil || c.store == nil {
		return "", fmt.Errorf("互动故事不存在")
	}
	storyCtx, err := c.store.StoryContext(c.storyID, c.branchID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(c.directorTask) == interactiveDirectorTaskMemoryUpdate {
		return c.buildMemoryRecorderInstruction(storyCtx, turn)
	}
	storyMemory, err := c.store.StoryMemoryContextSummary(c.storyID, storyCtx.Snapshot.BranchID, interactiveDirectorContextBytes)
	if err != nil {
		log.Printf("[interactive-director-agent] load story memory failed story_id=%s branch_id=%s err=%v", c.storyID, storyCtx.Snapshot.BranchID, err)
		storyMemory = ""
	}
	storyDirector := storyDirectorForSnapshot(c.storyDirector(storyCtx.Meta.StoryDirectorID), storyCtx.Meta.ActorStateSchema)
	strategyPrompt := interactive.StoryDirectorStrategyPromptMarkdown(storyDirector)
	turnMemory := buildInteractiveModelVisibleTurnMemory(storyCtx.Snapshot.Turns, storyCtx.Snapshot.ContextCompaction)
	turnHistory := formatInteractiveTurnMemoryHistory(turnMemory, storyCtx.Snapshot.ContextCompaction, "（暂无历史回合，请基于本回合审计更新导演计划。）")
	directorPlan := interactive.DirectorPlan{}
	if storyCtx.Snapshot.DirectorPlan != nil {
		directorPlan = *storyCtx.Snapshot.DirectorPlan
	} else if plan, err := c.store.DirectorPlan(c.storyID, storyCtx.Snapshot.BranchID); err == nil {
		directorPlan = plan
	}
	loreContext, err := buildInteractiveDirectorLoreContext(c.workspace, directorPlan, turn, c.cfg)
	if err != nil {
		return "", err
	}
	actorStateSnapshot := map[string]any{}
	if actors, ok := storyCtx.Snapshot.State["actors"]; ok {
		actorStateSnapshot = map[string]any{"actors": actors}
	}
	allowedPaths := c.store.DirectorPlanAllowedPaths(c.storyID, storyCtx.Snapshot.BranchID)
	budget := newDirectorContextBudget(c.cfg, interactiveDirectorTaskDirectorPlanUpdate)
	title := budget.take("story.title", storyCtx.Meta.Title, 512)
	turnAudit := budget.take("turn.audit", boundedJSON(interactiveDirectorTurnAudit(turn), interactiveDirectorContextBytes), interactiveDirectorContextBytes)
	planDocs := budget.take("director_plan.docs", boundedJSON(directorPlan.Docs, interactiveDirectorContextBytes), interactiveDirectorContextBytes)
	actorState := budget.take("actor_state.snapshot", boundedJSON(actorStateSnapshot, interactiveDirectorContextBytes), interactiveDirectorContextBytes)
	actorStateSchema := budget.take("actor_state.schema", interactive.ActorStateSchemaContext(storyDirector.ActorState, interactiveDirectorContextBytes), interactiveDirectorContextBytes)
	memoryContext := budget.take("story_memory.records", storyMemory, interactiveDirectorContextBytes)
	lore := budget.take("lore.relevant", loreContext, interactiveDirectorContextBytes)
	history := budget.take("turn.history", turnHistory, interactiveDirectorContextBytes)
	origin := budget.take("story.origin", storyCtx.Meta.Origin, interactiveDirectorContextBytes)
	planningTemplates := budget.take("director.strategy.templates", boundedJSON(storyDirector.Strategy.PlanningTemplates, interactiveDirectorContextBytes), interactiveDirectorContextBytes)
	planningSummary := budget.take("director.planning_summary", interactive.StoryDirectorPlanningSummary(storyDirector, interactiveDirectorContextBytes), interactiveDirectorContextBytes)
	strategyContext := budget.take("director.strategy.prompt", strategyPrompt, interactiveDirectorContextBytes)
	eventOpportunity, eventRuntime, eventIndex, eventErr := c.store.DirectorEventContext(c.storyID, storyCtx.Snapshot.BranchID, turn.ID)
	if eventErr != nil {
		return "", fmt.Errorf("读取事件编排上下文失败: %w", eventErr)
	}
	eventCatalog := ""
	if len(eventIndex) > 0 {
		eventCatalog = budget.take("director.events", boundedJSON(eventIndex, interactiveDirectorContextBytes), interactiveDirectorContextBytes)
	}
	instruction := prompts.InteractiveDirectorInstruction(prompts.InteractiveDirectorPromptInput{
		Title:                       title,
		Origin:                      origin,
		StoryTellerID:               budget.take("story.teller_id", storyCtx.Meta.StoryTellerID, 128),
		StoryDirectorID:             budget.take("story.director_id", storyCtx.Meta.StoryDirectorID, 128),
		BranchID:                    budget.take("story.branch_id", storyCtx.Snapshot.BranchID, 128),
		TaskHint:                    budget.take("director.task", c.directorTaskHint(), 1024),
		DirectorPlanPaths:           budget.take("director_plan.paths", strings.Join(allowedPaths, "\n"), 2*1024),
		DirectorPlanDocs:            planDocs,
		PlanningTemplates:           planningTemplates,
		BranchPlanningTurns:         storyDirector.Strategy.BranchPlanningTurns,
		LoreContext:                 lore,
		TurnAuditJSON:               turnAudit,
		TurnHistory:                 history,
		StoryMemory:                 memoryContext,
		ActorStateSchema:            actorStateSchema,
		ActorState:                  actorState,
		StoryMemorySummary:          "",
		StoryDirectorPlan:           planningSummary,
		StoryDirectorStrategyPrompt: strategyContext,
		DirectorEventCatalog:        eventCatalog,
		EventOpportunity:            budget.take("director.event_opportunity", boundedJSON(eventOpportunity, 4*1024), 4*1024),
		EventRuntime:                budget.take("director.event_runtime", boundedJSON(eventRuntime, 8*1024), 8*1024),
	})
	log.Printf("[interactive-director-agent] context budget story_id=%s branch_id=%s turn_id=%s instruction_bytes=%d model_window_tokens=%d threshold_tokens=%d source_budget_tokens=%d fragments=%s", c.storyID, storyCtx.Snapshot.BranchID, turn.ID, len(instruction), budget.contextWindowTokens, budget.thresholdTokens, budget.initialTokens, budget.trace())
	log.Printf(
		"[interactive-director-agent] context composition story_id=%s branch_id=%s turn_id=%s teller_id=%s story_director_id=%s director_plan=%s allowed_paths=%d teller_memory_rules=%s lore=%s turn_audit=%s story_memory=%s story_memory_schema=%s actor_state=%s history=%s instruction=%s",
		c.storyID,
		storyCtx.Snapshot.BranchID,
		turn.ID,
		storyCtx.Meta.StoryTellerID,
		storyCtx.Meta.StoryDirectorID,
		interactivePartSummary(boundedJSON(directorPlan.Docs, interactiveDirectorContextBytes)),
		len(allowedPaths),
		"none",
		interactivePartSummary(loreContext),
		interactivePartSummary(boundedJSON(interactiveDirectorTurnAudit(turn), interactiveDirectorContextBytes)),
		interactivePartSummary(storyMemory),
		"not_injected",
		interactivePartSummary(boundedJSON(actorStateSnapshot, interactiveDirectorContextBytes)),
		interactivePartSummary(turnHistory),
		interactivePartSummary(instruction),
	)
	return instruction, nil
}

func (c *interactiveConversation) buildMemoryRecorderInstruction(storyCtx interactive.StoryContext, turn interactive.TurnEvent) (string, error) {
	storyMemory, err := c.store.StoryMemoryContextSummary(c.storyID, storyCtx.Snapshot.BranchID, interactive.DirectorContextMaxBytes)
	if err != nil {
		return "", fmt.Errorf("读取故事记忆失败: %w", err)
	}
	storyMemorySchema, err := c.store.StoryMemorySchemaContext(c.storyID, interactive.DirectorContextMaxBytes)
	if err != nil {
		return "", fmt.Errorf("读取故事记忆结构失败: %w", err)
	}
	turnMemory := buildInteractiveModelVisibleTurnMemory(storyCtx.Snapshot.Turns, storyCtx.Snapshot.ContextCompaction)
	turnHistory := formatInteractiveTurnMemoryHistory(turnMemory, storyCtx.Snapshot.ContextCompaction, "（暂无更早历史回合。）")
	teller := c.teller(storyCtx.Meta.StoryTellerID)
	budget := newDirectorContextBudget(c.cfg, interactiveDirectorTaskMemoryUpdate)
	instruction := prompts.InteractiveMemoryRecorderInstruction(prompts.InteractiveMemoryRecorderPromptInput{
		Title:                  budget.take("story.title", storyCtx.Meta.Title, 512),
		BranchID:               budget.take("story.branch_id", storyCtx.Snapshot.BranchID, 128),
		TurnAuditJSON:          budget.take("turn.audit", boundedNewestTurnAudits(interactiveMemoryRecorderTurnAudit(storyCtx.Snapshot.Turns, turn.ID), interactive.DirectorContextMaxBytes), interactive.DirectorContextMaxBytes),
		StoryMemorySchema:      budget.take("story_memory.schema", storyMemorySchema, interactive.DirectorContextMaxBytes),
		StoryMemory:            budget.take("story_memory.records", storyMemory, interactive.DirectorContextMaxBytes),
		StoryTellerMemoryRules: budget.take("teller.state_memory", teller.PromptForTargets("state_memory"), interactive.DirectorContextMaxBytes),
		TurnHistory:            budget.take("turn.history", turnHistory, interactive.DirectorContextMaxBytes),
	})
	log.Printf("[interactive-memory-recorder] context composition story_id=%s branch_id=%s turn_id=%s model_window_tokens=%d threshold_tokens=%d source_budget_tokens=%d fragments=%s instruction=%s", c.storyID, storyCtx.Snapshot.BranchID, turn.ID, budget.contextWindowTokens, budget.thresholdTokens, budget.initialTokens, budget.trace(), interactivePartSummary(instruction))
	return instruction, nil
}

func interactiveDirectorTurnAudit(turn interactive.TurnEvent) map[string]any {
	return map[string]any{
		"turn_id":          turn.ID,
		"branch_id":        turn.BranchID,
		"user_action":      boundedText(turn.User, 4*1024),
		"narrative":        boundedText(turn.Narrative, 16*1024),
		"rule_resolution":  turn.RuleResolution,
		"turn_result":      turn.TurnResult,
		"state_delta":      turn.StateDelta,
		"terminal_outcome": turn.TerminalOutcome,
	}
}

func boundedNewestTurnAudits(audits []map[string]any, limit int) string {
	if limit <= 0 {
		return "[]"
	}
	for start := 0; start < len(audits); start++ {
		payload := map[string]any{
			"omitted_older_turns": start,
			"turns":               audits[start:],
		}
		data, err := json.MarshalIndent(payload, "", "  ")
		if err == nil && len(data) <= limit {
			return string(data)
		}
	}
	return "{\"omitted_older_turns\":0,\"turns\":[]}"
}

func interactiveMemoryRecorderTurnAudit(turns []interactive.TurnEvent, currentTurnID string) []map[string]any {
	const maxMemoryAuditTurns = 12
	end := len(turns)
	for i := range turns {
		if turns[i].ID == currentTurnID {
			end = i + 1
			break
		}
	}
	start := max(0, end-maxMemoryAuditTurns)
	result := make([]map[string]any, 0, end-start)
	for _, turn := range turns[start:end] {
		result = append(result, interactiveDirectorTurnAudit(turn))
	}
	return result
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
		const marker = "\n...（已按上下文上限截断）"
		prefix, _ := trimStringToUTF8Bytes(value, max(0, limit-len(marker)))
		markerPart, _ := trimStringToUTF8Bytes(marker, limit-len(prefix))
		return prefix + markerPart
	}
	return trimmed
}

type directorContextBudget struct {
	remainingTokens     int
	initialTokens       int
	contextWindowTokens int
	thresholdTokens     int
	parts               []string
}

func newDirectorContextBudget(cfg *config.Config, task string) *directorContextBudget {
	model := config.ResolveAgentModel(cfg, config.AgentKindInteractiveDirector)
	window := model.ContextWindowTokens
	if window <= 0 {
		window = config.DefaultContextWindowTokens
	}
	contextSettings := config.ResolveAgentContext(cfg, config.AgentKindInteractiveDirector)
	threshold := contextSettings.CompactionThreshold
	if threshold <= 0 {
		threshold = 0.90
	}
	thresholdTokens := int(float64(window) * threshold)
	systemPrompt := prompts.BuildInteractiveDirectorSystemInstruction()
	emptyPrompt := prompts.InteractiveDirectorInstruction(prompts.InteractiveDirectorPromptInput{})
	if task == interactiveDirectorTaskMemoryUpdate {
		systemPrompt = prompts.BuildInteractiveMemoryRecorderSystemInstruction()
		emptyPrompt = prompts.InteractiveMemoryRecorderInstruction(prompts.InteractiveMemoryRecorderPromptInput{})
	}
	customPrompt := config.ResolveAgentPrompt(cfg, config.AgentKindInteractiveDirector).SystemPrompt
	overheadTokens := agent.EstimateContextTokens([]*schema.Message{
		schema.SystemMessage(systemPrompt + "\n" + customPrompt),
		schema.UserMessage(emptyPrompt),
	}, nil)
	completionReserve, toolReserve := agent.EstimateContextProjectionReserves(cfg, config.AgentKindInteractiveDirector, 1024)
	toolSchemaAndRuntimeHeadroom := max(2048, window/100)
	available := max(0, thresholdTokens-overheadTokens-completionReserve-toolReserve-toolSchemaAndRuntimeHeadroom)
	return &directorContextBudget{
		remainingTokens:     available,
		initialTokens:       available,
		contextWindowTokens: window,
		thresholdTokens:     thresholdTokens,
	}
}

func (b *directorContextBudget) take(source, value string, fragmentLimit int) string {
	originalBytes := len(value)
	if fragmentLimit <= 0 || fragmentLimit > interactive.DirectorContextMaxBytes {
		fragmentLimit = interactive.DirectorContextMaxBytes
	}
	kept := boundedText(value, fragmentLimit)
	kept = fitTextToTokenBudget(kept, b.remainingTokens)
	usedTokens := agent.EstimateContextTokens([]*schema.Message{schema.UserMessage(kept)}, nil)
	if strings.TrimSpace(kept) == "" {
		usedTokens = 0
	}
	b.remainingTokens = max(0, b.remainingTokens-usedTokens)
	b.parts = append(b.parts, fmt.Sprintf("%s:%dB->%dB/%dt", source, originalBytes, len(kept), usedTokens))
	return kept
}

func (b *directorContextBudget) trace() string {
	return strings.Join(b.parts, ",")
}

func fitTextToTokenBudget(value string, tokenBudget int) string {
	if tokenBudget <= 0 || strings.TrimSpace(value) == "" {
		return ""
	}
	if agent.EstimateContextTokens([]*schema.Message{schema.UserMessage(value)}, nil) <= tokenBudget {
		return value
	}
	low, high := 0, len(value)
	for low < high {
		mid := low + (high-low+1)/2
		candidate, _ := trimStringToUTF8Bytes(value, mid)
		if agent.EstimateContextTokens([]*schema.Message{schema.UserMessage(candidate)}, nil) <= tokenBudget {
			low = mid
		} else {
			high = mid - 1
		}
	}
	trimmed, _ := trimStringToUTF8Bytes(value, low)
	return trimmed
}

func (c *interactiveConversation) teller(tellerID string) interactive.Teller {
	return loadInteractiveTeller(c.novaDir, tellerID)
}

func (c *interactiveConversation) storyDirector(directorID string) interactive.StoryDirector {
	return loadStoryDirector(c.novaDir, directorID)
}

func storyDirectorForSnapshot(director interactive.StoryDirector, snapshot *interactive.ActorStateSchemaSnapshot) interactive.StoryDirector {
	if snapshot == nil || len(snapshot.System.Templates) == 0 {
		return director
	}
	director.ActorState = snapshot.System
	if len(snapshot.TRPGSystem.RuleTemplates) > 0 {
		director.TRPGSystem = snapshot.TRPGSystem
	}
	return director
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
	interactiveStoryRuntimeContextBytes = 16 * 1024
	interactiveDirectorContextBytes     = interactive.DirectorContextMaxBytes
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

func interactiveStorySourceSummary(title, origin string, teller interactive.Teller, storyMemory, directorPlanVisible, loreContext, ruleSummary, strategyPrompt string, turnMemory interactiveTurnMemory, userAction string) string {
	parts := []interactiveContextSource{
		{Source: "互动故事", Title: "故事标题", Content: title},
		{Source: "互动故事", Title: "开端", Content: origin},
	}
	parts = append(parts, interactiveTellerSlotSources(teller, "turn_context")...)
	if strings.TrimSpace(storyMemory) != "" {
		parts = append(parts, interactiveContextSource{Source: "故事记忆", Title: "当前分支可见故事记忆", Content: storyMemory})
	}
	if strings.TrimSpace(directorPlanVisible) != "" {
		parts = append(parts, interactiveContextSource{Source: "DirectorPlan", Title: "后台导演规划可读区", Content: directorPlanVisible, Note: "bounded"})
	}
	if strings.TrimSpace(loreContext) != "" {
		parts = append(parts, interactiveContextSource{Source: "LoreContext", Title: "规则与当前资料工作集", Content: loreContext, Note: "bounded"})
	}
	if strings.TrimSpace(ruleSummary) != "" {
		parts = append(parts, interactiveContextSource{Source: "StoryDirector", Title: "故事导演规则清单", Content: ruleSummary, Note: "bounded"})
	}
	if strings.TrimSpace(strategyPrompt) != "" {
		parts = append(parts, interactiveContextSource{Source: "StoryDirector.strategy.prompt_markdown", Title: "故事导演 Markdown 策略提示", Content: strategyPrompt, Note: "bounded"})
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
