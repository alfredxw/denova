package runtime

import (
	"fmt"
	"strings"
)

func (s *harnessState) reduce(event Event) error {
	if event.Durability != EventDurable || event.Cursor != s.cursor+1 {
		return fmt.Errorf("non-contiguous durable event cursor %d after %d", event.Cursor, s.cursor)
	}
	s.cursor = event.Cursor
	event.Payload = normalizeRuntimeEventPayload(event.Payload)
	switch payload := event.Payload.(type) {
	case CommandAcceptedEvent:
		if s.retainCommandIndex {
			s.receipts[payload.CommandID] = Receipt{CommandID: payload.CommandID, OperationID: payload.OperationID, Cursor: event.Cursor}
			s.fingerprints[payload.CommandID] = payload.Fingerprint
			s.commandOrder = append(s.commandOrder, payload.CommandID)
			s.trimCommands()
			s.recomputeRetainedCommandBytes()
		}
		if payload.CommandKind == "start_turn" || payload.CommandKind == "next_turn" ||
			payload.CommandKind == "compact_context" || payload.CommandKind == "remove_compaction" {
			s.operationCommands[payload.OperationID] = payload.CommandID
			s.operationAcceptances[payload.OperationID] = CommandRecord{
				Receipt:     Receipt{CommandID: payload.CommandID, OperationID: payload.OperationID, Cursor: event.Cursor},
				Fingerprint: payload.Fingerprint,
			}
		}
	case OperationStartedEvent:
		phase := payload.Phase
		if phase == "" {
			phase = PhaseRunning
		}
		if phase != PhaseRunning && phase != PhaseCompacting {
			return fmt.Errorf("unsupported operation phase %q", phase)
		}
		if phase == PhaseCompacting && payload.Structural == nil {
			return fmt.Errorf("compacting operation requires a structural snapshot")
		}
		if phase == PhaseRunning && payload.Structural != nil {
			return fmt.Errorf("running turn cannot carry a structural snapshot")
		}
		s.phase = phase
		s.activeOperation = payload.OperationID
		s.activeCycle = 0
		s.activeSnapshotID = ""
		s.activeStructural = nil
		s.recoveryPaused = false
		if payload.Structural != nil {
			structural := cloneStructuralOperationSnapshot(payload.Structural)
			if structural.OperationID != payload.OperationID || structural.CommandID != s.operationCommands[payload.OperationID] || structural.Binding != s.binding || structural.Cycle <= 0 {
				return fmt.Errorf("structural snapshot does not match active operation")
			}
			s.activeStructural = structural
			s.activeCycle = structural.Cycle
		}
		s.activeInput = UserInput{}
		s.activeOutputError = nil
		s.activeOutputRehydrated = false
		s.abortRequested = false
		s.abortReason = ""
		s.activeCommandID = s.operationCommands[payload.OperationID]
		s.activeCycleCommandID = s.activeCommandID
		s.pendingCycleCommandID = ""
		s.domainCommits = make(map[DomainCommitStage]*DomainCommitState)
	case QueueEnqueuedEvent:
		s.queue = append(s.queue, cloneQueuedInput(payload.Item))
	case QueueConsumedEvent:
		if !s.removeQueued(payload.CommandID) {
			return fmt.Errorf("consume unknown queued command %q", payload.CommandID)
		}
		s.pendingCycleCommandID = payload.CommandID
	case QueueCancelledEvent:
		if !s.removeQueued(payload.CommandID) {
			return fmt.Errorf("cancel unknown queued command %q", payload.CommandID)
		}
	case UserMessageCommittedEvent:
		message := payload.Message
		if s.retainTimeline {
			s.retainMessage(payload.Message)
		}
		if message.Operation == s.activeOperation {
			s.activeInput = cloneUserInput(message.Input)
		}
	case AssistantMessageCommittedEvent:
		if s.retainTimeline {
			s.retainMessage(payload.Message)
		}
		if payload.Message.Operation == s.activeOperation {
			s.activeContent.Reset()
			s.activeThinking.Reset()
			s.activeOutputRehydrated = false
		}
	case CycleStartedEvent:
		if payload.OperationID != s.activeOperation {
			return fmt.Errorf("cycle operation %q does not match active operation %q", payload.OperationID, s.activeOperation)
		}
		s.activeCycle = payload.Cycle
		s.activeSnapshotID = payload.SnapshotID
		s.recoveryPaused = false
		if s.pendingCycleCommandID != "" {
			s.activeCycleCommandID = s.pendingCycleCommandID
			s.pendingCycleCommandID = ""
		}
		s.domainCommits = make(map[DomainCommitStage]*DomainCommitState)
		s.activeContent.Reset()
		s.activeThinking.Reset()
		s.activeOutputError = nil
		s.activeOutputRehydrated = false
	case OperationRecoveryPausedEvent:
		if payload.OperationID != s.activeOperation || payload.Cycle != s.activeCycle || s.phase == PhaseIdle {
			return fmt.Errorf("recovery pause does not match the active operation cycle")
		}
		if s.recoveryPaused {
			return fmt.Errorf("active operation cycle is already recovery-paused")
		}
		s.recoveryPaused = true
	case InputMaterializationRecoveryPendingEvent:
		if s.phase != PhaseRunning || payload.OperationID != s.activeOperation || payload.Cycle != s.activeCycle ||
			payload.CommandID != s.activeCycleCommandID || !validDeliveryKind(payload.Delivery) {
			return fmt.Errorf("input materialization recovery marker does not match the active cycle")
		}
		if s.inputRecovery != nil || s.recoveryPaused {
			return fmt.Errorf("active cycle already has a recovery marker")
		}
		s.inputRecovery = &InputMaterializationRecovery{
			CommandID: payload.CommandID, OperationID: payload.OperationID,
			Cycle: payload.Cycle, Delivery: payload.Delivery,
		}
		s.recoveryPaused = true
	case InputMaterializationRecoveryResumedEvent:
		if s.inputRecovery == nil || payload.OperationID != s.activeOperation || payload.Cycle != s.activeCycle {
			return fmt.Errorf("input materialization recovery resume does not match the active cycle")
		}
		s.inputRecovery = nil
		s.recoveryPaused = false
	case ToolCallStartedEvent:
		if _, exists := s.openToolCalls[payload.Call.CallID]; exists {
			return fmt.Errorf("tool call %q already started", payload.Call.CallID)
		}
		call := normalizeToolCallState(payload.Call)
		s.openToolCalls[call.CallID] = call
	case ToolCallFinishedEvent:
		call, exists := s.openToolCalls[payload.CallID]
		if !exists {
			return fmt.Errorf("finish unknown tool call %q", payload.CallID)
		}
		if err := s.validateHostEffectAdmission(s.binding, payload.HostEffects); err != nil {
			return err
		}
		for index, effect := range payload.HostEffects {
			if effect.OperationID != call.OperationID || effect.Cycle != call.Cycle || effect.CallID != call.CallID || effect.Index != index {
				return fmt.Errorf("%w: host effect does not match its finished tool call", ErrInvalidCommand)
			}
			cloned := cloneHostEffect(effect)
			s.pendingHostEffects[cloned.ID] = cloned
			s.pendingHostEffectOrder = append(s.pendingHostEffectOrder, cloned.ID)
		}
		delete(s.openToolCalls, payload.CallID)
	case HostEffectAcknowledgedEvent:
		if err := s.removePendingHostEffect(payload.ID, "acknowledge"); err != nil {
			return err
		}
	case HostEffectAbandonedEvent:
		if strings.TrimSpace(payload.Reason) == "" {
			return fmt.Errorf("abandon host effect %q without a reason", payload.ID)
		}
		if err := s.removePendingHostEffect(payload.ID, "abandon"); err != nil {
			return err
		}
	case AbortRequestedEvent:
		if payload.OperationID == s.activeOperation {
			s.abortRequested = true
			s.abortReason = payload.Reason
		}
	case DomainCommitIntentAcceptedEvent:
		if err := s.validateDomainCommitIdentity(payload.Identity); err != nil {
			return err
		}
		if s.domainCommits[payload.Identity.Stage] != nil {
			return fmt.Errorf("domain commit intent for stage %q already accepted", payload.Identity.Stage)
		}
		commit := DomainCommitState{Identity: payload.Identity, Hash: payload.Hash}
		s.domainCommits[payload.Identity.Stage] = &commit
		s.lastDomainCommits[payload.Identity.Stage] = cloneDomainCommitState(&commit)
		s.lastDomainCommit = cloneDomainCommitState(&commit)
	case DomainCommitReconciliationAbandonedEvent:
		commit := s.domainCommits[payload.Identity.Stage]
		if commit == nil || commit.Identity != payload.Identity || commit.Hash != payload.Hash || commit.Revision != "" {
			return fmt.Errorf("abandoned domain commit does not match a pending intent")
		}
		abandoned := *commit
		abandoned.Abandoned = true
		abandoned.Reason = payload.Reason
		delete(s.domainCommits, payload.Identity.Stage)
		s.lastDomainCommits[payload.Identity.Stage] = cloneDomainCommitState(&abandoned)
		s.lastDomainCommit = cloneDomainCommitState(&abandoned)
	case DomainCommitReceiptEvent:
		commit := s.domainCommits[payload.Identity.Stage]
		if commit == nil || commit.Identity != payload.Identity || commit.Hash != payload.Hash {
			return fmt.Errorf("domain commit receipt does not match the accepted intent")
		}
		commit.Revision = payload.Revision
		s.lastDomainCommits[payload.Identity.Stage] = cloneDomainCommitState(commit)
		s.lastDomainCommit = cloneDomainCommitState(commit)
	case SavePointCommittedEvent:
	case OperationSettledEvent:
		if payload.OperationID != s.activeOperation {
			return fmt.Errorf("settled operation %q does not match active operation %q", payload.OperationID, s.activeOperation)
		}
		s.recordTerminalOperation(s.terminalOperationSummary(payload.OperationID, payload.Status, payload.Reason))
		delete(s.operationCommands, payload.OperationID)
		delete(s.operationAcceptances, payload.OperationID)
		s.clearActiveOperation()
	case OperationInterruptedEvent:
		if payload.OperationID != s.activeOperation {
			return fmt.Errorf("interrupted operation %q does not match active operation %q", payload.OperationID, s.activeOperation)
		}
		s.recordTerminalOperation(s.terminalOperationSummary(payload.OperationID, OperationInterrupted, payload.Reason))
		delete(s.operationCommands, payload.OperationID)
		delete(s.operationAcceptances, payload.OperationID)
		s.clearActiveOperation()
	default:
		return fmt.Errorf("unsupported durable event payload %T", event.Payload)
	}
	if err := s.validatePendingInputBudget(); err != nil {
		return err
	}
	if err := s.validatePendingHostEffectBudget(); err != nil {
		return err
	}
	s.retainEvent(event)
	s.recomputeRetainedCommandBytes()
	s.enforceRetainedByteBudget()
	return nil
}

func (s *harnessState) removePendingHostEffect(id HostEffectID, action string) error {
	if _, exists := s.pendingHostEffects[id]; !exists {
		return fmt.Errorf("%s unknown host effect %q", action, id)
	}
	delete(s.pendingHostEffects, id)
	for index, pendingID := range s.pendingHostEffectOrder {
		if pendingID != id {
			continue
		}
		copy(s.pendingHostEffectOrder[index:], s.pendingHostEffectOrder[index+1:])
		s.pendingHostEffectOrder[len(s.pendingHostEffectOrder)-1] = ""
		s.pendingHostEffectOrder = s.pendingHostEffectOrder[:len(s.pendingHostEffectOrder)-1]
		break
	}
	return nil
}

func (s *harnessState) terminalOperationSummary(operationID OperationID, status OperationStatus, reason string) OperationSummary {
	summary := OperationSummary{OperationID: operationID, CommandID: s.activeCommandID, Status: status, Reason: reason}
	if accepted, ok := s.operationAcceptances[operationID]; ok {
		summary.CommandID = accepted.Receipt.CommandID
		summary.CommandFingerprint = accepted.Fingerprint
		summary.ReceiptCursor = accepted.Receipt.Cursor
	}
	return summary
}

func (s *harnessState) clearActiveOperation() {
	s.phase = PhaseIdle
	s.activeOperation = ""
	s.activeCycle = 0
	s.activeSnapshotID = ""
	s.activeStructural = nil
	s.recoveryPaused = false
	s.inputRecovery = nil
	s.activeInput = UserInput{}
	s.activeContent.Reset()
	s.activeThinking.Reset()
	s.activeOutputError = nil
	s.activeOutputRehydrated = false
	s.abortRequested = false
	s.abortReason = ""
	s.engineControls = nil
	s.pendingEngineDone = nil
	s.activeCommandID = ""
	s.activeCycleCommandID = ""
	s.pendingCycleCommandID = ""
	s.domainCommits = make(map[DomainCommitStage]*DomainCommitState)
}
