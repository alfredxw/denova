package agent

import (
	"context"
	"reflect"
	"testing"
)

func TestNestedEventPreservesCompleteTypedChildLifecycleEnvelope(t *testing.T) {
	childPayloads := []EventPayload{
		InteractionRequested{Request: InteractionRequest{ID: "ask-child", Kind: InteractionAsk}},
		RecoveryRequired{Reason: "resume child"},
		ArtifactProduced{CallID: "child-call", Artifact: ToolArtifactRef{ID: "artifact-child", Complete: true}},
		CleanupCommitted{State: CleanupState{ID: "cleanup-child", Revision: 1}},
		CompactionCommitted{State: CompactionState{ID: "compact-child", Revision: 1}},
		RunSettled{Status: ResultCompleted},
	}
	tool, err := InferTool("delegate", "delegate", func(ctx context.Context, _ struct{}) (ToolResult, error) {
		for index, payload := range childPayloads {
			durability := EphemeralEvent
			if index%2 == 0 {
				durability = DurableEvent
			}
			if err := ForwardNestedEvent(ctx, NestedEvent{
				Source: EventSource{
					Name: "researcher", Path: []string{"root", "researcher"},
					InvocationID: "task-session/child-run", InvocationType: "task",
				},
				SessionID: "task-session",
				Child:     Event{Cursor: Cursor(index + 1), Durability: durability, RunID: "child-run", Payload: payload},
			}); err != nil {
				return ToolResult{}, err
			}
		}
		return TextToolResult("child result"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	definition := testToolDefinition(tool)
	definition.Descriptor.Source = ToolSourceOther
	definition.Descriptor.Execution = ToolExecutionChild
	model := &lifecycleModel{responses: []*Message{
		AssistantMessage("", []ToolCall{{
			ID: "delegate-call", Type: "function", Function: FunctionCall{Name: "delegate", Arguments: `{}`},
		}}),
		AssistantMessage("root final", nil),
	}}
	owner, err := New(context.Background(), Definition{
		Name: "root", Model: model,
		Tools: mustStaticToolsIdentified(t, CapabilityIdentity{Kind: "test.nested.typed", Version: 1}, definition),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	run, err := owner.Run(context.Background(), Text("delegate"))
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
		t.Fatalf("result=%#v error=%v", result, waitErr)
	}
	var nested []NestedEvent
	for event := range run.Events() {
		if payload, ok := event.Payload.(NestedEvent); ok {
			if event.RunID != run.ID() || event.Durability != EphemeralEvent {
				t.Fatalf("outer parent identity changed: %#v", event)
			}
			nested = append(nested, payload)
		}
	}
	if len(nested) != len(childPayloads) {
		t.Fatalf("nested events=%#v", nested)
	}
	for index, event := range nested {
		if event.SessionID != "task-session" || event.Source.InvocationID != "task-session/child-run" ||
			event.Source.InvocationType != "task" || event.Child.RunID != "child-run" ||
			event.Child.Cursor != Cursor(index+1) || reflect.TypeOf(event.Child.Payload) != reflect.TypeOf(childPayloads[index]) {
			t.Fatalf("nested event %d = %#v", index, event)
		}
		wantDurability := EphemeralEvent
		if index%2 == 0 {
			wantDurability = DurableEvent
		}
		if event.Child.Durability != wantDurability {
			t.Fatalf("nested event %d durability=%q want=%q", index, event.Child.Durability, wantDurability)
		}
	}
}
