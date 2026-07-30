package agents

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	producttools "denova/internal/agents/tools"
)

const maxToolErrorDiagnosticBytes = 4 * 1024

// toolOrchestratorMiddleware is Denova's single product-level tool seam. It
// preserves authorization, workspace coordination, lifecycle receipts, and
// result projection without owning batch scheduling.
type toolOrchestratorMiddleware struct {
	*agent.BaseMiddleware
	agentKind                string
	policyKind               string
	toolSettings             config.ResolvedAgentToolSettings
	enforceToolSettings      bool
	toolResultMaxBytes       int
	toolResultEagerMinTokens int
	contextWindowTokens      int
	executionGate            *toolExecutionGate
}

type interactiveStoryToolMiddleware struct{ *agent.BaseMiddleware }
type interactiveDirectorPlanFileMiddleware struct{ *agent.BaseMiddleware }

func newInteractiveStoryToolMiddleware() *interactiveStoryToolMiddleware {
	return &interactiveStoryToolMiddleware{BaseMiddleware: &agent.BaseMiddleware{}}
}

func newInteractiveDirectorPlanFileMiddleware() *interactiveDirectorPlanFileMiddleware {
	return &interactiveDirectorPlanFileMiddleware{BaseMiddleware: &agent.BaseMiddleware{}}
}

func (m *interactiveDirectorPlanFileMiddleware) WrapToolCall(
	_ context.Context,
	endpoint agent.ToolCallEndpoint,
	toolCtx *agent.ToolContext,
) (agent.ToolCallEndpoint, error) {
	return func(ctx context.Context, args string, opts ...agent.ToolOption) (agent.ToolResult, error) {
		if msg := m.blockedDirectorToolMessage(toolName(toolCtx), args); msg != "" {
			return agent.SyntheticToolResult(agent.ToolResultBlocked, agent.ToolSyntheticPolicyBlocked, msg), nil
		}
		return endpoint(ctx, args, opts...)
	}, nil
}

func (m *interactiveDirectorPlanFileMiddleware) blockedDirectorToolMessage(name, args string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "read":
		path := strings.ToLower(strings.TrimSpace(toolPathFromArgs(args)))
		if strings.HasPrefix(path, "event://") {
			return ""
		}
		return fmt.Sprintf("[tool error] Director 的 read 仅允许当前机会索引中的 event:// 事件卡；规划文档已在上下文中完整提供，请用 %s 提交带 base_hash 的 Markdown Patch。", submitDirectorPlanUpdateToolName)
	case "list_lore_items", "read_lore_items", "search_story_history", submitDirectorPlanUpdateToolName:
		return ""
	case "write", "edit":
		return fmt.Sprintf("[tool error] Director 规划文档已在上下文中完整提供；请用 %s 提交带 base_hash 的 Markdown Patch，拒绝工具: %s", submitDirectorPlanUpdateToolName, name)
	case "apply_actor_state_patch":
		return fmt.Sprintf("[tool error] Director 只维护 ArcPlan，不能写 Actor State，拒绝工具: %s", name)
	default:
		return fmt.Sprintf("[tool error] Director 只能使用 %s、历史检索、资料库只读和 read(event://...)，拒绝工具: %s", submitDirectorPlanUpdateToolName, name)
	}
}

func (m *interactiveStoryToolMiddleware) WrapToolCall(
	_ context.Context,
	endpoint agent.ToolCallEndpoint,
	toolCtx *agent.ToolContext,
) (agent.ToolCallEndpoint, error) {
	return func(ctx context.Context, args string, opts ...agent.ToolOption) (agent.ToolResult, error) {
		if isInteractiveStoryForbiddenMutation(toolCtx) {
			return agent.SyntheticToolResult(agent.ToolResultBlocked, agent.ToolSyntheticPolicyBlocked, interactiveStoryWriteToolBlockedMessage(toolName(toolCtx))), nil
		}
		return endpoint(ctx, args, opts...)
	}, nil
}

func toolName(toolCtx *agent.ToolContext) string {
	if toolCtx == nil {
		return ""
	}
	return toolCtx.Name
}

func isInteractiveStoryForbiddenMutation(toolCtx *agent.ToolContext) bool {
	if toolCtx != nil {
		descriptor := toolCtx.Definition.Descriptor
		if descriptor.Capability == config.AgentToolShell || descriptor.MutationScope == agent.ToolMutationWorkspace {
			return true
		}
	}
	return isInteractiveStoryWriteTool(toolName(toolCtx))
}

func isInteractiveStoryWriteTool(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "write", "edit", "bash", "pwsh", "delete_file", "create_file", "move_file", "copy_file", "rename_file", "mkdir", "remove_file":
		return true
	}
	return strings.HasPrefix(name, "write_") || strings.HasPrefix(name, "edit_") ||
		strings.HasPrefix(name, "delete_") || strings.HasPrefix(name, "create_") ||
		strings.HasPrefix(name, "move_") || strings.HasPrefix(name, "copy_") ||
		strings.HasPrefix(name, "rename_")
}

func interactiveStoryWriteToolBlockedMessage(name string) string {
	return fmt.Sprintf("[tool error] 游戏模式禁止使用可能产生 workspace 或宿主副作用的工具 %q。请先直接输出完整故事正文，再用 submit_interactive_turn 提交一致的隐藏回合结果。 / Interactive story mode blocks tool %q because it may mutate the workspace or host. Output the complete story first, then submit the matching hidden turn result.", name, name)
}

type ToolDecision struct {
	ToolName          string               `json:"tool_name"`
	ProviderCallID    string               `json:"provider_call_id,omitempty"`
	ExecutionID       string               `json:"execution_id,omitempty"`
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

// ToolExecutionRecord is the bounded lifecycle projection stored by durable
// runtime. DisplayContent and Details deliberately do not live here.
type ToolExecutionRecord struct {
	ToolName        string `json:"tool_name"`
	ProviderCallID  string `json:"provider_call_id,omitempty"`
	ExecutionID     string `json:"execution_id,omitempty"`
	Workspace       string `json:"workspace,omitempty"`
	Status          string `json:"status"`
	SyntheticReason string `json:"synthetic_reason,omitempty"`
	// Result is the bounded model projection required by the recoverable harness
	// runtime. RunLedger records only its byte counts and status, never this body.
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

func (m *toolOrchestratorMiddleware) WrapToolCall(
	_ context.Context,
	endpoint agent.ToolCallEndpoint,
	toolCtx *agent.ToolContext,
) (agent.ToolCallEndpoint, error) {
	return func(ctx context.Context, args string, opts ...agent.ToolOption) (agent.ToolResult, error) {
		decision := m.buildToolDecision(ctx, toolCtx, args)
		observer := RunObserverFromContext(ctx)
		outcome := LLMOutcome{}
		if observer != nil {
			outcome = observer.LastLLMOutcome()
		}
		decision = applyModelOutputToolSafety(decision, outcome)
		decision = applyToolArgumentValidation(decision, args, outcome)
		if observer != nil {
			observer.RecordToolDecision(decision)
		}
		if decision.Action == "blocked" {
			message := decision.Reason
			if message == "" {
				message = fmt.Sprintf("[tool error] 工具 %q 被当前 Agent 策略阻止。", decision.ToolName)
			}
			if observer != nil {
				observer.RecordToolExecution(blockedToolExecutionRecord(decision, message))
			}
			reason := agent.ToolSyntheticPolicyBlocked
			if decision.ArgsComplete != nil && !*decision.ArgsComplete && decision.ModelFinishReason != "" {
				reason = agent.ToolSyntheticModelIncomplete
			}
			filtered := filterStructuredToolResultWithDescriptor(
				decision.ToolName, decision.Descriptor, args,
				agent.SyntheticToolResult(agent.ToolResultBlocked, reason, message), m.toolResultLimitBytes(),
			)
			return filtered.Result, nil
		}

		release, err := m.acquireToolExecution(ctx, decision)
		if err != nil {
			return agent.ToolResult{}, err
		}
		defer release()
		if err := ctx.Err(); err != nil {
			return agent.ToolResult{}, err
		}
		if err := recordToolStart(ctx, decision, args); err != nil {
			return agent.ToolErrorResult(err.Error(), err.Error()), err
		}
		if err := ctx.Err(); err != nil {
			return agent.ToolResult{}, err
		}

		result, err := endpoint(ctx, args, opts...)
		if err != nil {
			if decision.Descriptor.Steering == agent.SteeringInterruptibleWait && agent.ToolSteeringPending(ctx) {
				result = agent.SyntheticToolResult(agent.ToolResultSkipped, agent.ToolSyntheticSteeringInterrupted,
					fmt.Sprintf("tool %q was interrupted to apply pending user steering", decision.ToolName))
			} else if agent.IsInterruptError(err) || ctx.Err() != nil {
				// Immediate abort keeps an unmatched durable start. Recovery will
				// materialize effect_unknown and never auto-retry the side effect.
				return agent.ToolResult{}, err
			} else {
				result, record := projectToolError(decision, args, result, err, m.toolResultLimitBytes())
				if recordErr := recordToolFinish(ctx, record); recordErr != nil {
					return result, recordErr
				}
				if agent.IsToolControlError(err) {
					return result, err
				}
				return result, nil
			}
		}

		processed, processErr := processToolResult(ctx, decision, args, result, toolResultProcessingPolicy{
			MaxBytes: m.toolResultLimitBytes(), EagerMinTokens: m.toolResultEagerMinTokens,
			ContextWindowTokens: m.contextWindowTokens,
		})
		if processErr != nil {
			projected, record := projectToolError(decision, args, processed, processErr, m.toolResultLimitBytes())
			if recordErr := recordToolFinish(ctx, record); recordErr != nil {
				return projected, recordErr
			}
			if agent.IsToolControlError(processErr) {
				return projected, processErr
			}
			return projected, nil
		}
		result = processed
		filtered := filterStructuredToolResultWithDescriptor(
			toolName(toolCtx), decision.Descriptor, args, result, m.toolResultLimitBytes(),
		)
		record := toolExecutionRecordFromFiltered(decision, filtered, string(filtered.Result.Status))
		applyToolMutationReceiptToExecutionRecord(&record, result)
		applyInteractiveTurnReceiptToExecutionRecord(&record, filtered.Result)
		if err := recordToolFinish(ctx, record); err != nil {
			return filtered.Result, err
		}
		return filtered.Result, nil
	}, nil
}

func toolEndpointErrorMessage(toolName string, err error) string {
	if message, ok := producttools.FormatWorkspaceChangeError(toolName, err); ok {
		return message
	}
	return fmt.Sprintf("[tool error] %v", err)
}

func projectToolError(decision ToolDecision, args string, returned agent.ToolResult, err error, maxBytes int) (agent.ToolResult, ToolExecutionRecord) {
	message := strings.ToValidUTF8(toolEndpointErrorMessage(decision.ToolName, err), "\uFFFD")
	errorResult := agent.ToolErrorResult(message, boundedToolErrorDiagnostic(err))
	// Details is a terminal durability receipt, not display content. Preserve a
	// valid receipt even when the tool reports a transport/domain error after the
	// workspace effect committed.
	if len(returned.Details) != 0 {
		errorResult.Details = append(errorResult.Details[:0], returned.Details...)
	}
	errorResult.Artifacts = append([]agent.ToolArtifactRef(nil), returned.Artifacts...)
	errorResult.ContextHints = returned.ContextHints
	errorResult.Metadata.OriginalModelBytes = returned.Metadata.OriginalModelBytes
	errorResult.Metadata.OriginalDisplayBytes = returned.Metadata.OriginalDisplayBytes
	errorResult.Metadata.ModelTruncated = returned.Metadata.ModelTruncated
	errorResult.Metadata.DisplayTruncated = returned.Metadata.DisplayTruncated
	errorResult.Metadata.ArtifactPersistence = returned.Metadata.ArtifactPersistence
	filtered := filterStructuredToolResultWithDescriptor(
		decision.ToolName, decision.Descriptor, args,
		errorResult, maxBytes,
	)
	record := toolExecutionRecordFromFiltered(decision, filtered, "error")
	record.Error = boundedToolErrorDiagnostic(err)
	applyToolMutationReceiptToExecutionRecord(&record, returned)
	return filtered.Result, record
}

func toolExecutionRecordFromFiltered(decision ToolDecision, filtered FilteredToolResult, status string) ToolExecutionRecord {
	return ToolExecutionRecord{
		ToolName: filtered.Manifest.Name, ProviderCallID: decision.ProviderCallID,
		ExecutionID: decision.ExecutionID, Status: status,
		SyntheticReason: string(filtered.Result.SyntheticReason),
		Capability:      filtered.Manifest.Capability,
		OriginalBytes:   filtered.Result.Metadata.OriginalModelBytes,
		ReturnedBytes:   filtered.Result.Metadata.ReturnedModelBytes,
		Truncated:       filtered.Result.Metadata.ModelTruncated,
		Target:          filtered.Result.Metadata.Target,
		IdempotencyKey:  filtered.Result.Metadata.IdempotencyKey,
		Result:          filtered.Result.ModelContent, Descriptor: decision.Descriptor,
	}
}

func boundedToolErrorDiagnostic(err error) string {
	diagnostic := "tool execution failed"
	if err != nil {
		if value := strings.TrimSpace(strings.ToValidUTF8(err.Error(), "\uFFFD")); value != "" {
			diagnostic = value
		}
	}
	if len(diagnostic) <= maxToolErrorDiagnosticBytes {
		return diagnostic
	}
	const suffix = "\n[tool error diagnostic truncated]"
	end := maxToolErrorDiagnosticBytes - len(suffix)
	for end > 0 && !utf8.RuneStart(diagnostic[end]) {
		end--
	}
	return strings.TrimSpace(diagnostic[:end]) + suffix
}

func (m *toolOrchestratorMiddleware) acquireToolExecution(ctx context.Context, decision ToolDecision) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m == nil || m.executionGate == nil {
		return func() {}, nil
	}
	return m.executionGate.acquire(ctx, executionModeForTool(manifestForDefinition(decision.ToolName, decision.Descriptor)))
}

func blockedToolExecutionRecord(decision ToolDecision, message string) ToolExecutionRecord {
	return ToolExecutionRecord{
		ToolName: decision.ToolName, ProviderCallID: decision.ProviderCallID,
		ExecutionID: decision.ExecutionID, Status: "blocked",
		Capability: decision.Capability, Target: decision.Target, Error: message,
		ArgsBytes: decision.ArgsBytes, ArgsComplete: decision.ArgsComplete,
		ModelFinishReason: decision.ModelFinishReason, Descriptor: decision.Descriptor,
	}
}

func (m *toolOrchestratorMiddleware) toolResultLimitBytes() int {
	if m == nil {
		return 0
	}
	return normalizeToolResultLimitBytes(m.toolResultMaxBytes)
}

func (m *toolOrchestratorMiddleware) buildToolDecision(ctx context.Context, toolCtx *agent.ToolContext, args string) ToolDecision {
	name := toolName(toolCtx)
	manifest := unknownToolManifest(name)
	declared := toolCtx != nil && toolCtx.Definition.Info != nil
	if declared {
		manifest = manifestForDefinition(name, toolCtx.Definition.Descriptor)
	}
	providerCallID := toolCallID(toolCtx)
	executionID := ""
	if toolCtx != nil {
		executionID = strings.TrimSpace(toolCtx.ExecutionID)
	}
	if executionID == "" {
		executionID = agent.ToolExecutionID(ctx, providerCallID)
	}
	decision := ToolDecision{
		ToolName: manifest.Name, ProviderCallID: providerCallID,
		ExecutionID: executionID, Source: manifest.Source,
		Capability: manifest.Capability, Action: "allowed",
		MutationScope: manifest.MutationScope,
		PostCheck:     manifest.PostCheck,
		Target:        toolPathFromArgs(args), ArgsBytes: len(args), Descriptor: manifest.ToolDescriptor,
	}
	if m != nil && m.effectivePolicyKind() == AgentKindInteractiveStory && isInteractiveStoryForbiddenMutation(toolCtx) {
		decision.Action = "blocked"
		decision.Reason = interactiveStoryWriteToolBlockedMessage(name)
		return decision
	}
	if m != nil && m.enforceToolSettings && !declared {
		decision.Action = "blocked"
		decision.Reason = fmt.Sprintf("[tool error] 工具 %q 没有显式 ToolDescriptor，已在执行前拒绝。 / Tool %q has no explicit ToolDescriptor and was rejected before execution.", manifest.Name, manifest.Name)
		return decision
	}
	if m != nil && m.enforceToolSettings && manifest.Capability != "" && !config.AgentToolAllowed(m.toolSettings, manifest.Capability) {
		decision.Action = "blocked"
		decision.Reason = disabledToolCapabilityMessage(manifest.Name, manifest.Capability)
	}
	return decision
}

func disabledToolCapabilityMessage(name, capability string) string {
	return fmt.Sprintf("[tool error] 工具 %q 需要当前 Agent 启用 %s 能力，但该能力已关闭。请改用已授权工具，或请用户在 Agent Tools 中开启该能力。 / Tool %q requires capability %s, which is disabled for this Agent.", name, capability, name, capability)
}

func (m *toolOrchestratorMiddleware) effectivePolicyKind() string {
	if m == nil {
		return ""
	}
	if strings.TrimSpace(m.policyKind) != "" {
		return m.policyKind
	}
	return m.agentKind
}

func toolCallID(toolCtx *agent.ToolContext) string {
	if toolCtx == nil {
		return ""
	}
	return strings.TrimSpace(toolCtx.ProviderCallID)
}
