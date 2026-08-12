package continuallearning

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
	agentstate "github.com/alfredxw/denova/agent/state"
)

const (
	optimizerValidationFeedbackPrefix = "[Harness State validation feedback / Harness State 校验反馈]"
	optimizerValidationRetryCode      = "harness_state_draft_invalid"
)

type optimizerValidationRetryReason struct {
	Code string `json:"code"`
}

// newOptimizerCompletionGuard validates the complete draft only when the
// model tries to finish. Ordinary tool calls remain untouched, while an
// invalid final draft is returned to the same bounded model loop for repair.
func newOptimizerCompletionGuard(validate func(context.Context) error) func(context.Context, *agent.RetryContext) *agent.RetryDecision {
	return func(ctx context.Context, retryCtx *agent.RetryContext) *agent.RetryDecision {
		if validate == nil || retryCtx == nil || retryCtx.Err != nil || retryCtx.OutputMessage == nil || len(retryCtx.OutputMessage.ToolCalls) > 0 {
			return nil
		}
		if err := validate(ctx); err != nil {
			if ctx != nil && ctx.Err() != nil {
				return nil
			}
			messages := optimizerRetryBaseMessages(retryCtx.Messages)
			messages = append(messages, agent.UserMessage(optimizerValidationFeedback(err)))
			return &agent.RetryDecision{
				Retry:        true,
				Messages:     messages,
				RejectReason: optimizerValidationRetryReason{Code: optimizerValidationRetryCode},
			}
		}
		return nil
	}
}

func optimizerRetryBaseMessages(messages []*agent.Message) []*agent.Message {
	result := make([]*agent.Message, 0, len(messages))
	for _, message := range messages {
		if message == nil || (message.Role == agent.User && strings.HasPrefix(message.Content, optimizerValidationFeedbackPrefix)) {
			continue
		}
		result = append(result, message.Clone())
	}
	return result
}

func optimizerValidationFeedback(err error) string {
	var validation *agentstate.ValidationError
	if errors.As(err, &validation) {
		encoded, marshalErr := json.MarshalIndent(validation.Diagnostics, "", "  ")
		if marshalErr == nil {
			return fmt.Sprintf(`%s
The isolated draft is not publishable. Repair every diagnostic with the ordinary file tools, validate the complete State mentally, and only then finish. Do not discard otherwise useful edits.
当前隔离草稿无法发布。请使用普通文件工具修复下面的全部问题，确认完整 State 有效后再结束；不要丢弃其他有效修改。

diagnostics:
%s`, optimizerValidationFeedbackPrefix, encoded)
		}
	}
	return fmt.Sprintf(`%s
The isolated draft is not publishable. Inspect and repair the draft with ordinary file tools before finishing.
当前隔离草稿无法发布。请先使用普通文件工具检查并修复草稿，再结束本轮。

error: %s`, optimizerValidationFeedbackPrefix, err)
}
