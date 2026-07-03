package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// toolOrchestratorMiddleware centralizes Nova's internal tool execution policy.
type toolOrchestratorMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	agentKind          string
	policyKind         string
	toolResultMaxBytes int
}

type interactiveStoryToolMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
}

type interactiveDirectorPlanFileMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	allowedPaths map[string]bool
}

func newInteractiveStoryToolMiddleware() *interactiveStoryToolMiddleware {
	return &interactiveStoryToolMiddleware{}
}

func newInteractiveDirectorPlanFileMiddleware(allowedPaths []string) *interactiveDirectorPlanFileMiddleware {
	allowed := make(map[string]bool, len(allowedPaths))
	for _, path := range allowedPaths {
		cleaned := cleanDirectorPlanToolPath(path)
		if cleaned != "" {
			allowed[cleaned] = true
		}
	}
	return &interactiveDirectorPlanFileMiddleware{allowedPaths: allowed}
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

func (m *interactiveDirectorPlanFileMiddleware) blockedDirectorToolMessage(name, args string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "read_file", "write_file", "edit_file":
		target := cleanDirectorPlanToolPath(toolPathFromArgs(args))
		if target == "" {
			return fmt.Sprintf("[tool error] interactive_director 工具 %q 必须提供 file_path。", name)
		}
		if m == nil || !m.allowedPaths[target] {
			return fmt.Sprintf("[tool error] interactive_director 只能访问当前分支导演规划文件，拒绝路径: %s", target)
		}
		return ""
	default:
		return fmt.Sprintf("[tool error] interactive_director 只能使用 read_file、write_file、edit_file 更新当前分支导演规划，拒绝工具: %s", name)
	}
}

func cleanDirectorPlanToolPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return filepath.Clean(path)
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
	return fmt.Sprintf("[tool error] 游戏模式禁止使用写文件工具 %q。请不要修改 workspace 文件，只输出本回合故事正文；状态变化由后端状态 Agent 异步写入 story jsonl。", name)
}

type ToolDecision struct {
	ToolName          string     `json:"tool_name"`
	ToolCallID        string     `json:"tool_call_id,omitempty"`
	Source            ToolSource `json:"source"`
	Action            string     `json:"action"`
	Reason            string     `json:"reason,omitempty"`
	MutatesWorkspace  bool       `json:"mutates_workspace"`
	RequiresPostCheck bool       `json:"requires_post_check"`
	Target            string     `json:"target,omitempty"`
}

type ToolExecutionRecord struct {
	ToolName       string `json:"tool_name"`
	ToolCallID     string `json:"tool_call_id,omitempty"`
	Status         string `json:"status"`
	OriginalBytes  int    `json:"original_bytes,omitempty"`
	ReturnedBytes  int    `json:"returned_bytes,omitempty"`
	Truncated      bool   `json:"truncated,omitempty"`
	Target         string `json:"target,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
	Error          string `json:"error,omitempty"`
}

func (m *toolOrchestratorMiddleware) WrapInvokableToolCall(
	_ context.Context,
	endpoint adk.InvokableToolCallEndpoint,
	toolCtx *adk.ToolContext,
) (adk.InvokableToolCallEndpoint, error) {
	return func(ctx context.Context, args string, opts ...tool.Option) (string, error) {
		decision := m.buildToolDecision(toolCtx, args)
		decision = applyToolArgumentValidation(decision, args)
		observer := RunObserverFromContext(ctx)
		observer.RecordToolDecision(decision)
		if decision.Action == "blocked" {
			msg := decision.Reason
			if msg == "" {
				msg = fmt.Sprintf("[tool error] 工具 %q 被当前 Agent 策略阻止。", decision.ToolName)
			}
			observer.RecordToolExecution(ToolExecutionRecord{
				ToolName:   decision.ToolName,
				ToolCallID: decision.ToolCallID,
				Status:     "blocked",
				Target:     decision.Target,
				Error:      msg,
			})
			return msg, nil
		}
		result, err := endpoint(ctx, args, opts...)
		if err != nil {
			if _, ok := compose.IsInterruptRerunError(err); ok {
				return "", err
			}
			msg := fmt.Sprintf("[tool error] %v", err)
			observer.RecordToolExecution(ToolExecutionRecord{
				ToolName:   decision.ToolName,
				ToolCallID: decision.ToolCallID,
				Status:     "error",
				Target:     decision.Target,
				Error:      err.Error(),
			})
			return msg, nil
		}
		filtered := FilterToolResultForModelWithLimit(toolName(toolCtx), args, result, m.toolResultLimitBytes())
		observer.RecordToolExecution(ToolExecutionRecord{
			ToolName:       filtered.Manifest.Name,
			ToolCallID:     decision.ToolCallID,
			Status:         "success",
			OriginalBytes:  filtered.OriginalBytes,
			ReturnedBytes:  filtered.ReturnedBytes,
			Truncated:      filtered.Truncated,
			Target:         filtered.Target,
			IdempotencyKey: filtered.IdempotencyKey,
		})
		return filtered.Content, nil
	}, nil
}

func (m *toolOrchestratorMiddleware) WrapStreamableToolCall(
	_ context.Context,
	endpoint adk.StreamableToolCallEndpoint,
	toolCtx *adk.ToolContext,
) (adk.StreamableToolCallEndpoint, error) {
	return func(ctx context.Context, args string, opts ...tool.Option) (*schema.StreamReader[string], error) {
		decision := m.buildToolDecision(toolCtx, args)
		decision = applyToolArgumentValidation(decision, args)
		observer := RunObserverFromContext(ctx)
		observer.RecordToolDecision(decision)
		if decision.Action == "blocked" {
			msg := decision.Reason
			if msg == "" {
				msg = fmt.Sprintf("[tool error] 工具 %q 被当前 Agent 策略阻止。", decision.ToolName)
			}
			observer.RecordToolExecution(ToolExecutionRecord{
				ToolName:   decision.ToolName,
				ToolCallID: decision.ToolCallID,
				Status:     "blocked",
				Target:     decision.Target,
				Error:      msg,
			})
			return singleChunkReader(msg), nil
		}
		sr, err := endpoint(ctx, args, opts...)
		if err != nil {
			if _, ok := compose.IsInterruptRerunError(err); ok {
				return nil, err
			}
			observer.RecordToolExecution(ToolExecutionRecord{
				ToolName:   decision.ToolName,
				ToolCallID: decision.ToolCallID,
				Status:     "error",
				Target:     decision.Target,
				Error:      err.Error(),
			})
			return singleChunkReader(fmt.Sprintf("[tool error] %v", err)), nil
		}
		return filterToolResultReader(ctx, sr, toolCtx, args, m.toolResultLimitBytes()), nil
	}, nil
}

func singleChunkReader(msg string) *schema.StreamReader[string] {
	r, w := schema.Pipe[string](1)
	_ = w.Send(msg, nil)
	w.Close()
	return r
}

func safeWrapReader(sr *schema.StreamReader[string]) *schema.StreamReader[string] {
	r, w := schema.Pipe[string](64)
	go func() {
		defer w.Close()
		for {
			chunk, err := sr.Recv()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				_ = w.Send(fmt.Sprintf("\n[tool error] %v", err), nil)
				return
			}
			_ = w.Send(chunk, nil)
		}
	}()
	return r
}

func filterToolResultReader(ctx context.Context, sr *schema.StreamReader[string], toolCtx *adk.ToolContext, args string, maxBytes int) *schema.StreamReader[string] {
	r, w := schema.Pipe[string](1)
	go func() {
		defer w.Close()
		name := toolName(toolCtx)
		manifest := ManifestForTool(name)
		manifest.MaxResultBytes = normalizeToolResultLimitBytes(maxBytes)
		limit := normalizedToolResultLimit(manifest)
		var content strings.Builder
		originalBytes := 0
		for {
			chunk, err := sr.Recv()
			if errors.Is(err, io.EOF) {
				filtered := filteredToolResultFromBody(manifest, args, content.String(), originalBytes, originalBytes > content.Len())
				RunObserverFromContext(ctx).RecordToolExecution(ToolExecutionRecord{
					ToolName:       filtered.Manifest.Name,
					ToolCallID:     toolCallID(toolCtx),
					Status:         "success",
					OriginalBytes:  filtered.OriginalBytes,
					ReturnedBytes:  filtered.ReturnedBytes,
					Truncated:      filtered.Truncated,
					Target:         filtered.Target,
					IdempotencyKey: filtered.IdempotencyKey,
				})
				_ = w.Send(filtered.Content, nil)
				return
			}
			if err != nil {
				RunObserverFromContext(ctx).RecordToolExecution(ToolExecutionRecord{
					ToolName:   manifest.Name,
					ToolCallID: toolCallID(toolCtx),
					Status:     "error",
					Target:     toolPathFromArgs(args),
					Error:      err.Error(),
				})
				_ = w.Send(fmt.Sprintf("\n[tool error] %v", err), nil)
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

func (m *toolOrchestratorMiddleware) toolResultLimitBytes() int {
	if m == nil {
		return 0
	}
	return normalizeToolResultLimitBytes(m.toolResultMaxBytes)
}

func (m *toolOrchestratorMiddleware) buildToolDecision(toolCtx *adk.ToolContext, args string) ToolDecision {
	name := toolName(toolCtx)
	manifest := ManifestForTool(name)
	decision := ToolDecision{
		ToolName:          manifest.Name,
		ToolCallID:        toolCallID(toolCtx),
		Source:            manifest.Source,
		Action:            "allowed",
		MutatesWorkspace:  manifest.MutatesWorkspace,
		RequiresPostCheck: manifest.RequiresPostCheck,
		Target:            toolPathFromArgs(args),
	}
	if m != nil && m.effectivePolicyKind() == AgentKindInteractiveStory && isInteractiveStoryWriteTool(name) {
		decision.Action = "blocked"
		decision.Reason = interactiveStoryWriteToolBlockedMessage(name)
	}
	return decision
}

func applyToolArgumentValidation(decision ToolDecision, args string) ToolDecision {
	if decision.Action == "blocked" {
		return decision
	}
	if msg := invalidToolArgumentsMessage(decision.ToolName, args); msg != "" {
		decision.Action = "blocked"
		decision.Reason = msg
	}
	return decision
}

func invalidToolArgumentsMessage(toolName, args string) string {
	if err := validateToolArgumentsJSON(args); err != nil {
		return fmt.Sprintf("[tool error] 工具 %q 的参数不是完整 JSON 对象：%v。请重新发起同一个工具调用，并保证 arguments 是完整、合法的 JSON object；字符串里的换行、引号和反斜杠必须正确转义。 / Tool arguments must be a complete JSON object; escape newlines, quotes, and backslashes inside strings.", toolName, err)
	}
	return ""
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
