package toolruntime

import (
	"context"
	"denova/internal/agents/run"
	"denova/internal/agents/tool"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/agents/toolapproval"
	"denova/internal/agents/toolresult"
	producttools "denova/internal/agents/tools"
)

const maxToolErrorDiagnosticBytes = 4 * 1024

// OrchestratorMiddleware is Denova's single product-level tool seam. It
// preserves authorization, workspace coordination, lifecycle receipts, and
// result projection without owning batch scheduling.
type OrchestratorMiddleware struct {
	*agent.BaseMiddleware
	agentKind                string
	policyKind               string
	toolSettings             config.ResolvedAgentToolSettings
	enforceToolSettings      bool
	enforceApprovalPolicy    bool
	approvalMode             config.AgentApprovalMode
	projectID                string
	workspace                string
	approvalRulesMu          sync.RWMutex
	approvalRules            []config.AgentApprovalRule
	toolResultMaxBytes       int
	toolResultEagerMinTokens int
	contextWindowTokens      int
	executionGate            *toolExecutionGate
}

// OrchestratorConfig declares the product policy applied around every tool
// call. Construction owns the shared workspace execution gate so callers
// cannot accidentally create competing mutation coordinators.
type OrchestratorConfig struct {
	AgentKind                string
	PolicyKind               string
	ToolSettings             config.ResolvedAgentToolSettings
	EnforceToolSettings      bool
	EnforceApprovalPolicy    bool
	ApprovalMode             config.AgentApprovalMode
	ProjectID                string
	Workspace                string
	ApprovalRules            []config.AgentApprovalRule
	ToolResultMaxBytes       int
	ToolResultEagerMinTokens int
	ContextWindowTokens      int
}

func NewOrchestratorMiddleware(cfg OrchestratorConfig) *OrchestratorMiddleware {
	return &OrchestratorMiddleware{
		BaseMiddleware:           &agent.BaseMiddleware{},
		agentKind:                cfg.AgentKind,
		policyKind:               cfg.PolicyKind,
		toolSettings:             cfg.ToolSettings,
		enforceToolSettings:      cfg.EnforceToolSettings,
		enforceApprovalPolicy:    cfg.EnforceApprovalPolicy,
		approvalMode:             cfg.ApprovalMode,
		projectID:                strings.TrimSpace(cfg.ProjectID),
		workspace:                cfg.Workspace,
		approvalRules:            config.NormalizeAgentApprovalRules(cfg.ApprovalRules),
		toolResultMaxBytes:       cfg.ToolResultMaxBytes,
		toolResultEagerMinTokens: cfg.ToolResultEagerMinTokens,
		contextWindowTokens:      cfg.ContextWindowTokens,
		executionGate:            sharedToolExecutionGate(cfg.Workspace),
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
		EnforceToolSettings: m.enforceToolSettings, EnforceApprovalPolicy: m.enforceApprovalPolicy,
		ApprovalMode: m.approvalMode, ProjectID: m.projectID, Workspace: m.workspace,
		ApprovalRules:      m.approvalRulesSnapshot(),
		ToolResultMaxBytes: m.toolResultLimitBytes(), ToolResultEagerMinTokens: m.toolResultEagerMinTokens,
		ContextWindowTokens: m.contextWindowTokens,
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
		return fmt.Sprintf("[tool error] Director 的 read 仅允许当前机会索引中的 event:// 事件卡；规划文档已在上下文中完整提供，请用 %s 提交带 base_hash 的 Markdown Patch。", SubmitDirectorPlanUpdateToolName)
	case "list_lore_items", "read_lore_items", "search_story_history", SubmitDirectorPlanUpdateToolName:
		return ""
	case "write", "edit":
		return fmt.Sprintf("[tool error] Director 规划文档已在上下文中完整提供；请用 %s 提交带 base_hash 的 Markdown Patch，拒绝工具: %s", SubmitDirectorPlanUpdateToolName, name)
	case "apply_actor_state_patch":
		return fmt.Sprintf("[tool error] Director 只维护 ArcPlan，不能写 Actor State，拒绝工具: %s", name)
	default:
		return fmt.Sprintf("[tool error] Director 只能使用 %s、历史检索、资料库只读和 read(event://...)，拒绝工具: %s", SubmitDirectorPlanUpdateToolName, name)
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
		var approvalErr error
		decision, approvalErr = m.applyApprovalPolicy(ctx, decision, args)
		if approvalErr != nil {
			return agent.ToolResult{}, approvalErr
		}
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
			filtered := toolresult.FilterStructured(
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
			toolErr := err
			if decision.Descriptor.Steering == agent.SteeringInterruptibleWait && agent.ToolSteeringPending(ctx) {
				result = agent.SyntheticToolResult(agent.ToolResultSkipped, agent.ToolSyntheticSteeringInterrupted,
					fmt.Sprintf("tool %q was interrupted to apply pending user steering", decision.ToolName))
			} else if agent.IsInterruptError(err) || ctx.Err() != nil {
				// Immediate abort keeps an unmatched durable start. Recovery will
				// materialize effect_unknown and never auto-retry the side effect.
				return agent.ToolResult{}, err
			} else {
				result, record := projectToolError(decision, args, result, toolErr, m.toolResultLimitBytes())
				result, effectErr := appendAgentMutationEffect(result, record)
				if effectErr != nil {
					return result, effectErr
				}
				if recordErr := recordToolFinish(ctx, record); recordErr != nil {
					return result, recordErr
				}
				if agent.IsToolControlError(toolErr) {
					return result, toolErr
				}
				return result, nil
			}
		}

		processed, processErr := processToolResult(ctx, decision, args, result, toolresult.ProcessingPolicy{
			MaxBytes: m.toolResultLimitBytes(), EagerMinTokens: m.toolResultEagerMinTokens,
			ContextWindowTokens: m.contextWindowTokens,
		})
		if processErr != nil {
			projected, record := projectToolError(decision, args, processed, processErr, m.toolResultLimitBytes())
			projected, effectErr := appendAgentMutationEffect(projected, record)
			if effectErr != nil {
				return projected, effectErr
			}
			if recordErr := recordToolFinish(ctx, record); recordErr != nil {
				return projected, recordErr
			}
			if agent.IsToolControlError(processErr) {
				return projected, processErr
			}
			return projected, nil
		}
		result = processed
		filtered := toolresult.FilterStructured(
			toolName(toolCtx), decision.Descriptor, args, result, m.toolResultLimitBytes(),
		)
		record := toolExecutionRecordFromFiltered(decision, filtered, string(filtered.Result.Status))
		applyToolMutationReceiptToExecutionRecord(&record, result)
		applyInteractiveTurnReceiptToExecutionRecord(&record, filtered.Result)
		filtered.Result, err = appendAgentMutationEffect(filtered.Result, record)
		if err != nil {
			return filtered.Result, err
		}
		if err := recordToolFinish(ctx, record); err != nil {
			return filtered.Result, err
		}
		return filtered.Result, nil
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

func (m *OrchestratorMiddleware) applyApprovalPolicy(
	ctx context.Context,
	decision agenttool.Decision,
	args string,
) (agenttool.Decision, error) {
	if m == nil || !m.enforceApprovalPolicy || decision.Action == "blocked" {
		return decision, nil
	}
	mode := config.NormalizeAgentApprovalMode(m.approvalMode)
	policyDecision := toolapproval.Evaluate(toolapproval.Request{
		Mode: mode, ProjectID: m.projectID, Workspace: m.workspace, ToolName: decision.ToolName,
		Arguments: args, Descriptor: decision.Descriptor,
		Rules: m.approvalRulesSnapshot(),
	})
	if err := policyDecision.Validate(); err != nil {
		return decision, fmt.Errorf("evaluate tool approval policy: %w", err)
	}
	decision.ApprovalMode = mode
	decision.ApprovalRuleID = policyDecision.RuleID
	decision.ApprovalRisk = policyDecision.Risk
	switch policyDecision.Action {
	case toolapproval.ActionAllow:
		return decision, nil
	case toolapproval.ActionDeny:
		decision.Action = "blocked"
		decision.Reason = "[tool error] " + policyDecision.Reason
		return decision, nil
	case toolapproval.ActionPrompt:
		decision.ApprovalRequired = true
		host, ok := approvalHostFromContext(ctx)
		if !ok {
			granted := false
			decision.ApprovalGranted = &granted
			decision.Action = "blocked"
			decision.Reason = "[tool error] This call requires approval, but the run has no recoverable interactive host and was safely blocked."
			return decision, nil
		}
		approval, err := host.ApproveTool(ctx, ApprovalRequest{
			Mode: mode, ToolName: decision.ToolName,
			ProviderCallID: decision.ProviderCallID, ExecutionID: decision.ExecutionID,
			Arguments: args, Decision: policyDecision,
		})
		if err != nil {
			return decision, fmt.Errorf("await approval for tool %q: %w", decision.ToolName, err)
		}
		if err := approval.Validate(); err != nil {
			return decision, err
		}
		granted := approval.Choice != ApprovalDenied
		decision.ApprovalGranted = &granted
		if !granted {
			decision.Action = "blocked"
			decision.Reason = "[tool error] The user denied this tool call."
			return decision, nil
		}
		if approval.Choice == ApprovalAllowWorkspace {
			if policyDecision.Remember == nil {
				return decision, fmt.Errorf("tool approval rule %q cannot be remembered", policyDecision.RuleID)
			}
			rule, ruleErr := toolapproval.NewWorkspaceRule(
				m.projectID, m.workspace, decision.ToolName, *policyDecision.Remember,
				toolapproval.ArgumentsHash(args), policyDecision.Command, policyDecision.Cwd,
				policyDecision.RuleID, time.Now(),
			)
			if ruleErr != nil {
				return decision, ruleErr
			}
			m.rememberApprovalRule(rule)
		}
		return decision, nil
	default:
		return decision, fmt.Errorf("unhandled tool approval action %q", policyDecision.Action)
	}
}

func (m *OrchestratorMiddleware) approvalRulesSnapshot() []config.AgentApprovalRule {
	if m == nil {
		return nil
	}
	m.approvalRulesMu.RLock()
	defer m.approvalRulesMu.RUnlock()
	return config.NormalizeAgentApprovalRules(m.approvalRules)
}

func (m *OrchestratorMiddleware) rememberApprovalRule(rule config.AgentApprovalRule) {
	if m == nil {
		return
	}
	m.approvalRulesMu.Lock()
	defer m.approvalRulesMu.Unlock()
	for index := range m.approvalRules {
		if m.approvalRules[index].ID == rule.ID {
			m.approvalRules[index] = rule
			return
		}
	}
	m.approvalRules = append(m.approvalRules, rule)
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
	filtered := toolresult.FilterStructured(
		decision.ToolName, decision.Descriptor, args,
		errorResult, maxBytes,
	)
	record := toolExecutionRecordFromFiltered(decision, filtered, "error")
	record.Error = boundedToolErrorDiagnostic(err)
	applyToolMutationReceiptToExecutionRecord(&record, returned)
	return filtered.Result, record
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
		decision.Reason = fmt.Sprintf("[tool error] 工具 %q 没有显式 ToolDescriptor，已在执行前拒绝。 / Tool %q has no explicit ToolDescriptor and was rejected before execution.", manifest.Name, manifest.Name)
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
	return fmt.Sprintf("[tool error] 工具 %q 需要当前 Agent 启用 %s 能力，但该能力已关闭。请改用已授权工具，或请用户在 Agent Tools 中开启该能力。 / Tool %q requires capability %s, which is disabled for this Agent.", name, capability, name, capability)
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
