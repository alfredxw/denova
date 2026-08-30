package conversation

import (
	"context"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/toolresult"
	"fmt"
	"strconv"
	"strings"
	"sync"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"

	"denova/config"
	"denova/internal/agents/session"
	novaskills "denova/internal/agents/skills"
)

type SessionConversation struct {
	session             *session.Session
	cfg                 *config.Config
	agentKind           string
	stableContextTitle  string
	stableContext       string
	dynamicContextTitle string
	dynamicContext      string
	lastContextSummary  string
	inputVisibility     agentrun.InputVisibility
	inputDisplayContent string

	cycleMu            sync.Mutex
	cycleIdentity      agentrun.CycleIdentity
	cycleCursor        session.ContextCursor
	pendingCommits     map[agentrun.DomainCommitStage]*session.DomainCommitIntent
	lastCommitReceipts map[agentrun.DomainCommitStage]*session.DomainCommitReceipt
	inputCommit        func() error
}

// WithInputVisibility binds the transcript projection chosen by the host for
// this accepted turn. It must match durable input materialization semantics.
func (c *SessionConversation) WithInputVisibility(visibility agentrun.InputVisibility) *SessionConversation {
	if c != nil {
		c.inputVisibility = visibility
	}
	return c
}

// WithInputDisplayContent binds an alternate creator-facing projection for
// host-enriched user input while preserving the canonical model message.
func (c *SessionConversation) WithInputDisplayContent(content string) *SessionConversation {
	if c != nil {
		c.inputDisplayContent = content
	}
	return c
}

func (c *SessionConversation) BindAgentKind(agentKind string) {
	if c == nil {
		return
	}
	c.cycleMu.Lock()
	c.agentKind = strings.TrimSpace(agentKind)
	c.cycleMu.Unlock()
}

func (c *SessionConversation) ResolveExplicitSkills(ctx context.Context, message string) ([]novaskills.Invocation, error) {
	if c == nil {
		return nil, nil
	}
	c.cycleMu.Lock()
	cfg, agentKind := c.cfg, c.agentKind
	c.cycleMu.Unlock()
	return novaskills.ResolveConfiguredInvocations(ctx, cfg, agentKind, message)
}

func NewSessionConversation(sess *session.Session, options ...SessionConversationOption) *SessionConversation {
	c := &SessionConversation{session: sess}
	for _, option := range options {
		if option != nil {
			option(c)
		}
	}
	return c
}

// CanonicalSession returns the product Session owned by this conversation.
// The execution host uses it only to construct the public canonical adapter;
// model/tool orchestration must remain on Conversation interfaces.
func (c *SessionConversation) CanonicalSession() *session.Session {
	if c == nil {
		return nil
	}
	return c.session
}

func NewSessionConversationForAgent(sess *session.Session, cfg *config.Config, agentKind string) *SessionConversation {
	return NewSessionConversation(
		sess,
		WithSessionContextConfig(cfg, agentKind),
	)
}

func NewSessionConversationForAgentWithRuntimeContext(sess *session.Session, cfg *config.Config, agentKind, title, content string) *SessionConversation {
	return NewSessionConversation(
		sess,
		WithSessionContextConfig(cfg, agentKind),
		WithSessionRuntimeContext(title, content),
	)
}

func NewSessionConversationForAgentWithRuntimeContexts(sess *session.Session, cfg *config.Config, agentKind, stableTitle, stableContent, dynamicTitle, dynamicContent string) *SessionConversation {
	return NewSessionConversation(
		sess,
		WithSessionContextConfig(cfg, agentKind),
		WithSessionStableRuntimeContext(stableTitle, stableContent),
		WithSessionRuntimeContext(dynamicTitle, dynamicContent),
	)
}

type SessionConversationOption func(*SessionConversation)

func WithSessionContextConfig(cfg *config.Config, agentKind string) SessionConversationOption {
	return func(c *SessionConversation) {
		c.cfg = cfg
		c.agentKind = agentKind
	}
}

func WithSessionRuntimeContext(title, content string) SessionConversationOption {
	return func(c *SessionConversation) {
		c.dynamicContextTitle = title
		c.dynamicContext = content
	}
}

func WithSessionStableRuntimeContext(title, content string) SessionConversationOption {
	return func(c *SessionConversation) {
		c.stableContextTitle = title
		c.stableContext = content
	}
}

func (c *SessionConversation) ContextSourceSummary() string {
	if c == nil {
		return ""
	}
	c.cycleMu.Lock()
	defer c.cycleMu.Unlock()
	return c.lastContextSummary
}

func (c *SessionConversation) AppendAssistant(content string) error {
	return c.AppendAssistantWithMetadata(content, "", session.MessageMetadata{})
}

func (c *SessionConversation) AppendAssistantWithMetadata(content, _ string, metadata session.MessageMetadata) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("会话不存在")
	}
	identity := c.agentCycleIdentitySnapshot()
	if !agentrun.ValidCycleIdentity(identity) {
		return ErrMissingAgentCycleIdentity
	}
	message := agent.AssistantMessage(content, nil)
	message.Extra = providers.ContinuationExtra(metadata.ProviderContinuation)
	intent, err := session.NewDomainCommitIntent(session.DomainCommitIdentity{
		CommandID: string(identity.CommandID), OperationID: string(identity.OperationID), Cycle: identity.Cycle,
	}, message, metadata)
	if err != nil {
		return err
	}
	c.cycleMu.Lock()
	defer c.cycleMu.Unlock()
	c.ensureCycleCommitMapsLocked()
	intent = intent.WithExpectedContextCursor(c.cycleCursor)
	if pending := c.pendingCommits[agentrun.DomainCommitOutput]; pending != nil && pending.Hash != intent.Hash {
		return fmt.Errorf("agent cycle staged multiple assistant payloads")
	}
	c.pendingCommits[agentrun.DomainCommitOutput] = &intent
	return nil
}

// CommitAgentCanonicalOutput writes the final provider-neutral assistant
// message and public Agent hash atomically. Product projections may choose a
// different transcript view, but recovery always proves this exact raw output.
func (c *SessionConversation) CommitAgentCanonicalOutput(
	ctx context.Context,
	message *agent.Message,
	metadata session.MessageMetadata,
	agentCanonicalHash string,
) (session.DomainCommitReceipt, error) {
	if c == nil || c.session == nil {
		return session.DomainCommitReceipt{}, fmt.Errorf("会话不存在")
	}
	if message == nil || message.Role != agent.Assistant || len(message.ToolCalls) != 0 {
		return session.DomainCommitReceipt{}, fmt.Errorf("canonical session output requires a final assistant message")
	}
	identity := c.agentCycleIdentitySnapshot()
	if !agentrun.ValidCycleIdentity(identity) {
		return session.DomainCommitReceipt{}, ErrMissingAgentCycleIdentity
	}
	intent, err := session.NewDomainCommitIntent(session.DomainCommitIdentity{
		CommandID: string(identity.CommandID), OperationID: string(identity.OperationID), Cycle: identity.Cycle,
	}, message, metadata)
	if err != nil {
		return session.DomainCommitReceipt{}, err
	}
	intent, err = intent.WithAgentCanonicalHash(agentCanonicalHash)
	if err != nil {
		return session.DomainCommitReceipt{}, err
	}
	c.cycleMu.Lock()
	intent = intent.WithExpectedContextCursor(c.cycleCursor)
	c.cycleMu.Unlock()
	receipt, err := c.session.CommitDomainMessageContext(ctx, intent)
	if err != nil {
		return session.DomainCommitReceipt{}, err
	}
	c.cycleMu.Lock()
	c.cycleCursor = c.session.ContextCursor()
	c.cycleMu.Unlock()
	return receipt, nil
}

// BindAgentCycleIdentity resets process-local staging for the exact durable
// coordinator cycle selected before model execution.
func (c *SessionConversation) BindAgentCycleIdentity(identity agentrun.CycleIdentity) {
	if c == nil {
		return
	}
	cursor := c.session.ContextCursor()
	c.cycleMu.Lock()
	c.cycleIdentity = identity
	c.cycleCursor = cursor
	c.pendingCommits = make(map[agentrun.DomainCommitStage]*session.DomainCommitIntent)
	c.lastCommitReceipts = make(map[agentrun.DomainCommitStage]*session.DomainCommitReceipt)
	c.inputCommit = nil
	c.cycleMu.Unlock()
}

func (c *SessionConversation) BindAgentCycleInputCommit(commit func() error) {
	if c == nil {
		return
	}
	c.cycleMu.Lock()
	c.inputCommit = commit
	c.cycleMu.Unlock()
}

func (c *SessionConversation) agentCycleIdentitySnapshot() agentrun.CycleIdentity {
	if c == nil {
		return agentrun.CycleIdentity{}
	}
	c.cycleMu.Lock()
	defer c.cycleMu.Unlock()
	return c.cycleIdentity
}

func (c *SessionConversation) PendingAgentCycleCommit(stage agentrun.DomainCommitStage) (agentrun.DomainCommitIntent, bool, error) {
	if c == nil {
		return agentrun.DomainCommitIntent{}, false, nil
	}
	c.cycleMu.Lock()
	defer c.cycleMu.Unlock()
	pending := c.pendingCommits[stage]
	if pending == nil {
		return agentrun.DomainCommitIntent{}, false, nil
	}
	return agentrun.DomainCommitIntent{Identity: c.cycleIdentity, Stage: stage, Hash: pending.Hash}, true, nil
}

// CommitAgentCycle publishes only actor-authorized terminal outcomes. Abort and
// failure discard staged output; the accepted user input remains canonical.
func (c *SessionConversation) CommitAgentCycle(ctx context.Context, outcome agentrun.Outcome) error {
	return c.CommitAgentCycleStage(ctx, agentrun.DomainCommitOutput, outcome)
}

func (c *SessionConversation) CommitAgentCycleStage(_ context.Context, stage agentrun.DomainCommitStage, outcome agentrun.Outcome) error {
	if c == nil || c.session == nil {
		return nil
	}
	c.cycleMu.Lock()
	c.ensureCycleCommitMapsLocked()
	if stage == agentrun.DomainCommitOutput && !agentrun.OutcomeMayCommitDomain(outcome) {
		delete(c.pendingCommits, stage)
		delete(c.lastCommitReceipts, stage)
		c.cycleMu.Unlock()
		return nil
	}
	if c.pendingCommits[stage] == nil {
		delete(c.lastCommitReceipts, stage)
		c.cycleMu.Unlock()
		return nil
	}
	intent := *c.pendingCommits[stage]
	c.cycleMu.Unlock()

	receipt, err := c.session.CommitDomainMessage(intent)
	if err != nil {
		return err
	}
	cursor := c.session.ContextCursor()
	c.cycleMu.Lock()
	delete(c.pendingCommits, stage)
	c.lastCommitReceipts[stage] = &receipt
	c.cycleCursor = cursor
	c.cycleMu.Unlock()
	return nil
}

func (c *SessionConversation) LastAgentCycleCommitReceipt(stage agentrun.DomainCommitStage) (agentrun.DomainCommitReceipt, bool) {
	if c == nil {
		return agentrun.DomainCommitReceipt{}, false
	}
	c.cycleMu.Lock()
	defer c.cycleMu.Unlock()
	receipt := c.lastCommitReceipts[stage]
	if receipt == nil {
		return agentrun.DomainCommitReceipt{}, false
	}
	return agentrun.DomainCommitReceipt{
		Identity: c.cycleIdentity, Stage: stage, Hash: receipt.Hash,
		Revision: strconv.FormatUint(receipt.ContextRevision, 10),
	}, true
}

func (c *SessionConversation) ensureCycleCommitMapsLocked() {
	if c.pendingCommits == nil {
		c.pendingCommits = make(map[agentrun.DomainCommitStage]*session.DomainCommitIntent)
	}
	if c.lastCommitReceipts == nil {
		c.lastCommitReceipts = make(map[agentrun.DomainCommitStage]*session.DomainCommitReceipt)
	}
}

func (c *SessionConversation) advanceCycleCursor() {
	if c == nil || c.session == nil {
		return
	}
	cursor := c.session.ContextCursor()
	c.cycleMu.Lock()
	c.cycleCursor = cursor
	c.cycleMu.Unlock()
}

func (c *SessionConversation) agentCycleCursorSnapshot() session.ContextCursor {
	if c == nil || c.session == nil {
		return session.ContextCursor{}
	}
	c.cycleMu.Lock()
	identity := c.cycleIdentity
	cursor := c.cycleCursor
	c.cycleMu.Unlock()
	if !agentrun.ValidCycleIdentity(identity) {
		return c.session.ContextCursor()
	}
	return cursor
}

func (c *SessionConversation) AppendContextMessage(msg *agent.Message) error {
	if msg == nil || (msg.Role == "" && strings.TrimSpace(msg.Content) == "" && len(msg.ToolCalls) == 0) {
		return nil
	}
	return c.AppendContextMessages(msg)
}

func (c *SessionConversation) AppendContextMessages(messages ...*agent.Message) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("会话不存在")
	}
	if len(messages) == 0 {
		return nil
	}
	identity := c.agentCycleIdentitySnapshot()
	if !agentrun.ValidCycleIdentity(identity) {
		return ErrMissingAgentCycleIdentity
	}
	err := c.session.AppendContextMessagesAt(c.agentCycleCursorSnapshot(), messages...)
	if err != nil {
		return err
	}
	c.advanceCycleCursor()
	return nil
}

func (c *SessionConversation) ToolResultContextPolicy() toolresult.ContextPolicy {
	if c == nil {
		return toolresult.ContextPolicy{}
	}
	agentKind := c.agentKind
	if strings.TrimSpace(agentKind) == "" {
		agentKind = config.AgentKindIDE
	}
	return toolresult.ResolveContextPolicy(c.cfg, agentKind)
}

func (c *SessionConversation) ToolArtifactStore() agent.ToolArtifactBackend {
	if c == nil || c.session == nil {
		return nil
	}
	return c.session.ToolArtifactStore()
}

func (c *SessionConversation) AppendDisplayEvent(event session.DisplayEvent) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("会话不存在")
	}
	return c.session.AppendDisplayEvent(event)
}

func (c *SessionConversation) RecordDisplayAsk(event session.DisplayEvent) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("session does not exist")
	}
	return c.session.RecordDisplayAsk(event)
}

func (c *SessionConversation) UpdateDisplayToolStatus(id, name, status string) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("会话不存在")
	}
	return c.session.UpdateDisplayToolStatus(id, name, status)
}

func (c *SessionConversation) AppendDisplayToolArgs(id, name, delta string) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("会话不存在")
	}
	return c.session.AppendDisplayToolArgs(id, name, delta)
}

func (c *SessionConversation) AppendDisplayEventContent(id, role, delta string) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("会话不存在")
	}
	return c.session.AppendDisplayEventContent(id, role, delta)
}

func (c *SessionConversation) FlushDisplayEventContent(id, role string) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("会话不存在")
	}
	return c.session.FlushDisplayEventContent(id, role)
}

func (c *SessionConversation) FinalizeDisplayAssistantRun(runID, finalSegmentID, terminalPhase string) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("会话不存在")
	}
	return c.session.FinalizeDisplayAssistantRun(runID, finalSegmentID, terminalPhase)
}

func (c *SessionConversation) UpdateDisplayToolResult(id, name, status, result string, presentation *agent.ToolPresentation) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("会话不存在")
	}
	return c.session.UpdateDisplayToolResult(id, name, status, result, presentation)
}

func (c *SessionConversation) UpdateDisplayToolIllustration(id, name string, illustration *session.ChapterIllustration) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("会话不存在")
	}
	return c.session.UpdateDisplayToolIllustration(id, name, illustration)
}

func (c *SessionConversation) MarkInterrupted(userMessage, assistantContent, reason string) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("会话不存在")
	}
	return c.session.MarkInterrupted(userMessage, assistantContent, reason)
}

func (c *SessionConversation) PendingInterruption() *session.Interruption {
	if c == nil || c.session == nil {
		return nil
	}
	return c.session.PendingInterruption()
}

func (c *SessionConversation) ResolveInterruption(id string) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("会话不存在")
	}
	return c.session.ResolveInterruption(id)
}
