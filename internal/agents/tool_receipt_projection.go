package agents

import (
	"encoding/json"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	producttools "denova/internal/agents/tools"
)

func applyInteractiveTurnReceiptToExecutionRecord(record *ToolExecutionRecord, result agent.ToolResult) {
	if record == nil || !IsInteractiveTurnSubmissionTool(record.ToolName) {
		return
	}
	var receipt struct {
		Ready        bool              `json:"ready"`
		ModuleStatus map[string]string `json:"module_status"`
		Diagnostics  []json.RawMessage `json:"diagnostics"`
		RetryModules []string          `json:"retry_modules"`
	}
	if err := json.Unmarshal(toolResultDomainPayload(result), &receipt); err != nil || receipt.ModuleStatus == nil {
		return
	}
	record.DomainDiagnosticCount = len(receipt.Diagnostics)
	record.RetryModules = append([]string(nil), receipt.RetryModules...)
	switch {
	case receipt.Ready:
		record.DomainStatus = "accepted"
	case turnSubmissionReceiptHasStatus(receipt.ModuleStatus, "rejected"):
		record.DomainStatus = "rejected"
	default:
		record.DomainStatus = "pending"
	}
}

func turnSubmissionReceiptHasStatus(statuses map[string]string, target string) bool {
	for _, status := range statuses {
		if status == target {
			return true
		}
	}
	return false
}

func toolResultDomainPayload(result agent.ToolResult) []byte {
	if len(result.Details) != 0 && json.Valid(result.Details) {
		return result.Details
	}
	return []byte(result.ModelContent)
}

func applyWorkspaceChangeReceiptToExecutionRecord(record *ToolExecutionRecord, result agent.ToolResult) {
	if record == nil {
		return
	}
	receipt, ok := producttools.ParseWorkspaceChangeReceipt(record.ToolName, string(toolResultDomainPayload(result)))
	if !ok {
		return
	}
	record.Workspace = receipt.Workspace
	record.ChangeGroupID = receipt.ChangeGroupID
	record.ReviewThreadID = receipt.ReviewThreadID
	record.ChangeSetID = receipt.ChangeSetID
	record.BaseRevision = receipt.BaseRevision
	record.Revision = receipt.Revision
	record.ReviewStatus = receipt.ReviewStatus
	record.ApplyState = receipt.ApplyState
	if strings.TrimSpace(receipt.Path) != "" {
		record.Target = receipt.Path
	}
}

func applyToolMutationReceiptToExecutionRecord(record *ToolExecutionRecord, result agent.ToolResult) {
	if record == nil {
		return
	}
	applyWorkspaceChangeReceiptToExecutionRecord(record, result)
	payload := string(toolResultDomainPayload(result))
	itemIDs, deletedIDs := parseWriteLoreItemsToolResult(record.ToolName, payload)
	record.LoreItemIDs = uniqueStrings(itemIDs)
	record.DeletedLoreItemIDs = uniqueStrings(deletedIDs)
}
