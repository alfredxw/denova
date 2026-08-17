package tools

import (
	"context"
	"fmt"

	agent "github.com/alfredxw/denova/agent"
)

// concreteToolForTest keeps tests focused on product behavior while accepting
// both raw factory tools and model-visible definitions.
func concreteToolForTest(candidate any) (agent.Tool, error) {
	switch value := candidate.(type) {
	case agent.ToolDefinition:
		if value.Tool == nil {
			return nil, fmt.Errorf("tool definition has no implementation")
		}
		return value.Tool, nil
	case *agent.ToolDefinition:
		if value == nil || value.Tool == nil {
			return nil, fmt.Errorf("tool definition has no implementation")
		}
		return value.Tool, nil
	case agent.Tool:
		return value, nil
	default:
		return nil, fmt.Errorf("unsupported tool test value %T", candidate)
	}
}

func toolInfoForTest(ctx context.Context, candidate any) (*agent.ToolInfo, error) {
	tool, err := concreteToolForTest(candidate)
	if err != nil {
		return nil, err
	}
	return tool.Info(ctx)
}

func runToolResultForTest(ctx context.Context, candidate any, arguments string) (agent.ToolResult, error) {
	tool, err := concreteToolForTest(candidate)
	if err != nil {
		return agent.ToolResult{}, err
	}
	return tool.Run(ctx, arguments)
}

func runToolForTest(ctx context.Context, candidate any, arguments string) (string, error) {
	result, err := runToolResultForTest(ctx, candidate, arguments)
	return result.ModelContent, err
}
