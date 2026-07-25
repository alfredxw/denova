package agents

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// applyModelOutputToolSafety rejects calls from an incomplete model response.
// The core scheduler still creates exactly one paired synthetic result.
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

中文：模型达到了输出 token 上限，工具参数不能视为完整意图。Denova 已阻止执行且未产生副作用；请缩短参数或拆分任务后重试。
English: The model reached its output-token limit, so the tool arguments are incomplete. Denova blocked execution with no side effects; retry with shorter arguments or split the task.`, decision.ToolName, decision.ArgsBytes, finishReason, target)
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

中文：内容过滤中断了模型回复，工具参数不能视为完整意图；Denova 已阻止执行且未产生副作用。
English: Content filtering interrupted the model response, so the tool arguments are incomplete. Denova blocked execution with no side effects.`, decision.ToolName, decision.ArgsBytes, finishReason, target)
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
		return fmt.Sprintf("[tool error] 工具 %q 的参数被内容过滤中断，未执行：%v / Tool %q arguments were interrupted by content filtering and were not executed: %v", decision.ToolName, err, decision.ToolName, err)
	}
	return fmt.Sprintf("[tool error] 工具 %q 的参数不是完整 JSON 对象：%v。请修正 arguments 后重试。 / Tool %q arguments are not a complete JSON object: %v. Fix the arguments and retry. (bytes=%d)", decision.ToolName, err, decision.ToolName, err, len(args))
}

func isContentFilterInterruptedArguments(err error, decision ToolDecision, outcome LLMOutcome) bool {
	return isIncompleteJSONArgumentsError(err) &&
		strings.EqualFold(strings.TrimSpace(outcome.FinishReason), "content_filter") &&
		(decision.MutatesWorkspace || decision.Source == ToolSourceWrite)
}

func isIncompleteJSONArgumentsError(err error) bool {
	return errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) ||
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
