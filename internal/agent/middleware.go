package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"denova/config"
)

const maxToolErrorDiagnosticBytes = 4 * 1024

// toolOrchestratorMiddleware centralizes Nova's internal tool execution policy.
type toolOrchestratorMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	agentKind           string
	policyKind          string
	toolSettings        config.ResolvedAgentToolSettings
	enforceToolSettings bool
	toolResultMaxBytes  int
	executionGate       *toolExecutionGate
}

type interactiveStoryToolMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
}

type interactiveDirectorPlanFileMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
}

func newInteractiveStoryToolMiddleware() *interactiveStoryToolMiddleware {
	return &interactiveStoryToolMiddleware{}
}

func newInteractiveDirectorPlanFileMiddleware() *interactiveDirectorPlanFileMiddleware {
	return &interactiveDirectorPlanFileMiddleware{}
}

func (m *interactiveDirectorPlanFileMiddleware) WrapInvokableToolCall(
	_ context.Context,
	endpoint adk.InvokableToolCallEndpoint,
	toolCtx *adk.ToolContext,
) (adk.InvokableToolCallEndpoint, error) {
	return func(ctx context.Context, args string, opts ...tool.Option) (string, error) {
		if msg := m.blockedDirectorToolMessage(toolName(toolCtx), args); msg != "" {
			return msg, nil
		}
		return endpoint(ctx, args, opts...)
	}, nil
}

func (m *interactiveDirectorPlanFileMiddleware) WrapStreamableToolCall(
	_ context.Context,
	endpoint adk.StreamableToolCallEndpoint,
	toolCtx *adk.ToolContext,
) (adk.StreamableToolCallEndpoint, error) {
	return func(ctx context.Context, args string, opts ...tool.Option) (*schema.StreamReader[string], error) {
		if msg := m.blockedDirectorToolMessage(toolName(toolCtx), args); msg != "" {
			return singleChunkReader(msg), nil
		}
		return endpoint(ctx, args, opts...)
	}, nil
}

func (m *interactiveDirectorPlanFileMiddleware) blockedDirectorToolMessage(name, _ string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "read_event_cards", "list_lore_items", "read_lore_items", "search_story_history", submitDirectorPlanUpdateToolName:
		return ""
	case "read_file", "write_file", "edit_file":
		return fmt.Sprintf("[tool error] Director 规划文档已在上下文中完整提供；请用 %s 提交带 base_hash 的 Markdown Patch，拒绝工具: %s", submitDirectorPlanUpdateToolName, name)
	case "apply_actor_state_patch":
		return fmt.Sprintf("[tool error] Director 只维护 ArcPlan，不能写 Actor State，拒绝工具: %s", name)
	default:
		return fmt.Sprintf("[tool error] Director 只能使用 %s、历史检索、资料库只读和事件卡工具，拒绝工具: %s", submitDirectorPlanUpdateToolName, name)
	}
}

func (m *interactiveStoryToolMiddleware) WrapInvokableToolCall(
	_ context.Context,
	endpoint adk.InvokableToolCallEndpoint,
	toolCtx *adk.ToolContext,
) (adk.InvokableToolCallEndpoint, error) {
	return func(ctx context.Context, args string, opts ...tool.Option) (string, error) {
		if isInteractiveStoryWriteTool(toolName(toolCtx)) {
			return interactiveStoryWriteToolBlockedMessage(toolName(toolCtx)), nil
		}
		return endpoint(ctx, args, opts...)
	}, nil
}

func (m *interactiveStoryToolMiddleware) WrapStreamableToolCall(
	_ context.Context,
	endpoint adk.StreamableToolCallEndpoint,
	toolCtx *adk.ToolContext,
) (adk.StreamableToolCallEndpoint, error) {
	return func(ctx context.Context, args string, opts ...tool.Option) (*schema.StreamReader[string], error) {
		if isInteractiveStoryWriteTool(toolName(toolCtx)) {
			return singleChunkReader(interactiveStoryWriteToolBlockedMessage(toolName(toolCtx))), nil
		}
		return endpoint(ctx, args, opts...)
	}, nil
}

func toolName(toolCtx *adk.ToolContext) string {
	if toolCtx == nil {
		return ""
	}
	return toolCtx.Name
}

func isInteractiveStoryWriteTool(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "write_file", "edit_file", "delete_file", "create_file", "move_file", "copy_file", "rename_file", "mkdir", "remove_file":
		return true
	}
	return strings.HasPrefix(name, "write_") ||
		strings.HasPrefix(name, "edit_") ||
		strings.HasPrefix(name, "delete_") ||
		strings.HasPrefix(name, "create_") ||
		strings.HasPrefix(name, "move_") ||
		strings.HasPrefix(name, "copy_") ||
		strings.HasPrefix(name, "rename_")
}

func interactiveStoryWriteToolBlockedMessage(name string) string {
	return fmt.Sprintf("[tool error] 游戏模式禁止使用写文件工具 %q。请不要修改 workspace 文件；先直接输出完整故事正文，再用 submit_interactive_turn 提交一致的隐藏回合结果。", name)
}

type ToolDecision struct {
	ToolName          string     `json:"tool_name"`
	ToolCallID        string     `json:"tool_call_id,omitempty"`
	Source            ToolSource `json:"source"`
	Capability        string     `json:"capability,omitempty"`
	Action            string     `json:"action"`
	Reason            string     `json:"reason,omitempty"`
	MutatesWorkspace  bool       `json:"mutates_workspace"`
	RequiresPostCheck bool       `json:"requires_post_check"`
	Target            string     `json:"target,omitempty"`
	ArgsBytes         int        `json:"args_bytes,omitempty"`
	ArgsComplete      *bool      `json:"args_complete,omitempty"`
	ModelFinishReason string     `json:"model_finish_reason,omitempty"`
}

type ToolExecutionRecord struct {
	ToolName              string   `json:"tool_name"`
	ToolCallID            string   `json:"tool_call_id,omitempty"`
	Workspace             string   `json:"workspace,omitempty"`
	Status                string   `json:"status"`
	Result                string   `json:"result,omitempty"`
	DomainStatus          string   `json:"domain_status,omitempty"`
	DomainDiagnosticCount int      `json:"domain_diagnostic_count,omitempty"`
	RetryModules          []string `json:"retry_modules,omitempty"`
	Capability            string   `json:"capability,omitempty"`
	OriginalBytes         int      `json:"original_bytes,omitempty"`
	ReturnedBytes         int      `json:"returned_bytes,omitempty"`
	Truncated             bool     `json:"truncated,omitempty"`
	Target                string   `json:"target,omitempty"`
	IdempotencyKey        string   `json:"idempotency_key,omitempty"`
	Error                 string   `json:"error,omitempty"`
	ArgsBytes             int      `json:"args_bytes,omitempty"`
	ArgsComplete          *bool    `json:"args_complete,omitempty"`
	ModelFinishReason     string   `json:"model_finish_reason,omitempty"`
	ChangeGroupID         string   `json:"change_group_id,omitempty"`
	ReviewThreadID        string   `json:"review_thread_id,omitempty"`
	ChangeSetID           string   `json:"change_set_id,omitempty"`
	BaseRevision          string   `json:"base_revision,omitempty"`
	Revision              string   `json:"revision,omitempty"`
	ReviewStatus          string   `json:"review_status,omitempty"`
	ApplyState            string   `json:"apply_state,omitempty"`
	LoreItemIDs           []string `json:"lore_item_ids,omitempty"`
	DeletedLoreItemIDs    []string `json:"deleted_lore_item_ids,omitempty"`
}

func (m *toolOrchestratorMiddleware) WrapInvokableToolCall(
	_ context.Context,
	endpoint adk.InvokableToolCallEndpoint,
	toolCtx *adk.ToolContext,
) (adk.InvokableToolCallEndpoint, error) {
	return func(ctx context.Context, args string, opts ...tool.Option) (string, error) {
		decision := m.buildToolDecision(toolCtx, args)
		observer := RunObserverFromContext(ctx)
		outcome := LLMOutcome{}
		if observer != nil {
			outcome = observer.LastLLMOutcome()
		}
		decision = applyModelOutputToolSafety(decision, outcome)
		decision = applyToolArgumentValidation(decision, args, outcome)
		observer.RecordToolDecision(decision)
		if decision.Action == "blocked" {
			msg := decision.Reason
			if msg == "" {
				msg = fmt.Sprintf("[tool error] 工具 %q 被当前 Agent 策略阻止。", decision.ToolName)
			}
			observer.RecordToolExecution(blockedToolExecutionRecord(decision, msg))
			return msg, nil
		}
		release := m.acquireToolExecution(decision)
		defer release()
		if err := recordToolStart(ctx, decision, args); err != nil {
			return "", err
		}
		result, err := endpoint(ctx, args, opts...)
		if err != nil {
			msg, record := projectToolError(decision, args, err, m.toolResultLimitBytes())
			if recordErr := recordToolFinish(ctx, record); recordErr != nil {
				return "", recordErr
			}
			if _, ok := compose.IsInterruptRerunError(err); ok {
				return "", err
			}
			return msg, nil
		}
		filtered := FilterToolResultForModelWithLimit(toolName(toolCtx), args, result, m.toolResultLimitBytes())
		record := toolExecutionRecordFromFiltered(decision, filtered, "success")
		applyToolMutationReceiptToExecutionRecord(&record, result)
		applyInteractiveTurnReceiptToExecutionRecord(&record, result)
		if err := recordToolFinish(ctx, record); err != nil {
			return "", err
		}
		return filtered.Content, nil
	}, nil
}

func applyInteractiveTurnReceiptToExecutionRecord(record *ToolExecutionRecord, result string) {
	if record == nil || !IsInteractiveTurnSubmissionTool(record.ToolName) {
		return
	}
	var receipt struct {
		Ready        bool              `json:"ready"`
		ModuleStatus map[string]string `json:"module_status"`
		Diagnostics  []json.RawMessage `json:"diagnostics"`
		RetryModules []string          `json:"retry_modules"`
	}
	if err := json.Unmarshal([]byte(result), &receipt); err != nil || receipt.ModuleStatus == nil {
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

func (m *toolOrchestratorMiddleware) WrapStreamableToolCall(
	_ context.Context,
	endpoint adk.StreamableToolCallEndpoint,
	toolCtx *adk.ToolContext,
) (adk.StreamableToolCallEndpoint, error) {
	return func(ctx context.Context, args string, opts ...tool.Option) (*schema.StreamReader[string], error) {
		decision := m.buildToolDecision(toolCtx, args)
		observer := RunObserverFromContext(ctx)
		outcome := LLMOutcome{}
		if observer != nil {
			outcome = observer.LastLLMOutcome()
		}
		decision = applyModelOutputToolSafety(decision, outcome)
		decision = applyToolArgumentValidation(decision, args, outcome)
		observer.RecordToolDecision(decision)
		if decision.Action == "blocked" {
			msg := decision.Reason
			if msg == "" {
				msg = fmt.Sprintf("[tool error] 工具 %q 被当前 Agent 策略阻止。", decision.ToolName)
			}
			observer.RecordToolExecution(blockedToolExecutionRecord(decision, msg))
			return singleChunkReader(msg), nil
		}
		release := m.acquireToolExecution(decision)
		if err := recordToolStart(ctx, decision, args); err != nil {
			release()
			return nil, err
		}
		sr, err := endpoint(ctx, args, opts...)
		if err != nil {
			release()
			msg, record := projectToolError(decision, args, err, m.toolResultLimitBytes())
			if recordErr := recordToolFinish(ctx, record); recordErr != nil {
				return nil, recordErr
			}
			if _, ok := compose.IsInterruptRerunError(err); ok {
				return nil, err
			}
			return singleChunkReader(msg), nil
		}
		return filterToolResultReader(ctx, sr, decision, args, m.toolResultLimitBytes(), release), nil
	}, nil
}

func toolEndpointErrorMessage(toolName string, err error) string {
	if msg, ok := formatWorkspaceChangeToolError(toolName, err); ok {
		return msg
	}
	return fmt.Sprintf("[tool error] %v", err)
}

func projectToolError(decision ToolDecision, args string, err error, maxBytes int) (string, ToolExecutionRecord) {
	modelError := strings.ToValidUTF8(toolEndpointErrorMessage(decision.ToolName, err), "\uFFFD")
	filtered := FilterToolResultForModelWithLimit(
		decision.ToolName,
		args,
		modelError,
		maxBytes,
	)
	record := toolExecutionRecordFromFiltered(decision, filtered, "error")
	record.Error = boundedToolErrorDiagnostic(err)
	return filtered.Content, record
}

func toolExecutionRecordFromFiltered(decision ToolDecision, filtered FilteredToolResult, status string) ToolExecutionRecord {
	return ToolExecutionRecord{
		ToolName:       filtered.Manifest.Name,
		ToolCallID:     decision.ToolCallID,
		Status:         status,
		Capability:     filtered.Manifest.Capability,
		OriginalBytes:  filtered.OriginalBytes,
		ReturnedBytes:  filtered.ReturnedBytes,
		Truncated:      filtered.Truncated,
		Target:         filtered.Target,
		IdempotencyKey: filtered.IdempotencyKey,
		Result:         filtered.Content,
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

func (m *toolOrchestratorMiddleware) acquireToolExecution(decision ToolDecision) func() {
	if m == nil || m.executionGate == nil {
		return func() {}
	}
	manifest := ManifestForTool(decision.ToolName)
	return m.executionGate.acquire(executionModeForTool(manifest))
}

func singleChunkReader(msg string) *schema.StreamReader[string] {
	r, w := schema.Pipe[string](1)
	_ = w.Send(msg, nil)
	w.Close()
	return r
}

func filterToolResultReader(ctx context.Context, sr *schema.StreamReader[string], decision ToolDecision, args string, maxBytes int, releases ...func()) *schema.StreamReader[string] {
	r, w := schema.Pipe[string](1)
	go func() {
		defer w.Close()
		defer func() {
			for _, release := range releases {
				if release != nil {
					release()
				}
			}
		}()
		defer func() {
			if recovered := recover(); recovered != nil {
				panicErr := fmt.Errorf("panic while reading tool result: %v", recovered)
				msg, record := projectToolError(decision, args, panicErr, maxBytes)
				if err := recordToolFinish(ctx, record); err != nil {
					_ = w.Send("", err)
					return
				}
				_ = w.Send(msg, nil)
			}
		}()
		if sr == nil {
			streamErr := errors.New("streamable tool returned a nil result stream")
			msg, record := projectToolError(decision, args, streamErr, maxBytes)
			if err := recordToolFinish(ctx, record); err != nil {
				_ = w.Send("", err)
				return
			}
			_ = w.Send(msg, nil)
			return
		}
		defer sr.Close()
		manifest := ManifestForTool(decision.ToolName)
		manifest.MaxResultBytes = normalizeToolResultLimitBytes(maxBytes)
		limit := normalizedToolResultLimit(manifest)
		var content strings.Builder
		originalBytes := 0
		for {
			// This goroutine owns both the source stream and the workspace safety
			// lease. Keep draining the actual producer after caller cancellation:
			// releasing the lease while a non-cooperative tool may still mutate
			// files would let another writer overlap it.
			chunk, err := sr.Recv()
			if errors.Is(err, io.EOF) {
				body := content.String()
				truncated := originalBytes > len(body)
				filtered := filteredToolResultFromBody(manifest, args, body, originalBytes, truncated)
				record := toolExecutionRecordFromFiltered(decision, filtered, "success")
				// Structured receipts are safety evidence, not display hints. Never
				// infer them from the bounded model preview: a complete-looking JSON
				// prefix can still omit fields from the actual stream. The durable
				// execution record still carries tool identity and the args-derived
				// target, so mutating streams produce a conservative HostEffect.
				if !truncated {
					applyToolMutationReceiptToExecutionRecord(&record, body)
					applyInteractiveTurnReceiptToExecutionRecord(&record, body)
				}
				if err := recordToolFinish(ctx, record); err != nil {
					_ = w.Send("", err)
					return
				}
				_ = w.Send(filtered.Content, nil)
				return
			}
			if err != nil {
				msg, record := projectToolError(decision, args, err, maxBytes)
				if recordErr := recordToolFinish(ctx, record); recordErr != nil {
					_ = w.Send("", recordErr)
					return
				}
				_ = w.Send(msg, nil)
				return
			}
			originalBytes += len(chunk)
			if limit <= 0 {
				content.WriteString(chunk)
				continue
			}
			if content.Len() >= limit {
				continue
			}
			remaining := limit - content.Len()
			if len(chunk) <= remaining {
				content.WriteString(chunk)
				continue
			}
			fragment, _ := truncateUTF8Bytes(chunk, remaining)
			content.WriteString(strings.TrimSuffix(fragment, "\n[tool result truncated]"))
		}
	}()
	return r
}

func applyWorkspaceChangeReceiptToExecutionRecord(record *ToolExecutionRecord, content string) {
	if record == nil {
		return
	}
	receipt, ok := parseWorkspaceChangeToolReceipt(record.ToolName, content)
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

func applyToolMutationReceiptToExecutionRecord(record *ToolExecutionRecord, content string) {
	if record == nil {
		return
	}
	applyWorkspaceChangeReceiptToExecutionRecord(record, content)
	itemIDs, deletedIDs := parseWriteLoreItemsToolResult(record.ToolName, content)
	record.LoreItemIDs = uniqueStrings(itemIDs)
	record.DeletedLoreItemIDs = uniqueStrings(deletedIDs)
}

func blockedToolExecutionRecord(decision ToolDecision, msg string) ToolExecutionRecord {
	return ToolExecutionRecord{
		ToolName:          decision.ToolName,
		ToolCallID:        decision.ToolCallID,
		Status:            "blocked",
		Capability:        decision.Capability,
		Target:            decision.Target,
		Error:             msg,
		ArgsBytes:         decision.ArgsBytes,
		ArgsComplete:      decision.ArgsComplete,
		ModelFinishReason: decision.ModelFinishReason,
	}
}

func (m *toolOrchestratorMiddleware) toolResultLimitBytes() int {
	if m == nil {
		return 0
	}
	return normalizeToolResultLimitBytes(m.toolResultMaxBytes)
}

func (m *toolOrchestratorMiddleware) buildToolDecision(toolCtx *adk.ToolContext, args string) ToolDecision {
	name := toolName(toolCtx)
	manifest, declared := declaredToolDescriptor(name)
	if !declared {
		manifest = DescriptorForTool(name)
	}
	decision := ToolDecision{
		ToolName:          manifest.Name,
		ToolCallID:        toolCallID(toolCtx),
		Source:            manifest.Source,
		Capability:        manifest.Capability,
		Action:            "allowed",
		MutatesWorkspace:  manifest.MutatesWorkspace,
		RequiresPostCheck: manifest.RequiresPostCheck,
		Target:            toolPathFromArgs(args),
		ArgsBytes:         len(args),
	}
	if m != nil && m.effectivePolicyKind() == AgentKindInteractiveStory && isInteractiveStoryWriteTool(name) {
		decision.Action = "blocked"
		decision.Reason = interactiveStoryWriteToolBlockedMessage(name)
		return decision
	}
	if m != nil && m.enforceToolSettings && !declared {
		decision.Action = "blocked"
		decision.Reason = fmt.Sprintf("[tool error] 工具 %q 没有显式 ToolDescriptor，已在执行前拒绝；请先注册能力、并发和恢复策略。 / Tool %q has no explicit ToolDescriptor and was rejected before execution.", manifest.Name, manifest.Name)
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

// applyModelOutputToolSafety rejects every tool call from a model response
// that ended at its output-token limit. A truncated suffix can still happen to
// be valid JSON; syntax validation alone therefore cannot prove the arguments
// represent the model's complete intent.
func applyModelOutputToolSafety(decision ToolDecision, outcome LLMOutcome) ToolDecision {
	reason := strings.TrimSpace(outcome.FinishReason)
	outputLimited := isOutputTokenLimitFinishReason(reason)
	contentFiltered := strings.EqualFold(reason, "content_filter")
	if !outputLimited && !contentFiltered {
		return decision
	}
	argsComplete := false
	decision.ArgsComplete = &argsComplete
	decision.ModelFinishReason = reason
	if decision.Action == "blocked" {
		return decision
	}
	decision.Action = "blocked"
	if contentFiltered {
		decision.Reason = contentFilteredModelToolArgumentsMessage(decision, reason)
	} else {
		decision.Reason = truncatedModelToolArgumentsMessage(decision, reason)
	}
	return decision
}

func isOutputTokenLimitFinishReason(reason string) bool {
	normalized := strings.ToLower(strings.TrimSpace(reason))
	normalized = strings.NewReplacer("-", "_", " ", "_").Replace(normalized)
	switch normalized {
	case "length", "max_tokens", "max_output_tokens", "token_limit":
		return true
	default:
		return false
	}
}

func truncatedModelToolArgumentsMessage(decision ToolDecision, finishReason string) string {
	target := strings.TrimSpace(decision.Target)
	if target == "" {
		target = "(unknown)"
	}
	return fmt.Sprintf(`[tool error]
type: incomplete_tool_arguments
tool: %s
reason: model_output_token_limit
retryable: true
workspace_mutated: false
args_complete: false
args_bytes: %d
model_finish_reason: %s
target: %s

中文：模型在生成本条回复时达到了输出 token 上限，因此本条回复中的工具参数不能视为完整意图；即使 arguments 恰好是合法 JSON，Denova 也已阻止执行且未产生工具副作用。请缩短参数或拆分任务后重新发起工具调用。
English: The model reached its output-token limit while generating this response, so this response's tool arguments cannot be treated as complete intent. Denova blocked execution even though the remaining arguments may be valid JSON, and the tool produced no side effects. Retry with shorter arguments or split the task.`, decision.ToolName, decision.ArgsBytes, finishReason, target)
}

func contentFilteredModelToolArgumentsMessage(decision ToolDecision, finishReason string) string {
	target := strings.TrimSpace(decision.Target)
	if target == "" {
		target = "(unknown)"
	}
	return fmt.Sprintf(`[tool error]
type: incomplete_tool_arguments
tool: %s
reason: model_output_interrupted_by_content_filter
retryable: false
workspace_mutated: false
args_complete: false
args_bytes: %d
model_finish_reason: %s
target: %s

中文：模型回复被内容过滤中断，因此其中的工具参数不能视为完整意图；即使 arguments 恰好是合法 JSON，Denova 也已阻止执行，文件未写入且工具未产生副作用。请直接告知用户本次失败原因，不要重试同一个工具调用。
English: Content filtering interrupted the model response, so its tool arguments cannot be treated as complete intent. Denova blocked execution even if the remaining arguments happen to be valid JSON; no file was written and the tool produced no side effects. Tell the user why it failed and do not retry the same tool call.`, decision.ToolName, decision.ArgsBytes, finishReason, target)
}

func applyToolArgumentValidation(decision ToolDecision, args string, outcome LLMOutcome) ToolDecision {
	if decision.Action == "blocked" {
		return decision
	}
	if err := validateToolArgumentsJSON(args); err != nil {
		argsComplete := false
		decision.ArgsComplete = &argsComplete
		decision.ModelFinishReason = strings.TrimSpace(outcome.FinishReason)
		decision.Action = "blocked"
		decision.Reason = invalidToolArgumentsMessage(decision, args, err, outcome)
	}
	return decision
}

func invalidToolArgumentsMessage(decision ToolDecision, args string, err error, outcome LLMOutcome) string {
	if isContentFilterInterruptedArguments(err, decision, outcome) {
		target := strings.TrimSpace(decision.Target)
		if target == "" {
			target = "(unknown)"
		}
		return fmt.Sprintf(`[tool error]
type: invalid_tool_arguments
tool: %s
reason: model_output_interrupted_by_content_filter
retryable: false
workspace_mutated: false
args_complete: false
args_bytes: %d
model_finish_reason: %s
target: %s

中文：模型在生成工具参数时被内容过滤中断，arguments 不是完整 JSON 对象：%v。Denova 已阻止工具执行，文件未写入。请直接告知用户本次写入失败的原因，不要重试同一个写入工具。
English: The model output was stopped by content filtering while producing tool arguments, so arguments are not a complete JSON object: %v. Denova blocked tool execution and no file was written. Tell the user what happened; do not retry the same write tool.`, decision.ToolName, len(args), strings.TrimSpace(outcome.FinishReason), target, err, err)
	}
	return fmt.Sprintf(`[tool error]
type: invalid_tool_arguments
tool: %s
retryable: true
workspace_mutated: false
args_complete: false
args_bytes: %d

中文：工具 %q 的参数不是完整 JSON 对象：%v。请修正 arguments，确保它是完整、合法的 JSON object；字符串里的换行、引号和反斜杠必须正确转义。
English: Tool %q arguments are not a complete JSON object: %v. Tool arguments must be a complete JSON object; fix arguments and escape newlines, quotes, and backslashes inside strings.`, decision.ToolName, len(args), decision.ToolName, err, decision.ToolName, err)
}

func isContentFilterInterruptedArguments(err error, decision ToolDecision, outcome LLMOutcome) bool {
	if !isIncompleteJSONArgumentsError(err) {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(outcome.FinishReason), "content_filter") {
		return false
	}
	return decision.MutatesWorkspace || decision.Source == ToolSourceWrite
}

func isIncompleteJSONArgumentsError(err error) bool {
	return errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, io.EOF) ||
		strings.Contains(strings.ToLower(err.Error()), "unexpected eof")
}

func validateToolArgumentsJSON(args string) error {
	args = strings.TrimSpace(args)
	if args == "" {
		return nil
	}
	decoder := json.NewDecoder(strings.NewReader(args))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		return err
	}
	if payload == nil {
		return fmt.Errorf("arguments must be a JSON object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("arguments contain trailing JSON data")
		}
		return fmt.Errorf("arguments contain trailing data: %w", err)
	}
	return nil
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

func toolCallID(toolCtx *adk.ToolContext) string {
	if toolCtx == nil {
		return ""
	}
	return toolCtx.CallID
}
