package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const (
	interactiveCompletionRetryCode   = "interactive_turn_result_missing"
	interactiveRetryDraftMaxBytes    = 2048
	interactiveRetryFeedbackMaxBytes = 1024
	interactiveRetryDraftPrefix      = "[Rejected narrative candidate; source=current model output;"
	interactiveRetryFeedbackPrefix   = "[Interactive turn protocol feedback; source=backend completion guard]"
)

type interactiveCompletionRetryReason struct {
	Code string `json:"code"`
}

// interactiveTurnProtocolMiddleware keeps the tool schema stable for prompt
// caching, then switches the accepted TurnResult phase to tool_choice=none.
type interactiveTurnProtocolMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	ready func() bool
}

func newInteractiveTurnProtocolMiddleware(ready func() bool) *interactiveTurnProtocolMiddleware {
	return &interactiveTurnProtocolMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		ready:                        ready,
	}
}

func (m *interactiveTurnProtocolMiddleware) WrapModel(_ context.Context, wrapped model.BaseChatModel, _ *adk.ModelContext) (model.BaseChatModel, error) {
	if m == nil || m.ready == nil || !m.ready() {
		return wrapped, nil
	}
	return &interactiveNarrativeOnlyModel{BaseChatModel: wrapped}, nil
}

func (m *interactiveTurnProtocolMiddleware) AfterModelRewriteState(ctx context.Context, state *adk.ChatModelAgentState, _ *adk.ModelContext) (context.Context, *adk.ChatModelAgentState, error) {
	if m == nil || m.ready == nil || !m.ready() || state == nil || len(state.Messages) == 0 {
		return ctx, state, nil
	}
	last := state.Messages[len(state.Messages)-1]
	if last != nil && len(last.ToolCalls) > 0 {
		return ctx, state, errors.New("TurnResult 已提交，禁止继续调用工具 / tools are forbidden after TurnResult acceptance")
	}
	return ctx, state, nil
}

type interactiveNarrativeOnlyModel struct {
	model.BaseChatModel
}

func (m *interactiveNarrativeOnlyModel) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	narrativeOpts := append([]model.Option(nil), opts...)
	narrativeOpts = append(narrativeOpts, model.WithToolChoice(schema.ToolChoiceForbidden))
	return m.BaseChatModel.Generate(ctx, messages, narrativeOpts...)
}

func (m *interactiveNarrativeOnlyModel) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	narrativeOpts := append([]model.Option(nil), opts...)
	narrativeOpts = append(narrativeOpts, model.WithToolChoice(schema.ToolChoiceForbidden))
	return m.BaseChatModel.Stream(ctx, messages, narrativeOpts...)
}

// newInteractiveCompletionGuard rejects a final model answer while the hidden
// TurnResult is still missing. Eino retries the same model call with bounded,
// ephemeral protocol feedback before any persistence is attempted.
func newInteractiveCompletionGuard(ready func() bool) func(context.Context, *adk.RetryContext) *adk.RetryDecision {
	return func(_ context.Context, retryCtx *adk.RetryContext) *adk.RetryDecision {
		if ready == nil || ready() || retryCtx == nil || retryCtx.Err != nil {
			return nil
		}
		if retryCtx.OutputMessage != nil && len(retryCtx.OutputMessage.ToolCalls) > 0 {
			return nil
		}

		messages := interactiveRetryBaseMessages(retryCtx.InputMessages)
		if retryCtx.OutputMessage != nil && strings.TrimSpace(retryCtx.OutputMessage.Content) != "" {
			draft, _ := truncateUTF8Bytes(retryCtx.OutputMessage.Content, interactiveRetryDraftMaxBytes)
			messages = append(messages, schema.AssistantMessage(fmt.Sprintf(
				"%s limit=%d bytes]\n%s",
				interactiveRetryDraftPrefix,
				interactiveRetryDraftMaxBytes,
				draft,
			), nil))
		}
		feedback, _ := truncateUTF8Bytes(strings.Join([]string{
			interactiveRetryFeedbackPrefix,
			"你刚才尝试直接结束本回合，但本回合尚未成功调用 submit_interactive_turn_result。",
			"请先调用该工具提交与正文候选一致的隐藏 TurnResult；若工具返回 accepted=false，请按 diagnostics 修正后重试；accepted=true（即使包含 warning）后再只输出故事正文。",
			"Do not finish this turn before submit_interactive_turn_result is accepted.",
		}, "\n"), interactiveRetryFeedbackMaxBytes)
		messages = append(messages, schema.UserMessage(feedback))
		return &adk.RetryDecision{
			Retry:                        true,
			ModifiedInputMessages:        messages,
			PersistModifiedInputMessages: false,
			RejectReason:                 interactiveCompletionRetryReason{Code: interactiveCompletionRetryCode},
		}
	}
}

func interactiveRetryBaseMessages(messages []*schema.Message) []*schema.Message {
	base := make([]*schema.Message, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		if message.Role == schema.Assistant && strings.HasPrefix(message.Content, interactiveRetryDraftPrefix) {
			continue
		}
		if message.Role == schema.User && strings.HasPrefix(message.Content, interactiveRetryFeedbackPrefix) {
			continue
		}
		base = append(base, message)
	}
	return base
}

type interactiveRetryReasonCarrier interface {
	RejectReason() any
}

func interactiveCompletionRetryFromError(err error) (interactiveCompletionRetryReason, bool) {
	if err == nil {
		return interactiveCompletionRetryReason{}, false
	}
	var carrier interactiveRetryReasonCarrier
	if !errors.As(err, &carrier) {
		return interactiveCompletionRetryReason{}, false
	}
	switch reason := carrier.RejectReason().(type) {
	case interactiveCompletionRetryReason:
		return reason, reason.Code == interactiveCompletionRetryCode
	case *interactiveCompletionRetryReason:
		if reason != nil && reason.Code == interactiveCompletionRetryCode {
			return *reason, true
		}
	}
	return interactiveCompletionRetryReason{}, false
}
