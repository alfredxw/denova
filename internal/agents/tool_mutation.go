package agents

import (
	"fmt"
	"path/filepath"
	"strings"
)

const (
	mutationReceiptWorkspaceChange = "workspace_change.tool_result.v1"
	mutationReceiptLoreWrite       = "lore.write.v1"
	mutationReceiptGeneratedImage  = "generated_image.workspace.v1"
)

type ToolMutation struct {
	ToolName           string              `json:"tool_name"`
	ToolCallID         string              `json:"tool_call_id,omitempty"`
	Workspace          string              `json:"workspace,omitempty"`
	Target             string              `json:"target,omitempty"`
	Source             ToolSource          `json:"source"`
	MutationScope      ToolMutationScope   `json:"mutation_scope"`
	PostCheck          ToolPostCheckPolicy `json:"post_check"`
	IdempotencyKey     string              `json:"idempotency_key,omitempty"`
	LoreItemIDs        []string            `json:"lore_item_ids,omitempty"`
	DeletedLoreItemIDs []string            `json:"deleted_lore_item_ids,omitempty"`
	ChangeGroupID      string              `json:"change_group_id,omitempty"`
	ReviewThreadID     string              `json:"review_thread_id,omitempty"`
	ChangeSetID        string              `json:"change_set_id,omitempty"`
	BaseRevision       string              `json:"base_revision,omitempty"`
	Revision           string              `json:"revision,omitempty"`
	ReviewStatus       string              `json:"review_status,omitempty"`
	ApplyState         string              `json:"apply_state,omitempty"`
}

// toolMutationResolution is the single conversion result consumed by durable
// HostEffects, post-run verification, and product callbacks.
type toolMutationResolution struct {
	Mutation  ToolMutation
	Committed bool
	Warning   string
}

func resolveToolMutation(record ToolExecutionRecord) toolMutationResolution {
	manifest := manifestForDefinition(record.ToolName, record.Descriptor)
	if manifest.MutationScope != ToolMutationWorkspace {
		return toolMutationResolution{}
	}
	status := strings.ToLower(strings.TrimSpace(record.Status))
	if strings.TrimSpace(record.SyntheticReason) != "" || status == "blocked" || status == "skipped" || status == "" {
		return toolMutationResolution{}
	}
	if status != "success" && status != "error" {
		return toolMutationResolution{}
	}

	mutation := ToolMutation{
		ToolName: manifest.Name, ToolCallID: strings.TrimSpace(record.ExecutionID),
		Workspace: strings.TrimSpace(record.Workspace), Target: filepath.ToSlash(strings.TrimSpace(record.Target)),
		Source: manifest.Source, MutationScope: manifest.MutationScope, PostCheck: manifest.PostCheck,
		IdempotencyKey: strings.TrimSpace(record.IdempotencyKey),
		ChangeGroupID:  strings.TrimSpace(record.ChangeGroupID), ReviewThreadID: strings.TrimSpace(record.ReviewThreadID),
		ChangeSetID: strings.TrimSpace(record.ChangeSetID), BaseRevision: strings.TrimSpace(record.BaseRevision),
		Revision: strings.TrimSpace(record.Revision), ReviewStatus: strings.TrimSpace(record.ReviewStatus),
		ApplyState:  strings.TrimSpace(record.ApplyState),
		LoreItemIDs: uniqueStrings(record.LoreItemIDs), DeletedLoreItemIDs: uniqueStrings(record.DeletedLoreItemIDs),
	}
	if validToolMutationReceipt(record, mutation) {
		return toolMutationResolution{Mutation: mutation, Committed: true}
	}
	return toolMutationResolution{Warning: fmt.Sprintf(
		"workspace mutation tool %q completed with status %q without a valid mutation receipt (execution_id=%s)",
		manifest.Name, status, strings.TrimSpace(record.ExecutionID),
	)}
}

func validToolMutationReceipt(record ToolExecutionRecord, mutation ToolMutation) bool {
	switch strings.TrimSpace(record.MutationReceiptSchema) {
	case mutationReceiptWorkspaceChange:
		return mutation.Workspace != "" && mutation.Target != "" &&
			mutation.ChangeGroupID != "" && mutation.ChangeSetID != ""
	case mutationReceiptLoreWrite:
		return len(mutation.LoreItemIDs) > 0 || len(mutation.DeletedLoreItemIDs) > 0
	case mutationReceiptGeneratedImage:
		return mutation.Target != ""
	default:
		return false
	}
}

func toolMutationFromExecutionRecord(record ToolExecutionRecord) (ToolMutation, bool) {
	resolution := resolveToolMutation(record)
	return resolution.Mutation, resolution.Committed
}

func eventDataStringSlice(data any, key string) []string {
	switch typed := data.(type) {
	case map[string]interface{}:
		value, ok := typed[key]
		if !ok {
			return nil
		}
		return anyToStringSlice(value)
	case map[string][]string:
		return typed[key]
	default:
		return nil
	}
}

func anyToStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
