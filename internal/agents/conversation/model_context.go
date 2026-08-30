package conversation

import (
	"context"
	"errors"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	agentcontext "denova/internal/agents/context"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	"denova/internal/agents/toolresult"
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
	intent, err := c.acceptedInputDomainCommitIntent(input.UserMessage, input.Attachments, input.UserReferences)
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
		snapshot, err = c.session.SnapshotContext()
		if err != nil {
			return agentcontext.ModelContextResult{}, fmt.Errorf("snapshot session model context: %w", err)
		}
	}
	fragments := make([]agentcontext.Fragment, 0, len(input.Fragments)+1)
	fragments = append(fragments, input.Fragments...)
	fragments = append(fragments, c.runtimeContextFragments()...)
	canonicalMessages := c.modelMessagesWithAcceptedInput(snapshot, inputIndex, materialized, input.UserMessage, input.Attachments)
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
	c.rememberContextAssembly(assembled.Context)
	return nil
}

// MaterializeAgentCanonicalInput publishes the accepted user input before any
// model-context assembly. The public Agent stage hash and product payload hash
// form the exact idempotency boundary across process recovery.
func (c *SessionConversation) MaterializeAgentCanonicalInput(
	ctx context.Context,
	message string,
	attachments []agent.Attachment,
	references []agentcontext.UserReference,
	agentCanonicalHash string,
) (session.DomainCommitReceipt, error) {
	if c == nil || c.session == nil {
		return session.DomainCommitReceipt{}, fmt.Errorf("会话不存在")
	}
	if err := ctx.Err(); err != nil {
		return session.DomainCommitReceipt{}, err
	}
	intent, err := c.acceptedInputDomainCommitIntent(message, attachments, references)
	if err != nil {
		return session.DomainCommitReceipt{}, err
	}
	intent, err = intent.WithAgentCanonicalHash(agentCanonicalHash)
	if err != nil {
		return session.DomainCommitReceipt{}, err
	}
	receipt, err := c.session.CommitDomainMessageContext(ctx, intent)
	if err != nil {
		return session.DomainCommitReceipt{}, err
	}
	c.cycleMu.Lock()
	c.cycleCursor = c.session.ContextCursor()
	c.cycleMu.Unlock()
	return receipt, nil
}

// ApplyAgentPreparedContext records the exact context projection used by the
// current public Agent cycle without appending the already-durable user input.
func (c *SessionConversation) ApplyAgentPreparedContext(assembled agentcontext.ModelContextResult) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("会话不存在")
	}
	state, ok := assembled.CommitState.(sessionModelContextCommitState)
	if !ok {
		return fmt.Errorf("session model context is missing commit state")
	}
	if !state.durable {
		return ErrMissingAgentCycleIdentity
	}
	c.cycleMu.Lock()
	c.cycleCursor = state.cursor
	c.cycleMu.Unlock()
	c.rememberContextAssembly(assembled.Context)
	return nil
}

func (c *SessionConversation) acceptedInputDomainCommitIntent(
	message string,
	attachments []agent.Attachment,
	references []agentcontext.UserReference,
) (session.DomainCommitIntent, error) {
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
	}, agent.UserMessageWithAttachments(message, attachments), session.MessageMetadata{
		AgentKind: c.agentKind, UserReferences: userReferences, DisplayContent: c.inputDisplayContent,
		ContextOnly: c.inputVisibility == agentrun.InputModelOnly,
	})
}

func (c *SessionConversation) modelMessagesWithAcceptedInput(
	snapshot session.ContextSnapshot,
	inputIndex int,
	materialized bool,
	agentMessage string,
	attachments []agent.Attachment,
) []*agent.Message {
	if materialized {
		snapshot.EffectiveMessages = append([]*agent.Message(nil), snapshot.EffectiveMessages...)
		acceptedInput := snapshot.EffectiveMessages[inputIndex].Clone()
		acceptedInput.Content = agentMessage
		snapshot.EffectiveMessages[inputIndex] = acceptedInput
		return c.modelHistory(snapshot)
	}
	history := c.modelHistory(snapshot)
	return append(history, agent.UserMessageWithAttachments(agentMessage, attachments))
}

func (c *SessionConversation) modelHistory(snapshot session.ContextSnapshot) []*agent.Message {
	history := append([]*agent.Message(nil), snapshot.EffectiveMessages...)
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
			title = "Stable Context"
		}
		fragments = append(fragments, agentcontext.Fragment{
			ID: "workspace_runtime_stable", Source: "workspace.runtime.stable", Title: title,
			Purpose: "provide cache-stable workspace sources for the current agent session",
			Content: c.stableContext, Placement: agentcontext.PlacementLeadingMessage, Included: true,
			Stability: agent.ContextStablePrefix,
			Note:      "source=workspace source projection; lifecycle=replaceable stable prefix; file bodies=on demand",
		})
	}
	if strings.TrimSpace(c.dynamicContext) != "" {
		title := strings.TrimSpace(c.dynamicContextTitle)
		if title == "" {
			title = "Current Turn Dynamic Context"
		}
		fragments = append(fragments, agentcontext.Fragment{
			ID: "workspace_runtime_dynamic", Source: "workspace.runtime.dynamic", Title: title,
			Purpose: "provide turn-scoped workspace sources for the current request",
			Content: c.dynamicContext, Placement: agentcontext.PlacementFinalUserPrefix, Included: true,
			Note: "source=workspace source projection; placement=final user prefix; file bodies=on demand",
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
