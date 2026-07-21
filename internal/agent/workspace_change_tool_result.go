package agent

import (
	"encoding/json"
	"strings"
)

const workspaceChangeToolResultSchema = "workspace_change.tool_result.v1"

type workspaceChangeToolReceipt struct {
	Schema         string                       `json:"schema"`
	Status         string                       `json:"status"`
	Workspace      string                       `json:"workspace"`
	ChangeGroupID  string                       `json:"change_group_id"`
	ReviewThreadID string                       `json:"review_thread_id"`
	ChangeSetID    string                       `json:"change_set_id"`
	Path           string                       `json:"path"`
	BaseRevision   string                       `json:"base_revision"`
	Revision       string                       `json:"revision"`
	ReviewStatus   string                       `json:"review_status"`
	ApplyState     string                       `json:"apply_state"`
	Edits          []workspaceChangeEditReceipt `json:"edits,omitempty"`
}

type workspaceChangeEditReceipt struct {
	ID           string `json:"id,omitempty"`
	Replacements int    `json:"replacements"`
}

type workspaceChangeToolModelReceipt struct {
	Schema         string `json:"schema"`
	Status         string `json:"status"`
	Workspace      string `json:"workspace"`
	ChangeGroupID  string `json:"change_group_id"`
	ReviewThreadID string `json:"review_thread_id,omitempty"`
	ChangeSetID    string `json:"change_set_id"`
	Path           string `json:"path"`
	ReviewStatus   string `json:"review_status"`
	ApplyState     string `json:"apply_state"`
}

func workspaceChangeToolResultForModel(toolName, content string) string {
	receipt, ok := parseWorkspaceChangeToolReceipt(toolName, content)
	if !ok {
		return content
	}
	public, err := json.Marshal(workspaceChangeToolModelReceipt{
		Schema:         receipt.Schema,
		Status:         receipt.Status,
		Workspace:      receipt.Workspace,
		ChangeGroupID:  receipt.ChangeGroupID,
		ReviewThreadID: receipt.ReviewThreadID,
		ChangeSetID:    receipt.ChangeSetID,
		Path:           receipt.Path,
		ReviewStatus:   receipt.ReviewStatus,
		ApplyState:     receipt.ApplyState,
	})
	if err != nil {
		return content
	}
	return string(public)
}

func parseWorkspaceChangeToolReceipt(toolName, content string) (workspaceChangeToolReceipt, bool) {
	if !isWorkspaceChangeReceiptTool(toolName) {
		return workspaceChangeToolReceipt{}, false
	}
	content = strings.TrimSpace(toolResultBody(content))
	if content == "" || !strings.HasPrefix(content, "{") {
		return workspaceChangeToolReceipt{}, false
	}
	var receipt workspaceChangeToolReceipt
	if err := json.Unmarshal([]byte(content), &receipt); err != nil {
		return workspaceChangeToolReceipt{}, false
	}
	if receipt.Schema != workspaceChangeToolResultSchema ||
		strings.TrimSpace(receipt.Workspace) == "" ||
		strings.TrimSpace(receipt.ChangeGroupID) == "" ||
		strings.TrimSpace(receipt.ChangeSetID) == "" ||
		strings.TrimSpace(receipt.Path) == "" {
		return workspaceChangeToolReceipt{}, false
	}
	return receipt, true
}

func toolResultBody(content string) string {
	content = strings.TrimRight(content, "\n")
	for _, separator := range []string{"\n\n" + toolResultMetadataHeader, "\n" + toolResultMetadataHeader} {
		if before, _, ok := strings.Cut(content, separator); ok {
			return strings.TrimRight(before, "\n")
		}
	}
	if strings.HasPrefix(strings.TrimSpace(content), toolResultMetadataHeader) {
		return ""
	}
	return content
}

func isWorkspaceChangeReceiptTool(toolName string) bool {
	switch normalizeToolName(toolName) {
	case "edit_file", "write_file":
		return true
	default:
		return false
	}
}
