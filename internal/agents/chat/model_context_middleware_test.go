package chat

import (
	"context"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

func TestModelContextMiddlewaresContainOnlyNormalizer(t *testing.T) {
	middlewares := NewModelContextMiddlewares()
	if len(middlewares) != 1 {
		t.Fatalf("model context middleware count = %d, want 1", len(middlewares))
	}
	if _, ok := middlewares[0].(*contextNormalizerMiddleware); !ok {
		t.Fatalf("model context middleware = %T, want normalizer", middlewares[0])
	}
}

func TestContextNormalizerRepairsToolProtocol(t *testing.T) {
	missing := agent.ToolCall{ID: "missing", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{}`}}
	call := &agent.ModelCall{Messages: []*agent.Message{
		agent.SystemMessage("stable"),
		agent.AssistantMessage("", []agent.ToolCall{missing}),
	}}
	middleware := &contextNormalizerMiddleware{BaseMiddleware: &agent.BaseMiddleware{}}
	_, normalized, err := middleware.BeforeModelCall(context.Background(), call, &agent.ModelContext{})
	if err != nil {
		t.Fatal(err)
	}
	if len(normalized.Messages) != 3 {
		t.Fatalf("normalized messages = %#v", normalized.Messages)
	}
}
