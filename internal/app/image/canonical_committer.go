package imageapp

import (
	"context"
	"errors"

	agentchat "denova/internal/agents/chat"
	agentlifecycle "denova/internal/agents/lifecycle"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"

	agent "github.com/alfredxw/denova/agent"
)

func (conversation *imageAgentConversation) NewAgentConversationCommitter(
	options agentrun.Options,
	applyEffects agentlifecycle.ToolEffectApplier,
) (agentlifecycle.ConversationCommitter, error) {
	delegate, err := agentlifecycle.NewSessionConversationCommitter(agentlifecycle.SessionCommitterConfig{
		Conversation: conversation.journal,
		Session:      conversation.journal.CanonicalSession(),
		Options:      options,
		Request:      agentchat.ChatRequest{Message: conversation.message},
		ApplyEffects: applyEffects,
		ProjectOutput: func(
			_ context.Context,
			_ agentchat.AgentContextPreparation,
			request agent.OutputCommitRequest,
			metadata session.MessageMetadata,
		) (agentlifecycle.SessionOutputCommit, error) {
			conversation.assistant = request.Message.Content
			return agentlifecycle.SessionOutputCommit{Message: request.Message.Clone(), Metadata: metadata}, nil
		},
	})
	if err != nil {
		return nil, err
	}
	return &imageAgentConversationCommitter{
		ConversationCommitter: delegate,
		conversation:          conversation,
	}, nil
}

// imageAgentConversationCommitter keeps canonical Session persistence in the
// shared committer while validating the stateless context assembled by the
// Image Agent conversation itself.
type imageAgentConversationCommitter struct {
	agentlifecycle.ConversationCommitter
	conversation *imageAgentConversation
}

func (committer *imageAgentConversationCommitter) ApplyPreparedContext(
	ctx context.Context,
	prepared agentchat.AgentContextPreparation,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if committer == nil || committer.conversation == nil {
		return errors.New("Image Agent context committer is unavailable")
	}
	state, ok := prepared.ModelContext.CommitState.(imageAgentContextCommitState)
	if !ok || state.conversation == nil || state.messageHash == "" {
		return errors.New("Image Agent context is missing commit state")
	}
	if state.conversation != committer.conversation || state.messageHash != imageAgentSemanticHash(committer.conversation.message) {
		return errors.New("Image Agent context commit state does not match the current request")
	}
	return nil
}

func (committer *imageAgentConversationCommitter) CommitContext(
	ctx context.Context,
	request agent.ContextCommitRequest,
) (agent.CommitReceipt, error) {
	delegate, ok := committer.ConversationCommitter.(agentlifecycle.ConversationContextCommitter)
	if !ok {
		return agent.CommitReceipt{}, agent.ErrCapabilityUnsupported
	}
	return delegate.CommitContext(ctx, request)
}

var _ agentlifecycle.ConversationCommitterProvider = (*imageAgentConversation)(nil)
var _ agentlifecycle.ConversationCommitter = (*imageAgentConversationCommitter)(nil)
var _ agentlifecycle.ConversationContextCommitter = (*imageAgentConversationCommitter)(nil)
