package runtime

import (
	"encoding/json"
	"testing"
)

func TestJournalCheckpointAdapterRoundTripsAllPendingObligations(t *testing.T) {
	ref := testBinding("checkpoint-adapter-obligations")
	state := newHarnessState(ref)
	state.maxRetainedEvents = 128
	state.maxRetainedMessages = 128
	state.maxRetainedCommands = 128
	effect, err := NewToolHostEffect(ref, "operation", 1, "finished-call", 0, HostEffectToolMutationCommitted, json.RawMessage(`{"version":1,"mutation":"pending"}`))
	if err != nil {
		t.Fatal(err)
	}
	identity := DomainCommitIdentity{CommandID: "start", OperationID: "operation", Cycle: 1, Stage: DomainCommitOutput}
	payloads := []EventPayload{
		CommandAcceptedEvent{CommandID: "start", CommandKind: "start_turn", OperationID: "operation", Fingerprint: "start-fingerprint"},
		OperationStartedEvent{OperationID: "operation"},
		UserMessageCommittedEvent{Message: Message{ID: "user", Role: RoleUser, Content: "write", Input: UserInput{Text: "write", RestoreDescriptor: json.RawMessage(`{"restore":1}`)}, Operation: "operation"}},
		CycleStartedEvent{OperationID: "operation", Cycle: 1, SnapshotID: "snapshot"},
		CommandAcceptedEvent{CommandID: "follow", CommandKind: "follow_up", OperationID: "operation", Fingerprint: "follow-fingerprint"},
		QueueEnqueuedEvent{Item: QueuedInput{CommandID: "follow", OperationID: "operation", Delivery: DeliveryFollowUp, Input: UserInput{Text: "continue", RestoreDescriptor: json.RawMessage(`{"restore":2}`)}}},
		ToolCallStartedEvent{Call: ToolCallState{CallID: "open-call", Name: "open", OperationID: "operation", Cycle: 1}},
		ToolCallStartedEvent{Call: ToolCallState{CallID: "finished-call", Name: "mutate", OperationID: "operation", Cycle: 1}},
		ToolCallFinishedEvent{CallID: "finished-call", Name: "mutate", HostEffects: []HostEffect{effect}},
		DomainCommitIntentAcceptedEvent{Identity: identity, Hash: "domain-hash"},
		InputMaterializationRecoveryPendingEvent{OperationID: "operation", Cycle: 1, CommandID: "start", Delivery: DeliveryFollowUp},
	}
	for index, payload := range payloads {
		if err := state.reduce(Event{Cursor: Cursor(index + 1), Durability: EventDurable, Payload: payload}); err != nil {
			t.Fatalf("reduce cursor %d: %v", index+1, err)
		}
	}
	state.activeContent.WriteString("partial content")
	state.activeThinking.WriteString("partial thinking")

	encoded, err := (journalCheckpointState{state: &state}).MarshalCheckpoint()
	if err != nil {
		t.Fatal(err)
	}
	restored := newHarnessState(ref)
	restored.maxRetainedEvents = 128
	restored.maxRetainedMessages = 128
	restored.maxRetainedCommands = 128
	if err := (journalCheckpointState{state: &restored}).RestoreCheckpoint(encoded); err != nil {
		t.Fatal(err)
	}
	if restored.inputRecovery == nil || len(restored.queue) != 1 || len(restored.openToolCalls) != 1 ||
		len(restored.pendingHostEffects) != 1 || restored.pendingDomainCommit() == nil {
		t.Fatalf("checkpoint lost obligations: input=%#v queue=%#v tools=%#v effects=%#v domains=%#v",
			restored.inputRecovery, restored.queue, restored.openToolCalls, restored.pendingHostEffects, restored.domainCommits)
	}
	if string(restored.activeInput.RestoreDescriptor) != `{"restore":1}` || string(restored.queue[0].Input.RestoreDescriptor) != `{"restore":2}` {
		t.Fatalf("private restore descriptors were not restored: active=%s queued=%s", restored.activeInput.RestoreDescriptor, restored.queue[0].Input.RestoreDescriptor)
	}
	if got := string(restored.pendingHostEffects[effect.ID].Payload); got != string(effect.Payload) {
		t.Fatalf("pending host effect payload = %s, want %s", got, effect.Payload)
	}
	if restored.activeContent.String() != "partial content" || restored.activeThinking.String() != "partial thinking" || !restored.activeOutputRehydrated {
		t.Fatalf("active stream recovery = content %q thinking %q rehydrate=%v", restored.activeContent.String(), restored.activeThinking.String(), restored.activeOutputRehydrated)
	}
}
