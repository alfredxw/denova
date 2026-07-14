package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const (
	interactiveActorStatePatchesToolName = "submit_actor_state_patches"
	interactiveChoicesToolName           = "submit_choices"
	interactiveCompletionRetryCode       = "interactive_turn_result_missing"
	interactiveRetryDraftMaxBytes        = 16 * 1024
	interactiveRetryFeedbackMaxBytes     = 1024
	interactiveRetryCandidatePrefix      = "[Retained narrative candidate; source=first accepted model prose;"
	interactiveRetryFeedbackPrefix       = "[Interactive turn protocol feedback; source=backend completion guard]"
)

type interactiveTurnProtocolStateKey struct{}
type interactiveTurnCancelKey struct{}

type interactiveTurnProtocolRunState struct {
	narrativeCandidateReady atomic.Bool
	mu                      sync.Mutex
	narrativeCandidate      string
}

func (s *interactiveTurnProtocolRunState) retainNarrativeCandidate(content string) string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.narrativeCandidate == "" && strings.TrimSpace(content) != "" {
		s.narrativeCandidate = content
		s.narrativeCandidateReady.Store(true)
	}
	return s.narrativeCandidate
}

func (s *interactiveTurnProtocolRunState) retainedNarrativeCandidate() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.narrativeCandidate
}

func interactiveTurnProtocolState(ctx context.Context) *interactiveTurnProtocolRunState {
	state, _ := ctx.Value(interactiveTurnProtocolStateKey{}).(*interactiveTurnProtocolRunState)
	return state
}

func withInteractiveTurnCancel(ctx context.Context, cancel adk.AgentCancelFunc) context.Context {
	return context.WithValue(ctx, interactiveTurnCancelKey{}, cancel)
}

func requestInteractiveTurnCompletion(ctx context.Context) bool {
	state := interactiveTurnProtocolState(ctx)
	if state == nil || !state.narrativeCandidateReady.Load() {
		return false
	}
	cancel, _ := ctx.Value(interactiveTurnCancelKey{}).(adk.AgentCancelFunc)
	if cancel == nil {
		return false
	}
	_, contributed := cancel(adk.WithAgentCancelMode(adk.CancelAfterToolCalls))
	return contributed
}

type interactiveCompletionRetryReason struct {
	Code string `json:"code"`
}

// interactiveTurnProtocolMiddleware keeps the tool schema stable for prompt
// caching and provides a narrative-only fallback when a model submits before
// producing a prose candidate.
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

func (m *interactiveTurnProtocolMiddleware) BeforeAgent(ctx context.Context, runCtx *adk.ChatModelAgentContext) (context.Context, *adk.ChatModelAgentContext, error) {
	return context.WithValue(ctx, interactiveTurnProtocolStateKey{}, &interactiveTurnProtocolRunState{}), runCtx, nil
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

// newInteractiveCompletionGuard retains a prose-only response as the visible
// candidate while the hidden TurnResult is still missing. Eino retries with a
// bounded, ephemeral copy so the model can submit matching structured state.
func newInteractiveCompletionGuard(ready func() bool) func(context.Context, *adk.RetryContext) *adk.RetryDecision {
	return func(ctx context.Context, retryCtx *adk.RetryContext) *adk.RetryDecision {
		if ready == nil || ready() || retryCtx == nil || retryCtx.Err != nil {
			return nil
		}
		state := interactiveTurnProtocolState(ctx)
		if interactiveOutputContainsNarrativeCandidate(retryCtx.OutputMessage) && state != nil {
			state.retainNarrativeCandidate(retryCtx.OutputMessage.Content)
		}
		if retryCtx.OutputMessage != nil && len(retryCtx.OutputMessage.ToolCalls) > 0 {
			return nil
		}

		messages := interactiveRetryBaseMessages(retryCtx.InputMessages)
		candidate := ""
		if state != nil {
			candidate = state.retainedNarrativeCandidate()
		}
		if strings.TrimSpace(candidate) != "" {
			draft, _ := truncateUTF8Bytes(candidate, interactiveRetryDraftMaxBytes)
			messages = append(messages, schema.AssistantMessage(fmt.Sprintf(
				"%s limit=%d bytes]\n%s",
				interactiveRetryCandidatePrefix,
				interactiveRetryDraftMaxBytes,
				draft,
			), nil))
		}
		feedback, _ := truncateUTF8Bytes(strings.Join([]string{
			interactiveRetryFeedbackPrefix,
			"你刚才尝试直接结束本回合，但 actor_state_patches 与 choices 尚未全部成功提交。",
			"首个正文候选已经锁定并展示。现在只调用 retry_modules 对应的 submit_actor_state_patches 或 submit_choices；已 accepted 的模块不要重交，ready=true 后不要重复输出或改写正文。",
			"Do not finish this turn before both submission modules are accepted.",
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

func interactiveOutputContainsNarrativeCandidate(message *schema.Message) bool {
	if message == nil || strings.TrimSpace(message.Content) == "" {
		return false
	}
	for _, call := range message.ToolCalls {
		if !isInteractiveTurnSubmissionTool(call.Function.Name) {
			return false
		}
	}
	return true
}

func isInteractiveTurnSubmissionTool(name string) bool {
	switch strings.TrimSpace(name) {
	case interactiveActorStatePatchesToolName, interactiveChoicesToolName:
		return true
	default:
		return false
	}
}

func interactiveRetryBaseMessages(messages []*schema.Message) []*schema.Message {
	base := make([]*schema.Message, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		if message.Role == schema.Assistant && strings.HasPrefix(message.Content, interactiveRetryCandidatePrefix) {
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
