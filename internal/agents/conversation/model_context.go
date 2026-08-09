package conversation

import (
	"context"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/toolresult"
	"errors"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	agentcontext "denova/internal/agents/context"
	agentcompaction "denova/internal/agents/context/compaction"
	"denova/internal/agents/session"
)

var ErrMissingAgentCycleIdentity = errors.New("session canonical write requires durable agent cycle identity")

func (c *SessionConversation) ModelContextBudget() agentcontext.Budget {
	if c == nil {
		return agentcontext.DefaultBudget()
	}
	return agentcontext.ContextBudgetForAgent(c.cfg, c.agentKind)
}

func (c *SessionConversation) AssembleModelContext(ctx context.Context, _ string, input agentcontext.ModelContextInput) (agentcontext.ModelContextResult, error) {
	if c == nil || c.session == nil {
		return agentcontext.ModelContextResult{}, fmt.Errorf("会话不存在")
	}
	if err := ctx.Err(); err != nil {
		return agentcontext.ModelContextResult{}, err
	}
	if err := c.session.RefreshCanonical(ctx); err != nil {
		return agentcontext.ModelContextResult{}, err
	}
	intent, err := c.acceptedInputDomainCommitIntent(input.UserMessage, input.UserReferences)
	durableInput := err == nil
	if err != nil && !errors.Is(err, ErrMissingAgentCycleIdentity) {
		return agentcontext.ModelContextResult{}, err
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
			agent.User,
			intent.Hash,
		)
		if err != nil {
			return agentcontext.ModelContextResult{}, err
		}
	} else {
		// Context analysis deliberately has no durable cycle identity. It may
		// reuse the exact pure assembly path, but CommitModelInput below remains
		// fail-closed so production cannot cross the provider boundary this way.
		snapshot, err = c.session.SnapshotContext(c.agentKind)
		if err != nil {
			return agentcontext.ModelContextResult{}, fmt.Errorf("snapshot session model context: %w", err)
		}
	}
	fragments := make([]agentcontext.Fragment, 0, len(input.Fragments)+2)
	fragments = append(fragments, input.Fragments...)
	fragments = append(fragments, c.runtimeContextFragments()...)
	canonicalMessages := c.modelMessagesWithAcceptedInput(snapshot, inputIndex, materialized, input.UserMessage)
	assembled, err := agentcontext.NewAssembler(input.Budget).Assemble(ctx, agentcontext.AssembleRequest{
		Messages:  canonicalMessages,
		Fragments: fragments,
	})
	if err != nil {
		return agentcontext.ModelContextResult{}, err
	}
	return agentcontext.ModelContextResult{
		Messages: assembled.Messages, Context: assembled,
		CommitState: sessionModelContextCommitState{
			cursor: snapshot.Cursor, input: intent, durable: durableInput,
			canonicalMessages: agentcontext.CloneMessages(canonicalMessages),
			effectiveMessages: agentcontext.CloneMessages(assembled.Messages),
		},
	}, nil
}

type sessionModelContextCommitState struct {
	cursor            session.ContextCursor
	input             session.DomainCommitIntent
	durable           bool
	canonicalMessages []*agent.Message
	effectiveMessages []*agent.Message
}

func (c *SessionConversation) CommitModelInput(ctx context.Context, _ string, assembled agentcontext.ModelContextResult) error {
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
	c.pendingCommits[agentrun.DomainCommitInput] = &intent
	c.cycleCursor = state.cursor
	commitInput := c.inputCommit
	c.cycleMu.Unlock()
	var appendErr error
	if commitInput != nil {
		appendErr = commitInput()
	} else {
		appendErr = c.CommitAgentCycleStage(context.Background(), agentrun.DomainCommitInput, agentrun.Outcome{Status: agentrun.OutcomeCompleted})
	}
	if appendErr != nil {
		return appendErr
	}
	c.rememberCompactionModelBase(state.canonicalMessages, state.effectiveMessages)
	c.rememberContextAssembly(assembled.Context)
	return nil
}

func (c *SessionConversation) acceptedInputDomainCommitIntent(message string, references []agentcontext.UserReference) (session.DomainCommitIntent, error) {
	identity := c.agentCycleIdentitySnapshot()
	if !agentrun.ValidCycleIdentity(identity) {
		return session.DomainCommitIntent{}, ErrMissingAgentCycleIdentity
	}
	userReferences := make([]agentcontext.UserReference, len(references))
	for index, reference := range references {
		userReferences[index] = agentcontext.UserReference{
			Kind: reference.Kind, ID: reference.ID, Label: reference.Label, Detail: reference.Detail,
			StartLine: reference.StartLine, EndLine: reference.EndLine,
		}
	}
	return session.NewDomainCommitIntent(session.DomainCommitIdentity{
		CommandID: string(identity.CommandID), OperationID: string(identity.OperationID), Cycle: identity.Cycle,
	}, agent.UserMessage(message), session.MessageMetadata{AgentKind: c.agentKind, UserReferences: userReferences})
}

func (c *SessionConversation) modelMessagesWithAcceptedInput(
	snapshot session.ContextSnapshot,
	inputIndex int,
	materialized bool,
	agentMessage string,
) []*agent.Message {
	if materialized {
		snapshot.EffectiveMessages = append([]*agent.Message(nil), snapshot.EffectiveMessages...)
		snapshot.EffectiveMessages[inputIndex] = agent.UserMessage(agentMessage)
		return c.modelHistory(snapshot)
	}
	history := c.modelHistory(snapshot)
	return append(history, agent.UserMessage(agentMessage))
}

func (c *SessionConversation) modelHistory(snapshot session.ContextSnapshot) []*agent.Message {
	history := append([]*agent.Message(nil), snapshot.EffectiveMessages...)
	policy := c.compactionPolicy()
	effectiveStart := snapshot.Cursor.MessageCount - len(history)
	if snapshot.ToolResultCleanup != nil {
		history = applyToolResultCleanupProjection(history, effectiveStart, *snapshot.ToolResultCleanup)
	}
	if snapshot.Compaction != nil && strings.TrimSpace(snapshot.Compaction.Summary) != "" {
		compaction := *snapshot.Compaction
		retainedTurns := compaction.RetainedTurns
		if retainedTurns <= 0 {
			retainedTurns = policy.RetainedTurns
		}
		tail := agentcompaction.TailAfterSource(history, effectiveStart, compaction.SourceEndIndex, retainedTurns)
		history = make([]*agent.Message, 0, 1+len(tail))
		history = append(history, agentcontext.NewCompactionSummaryMessage(compaction.Epoch, compaction.Summary))
		history = append(history, tail...)
	}
	return toolresult.ApplyContextPolicy(history, c.ToolResultContextPolicy())
}

func (c *SessionConversation) leadingRuntimeMessages() []*agent.Message {
	if c == nil || strings.TrimSpace(c.stableContext) == "" {
		return nil
	}
	content := agentcontext.StandaloneMessage(c.stableContextTitle, c.stableContext, "")
	if strings.TrimSpace(content) == "" {
		return nil
	}
	return []*agent.Message{agent.UserMessage(content)}
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
	c.lastContextSummary = agentcontext.SourceSummary(sources, agentcontext.DefaultAuditPreviewChars)
	c.cycleMu.Unlock()
}
