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
	ApplyEffects func(context.Context, []agent.EffectRequest) ([]agent.EffectResult, error)
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
	applyEffects agentlifecycle.ToolEffectApplier,
) (agentlifecycle.ConversationCommitter, error) {
	return NewCanonicalCommitter(CanonicalCommitterConfig{
		Conversation: conversation, Options: options, ApplyEffects: applyEffects,
	})
}

func (committer *canonicalConversationCommitter) MaterializeInput(
	ctx context.Context,
	request agent.InputCommitRequest,
) (agent.CommitReceipt, error) {
	receipt, err := committer.config.Conversation.MaterializeAgentCanonicalInput(ctx, request.Hash)
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

func (committer *canonicalConversationCommitter) CommitOutput(
	ctx context.Context,
	_ agentchat.AgentContextPreparation,
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
		ctx, request.Message.Clone(), metadata, request.Hash,
	)
	if err != nil {
		return agent.OutputCommitReceipt{}, err
	}
	return agent.OutputCommitReceipt{
		Revision: receipt.Revision,
		Transcript: &agent.OutputProjection{
			Content: receipt.Turn.Narrative, Thinking: receipt.Turn.Thinking,
		},
	}, nil
}

func (committer *canonicalConversationCommitter) ApplyEffects(
	ctx context.Context,
	requests []agent.EffectRequest,
) ([]agent.EffectResult, error) {
	if len(requests) == 0 {
		return nil, nil
	}
	if committer.config.ApplyEffects == nil {
		return nil, errors.New("Denova Game has Tool effects but no effect applier")
	}
	return committer.config.ApplyEffects(ctx, requests)
}

var _ agentlifecycle.ConversationCommitter = (*canonicalConversationCommitter)(nil)
