package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

func TestInteractiveCompletionGuardRetriesFinalAnswerBeforeTurnSubmission(t *testing.T) {
	guard := newInteractiveCompletionGuard(func() bool { return false })
	draft := schema.AssistantMessage("门后传来锁链拖地的声音。", nil)
	decision := guard(context.Background(), &adk.RetryContext{
		RetryAttempt:  1,
		InputMessages: []*schema.Message{schema.UserMessage("推开石门")},
		OutputMessage: draft,
	})

	if decision == nil || !decision.Retry || decision.PersistModifiedInputMessages {
		t.Fatalf("missing submission should retry ephemerally: %#v", decision)
	}
	if len(decision.ModifiedInputMessages) != 3 {
		t.Fatalf("retry context should include input, bounded draft, and feedback: %#v", decision.ModifiedInputMessages)
	}
	feedback := decision.ModifiedInputMessages[len(decision.ModifiedInputMessages)-1]
	if feedback.Role != schema.User || !strings.Contains(feedback.Content, "submit_interactive_turn_result") {
		t.Fatalf("retry feedback does not explain the protocol: %#v", feedback)
	}
	secondDecision := guard(context.Background(), &adk.RetryContext{
		RetryAttempt:  2,
		InputMessages: decision.ModifiedInputMessages,
		OutputMessage: schema.AssistantMessage("第二版候选。", nil),
	})
	if secondDecision == nil || len(secondDecision.ModifiedInputMessages) != 3 {
		t.Fatalf("ephemeral retry feedback must not accumulate across attempts: %#v", secondDecision)
	}
	wrapped := interactiveRetryErrorForTest{reason: decision.RejectReason}
	if _, ok := interactiveCompletionRetryFromError(wrapped); !ok {
		t.Fatalf("protocol retry reason should survive WillRetryError: %v", wrapped)
	}
}

func TestInteractiveCompletionGuardAcceptsToolCallsAndSubmittedNarrative(t *testing.T) {
	ready := false
	guard := newInteractiveCompletionGuard(func() bool { return ready })
	toolCall := schema.AssistantMessage("", []schema.ToolCall{{
		ID:       "call-submit",
		Function: schema.FunctionCall{Name: "submit_interactive_turn_result", Arguments: `{}`},
	}})
	if decision := guard(context.Background(), &adk.RetryContext{OutputMessage: toolCall}); decision != nil && decision.Retry {
		t.Fatalf("tool calls must enter the normal ReAct loop: %#v", decision)
	}
	ready = true
	if decision := guard(context.Background(), &adk.RetryContext{OutputMessage: schema.AssistantMessage("石门缓缓开启。", nil)}); decision != nil && decision.Retry {
		t.Fatalf("submitted narrative should complete normally: %#v", decision)
	}
}

func TestInteractiveTurnProtocolMiddlewareKeepsStableToolsAndForbidsCallsAfterSubmission(t *testing.T) {
	ready := false
	middleware := newInteractiveTurnProtocolMiddleware(func() bool { return ready })
	state := &adk.ChatModelAgentState{ToolInfos: []*schema.ToolInfo{{Name: "submit_interactive_turn_result"}}}
	_, state, err := middleware.BeforeModelRewriteState(context.Background(), state, &adk.ModelContext{})
	if err != nil || len(state.ToolInfos) != 1 {
		t.Fatalf("collecting phase should retain tools: state=%#v err=%v", state, err)
	}
	ready = true
	_, state, err = middleware.BeforeModelRewriteState(context.Background(), state, &adk.ModelContext{})
	if err != nil || len(state.ToolInfos) != 1 {
		t.Fatalf("submitted phase should keep the stable tool schema: state=%#v err=%v", state, err)
	}

	base := &interactiveProtocolOptionModel{}
	wrapped, err := middleware.WrapModel(context.Background(), base, &adk.ModelContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrapped.Generate(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if base.toolChoice == nil || *base.toolChoice != schema.ToolChoiceForbidden {
		t.Fatalf("submitted phase must forbid further tool calls while retaining schemas: %#v", base.toolChoice)
	}
	state.Messages = append(state.Messages, schema.AssistantMessage("", []schema.ToolCall{{
		ID:       "unexpected-call",
		Function: schema.FunctionCall{Name: "read_file", Arguments: `{}`},
	}}))
	if _, _, err := middleware.AfterModelRewriteState(context.Background(), state, &adk.ModelContext{}); err == nil {
		t.Fatal("backend guard must reject a provider that ignores tool_choice=none")
	}
}

type interactiveProtocolOptionModel struct {
	toolChoice *schema.ToolChoice
}

func (m *interactiveProtocolOptionModel) Generate(_ context.Context, _ []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	common := model.GetCommonOptions(&model.Options{}, opts...)
	m.toolChoice = common.ToolChoice
	return schema.AssistantMessage("正文", nil), nil
}

func (m *interactiveProtocolOptionModel) Stream(_ context.Context, _ []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	common := model.GetCommonOptions(&model.Options{}, opts...)
	m.toolChoice = common.ToolChoice
	return schema.StreamReaderFromArray([]*schema.Message{schema.AssistantMessage("正文", nil)}), nil
}

type interactiveRetryErrorForTest struct {
	reason any
}

func (e interactiveRetryErrorForTest) Error() string {
	return "stream rejected"
}

func (e interactiveRetryErrorForTest) RejectReason() any {
	return e.reason
}
