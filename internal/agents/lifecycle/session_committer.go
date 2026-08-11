package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	agentchat "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"

	agent "github.com/alfredxw/denova/agent"
)

// SessionOutputCommit is the product-approved session projection of one raw
// Agent output. Message is the canonical product record; Transcript controls
// only future public Agent model context.
type SessionOutputCommit struct {
	Message    *agent.Message
	Metadata   session.MessageMetadata
	Transcript *agent.OutputProjection
}

type SessionOutputProjector func(
	context.Context,
	agentchat.AgentContextPreparation,
	agent.OutputCommitRequest,
	session.MessageMetadata,
) (SessionOutputCommit, error)

// SessionCommitterConfig wires a SessionConversation to the public canonical
// boundary without exposing session storage to the reusable Agent package.
type SessionCommitterConfig struct {
	Conversation *agentconversation.SessionConversation
	Session      *session.Session
	Options      agentrun.Options
	Request      agentchat.ChatRequest

	ProjectOutput  SessionOutputProjector
	ApplyEffects   func(context.Context, []agent.EffectRequest) ([]agent.EffectResult, error)
	InputCommitted func(context.Context) error
}

type sessionConversationCommitter struct {
	config         SessionCommitterConfig
	mu             sync.Mutex
	inputCallbacks map[string]struct{}
}

func NewSessionConversationCommitter(config SessionCommitterConfig) (ConversationCommitter, error) {
	if config.Conversation == nil || config.Session == nil {
		return nil, errors.New("Denova Session committer requires conversation and session")
	}
	config.Options = config.Options.Normalize(config.Options.Workspace)
	return &sessionConversationCommitter{config: config, inputCallbacks: make(map[string]struct{})}, nil
}

func (committer *sessionConversationCommitter) MaterializeInput(
	ctx context.Context,
	request agent.InputCommitRequest,
) (agent.CommitReceipt, error) {
	receipt, err := committer.config.Conversation.MaterializeAgentCanonicalInput(
		ctx,
		committer.config.Request.Message,
		agentchat.UserMessageReferences(committer.config.Request),
		request.Hash,
	)
	if err != nil {
		return agent.CommitReceipt{}, err
	}
	if committer.config.InputCommitted != nil {
		committer.mu.Lock()
		_, called := committer.inputCallbacks[request.Hash]
		committer.mu.Unlock()
		if !called {
			if err := committer.config.InputCommitted(ctx); err != nil {
				return agent.CommitReceipt{}, fmt.Errorf("run Denova input-commit callback: %w", err)
			}
			committer.mu.Lock()
			committer.inputCallbacks[request.Hash] = struct{}{}
			committer.mu.Unlock()
		}
	}
	return agent.CommitReceipt{Revision: strconv.FormatUint(receipt.ContextRevision, 10)}, nil
}

func (committer *sessionConversationCommitter) ApplyPreparedContext(
	_ context.Context,
	prepared agentchat.AgentContextPreparation,
) error {
	return committer.config.Conversation.ApplyAgentPreparedContext(prepared.ModelContext)
}

func (committer *sessionConversationCommitter) CommitOutput(
	ctx context.Context,
	prepared agentchat.AgentContextPreparation,
	request agent.OutputCommitRequest,
) (agent.OutputCommitReceipt, error) {
	options := committer.config.Options
	metadata := session.MessageMetadata{
		RunID: request.Identity.RunID, AgentKind: options.AgentKind,
		AgentName: options.RootAgentName, RootAgentName: options.RootAgentName,
	}
	if options.RootAgentName != "" {
		metadata.RunPath = []string{options.RootAgentName}
	}
	projection := SessionOutputCommit{Message: request.Message.Clone(), Metadata: metadata}
	var err error
	if committer.config.ProjectOutput != nil {
		projection, err = committer.config.ProjectOutput(ctx, prepared, request, metadata)
		if err != nil {
			return agent.OutputCommitReceipt{}, err
		}
	}
	if projection.Message == nil {
		return agent.OutputCommitReceipt{}, errors.New("Denova Session output projector returned no canonical message")
	}
	receipt, err := committer.config.Conversation.CommitAgentCanonicalOutput(
		ctx, projection.Message, projection.Metadata, request.Hash,
	)
	if err != nil {
		return agent.OutputCommitReceipt{}, err
	}
	if prepared.ResumeInterruption != nil {
		if err := committer.config.Conversation.ResolveInterruption(prepared.ResumeInterruption.ID); err != nil {
			return agent.OutputCommitReceipt{}, fmt.Errorf("resolve Denova interruption: %w", err)
		}
	}
	return agent.OutputCommitReceipt{
		Revision: strconv.FormatUint(receipt.ContextRevision, 10), Transcript: projection.Transcript,
	}, nil
}

func (committer *sessionConversationCommitter) Reconcile(
	_ context.Context,
	request agent.ReconcileRequest,
) (agent.ReconcileResult, error) {
	identity := session.DomainCommitIdentity{
		CommandID:   strings.TrimSpace(request.Identity.CommandID),
		OperationID: strings.TrimSpace(request.Identity.RunID),
		Cycle:       request.Identity.Cycle,
	}
	role := agent.User
	if request.Identity.Stage == agent.CommitOutput {
		role = agent.Assistant
	} else if request.Identity.Stage != agent.CommitInput {
		return agent.ReconcileResult{}, fmt.Errorf("unsupported Session canonical stage %q", request.Identity.Stage)
	}
	receipt, found, err := committer.config.Session.FindAgentCanonicalCommit(identity, role, request.Hash)
	if err != nil || !found {
		return agent.ReconcileResult{Found: found}, err
	}
	return agent.ReconcileResult{
		Found: true, Revision: strconv.FormatUint(receipt.ContextRevision, 10),
	}, nil
}

func (committer *sessionConversationCommitter) ApplyEffects(
	ctx context.Context,
	requests []agent.EffectRequest,
) ([]agent.EffectResult, error) {
	if len(requests) == 0 {
		return nil, nil
	}
	if committer.config.ApplyEffects == nil {
		return nil, errors.New("Denova Session has Tool effects but no effect applier")
	}
	return committer.config.ApplyEffects(ctx, requests)
}

var _ ConversationCommitter = (*sessionConversationCommitter)(nil)
