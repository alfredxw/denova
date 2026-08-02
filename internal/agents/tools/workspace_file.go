package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	agent "github.com/alfredxw/denova/agent"
	agenttools "github.com/alfredxw/denova/agent/tools"

	workspacechange "denova/internal/workspace/change"
)

type workspaceChangeService interface {
	Workspace() string
	ReadFile(string) (content string, revision string, err error)
	ApplyEdits(context.Context, workspacechange.ApplyEditsRequest) (workspacechange.ChangeSet, error)
	ReplaceFile(context.Context, workspacechange.ReplaceFileRequest) (workspacechange.ChangeSet, error)
}

type WorkspaceMetadataProvider func(context.Context) workspacechange.ChangeMetadata

type workspaceMutationAdapter struct {
	changes   workspaceChangeService
	workspace string
	metadata  WorkspaceMetadataProvider
}

// newWorkspaceMutationAdapter bridges the reusable write/edit Interface to
// Denova's durable review and optimistic-concurrency implementation.
func newWorkspaceMutationAdapter(changes workspaceChangeService, metadataProviders ...WorkspaceMetadataProvider) (agenttools.MutationAdapter, error) {
	if changes == nil {
		return nil, fmt.Errorf("workspace change service is nil")
	}
	workspace, err := canonicalChangeWorkspace(changes)
	if err != nil {
		return nil, err
	}
	return &workspaceMutationAdapter{
		changes: changes, workspace: workspace,
		metadata: firstWorkspaceMetadataProvider(metadataProviders),
	}, nil
}

func (adapter *workspaceMutationAdapter) Edit(ctx context.Context, request agenttools.EditRequest) (agent.ToolResult, error) {
	if adapter == nil || adapter.changes == nil {
		return agent.ToolResult{}, fmt.Errorf("workspace mutation adapter is not configured")
	}
	baseRevision, err := currentWorkspaceBaseRevision(adapter.changes, request.Path)
	if err != nil {
		return agent.ToolResult{}, err
	}
	edits := make([]workspacechange.TextEdit, len(request.Edits))
	for index, edit := range request.Edits {
		edits[index] = workspacechange.TextEdit{
			OldString: edit.OldString, NewString: edit.NewString, ReplaceAll: edit.ReplaceAll,
		}
	}
	changeSet, err := adapter.changes.ApplyEdits(ctx, workspacechange.ApplyEditsRequest{
		Path: request.Path, BaseRevision: baseRevision,
		Edits:    edits,
		Metadata: workspaceChangeMetadata(ctx, adapter.metadata),
	})
	if err != nil {
		return agent.ToolResult{}, err
	}
	return workspaceChangeToolResult(adapter.workspace, changeSet)
}

func (adapter *workspaceMutationAdapter) Write(ctx context.Context, request agenttools.WriteRequest) (agent.ToolResult, error) {
	if adapter == nil || adapter.changes == nil {
		return agent.ToolResult{}, fmt.Errorf("workspace mutation adapter is not configured")
	}
	baseRevision, err := currentWorkspaceBaseRevisionOrMissing(adapter.changes, request.Path)
	if err != nil {
		return agent.ToolResult{}, err
	}
	changeSet, err := adapter.changes.ReplaceFile(ctx, workspacechange.ReplaceFileRequest{
		Path: request.Path, Content: request.Content, BaseRevision: baseRevision,
		Metadata: workspaceChangeMetadata(ctx, adapter.metadata),
	})
	if err != nil {
		return agent.ToolResult{}, err
	}
	return workspaceChangeToolResult(adapter.workspace, changeSet)
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

func firstWorkspaceMetadataProvider(providers []WorkspaceMetadataProvider) WorkspaceMetadataProvider {
	if len(providers) == 0 {
		return nil
	}
	return providers[0]
}

func workspaceChangeMetadata(ctx context.Context, provider WorkspaceMetadataProvider) workspacechange.ChangeMetadata {
	if provider != nil {
		return provider(ctx)
	}
	callID := agent.ToolExecutionID(ctx, strings.TrimSpace(agent.ToolCallID(ctx)))
	return workspacechange.ChangeMetadata{
		Origin:        workspacechange.OriginAgent,
		ChangeGroupID: callID,
		ToolCallID:    callID,
	}
}

func workspaceChangeToolResult(workspace string, changeSet workspacechange.ChangeSet) (agent.ToolResult, error) {
	content, err := workspacechange.MarshalToolReceipt(workspace, changeSet)
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("serialize workspace change receipt: %w", err)
	}
	result := agent.TextToolResult(content)
	result.Details = json.RawMessage(content)
	return result, nil
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

// FormatWorkspaceChangeError converts structured workspace-change failures to
// the stable tool error protocol used by the Agent middleware.
func FormatWorkspaceChangeError(toolName string, err error) (string, bool) {
	return formatWorkspaceChangeToolError(toolName, err)
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
