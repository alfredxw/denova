package app

import (
	"context"
	"fmt"
	"log"
	"strings"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/agent/session"
	"denova/internal/interactive"
)

func (c *interactiveConversation) PrepareInteractiveTurn(ctx context.Context, request interactive.TurnCheckRequest) (interactive.RuleResolution, error) {
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
	storyCtx, err := c.storyContextForCycle()
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

func (c *interactiveConversation) CompactContextIfNeeded(ctx context.Context, input agent.ContextCompactionInput) ([]*agent.Message, agent.ContextCompactionResult, error) {
	if c == nil || c.store == nil {
		return input.Messages, agent.ContextCompactionResult{}, fmt.Errorf("互动故事不存在")
	}
	storyCtx, err := c.storyContextForCycle()
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
	newMessages, result, err := agent.PrepareContextCompaction(ctx, c.cfg, config.AgentKindInteractiveStory, input, epoch)
	if err != nil || !result.Triggered {
		return newMessages, result, err
	}
	newMessages = preserveInteractiveStableLeadingMessage(newMessages, stableLeadingMessage)
	result = interactiveCompactionResultForMessages(result, newMessages, input.Tools)
	if !input.Force && (result.Phase == "pre_run" || result.Phase == "mid_run") {
		c.stagePreparedInteractiveCompaction(preparedInteractiveContextCompaction{
			Result: result, SourceTurnCount: len(storyCtx.Snapshot.Turns),
		})
	}
	// Model-cycle compaction remains a transient projection. Canonical story
	// checkpoints are branch-head mutations and therefore run only through the
	// binding's durable CompactIfNeeded structural command after settlement or
	// on an explicit manual request.
	return newMessages, result, nil
}

func interactiveTurnMessages(turns []interactive.TurnEvent) []*agent.Message {
	messages := make([]*agent.Message, 0, len(turns)*2)
	for _, turn := range turns {
		if strings.TrimSpace(turn.User) != "" {
			messages = append(messages, agent.UserMessage(turn.User))
		}
		messages = append(messages, schemaMessagesFromInteractiveContext(turn.ModelContextMessages)...)
		if strings.TrimSpace(turn.Narrative) != "" {
			messages = append(messages, agent.AssistantMessage(turn.Narrative, nil))
		}
	}
	return messages
}

func interactiveContextMessageFromSchema(msg *agent.Message) (interactive.ModelContextMessage, bool) {
	if msg == nil {
		return interactive.ModelContextMessage{}, false
	}
	switch msg.Role {
	case agent.RoleAssistant:
		calls := interactiveToolCallsFromSchema(msg.ToolCalls)
		if len(calls) == 0 {
			return interactive.ModelContextMessage{}, false
		}
		return interactive.ModelContextMessage{Role: string(agent.RoleAssistant), ToolCalls: calls}, true
	case agent.RoleTool:
		if strings.TrimSpace(msg.ToolCallID) == "" && strings.TrimSpace(msg.ToolName) == "" {
			return interactive.ModelContextMessage{}, false
		}
		return interactive.ModelContextMessage{
			Role:       string(agent.RoleTool),
			Content:    msg.Content,
			Name:       msg.Name,
			ToolCallID: msg.ToolCallID,
			ToolName:   msg.ToolName,
		}, true
	default:
		return interactive.ModelContextMessage{}, false
	}
}

func interactiveToolCallsFromSchema(calls []agent.ToolCall) []interactive.ModelContextToolCall {
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

func schemaMessagesFromInteractiveContext(messages []interactive.ModelContextMessage) []*agent.Message {
	if len(messages) == 0 {
		return nil
	}
	result := make([]*agent.Message, 0, len(messages))
	for _, msg := range messages {
		switch strings.TrimSpace(msg.Role) {
		case string(agent.RoleAssistant):
			calls := schemaToolCallsFromInteractive(msg.ToolCalls)
			if len(calls) > 0 {
				result = append(result, agent.AssistantMessage("", calls))
			}
		case string(agent.RoleTool):
			if strings.TrimSpace(msg.ToolCallID) != "" || strings.TrimSpace(msg.ToolName) != "" {
				result = append(result, agent.ToolMessage(msg.Content, msg.ToolCallID, agent.WithToolName(msg.ToolName)))
			}
		}
	}
	return result
}

func schemaToolCallsFromInteractive(calls []interactive.ModelContextToolCall) []agent.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	result := make([]agent.ToolCall, 0, len(calls))
	for _, call := range calls {
		if strings.TrimSpace(call.Function.Name) == "" {
			continue
		}
		result = append(result, agent.ToolCall{
			Index: call.Index,
			ID:    call.ID,
			Type:  call.Type,
			Function: agent.FunctionCall{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
			Extra: call.Extra,
		})
	}
	return result
}

func interactiveCompactionSource(turns []interactive.TurnEvent, compaction *interactive.ContextCompactionEvent) ([]*agent.Message, string) {
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

func interactiveCompactionTurnMessages(turns []interactive.TurnEvent) []*agent.Message {
	messages := make([]*agent.Message, 0, len(turns)*2)
	for _, turn := range turns {
		source := fmt.Sprintf("[source turn_id=%s branch_id=%s]", turn.ID, turn.BranchID)
		if strings.TrimSpace(turn.User) != "" {
			messages = append(messages, agent.UserMessage(source+"\n"+turn.User))
		}
		messages = append(messages, schemaMessagesFromInteractiveContext(turn.ModelContextMessages)...)
		if strings.TrimSpace(turn.Narrative) != "" {
			messages = append(messages, agent.AssistantMessage(source+"\n"+turn.Narrative, nil))
		}
	}
	return messages
}

func (c *interactiveConversation) AppendAssistant(content string) error {
	return c.AppendAssistantWithThinking(content, "")
}

func (c *interactiveConversation) AppendContextMessage(msg *agent.Message) error {
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
	cycleIdentity := c.agentCycleIdentitySnapshot()
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
		DisplayEvents:        withInteractiveNarrativeAnchor(c.displayEventsSnapshot()),
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
