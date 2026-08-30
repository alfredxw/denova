package change

import (
	"encoding/json"
	"strings"
)

// ToolResultSchema identifies the durable receipt emitted by workspace
// mutation tools.
const ToolResultSchema = "workspace_change.tool_result.v1"

const legacyToolResultMetadataHeader = "[Denova tool result metadata]"

// ToolReceipt is the complete durable mutation identity. Base and resulting
// revisions are retained for recovery but removed from model projections.
type ToolReceipt struct {
	Schema string `json:"schema"`
	Status string `json:"status"`
	// Workspace is decoded only for v0.3.3 receipts. Final receipts identify
	// their owner through the enclosing Project Session and persist only a
	// Project-relative Path.
	Workspace      string            `json:"workspace,omitempty"`
	ChangeGroupID  string            `json:"change_group_id"`
	ReviewThreadID string            `json:"review_thread_id"`
	ChangeSetID    string            `json:"change_set_id"`
	Path           string            `json:"path"`
	BaseRevision   string            `json:"base_revision"`
	Revision       string            `json:"revision"`
	ReviewStatus   string            `json:"review_status"`
	ApplyState     string            `json:"apply_state"`
	Edits          []ToolReceiptEdit `json:"edits,omitempty"`
	FileStats      *FileStats        `json:"file_stats,omitempty"`
	Warnings       []string          `json:"warnings,omitempty"`
}

// ToolReceiptEdit is the bounded per-edit projection needed by review and
// recovery consumers.
type ToolReceiptEdit struct {
	ID           string `json:"id,omitempty"`
	Replacements int    `json:"replacements"`
}

type modelToolReceipt struct {
	Schema         string            `json:"schema"`
	Status         string            `json:"status"`
	ChangeGroupID  string            `json:"change_group_id"`
	ReviewThreadID string            `json:"review_thread_id,omitempty"`
	ChangeSetID    string            `json:"change_set_id"`
	Path           string            `json:"path"`
	ReviewStatus   string            `json:"review_status"`
	ApplyState     string            `json:"apply_state"`
	Edits          []ToolReceiptEdit `json:"edits,omitempty"`
	FileStats      *FileStats        `json:"file_stats,omitempty"`
	Warnings       []string          `json:"warnings,omitempty"`
}

// NewToolReceipt projects a committed change set into the stable tool
// protocol without making the tool package own workspace durability fields.
func NewToolReceipt(changeSet ChangeSet, warnings ...string) ToolReceipt {
	edits := make([]ToolReceiptEdit, 0, len(changeSet.Edits))
	for _, edit := range changeSet.Edits {
		edits = append(edits, ToolReceiptEdit{ID: edit.ID, Replacements: len(edit.Hunks)})
	}
	var fileStats *FileStats
	if changeSet.afterFileStats != nil {
		stats := *changeSet.afterFileStats
		fileStats = &stats
	}
	return ToolReceipt{
		Schema: ToolResultSchema, Status: toolReceiptStatus(changeSet),
		ChangeGroupID: changeSet.GroupID, ReviewThreadID: changeSet.ReviewThreadID,
		ChangeSetID: changeSet.ID, Path: changeSet.Path,
		BaseRevision: changeSet.BaseRevision, Revision: changeSet.Revision,
		ReviewStatus: changeSet.ReviewStatus, ApplyState: changeSet.ApplyState, Edits: edits,
		FileStats: fileStats, Warnings: append([]string(nil), warnings...),
	}
}

// MarshalToolReceipt serializes the complete recovery receipt.
func MarshalToolReceipt(changeSet ChangeSet, warnings ...string) (string, error) {
	data, err := json.Marshal(NewToolReceipt(changeSet, warnings...))
	return string(data), err
}

// ParseToolReceipt recognizes trusted mutation tools and validates the durable
// identity before consumers act on it.
func ParseToolReceipt(toolName, content string) (ToolReceipt, bool) {
	if !isReceiptTool(toolName) {
		return ToolReceipt{}, false
	}
	content = strings.TrimSpace(toolResultBody(content))
	if content == "" || !strings.HasPrefix(content, "{") {
		return ToolReceipt{}, false
	}
	var receipt ToolReceipt
	if err := json.Unmarshal([]byte(content), &receipt); err != nil {
		return ToolReceipt{}, false
	}
	if receipt.Schema != ToolResultSchema || strings.TrimSpace(receipt.ChangeGroupID) == "" || strings.TrimSpace(receipt.ChangeSetID) == "" ||
		strings.TrimSpace(receipt.Path) == "" {
		return ToolReceipt{}, false
	}
	return receipt, true
}

// ToolReceiptForModel removes internal revisions while preserving the durable
// change identity needed by the next model step.
func ToolReceiptForModel(toolName, content string) string {
	receipt, ok := ParseToolReceipt(toolName, content)
	if !ok {
		return content
	}
	projection, err := json.Marshal(modelToolReceipt{
		Schema: receipt.Schema, Status: receipt.Status,
		ChangeGroupID: receipt.ChangeGroupID, ReviewThreadID: receipt.ReviewThreadID,
		ChangeSetID: receipt.ChangeSetID, Path: receipt.Path,
		ReviewStatus: receipt.ReviewStatus, ApplyState: receipt.ApplyState, Edits: receipt.Edits,
		FileStats: receipt.FileStats, Warnings: receipt.Warnings,
	})
	if err != nil {
		return content
	}
	return string(projection)
}

func toolReceiptStatus(changeSet ChangeSet) string {
	if strings.TrimSpace(changeSet.ApplyState) == "" || changeSet.ApplyState == ApplyStateApplied {
		return "applied"
	}
	return changeSet.ApplyState
}

func toolResultBody(content string) string {
	content = strings.TrimRight(content, "\n")
	for _, separator := range []string{"\n\n" + legacyToolResultMetadataHeader, "\n" + legacyToolResultMetadataHeader} {
		if before, _, ok := strings.Cut(content, separator); ok {
			return strings.TrimRight(before, "\n")
		}
	}
	if strings.HasPrefix(strings.TrimSpace(content), legacyToolResultMetadataHeader) {
		return ""
	}
	return content
}

func isReceiptTool(toolName string) bool {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "edit", "write":
		return true
	default:
		return false
	}
}
