package interactiveapp

import (
	"context"
	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/toolresult"
	"fmt"
	"log/slog"
	"strings"

	"denova/config"
	agents "denova/internal/agents"
	agentcompaction "denova/internal/agents/context/compaction"
	"denova/internal/agents/session"
	"denova/internal/interactive"
)

func (c *Conversation) PrepareInteractiveTurn(ctx context.Context, request interactive.TurnCheckRequest) (interactive.RuleResolution, error) {
	if c == nil || c.store == nil {
		return interactive.RuleResolution{}, fmt.Errorf("互动故事不存在")
	}
	storyCtx, err := c.storyContextForCycle()
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
	storyDirector := storyDirectorForSnapshot(c.StoryDirectorForMeta(storyCtx.Meta), storyCtx.Meta.ActorStateSchema)
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
func (c *Conversation) SubmitTurnResult(ctx context.Context, input interactive.TurnSubmissionInput) (interactive.TurnSubmissionReceipt, error) {
	if c == nil || c.store == nil {
		return interactive.TurnSubmissionReceipt{}, fmt.Errorf("互动故事不存在")
	}
	select {
	case <-ctx.Done():
		return interactive.TurnSubmissionReceipt{}, ctx.Err()
	default:
	}
	if c.InteractiveNarrativeReady() {
		slog.WarnContext(ctx, fmt.Sprintf("[interactive-agent] ignored duplicate turn result before validation story_id=%s branch_id=%s", c.storyID, c.branchID))
		return interactiveTurnResultAlreadyAcceptedReceipt(), nil
	}
	storyCtx, err := c.storyContextForCycle()
	if err != nil {
		return interactive.TurnSubmissionReceipt{}, err
	}
	actorState, currentState, err := c.effectiveTurnState(storyCtx)
	if err != nil {
		return interactive.TurnSubmissionReceipt{}, err
	}
	director := c.StoryDirectorForMeta(storyCtx.Meta)
	c.mu.Lock()
	current := c.turnProtocol.draft()
	prepared, receipt := interactive.PrepareTurnSubmission(interactive.TurnSubmissionContext{
		ActorState:                  actorState,
		CurrentState:                currentState,
		ChoiceCount:                 storyCtx.Meta.ChoiceCount,
		RuleResolution:              c.ruleResolution,
		RuleStateConsumptionMode:    director.Strategy.RuleStateConsumptionMode,
		RequireCompleteInitialState: c.openingStateSchemaDraft != nil && SnapshotTurnCount(storyCtx.Snapshot) == 0,
	}, current, input)
	staged := c.turnProtocol.update(prepared)
	c.mu.Unlock()
	if !staged {
		receipt = interactiveTurnResultAlreadyAcceptedReceipt()
		slog.WarnContext(ctx, fmt.Sprintf("[interactive-agent] ignored turn result update after protocol lock story_id=%s branch_id=%s", c.storyID, c.branchID))
		return receipt, nil
	}
	stagedResult := prepared.TurnResult()
	slog.InfoContext(ctx, fmt.Sprintf("[interactive-agent] updated turn result draft story_id=%s branch_id=%s ready=%t state_updates=%d choices=%d state_changes_status=%s choices_status=%s diagnostics=%q", c.storyID, c.branchID, receipt.Ready, len(stagedResult.StateUpdates), len(stagedResult.Choices), receipt.ModuleStatus.StateChanges, receipt.ModuleStatus.Choices, interactiveTurnSubmissionDiagnosticSummary(receipt.Diagnostics)))
	return receipt, nil
}

func (c *Conversation) InteractiveNarrativeReady() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.turnProtocol.narrativeReady()
}

func (c *Conversation) CompactContextIfNeeded(ctx context.Context, input agentcompaction.Input) ([]*agents.Message, agentcompaction.Result, error) {
	if c == nil || c.store == nil {
		return input.Messages, agentcompaction.Result{}, fmt.Errorf("互动故事不存在")
	}
	storyCtx, err := c.storyContextForCycle()
	if err != nil {
		return input.Messages, agentcompaction.Result{}, err
	}
	modelHistory, activeCompaction, err := c.modelHistoryForCycle(storyCtx)
	if err != nil {
		return input.Messages, agentcompaction.Result{}, err
	}
	if !input.Force && storyCtx.Snapshot.ContextCompactionRemoval != nil && storyCtx.Snapshot.ContextCompactionRemoval.SourceTurnCount >= modelHistory.EndTurn {
		return input.Messages, agentcompaction.Result{SkippedReason: "removed_same_source"}, nil
	}
	modelProjection, err := BuildModelContextProjection(
		modelHistory, activeCompaction, storyCtx.Snapshot, c.ToolResultContextPolicy(), c.AgentCycleIdentitySnapshot(),
	)
	if err != nil {
		return input.Messages, agentcompaction.Result{}, err
	}
	source := modelProjection.SourceMessages
	existingCheckpoint := modelProjection.ExistingCheckpoint
	input.CandidateFingerprint, input.CandidateGeneration = agentcompaction.CandidateIdentity(input.Messages, 0)
	healthRevision, health, hasHealth, err := c.store.ContextCompactionHealthState(
		c.storyID, storyCtx.Snapshot.BranchID, config.AgentKindInteractiveStory,
	)
	if err != nil {
		return input.Messages, agentcompaction.Result{}, err
	}
	structureFingerprint, err := c.contextCompactionStructureFingerprint(storyCtx, input)
	if err != nil {
		return input.Messages, agentcompaction.Result{}, err
	}
	maximumFailures := config.DefaultContextCompactionMaxConsecutiveFailures
	if hasHealth && (agentcompaction.FailureState{
		StructureFingerprint: health.StructureFingerprint,
		ConsecutiveFailures:  health.ConsecutiveFailures,
	}).Blocks(structureFingerprint, maximumFailures, input.Automatic) {
		input.PreflightSkipReason = "consecutive_failure_fuse"
		input.ConsecutiveFailures = health.ConsecutiveFailures
		input.FailureFuseOpen = true
	}
	if input.Automatic && strings.TrimSpace(input.PreflightSkipReason) == "" && activeCompaction != nil &&
		agentcompaction.NoProgressLatched(
			activeCompaction.TokensAfter, activeCompaction.ContextWindowTokens, activeCompaction.Threshold,
			config.DefaultContextCompactionRecoveryBand,
			agentcontext.EstimateTokens(source, nil),
			agentcontext.EffectiveToolResultCleanupMinimum(input.Messages, input.Tools, c.ContextPressurePolicy(input.Messages)),
			activeCompaction.CandidateFingerprint, activeCompaction.CandidateGeneration,
			input.CandidateFingerprint, input.CandidateGeneration,
		) {
		input.PreflightSkipReason = "degraded_no_progress_latch"
	}
	epoch := 1
	if activeCompaction != nil {
		epoch = activeCompaction.Epoch + 1
	}
	input.SourceMessages = source
	input.SourceMessagesSet = true
	if strings.TrimSpace(input.ExistingCheckpoint) == "" {
		input.ExistingCheckpoint = existingCheckpoint
	}
	input.KeepLatestUser = true
	stableLeadingMessage := c.stableLeadingMessageSnapshot()
	completionReserve, toolReserve := agentcompaction.EstimateProjectionReserves(c.cfg, config.AgentKindInteractiveStory, c.replyTargetChars)
	if input.ReservedCompletionTokens <= 0 {
		input.ReservedCompletionTokens = completionReserve
	}
	if input.ReservedToolResultTokens <= 0 {
		input.ReservedToolResultTokens = toolReserve
	}
	newMessages, result, err := agentcompaction.Prepare(ctx, c.cfg, config.AgentKindInteractiveStory, input, epoch)
	if err != nil {
		c.stageInteractiveCompactionHealth(healthRevision, storyCtx.Snapshot.BranchID, structureFingerprint, agentcompaction.HealthFailure, &result)
		return newMessages, result, err
	}
	if !result.Triggered {
		return newMessages, result, err
	}
	newMessages = PreserveStableLeadingMessage(newMessages, stableLeadingMessage)
	newMessages, result, err = ValidateCompactionProjection(input.Messages, newMessages, result, input.Tools)
	if err != nil {
		c.stageInteractiveCompactionHealth(healthRevision, storyCtx.Snapshot.BranchID, structureFingerprint, agentcompaction.HealthFailure, &result)
		return newMessages, result, err
	}
	c.stageInteractiveCompactionHealth(healthRevision, storyCtx.Snapshot.BranchID, structureFingerprint, agentcompaction.HealthSuccess, &result)
	if !input.Force && result.Phase == "model_step" {
		c.stagePreparedInteractiveCompaction(preparedInteractiveContextCompaction{
			Result: result, SourceTurnCount: modelProjection.SourceTurnCount,
		})
	}
	// Model-cycle compaction remains a transient projection. Canonical story
	// checkpoints are branch-head mutations and therefore run only through the
	// binding's durable CompactIfNeeded structural command after settlement or
	// on an explicit manual request.
	return newMessages, result, nil
}

// ValidateCompactionProjection is the final safety boundary after
// Game-specific stable context has been re-injected. A candidate that no
// longer shrinks the true provider-visible context must not replace the live
// model input or become a durable checkpoint.
func ValidateCompactionProjection(
	originalMessages []*agents.Message,
	compactedMessages []*agents.Message,
	result agentcompaction.Result,
	tools []*agents.ToolInfo,
) ([]*agents.Message, agentcompaction.Result, error) {
	normalized, err := agentcontext.NormalizeModelContextMessages(compactedMessages)
	if err != nil {
		result.Triggered = false
		result.SkippedReason = "protocol_invalid"
		return originalMessages, result, err
	}
	compactedMessages = normalized
	result.CandidateFingerprint, result.CandidateGeneration = agentcompaction.CandidateIdentity(compactedMessages, 0)
	result = interactiveCompactionResultForMessages(result, compactedMessages, tools)
	if result.RecoveryTargetTokens > 0 {
		result.RecoveryBandMet = result.TokensAfter <= result.RecoveryTargetTokens
		result.Degraded = !result.RecoveryBandMet && result.ContextWindowTokens > 0 &&
			result.TokensAfter < agentcompaction.PublishLimit(result.ContextWindowTokens, result.Threshold)
	}
	if err := agentcompaction.Validate(result); err != nil {
		result.Triggered = false
		result.SkippedReason = "no_progress"
		return originalMessages, result, err
	}
	return compactedMessages, result, nil
}

func interactiveContextMessageFromSchema(msg *agents.Message) (interactive.ModelContextMessage, bool) {
	if msg == nil {
		return interactive.ModelContextMessage{}, false
	}
	cloned := msg.Clone()
	switch cloned.Role {
	case agents.RoleAssistant:
		calls := interactiveToolCallsFromSchema(cloned.ToolCalls)
		if len(calls) == 0 {
			return interactive.ModelContextMessage{}, false
		}
		return interactive.ModelContextMessage{Role: string(agents.RoleAssistant), Content: cloned.Content, ToolCalls: calls}, true
	case agents.RoleTool:
		if strings.TrimSpace(cloned.ToolCallID) == "" && strings.TrimSpace(cloned.ToolName) == "" {
			return interactive.ModelContextMessage{}, false
		}
		return interactive.ModelContextMessage{
			Role:       string(agents.RoleTool),
			Content:    cloned.Content,
			Name:       cloned.Name,
			ToolCallID: cloned.ToolCallID,
			ToolName:   cloned.ToolName,
			ToolResult: cloned.ToolResult,
		}, true
	default:
		return interactive.ModelContextMessage{}, false
	}
}

func interactiveToolCallsFromSchema(calls []agents.ToolCall) []interactive.ModelContextToolCall {
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

func schemaMessagesFromInteractiveContext(messages []interactive.ModelContextMessage) []*agents.Message {
	if len(messages) == 0 {
		return nil
	}
	result := make([]*agents.Message, 0, len(messages))
	for _, msg := range messages {
		switch strings.TrimSpace(msg.Role) {
		case string(agents.RoleAssistant):
			calls := schemaToolCallsFromInteractive(msg.ToolCalls)
			if len(calls) > 0 {
				result = append(result, agents.AssistantMessage(msg.Content, calls))
			}
		case string(agents.RoleTool):
			if strings.TrimSpace(msg.ToolCallID) != "" || strings.TrimSpace(msg.ToolName) != "" {
				result = append(result, (&agents.Message{
					Role:       agents.RoleTool,
					Content:    msg.Content,
					Name:       msg.Name,
					ToolCallID: msg.ToolCallID,
					ToolName:   msg.ToolName,
					ToolResult: msg.ToolResult,
				}).Clone())
			}
		}
	}
	return result
}

func schemaToolCallsFromInteractive(calls []interactive.ModelContextToolCall) []agents.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	result := make([]agents.ToolCall, 0, len(calls))
	for _, call := range calls {
		if strings.TrimSpace(call.Function.Name) == "" {
			continue
		}
		result = append(result, agents.ToolCall{
			Index: call.Index,
			ID:    call.ID,
			Type:  call.Type,
			Function: agents.FunctionCall{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
			Extra: call.Extra,
		})
	}
	return result
}

func interactiveCompactionSource(turns []interactive.TurnEvent, compaction *interactive.ContextCompactionEvent) ([]*agents.Message, string) {
	return interactiveCompactionWindowSource(turns, 0, compaction)
}

func interactiveCompactionWindowSource(turns []interactive.TurnEvent, turnStart int, compaction *interactive.ContextCompactionEvent) ([]*agents.Message, string) {
	sourceStart := 0
	existingCheckpoint := ""
	if compaction != nil && strings.TrimSpace(compaction.Summary) != "" {
		existingCheckpoint = compaction.Summary
		sourceStart = compaction.SourceTurnCount - turnStart
		if sourceStart < 0 {
			sourceStart = 0
		}
		if sourceStart > len(turns) {
			sourceStart = len(turns)
		}
	}
	return interactiveCompactionTurnMessages(turns[sourceStart:]), existingCheckpoint
}

func interactiveCompactionTurnMessages(turns []interactive.TurnEvent) []*agents.Message {
	messages := make([]*agents.Message, 0, len(turns)*2)
	for _, turn := range turns {
		source := fmt.Sprintf("[source turn_id=%s branch_id=%s]", turn.ID, turn.BranchID)
		if strings.TrimSpace(turn.User) != "" {
			messages = append(messages, agents.UserMessage(source+"\n"+turn.User))
		}
		messages = append(messages, settledTurnToolContextMessages(turn.ModelContextMessages)...)
		if strings.TrimSpace(turn.Narrative) != "" {
			messages = append(messages, agents.AssistantMessage(source+"\n"+turn.Narrative, nil))
		}
	}
	return messages
}

func (c *Conversation) AppendAssistant(content string) error {
	return c.AppendAssistantWithThinking(content, "")
}

func (c *Conversation) AppendContextMessage(msg *agents.Message) error {
	return c.AppendContextMessages(msg)
}

func (c *Conversation) AppendContextMessages(messages ...*agents.Message) error {
	if c == nil || len(messages) == 0 {
		return nil
	}
	converted := make([]interactive.ModelContextMessage, 0, len(messages))
	for _, message := range messages {
		if next, ok := interactiveContextMessageFromSchema(message); ok {
			converted = append(converted, next)
		}
	}
	if len(converted) == 0 {
		return nil
	}
	c.modelContextAppendMu.Lock()
	defer c.modelContextAppendMu.Unlock()

	c.mu.Lock()
	identity := c.agentCycleIdentity
	ordinal := c.modelContextBatchOrdinal
	store := c.store
	storyID := c.storyID
	branchID := c.branchID
	c.mu.Unlock()
	commandID := strings.TrimSpace(string(identity.CommandID))
	operationID := strings.TrimSpace(string(identity.OperationID))
	hasIdentity := commandID != "" || operationID != "" || identity.Cycle != 0
	if hasIdentity && (commandID == "" || operationID == "" || identity.Cycle <= 0) {
		return fmt.Errorf("model context batch requires a complete durable cycle identity")
	}
	if !hasIdentity || store == nil {
		// Legacy/non-runtime callers have no durable cycle to attach to. Keep the
		// historical in-memory behavior for those isolated paths only.
		c.mu.Lock()
		c.modelContextMessages = append(c.modelContextMessages, interactive.CloneModelContextMessages(converted)...)
		c.mu.Unlock()
		return nil
	}
	if strings.TrimSpace(branchID) == "" {
		storyContext, err := store.StoryContext(storyID, "")
		if err != nil {
			return err
		}
		branchID = storyContext.Snapshot.BranchID
	}
	intents, err := interactive.NewModelContextBatchIntents(interactive.DomainCommitIdentity{
		CommandID: commandID, OperationID: operationID, Cycle: identity.Cycle,
	}, branchID, ordinal, converted)
	if err != nil {
		return err
	}
	for _, intent := range intents {
		receipt, err := store.AppendModelContextBatch(storyID, intent)
		if err != nil {
			return err
		}
		c.mu.Lock()
		c.modelContextMessages = append(c.modelContextMessages, interactive.CloneModelContextMessages(receipt.Event.Messages)...)
		c.modelContextBatchOrdinal = receipt.Event.BatchOrdinal + 1
		c.mu.Unlock()
	}
	return nil
}

func (c *Conversation) ToolResultContextPolicy() toolresult.ContextPolicy {
	return toolresult.ResolveContextPolicy(c.cfg, config.AgentKindInteractiveStory)
}

func (c *Conversation) AppendAssistantWithThinking(content, thinking string) error {
	return c.AppendAssistantWithMetadata(content, thinking, session.MessageMetadata{})
}

func (c *Conversation) AppendAssistantWithMetadata(content, thinking string, metadata session.MessageMetadata) error {
	if c == nil || c.store == nil {
		return fmt.Errorf("互动故事不存在")
	}
	if strings.TrimSpace(metadata.RunID) != "" {
		c.mu.Lock()
		c.assistantMetadata = metadata
		c.mu.Unlock()
	}
	slog.InfoContext(context.Background(), fmt.Sprintf("[interactive-agent] parse assistant output content story_id=%s branch_id=%s content=%q", c.storyID, c.branchID, content))
	narrative, parseErr := ParseAssistantOutput(content)
	if parseErr != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-agent] parse assistant output failed story_id=%s branch_id=%s err=%v content=%q", c.storyID, c.branchID, parseErr, content))
		return parseErr
	}
	slog.InfoContext(context.Background(), fmt.Sprintf("[interactive-agent] parse assistant output result story_id=%s branch_id=%s narrative=%q", c.storyID, c.branchID, narrative))
	assistantMetadata := c.assistantMetadataSnapshot()
	cycleIdentity := c.AgentCycleIdentitySnapshot()
	turnResult := c.turnResultSnapshot()
	if turnResult == nil {
		return fmt.Errorf("互动回合的 state_changes 或 choices 尚未完整提交，已拒绝写入不完整状态")
	}
	if strings.TrimSpace(string(cycleIdentity.CommandID)) == "" || strings.TrimSpace(string(cycleIdentity.OperationID)) == "" || cycleIdentity.Cycle <= 0 {
		return fmt.Errorf("互动回合输出缺少 durable cycle identity，已拒绝绕过 Agent 提交屏障")
	}
	request := interactive.AppendTurnWithStateRequest{
		BranchID:             c.branchID,
		ExpectedParentID:     c.baseParentIDSnapshot(),
		ReplaceTurnID:        c.regenerateTargetSnapshot(),
		User:                 c.user,
		Narrative:            narrative,
		Thinking:             thinking,
		RunID:                assistantMetadata.RunID,
		AgentKind:            assistantMetadata.AgentKind,
		AgentCommandID:       string(cycleIdentity.CommandID),
		AgentOperationID:     string(cycleIdentity.OperationID),
		AgentCycle:           cycleIdentity.Cycle,
		DisplayEvents:        withInteractiveNarrativeAnchor(c.DisplayEventsSnapshot()),
		ModelContextMessages: c.modelContextMessagesSnapshot(),
		RuleResolution:       c.ruleResolutionSnapshot(),
		TurnResult:           turnResult,
		TerminalOutcome:      c.terminalOutcomeSnapshot(narrative),
		StateSchemaProposal:  c.openingStateSchemaProposalSnapshot(),
	}
	intent, err := interactive.NewDomainCommitIntent(request)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pendingDomainCommit != nil && c.pendingDomainCommit.Hash != intent.Hash {
		return fmt.Errorf("互动回合同一 Agent cycle 生成了不同的提交内容")
	}
	c.pendingDomainCommit = &intent
	// The structured protocol is now immutable, but the story branch does not
	// advance until the durable actor authorizes the output stage.
	c.turnProtocol.markCommitted()
	return nil
}

func (c *Conversation) assistantMetadataSnapshot() session.MessageMetadata {
	c.mu.Lock()
	defer c.mu.Unlock()
	metadata := c.assistantMetadata
	metadata.RunPath = append([]string(nil), metadata.RunPath...)
	return metadata
}

func (c *Conversation) modelContextMessagesSnapshot() []interactive.ModelContextMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.modelContextMessages) == 0 {
		return nil
	}
	return interactive.CloneModelContextMessages(c.modelContextMessages)
}

func (c *Conversation) ruleResolutionSnapshot() *interactive.RuleResolution {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ruleResolution == nil {
		return nil
	}
	resolution := *c.ruleResolution
	return &resolution
}

func (c *Conversation) turnResultSnapshot() *interactive.TurnResult {
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

func (c *Conversation) baseParentIDSnapshot() *string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.baseParentID == nil {
		return nil
	}
	value := *c.baseParentID
	return &value
}

func (c *Conversation) terminalOutcomeSnapshot(narrative string) *interactive.TerminalOutcome {
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

func (c *Conversation) LastTurnForState() (interactive.TurnEvent, bool, bool) {
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
