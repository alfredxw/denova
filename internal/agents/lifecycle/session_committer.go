package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strconv"
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

	ProjectOutput SessionOutputProjector
	InputEffect   agentrun.InputCommitEffect
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
		request.Input.Text,
		request.Input.Attachments,
		agentchat.UserMessageReferences(committer.config.Request),
	)
	if err != nil {
		return agent.CommitReceipt{}, err
	}
	if committer.config.InputEffect != nil {
		effectRequest := inputEffectRequest(request.Identity, request.Hash)
		committer.mu.Lock()
		_, called := committer.inputCallbacks[request.Hash]
		committer.mu.Unlock()
		if !called {
			if err := committer.config.InputEffect.Apply(ctx, effectRequest); err != nil {
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

func (committer *sessionConversationCommitter) CommitContext(
	ctx context.Context,
	request agent.ContextCommitRequest,
) (agent.CommitReceipt, error) {
	receipt, err := committer.config.Conversation.CommitAgentCanonicalContext(ctx, request)
	if err != nil {
		return agent.CommitReceipt{}, err
	}
	return agent.CommitReceipt{Revision: strconv.FormatUint(receipt.ContextRevision, 10)}, nil
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
	if prepared.ResumeInterruption != nil {
		metadata.ResolveInterruptionID = prepared.ResumeInterruption.ID
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
		ctx, projection.Message, projection.Metadata,
	)
	if err != nil {
		return agent.OutputCommitReceipt{}, err
	}
	return agent.OutputCommitReceipt{
		Revision: strconv.FormatUint(receipt.ContextRevision, 10), Transcript: projection.Transcript,
	}, nil
}

func inputEffectRequest(identity agent.CommitIdentity, hash string) agentrun.InputCommitEffectRequest {
	return agentrun.InputCommitEffectRequest{
		CommandID: identity.CommandID, OperationID: identity.RunID, Cycle: identity.Cycle, Hash: hash,
	}
}

var _ ConversationCommitter = (*sessionConversationCommitter)(nil)
