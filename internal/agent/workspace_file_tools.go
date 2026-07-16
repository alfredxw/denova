package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"

	"denova/internal/workspacechange"
)

var workspaceEditFileToolDescription = strings.TrimSpace(`Apply one or more exact text edits to a single workspace file as one reviewed change.
- Read the file before editing it.
- file_path must identify one file inside the current workspace.
- Every item in edits is matched against the same original file snapshot, not against the result of an earlier item.
- Keep edits non-overlapping. Use replace_all only when every exact occurrence should change.
- Put dependent changes to the same file in one call. Independent files may use separate edit_file calls in the same assistant response.
- base_revision is required and must be the complete-file revision returned by read_file. A stale revision rejects the whole call without writing.

将一个或多个精确文本修改作为一次可审阅变更应用到同一个 workspace 文件。
- 编辑前必须先读取文件。
- file_path 必须指向当前 workspace 内的单个文件。
- edits 中的每一项都基于同一份原始文件快照匹配，不基于前一项修改后的结果。
- 各修改区间不得重叠；只有确实需要替换全部精确匹配时才使用 replace_all。
- 同一文件内相互依赖的修改必须放在一次调用中；不同文件的独立修改可以在同一轮分别调用 edit_file。
- base_revision 必填，必须使用 read_file 返回的完整文件 revision；版本过期时整次调用将零写入失败。`)

var workspaceWriteFileToolDescription = strings.TrimSpace(`Replace the complete content of one workspace file as a reviewed change.
- Use edit_file for localized changes; use write_file only for a new file or an intentional full rewrite.
- file_path must identify one file inside the current workspace.
- base_revision is required. Use the complete-file revision returned by read_file when replacing an existing file; use the literal "missing" only when intentionally creating a new file.

将一个 workspace 文件的完整内容替换为新内容，并记录为可审阅变更。
- 局部修改使用 edit_file；只有新建文件或明确需要整体重写时才使用 write_file。
- file_path 必须指向当前 workspace 内的单个文件。
- base_revision 必填。覆盖已有文件时使用 read_file 返回的完整文件 revision；仅在明确新建文件时使用字面值 "missing"。`)

type workspaceChangeService interface {
	Workspace() string
	ApplyEdits(context.Context, workspacechange.ApplyEditsRequest) (workspacechange.ChangeSet, error)
	ReplaceFile(context.Context, workspacechange.ReplaceFileRequest) (workspacechange.ChangeSet, error)
}

type workspaceEditFileInput struct {
	FilePath     string                      `json:"file_path" jsonschema:"required,description=Absolute or workspace-relative path of the single file to edit"`
	BaseRevision string                      `json:"base_revision" jsonschema:"required,description=Complete-file sha256 revision returned by read_file"`
	Edits        []workspaceEditFileTextEdit `json:"edits" jsonschema:"required,description=One or more non-overlapping exact replacements evaluated against the same original file snapshot"`
}

type workspaceEditFileTextEdit struct {
	ID         string `json:"id,omitempty" jsonschema:"description=Optional stable identifier used to associate review comments with this edit"`
	OldString  string `json:"old_string" jsonschema:"required,description=Exact non-empty text to replace in the original file snapshot"`
	NewString  string `json:"new_string" jsonschema:"description=Replacement text; an empty string deletes the matched text"`
	ReplaceAll bool   `json:"replace_all,omitempty" jsonschema:"description=Replace every exact occurrence of old_string; defaults to false"`
}

type workspaceWriteFileInput struct {
	FilePath     string `json:"file_path" jsonschema:"required,description=Absolute or workspace-relative path of the file to replace"`
	Content      string `json:"content" jsonschema:"description=Complete new file content"`
	BaseRevision string `json:"base_revision" jsonschema:"required,description=Complete-file sha256 revision from read_file; use missing only to create a new file"`
}

func newWorkspaceEditFileTool(changes workspaceChangeService) (tool.BaseTool, error) {
	if changes == nil {
		return nil, fmt.Errorf("workspace change service is nil")
	}
	workspace, err := canonicalChangeWorkspace(changes)
	if err != nil {
		return nil, err
	}
	return utils.InferTool("edit_file", workspaceEditFileToolDescription, func(ctx context.Context, input workspaceEditFileInput) (string, error) {
		baseRevision, err := requiredBaseRevision(input.FilePath, input.BaseRevision, false)
		if err != nil {
			return "", err
		}
		edits := make([]workspacechange.TextEdit, 0, len(input.Edits))
		for _, edit := range input.Edits {
			edits = append(edits, workspacechange.TextEdit{
				ID:         edit.ID,
				OldString:  edit.OldString,
				NewString:  edit.NewString,
				ReplaceAll: edit.ReplaceAll,
			})
		}
		changeSet, err := changes.ApplyEdits(ctx, workspacechange.ApplyEditsRequest{
			Path:         input.FilePath,
			BaseRevision: baseRevision,
			Edits:        edits,
			Metadata:     workspaceChangeMetadata(ctx),
		})
		if err != nil {
			return "", err
		}
		return marshalWorkspaceChangeToolReceipt(workspace, changeSet)
	})
}

func newWorkspaceWriteFileTool(changes workspaceChangeService) (tool.BaseTool, error) {
	if changes == nil {
		return nil, fmt.Errorf("workspace change service is nil")
	}
	workspace, err := canonicalChangeWorkspace(changes)
	if err != nil {
		return nil, err
	}
	return utils.InferTool("write_file", workspaceWriteFileToolDescription, func(ctx context.Context, input workspaceWriteFileInput) (string, error) {
		baseRevision, err := requiredBaseRevision(input.FilePath, input.BaseRevision, true)
		if err != nil {
			return "", err
		}
		changeSet, err := changes.ReplaceFile(ctx, workspacechange.ReplaceFileRequest{
			Path:         input.FilePath,
			Content:      input.Content,
			BaseRevision: baseRevision,
			Metadata:     workspaceChangeMetadata(ctx),
		})
		if err != nil {
			return "", err
		}
		return marshalWorkspaceChangeToolReceipt(workspace, changeSet)
	})
}

func canonicalChangeWorkspace(changes workspaceChangeService) (string, error) {
	workspace := strings.TrimSpace(changes.Workspace())
	if workspace == "" {
		return "", fmt.Errorf("workspace change service has no workspace identity")
	}
	if !filepath.IsAbs(workspace) {
		return "", fmt.Errorf("workspace change service path is not absolute: %s", workspace)
	}
	return filepath.Clean(workspace), nil
}

func requiredBaseRevision(path, revision string, allowMissing bool) (string, error) {
	revision = strings.TrimSpace(revision)
	if revision == "" {
		return "", &workspacechange.Error{
			Code:    workspacechange.ErrorCodeInvalidEdit,
			Message: "base_revision is required; read the complete file before changing it",
			Details: map[string]any{"path": path, "workspace_mutated": false, "required_source": "read_file"},
		}
	}
	if revision == "missing" && !allowMissing {
		return "", &workspacechange.Error{
			Code:    workspacechange.ErrorCodeInvalidEdit,
			Message: "edit_file requires the sha256 revision of an existing file",
			Details: map[string]any{"path": path, "workspace_mutated": false, "required_source": "read_file"},
		}
	}
	return revision, nil
}

func workspaceChangeMetadata(ctx context.Context) workspacechange.ChangeMetadata {
	callID := strings.TrimSpace(compose.GetToolCallID(ctx))
	runID := ""
	sessionID := ""
	reviewThreadID := ""
	if observer := RunObserverFromContext(ctx); observer != nil {
		runID = strings.TrimSpace(observer.RunID())
		sessionID = strings.TrimSpace(observer.SessionID())
		reviewThreadID = strings.TrimSpace(observer.ReviewThreadID())
	}
	groupID := runID
	if groupID == "" {
		groupID = callID
	}
	return workspacechange.ChangeMetadata{
		Origin:         workspacechange.OriginAgent,
		ChangeGroupID:  groupID,
		RunID:          runID,
		SessionID:      sessionID,
		ReviewThreadID: reviewThreadID,
		ToolCallID:     callID,
	}
}

func marshalWorkspaceChangeToolReceipt(workspace string, changeSet workspacechange.ChangeSet) (string, error) {
	receipt := workspaceChangeToolReceipt{
		Schema:         workspaceChangeToolResultSchema,
		Status:         workspaceChangeReceiptStatus(changeSet),
		Workspace:      workspace,
		ChangeGroupID:  changeSet.GroupID,
		ReviewThreadID: changeSet.ReviewThreadID,
		ChangeSetID:    changeSet.ID,
		Path:           changeSet.Path,
		BaseRevision:   changeSet.BaseRevision,
		Revision:       changeSet.Revision,
		ReviewStatus:   changeSet.ReviewStatus,
		ApplyState:     changeSet.ApplyState,
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return "", fmt.Errorf("serialize workspace change receipt: %w", err)
	}
	return string(data), nil
}

func workspaceChangeReceiptStatus(changeSet workspacechange.ChangeSet) string {
	if strings.TrimSpace(changeSet.ApplyState) == "" || changeSet.ApplyState == workspacechange.ApplyStateApplied {
		return "applied"
	}
	return changeSet.ApplyState
}

type workspaceChangeToolErrorReceipt struct {
	Schema           string         `json:"schema"`
	Status           string         `json:"status"`
	Tool             string         `json:"tool"`
	Code             string         `json:"code"`
	Message          string         `json:"message"`
	Details          map[string]any `json:"details,omitempty"`
	Retryable        bool           `json:"retryable"`
	WorkspaceMutated bool           `json:"workspace_mutated"`
}

func formatWorkspaceChangeToolError(toolName string, err error) (string, bool) {
	var changeErr *workspacechange.Error
	if !errors.As(err, &changeErr) || changeErr == nil {
		return "", false
	}
	receipt := workspaceChangeToolErrorReceipt{
		Schema:           "workspace_change.tool_error.v1",
		Status:           "rejected",
		Tool:             normalizeToolName(toolName),
		Code:             changeErr.Code,
		Message:          changeErr.Message,
		Details:          changeErr.Details,
		Retryable:        workspaceChangeErrorRetryable(changeErr.Code),
		WorkspaceMutated: workspaceChangeErrorMutated(changeErr),
	}
	data, marshalErr := json.Marshal(receipt)
	if marshalErr != nil {
		return "", false
	}
	return "[tool error]\n" + string(data), true
}

func workspaceChangeErrorMutated(changeErr *workspacechange.Error) bool {
	if changeErr == nil || changeErr.Details == nil {
		return false
	}
	mutated, _ := changeErr.Details["workspace_mutated"].(bool)
	return mutated
}

func workspaceChangeErrorRetryable(code string) bool {
	switch code {
	case workspacechange.ErrorCodeInvalidEdit,
		workspacechange.ErrorCodeRevisionConflict,
		workspacechange.ErrorCodeNotFound,
		workspacechange.ErrorCodeConflict,
		workspacechange.ErrorCodeDurabilityPending:
		return true
	default:
		return false
	}
}
