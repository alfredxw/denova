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

// EditReplacement describes one exact replacement evaluated against the
// shared base snapshot of an EditRequest.
type EditReplacement struct {
	OldString  string
	NewString  string
	ReplaceAll bool
}

// EditRequest applies non-overlapping exact replacements to one file as one
// mutation. Every replacement is evaluated against the same current snapshot.
type EditRequest struct {
	Path  string
	Edits []EditReplacement
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
	Path  string           `json:"path" jsonschema:"maxLength=4096" jsonschema_description:"Absolute or workspace-relative path of the single file to edit."`
	Edits []editEntryInput `json:"edits" jsonschema:"minItems=1,maxItems=256" jsonschema_description:"Non-overlapping exact replacements evaluated against the same original file snapshot and committed together."`
}

type editEntryInput struct {
	OldString  string `json:"old_string" jsonschema:"maxLength=4194304" jsonschema_description:"Exact non-empty text to replace in the original file snapshot."`
	NewString  string `json:"new_string" jsonschema:"maxLength=4194304" jsonschema_description:"Replacement text; an empty string deletes the matched text."`
	ReplaceAll bool   `json:"replace_all,omitempty" jsonschema_description:"Replace every exact occurrence in the original snapshot; otherwise old_string must match exactly once."`
}

// Edit defines one single-file atomic exact-replacement batch.
func Edit(adapter MutationAdapter, options ...DefinitionOption) (agent.ToolDefinition, error) {
	if adapter == nil {
		return agent.ToolDefinition{}, errors.New("edit MutationAdapter is nil")
	}
	tool, err := agent.InferTool("edit", `Apply one or more exact replacements to a single workspace file as one atomic change. Every edits item is matched against the same current file snapshot, not against earlier replacements in the list. Without replace_all, old_string must occur exactly once. All ranges must be non-overlapping; if any item is invalid, the file is not changed. The file may have changed since an earlier read as long as every old_string still matches the current content exactly as required.

将一个或多个精确替换作为一次原子变更应用到同一个工作区文件。edits 中每一项都匹配同一份当前文件快照，而不是前一项替换后的结果。未设置 replace_all 时，old_string 必须恰好出现一次；所有区间必须互不重叠，任一项无效时文件都不会改变。只要每个 old_string 仍按要求精确匹配，文件可以在之前读取后发生其它变化。`, func(ctx context.Context, input editInput) (agent.ToolResult, error) {
		path := strings.TrimSpace(input.Path)
		if path == "" {
			return agent.ToolResult{}, errors.New("edit path is required")
		}
		if len(input.Edits) == 0 {
			return agent.ToolResult{}, errors.New("edit requires at least one edits item")
		}
		if len(input.Edits) > maxMutationEdits {
			return agent.ToolResult{}, fmt.Errorf("edit exceeds the %d-item mutation limit", maxMutationEdits)
		}
		replacements := make([]EditReplacement, len(input.Edits))
		for index, edit := range input.Edits {
			replacements[index] = EditReplacement{
				OldString: edit.OldString, NewString: edit.NewString, ReplaceAll: edit.ReplaceAll,
			}
		}
		result, err := adapter.Edit(ctx, EditRequest{Path: path, Edits: replacements})
		if err == nil && result.Status == "" {
			result.Status = agent.ToolResultSuccess
		}
		return result, err
	})
	return agent.ToolDefinition{Tool: tool, Descriptor: writeDescriptor(options...)}, err
}
