package agents

import (
	"context"

	agent "github.com/alfredxw/denova/agent"
)

type testTextToolEndpoint func(context.Context, string, ...agent.ToolOption) (string, error)

// wrapTextToolCallForTest adapts concise string fixtures to the production
// structured ToolResult middleware seam.
func wrapTextToolCallForTest(
	middleware agent.Middleware,
	endpoint testTextToolEndpoint,
	toolCtx *agent.ToolContext,
) (testTextToolEndpoint, error) {
	wrapped, err := middleware.WrapToolCall(
		context.Background(),
		func(ctx context.Context, arguments string, options ...agent.ToolOption) (agent.ToolResult, error) {
			content, runErr := endpoint(ctx, arguments, options...)
			return agent.TextToolResult(content), runErr
		},
		toolCtx,
	)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, arguments string, options ...agent.ToolOption) (string, error) {
		result, runErr := wrapped(ctx, arguments, options...)
		return result.ModelContent, runErr
	}, nil
}
