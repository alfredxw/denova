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

	agent "github.com/alfredxw/denova/agent"
)

// ConversationCommitter keeps product persistence outside the reusable Agent
// package while the Boundary guarantees that canonical input and context use
// the same pure Denova preparation. Implementations must make each commit
// idempotent by the supplied identity and Agent hash.
type ConversationCommitter interface {
	// MaterializeInput must persist the accepted user input without assembling
	// model context. This keeps the canonical input fence ahead of every
	// expensive or failure-prone context source.
	MaterializeInput(context.Context, agent.InputCommitRequest) (agent.CommitReceipt, error)
	// ApplyPreparedContext publishes process-local context state after the
	// accepted input is durable. It must not append another user message.
	ApplyPreparedContext(context.Context, agentchat.AgentContextPreparation) error
	CommitOutput(context.Context, agentchat.AgentContextPreparation, agent.OutputCommitRequest) (agent.OutputCommitReceipt, error)
	Reconcile(context.Context, agent.ReconcileRequest) (agent.ReconcileResult, error)
	ApplyEffects(context.Context, []agent.EffectRequest) ([]agent.EffectResult, error)
}

// ConversationCommitterProvider is implemented by Denova-owned conversation
// types whose product store cannot be imported by the generic execution host.
// It keeps game/lore persistence in the application package while exposing
// only the canonical lifecycle contract.
type ConversationCommitterProvider interface {
	NewAgentConversationCommitter(agentrun.Options, ToolEffectApplier) (ConversationCommitter, error)
}

// ConversationBoundaryConfig declares one exact Denova product turn. The
// caller owns app-specific persistence; the Boundary owns ordering, shared
// preparation, and the public ContextSource/CanonicalAdapter contracts.
type ConversationBoundaryConfig struct {
	Conversation agentchat.Conversation
	BookService  *book.Service
	Request      agentchat.ChatRequest
	Options      agentrun.Options

	ContextIdentity   agent.CapabilityIdentity
	CanonicalIdentity agent.CapabilityIdentity
	Committer         ConversationCommitter
	OnPrepared        func(agentchat.AgentContextPreparation)
}

type ConversationBoundary struct {
	config ConversationBoundaryConfig

	mu       sync.Mutex
	prepared *agentchat.AgentContextPreparation
	run      agent.RunView
}

type conversationBoundaryContext struct{ boundary *ConversationBoundary }
type conversationBoundaryCanonical struct{ boundary *ConversationBoundary }

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

func validateBoundaryIdentity(name string, identity agent.CapabilityIdentity) error {
	if identity.Kind == "" || identity.Version == 0 {
		return fmt.Errorf("Denova Conversation Boundary requires a stable %s identity", name)
	}
	return nil
}

// ContextSource and CanonicalAdapter are lightweight views over the same
// Boundary, so they can have distinct stable capability identities without
// preparing the product turn independently.
func (boundary *ConversationBoundary) ContextSource() agent.ContextSource {
	return conversationBoundaryContext{boundary: boundary}
}

func (boundary *ConversationBoundary) CanonicalAdapter() agent.CanonicalAdapter {
	return conversationBoundaryCanonical{boundary: boundary}
}

func (source conversationBoundaryContext) Identity() agent.CapabilityIdentity {
	return source.boundary.config.ContextIdentity
}

func (source conversationBoundaryContext) Materialize(ctx context.Context, request agent.ContextRequest) ([]agent.ContextFragment, error) {
	if err := bindAgentCompaction(source.boundary.config.Conversation, request.Compaction); err != nil {
		return nil, err
	}
	return source.boundary.materializeContext(ctx, request)
}

func (adapter conversationBoundaryCanonical) Identity() agent.CapabilityIdentity {
	return adapter.boundary.config.CanonicalIdentity
}

func (adapter conversationBoundaryCanonical) MaterializeInput(ctx context.Context, request agent.InputCommitRequest) (agent.CommitReceipt, error) {
	return adapter.boundary.materializeInput(ctx, request)
}

func (adapter conversationBoundaryCanonical) CommitOutput(ctx context.Context, request agent.OutputCommitRequest) (agent.OutputCommitReceipt, error) {
	return adapter.boundary.commitOutput(ctx, request)
}

func (adapter conversationBoundaryCanonical) Reconcile(ctx context.Context, request agent.ReconcileRequest) (agent.ReconcileResult, error) {
	return adapter.boundary.config.Committer.Reconcile(ctx, request)
}

func (adapter conversationBoundaryCanonical) ApplyEffects(ctx context.Context, requests []agent.EffectRequest) ([]agent.EffectResult, error) {
	return adapter.boundary.config.Committer.ApplyEffects(ctx, requests)
}

func (boundary *ConversationBoundary) materializeContext(ctx context.Context, request agent.ContextRequest) ([]agent.ContextFragment, error) {
	prepared, err := boundary.prepare(ctx, request.Run)
	if err != nil {
		return nil, err
	}
	return projectConversationContext(
		prepared,
		request,
		agentcontext.ModelContextBudgetFor(boundary.config.Conversation),
	)
}

func (boundary *ConversationBoundary) materializeInput(ctx context.Context, request agent.InputCommitRequest) (agent.CommitReceipt, error) {
	if request.Identity.Stage != agent.CommitInput {
		return agent.CommitReceipt{}, errors.New("Denova Conversation Boundary received a non-input materialization")
	}
	run := agent.RunView{ID: request.Identity.RunID, CommandID: request.Identity.CommandID, Cycle: request.Identity.Cycle}
	if err := bindConversationCycle(boundary.config.Conversation, boundary.config.Options.AgentKind, run); err != nil {
		return agent.CommitReceipt{}, err
	}
	return boundary.config.Committer.MaterializeInput(ctx, request)
}

func (boundary *ConversationBoundary) commitOutput(ctx context.Context, request agent.OutputCommitRequest) (agent.OutputCommitReceipt, error) {
	if request.Identity.Stage != agent.CommitOutput {
		return agent.OutputCommitReceipt{}, errors.New("Denova Conversation Boundary received a non-output commit")
	}
	run := agent.RunView{ID: request.Identity.RunID, CommandID: request.Identity.CommandID, Cycle: request.Identity.Cycle}
	prepared, err := boundary.prepare(ctx, run)
	if err != nil {
		return agent.OutputCommitReceipt{}, err
	}
	return boundary.config.Committer.CommitOutput(ctx, prepared, request)
}

func (boundary *ConversationBoundary) prepare(ctx context.Context, run agent.RunView) (agentchat.AgentContextPreparation, error) {
	if boundary == nil || boundary.config.Conversation == nil {
		return agentchat.AgentContextPreparation{}, errors.New("Denova Conversation Boundary is unavailable")
	}
	if err := bindConversationCycle(boundary.config.Conversation, boundary.config.Options.AgentKind, run); err != nil {
		return agentchat.AgentContextPreparation{}, err
	}
	boundary.mu.Lock()
	defer boundary.mu.Unlock()
	if boundary.prepared != nil {
		if boundary.run != run {
			return agentchat.AgentContextPreparation{}, errors.New("Denova Conversation Boundary was reused across Agent cycles")
		}
		return *boundary.prepared, nil
	}
	prepared, err := agentchat.PrepareAgentContext(
		ctx,
		boundary.config.Conversation,
		boundary.config.Request,
		boundary.config.BookService,
		boundary.config.Options.Workspace,
	)
	if err != nil {
		return agentchat.AgentContextPreparation{}, err
	}
	if err := boundary.config.Committer.ApplyPreparedContext(ctx, prepared); err != nil {
		return agentchat.AgentContextPreparation{}, fmt.Errorf("apply prepared Denova context: %w", err)
	}
	boundary.prepared = &prepared
	boundary.run = run
	if boundary.config.OnPrepared != nil {
		boundary.config.OnPrepared(prepared)
	}
	return prepared, nil
}

var _ agent.ContextSource = conversationBoundaryContext{}
var _ agent.CanonicalAdapter = conversationBoundaryCanonical{}
