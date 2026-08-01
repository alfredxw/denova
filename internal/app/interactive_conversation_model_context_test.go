package app

import (
	"context"
	agentcontext "denova/internal/agents/context"

	agents "denova/internal/agents"
)

func assembleAndCommitInteractiveContextForTest(conversation *interactiveConversation, originalMessage, userMessage string) ([]*agents.Message, error) {
	result, err := conversation.AssembleModelContext(context.Background(), originalMessage, agentcontext.ModelContextInput{
		UserMessage: userMessage,
		Budget:      conversation.ModelContextBudget(),
	})
	if err == nil {
		err = conversation.CommitModelInput(context.Background(), originalMessage, result)
	}
	return result.Messages, err
}
