package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// safeToolMiddleware 将工具执行错误转为可读的错误消息返回给模型。
type safeToolMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
}

type interactiveStoryToolMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
}

func newInteractiveStoryToolMiddleware() *interactiveStoryToolMiddleware {
	return &interactiveStoryToolMiddleware{}
}

func (m *interactiveStoryToolMiddleware) WrapInvokableToolCall(
	_ context.Context,
	endpoint adk.InvokableToolCallEndpoint,
	toolCtx *adk.ToolContext,
) (adk.InvokableToolCallEndpoint, error) {
	return func(ctx context.Context, args string, opts ...tool.Option) (string, error) {
		if isInteractiveStoryWriteTool(toolName(toolCtx)) {
			return interactiveStoryWriteToolBlockedMessage(toolName(toolCtx)), nil
		}
		return endpoint(ctx, args, opts...)
	}, nil
}

func (m *interactiveStoryToolMiddleware) WrapStreamableToolCall(
	_ context.Context,
	endpoint adk.StreamableToolCallEndpoint,
	toolCtx *adk.ToolContext,
) (adk.StreamableToolCallEndpoint, error) {
	return func(ctx context.Context, args string, opts ...tool.Option) (*schema.StreamReader[string], error) {
		if isInteractiveStoryWriteTool(toolName(toolCtx)) {
			return singleChunkReader(interactiveStoryWriteToolBlockedMessage(toolName(toolCtx))), nil
		}
		return endpoint(ctx, args, opts...)
	}, nil
}

func toolName(toolCtx *adk.ToolContext) string {
	if toolCtx == nil {
		return ""
	}
	return toolCtx.Name
}

func isInteractiveStoryWriteTool(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "write_file", "edit_file", "delete_file", "create_file", "move_file", "copy_file", "rename_file", "mkdir", "remove_file":
		return true
	}
	return strings.HasPrefix(name, "write_") ||
		strings.HasPrefix(name, "edit_") ||
		strings.HasPrefix(name, "delete_") ||
		strings.HasPrefix(name, "create_") ||
		strings.HasPrefix(name, "move_") ||
		strings.HasPrefix(name, "copy_") ||
		strings.HasPrefix(name, "rename_")
}

func interactiveStoryWriteToolBlockedMessage(name string) string {
	return fmt.Sprintf("[tool error] 互动故事模式禁止使用写文件工具 %q。请不要修改 workspace 文件，只输出本回合故事正文；状态变化由后端状态 Agent 异步写入 story jsonl。", name)
}

func (m *safeToolMiddleware) WrapInvokableToolCall(
	_ context.Context,
	endpoint adk.InvokableToolCallEndpoint,
	toolCtx *adk.ToolContext,
) (adk.InvokableToolCallEndpoint, error) {
	return func(ctx context.Context, args string, opts ...tool.Option) (string, error) {
		result, err := endpoint(ctx, args, opts...)
		if err != nil {
			if _, ok := compose.IsInterruptRerunError(err); ok {
				return "", err
			}
			return fmt.Sprintf("[tool error] %v", err), nil
		}
		return FilterToolResultForModel(toolName(toolCtx), args, result).Content, nil
	}, nil
}

func (m *safeToolMiddleware) WrapStreamableToolCall(
	_ context.Context,
	endpoint adk.StreamableToolCallEndpoint,
	toolCtx *adk.ToolContext,
) (adk.StreamableToolCallEndpoint, error) {
	return func(ctx context.Context, args string, opts ...tool.Option) (*schema.StreamReader[string], error) {
		sr, err := endpoint(ctx, args, opts...)
		if err != nil {
			if _, ok := compose.IsInterruptRerunError(err); ok {
				return nil, err
			}
			return singleChunkReader(fmt.Sprintf("[tool error] %v", err)), nil
		}
		return filterToolResultReader(sr, toolName(toolCtx), args), nil
	}, nil
}

func singleChunkReader(msg string) *schema.StreamReader[string] {
	r, w := schema.Pipe[string](1)
	_ = w.Send(msg, nil)
	w.Close()
	return r
}

func safeWrapReader(sr *schema.StreamReader[string]) *schema.StreamReader[string] {
	r, w := schema.Pipe[string](64)
	go func() {
		defer w.Close()
		for {
			chunk, err := sr.Recv()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				_ = w.Send(fmt.Sprintf("\n[tool error] %v", err), nil)
				return
			}
			_ = w.Send(chunk, nil)
		}
	}()
	return r
}

func filterToolResultReader(sr *schema.StreamReader[string], toolName, args string) *schema.StreamReader[string] {
	r, w := schema.Pipe[string](1)
	go func() {
		defer w.Close()
		manifest := ManifestForTool(toolName)
		limit := normalizedToolResultLimit(manifest)
		var content strings.Builder
		originalBytes := 0
		for {
			chunk, err := sr.Recv()
			if errors.Is(err, io.EOF) {
				filtered := filteredToolResultFromBody(manifest, args, content.String(), originalBytes, originalBytes > content.Len())
				_ = w.Send(filtered.Content, nil)
				return
			}
			if err != nil {
				_ = w.Send(fmt.Sprintf("\n[tool error] %v", err), nil)
				return
			}
			originalBytes += len(chunk)
			if content.Len() >= limit {
				continue
			}
			remaining := limit - content.Len()
			if len(chunk) <= remaining {
				content.WriteString(chunk)
				continue
			}
			fragment, _ := truncateUTF8Bytes(chunk, remaining)
			content.WriteString(strings.TrimSuffix(fragment, "\n[tool result truncated]"))
		}
	}()
	return r
}
