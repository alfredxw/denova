package app

import (
	"context"

	"denova/internal/agent"
)

func assembleAndCommitInteractiveContextForTest(conversation *interactiveConversation, originalMessage, userMessage string) ([]*agent.Message, error) {
	result, err := conversation.AssembleModelContext(context.Background(), originalMessage, agent.ModelContextInput{
		UserMessage: userMessage,
		Budget:      conversation.ModelContextBudget(),
	})
	if err == nil {
		err = conversation.CommitModelInput(context.Background(), originalMessage, result)
	}
	return result.Messages, err
}
