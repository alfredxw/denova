package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sync"

	agentchat "denova/internal/agents/chat"
	agentcontext "denova/internal/agents/context"
	agentrun "denova/internal/agents/run"
	"denova/internal/book"

	sdkcontext "github.com/alfredxw/denova/agent/context"
	agentcompaction "github.com/alfredxw/denova/agent/context/compaction"
	agentexecution "github.com/alfredxw/denova/agent/engine/execution"
	agentschema "github.com/alfredxw/denova/agent/schema"
	agentcanonical "github.com/alfredxw/denova/agent/session/canonical"
)

// ConversationCommitter keeps product persistence outside the reusable Agent
// package while the Boundary guarantees that canonical input and context use
// the same pure Denova preparation. Implementations must make each commit
// idempotent by the supplied identity and Agent hash.
type ConversationCommitter interface {
	// MaterializeInput must persist the accepted user input without assembling
	// model context. This keeps the canonical input fence ahead of every
	// expensive or failure-prone context source.
	MaterializeInput(context.Context, agentcanonical.InputCommitRequest) (agentcanonical.CommitReceipt, error)
	// ApplyPreparedContext publishes process-local context state after the
	// accepted input is durable. It must not append another user message.
	// Same-cycle recovery can reuse journaled context without calling it.
	ApplyPreparedContext(context.Context, agentchat.AgentContextPreparation) error
	CommitOutput(context.Context, agentchat.AgentContextPreparation, agentcanonical.OutputCommitRequest) (agentcanonical.OutputCommitReceipt, error)
}

// ConversationContextCommitter is implemented by product journals that own
// Agent's UI-hidden protocol messages as well as visible input/output.
type ConversationContextCommitter interface {
	CommitContext(context.Context, agentcanonical.ContextCommitRequest) (agentcanonical.CommitReceipt, error)
}

// ConversationCommitterProvider is implemented by Denova-owned conversation
// types whose product store cannot be imported by the generic execution host.
// It keeps game/lore persistence in the application package while exposing
// only the canonical lifecycle contract.
type ConversationCommitterProvider interface {
	NewAgentConversationCommitter(agentrun.Options) (ConversationCommitter, error)
}

// ConversationBoundaryConfig declares one exact Denova product turn. The
// caller owns app-specific persistence; the Boundary owns ordering, shared
// preparation, and the public ContextSource/CanonicalAdapter contracts.
type ConversationBoundaryConfig struct {
	Conversation agentchat.Conversation
	BookService  *book.Service
	Request      agentchat.ChatRequest
	Options      agentrun.Options

	ContextIdentity   agentschema.CapabilityIdentity
	CanonicalIdentity agentschema.CapabilityIdentity
	Committer         ConversationCommitter
	OnPrepared        func(agentchat.AgentContextPreparation)
	// BeforeCommitOutput optionally drains in-process display projections up to
	// the synchronous output boundary so intermediate step cards always precede
	// the canonical final message in the journal.
	BeforeCommitOutput func(context.Context) error
	// ProjectOutput may replace the provider-neutral final message while the
	// Boundary preserves the original Agent hash as the idempotency identity.
	ProjectOutput func(agentcanonical.OutputCommitRequest) (*agentschema.Message, *agentcanonical.OutputProjection)
}

type ConversationBoundary struct {
	config ConversationBoundaryConfig

	mu       sync.Mutex
	prepared *agentchat.AgentContextPreparation
	run      agentschema.RunView
	// contextRevision identifies the Agent-owned checkpoint used to assemble
	// prepared. A same-cycle checkpoint must invalidate host context while the
	// canonical output still reuses the newest successful preparation.
	contextRevision string
}

type conversationBoundaryContext struct{ boundary *ConversationBoundary }
type conversationBoundaryCanonical struct{ boundary *ConversationBoundary }
type conversationBoundaryCanonicalContext struct{ conversationBoundaryCanonical }

// NewConversationBoundary creates the paired ContextSource and
// CanonicalAdapter used by a Definition. Successful preparation is cached for
// this exact cycle; transient preparation failures remain retryable.
func NewConversationBoundary(config ConversationBoundaryConfig) (*ConversationBoundary, error) {
	if config.Conversation == nil {
		return nil, errors.New("Denova Conversation Boundary requires a conversation")
	}
	if config.Committer == nil {
		return nil, errors.New("Denova Conversation Boundary requires a product committer")
	}
	if err := validateBoundaryIdentity("Context", config.ContextIdentity); err != nil {
		return nil, err
	}
	if err := validateBoundaryIdentity("Canonical", config.CanonicalIdentity); err != nil {
		return nil, err
	}
	config.Request = agentchat.CaptureChatRequestCallerInput(config.Request)
	return &ConversationBoundary{config: config}, nil
}

func validateBoundaryIdentity(name string, identity agentschema.CapabilityIdentity) error {
	if identity.Kind == "" || identity.Version == 0 {
		return fmt.Errorf("Denova Conversation Boundary requires a stable %s identity", name)
	}
	return nil
}

// ContextSource and CanonicalAdapter are lightweight views over the same
// Boundary, so they can have distinct stable capability identities without
// preparing the product turn independently.
func (boundary *ConversationBoundary) ContextSource() sdkcontext.ContextSource {
	return conversationBoundaryContext{boundary: boundary}
}

func (boundary *ConversationBoundary) CanonicalAdapter() agentcanonical.CanonicalAdapter {
	base := conversationBoundaryCanonical{boundary: boundary}
	if _, ok := boundary.config.Committer.(ConversationContextCommitter); ok {
		return conversationBoundaryCanonicalContext{conversationBoundaryCanonical: base}
	}
	return base
}

func (source conversationBoundaryContext) Identity() agentschema.CapabilityIdentity {
	return source.boundary.config.ContextIdentity
}

func (source conversationBoundaryContext) Materialize(ctx context.Context, request sdkcontext.ContextRequest) ([]agentschema.ContextFragment, error) {
	if err := bindAgentCompaction(source.boundary.config.Conversation, request.Compaction); err != nil {
		return nil, err
	}
	return source.boundary.materializeContext(ctx, request, compactionContextRevision(request.Compaction))
}

func (adapter conversationBoundaryCanonical) Identity() agentschema.CapabilityIdentity {
	return adapter.boundary.config.CanonicalIdentity
}

func (adapter conversationBoundaryCanonical) MaterializeInput(ctx context.Context, request agentcanonical.InputCommitRequest) (agentcanonical.CommitReceipt, error) {
	return adapter.boundary.materializeInput(ctx, request)
}

func (adapter conversationBoundaryCanonical) CommitOutput(ctx context.Context, request agentcanonical.OutputCommitRequest) (agentcanonical.OutputCommitReceipt, error) {
	return adapter.boundary.commitOutput(ctx, request)
}

func (adapter conversationBoundaryCanonical) PendingOutput(ctx context.Context, identity agentcanonical.CommitIdentity) (*agentschema.Message, error) {
	run := agentschema.RunView{ID: identity.RunID, CommandID: identity.CommandID, Cycle: identity.Cycle}
	if err := bindConversationCycle(adapter.boundary.config.Conversation, adapter.boundary.config.Options.AgentKind, run); err != nil {
		return nil, err
	}
	if source, ok := adapter.boundary.config.Conversation.(agentcanonical.CanonicalPreparedOutput); ok {
		return source.PendingOutput(ctx, identity)
	}
	return nil, nil
}

func (adapter conversationBoundaryCanonicalContext) CommitContext(ctx context.Context, request agentcanonical.ContextCommitRequest) (agentcanonical.CommitReceipt, error) {
	return adapter.boundary.commitContext(ctx, request)
}

func (boundary *ConversationBoundary) materializeContext(
	ctx context.Context,
	request sdkcontext.ContextRequest,
	contextRevision string,
) ([]agentschema.ContextFragment, error) {
	prepared, err := boundary.prepare(ctx, request.Run, contextRevision)
	if err != nil {
		return nil, err
	}
	return projectConversationContext(
		prepared,
		request,
		agentcontext.ModelContextBudgetFor(boundary.config.Conversation),
	)
}

func (boundary *ConversationBoundary) materializeInput(ctx context.Context, request agentcanonical.InputCommitRequest) (agentcanonical.CommitReceipt, error) {
	if request.Identity.Stage != agentcanonical.CommitInput {
		return agentcanonical.CommitReceipt{}, errors.New("Denova Conversation Boundary received a non-input materialization")
	}
	run := agentschema.RunView{ID: request.Identity.RunID, CommandID: request.Identity.CommandID, Cycle: request.Identity.Cycle}
	if err := bindConversationCycle(boundary.config.Conversation, boundary.config.Options.AgentKind, run); err != nil {
		return agentcanonical.CommitReceipt{}, err
	}
	return boundary.config.Committer.MaterializeInput(ctx, request)
}

func (boundary *ConversationBoundary) commitOutput(ctx context.Context, request agentcanonical.OutputCommitRequest) (agentcanonical.OutputCommitReceipt, error) {
	if request.Identity.Stage != agentcanonical.CommitOutput {
		return agentcanonical.OutputCommitReceipt{}, errors.New("Denova Conversation Boundary received a non-output commit")
	}
	run := agentschema.RunView{ID: request.Identity.RunID, CommandID: request.Identity.CommandID, Cycle: request.Identity.Cycle}
	prepared, err := boundary.prepareOutput(run)
	if err != nil {
		return agentcanonical.OutputCommitReceipt{}, err
	}
	var transcript *agentcanonical.OutputProjection
	if boundary.config.ProjectOutput != nil {
		message, projectedTranscript := boundary.config.ProjectOutput(request)
		if message == nil {
			return agentcanonical.OutputCommitReceipt{}, errors.New("Denova Conversation Boundary output projector returned no canonical message")
		}
		request.Message = *message
		transcript = projectedTranscript
	}
	if boundary.config.BeforeCommitOutput != nil {
		if err := boundary.config.BeforeCommitOutput(ctx); err != nil {
			return agentcanonical.OutputCommitReceipt{}, err
		}
	}
	receipt, err := boundary.config.Committer.CommitOutput(ctx, prepared, request)
	if err != nil {
		return agentcanonical.OutputCommitReceipt{}, err
	}
	if receipt.Transcript == nil {
		receipt.Transcript = transcript
	}
	return receipt, nil
}

// A restored Agent cycle already owns its model context. Output persistence
// needs the accepted product route, not another read of mutable prompt sources.
func (boundary *ConversationBoundary) prepareOutput(run agentschema.RunView) (agentchat.AgentContextPreparation, error) {
	if err := bindConversationCycle(boundary.config.Conversation, boundary.config.Options.AgentKind, run); err != nil {
		return agentchat.AgentContextPreparation{}, err
	}
	boundary.mu.Lock()
	defer boundary.mu.Unlock()
	if boundary.prepared != nil {
		if boundary.run.ID != run.ID || boundary.run.CommandID != run.CommandID || boundary.run.Cycle != run.Cycle {
			return agentchat.AgentContextPreparation{}, errors.New("Denova Conversation Boundary was reused across Agent cycles")
		}
		return *boundary.prepared, nil
	}
	interruption, err := agentchat.ResolveRequestedInterruption(boundary.config.Request, boundary.config.Conversation.PendingInterruption())
	if err != nil {
		return agentchat.AgentContextPreparation{}, err
	}
	return agentchat.AgentContextPreparation{
		OriginalMessage: boundary.config.Request.Message, ResumeInterruption: interruption,
	}, nil
}

func (boundary *ConversationBoundary) commitContext(ctx context.Context, request agentcanonical.ContextCommitRequest) (agentcanonical.CommitReceipt, error) {
	if request.Identity.Stage != agentcanonical.CommitContext {
		return agentcanonical.CommitReceipt{}, errors.New("Denova Conversation Boundary received a non-context commit")
	}
	if err := agentcanonical.ValidateContextCommit(request); err != nil {
		return agentcanonical.CommitReceipt{}, err
	}
	run := agentschema.RunView{ID: request.Identity.RunID, CommandID: request.Identity.CommandID, Cycle: request.Identity.Cycle}
	if err := bindConversationCycle(boundary.config.Conversation, boundary.config.Options.AgentKind, run); err != nil {
		return agentcanonical.CommitReceipt{}, err
	}
	committer, ok := boundary.config.Committer.(ConversationContextCommitter)
	if !ok {
		return agentcanonical.CommitReceipt{}, agentschema.ErrCapabilityUnsupported
	}
	return committer.CommitContext(ctx, request)
}

func (boundary *ConversationBoundary) prepare(
	ctx context.Context,
	run agentschema.RunView,
	contextRevision string,
) (agentchat.AgentContextPreparation, error) {
	if boundary == nil || boundary.config.Conversation == nil {
		return agentchat.AgentContextPreparation{}, errors.New("Denova Conversation Boundary is unavailable")
	}
	if err := bindConversationCycle(boundary.config.Conversation, boundary.config.Options.AgentKind, run); err != nil {
		return agentchat.AgentContextPreparation{}, err
	}
	boundary.mu.Lock()
	defer boundary.mu.Unlock()
	if boundary.prepared != nil {
		// Delivery and Autonomous describe how this already-identified cycle was
		// admitted. Canonical commit identities intentionally need only the exact
		// Run/command/cycle tuple, so compare that durable identity rather than
		// requiring callers to reconstruct incidental preparation metadata.
		if boundary.run.ID != run.ID || boundary.run.CommandID != run.CommandID || boundary.run.Cycle != run.Cycle {
			return agentchat.AgentContextPreparation{}, errors.New("Denova Conversation Boundary was reused across Agent cycles")
		}
		if contextRevision == "" || boundary.contextRevision == contextRevision {
			return *boundary.prepared, nil
		}
	}
	prepared, err := agentchat.PrepareAgentContext(
		ctx,
		boundary.config.Conversation,
		boundary.config.Request,
		boundary.config.BookService,
		boundary.config.Options.Workspace,
		run.StartedAt,
	)
	if err != nil {
		return agentchat.AgentContextPreparation{}, err
	}
	if !agentexecution.IsInspection(ctx) {
		if err := boundary.config.Committer.ApplyPreparedContext(ctx, prepared); err != nil {
			return agentchat.AgentContextPreparation{}, fmt.Errorf("apply prepared Denova context: %w", err)
		}
	}
	boundary.prepared = &prepared
	boundary.run = run
	boundary.contextRevision = contextRevision
	if boundary.config.OnPrepared != nil && !agentexecution.IsInspection(ctx) {
		boundary.config.OnPrepared(prepared)
	}
	return prepared, nil
}

func compactionContextRevision(state *agentcompaction.CompactionState) string {
	if state == nil {
		return "none"
	}
	return fmt.Sprintf("%s:%d", state.ID, state.Revision)
}

var _ sdkcontext.ContextSource = conversationBoundaryContext{}
var _ agentcanonical.CanonicalAdapter = conversationBoundaryCanonical{}
var _ agentcanonical.CanonicalContextAdapter = conversationBoundaryCanonicalContext{}
