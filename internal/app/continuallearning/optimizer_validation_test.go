package continuallearning

import (
	"context"
	"errors"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	agentstate "github.com/alfredxw/denova/agent/state"
)

func TestOptimizerCompletionGuardReturnsEveryDraftDiagnosticForRepair(t *testing.T) {
	guard := newOptimizerCompletionGuard(func(context.Context) error {
		return &agentstate.ValidationError{Diagnostics: []agentstate.Diagnostic{
			{Code: "invalid_frontmatter", Path: "prompts/general.md", Line: 2, Message: "missing agents"},
			{Code: "unknown_tool", Path: "tools.toml", Message: "unknown tool read_file"},
		}}
	})
	decision := guard(context.Background(), &agent.RetryContext{
		Messages:      []*agent.Message{agent.UserMessage("optimize")},
		OutputMessage: agent.AssistantMessage("done", nil),
	})
	if decision == nil || !decision.Retry || len(decision.Messages) != 2 {
		t.Fatalf("invalid final draft was not returned for repair: %#v", decision)
	}
	feedback := decision.Messages[1].Content
	if !strings.Contains(feedback, "invalid_frontmatter") || !strings.Contains(feedback, "unknown_tool") {
		t.Fatalf("retry feedback omitted diagnostics: %s", feedback)
	}

	second := guard(context.Background(), &agent.RetryContext{
		Messages:      decision.Messages,
		OutputMessage: agent.AssistantMessage("done again", nil),
	})
	if second == nil || len(second.Messages) != 2 {
		t.Fatalf("validation feedback accumulated across retries: %#v", second)
	}
}

func TestOptimizerCompletionGuardLeavesToolCallsAndValidDraftsAlone(t *testing.T) {
	validationCalls := 0
	guard := newOptimizerCompletionGuard(func(context.Context) error {
		validationCalls++
		return nil
	})
	toolCall := agent.AssistantMessage("", []agent.ToolCall{{
		ID: "call-read", Function: agent.FunctionCall{Name: "read", Arguments: `{}`},
	}})
	if decision := guard(context.Background(), &agent.RetryContext{OutputMessage: toolCall}); decision != nil {
		t.Fatalf("ordinary tool call was rejected: %#v", decision)
	}
	if validationCalls != 0 {
		t.Fatalf("tool call triggered premature validation: %d", validationCalls)
	}
	if decision := guard(context.Background(), &agent.RetryContext{OutputMessage: agent.AssistantMessage("done", nil)}); decision != nil {
		t.Fatalf("valid final draft was rejected: %#v", decision)
	}
	if validationCalls != 1 {
		t.Fatalf("final answer validation calls = %d, want 1", validationCalls)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	guard = newOptimizerCompletionGuard(func(context.Context) error { return errors.New("unreachable draft") })
	if decision := guard(cancelled, &agent.RetryContext{OutputMessage: agent.AssistantMessage("done", nil)}); decision != nil {
		t.Fatalf("cancelled run should not start a retry: %#v", decision)
	}
}
