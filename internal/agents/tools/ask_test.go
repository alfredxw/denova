package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/agents/session"
)

type askToolTestInteraction struct {
	request    session.AskInteraction
	resolution session.AskResolution
}

func (interaction *askToolTestInteraction) Ask(_ context.Context, request session.AskInteraction) (session.AskResolution, error) {
	interaction.request = request
	return interaction.resolution, nil
}

func TestAskToolUsesInteractiveWaitContractAndStructuredResult(t *testing.T) {
	definition, err := newAskTool()
	if err != nil {
		t.Fatal(err)
	}
	if definition.Descriptor.Capability != config.AgentToolAsk ||
		definition.Descriptor.Execution != agent.ToolExecutionInteractiveWait ||
		definition.Descriptor.MutationScope != agent.ToolMutationSession ||
		definition.Descriptor.PostCheck != agent.ToolPostCheckSessionState ||
		definition.Descriptor.Recovery != agent.ToolRecoveryReconcilable {
		t.Fatalf("ask descriptor = %#v", definition.Descriptor)
	}
	host := &askToolTestInteraction{resolution: session.AskResolution{
		Schema: "ask.result.v1", ID: "call-ask", Status: session.AskAnswered,
		Answers: []session.AskAnswerResult{{QuestionID: "choice", Question: "Choose", SelectedOptions: []session.AskSelectedOption{{ID: "a", Label: "A"}}}},
	}}
	ctx := ContextWithAskInteraction(agent.ContextWithToolCall(context.Background(), "call-ask", "ask"), host)
	result, err := definition.Tool.Run(ctx, `{"questions":[{"id":"choice","question":"Choose","options":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"recommended_option_id":"a"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if host.request.ID != "call-ask" || host.request.ToolCallID != "call-ask" ||
		host.request.ProviderCallID != "call-ask" || len(host.request.Questions) != 1 {
		t.Fatalf("host request = %#v", host.request)
	}
	var payload session.AskResolution
	if err := json.Unmarshal(result.Details, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status != session.AskAnswered || result.ModelContent != string(result.Details) {
		t.Fatalf("ask result = %#v payload=%#v", result, payload)
	}
}

func TestAskToolRequiresAnInteractiveHost(t *testing.T) {
	definition, err := newAskTool()
	if err != nil {
		t.Fatal(err)
	}
	ctx := agent.ContextWithToolCall(context.Background(), "call-ask", "ask")
	if _, err := definition.Tool.Run(ctx, `{"questions":[{"id":"free","question":"Explain"}]}`); err == nil {
		t.Fatal("ask ran without an interactive host")
	}
}

func TestAskToolFailsClosedInChildInvocation(t *testing.T) {
	definition, err := newAskTool()
	if err != nil {
		t.Fatal(err)
	}
	host := &askToolTestInteraction{resolution: session.AskResolution{
		Schema: "ask.result.v1", ID: "must-not-run", Status: session.AskAnswered,
	}}
	parentCtx := ContextWithAskInteraction(agent.ContextWithToolCall(context.Background(), "call-task", "task"), host)
	childCtx, finish, err := agent.BeginChildInvocation(parentCtx, "researcher")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := finish(); err != nil {
			t.Fatal(err)
		}
	}()
	callCtx := ContextWithAskInteraction(agent.ContextWithToolCall(childCtx, "call-ask", "ask"), host)
	if _, ok := askInteractionFromContext(callCtx); ok {
		t.Fatal("child invocation inherited an interactive ask host")
	}
	if _, err := definition.Tool.Run(callCtx, `{"questions":[{"id":"free","question":"Explain"}]}`); err == nil ||
		!strings.Contains(err.Error(), "root Agent invocation") {
		t.Fatalf("child ask error = %v", err)
	}
	if host.request.ID != "" {
		t.Fatalf("child ask reached interactive host: %#v", host.request)
	}
}
