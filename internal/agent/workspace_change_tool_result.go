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

func parseWorkspaceChangeToolReceipt(toolName, content string) (workspaceChangeToolReceipt, bool) {
	if !isWorkspaceChangeReceiptTool(toolName) {
		return workspaceChangeToolReceipt{}, false
	}
	content = strings.TrimSpace(stripToolResultMetadata(content))
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

func isWorkspaceChangeReceiptTool(toolName string) bool {
	switch normalizeToolName(toolName) {
	case "edit_file", "write_file":
		return true
	default:
		return false
	}
}
