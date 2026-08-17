package agent

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
)

type toolNameMiddleware struct {
	BaseMiddleware
	mu    sync.Mutex
	names []string
}

func (middleware *toolNameMiddleware) WrapToolCall(
	_ context.Context,
	endpoint ToolCallEndpoint,
	toolContext *ToolContext,
) (ToolCallEndpoint, error) {
	return func(ctx context.Context, arguments string, options ...ToolOption) (ToolResult, error) {
		middleware.mu.Lock()
		middleware.names = append(middleware.names, toolContext.Name)
		middleware.mu.Unlock()
		return endpoint(ctx, arguments, options...)
	}, nil
}

func TestNestedToolsReenterExecutorWithoutAddingTranscriptMessages(t *testing.T) {
	model := &scriptedModel{responses: []scriptedModelResponse{
		{message: AssistantMessage("", []ToolCall{{
			ID: "outer-provider", Type: "function", Function: FunctionCall{Name: "outer", Arguments: `{}`},
		}})},
		{message: AssistantMessage("done", nil)},
	}}
	child := schedulerDefinition("child", schedulerReadDescriptor(SteeringFinishCurrent), func(_ context.Context, arguments string) (ToolResult, error) {
		return TextToolResult(arguments), nil
	})
	outer := schedulerDefinition("outer", schedulerChildDescriptor(), func(ctx context.Context, _ string) (ToolResult, error) {
		outcomes, err := CallNestedTools(ctx, []NestedToolCall{
			{Name: "child", Arguments: json.RawMessage(`{"value":1}`)},
			{Name: "missing", Arguments: json.RawMessage(`{}`)},
		})
		if err != nil {
			return ToolResult{}, err
		}
		encoded, err := json.Marshal(outcomes)
		if err != nil {
			return ToolResult{}, err
		}
		return TextToolResult(string(encoded)), nil
	})
	middleware := &toolNameMiddleware{}
	native, err := newModelToolLoop(context.Background(), loopConfig{
		Name: "nested", Model: model, Tools: []ToolDefinition{outer, child}, Middlewares: []Middleware{middleware},
	})
	if err != nil {
		t.Fatal(err)
	}
	iterator := newLoopRunner(loopRunnerConfig{Agent: native}).Query(context.Background(), "go")
	toolMessages := 0
	var outerID string
	var children []toolExecutionEvent
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			t.Fatal(event.Err)
		}
		if event.Output == nil {
			continue
		}
		if message := event.Output.MessageOutput; message != nil && message.Role == ToolRole {
			toolMessages++
		}
		if execution := event.Output.ToolExecution; execution != nil {
			if execution.ToolName == "outer" {
				outerID = execution.ExecutionID
			} else if execution.ParentCallID != "" && execution.Phase == toolExecutionFinished {
				children = append(children, *execution)
			}
		}
	}
	if toolMessages != 1 {
		t.Fatalf("tool transcript messages = %d, want only outer result", toolMessages)
	}
	if len(children) != 2 || outerID == "" || children[0].ParentCallID != outerID || children[1].ParentCallID != outerID {
		t.Fatalf("outer=%q children=%+v", outerID, children)
	}
	if children[0].ExecutionID == children[1].ExecutionID || children[0].Index != 0 || children[1].Index != 1 {
		t.Fatalf("nested identities = %+v", children)
	}
	middleware.mu.Lock()
	names := append([]string(nil), middleware.names...)
	middleware.mu.Unlock()
	if len(names) != 2 || names[0] != "outer" || names[1] != "child" {
		t.Fatalf("middleware calls = %v", names)
	}
}

func TestCallNestedToolsFailsClosedOutsideExecution(t *testing.T) {
	if _, err := CallNestedTools(context.Background(), []NestedToolCall{{Name: "read", Arguments: json.RawMessage(`{}`)}}); err == nil {
		t.Fatal("expected unavailable nested executor")
	}
}
