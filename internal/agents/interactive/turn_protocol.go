package interactive

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	agent "github.com/alfredxw/denova/agent"

	producttools "denova/internal/agents/tools"
)

const (
	interactiveTurnSubmissionToolName = producttools.SubmitInteractiveTurnToolName
	legacyActorStatePatchesToolName   = "submit_actor_state_patches"
	legacyInteractiveChoicesToolName  = "submit_choices"
	interactiveCompletionRetryCode    = "interactive_turn_result_missing"
	interactiveRetryDraftMaxBytes     = 16 * 1024
	interactiveRetryFeedbackMaxBytes  = 1024
	interactiveRetryCandidatePrefix   = "[Retained narrative candidate; source=first accepted model prose;"
	interactiveRetryFeedbackPrefix    = "[Interactive turn protocol feedback; source=backend completion guard]"
)

const (
	CompletionRetryCode    = interactiveCompletionRetryCode
	TurnSubmissionToolName = interactiveTurnSubmissionToolName
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

func WithTurnCancel(ctx context.Context, cancel agent.AgentCancelFunc) context.Context {
	return context.WithValue(ctx, interactiveTurnCancelKey{}, cancel)
}

func RequestTurnCompletion(ctx context.Context) bool {
	state := interactiveTurnProtocolState(ctx)
	if state == nil || !state.narrativeCandidateReady.Load() {
		return false
	}
	cancel, _ := ctx.Value(interactiveTurnCancelKey{}).(agent.AgentCancelFunc)
	if cancel == nil {
		return false
	}
	_, contributed := cancel(agent.WithAgentCancelMode(agent.CancelAfterToolCalls))
	return contributed
}

type CompletionRetryReason struct {
	Code string `json:"code"`
}

// TurnProtocolMiddleware keeps the tool schema stable for prompt
// caching and provides a narrative-only fallback when a model submits before
// producing a prose candidate.
type TurnProtocolMiddleware struct {
	*agent.BaseMiddleware
	ready func() bool
}

func NewTurnProtocolMiddleware(ready func() bool) *TurnProtocolMiddleware {
	return &TurnProtocolMiddleware{
		BaseMiddleware: &agent.BaseMiddleware{},
		ready:          ready,
	}
}

func (m *TurnProtocolMiddleware) BeforeAgent(ctx context.Context, runCtx *agent.RunContext) (context.Context, *agent.RunContext, error) {
	return context.WithValue(ctx, interactiveTurnProtocolStateKey{}, &interactiveTurnProtocolRunState{}), runCtx, nil
}

func (m *TurnProtocolMiddleware) WrapModel(_ context.Context, wrapped agent.BaseChatModel, _ *agent.ModelContext) (agent.BaseChatModel, error) {
	if m == nil || m.ready == nil || !m.ready() {
		return wrapped, nil
	}
	return &interactiveNarrativeOnlyModel{BaseChatModel: wrapped}, nil
}

func (m *TurnProtocolMiddleware) AfterModelRewriteState(ctx context.Context, state *agent.RunState, _ *agent.ModelContext) (context.Context, *agent.RunState, error) {
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
	agent.BaseChatModel
}

func (m *interactiveNarrativeOnlyModel) Generate(ctx context.Context, messages []*agent.Message, opts ...agent.ModelOption) (*agent.Message, error) {
	narrativeOpts := append([]agent.ModelOption(nil), opts...)
	narrativeOpts = append(narrativeOpts, agent.WithToolChoice(agent.ToolChoiceForbidden))
	return m.BaseChatModel.Generate(ctx, messages, narrativeOpts...)
}

func (m *interactiveNarrativeOnlyModel) Stream(ctx context.Context, messages []*agent.Message, opts ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	narrativeOpts := append([]agent.ModelOption(nil), opts...)
	narrativeOpts = append(narrativeOpts, agent.WithToolChoice(agent.ToolChoiceForbidden))
	return m.BaseChatModel.Stream(ctx, messages, narrativeOpts...)
}

// newInteractiveCompletionGuard retains a prose-only response as the visible
// candidate while the hidden TurnResult is still missing. The native loop retries with a
// bounded, ephemeral copy so the model can submit matching structured state.
func NewCompletionGuard(ready func() bool) func(context.Context, *agent.RetryContext) *agent.RetryDecision {
	return func(ctx context.Context, retryCtx *agent.RetryContext) *agent.RetryDecision {
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

		messages := interactiveRetryBaseMessages(retryCtx.Messages)
		candidate := ""
		if state != nil {
			candidate = state.retainedNarrativeCandidate()
		}
		if strings.TrimSpace(candidate) != "" {
			draft := truncateUTF8StringBytes(candidate, interactiveRetryDraftMaxBytes)
			messages = append(messages, agent.AssistantMessage(fmt.Sprintf(
				"%s limit=%d bytes]\n%s",
				interactiveRetryCandidatePrefix,
				interactiveRetryDraftMaxBytes,
				draft,
			), nil))
		}
		feedback := truncateUTF8StringBytes(strings.Join([]string{
			interactiveRetryFeedbackPrefix,
			"你刚才尝试直接结束本回合，但 state_changes 与 choices 尚未全部成功提交。",
			"首个正文候选已经锁定并展示。现在只调用 submit_interactive_turn，并只提供 retry_modules 指定的字段；已 accepted 的模块不要重交，ready=true 后不要重复输出或改写正文。",
			"Do not finish this turn before both submission modules are accepted.",
		}, "\n"), interactiveRetryFeedbackMaxBytes)
		messages = append(messages, agent.UserMessage(feedback))
		return &agent.RetryDecision{
			Retry:        true,
			Messages:     messages,
			RejectReason: CompletionRetryReason{Code: interactiveCompletionRetryCode},
		}
	}
}

func interactiveOutputContainsNarrativeCandidate(message *agent.Message) bool {
	if message == nil || strings.TrimSpace(message.Content) == "" {
		return false
	}
	for _, call := range message.ToolCalls {
		if !IsInteractiveTurnSubmissionTool(call.Function.Name) {
			return false
		}
	}
	return true
}

// IsInteractiveTurnSubmissionTool reports whether the tool finalizes the
// current interactive turn. Submission tool calls always come after the
// narrative prose, so they anchor the narrative position in display events.
func IsInteractiveTurnSubmissionTool(name string) bool {
	switch strings.TrimSpace(name) {
	case interactiveTurnSubmissionToolName, legacyActorStatePatchesToolName, legacyInteractiveChoicesToolName:
		return true
	default:
		return false
	}
}

func interactiveRetryBaseMessages(messages []*agent.Message) []*agent.Message {
	base := make([]*agent.Message, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		if message.Role == agent.Assistant && strings.HasPrefix(message.Content, interactiveRetryCandidatePrefix) {
			continue
		}
		if message.Role == agent.User && strings.HasPrefix(message.Content, interactiveRetryFeedbackPrefix) {
			continue
		}
		base = append(base, message)
	}
	return base
}

type interactiveRetryReasonCarrier interface {
	RejectReason() any
}

func CompletionRetryFromError(err error) (CompletionRetryReason, bool) {
	if err == nil {
		return CompletionRetryReason{}, false
	}
	var carrier interactiveRetryReasonCarrier
	if !errors.As(err, &carrier) {
		return CompletionRetryReason{}, false
	}
	switch reason := carrier.RejectReason().(type) {
	case CompletionRetryReason:
		return reason, reason.Code == interactiveCompletionRetryCode
	case *CompletionRetryReason:
		if reason != nil && reason.Code == interactiveCompletionRetryCode {
			return *reason, true
		}
	}
	return CompletionRetryReason{}, false
}

func truncateUTF8StringBytes(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	for maxBytes > 0 && (value[maxBytes]&0xC0) == 0x80 {
		maxBytes--
	}
	return value[:maxBytes]
}
