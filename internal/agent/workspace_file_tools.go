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
- file_path must identify one file inside the current workspace.
- Every item in edits is matched against the same original file snapshot, not against the result of an earlier item.
- Keep edits non-overlapping. Use replace_all only when every exact occurrence should change.
- Put dependent changes to the same file in one call. Independent files may use separate edit_file calls in the same assistant response.
- The tool captures and protects the current file snapshot internally when the call starts.

将一个或多个精确文本修改作为一次可审阅变更应用到同一个 workspace 文件。
- file_path 必须指向当前 workspace 内的单个文件。
- edits 中的每一项都基于同一份原始文件快照匹配，不基于前一项修改后的结果。
- 各修改区间不得重叠；只有确实需要替换全部精确匹配时才使用 replace_all。
- 同一文件内相互依赖的修改必须放在一次调用中；不同文件的独立修改可以在同一轮分别调用 edit_file。
- 工具会在调用开始时自行获取并保护当前文件快照。`)

var workspaceWriteFileToolDescription = strings.TrimSpace(`Replace the complete content of one workspace file as a reviewed change.
- Use edit_file for localized changes; use write_file only for a new file or an intentional full rewrite.
- file_path must identify one file inside the current workspace.
- The tool detects whether the file exists and protects its current snapshot internally.

将一个 workspace 文件的完整内容替换为新内容，并记录为可审阅变更。
- 局部修改使用 edit_file；只有新建文件或明确需要整体重写时才使用 write_file。
- file_path 必须指向当前 workspace 内的单个文件。
- 工具会自行判断文件是否存在并保护当前快照。`)

type workspaceChangeService interface {
	Workspace() string
	ReadFile(string) (content string, revision string, err error)
	ApplyEdits(context.Context, workspacechange.ApplyEditsRequest) (workspacechange.ChangeSet, error)
	ReplaceFile(context.Context, workspacechange.ReplaceFileRequest) (workspacechange.ChangeSet, error)
}

type workspaceEditFileInput struct {
	FilePath string                      `json:"file_path" jsonschema:"required,description=Absolute or workspace-relative path of the single file to edit"`
	Edits    []workspaceEditFileTextEdit `json:"edits" jsonschema:"required,description=One or more non-overlapping exact replacements evaluated against the same original file snapshot"`
}

type workspaceEditFileTextEdit struct {
	ID         string `json:"id,omitempty" jsonschema:"description=Optional stable identifier used to associate review comments with this edit"`
	OldString  string `json:"old_string" jsonschema:"required,description=Exact non-empty text to replace in the original file snapshot"`
	NewString  string `json:"new_string" jsonschema:"description=Replacement text; an empty string deletes the matched text"`
	ReplaceAll bool   `json:"replace_all,omitempty" jsonschema:"description=Replace every exact occurrence of old_string; defaults to false"`
}

type workspaceWriteFileInput struct {
	FilePath string `json:"file_path" jsonschema:"required,description=Absolute or workspace-relative path of the file to replace"`
	Content  string `json:"content" jsonschema:"description=Complete new file content"`
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
		baseRevision, err := currentWorkspaceBaseRevision(changes, input.FilePath)
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
		baseRevision, err := currentWorkspaceBaseRevisionOrMissing(changes, input.FilePath)
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

func currentWorkspaceBaseRevision(changes workspaceChangeService, path string) (string, error) {
	_, revision, err := changes.ReadFile(path)
	if err != nil {
		return "", err
	}
	revision = strings.TrimSpace(revision)
	if revision != "" {
		return revision, nil
	}
	return "", &workspacechange.Error{
		Code:    workspacechange.ErrorCodeConflict,
		Message: "workspace change service returned an empty current revision",
		Details: map[string]any{"path": path, "workspace_mutated": false},
	}
}

func currentWorkspaceBaseRevisionOrMissing(changes workspaceChangeService, path string) (string, error) {
	revision, err := currentWorkspaceBaseRevision(changes, path)
	if err == nil {
		return revision, nil
	}
	var changeErr *workspacechange.Error
	if errors.As(err, &changeErr) && changeErr.Code == workspacechange.ErrorCodeNotFound {
		return "missing", nil
	}
	return "", err
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
		Message:          workspaceChangeToolPublicErrorMessage(changeErr),
		Details:          workspaceChangeToolPublicErrorDetails(changeErr.Details),
		Retryable:        workspaceChangeErrorRetryable(changeErr.Code),
		WorkspaceMutated: workspaceChangeErrorMutated(changeErr),
	}
	data, marshalErr := json.Marshal(receipt)
	if marshalErr != nil {
		return "", false
	}
	return "[tool error]\n" + string(data), true
}

func workspaceChangeToolPublicErrorMessage(changeErr *workspacechange.Error) string {
	if changeErr != nil && changeErr.Code == workspacechange.ErrorCodeRevisionConflict {
		return "Workspace file changed during the tool call; retry the operation. / 工具调用期间文件发生变化，请重试。"
	}
	if changeErr == nil {
		return ""
	}
	return changeErr.Message
}

func workspaceChangeToolPublicErrorDetails(details map[string]any) map[string]any {
	if len(details) == 0 {
		return nil
	}
	public := make(map[string]any, len(details))
	for key, value := range details {
		if strings.Contains(strings.ToLower(key), "revision") {
			continue
		}
		public[key] = value
	}
	if len(public) == 0 {
		return nil
	}
	return public
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
