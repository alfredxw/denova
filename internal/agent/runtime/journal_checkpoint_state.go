package runtime

import "fmt"

const harnessCheckpointVersion = 1

// harnessCheckpoint is the reducer state needed to continue from Cursor. It
// intentionally excludes subscribers, control channels, and process-local
// engine completion. Every durable obligation and bounded active stream
// builder is included, including private queued descriptors and host-effect
// payloads.
type harnessCheckpoint struct {
	Version     int         `json:"version"`
	Binding     BindingRef  `json:"binding"`
	Cursor      Cursor      `json:"cursor"`
	Phase       Phase       `json:"phase"`
	OperationID OperationID `json:"operation_id,omitempty"`
	Cycle       int         `json:"cycle,omitempty"`
	SnapshotID  SnapshotID  `json:"snapshot_id,omitempty"`

	ActiveStructural      *StructuralOperationSnapshot  `json:"active_structural,omitempty"`
	RecoveryPaused        bool                          `json:"recovery_paused,omitempty"`
	InputRecovery         *InputMaterializationRecovery `json:"input_recovery,omitempty"`
	ActiveInput           UserInput                     `json:"active_input,omitempty"`
	ActiveContent         string                        `json:"active_content,omitempty"`
	ActiveThinking        string                        `json:"active_thinking,omitempty"`
	Messages              []Message                     `json:"messages,omitempty"`
	Queue                 []QueuedInput                 `json:"queue,omitempty"`
	OpenToolCalls         []ToolCallState               `json:"open_tool_calls,omitempty"`
	PendingEffects        []HostEffect                  `json:"pending_host_effects,omitempty"`
	RetainedEvents        []encodedEvent                `json:"retained_events,omitempty"`
	Commands              []checkpointCommand           `json:"hot_commands,omitempty"`
	OperationCommands     map[OperationID]CommandID     `json:"operation_commands,omitempty"`
	OperationAcceptances  map[OperationID]CommandRecord `json:"operation_acceptances,omitempty"`
	ActiveCommandID       CommandID                     `json:"active_command_id,omitempty"`
	ActiveCycleCommandID  CommandID                     `json:"active_cycle_command_id,omitempty"`
	PendingCycleCommandID CommandID                     `json:"pending_cycle_command_id,omitempty"`
	LastOperation         *OperationSummary             `json:"last_operation,omitempty"`
	RecentOperations      []OperationSummary            `json:"recent_operations,omitempty"`
	AbortReason           string                        `json:"abort_reason,omitempty"`
	AbortRequested        bool                          `json:"abort_requested,omitempty"`
	DomainCommits         []DomainCommitState           `json:"domain_commits,omitempty"`
	LastDomainCommits     []DomainCommitState           `json:"last_domain_commits,omitempty"`
	LastDomainCommit      *DomainCommitState            `json:"last_domain_commit,omitempty"`
	MessagesTruncated     bool                          `json:"messages_truncated,omitempty"`
}

type checkpointCommand struct {
	ID          CommandID   `json:"id"`
	OperationID OperationID `json:"operation_id"`
	Cursor      Cursor      `json:"cursor"`
	Fingerprint string      `json:"fingerprint"`
}

func (s *harnessState) checkpointSafe() bool {
	return s != nil && s.activeOutputError == nil && s.pendingEngineDone == nil
}

func (s *harnessState) checkpoint() (harnessCheckpoint, error) {
	if !s.checkpointSafe() {
		return harnessCheckpoint{}, fmt.Errorf("agent runtime state is not at a checkpoint-safe transaction boundary")
	}
	checkpoint := harnessCheckpoint{
		Version: harnessCheckpointVersion, Binding: s.binding, Cursor: s.cursor,
		Phase: s.phase, OperationID: s.activeOperation, Cycle: s.activeCycle, SnapshotID: s.activeSnapshotID,
		ActiveStructural: cloneStructuralOperationSnapshot(s.activeStructural),
		RecoveryPaused:   s.recoveryPaused, InputRecovery: cloneInputMaterializationRecovery(s.inputRecovery),
		ActiveInput: cloneUserInput(s.activeInput), ActiveContent: s.activeContent.String(), ActiveThinking: s.activeThinking.String(),
		Messages: cloneMessages(s.messages), Queue: cloneQueue(s.queue),
		ActiveCommandID: s.activeCommandID, ActiveCycleCommandID: s.activeCycleCommandID,
		PendingCycleCommandID: s.pendingCycleCommandID,
		LastOperation:         cloneOperationSummary(s.lastOperation), RecentOperations: cloneOperationSummaries(s.recentOperations),
		AbortReason: s.abortReason, AbortRequested: s.abortRequested,
		DomainCommits: cloneDomainCommitStates(s.domainCommits), LastDomainCommits: cloneDomainCommitStates(s.lastDomainCommits),
		LastDomainCommit: cloneDomainCommitState(s.lastDomainCommit), MessagesTruncated: s.messagesTruncated,
		OperationCommands:    cloneOperationCommands(s.operationCommands),
		OperationAcceptances: cloneOperationAcceptances(s.operationAcceptances),
	}
	for _, call := range s.activeToolCallsForCheckpoint() {
		checkpoint.OpenToolCalls = append(checkpoint.OpenToolCalls, call)
	}
	for _, id := range s.pendingHostEffectOrder {
		if effect, ok := s.pendingHostEffects[id]; ok {
			checkpoint.PendingEffects = append(checkpoint.PendingEffects, cloneHostEffect(effect))
		}
	}
	for _, event := range s.events {
		encoded, err := encodeDurableEvent(displayEventForRetention(event))
		if err != nil {
			return harnessCheckpoint{}, fmt.Errorf("encode retained checkpoint event at cursor %d: %w", event.Cursor, err)
		}
		checkpoint.RetainedEvents = append(checkpoint.RetainedEvents, encoded)
	}
	for _, commandID := range s.commandOrder {
		receipt, ok := s.receipts[commandID]
		if !ok {
			continue
		}
		checkpoint.Commands = append(checkpoint.Commands, checkpointCommand{
			ID: commandID, OperationID: receipt.OperationID, Cursor: receipt.Cursor,
			Fingerprint: s.fingerprints[commandID],
		})
	}
	return checkpoint, nil
}

func restoreHarnessCheckpoint(target *harnessState, checkpoint harnessCheckpoint) error {
	if target == nil {
		return fmt.Errorf("checkpoint restore target is nil")
	}
	if checkpoint.Version != harnessCheckpointVersion || checkpoint.Binding != target.binding || checkpoint.Cursor == 0 {
		return fmt.Errorf("checkpoint identity, version, or cursor is invalid")
	}
	if checkpoint.Phase != PhaseIdle && checkpoint.Phase != PhaseRunning && checkpoint.Phase != PhaseCompacting {
		return fmt.Errorf("checkpoint phase %q is invalid", checkpoint.Phase)
	}
	if (checkpoint.Phase == PhaseIdle) != (checkpoint.OperationID == "") {
		return fmt.Errorf("checkpoint active operation does not match phase")
	}

	retainedTimeline := target.retainTimeline
	retainedCommands := target.retainCommandIndex
	maxEvents, maxMessages, maxCommands := target.maxRetainedEvents, target.maxRetainedMessages, target.maxRetainedCommands
	memoryLimits := target.memoryLimits.normalized()
	restored := newHarnessState(checkpoint.Binding)
	restored.retainTimeline = retainedTimeline
	restored.retainCommandIndex = retainedCommands
	restored.maxRetainedEvents, restored.maxRetainedMessages, restored.maxRetainedCommands = maxEvents, maxMessages, maxCommands
	restored.memoryLimits = memoryLimits
	restored.cursor = checkpoint.Cursor
	restored.phase = checkpoint.Phase
	restored.activeOperation, restored.activeCycle, restored.activeSnapshotID = checkpoint.OperationID, checkpoint.Cycle, checkpoint.SnapshotID
	restored.activeStructural = cloneStructuralOperationSnapshot(checkpoint.ActiveStructural)
	restored.recoveryPaused = checkpoint.RecoveryPaused
	restored.inputRecovery = cloneInputMaterializationRecovery(checkpoint.InputRecovery)
	restored.activeInput = cloneUserInput(checkpoint.ActiveInput)
	if int64(len(checkpoint.ActiveContent)+len(checkpoint.ActiveThinking)) > memoryLimits.MaxActiveOutputBytes {
		return fmt.Errorf("checkpoint active output exceeds %d bytes", memoryLimits.MaxActiveOutputBytes)
	}
	restored.activeContent.WriteString(checkpoint.ActiveContent)
	restored.activeThinking.WriteString(checkpoint.ActiveThinking)
	restored.activeOutputRehydrated = checkpoint.ActiveContent != "" || checkpoint.ActiveThinking != ""
	restored.queue = cloneQueue(checkpoint.Queue)
	restored.activeCommandID = checkpoint.ActiveCommandID
	restored.activeCycleCommandID = checkpoint.ActiveCycleCommandID
	restored.pendingCycleCommandID = checkpoint.PendingCycleCommandID
	restored.lastOperation = cloneOperationSummary(checkpoint.LastOperation)
	restored.recentOperations = cloneOperationSummaries(checkpoint.RecentOperations)
	restored.abortReason, restored.abortRequested = checkpoint.AbortReason, checkpoint.AbortRequested
	restored.operationCommands = cloneOperationCommands(checkpoint.OperationCommands)
	restored.operationAcceptances = cloneOperationAcceptances(checkpoint.OperationAcceptances)
	restored.domainCommits = domainCommitStateMap(checkpoint.DomainCommits)
	restored.lastDomainCommits = domainCommitStateMap(checkpoint.LastDomainCommits)
	restored.lastDomainCommit = cloneDomainCommitState(checkpoint.LastDomainCommit)
	restored.messagesTruncated = checkpoint.MessagesTruncated
	for _, call := range checkpoint.OpenToolCalls {
		call = normalizeToolCallState(call)
		if call.CallID == "" || call.OperationID == "" || call.Cycle <= 0 {
			return fmt.Errorf("checkpoint contains an invalid open tool call")
		}
		if _, duplicate := restored.openToolCalls[call.CallID]; duplicate {
			return fmt.Errorf("checkpoint contains duplicate tool call %q", call.CallID)
		}
		restored.openToolCalls[call.CallID] = call
	}
	for _, effect := range checkpoint.PendingEffects {
		if err := validateHostEffect(checkpoint.Binding, effect, memoryLimits); err != nil {
			return fmt.Errorf("checkpoint host effect %q: %w", effect.ID, err)
		}
		if _, duplicate := restored.pendingHostEffects[effect.ID]; duplicate {
			return fmt.Errorf("checkpoint contains duplicate host effect %q", effect.ID)
		}
		restored.pendingHostEffects[effect.ID] = cloneHostEffect(effect)
		restored.pendingHostEffectOrder = append(restored.pendingHostEffectOrder, effect.ID)
	}
	if retainedTimeline {
		for _, message := range checkpoint.Messages {
			restored.retainMessage(message)
		}
		var previous Cursor
		for _, encoded := range checkpoint.RetainedEvents {
			event, err := decodeDurableEvent(encoded)
			if err != nil {
				return fmt.Errorf("decode retained checkpoint event: %w", err)
			}
			if event.Cursor <= previous || event.Cursor > checkpoint.Cursor {
				return fmt.Errorf("checkpoint retained event cursor %d is invalid", event.Cursor)
			}
			previous = event.Cursor
			restored.retainEvent(event)
		}
	}
	if retainedCommands {
		for _, command := range checkpoint.Commands {
			if command.ID == "" || command.OperationID == "" || command.Cursor == 0 || command.Cursor > checkpoint.Cursor {
				return fmt.Errorf("checkpoint contains an invalid hot command receipt")
			}
			if _, duplicate := restored.receipts[command.ID]; duplicate {
				return fmt.Errorf("checkpoint contains duplicate hot command %q", command.ID)
			}
			restored.receipts[command.ID] = Receipt{CommandID: command.ID, OperationID: command.OperationID, Cursor: command.Cursor}
			restored.fingerprints[command.ID] = command.Fingerprint
			restored.commandOrder = append(restored.commandOrder, command.ID)
		}
		restored.trimCommands()
	}
	restored.recomputeRetainedCommandBytes()
	restored.enforceRetainedByteBudget()
	if err := restored.validatePendingInputBudget(); err != nil {
		return fmt.Errorf("checkpoint pending input budget: %w", err)
	}
	if err := restored.validatePendingHostEffectBudget(); err != nil {
		return fmt.Errorf("checkpoint host effect budget: %w", err)
	}
	*target = restored
	return nil
}

func (s *harnessState) activeToolCallsForCheckpoint() []ToolCallState {
	calls := make([]ToolCallState, 0, len(s.openToolCalls))
	for _, call := range s.openToolCalls {
		calls = append(calls, normalizeToolCallState(call))
	}
	// activeToolCalls already gives stable ordering and all durable open calls
	// belong to the one active operation.
	return sortToolCalls(calls)
}

func sortToolCalls(calls []ToolCallState) []ToolCallState {
	for left := 1; left < len(calls); left++ {
		for right := left; right > 0 && calls[right].CallID < calls[right-1].CallID; right-- {
			calls[right], calls[right-1] = calls[right-1], calls[right]
		}
	}
	return calls
}

func cloneOperationCommands(source map[OperationID]CommandID) map[OperationID]CommandID {
	cloned := make(map[OperationID]CommandID, len(source))
	for operationID, commandID := range source {
		cloned[operationID] = commandID
	}
	return cloned
}

func cloneOperationAcceptances(source map[OperationID]CommandRecord) map[OperationID]CommandRecord {
	cloned := make(map[OperationID]CommandRecord, len(source))
	for operationID, record := range source {
		cloned[operationID] = record
	}
	return cloned
}

func domainCommitStateMap(states []DomainCommitState) map[DomainCommitStage]*DomainCommitState {
	result := make(map[DomainCommitStage]*DomainCommitState, len(states))
	for index := range states {
		state := states[index]
		result[state.Identity.Stage] = cloneDomainCommitState(&state)
	}
	return result
}
