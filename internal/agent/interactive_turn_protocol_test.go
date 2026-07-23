package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/alfredxw/denova/adk"
)

func TestInteractiveCompletionGuardRetriesFinalAnswerBeforeTurnSubmission(t *testing.T) {
	guard := newInteractiveCompletionGuard(func() bool { return false })
	ctx := context.WithValue(context.Background(), interactiveTurnProtocolStateKey{}, &interactiveTurnProtocolRunState{})
	draft := adk.AssistantMessage("门后传来锁链拖地的声音。", nil)
	decision := guard(ctx, &adk.RetryContext{
		Attempt:       1,
		Messages:      []*adk.Message{adk.UserMessage("推开石门")},
		OutputMessage: draft,
	})

	if decision == nil || !decision.Retry {
		t.Fatalf("missing submission should retry ephemerally: %#v", decision)
	}
	if len(decision.Messages) != 3 {
		t.Fatalf("retry context should include input, bounded draft, and feedback: %#v", decision.Messages)
	}
	feedback := decision.Messages[len(decision.Messages)-1]
	if feedback.Role != adk.User || !strings.Contains(feedback.Content, "submit_interactive_turn") || !strings.Contains(feedback.Content, "retry_modules") {
		t.Fatalf("retry feedback does not explain the protocol: %#v", feedback)
	}
	secondDecision := guard(ctx, &adk.RetryContext{
		Attempt:       2,
		Messages:      decision.Messages,
		OutputMessage: adk.AssistantMessage("第二版候选。", nil),
	})
	if secondDecision == nil || len(secondDecision.Messages) != 3 {
		t.Fatalf("ephemeral retry feedback must not accumulate across attempts: %#v", secondDecision)
	}
	if !protocolMessagesContain(secondDecision.Messages, "门后传来锁链拖地的声音。") || protocolMessagesContain(secondDecision.Messages, "第二版候选。") {
		t.Fatalf("the first narrative candidate must win across retries: %#v", secondDecision.Messages)
	}
	wrapped := interactiveRetryErrorForTest{reason: decision.RejectReason}
	if _, ok := interactiveCompletionRetryFromError(wrapped); !ok {
		t.Fatalf("protocol retry reason should survive WillRetryError: %v", wrapped)
	}
}

func protocolMessagesContain(messages []*adk.Message, needle string) bool {
	for _, message := range messages {
		if message != nil && strings.Contains(message.Content, needle) {
			return true
		}
	}
	return false
}

func TestInteractiveCompletionGuardAcceptsToolCallsAndSubmittedNarrative(t *testing.T) {
	ready := false
	guard := newInteractiveCompletionGuard(func() bool { return ready })
	toolCall := adk.AssistantMessage("", []adk.ToolCall{{
		ID:       "call-submit",
		Function: adk.FunctionCall{Name: interactiveTurnSubmissionToolName, Arguments: `{}`},
	}})
	if decision := guard(context.Background(), &adk.RetryContext{OutputMessage: toolCall}); decision != nil && decision.Retry {
		t.Fatalf("tool calls must enter the normal ReAct loop: %#v", decision)
	}
	ready = true
	if decision := guard(context.Background(), &adk.RetryContext{OutputMessage: adk.AssistantMessage("石门缓缓开启。", nil)}); decision != nil && decision.Retry {
		t.Fatalf("submitted narrative should complete normally: %#v", decision)
	}
}

func TestInteractiveTurnProtocolMiddlewareKeepsStableToolsAndForbidsCallsAfterSubmission(t *testing.T) {
	ready := false
	middleware := newInteractiveTurnProtocolMiddleware(func() bool { return ready })
	state := &adk.RunState{ToolInfos: []*adk.ToolInfo{{Name: interactiveTurnSubmissionToolName}}}
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
	if base.toolChoice == nil || *base.toolChoice != adk.ToolChoiceForbidden {
		t.Fatalf("submitted phase must forbid further tool calls while retaining schemas: %#v", base.toolChoice)
	}
	state.Messages = append(state.Messages, adk.AssistantMessage("", []adk.ToolCall{{
		ID:       "unexpected-call",
		Function: adk.FunctionCall{Name: "read_file", Arguments: `{}`},
	}}))
	if _, _, err := middleware.AfterModelRewriteState(context.Background(), state, &adk.ModelContext{}); err == nil {
		t.Fatal("backend guard must reject a provider that ignores tool_choice=none")
	}
}

func TestInteractiveTurnProtocolAppliesStoryCompletionBudgetOnlyToNarrativeCandidate(t *testing.T) {
	middleware := newInteractiveTurnProtocolMiddleware(func() bool { return false }, 1234)
	base := &interactiveProtocolOptionModel{}
	wrapped, err := middleware.WrapModel(context.Background(), base, &adk.ModelContext{})
	if err != nil {
		t.Fatal(err)
	}
	state := &interactiveTurnProtocolRunState{}
	ctx := context.WithValue(context.Background(), interactiveTurnProtocolStateKey{}, state)
	if _, err := wrapped.Generate(ctx, nil, adk.WithMaxTokens(9999)); err != nil {
		t.Fatal(err)
	}
	if base.maxTokens == nil || *base.maxTokens != 1234 {
		t.Fatalf("first visible narrative should use the story-derived completion budget: %#v", base.maxTokens)
	}
	state.retainNarrativeCandidate("正文候选")
	if _, err := wrapped.Generate(ctx, nil, adk.WithMaxTokens(9999)); err != nil {
		t.Fatal(err)
	}
	if base.maxTokens == nil || *base.maxTokens != 9999 {
		t.Fatalf("structured retry must keep the provider/model budget: %#v", base.maxTokens)
	}
}

type interactiveProtocolOptionModel struct {
	toolChoice *adk.ToolChoice
	maxTokens  *int
}

func (m *interactiveProtocolOptionModel) Generate(_ context.Context, _ []*adk.Message, opts ...adk.ModelOption) (*adk.Message, error) {
	common := adk.GetCommonOptions(&adk.Options{}, opts...)
	m.toolChoice = common.ToolChoice
	m.maxTokens = common.MaxTokens
	return adk.AssistantMessage("正文", nil), nil
}

func (m *interactiveProtocolOptionModel) Stream(_ context.Context, _ []*adk.Message, opts ...adk.ModelOption) (*adk.StreamReader[*adk.Message], error) {
	common := adk.GetCommonOptions(&adk.Options{}, opts...)
	m.toolChoice = common.ToolChoice
	m.maxTokens = common.MaxTokens
	return adk.StreamReaderFromArray([]*adk.Message{adk.AssistantMessage("正文", nil)}), nil
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
