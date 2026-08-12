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

// EditOperation selects the single-file mutation performed by edit. The empty
// value is equivalent to replace so ordinary text edits keep the compact form.
type EditOperation string

const (
	EditOperationReplace EditOperation = "replace"
	EditOperationDelete  EditOperation = "delete"
)

// EditRequest applies one explicit single-file mutation. Replace evaluates all
// exact replacements against the same current snapshot. Delete requires no
// replacements and removes the complete file through the mutation Adapter.
type EditRequest struct {
	Path      string
	Operation EditOperation
	Edits     []EditReplacement
}

// MutationAdapter is the product seam behind write and edit. Identity must
// change with mutation/review semantics. Implementations own concurrency
// control, durable review history, and structured receipts.
type MutationAdapter interface {
	Identity() agent.CapabilityIdentity
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
	if err := validateAdapterIdentity("write MutationAdapter", adapter.Identity()); err != nil {
		return agent.ToolDefinition{}, err
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
	return agent.ToolDefinition{
		Tool: tool, Descriptor: writeDescriptor(options...),
		ImplementationIdentity: toolsetIdentity("tools.write", adapter.Identity()),
	}, err
}

type editInput struct {
	Path      string           `json:"path" jsonschema:"maxLength=4096" jsonschema_description:"Absolute or workspace-relative path of the single file to edit or delete."`
	Operation EditOperation    `json:"operation,omitempty" jsonschema:"enum=replace,enum=delete" jsonschema_description:"Optional operation. Omit for replace. Delete must be explicit and must not include edits."`
	Edits     []editEntryInput `json:"edits,omitempty" jsonschema:"minItems=1,maxItems=256" jsonschema_description:"Required for replace: non-overlapping exact replacements evaluated against the same original file snapshot and committed together. Omit for delete."`
}

type editEntryInput struct {
	OldString  string `json:"old_string" jsonschema:"maxLength=4194304" jsonschema_description:"Exact non-empty text to replace in the original file snapshot."`
	NewString  string `json:"new_string" jsonschema:"maxLength=4194304" jsonschema_description:"Replacement text; an empty string deletes the matched text."`
	ReplaceAll bool   `json:"replace_all,omitempty" jsonschema_description:"Replace every exact occurrence in the original snapshot; otherwise old_string must match exactly once."`
}

// Edit defines one single-file atomic replace or delete operation.
func Edit(adapter MutationAdapter, options ...DefinitionOption) (agent.ToolDefinition, error) {
	if adapter == nil {
		return agent.ToolDefinition{}, errors.New("edit MutationAdapter is nil")
	}
	if err := validateAdapterIdentity("edit MutationAdapter", adapter.Identity()); err != nil {
		return agent.ToolDefinition{}, err
	}
	tool, err := agent.InferTool("edit", `Replace text in or delete one workspace file as one atomic, reviewable change. Omit operation for ordinary replacement and provide edits. File deletion must be explicit with operation=delete and no edits. For replacement, every edits item is matched against the same current file snapshot, not against earlier replacements in the list. Without replace_all, old_string must occur exactly once. All ranges must be non-overlapping; if any item is invalid, the file is not changed. The file may have changed since an earlier read as long as every old_string still matches the current content exactly as required.

将文本替换或整个文件删除作为一次原子、可审阅的工作区变更。普通替换可省略 operation 并提供 edits；删除必须显式使用 operation=delete 且不能提供 edits。替换时，edits 中每一项都匹配同一份当前文件快照，而不是前一项替换后的结果。未设置 replace_all 时，old_string 必须恰好出现一次；所有区间必须互不重叠，任一项无效时文件都不会改变。`, func(ctx context.Context, input editInput) (agent.ToolResult, error) {
		path := strings.TrimSpace(input.Path)
		if path == "" {
			return agent.ToolResult{}, errors.New("edit path is required")
		}
		operation := EditOperation(strings.TrimSpace(string(input.Operation)))
		switch operation {
		case "", EditOperationReplace:
			operation = EditOperationReplace
			if len(input.Edits) == 0 {
				return agent.ToolResult{}, errors.New("edit replace requires at least one edits item")
			}
			if len(input.Edits) > maxMutationEdits {
				return agent.ToolResult{}, fmt.Errorf("edit exceeds the %d-item mutation limit", maxMutationEdits)
			}
		case EditOperationDelete:
			if len(input.Edits) != 0 {
				return agent.ToolResult{}, errors.New("edit delete must not include edits")
			}
		default:
			return agent.ToolResult{}, fmt.Errorf("unsupported edit operation %q", operation)
		}
		replacements := make([]EditReplacement, len(input.Edits))
		for index, edit := range input.Edits {
			replacements[index] = EditReplacement{
				OldString: edit.OldString, NewString: edit.NewString, ReplaceAll: edit.ReplaceAll,
			}
		}
		result, err := adapter.Edit(ctx, EditRequest{Path: path, Operation: operation, Edits: replacements})
		if err == nil && result.Status == "" {
			result.Status = agent.ToolResultSuccess
		}
		return result, err
	})
	return agent.ToolDefinition{
		Tool: tool, Descriptor: writeDescriptor(options...),
		ImplementationIdentity: toolsetIdentity("tools.edit", adapter.Identity()),
	}, err
}
