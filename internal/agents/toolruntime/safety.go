package toolruntime

import (
	"errors"
	"fmt"
	"io"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/run"
	"denova/internal/agents/tool"
)

// applyModelOutputToolSafety rejects calls from an incomplete model response.
// The core scheduler still creates exactly one paired synthetic result.
func applyModelOutputToolSafety(decision agenttool.Decision, outcome agentrun.LLMOutcome) agenttool.Decision {
	reason := strings.TrimSpace(outcome.FinishReason)
	class := agent.ClassifyModelFinishReason(reason)
	if !class.Incomplete() {
		return decision
	}
	argsComplete := false
	decision.ArgsComplete = &argsComplete
	decision.ModelFinishReason = reason
	if decision.Action == "blocked" {
		return decision
	}
	decision.Action = "blocked"
	decision.Reason = incompleteModelToolArgumentsMessage(decision, reason, class)
	return decision
}

func incompleteModelToolArgumentsMessage(
	decision agenttool.Decision,
	finishReason string,
	class agent.ModelFinishReasonClass,
) string {
	target := strings.TrimSpace(decision.Target)
	if target == "" {
		target = "(unknown)"
	}
	reason := "model_output_incomplete"
	retryable := true
	explanation := "The provider returned an incomplete model response, so the tool arguments may be incomplete. Denova blocked execution with no side effects; retry the model step."
	switch class {
	case agent.ModelFinishReasonOutputLimit:
		reason = "model_output_token_limit"
		explanation = "The model reached its output-token limit, so the tool arguments are incomplete. Denova blocked execution with no side effects; retry with shorter arguments or split the task."
	case agent.ModelFinishReasonContextLimit:
		reason = "model_context_window_exceeded"
		explanation = "The model reached its context-window limit, so the tool arguments may be incomplete. Denova blocked execution with no side effects; reduce the request or output limit before retrying."
	case agent.ModelFinishReasonContentFilter:
		reason = "model_output_interrupted_by_content_filter"
		retryable = false
		explanation = "Content filtering interrupted the model response, so the tool arguments are incomplete. Denova blocked execution with no side effects."
	}
	return fmt.Sprintf(`[tool error]
type: incomplete_tool_arguments
tool: %s
reason: %s
retryable: %t
workspace_mutated: false
args_complete: false
args_bytes: %d
model_finish_reason: %s
target: %s

%s`, decision.ToolName, reason, retryable, decision.ArgsBytes, finishReason, target, explanation)
}

func applyToolArgumentValidation(decision agenttool.Decision, args string, outcome agentrun.LLMOutcome) agenttool.Decision {
	if decision.Action == "blocked" {
		return decision
	}
	if err := agentcontext.ValidateToolArgumentsJSON(args); err != nil {
		argsComplete := false
		decision.ArgsComplete = &argsComplete
		decision.ModelFinishReason = strings.TrimSpace(outcome.FinishReason)
		decision.Action = "blocked"
		decision.Reason = invalidToolArgumentsMessage(decision, args, err, outcome)
	}
	return decision
}

func invalidToolArgumentsMessage(decision agenttool.Decision, args string, err error, outcome agentrun.LLMOutcome) string {
	if isContentFilterInterruptedArguments(err, decision, outcome) {
		return fmt.Sprintf("[tool error] Tool %q arguments were interrupted by content filtering and were not executed: %v", decision.ToolName, err)
	}
	return fmt.Sprintf("[tool error] Tool %q arguments are not a complete JSON object: %v. Fix the arguments and retry. (bytes=%d)", decision.ToolName, err, len(args))
}

func isContentFilterInterruptedArguments(err error, decision agenttool.Decision, outcome agentrun.LLMOutcome) bool {
	return isIncompleteJSONArgumentsError(err) &&
		strings.EqualFold(strings.TrimSpace(outcome.FinishReason), "content_filter") &&
		(decision.MutationScope != agenttool.ToolMutationNone || decision.Source == agenttool.ToolSourceWrite)
}

func isIncompleteJSONArgumentsError(err error) bool {
	return errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) ||
		strings.Contains(strings.ToLower(err.Error()), "unexpected eof")
}
