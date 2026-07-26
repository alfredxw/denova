package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

// WriteRequest replaces one complete workspace file.
type WriteRequest struct {
	Path    string
	Content string
}

// EditRequest applies one exact current-content replacement.
type EditRequest struct {
	Path       string
	OldString  string
	NewString  string
	ReplaceAll bool
}

// MutationAdapter is the product seam behind write and edit. Implementations
// own concurrency control, durable review history, and structured receipts.
type MutationAdapter interface {
	Write(context.Context, WriteRequest) (agent.ToolResult, error)
	Edit(context.Context, EditRequest) (agent.ToolResult, error)
}

type writeInput struct {
	Path    string `json:"path" jsonschema:"maxLength=4096" jsonschema_description:"Absolute or workspace-relative path of the file to create or completely replace."`
	Content string `json:"content" jsonschema:"maxLength=16777216" jsonschema_description:"Complete new file content, up to the mutation safety limit."`
}

// Write defines the complete-file replacement tool.
func Write(adapter MutationAdapter, options ...DefinitionOption) (agent.ToolDefinition, error) {
	if adapter == nil {
		return agent.ToolDefinition{}, errors.New("write MutationAdapter is nil")
	}
	tool, err := agent.InferTool("write", `Create or completely replace one workspace file through the configured mutation adapter. Use edit for localized changes.

通过已配置的变更适配器创建或完整替换一个工作区文件；局部修改请使用 edit。`, func(ctx context.Context, input writeInput) (agent.ToolResult, error) {
		path := strings.TrimSpace(input.Path)
		if path == "" {
			return agent.ToolResult{}, errors.New("write path is required")
		}
		if len(input.Content) > maxMutationFileBytes {
			return agent.ToolResult{}, fmt.Errorf("write content exceeds the %d-byte mutation limit", maxMutationFileBytes)
		}
		result, err := adapter.Write(ctx, WriteRequest{Path: path, Content: input.Content})
		if err == nil && result.Status == "" {
			result.Status = agent.ToolResultSuccess
		}
		return result, err
	})
	return agent.ToolDefinition{Tool: tool, Descriptor: writeDescriptor(options...)}, err
}

type editInput struct {
	Path       string `json:"path" jsonschema:"maxLength=4096" jsonschema_description:"Absolute or workspace-relative path of the file to edit."`
	OldString  string `json:"old_string" jsonschema:"maxLength=4194304" jsonschema_description:"Exact non-empty current text to replace, up to the mutation fragment safety limit."`
	NewString  string `json:"new_string" jsonschema:"maxLength=4194304" jsonschema_description:"Replacement text, up to the mutation fragment safety limit; an empty string deletes the matched text."`
	ReplaceAll bool   `json:"replace_all,omitempty" jsonschema_description:"Replace every exact occurrence; otherwise old_string must match exactly once."`
}

// Edit defines one Claude Code-style exact string replacement.
func Edit(adapter MutationAdapter, options ...DefinitionOption) (agent.ToolDefinition, error) {
	if adapter == nil {
		return agent.ToolDefinition{}, errors.New("edit MutationAdapter is nil")
	}
	tool, err := agent.InferTool("edit", `Replace exact current text in one workspace file. Without replace_all, old_string must occur exactly once. The file may have changed since an earlier read as long as old_string still matches the current content exactly and uniquely.

在一个工作区文件的当前内容中执行精确文本替换。未设置 replace_all 时，old_string 必须恰好出现一次；只要它仍能在当前内容中精确且唯一匹配，文件可以在之前读取后发生其它变化。`, func(ctx context.Context, input editInput) (agent.ToolResult, error) {
		path := strings.TrimSpace(input.Path)
		if path == "" {
			return agent.ToolResult{}, errors.New("edit path is required")
		}
		if input.OldString == "" {
			return agent.ToolResult{}, errors.New("edit old_string must not be empty")
		}
		if input.OldString == input.NewString {
			return agent.ToolResult{}, errors.New("edit new_string must differ from old_string")
		}
		if len(input.OldString) > maxMutationFragmentBytes || len(input.NewString) > maxMutationFragmentBytes {
			return agent.ToolResult{}, fmt.Errorf("edit text exceeds the %d-byte mutation fragment limit", maxMutationFragmentBytes)
		}
		result, err := adapter.Edit(ctx, EditRequest{
			Path: path, OldString: input.OldString, NewString: input.NewString, ReplaceAll: input.ReplaceAll,
		})
		if err == nil && result.Status == "" {
			result.Status = agent.ToolResultSuccess
		}
		return result, err
	})
	return agent.ToolDefinition{Tool: tool, Descriptor: writeDescriptor(options...)}, err
}
