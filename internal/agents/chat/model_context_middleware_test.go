package chat

import (
	"context"
	"testing"

	"denova/internal/agents/toolresult"

	agent "github.com/alfredxw/denova/agent"
)

func TestModelContextMiddlewaresContainProjectionThenNormalizer(t *testing.T) {
	middlewares := NewModelContextMiddlewares(toolresult.ContextPolicy{Enabled: true})
	if len(middlewares) != 2 {
		t.Fatalf("model context middleware count = %d, want 2", len(middlewares))
	}
	if _, ok := middlewares[0].(*modelHistoryProjectionMiddleware); !ok {
		t.Fatalf("first model context middleware = %T, want history projection", middlewares[0])
	}
	if _, ok := middlewares[1].(*contextNormalizerMiddleware); !ok {
		t.Fatalf("second model context middleware = %T, want normalizer", middlewares[1])
	}
}

func TestModelHistoryProjectionHidesDisabledSettledToolsButPreservesProviderReasoning(t *testing.T) {
	call := agent.ToolCall{ID: "historical", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{}`}}
	current := agent.ToolCall{ID: "current", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{}`}}
	middleware := NewModelHistoryProjectionMiddleware(toolresult.ContextPolicy{Enabled: false})
	historicalAnswer := agent.AssistantMessage("old answer", nil)
	historicalAnswer.ReasoningContent = "private historical reasoning"
	currentCall := agent.AssistantMessage("", []agent.ToolCall{current})
	currentCall.ReasoningContent = "current tool reasoning"
	_, projected, err := middleware.BeforeModelCall(context.Background(), &agent.ModelCall{Messages: []*agent.Message{
		agent.UserMessage("old request"),
		agent.AssistantMessage("", []agent.ToolCall{call}),
		{Role: agent.ToolRole, ToolCallID: "historical", ToolName: "read", Content: "old result"},
		historicalAnswer,
		agent.UserMessage("current request"),
		currentCall,
		{Role: agent.ToolRole, ToolCallID: "current", ToolName: "read", Content: "current result"},
	}}, &agent.ModelContext{})
	if err != nil {
		t.Fatal(err)
	}
	if len(projected.Messages) != 5 {
		t.Fatalf("projected messages = %#v", projected.Messages)
	}
	for _, message := range projected.Messages {
		if message != nil && message.ToolCallID == "historical" {
			t.Fatalf("historical tool result remained model-visible: %#v", projected.Messages)
		}
	}
	if projected.Messages[1].ReasoningContent != "private historical reasoning" {
		t.Fatalf("historical provider reasoning was filtered: %#v", projected.Messages)
	}
	if currentCall.ReasoningContent != "current tool reasoning" || projected.Messages[len(projected.Messages)-2].ReasoningContent != "current tool reasoning" {
		t.Fatalf("current-cycle reasoning was filtered: %#v", projected.Messages)
	}
	last := projected.Messages[len(projected.Messages)-1]
	if last == nil || last.ToolCallID != "current" || last.Content != "current result" {
		t.Fatalf("current tool batch was filtered: %#v", projected.Messages)
	}
}

func TestContextNormalizerEmitsBoundedRepairMetric(t *testing.T) {
	missing := agent.ToolCall{ID: "missing", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{}`}}
	call := &agent.ModelCall{Messages: []*agent.Message{
		agent.SystemMessage("stable"),
		agent.AssistantMessage("", []agent.ToolCall{missing}),
	}}
	middleware := &contextNormalizerMiddleware{BaseMiddleware: &agent.BaseMiddleware{}}
	modelContext := &agent.ModelContext{}
	_, normalized, err := middleware.BeforeModelCall(context.Background(), call, modelContext)
	if err != nil {
		t.Fatal(err)
	}
	if len(normalized.Messages) != 3 {
		t.Fatalf("normalized messages = %#v", normalized.Messages)
	}
	metrics, ok := modelContext.ContextNormalization()
	if !ok || metrics != (agent.ContextNormalizationMetrics{RepairCount: 1, MessagesBefore: 2, MessagesAfter: 3}) {
		t.Fatalf("normalization metrics = %#v, %v", metrics, ok)
	}
}
