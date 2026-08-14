package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/cloudwego/eino-ext/libs/acl/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"denova/internal/interactive"
)

const (
	interactiveTurnSubmissionToolName    = "submit_interactive_turn"
	legacyActorStatePatchesToolName      = "submit_actor_state_patches"
	legacyInteractiveChoicesToolName     = "submit_choices"
	interactiveCompletionRetryCode       = "interactive_turn_result_missing"
	interactiveCompletionGuardMaxRetries = 2
	// interactiveRetryCompletionBudget caps total completion tokens (visible
	// output + reasoning) during the retry phase. Large enough to fit a
	// complex opening's full state_changes submission, yet far below the
	// unbounded default that let reasoning balloon to tens of thousands of
	// tokens per retry.
	interactiveRetryCompletionBudget = 8192
	interactiveRetryDraftMaxBytes    = 16 * 1024
	interactiveRetryFeedbackMaxBytes = 1024
	interactiveRetryCandidatePrefix  = "[Retained narrative candidate; source=first accepted model prose;"
	interactiveRetryFeedbackPrefix   = "[Interactive turn protocol feedback; source=backend completion guard]"
)

// ErrInteractiveCompletionRetriesExceeded is returned when the completion
// guard drives more than interactiveCompletionGuardMaxRetries retries in a
// single turn without a complete TurnResult. The retained narrative candidate
// is preserved; the run fails fast instead of silently burning the shared
// model-retry budget on full reasoning passes.
var ErrInteractiveCompletionRetriesExceeded = errors.New("interactive turn completion retries exceeded")

type interactiveTurnProtocolStateKey struct{}
type interactiveTurnCancelKey struct{}

type interactiveTurnProtocolRunState struct {
	narrativeCandidateReady atomic.Bool
	guardRetries            atomic.Int32
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
	ready              func() bool
	narrativeMaxTokens int
}

func newInteractiveTurnProtocolMiddleware(ready func() bool, narrativeMaxTokens ...int) *interactiveTurnProtocolMiddleware {
	middleware := &interactiveTurnProtocolMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		ready:                        ready,
	}
	if len(narrativeMaxTokens) > 0 && narrativeMaxTokens[0] > 0 {
		middleware.narrativeMaxTokens = narrativeMaxTokens[0]
	}
	return middleware
}

func (m *interactiveTurnProtocolMiddleware) BeforeAgent(ctx context.Context, runCtx *adk.ChatModelAgentContext) (context.Context, *adk.ChatModelAgentContext, error) {
	return context.WithValue(ctx, interactiveTurnProtocolStateKey{}, &interactiveTurnProtocolRunState{}), runCtx, nil
}

func (m *interactiveTurnProtocolMiddleware) WrapModel(_ context.Context, wrapped model.BaseChatModel, _ *adk.ModelContext) (model.BaseChatModel, error) {
	if m != nil && m.narrativeMaxTokens > 0 {
		wrapped = &interactiveNarrativeBudgetModel{BaseChatModel: wrapped, maxTokens: m.narrativeMaxTokens}
	}
	if m == nil || m.ready == nil || !m.ready() {
		return wrapped, nil
	}
	return &interactiveNarrativeOnlyModel{BaseChatModel: wrapped}, nil
}

// interactiveNarrativeBudgetModel applies the story-derived completion reserve
// only while producing the first visible narrative. The retry phase is capped
// separately (see interactiveNarrativeBudgetOptions) so a bounded, targeted
// submission replaces an unbounded full reasoning pass.
type interactiveNarrativeBudgetModel struct {
	model.BaseChatModel
	maxTokens int
}

func (m *interactiveNarrativeBudgetModel) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return m.BaseChatModel.Generate(ctx, messages, interactiveNarrativeBudgetOptions(ctx, m.maxTokens, opts)...)
}

func (m *interactiveNarrativeBudgetModel) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return m.BaseChatModel.Stream(ctx, messages, interactiveNarrativeBudgetOptions(ctx, m.maxTokens, opts)...)
}

func interactiveNarrativeBudgetOptions(ctx context.Context, maxTokens int, opts []model.Option) []model.Option {
	state := interactiveTurnProtocolState(ctx)
	// Retry phase: the model only needs to emit a small structured
	// submit_interactive_turn payload. Cap total completion tokens — on
	// Minimax-M3 thinking counts toward this limit, so it is the one
	// provider-side lever that actually constrains reasoning. Also drop
	// reasoning effort (honored by OpenAI/Gemini; ignored but harmless on
	// Minimax) so a retry stops re-generating a full reasoning pass.
	if state != nil && state.narrativeCandidateReady.Load() {
		bounded := append([]model.Option(nil), opts...)
		bounded = append(bounded, openai.WithMaxCompletionTokens(interactiveRetryCompletionBudget))
		bounded = append(bounded, openai.WithReasoningEffort(openai.ReasoningEffortLevelLow))
		return bounded
	}
	if maxTokens <= 0 {
		return opts
	}
	common := model.GetCommonOptions(&model.Options{}, opts...)
	if common.MaxTokens != nil && *common.MaxTokens <= maxTokens {
		return opts
	}
	bounded := append([]model.Option(nil), opts...)
	return append(bounded, model.WithMaxTokens(maxTokens))
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

		// Circuit breaker: cap guard-driven retries per turn. Without this the
		// guard silently consumes the shared MaxRetries budget, re-generating a
		// full reasoning pass on every attempt. Fail fast (and surface the
		// retained narrative candidate) instead.
		if state != nil && state.guardRetries.Add(1) > interactiveCompletionGuardMaxRetries {
			return &adk.RetryDecision{
				Retry:        false,
				RewriteError: ErrInteractiveCompletionRetriesExceeded,
			}
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
		feedbackLines := []string{
			interactiveRetryFeedbackPrefix,
			"你刚才尝试直接结束本回合，但 state_changes 与 choices 尚未全部成功提交。",
			"首个正文候选已经锁定并展示。现在只调用 submit_interactive_turn，并只提供 retry_modules 指定的字段；已 accepted 的模块不要重交，ready=true 后不要重复输出或改写正文。",
		}
		if receipt, ok := lastInteractiveSubmitReceipt(retryCtx.InputMessages); ok {
			if detail := interactiveRetryFeedbackFromReceipt(receipt); detail != "" {
				feedbackLines = append(feedbackLines, detail)
			}
		}
		feedbackLines = append(feedbackLines, "Do not finish this turn before both submission modules are accepted.")
		feedback, _ := truncateUTF8Bytes(strings.Join(feedbackLines, "\n"), interactiveRetryFeedbackMaxBytes)
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
		if !IsInteractiveTurnSubmissionTool(call.Function.Name) {
			return false
		}
	}
	return true
}

// lastInteractiveSubmitReceipt scans the conversation history (most-recent
// first) for the result of the most recent submit_interactive_turn call and
// decodes its receipt. It returns false when no prior submission exists, so
// the completion guard can fall back to generic feedback. The guard only runs
// when the model emitted prose without a finishing submission, but earlier
// ReAct iterations may still have produced a rejected receipt whose
// diagnostics tell the model exactly what to fix.
func lastInteractiveSubmitReceipt(messages []*schema.Message) (interactive.TurnSubmissionReceipt, bool) {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg == nil || msg.Role != schema.Assistant || len(msg.ToolCalls) == 0 {
			continue
		}
		var submitID string
		for _, call := range msg.ToolCalls {
			if IsInteractiveTurnSubmissionTool(call.Function.Name) {
				submitID = call.ID
				break
			}
		}
		if submitID == "" {
			continue
		}
		for j := i + 1; j < len(messages); j++ {
			result := messages[j]
			if result == nil || result.Role != schema.Tool || result.ToolCallID != submitID {
				continue
			}
			var receipt interactive.TurnSubmissionReceipt
			if err := json.Unmarshal([]byte(result.Content), &receipt); err != nil {
				return interactive.TurnSubmissionReceipt{}, false
			}
			return receipt, true
		}
	}
	return interactive.TurnSubmissionReceipt{}, false
}

// interactiveRetryFeedbackFromReceipt turns a prior submission receipt into a
// concise, field-specific correction hint. Generic "both modules missing"
// feedback forces the model to re-guess; naming the failing module, path and
// expected/actual type lets it self-correct in one retry instead of looping.
func interactiveRetryFeedbackFromReceipt(receipt interactive.TurnSubmissionReceipt) string {
	var lines []string
	pendingModules := uniqueNonEmptyStrings(append(append([]string{}, receipt.RetryModules...), receipt.MissingModules...))
	if len(pendingModules) > 0 {
		lines = append(lines, "仍需补交的模块："+strings.Join(pendingModules, "、"))
	}
	for _, d := range receipt.Diagnostics {
		header := strings.TrimSpace(strings.Join([]string{d.Module, d.Code}, ":"))
		message := strings.TrimSpace(d.MessageZH)
		if message == "" {
			message = strings.TrimSpace(d.MessageEN)
		}
		if d.Expected != "" || d.Actual != "" {
			message = strings.TrimSpace(message + "（期望 " + d.Expected + " / 实际 " + d.Actual + "）")
		}
		if d.Path != "" {
			message = strings.TrimSpace(d.Path + " " + message)
		}
		if message == "" {
			continue
		}
		if header == "" {
			lines = append(lines, message)
		} else {
			lines = append(lines, header+"："+message)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return "上一次 submit_interactive_turn 的具体回执（按此定向修正，勿整段重写）：\n" + strings.Join(lines, "\n")
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
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
