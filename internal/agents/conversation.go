package agents

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/agents/session"
)

// Conversation 抽象 Agent 对话的上下文读取与结果写入。
// 写作模式写入普通 session，游戏模式可写入 interactive/story。
type Conversation interface {
	ModelContextAssembler
	AppendAssistant(content string) error
	MarkInterrupted(userMessage, assistantContent, reason string) error
	PendingInterruption() *session.Interruption
	ResolveInterruption(id string) error
}

// ContextSourceReporter 可由 Conversation 提供本轮已拼装的业务上下文来源。
// ChatService 会在 CommitModelInput 后追加打印，便于排查非通用注入内容。
type ContextSourceReporter interface {
	ContextSourceSummary() string
}

type toolArtifactStoreProvider interface {
	ToolArtifactStore() agent.ToolArtifactStore
}

// ContextLedgerReporter exposes bounded metadata for the actual domain context
// fragments assembled by a Conversation. Full fragment content is never
// persisted by the runtime.
type ContextLedgerReporter interface {
	ContextLedgerParts() []ContextLedgerPart
}

// FinalContextLedgerReporter rebuilds domain context audit metadata from the
// exact message list sent to the model after context compaction. Implementers
// must not retain full message bodies in the returned durable records.
type FinalContextLedgerReporter interface {
	ContextLedgerPartsForMessages(messages []*agent.Message) []ContextLedgerPart
}

// RunTraceMetadata is the bounded interactive identity attached to one run.
// A Conversation may fill fields such as TurnID only after its final output is
// committed, so the runtime resolves it again during finish.
type RunTraceMetadata struct {
	StoryID         string `json:"story_id,omitempty"`
	BranchID        string `json:"branch_id,omitempty"`
	TurnID          string `json:"turn_id,omitempty"`
	MaintenanceTask string `json:"maintenance_task,omitempty"`
}

type RunTraceMetadataReporter interface {
	RunTraceMetadata() RunTraceMetadata
}

// InteractiveNarrativeReadinessReporter marks the protocol boundary after a
// Game Agent has successfully staged its hidden TurnResult. Runtime uses it to
// recognize cancellation after submission as successful turn completion.
type InteractiveNarrativeReadinessReporter interface {
	InteractiveNarrativeReady() bool
}

type SessionConversation struct {
	session             *session.Session
	cfg                 *config.Config
	agentKind           string
	stableContextTitle  string
	stableContext       string
	dynamicContextTitle string
	dynamicContext      string
	lastContextSummary  string

	cycleMu            sync.Mutex
	cycleIdentity      HarnessCycleIdentity
	cycleCursor        session.ContextCursor
	structuralCursor   *session.ContextCursor
	structuralCommit   func(func() error) error
	pendingCommits     map[HarnessDomainCommitStage]*session.DomainCommitIntent
	lastCommitReceipts map[HarnessDomainCommitStage]*session.DomainCommitReceipt
	inputCommit        func() error
	pendingCompaction  *preparedSessionContextCompaction
	pendingContextOps  []session.ContextOperation
	contextWindowBase  *contextWindowModelBase
}

func (c *SessionConversation) BindHarnessAgentKind(agentKind string) {
	if c == nil {
		return
	}
	c.cycleMu.Lock()
	c.agentKind = strings.TrimSpace(agentKind)
	c.cycleMu.Unlock()
}

func (c *SessionConversation) ResolveExplicitSkills(ctx context.Context, message string) ([]ExplicitSkillInvocation, error) {
	if c == nil {
		return nil, nil
	}
	c.cycleMu.Lock()
	cfg, agentKind := c.cfg, c.agentKind
	c.cycleMu.Unlock()
	return ResolveExplicitSkillInvocations(ctx, cfg, agentKind, message)
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

// WithContextCursorBarrier pins structural context writes to an immutable
// session snapshot even when the conversation is not running inside a Harness
// cycle (for example a manual compaction request).
func (c *SessionConversation) WithContextCursorBarrier(cursor session.ContextCursor) *SessionConversation {
	if c == nil {
		return c
	}
	c.cycleMu.Lock()
	c.cycleCursor = cursor
	c.structuralCursor = &cursor
	c.cycleMu.Unlock()
	return c
}

// WithContextCommitGate lets the App revalidate its workspace generation and
// hold its short admission lock across the final journal append. The gate must
// not perform model work or wait for task settlement.
func (c *SessionConversation) WithContextCommitGate(gate func(func() error) error) *SessionConversation {
	if c == nil {
		return c
	}
	c.cycleMu.Lock()
	c.structuralCommit = gate
	c.cycleMu.Unlock()
	return c
}

func (c *SessionConversation) ContextSourceSummary() string {
	if c == nil {
		return ""
	}
	c.cycleMu.Lock()
	defer c.cycleMu.Unlock()
	return c.lastContextSummary
}

func (c *SessionConversation) CompactContextIfNeeded(ctx context.Context, input ContextCompactionInput) ([]*agent.Message, ContextCompactionResult, error) {
	policy := c.compactionPolicy()
	if input.ContextWindowTokens > 0 {
		policy.ContextWindowTokens = input.ContextWindowTokens
	}
	input = withDefaultContextProjectionReserves(c.cfg, c.agentKind, input, 0)
	phase := strings.TrimSpace(input.Phase)
	if phase == "" {
		phase = contextCompactionPhasePreRun
	}
	estimatedTokensBefore := EstimateContextTokens(input.Messages, input.Tools)
	tokensBefore := calibratedContextTokens(estimatedTokensBefore, input)
	projectedTokensBefore := projectedContextTokens(tokensBefore, input)
	result := ContextCompactionResult{
		Phase:                    phase,
		EstimatedTokensBefore:    estimatedTokensBefore,
		ObservedPromptTokens:     input.ObservedPromptTokens,
		ObservedEstimateTokens:   input.ObservedEstimateTokens,
		TokensBefore:             tokensBefore,
		ProjectedTokensBefore:    projectedTokensBefore,
		ReservedCompletionTokens: input.ReservedCompletionTokens,
		ReservedToolResultTokens: input.ReservedToolResultTokens,
		ContextWindowTokens:      policy.ContextWindowTokens,
		Strategy:                 policy.Strategy,
		Threshold:                policy.Threshold,
		MessageCountBefore:       len(input.Messages),
		RetainedTurns:            policy.RetainedTurns,
	}
	shouldCompact, skipped := policy.shouldCompact(projectedTokensBefore, input.Force)
	if !shouldCompact {
		result.SkippedReason = skipped
		return input.Messages, result, nil
	}
	source, existingCheckpoint, sourceStart, sourceEnd, err := c.compactionIncrementalSource(ctx, input.KeepLatestUser)
	if err != nil {
		return input.Messages, result, fmt.Errorf("read canonical compaction source: %w", err)
	}
	if strings.TrimSpace(input.ExistingCheckpoint) != "" {
		existingCheckpoint = input.ExistingCheckpoint
	}
	if len(source) == 0 && strings.TrimSpace(existingCheckpoint) == "" && strings.TrimSpace(input.ReferenceContext) == "" {
		result.SkippedReason = "empty_source"
		return input.Messages, result, nil
	}
	if !input.Force {
		if removal, ok := c.session.LatestContextCompactionRemoval(c.agentKind); ok && removal.SourceStartIndex == sourceStart && removal.SourceEndIndex >= sourceEnd {
			result.SkippedReason = "removed_same_source"
			return input.Messages, result, nil
		}
	}
	sourceTokens := EstimateContextTokens(source, nil)
	emitContextCompactionEvent(input.Emit, phase, "started", result)
	summary, inputChars, err := summarizeContextInLayers(ctx, c.cfg, c.agentKind, existingCheckpoint, source, input.ReferenceContext, sourceTokens, policy, func(attempt int, delta string) {
		emitContextCompactionDeltaEvent(input.Emit, phase, result, attempt, delta)
	})
	if err != nil {
		emitContextCompactionEvent(input.Emit, phase, "failed", result)
		return input.Messages, result, err
	}
	epoch := c.nextCompactionEpoch()
	leading, compactableMessages := c.splitLeadingRuntimeMessages(input.Messages)
	newMessages := compactMessagesForModel(compactableMessages, summary, epoch, policy.RetainedTurns)
	if len(leading) > 0 {
		newMessages = append(append([]*agent.Message(nil), leading...), newMessages...)
	}
	result.Triggered = true
	result.Epoch = epoch
	result.Summary = summary
	result.TokensAfter = calibratedContextTokens(EstimateContextTokens(newMessages, input.Tools), input)
	result.ProjectedTokensAfter = projectedContextTokens(result.TokensAfter, input)
	result.TargetRatio = contextCompactionRatio(countRunes(summary), inputChars)
	result.SourceMessageCount = len(source)
	result.MessageCountAfter = len(newMessages)
	if !input.Force && (phase == contextCompactionPhasePreRun || phase == contextCompactionPhaseMidRun) {
		c.stagePreparedSessionCompaction(preparedSessionContextCompaction{
			Result: result, SourceStartIndex: sourceStart, SourceEndIndex: sourceEnd,
		})
	}
	// Automatic pre/mid-run compaction is intentionally transient. Publishing
	// a checkpoint while the turn actor is still running would let canonical
	// history advance outside a typed structural operation and could survive a
	// failed turn. Manual and post-settlement persistence use CompactIfNeeded.
	emitContextCompactionEvent(input.Emit, phase, "completed", result)
	return newMessages, result, nil
}

func (c *SessionConversation) splitLeadingRuntimeMessages(messages []*agent.Message) ([]*agent.Message, []*agent.Message) {
	leading := c.leadingRuntimeMessages()
	if len(leading) == 0 || len(messages) < len(leading) {
		return nil, messages
	}
	for i := range leading {
		if messages[i] == nil || leading[i] == nil || messages[i].Role != leading[i].Role || messages[i].Content != leading[i].Content {
			return nil, messages
		}
	}
	return messages[:len(leading)], messages[len(leading):]
}

func (c *SessionConversation) compactionPolicy() contextCompactionPolicy {
	if c == nil {
		return contextCompactionPolicy{}
	}
	agentKind := c.agentKind
	if strings.TrimSpace(agentKind) == "" {
		agentKind = config.AgentKindIDE
	}
	policy := resolveContextCompactionPolicy(c.cfg, agentKind)
	return policy
}

func (c *SessionConversation) nextCompactionEpoch() int {
	return c.session.NextContextCompactionEpoch(c.agentKind)
}

func (c *SessionConversation) compactionIncrementalSource(ctx context.Context, keepLatestUser bool) ([]*agent.Message, string, int, int, error) {
	if c == nil || c.session == nil {
		return nil, "", 0, 0, nil
	}
	total := c.session.MessageCountTotal()
	sourceStart := total - c.session.MessageCountSinceClear()
	if sourceStart < 0 {
		sourceStart = 0
	}
	existingCheckpoint := ""
	if compaction, ok := c.session.LatestContextCompaction(c.agentKind); ok {
		existingCheckpoint = compaction.Summary
		if compaction.SourceEndIndex > sourceStart {
			sourceStart = compaction.SourceEndIndex
		}
	}
	if sourceStart > total {
		sourceStart = total
	}
	sourceEnd := total
	if !keepLatestUser && sourceEnd > sourceStart {
		latest, err := c.session.ReadMessageRange(ctx, sourceEnd-1, sourceEnd)
		if err != nil {
			return nil, "", sourceStart, sourceEnd, err
		}
		if len(latest) == 1 && latest[0] != nil && latest[0].Role == agent.User {
			sourceEnd--
		}
	}
	if sourceEnd < sourceStart {
		sourceEnd = sourceStart
	}
	messages, err := c.session.ReadMessageRange(ctx, sourceStart, sourceEnd)
	if err != nil {
		return nil, "", sourceStart, sourceEnd, err
	}
	source := compactionSourceMessages(applyToolResultContextPolicy(messages, c.ToolResultContextPolicy()), true)
	return source, existingCheckpoint, sourceStart, sourceEnd, nil
}

func compactionSourceMessages(messages []*agent.Message, keepLatestUser bool) []*agent.Message {
	source := make([]*agent.Message, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		if isContextCompactionMessage(msg) {
			continue
		}
		source = append(source, sanitizeCompactionSourceMessage(msg))
	}
	if !keepLatestUser && len(source) > 0 && source[len(source)-1].Role == agent.User {
		source = source[:len(source)-1]
	}
	return source
}

func sanitizeCompactionSourceMessage(msg *agent.Message) *agent.Message {
	if msg == nil {
		return nil
	}
	copied := *msg
	copied.ReasoningContent = ""
	return &copied
}

func retainTailByUserTurns(messages []*agent.Message, retainedTurns int) []*agent.Message {
	if retainedTurns <= 0 {
		retainedTurns = config.DefaultContextCompactionRetainedTurns
	}
	if retainedTurns > config.MaxContextCompactionRetainedTurns {
		retainedTurns = config.MaxContextCompactionRetainedTurns
	}
	userCount := 0
	start := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i] == nil || messages[i].Role != agent.User {
			continue
		}
		userCount++
		if userCount == retainedTurns {
			start = i
			break
		}
	}
	if userCount < retainedTurns {
		return messages
	}
	return append([]*agent.Message(nil), messages[start:]...)
}

func (c *SessionConversation) AppendAssistant(content string) error {
	return c.AppendAssistantWithMetadata(content, "", session.MessageMetadata{})
}

func (c *SessionConversation) AppendAssistantWithMetadata(content, _ string, metadata session.MessageMetadata) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("会话不存在")
	}
	identity := c.agentCycleIdentitySnapshot()
	if !validHarnessCycleIdentity(identity) {
		return ErrMissingAgentCycleIdentity
	}
	c.cycleMu.Lock()
	metadata.ContextOperations = append(
		append([]session.ContextOperation(nil), metadata.ContextOperations...),
		c.pendingContextOps...,
	)
	c.cycleMu.Unlock()
	intent, err := session.NewDomainCommitIntent(session.DomainCommitIdentity{
		CommandID: string(identity.CommandID), OperationID: string(identity.OperationID), Cycle: identity.Cycle,
	}, agent.AssistantMessage(content, nil), metadata)
	if err != nil {
		return err
	}
	c.cycleMu.Lock()
	defer c.cycleMu.Unlock()
	c.ensureCycleCommitMapsLocked()
	intent = intent.WithExpectedContextCursor(c.cycleCursor)
	if pending := c.pendingCommits[HarnessDomainCommitOutput]; pending != nil && pending.Hash != intent.Hash {
		return fmt.Errorf("agent cycle staged multiple assistant payloads")
	}
	c.pendingCommits[HarnessDomainCommitOutput] = &intent
	return nil
}

// BindAgentCycleIdentity resets process-local staging for the exact durable
// coordinator cycle selected before model execution.
func (c *SessionConversation) BindAgentCycleIdentity(identity HarnessCycleIdentity) {
	if c == nil {
		return
	}
	cursor := c.session.ContextCursor()
	c.cycleMu.Lock()
	c.cycleIdentity = identity
	c.cycleCursor = cursor
	c.structuralCursor = nil
	c.structuralCommit = nil
	c.pendingCommits = make(map[HarnessDomainCommitStage]*session.DomainCommitIntent)
	c.lastCommitReceipts = make(map[HarnessDomainCommitStage]*session.DomainCommitReceipt)
	c.inputCommit = nil
	c.pendingContextOps = nil
	c.contextWindowBase = nil
	c.cycleMu.Unlock()
}

func (c *SessionConversation) ActiveContextCheckpoints(agentKind string) ([]session.ContextOperation, error) {
	if c == nil || c.session == nil {
		return nil, nil
	}
	return c.session.ActiveContextCheckpoints(agentKind)
}

// StageContextOperation keeps structural context metadata process-local until
// the assistant output is authorized and committed by the durable harness.
func (c *SessionConversation) StageContextOperation(operation session.ContextOperation) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("会话不存在")
	}
	c.cycleMu.Lock()
	defer c.cycleMu.Unlock()
	if !validHarnessCycleIdentity(c.cycleIdentity) {
		return ErrMissingAgentCycleIdentity
	}
	// Mutation observations enrich the active checkpoint while the run is
	// still process-local. Replace that staged checkpoint so a preempted output
	// commits the latest bounded receipts instead of an obsolete empty marker.
	if operation.Kind == session.ContextOperationCheckpoint {
		for index, pending := range c.pendingContextOps {
			if pending.Kind == operation.Kind && pending.AgentKind == operation.AgentKind && pending.CheckpointID == operation.CheckpointID {
				c.pendingContextOps[index] = operation
				return nil
			}
		}
	}
	if len(c.pendingContextOps) >= 16 {
		return fmt.Errorf("too many context operations in one agent cycle")
	}
	c.pendingContextOps = append(c.pendingContextOps, operation)
	return nil
}

// HasPendingContextOperations reports whether an otherwise empty assistant
// commit is still required to durably publish structural context metadata.
func (c *SessionConversation) HasPendingContextOperations() bool {
	if c == nil {
		return false
	}
	c.cycleMu.Lock()
	defer c.cycleMu.Unlock()
	return len(c.pendingContextOps) > 0
}

func (c *SessionConversation) BindAgentCycleInputCommit(commit func() error) {
	if c == nil {
		return
	}
	c.cycleMu.Lock()
	c.inputCommit = commit
	c.cycleMu.Unlock()
}

func (c *SessionConversation) agentCycleIdentitySnapshot() HarnessCycleIdentity {
	if c == nil {
		return HarnessCycleIdentity{}
	}
	c.cycleMu.Lock()
	defer c.cycleMu.Unlock()
	return c.cycleIdentity
}

func (c *SessionConversation) PendingAgentCycleCommit(stage HarnessDomainCommitStage) (HarnessDomainCommitIntent, bool, error) {
	if c == nil {
		return HarnessDomainCommitIntent{}, false, nil
	}
	c.cycleMu.Lock()
	defer c.cycleMu.Unlock()
	pending := c.pendingCommits[stage]
	if pending == nil {
		return HarnessDomainCommitIntent{}, false, nil
	}
	return HarnessDomainCommitIntent{Identity: c.cycleIdentity, Stage: stage, Hash: pending.Hash}, true, nil
}

// CommitAgentCycle publishes only actor-authorized terminal outcomes. Abort and
// failure discard staged output; the accepted user input remains canonical.
func (c *SessionConversation) CommitAgentCycle(ctx context.Context, outcome RunOutcome) error {
	return c.CommitAgentCycleStage(ctx, HarnessDomainCommitOutput, outcome)
}

func (c *SessionConversation) CommitAgentCycleStage(_ context.Context, stage HarnessDomainCommitStage, outcome RunOutcome) error {
	if c == nil || c.session == nil {
		return nil
	}
	c.cycleMu.Lock()
	c.ensureCycleCommitMapsLocked()
	if stage == HarnessDomainCommitOutput && !runOutcomeMayCommitDomain(outcome) {
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

func (c *SessionConversation) LastAgentCycleCommitReceipt(stage HarnessDomainCommitStage) (HarnessDomainCommitReceipt, bool) {
	if c == nil {
		return HarnessDomainCommitReceipt{}, false
	}
	c.cycleMu.Lock()
	defer c.cycleMu.Unlock()
	receipt := c.lastCommitReceipts[stage]
	if receipt == nil {
		return HarnessDomainCommitReceipt{}, false
	}
	return HarnessDomainCommitReceipt{
		Identity: c.cycleIdentity, Stage: stage, Hash: receipt.Hash,
		Revision: strconv.FormatUint(receipt.ContextRevision, 10),
	}, true
}

func (c *SessionConversation) ensureCycleCommitMapsLocked() {
	if c.pendingCommits == nil {
		c.pendingCommits = make(map[HarnessDomainCommitStage]*session.DomainCommitIntent)
	}
	if c.lastCommitReceipts == nil {
		c.lastCommitReceipts = make(map[HarnessDomainCommitStage]*session.DomainCommitReceipt)
	}
}

func (c *SessionConversation) advanceCycleCursor() {
	if c == nil || c.session == nil {
		return
	}
	cursor := c.session.ContextCursor()
	c.cycleMu.Lock()
	c.cycleCursor = cursor
	if c.structuralCursor != nil {
		*c.structuralCursor = cursor
	}
	c.cycleMu.Unlock()
}

func (c *SessionConversation) agentCycleCursorSnapshot() session.ContextCursor {
	if c == nil || c.session == nil {
		return session.ContextCursor{}
	}
	c.cycleMu.Lock()
	identity := c.cycleIdentity
	cursor := c.cycleCursor
	structural := c.structuralCursor != nil
	c.cycleMu.Unlock()
	if !structural && !validHarnessCycleIdentity(identity) {
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
	c.cycleMu.Lock()
	structural := c.structuralCursor != nil
	c.cycleMu.Unlock()
	var err error
	if structural || validHarnessCycleIdentity(identity) {
		commit := func() error { return c.session.AppendContextMessagesAt(c.agentCycleCursorSnapshot(), messages...) }
		c.cycleMu.Lock()
		gate := c.structuralCommit
		c.cycleMu.Unlock()
		if gate != nil {
			err = gate(commit)
		} else {
			err = commit()
		}
	} else {
		return ErrMissingAgentCycleIdentity
	}
	if err != nil {
		return err
	}
	c.advanceCycleCursor()
	return nil
}

func (c *SessionConversation) ToolResultContextPolicy() ToolResultContextPolicy {
	if c == nil {
		return ToolResultContextPolicy{}
	}
	agentKind := c.agentKind
	if strings.TrimSpace(agentKind) == "" {
		agentKind = config.AgentKindIDE
	}
	return resolveToolResultContextPolicy(c.cfg, agentKind)
}

func (c *SessionConversation) ToolArtifactStore() agent.ToolArtifactStore {
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

func (c *SessionConversation) UpdateDisplayToolResult(id, name, status, result string) error {
	if c == nil || c.session == nil {
		return fmt.Errorf("会话不存在")
	}
	return c.session.UpdateDisplayToolResult(id, name, status, result)
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
