package toolruntime

import (
	agentinteractive "denova/internal/agents/interactive"
	agenttool "denova/internal/agents/tool"
	"encoding/json"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	producttools "denova/internal/agents/tools"
	workspacechange "denova/internal/workspace/change"
)

func applyInteractiveTurnReceiptToExecutionRecord(record *agenttool.ExecutionRecord, result agent.ToolResult) {
	if record == nil || !agentinteractive.IsInteractiveTurnSubmissionTool(record.ToolName) {
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

func applyWorkspaceChangeReceiptToExecutionRecord(record *agenttool.ExecutionRecord, result agent.ToolResult) {
	if record == nil {
		return
	}
	receipt, ok := workspacechange.ParseToolReceipt(record.ToolName, string(toolResultDomainPayload(result)))
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
	record.MutationReceiptSchema = agenttool.MutationReceiptWorkspaceChange
	if strings.TrimSpace(receipt.Path) != "" {
		record.Target = receipt.Path
	}
}

func applyToolMutationReceiptToExecutionRecord(record *agenttool.ExecutionRecord, result agent.ToolResult) {
	if record == nil {
		return
	}
	applyWorkspaceChangeReceiptToExecutionRecord(record, result)
	payload := string(toolResultDomainPayload(result))
	if itemIDs, deletedIDs, ok := parseLoreMutationReceipt(record.ToolName, payload); ok {
		record.LoreItemIDs = agenttool.NormalizeIDs(itemIDs)
		record.DeletedLoreItemIDs = agenttool.NormalizeIDs(deletedIDs)
		record.MutationReceiptSchema = agenttool.MutationReceiptLoreWrite
		return
	}
	if target := parseGeneratedImageMutationTarget(record.ToolName, payload); target != "" {
		record.Target = target
		record.MutationReceiptSchema = agenttool.MutationReceiptGeneratedImage
	}
}

func parseLoreMutationReceipt(toolName, payload string) ([]string, []string, bool) {
	if agenttool.NormalizeName(toolName) != "write_lore_items" {
		return nil, nil, false
	}
	var receipt struct {
		Schema     string   `json:"schema"`
		ItemIDs    []string `json:"item_ids"`
		DeletedIDs []string `json:"deleted_ids"`
	}
	if err := json.Unmarshal([]byte(payload), &receipt); err != nil || receipt.Schema != agenttool.MutationReceiptLoreWrite {
		return nil, nil, false
	}
	return receipt.ItemIDs, receipt.DeletedIDs, true
}

func parseGeneratedImageMutationTarget(toolName, payload string) string {
	if illustration, err := producttools.ParseChapterIllustrationResult(toolName, payload); err == nil && illustration != nil {
		return strings.TrimSpace(illustration.MetaPath)
	}
	if interactiveImage, err := producttools.ParseInteractiveImageResult(toolName, payload); err == nil && interactiveImage != nil {
		return strings.TrimSpace(interactiveImage.MetaPath)
	}
	return strings.TrimSpace(producttools.ParseGeneratedImageTarget(toolName, payload))
}
