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
	store                   *interactive.Store
	novaDir                 string
	workspace               string
	cfg                     *config.Config
	storyID                 string
	branchID                string
	user                    string
	replyTargetChars        int
	directorTask            string
	mu                      sync.Mutex
	lastTurn                *interactive.TurnEvent
	lastStateReady          bool
	lastSources             string
	lastContextSources      []interactiveContextSource
	lastContextLedgerParts  []agent.ContextLedgerPart
	stableLeadingMessage    string
	assistantMetadata       session.MessageMetadata
	displayEvents           []interactive.DisplayEvent
	modelContextMessages    []interactive.ModelContextMessage
	ruleResolution          *interactive.RuleResolution
	turnProtocol            interactiveTurnProtocol
	baseParentID            *string
	directorTasks           *workspaceDirectorTaskGroup
	directorGenerator       interactiveDirectorGenerator
	customDirectorGenerator bool
	openingStateSchemaDraft *interactive.ActorStateSchemaBatchDraft
	openingStateSchemaAudit interactive.ActorStateSchemaBatchAudit
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
			c.customDirectorGenerator = true
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

func (c *interactiveConversation) withOpeningStateSchema(storyCtx interactive.StoryContext) *interactiveConversation {
	if c == nil || !interactive.StoryStateSchemaPolicyUsesOpeningGameAgent(storyCtx.Meta.StateSchemaPolicy) || storyCtx.Meta.StateSchemaInitialization == nil || storyCtx.Meta.StateSchemaInitialization.Status != interactive.StateSchemaInitializationWaitingOpening || storyCtx.Meta.ActorStateSchema == nil || len(storyCtx.Snapshot.Turns) > 0 {
		return c
	}
	trpgSourceIDs := make([]string, 0, len(storyCtx.Meta.ActorStateSchema.TRPGSystem.RuleTemplates))
	for _, rule := range storyCtx.Meta.ActorStateSchema.TRPGSystem.RuleTemplates {
		if id := strings.TrimSpace(rule.ID); id != "" {
			trpgSourceIDs = append(trpgSourceIDs, id)
		}
	}
	c.mu.Lock()
	c.openingStateSchemaDraft = interactive.NewOpeningActorStateSchemaBatchDraft(storyCtx.Meta.ActorStateSchema.System, storyCtx.Meta.ActorStateSchema.TRPGSystem)
	c.openingStateSchemaAudit = interactive.ActorStateSchemaBatchAudit{
		OpeningSourceIDs: []string{"opening-draft"},
		TRPGSourceIDs:    trpgSourceIDs,
		CurrentState:     storyCtx.Snapshot.State,
	}
	c.mu.Unlock()
	log.Printf("[interactive-agent] enabled opening state schema draft story_id=%s branch_id=%s mode=%s base_revision=%d", c.storyID, storyCtx.Snapshot.BranchID, storyCtx.Meta.StateSchemaPolicy.Mode, storyCtx.Meta.ActorStateSchema.Revision)
	return c
}

func (c *interactiveConversation) SubmitOpeningStateSchemaBatch(ctx context.Context, batch interactive.ActorStateSchemaBatch) (interactive.ActorStateSchemaBatchResult, error) {
	if c == nil {
		return interactive.ActorStateSchemaBatchResult{}, fmt.Errorf("互动故事不存在")
	}
	select {
	case <-ctx.Done():
		return interactive.ActorStateSchemaBatchResult{}, ctx.Err()
	default:
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.openingStateSchemaDraft == nil {
		return interactive.ActorStateSchemaBatchResult{}, fmt.Errorf("当前故事不需要 Game Agent 初始化状态结构")
	}
	result := c.openingStateSchemaDraft.SubmitStructureOnly(batch, c.openingStateSchemaAudit)
	log.Printf("[interactive-agent] staged opening state schema story_id=%s branch_id=%s accepted=%d rejected=%d blocked=%d finalized=%t draft_items=%d", c.storyID, c.branchID, len(result.Accepted), len(result.Rejected), len(result.Blocked), result.Finalized, result.DraftAcceptedItems)
	return result, nil
}

func (c *interactiveConversation) openingStateSchemaProposalSnapshot() *interactive.ActorStateSchemaProposal {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	proposal, ok := c.openingStateSchemaDraft.FinalProposal()
	if !ok {
		return nil
	}
	return &proposal
}

func (c *interactiveConversation) effectiveTurnState(storyCtx interactive.StoryContext) (interactive.StoryDirectorActorStateSystem, map[string]any, error) {
	actorState := interactive.StoryDirectorActorStateSystem{}
	if storyCtx.Meta.ActorStateSchema != nil {
		actorState = storyCtx.Meta.ActorStateSchema.System
	} else {
		actorState = c.storyDirectorForMeta(storyCtx.Meta).ActorState
	}
	state := storyCtx.Snapshot.State
	c.mu.Lock()
	proposal, hasProposal := c.openingStateSchemaDraft.FinalProposal()
	draftRequired := c.openingStateSchemaDraft != nil
	c.mu.Unlock()
	if draftRequired && !hasProposal {
		return interactive.StoryDirectorActorStateSystem{}, nil, fmt.Errorf("请先调用 initialize_story_state_schema 并完成 finalize，再提交开局状态")
	}
	if hasProposal {
		if storyCtx.Meta.ActorStateSchema == nil {
			return interactive.StoryDirectorActorStateSystem{}, nil, fmt.Errorf("故事缺少开局状态结构基线")
		}
		target, _, err := interactive.ApplyActorStateSchemaAdaptation(storyCtx.Meta.ActorStateSchema.System, storyCtx.Meta.ActorStateSchema.TRPGSystem, proposal.Adaptation)
		if err != nil {
			return interactive.StoryDirectorActorStateSystem{}, nil, err
		}
		initialState, err := interactive.BuildActorStateInitialSnapshot(target, storyCtx.Meta.InitialTraitRolls)
		if err != nil {
			return interactive.StoryDirectorActorStateSystem{}, nil, err
		}
		return target, initialState, nil
	}
	if interactive.StoryStateSchemaPolicyUsesOpeningGameAgent(storyCtx.Meta.StateSchemaPolicy) {
		rawActors, _ := state["actors"].(map[string]any)
		if len(rawActors) == 0 {
			initialState, err := interactive.BuildActorStateInitialSnapshot(actorState, storyCtx.Meta.InitialTraitRolls)
			if err != nil {
				return interactive.StoryDirectorActorStateSystem{}, nil, err
			}
			state = initialState
		}
	}
	return actorState, state, nil
}

func (c *interactiveConversation) directorTaskHint() string {
	if c == nil {
		return ""
	}
	switch strings.TrimSpace(c.directorTask) {
	case interactiveDirectorTaskOpeningPlan:
		return "opening_plan：在首个 Game Agent 回合前建立 director.md、agent-brief.md 与 lore-context.md；基于开局设定和资料名称目录完成初始选角、场景与分支规划。"
	case "director_plan_update":
		return "director_plan_update：Game Agent 已提示本回合对后续规划有实质影响；判断 keep、patch 或 replan。普通更新默认只 Patch agent-brief.md，只有重大偏差才修改 director.md，只有资料工作集变化才修改 lore-context.md。"
	default:
		return "director_plan_update：观察已提交事实并判断 keep、patch 或 replan；只 Patch 实际变化的导演 Markdown 文件，不得改写历史 Turn 或 Actor State。"
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
	storyDirector := storyDirectorForSnapshot(c.storyDirectorForMeta(storyCtx.Meta), storyCtx.Meta.ActorStateSchema)
	tellerTurnContextPrompt := teller.PromptForTargets("turn_context")
	turnHistory := buildInteractiveModelVisibleTurnHistory(storyCtx.Snapshot.Turns, storyCtx.Snapshot.ContextCompaction)
	checkpointSummary := ""
	if storyCtx.Snapshot.ContextCompaction != nil {
		checkpointSummary = strings.TrimSpace(storyCtx.Snapshot.ContextCompaction.Summary)
	}
	directorPlanVisible := ""
	directorPlan := interactive.DirectorPlan{}
	if storyCtx.Snapshot.DirectorPlan != nil {
		directorPlan = *storyCtx.Snapshot.DirectorPlan
		directorPlanVisible = interactive.DirectorPlanVisibleContext(directorPlan, interactiveStoryRuntimeContextBytes)
	}
	loreRuntime, err := buildInteractiveStoryLoreContext(c.workspace, directorPlan, agentMessage)
	if err != nil {
		return nil, err
	}
	loreStore := book.NewLoreStore(c.workspace)
	residentLore, err := loreStore.ResidentContextMarkdown()
	if err != nil {
		return nil, fmt.Errorf("读取常驻资料失败: %w", err)
	}
	residentContentBytes, err := loreStore.ResidentContentBytes()
	if err != nil {
		return nil, fmt.Errorf("读取常驻资料预算失败: %w", err)
	}
	if residentContentBytes > book.ResidentLoreSafetyMaxBytes {
		return nil, fmt.Errorf("常驻资料正文异常过大（%d KB）；请检查是否误将大型文件设为常驻资料", (residentContentBytes+1023)/1024)
	}
	if len([]byte(residentLore)) > interactiveResidentLoreMessageMaxBytes {
		return nil, fmt.Errorf("常驻资料模型上下文过大: %d > %d bytes", len([]byte(residentLore)), interactiveResidentLoreMessageMaxBytes)
	}
	loreRevision, err := loreStore.Revision()
	if err != nil {
		return nil, fmt.Errorf("读取资料库 revision 失败: %w", err)
	}
	ruleSummary := interactive.StoryDirectorRuleSummary(storyDirector, interactiveStoryRuntimeContextBytes)
	actorStateRuntime := interactive.ActorStateRuntimeContext(storyDirector.ActorState, storyCtx.Snapshot.State, interactiveStoryRuntimeContextBytes, storyCtx.Meta.ChoiceCount)
	stateSchemaInitialization := interactive.OpeningGameStateSchemaInstruction(storyCtx.Meta)
	strategyPrompt := interactive.StoryDirectorStrategyPromptMarkdown(storyDirector)
	runtimeContext := prompts.InteractiveStoryRuntimeContext(prompts.InteractiveStoryPromptInput{
		Title:                       storyCtx.Meta.Title,
		Origin:                      storyCtx.Meta.Origin,
		StoryTellerID:               storyCtx.Meta.StoryTellerID,
		StoryDirectorID:             storyCtx.Meta.StoryDirectorID,
		BranchID:                    storyCtx.Snapshot.BranchID,
		ReplyTargetChars:            c.replyTargetChars,
		ChoiceCount:                 storyCtx.Meta.ChoiceCount,
		DirectorPlanVisible:         directorPlanVisible,
		StoryDirectorRules:          ruleSummary,
		ActorState:                  actorStateRuntime,
		StateSchemaInitialization:   stateSchemaInitialization,
		StoryDirectorStrategyPrompt: strategyPrompt,
		PreviousTurnsSummary:        turnHistory.PreviousSummary,
		LoreContext:                 loreRuntime,
	})
	history := make([]*schema.Message, 0, len(turnHistory.Turns)*2+4)
	stableLeadingMessage := ""
	if residentLore != "" {
		stableLeadingMessage = agentcontext.StandaloneMessage("常驻资料库", residentLore, "source: enabled resident lore; stable leading context")
		if len([]byte(stableLeadingMessage)) > interactiveResidentLoreMessageMaxBytes {
			return nil, fmt.Errorf("常驻资料最终模型消息过大: %d > %d bytes", len([]byte(stableLeadingMessage)), interactiveResidentLoreMessageMaxBytes)
		}
		history = append(history, schema.UserMessage(stableLeadingMessage))
	}
	if storyCtx.Snapshot.ContextCompaction != nil && strings.TrimSpace(storyCtx.Snapshot.ContextCompaction.Summary) != "" {
		history = append(history, agent.NewContextCompactionSummaryMessage(storyCtx.Snapshot.ContextCompaction.Epoch, storyCtx.Snapshot.ContextCompaction.Summary))
	}
	for _, turn := range turnHistory.Turns {
		history = append(history, schema.UserMessage(turn.User))
		history = append(history, schemaMessagesFromInteractiveContext(turn.ModelContextMessages)...)
		history = append(history, schema.AssistantMessage(turn.Narrative, nil))
	}
	history = agent.ApplyToolResultContextPolicyForConversation(history, c.ToolResultContextPolicy())
	history = append(history, schema.UserMessage(prompts.InteractiveStoryTurnInstruction(agentMessage, tellerTurnContextPrompt, runtimeContext)))
	sourceParts := interactiveStoryContextSources(storyCtx.Meta.Title, storyCtx.Meta.Origin, teller, checkpointSummary, directorPlanVisible, residentLore, loreRevision, loreRuntime, ruleSummary, actorStateRuntime, stateSchemaInitialization, strategyPrompt, turnHistory, agentMessage)
	sourceSummary := interactiveContextSourceListSummary(sourceParts)
	contextLedgerParts := interactiveContextLedgerParts(sourceParts, history, c.ToolResultContextPolicy())
	c.mu.Lock()
	c.lastSources = sourceSummary
	c.lastContextSources = cloneInteractiveContextSources(sourceParts)
	c.lastContextLedgerParts = contextLedgerParts
	c.stableLeadingMessage = stableLeadingMessage
	c.mu.Unlock()
	log.Printf(
		"[interactive-agent] context composition story_id=%s branch_id=%s story_title=%s origin=%s teller_id=%s story_director_id=%s teller_slots=%s teller_turn_context=%s history_checkpoint=%s director_plan=%s turns=%d model_turns=%d history=%s turn_instruction=%s sources=%s",
		c.storyID,
		storyCtx.Snapshot.BranchID,
		interactivePartSummary(storyCtx.Meta.Title),
		interactivePartSummary(storyCtx.Meta.Origin),
		storyCtx.Meta.StoryTellerID,
		storyCtx.Meta.StoryDirectorID,
		interactiveTellerSlotSummary(teller, "turn_context"),
		interactivePartSummary(tellerTurnContextPrompt),
		interactivePartSummary(checkpointSummary),
		interactivePartSummary(directorPlanVisible),
		len(storyCtx.Snapshot.Turns),
		len(turnHistory.Turns),
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

func (c *interactiveConversation) ContextLedgerParts() []agent.ContextLedgerPart {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]agent.ContextLedgerPart(nil), c.lastContextLedgerParts...)
}

func (c *interactiveConversation) ContextLedgerPartsForMessages(messages []*schema.Message) []agent.ContextLedgerPart {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	sources := cloneInteractiveContextSources(c.lastContextSources)
	c.mu.Unlock()
	parts := interactiveContextLedgerParts(sources, messages, c.ToolResultContextPolicy())
	c.mu.Lock()
	c.lastContextLedgerParts = append([]agent.ContextLedgerPart(nil), parts...)
	c.mu.Unlock()
	return parts
}

func (c *interactiveConversation) RunTraceMetadata() agent.RunTraceMetadata {
	if c == nil {
		return agent.RunTraceMetadata{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	metadata := agent.RunTraceMetadata{
		StoryID:         c.storyID,
		BranchID:        c.branchID,
		MaintenanceTask: c.directorTask,
	}
	if c.lastTurn != nil {
		metadata.BranchID = c.lastTurn.BranchID
		metadata.TurnID = c.lastTurn.ID
	}
	return metadata
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
	actorState, currentState, err := c.effectiveTurnState(storyCtx)
	if err != nil {
		return interactive.RuleResolution{}, err
	}
	storyDirector := storyDirectorForSnapshot(c.storyDirectorForMeta(storyCtx.Meta), storyCtx.Meta.ActorStateSchema)
	storyDirector.ActorState = actorState
	resolution, err := interactive.ResolveTurnRulesWithDirector(c.storyID, storyCtx.Snapshot.BranchID, currentState, storyDirector, request)
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
func (c *interactiveConversation) SubmitTurnResult(ctx context.Context, input interactive.TurnSubmissionInput) (interactive.TurnSubmissionReceipt, error) {
	if c == nil || c.store == nil {
		return interactive.TurnSubmissionReceipt{}, fmt.Errorf("互动故事不存在")
	}
	select {
	case <-ctx.Done():
		return interactive.TurnSubmissionReceipt{}, ctx.Err()
	default:
	}
	if c.InteractiveNarrativeReady() {
		log.Printf("[interactive-agent] ignored duplicate turn result before validation story_id=%s branch_id=%s", c.storyID, c.branchID)
		return interactiveTurnResultAlreadyAcceptedReceipt(), nil
	}
	storyCtx, err := c.store.StoryContext(c.storyID, c.branchID)
	if err != nil {
		return interactive.TurnSubmissionReceipt{}, err
	}
	actorState, currentState, err := c.effectiveTurnState(storyCtx)
	if err != nil {
		return interactive.TurnSubmissionReceipt{}, err
	}
	director := c.storyDirectorForMeta(storyCtx.Meta)
	c.mu.Lock()
	current := c.turnProtocol.draft()
	prepared, receipt := interactive.PrepareTurnSubmission(interactive.TurnSubmissionContext{
		ActorState:                  actorState,
		CurrentState:                currentState,
		ChoiceCount:                 storyCtx.Meta.ChoiceCount,
		RuleResolution:              c.ruleResolution,
		RuleStateConsumptionMode:    director.Strategy.RuleStateConsumptionMode,
		RequireCompleteInitialState: c.openingStateSchemaDraft != nil && len(storyCtx.Snapshot.Turns) == 0,
	}, current, input)
	staged := c.turnProtocol.update(prepared)
	c.mu.Unlock()
	if !staged {
		receipt = interactiveTurnResultAlreadyAcceptedReceipt()
		log.Printf("[interactive-agent] ignored turn result update after protocol lock story_id=%s branch_id=%s", c.storyID, c.branchID)
		return receipt, nil
	}
	stagedResult := prepared.TurnResult()
	log.Printf("[interactive-agent] updated turn result draft story_id=%s branch_id=%s ready=%t state_updates=%d choices=%d state_changes_status=%s choices_status=%s diagnostics=%q", c.storyID, c.branchID, receipt.Ready, len(stagedResult.StateUpdates), len(stagedResult.Choices), receipt.ModuleStatus.StateChanges, receipt.ModuleStatus.Choices, interactiveTurnSubmissionDiagnosticSummary(receipt.Diagnostics))
	return receipt, nil
}

func (c *interactiveConversation) InteractiveNarrativeReady() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.turnProtocol.narrativeReady()
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
	source, existingCheckpoint := interactiveCompactionSource(storyCtx.Snapshot.Turns, storyCtx.Snapshot.ContextCompaction)
	source = agent.ApplyToolResultContextPolicyForConversation(source, c.ToolResultContextPolicy())
	epoch := 1
	if storyCtx.Snapshot.ContextCompaction != nil {
		epoch = storyCtx.Snapshot.ContextCompaction.Epoch + 1
	}
	input.SourceMessages = source
	if strings.TrimSpace(input.ExistingCheckpoint) == "" {
		input.ExistingCheckpoint = existingCheckpoint
	}
	input.KeepLatestUser = true
	stableLeadingMessage := c.stableLeadingMessageSnapshot()
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
	newMessages = preserveInteractiveStableLeadingMessage(newMessages, stableLeadingMessage)
	result = interactiveCompactionResultForMessages(result, newMessages, input.Tools)
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
		newMessages = preserveInteractiveStableLeadingMessage(newMessages, stableLeadingMessage)
		result = interactiveCompactionResultForMessages(result, newMessages, input.Tools)
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
	existingCheckpoint := ""
	if compaction != nil && strings.TrimSpace(compaction.Summary) != "" {
		existingCheckpoint = compaction.Summary
		sourceStart = compaction.SourceTurnCount
		if sourceStart < 0 {
			sourceStart = 0
		}
		if sourceStart > len(turns) {
			sourceStart = len(turns)
		}
	}
	return interactiveCompactionTurnMessages(turns[sourceStart:]), existingCheckpoint
}

func interactiveCompactionTurnMessages(turns []interactive.TurnEvent) []*schema.Message {
	messages := make([]*schema.Message, 0, len(turns)*2)
	for _, turn := range turns {
		source := fmt.Sprintf("[source turn_id=%s branch_id=%s]", turn.ID, turn.BranchID)
		if strings.TrimSpace(turn.User) != "" {
			messages = append(messages, schema.UserMessage(source+"\n"+turn.User))
		}
		messages = append(messages, schemaMessagesFromInteractiveContext(turn.ModelContextMessages)...)
		if strings.TrimSpace(turn.Narrative) != "" {
			messages = append(messages, schema.AssistantMessage(source+"\n"+turn.Narrative, nil))
		}
	}
	return messages
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
		return fmt.Errorf("互动回合的 state_changes 或 choices 尚未完整提交，已拒绝写入不完整状态")
	}
	turn, _, err := c.store.AppendTurnWithState(c.storyID, interactive.AppendTurnWithStateRequest{
		BranchID:             c.branchID,
		ExpectedParentID:     c.baseParentIDSnapshot(),
		User:                 c.user,
		Narrative:            narrative,
		Thinking:             thinking,
		RunID:                assistantMetadata.RunID,
		AgentKind:            assistantMetadata.AgentKind,
		DisplayEvents:        withInteractiveNarrativeAnchor(c.displayEventsSnapshot()),
		ModelContextMessages: c.modelContextMessagesSnapshot(),
		RuleResolution:       c.ruleResolutionSnapshot(),
		TurnResult:           turnResult,
		TerminalOutcome:      c.terminalOutcomeSnapshot(narrative),
		StateSchemaProposal:  c.openingStateSchemaProposalSnapshot(),
	})
	if err == nil {
		c.mu.Lock()
		c.lastTurn = &turn
		c.lastStateReady = turn.StateStatus == "ready"
		c.turnProtocol.markCommitted()
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
	return c.turnProtocol.turnResult()
}

func interactiveTurnSubmissionDiagnosticSummary(diagnostics []interactive.TurnSubmissionDiagnostic) string {
	parts := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		parts = append(parts, strings.Join([]string{diagnostic.Module, diagnostic.Code, diagnostic.Path, diagnostic.MessageZH}, ":"))
	}
	return strings.Join(parts, "; ")
}

func interactiveTurnResultAlreadyAcceptedReceipt() interactive.TurnSubmissionReceipt {
	return interactive.TurnSubmissionReceipt{
		Ready: true,
		ModuleStatus: interactive.TurnSubmissionModuleStatus{
			StateChanges: interactive.TurnSubmissionModuleAccepted,
			Choices:      interactive.TurnSubmissionModuleAccepted,
		},
		Diagnostics: []interactive.TurnSubmissionDiagnostic{{
			Module:    "submission",
			Code:      "turn_result_already_accepted",
			Severity:  "warning",
			Retryable: false,
			MessageZH: "本回合已有完整 TurnResult，已保留首次接受的模块；无需重试。",
			MessageEN: "This turn already has a complete TurnResult; the first accepted modules were retained.",
		}},
	}
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
	_, instruction, err := c.buildDirectorModelInput(turn)
	return instruction, err
}

func (c *interactiveConversation) buildDirectorModelInput(turn interactive.TurnEvent) (interactiveDirectorStableContext, string, error) {
	stableContext, err := buildInteractiveDirectorStableContext(c.workspace)
	if err != nil {
		return interactiveDirectorStableContext{}, "", err
	}
	instruction, err := c.buildDirectorInstruction(turn, stableContext)
	if err != nil {
		return interactiveDirectorStableContext{}, "", err
	}
	assembledRevision, err := book.NewLoreStore(c.workspace).Revision()
	if err != nil {
		return interactiveDirectorStableContext{}, "", fmt.Errorf("读取导演资料库装配后 revision 失败: %w", err)
	}
	if strings.TrimSpace(assembledRevision) != strings.TrimSpace(stableContext.Revision) {
		return interactiveDirectorStableContext{}, "", fmt.Errorf("资料库在导演上下文装配期间发生变化: stable=%s dynamic=%s", strings.TrimSpace(stableContext.Revision), strings.TrimSpace(assembledRevision))
	}
	return stableContext, instruction, nil
}

func (c *interactiveConversation) buildDirectorInstruction(turn interactive.TurnEvent, stableContext interactiveDirectorStableContext) (string, error) {
	if c == nil || c.store == nil {
		return "", fmt.Errorf("互动故事不存在")
	}
	storyCtx, err := c.store.StoryContext(c.storyID, c.branchID)
	if err != nil {
		return "", err
	}
	storyDirector := storyDirectorForSnapshot(c.storyDirectorForMeta(storyCtx.Meta), storyCtx.Meta.ActorStateSchema)
	strategyPrompt := interactive.StoryDirectorStrategyPromptMarkdown(storyDirector)
	visibleHistory := buildInteractiveModelVisibleTurnHistory(storyCtx.Snapshot.Turns, storyCtx.Snapshot.ContextCompaction)
	historyText := formatInteractiveTurnHistoryWithCheckpoint(visibleHistory, storyCtx.Snapshot.ContextCompaction, "（暂无历史回合，请基于本回合审计更新导演计划。）")
	directorPlan := interactive.DirectorPlan{}
	if storyCtx.Snapshot.DirectorPlan != nil {
		directorPlan = *storyCtx.Snapshot.DirectorPlan
	} else if plan, err := c.store.DirectorPlan(c.storyID, storyCtx.Snapshot.BranchID); err == nil {
		directorPlan = plan
	}
	loreContext, err := buildInteractiveDirectorLoreContext(c.workspace, directorPlan, turn)
	if err != nil {
		return "", err
	}
	actorStateSnapshot := map[string]any{}
	if actors, ok := storyCtx.Snapshot.State["actors"]; ok {
		actorStateSnapshot = map[string]any{"actors": actors}
	}
	openingInitialization := strings.TrimSpace(c.directorTask) == interactiveDirectorTaskOpeningPlan
	budget := newDirectorContextBudget(c.cfg, c.directorTask, stableContext)
	title := budget.take("story.title", storyCtx.Meta.Title, 512)
	turnAudit := ""
	if !openingInitialization {
		turnAudit = budget.take("turn.audit", boundedJSON(interactiveDirectorTurnAudit(turn), interactiveDirectorContextBytes), interactiveDirectorContextBytes)
	}
	planDocsMarkdown := formatDirectorDocumentsContext(directorPlan.Docs, directorPlan.Metadata.Docs)
	planDocs := budget.take("director_plan.docs", planDocsMarkdown, interactiveDirectorContextBytes)
	actorState := budget.take("actor_state.snapshot", boundedJSON(actorStateSnapshot, interactiveDirectorContextBytes), interactiveDirectorContextBytes)
	actorStateSchema := budget.take("actor_state.schema", interactive.ActorStateSchemaContext(storyDirector.ActorState, interactiveDirectorContextBytes), interactiveDirectorContextBytes)
	lore := budget.take("lore.relevant", loreContext, interactiveDirectorContextBytes)
	history := budget.take("turn.history", historyText, interactiveDirectorContextBytes)
	origin := budget.take("story.origin", storyCtx.Meta.Origin, interactiveDirectorContextBytes)
	planningTemplates := budget.take("director.strategy.templates", boundedJSON(storyDirector.Strategy.PlanningTemplates, interactiveDirectorContextBytes), interactiveDirectorContextBytes)
	planningSummary := budget.take("director.planning_summary", interactive.StoryDirectorPlanningSummary(storyDirector, interactiveDirectorContextBytes), interactiveDirectorContextBytes)
	strategyContext := budget.take("director.strategy.prompt", strategyPrompt, interactiveDirectorContextBytes)
	openingContext := ""
	if openingInitialization {
		openingContext = budget.take("story.opening_input", turn.User, 4*1024)
	}
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
		OpeningContext:              openingContext,
		OpeningInitialization:       openingInitialization,
		StoryTellerID:               budget.take("story.teller_id", storyCtx.Meta.StoryTellerID, 128),
		StoryDirectorID:             budget.take("story.director_id", storyCtx.Meta.StoryDirectorID, 128),
		BranchID:                    budget.take("story.branch_id", storyCtx.Snapshot.BranchID, 128),
		TaskHint:                    budget.take("director.task", c.directorTaskHint(), 1024),
		DirectorPlanDocs:            planDocs,
		PlanningTemplates:           planningTemplates,
		BranchPlanningTurns:         storyDirector.Strategy.BranchPlanningTurns,
		LoreContext:                 lore,
		TurnAuditJSON:               turnAudit,
		TurnHistory:                 history,
		ActorStateSchema:            actorStateSchema,
		ActorState:                  actorState,
		StoryDirectorPlan:           planningSummary,
		StoryDirectorStrategyPrompt: strategyContext,
		DirectorEventCatalog:        eventCatalog,
		EventOpportunity:            budget.take("director.event_opportunity", boundedJSON(eventOpportunity, 4*1024), 4*1024),
		EventRuntime:                budget.take("director.event_runtime", boundedJSON(eventRuntime, 8*1024), 8*1024),
	})
	log.Printf("[interactive-director-agent] context budget story_id=%s branch_id=%s turn_id=%s instruction_bytes=%d stable_bytes=%d model_window_tokens=%d threshold_tokens=%d source_budget_tokens=%d fragments=%s", c.storyID, storyCtx.Snapshot.BranchID, turn.ID, len(instruction), len([]byte(stableContext.Content)), budget.contextWindowTokens, budget.thresholdTokens, budget.initialTokens, budget.trace())
	log.Printf(
		"[interactive-director-agent] context composition story_id=%s branch_id=%s turn_id=%s teller_id=%s story_director_id=%s director_plan=%s lore=%s turn_audit=%s actor_state=%s history=%s instruction=%s",
		c.storyID,
		storyCtx.Snapshot.BranchID,
		turn.ID,
		storyCtx.Meta.StoryTellerID,
		storyCtx.Meta.StoryDirectorID,
		interactivePartSummary(planDocsMarkdown),
		interactivePartSummary(loreContext),
		interactivePartSummary(turnAudit),
		interactivePartSummary(boundedJSON(actorStateSnapshot, interactiveDirectorContextBytes)),
		interactivePartSummary(historyText),
		interactivePartSummary(instruction),
	)
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

func newDirectorContextBudget(cfg *config.Config, task string, stableContext interactiveDirectorStableContext) *directorContextBudget {
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
	if task == interactiveDirectorTaskOpeningPlan {
		emptyPrompt = prompts.InteractiveDirectorInstruction(prompts.InteractiveDirectorPromptInput{OpeningInitialization: true})
	}
	customPrompt := config.ResolveAgentPrompt(cfg, config.AgentKindInteractiveDirector).SystemPrompt
	overheadMessages := []*schema.Message{
		schema.SystemMessage(systemPrompt + "\n" + customPrompt),
		schema.UserMessage(emptyPrompt),
	}
	if stable := strings.TrimSpace(stableContext.Content); stable != "" {
		title := strings.TrimSpace(stableContext.Title)
		if title == "" {
			title = "稳定模型上下文"
		}
		overheadMessages = append(overheadMessages, schema.UserMessage(fmt.Sprintf("# %s\n\n%s", title, stable)))
	}
	overheadTokens := agent.EstimateContextTokens(overheadMessages, nil)
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

func (c *interactiveConversation) storyDirectorForMeta(meta interactive.StoryMeta) interactive.StoryDirector {
	return loadStoryDirectorForMeta(c.novaDir, meta)
}

func loadStoryDirectorForMeta(novaDir string, meta interactive.StoryMeta) interactive.StoryDirector {
	director := loadStoryDirector(novaDir, meta.StoryDirectorID)
	if meta.ModuleRefs == nil {
		return director
	}
	director.ModuleRefs = interactive.NormalizeStoryDirectorModuleRefs(*meta.ModuleRefs)
	director.ResolvedSnapshot = interactive.StoryDirectorResolvedSnapshot{}
	return interactive.ResolveStoryDirectorModules(novaDir, director)
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

type interactiveTurnHistory struct {
	PreviousSummary string
	Turns           []interactive.TurnEvent
	PreviousCount   int
	OmittedCount    int
}

const (
	interactiveStoryRuntimeContextBytes = interactive.DirectorContextMaxBytes
	interactiveDirectorContextBytes     = interactive.DirectorContextMaxBytes
	// The raw resident bodies keep their 1 MiB safety ceiling. This additional
	// bounded allowance covers deterministic Lore metadata and the standalone
	// message wrapper while still constraining the exact model-visible fragment.
	interactiveResidentLoreMessageMaxBytes = book.ResidentLoreSafetyMaxBytes + interactive.DirectorContextMaxBytes
)

func buildInteractiveTurnHistory(turns []interactive.TurnEvent) interactiveTurnHistory {
	return interactiveTurnHistory{Turns: append([]interactive.TurnEvent(nil), turns...)}
}

func buildInteractiveModelVisibleTurnHistory(turns []interactive.TurnEvent, compaction *interactive.ContextCompactionEvent) interactiveTurnHistory {
	return buildInteractiveTurnHistoryWithCompaction(turns, compaction, retainedTurnsForInteractiveCompaction(compaction))
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

func buildInteractiveTurnHistoryWithCompaction(turns []interactive.TurnEvent, compaction *interactive.ContextCompactionEvent, retainedTurns int) interactiveTurnHistory {
	if compaction == nil || strings.TrimSpace(compaction.Summary) == "" {
		return buildInteractiveTurnHistory(turns)
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
	return interactiveTurnHistory{
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

func formatInteractiveTurnHistoryWithCheckpoint(turnHistory interactiveTurnHistory, compaction *interactive.ContextCompactionEvent, emptyMessage string) string {
	var sb strings.Builder
	if compaction != nil && strings.TrimSpace(compaction.Summary) != "" {
		sb.WriteString("[历史上下文检查点]\n")
		sb.WriteString(agent.NewContextCompactionSummaryMessage(compaction.Epoch, compaction.Summary).Content)
		sb.WriteString("\n\n")
	}
	if len(turnHistory.Turns) > 0 {
		sb.WriteString(formatInteractiveTurnHistory(turnHistory.Turns, emptyMessage))
	}
	result := strings.TrimSpace(sb.String())
	if result == "" {
		return emptyMessage
	}
	return result
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
