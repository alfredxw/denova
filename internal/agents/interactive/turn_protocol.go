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

type interactiveTurnProtocolRunState struct {
	narrativeCandidateReady atomic.Bool
	mu                      sync.Mutex
	narrativeCandidate      string
	restored                bool
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

func RequestTurnCompletion(ctx context.Context) bool {
	state := interactiveTurnProtocolState(ctx)
	if state == nil || !state.narrativeCandidateReady.Load() {
		return false
	}
	return agent.RequestCompletionAfterTools(ctx)
}

// TurnProtocolMiddleware keeps the tool schema stable for prompt
// caching and provides a narrative-only fallback when a model submits before
// producing a prose candidate.
type TurnProtocolMiddleware struct {
	*agent.BaseMiddleware
	ready           func() bool
	loadNarrative   func(context.Context) (string, error)
	acceptNarrative func(context.Context, string) error
}

func NewTurnProtocolMiddleware(config InteractiveStoryToolContext) *TurnProtocolMiddleware {
	return &TurnProtocolMiddleware{
		BaseMiddleware: &agent.BaseMiddleware{},
		ready:          config.TurnResultReady, loadNarrative: config.LoadNarrativeCandidate, acceptNarrative: config.AcceptNarrativeCandidate,
	}
}

func (m *TurnProtocolMiddleware) BeforeAgent(ctx context.Context, runCtx *agent.RunContext) (context.Context, *agent.RunContext, error) {
	state := &interactiveTurnProtocolRunState{}
	if m.loadNarrative != nil {
		narrative, err := m.loadNarrative(ctx)
		if err != nil {
			return ctx, runCtx, err
		}
		state.retainNarrativeCandidate(narrative)
		state.restored = narrative != ""
	}
	return context.WithValue(ctx, interactiveTurnProtocolStateKey{}, state), runCtx, nil
}

func (m *TurnProtocolMiddleware) BeforeModelRewriteState(ctx context.Context, state *agent.RunState, model *agent.ModelContext) (context.Context, *agent.RunState, error) {
	progress := interactiveTurnProtocolState(ctx)
	if progress == nil || !progress.restored || model.Iteration != 0 || (m.ready != nil && m.ready()) {
		return ctx, state, nil
	}
	// The canonical draft stays complete in Story storage. Only this explicitly
	// attributed request-local reminder uses the bounded feedback projection.
	for _, fragment := range interactiveProtocolFeedback(progress.retainedNarrativeCandidate()) {
		state.Messages = append(state.Messages, &agent.Message{Role: fragment.Role, Content: fragment.Content})
	}
	return ctx, state, nil
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
		return ctx, state, errors.New("tools are forbidden after TurnResult acceptance")
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

// ReviewModelOutput retains complete prose and asks for missing modules using
// bounded feedback. Network failures never enter this product acceptance seam.
func (m *TurnProtocolMiddleware) ReviewModelOutput(ctx context.Context, output agent.ModelOutput) (agent.ModelOutputReview, error) {
	state := interactiveTurnProtocolState(ctx)
	if interactiveOutputContainsNarrativeCandidate(output.Message) && state != nil {
		if state.retainedNarrativeCandidate() == "" && m.acceptNarrative != nil {
			if err := m.acceptNarrative(ctx, output.Message.Content); err != nil {
				return agent.ModelOutputReview{}, err
			}
		}
		state.retainNarrativeCandidate(output.Message.Content)
	}
	if m.ready == nil || m.ready() {
		return agent.ModelOutputReview{Action: agent.ModelOutputAccept}, nil
	}
	if output.Message != nil && len(output.Message.ToolCalls) > 0 {
		return agent.ModelOutputReview{Action: agent.ModelOutputAccept}, nil
	}

	candidate := ""
	if state != nil {
		candidate = state.retainedNarrativeCandidate()
	}
	return agent.ModelOutputReview{Action: agent.ModelOutputRepair, Feedback: interactiveProtocolFeedback(candidate), Reason: interactiveCompletionRetryCode}, nil
}

func interactiveProtocolFeedback(candidate string) []agent.ContextFragment {
	var fragments []agent.ContextFragment
	if strings.TrimSpace(candidate) != "" {
		draft := truncateUTF8StringBytes(candidate, interactiveRetryDraftMaxBytes)
		fragments = append(fragments, agent.ContextFragment{
			Source: "interactive_turn_protocol", Purpose: "retained_narrative", Role: agent.Assistant,
			HardLimit: interactiveRetryDraftMaxBytes,
			Content: truncateUTF8StringBytes(fmt.Sprintf(
				"%s limit=%d bytes]\n%s",
				interactiveRetryCandidatePrefix,
				interactiveRetryDraftMaxBytes,
				draft,
			), interactiveRetryDraftMaxBytes)})
	}
	feedback := truncateUTF8StringBytes(strings.Join([]string{
		interactiveRetryFeedbackPrefix,
		"You attempted to finish the turn before both state_changes and choices were accepted.",
		"The first prose candidate is locked and already displayed. Call only submit_interactive_turn now, providing only fields named by retry_modules. Do not resubmit accepted modules, and do not repeat or rewrite prose after ready=true.",
		"Do not finish this turn before both submission modules are accepted.",
	}, "\n"), interactiveRetryFeedbackMaxBytes)
	fragments = append(fragments, agent.ContextFragment{
		Source: "interactive_turn_protocol", Purpose: "missing_modules", Role: agent.User,
		Content: feedback, HardLimit: interactiveRetryFeedbackMaxBytes,
	})
	return fragments
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

func truncateUTF8StringBytes(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	for maxBytes > 0 && (value[maxBytes]&0xC0) == 0x80 {
		maxBytes--
	}
	return value[:maxBytes]
}
