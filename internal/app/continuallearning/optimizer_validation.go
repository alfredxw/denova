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
	optimizerValidationFeedbackPrefix = "[Harness State Validation Feedback]"
	optimizerValidationRetryCode      = "harness_state_invalid"
)

type optimizerValidationRetryReason struct {
	Code string `json:"code"`
}

// newOptimizerCompletionGuard validates the complete live directory only when
// the model tries to finish. Ordinary tool calls remain untouched, while an
// invalid final State is returned to the same bounded model loop for repair.
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
The live Harness State directory is invalid. Repair every diagnostic with the ordinary file tools, validate the complete State mentally, and only then finish. Every edit is already effective.
The current Harness State directory is invalid. Use ordinary file tools to fix every issue below, verify that the complete State is valid, and only then end the turn. Every change takes effect immediately.

diagnostics:
%s`, optimizerValidationFeedbackPrefix, encoded)
		}
	}
	return fmt.Sprintf(`%s
The live Harness State directory is invalid. Inspect and repair it with ordinary file tools before finishing.
The current Harness State directory is invalid. Inspect and repair it with ordinary file tools before ending the turn.

error: %s`, optimizerValidationFeedbackPrefix, err)
}
