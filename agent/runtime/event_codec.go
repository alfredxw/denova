package runtime

import (
	"encoding/json"
	"fmt"
)

// JournalEvent is the stable JSON envelope persisted by Journal
// implementations. Data contains the typed durable payload selected by Type.
type JournalEvent struct {
	Cursor Cursor          `json:"cursor"`
	Type   string          `json:"type"`
	Data   json.RawMessage `json:"data"`
}

type encodedEvent = JournalEvent

// EncodeJournalEvent validates and encodes one durable runtime event.
func EncodeJournalEvent(event Event) (JournalEvent, error) {
	return encodeDurableEvent(event)
}

// DecodeJournalEvent validates and decodes one durable runtime event.
func DecodeJournalEvent(encoded JournalEvent) (Event, error) {
	return decodeDurableEvent(encoded)
}

// MarshalJournalEvent encodes the stable durable event envelope used by
// Journal storage implementations. Display-only events are rejected.
func MarshalJournalEvent(event Event) (json.RawMessage, error) {
	encoded, err := encodeDurableEvent(event)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(encoded)
	if err != nil {
		return nil, fmt.Errorf("encode durable event envelope: %w", err)
	}
	return data, nil
}

// UnmarshalJournalEvent decodes and validates one stable durable event
// envelope produced by MarshalJournalEvent.
func UnmarshalJournalEvent(data json.RawMessage) (Event, error) {
	var encoded encodedEvent
	if err := json.Unmarshal(data, &encoded); err != nil {
		return Event{}, fmt.Errorf("decode durable event envelope: %w", err)
	}
	return decodeDurableEvent(encoded)
}

func encodeDurableEvent(event Event) (encodedEvent, error) {
	if event.Durability != EventDurable {
		return encodedEvent{}, fmt.Errorf("cannot encode %q event", event.Durability)
	}
	eventType, err := durableEventType(event.Payload)
	if err != nil {
		return encodedEvent{}, err
	}
	data, err := json.Marshal(event.Payload)
	if err != nil {
		return encodedEvent{}, fmt.Errorf("encode %s payload: %w", eventType, err)
	}
	return encodedEvent{Cursor: event.Cursor, Type: eventType, Data: data}, nil
}

func decodeDurableEvent(encoded encodedEvent) (Event, error) {
	var payload EventPayload
	switch encoded.Type {
	case "command.accepted":
		payload = &CommandAcceptedEvent{}
	case "operation.started":
		payload = &OperationStartedEvent{}
	case "queue.enqueued":
		payload = &QueueEnqueuedEvent{}
	case "queue.consumed":
		payload = &QueueConsumedEvent{}
	case "queue.steer_requested":
		payload = &QueueSteerRequestedEvent{}
	case "queue.cancelled":
		payload = &QueueCancelledEvent{}
	case "message.user_committed":
		payload = &UserMessageCommittedEvent{}
	case "message.assistant_committed":
		payload = &AssistantMessageCommittedEvent{}
	case "cycle.started":
		payload = &CycleStartedEvent{}
	case "operation.recovery_paused":
		payload = &OperationRecoveryPausedEvent{}
	case "input_materialization.recovery_pending":
		payload = &InputMaterializationRecoveryPendingEvent{}
	case "input_materialization.recovery_resumed":
		payload = &InputMaterializationRecoveryResumedEvent{}
	case "tool.started":
		payload = &ToolCallStartedEvent{}
	case "tool.finished":
		payload = &ToolCallFinishedEvent{}
	case "host_effect.acknowledged":
		payload = &HostEffectAcknowledgedEvent{}
	case "host_effect.abandoned":
		payload = &HostEffectAbandonedEvent{}
	case "operation.abort_requested":
		payload = &AbortRequestedEvent{}
	case "savepoint.committed":
		payload = &SavePointCommittedEvent{}
	case "domain_commit.intent_accepted":
		payload = &DomainCommitIntentAcceptedEvent{}
	case "domain_commit.reconciliation_abandoned":
		payload = &DomainCommitReconciliationAbandonedEvent{}
	case "domain_commit.receipt":
		payload = &DomainCommitReceiptEvent{}
	case "operation.settled":
		payload = &OperationSettledEvent{}
	case "operation.interrupted":
		payload = &OperationInterruptedEvent{}
	default:
		return Event{}, fmt.Errorf("unsupported durable event type %q", encoded.Type)
	}
	if err := json.Unmarshal(encoded.Data, payload); err != nil {
		return Event{}, fmt.Errorf("decode %s payload: %w", encoded.Type, err)
	}
	return Event{Cursor: encoded.Cursor, Durability: EventDurable, Payload: dereferencePayload(payload)}, nil
}

func durableEventType(payload EventPayload) (string, error) {
	switch payload.(type) {
	case CommandAcceptedEvent:
		return "command.accepted", nil
	case OperationStartedEvent:
		return "operation.started", nil
	case QueueEnqueuedEvent:
		return "queue.enqueued", nil
	case QueueConsumedEvent:
		return "queue.consumed", nil
	case QueueSteerRequestedEvent:
		return "queue.steer_requested", nil
	case QueueCancelledEvent:
		return "queue.cancelled", nil
	case UserMessageCommittedEvent:
		return "message.user_committed", nil
	case AssistantMessageCommittedEvent:
		return "message.assistant_committed", nil
	case CycleStartedEvent:
		return "cycle.started", nil
	case OperationRecoveryPausedEvent:
		return "operation.recovery_paused", nil
	case InputMaterializationRecoveryPendingEvent:
		return "input_materialization.recovery_pending", nil
	case InputMaterializationRecoveryResumedEvent:
		return "input_materialization.recovery_resumed", nil
	case ToolCallStartedEvent:
		return "tool.started", nil
	case ToolCallFinishedEvent:
		return "tool.finished", nil
	case HostEffectAcknowledgedEvent:
		return "host_effect.acknowledged", nil
	case HostEffectAbandonedEvent:
		return "host_effect.abandoned", nil
	case AbortRequestedEvent:
		return "operation.abort_requested", nil
	case SavePointCommittedEvent:
		return "savepoint.committed", nil
	case DomainCommitIntentAcceptedEvent:
		return "domain_commit.intent_accepted", nil
	case DomainCommitReconciliationAbandonedEvent:
		return "domain_commit.reconciliation_abandoned", nil
	case DomainCommitReceiptEvent:
		return "domain_commit.receipt", nil
	case OperationSettledEvent:
		return "operation.settled", nil
	case OperationInterruptedEvent:
		return "operation.interrupted", nil
	default:
		return "", fmt.Errorf("unsupported durable event payload %T", payload)
	}
}

func dereferencePayload(payload EventPayload) EventPayload {
	switch payload := payload.(type) {
	case *CommandAcceptedEvent:
		return *payload
	case *OperationStartedEvent:
		return *payload
	case *QueueEnqueuedEvent:
		return *payload
	case *QueueConsumedEvent:
		return *payload
	case *QueueSteerRequestedEvent:
		return *payload
	case *QueueCancelledEvent:
		return *payload
	case *UserMessageCommittedEvent:
		return *payload
	case *AssistantMessageCommittedEvent:
		return *payload
	case *CycleStartedEvent:
		return *payload
	case *OperationRecoveryPausedEvent:
		return *payload
	case *InputMaterializationRecoveryPendingEvent:
		return *payload
	case *InputMaterializationRecoveryResumedEvent:
		return *payload
	case *ToolCallStartedEvent:
		return *payload
	case *ToolCallFinishedEvent:
		return *payload
	case *HostEffectAcknowledgedEvent:
		return *payload
	case *HostEffectAbandonedEvent:
		return *payload
	case *AbortRequestedEvent:
		return *payload
	case *SavePointCommittedEvent:
		return *payload
	case *DomainCommitIntentAcceptedEvent:
		return *payload
	case *DomainCommitReconciliationAbandonedEvent:
		return *payload
	case *DomainCommitReceiptEvent:
		return *payload
	case *OperationSettledEvent:
		return *payload
	case *OperationInterruptedEvent:
		return *payload
	default:
		panic(fmt.Sprintf("unexpected event payload pointer %T", payload))
	}
}
