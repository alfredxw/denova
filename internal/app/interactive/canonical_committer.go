package interactiveapp

import (
	"context"
	"errors"

	agentchat "denova/internal/agents/chat"
	agentlifecycle "denova/internal/agents/lifecycle"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"

	agent "github.com/alfredxw/denova/agent"
)

type CanonicalCommitterConfig struct {
	Conversation *Conversation
	Options      agentrun.Options
}

type canonicalConversationCommitter struct{ config CanonicalCommitterConfig }

func NewCanonicalCommitter(config CanonicalCommitterConfig) (agentlifecycle.ConversationCommitter, error) {
	if config.Conversation == nil {
		return nil, errors.New("Denova Game committer requires a conversation")
	}
	config.Options = config.Options.Normalize(config.Options.Workspace)
	return &canonicalConversationCommitter{config: config}, nil
}

// NewAgentConversationCommitter exposes only the reusable lifecycle seam to
// the execution host; game turn and lore persistence remain app-owned.
func (conversation *Conversation) NewAgentConversationCommitter(
	options agentrun.Options,
) (agentlifecycle.ConversationCommitter, error) {
	return NewCanonicalCommitter(CanonicalCommitterConfig{
		Conversation: conversation, Options: options,
	})
}

func (committer *canonicalConversationCommitter) MaterializeInput(
	ctx context.Context,
	request agent.InputCommitRequest,
) (agent.CommitReceipt, error) {
	receipt, err := committer.config.Conversation.MaterializeAgentCanonicalInput(
		ctx,
		request.Input.Text,
		request.Input.Attachments,
	)
	if err != nil {
		return agent.CommitReceipt{}, err
	}
	return agent.CommitReceipt{Revision: receipt.Revision}, nil
}

func (committer *canonicalConversationCommitter) ApplyPreparedContext(
	ctx context.Context,
	prepared agentchat.AgentContextPreparation,
) error {
	return committer.config.Conversation.CommitModelInput(ctx, prepared.OriginalMessage, prepared.ModelContext)
}

func (committer *canonicalConversationCommitter) CommitContext(
	ctx context.Context,
	request agent.ContextCommitRequest,
) (agent.CommitReceipt, error) {
	revision, err := committer.config.Conversation.CommitAgentCanonicalContext(ctx, request)
	if err != nil {
		return agent.CommitReceipt{}, err
	}
	return agent.CommitReceipt{Revision: revision}, nil
}

func (committer *canonicalConversationCommitter) CommitOutput(
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
	receipt, err := committer.config.Conversation.CommitAgentCanonicalOutput(
		ctx, request.Message.Clone(), metadata,
	)
	if err != nil {
		return agent.OutputCommitReceipt{}, err
	}
	if prepared.ResumeInterruption != nil {
		if err := committer.config.Conversation.ResolveInterruption(prepared.ResumeInterruption.ID); err != nil {
			return agent.OutputCommitReceipt{}, err
		}
	}
	return agent.OutputCommitReceipt{
		Revision: receipt.Revision,
		Transcript: &agent.OutputProjection{
			Content: receipt.Turn.Narrative, Thinking: receipt.Turn.Thinking,
		},
	}, nil
}

var _ agentlifecycle.ConversationCommitter = (*canonicalConversationCommitter)(nil)
var _ agentlifecycle.ConversationContextCommitter = (*canonicalConversationCommitter)(nil)
