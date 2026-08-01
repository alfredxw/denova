package chat

import (
	"context"
	"denova/internal/agents/run"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

func TestContextNormalizerRunsImmediatelyBeforeContextMaintenance(t *testing.T) {
	middlewares := NewModelContextMiddlewares("ide", true)
	normalizerIndex := -1
	maintenanceIndex := -1
	for index, middleware := range middlewares {
		switch middleware.(type) {
		case *contextNormalizerMiddleware:
			normalizerIndex = index
		case *contextMaintenanceMiddleware:
			maintenanceIndex = index
		}
	}
	if normalizerIndex < 0 || maintenanceIndex != normalizerIndex+1 {
		t.Fatalf("middleware order normalizer=%d maintenance=%d: %#v", normalizerIndex, maintenanceIndex, middlewares)
	}
}

func TestContextNormalizerEmitsBoundedRepairMetric(t *testing.T) {
	missing := agent.ToolCall{ID: "missing", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{}`}}
	call := &agent.ModelCall{Messages: []*agent.Message{
		agent.SystemMessage("stable"),
		agent.AssistantMessage("", []agent.ToolCall{missing}),
	}}
	var events []agentrun.Event
	ctx := context.WithValue(context.Background(), contextCompactionContextKey{}, &contextCompactionController{
		emit: func(event agentrun.Event) { events = append(events, event) },
	})
	middleware := &contextNormalizerMiddleware{BaseMiddleware: &agent.BaseMiddleware{}}
	_, normalized, err := middleware.BeforeModelCall(ctx, call, &agent.ModelContext{})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := events[0].Data.(map[string]any)
	if len(normalized.Messages) != 3 || len(events) != 1 || events[0].Type != "context_normalizer" ||
		data["context_normalizer_repair_count"] != 1 {
		t.Fatalf("normalizer repair metric/result = events:%#v messages:%#v", events, normalized.Messages)
	}
}
