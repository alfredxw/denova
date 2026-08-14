package toolruntime

import (
	"context"
	"denova/internal/agents/run"
	"denova/internal/agents/tool"
	"fmt"
	"strings"
	"unicode/utf8"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/agents/toolresult"
	producttools "denova/internal/agents/tools"
)

const maxToolErrorDiagnosticBytes = 4 * 1024

// OrchestratorMiddleware is Denova's single product-level tool seam. It
// preserves product tool settings, workspace coordination, lifecycle receipts,
// and result projection without owning permission or batch scheduling.
type OrchestratorMiddleware struct {
	*agent.BaseMiddleware
	agentKind           string
	policyKind          string
	toolSettings        config.ResolvedAgentToolSettings
	enforceToolSettings bool
	workspace           string
	toolResultMaxBytes  int
	executionGate       *toolExecutionGate
}

// OrchestratorConfig declares the product policy applied around every tool
// call. Construction owns the shared workspace execution gate so callers
// cannot accidentally create competing mutation coordinators.
type OrchestratorConfig struct {
	AgentKind           string
	PolicyKind          string
	ToolSettings        config.ResolvedAgentToolSettings
	EnforceToolSettings bool
	Workspace           string
	ToolResultMaxBytes  int
}

func NewOrchestratorMiddleware(cfg OrchestratorConfig) *OrchestratorMiddleware {
	return &OrchestratorMiddleware{
		BaseMiddleware:      &agent.BaseMiddleware{},
		agentKind:           cfg.AgentKind,
		policyKind:          cfg.PolicyKind,
		toolSettings:        cfg.ToolSettings,
		enforceToolSettings: cfg.EnforceToolSettings,
		workspace:           cfg.Workspace,
		toolResultMaxBytes:  cfg.ToolResultMaxBytes,
		executionGate:       sharedToolExecutionGate(cfg.Workspace),
	}
}

// Configuration returns the immutable policy snapshot captured when the
// middleware was assembled. It is safe for diagnostics and architecture tests;
// mutating the returned value cannot change an admitted Agent run.
func (m *OrchestratorMiddleware) Configuration() OrchestratorConfig {
	if m == nil {
		return OrchestratorConfig{}
	}
	return OrchestratorConfig{
		AgentKind: m.agentKind, PolicyKind: m.effectivePolicyKind(), ToolSettings: m.toolSettings,
		EnforceToolSettings: m.enforceToolSettings, Workspace: m.workspace,
		ToolResultMaxBytes: m.toolResultLimitBytes(),
	}
}

type interactiveStoryToolMiddleware struct{ *agent.BaseMiddleware }
type interactiveDirectorPlanFileMiddleware struct{ *agent.BaseMiddleware }

func NewInteractiveStoryMiddleware() agent.Middleware {
	return &interactiveStoryToolMiddleware{BaseMiddleware: &agent.BaseMiddleware{}}
}

func NewInteractiveDirectorPlanFileMiddleware() agent.Middleware {
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
		path := strings.ToLower(strings.TrimSpace(toolresult.TargetFromArguments(args)))
		if strings.HasPrefix(path, "event://") {
			return ""
		}
		return fmt.Sprintf("[tool error] Director read access is limited to event:// cards in the current opportunity index. Planning documents are already present in context; submit a Markdown Patch with base_hash through %s.", SubmitDirectorPlanUpdateToolName)
	case "list_lore_items", "read_lore_items", "search_story_history", SubmitDirectorPlanUpdateToolName:
		return ""
	case "write", "edit":
		return fmt.Sprintf("[tool error] Director planning documents are already present in context. Submit a Markdown Patch with base_hash through %s. Blocked tool: %s", SubmitDirectorPlanUpdateToolName, name)
	case "apply_actor_state_patch":
		return fmt.Sprintf("[tool error] Director maintains ArcPlan only and cannot write Actor State. Blocked tool: %s", name)
	default:
		return fmt.Sprintf("[tool error] Director may use only %s, history search, read-only lore, and read(event://...). Blocked tool: %s", SubmitDirectorPlanUpdateToolName, name)
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
	return fmt.Sprintf("[tool error] Interactive story mode blocks tool %q because it may mutate the workspace or host. Output the complete story first, then submit the matching hidden turn result.", name)
}

func (m *OrchestratorMiddleware) WrapToolCall(
	_ context.Context,
	endpoint agent.ToolCallEndpoint,
	toolCtx *agent.ToolContext,
) (agent.ToolCallEndpoint, error) {
	return func(ctx context.Context, args string, opts ...agent.ToolOption) (agent.ToolResult, error) {
		decision := m.buildToolDecision(ctx, toolCtx, args)
		observer := agentrun.ObserverFromContext(ctx)
		outcome := agentrun.LLMOutcome{}
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
				message = fmt.Sprintf("[tool error] Tool %q was blocked by the current Agent policy.", decision.ToolName)
			}
			if observer != nil {
				observer.RecordToolExecution(blockedToolExecutionRecord(decision, message))
			}
			reason := agent.ToolSyntheticPolicyBlocked
			if decision.ArgsComplete != nil && !*decision.ArgsComplete && decision.ModelFinishReason != "" {
				reason = agent.ToolSyntheticModelIncomplete
			}
			prepared := toolresult.PrepareStructured(
				decision.ToolName, decision.Descriptor, args,
				agent.SyntheticToolResult(agent.ToolResultBlocked, reason, message),
			)
			return prepared.Result, nil
		}

		release, err := m.acquireToolExecution(ctx, decision)
		if err != nil {
			return agent.ToolResult{}, err
		}
		defer release()
		if err := ctx.Err(); err != nil {
			return agent.ToolResult{}, err
		}
		result, err := endpoint(ctx, args, opts...)
		if err != nil {
			toolErr := err
			if decision.Descriptor.Steering == agent.SteeringInterruptibleWait && agent.ToolSteeringPending(ctx) {
				result = agent.SyntheticToolResult(agent.ToolResultSkipped, agent.ToolSyntheticSteeringInterrupted,
					fmt.Sprintf("tool %q was interrupted to apply pending user steering", decision.ToolName))
			} else if ctx.Err() != nil {
				return agent.ToolResult{}, err
			} else {
				result, record := projectToolError(decision, args, result, toolErr, m.toolResultLimitBytes())
				result, effectErr := appendAgentMutationEffect(result, record)
				if effectErr != nil {
					return result, effectErr
				}
				recordToolExecution(ctx, record)
				if agent.IsToolControlError(toolErr) {
					return result, toolErr
				}
				return result, nil
			}
		}

		// Lossless materialization and protected receipts are owned by the
		// public Loop's fixed ResultProcessor stage. This middleware persists
		// only Denova's product mutation/audit projection; it must never truncate
		// or artifact-process the result first.
		prepared := toolresult.PrepareStructured(toolName(toolCtx), decision.Descriptor, args, result)
		// The execution ledger receives a separately bounded audit projection.
		// The returned value remains lossless for the public fixed processor.
		filtered := toolresult.ProjectAudit(toolName(toolCtx), decision.Descriptor, args, prepared.Result, m.toolResultLimitBytes())
		record := toolExecutionRecordFromFiltered(decision, filtered, string(filtered.Result.Status))
		applyToolMutationReceiptToExecutionRecord(&record, prepared.Result)
		applyInteractiveTurnReceiptToExecutionRecord(&record, prepared.Result)
		prepared.Result, err = appendAgentMutationEffect(prepared.Result, record)
		if err != nil {
			return prepared.Result, err
		}
		recordToolExecution(ctx, record)
		return prepared.Result, nil
	}, nil
}

func appendAgentMutationEffect(result agent.ToolResult, record agenttool.ExecutionRecord) (agent.ToolResult, error) {
	effect, present, err := AgentToolMutationEffect(record)
	if err != nil || !present {
		return result, err
	}
	result.Effects = append(result.Effects, effect)
	return result, nil
}

func toolEndpointErrorMessage(toolName string, err error) string {
	if message, ok := producttools.FormatWorkspaceChangeError(toolName, err); ok {
		return message
	}
	return fmt.Sprintf("[tool error] %v", err)
}

func projectToolError(decision agenttool.Decision, args string, returned agent.ToolResult, err error, maxBytes int) (agent.ToolResult, agenttool.ExecutionRecord) {
	message := strings.ToValidUTF8(toolEndpointErrorMessage(decision.ToolName, err), "\uFFFD")
	errorResult := agent.ToolErrorResult(message, boundedToolErrorDiagnostic(err))
	// Details is a terminal product receipt, not display content. Preserve a
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
	prepared := toolresult.PrepareStructured(
		decision.ToolName, decision.Descriptor, args,
		errorResult,
	)
	filtered := toolresult.ProjectAudit(decision.ToolName, decision.Descriptor, args, prepared.Result, maxBytes)
	record := toolExecutionRecordFromFiltered(decision, filtered, "error")
	record.Error = boundedToolErrorDiagnostic(err)
	applyToolMutationReceiptToExecutionRecord(&record, returned)
	return prepared.Result, record
}

func toolExecutionRecordFromFiltered(decision agenttool.Decision, filtered toolresult.Filtered, status string) agenttool.ExecutionRecord {
	return agenttool.ExecutionRecord{
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

func (m *OrchestratorMiddleware) acquireToolExecution(ctx context.Context, decision agenttool.Decision) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m == nil || m.executionGate == nil {
		return func() {}, nil
	}
	return m.executionGate.acquire(ctx, executionModeForTool(toolresult.ManifestForDefinition(decision.ToolName, decision.Descriptor)))
}

func blockedToolExecutionRecord(decision agenttool.Decision, message string) agenttool.ExecutionRecord {
	return agenttool.ExecutionRecord{
		ToolName: decision.ToolName, ProviderCallID: decision.ProviderCallID,
		ExecutionID: decision.ExecutionID, Status: "blocked",
		Capability: decision.Capability, Target: decision.Target, Error: message,
		ArgsBytes: decision.ArgsBytes, ArgsComplete: decision.ArgsComplete,
		ModelFinishReason: decision.ModelFinishReason, Descriptor: decision.Descriptor,
	}
}

func (m *OrchestratorMiddleware) toolResultLimitBytes() int {
	if m == nil {
		return 0
	}
	return toolresult.NormalizeLimitBytes(m.toolResultMaxBytes)
}

func (m *OrchestratorMiddleware) buildToolDecision(ctx context.Context, toolCtx *agent.ToolContext, args string) agenttool.Decision {
	name := toolName(toolCtx)
	manifest := toolresult.UnknownManifest(name)
	declared := toolCtx != nil && toolCtx.Definition.Info != nil
	if declared {
		manifest = toolresult.ManifestForDefinition(name, toolCtx.Definition.Descriptor)
	}
	providerCallID := toolCallID(toolCtx)
	executionID := ""
	if toolCtx != nil {
		executionID = strings.TrimSpace(toolCtx.ExecutionID)
	}
	if executionID == "" {
		executionID = agent.ToolExecutionID(ctx, providerCallID)
	}
	decision := agenttool.Decision{
		ToolName: manifest.Name, ProviderCallID: providerCallID,
		ExecutionID: executionID, Source: manifest.Source,
		Capability: manifest.Capability, Action: "allowed",
		MutationScope: manifest.MutationScope,
		PostCheck:     manifest.PostCheck,
		Target:        toolresult.TargetFromArguments(args), ArgsBytes: len(args), Descriptor: manifest.ToolDescriptor,
	}
	if m != nil && m.effectivePolicyKind() == agentrun.AgentKindInteractiveStory && isInteractiveStoryForbiddenMutation(toolCtx) {
		decision.Action = "blocked"
		decision.Reason = interactiveStoryWriteToolBlockedMessage(name)
		return decision
	}
	if m != nil && m.enforceToolSettings && !declared {
		decision.Action = "blocked"
		decision.Reason = fmt.Sprintf("[tool error] Tool %q has no explicit ToolDescriptor and was rejected before execution.", manifest.Name)
		return decision
	}
	if mode := toolAccessModeFromContext(ctx); !toolAllowedByAccessMode(mode, manifest.ToolDescriptor) {
		decision.Action = "blocked"
		decision.Reason = toolAccessModeBlockedMessage(mode, manifest.Name, manifest.ToolDescriptor)
		return decision
	}
	if m != nil && m.enforceToolSettings && manifest.Capability != "" && !config.AgentToolAllowed(m.toolSettings, manifest.Capability) {
		decision.Action = "blocked"
		decision.Reason = disabledToolCapabilityMessage(manifest.Name, manifest.Capability)
	}
	return decision
}

func disabledToolCapabilityMessage(name, capability string) string {
	return fmt.Sprintf("[tool error] Tool %q requires capability %s, which is disabled for this Agent. Use an authorized tool or ask the user to enable the capability in Agent Tools.", name, capability)
}

func (m *OrchestratorMiddleware) effectivePolicyKind() string {
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
