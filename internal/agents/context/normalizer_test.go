package context

import (
	"errors"
	"reflect"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

func TestNormalizeModelContextMessagesPreservesValidRichExchange(t *testing.T) {
	call := contextNormalizerTestCall("text_tool_call_stable", "read", `{"path":"chapter.md"}`)
	result := agent.ToolMessage(agent.ToolResult{
		ModelContent: "complete rich chapter", DisplayContent: "complete rich chapter", Status: agent.ToolResultSuccess,
		ResultRetention: agent.ToolResultProtected,
		ContextHints: &agent.ToolResultContextHints{
			Recovery: agent.ToolResultRecoveryHint{
				Kind: agent.ToolResultRecoveryRead, Reference: map[string]any{"path": "chapter.md"},
			},
			ContextValue: agent.ToolResultContextDiscardable,
		},
	}, call.ID, agent.WithToolName("read"))
	input := []*agent.Message{
		agent.SystemMessage("system"),
		agent.AssistantMessage("I will inspect it.", []agent.ToolCall{call}),
		result,
		agent.UserMessage("continue"),
	}

	normalized, err := NormalizeModelContextMessages(input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(normalized, input) {
		t.Fatalf("valid rich exchange changed:\nwant=%#v\ngot=%#v", input, normalized)
	}
	if normalized[1] == input[1] || normalized[2] == input[2] || normalized[2].ToolResult == input[2].ToolResult {
		t.Fatal("normalizer returned caller-owned message state")
	}
	normalized[2].ToolResult.ContextHints.Recovery.Reference["path"] = "mutated"
	if got := input[2].ToolResult.ContextHints.Recovery.Reference["path"]; got != "chapter.md" {
		t.Fatalf("normalizer aliased rich result metadata: %v", got)
	}
	if normalized[2].ToolResult.ResultRetention != agent.ToolResultProtected {
		t.Fatalf("normalizer applied retention policy: %#v", normalized[2].ToolResult)
	}
}

func TestNormalizeModelContextMessagesRepairsUniqueMissingResultDeterministically(t *testing.T) {
	call := contextNormalizerTestCall("text_tool_call_stable", "write", `{"path":"chapter.md"}`)
	input := []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{call}),
		agent.UserMessage("continue"),
	}

	first, err := NormalizeModelContextMessages(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NormalizeModelContextMessages(first)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("normalization is not idempotent:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if len(first) != 3 || first[0].ToolCalls[0].ID != call.ID {
		t.Fatalf("missing-result repair changed the stable call identity: %#v", first)
	}
	result := first[1]
	if result.Role != agent.ToolRole || result.ToolCallID != call.ID || result.ToolName != call.Function.Name ||
		result.ToolResult == nil || result.ToolResult.Status != agent.ToolResultError ||
		result.ToolResult.SyntheticReason != agent.ToolSyntheticEffectUnknown || !IsUnknownToolEffectResult(result.Content) {
		t.Fatalf("unexpected effect_unknown completion: %#v", result)
	}
	if result.ToolResult.ResultRetention != "" {
		t.Fatalf("normalizer assigned retention to a recovery result: %#v", result.ToolResult)
	}
}

func TestNormalizeModelContextMessagesDropsAmbiguousHalvesAtomically(t *testing.T) {
	valid := contextNormalizerTestCall("valid", "read", `{}`)
	missing := contextNormalizerTestCall("missing", "write", `{}`)
	invalid := contextNormalizerTestCall("invalid", "read", `{"path":`)
	duplicateA := contextNormalizerTestCall("duplicate-call", "read", `{}`)
	duplicateB := contextNormalizerTestCall("duplicate-call", "write", `{}`)
	duplicateResult := contextNormalizerTestCall("duplicate-result", "read", `{}`)
	validResult := agent.ToolMessage(agent.TextToolResult("rich valid result"), valid.ID, agent.WithToolName("read"))
	validResult.ToolResult.ResultRetention = agent.ToolResultProtected
	input := []*agent.Message{
		agent.AssistantMessage("useful narration", []agent.ToolCall{valid, missing, invalid, duplicateA, duplicateB, duplicateResult}),
		validResult,
		agent.ToolMessage(agent.TextToolResult("invalid result"), invalid.ID),
		agent.ToolMessage(agent.TextToolResult("ambiguous call result"), duplicateA.ID),
		agent.ToolMessage(agent.TextToolResult("first duplicate result"), duplicateResult.ID),
		agent.ToolMessage(agent.TextToolResult("second duplicate result"), duplicateResult.ID),
		agent.ToolMessage(agent.TextToolResult("orphan result"), "orphan"),
		agent.UserMessage("continue"),
	}

	normalized, err := NormalizeModelContextMessages(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(normalized) != 4 || normalized[0].Role != agent.Assistant || normalized[3].Role != agent.User {
		t.Fatalf("unexpected normalized transcript: %#v", normalized)
	}
	if got := normalized[0].ToolCalls; len(got) != 2 || got[0].ID != valid.ID || got[1].ID != missing.ID {
		t.Fatalf("ambiguous calls were not removed atomically: %#v", got)
	}
	if normalized[1].ToolCallID != missing.ID || !IsUnknownToolEffectResult(normalized[1].Content) {
		t.Fatalf("unique missing result was not repaired: %#v", normalized[1])
	}
	if normalized[2].ToolCallID != valid.ID || normalized[2].Content != validResult.Content ||
		normalized[2].ToolResult.ResultRetention != agent.ToolResultProtected {
		t.Fatalf("valid rich result was not preserved: %#v", normalized[2])
	}
}

func TestNormalizeModelContextMessagesRepairsMissingCallAndDropsLateOrphan(t *testing.T) {
	call := contextNormalizerTestCall("late-result", "write", `{}`)
	input := []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{call}),
		agent.UserMessage("new turn"),
		agent.ToolMessage(agent.TextToolResult("late"), call.ID),
	}

	normalized, err := NormalizeModelContextMessages(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(normalized) != 3 || normalized[0].Role != agent.Assistant || normalized[1].ToolCallID != call.ID ||
		!IsUnknownToolEffectResult(normalized[1].Content) || normalized[2].Role != agent.User || normalized[2].Content != "new turn" {
		t.Fatalf("missing call and late orphan were not normalized independently: %#v", normalized)
	}
}

func TestNormalizeModelContextMessagesScopesCallIdentityToOneAssistantBatch(t *testing.T) {
	firstCall := contextNormalizerTestCall("provider-local-id", "read", `{"path":"one.md"}`)
	secondCall := contextNormalizerTestCall("provider-local-id", "read", `{"path":"two.md"}`)
	input := []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{firstCall}),
		agent.ToolMessage(agent.TextToolResult("one"), firstCall.ID),
		agent.UserMessage("next"),
		agent.AssistantMessage("", []agent.ToolCall{secondCall}),
	}

	normalized, err := NormalizeModelContextMessages(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(normalized) != 5 || !reflect.DeepEqual(normalized[:4], input) ||
		normalized[4].ToolCallID != secondCall.ID || !IsUnknownToolEffectResult(normalized[4].Content) {
		t.Fatalf("provider-local call id reuse suppressed the later recovery result: %#v", normalized)
	}
}

func TestNormalizeModelContextMessagesRejectsIrreparableProtocol(t *testing.T) {
	_, err := NormalizeModelContextMessages([]*agent.Message{{Role: agent.RoleType("unsupported"), Content: "unsupported"}})
	if !errors.Is(err, ErrInvalidModelContextProtocol) {
		t.Fatalf("unsupported role error = %v, want %v", err, ErrInvalidModelContextProtocol)
	}
}

func contextNormalizerTestCall(id, name, arguments string) agent.ToolCall {
	return agent.ToolCall{
		ID: id, Type: "function", Function: agent.FunctionCall{Name: name, Arguments: arguments},
	}
}
