package agent

import (
	"context"
	"testing"
)

func TestToolStartReceiptPrecedesAndPreservesAskInteraction(t *testing.T) {
	tool, err := InferTool("ask", "ask for one choice", func(ctx context.Context, _ struct{}) (string, error) {
		resolution, err := RequestInteraction(ctx, InteractionRequest{
			ID: "ask-" + CurrentToolExecutionID(ctx), Kind: InteractionAsk, AllowOther: true,
			Questions: []InteractionQuestion{{
				ID: "scope", Prompt: LocalizedText{Chinese: "范围？", English: "Scope?"},
				Options: []InteractionOption{
					{Value: "small", Label: LocalizedText{Chinese: "小", English: "Small"}, Recommended: true},
					{Value: "large", Label: LocalizedText{Chinese: "大", English: "Large"}},
				},
			}},
		})
		if err != nil {
			return "", err
		}
		return resolution.Answers[0].Values[0], nil
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &lifecycleModel{responses: []*Message{
		AssistantMessage("", []ToolCall{{
			ID: "provider-ask", Type: "function", Function: FunctionCall{Name: "ask", Arguments: `{}`},
		}}),
		AssistantMessage("done", nil),
	}}
	owner, err := New(context.Background(), Definition{
		Name: "root", Model: model,
		Tools: mustStaticToolsIdentified(t, CapabilityIdentity{Kind: "tools.ask-order", Version: 1}, ToolDefinition{
			Tool: tool, Descriptor: ToolDescriptor{
				Source: ToolSourceOther, Execution: ToolExecutionInteractiveWait,
				MutationScope: ToolMutationNone, PostCheck: ToolPostCheckNone,
				Recovery: ToolRecoveryReadOnly, ResultProjection: ToolResultBoundedModelContext,
				ResultRetention: ToolResultProtected, Steering: SteeringInterruptibleWait, MaxResultBytes: 64 << 10,
			},
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("ask-start-order"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), Text("ask me"))
	if err != nil {
		t.Fatal(err)
	}

	started := false
	var request InteractionRequest
	for event := range run.Events() {
		switch payload := event.Payload.(type) {
		case ToolStarted:
			started = true
		case InteractionRequested:
			if !started {
				t.Fatal("InteractionRequested was published before the durable ToolStarted receipt")
			}
			request = payload.Request
		}
		if request.ID != "" {
			break
		}
	}
	if request.ID == "" {
		t.Fatal("Ask Interaction was not published")
	}
	snapshot, err := session.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.OpenTools) != 1 || snapshot.OpenTools[0].Name != "ask" {
		t.Fatalf("open tools = %#v", snapshot.OpenTools)
	}
	if len(snapshot.PendingInteractions) != 1 || snapshot.PendingInteractions[0].ID != request.ID {
		t.Fatalf("pending interactions = %#v, want %q", snapshot.PendingInteractions, request.ID)
	}
	if err := run.Respond(context.Background(), request.ID, InteractionResponse{Answers: []InteractionAnswer{{
		QuestionID: "scope", Values: []string{"small"},
	}}}); err != nil {
		t.Fatal(err)
	}
	if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
		t.Fatalf("result=%#v error=%v", result, waitErr)
	}
}
