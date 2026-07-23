package agentruntime

import (
	"encoding/json"
	"fmt"
)

func normalizeRuntimeEventPayload(payload EventPayload) EventPayload {
	switch payload := payload.(type) {
	case ToolCallStartedEvent:
		payload.Call = normalizeToolCallState(payload.Call)
		return payload
	case ToolCallFinishedEvent:
		return normalizeToolFinished(payload)
	default:
		return payload
	}
}

func cloneOperationSummary(summary *OperationSummary) *OperationSummary {
	if summary == nil {
		return nil
	}
	cloned := *summary
	var truncated bool
	cloned.Reason, truncated = boundUTF8WithTruncation(cloned.Reason, 16<<10)
	cloned.ReasonTruncated = cloned.ReasonTruncated || truncated
	return &cloned
}

func cloneOperationSummaries(summaries []OperationSummary) []OperationSummary {
	cloned := make([]OperationSummary, len(summaries))
	for index := range summaries {
		cloned[index] = *cloneOperationSummary(&summaries[index])
	}
	return cloned
}

func cloneDomainCommitState(state *DomainCommitState) *DomainCommitState {
	if state == nil {
		return nil
	}
	cloned := *state
	return &cloned
}

func cloneDomainCommitStates(states map[DomainCommitStage]*DomainCommitState) []DomainCommitState {
	cloned := make([]DomainCommitState, 0, len(states))
	for _, stage := range []DomainCommitStage{DomainCommitInput, DomainCommitOutput} {
		if state := states[stage]; state != nil {
			cloned = append(cloned, *cloneDomainCommitState(state))
		}
	}
	return cloned
}

func cloneStatusSnapshot(snapshot StatusSnapshot) StatusSnapshot {
	snapshot.Queue = cloneQueue(snapshot.Queue)
	snapshot.OpenToolCalls = append([]ToolCallState(nil), snapshot.OpenToolCalls...)
	snapshot.LastOperation = cloneOperationSummary(snapshot.LastOperation)
	snapshot.RecentOperations = cloneOperationSummaries(snapshot.RecentOperations)
	snapshot.LastDomainCommit = cloneDomainCommitState(snapshot.LastDomainCommit)
	snapshot.DomainCommits = append([]DomainCommitState(nil), snapshot.DomainCommits...)
	snapshot.ActiveStructural = cloneStructuralOperationSnapshot(snapshot.ActiveStructural)
	snapshot.InputRecovery = cloneInputMaterializationRecovery(snapshot.InputRecovery)
	snapshot.PendingHostEffects = append([]HostEffectSnapshot(nil), snapshot.PendingHostEffects...)
	return snapshot
}

func cloneInputMaterializationRecovery(recovery *InputMaterializationRecovery) *InputMaterializationRecovery {
	if recovery == nil {
		return nil
	}
	cloned := *recovery
	return &cloned
}

func validDeliveryKind(delivery DeliveryKind) bool {
	return delivery == DeliverySteer || delivery == DeliveryFollowUp || delivery == DeliveryNextTurn
}

func cloneStructuralOperationSnapshot(snapshot *StructuralOperationSnapshot) *StructuralOperationSnapshot {
	if snapshot == nil {
		return nil
	}
	cloned := *snapshot
	cloned.Ref = cloneContextCompactionRef(snapshot.Ref)
	return &cloned
}

func cloneContextCompactionRef(ref ContextCompactionRef) ContextCompactionRef {
	ref.RestoreDescriptor = append(json.RawMessage(nil), ref.RestoreDescriptor...)
	return ref
}

func boundUTF8(value string, limit int) string {
	bounded, _ := boundUTF8WithTruncation(value, limit)
	return bounded
}

func boundUTF8WithTruncation(value string, limit int) (string, bool) {
	if limit <= 0 || len(value) <= limit {
		return value, false
	}
	end := limit
	for end > 0 && (value[end]&0xc0) == 0x80 {
		end--
	}
	return value[:end], true
}

func (s *harnessState) sendControl(control EngineControl) {
	if s.engineControls == nil {
		return
	}
	select {
	case s.engineControls <- control:
	default:
		// A control of the same or stronger kind is already pending. The actor
		// stays non-blocking while the engine drains its control channel.
	}
}

func (s *harnessState) publish(event Event) {
	for id, sub := range s.subscribers {
		select {
		case sub.events <- event:
		default:
			s.removeSubscriber(id, fmt.Errorf("subscriber lagged after cursor %d", event.Cursor))
		}
	}
}

func (s *harnessState) removeSubscriber(id uint64, err error) {
	sub := s.subscribers[id]
	if sub == nil {
		return
	}
	delete(s.subscribers, id)
	if err != nil {
		sub.errors <- err
	}
	close(sub.errors)
	close(sub.events)
}

func (s *harnessState) closeSubscribers(err error) {
	ids := make([]uint64, 0, len(s.subscribers))
	for id := range s.subscribers {
		ids = append(ids, id)
	}
	for _, id := range ids {
		s.removeSubscriber(id, err)
	}
}

func cloneMessages(messages []Message) []Message {
	cloned := make([]Message, len(messages))
	for index, message := range messages {
		cloned[index] = cloneMessage(message)
	}
	return cloned
}

func displayMessages(messages []Message) []Message {
	cloned := cloneMessages(messages)
	for index := range cloned {
		cloned[index].Input.TurnSpecRef = ""
		cloned[index].Input.RestoreDescriptor = nil
	}
	return cloned
}

func displayQueue(queue []QueuedInput) []QueuedInput {
	cloned := cloneQueue(queue)
	for index := range cloned {
		cloned[index].Input.TurnSpecRef = ""
		cloned[index].Input.RestoreDescriptor = nil
	}
	return cloned
}

func cloneMessage(message Message) Message {
	message.Input = cloneUserInput(message.Input)
	return message
}

func cloneUserInput(input UserInput) UserInput {
	input.ContextRefs = append([]ContextRef(nil), input.ContextRefs...)
	input.RestoreDescriptor = append([]byte(nil), input.RestoreDescriptor...)
	return input
}

func cloneQueuedInput(item QueuedInput) QueuedInput {
	item.Input = cloneUserInput(item.Input)
	return item
}

func cloneQueue(queue []QueuedInput) []QueuedInput {
	cloned := make([]QueuedInput, len(queue))
	for index, item := range queue {
		cloned[index] = cloneQueuedInput(item)
	}
	return cloned
}
