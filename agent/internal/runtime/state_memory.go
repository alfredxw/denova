package runtime

import (
	"encoding/json"
	"fmt"
)

// The accounting in this file deliberately counts logical payload bytes plus
// conservative fixed overhead. It does not attempt to mirror a particular Go
// allocator version. Strings shared by immutable runtime records are counted
// conservatively more than once; opaque restore descriptors are retained only
// by the active/queued obligation state, never by the display timeline.

const retainedObjectOverhead int64 = 128

func userInputPayloadBytes(input UserInput) int64 {
	bytes := retainedObjectOverhead + int64(len(input.Text)+len(input.TurnSpecRef)+len(input.RestoreDescriptor))
	for _, ref := range input.ContextRefs {
		bytes += retainedObjectOverhead + int64(len(ref.Source)+len(ref.Resource)+len(ref.Selector)+len(ref.Revision))
	}
	return bytes
}

func contextCompactionPayloadBytes(ref ContextCompactionRef) int64 {
	return retainedObjectOverhead + int64(
		len(ref.SpecRef)+len(ref.Source)+len(ref.Purpose)+len(ref.Resource)+
			len(ref.ExpectedRevision)+len(ref.CompactionID)+len(ref.RestoreDescriptor),
	)
}

func queuedInputPayloadBytes(item QueuedInput) int64 {
	return retainedObjectOverhead + int64(len(item.CommandID)+len(item.OperationID)+len(item.Delivery)) + userInputPayloadBytes(item.Input)
}

func messagePayloadBytes(message Message) int64 {
	return retainedObjectOverhead + int64(len(message.ID)+len(message.Role)+len(message.Content)+len(message.Thinking)+len(message.Operation)) + userInputPayloadBytes(message.Input)
}

func operationSummaryPayloadBytes(summary OperationSummary) int64 {
	return retainedObjectOverhead + int64(
		len(summary.OperationID)+len(summary.CommandID)+len(summary.CommandFingerprint)+
			len(summary.Status)+len(summary.Reason),
	)
}

func hostEffectPayloadBytes(effect HostEffect) int64 {
	return retainedObjectOverhead + int64(
		len(effect.ID)+len(effect.Kind)+len(effect.OperationID)+len(effect.CallID)+
			len(effect.Payload)+len(effect.PayloadDescriptor.SHA256),
	)
}

func durableEventPayloadBytes(event Event) int64 {
	bytes := retainedObjectOverhead
	switch payload := event.Payload.(type) {
	case CommandAcceptedEvent:
		bytes += int64(len(payload.CommandID) + len(payload.CommandKind) + len(payload.OperationID) + len(payload.Fingerprint))
	case OperationStartedEvent:
		bytes += int64(len(payload.OperationID) + len(payload.Phase))
		if payload.Structural != nil {
			bytes += retainedObjectOverhead + int64(len(payload.Structural.CommandID)+len(payload.Structural.OperationID)+len(payload.Structural.Kind))
			bytes += contextCompactionPayloadBytes(payload.Structural.Ref)
		}
	case QueueEnqueuedEvent:
		bytes += queuedInputPayloadBytes(payload.Item)
	case QueueConsumedEvent:
		bytes += int64(len(payload.CommandID) + len(payload.Delivery))
	case QueueSteerRequestedEvent:
		bytes += int64(len(payload.CommandID))
	case QueueCancelledEvent:
		bytes += int64(len(payload.CommandID) + len(payload.Reason))
	case UserMessageCommittedEvent:
		bytes += messagePayloadBytes(payload.Message)
	case AssistantMessageCommittedEvent:
		bytes += messagePayloadBytes(payload.Message)
	case EngineStateCommittedEvent:
		bytes += int64(len(payload.State) + len(payload.Descriptor.SHA256))
	case CapabilityStateCommittedEvent:
		bytes += int64(len(payload.Capability) + len(payload.Expected.SHA256) + len(payload.State) + len(payload.OperationID))
	case CleanupCompletedEvent:
		bytes += int64(len(payload.OperationID) + len(payload.ID) + len(payload.Reason))
	case InteractionRequestedEvent:
		bytes += int64(len(payload.ID) + len(payload.OperationID) + len(payload.ToolCallID) + len(payload.Request) + len(payload.Descriptor.SHA256))
	case InteractionResolvedEvent:
		bytes += int64(len(payload.ID) + len(payload.OperationID) + len(payload.Response) + len(payload.ResponseDescriptor.SHA256))
	case InteractionRecoveryResumedEvent:
		bytes += int64(len(payload.ID) + len(payload.OperationID))
	case CycleStartedEvent:
		bytes += int64(len(payload.OperationID) + len(payload.SnapshotID))
	case OperationRecoveryPausedEvent:
		bytes += int64(len(payload.OperationID) + len(payload.Reason))
	case InputMaterializationRecoveryPendingEvent:
		bytes += int64(len(payload.OperationID) + len(payload.CommandID) + len(payload.Delivery))
	case InputMaterializationRecoveryResumedEvent:
		bytes += int64(len(payload.OperationID))
	case ToolCallStartedEvent:
		bytes += retainedObjectOverhead + int64(len(payload.Call.CallID)+len(payload.Call.Name)+len(payload.Call.OperationID)+len(payload.Call.ArgumentsDescriptor.SHA256))
	case ToolCallFinishedEvent:
		bytes += int64(len(payload.CallID) + len(payload.Name) + len(payload.ResultDescriptor.SHA256) + len(payload.RetrySafety))
		for _, effect := range payload.HostEffects {
			bytes += hostEffectPayloadBytes(effect)
		}
	case ArtifactProducedEvent:
		bytes += int64(len(payload.OperationID) + len(payload.CallID) + len(payload.Artifact))
	case HostEffectAcknowledgedEvent:
		bytes += int64(len(payload.ID))
	case HostEffectAbandonedEvent:
		bytes += int64(len(payload.ID) + len(payload.Reason))
	case AbortRequestedEvent:
		bytes += int64(len(payload.OperationID) + len(payload.Reason))
	case SavePointCommittedEvent:
		bytes += int64(len(payload.OperationID))
	case DomainCommitIntentAcceptedEvent:
		bytes += domainCommitIdentityPayloadBytes(payload.Identity) + int64(len(payload.Hash))
	case DomainCommitReconciliationAbandonedEvent:
		bytes += domainCommitIdentityPayloadBytes(payload.Identity) + int64(len(payload.Hash)+len(payload.Reason))
	case DomainCommitReceiptEvent:
		bytes += domainCommitIdentityPayloadBytes(payload.Identity) + int64(len(payload.Hash)+len(payload.Revision))
	case OperationSettledEvent:
		bytes += int64(len(payload.OperationID) + len(payload.Status) + len(payload.Reason))
	case OperationInterruptedEvent:
		bytes += int64(len(payload.OperationID) + len(payload.Reason))
	}
	return bytes
}

func domainCommitIdentityPayloadBytes(identity DomainCommitIdentity) int64 {
	return retainedObjectOverhead + int64(len(identity.CommandID)+len(identity.OperationID)+len(identity.Stage))
}

func displayUserInput(input UserInput) UserInput {
	input.ContextRefs = append([]ContextRef(nil), input.ContextRefs...)
	input.TurnSpecRef = ""
	input.RestoreDescriptor = nil
	return input
}

func displayMessageForRetention(message Message) Message {
	message.Input = displayUserInput(message.Input)
	return message
}

func displayEventForRetention(event Event) Event {
	switch payload := event.Payload.(type) {
	case OperationStartedEvent:
		payload.Structural = cloneStructuralOperationSnapshot(payload.Structural)
		if payload.Structural != nil {
			payload.Structural.Ref.RestoreDescriptor = nil
		}
		event.Payload = payload
	case QueueEnqueuedEvent:
		payload.Item.Input = displayUserInput(payload.Item.Input)
		event.Payload = payload
	case UserMessageCommittedEvent:
		payload.Message = displayMessageForRetention(payload.Message)
		event.Payload = payload
	case AssistantMessageCommittedEvent:
		payload.Message = displayMessageForRetention(payload.Message)
		event.Payload = payload
	case EngineStateCommittedEvent:
		payload.State = nil
		event.Payload = payload
	case ToolCallFinishedEvent:
		payload = normalizeToolFinished(payload)
		for index := range payload.HostEffects {
			payload.HostEffects[index].Payload = nil
		}
		event.Payload = payload
	}
	return event
}

func (s *harnessState) pendingInputBytes() int64 {
	var bytes int64
	if s.activeInput.Text != "" || s.activeInput.TurnSpecRef != "" || len(s.activeInput.ContextRefs) != 0 || len(s.activeInput.RestoreDescriptor) != 0 {
		bytes += userInputPayloadBytes(s.activeInput)
	}
	for _, item := range s.queue {
		bytes += queuedInputPayloadBytes(item)
	}
	if s.activeStructural != nil {
		bytes += contextCompactionPayloadBytes(s.activeStructural.Ref)
	}
	return bytes
}

func (s *harnessState) activeOutputBytes() int64 {
	return int64(s.activeContent.Len() + s.activeThinking.Len())
}

func (s *harnessState) pendingHostEffectBytes() int64 {
	var bytes int64
	for _, effect := range s.pendingHostEffects {
		bytes += hostEffectPayloadBytes(effect)
	}
	return bytes
}

func (s *harnessState) retainedBytes() int64 {
	return s.retainedEventBytes + s.retainedMessageBytes + s.retainedCommandBytes
}

func (s *harnessState) memorySnapshot() BindingMemorySnapshot {
	return BindingMemorySnapshot{
		RetainedBytes: s.retainedBytes(), PendingInputBytes: s.pendingInputBytes(),
		ActiveOutputBytes: s.activeOutputBytes(), Limits: s.memoryLimits.normalized(),
		PendingHostEffectBytes: s.pendingHostEffectBytes(), PendingHostEffects: len(s.pendingHostEffects),
		InteractionBytes: s.interactionBytes(), PendingInteractions: len(s.interactions),
	}
}

func (s *harnessState) validateHostEffectAdmission(binding BindingRef, effects []HostEffect) error {
	limits := s.memoryLimits.normalized()
	currentBytes := s.pendingHostEffectBytes()
	currentCount := len(s.pendingHostEffects)
	seen := make(map[HostEffectID]struct{}, len(effects))
	var incomingBytes int64
	for _, effect := range effects {
		if err := validateHostEffect(binding, effect, limits); err != nil {
			return err
		}
		if _, exists := s.pendingHostEffects[effect.ID]; exists {
			return fmt.Errorf("%w: host effect %q is already pending", ErrInvalidCommand, effect.ID)
		}
		if _, duplicate := seen[effect.ID]; duplicate {
			return fmt.Errorf("%w: duplicate host effect %q in tool finish", ErrInvalidCommand, effect.ID)
		}
		seen[effect.ID] = struct{}{}
		incomingBytes += hostEffectPayloadBytes(effect)
	}
	if currentCount+len(effects) > limits.MaxPendingHostEffects || currentBytes+incomingBytes > limits.MaxPendingHostEffectBytes {
		return &ByteBudgetError{
			Scope: ByteBudgetHostEffect, Current: currentBytes, Incoming: incomingBytes,
			Limit: limits.MaxPendingHostEffectBytes,
		}
	}
	return nil
}

func (s *harnessState) validatePendingHostEffectBudget() error {
	limits := s.memoryLimits.normalized()
	bytes := s.pendingHostEffectBytes()
	if len(s.pendingHostEffects) <= limits.MaxPendingHostEffects && bytes <= limits.MaxPendingHostEffectBytes {
		return nil
	}
	return &ByteBudgetError{Scope: ByteBudgetHostEffect, Current: bytes, Limit: limits.MaxPendingHostEffectBytes}
}

func (s *harnessState) admitPendingInput(input UserInput) error {
	limits := s.memoryLimits.normalized()
	current := s.pendingInputBytes()
	incoming := userInputPayloadBytes(input)
	if current+incoming <= limits.MaxPendingInputBytes {
		return nil
	}
	return &ByteBudgetError{Scope: ByteBudgetPendingInput, Current: current, Incoming: incoming, Limit: limits.MaxPendingInputBytes}
}

func (s *harnessState) admitStructuralRef(ref ContextCompactionRef) error {
	limits := s.memoryLimits.normalized()
	current := s.pendingInputBytes()
	incoming := contextCompactionPayloadBytes(ref)
	if current+incoming <= limits.MaxPendingInputBytes {
		return nil
	}
	return &ByteBudgetError{Scope: ByteBudgetPendingInput, Current: current, Incoming: incoming, Limit: limits.MaxPendingInputBytes}
}

func (s *harnessState) validatePendingInputBudget() error {
	limits := s.memoryLimits.normalized()
	current := s.pendingInputBytes()
	if current <= limits.MaxPendingInputBytes {
		return nil
	}
	return &ByteBudgetError{Scope: ByteBudgetPendingInput, Current: current, Limit: limits.MaxPendingInputBytes}
}

func (s *harnessState) admitActiveBytes(scope ByteBudgetScope, incoming int) error {
	if s.activeOutputError != nil {
		cloned := *s.activeOutputError
		return &cloned
	}
	limits := s.memoryLimits.normalized()
	current := s.activeOutputBytes()
	if incoming >= 0 && current+int64(incoming) <= limits.MaxActiveOutputBytes {
		return nil
	}
	err := &ByteBudgetError{Scope: scope, Current: current, Incoming: int64(incoming), Limit: limits.MaxActiveOutputBytes}
	s.activeOutputError = err
	return err
}

func (s *harnessState) admitFinalOutput(content, thinking string) error {
	if s.activeOutputError != nil {
		cloned := *s.activeOutputError
		return &cloned
	}
	limits := s.memoryLimits.normalized()
	incoming := int64(len(content) + len(thinking))
	if incoming <= limits.MaxActiveOutputBytes {
		return nil
	}
	err := &ByteBudgetError{Scope: ByteBudgetActiveOutput, Incoming: incoming, Limit: limits.MaxActiveOutputBytes}
	s.activeOutputError = err
	return err
}

func (s *harnessState) validateEngineState(state json.RawMessage) error {
	if state == nil {
		return nil
	}
	if !json.Valid(state) {
		return fmt.Errorf("%w: engine state is not valid JSON", ErrInvalidCommand)
	}
	limit := s.memoryLimits.normalized().MaxEngineStateBytes
	if int64(len(state)) > limit {
		return &ByteBudgetError{Scope: ByteBudgetEngineState, Incoming: int64(len(state)), Limit: limit}
	}
	return nil
}

func (s *harnessState) retainEvent(event Event) {
	if !s.retainTimeline {
		return
	}
	event = displayEventForRetention(event)
	s.events = append(s.events, event)
	s.retainedEventBytes += durableEventPayloadBytes(event)
	for s.maxRetainedEvents > 0 && len(s.events) > s.maxRetainedEvents {
		s.dropOldestEvent()
	}
}

func (s *harnessState) retainMessage(message Message) {
	if !s.retainTimeline {
		return
	}
	message = displayMessageForRetention(message)
	s.messages = append(s.messages, message)
	s.retainedMessageBytes += messagePayloadBytes(message)
	for s.maxRetainedMessages > 0 && len(s.messages) > s.maxRetainedMessages {
		s.dropOldestMessage()
	}
}

func (s *harnessState) dropOldestEvent() {
	if len(s.events) == 0 {
		return
	}
	s.retainedEventBytes -= durableEventPayloadBytes(s.events[0])
	copy(s.events, s.events[1:])
	s.events[len(s.events)-1] = Event{}
	s.events = s.events[:len(s.events)-1]
}

func (s *harnessState) dropOldestMessage() {
	if len(s.messages) == 0 {
		return
	}
	s.retainedMessageBytes -= messagePayloadBytes(s.messages[0])
	copy(s.messages, s.messages[1:])
	s.messages[len(s.messages)-1] = Message{}
	s.messages = s.messages[:len(s.messages)-1]
	s.messagesTruncated = true
}

func (s *harnessState) recomputeRetainedCommandBytes() {
	var bytes int64
	for _, commandID := range s.commandOrder {
		bytes += retainedObjectOverhead + int64(len(commandID)+len(s.fingerprints[commandID]))
	}
	for _, summary := range s.recentOperations {
		bytes += operationSummaryPayloadBytes(summary)
	}
	s.retainedCommandBytes = bytes
}

func (s *harnessState) enforceRetainedByteBudget() {
	limits := s.memoryLimits.normalized()
	for s.retainedBytes() > limits.MaxRetainedBytes {
		switch {
		case len(s.events) > 0:
			s.dropOldestEvent()
		case len(s.messages) > 0:
			s.dropOldestMessage()
		case len(s.commandOrder) > 0:
			s.dropOldestCommands(1)
		default:
			return
		}
	}
}

func (s *harnessState) dropOldestCommands(count int) {
	if count <= 0 || len(s.commandOrder) == 0 {
		return
	}
	if count > len(s.commandOrder) {
		count = len(s.commandOrder)
	}
	dropped := make(map[CommandID]struct{}, count)
	for _, commandID := range s.commandOrder[:count] {
		delete(s.receipts, commandID)
		delete(s.fingerprints, commandID)
		dropped[commandID] = struct{}{}
	}
	copy(s.commandOrder, s.commandOrder[count:])
	clear(s.commandOrder[len(s.commandOrder)-count:])
	s.commandOrder = s.commandOrder[:len(s.commandOrder)-count]
	retained := s.recentOperations[:0]
	for _, summary := range s.recentOperations {
		if _, removed := dropped[summary.CommandID]; !removed {
			retained = append(retained, summary)
		}
	}
	clear(s.recentOperations[len(retained):])
	s.recentOperations = retained
	s.recomputeRetainedCommandBytes()
}
