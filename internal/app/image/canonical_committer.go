package imageapp

import (
	"context"

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
	return agentlifecycle.NewSessionConversationCommitter(agentlifecycle.SessionCommitterConfig{
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
}

var _ agentlifecycle.ConversationCommitterProvider = (*imageAgentConversation)(nil)
