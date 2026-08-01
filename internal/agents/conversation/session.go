package conversation

import (
	"context"
	agentcontext "denova/internal/agents/context"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/toolresult"
	"fmt"
	"strconv"
	"strings"
	"sync"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"

	"denova/config"
	agentcompaction "denova/internal/agents/context/compaction"
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

	cycleMu                 sync.Mutex
	cycleIdentity           agentrun.CycleIdentity
	cycleCursor             session.ContextCursor
	structuralCursor        *session.ContextCursor
	structuralCommit        func(func() error) error
	pendingCommits          map[agentrun.DomainCommitStage]*session.DomainCommitIntent
	lastCommitReceipts      map[agentrun.DomainCommitStage]*session.DomainCommitReceipt
	inputCommit             func() error
	pendingCompaction       *preparedSessionContextCompaction
	pendingCompactionHealth *preparedSessionContextCompactionHealth
	pendingCleanup          *preparedSessionToolResultCleanup
	pendingContextOps       []session.ContextOperation
	contextWindowBase       *contextWindowModelBase
}

func (c *SessionConversation) BindHarnessAgentKind(agentKind string) {
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

func (c *SessionConversation) CompactContextIfNeeded(ctx context.Context, input agentcompaction.Input) ([]*agent.Message, agentcompaction.Result, error) {
	policy := c.compactionPolicy()
	if input.ContextWindowTokens > 0 {
		policy.ContextWindowTokens = input.ContextWindowTokens
	}
	if strings.TrimSpace(input.Phase) == "" {
		input.Phase = agentcompaction.PhasePreRun
	}
	if input.Automatic && c.hasPendingContextRewind() {
		// One run may publish only one structural context mutation. The next turn
		// can compact the durable rewind projection safely.
		input.PreflightSkipReason = "staged_rewind_pending"
		input.ObservedPromptTokens = 0
		input.ObservedEstimateTokens = 0
	}

	source, providerSource, existingCheckpoint, sourceStart, sourceEnd, stagedRewind, err := c.stagedRewindCompactionSource(
		input.Messages, input.KeepLatestUser,
	)
	if err != nil {
		return input.Messages, agentcompaction.Result{}, fmt.Errorf("read staged rewind compaction source: %w", err)
	}
	if !stagedRewind {
		source, existingCheckpoint, sourceStart, sourceEnd, err = c.compactionIncrementalSource(ctx, input.KeepLatestUser)
		if err != nil {
			return input.Messages, agentcompaction.Result{}, fmt.Errorf("read canonical compaction source: %w", err)
		}
		if mapped, ok := c.providerVisibleCompactionSource(source, sourceStart); ok {
			providerSource = mapped
		} else {
			providerSource = source
		}
	}
	if strings.TrimSpace(input.ExistingCheckpoint) != "" {
		existingCheckpoint = input.ExistingCheckpoint
	}
	if !input.Force {
		if removal, ok := c.session.LatestContextCompactionRemoval(c.agentKind); ok && removal.SourceStartIndex == sourceStart && removal.SourceEndIndex >= sourceEnd {
			input.PreflightSkipReason = "removed_same_source"
		}
	}

	input.SourceMessages = source
	input.SourceMessagesSet = true
	input.ProviderSourceMessages = providerSource
	input.ExistingCheckpoint = existingCheckpoint
	if input.Summarize == nil {
		input.Summarize = agentcompaction.GenerateSummary
	}
	input.CandidateFingerprint, input.CandidateGeneration = agentcompaction.CandidateIdentity(input.Messages, 0)
	healthCursor := c.session.ContextCursor()
	structureFingerprint := c.compactionStructureFingerprint(input)
	if health, ok := c.session.LatestContextCompactionHealth(c.agentKind); ok &&
		(agentcompaction.FailureState{
			StructureFingerprint: health.StructureFingerprint,
			ConsecutiveFailures:  health.ConsecutiveFailures,
		}).Blocks(structureFingerprint, policy.MaxConsecutiveFailures, input.Automatic) {
		input.PreflightSkipReason = "consecutive_failure_fuse"
		input.ConsecutiveFailures = health.ConsecutiveFailures
		input.FailureFuseOpen = true
	}
	if input.Automatic && input.PreflightSkipReason == "" && !c.hasActiveDurableContextRewind() {
		cleanupMinimum := agentcontext.EffectiveToolResultCleanupMinimum(
			input.Messages, input.Tools, resolveContextPressurePolicy(c.cfg, c.agentKind, input.Messages),
		)
		if previous, ok := c.session.LatestContextCompaction(c.agentKind); ok && agentcompaction.NoProgressLatched(
			previous.TokensAfter, previous.ContextWindowTokens, previous.Threshold, policy.RecoveryBand,
			agentcontext.EstimateTokens(source, nil), cleanupMinimum,
			previous.CandidateFingerprint, previous.CandidateGeneration,
			input.CandidateFingerprint, input.CandidateGeneration,
		) {
			input.PreflightSkipReason = "degraded_no_progress_latch"
		}
	}

	newMessages, result, err := agentcompaction.Prepare(
		ctx, c.cfg, c.agentKind, input, c.nextCompactionEpoch(),
	)
	if err != nil {
		c.stageSessionCompactionHealth(healthCursor, structureFingerprint, agentcompaction.HealthFailure, &result)
		return input.Messages, result, err
	}
	if !result.Triggered {
		return newMessages, result, nil
	}
	if !input.Force && result.Phase == agentcompaction.PhaseModelStep {
		c.stagePreparedSessionCompaction(preparedSessionContextCompaction{
			Result: result, SourceStartIndex: sourceStart, SourceEndIndex: sourceEnd,
		})
	}
	c.stageSessionCompactionHealth(healthCursor, structureFingerprint, agentcompaction.HealthSuccess, &result)
	// Automatic model-step compaction stays transient until the harness commits
	// its typed structural operation after the turn settles.
	return newMessages, result, nil
}

func (c *SessionConversation) hasActiveDurableContextRewind() bool {
	if c == nil || c.session == nil {
		return false
	}
	snapshot, err := c.session.SnapshotContext(c.agentKind)
	return err == nil && snapshot.ContextWindow != nil &&
		(snapshot.Compaction == nil || snapshot.ContextWindow.ContextRevision > snapshot.Compaction.ContextRevision)
}

func (c *SessionConversation) compactionPolicy() agentcompaction.Policy {
	if c == nil {
		return agentcompaction.Policy{}
	}
	agentKind := c.agentKind
	if strings.TrimSpace(agentKind) == "" {
		agentKind = config.AgentKindIDE
	}
	policy := agentcompaction.ResolvePolicy(c.cfg, agentKind)
	return policy
}

func (c *SessionConversation) nextCompactionEpoch() int {
	return c.session.NextContextCompactionEpoch(c.agentKind)
}

// SessionContextCompactionProjection freezes the canonical model branch and
// its raw persistence boundary under one Session revision. Messages are the
// provider-neutral model projection; Source excludes checkpoint records and
// transient runtime wrappers.
type SessionContextCompactionProjection struct {
	Messages           []*agent.Message
	Source             []*agent.Message
	ExistingCheckpoint string
	SourceStartIndex   int
	SourceEndIndex     int
	Cursor             session.ContextCursor
}

// SnapshotContextCompaction returns one exact source/projection pair for both
// automatic and manual compaction. In particular, an active rewind is resolved
// before the source is exposed so discarded raw exploration cannot re-enter a
// checkpoint through a cold/manual fallback.
func (c *SessionConversation) SnapshotContextCompaction(ctx context.Context, keepLatestUser bool) (SessionContextCompactionProjection, error) {
	if c == nil || c.session == nil {
		return SessionContextCompactionProjection{}, nil
	}
	snapshot, err := c.session.SnapshotContext(c.agentKind)
	if err != nil {
		return SessionContextCompactionProjection{}, err
	}
	source, checkpoint, sourceStart, sourceEnd, err := c.compactionIncrementalSourceFromSnapshot(ctx, snapshot, keepLatestUser)
	if err != nil {
		return SessionContextCompactionProjection{}, err
	}
	return SessionContextCompactionProjection{
		Messages: c.modelHistory(snapshot), Source: source, ExistingCheckpoint: checkpoint,
		SourceStartIndex: sourceStart, SourceEndIndex: sourceEnd, Cursor: snapshot.Cursor,
	}, nil
}

func (c *SessionConversation) compactionIncrementalSource(ctx context.Context, keepLatestUser bool) ([]*agent.Message, string, int, int, error) {
	projection, err := c.SnapshotContextCompaction(ctx, keepLatestUser)
	if err != nil {
		return nil, "", 0, 0, err
	}
	return projection.Source, projection.ExistingCheckpoint, projection.SourceStartIndex, projection.SourceEndIndex, nil
}

func (c *SessionConversation) compactionIncrementalSourceFromSnapshot(
	ctx context.Context,
	snapshot session.ContextSnapshot,
	keepLatestUser bool,
) ([]*agent.Message, string, int, int, error) {
	total := snapshot.Cursor.MessageCount
	sourceStart := snapshot.Cursor.ClearAfterIndex
	if sourceStart < 0 {
		sourceStart = 0
	}
	existingCheckpoint := ""
	if snapshot.Compaction != nil {
		compaction := *snapshot.Compaction
		existingCheckpoint = compaction.Summary
		if compaction.SourceEndIndex > sourceStart {
			sourceStart = compaction.SourceEndIndex
		}
	}
	if sourceStart > total {
		sourceStart = total
	}
	activeRewind := snapshot.ContextWindow != nil &&
		(snapshot.Compaction == nil || snapshot.ContextWindow.ContextRevision > snapshot.Compaction.ContextRevision)
	minimumSourceStart := sourceStart
	if activeRewind {
		minimumSourceStart = snapshot.Cursor.ClearAfterIndex
	}
	sourceEnd := total
	if !keepLatestUser && sourceEnd > minimumSourceStart {
		latest, err := c.session.ReadMessageRange(ctx, sourceEnd-1, sourceEnd)
		if err != nil {
			return nil, "", sourceStart, sourceEnd, err
		}
		if len(latest) == 1 && latest[0] != nil && latest[0].Role == agent.User {
			sourceEnd--
		}
	}
	if sourceEnd < minimumSourceStart {
		sourceEnd = minimumSourceStart
	}
	if activeRewind {
		// A rewind defines a new canonical model branch without deleting the raw
		// transcript. Compact the exact rewind projection; reading the raw range
		// here would summarize discarded exploration and cannot match the final
		// primary request. The newer compaction revision will supersede the rewind.
		projected := c.modelHistory(snapshot)
		if !keepLatestUser && sourceEnd < total && len(projected) > 0 && projected[len(projected)-1] != nil && projected[len(projected)-1].Role == agent.User {
			projected = projected[:len(projected)-1]
		}
		if sourceEnd < snapshot.Cursor.ClearAfterIndex {
			sourceEnd = snapshot.Cursor.ClearAfterIndex
		}
		source := compactionSourceMessages(projected, true)
		return source, existingCheckpoint, snapshot.Cursor.ClearAfterIndex, sourceEnd, nil
	}
	messages, err := c.session.ReadMessageRange(ctx, sourceStart, sourceEnd)
	if err != nil {
		return nil, "", sourceStart, sourceEnd, err
	}
	// Match the exact projection already assembled for the primary provider
	// request. Canonical rich results remain append-only, but a persisted cleanup
	// placeholder is what both the checkpoint fork and the next model can see.
	if cleanup := snapshot.ToolResultCleanup; cleanup != nil {
		messages = applyToolResultCleanupProjection(messages, sourceStart, *cleanup)
	}
	source := compactionSourceMessages(toolresult.ApplyContextPolicy(messages, c.ToolResultContextPolicy()), true)
	return source, existingCheckpoint, sourceStart, sourceEnd, nil
}

func compactionSourceMessages(messages []*agent.Message, keepLatestUser bool) []*agent.Message {
	source := make([]*agent.Message, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		if agentcontext.IsCompactionSummaryMessage(msg) {
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
	if !agentrun.ValidCycleIdentity(identity) {
		return ErrMissingAgentCycleIdentity
	}
	c.cycleMu.Lock()
	metadata.ContextOperations = append(
		append([]session.ContextOperation(nil), metadata.ContextOperations...),
		c.pendingContextOps...,
	)
	c.cycleMu.Unlock()
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
	c.structuralCursor = nil
	c.structuralCommit = nil
	c.pendingCommits = make(map[agentrun.DomainCommitStage]*session.DomainCommitIntent)
	c.lastCommitReceipts = make(map[agentrun.DomainCommitStage]*session.DomainCommitReceipt)
	c.inputCommit = nil
	c.pendingCompaction = nil
	c.pendingCompactionHealth = nil
	c.pendingCleanup = nil
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
	if !agentrun.ValidCycleIdentity(c.cycleIdentity) {
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
	if !structural && !agentrun.ValidCycleIdentity(identity) {
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
	if structural || agentrun.ValidCycleIdentity(identity) {
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
