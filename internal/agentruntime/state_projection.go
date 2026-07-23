package agentruntime

import (
	"fmt"
	"sort"
)

func (s *harnessState) snapshot() StateSnapshot {
	tools := make([]ToolCallState, 0, len(s.openToolCalls))
	for _, call := range s.openToolCalls {
		call = normalizeToolCallState(call)
		tools = append(tools, call)
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].CallID < tools[j].CallID })
	return StateSnapshot{
		Binding: s.binding, Cursor: s.cursor, Phase: s.phase,
		ActiveOperation:  s.activeOperation,
		ActiveCycle:      s.activeCycle,
		RecoveryPaused:   s.recoveryPaused,
		InputRecovery:    cloneInputMaterializationRecovery(s.inputRecovery),
		ActiveStructural: cloneStructuralOperationSnapshot(s.activeStructural),
		ActiveOutput: ActiveOutputSnapshot{
			OperationID:       s.activeOperation,
			Cycle:             s.activeCycle,
			Content:           s.activeContent.String(),
			Thinking:          s.activeThinking.String(),
			RehydrateRequired: s.activeOutputRehydrated,
		},
		Messages:            displayMessages(s.messages),
		Queue:               displayQueue(s.queue),
		OpenToolCalls:       tools,
		PendingHostEffects:  s.pendingHostEffectSnapshots(),
		LastOperation:       cloneOperationSummary(s.lastOperation),
		RecentOperations:    cloneOperationSummaries(s.recentOperations),
		LastDomainCommit:    cloneDomainCommitState(s.lastDomainCommit),
		DomainCommits:       cloneDomainCommitStates(s.lastDomainCommits),
		TimelineStartCursor: s.timelineStartCursor(),
		MessagesTruncated:   s.messagesTruncated,
		Memory:              s.memorySnapshot(),
	}
}

func (s *harnessState) trimMessages() {
	for s.maxRetainedMessages > 0 && len(s.messages) > s.maxRetainedMessages {
		s.dropOldestMessage()
	}
}

func (s *harnessState) trimCommands() {
	if s.maxRetainedCommands <= 0 || len(s.commandOrder) <= s.maxRetainedCommands {
		return
	}
	s.dropOldestCommands(len(s.commandOrder) - s.maxRetainedCommands)
}

func (s *harnessState) recordTerminalOperation(summary OperationSummary) {
	cloned := cloneOperationSummary(&summary)
	s.lastOperation = cloned
	s.recentOperations = append(s.recentOperations, *cloned)
	if s.maxRetainedCommands > 0 && len(s.recentOperations) > s.maxRetainedCommands {
		drop := len(s.recentOperations) - s.maxRetainedCommands
		copy(s.recentOperations, s.recentOperations[drop:])
		s.recentOperations = s.recentOperations[:s.maxRetainedCommands]
	}
	s.recomputeRetainedCommandBytes()
}

func (s *harnessState) timelineStartCursor() Cursor {
	if len(s.events) == 0 {
		return s.cursor + 1
	}
	return s.events[0].Cursor
}

func (s *harnessState) cursorExpired(after Cursor) bool {
	if after == s.cursor {
		return false
	}
	start := s.timelineStartCursor()
	return start > 0 && after+1 < start
}

func (s *harnessState) statusSnapshot(maxTextBytes int) StatusSnapshot {
	if maxTextBytes <= 0 {
		maxTextBytes = 1 << 20
	}
	tools := make([]ToolCallState, 0, len(s.openToolCalls))
	for _, call := range s.openToolCalls {
		tools = append(tools, normalizeToolCallState(call))
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].CallID < tools[j].CallID })
	queue := cloneQueue(s.queue)
	for index := range queue {
		queue[index].Input.Text, queue[index].InputTextTruncated = boundUTF8WithTruncation(queue[index].Input.Text, maxTextBytes)
		queue[index].Input.ContextRefs = nil
		queue[index].Input.TurnSpecRef = ""
		queue[index].Input.RestoreDescriptor = nil
	}
	content, contentTruncated := boundUTF8WithTruncation(s.activeContent.String(), maxTextBytes)
	thinking, thinkingTruncated := boundUTF8WithTruncation(s.activeThinking.String(), maxTextBytes)
	activeFingerprint := ""
	activeReceiptCursor := Cursor(0)
	if accepted, ok := s.operationAcceptances[s.activeOperation]; ok {
		activeFingerprint = accepted.Fingerprint
		activeReceiptCursor = accepted.Receipt.Cursor
	}
	return StatusSnapshot{
		Binding: s.binding, Cursor: s.cursor, Phase: s.phase,
		ActiveCommandID: s.activeCommandID, ActiveCommandFingerprint: activeFingerprint,
		ActiveReceiptCursor: activeReceiptCursor, ActiveOperation: s.activeOperation, ActiveCycle: s.activeCycle,
		RecoveryPaused:   s.recoveryPaused,
		InputRecovery:    cloneInputMaterializationRecovery(s.inputRecovery),
		ActiveStructural: cloneStructuralOperationSnapshot(s.activeStructural),
		ActiveOutput: ActiveOutputSnapshot{
			OperationID: s.activeOperation, Cycle: s.activeCycle,
			Content: content, Thinking: thinking,
			ContentTruncated: contentTruncated, ThinkingTruncated: thinkingTruncated,
			RehydrateRequired: s.activeOutputRehydrated || contentTruncated || thinkingTruncated,
		},
		Queue: queue, OpenToolCalls: tools,
		PendingHostEffects: s.pendingHostEffectSnapshots(),
		LastOperation:      cloneOperationSummary(s.lastOperation),
		RecentOperations:   cloneOperationSummaries(s.recentOperations),
		LastDomainCommit:   cloneDomainCommitState(s.lastDomainCommit),
		DomainCommits:      cloneDomainCommitStates(s.lastDomainCommits),
		Memory:             s.memorySnapshot(),
	}
}

func (s *harnessState) pendingHostEffectSnapshots() []HostEffectSnapshot {
	if len(s.pendingHostEffectOrder) == 0 {
		return nil
	}
	snapshots := make([]HostEffectSnapshot, 0, len(s.pendingHostEffectOrder))
	for _, id := range s.pendingHostEffectOrder {
		if effect, ok := s.pendingHostEffects[id]; ok {
			snapshots = append(snapshots, hostEffectSnapshot(effect))
		}
	}
	return snapshots
}

func (s *harnessState) conservativeStoredStatus(maxTextBytes int) StatusSnapshot {
	status := s.statusSnapshot(maxTextBytes)
	if status.Phase == PhaseIdle {
		return status
	}
	status.RecoveryPending = true
	reason := "recovery_pending: durable journal contains an unfinished operation"
	operationStatus := OperationInterrupted
	if s.acknowledgedOutputCommit() != nil {
		reason = "recovery_pending: output domain commit is acknowledged and awaits durable settlement"
		operationStatus = OperationSucceeded
	}
	if pending := s.pendingDomainCommit(); pending != nil {
		reason = fmt.Sprintf("recovery_pending: %s domain commit intent has no receipt", pending.Identity.Stage)
		operationStatus = OperationInterrupted
	}
	status.Phase = PhaseIdle
	status.LastOperation = &OperationSummary{
		OperationID: s.activeOperation, CommandID: s.activeCommandID,
		CommandFingerprint: status.ActiveCommandFingerprint, ReceiptCursor: status.ActiveReceiptCursor,
		Status: operationStatus, Reason: reason,
	}
	status.ActiveOperation = ""
	status.ActiveCommandID = ""
	status.ActiveCycle = 0
	status.RecoveryPaused = false
	status.ActiveStructural = nil
	status.ActiveOutput = ActiveOutputSnapshot{}
	queue := status.Queue[:0]
	for _, item := range status.Queue {
		if item.Delivery == DeliveryNextTurn {
			queue = append(queue, item)
		}
	}
	status.Queue = queue
	if len(queue) > 0 {
		status.LastOperation.Reason += "; accepted NextTurn remains durable and retryable after recovery"
	}
	status.OpenToolCalls = nil
	return status
}
