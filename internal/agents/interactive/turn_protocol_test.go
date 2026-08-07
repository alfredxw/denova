package interactive

import (
	"context"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

func TestInteractiveCompletionGuardRetriesFinalAnswerBeforeTurnSubmission(t *testing.T) {
	guard := NewCompletionGuard(func() bool { return false })
	ctx := context.WithValue(context.Background(), interactiveTurnProtocolStateKey{}, &interactiveTurnProtocolRunState{})
	draft := agent.AssistantMessage("门后传来锁链拖地的声音。", nil)
	decision := guard(ctx, &agent.RetryContext{
		Attempt:       1,
		Messages:      []*agent.Message{agent.UserMessage("推开石门")},
		OutputMessage: draft,
	})

	if decision == nil || !decision.Retry {
		t.Fatalf("missing submission should retry ephemerally: %#v", decision)
	}
	if len(decision.Messages) != 3 {
		t.Fatalf("retry context should include input, bounded draft, and feedback: %#v", decision.Messages)
	}
	feedback := decision.Messages[len(decision.Messages)-1]
	if feedback.Role != agent.User || !strings.Contains(feedback.Content, "submit_interactive_turn") || !strings.Contains(feedback.Content, "retry_modules") {
		t.Fatalf("retry feedback does not explain the protocol: %#v", feedback)
	}
	secondDecision := guard(ctx, &agent.RetryContext{
		Attempt:       2,
		Messages:      decision.Messages,
		OutputMessage: agent.AssistantMessage("第二版候选。", nil),
	})
	if secondDecision == nil || len(secondDecision.Messages) != 3 {
		t.Fatalf("ephemeral retry feedback must not accumulate across attempts: %#v", secondDecision)
	}
	if !protocolMessagesContain(secondDecision.Messages, "门后传来锁链拖地的声音。") || protocolMessagesContain(secondDecision.Messages, "第二版候选。") {
		t.Fatalf("the first narrative candidate must win across retries: %#v", secondDecision.Messages)
	}
	wrapped := interactiveRetryErrorForTest{reason: decision.RejectReason}
	if _, ok := CompletionRetryFromError(wrapped); !ok {
		t.Fatalf("protocol retry reason should survive WillRetryError: %v", wrapped)
	}
}

func protocolMessagesContain(messages []*agent.Message, needle string) bool {
	for _, message := range messages {
		if message != nil && strings.Contains(message.Content, needle) {
			return true
		}
	}
	return false
}

func TestInteractiveCompletionGuardAcceptsToolCallsAndSubmittedNarrative(t *testing.T) {
	ready := false
	guard := NewCompletionGuard(func() bool { return ready })
	toolCall := agent.AssistantMessage("", []agent.ToolCall{{
		ID:       "call-submit",
		Function: agent.FunctionCall{Name: interactiveTurnSubmissionToolName, Arguments: `{}`},
	}})
	if decision := guard(context.Background(), &agent.RetryContext{OutputMessage: toolCall}); decision != nil && decision.Retry {
		t.Fatalf("tool calls must enter the normal ReAct loop: %#v", decision)
	}
	ready = true
	if decision := guard(context.Background(), &agent.RetryContext{OutputMessage: agent.AssistantMessage("石门缓缓开启。", nil)}); decision != nil && decision.Retry {
		t.Fatalf("submitted narrative should complete normally: %#v", decision)
	}
}

func TestInteractiveTurnProtocolMiddlewareKeepsStableToolsAndForbidsCallsAfterSubmission(t *testing.T) {
	ready := false
	middleware := NewTurnProtocolMiddleware(func() bool { return ready })
	state := &agent.RunState{ToolInfos: []*agent.ToolInfo{{Name: interactiveTurnSubmissionToolName}}}
	_, state, err := middleware.BeforeModelRewriteState(context.Background(), state, &agent.ModelContext{})
	if err != nil || len(state.ToolInfos) != 1 {
		t.Fatalf("collecting phase should retain tools: state=%#v err=%v", state, err)
	}
	ready = true
	_, state, err = middleware.BeforeModelRewriteState(context.Background(), state, &agent.ModelContext{})
	if err != nil || len(state.ToolInfos) != 1 {
		t.Fatalf("submitted phase should keep the stable tool schema: state=%#v err=%v", state, err)
	}

	base := &interactiveProtocolOptionModel{}
	wrapped, err := middleware.WrapModel(context.Background(), base, &agent.ModelContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrapped.Generate(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if base.toolChoice == nil || *base.toolChoice != agent.ToolChoiceForbidden {
		t.Fatalf("submitted phase must forbid further tool calls while retaining schemas: %#v", base.toolChoice)
	}
	state.Messages = append(state.Messages, agent.AssistantMessage("", []agent.ToolCall{{
		ID:       "unexpected-call",
		Function: agent.FunctionCall{Name: "read", Arguments: `{}`},
	}}))
	if _, _, err := middleware.AfterModelRewriteState(context.Background(), state, &agent.ModelContext{}); err == nil {
		t.Fatal("backend guard must reject a provider that ignores tool_choice=none")
	}
}

func TestInteractiveTurnProtocolDoesNotOverrideConfiguredOutputLimit(t *testing.T) {
	middleware := NewTurnProtocolMiddleware(func() bool { return false })
	base := &interactiveProtocolOptionModel{}
	wrapped, err := middleware.WrapModel(context.Background(), base, &agent.ModelContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrapped.Generate(context.Background(), nil, agent.WithMaxTokens(9999)); err != nil {
		t.Fatal(err)
	}
	if base.maxTokens == nil || *base.maxTokens != 9999 {
		t.Fatalf("turn protocol must preserve the provider/model output limit: %#v", base.maxTokens)
	}
}

type interactiveProtocolOptionModel struct {
	toolChoice *agent.ToolChoice
	maxTokens  *int
}

func (m *interactiveProtocolOptionModel) Generate(_ context.Context, _ []*agent.Message, opts ...agent.ModelOption) (*agent.Message, error) {
	common := agent.GetCommonOptions(&agent.Options{}, opts...)
	m.toolChoice = common.ToolChoice
	m.maxTokens = common.MaxTokens
	return agent.AssistantMessage("正文", nil), nil
}

func (m *interactiveProtocolOptionModel) Stream(_ context.Context, _ []*agent.Message, opts ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	common := agent.GetCommonOptions(&agent.Options{}, opts...)
	m.toolChoice = common.ToolChoice
	m.maxTokens = common.MaxTokens
	return agent.StreamReaderFromArray([]*agent.Message{agent.AssistantMessage("正文", nil)}), nil
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
