package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"

	agentcontext "denova/internal/agent/context"
	"denova/internal/session"
)

var ErrMissingAgentCycleIdentity = errors.New("session canonical write requires durable agent cycle identity")

func (c *SessionConversation) ModelContextBudget() agentcontext.Budget {
	if c == nil {
		return agentcontext.DefaultBudget()
	}
	return contextBudgetForAgent(c.cfg, c.agentKind)
}

func (c *SessionConversation) AssembleModelContext(ctx context.Context, _ string, input ModelContextInput) (ModelContextResult, error) {
	if c == nil || c.session == nil {
		return ModelContextResult{}, fmt.Errorf("会话不存在")
	}
	if err := ctx.Err(); err != nil {
		return ModelContextResult{}, err
	}
	if err := c.session.RefreshCanonical(ctx); err != nil {
		return ModelContextResult{}, err
	}
	intent, err := c.acceptedInputDomainCommitIntent(input.UserMessage)
	durableInput := err == nil
	if err != nil && !errors.Is(err, ErrMissingAgentCycleIdentity) {
		return ModelContextResult{}, err
	}
	var (
		snapshot     session.ContextSnapshot
		inputIndex   int
		materialized bool
	)
	if durableInput {
		snapshot, inputIndex, materialized, err = c.session.SnapshotContextForDomainCommit(
			c.agentKind,
			intent.Identity,
			schema.User,
			intent.Hash,
		)
		if err != nil {
			return ModelContextResult{}, err
		}
	} else {
		// Context analysis deliberately has no durable cycle identity. It may
		// reuse the exact pure assembly path, but CommitModelInput below remains
		// fail-closed so production cannot cross the provider boundary this way.
		snapshot = c.session.SnapshotContext(c.agentKind)
	}
	fragments := make([]agentcontext.Fragment, 0, len(input.Fragments)+2)
	fragments = append(fragments, input.Fragments...)
	fragments = append(fragments, c.runtimeContextFragments()...)
	assembled, err := agentcontext.NewAssembler(input.Budget).Assemble(ctx, agentcontext.AssembleRequest{
		Messages:  c.modelMessagesWithAcceptedInput(snapshot, inputIndex, materialized, input.UserMessage),
		Fragments: fragments,
	})
	if err != nil {
		return ModelContextResult{}, err
	}
	return ModelContextResult{
		Messages: assembled.Messages, Context: assembled,
		CommitState: sessionModelContextCommitState{cursor: snapshot.Cursor, input: intent, durable: durableInput},
	}, nil
}

type sessionModelContextCommitState struct {
	cursor  session.ContextCursor
	input   session.DomainCommitIntent
	durable bool
}

func (c *SessionConversation) CommitModelInput(ctx context.Context, _ string, assembled ModelContextResult) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("会话不存在")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	state, ok := assembled.CommitState.(sessionModelContextCommitState)
	if !ok {
		return fmt.Errorf("session model context is missing commit state")
	}
	if !state.durable {
		return ErrMissingAgentCycleIdentity
	}
	intent := state.input.WithExpectedContextCursor(state.cursor)
	c.cycleMu.Lock()
	c.ensureCycleCommitMapsLocked()
	c.pendingCommits[HarnessDomainCommitInput] = &intent
	c.cycleCursor = state.cursor
	commitInput := c.inputCommit
	c.cycleMu.Unlock()
	var appendErr error
	if commitInput != nil {
		appendErr = commitInput()
	} else {
		appendErr = c.CommitAgentCycleStage(context.Background(), HarnessDomainCommitInput, RunOutcome{Status: RunOutcomeCompleted})
	}
	if appendErr != nil {
		return appendErr
	}
	c.rememberContextAssembly(assembled.Context)
	return nil
}

func (c *SessionConversation) acceptedInputDomainCommitIntent(message string) (session.DomainCommitIntent, error) {
	c.cycleMu.Lock()
	references := append([]session.UserMessageReference(nil), c.userMessageReferences...)
	c.cycleMu.Unlock()
	identity := c.agentCycleIdentitySnapshot()
	if !validHarnessCycleIdentity(identity) {
		return session.DomainCommitIntent{}, ErrMissingAgentCycleIdentity
	}
	return session.NewDomainCommitIntent(session.DomainCommitIdentity{
		CommandID: string(identity.CommandID), OperationID: string(identity.OperationID), Cycle: identity.Cycle,
	}, schema.UserMessage(message), session.MessageMetadata{AgentKind: c.agentKind, UserReferences: references})
}

func (c *SessionConversation) modelMessagesWithAcceptedInput(
	snapshot session.ContextSnapshot,
	inputIndex int,
	materialized bool,
	agentMessage string,
) []*schema.Message {
	if materialized {
		snapshot.EffectiveMessages = append([]*schema.Message(nil), snapshot.EffectiveMessages...)
		snapshot.EffectiveMessages[inputIndex] = schema.UserMessage(agentMessage)
		return c.modelHistory(snapshot)
	}
	history := c.modelHistory(snapshot)
	return append(history, schema.UserMessage(agentMessage))
}

func (c *SessionConversation) modelHistory(snapshot session.ContextSnapshot) []*schema.Message {
	history := append([]*schema.Message(nil), snapshot.EffectiveMessages...)
	policy := c.compactionPolicy()
	if snapshot.Compaction != nil && strings.TrimSpace(snapshot.Compaction.Summary) != "" {
		compaction := *snapshot.Compaction
		total := snapshot.Cursor.MessageCount
		effectiveStart := total - len(history)
		retainedTurns := compaction.RetainedTurns
		if retainedTurns <= 0 {
			retainedTurns = policy.RetainedTurns
		}
		tail := compactedMessagesAfterSource(history, effectiveStart, compaction.SourceEndIndex, retainedTurns)
		history = make([]*schema.Message, 0, 1+len(tail))
		history = append(history, NewContextCompactionSummaryMessage(compaction.Epoch, compaction.Summary))
		history = append(history, tail...)
	}
	return applyToolResultContextPolicy(history, c.ToolResultContextPolicy())
}

func (c *SessionConversation) leadingRuntimeMessages() []*schema.Message {
	if c == nil || strings.TrimSpace(c.stableContext) == "" {
		return nil
	}
	content := agentcontext.StandaloneMessage(c.stableContextTitle, c.stableContext, "")
	if strings.TrimSpace(content) == "" {
		return nil
	}
	return []*schema.Message{schema.UserMessage(content)}
}

func (c *SessionConversation) runtimeContextFragments() []agentcontext.Fragment {
	if c == nil {
		return nil
	}
	fragments := make([]agentcontext.Fragment, 0, 2)
	if strings.TrimSpace(c.stableContext) != "" {
		title := strings.TrimSpace(c.stableContextTitle)
		if title == "" {
			title = "稳定上下文"
		}
		fragments = append(fragments, agentcontext.Fragment{
			ID: "workspace_runtime_stable", Source: "workspace.runtime.stable", Title: title,
			Purpose: "provide stable workspace state for the current agent turn",
			Content: c.stableContext, Placement: agentcontext.PlacementLeadingMessage, Included: true,
			Note: "source=workspace snapshot; placement=stable prefix",
		})
	}
	if strings.TrimSpace(c.dynamicContext) != "" {
		title := strings.TrimSpace(c.dynamicContextTitle)
		if title == "" {
			title = "本轮动态上下文"
		}
		fragments = append(fragments, agentcontext.Fragment{
			ID: "workspace_runtime_dynamic", Source: "workspace.runtime.dynamic", Title: title,
			Purpose: "provide turn-scoped workspace state for the current request",
			Content: c.dynamicContext, Placement: agentcontext.PlacementFinalUserPrefix, Included: true,
			Note: "source=workspace snapshot; placement=final user prefix",
		})
	}
	return fragments
}

func (c *SessionConversation) rememberContextAssembly(result agentcontext.Result) {
	if c == nil {
		return
	}
	sources := make([]agentcontext.Source, 0, len(result.Fragments))
	for _, fragment := range result.Fragments {
		sources = append(sources, agentcontext.Source{
			Source: fragment.Source, Title: fragment.Title, Purpose: fragment.Purpose,
			Content: fragment.Content, Placement: fragment.Placement, Limit: fragment.Limit,
			Included: fragment.Included, Truncated: fragment.Truncated, Note: fragment.Note,
		})
	}
	c.cycleMu.Lock()
	c.lastContextSummary = agentcontext.SourceSummary(sources, defaultContextLedgerPreviewChars)
	c.cycleMu.Unlock()
}
