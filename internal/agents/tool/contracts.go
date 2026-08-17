// Package tool owns the stable product-level vocabulary for Agent tool
// execution, mutation receipts, and post-run verification.
package agenttool

import (
	"fmt"
	"path/filepath"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/agents/toolresult"
)

// Agent orchestration uses the public runtime vocabulary directly; these
// aliases keep lifecycle records readable without introducing duplicate enum
// definitions.
type ToolSource = agent.ToolSource
type ToolExecutionClass = agent.ToolExecutionClass
type ToolMutationScope = agent.ToolMutationScope
type ToolPostCheckPolicy = agent.ToolPostCheckPolicy
type ToolRecoveryClass = agent.ToolRecoveryClass

const (
	ToolSourceOther = agent.ToolSourceOther
	ToolSourceRead  = agent.ToolSourceRead
	ToolSourceWrite = agent.ToolSourceWrite
	ToolSourceShell = agent.ToolSourceShell
	// ToolSourceLore is Denova's product-specific lore capability. Public Agent
	// tool vocabulary intentionally remains product-neutral.
	ToolSourceLore    = agent.ToolSource("denova.lore")
	ToolSourceHistory = agent.ToolSourceHistory
	ToolSourceWeb     = agent.ToolSourceWeb
	ToolSourceImage   = agent.ToolSourceImage
)

const (
	ToolExecutionParallelRead       = agent.ToolExecutionParallelRead
	ToolExecutionWorkspaceExclusive = agent.ToolExecutionWorkspaceExclusive
	ToolExecutionSessionExclusive   = agent.ToolExecutionSessionExclusive
	ToolExecutionConfigExclusive    = agent.ToolExecutionConfigExclusive
	ToolExecutionInteractiveWait    = agent.ToolExecutionInteractiveWait
	ToolExecutionChild              = agent.ToolExecutionChild
)

const (
	ToolMutationNone      = agent.ToolMutationNone
	ToolMutationWorkspace = agent.ToolMutationWorkspace
	ToolMutationSession   = agent.ToolMutationSession
	ToolMutationConfig    = agent.ToolMutationConfig
	ToolMutationExternal  = agent.ToolMutationExternal
)

const (
	ToolPostCheckNone            = agent.ToolPostCheckNone
	ToolPostCheckWorkspaceChange = agent.ToolPostCheckWorkspaceChange
	ToolPostCheckSessionState    = agent.ToolPostCheckSessionState
	ToolPostCheckConfigRevision  = agent.ToolPostCheckConfigRevision
	ToolPostCheckExternalReceipt = agent.ToolPostCheckExternalReceipt
)

const (
	ToolRecoveryReadOnly      = agent.ToolRecoveryReadOnly
	ToolRecoveryIdempotent    = agent.ToolRecoveryIdempotent
	ToolRecoveryReconcilable  = agent.ToolRecoveryReconcilable
	ToolRecoveryNonIdempotent = agent.ToolRecoveryNonIdempotent
)

const ToolResultBoundedModelContext = agent.ToolResultBoundedModelContext

// NormalizeName returns the canonical lookup form of a tool name.
func NormalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// Mutation is the canonical projection of one committed workspace mutation.
type Mutation struct {
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

// Decision is the bounded authorization and execution projection for one tool
// call. It contains no raw tool result or model content.
type Decision struct {
	ToolName          string               `json:"tool_name"`
	ProviderCallID    string               `json:"provider_call_id,omitempty"`
	ExecutionID       string               `json:"execution_id,omitempty"`
	ParentCallID      string               `json:"parent_call_id,omitempty"`
	Source            ToolSource           `json:"source"`
	Capability        string               `json:"capability,omitempty"`
	Action            string               `json:"action"`
	Reason            string               `json:"reason,omitempty"`
	MutationScope     ToolMutationScope    `json:"mutation_scope"`
	PostCheck         ToolPostCheckPolicy  `json:"post_check"`
	Target            string               `json:"target,omitempty"`
	ArgsBytes         int                  `json:"args_bytes,omitempty"`
	ArgsComplete      *bool                `json:"args_complete,omitempty"`
	ModelFinishReason string               `json:"model_finish_reason,omitempty"`
	Descriptor        agent.ToolDescriptor `json:"descriptor"`
}

// ExecutionRecord is the bounded lifecycle projection stored by the durable
// runtime. Display content and unrestricted diagnostic details are excluded.
type ExecutionRecord struct {
	ToolName              string               `json:"tool_name"`
	ProviderCallID        string               `json:"provider_call_id,omitempty"`
	ExecutionID           string               `json:"execution_id,omitempty"`
	ParentCallID          string               `json:"parent_call_id,omitempty"`
	Workspace             string               `json:"workspace,omitempty"`
	Status                string               `json:"status"`
	SyntheticReason       string               `json:"synthetic_reason,omitempty"`
	Result                string               `json:"result,omitempty"`
	DomainStatus          string               `json:"domain_status,omitempty"`
	DomainDiagnosticCount int                  `json:"domain_diagnostic_count,omitempty"`
	RetryModules          []string             `json:"retry_modules,omitempty"`
	Capability            string               `json:"capability,omitempty"`
	OriginalBytes         int                  `json:"original_bytes,omitempty"`
	ReturnedBytes         int                  `json:"returned_bytes,omitempty"`
	Truncated             bool                 `json:"truncated,omitempty"`
	Target                string               `json:"target,omitempty"`
	IdempotencyKey        string               `json:"idempotency_key,omitempty"`
	Error                 string               `json:"error,omitempty"`
	ArgsBytes             int                  `json:"args_bytes,omitempty"`
	ArgsComplete          *bool                `json:"args_complete,omitempty"`
	ModelFinishReason     string               `json:"model_finish_reason,omitempty"`
	ChangeGroupID         string               `json:"change_group_id,omitempty"`
	ReviewThreadID        string               `json:"review_thread_id,omitempty"`
	ChangeSetID           string               `json:"change_set_id,omitempty"`
	BaseRevision          string               `json:"base_revision,omitempty"`
	Revision              string               `json:"revision,omitempty"`
	ReviewStatus          string               `json:"review_status,omitempty"`
	ApplyState            string               `json:"apply_state,omitempty"`
	LoreItemIDs           []string             `json:"lore_item_ids,omitempty"`
	DeletedLoreItemIDs    []string             `json:"deleted_lore_item_ids,omitempty"`
	MutationReceiptSchema string               `json:"mutation_receipt_schema,omitempty"`
	Descriptor            agent.ToolDescriptor `json:"descriptor"`
}

const (
	MutationReceiptWorkspaceChange = "workspace_change.tool_result.v1"
	MutationReceiptLoreWrite       = "lore.write.v1"
	MutationReceiptGeneratedImage  = "generated_image.workspace.v1"
)

// MutationResolution is the single conversion result consumed by durable host
// effects, post-run verification, and product callbacks.
type MutationResolution struct {
	Mutation  Mutation
	Committed bool
	Warning   string
}

// ResolveMutation validates the canonical mutation receipt embedded in one
// terminal execution record.
func ResolveMutation(record ExecutionRecord) MutationResolution {
	manifest := toolresult.ManifestForDefinition(record.ToolName, record.Descriptor)
	if manifest.MutationScope != ToolMutationWorkspace {
		return MutationResolution{}
	}
	status := strings.ToLower(strings.TrimSpace(record.Status))
	if strings.TrimSpace(record.SyntheticReason) != "" || status == "blocked" || status == "skipped" || status == "" {
		return MutationResolution{}
	}
	if status != "success" && status != "error" {
		return MutationResolution{}
	}

	mutation := Mutation{
		ToolName: manifest.Name, ToolCallID: strings.TrimSpace(record.ExecutionID),
		Workspace: strings.TrimSpace(record.Workspace), Target: filepath.ToSlash(strings.TrimSpace(record.Target)),
		Source: manifest.Source, MutationScope: manifest.MutationScope, PostCheck: manifest.PostCheck,
		IdempotencyKey: strings.TrimSpace(record.IdempotencyKey),
		ChangeGroupID:  strings.TrimSpace(record.ChangeGroupID), ReviewThreadID: strings.TrimSpace(record.ReviewThreadID),
		ChangeSetID: strings.TrimSpace(record.ChangeSetID), BaseRevision: strings.TrimSpace(record.BaseRevision),
		Revision: strings.TrimSpace(record.Revision), ReviewStatus: strings.TrimSpace(record.ReviewStatus),
		ApplyState:  strings.TrimSpace(record.ApplyState),
		LoreItemIDs: NormalizeIDs(record.LoreItemIDs), DeletedLoreItemIDs: NormalizeIDs(record.DeletedLoreItemIDs),
	}
	if validMutationReceipt(record, mutation) {
		return MutationResolution{Mutation: mutation, Committed: true}
	}
	return MutationResolution{Warning: fmt.Sprintf(
		"workspace mutation tool %q completed with status %q without a valid mutation receipt (execution_id=%s)",
		manifest.Name, status, strings.TrimSpace(record.ExecutionID),
	)}
}

// MutationFromExecutionRecord returns a committed mutation when its receipt is
// valid for the tool's declared mutation scope.
func MutationFromExecutionRecord(record ExecutionRecord) (Mutation, bool) {
	resolution := ResolveMutation(record)
	return resolution.Mutation, resolution.Committed
}

func validMutationReceipt(record ExecutionRecord, mutation Mutation) bool {
	switch strings.TrimSpace(record.MutationReceiptSchema) {
	case MutationReceiptWorkspaceChange:
		return mutation.Workspace != "" && mutation.Target != "" && mutation.ChangeGroupID != "" && mutation.ChangeSetID != ""
	case MutationReceiptLoreWrite:
		return len(mutation.LoreItemIDs) > 0 || len(mutation.DeletedLoreItemIDs) > 0
	case MutationReceiptGeneratedImage:
		return mutation.Target != ""
	default:
		return false
	}
}

// NormalizeIDs trims, removes empty entries, and preserves the first occurrence
// of each identifier.
func NormalizeIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
